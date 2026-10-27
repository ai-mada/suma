package cli

import (
	"context"
	"encoding/json"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The CLI must preserve scope/idempotency and reject malformed JSON locally;
// it only uses the control API, never a host MCP endpoint or filesystem path.
func TestWorkspaceObservationCLIScopedReadsAndExactCreate(t *testing.T) {
	scope := woc.Scope{RunID: "run-one", NodeID: "node-one", Attempt: 1, Fence: 2, WorkerID: "worker-one", ToolProfileID: "native-one", ProfileSHA256: strings.Repeat("a", 64)}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("project_id") != "project-one" {
			t.Error("project scope missing")
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"project_id":"project-one","items":[]}`))
			return
		}
		var req woc.CreateRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Scope != scope || req.ProjectID != "project-one" || req.IdempotencyKey != "read-one" || req.Operation != "read" || req.Arguments.Path != "src/main.ts" {
			t.Error("create request changed", req)
		}
		if r.URL.Path != "/api/v1/workspace/observations" {
			t.Error(r.URL)
		}
		_ = json.NewEncoder(w).Encode(woc.Observation{ID: "observation-one", ProjectID: req.ProjectID, Scope: req.Scope, State: "requested", Operation: req.Operation, Arguments: req.Arguments})
	}))
	defer server.Close()
	base := []string{"--control", server.URL, "--project", "project-one", "workspace"}
	var output, stderr strings.Builder
	execute := func(args []string, input string) int {
		output.Reset()
		stderr.Reset()
		return Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(input), &output, &stderr)
	}
	if code := execute([]string{"targets"}, ""); code != 0 {
		t.Fatal(code, stderr.String())
	}
	req := woc.CreateRequest{Scope: scope, Operation: "read", Arguments: woc.Arguments{Path: "src/main.ts"}, IdempotencyKey: "read-one"}
	raw, _ := json.Marshal(req)
	if code := execute([]string{"observe", "--file", "-"}, string(raw)); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(output.String(), "observation-one") {
		t.Fatal(output.String())
	}
	for _, bad := range []string{`{"scope":{},"operation":"read","operation":"list"}`, `{"unknown":1}`, strings.Replace(string(raw), "src/main.ts", "../secret", 1)} {
		if code := execute([]string{"observe", "--file", "-"}, bad); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 2 {
		t.Fatal("invalid input reached transport", calls)
	}
}
