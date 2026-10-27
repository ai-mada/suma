package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/localization"
)

// LocalePreferenceUpdateRequest changes only presentation defaults. The HTTP
// adapter requires all fields explicitly, including expected_version zero.
type LocalePreferenceUpdateRequest struct {
	Locale          string `json:"locale"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type AccountPreferences struct {
	UserID    string `json:"user_id"`
	Locale    string `json:"locale"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

type OrganizationPreferences struct {
	OrganizationID string `json:"organization_id"`
	Locale         string `json:"locale"`
	Version        int    `json:"version"`
	UpdatedAt      string `json:"updated_at"`
}

type localePreferenceState struct {
	Locale    string `json:"locale"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

// Browser cookies are shared by tabs. Expected scope binds the operator's
// original view to the current principal without adding scope to the JSON body
// or allowing the client to choose a different authenticated organization.
type localePreferenceExpectation struct {
	users         []string
	organizations []string
}

var localePreferenceScopeID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validateLocalePreferenceExpectation(p Principal, organization bool, expected localePreferenceExpectation) error {
	if len(expected.users) != 1 || !localePreferenceScopeID.MatchString(expected.users[0]) || organization && (len(expected.organizations) != 1 || !localePreferenceScopeID.MatchString(expected.organizations[0])) || !organization && len(expected.organizations) != 0 {
		return invalid("语言偏好写入需要唯一且有效的预期账户与组织范围标头")
	}
	if expected.users[0] != p.ID || organization && expected.organizations[0] != p.TenantID {
		return &APIError{409, "preference_scope_conflict", "当前账户或组织已变化，请核对原请求范围后重试", nil}
	}
	return nil
}

// Independent tables leave existing users, organizations and all immutable
// evidence untouched. An absent row means inherit at version zero; clearing an
// existing value keeps its version, so stale creation cannot restore it.
func (s *Service) InitializeLocalePreferences(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_account_preferences(user_id VARCHAR(64) PRIMARY KEY,locale VARCHAR(20) NOT NULL,version INTEGER NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_organization_preferences(tenant_id VARCHAR(64) PRIMARY KEY,locale VARCHAR(20) NOT NULL,version INTEGER NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_account_preference_locks(user_id VARCHAR(64) PRIMARY KEY,sequence BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_locale_preference_operations(scope_kind VARCHAR(20) NOT NULL,scope_id VARCHAR(64) NOT NULL,actor_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,response TEXT NOT NULL,PRIMARY KEY(scope_kind,scope_id,actor_id,operation_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return nil
}

func validateLocalePreference(request LocalePreferenceUpdateRequest) (string, error) {
	if request.ExpectedVersion < 0 || request.ExpectedVersion > 2147483646 {
		return "", invalid("语言偏好的 expected_version 必须为 0–2147483646 的整数")
	}
	if len(request.IdempotencyKey) < 8 || len(request.IdempotencyKey) > 128 || !utf8.ValidString(request.IdempotencyKey) || strings.TrimSpace(request.IdempotencyKey) == "" || strings.IndexFunc(request.IdempotencyKey, unicode.IsControl) >= 0 {
		return "", invalid("语言偏好需要 8–128 字节且不含控制字符的 idempotency_key")
	}
	if request.Locale == "" {
		return "", nil
	}
	locale, ok := localization.Normalize(request.Locale)
	if !ok {
		return "", invalid("语言偏好必须为支持的语言标签，或使用空字符串继承")
	}
	return string(locale), nil
}

func preferenceTable(organization bool) (string, string) {
	if organization {
		return "suma_organization_preferences", "tenant_id"
	}
	return "suma_account_preferences", "user_id"
}

func (s *Service) readLocalePreference(ctx context.Context, q governanceQuery, organization bool, id string) (localePreferenceState, error) {
	var out localePreferenceState
	table, column := preferenceTable(organization)
	err := q.QueryRowContext(ctx, s.q(`SELECT locale,version,updated_at FROM `+table+` WHERE `+column+`=?`), id).Scan(&out.Locale, &out.Version, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// Preferences use the same current browser credential contract as business
// commands. The account row remains global across organization switches.
func (s *Service) localePreferences(ctx context.Context, actor Actor, organization bool) (localePreferenceState, error) {
	var out localePreferenceState
	p := actor.Projection()
	purpose := purposeAccountPreferences
	if organization {
		purpose = purposeOrganizationPreferences
	}
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purpose, func(auth *authorizedCommand) error {
		id := auth.currentPrincipal.ID
		if organization {
			id = auth.currentPrincipal.TenantID
		}
		var err error
		out, err = s.readLocalePreference(auth.ctx, auth.tx, organization, id)
		return err
	})
	return out, err
}

func (s *Service) AccountPreferences(ctx context.Context, actor Actor) (AccountPreferences, error) {
	out, err := s.localePreferences(ctx, actor, false)
	return AccountPreferences{actor.Projection().ID, out.Locale, out.Version, out.UpdatedAt}, err
}

func (s *Service) OrganizationPreferences(ctx context.Context, actor Actor) (OrganizationPreferences, error) {
	out, err := s.localePreferences(ctx, actor, true)
	return OrganizationPreferences{actor.Projection().TenantID, out.Locale, out.Version, out.UpdatedAt}, err
}

func (s *Service) UpdateAccountPreferences(ctx context.Context, actor Actor, request LocalePreferenceUpdateRequest) (AccountPreferences, error) {
	p := actor.Projection()
	out, err := s.updateLocalePreference(ctx, actor, request, false, localePreferenceExpectation{users: []string{p.ID}})
	return AccountPreferences{p.ID, out.Locale, out.Version, out.UpdatedAt}, err
}

func (s *Service) UpdateOrganizationPreferences(ctx context.Context, actor Actor, request LocalePreferenceUpdateRequest) (OrganizationPreferences, error) {
	p := actor.Projection()
	out, err := s.updateLocalePreference(ctx, actor, request, true, localePreferenceExpectation{users: []string{p.ID}, organizations: []string{p.TenantID}})
	return OrganizationPreferences{p.TenantID, out.Locale, out.Version, out.UpdatedAt}, err
}

func (s *Service) updateLocalePreference(ctx context.Context, actor Actor, request LocalePreferenceUpdateRequest, organization bool, expected localePreferenceExpectation) (localePreferenceState, error) {
	var out localePreferenceState
	locale, err := validateLocalePreference(request)
	if err != nil {
		return out, err
	}
	ctx, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cleanup()
	defer tx.Rollback()
	purpose := purposeAccountPreferences
	if organization {
		purpose = purposeOrganizationPreferences
	}
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.Projection().TenantID, projectID: organizationScope}, purpose)
	if err != nil {
		return out, err
	}
	ctx = auth.ctx
	p := auth.currentPrincipal
	// All organization, participant and source locks are already held by the
	// proof. The account preference row is additionally serialized globally.
	if !organization {
		insert := `INSERT INTO suma_account_preference_locks(user_id,sequence) VALUES(?,0) ON CONFLICT(user_id) DO NOTHING`
		if s.Dialect == "oceanbase" {
			insert = `INSERT IGNORE INTO suma_account_preference_locks(user_id,sequence) VALUES(?,0)`
		}
		if _, err = tx.ExecContext(ctx, s.q(insert), p.ID); err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_account_preference_locks SET sequence=sequence+1 WHERE user_id=?`), p.ID); err != nil {
			return out, err
		}
	}
	if organization && !orgAdmin(p) {
		return out, forbidden()
	}
	if err = validateLocalePreferenceExpectation(p, organization, expected); err != nil {
		return out, err
	}
	scope, id, action, title := "account", p.ID, "account.preferences.update", "更新账户语言偏好"
	if organization {
		scope, id, action, title = "organization", p.TenantID, "organization.preferences.update", "更新组织默认语言"
	}
	// Receipt identity is the account globally, or the current organization,
	// plus the real actor. The exact submitted locale binds the request hash;
	// presentation headers and canonicalization never alter that identity.
	hash := digest(p.ID + stringify(map[string]any{"scope": scope, "scope_id": id, "request": request}))
	var stored, response string
	err = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_locale_preference_operations WHERE scope_kind=? AND scope_id=? AND actor_id=? AND operation_id=?`), scope, id, p.ID, request.IdempotencyKey).Scan(&stored, &response)
	if err == nil {
		if stored != hash {
			return out, &APIError{409, "idempotency_conflict", "语言偏好操作标识已用于其他内容", nil}
		}
		err = json.Unmarshal([]byte(response), &out)
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	current, err := s.readLocalePreference(ctx, tx, organization, id)
	if err != nil {
		return out, err
	}
	if current.Version != request.ExpectedVersion {
		return out, conflict("语言偏好已变化，请刷新后重试")
	}
	out = localePreferenceState{locale, current.Version + 1, now()}
	table, column := preferenceTable(organization)
	if current.Version == 0 {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO `+table+`(`+column+`,locale,version,updated_at) VALUES(?,?,?,?)`), id, out.Locale, out.Version, out.UpdatedAt)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, s.q(`UPDATE `+table+` SET locale=?,version=?,updated_at=? WHERE `+column+`=? AND version=?`), out.Locale, out.Version, out.UpdatedAt, id, current.Version)
		if err == nil {
			var count int64
			count, err = result.RowsAffected()
			if err == nil && count != 1 {
				return localePreferenceState{}, conflict("语言偏好已变化，请刷新后重试")
			}
		}
	}
	if err != nil {
		return localePreferenceState{}, err
	}
	if err = s.commandActivity(ctx, auth, action, id, title); err != nil {
		return localePreferenceState{}, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_locale_preference_operations(scope_kind,scope_id,actor_id,operation_id,request_hash,response) VALUES(?,?,?,?,?,?)`), scope, id, p.ID, request.IdempotencyKey, hash, stringify(out)); err != nil {
		return localePreferenceState{}, err
	}
	return out, tx.Commit()
}
