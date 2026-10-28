package platform

import (
	"context"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Opt-in browser inspection uses a fresh initialized database and one real
// control-plane preview. The inventory is an owned protocol fixture: it never
// invokes a vendor CLI/model, starts a run or implies vendor acceptance.
func TestOpenCodeWorkbenchFixedConfigurationPreview(t *testing.T) {
	directory := os.Getenv("SUMA_OPENCODE_UI_DIR")
	if directory == "" {
		t.Skip("opt-in local browser evidence")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", "opencode-104"))
	if err != nil {
		t.Fatal(err)
	}
	directory, err = filepath.Abs(directory)
	web, webErr := filepath.Abs(os.Getenv("SUMA_OPENCODE_WEB_DIR"))
	if err != nil || webErr != nil || !under(root, directory) || !under(root, web) {
		t.Fatal("UI evidence must stay in this stage cache")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	f, token, request, profile := openCodeExecutionFixture(t)
	plan, err := f.s.CreateTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	server := &Server{Service: f.s, Engine: f.e, WebDir: web, PublicURL: origin, PublicURLSource: "configured", AllowedOrigins: []string{origin}}
	control := httptest.NewUnstartedServer(server.Handler())
	_ = control.Listener.Close()
	control.Listener = listener
	control.Start()
	defer control.Close()
	actor := toolActorForTest(t, f, token)
	refresh := func() {
		_, reportErr := f.s.ReportToolInventory(context.Background(), actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true, WorkspaceObservationProtocol: "workspace-observation-v1", WorkspaceWriteProtocol: "workspace-write-v1"}}})
		if reportErr != nil {
			t.Fatal(reportErr)
		}
	}
	codexActualJSON(t, filepath.Join(directory, "ready.public.json"), map[string]any{"origin": origin, "project_id": f.project, "task_id": request.TaskID, "plan_id": plan.ID, "tool_profile_id": profile.ID, "model_calls": 0, "vendor_cli_calls": 0, "inventory_basis": "owned_protocol_fixture"})
	t.Logf("OpenCode fixed configuration GUI ready: %s", origin)
	deadline, nextRefresh := time.Now().Add(10*time.Minute), time.Now().Add(10*time.Second)
	for time.Now().Before(deadline) {
		if _, err = os.Stat(filepath.Join(directory, "stop.request")); err == nil {
			break
		}
		if time.Now().After(nextRefresh) {
			refresh()
			nextRefresh = time.Now().Add(10 * time.Second)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if expertCount(t, f, "dag_runs") != 0 || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("configuration inspection unexpectedly started execution")
	}
	codexActualJSON(t, filepath.Join(directory, "closed.public.json"), map[string]any{"runs": 0, "usage_attempts": 0, "model_calls": 0, "vendor_cli_calls": 0})
}
