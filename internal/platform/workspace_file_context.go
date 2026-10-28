package platform

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
)

// WorkspaceFileDocument carries the exact directory context observed while
// reading. ContextSHA256 is a concurrency basis, never an authorization token.
type WorkspaceFileDocument struct {
	ProjectID     string `json:"project_id"`
	Path          string `json:"path"`
	Content       string `json:"content"`
	Hash          string `json:"hash"`
	Language      string `json:"language"`
	WorkspaceName string `json:"workspace_name"`
	ContextSHA256 string `json:"context_sha256"`
}

type WorkspaceFileSaveRequest struct {
	ProjectID             string `json:"project_id"`
	Path                  string `json:"path"`
	Content               string `json:"content"`
	ExpectedHash          string `json:"expected_hash"`
	ExpectedContextSHA256 string `json:"expected_context_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}

type workspaceFileContext struct {
	root, name, sha256 string
}

// Derive the digest from the actual opened descriptor. A fresh binding ID
// prevents A -> B -> A (including a repeated selection of A) reviving an old
// editor context, even when paths, file contents and OS identities are equal.
func (s *Service) openWorkspaceFileContextTx(ctx context.Context, tx *sql.Tx, tenant, project, expectedRoot string) (*os.Root, workspaceFileContext, error) {
	var basis workspaceFileContext
	state, err := s.workspaceBindingStateTx(ctx, tx, tenant, project)
	if err != nil {
		return nil, basis, err
	}
	root, err := s.openWorkspaceFileRootTx(ctx, tx, tenant, project, expectedRoot)
	if err != nil {
		return nil, basis, err
	}
	info, err := root.Lstat(".")
	if err != nil || !info.IsDir() || !desktopWorkspaceHex(state.bindingID, 32) {
		root.Close()
		return nil, basis, forbidden()
	}
	identity, err := workspaceTreeFileIdentity(ctx, root, ".", info)
	if err != nil {
		root.Close()
		return nil, basis, err
	}
	basis = workspaceFileContext{root: state.root, name: filepath.Base(state.root), sha256: digest("suma-workspace-file-context-v1\x00" + stringify([]string{tenant, project, state.bindingID, identity}))}
	return root, basis, nil
}

func (s *Service) verifyWorkspaceFileContextTx(ctx context.Context, tx *sql.Tx, tenant, project string, expected workspaceFileContext) error {
	root, current, err := s.openWorkspaceFileContextTx(ctx, tx, tenant, project, expected.root)
	if err != nil {
		return err
	}
	root.Close()
	if current.sha256 != expected.sha256 {
		return conflict("工作区绑定已变化，请重新加载")
	}
	return ctx.Err()
}

func workspaceDocument(project, rel, content string, basis workspaceFileContext) WorkspaceFileDocument {
	fields := fileResponse(rel, content)
	return WorkspaceFileDocument{ProjectID: project, Path: str(fields, "path"), Content: content, Hash: digest(content), Language: str(fields, "language"), WorkspaceName: basis.name, ContextSHA256: basis.sha256}
}

func workspaceDocumentMap(document WorkspaceFileDocument) map[string]any {
	return map[string]any{"project_id": document.ProjectID, "path": document.Path, "content": document.Content, "hash": document.Hash, "language": document.Language, "workspace_name": document.WorkspaceName, "context_sha256": document.ContextSHA256}
}

// Filesystem work runs between two fresh current-authority checks. The second
// opens the current root again and compares both the binding and OS identity
// before any bytes are published to the caller.
func (s *Service) GetWorkspaceFile(ctx context.Context, actor Actor, project, rel string) (WorkspaceFileDocument, error) {
	var opened *os.Root
	var basis workspaceFileContext
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	err := s.withCurrentRead(ctx, actor, target, purposeProjectRead, func(auth *authorizedCommand) error {
		if opened != nil {
			opened.Close()
		}
		var err error
		opened, basis, err = s.openWorkspaceFileContextTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, "")
		return err
	})
	if opened != nil {
		defer opened.Close()
	}
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	content, _, err := readWorkspaceRootedFile(ctx, opened, rel)
	if err == nil {
		err = s.withCurrentRead(ctx, actor, target, purposeProjectRead, func(auth *authorizedCommand) error {
			return s.verifyWorkspaceFileContextTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, basis)
		})
	}
	if err != nil {
		return WorkspaceFileDocument{}, err
	}
	return workspaceDocument(project, rel, content, basis), nil
}

func workspaceContextChanged(project, rel, key, expected string) error {
	return &APIError{409, "workspace_context_changed", "Workspace context changed; reload the file before saving", map[string]any{"project_id": project, "path": rel, "operation_id": key, "expected_context_sha256": expected, "not_accepted": true}}
}

func workspaceReconciliation(op workspaceOperation) error {
	return &APIError{409, "workspace_reconciliation_required", "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", map[string]any{"operation_id": op.key, "path": op.path}}
}

// This runs only after current authority, project locking and original receipt
// lookup. Malformed new requests cannot replace the semantics of an original ACK.
func workspaceSaveRequest(project string, m map[string]any) (WorkspaceFileSaveRequest, error) {
	var out WorkspaceFileSaveRequest
	if !desktopWorkspaceHex(str(m, "expected_context_sha256"), 64) {
		return out, invalid("expected_context_sha256 must be a 64-character lowercase SHA-256 digest")
	}
	allowed := map[string]bool{"project_id": true, "path": true, "content": true, "expected_hash": true, "expected_context_sha256": true, "idempotency_key": true}
	if len(m) != len(allowed) {
		return out, invalid("Workspace file request is invalid")
	}
	for field, value := range m {
		if _, ok := value.(string); !ok || !allowed[field] {
			return out, invalid("Workspace file request is invalid")
		}
	}
	if str(m, "project_id") != project || str(m, "path") == "" || !desktopWorkspaceHex(str(m, "expected_hash"), 64) {
		return out, invalid("Workspace file request is invalid")
	}
	out = WorkspaceFileSaveRequest{ProjectID: project, Path: str(m, "path"), Content: str(m, "content"), ExpectedHash: str(m, "expected_hash"), ExpectedContextSHA256: str(m, "expected_context_sha256"), IdempotencyKey: str(m, "idempotency_key")}
	return out, nil
}
