package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/strictjson"
)

// DesktopRunnerRenewalRequest belongs to the private loopback/HMAC endpoint.
// The desktop persists its new secret first; only its digest crosses this API.
type DesktopRunnerRenewalRequest struct {
	ProjectID                string `json:"project_id"`
	UserID                   string `json:"user_id"`
	TenantID                 string `json:"tenant_id"`
	WorkerID                 string `json:"worker_id"`
	AdapterID                string `json:"adapter_id"`
	InstanceID               string `json:"instance_id"`
	ExpectedBindingSHA256    string `json:"expected_binding_sha256"`
	BaseCommit               string `json:"base_commit"`
	RegistrationOperationID  string `json:"registration_operation_id"`
	ExpectedWorkerVersion    int    `json:"expected_worker_version"`
	PreviousCredentialSHA256 string `json:"previous_credential_sha256"`
	CredentialSHA256         string `json:"credential_sha256"`
	IdempotencyKey           string `json:"idempotency_key"`
}

func (r *DesktopRunnerRenewalRequest) UnmarshalJSON(raw []byte) error {
	type wire DesktopRunnerRenewalRequest
	var value wire
	if strictjson.Decode(raw, &value, 8192) != nil {
		return desktopRunnerInvalid()
	}
	*r = DesktopRunnerRenewalRequest(value)
	return nil
}

func desktopRunnerRenewalOperation(value, prefix string) bool {
	return strings.HasPrefix(value, prefix) && desktopWorkspaceHex(strings.TrimPrefix(value, prefix), 32)
}

func (s *Service) renewDesktopRunner(parent context.Context, actor Actor, req DesktopRunnerRenewalRequest, proof desktopRunnerProof) (DesktopRunnerRegistration, error) {
	var out DesktopRunnerRegistration
	if !desktopRunnerRenewalOperation(req.RegistrationOperationID, "desktop-runner-") || !desktopRunnerRenewalOperation(req.IdempotencyKey, "desktop-runner-renew-") || !strings.HasPrefix(req.WorkerID, "desktop-") || !desktopWorkspaceHex(strings.TrimPrefix(req.WorkerID, "desktop-"), 32) || req.ExpectedWorkerVersion < 1 || !desktopWorkspaceHex(req.InstanceID, 64) || !desktopWorkspaceHex(req.ExpectedBindingSHA256, 64) || !gitRevision.MatchString(req.BaseCommit) || !desktopWorkspaceHex(req.PreviousCredentialSHA256, 64) || !desktopWorkspaceHex(req.CredentialSHA256, 64) || req.PreviousCredentialSHA256 == req.CredentialSHA256 {
		return out, desktopRunnerInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	// Human authority locks include this user's complete credential set. The old
	// Runner and its callbacks therefore share the original lock order.
	auth, err := s.artifactAdminTx(ctx, tx, actor, req.ProjectID)
	if err != nil {
		return out, err
	}
	if err = s.validateDesktopRunnerProof(auth, proof, "renew", req.ProjectID, req.UserID, req.TenantID, req.WorkerID, req.AdapterID, req.InstanceID, req.ExpectedBindingSHA256, req.BaseCommit, req.RegistrationOperationID, strconv.Itoa(req.ExpectedWorkerVersion), req.PreviousCredentialSHA256, req.CredentialSHA256, req.IdempotencyKey); err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	var worker ReleaseWorker
	var rawProfiles, currentDigest, holder, createdBy string
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT w.id,w.project_id,w.worker_id,w.version,w.status,w.profiles,w.deployment,w.created_at,w.expires_at,w.token_hash,x.user_id,w.created_by FROM suma_release_workers w JOIN suma_sessions x ON x.token_hash=w.token_hash WHERE w.tenant_id=? AND w.project_id=? AND w.worker_id=?`)), p.TenantID, req.ProjectID, req.WorkerID).Scan(&worker.ID, &worker.ProjectID, &worker.WorkerID, &worker.Version, &worker.Status, &rawProfiles, &worker.Deployment, &worker.CreatedAt, &worker.ExpiresAt, &currentDigest, &holder, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	if err != nil {
		return out, err
	}
	if holder != p.ID || createdBy != p.ID {
		return out, forbidden()
	}
	var originalRaw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, req.ProjectID, req.RegistrationOperationID).Scan(&originalRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	if err != nil {
		return out, err
	}
	var original DesktopRunnerRegistration
	if err = json.Unmarshal([]byte(originalRaw), &original); err != nil {
		return out, err
	}
	if original.OperationID != req.RegistrationOperationID || original.ProjectID != req.ProjectID || original.UserID != p.ID || original.TenantID != p.TenantID || original.AdapterID != req.AdapterID || original.InstanceID != req.InstanceID || original.BindingSHA256 != req.ExpectedBindingSHA256 || original.BaseCommit != req.BaseCommit || original.Worker.ID != worker.ID || original.Worker.WorkerID != worker.WorkerID || original.Worker.CreatedAt != worker.CreatedAt {
		return out, desktopRunnerError(409, "context_changed")
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"desktop_runner_renew": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if worker.Status != "active" {
		return out, forbidden()
	}
	if worker.Version != req.ExpectedWorkerVersion || currentDigest != req.PreviousCredentialSHA256 {
		return out, desktopRunnerError(409, "credential_changed")
	}
	// Expiry alone is renewable. Explicit revocation, source changes and a
	// replaced slot digest are not; no expired credential is used to authorize.
	var eligible int
	err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_credential_authority a JOIN suma_credential_slots c ON c.id=a.slot_id JOIN suma_credential_grants g ON g.id=a.grant_id JOIN suma_auth_sources z ON z.id=a.source_id WHERE a.token_hash=? AND a.credential_kind=? AND a.revoked=0 AND c.current_digest=a.token_hash AND c.revoked=0 AND c.user_id=? AND c.tenant_id=? AND c.project_id=? AND c.credential_kind=a.credential_kind AND c.grant_id=g.id AND c.source_id=a.source_id AND g.revoked=0 AND g.user_id=c.user_id AND g.tenant_id=c.tenant_id AND g.project_id=c.project_id AND g.credential_kind=a.credential_kind AND g.scope_kind='runner' AND g.approved_by=c.user_id AND g.source_id=a.source_id AND g.source_generation=a.source_generation AND z.user_id=c.user_id AND z.revoked=0 AND z.generation=a.source_generation`), currentDigest, kindName(actorProjectRunner), p.ID, p.TenantID, req.ProjectID).Scan(&eligible)
	if err != nil {
		return out, err
	}
	if eligible != 1 {
		return out, forbidden()
	}
	local, err := s.desktopRunnerContextTx(ctx, tx, auth)
	if err != nil {
		return out, err
	}
	if local.InstanceID != req.InstanceID || local.BindingSHA256 != req.ExpectedBindingSHA256 || local.BaseCommit != req.BaseCommit {
		return out, desktopRunnerError(409, "context_changed")
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, req.ProjectID); err != nil {
		return out, err
	}
	if err = s.desktopRunnerRenewalDrainedTx(ctx, tx, auth, req.WorkerID, currentDigest); err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(rawProfiles), &worker.Profiles); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=? AND revoked=0`), currentDigest); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_slots SET revoked=1 WHERE id IN (SELECT slot_id FROM suma_credential_authority WHERE token_hash=?)`), currentDigest); err != nil {
		return out, err
	}
	expires, err := s.issueDesktopRunnerDigestTx(ctx, tx, auth, "release-worker:"+req.WorkerID, req.IdempotencyKey, req.CredentialSHA256)
	if err != nil {
		return out, err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_release_workers SET token_hash=?,version=version+1,expires_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND token_hash=? AND status='active'`), req.CredentialSHA256, expires, p.TenantID, req.ProjectID, worker.ID, req.ExpectedWorkerVersion, currentDigest)
	if err != nil {
		return out, err
	}
	if rows, e := result.RowsAffected(); e != nil {
		return out, e
	} else if rows != 1 {
		return out, desktopRunnerError(409, "credential_changed")
	}
	worker.Version++
	worker.ExpiresAt = expires
	out = DesktopRunnerRegistration{OperationID: req.IdempotencyKey, ProjectID: req.ProjectID, UserID: p.ID, TenantID: p.TenantID, InstanceID: local.InstanceID, AdapterID: req.AdapterID, BindingSHA256: local.BindingSHA256, BaseCommit: local.BaseCommit, Worker: worker}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	if err = s.commandActivity(ctx, auth, "desktop_runner.credential_renewed", worker.ID, worker.WorkerID); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	if err = tx.Commit(); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	return out, nil
}

func (s *Service) desktopRunnerRenewalDrainedTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, worker, oldDigest string) error {
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return err
	}
	tenant, project := auth.currentPrincipal.TenantID, auth.target.projectID
	queries := []struct {
		sql  string
		args []any
	}{
		{`SELECT COUNT(*) FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.worker_id=? AND n.state IN ('running','reconciling')`, []any{tenant, project, worker}},
		{`SELECT COUNT(*) FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.worker_id=? AND a.state IN ('running','reconciling')`, []any{tenant, project, worker}},
		{`SELECT COUNT(*) FROM dag_resource_ledger l JOIN dag_attempts a ON a.run_id=l.run_id AND a.node_id=l.node_id AND a.attempt=l.attempt AND a.fence=l.fence WHERE l.tenant_id=? AND l.project_id=? AND a.worker_id=? AND l.state='active'`, []any{tenant, project, worker}},
		{`SELECT COUNT(*) FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=? AND worker_id=? AND token_hash=? AND state<>'sealed'`, []any{tenant, project, worker, oldDigest}},
		{`SELECT COUNT(*) FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND worker_id=? AND (state IN ('leased','reconciling') OR (state='requested' AND expires_at>?))`, []any{tenant, project, worker, auth.checkedAt.Unix()}},
		{`SELECT COUNT(*) FROM suma_workspace_observations WHERE tenant_id=? AND project_id=? AND worker_id=? AND (state IN ('leased','reconciling') OR (state='requested' AND expires_at>?))`, []any{tenant, project, worker, auth.checkedAt.Unix()}},
		{`SELECT COUNT(*) FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND worker_token_hash=? AND state='issued' AND expires_at>?`, []any{tenant, project, oldDigest, auth.checkedAt.Format(time.RFC3339Nano)}},
		{`SELECT COUNT(*) FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND worker_token_hash=? AND state='issued' AND expires_at>?`, []any{tenant, project, oldDigest, auth.checkedAt.Format(time.RFC3339Nano)}},
		{`SELECT COUNT(*) FROM suma_workbuddy_cloud_submissions c LEFT JOIN dag_nodes n ON n.run_id=c.run_id AND n.id=c.node_id WHERE c.tenant_id=? AND c.project_id=? AND c.worker_credential_hash=? AND (c.state IN ('prepared','unknown') OR (n.worker_id=? AND n.state='waiting'))`, []any{tenant, project, oldDigest, worker}},
		{`SELECT COUNT(*) FROM suma_workspace_terminal_sessions t WHERE t.tenant_id=? AND t.project_id=? AND t.state NOT IN ('stopped','exited') AND EXISTS (SELECT 1 FROM dag_attempts a WHERE a.run_id=t.run_id AND a.worker_id=?)`, []any{tenant, project, worker}},
	}
	for _, query := range queries {
		var count int
		if err := tx.QueryRowContext(ctx, s.q(query.sql), query.args...).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return desktopRunnerError(409, "renewal_pending")
		}
	}
	// Transfer state is inside its original typed binding, not a parallel state
	// column. A sealed historical transfer does not block later idle renewal.
	rows, err := tx.QueryContext(ctx, s.q(`SELECT binding FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, worker)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var value struct {
			State string `json:"state"`
		}
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal([]byte(raw), &value) != nil || value.State != "sealed" {
			return desktopRunnerError(409, "renewal_pending")
		}
	}
	return rows.Err()
}
