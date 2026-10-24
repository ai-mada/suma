package platform

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
)

func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func passwordHash(password, salt string) string {
	key, _ := pbkdf2.Key(sha256.New, password, []byte(salt), 200000, 32)
	return salt + ":" + hex.EncodeToString(key)
}
func (s *Service) SetupRequired(ctx context.Context) bool {
	var n int
	return s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM suma_users`).Scan(&n) == nil && n == 0
}
func (s *Service) Setup(ctx context.Context, m map[string]any) (Principal, string, error) {
	email := strings.ToLower(strings.TrimSpace(str(m, "email")))
	password := str(m, "password")
	name := strings.TrimSpace(str(m, "name"))
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
		return Principal{}, "", invalid("请输入有效邮箱")
	}
	if len(password) < 12 || len(password) > 1024 {
		return Principal{}, "", invalid("密码至少需要 12 个字符")
	}
	if name == "" {
		return Principal{}, "", invalid("请填写姓名")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Principal{}, "", err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM suma_users`).Scan(&count); err != nil {
		return Principal{}, "", err
	}
	if count != 0 {
		return Principal{}, "", &APIError{409, "already_initialized", "平台已初始化，请登录", nil}
	}
	user := Principal{ID: newID(), Name: name, Email: email, TenantID: newID(), Role: "owner"}
	org := str(m, "organization")
	if org == "" {
		org = "我的团队"
	}
	project := str(m, "project")
	if project == "" {
		project = "我的研发项目"
	}
	at := now()
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO suma_organizations(id,name,created_at) VALUES(?,?,?)`, []any{user.TenantID, org, at}},
		{`INSERT INTO suma_users(id,name,email,password_hash,created_at) VALUES(?,?,?,?,?)`, []any{user.ID, name, email, passwordHash(password, newID()), at}},
		{`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,?)`, []any{user.TenantID, user.ID, "owner"}},
		{`INSERT INTO suma_projects(id,tenant_id,version,title,description,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, []any{newID(), user.TenantID, 1, project, "", "active", at, at}},
	}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, s.q(q.sql), q.args...); err != nil {
			return Principal{}, "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return Principal{}, "", err
	}
	token, err := s.CreateToken(ctx, user, "browser", 7*24*time.Hour)
	return user, token, err
}
func (s *Service) Login(ctx context.Context, email, password string) (Principal, string, error) {
	if len(email) > 254 || len(password) > 1024 {
		return Principal{}, "", invalid("登录内容超出限制")
	}
	var p Principal
	var hash string
	err := s.DB.QueryRowContext(ctx, s.q(`SELECT u.id,u.name,u.email,u.password_hash,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.email=? ORDER BY m.tenant_id LIMIT 1`), strings.ToLower(strings.TrimSpace(email))).Scan(&p.ID, &p.Name, &p.Email, &hash, &p.TenantID, &p.Role)
	if err != nil {
		passwordHash(password, "invalid-account-salt")
		return p, "", &APIError{401, "invalid_credentials", "邮箱或密码不正确", nil}
	}
	parts := strings.Split(hash, ":")
	if len(parts) != 2 || subtle.ConstantTimeCompare([]byte(passwordHash(password, parts[0])), []byte(hash)) != 1 {
		return p, "", &APIError{401, "invalid_credentials", "邮箱或密码不正确", nil}
	}
	token, err := s.CreateToken(ctx, p, "browser", 7*24*time.Hour)
	return p, token, err
}

func (s *Service) TokenScope(ctx context.Context, token string) (string, string) {
	var kind, project string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT kind,project_id FROM suma_token_scopes WHERE token_hash=?`), digest(token)).Scan(&kind, &project); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ""
		}
		return "invalid", ""
	}
	return kind, project
}
func (s *Service) CreateToken(ctx context.Context, p Principal, label string, ttl time.Duration) (string, error) {
	token := newID() + newID()
	_, err := s.DB.ExecContext(ctx, s.q(`INSERT INTO suma_sessions(token_hash,user_id,tenant_id,expires_at,label) VALUES(?,?,?,?,?)`), digest(token), p.ID, p.TenantID, time.Now().UTC().Add(ttl).Format(time.RFC3339Nano), label)
	return token, err
}
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	var p Principal
	var expires string
	if token == "" {
		return p, &APIError{401, "unauthenticated", "请登录", map[string]any{"setup_required": s.SetupRequired(ctx)}}
	}
	err := s.DB.QueryRowContext(ctx, s.q(`SELECT u.id,u.name,u.email,m.tenant_id,m.role,t.expires_at FROM suma_sessions t JOIN suma_users u ON u.id=t.user_id JOIN suma_org_members m ON m.user_id=t.user_id AND m.tenant_id=t.tenant_id WHERE t.token_hash=?`), digest(token)).Scan(&p.ID, &p.Name, &p.Email, &p.TenantID, &p.Role, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return p, &APIError{401, "unauthenticated", "登录已失效", nil}
	}
	if err != nil {
		return p, err
	}
	expiry, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !time.Now().Before(expiry) {
		return p, &APIError{401, "session_expired", "登录已过期", nil}
	}
	return p, nil
}
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, s.q(`DELETE FROM suma_sessions WHERE token_hash=?`), digest(token))
	return err
}
