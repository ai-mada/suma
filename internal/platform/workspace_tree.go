package platform

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	workspaceTreeInvalid        = "Workspace tree request is invalid"
	workspaceTreeForbidden      = "Workspace directory path is not allowed"
	workspaceTreeUnavailable    = "Workspace directory is unavailable"
	workspaceTreeDirectoryLimit = "Workspace directory exceeds the 10000-entry limit"
	workspaceTreeChanged        = "Workspace directory changed; reload and retry"
	workspaceTreeMaxEntries     = 10000
)

func workspaceTreeError(status int, code, message string) error {
	return &APIError{status, "workspace_tree_" + code, message, nil}
}

func workspaceTreeChangedError() error {
	return workspaceTreeError(409, "changed", workspaceTreeChanged)
}

func workspaceTreeSamePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func workspaceTreePath(name string, root bool) bool {
	if name == "" {
		return root
	}
	if len(name) > 1024 || !utf8.ValidString(name) || filepath.IsAbs(name) || strings.ContainsAny(name, "\\:\x00") || forbiddenPath(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}

func workspaceTreeSameMetadata(before, after os.FileInfo) bool {
	return before != nil && after != nil && os.SameFile(before, after) && before.Mode() == after.Mode() && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

// Both calls check the original live credential and the current project
// binding. Filesystem work never holds an authorization transaction open.
func (s *Service) workspaceTreeGuard(ctx context.Context, actor Actor, project, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.workspaceReadGuard(ctx, actor, project, root); err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 409 || api.Code == "workspace_not_configured") {
			return workspaceTreeChangedError()
		}
		return err
	}
	allowed, err := allowedWorkspaceRoot()
	if err != nil {
		return workspaceTreeError(403, "forbidden", workspaceTreeForbidden)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	if !workspaceTreeSamePath(root, resolved) || !under(allowed, resolved) {
		return workspaceTreeError(403, "forbidden", workspaceTreeForbidden)
	}
	return ctx.Err()
}

// Root prevents escape even during a path exchange. Explicit Lstat of every
// component additionally rejects links whose target happens to be inside Root.
func workspaceTreeDirectoryChain(ctx context.Context, root *os.Root, directory string) ([]os.FileInfo, error) {
	var chain []os.FileInfo
	if directory == "" {
		info, err := root.Lstat(".")
		if err != nil || !info.IsDir() {
			return nil, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
		}
		return []os.FileInfo{info}, nil
	}
	parts := strings.Split(directory, "/")
	for i := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, workspaceTreeError(403, "forbidden", workspaceTreeForbidden)
		}
		if !info.IsDir() {
			return nil, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
		}
		chain = append(chain, info)
	}
	return chain, nil
}

type workspaceTreeObservedEntry struct {
	Entry    WorkspaceTreeEntry `json:"entry"`
	Identity string             `json:"identity"`
	Size     int64              `json:"size"`
	Mtime    int64              `json:"mtime"`
	Mode     uint32             `json:"mode"`
	info     os.FileInfo
}

// The snapshot binds all visible names and their metadata in this one
// directory. It is not an atomic snapshot of file contents or the workspace.
func (s *Service) listWorkspaceTree(ctx context.Context, actor Actor, project, root, directory string, limit int, before string) (WorkspaceTreePage, error) {
	out := WorkspaceTreePage{ProjectID: project, Directory: directory, Entries: []WorkspaceTreeEntry{}}
	if limit == 0 {
		limit = 100
	}
	out.Limit = limit
	if err := s.workspaceTreeGuard(ctx, actor, project, root); err != nil {
		return out, err
	}
	if limit < 1 || limit > 200 {
		return out, workspaceTreeError(400, "invalid", workspaceTreeInvalid)
	}
	if !workspaceTreePath(directory, true) {
		return out, workspaceTreeError(403, "forbidden", workspaceTreeForbidden)
	}
	if !filepath.IsAbs(root) {
		return out, workspaceTreeError(400, "invalid", workspaceTreeInvalid)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() {
		return out, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return out, workspaceTreeError(403, "forbidden", workspaceTreeForbidden)
	}
	jail, err := os.OpenRoot(root)
	if err != nil {
		return out, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	defer jail.Close()
	openedRoot, err := jail.Lstat(".")
	if err != nil || !os.SameFile(rootInfo, openedRoot) {
		return out, workspaceTreeChangedError()
	}
	chain, err := workspaceTreeDirectoryChain(ctx, jail, directory)
	if err != nil {
		return out, err
	}
	name := directory
	if name == "" {
		name = "."
	}
	directoryRoot, err := jail.OpenRoot(name)
	if err != nil {
		return out, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	defer directoryRoot.Close()
	reader, err := directoryRoot.Open(".")
	if err != nil {
		return out, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	defer reader.Close()
	openedDirectory, err := reader.Stat()
	if err != nil || !workspaceTreeSameMetadata(chain[len(chain)-1], openedDirectory) {
		return out, workspaceTreeChangedError()
	}
	directoryIdentity, err := workspaceTreeFileIdentity(ctx, directoryRoot, ".", openedDirectory)
	if err != nil {
		return out, err
	}
	items := []workspaceTreeObservedEntry{}
	rawCount := 0
	for {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		batch, readErr := reader.ReadDir(min(128, workspaceTreeMaxEntries+1-rawCount))
		rawCount += len(batch)
		if rawCount > workspaceTreeMaxEntries {
			return out, workspaceTreeError(413, "directory_limit", workspaceTreeDirectoryLimit)
		}
		if readErr != nil && readErr != io.EOF {
			return out, workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
		}
		for _, entry := range batch {
			if err = ctx.Err(); err != nil {
				return out, err
			}
			path := entry.Name()
			if directory != "" {
				path = directory + "/" + path
			}
			if !workspaceTreePath(path, false) {
				continue
			}
			info, statErr := directoryRoot.Lstat(entry.Name())
			if statErr != nil {
				return out, workspaceTreeChangedError()
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
				continue
			}
			kind := "file"
			if info.IsDir() {
				kind = "directory"
			}
			identity, identityErr := workspaceTreeFileIdentity(ctx, directoryRoot, entry.Name(), info)
			if identityErr != nil {
				return out, identityErr
			}
			items = append(items, workspaceTreeObservedEntry{Entry: WorkspaceTreeEntry{Path: path, Name: entry.Name(), Kind: kind}, Identity: identity, Size: info.Size(), Mtime: info.ModTime().UnixNano(), Mode: uint32(info.Mode()), info: info})
		}
		if readErr == io.EOF || len(batch) == 0 {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Entry.Kind != items[j].Entry.Kind {
			return items[i].Entry.Kind == "directory"
		}
		return items[i].Entry.Name < items[j].Entry.Name
	})
	for _, item := range items {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		current, statErr := directoryRoot.Lstat(item.Entry.Name)
		if statErr != nil || !workspaceTreeSameMetadata(item.info, current) {
			return out, workspaceTreeChangedError()
		}
	}
	afterDirectory, err := reader.Stat()
	if err != nil || !workspaceTreeSameMetadata(openedDirectory, afterDirectory) {
		return out, workspaceTreeChangedError()
	}
	afterChain, err := workspaceTreeDirectoryChain(ctx, jail, directory)
	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, workspaceTreeChangedError()
	}
	if len(afterChain) != len(chain) {
		return out, workspaceTreeChangedError()
	}
	for i := range chain {
		if !workspaceTreeSameMetadata(chain[i], afterChain[i]) {
			return out, workspaceTreeChangedError()
		}
	}
	afterRoot, err := os.Lstat(root)
	if err != nil || afterRoot.Mode()&os.ModeSymlink != 0 || !os.SameFile(rootInfo, afterRoot) {
		return out, workspaceTreeChangedError()
	}
	if err = s.workspaceTreeGuard(ctx, actor, project, root); err != nil {
		return out, err
	}
	out.SnapshotSHA256 = digest(stringify(struct {
		Directory string                       `json:"directory"`
		Identity  string                       `json:"identity"`
		Mtime     int64                        `json:"mtime"`
		Entries   []workspaceTreeObservedEntry `json:"entries"`
	}{directory, directoryIdentity, openedDirectory.ModTime().UnixNano(), items}))
	p := actor.Projection()
	collection := "workspace-tree:" + digest(stringify(struct {
		Actor      string
		Credential string
		Kind       actorKind
		Tenant     string
		Project    string
		Root       string
		Directory  string
		Snapshot   string
	}{p.ID, actor.credential.digest, actor.kind, p.TenantID, project, digest(root), directory, out.SnapshotSHA256}))
	anchor, cursorErr := automationDecodeCursor(p, project, collection, before)
	if cursorErr != nil {
		return WorkspaceTreePage{}, workspaceTreeChangedError()
	}
	start := 0
	if before != "" {
		matches := 0
		for i, item := range items {
			if digest(item.Entry.Path)[:32] == anchor {
				start = i + 1
				matches++
			}
		}
		if matches != 1 {
			return WorkspaceTreePage{}, workspaceTreeChangedError()
		}
	}
	end := min(start+limit, len(items))
	for _, item := range items[start:end] {
		out.Entries = append(out.Entries, item.Entry)
	}
	if end < len(items) {
		out.NextCursor = automationEncodeCursor(p, project, collection, digest(items[end-1].Entry.Path)[:32])
	}
	out.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return out, nil
}
