package platform

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func workspaceTreeTestURL(project, directory string, limit int, before string) string {
	q := url.Values{"project_id": {project}, "directory": {directory}}
	if limit != 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if before != "" {
		q.Set("before", before)
	}
	return "/workspace/tree?" + q.Encode()
}

func workspaceTreeTestWrite(t *testing.T, root, path string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("directory test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

// Validate the public shape without reproducing the reader's sorting or cursor
// algorithm. In particular, a page must never become a recursive file inventory.
func workspaceTreeTestEntries(t *testing.T, page map[string]any, project, directory string, limit int) []map[string]any {
	t.Helper()
	if len(page) != 7 || page["project_id"] != project || page["directory"] != directory || page["limit"] != float64(limit) {
		t.Fatalf("wrong tree page scope or shape: %v", page)
	}
	if _, ok := page["next_cursor"].(string); !ok {
		t.Fatalf("next_cursor is not an explicit string: %v", page)
	}
	snapshot, _ := page["snapshot_sha256"].(string)
	if raw, err := hex.DecodeString(snapshot); err != nil || len(raw) != 32 || snapshot != strings.ToLower(snapshot) {
		t.Fatalf("invalid directory snapshot digest: %q", snapshot)
	}
	observed, _ := page["observed_at"].(string)
	if _, err := time.Parse(time.RFC3339Nano, observed); err != nil {
		t.Fatalf("invalid directory observation time: %q", observed)
	}
	entries, ok := page["entries"].([]any)
	if !ok || len(entries) > limit {
		t.Fatalf("entries must be a bounded non-null array: %v", page)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok || len(entry) != 3 {
			t.Fatalf("unexpected entry fields: %v", item)
		}
		name, nameOK := entry["name"].(string)
		path, pathOK := entry["path"].(string)
		if !nameOK || !pathOK || name == "" || path == "" || entry["kind"] != "file" && entry["kind"] != "directory" {
			t.Fatalf("invalid entry: %v", entry)
		}
		wantPath := name
		if directory != "" {
			wantPath = directory + "/" + name
		}
		if strings.ContainsAny(name, "/\\") || path != wantPath {
			t.Fatalf("entry is not an immediate relative child: %v", entry)
		}
		out = append(out, entry)
	}
	return out
}

func workspaceTreeTestPaths(entries []map[string]any) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry["path"].(string))
	}
	return paths
}

func TestWorkspaceTreePaginationIsStableAndNonRecursive(t *testing.T) {
	f, root := workspaceFixture(t)
	for _, path := range []string{"zeta/deep/hidden.txt", "alpha/child.txt", "aaa.txt", "zzz.txt"} {
		workspaceTreeTestWrite(t, root, path)
	}
	var operations, activity int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&activity); err != nil {
		t.Fatal(err)
	}
	first := f.call("GET", workspaceTreeTestURL(f.project, "", 2, ""), nil, 200)
	page, snapshot := first, first["snapshot_sha256"]
	paths, seen, cursors := []string{}, map[string]bool{}, map[string]bool{}
	for n := 0; ; n++ {
		if n > 4 {
			t.Fatal("pagination did not terminate")
		}
		if page["snapshot_sha256"] != snapshot {
			t.Fatal("one unchanged directory produced different page snapshots")
		}
		for _, entry := range workspaceTreeTestEntries(t, page, f.project, "", 2) {
			path := entry["path"].(string)
			if seen[path] {
				t.Fatalf("entry repeated across pages: %s", path)
			}
			seen[path] = true
			paths = append(paths, path)
		}
		cursor := page["next_cursor"].(string)
		if cursor == "" {
			break
		}
		if cursors[cursor] {
			t.Fatal("pagination repeated a cursor")
		}
		cursors[cursor] = true
		page = f.call("GET", workspaceTreeTestURL(f.project, "", 2, cursor), nil, 200)
	}
	if want := []string{"alpha", "zeta", "aaa.txt", "main.go", "zzz.txt"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("directory-first page order or immediate scope changed: got %v, want %v", paths, want)
	}
	for _, entry := range workspaceTreeTestEntries(t, first, f.project, "", 2) {
		if entry["kind"] != "directory" {
			t.Fatal("directories did not sort before files")
		}
	}
	all := f.call("GET", "/workspace/tree?project_id="+f.project, nil, 200)
	if got := workspaceTreeTestPaths(workspaceTreeTestEntries(t, all, f.project, "", 100)); !reflect.DeepEqual(got, paths) || all["next_cursor"] != "" {
		t.Fatalf("default page differs from complete paginated directory: %v", all)
	}
	var afterOperations, afterActivity int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&afterOperations); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&afterActivity); err != nil {
		t.Fatal(err)
	}
	if operations != afterOperations || activity != afterActivity {
		t.Fatal("directory GET created a business receipt or activity")
	}
}

func TestWorkspaceTreeSubdirectoriesExcludeSecretsAndSymlinks(t *testing.T) {
	f, root := workspaceFixture(t)
	for _, path := range []string{
		"src/nested/deeper.go", "src/visible.go", ".editorconfig", ".env", ".ENV.production",
		".aws/credentials", ".git/config", "node_modules/pkg/index.js", ".cache/result.txt",
		"private.key", "secrets.json", ".suma-save-owned", "src/.env.local", "src/key.pem", "src/secrets/hidden.txt",
	} {
		workspaceTreeTestWrite(t, root, path)
	}
	linkedDirectory := false
	if err := os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "linked-directory")); err != nil {
		t.Logf("directory symlink unavailable on this host: %v", err)
	} else {
		linkedDirectory = true
	}
	if err := os.Symlink(filepath.Join(root, "main.go"), filepath.Join(root, "linked-file.go")); err != nil {
		t.Logf("file symlink unavailable on this host: %v", err)
	}
	page := f.call("GET", workspaceTreeTestURL(f.project, "", 200, ""), nil, 200)
	if got, want := workspaceTreeTestPaths(workspaceTreeTestEntries(t, page, f.project, "", 200)), []string{"src", ".editorconfig", "main.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("secret, generated or linked entry was exposed: got %v, want %v", got, want)
	}
	child := f.call("GET", workspaceTreeTestURL(f.project, "src", 200, ""), nil, 200)
	if got, want := workspaceTreeTestPaths(workspaceTreeTestEntries(t, child, f.project, "src", 200)), []string{"src/nested", "src/visible.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("child directory was recursive or exposed a secret: got %v, want %v", got, want)
	}
	for _, directory := range []string{".aws", ".git", "src/secrets"} {
		f.call("GET", workspaceTreeTestURL(f.project, directory, 100, ""), nil, 403)
	}
	if linkedDirectory {
		f.call("GET", workspaceTreeTestURL(f.project, "linked-directory", 100, ""), nil, 403)
		f.call("GET", workspaceTreeTestURL(f.project, "linked-directory/nested", 100, ""), nil, 403)
	}
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0700); err != nil {
		t.Fatal(err)
	}
	page = f.call("GET", workspaceTreeTestURL(f.project, "empty", 1, ""), nil, 200)
	if entries := workspaceTreeTestEntries(t, page, f.project, "empty", 1); len(entries) != 0 || page["next_cursor"] != "" {
		t.Fatalf("empty directory did not end with an explicit empty page: %v", page)
	}
}

func TestWorkspaceTreeRejectsInvalidRequests(t *testing.T) {
	f, _ := workspaceFixture(t)
	base := "/workspace/tree?project_id=" + f.project
	for _, suffix := range []string{
		"&limit=0", "&limit=-1", "&limit=201", "&limit=1.5", "&limit=x", "&limit=", "&limit=0100", "&limit=%2B100",
		"&project_id=" + f.project, "&directory=&directory=src", "&limit=1&limit=1", "&before=a&before=b",
		"&before=", "&extra=x", "&path=main.go", "&directory=%ZZ", "&directory=" + strings.Repeat("x", 1025), "&before=" + strings.Repeat("x", 1025),
	} {
		f.call("GET", base+suffix, nil, 400)
	}
	f.call("GET", "/workspace/tree", nil, 400)
	f.call("GET", "/workspace/tree?project_id=", nil, 400)
	f.call("POST", base, map[string]any{}, 405)
	for _, directory := range []string{"..", "../outside", ".", "/absolute", "C:/outside", "src\\nested", "src/../outside", "src//nested", "src\x00child"} {
		f.call("GET", workspaceTreeTestURL(f.project, directory, 100, ""), nil, 403)
	}
	for _, directory := range []string{"missing", "main.go"} {
		f.call("GET", workspaceTreeTestURL(f.project, directory, 100, ""), nil, 422)
	}
	assertWorkspaceLocalizedError(t, f, "GET", base+"&limit=0", nil, 400, "workspace_tree_invalid", "workspace_tree.invalid", "目录浏览请求无效，请检查项目、目录和分页参数", "Workspace tree request is invalid. Check the project, directory and page parameters.")
}

func TestWorkspaceTreeCursorBindsScopeAndSnapshot(t *testing.T) {
	f, root := workspaceFixture(t)
	for _, path := range []string{"src/a.go", "src/b.go", "rebound/other.go"} {
		workspaceTreeTestWrite(t, root, path)
	}
	first := f.call("GET", workspaceTreeTestURL(f.project, "", 1, ""), nil, 200)
	workspaceTreeTestEntries(t, first, f.project, "", 1)
	cursor := first["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("fixture must produce a continuation cursor")
	}
	for _, before := range []string{"not-a-cursor!", cursor[:len(cursor)/2] + "!" + cursor[len(cursor)/2:]} {
		f.call("GET", workspaceTreeTestURL(f.project, "", 1, before), nil, 409)
	}
	f.call("GET", workspaceTreeTestURL(f.project, "src", 1, cursor), nil, 409)
	other := f.call("POST", "/projects", map[string]any{"title": "other workspace scope", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("POST", "/workspace/binding", map[string]any{"project_id": other, "path": root, "idempotency_key": newID()}, 200)
	f.call("GET", workspaceTreeTestURL(other, "", 1, cursor), nil, 409)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": filepath.Join(root, "rebound")}), 200)
	f.call("GET", workspaceTreeTestURL(f.project, "", 1, cursor), nil, 409)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	workspaceTreeTestWrite(t, root, "added-after-first-page.go")
	changed := f.call("GET", workspaceTreeTestURL(f.project, "", 1, cursor), nil, 409)
	if detail, ok := changed["error"].(map[string]any); !ok || detail["code"] != "workspace_tree_changed" {
		t.Fatalf("stale snapshot did not demand a directory reload: %v", changed)
	}
	fresh := f.call("GET", workspaceTreeTestURL(f.project, "", 1, ""), nil, 200)
	if fresh["snapshot_sha256"] == first["snapshot_sha256"] {
		t.Fatal("a changed directory retained its old snapshot")
	}
	assertWorkspaceLocalizedError(t, f, "GET", workspaceTreeTestURL(f.project, "", 1, cursor), nil, 409, "workspace_tree_changed", "workspace_tree.changed", "目录或工作区绑定已变化，请刷新目录后重试", "The directory or workspace binding has changed. Refresh the directory and try again.")
}

func TestWorkspaceTreeRechecksCurrentAuthorityAndBinding(t *testing.T) {
	f, root := workspaceFixture(t)
	workspaceTreeTestWrite(t, root, "second.go")
	ownerPage := f.call("GET", workspaceTreeTestURL(f.project, "", 1, ""), nil, 200)
	ownerCursor := ownerPage["next_cursor"].(string)
	ownerCookie := f.cookie
	invitation := governanceInvite(f, "workspace-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invitation, "workspace-viewer@example.test")
	viewer, err := f.s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := f.cookie
	f.call("GET", workspaceTreeTestURL(f.project, "", 1, ""), nil, 200)
	f.call("GET", workspaceTreeTestURL(f.project, "", 1, ownerCursor), nil, 409)
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	if _, err = f.s.SaveProjectAccess(context.Background(), f.actor, map[string]any{"project_id": f.project, "user_id": viewer.Projection().ID, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.listWorkspaceTree(context.Background(), viewer, f.project, root, "", 100, "")
	requireAuthorityStatus(t, err, 403)
	f.cookie = viewerCookie
	f.call("GET", workspaceTreeTestURL(f.project, "", 100, ""), nil, 403)
	f.call("GET", "/workspace?project_id="+f.project, nil, 403)
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	rebound := filepath.Join(root, "rebound")
	if err = os.Mkdir(rebound, 0700); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": rebound}), 200)
	_, err = f.s.listWorkspaceTree(context.Background(), f.actor, f.project, root, "", 100, "")
	requireAuthorityStatus(t, err, 409)
	if err = logoutCredentialForTest(context.Background(), f.s, ownerCookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.listWorkspaceTree(context.Background(), f.actor, f.project, rebound, "", 100, "")
	requireAuthorityStatus(t, err, 401)
	f.call("GET", workspaceTreeTestURL(f.project, "", 100, ""), nil, 401)
}

func TestWorkspaceTreeMetadataContainsOnlySummary(t *testing.T) {
	f, root := workspaceFixture(t)
	workspaceTreeTestWrite(t, root, "src/nested/unlisted.go")
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), []byte{0, 1}, 0600); err != nil {
		t.Fatal(err)
	}
	metadata := f.call("GET", "/workspace?project_id="+f.project, nil, 200)
	if metadata["configured"] != true || metadata["binding_available"] != true || metadata["name"] != filepath.Base(root) {
		t.Fatalf("configured metadata lost its binding summary: %v", metadata)
	}
	for key := range metadata {
		if key != "configured" && key != "binding_available" && key != "name" && key != "reason" {
			t.Fatalf("workspace metadata returned an inventory or private field: %q", key)
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{root, filepath.ToSlash(root), "unlisted.go", "binary.bin"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("metadata exposed a host root or recursive entry: %s", private)
		}
	}
	for _, suffix := range []string{"&limit=1", "&directory=src", "&before=cursor", "&project_id=" + f.project} {
		f.call("GET", "/workspace?project_id="+f.project+suffix, nil, 400)
	}
}
