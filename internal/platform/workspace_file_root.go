package platform

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (s *Service) openWorkspaceFileRootTx(ctx context.Context, tx *sql.Tx, tenant, project, expected string) (opened *os.Root, err error) {
	state, err := s.workspaceBindingStateTx(ctx, tx, tenant, project)
	if err != nil {
		return nil, err
	}
	if expected != "" && expected != state.root {
		return nil, conflict("工作区绑定已变化，请重新加载")
	}
	defer func() {
		if err != nil && opened != nil {
			opened.Close()
			opened = nil
		}
	}()
	if state.proof != nil {
		if err = s.validateDesktopWorkspaceRootProof(tenant, project, state); err != nil {
			return nil, err
		}
		if !desktopWorkspacePath(state.root) {
			return nil, desktopWorkspaceRootInvalid()
		}
		resolved, e := filepath.EvalSymlinks(state.root)
		if e != nil {
			return nil, desktopWorkspaceRootUnavailable()
		}
		if !workspaceTreeSamePath(state.root, resolved) {
			return nil, desktopWorkspaceRootInvalid()
		}
		opened, err = os.OpenRoot(state.root)
		if err != nil {
			return nil, desktopWorkspaceRootUnavailable()
		}
		info, e := opened.Lstat(".")
		if e != nil || !info.IsDir() {
			return opened, desktopWorkspaceRootUnavailable()
		}
		// Compare the persistent proof to this exact handle, not to a separate
		// path Stat followed by an unverified re-open of the directory.
		identity, e := workspaceTreeFileIdentity(ctx, opened, ".", info)
		if e != nil {
			if ctx.Err() != nil {
				return opened, ctx.Err()
			}
			return opened, desktopWorkspaceRootUnavailable()
		}
		if identity != state.proof.Identity {
			return opened, desktopWorkspaceRootChanged()
		}
		return opened, ctx.Err()
	}
	if err = s.validateWorkspaceBindingRoot(ctx, tenant, project, state); err != nil {
		return nil, err
	}
	allowed, err := allowedWorkspaceRoot()
	if err != nil {
		return nil, forbidden()
	}
	parent, err := os.OpenRoot(allowed)
	if err != nil {
		return nil, forbidden()
	}
	defer parent.Close()
	rel, err := filepath.Rel(allowed, state.root)
	if err != nil || !under(allowed, state.root) {
		return nil, forbidden()
	}
	// Resolve the configured binding through its allowed parent descriptor;
	// even a path exchange cannot redirect this open outside that parent.
	directory := filepath.ToSlash(rel)
	if directory == "." {
		directory = ""
	}
	chain, err := workspaceTreeDirectoryChain(ctx, parent, directory)
	if err != nil {
		return nil, err
	}
	opened, err = parent.OpenRoot(rel)
	if err != nil {
		return nil, forbidden()
	}
	info, err := opened.Lstat(".")
	if err != nil || !os.SameFile(chain[len(chain)-1], info) {
		return opened, conflict("工作区绑定已变化，请重新加载")
	}
	return opened, ctx.Err()
}

func (s *Service) openWorkspaceFileRoot(ctx context.Context, actor Actor, project, expected string) (opened *os.Root, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		opened, err = s.openWorkspaceFileRootTx(ctx, auth.tx, auth.currentPrincipal.TenantID, project, expected)
		return err
	})
	if err != nil && opened != nil {
		opened.Close()
		opened = nil
	}
	return opened, err
}

func workspaceFileRelative(rel string) (string, error) {
	if rel == "" || len(rel) > 4096 || !utf8.ValidString(rel) || filepath.IsAbs(rel) || strings.Contains(rel, ":") || strings.IndexFunc(rel, unicode.IsControl) >= 0 || forbiddenPath(rel) {
		return "", forbidden()
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return "", missing()
		}
		if part == "" || part == "." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", forbidden()
		}
	}
	return rel, nil
}

func workspaceRootedFileInfo(root *os.Root, rel string) (os.FileInfo, error) {
	parts := strings.Split(rel, "/")
	var info os.FileInfo
	for i := range parts {
		var err error
		info, err = root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, missing()
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, forbidden()
		}
	}
	return info, nil
}

func readWorkspaceRootedFile(ctx context.Context, root *os.Root, rel string) (string, os.FileInfo, error) {
	name, err := workspaceFileRelative(rel)
	if err != nil {
		return "", nil, err
	}
	before, err := workspaceRootedFileInfo(root, name)
	if err != nil {
		return "", nil, err
	}
	if err = ctx.Err(); err != nil {
		return "", nil, err
	}
	file, err := root.Open(name)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !workspaceTreeSameMetadata(before, opened) || !opened.Mode().IsRegular() {
		return "", nil, conflict("文件保存冲突，请重新加载")
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return "", nil, err
	}
	if len(raw) > 1<<20 || strings.ContainsRune(string(raw), 0) || !utf8.Valid(raw) {
		return "", nil, invalid("编辑器仅打开 1 MiB 以内的文本文件")
	}
	after, err := file.Stat()
	if err != nil || !workspaceTreeSameMetadata(opened, after) || int64(len(raw)) != after.Size() {
		return "", nil, conflict("文件保存冲突，请重新加载")
	}
	current, err := workspaceRootedFileInfo(root, name)
	if err != nil {
		return "", nil, err
	}
	if !workspaceTreeSameMetadata(after, current) {
		return "", nil, conflict("文件保存冲突，请重新加载")
	}
	return string(raw), current, ctx.Err()
}

func replaceWorkspaceRootedFile(ctx context.Context, root *os.Root, rel, expected, content string) error {
	name, err := workspaceFileRelative(rel)
	if err != nil {
		return err
	}
	info, err := workspaceRootedFileInfo(root, name)
	if err != nil {
		return err
	}
	directory, base := path.Dir(name), path.Base(name)
	parentInfo, err := root.Lstat(directory)
	if err != nil {
		return err
	}
	parent, err := root.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	openedParent, err := parent.Lstat(".")
	if err != nil || !os.SameFile(parentInfo, openedParent) {
		return conflict("文件保存冲突，请重新加载")
	}
	temporary := ".suma-save-" + newID()
	file, err := parent.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer parent.Remove(temporary)
	if err = file.Chmod(info.Mode().Perm()); err == nil {
		_, err = file.WriteString(content)
	}
	if err == nil {
		err = file.Sync()
	}
	temporaryInfo, statErr := file.Stat()
	if err == nil {
		err = statErr
	}
	closed := file.Close()
	if err == nil {
		err = closed
	}
	if err != nil {
		return err
	}
	current, checked, err := readWorkspaceRootedFile(ctx, parent, base)
	if err != nil {
		return err
	}
	if !workspaceTreeSameMetadata(info, checked) || digest(current) != expected {
		return conflict("文件保存冲突，请重新加载")
	}
	linked, err := workspaceRootedFileInfo(root, name)
	if err != nil {
		return err
	}
	if !workspaceTreeSameMetadata(checked, linked) {
		return conflict("文件保存冲突，请重新加载")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	currentTemporary, err := parent.Lstat(temporary)
	if err != nil || !currentTemporary.Mode().IsRegular() || !workspaceTreeSameMetadata(temporaryInfo, currentTemporary) {
		return conflict("文件保存冲突，请重新加载")
	}
	// Both names belong to this already-open parent directory. No absolute-path
	// rename or cleanup can follow a concurrently replaced project root.
	if err = parent.Rename(temporary, base); err != nil {
		return err
	}
	saved, savedInfo, err := readWorkspaceRootedFile(ctx, parent, base)
	if err != nil {
		return err
	}
	if !os.SameFile(temporaryInfo, savedInfo) || digest(saved) != digest(content) {
		return conflict("文件保存冲突，请重新加载")
	}
	if runtime.GOOS != "windows" {
		directoryFile, err := parent.Open(".")
		if err != nil {
			return err
		}
		defer directoryFile.Close()
		return directoryFile.Sync()
	}
	return nil
}

func (s *Service) recoverWorkspaceFile(ctx context.Context, tx *sql.Tx, p Principal, project string, op workspaceOperation) error {
	if op.root == "" {
		return forbidden()
	}
	root, err := s.openWorkspaceFileRootTx(ctx, tx, p.TenantID, project, op.root)
	if err != nil {
		return err
	}
	defer root.Close()
	current, _, err := readWorkspaceRootedFile(ctx, root, op.path)
	if err != nil {
		return err
	}
	hash := digest(current)
	if hash == op.expected {
		if err = replaceWorkspaceRootedFile(ctx, root, op.path, op.expected, op.content); err != nil {
			return err
		}
	} else if hash != op.next {
		return &APIError{409, "workspace_reconciliation_required", "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", map[string]any{"operation_id": op.key, "path": op.path}}
	}
	// If a rename exchanged the project root during I/O, preserve the prepared
	// journal instead of certifying that its current bound path was committed.
	_, err = s.workspaceRootAuthorityTx(ctx, tx, p.TenantID, project, op.root)
	return err
}
