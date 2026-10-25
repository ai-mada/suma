package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func workspaceFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	root := t.TempDir()
	t.Setenv("SUMA_WORKSPACE_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	return f, root
}

func TestWorkspaceLostAcknowledgementReplaysOriginalResult(t *testing.T) {
	f, root := workspaceFixture(t)
	first := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": first["hash"], "content": "saved"})
	saved := f.call("POST", "/workspace/file", body, 200)
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("later external edit"), 0600); err != nil {
		t.Fatal(err)
	}
	replayed := f.call("POST", "/workspace/file", body, 200)
	if stringify(saved) != stringify(replayed) {
		t.Fatalf("replay changed original result: %v / %v", saved, replayed)
	}
	actual, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(actual) != "later external edit" {
		t.Fatal("retry overwrote later edit")
	}
	body["content"] = "different"
	f.call("POST", "/workspace/file", body, 409)
}

func TestWorkspaceConcurrentSavesRejectStaleWriter(t *testing.T) {
	f, root := workspaceFixture(t)
	var seq int
	var dbName, dbPath string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &dbName, &dbPath); err != nil {
		t.Fatal(err)
	}
	secondDB, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer secondDB.Close()
	secondHandler := (&Server{Service: &Service{DB: secondDB, Dialect: "sqlite"}, Engine: f.e}).Handler()
	first := f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	view := f.view
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for index, value := range []string{"writer one", "writer two"} {
		handler := f.handler
		if index == 1 {
			handler = secondHandler
		}
		body := map[string]any{"project_id": f.project, "idempotency_key": newID(), "path": "main.go", "expected_hash": first["hash"], "content": value}
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, _ := json.Marshal(body)
			r := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/workspace/file", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-Suma-Expected-User", view.ID)
			r.Header.Set("X-Suma-Expected-Organization", view.TenantID)
			r.AddCookie(f.cookie)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent save statuses: %v", counts)
	}
	actual, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(actual) != "writer one" && string(actual) != "writer two" {
		t.Fatal("file content corrupted")
	}
}

func TestWorkspaceReadNeverMutatesBusinessState(t *testing.T) {
	f, root := workspaceFixture(t)
	ctx := context.Background()
	if _, err := f.s.DB.ExecContext(ctx, `DROP TABLE suma_workspace_file_operations`); err != nil {
		t.Fatal(err)
	}
	var operations, activity int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&operations)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&activity)
	f.call("GET", "/workspace?project_id="+f.project, nil, 200)
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=main.go", nil, 200)
	var afterOperations, afterActivity, journal int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&afterOperations)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&afterActivity)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='suma_workspace_file_operations'`).Scan(&journal)
	if operations != afterOperations || activity != afterActivity || journal != 0 {
		t.Fatal("protected GET mutated business state or recreated operation journal")
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("GET mutated workspace file", err)
	}
}

func TestWorkspaceRecoversPreparedBeforeAndAfterFilesystemCommit(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			f, root := workspaceFixture(t)
			body := f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "prepared content"})
			response := fileResponse("main.go", "prepared content")
			hash := digest(f.user.ID + stringify(map[string]any{"kind": "workspace.file", "body": body}))
			ctx, tx, cancel, err := f.s.beginAuthorityTx(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			auth, err := f.s.authorizeTx(ctx, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
			if err == nil {
				err = f.s.prepareWorkspaceOperationTx(ctx, auth, str(body, "idempotency_key"), hash, root, "main.go", "original", "prepared content", response)
			}
			if err == nil {
				err = tx.Commit()
			}
			tx.Rollback()
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if committed {
				if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("prepared content"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			out := f.call("POST", "/workspace/file", body, 200)
			if out["content"] != "prepared content" {
				t.Fatal("wrong recovered response")
			}
			actual, _ := os.ReadFile(filepath.Join(root, "main.go"))
			if string(actual) != "prepared content" {
				t.Fatal("prepared content not recovered")
			}
			var state string
			f.s.DB.QueryRow(`SELECT state FROM suma_workspace_file_operations WHERE operation_id=?`, body["idempotency_key"]).Scan(&state)
			if state != "completed" {
				t.Fatalf("journal not finalized: %s", state)
			}
		})
	}
}

func TestWorkspaceRejectsEscapesSecretsAndSymlinks(t *testing.T) {
	f, root := workspaceFixture(t)
	for path, want := range map[string]int{"../main.go": 404, ".env": 403, ".aws/credentials": 403, "main.go:stream": 403} {
		f.call("GET", "/workspace/file?project_id="+f.project+"&path="+path, nil, want)
	}
	if err := os.Symlink(filepath.Join(root, "main.go"), filepath.Join(root, "linked.go")); err != nil {
		t.Logf("symlink unavailable: %v", err)
		return
	}
	f.call("GET", "/workspace/file?project_id="+f.project+"&path=linked.go", nil, 403)
	f.call("POST", "/workspace/file", f.body(map[string]any{"path": "linked.go", "content": "bad", "expected_hash": digest("original")}), 403)
}

func TestWorkspaceUnknownPreparedResultRequiresReconciliation(t *testing.T) {
	f, root := workspaceFixture(t)
	body := f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "saved"})
	f.call("POST", "/workspace/file", body, 200)
	if _, err := f.s.DB.Exec(`UPDATE suma_workspace_file_operations SET state='prepared' WHERE operation_id=?`, body["idempotency_key"]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("unrelated edit"), 0600); err != nil {
		t.Fatal(err)
	}
	out := f.call("POST", "/workspace/file", body, 409)
	if out["error"].(map[string]any)["code"] != "workspace_reconciliation_required" {
		t.Fatalf("uncertain result misclassified: %v", out)
	}
	actual, _ := os.ReadFile(filepath.Join(root, "main.go"))
	if string(actual) != "unrelated edit" {
		t.Fatal("unknown external state overwritten")
	}
}

func TestRecordPaginationAndCompleteKeywordSearch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 1005; i++ {
		r := Record{ID: fmt.Sprintf("row-%04d", i), TenantID: f.user.TenantID, ProjectID: f.project, Version: 1, Title: "ordinary", Status: "todo", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z", DependsOn: []string{}, AcceptanceCriteria: []string{}, Data: map[string]any{}}
		if i == 1004 {
			r.Title = "Needle 100%_literal"
		}
		if err = f.s.insertRecord(ctx, tx, r, "documents"); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []Record{
		{ID: "other-project", TenantID: f.user.TenantID, ProjectID: "unauthorized-project", Version: 1, Title: "Needle 100%_literal", Data: map[string]any{}},
		{ID: "other-tenant", TenantID: "another-tenant", ProjectID: f.project, Version: 1, Title: "Needle 100%_literal", Data: map[string]any{}},
	} {
		if err = f.s.insertRecord(ctx, tx, r, "documents"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		items, total, next, e := f.s.ListPage(ctx, f.actor, f.project, "documents", 200, cursor)
		if e != nil {
			t.Fatal(e)
		}
		if total != 1005 {
			t.Fatalf("total truncated: %d", total)
		}
		for _, r := range items {
			if seen[r.ID] {
				t.Fatalf("duplicate %s", r.ID)
			}
			seen[r.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 1005 {
		t.Fatalf("missing records: %d", len(seen))
	}
	if _, _, _, err = f.s.ListPage(ctx, f.actor, f.project, "tasks", 200, cursor); err == nil {
		t.Fatal("cursor crossed record kind")
	}
	result, err := f.s.SearchKeywords(ctx, f.actor, f.project, "needle 100%_literal")
	if err != nil {
		t.Fatal(err)
	}
	hits := result["items"].([]map[string]any)
	if result["total"] != 1 || len(hits) != 1 || hits[0]["id"] != "row-1004" {
		t.Fatalf("old record not searched: %v", result)
	}
	result, err = f.s.SearchKeywords(ctx, f.actor, f.project, "%")
	if err != nil {
		t.Fatal(err)
	}
	if result["total"] != 1 {
		t.Fatalf("literal wildcard was interpreted as SQL pattern: %v", result["total"])
	}
}
