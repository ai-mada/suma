package platform

import (
	"context"
	"database/sql"
	"sort"
)

func sortedAuthorityIDs(ids []string) []string {
	set := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	ids = make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *Service) authorityOrganizations(ctx context.Context, tx *sql.Tx, user string, extra ...string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT tenant_id FROM suma_org_members WHERE user_id=? UNION SELECT tenant_id FROM suma_sessions WHERE user_id=?`), user, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := append([]string{}, extra...)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return sortedAuthorityIDs(ids), rows.Err()
}

func (s *Service) currentAuthorityOrganizations(ctx context.Context, tx *sql.Tx, user string) ([]string, error) {
	var ids []string
	for _, query := range []string{`SELECT tenant_id FROM suma_org_members WHERE user_id=? ORDER BY tenant_id`, `SELECT tenant_id FROM suma_sessions WHERE user_id=? ORDER BY token_hash`} {
		rows, err := tx.QueryContext(ctx, s.q(s.currentSQL(query)), user)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return sortedAuthorityIDs(ids), nil
}

func (s *Service) lockAuthorityRow(ctx context.Context, tx *sql.Tx, table, column, key string) error {
	query := `INSERT INTO ` + table + `(` + column + `,sequence) VALUES(?,0) ON CONFLICT(` + column + `) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT IGNORE INTO ` + table + `(` + column + `,sequence) VALUES(?,0)`
	}
	if _, err := tx.ExecContext(ctx, s.q(query), key); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`UPDATE `+table+` SET sequence=sequence+1 WHERE `+column+`=?`), key)
	return err
}

// Every lifecycle and human command takes the complete sorted organization
// set, then account coordination, then stable credential keys. A changed set
// rolls back; adding a smaller organization after account locks is forbidden.
func (s *Service) lockHumanAuthority(ctx context.Context, tx *sql.Tx, user string, organizations, keys []string) error {
	return s.lockHumanAuthorities(ctx, tx, []string{user}, organizations, keys)
}

func (s *Service) lockHumanAuthorities(ctx context.Context, tx *sql.Tx, users, organizations, keys []string) error {
	organizations = sortedAuthorityIDs(organizations)
	for _, org := range organizations {
		if err := s.lockOrganization(ctx, tx, org); err != nil {
			return err
		}
	}
	allowed := map[string]bool{}
	for _, org := range organizations {
		allowed[org] = true
	}
	users = sortedAuthorityIDs(users)
	for _, user := range users {
		if err := s.lockAuthorityRow(ctx, tx, "suma_identity_authority_locks", "user_id", user); err != nil {
			return err
		}
	}
	for _, user := range users {
		current, err := s.currentAuthorityOrganizations(ctx, tx, user)
		if err != nil {
			return err
		}
		for _, org := range current {
			if !allowed[org] {
				return &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
			}
		}
	}
	// Enumerate every participant's credential set while account coordination
	// is held, before taking the first stable source/family/slot/credential key.
	for _, user := range users {
		rows, err := tx.QueryContext(ctx, s.q(s.currentSQL(`SELECT a.token_hash,a.source_id,a.family_id,a.grant_id,a.slot_id FROM suma_credential_authority a JOIN suma_sessions t ON t.token_hash=a.token_hash WHERE t.user_id=? ORDER BY a.token_hash`)), user)
		if err != nil {
			return err
		}
		for rows.Next() {
			var ref credentialRef
			if err = rows.Scan(&ref.digest, &ref.sourceID, &ref.familyID, &ref.grantID, &ref.slotID); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, credentialLockKeys(ref)...)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	for _, key := range sortedAuthorityIDs(keys) {
		if err := s.lockAuthorityRow(ctx, tx, "suma_authority_credential_locks", "lock_key", key); err != nil {
			return err
		}
	}
	return nil
}

func credentialLockKeys(ref credentialRef) []string {
	var keys []string
	for _, entry := range []struct{ prefix, value string }{{"source:", ref.sourceID}, {"family:", ref.familyID}, {"slot:", ref.slotID}, {"credential:", ref.digest}, {"grant:", ref.grantID}} {
		if entry.value != "" {
			keys = append(keys, entry.prefix+entry.value)
		}
	}
	return keys
}

func (s *Service) lockOrganization(ctx context.Context, tx *sql.Tx, tenant string) error {
	// UPDATE acquires a writer/row lock on every supported SQL backend. All
	// governance writes take it before reading roles, versions or last owners.
	// The initial setup may have created an organization after migrations ran.
	// Conflict-aware insertion also handles two processes' first governance
	// operation without a read-then-insert uniqueness race on PostgreSQL.
	insert := `INSERT INTO suma_governance_locks(tenant_id,sequence) VALUES(?,0) ON CONFLICT(tenant_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		insert = `INSERT IGNORE INTO suma_governance_locks(tenant_id,sequence) VALUES(?,0)`
	}
	_, err := tx.ExecContext(ctx, s.q(insert), tenant)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_governance_locks SET sequence=sequence+1 WHERE tenant_id=?`), tenant)
	if err == nil {
		s.authorityBudgetMu.Lock()
		if value, ok := s.authorityBudgets.Load(tx); ok {
			value.(*authorityTxBudget).lockedOrganizations[tenant] = true
		}
		s.authorityBudgetMu.Unlock()
	}
	return err
}
