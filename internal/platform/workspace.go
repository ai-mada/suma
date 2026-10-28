package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// Reject every symlink component, even when its destination is inside the jail.
func workspaceFile(root, rel string) (string, error) {
	resolved, err := filepath.EvalSymlinks(root)
	sameRoot := filepath.Clean(resolved) == filepath.Clean(root)
	if runtime.GOOS == "windows" {
		sameRoot = strings.EqualFold(filepath.Clean(resolved), filepath.Clean(root))
	}
	if err != nil || !sameRoot {
		return "", forbidden()
	}
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, ":") || strings.ContainsRune(rel, 0) || forbiddenPath(rel) {
		return "", forbidden()
	}
	parts := strings.Split(strings.ReplaceAll(rel, "\\", "/"), "/")
	for _, part := range parts {
		if part == ".." {
			return "", missing()
		}
		if part == "" || part == "." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", forbidden()
		}
	}
	target := root
	for i, part := range parts {
		target = filepath.Join(target, part)
		info, e := os.Lstat(target)
		if e != nil {
			return "", missing()
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", forbidden()
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", forbidden()
		}
	}
	if !under(root, target) {
		return "", forbidden()
	}
	return target, nil
}
func (s *Service) workspaceReadGuard(ctx context.Context, actor Actor, project, expected string) (root string, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		e := auth.tx.QueryRowContext(ctx, s.q(`SELECT root_path FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), auth.currentPrincipal.TenantID, project).Scan(&root)
		if errors.Is(e, sql.ErrNoRows) {
			return &APIError{422, "workspace_not_configured", "请先配置当前项目的工作区", nil}
		}
		if e != nil {
			return e
		}
		if expected != "" && root != expected {
			return conflict("工作区绑定已变化，请重新加载")
		}
		return nil
	})
	return root, err
}

var errWorkspaceDisabled = forbidden()

func (s *Service) workspaceRoot(ctx context.Context, actor Actor, project string) (string, error) {
	if strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT")) == "" {
		if err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(*authorizedCommand) error { return nil }); err != nil {
			return "", err
		}
		return "", errWorkspaceDisabled
	}
	root, e := s.workspaceReadGuard(ctx, actor, project, "")
	if e != nil {
		return "", e
	}
	if strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT")) == "" {
		return "", forbidden()
	}
	resolved, e := filepath.EvalSymlinks(root)
	if e != nil {
		return "", invalid("工作区目录不可用")
	}
	allowed, e := allowedWorkspaceRoot()
	if e != nil || !under(allowed, resolved) {
		return "", forbidden()
	}
	info, e := os.Stat(resolved)
	if e != nil || !info.IsDir() {
		return "", invalid("工作区目录不可用")
	}
	return resolved, nil

}

func (s *Service) InitializeWorkspace(ctx context.Context) error {
	payloadType := "TEXT"
	if s.Dialect == "oceanbase" {
		payloadType = "MEDIUMTEXT"
	}
	_, e := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_file_operations (tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,path TEXT NOT NULL,expected_hash VARCHAR(64) NOT NULL,new_hash VARCHAR(64) NOT NULL,content `+payloadType+` NOT NULL,response `+payloadType+` NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_accepted_operations(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,accepted_by VARCHAR(64) NOT NULL,accepted_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
	return e
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

type workspaceOperation struct{ key, root, path, expected, next, content, response string }

func fileResponse(rel, content string) map[string]any {
	lang := map[string]string{".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript", ".go": "go", ".py": "python", ".java": "java", ".rs": "rust", ".md": "markdown", ".json": "json", ".yaml": "yaml", ".yml": "yaml", ".css": "css", ".html": "html", ".cpp": "cpp", ".c": "c"}[filepath.Ext(rel)]
	if lang == "" {
		lang = "plaintext"
	}
	return map[string]any{"path": filepath.ToSlash(rel), "content": content, "hash": digest(content), "language": lang}
}
func readWorkspaceFile(target string) (string, error) {
	f, e := os.Open(target)
	if e != nil {
		return "", e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return "", e
	}
	if len(b) > 1<<20 || strings.ContainsRune(string(b), 0) || !utf8.Valid(b) {
		return "", invalid("编辑器仅打开 1 MiB 以内的文本文件")
	}
	return string(b), nil
}
func replaceWorkspaceFile(root, rel, expected, content string) error {
	target, e := workspaceFile(root, rel)
	if e != nil {
		return e
	}
	info, e := os.Stat(target)
	if e != nil {
		return e
	}
	temp, e := os.CreateTemp(filepath.Dir(target), ".suma-save-*")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	if e = temp.Chmod(info.Mode().Perm()); e == nil {
		_, e = temp.WriteString(content)
	}
	if e == nil {
		e = temp.Sync()
	}
	closed := temp.Close()
	if e == nil {
		e = closed
	}
	if e != nil {
		return e
	}
	checked, e := workspaceFile(root, rel)
	if e != nil {
		return e
	}
	current, e := readWorkspaceFile(checked)
	if e != nil {
		return e
	}
	if digest(current) != expected {
		return conflict("文件保存冲突，请重新加载")
	}
	if e = os.Rename(temp.Name(), checked); e != nil {
		return e
	}
	// Windows does not expose portable directory fsync through os.File.Sync.
	if runtime.GOOS != "windows" {
		dir, e := os.Open(filepath.Dir(checked))
		if e != nil {
			return e
		}
		e = dir.Sync()
		dir.Close()
		return e
	}
	return nil
}

// Recover under the project lock before any later save. Unknown external edits
// require reconciliation instead of inventing either success or failure.
func (s *Service) recoverWorkspace(ctx context.Context, tx *sql.Tx, p Principal, project string) error {
	rows, e := tx.QueryContext(ctx, s.q(`SELECT operation_id,root_path,path,expected_hash,new_hash,content,response FROM suma_workspace_file_operations WHERE tenant_id=? AND project_id=? AND state='prepared' ORDER BY created_at,operation_id`), p.TenantID, project)
	if e != nil {
		return e
	}
	var pending []workspaceOperation
	for rows.Next() {
		var op workspaceOperation
		if e = rows.Scan(&op.key, &op.root, &op.path, &op.expected, &op.next, &op.content, &op.response); e != nil {
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
		allowed, e := allowedWorkspaceRoot()
		if e != nil || !under(allowed, op.root) {
			return forbidden()
		}
		target, e := workspaceFile(op.root, op.path)
		if e != nil {
			return e
		}
		current, e := readWorkspaceFile(target)
		if e != nil {
			return e
		}
		hash := digest(current)
		if hash == op.expected {
			if e = replaceWorkspaceFile(op.root, op.path, op.expected, op.content); e != nil {
				return e
			}
		} else if hash != op.next {
			return &APIError{409, "workspace_reconciliation_required", "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", map[string]any{"operation_id": op.key, "path": op.path}}
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
	if _, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_file_operations(tenant_id,project_id,operation_id,request_hash,root_path,path,expected_hash,new_hash,content,response,state,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), p.TenantID, project, key, hash, root, rel, digest(current), digest(content), content, stringify(out), "prepared", automationStamp(auth.checkedAt)); err != nil {
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
	if e = s.recoverWorkspace(ctx, tx, p, project); e != nil {
		return nil, e
	}
	if stored != "" {
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		var out map[string]any
		e = json.Unmarshal([]byte(response), &out)
		return out, e
	}
	// Use the transaction to avoid acquiring a second connection from a
	// single-connection SQLite pool while the writer lock is held.
	var root string
	if e = tx.QueryRowContext(ctx, s.q(`SELECT root_path FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&root); e != nil {
		return nil, e
	}
	allowed, e := allowedWorkspaceRoot()
	if e != nil || !under(allowed, root) {
		return nil, forbidden()
	}
	target, e := workspaceFile(root, rel)
	if e != nil {
		return nil, e
	}
	current, e := readWorkspaceFile(target)
	if e != nil {
		return nil, e
	}
	if str(m, "expected_hash") != digest(current) {
		return nil, conflict("文件已被其他操作修改，请比较后保存")
	}
	out := fileResponse(rel, content)
	if e = s.prepareWorkspaceOperationTx(ctx, auth, key, hash, root, rel, current, content, out); e != nil {
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
func (s *Server) workspace(w http.ResponseWriter, r *http.Request, actor Actor, project, path string, m map[string]any) {
	ctx := r.Context()
	p := actor.Projection()
	if path == "workspace/binding" && r.Method == "POST" {
		allowed := os.Getenv("SUMA_WORKSPACE_ROOT")
		if allowed == "" {
			writeError(w, invalid("服务端尚未配置 SUMA_WORKSPACE_ROOT"))
			return
		}
		allowed, e := filepath.Abs(allowed)
		if e == nil {
			allowed, e = filepath.EvalSymlinks(allowed)
		}
		if e != nil {
			writeError(w, e)
			return
		}
		target := str(m, "path")
		if target == "" {
			target = allowed
		}
		target, e = filepath.Abs(target)
		if e == nil {
			target, e = filepath.EvalSymlinks(target)
		}
		if e != nil || !under(allowed, target) {
			writeError(w, invalid("工作区必须位于服务端允许根目录内"))
			return
		}
		info, e := os.Stat(target)
		if e != nil || !info.IsDir() {
			writeError(w, invalid("工作区必须是目录"))
			return
		}
		ctx, tx, cancel, e := s.Service.beginAuthorityTx(ctx)
		if e != nil {
			writeError(w, e)
			return
		}
		defer cancel()
		defer tx.Rollback()
		auth, e := s.Service.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project}, purposeProjectApprove)
		if e != nil {
			writeError(w, e)
			return
		}
		p = auth.currentPrincipal
		if !orgAdmin(p) {
			writeError(w, forbidden())
			return
		}
		if e = s.Service.workspaceLock(ctx, tx, p, project); e == nil {
			e = s.Service.recoverWorkspace(ctx, tx, p, project)
		}
		if e == nil {
			_, e = tx.ExecContext(ctx, s.Service.q(`DELETE FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), p.TenantID, project)
		}
		if e == nil {
			_, e = tx.ExecContext(ctx, s.Service.q(`INSERT INTO suma_workspace_bindings(tenant_id,project_id,root_path)VALUES(?,?,?)`), p.TenantID, project, target)
		}
		if e == nil {
			e = tx.Commit()
		}
		respond(w, 200, map[string]any{"configured": e == nil, "root": target}, e)
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
	root, e := s.Service.workspaceRoot(ctx, actor, project)
	if e != nil {
		if automationAuthorityFailure(e) && !errors.Is(e, errWorkspaceDisabled) {
			writeError(w, e)
			return
		}
		if path == "workspace" {
			writeJSON(w, 200, WorkspaceMetadata{Configured: false, Reason: e.Error(), BindingAvailable: os.Getenv("SUMA_WORKSPACE_ROOT") != ""})
			return
		}
		writeError(w, e)
		return
	}
	if path == "workspace" {
		if _, e = s.Service.workspaceReadGuard(ctx, actor, project, root); e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, WorkspaceMetadata{Configured: true, BindingAvailable: true, Name: filepath.Base(root)})
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
	rel := r.URL.Query().Get("path")
	target, e := workspaceFile(root, rel)
	if e != nil {
		writeError(w, e)
		return
	}
	content, e := readWorkspaceFile(target)
	if e == nil {
		_, e = s.Service.workspaceReadGuard(ctx, actor, project, root)
	}
	respond(w, 200, fileResponse(rel, content), e)
}
