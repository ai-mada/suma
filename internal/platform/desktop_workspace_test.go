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
	"syscall"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

func desktopWorkspaceFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	t.Setenv("SUMA_WORKSPACE_ROOT", "")
	f.s.DesktopWorkspaceKey = []byte(strings.Repeat("k", 32))
	selected := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(selected, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(selected)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	return f, root
}

// Sign the transport's exact field order independently of the service helper.
func desktopWorkspaceTestSign(key []byte, grant *WorkspaceDesktopGrant) {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.Join([]string{grant.Protocol, grant.OperationID, grant.UserID, grant.TenantID, grant.ProjectID, grant.Path, grant.ExpiresAt, grant.Nonce}, "\x00")))
	grant.Signature = hex.EncodeToString(mac.Sum(nil))
}

func desktopWorkspaceTestRequest(f *fixture, root, expected string) WorkspaceDesktopBindingRequest {
	id := strings.ReplaceAll(newID(), "-", "")
	grant := WorkspaceDesktopGrant{Protocol: "suma-desktop-workspace-v1", OperationID: "desktop-workspace-" + id, UserID: f.user.ID, TenantID: f.user.TenantID, ProjectID: f.project, Path: root, ExpiresAt: time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339Nano), Nonce: id}
	desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &grant)
	return WorkspaceDesktopBindingRequest{ProjectID: f.project, ExpectedBindingSHA256: expected, Grant: grant, IdempotencyKey: grant.OperationID}
}

func TestDesktopWorkspaceBindsProjectWithoutChangingServerRootAndSurvivesRestart(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	request := desktopWorkspaceTestRequest(f, root, "")
	bound, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, request)
	if err != nil || !bound.Configured || !bound.DesktopBindingAvailable || bound.ProjectID != f.project || bound.BindingSHA256 == "" || bound.BindingPathSHA256 != digest(root) || bound.BindingOperationID != request.Grant.OperationID {
		t.Fatalf("native selection was not bound to the current project: %+v %v", bound, err)
	}
	if os.Getenv("SUMA_WORKSPACE_ROOT") != "" {
		t.Fatal("native selection changed the global server root")
	}
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	f.call("GET", workspaceTreeTestURL(f.project, "", 100, ""), nil, 200)
	f.call("POST", "/workspace/file", f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "native save"}), 200)
	if _, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project); err != nil {
		t.Fatalf("native root did not reach the guarded Git reader: %v", err)
	}
	var dbPath string
	if err = f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	restarted := &Service{DB: reopened, Dialect: "sqlite", DataDir: f.s.DataDir, DesktopWorkspaceKey: append([]byte(nil), f.s.DesktopWorkspaceKey...)}
	metadata, err := restarted.GetWorkspaceMetadata(ctx, f.actor, f.project)
	if err != nil || !metadata.Configured || metadata.BindingSHA256 != bound.BindingSHA256 || metadata.BindingOperationID != bound.BindingOperationID {
		t.Fatalf("stable key did not verify the registered root after restart: %+v %v", metadata, err)
	}
	wrongKey := &Service{DB: f.s.DB, Dialect: "sqlite", DataDir: f.s.DataDir, DesktopWorkspaceKey: []byte(strings.Repeat("x", 32))}
	metadata, err = wrongKey.GetWorkspaceMetadata(ctx, f.actor, f.project)
	if err != nil || metadata.Configured || metadata.BindingSHA256 != bound.BindingSHA256 {
		t.Fatalf("invalid key became authority or erased the recovery CAS: %+v %v", metadata, err)
	}
	_, err = wrongKey.workspaceRoot(ctx, f.actor, f.project)
	requireAuthorityStatus(t, err, 403)
}

func TestDesktopWorkspaceRejectsInvalidGrantsWithoutBindingOrReceipt(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	for _, tc := range []struct {
		name   string
		change func(*WorkspaceDesktopBindingRequest)
		status int
	}{
		{"protocol", func(r *WorkspaceDesktopBindingRequest) { r.Grant.Protocol = "other" }, 403},
		{"user", func(r *WorkspaceDesktopBindingRequest) { r.Grant.UserID = "other-user" }, 403},
		{"tenant", func(r *WorkspaceDesktopBindingRequest) { r.Grant.TenantID = "other-tenant" }, 403},
		{"project", func(r *WorkspaceDesktopBindingRequest) { r.Grant.ProjectID = "other-project" }, 403},
		{"signature", func(r *WorkspaceDesktopBindingRequest) { r.Grant.Signature = strings.Repeat("0", 64) }, 403},
		{"nonce", func(r *WorkspaceDesktopBindingRequest) { r.Grant.Nonce = "short" }, 403},
		{"operation", func(r *WorkspaceDesktopBindingRequest) { r.IdempotencyKey = "other-operation" }, 400},
		{"expired", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 410},
		{"future", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.ExpiresAt = time.Now().UTC().Add(181 * time.Second).Format(time.RFC3339Nano)
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 403},
		{"relative", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = "relative"
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 400},
		{"unc", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = `\\server\share`
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 400},
		{"unclean", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = root + string(filepath.Separator) + "."
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 400},
		{"control", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = root + "\n"
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 400},
		{"not_directory", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = filepath.Join(root, "main.go")
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 422},
		{"missing_directory", func(r *WorkspaceDesktopBindingRequest) {
			r.Grant.Path = filepath.Join(root, "missing")
			desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &r.Grant)
		}, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := desktopWorkspaceTestRequest(f, root, "")
			tc.change(&request)
			_, err := f.s.BindDesktopWorkspace(context.Background(), f.actor, f.project, request)
			requireAuthorityStatus(t, err, tc.status)
		})
	}
	if err := os.Symlink(root, root+"-link"); err != nil {
		t.Logf("directory symlink unavailable on this host: %v", err)
	} else {
		_, err = f.s.BindDesktopWorkspace(context.Background(), f.actor, f.project, desktopWorkspaceTestRequest(f, root+"-link", ""))
		requireAuthorityStatus(t, err, 403)
	}
	for _, table := range []string{"suma_workspace_bindings", "suma_desktop_workspace_roots", "suma_operations"} {
		if n := domainACount(t, f.s, table); n != 0 {
			t.Fatalf("rejected native grant wrote %s: %d", table, n)
		}
	}
	f.s.DesktopWorkspaceKey = nil
	_, err := f.s.BindDesktopWorkspace(context.Background(), f.actor, f.project, desktopWorkspaceTestRequest(f, root, ""))
	requireAuthorityStatus(t, err, 503)
}

func TestDesktopWorkspaceCASAndExpiredOriginalReceiptNeverRebind(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	first := desktopWorkspaceTestRequest(f, root, "")
	expiry := time.Now().UTC().Truncate(time.Second).Add(2 * time.Second)
	first.Grant.ExpiresAt = expiry.Format(time.RFC3339Nano)
	desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &first.Grant)
	original, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, first)
	if err != nil {
		t.Fatal(err)
	}
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second := desktopWorkspaceTestRequest(f, other, "")
	_, err = f.s.BindDesktopWorkspace(ctx, f.actor, f.project, second)
	requireAuthorityStatus(t, err, 409)
	second.ExpectedBindingSHA256 = original.BindingSHA256
	current, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, second)
	if err != nil {
		t.Fatal(err)
	}
	// A short real expiry distinguishes original receipt replay from accepting a
	// new expired grant. No test clock or authority bypass is used.
	if wait := time.Until(expiry.Add(20 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	replayed, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, first)
	if err != nil || stringify(replayed) != stringify(original) {
		t.Fatalf("expired original acknowledgement was not recoverable: %+v %v", replayed, err)
	}
	metadata, err := f.s.GetWorkspaceMetadata(ctx, f.actor, f.project)
	if err != nil || metadata.BindingSHA256 != current.BindingSHA256 || metadata.BindingPathSHA256 != digest(other) {
		t.Fatalf("receipt replay reselected the old directory: %+v %v", metadata, err)
	}
	changed := first
	changed.Grant.Path = other
	desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &changed.Grant)
	_, err = f.s.BindDesktopWorkspace(ctx, f.actor, f.project, changed)
	requireAuthorityStatus(t, err, 409)
	if n := domainACount(t, f.s, "suma_operations"); n != 2 {
		t.Fatalf("native operation replay duplicated receipts: %d", n)
	}
}

func TestDesktopWorkspaceInvalidProofAndReplacedRootKeepCASWithoutFallback(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	bound, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, desktopWorkspaceTestRequest(f, root, ""))
	if err != nil {
		t.Fatal(err)
	}
	// The ordinary configured source would allow this path. A present native
	// proof must still be verified rather than silently falling back to it.
	t.Setenv("SUMA_WORKSPACE_ROOT", filepath.Dir(root))
	var signature string
	if err = f.s.DB.QueryRow(`SELECT signature FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&signature); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_desktop_workspace_roots SET root_path=? WHERE tenant_id=? AND project_id=?`, filepath.Dir(root), f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	metadata, err := f.s.GetWorkspaceMetadata(ctx, f.actor, f.project)
	if err != nil || metadata.Configured || metadata.BindingSHA256 != bound.BindingSHA256 || metadata.BindingPathSHA256 != digest(root) || metadata.ReasonMessageKey != "desktop_workspace.root_invalid" {
		t.Fatalf("tampered proof fell back or removed binding recovery fields: %+v %v", metadata, err)
	}
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 403)
	f.call("GET", workspaceTreeTestURL(f.project, "", 100, ""), nil, 403)
	f.call("POST", "/workspace/file", f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "must not save"}), 403)
	if _, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project); err == nil {
		t.Fatal("Git trusted a tampered native root")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_desktop_workspace_roots SET root_path=?,signature=? WHERE tenant_id=? AND project_id=?`, root, signature, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(root, root+"-original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("replacement directory"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata, err = f.s.GetWorkspaceMetadata(ctx, f.actor, f.project)
	if err != nil || metadata.Configured || metadata.BindingSHA256 != bound.BindingSHA256 || metadata.ReasonMessageKey != "desktop_workspace.root_changed" {
		t.Fatalf("replaced directory identity retained authority: %+v %v", metadata, err)
	}
	_, err = f.s.workspaceRoot(ctx, f.actor, f.project)
	requireAuthorityStatus(t, err, 409)
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 409)
	f.call("POST", "/workspace/file", f.body(map[string]any{"path": "main.go", "expected_hash": digest("replacement directory"), "content": "must not save"}), 409)
	newBinding, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, desktopWorkspaceTestRequest(f, root, bound.BindingSHA256))
	if err != nil || !newBinding.Configured || newBinding.BindingSHA256 == bound.BindingSHA256 {
		t.Fatalf("explicit fresh selection could not repair a replaced root: %+v %v", newBinding, err)
	}
}

func TestDesktopWorkspaceHeldRootNeverRedirectsAfterDirectoryReplacement(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	if _, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, desktopWorkspaceTestRequest(f, root, "")); err != nil {
		t.Fatal(err)
	}
	held, err := f.s.openWorkspaceFileRoot(ctx, f.actor, f.project, root)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	moved := root + "-held"
	expectedFiles := map[string]string{root: "anchored save"}
	err = os.Rename(root, moved)
	if runtime.GOOS == "windows" {
		// Windows protects the opened directory from rename. Require this exact
		// OS denial; an unrelated permission/path error is not containment proof.
		const errorSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION.
		var renameError *os.LinkError
		if !errors.As(err, &renameError) || renameError.Op != "rename" || renameError.Old != root || renameError.New != moved || !errors.Is(err, errorSharingViolation) {
			t.Fatalf("opened root rename did not return Windows sharing violation: %v", err)
		}
		if _, statErr := os.Lstat(moved); !os.IsNotExist(statErr) {
			t.Fatalf("denied rename created a destination: %v", statErr)
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("replacement directory"), 0600); err != nil {
			t.Fatal(err)
		}
		expectedFiles = map[string]string{root: "replacement directory", moved: "anchored save"}
	}
	external := filepath.Join(filepath.Dir(root), "external-original.go")
	if err = os.WriteFile(external, []byte("external original"), 0600); err != nil {
		t.Fatal(err)
	}
	content, _, err := readWorkspaceRootedFile(ctx, held, "main.go")
	if err != nil || content != "original" {
		t.Fatalf("held root read followed a replaced absolute path: %q %v", content, err)
	}
	// Both platform behaviors keep operations on the originally opened object.
	// Public guards additionally reject a replacement when the OS permits it.
	err = replaceWorkspaceRootedFile(ctx, held, "main.go", digest("stale"), "must not save")
	requireAuthorityStatus(t, err, 409)
	err = replaceWorkspaceRootedFile(ctx, held, "../external-original.go", digest("external original"), "must not save")
	requireAuthorityStatus(t, err, 404)
	if err = replaceWorkspaceRootedFile(ctx, held, "main.go", digest("original"), "anchored save"); err != nil {
		t.Fatal(err)
	}
	for directory, want := range expectedFiles {
		actual, err := os.ReadFile(filepath.Join(directory, "main.go"))
		if err != nil || string(actual) != want {
			t.Fatalf("rooted rename reached the wrong directory: %q %v", actual, err)
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".suma-save-") {
				t.Fatal("rooted temporary file survived completion or failed CAS")
			}
		}
	}
	externalContent, err := os.ReadFile(external)
	if err != nil || string(externalContent) != "external original" {
		t.Fatalf("rooted write changed an external original: %q %v", externalContent, err)
	}
	_, err = f.s.workspaceReadGuard(ctx, f.actor, f.project, root)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("denied directory rename invalidated the protected root: %v", err)
		}
	} else {
		requireAuthorityStatus(t, err, 409)
	}
}

func TestDesktopWorkspaceCurrentAuthorityAndCrossProjectProof(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	request := desktopWorkspaceTestRequest(f, root, "")
	if _, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, request); err != nil {
		t.Fatal(err)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "separate native scope", "idempotency_key": newID()}, 201)["id"].(string)
	wrongProject := request
	wrongProject.ProjectID = other
	_, err := f.s.BindDesktopWorkspace(ctx, f.actor, other, wrongProject)
	requireAuthorityStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`INSERT INTO suma_workspace_bindings(tenant_id,project_id,root_path) VALUES(?,?,?)`, f.user.TenantID, other, root); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_desktop_workspace_roots(tenant_id,project_id,root_path,root_identity,registered_by,operation_id,signature,created_at) SELECT tenant_id,?,root_path,root_identity,registered_by,operation_id,signature,created_at FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`, other, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.workspaceRoot(ctx, f.actor, other)
	requireAuthorityStatus(t, err, 403)
	api := applicationActor(t, f, "api")
	_, err = f.s.BindDesktopWorkspace(ctx, api, f.project, request)
	requireAuthorityStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,'admin') ON CONFLICT(project_id,user_id) DO UPDATE SET role='admin'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.BindDesktopWorkspace(ctx, f.actor, f.project, request)
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.BindDesktopWorkspace(ctx, f.actor, f.project, request)
	requireAuthorityStatus(t, err, 401)
}

func TestDesktopWorkspacePreparedSaveRecoveryRequiresCurrentRootProof(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	ctx := context.Background()
	if _, err := f.s.BindDesktopWorkspace(ctx, f.actor, f.project, desktopWorkspaceTestRequest(f, root, "")); err != nil {
		t.Fatal(err)
	}
	body := f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "recovered native save"})
	response := fileResponse("main.go", "recovered native save")
	hash := digest(f.user.ID + stringify(map[string]any{"kind": "workspace.file", "body": body}))
	bounded, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := f.s.authorizeTx(bounded, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err == nil {
		err = f.s.prepareWorkspaceOperationTx(bounded, auth, str(body, "idempotency_key"), hash, root, "main.go", "original", "recovered native save", response)
	}
	if err == nil {
		err = tx.Commit()
	}
	tx.Rollback()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	var signature string
	if err = f.s.DB.QueryRow(`SELECT signature FROM suma_desktop_workspace_roots WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&signature); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_desktop_workspace_roots SET signature=? WHERE tenant_id=? AND project_id=?`, strings.Repeat("0", 64), f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/file", body, 403)
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("recovery changed a file without a current native proof", err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_desktop_workspace_roots SET signature=? WHERE tenant_id=? AND project_id=?`, signature, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	out := f.call("POST", "/workspace/file", body, 200)
	if out["content"] != "recovered native save" {
		t.Fatal("restored current proof did not recover the original accepted save")
	}
}

func TestDesktopWorkspaceHTTPRequiresClosedBodyAndReturnsScopedAcknowledgement(t *testing.T) {
	f, root := desktopWorkspaceFixture(t)
	request := desktopWorkspaceTestRequest(f, root, "")
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	decode := func() map[string]any {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	for _, change := range []func(map[string]any){
		func(m map[string]any) { m["extra"] = true },
		func(m map[string]any) { m["Project_ID"] = f.project },
		func(m map[string]any) { m["grant"].(map[string]any)["extra"] = true },
		func(m map[string]any) { delete(m, "expected_binding_sha256") },
		func(m map[string]any) { m["grant"].(map[string]any)["path"] = nil },
	} {
		body := decode()
		change(body)
		f.call("POST", "/workspace/desktop-binding", body, 400)
	}
	bound := f.call("POST", "/workspace/desktop-binding", request, 200)
	if bound["project_id"] != f.project || bound["binding_path_sha256"] != digest(root) || bound["binding_operation_id"] != request.Grant.OperationID || bound["configured"] != true {
		t.Fatalf("HTTP native ACK lost its selected scope: %v", bound)
	}
	metadata := f.call("GET", "/workspace?project_id="+f.project, nil, 200)
	if metadata["binding_sha256"] != bound["binding_sha256"] || metadata["binding_path_sha256"] != bound["binding_path_sha256"] || metadata["binding_operation_id"] != bound["binding_operation_id"] {
		t.Fatalf("metadata did not retain the native acknowledgement: %v", metadata)
	}
	if strings.Contains(stringify(bound), root) || bound["root"] != nil || bound["path"] != nil || bound["grant"] != nil {
		t.Fatal("binding ACK exposed a private path or grant")
	}
	expired := desktopWorkspaceTestRequest(f, root, bound["binding_sha256"].(string))
	expired.Grant.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	desktopWorkspaceTestSign(f.s.DesktopWorkspaceKey, &expired.Grant)
	assertWorkspaceLocalizedError(t, f, "POST", "/workspace/desktop-binding", expired, 410, "desktop_workspace_grant_expired", "desktop_workspace.grant_expired", "本次目录选择已过期，请重新选择后绑定", "The directory selection has expired. Choose the directory again before binding.")
}
