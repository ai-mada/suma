package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An explicit, owned hold reuses the actual HTTP/Runner/model/ledger journey.
// It does not seed fabricated model results or start an extra model process.
func holdActualModelGatewayUI(t *testing.T, f *fixture, metadata map[string]any) {
	t.Helper()
	directory, web := os.Getenv("SUMA_MODEL_GATEWAY_UI_DIR"), os.Getenv("SUMA_MODEL_GATEWAY_WEB_DIR")
	if directory == "" && web == "" {
		return
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(repository, ".cache", "e2e", "model-118")
	stage := "T-MODEL-118"
	if metadata["stage"] == "T-WORKSPACE-120" {
		owned = filepath.Join(repository, ".cache", "e2e", "workspace-120")
		stage = "T-WORKSPACE-120"
	}
	directory, err = filepath.Abs(directory)
	if err != nil || !under(owned, directory) {
		t.Fatal("model GUI receipts must stay inside the owned stage")
	}
	web, err = filepath.Abs(web)
	if err != nil || !under(owned, web) {
		t.Fatal("model GUI assets must stay inside the owned stage")
	}
	if _, err := os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal("GUI hold requires a fresh exact directory", err)
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != directory {
		t.Fatal("GUI hold cannot traverse a linked directory")
	}
	before := modelSelectionGUICounts(t, f)
	server, origin, _ := modelSelectionGUIServer(t, f, web, "127.0.0.1")
	defer server.Close()
	private := map[string]any{"base": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "metadata": metadata}
	modelSelectionGUIJSON(t, filepath.Join(directory, "context.private.json"), private)
	modelSelectionGUIJSON(t, filepath.Join(directory, "ready.public.json"), map[string]any{"stage": stage, "base": origin, "project_id": f.project, "metadata": metadata, "kind": "actual_native_gateway_and_ledger", "new_model_requests_during_ui": 0, "before": before})
	t.Logf("owned actual gateway UI ready: %s", origin)
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
waiting:
	for {
		select {
		case <-deadline.C:
			t.Fatal("explicit GUI hold timed out; no process restart authorized")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(directory, "stop.request")); err == nil {
				break waiting
			}
		}
	}
	after := modelSelectionGUICounts(t, f)
	for table, count := range before {
		if after[table] != count {
			t.Fatalf("GUI created unexpected execution/review/accounting in %s", table)
		}
	}
	// Retain unique evidence before t.TempDir cleanup. The SQLite snapshot is
	// task-private, ignored and parameterized; never overwrite an existing file.
	snapshot := filepath.Join(directory, "actual.sqlite")
	if _, err := f.s.DB.Exec(`VACUUM INTO ?`, snapshot); err != nil {
		t.Fatal("retain owned actual database snapshot", err)
	}
	data, _ := json.Marshal(metadata)
	modelSelectionGUIJSON(t, filepath.Join(directory, "closed.public.json"), map[string]any{"stage": stage, "status": "closed_without_new_execution", "metadata": json.RawMessage(data), "counts_unchanged": true, "before": before, "after": after, "database_snapshot": "actual.sqlite", "closed_at": time.Now().UTC().Format(time.RFC3339Nano)})
}
