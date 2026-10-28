package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorkspaceContextRejectsSameBytesAfterBindingSwitch(t *testing.T) {
	f, root := workspaceFixture(t)
	first := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "main.go"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": other}), 200)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": first["hash"], "expected_context_sha256": first["context_sha256"], "content": "must not save"})
	out := f.call("POST", "/workspace/file", body, 409)
	api := out["error"].(map[string]any)
	if api["code"] != "workspace_context_changed" || api["details"].(map[string]any)["not_accepted"] != true {
		t.Fatalf("stale context did not prove rejection: %v", out)
	}
	for _, directory := range []string{root, other} {
		actual, err := os.ReadFile(filepath.Join(directory, "main.go"))
		if err != nil || string(actual) != "original" {
			t.Fatalf("stale editor modified a workspace: %q %v", actual, err)
		}
	}
	for _, table := range []string{"suma_workspace_file_operations", "suma_workspace_accepted_operations"} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("rejected context was accepted in %s: %d %v", table, count, err)
		}
	}
}

func workspaceContextTestRead(t *testing.T, f *fixture) map[string]any {
	t.Helper()
	return f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
}

func workspaceContextTestBody(f *fixture, document map[string]any, content string) map[string]any {
	return f.body(map[string]any{"path": document["path"], "expected_hash": document["hash"], "expected_context_sha256": document["context_sha256"], "content": content})
}

func TestWorkspaceContextABARequiresFreshReadAndKeepsOriginalAcknowledgement(t *testing.T) {
	f, root := workspaceFixture(t)
	first := workspaceContextTestRead(t, f)
	body := workspaceContextTestBody(f, first, "first saved")
	saved := f.call("POST", "/workspace/file", body, 200)
	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "main.go"), []byte("first saved"), 0600); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": other}), 200)
	if replayed := f.call("POST", "/workspace/file", body, 200); stringify(replayed) != stringify(saved) {
		t.Fatalf("binding switch changed original ACK: %v / %v", replayed, saved)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	stale := workspaceContextTestBody(f, saved, "must not save")
	f.call("POST", "/workspace/file", stale, 409)
	fresh := workspaceContextTestRead(t, f)
	if fresh["context_sha256"] == first["context_sha256"] || fresh["workspace_name"] != filepath.Base(root) || fresh["project_id"] != f.project {
		t.Fatalf("A -> B -> A revived the previous context: %v / %v", first, fresh)
	}
	if len(fresh) != 7 {
		t.Fatalf("file document leaked fields beyond the public contract: %v", fresh)
	}
	out := f.call("POST", "/workspace/file", workspaceContextTestBody(f, fresh, "fresh save"), 200)
	if out["context_sha256"] != fresh["context_sha256"] || out["workspace_name"] != fresh["workspace_name"] {
		t.Fatal("successful save changed the editor's directory context", out)
	}
}

func TestWorkspaceContextRejectsMalformedInputWithoutAcceptance(t *testing.T) {
	f, root := workspaceFixture(t)
	first := workspaceContextTestRead(t, f)
	for _, value := range []any{nil, "", "short", strings.Repeat("A", 64), strings.Repeat("x", 64), 1, map[string]any{}} {
		body := workspaceContextTestBody(f, first, "must not save")
		body["expected_context_sha256"] = value
		out := f.call("POST", "/workspace/file", body, 400)
		if out["error"].(map[string]any)["code"] != "invalid_input" {
			t.Fatal("malformed context had unexpected code", out)
		}
	}
	body := workspaceContextTestBody(f, first, "must not save")
	delete(body, "expected_context_sha256")
	f.call("POST", "/workspace/file", body, 400)
	body = workspaceContextTestBody(f, first, "must not save")
	body["extra"] = "not part of request"
	f.call("POST", "/workspace/file", body, 400)
	for _, table := range []string{"suma_workspace_file_operations", "suma_workspace_accepted_operations"} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("malformed request entered %s: %d %v", table, count, err)
		}
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("malformed context changed file", err)
	}
}

func TestWorkspaceContextIsNotProjectAuthority(t *testing.T) {
	f, root := workspaceFixture(t)
	first := workspaceContextTestRead(t, f)
	other := f.call("POST", "/projects", map[string]any{"title": "another workspace", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("POST", "/workspace/binding", map[string]any{"project_id": other, "path": root, "idempotency_key": newID()}, 200)
	otherDoc := f.call("GET", "/workspace/file?project_id="+other+"&path=main.go", nil, 200)
	if otherDoc["context_sha256"] == first["context_sha256"] {
		t.Fatal("context digest crossed the project boundary")
	}
	body := workspaceContextTestBody(f, first, "must not save")
	body["project_id"] = other
	f.call("POST", "/workspace/file", body, 409)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.saveWorkspace(context.Background(), f.actor, f.project, workspaceContextTestBody(f, first, "must not save"))
	requireAuthorityStatus(t, err, 403)
}

func TestWorkspaceContextInitialSchemaRejectsOlderDeclaredShapesBeforeWrites(t *testing.T) {
	for _, oldShape := range []string{"missing_binding_id", "missing_rejection_receipts"} {
		t.Run(oldShape, func(t *testing.T) {
			f, _ := workspaceFixture(t)
			change := `ALTER TABLE suma_workspace_bindings DROP COLUMN binding_id`
			if oldShape == "missing_rejection_receipts" {
				change = `DROP TABLE suma_workspace_file_rejections`
			}
			if _, err := f.s.DB.Exec(change); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.DB.Exec(`DROP TABLE suma_workspace_accepted_operations`); err != nil {
				t.Fatal(err)
			}
			err := f.s.Initialize(context.Background())
			if !errors.Is(err, store.ErrInitialSchema) {
				t.Fatalf("older declared schema was not rejected: %v", err)
			}
			var created int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='suma_workspace_accepted_operations'`).Scan(&created); err != nil || created != 0 {
				t.Fatalf("schema rejection wrote current tables: %d %v", created, err)
			}
			if oldShape == "missing_rejection_receipts" {
				if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='suma_workspace_file_rejections'`).Scan(&created); err != nil || created != 0 {
					t.Fatalf("initialization silently added a rejection ledger: %d %v", created, err)
				}
			}
		})
	}
}

func workspaceContextTestPrepare(t *testing.T, f *fixture, root string, document map[string]any) map[string]any {
	t.Helper()
	body := workspaceContextTestBody(f, document, "prepared save")
	out := fileResponse("main.go", "prepared save")
	out["project_id"], out["workspace_name"], out["context_sha256"] = f.project, document["workspace_name"], document["context_sha256"]
	hash := digest(f.user.ID + stringify(map[string]any{"kind": "workspace.file", "body": body}))
	ctx, tx, cancel, err := f.s.beginAuthorityTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := f.s.authorizeTx(ctx, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err == nil {
		err = f.s.prepareWorkspaceOperationTx(ctx, auth, str(body, "idempotency_key"), hash, root, "main.go", str(document, "content"), "prepared save", out)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestWorkspaceContextPreparedRecoveryNeverProvesNotAcceptedAfterRootChanges(t *testing.T) {
	for _, change := range []string{"binding", "root"} {
		t.Run(change, func(t *testing.T) {
			f, root := workspaceFixture(t)
			document := workspaceContextTestRead(t, f)
			body := workspaceContextTestPrepare(t, f, root, document)
			if change == "binding" {
				if _, err := f.s.DB.Exec(`UPDATE suma_workspace_bindings SET binding_id=? WHERE project_id=?`, newID(), f.project); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(root, filepath.Join(t.TempDir(), "original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := f.call("POST", "/workspace/file", body, 409)
			if api := out["error"].(map[string]any); api["code"] != "workspace_reconciliation_required" || strings.Contains(stringify(api), "not_accepted") {
				t.Fatal("accepted operation was represented as a new rejection", out)
			}
			var state string
			if err := f.s.DB.QueryRow(`SELECT state FROM suma_workspace_file_operations WHERE operation_id=?`, body["idempotency_key"]).Scan(&state); err != nil || state != "prepared" {
				t.Fatal("uncertain operation lost its prepared record", state, err)
			}
			actual, err := os.ReadFile(filepath.Join(root, "main.go"))
			if err != nil || string(actual) != "original" {
				t.Fatal("recovery wrote to a different directory context", err)
			}
		})
	}
}

func TestWorkspaceContextAcceptedRecordWithoutJournalCannotProveRejection(t *testing.T) {
	f, _ := workspaceFixture(t)
	document := workspaceContextTestRead(t, f)
	body := workspaceContextTestBody(f, document, "uncertain")
	body["expected_context_sha256"] = strings.Repeat("0", 64)
	if _, err := f.s.DB.Exec(`INSERT INTO suma_workspace_accepted_operations(tenant_id,project_id,operation_id,request_hash,accepted_by,accepted_at) VALUES(?,?,?,?,?,?)`, f.user.TenantID, f.project, body["idempotency_key"], strings.Repeat("0", 64), f.user.ID, now()); err != nil {
		t.Fatal(err)
	}
	out := f.call("POST", "/workspace/file", body, 409)
	if api := out["error"].(map[string]any); api["code"] != "workspace_reconciliation_required" || strings.Contains(stringify(api), "not_accepted") {
		t.Fatal("accepted receipt without journal was called unaccepted", out)
	}
}

func TestWorkspaceContextTracksActualOpenedRootIdentityAndReadPublication(t *testing.T) {
	f, root := workspaceFixture(t)
	document := workspaceContextTestRead(t, f)
	var basis workspaceFileContext
	err := f.s.withCurrentRead(context.Background(), f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		held, observed, err := f.s.openWorkspaceFileContextTx(context.Background(), auth.tx, f.user.TenantID, f.project, "")
		if err == nil {
			basis = observed
			held.Close()
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(t.TempDir(), "original")
	if err = os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	// This is the final publication guard used by GetWorkspaceFile; equal paths
	// and bytes cannot certify that an earlier opened root is still current.
	err = f.s.withCurrentRead(context.Background(), f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		return f.s.verifyWorkspaceFileContextTx(context.Background(), auth.tx, f.user.TenantID, f.project, basis)
	})
	requireAuthorityStatus(t, err, 409)
	out := f.call("POST", "/workspace/file", workspaceContextTestBody(f, document, "must not save"), 409)
	if api := out["error"].(map[string]any); api["code"] != "workspace_context_changed" || api["details"].(map[string]any)["not_accepted"] != true {
		t.Fatal("new request against a replaced root was not rejected safely", out)
	}
	fresh := workspaceContextTestRead(t, f)
	if fresh["context_sha256"] == document["context_sha256"] {
		t.Fatal("different OS root identities shared one editor context")
	}
	for _, directory := range []string{root, original} {
		actual, readErr := os.ReadFile(filepath.Join(directory, "main.go"))
		if readErr != nil || string(actual) != "original" {
			t.Fatal("a replaced root was modified", readErr)
		}
	}
}

func TestWorkspaceContextOrdinaryBindingOriginalReceiptNeverRebinds(t *testing.T) {
	f, parent := workspaceFixture(t)
	a, b := filepath.Join(parent, "a"), filepath.Join(parent, "b")
	for _, directory := range []string{a, b} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	firstBody := f.body(map[string]any{"path": a})
	first := f.call("POST", "/workspace/binding", firstBody, 200)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": b}), 200)
	beforeReplay := workspaceContextTestRead(t, f)
	var bindingID string
	if err := f.s.DB.QueryRow(`SELECT binding_id FROM suma_workspace_bindings WHERE project_id=?`, f.project).Scan(&bindingID); err != nil {
		t.Fatal(err)
	}
	replayed := f.call("POST", "/workspace/binding", firstBody, 200)
	if stringify(replayed) != stringify(first) {
		t.Fatal("ordinary binding replay changed the original acknowledgement", replayed, first)
	}
	if current := workspaceContextTestRead(t, f); current["context_sha256"] != beforeReplay["context_sha256"] || current["workspace_name"] != "b" {
		t.Fatal("original ordinary binding replay reselected directory A", current)
	}
	if err := os.Rename(a, filepath.Join(parent, "a-unavailable")); err != nil {
		t.Fatal(err)
	}
	if replayed = f.call("POST", "/workspace/binding", firstBody, 200); stringify(replayed) != stringify(first) {
		t.Fatal("unavailable original directory prevented original ACK replay", replayed)
	}
	changed := f.body(map[string]any{"path": b, "idempotency_key": firstBody["idempotency_key"]})
	out := f.call("POST", "/workspace/binding", changed, 409)
	if out["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal("binding operation ID accepted a different request", out)
	}
	var afterReplayID string
	if err := f.s.DB.QueryRow(`SELECT binding_id FROM suma_workspace_bindings WHERE project_id=?`, f.project).Scan(&afterReplayID); err != nil || afterReplayID != bindingID {
		t.Fatal("ordinary receipt replay generated a binding identity", err)
	}
	if first["root"] != nil || first["project_id"] != f.project || first["binding_sha256"] == nil || first["name"] != "a" {
		t.Fatal("ordinary binding receipt is not scoped path-free metadata", first)
	}
}

func TestWorkspaceContextRejectedOperationCannotReviveAfterPhysicalRootABA(t *testing.T) {
	f, root := workspaceFixture(t)
	document := workspaceContextTestRead(t, f)
	body := workspaceContextTestBody(f, document, "must never save")
	original, replacement := filepath.Join(t.TempDir(), "original"), filepath.Join(t.TempDir(), "replacement")
	if err := os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	rejected := f.call("POST", "/workspace/file", body, 409)
	if err := os.Rename(root, replacement); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, root); err != nil {
		t.Fatal(err)
	}
	if restored := workspaceContextTestRead(t, f); restored["context_sha256"] != document["context_sha256"] {
		t.Fatal("fixture did not restore the exact original OS directory identity")
	}
	replayed := f.call("POST", "/workspace/file", body, 409)
	if stringify(replayed["error"]) != stringify(rejected["error"]) {
		t.Fatal("restoring the original root changed a definitive rejection", replayed, rejected)
	}
	var dbPath string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restarted := &Service{DB: reopened, Dialect: "sqlite", DataDir: f.s.DataDir}
	_, err = restarted.saveWorkspace(context.Background(), f.actor, f.project, body)
	var api *APIError
	if !errors.As(err, &api) || api.Code != "workspace_context_changed" || api.Details.(map[string]any)["not_accepted"] != true {
		t.Fatalf("rejection did not survive service restart: %v", err)
	}
	changed := workspaceContextTestBody(f, document, "different request")
	changed["idempotency_key"] = body["idempotency_key"]
	out := f.call("POST", "/workspace/file", changed, 409)
	if out["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal("rejected operation ID was reused for different content", out)
	}
	for _, directory := range []string{root, replacement} {
		actual, readErr := os.ReadFile(filepath.Join(directory, "main.go"))
		if readErr != nil || string(actual) != "original" {
			t.Fatal("a rejected stale packet wrote after root restoration", readErr)
		}
	}
	for table, want := range map[string]int{"suma_workspace_file_operations": 0, "suma_workspace_accepted_operations": 0, "suma_workspace_file_rejections": 1} {
		var count int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatalf("wrong durable rejection/acceptance state in %s: %d %v", table, count, err)
		}
	}
	if err := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.saveWorkspace(context.Background(), f.actor, f.project, body)
	requireAuthorityStatus(t, err, 401)
}

func TestWorkspaceContextRejectionFailureCannotPublishNotAcceptedProof(t *testing.T) {
	for _, failure := range []string{"insert", "commit"} {
		t.Run(failure, func(t *testing.T) {
			f, root := workspaceFixture(t)
			document := workspaceContextTestRead(t, f)
			var trigger string
			if failure == "insert" {
				trigger = `CREATE TRIGGER reject_workspace_rejection BEFORE INSERT ON suma_workspace_file_rejections BEGIN SELECT RAISE(ABORT,'rejection storage unavailable'); END`
			} else {
				var foreignKeys int
				if err := f.s.DB.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
					t.Fatalf("deferred commit fixture requires foreign keys: %d %v", foreignKeys, err)
				}
				if _, err := f.s.DB.Exec(`CREATE TABLE workspace_rejection_commit_guard(operation_id TEXT PRIMARY KEY,project_id TEXT NOT NULL REFERENCES suma_projects(id) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
					t.Fatal(err)
				}
				trigger = `CREATE TRIGGER reject_workspace_rejection AFTER INSERT ON suma_workspace_file_rejections BEGIN INSERT INTO workspace_rejection_commit_guard(operation_id,project_id) VALUES(NEW.operation_id,'missing-project'); END`
			}
			if _, err := f.s.DB.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			body := workspaceContextTestBody(f, document, "must not save")
			body["expected_context_sha256"] = strings.Repeat("0", 64)
			out := f.call("POST", "/workspace/file", body, 500)
			if strings.Contains(stringify(out), "not_accepted") || out["error"].(map[string]any)["code"] == "workspace_context_changed" {
				t.Fatal("failed durable rejection published a definitive proof", out)
			}
			for _, table := range []string{"suma_workspace_file_rejections", "suma_workspace_file_operations", "suma_workspace_accepted_operations"} {
				var count int
				if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("failed %s left durable state in %s: %d %v", failure, table, count, err)
				}
			}
			actual, err := os.ReadFile(filepath.Join(root, "main.go"))
			if err != nil || string(actual) != "original" {
				t.Fatal("failed rejection changed a file", err)
			}
		})
	}
}

func TestWorkspaceContextInitialSchemaResumesAfterInterruptedWorkspaceDDL(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "interrupted.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	// A conflicting index forces real DDL failure at the operation table. The
	// rejection table must already exist, while no binding table may precede it.
	if _, err = db.Exec(`CREATE INDEX suma_workspace_file_operations ON suma_schema_baselines(version)`); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
	if err = s.Initialize(ctx); err == nil {
		t.Fatal("fixture did not interrupt workspace initialization")
	}
	for table, want := range map[string]int{"suma_workspace_file_rejections": 1, "suma_workspace_bindings": 0} {
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != want {
			t.Fatalf("workspace initialization order stranded %s: %d %v", table, count, err)
		}
	}
	if _, err = db.Exec(`DROP INDEX suma_workspace_file_operations`); err != nil {
		t.Fatal(err)
	}
	if err = s.Initialize(ctx); err != nil {
		t.Fatalf("current initial setup could not resume after DDL failure: %v", err)
	}
	if err = s.Initialize(ctx); err != nil {
		t.Fatalf("completed current setup could not restart: %v", err)
	}
}
