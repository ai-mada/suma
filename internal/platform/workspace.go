package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func under(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func allowedWorkspaceRoot() (string, error) {
	configured := strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT"))
	if configured == "" {
		return "", forbidden()
	}
	abs, err := filepath.Abs(configured)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
func forbiddenPath(path string) bool {
	for _, part := range strings.Split(strings.ReplaceAll(path, "\\", "/"), "/") {
		part = strings.ToLower(part)
		switch part {
		case ".git", ".suma", ".cache", "node_modules", ".aws", ".ssh", ".pnpm-store", ".codex", ".agents", ".kube", ".azure", ".config", ".docker", ".npmrc", ".pypirc", ".netrc", ".git-credentials", "credentials", "credentials.json", "secrets", "secrets.json", "id_rsa", "id_ed25519":
			return true
		}
		if strings.HasPrefix(part, ".env") || strings.HasPrefix(part, ".suma-save-") || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") || strings.HasSuffix(part, ".p12") || strings.HasSuffix(part, ".pfx") {
			return true
		}
	}
	return false
}

func (s *Service) workspaceReadGuard(ctx context.Context, actor Actor, project, expected string) (root string, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		root, e = s.workspaceRootAuthorityTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, expected)
		return e
	})
	return root, err
}

var errWorkspaceDisabled = forbidden()

func (s *Service) workspaceRoot(ctx context.Context, actor Actor, project string) (string, error) {
	return s.workspaceReadGuard(ctx, actor, project, "")
}

func (s *Service) InitializeWorkspace(ctx context.Context) error {
	if err := s.validateWorkspaceInitialSchema(ctx); err != nil {
		return err
	}
	return s.initializeWorkspace(ctx)
}

func (s *Service) initializeWorkspace(ctx context.Context) error {
	payloadType := "TEXT"
	if s.Dialect == "oceanbase" {
		payloadType = "MEDIUMTEXT"
	}
	// This first workspace table distinguishes the current initial shape from
	// an older installation. Creating it first lets an interrupted fresh setup
	// resume without interpreting an existing installation as an upgrade.
	_, e := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_file_rejections(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,path TEXT NOT NULL,expected_context_sha256 VARCHAR(64) NOT NULL,rejected_by VARCHAR(64) NOT NULL,rejected_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_file_operations (tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,context_sha256 VARCHAR(64) NOT NULL,path TEXT NOT NULL,expected_hash VARCHAR(64) NOT NULL,new_hash VARCHAR(64) NOT NULL,content `+payloadType+` NOT NULL,response `+payloadType+` NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_accepted_operations(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,accepted_by VARCHAR(64) NOT NULL,accepted_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
	if e != nil {
		return e
	}
	return s.InitializeDesktopWorkspaces(ctx)
}

// Writing before reading obtains SQLite's writer lock or the project row lock.
func (s *Service) workspaceLock(ctx context.Context, tx *sql.Tx, p Principal, project string) error {
	_, e := tx.ExecContext(ctx, s.q(`UPDATE suma_projects SET version=version WHERE tenant_id=? AND id=?`), p.TenantID, project)
	if e != nil {
		return e
	}
	var id string
	e = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_projects WHERE tenant_id=? AND id=?`), p.TenantID, project).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return missing()
	}
	return e
}

type workspaceOperation struct{ key, root, contextSHA256, path, expected, next, content, response string }

func fileResponse(rel, content string) map[string]any {
	lang := map[string]string{".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript", ".go": "go", ".py": "python", ".java": "java", ".rs": "rust", ".md": "markdown", ".json": "json", ".yaml": "yaml", ".yml": "yaml", ".css": "css", ".html": "html", ".cpp": "cpp", ".c": "c"}[filepath.Ext(rel)]
	if lang == "" {
		lang = "plaintext"
	}
	return map[string]any{"path": filepath.ToSlash(rel), "content": content, "hash": digest(content), "language": lang}
}

// Recover under the project lock before any later save. Unknown external edits
// require reconciliation instead of inventing either success or failure.
func (s *Service) recoverWorkspace(ctx context.Context, tx *sql.Tx, p Principal, project string) error {
	rows, e := tx.QueryContext(ctx, s.q(`SELECT operation_id,root_path,context_sha256,path,expected_hash,new_hash,content,response FROM suma_workspace_file_operations WHERE tenant_id=? AND project_id=? AND state='prepared' ORDER BY created_at,operation_id`), p.TenantID, project)
	if e != nil {
		return e
	}
	var pending []workspaceOperation
	for rows.Next() {
		var op workspaceOperation
		if e = rows.Scan(&op.key, &op.root, &op.contextSHA256, &op.path, &op.expected, &op.next, &op.content, &op.response); e != nil {
			rows.Close()
			return e
		}
		pending = append(pending, op)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, op := range pending {
		var accepted string
		if e = tx.QueryRowContext(ctx, s.q(`SELECT a.request_hash FROM suma_workspace_accepted_operations a JOIN suma_workspace_file_operations o ON o.tenant_id=a.tenant_id AND o.project_id=a.project_id AND o.operation_id=a.operation_id AND o.request_hash=a.request_hash WHERE a.tenant_id=? AND a.project_id=? AND a.operation_id=?`), p.TenantID, project, op.key).Scan(&accepted); e != nil {
			return &APIError{409, "workspace_reconciliation_required", "先前保存缺少可信受理记录，需要核对", nil}
		}
		if e = s.recoverWorkspaceFile(ctx, tx, p, project, op); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_file_operations SET state='completed' WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, project, op.key); e != nil {
			return e
		}
	}
	return nil
}

// Only a current transaction-bound interactive command can accept a durable file
// operation. Recovery consumes this exact persisted scope/hash, never a Principal.
func (s *Service) prepareWorkspaceOperationTx(ctx context.Context, auth *authorizedCommand, key, hash, root, rel, current, content string, out map[string]any) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	contextSHA256 := str(out, "context_sha256")
	if !desktopWorkspaceHex(contextSHA256, 64) {
		return invalid("expected_context_sha256 must be a 64-character lowercase SHA-256 digest")
	}
	if err := s.verifyWorkspaceFileContextTx(ctx, auth.tx, p.TenantID, project, workspaceFileContext{root: root, sha256: contextSHA256}); err != nil {
		return err
	}
	if _, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_file_operations(tenant_id,project_id,operation_id,request_hash,root_path,context_sha256,path,expected_hash,new_hash,content,response,state,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), p.TenantID, project, key, hash, root, contextSHA256, rel, digest(current), digest(content), content, stringify(out), "prepared", automationStamp(auth.checkedAt)); err != nil {
		return err
	}
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_accepted_operations(tenant_id,project_id,operation_id,request_hash,accepted_by,accepted_at)VALUES(?,?,?,?,?,?)`), p.TenantID, project, key, hash, p.ID, automationStamp(auth.checkedAt))
	return err
}
func (s *Service) saveWorkspace(ctx context.Context, actor Actor, project string, m map[string]any) (map[string]any, error) {
	p := actor.Projection()
	parentCtx := ctx
	key, rel, content := str(m, "idempotency_key"), str(m, "path"), str(m, "content")
	if len(key) < 8 || len(key) > 128 {
		return nil, invalid("请提供 8–128 字符的 idempotency_key")
	}
	if len(content) > 1<<20 || strings.ContainsRune(content, 0) || !utf8.ValidString(content) {
		return nil, invalid("仅支持 1 MiB 以内的文本内容")
	}
	hash := digest(p.ID + stringify(map[string]any{"kind": "workspace.file", "body": m}))
	ctx, tx, cancel, e := s.beginAuthorityTx(parentCtx)
	if e != nil {
		return nil, e
	}
	defer cancel()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project}, purposeProjectWrite)
	if e != nil {
		return nil, e
	}
	p = auth.currentPrincipal
	if e = s.workspaceLock(ctx, tx, p, project); e != nil {
		return nil, e
	}
	var stored, response, state string
	e = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response,state FROM suma_workspace_file_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, project, key).Scan(&stored, &response, &state)
	if e == nil {
		if stored != hash {
			return nil, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
		}
		if state == "completed" {
			var out map[string]any
			e = json.Unmarshal([]byte(response), &out)
			return out, e
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if stored != "" {
		if e = s.recoverWorkspace(ctx, tx, p, project); e != nil {
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		var out map[string]any
		e = json.Unmarshal([]byte(response), &out)
		return out, e
	}
	// An accepted record without its journal is uncertain, never proof that
	// this operation was not accepted. Original lookup always precedes a new
	// context comparison, including when the current binding has changed.
	var accepted string
	e = tx.QueryRowContext(ctx, s.q(`SELECT request_hash FROM suma_workspace_accepted_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, project, key).Scan(&accepted)
	if e == nil {
		return nil, workspaceReconciliation(workspaceOperation{key: key, path: rel})
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	// A definitive rejection is durable too: physical A -> B -> A can restore
	// the original OS identity without a new binding. An old packet must never
	// become acceptable after the client has received its not-accepted proof.
	var rejectedHash, rejectedPath, rejectedContext string
	e = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,path,expected_context_sha256 FROM suma_workspace_file_rejections WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, project, key).Scan(&rejectedHash, &rejectedPath, &rejectedContext)
	if e == nil {
		if rejectedHash != hash {
			return nil, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
		}
		return nil, workspaceContextChanged(project, rejectedPath, key, rejectedContext)
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	request, e := workspaceSaveRequest(project, m)
	if e != nil {
		return nil, e
	}
	// Use the transaction to avoid acquiring a second connection from a
	// single-connection SQLite pool while the writer lock is held.
	jail, basis, e := s.openWorkspaceFileContextTx(ctx, tx, p.TenantID, project, "")
	if e != nil {
		return nil, e
	}
	if basis.sha256 != request.ExpectedContextSHA256 {
		jail.Close()
		if e = auth.assertTx(tx, auth.target, purposeProjectWrite); e != nil {
			return nil, e
		}
		if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_file_rejections(tenant_id,project_id,operation_id,request_hash,path,expected_context_sha256,rejected_by,rejected_at) VALUES(?,?,?,?,?,?,?,?)`), p.TenantID, project, key, hash, rel, request.ExpectedContextSHA256, p.ID, automationStamp(auth.checkedAt)); e != nil {
			return nil, e
		}
		// Publish proof only after the rejection is durable. A failed commit is
		// uncertainty, so it must return an error without not_accepted details.
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return nil, workspaceContextChanged(project, rel, key, request.ExpectedContextSHA256)
	}
	// Reject a stale new request before recovery can write any older prepared
	// operation. Accepted operations continue through their separate branch.
	if e = s.recoverWorkspace(ctx, tx, p, project); e != nil {
		jail.Close()
		return nil, e
	}
	current, _, e := readWorkspaceRootedFile(ctx, jail, rel)
	jail.Close()
	if e != nil {
		return nil, e
	}
	if e = s.verifyWorkspaceFileContextTx(ctx, tx, p.TenantID, project, basis); e != nil {
		return nil, e
	}
	if str(m, "expected_hash") != digest(current) {
		return nil, conflict("文件已被其他操作修改，请比较后保存")
	}
	out := workspaceDocumentMap(workspaceDocument(project, rel, content, basis))
	if e = s.prepareWorkspaceOperationTx(ctx, auth, key, hash, basis.root, rel, current, content, out); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	cancel()
	// Durable preparation survives crashes on either side of rename. Other
	// requests can safely finish it before this request reacquires the lock.
	finishCtx, finish, finishCancel, e := s.beginAuthorityTx(parentCtx)
	if e != nil {
		return nil, e
	}
	defer finishCancel()
	defer finish.Rollback()
	finishAuth, e := s.authorizeTx(finishCtx, finish, actor, commandTarget{tenantID: p.TenantID, projectID: project}, purposeProjectWrite)
	if e != nil {
		return nil, e
	}
	p = finishAuth.currentPrincipal
	if e = s.workspaceLock(finishCtx, finish, p, project); e != nil {
		return nil, e
	}
	if e = s.recoverWorkspace(finishCtx, finish, p, project); e != nil {
		return nil, e
	}
	if e = finish.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Service) bindWorkspace(parent context.Context, actor Actor, project string, request map[string]any) (WorkspaceMetadata, error) {
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
	if !orgAdmin(p) {
		return out, forbidden()
	}
	if err = s.workspaceLock(ctx, tx, p, project); err != nil {
		return out, err
	}
	key := str(request, "idempotency_key")
	stored, replay, err := s.commandReceipt(ctx, auth, key, map[string]any{"kind": "workspace.binding", "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(stored), &out)
		return out, err
	}
	// A committed receipt is an original operation fact. Only a new binding
	// needs the configured directory to remain available and permissible.
	allowed := os.Getenv("SUMA_WORKSPACE_ROOT")
	if allowed == "" {
		return out, invalid("服务端尚未配置 SUMA_WORKSPACE_ROOT")
	}
	allowed, err = filepath.Abs(allowed)
	if err == nil {
		allowed, err = filepath.EvalSymlinks(allowed)
	}
	if err != nil {
		return out, err
	}
	target := str(request, "path")
	if target == "" {
		target = allowed
	}
	target, err = filepath.Abs(target)
	if err == nil {
		target, err = filepath.EvalSymlinks(target)
	}
	if err != nil || !under(allowed, target) {
		return out, invalid("工作区必须位于服务端允许根目录内")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return out, invalid("工作区必须是目录")
	}
	if err = s.recoverWorkspace(ctx, tx, p, project); err != nil {
		return out, err
	}
	// An explicit ordinary binding chooses the configured authority source;
	// a bad native proof never disappears through an implicit read fallback.
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`), p.TenantID, project); err == nil {
		_, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), p.TenantID, project)
	}
	state := workspaceBindingState{root: target, bindingID: newID()}
	state.sha256 = workspaceBindingDigest(state)
	if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_bindings(tenant_id,project_id,root_path,binding_id)VALUES(?,?,?,?)`), p.TenantID, project, target, state.bindingID)
	}
	if err != nil {
		return out, err
	}
	out = s.workspaceBindingMetadata(project, state)
	out.Configured, out.Name = true, filepath.Base(target)
	if err = s.saveCommandReceipt(ctx, auth, key, stored, out); err != nil {
		return out, err
	}
	err = tx.Commit()
	return out, err
}

func (s *Server) workspace(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) {
	ctx := r.Context()
	if path == "workspace/binding" && r.Method == "POST" {
		out, err := s.Service.bindWorkspace(ctx, actor, project, m)
		respond(w, http.StatusOK, out, err)
		return
	}
	if path != "workspace" && path != "workspace/file" && path != "workspace/tree" {
		writeError(w, missing())
		return
	}
	if path == "workspace/file" && r.Method == "POST" {
		out, e := s.Service.saveWorkspace(ctx, actor, project, m)
		respond(w, 200, out, e)
		return
	}
	if r.Method != "GET" {
		writeError(w, &APIError{405, "method_not_allowed", "不支持此请求方法", nil})
		return
	}
	if path == "workspace" || path == "workspace/tree" {
		if _, _, _, queryErr := workspaceTreeQuery(r, project, path == "workspace/tree"); queryErr != nil {
			writeError(w, queryErr)
			return
		}
	}
	if path == "workspace" {
		metadata, metadataErr := s.Service.GetWorkspaceMetadata(ctx, actor, project)
		respond(w, http.StatusOK, metadata, metadataErr)
		return
	}
	if path == "workspace/file" {
		document, readErr := s.Service.GetWorkspaceFile(ctx, actor, project, r.URL.Query().Get("path"))
		respond(w, http.StatusOK, document, readErr)
		return
	}
	root, e := s.Service.workspaceRoot(ctx, actor, project)
	if e != nil {
		writeError(w, e)
		return
	}
	if path == "workspace/tree" {
		directory, limit, before, queryErr := workspaceTreeQuery(r, project, true)
		if queryErr != nil {
			writeError(w, queryErr)
			return
		}
		page, readErr := s.Service.listWorkspaceTree(ctx, actor, project, root, directory, limit, before)
		respond(w, http.StatusOK, page, readErr)
		return
	}
}
