package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const desktopWorkspaceProtocol = "suma-desktop-workspace-v1"

type WorkspaceDesktopGrant struct {
	Protocol    string `json:"protocol"`
	OperationID string `json:"operation_id"`
	UserID      string `json:"user_id"`
	TenantID    string `json:"tenant_id"`
	ProjectID   string `json:"project_id"`
	Path        string `json:"path"`
	ExpiresAt   string `json:"expires_at"`
	Nonce       string `json:"nonce"`
	Signature   string `json:"signature"`
}

type WorkspaceDesktopBindingRequest struct {
	ProjectID             string                `json:"project_id"`
	ExpectedBindingSHA256 string                `json:"expected_binding_sha256"`
	Grant                 WorkspaceDesktopGrant `json:"grant"`
	IdempotencyKey        string                `json:"idempotency_key"`
}

func desktopWorkspaceError(status int, code, message string) error {
	return &APIError{status, "desktop_workspace_" + code, message, nil}
}

func desktopWorkspaceInvalid() error {
	return desktopWorkspaceError(400, "invalid", "Desktop workspace request is invalid")
}

func desktopWorkspaceRootInvalid() error {
	return desktopWorkspaceError(403, "root_invalid", "Desktop workspace root proof is invalid; choose the directory again")
}

func desktopWorkspaceRootChanged() error {
	return desktopWorkspaceError(409, "root_changed", "Desktop workspace directory identity changed; choose the directory again")
}

func desktopWorkspaceRootUnavailable() error {
	return desktopWorkspaceError(422, "root_unavailable", "Desktop workspace directory is unavailable; choose the directory again")
}

func (s *Service) desktopWorkspaceAvailable() bool {
	// The desktop launcher supplies this key only to its loopback SQLite child.
	// A remote or shared database cannot opt into machine-local path approval.
	return s.Dialect == "sqlite" && len(s.DesktopWorkspaceKey) == sha256.Size
}

func desktopWorkspaceHex(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func desktopWorkspaceOperation(value string) bool {
	return strings.HasPrefix(value, "desktop-workspace-") && desktopWorkspaceHex(strings.TrimPrefix(value, "desktop-workspace-"), 32)
}

func desktopWorkspaceMAC(key []byte, fields ...string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.Join(fields, "\x00")))
	return hex.EncodeToString(mac.Sum(nil))
}

// Exact UTF-8 bytes are part of both the grant and the UI acknowledgement.
// Reject ambiguous paths rather than silently normalizing a signed selection.
func desktopWorkspacePath(path string) bool {
	if path == "" || len(path) > 4096 || !utf8.ValidString(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.IndexFunc(path, unicode.IsControl) >= 0 || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return false
	}
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(path)
		if len(volume) != 2 || volume[1] != ':' || !('A' <= volume[0] && volume[0] <= 'Z' || 'a' <= volume[0] && volume[0] <= 'z') {
			return false
		}
		for _, part := range strings.Split(strings.TrimPrefix(path, volume), string(filepath.Separator)) {
			if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.Contains(part, ":") {
				return false
			}
		}
	}
	return true
}

// Re-open the directory on every authority check. The persistent proof binds a
// stable OS identity, never just the database path or a now-expired selection.
func desktopWorkspaceRootIdentity(ctx context.Context, path string) (string, error) {
	if !desktopWorkspacePath(path) {
		return "", desktopWorkspaceRootInvalid()
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", desktopWorkspaceRootUnavailable()
	}
	if !workspaceTreeSamePath(path, resolved) {
		return "", desktopWorkspaceRootInvalid()
	}
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() {
		return "", desktopWorkspaceRootUnavailable()
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return "", desktopWorkspaceRootInvalid()
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", desktopWorkspaceRootUnavailable()
	}
	defer root.Close()
	opened, err := root.Lstat(".")
	if err != nil || !os.SameFile(before, opened) {
		return "", desktopWorkspaceRootChanged()
	}
	identity, err := workspaceTreeFileIdentity(ctx, root, ".", opened)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", desktopWorkspaceRootUnavailable()
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) {
		return "", desktopWorkspaceRootChanged()
	}
	resolved, err = filepath.EvalSymlinks(path)
	if err != nil || !workspaceTreeSamePath(path, resolved) {
		return "", desktopWorkspaceRootChanged()
	}
	return identity, ctx.Err()
}

func (s *Service) validateDesktopWorkspaceGrant(grant WorkspaceDesktopGrant, p Principal, project string, checkedAt time.Time) error {
	invalidGrant := func() error { return desktopWorkspaceError(403, "grant_invalid", "Desktop workspace grant is invalid") }
	if grant.Protocol != desktopWorkspaceProtocol || !desktopWorkspaceOperation(grant.OperationID) || !desktopWorkspaceHex(grant.UserID, 32) || !desktopWorkspaceHex(grant.TenantID, 32) || !desktopWorkspaceHex(grant.ProjectID, 32) || grant.UserID != p.ID || grant.TenantID != p.TenantID || grant.ProjectID != project || !desktopWorkspaceHex(grant.Nonce, 32) || !desktopWorkspaceHex(grant.Signature, 64) {
		return invalidGrant()
	}
	want := desktopWorkspaceMAC(s.DesktopWorkspaceKey, grant.Protocol, grant.OperationID, grant.UserID, grant.TenantID, grant.ProjectID, grant.Path, grant.ExpiresAt, grant.Nonce)
	if !hmac.Equal([]byte(grant.Signature), []byte(want)) {
		return invalidGrant()
	}
	expires, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || len(grant.ExpiresAt) > 40 || expires.After(checkedAt.Add(180*time.Second)) {
		return invalidGrant()
	}
	if !expires.After(checkedAt) {
		return desktopWorkspaceError(410, "grant_expired", "Desktop workspace grant has expired; choose the directory again")
	}
	if !desktopWorkspacePath(grant.Path) {
		return desktopWorkspaceInvalid()
	}
	return nil
}

func (s *Service) InitializeDesktopWorkspaces(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_desktop_workspace_roots(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,root_identity VARCHAR(256) NOT NULL,registered_by VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,signature VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id))`))
	return err
}

func (s *Service) BindDesktopWorkspace(parent context.Context, actor Actor, project string, request WorkspaceDesktopBindingRequest) (WorkspaceMetadata, error) {
	var out WorkspaceMetadata
	ctx, tx, cancel, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectApprove)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	if actor.kind != actorBrowser || !orgAdmin(p) {
		return out, forbidden()
	}
	if !s.desktopWorkspaceAvailable() {
		return out, desktopWorkspaceError(503, "unavailable", "Desktop workspace selection is unavailable")
	}
	if request.ProjectID != project || request.IdempotencyKey != request.Grant.OperationID || !desktopWorkspaceOperation(request.IdempotencyKey) || request.ExpectedBindingSHA256 != "" && !desktopWorkspaceHex(request.ExpectedBindingSHA256, 64) {
		return out, desktopWorkspaceInvalid()
	}
	if err = s.workspaceLock(ctx, tx, p, project); err != nil {
		return out, err
	}
	// Current authority precedes a receipt; an exact committed receipt precedes
	// fresh grant expiry/CAS checks. Recovery returns that original ACK only and
	// must never reselect an old directory after a later binding has committed.
	stored, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"kind": "workspace.desktop_binding", "request": request})
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code == "idempotency_conflict" {
			err = desktopWorkspaceError(409, "operation_conflict", "Desktop workspace operation ID was already used for different content")
		}
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(stored), &out)
		return out, err
	}
	checkedAt, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = s.validateDesktopWorkspaceGrant(request.Grant, p, project, checkedAt); err != nil {
		return out, err
	}
	state, err := s.workspaceBindingStateTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	if state.sha256 != request.ExpectedBindingSHA256 {
		return out, desktopWorkspaceError(409, "binding_changed", "Workspace binding changed; reload before choosing a directory")
	}
	identity, err := desktopWorkspaceRootIdentity(ctx, request.Grant.Path)
	if err != nil {
		return out, err
	}
	// Pending accepted saves retain the old root's current permission. An invalid
	// old proof blocks their recovery; choosing a new directory cannot bypass it.
	if err = s.recoverWorkspace(ctx, tx, p, project); err != nil {
		return out, err
	}
	proof := desktopWorkspaceRootProof{Root: request.Grant.Path, Identity: identity, RegisteredBy: p.ID, OperationID: request.Grant.OperationID, CreatedAt: automationStamp(checkedAt)}
	proof.Signature = s.signDesktopWorkspaceRoot(p.TenantID, project, proof)
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`), p.TenantID, project); err == nil {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_desktop_workspace_roots(tenant_id,project_id,root_path,root_identity,registered_by,operation_id,signature,created_at) VALUES(?,?,?,?,?,?,?,?)`), p.TenantID, project, proof.Root, proof.Identity, proof.RegisteredBy, proof.OperationID, proof.Signature, proof.CreatedAt)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), p.TenantID, project)
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_bindings(tenant_id,project_id,root_path) VALUES(?,?,?)`), p.TenantID, project, proof.Root)
	}
	if err != nil {
		return out, err
	}
	state = workspaceBindingState{root: proof.Root, proof: &proof, sha256: digest(proof.Root + proof.Signature)}
	if err = s.validateWorkspaceBindingRoot(ctx, p.TenantID, project, state); err != nil {
		return out, err
	}
	out = s.workspaceBindingMetadata(project, state)
	out.Configured, out.Name = true, filepath.Base(proof.Root)
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, stored, out); err != nil {
		return out, err
	}
	err = tx.Commit()
	return out, err
}
