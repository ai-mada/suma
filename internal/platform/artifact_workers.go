package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

type ReleaseProfileRegistration struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
}
type ReleaseWorker struct {
	ID         string                       `json:"id"`
	ProjectID  string                       `json:"project_id"`
	WorkerID   string                       `json:"worker_id"`
	Version    int                          `json:"version"`
	Status     string                       `json:"status"`
	Profiles   []ReleaseProfileRegistration `json:"profiles"`
	Deployment bool                         `json:"deployment"`
	CreatedAt  string                       `json:"created_at"`
	ExpiresAt  string                       `json:"expires_at"`
}

func (s *Service) artifactAdminTx(ctx context.Context, tx *sql.Tx, p Principal, project, token string) error {
	if err := s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return err
	}
	live, err := s.liveOrganization(ctx, tx, p)
	if err != nil {
		return err
	}
	if !orgAdmin(live) {
		var role string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, p.ID).Scan(&role); err != nil || role != "admin" {
			return forbidden()
		}
	}
	var label, expires string
	var scopes int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT label,expires_at FROM suma_sessions WHERE token_hash=? AND tenant_id=? AND user_id=?`), digest(token), p.TenantID, p.ID).Scan(&label, &expires); err != nil || label != "browser" || expires <= now() {
		return forbidden()
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_token_scopes WHERE token_hash=?`), digest(token)).Scan(&scopes); err != nil {
		return err
	}
	if scopes != 0 {
		return forbidden()
	}
	return nil
}

func (s *Service) RegisterReleaseWorker(ctx context.Context, p Principal, project, token string, m map[string]any) (map[string]any, error) {
	var request struct {
		ProjectID      string                       `json:"project_id"`
		WorkerID       string                       `json:"worker_id"`
		Profiles       []ReleaseProfileRegistration `json:"profiles"`
		Deployment     bool                         `json:"deployment"`
		IdempotencyKey string                       `json:"idempotency_key"`
	}
	if err := releaseDecode(m, &request); err != nil {
		return nil, err
	}
	worker := str(m, "worker_id")
	if !releasecontract.ValidProfileID(worker) {
		return nil, invalid("执行节点标识应为 1–100 位字母、数字、点、短横线或下划线")
	}
	var profiles []ReleaseProfileRegistration
	b, err := json.Marshal(m["profiles"])
	if err != nil || json.Unmarshal(b, &profiles) != nil || len(profiles) > 32 {
		return nil, invalid("发布命令配置无效")
	}
	seen := map[string]bool{}
	for _, pr := range profiles {
		if !releasecontract.ValidProfileID(pr.ID) || !releasecontract.ValidDigest(pr.SHA256) || pr.Kind != "build" && pr.Kind != "package_test" || seen[pr.ID] {
			return nil, invalid("命令需唯一标识、种类及真实配置摘要")
		}
		seen[pr.ID] = true
	}
	deployment, _ := m["deployment"].(bool)
	// A stable execution identity may be registered without release capabilities.
	// Tool inventories reuse this identity, while release authorization below
	// still requires its exact build/package/deployment grant.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.artifactAdminTx(ctx, tx, p, project, token); err != nil {
		return nil, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, str(m, "idempotency_key"), map[string]any{"register_release_worker": m})
	if err != nil {
		return nil, err
	}
	if replay {
		var out map[string]any
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=?`), p.TenantID, project, worker).Scan(&count); err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, conflict("执行节点标识已有登记；请使用不同标识，不能覆盖历史凭证")
	}
	id := newID()
	secret := newID() + newID()
	at := now()
	expires := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	queries := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO suma_sessions(token_hash,user_id,tenant_id,expires_at,label)VALUES(?,?,?,?,?)`, []any{digest(secret), p.ID, p.TenantID, expires, "release-worker:" + worker}},
		{`INSERT INTO suma_token_scopes(token_hash,kind,project_id)VALUES(?,?,?)`, []any{digest(secret), "runner", project}},
		{`INSERT INTO suma_release_workers(id,tenant_id,project_id,worker_id,token_hash,version,status,profiles,deployment,created_by,created_at,expires_at)VALUES(?,?,?,?,?,1,'active',?,?,?,?,?)`, []any{id, p.TenantID, project, worker, digest(secret), stringify(profiles), deployment, p.ID, at, expires}},
	}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, s.q(q.q), q.args...); err != nil {
			return nil, err
		}
	}
	out := map[string]any{"id": id, "project_id": project, "worker_id": worker, "version": 1, "status": "active", "profiles": profiles, "deployment": deployment, "created_at": at, "expires_at": expires}
	if err = s.saveReceipt(ctx, tx, p, project, str(m, "idempotency_key"), hash, out); err != nil {
		return nil, err
	}
	if err = s.activity(ctx, tx, p, project, "release_worker.registered", id, worker); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	out["token"] = secret // Once only; neither receipt nor public registry contains the secret.
	return out, nil
}

func (s *Service) RevokeReleaseWorker(ctx context.Context, p Principal, project, id, token string, m map[string]any) (ReleaseWorker, error) {
	var out ReleaseWorker
	var req struct {
		ProjectID       string `json:"project_id"`
		ExpectedVersion int    `json:"expected_version"`
		IdempotencyKey  string `json:"idempotency_key"`
		Status          string `json:"status"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return out, err
	}
	if req.Status != "revoked" || req.ExpectedVersion < 1 {
		return out, invalid("发布凭证仅能以明确版本撤销")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.artifactAdminTx(ctx, tx, p, project, token); err != nil {
		return out, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, req.IdempotencyKey, map[string]any{"revoke_release_worker": id, "body": m})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	var raw, tokenHash string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT id,project_id,worker_id,version,status,profiles,deployment,created_at,expires_at,token_hash FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&out.ID, &out.ProjectID, &out.WorkerID, &out.Version, &out.Status, &raw, &out.Deployment, &out.CreatedAt, &out.ExpiresAt, &tokenHash); err != nil {
		return out, missing()
	}
	if err = json.Unmarshal([]byte(raw), &out.Profiles); err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.Status != "active" {
		return out, conflict("发布执行凭证已变化，请刷新")
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_sessions WHERE token_hash=?`), tokenHash); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_token_scopes WHERE token_hash=?`), tokenHash); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_workers SET status='revoked',version=version+1 WHERE id=? AND version=?`), id, out.Version); err != nil {
		return out, err
	}
	out.Status = "revoked"
	out.Version++
	if err = s.activity(ctx, tx, p, project, "release_worker.revoked", id, out.WorkerID); err != nil {
		return out, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) releaseWorkerTx(ctx context.Context, tx *sql.Tx, p Principal, project, token, worker, profile, profileHash, kind string) error {
	if err := s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return err
	}
	if err := s.currentRunnerCredentialTx(ctx, tx, p, project, token); err != nil {
		return err
	}
	var actual, state, raw string
	var deploy bool
	if err := tx.QueryRowContext(ctx, s.q(`SELECT w.worker_id,w.status,w.profiles,w.deployment FROM suma_release_workers w JOIN suma_sessions x ON x.token_hash=w.token_hash JOIN suma_token_scopes t ON t.token_hash=x.token_hash WHERE w.token_hash=? AND w.tenant_id=? AND w.project_id=? AND x.user_id=? AND x.tenant_id=w.tenant_id AND t.kind='runner' AND t.project_id=w.project_id`), digest(token), p.TenantID, project, p.ID).Scan(&actual, &state, &raw, &deploy); err != nil || state != "active" || subtle.ConstantTimeCompare([]byte(actual), []byte(worker)) != 1 {
		return forbidden()
	}
	if kind == "deployment" {
		if !deploy {
			return forbidden()
		}
		return nil
	}
	if kind == "identity" {
		return nil
	}
	var profiles []ReleaseProfileRegistration
	if json.Unmarshal([]byte(raw), &profiles) != nil {
		return forbidden()
	}
	for _, v := range profiles {
		if v.ID == profile && v.Kind == kind && v.SHA256 == profileHash {
			return nil
		}
	}
	return forbidden()
}

func (s *Service) boundReleaseWorker(ctx context.Context, token string) (string, error) {
	var worker string
	err := s.DB.QueryRowContext(ctx, s.q(`SELECT worker_id FROM suma_release_workers WHERE token_hash=?`), digest(token)).Scan(&worker)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return worker, err
}

func (s *Service) runnerIdentityTx(ctx context.Context, tx *sql.Tx, p Principal, project, token, worker string) error {
	if err := s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return err
	}
	if err := s.currentRunnerCredentialTx(ctx, tx, p, project, token); err != nil {
		return err
	}
	var bound string
	err := tx.QueryRowContext(ctx, s.q(`SELECT worker_id FROM suma_release_workers WHERE token_hash=?`), digest(token)).Scan(&bound)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if bound != "" && bound != worker {
		return forbidden()
	}
	var expected string
	err = tx.QueryRowContext(ctx, s.q(`SELECT token_hash FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=?`), p.TenantID, project, worker).Scan(&expected)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if expected != "" {
		if expected != digest(token) {
			return forbidden()
		}
		return s.releaseWorkerTx(ctx, tx, p, project, token, worker, "", "", "identity")
	}
	return nil
}

// Caller holds the organization authority lock first. A locking current read
// prevents a stale outer Authenticate (or an OceanBase RR snapshot) from
// restoring a dispatch after credential revocation. Logout's row deletion waits
// for this transaction; no token is stored in scheduler receipts or events.
func (s *Service) currentRunnerCredentialTx(ctx context.Context, tx *sql.Tx, p Principal, project, token string) error {
	query := `SELECT x.user_id,x.tenant_id,x.expires_at,t.kind,t.project_id FROM suma_sessions x JOIN suma_token_scopes t ON t.token_hash=x.token_hash WHERE x.token_hash=?`
	if s.Dialect == "oceanbase" || s.Dialect == "postgres" {
		query += ` FOR UPDATE`
	}
	var user, tenant, expires, scope, boundProject string
	err := tx.QueryRowContext(ctx, s.q(query), digest(token)).Scan(&user, &tenant, &expires, &scope, &boundProject)
	if errors.Is(err, sql.ErrNoRows) {
		return forbidden()
	}
	if err != nil {
		return err
	}
	if user != p.ID || tenant != p.TenantID || scope != "runner" || boundProject != project {
		return forbidden()
	}
	expiry, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return forbidden()
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	// Database clock granularity is seconds; expire conservatively within its
	// final second rather than treating a rounded-down clock as extra lifetime.
	if expiry.Unix() <= clock.Unix() {
		return forbidden()
	}
	return nil
}

func (s *Service) releaseClaimEligibleTx(ctx context.Context, tx *sql.Tx, p Principal, project, token, worker, run string) (bool, error) {
	var n int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND run_id=?`), p.TenantID, project, run).Scan(&n); err != nil {
		return false, err
	}
	if n == 0 {
		return true, nil
	}
	if task, err := s.TaskDispatchRunTx(ctx, tx, p.TenantID, project, run); err != nil {
		return false, err
	} else if task {
		return true, nil
	}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=? AND token_hash=? AND deployment=1 AND status='active'`), p.TenantID, project, worker, digest(token)).Scan(&n); err != nil {
		return false, err
	}
	return n == 1, nil
}

func (s *Service) ReleaseWorkers(ctx context.Context, p Principal, project string) ([]ReleaseWorker, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT id,project_id,worker_id,version,status,profiles,deployment,created_at,expires_at FROM suma_release_workers WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseWorker{}
	for rows.Next() {
		var w ReleaseWorker
		var raw string
		if err = rows.Scan(&w.ID, &w.ProjectID, &w.WorkerID, &w.Version, &w.Status, &raw, &w.Deployment, &w.CreatedAt, &w.ExpiresAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &w.Profiles); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
