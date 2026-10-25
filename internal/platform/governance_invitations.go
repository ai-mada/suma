package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
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

func (s *Service) invitationsTx(ctx context.Context, auth *authorizedCommand) ([]map[string]any, error) {
	if err := assertOrganizationRead(auth); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	if !orgAdmin(p) {
		return nil, forbidden()
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT `+invitationColumns+` FROM suma_invitations WHERE tenant_id=? ORDER BY created_at DESC,id`), p.TenantID)
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

func (s *Service) CreateInvitation(ctx context.Context, actor Actor, m map[string]any) (map[string]any, error) {
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
	out, err := s.governanceWrite(ctx, actor, "organization.invitation.create", "", m, requireOrgAdmin, func(auth *authorizedCommand) (map[string]any, error) {
		tx, p := auth.tx, auth.currentPrincipal
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

func (s *Service) RevokeInvitation(ctx context.Context, actor Actor, id string, m map[string]any) (map[string]any, error) {
	return s.governanceWrite(ctx, actor, "organization.invitation.revoke", id, m, requireOrgAdmin, func(auth *authorizedCommand) (map[string]any, error) {
		tx, p := auth.tx, auth.currentPrincipal
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

func passwordMatches(password, hash string) bool {
	parts := strings.Split(hash, ":")
	return len(parts) == 2 && subtle.ConstantTimeCompare([]byte(passwordHash(password, parts[0])), []byte(hash)) == 1
}
