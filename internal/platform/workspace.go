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
	"sort"
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
func (s *Service) workspaceRoot(ctx context.Context, p Principal, project string) (string, error) {
	if strings.TrimSpace(os.Getenv("SUMA_WORKSPACE_ROOT")) == "" {
		return "", forbidden()
	}
	var root string
	e := s.DB.QueryRowContext(ctx, s.q(`SELECT root_path FROM suma_workspace_bindings WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&root)
	if errors.Is(e, sql.ErrNoRows) {
		return "", &APIError{422, "workspace_not_configured", "请先配置当前项目的工作区", nil}
	}
	if e != nil {
		return "", e
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
func (s *Service) MigrateWorkspace(ctx context.Context) error {
	payloadType := "TEXT"
	if s.Dialect == "oceanbase" {
		payloadType = "MEDIUMTEXT"
	}
	_, e := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS suma_workspace_file_operations (tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,root_path TEXT NOT NULL,path TEXT NOT NULL,expected_hash VARCHAR(64) NOT NULL,new_hash VARCHAR(64) NOT NULL,content `+payloadType+` NOT NULL,response `+payloadType+` NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id))`)
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
func (s *Service) saveWorkspace(ctx context.Context, p Principal, project string, m map[string]any) (map[string]any, error) {
	if e := s.Authorize(ctx, p, project, true, false); e != nil {
		return nil, e
	}
	key, rel, content := str(m, "idempotency_key"), str(m, "path"), str(m, "content")
	if len(key) < 8 || len(key) > 128 {
		return nil, invalid("请提供 8–128 字符的 idempotency_key")
	}
	if len(content) > 1<<20 || strings.ContainsRune(content, 0) || !utf8.ValidString(content) {
		return nil, invalid("仅支持 1 MiB 以内的文本内容")
	}
	hash := digest(p.ID + stringify(map[string]any{"kind": "workspace.file", "body": m}))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
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
	_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_file_operations(tenant_id,project_id,operation_id,request_hash,root_path,path,expected_hash,new_hash,content,response,state,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), p.TenantID, project, key, hash, root, rel, digest(current), digest(content), content, stringify(out), "prepared", now())
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	// Durable preparation survives crashes on either side of rename. Other
	// requests can safely finish it before this request reacquires the lock.
	finish, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer finish.Rollback()
	if e = s.workspaceLock(ctx, finish, p, project); e != nil {
		return nil, e
	}
	if e = s.recoverWorkspace(ctx, finish, p, project); e != nil {
		return nil, e
	}
	if e = finish.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Server) workspace(w http.ResponseWriter, r *http.Request, p Principal, project, path string, m map[string]any) {
	ctx := r.Context()
	if path == "workspace/binding" && r.Method == "POST" {
		if p.Role != "owner" && p.Role != "admin" {
			writeError(w, forbidden())
			return
		}
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
		tx, e := s.Service.DB.BeginTx(ctx, nil)
		if e != nil {
			writeError(w, e)
			return
		}
		defer tx.Rollback()
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
	if path != "workspace" && path != "workspace/file" {
		writeError(w, missing())
		return
	}
	if path == "workspace/file" && r.Method == "POST" {
		out, e := s.Service.saveWorkspace(ctx, p, project, m)
		respond(w, 200, out, e)
		return
	}
	if r.Method != "GET" {
		writeError(w, &APIError{405, "method_not_allowed", "不支持此请求方法", nil})
		return
	}
	root, e := s.Service.workspaceRoot(ctx, p, project)
	if e != nil {
		if path == "workspace" {
			writeJSON(w, 200, map[string]any{"configured": false, "files": []any{}, "reason": e.Error(), "binding_available": os.Getenv("SUMA_WORKSPACE_ROOT") != ""})
			return
		}
		writeError(w, e)
		return
	}
	if path == "workspace" {
		files := []map[string]any{}
		truncated := false
		filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if rel == "." {
				return nil
			}
			if forbiddenPath(rel) || d.Type()&os.ModeSymlink != 0 || strings.HasPrefix(d.Name(), ".suma-save-") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if len(files) >= 1000 {
				truncated = true
				return filepath.SkipAll
			}
			if !d.IsDir() {
				files = append(files, map[string]any{"path": filepath.ToSlash(rel), "name": d.Name(), "type": "file"})
			}
			return nil
		})
		sort.Slice(files, func(i, j int) bool { return files[i]["path"].(string) < files[j]["path"].(string) })
		writeJSON(w, 200, map[string]any{"configured": true, "root": root, "name": filepath.Base(root), "files": files, "limit": 1000, "truncated": truncated})
		return
	}
	rel := r.URL.Query().Get("path")
	target, e := workspaceFile(root, rel)
	if e != nil {
		writeError(w, e)
		return
	}
	content, e := readWorkspaceFile(target)
	respond(w, 200, fileResponse(rel, content), e)
}
