package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func TestWorkspaceWriteCLIStableCASAndUncertainExit(t *testing.T) {
	scope := wwc.Scope{RunID: "run-one", NodeID: "code", Attempt: 1, Fence: 2, WorkerID: "worker-one", ToolProfileID: "native", ProfileSHA256: strings.Repeat("a", 64)}
	request := wwc.CreateRequest{Scope: scope, WorkspacePolicySHA256: strings.Repeat("b", 64), Path: "source.txt", ExpectedSHA256: strings.Repeat("c", 64), Content: "new CLI text\n"}
	calls := 0
	var firstKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/writes" || r.URL.Query().Get("project_id") != "project-one" {
			t.Error(r.Method, r.URL)
		}
		var req wwc.CreateRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil || wwc.ValidateCreate(req) != nil || req.ProjectID != "project-one" || req.Scope != scope || req.ExpectedSHA256 != request.ExpectedSHA256 || req.Content != request.Content {
			t.Error("submitted CAS changed", req.Path)
		}
		if firstKey == "" {
			firstKey = req.IdempotencyKey
		} else if req.IdempotencyKey != firstKey {
			t.Error("identical CLI retry changed operation key")
		}
		command := wwc.Command{ID: "command-one", ProjectID: req.ProjectID, Version: 3, State: "applied", Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, Path: req.Path, ExpectedSHA256: req.ExpectedSHA256, ContentSHA256: req.ContentSHA256, RequestSHA256: wwc.RequestDigest(req), ExecutionFence: 1}
		command.Result = &wwc.Effect{NativeOperationID: wwc.NativeOperationID(command.ID), Path: req.Path, PreviousSHA256: req.ExpectedSHA256, SHA256: req.ContentSHA256, Generation: 1}
		if calls == 2 {
			command.State = "reconciling"
			command.Result = nil
			command.ErrorCode = "operation_unknown"
		}
		_ = json.NewEncoder(w).Encode(command)
	}))
	defer server.Close()
	raw, _ := json.Marshal(request)
	base := []string{"--control", server.URL, "--project", "project-one", "workspace", "save", "--file", "-", "--wait"}
	var out, stderr strings.Builder
	if code := Execute(context.Background(), base, strings.NewReader(string(raw)), &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if strings.Contains(out.String(), request.Content) || !strings.Contains(out.String(), "applied") {
		t.Fatal("metadata output differs", out.String())
	}
	out.Reset()
	stderr.Reset()
	if code := Execute(context.Background(), base, strings.NewReader(string(raw)), &out, &stderr); code != 1 || !strings.Contains(out.String(), "reconciling") {
		t.Fatal("unknown was treated as saved", code, out.String(), stderr.String())
	}
	if calls != 2 {
		t.Fatal("uncertain CLI save was automatically retried", calls)
	}
	for _, bad := range []string{`{"content":"a","content":"b"}`, strings.Replace(string(raw), "source.txt", "../secret", 1)} {
		if code := Execute(context.Background(), base, strings.NewReader(bad), &out, &stderr); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 2 {
		t.Fatal("invalid request reached transport")
	}
}

func TestWorkspaceWriteCLIRejectsReceiptForAnotherCommand(t *testing.T) {
	scope := wwc.Scope{RunID: "run-one", NodeID: "code", Attempt: 1, Fence: 1, WorkerID: "worker", ToolProfileID: "native", ProfileSHA256: strings.Repeat("a", 64)}
	req := wwc.CreateRequest{ProjectID: "project-one", Scope: scope, WorkspacePolicySHA256: strings.Repeat("b", 64), Path: "source.txt", ExpectedSHA256: strings.Repeat("c", 64), ContentSHA256: wwc.ContentDigest("text")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(wwc.Command{ID: "another-command", ProjectID: req.ProjectID, Version: 1, State: "requested", Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, Path: req.Path, ExpectedSHA256: req.ExpectedSHA256, ContentSHA256: req.ContentSHA256, RequestSHA256: wwc.RequestDigest(req)})
	}))
	defer server.Close()
	var out, stderr strings.Builder
	if code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one", "workspace", "save-status", "original-command"}, strings.NewReader(""), &out, &stderr); code != 1 {
		t.Fatal("accepted another command receipt", code, out.String(), stderr.String())
	}
}
