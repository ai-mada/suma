package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
)

const organizationScope = "@organization"

// Governance uses independent version tables so existing installations and
// existing project/member schemas retain their data and wire compatibility.
func (s *Service) MigrateGovernance(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_governance_locks(tenant_id VARCHAR(64) PRIMARY KEY,sequence BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_org_member_versions(tenant_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,PRIMARY KEY(tenant_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_project_grant_versions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,PRIMARY KEY(tenant_id,project_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS suma_invitations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,email VARCHAR(254) NOT NULL,role VARCHAR(30) NOT NULL,grants TEXT NOT NULL,token_hash VARCHAR(64) NOT NULL UNIQUE,status VARCHAR(30) NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,accepted_by VARCHAR(64) NOT NULL,accepted_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_teams(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,name VARCHAR(200) NOT NULL,description TEXT NOT NULL,status VARCHAR(30) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_team_members(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,team_id VARCHAR(64) NOT NULL,resource_kind VARCHAR(20) NOT NULL,resource_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,role VARCHAR(30) NOT NULL,version INTEGER NOT NULL,status VARCHAR(30) NOT NULL,UNIQUE(tenant_id,team_id,resource_kind,resource_id,project_id))`,
		`INSERT INTO suma_governance_locks(tenant_id,sequence) SELECT id,0 FROM suma_organizations o WHERE NOT EXISTS(SELECT 1 FROM suma_governance_locks l WHERE l.tenant_id=o.id)`,
	}
	for _, statement := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return nil
}

type governanceQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) liveOrganization(ctx context.Context, q governanceQuery, p Principal) (Principal, error) {
	err := q.QueryRowContext(ctx, s.q(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE m.tenant_id=? AND m.user_id=?`), p.TenantID, p.ID).Scan(&p.ID, &p.Name, &p.Email, &p.TenantID, &p.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, forbidden()
	}
	return p, err
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
	return err
}

func orgAdmin(p Principal) bool     { return p.Role == "owner" || p.Role == "admin" }
func validOrgRole(role string) bool { return role == "owner" || role == "admin" || role == "member" }
func validProjectRole(role string) bool {
	return role == "admin" || role == "reviewer" || role == "developer" || role == "viewer"
}

// Authority is checked before receipts: revoked callers cannot replay privileged
// responses. The receipt binds the actor, route, target and complete request.
func (s *Service) governanceWrite(ctx context.Context, p Principal, action, target string, m map[string]any, authorize func(*sql.Tx, Principal) error, mutate func(*sql.Tx, Principal) (map[string]any, error)) (map[string]any, error) {
	if raw, ok := m["expected_version"]; ok {
		v, ok := raw.(float64)
		if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > math.MaxInt32 || math.Trunc(v) != v {
			return nil, invalid("expected_version 必须为非负整数")
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, p.TenantID); err != nil {
		return nil, err
	}
	p, err = s.liveOrganization(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	if authorize != nil {
		if err = authorize(tx, p); err != nil {
			return nil, err
		}
	}
	body := map[string]any{"action": action, "target": target, "body": m}
	hash, replay, err := s.receipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), body)
	if err != nil {
		return nil, err
	}
	if replay {
		var out map[string]any
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err := mutate(tx, p)
	if err != nil {
		return nil, err
	}
	if err = s.activity(ctx, tx, p, organizationScope, action, target, action); err != nil {
		return nil, err
	}
	if err = s.saveReceipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), hash, out); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func requireOrgAdmin(_ *sql.Tx, p Principal) error {
	if !orgAdmin(p) {
		return forbidden()
	}
	return nil
}

func (s *Service) Organizations(ctx context.Context, p Principal) ([]map[string]any, error) {
	if _, err := s.liveOrganization(ctx, s.DB, p); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT o.id,o.name,m.role FROM suma_organizations o JOIN suma_org_members m ON m.tenant_id=o.id WHERE m.user_id=? ORDER BY o.created_at,o.id`), p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, role string
		if err = rows.Scan(&id, &name, &role); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "role": role})
	}
	return out, rows.Err()
}

func (s *Service) CreateOrganization(ctx context.Context, p Principal, m map[string]any) (map[string]any, error) {
	name := strings.TrimSpace(str(m, "name"))
	if name == "" || len(name) > 200 {
		return nil, invalid("组织名称长度应为 1–200 字节")
	}
	return s.governanceWrite(ctx, p, "organization.create", "", m, nil, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		id := newID()
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_organizations(id,name,created_at) VALUES(?,?,?)`), id, name, now()); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,?)`), id, p.ID, "owner"); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_org_member_versions(tenant_id,user_id,version) VALUES(?,?,1)`), id, p.ID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_governance_locks(tenant_id,sequence) VALUES(?,0)`), id); err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "name": name, "role": "owner"}, nil
	})
}

func (s *Service) OrganizationMembers(ctx context.Context, p Principal) ([]map[string]any, error) {
	if _, err := s.liveOrganization(ctx, s.DB, p); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT u.id,u.name,u.email,m.role,COALESCE(v.version,1) FROM suma_org_members m JOIN suma_users u ON u.id=m.user_id LEFT JOIN suma_org_member_versions v ON v.tenant_id=m.tenant_id AND v.user_id=m.user_id WHERE m.tenant_id=? ORDER BY u.name,u.id`), p.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, email, role string
		var version int
		if err = rows.Scan(&id, &name, &email, &role, &version); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"user_id": id, "name": name, "email": email, "role": role, "version": version})
	}
	return out, rows.Err()
}

func (s *Service) orgMemberVersion(ctx context.Context, tx *sql.Tx, tenant, user string) (int, error) {
	var version int
	err := tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_org_member_versions WHERE tenant_id=? AND user_id=?`), tenant, user).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		var n int
		err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_org_members WHERE tenant_id=? AND user_id=?`), tenant, user).Scan(&n)
		return n, err
	}
	return version, err
}

func (s *Service) setOrgMemberVersion(ctx context.Context, tx *sql.Tx, tenant, user string, version int) error {
	r, err := tx.ExecContext(ctx, s.q(`UPDATE suma_org_member_versions SET version=? WHERE tenant_id=? AND user_id=?`), version, tenant, user)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_org_member_versions(tenant_id,user_id,version) VALUES(?,?,?)`), tenant, user, version)
	}
	return err
}

func (s *Service) UpdateOrganizationMember(ctx context.Context, p Principal, user string, m map[string]any) (map[string]any, error) {
	return s.governanceWrite(ctx, p, "organization.member.update", user, m, requireOrgAdmin, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		var old string
		err := tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_org_members WHERE tenant_id=? AND user_id=?`), p.TenantID, user).Scan(&old)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, missing()
		}
		if err != nil {
			return nil, err
		}
		action, role := str(m, "action"), str(m, "role")
		if action != "" && action != "remove" && action != "transfer_owner" {
			return nil, invalid("无效组织成员操作")
		}
		if old == "owner" && p.Role != "owner" {
			return nil, forbidden()
		}
		if role == "owner" && (p.Role != "owner" || action != "transfer_owner") {
			return nil, forbidden()
		}
		if action == "transfer_owner" && (p.Role != "owner" || role != "owner") {
			return nil, forbidden()
		}
		if action != "remove" && !validOrgRole(role) {
			return nil, invalid("组织角色应为 owner、admin 或 member")
		}
		version, err := s.orgMemberVersion(ctx, tx, p.TenantID, user)
		if err != nil {
			return nil, err
		}
		if number(m, "expected_version") != version {
			return nil, conflict("组织成员已更新，请刷新后重试")
		}
		if old == "owner" && (action == "remove" || role != "owner") {
			var owners int
			if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_org_members WHERE tenant_id=? AND role='owner'`), p.TenantID).Scan(&owners); err != nil {
				return nil, err
			}
			if owners <= 1 {
				return nil, conflict("必须保留至少一名组织所有者")
			}
		}
		if action == "remove" {
			if err = s.revokeOrganizationProjectGrants(ctx, tx, p.TenantID, user); err != nil {
				return nil, err
			}
			queries := []struct {
				q    string
				args []any
			}{
				{`UPDATE suma_invitations SET status='revoked',version=version+1 WHERE tenant_id=? AND created_by=? AND status='pending'`, []any{p.TenantID, user}},
				{`DELETE FROM suma_token_scopes WHERE token_hash IN (SELECT token_hash FROM suma_sessions WHERE tenant_id=? AND user_id=?)`, []any{p.TenantID, user}},
				{`DELETE FROM suma_sessions WHERE tenant_id=? AND user_id=?`, []any{p.TenantID, user}},
				{`UPDATE suma_team_members SET status='removed',version=version+1 WHERE tenant_id=? AND resource_kind='human' AND resource_id=? AND status='active'`, []any{p.TenantID, user}},
				{`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, []any{p.TenantID, user}},
			}
			for _, q := range queries {
				if _, err = tx.ExecContext(ctx, s.q(q.q), q.args...); err != nil {
					return nil, err
				}
			}
		} else {
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_org_members SET role=? WHERE tenant_id=? AND user_id=?`), role, p.TenantID, user); err != nil {
				return nil, err
			}
		}
		if err = s.setOrgMemberVersion(ctx, tx, p.TenantID, user, version+1); err != nil {
			return nil, err
		}
		out := map[string]any{"user_id": user, "role": role, "version": version + 1, "status": "active"}
		if action == "remove" {
			out["status"] = "removed"
		}
		return out, nil
	})
}

// governance is dispatched before normal project authorization. The outer HTTP
// handler rejects all scoped credentials before reaching these methods.
func (s *Server) governance(w http.ResponseWriter, r *http.Request, p Principal, path string, m map[string]any) bool {
	parts := strings.Split(path, "/")
	ctx := r.Context()
	switch parts[0] {
	case "organizations", "organization", "teams", "project-access":
	default:
		return false
	}
	switch {
	case path == "organizations" && r.Method == "GET":
		items, err := s.Service.Organizations(ctx, p)
		respondList(w, items, err)
	case path == "organizations" && r.Method == "POST":
		out, err := s.Service.CreateOrganization(ctx, p, m)
		respond(w, 201, out, err)
	case path == "organization/members" && r.Method == "GET":
		items, err := s.Service.OrganizationMembers(ctx, p)
		respondList(w, items, err)
	case len(parts) == 3 && parts[0] == "organization" && parts[1] == "members" && r.Method == "PATCH":
		out, err := s.Service.UpdateOrganizationMember(ctx, p, parts[2], m)
		respond(w, 200, out, err)
	case path == "organization/invitations" && r.Method == "GET":
		items, err := s.Service.Invitations(ctx, p)
		respondList(w, items, err)
	case path == "organization/invitations" && r.Method == "POST":
		out, err := s.Service.CreateInvitation(ctx, p, m)
		respond(w, 201, out, err)
	case len(parts) == 4 && parts[0] == "organization" && parts[1] == "invitations" && parts[3] == "revoke" && r.Method == "POST":
		out, err := s.Service.RevokeInvitation(ctx, p, parts[2], m)
		respond(w, 200, out, err)
	case path == "teams" && r.Method == "GET":
		items, err := s.Service.Teams(ctx, p)
		respondList(w, items, err)
	case path == "teams" && r.Method == "POST":
		out, err := s.Service.SaveTeam(ctx, p, "", m)
		respond(w, 201, out, err)
	case len(parts) == 2 && parts[0] == "teams" && r.Method == "PATCH":
		out, err := s.Service.SaveTeam(ctx, p, parts[1], m)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[0] == "teams" && parts[2] == "members" && r.Method == "GET":
		items, err := s.Service.TeamMembers(ctx, p, parts[1])
		respondList(w, items, err)
	case len(parts) == 3 && parts[0] == "teams" && parts[2] == "members" && r.Method == "POST":
		out, err := s.Service.SaveTeamMember(ctx, p, parts[1], m)
		respond(w, 200, out, err)
	case path == "project-access" && r.Method == "GET":
		items, err := s.Service.ProjectAccess(ctx, p, r.URL.Query().Get("project_id"))
		respondList(w, items, err)
	case path == "project-access" && r.Method == "POST":
		out, err := s.Service.SaveProjectAccess(ctx, p, m)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
