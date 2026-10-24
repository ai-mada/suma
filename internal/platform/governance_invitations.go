package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"
)

type invitationGrant struct {
	ProjectID string `json:"project_id"`
	Role      string `json:"role"`
}
type governanceInvitation struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	Version    int               `json:"version"`
	Email      string            `json:"email"`
	Role       string            `json:"role"`
	Grants     []invitationGrant `json:"grants"`
	Status     string            `json:"status"`
	CreatedBy  string            `json:"created_by"`
	CreatedAt  string            `json:"created_at"`
	ExpiresAt  string            `json:"expires_at"`
	AcceptedBy string            `json:"accepted_by,omitempty"`
	AcceptedAt string            `json:"accepted_at,omitempty"`
}

const invitationColumns = `id,tenant_id,version,email,role,grants,status,created_by,created_at,expires_at,accepted_by,accepted_at`

func scanInvitation(row scanner) (governanceInvitation, error) {
	var item governanceInvitation
	var grants string
	err := row.Scan(&item.ID, &item.TenantID, &item.Version, &item.Email, &item.Role, &grants, &item.Status, &item.CreatedBy, &item.CreatedAt, &item.ExpiresAt, &item.AcceptedBy, &item.AcceptedAt)
	item.Grants = []invitationGrant{}
	if err == nil {
		err = json.Unmarshal([]byte(grants), &item.Grants)
	}
	return item, err
}

func invitationMap(item governanceInvitation) map[string]any {
	var out map[string]any
	_ = json.Unmarshal([]byte(stringify(item)), &out)
	if item.Status == "pending" {
		expiry, err := time.Parse(time.RFC3339Nano, item.ExpiresAt)
		if err != nil || !time.Now().Before(expiry) {
			out["status"] = "expired"
		}
	}
	return out
}

func invitationEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return "", invalid("请输入不含显示名称的有效邮箱")
	}
	return email, nil
}

func (s *Service) validateInvitationGrants(ctx context.Context, tx *sql.Tx, tenant string, grants []invitationGrant) error {
	if len(grants) > 100 {
		return invalid("单次邀请最多包含 100 个项目授权")
	}
	seen := map[string]bool{}
	for _, grant := range grants {
		if !validProjectRole(grant.Role) || grant.ProjectID == "" || seen[grant.ProjectID] {
			return invalid("邀请项目授权角色无效或项目重复")
		}
		seen[grant.ProjectID] = true
		var actual string
		err := tx.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_projects WHERE id=?`), grant.ProjectID).Scan(&actual)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && actual != tenant) {
			return invalid("邀请授权项目不属于当前组织")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Invitations(ctx context.Context, p Principal) ([]map[string]any, error) {
	p, err := s.liveOrganization(ctx, s.DB, p)
	if err != nil {
		return nil, err
	}
	if !orgAdmin(p) {
		return nil, forbidden()
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT `+invitationColumns+` FROM suma_invitations WHERE tenant_id=? ORDER BY created_at DESC,id`), p.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		item, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, invitationMap(item))
	}
	return out, rows.Err()
}

func (s *Service) CreateInvitation(ctx context.Context, p Principal, m map[string]any) (map[string]any, error) {
	email, err := invitationEmail(str(m, "email"))
	if err != nil {
		return nil, err
	}
	role := str(m, "role")
	if role != "member" && role != "admin" {
		return nil, invalid("邀请角色应为 admin 或 member；所有权须显式转交")
	}
	grants := []invitationGrant{}
	if m["grants"] != nil {
		if err = json.Unmarshal([]byte(stringify(m["grants"])), &grants); err != nil {
			return nil, invalid("无效项目授权列表")
		}
	}
	var secret string
	out, err := s.governanceWrite(ctx, p, "organization.invitation.create", "", m, requireOrgAdmin, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		if err := s.validateInvitationGrants(ctx, tx, p.TenantID, grants); err != nil {
			return nil, err
		}
		secret = newID() + newID()
		item := governanceInvitation{ID: newID(), TenantID: p.TenantID, Version: 1, Email: email, Role: role, Grants: grants, Status: "pending", CreatedBy: p.ID, CreatedAt: now(), ExpiresAt: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)}
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_invitations(`+invitationColumns+`,token_hash) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, item.TenantID, item.Version, item.Email, item.Role, stringify(item.Grants), item.Status, item.CreatedBy, item.CreatedAt, item.ExpiresAt, "", "", digest(secret))
		if err != nil {
			return nil, err
		}
		// Only metadata enters the durable operation receipt.
		result := invitationMap(item)
		result["token_revealed"] = false
		result["recovery"] = "邀请密钥仅显示一次；若遗失，请撤销此邀请并重新创建"
		return result, nil
	})
	if err != nil {
		return nil, err
	}
	if secret != "" {
		out["token"] = secret
		out["token_revealed"] = true
	}
	return out, nil
}

func (s *Service) RevokeInvitation(ctx context.Context, p Principal, id string, m map[string]any) (map[string]any, error) {
	return s.governanceWrite(ctx, p, "organization.invitation.revoke", id, m, requireOrgAdmin, func(tx *sql.Tx, p Principal) (map[string]any, error) {
		item, err := scanInvitation(tx.QueryRowContext(ctx, s.q(`SELECT `+invitationColumns+` FROM suma_invitations WHERE tenant_id=? AND id=?`), p.TenantID, id))
		if errors.Is(err, sql.ErrNoRows) {
			return nil, missing()
		}
		if err != nil {
			return nil, err
		}
		if number(m, "expected_version") != item.Version {
			return nil, conflict("邀请已更新，请刷新后重试")
		}
		if item.Status != "pending" {
			return nil, conflict("此邀请已使用或已撤销")
		}
		r, err := tx.ExecContext(ctx, s.q(`UPDATE suma_invitations SET status='revoked',version=version+1 WHERE tenant_id=? AND id=? AND version=? AND status='pending'`), p.TenantID, id, item.Version)
		if err != nil {
			return nil, err
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return nil, conflict("邀请已更新")
		}
		item.Status = "revoked"
		item.Version++
		return invitationMap(item), nil
	})
}

func (s *Service) browserSessionTx(ctx context.Context, tx *sql.Tx, p Principal) (string, error) {
	token := newID() + newID()
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_sessions(token_hash,user_id,tenant_id,expires_at,label) VALUES(?,?,?,?,?)`), digest(token), p.ID, p.TenantID, time.Now().UTC().Add(7*24*time.Hour).Format(time.RFC3339Nano), "browser")
	return token, err
}

func passwordMatches(password, hash string) bool {
	parts := strings.Split(hash, ":")
	return len(parts) == 2 && subtle.ConstantTimeCompare([]byte(passwordHash(password, parts[0])), []byte(hash)) == 1
}

// AcceptInvitation is intentionally not a login shortcut. Existing users prove
// their current password; a consumed token never starts a second acceptance.
func (s *Service) AcceptInvitation(ctx context.Context, m map[string]any) (Principal, string, error) {
	email, err := invitationEmail(str(m, "email"))
	if err != nil {
		return Principal{}, "", err
	}
	password, name, secret := str(m, "password"), strings.TrimSpace(str(m, "name")), str(m, "token")
	if len(password) < 12 || len(password) > 1024 || name == "" || len(name) > 200 {
		return Principal{}, "", invalid("姓名应为 1–200 字节，密码应为 12–1024 字节")
	}
	if len(secret) != 64 {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if _, err = hex.DecodeString(secret); err != nil {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if key := str(m, "idempotency_key"); len(key) < 8 || len(key) > 128 {
		return Principal{}, "", invalid("请提供 8–128 字符的 idempotency_key")
	}
	var tenant string
	if err = s.DB.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_invitations WHERE token_hash=?`), digest(secret)).Scan(&tenant); errors.Is(err, sql.ErrNoRows) {
		return Principal{}, "", invalid("邀请不存在或密钥无效")
	}
	if err != nil {
		return Principal{}, "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, "", err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, tenant); err != nil {
		return Principal{}, "", err
	}
	item, err := scanInvitation(tx.QueryRowContext(ctx, s.q(`SELECT `+invitationColumns+` FROM suma_invitations WHERE token_hash=?`), digest(secret)))
	if err != nil {
		return Principal{}, "", err
	}
	var stored string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT token_hash FROM suma_invitations WHERE id=? AND tenant_id=?`), item.ID, tenant).Scan(&stored); err != nil {
		return Principal{}, "", err
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(digest(secret))) != 1 {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if subtle.ConstantTimeCompare([]byte(item.Email), []byte(email)) != 1 {
		return Principal{}, "", invalid("邀请邮箱不匹配")
	}
	expiry, err := time.Parse(time.RFC3339Nano, item.ExpiresAt)
	if err != nil || !time.Now().Before(expiry) || item.Status != "pending" {
		return Principal{}, "", conflict("邀请已使用、已撤销或已过期；若先前已接受，请使用普通登录")
	}
	issuer, err := s.liveOrganization(ctx, tx, Principal{ID: item.CreatedBy, TenantID: tenant})
	if err != nil || !orgAdmin(issuer) {
		return Principal{}, "", conflict("邀请人已无组织管理权限，请重新申请邀请")
	}
	if item.Role != "admin" && item.Role != "member" {
		return Principal{}, "", invalid("无效邀请角色")
	}
	if err = s.validateInvitationGrants(ctx, tx, tenant, item.Grants); err != nil {
		return Principal{}, "", err
	}
	p := Principal{TenantID: tenant, Role: item.Role}
	var hash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,name,email,password_hash FROM suma_users WHERE email=?`), email).Scan(&p.ID, &p.Name, &p.Email, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		p.ID, p.Name, p.Email = newID(), name, email
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_users(id,name,email,password_hash,created_at) VALUES(?,?,?,?,?)`), p.ID, p.Name, p.Email, passwordHash(password, newID()), now()); err != nil {
			return Principal{}, "", err
		}
	} else if err != nil {
		return Principal{}, "", err
	} else if !passwordMatches(password, hash) {
		return Principal{}, "", &APIError{401, "invalid_credentials", "现有账号必须提供正确密码", nil}
	}
	var membership int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_org_members WHERE tenant_id=? AND user_id=?`), tenant, p.ID).Scan(&membership); err != nil {
		return Principal{}, "", err
	}
	if membership != 0 {
		return Principal{}, "", conflict("账号已是组织成员；邀请不会覆盖其角色或密码")
	}
	// Never persist a password-derived request digest: with known metadata it
	// would become a fast offline verifier beside the slow account password KDF.
	// Credentials are verified above; all subsequent uses of this invitation
	// fail its consumed-state check, including requests with the same key.
	body := map[string]any{"action": "invitation.accept", "invitation_id": item.ID, "email": email, "name": name}
	requestHash, replay, err := s.receipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), body)
	if err != nil {
		return Principal{}, "", err
	}
	if replay {
		return Principal{}, "", conflict("邀请已接受，请使用普通登录")
	}
	version, err := s.orgMemberVersion(ctx, tx, tenant, p.ID)
	if err != nil {
		return Principal{}, "", err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,?)`), tenant, p.ID, item.Role); err != nil {
		return Principal{}, "", err
	}
	if err = s.setOrgMemberVersion(ctx, tx, tenant, p.ID, version+1); err != nil {
		return Principal{}, "", err
	}
	for _, grant := range item.Grants {
		v, err := s.projectGrantVersion(ctx, tx, tenant, grant.ProjectID, p.ID)
		if err != nil {
			return Principal{}, "", err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,?)`), grant.ProjectID, p.ID, grant.Role); err != nil {
			return Principal{}, "", err
		}
		if err = s.setProjectGrantVersion(ctx, tx, tenant, grant.ProjectID, p.ID, v+1); err != nil {
			return Principal{}, "", err
		}
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_invitations SET status='accepted',accepted_by=?,accepted_at=?,version=version+1 WHERE id=? AND tenant_id=? AND status='pending' AND version=?`), p.ID, now(), item.ID, tenant, item.Version)
	if err != nil {
		return Principal{}, "", err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return Principal{}, "", conflict("邀请已使用")
	}
	if err = s.activity(ctx, tx, p, organizationScope, "organization.invitation.accept", item.ID, "接受组织邀请"); err != nil {
		return Principal{}, "", err
	}
	if err = s.saveReceipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), requestHash, map[string]any{"user_id": p.ID, "organization_id": tenant, "invitation_id": item.ID, "status": "accepted"}); err != nil {
		return Principal{}, "", err
	}
	token, err := s.browserSessionTx(ctx, tx, p)
	if err != nil {
		return Principal{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return Principal{}, "", err
	}
	return p, token, nil
}

func (s *Service) SwitchOrganization(ctx context.Context, p Principal, m map[string]any) (Principal, string, error) {
	target := str(m, "organization_id")
	if target == "" {
		return Principal{}, "", invalid("请选择组织")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, "", err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, target); err != nil {
		return Principal{}, "", err
	}
	if _, err = s.liveOrganization(ctx, tx, p); err != nil {
		return Principal{}, "", err
	}
	chosen, err := s.liveOrganization(ctx, tx, Principal{ID: p.ID, TenantID: target})
	if err != nil {
		return Principal{}, "", err
	}
	// Store the switch receipt in the source organization. It never contains a
	// browser secret and replay still checks both current memberships above.
	hash, replay, err := s.receipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), map[string]any{"action": "organization.switch", "body": m})
	if err != nil {
		return Principal{}, "", err
	}
	if !replay {
		if err = s.saveReceipt(ctx, tx, p, organizationScope, str(m, "idempotency_key"), hash, map[string]any{"organization_id": target, "user_id": p.ID}); err != nil {
			return Principal{}, "", err
		}
	}
	token, err := s.browserSessionTx(ctx, tx, chosen)
	if err != nil {
		return Principal{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return Principal{}, "", err
	}
	return chosen, token, nil
}
