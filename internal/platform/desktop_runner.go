package platform

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

type DesktopRunnerContextRequest struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	TenantID  string `json:"tenant_id"`
}

type DesktopRunnerRegistrationRequest struct {
	ProjectID             string `json:"project_id"`
	UserID                string `json:"user_id"`
	TenantID              string `json:"tenant_id"`
	WorkerID              string `json:"worker_id"`
	AdapterID             string `json:"adapter_id"`
	InstanceID            string `json:"instance_id"`
	ExpectedBindingSHA256 string `json:"expected_binding_sha256"`
	BaseCommit            string `json:"base_commit"`
	CredentialSHA256      string `json:"credential_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}

// This private endpoint's path and OS identity are never part of public
// WorkspaceMetadata or any renderer/preload response.
type DesktopRunnerContext struct {
	ProjectID          string `json:"project_id"`
	UserID             string `json:"user_id"`
	TenantID           string `json:"tenant_id"`
	InstanceID         string `json:"instance_id"`
	BindingSHA256      string `json:"binding_sha256"`
	RepositoryPath     string `json:"repository_path"`
	RepositoryIdentity string `json:"repository_identity"`
	BaseCommit         string `json:"base_commit"`
}

type DesktopRunnerRegistration struct {
	OperationID   string        `json:"operation_id"`
	ProjectID     string        `json:"project_id"`
	UserID        string        `json:"user_id"`
	TenantID      string        `json:"tenant_id"`
	InstanceID    string        `json:"instance_id"`
	AdapterID     string        `json:"adapter_id"`
	BindingSHA256 string        `json:"binding_sha256"`
	BaseCommit    string        `json:"base_commit"`
	Worker        ReleaseWorker `json:"worker"`
}

type desktopRunnerProof struct{ nonce, expires, signature string }

func desktopRunnerError(status int, code string) error {
	return &APIError{status, "desktop_runner_" + code, "desktop_runner_" + code, nil}
}
func desktopRunnerInvalid() error { return desktopRunnerError(400, "invalid") }

func (s *Service) desktopRunnerInstanceID() string {
	return desktopWorkspaceMAC(s.DesktopWorkspaceKey, "suma-desktop-instance-v1")
}

func (s *Service) validateDesktopRunnerProof(a *authorizedCommand, proof desktopRunnerProof, action string, fields ...string) error {
	if !s.desktopWorkspaceAvailable() {
		return desktopRunnerError(503, "unavailable")
	}
	if a == nil || a.actor.kind != actorBrowser || len(fields) < 3 || fields[0] != a.target.projectID || fields[1] != a.currentPrincipal.ID || fields[2] != a.currentPrincipal.TenantID {
		return forbidden()
	}
	for _, value := range fields[:3] {
		if !desktopWorkspaceHex(value, 32) {
			return desktopRunnerInvalid()
		}
	}
	if !desktopWorkspaceHex(proof.nonce, 32) || !desktopWorkspaceHex(proof.signature, 64) || len(proof.expires) > 40 || !strings.HasSuffix(proof.expires, "Z") {
		return desktopRunnerError(403, "proof_invalid")
	}
	expires, err := time.Parse(time.RFC3339Nano, proof.expires)
	if err != nil || !expires.After(a.checkedAt) || expires.After(a.checkedAt.Add(120*time.Second)) {
		return desktopRunnerError(403, "proof_expired")
	}
	values := append([]string{"suma-desktop-runner-v1", action}, fields...)
	values = append(values, proof.nonce, proof.expires)
	want := desktopWorkspaceMAC(s.DesktopWorkspaceKey, values...)
	if !hmac.Equal([]byte(proof.signature), []byte(want)) {
		return desktopRunnerError(403, "proof_invalid")
	}
	return nil
}

// Reuse the guarded, offline Git command and metadata reader while retaining
// this transaction's current identity and binding locks; no nested authority
// transaction, Git mutation or untrusted Git configuration is introduced.
func (s *Service) desktopRunnerContextTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand) (DesktopRunnerContext, error) {
	var out DesktopRunnerContext
	if err := a.assertTx(tx, a.target, a.purpose); err != nil {
		return out, err
	}
	state, err := s.workspaceBindingStateTx(ctx, tx, a.currentPrincipal.TenantID, a.target.projectID)
	if err != nil {
		return out, err
	}
	if err = s.validateWorkspaceBindingRoot(ctx, a.currentPrincipal.TenantID, a.target.projectID, state); err != nil {
		return out, err
	}
	identity, err := desktopWorkspaceRootIdentity(ctx, state.root)
	if err != nil {
		return out, err
	}
	g := &workspaceGitSession{s: s, actor: a.actor, project: a.target.projectID, path: state.root, ctx: ctx}
	g.root, err = os.OpenRoot(state.root)
	if err != nil {
		return out, desktopRunnerError(422, "repository_unavailable")
	}
	defer g.close()
	metadata, reason, err := g.inspectMetadata()
	if err != nil {
		return out, err
	}
	if reason != "" {
		return out, desktopRunnerError(422, "repository_unavailable")
	}
	g.executable, err = exec.LookPath("git")
	if err != nil {
		return out, desktopRunnerError(422, "repository_unavailable")
	}
	if safe, e := g.safeConfig(); e != nil || !safe {
		if e != nil {
			return out, e
		}
		return out, desktopRunnerError(422, "repository_unsafe")
	}
	top, err := g.required(8192, "rev-parse", "--show-toplevel")
	if err != nil {
		return out, err
	}
	if !workspaceGitSamePath(strings.TrimSpace(string(top)), state.root) {
		return out, desktopRunnerError(422, "repository_unavailable")
	}
	head, err := g.required(8192, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return out, desktopRunnerError(422, "head_unavailable")
	}
	commit := strings.TrimSpace(string(head))
	if !gitRevision.MatchString(commit) {
		return out, desktopRunnerError(422, "head_unavailable")
	}
	afterMetadata, reason, err := g.inspectMetadata()
	if err != nil {
		return out, err
	}
	afterIdentity, err := desktopWorkspaceRootIdentity(ctx, state.root)
	if err != nil {
		return out, err
	}
	if reason != "" || metadata != afterMetadata || identity != afterIdentity {
		return out, desktopRunnerError(409, "repository_changed")
	}
	if err = s.validateWorkspaceBindingRoot(ctx, a.currentPrincipal.TenantID, a.target.projectID, state); err != nil {
		return out, err
	}
	out = DesktopRunnerContext{ProjectID: a.target.projectID, UserID: a.currentPrincipal.ID, TenantID: a.currentPrincipal.TenantID, InstanceID: s.desktopRunnerInstanceID(), BindingSHA256: state.sha256, RepositoryPath: state.root, RepositoryIdentity: identity, BaseCommit: commit}
	return out, nil
}

func (s *Service) desktopRunnerContext(parent context.Context, actor Actor, request DesktopRunnerContextRequest, proof desktopRunnerProof) (DesktopRunnerContext, error) {
	var out DesktopRunnerContext
	ctx, tx, cancel, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, request.ProjectID)
	if err != nil {
		return out, err
	}
	if err = s.validateDesktopRunnerProof(auth, proof, "context", request.ProjectID, request.UserID, request.TenantID); err != nil {
		return out, err
	}
	out, err = s.desktopRunnerContextTx(ctx, tx, auth)
	if err != nil {
		return DesktopRunnerContext{}, err
	}
	if err = tx.Commit(); err != nil {
		return DesktopRunnerContext{}, err
	}
	return out, nil
}

func (s *Service) registerDesktopRunner(parent context.Context, actor Actor, request DesktopRunnerRegistrationRequest, proof desktopRunnerProof) (DesktopRunnerRegistration, error) {
	var out DesktopRunnerRegistration
	ctx, tx, cancel, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, request.ProjectID)
	if err != nil {
		return out, err
	}
	if err = s.validateDesktopRunnerProof(auth, proof, "register", request.ProjectID, request.UserID, request.TenantID, request.WorkerID, request.AdapterID, request.InstanceID, request.ExpectedBindingSHA256, request.BaseCommit, request.CredentialSHA256, request.IdempotencyKey); err != nil {
		return out, err
	}
	if !strings.HasPrefix(request.WorkerID, "desktop-") || !desktopWorkspaceHex(strings.TrimPrefix(request.WorkerID, "desktop-"), 32) || !desktopWorkspaceHex(request.InstanceID, 64) || !desktopWorkspaceHex(request.ExpectedBindingSHA256, 64) || !gitRevision.MatchString(request.BaseCommit) || !desktopWorkspaceHex(request.CredentialSHA256, 64) || len(request.IdempotencyKey) < 8 || len(request.IdempotencyKey) > 128 || strings.IndexFunc(request.IdempotencyKey, unicode.IsControl) >= 0 {
		return out, desktopRunnerInvalid()
	}
	switch request.AdapterID {
	case "codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli", "qoder-acp":
	default:
		return out, desktopRunnerInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"desktop_runner_register": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	local, err := s.desktopRunnerContextTx(ctx, tx, auth)
	if err != nil {
		return out, err
	}
	if local.InstanceID != request.InstanceID || local.BindingSHA256 != request.ExpectedBindingSHA256 || local.BaseCommit != request.BaseCommit {
		return out, desktopRunnerError(409, "context_changed")
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=?`), auth.currentPrincipal.TenantID, request.ProjectID, request.WorkerID).Scan(&count); err != nil {
		return out, err
	}
	if count != 0 {
		return out, desktopRunnerError(409, "worker_exists")
	}
	expires, err := s.issueDesktopRunnerDigestTx(ctx, tx, auth, "release-worker:"+request.WorkerID, request.IdempotencyKey, request.CredentialSHA256)
	if err != nil {
		return out, err
	}
	id, at := newID(), auth.checkedAt.Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_workers(id,tenant_id,project_id,worker_id,token_hash,version,status,profiles,deployment,created_by,created_at,expires_at)VALUES(?,?,?,?,?,1,'active','[]',0,?,?,?)`), id, auth.currentPrincipal.TenantID, request.ProjectID, request.WorkerID, request.CredentialSHA256, auth.currentPrincipal.ID, at, expires); err != nil {
		return out, err
	}
	out = DesktopRunnerRegistration{OperationID: request.IdempotencyKey, ProjectID: request.ProjectID, UserID: auth.currentPrincipal.ID, TenantID: auth.currentPrincipal.TenantID, InstanceID: local.InstanceID, AdapterID: request.AdapterID, BindingSHA256: local.BindingSHA256, BaseCommit: local.BaseCommit, Worker: ReleaseWorker{ID: id, ProjectID: request.ProjectID, WorkerID: request.WorkerID, Version: 1, Status: "active", Profiles: []ReleaseProfileRegistration{}, CreatedAt: at, ExpiresAt: expires}}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	if err = s.commandActivity(ctx, auth, "release_worker.registered", id, request.WorkerID); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	if err = tx.Commit(); err != nil {
		return DesktopRunnerRegistration{}, err
	}
	return out, nil
}
