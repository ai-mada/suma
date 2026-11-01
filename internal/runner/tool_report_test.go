package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestReportToolsProbesCurrentAllowlistWithoutReplayingTaskResults(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	// An existing result remains in the original outbox; this mode is not Run.
	assignment := Assignment{RunID: "original-run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
	if err = j.Accept(assignment); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: assignment, ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(j.path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/runner/tool-inventory" {
			t.Error("single report replayed task state", r.URL.Path)
		}
		var body struct {
			WorkerID string           `json:"worker_id"`
			Items    []tc.Observation `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.WorkerID != "worker" || len(body.Items) != 1 || !body.Items[0].Available || len(body.Items[0].Profile.ExecutableSHA256) != 64 {
			t.Errorf("actual local observation absent: %+v", body)
		}
		_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":"2026-11-01T02:00:00Z"}`))
	}))
	defer server.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	w := Worker{Client: &Client{BaseURL: server.URL, ProjectID: "project", Token: "registered-token"}, Journal: j, ID: "worker", Exec: ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"local": {Executable: exe, ReadOnly: true}}}}
	report, recovered, err := w.ReportTools(context.Background())
	if err != nil || recovered || !report.Acked || report.ObservedAt != "2026-11-01T02:00:00Z" || calls != 1 || j.PendingInventory() != nil || len(j.Pending()) != 1 {
		t.Fatalf("single report changed unrelated state: %+v %t %v calls=%d", report, recovered, err, calls)
	}
	after, err := os.ReadFile(j.path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("single report changed the task journal", err)
	}
}

func TestReportToolsRejectsDifferentPendingWorkerBeforeControlRequest(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	original, err := j.NewInventory("http://127.0.0.1:4310", "project", "original-worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := Worker{Client: &Client{BaseURL: "http://invalid.invalid", ProjectID: "project", Token: "registered-token"}, Journal: j, ID: "other-worker"}
	if _, _, err = w.ReportTools(context.Background()); err == nil {
		t.Fatal("another worker recovered the pending inventory")
	}
	if pending := j.PendingInventory(); pending == nil || pending.OperationID != original.OperationID {
		t.Fatal("original operation was lost")
	}
}
