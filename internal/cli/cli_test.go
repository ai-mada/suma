package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func invoke(t *testing.T, url string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	all := append([]string{"--control", url, "--project", "project-one", "--token", "test-secret"}, args...)
	code := Execute(context.Background(), all, strings.NewReader(""), &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestProjectQueryAndAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/tasks" || r.URL.Query().Get("project_id") != "project-one" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("unexpected request: %s %s auth=%v", r.Method, r.URL, r.Header.Get("Authorization") != "")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"task-1","title":"Real task"}],"total":1}`))
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL, "tasks")
	if code != 0 || !strings.Contains(out, "Real task") || stderr != "" {
		t.Fatalf("code=%d out=%s error=%s", code, out, stderr)
	}
}

func TestStructuredServerErrorAndExit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"code":"forbidden","message":"project access denied","details":{"project_id":"project-one"}}}`))
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL, "runs")
	if code != 1 || out != "" || !strings.Contains(stderr, `"code": "forbidden"`) || strings.Contains(stderr, "test-secret") {
		t.Fatalf("code=%d out=%s error=%s", code, out, stderr)
	}
}

func TestCommandFileHasStableOperationAndScope(t *testing.T) {
	file := filepath.Join(t.TempDir(), "command.json")
	if err := os.WriteFile(file, []byte(`{"action":"pause","expected_version":3,"payload":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/runs/run-1/commands" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["project_id"] != "project-one" || body["expected_version"] != float64(3) {
			t.Errorf("wrong body: %v", body)
		}
		keys = append(keys, body["idempotency_key"].(string))
		w.Write([]byte(`{"id":"run-1","state":"paused"}`))
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		if code, _, stderr := invoke(t, server.URL, "run", "command", "run-1", "--file", file); code != 0 {
			t.Fatal(stderr)
		}
	}
	if len(keys) != 2 || len(keys[0]) < 8 || keys[0] != keys[1] {
		t.Fatalf("unstable operation keys: %v", keys)
	}
}

func TestNonTerminalDefaultPrintsDoctorAndExits(t *testing.T) {
	paths := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths[r.URL.Path] = true
		if r.Method != "GET" {
			t.Errorf("doctor mutated: %s", r.Method)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL)
	if code != 0 || !strings.Contains(out, `"health"`) || stderr != "" || !paths["/healthz"] || !paths["/api/v1/auth/session"] || !paths["/api/v1/capabilities"] {
		t.Fatalf("code=%d out=%s error=%s paths=%v", code, out, stderr, paths)
	}
}

func TestUnsafeInputsNeverSendRequests(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{}`)) }))
	defer server.Close()
	for _, args := range [][]string{{"api", "https://evil.test/"}, {"api", "/../auth/session"}, {"api", "/tasks", "--method", "POST"}, {"run", "start", "--workflow", "wf", "--version", "1"}} {
		if code, _, _ := invoke(t, server.URL, args...); code != 2 {
			t.Errorf("expected usage exit for %v got %d", args, code)
		}
	}
	if calls != 0 {
		t.Fatalf("unsafe inputs made %d requests", calls)
	}
}

func TestRunStartPinsVersionAndOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/runs" || body["workflow_id"] != "workflow-1" || body["workflow_version"] != float64(7) || body["task_id"] != "task-1" || body["expected_version"] != float64(4) || body["idempotency_key"] != "start-task-1-v4" || body["project_id"] != "project-one" {
			t.Errorf("wrong start: %s %s %v", r.Method, r.URL, body)
		}
		w.Write([]byte(`{"id":"run-created","workflow_version":7}`))
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL, "run", "start", "--workflow", "workflow-1", "--version", "7", "--task", "task-1", "--expected-version", "4", "--operation", "start-task-1-v4")
	if code != 0 || !strings.Contains(out, "run-created") {
		t.Fatalf("code=%d out=%s err=%s", code, out, stderr)
	}
}

func TestPublishPreservesExplicitOperation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "workflow.json")
	if err := os.WriteFile(file, []byte(`{"idempotency_key":"publish-workflow-v1","workflow":{"id":"wf","name":"Flow","nodes":[],"edges":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/workflows" || body["idempotency_key"] != "publish-workflow-v1" || body["project_id"] != "project-one" || body["workflow"].(map[string]any)["id"] != "wf" {
			t.Errorf("wrong publish: %s %s %v", r.Method, r.URL, body)
		}
		w.Write([]byte(`{"id":"wf","version":1}`))
	}))
	defer server.Close()
	if code, _, stderr := invoke(t, server.URL, "workflow", "publish", "--file", file); code != 0 {
		t.Fatal(stderr)
	}
}

func TestRunEventsPreservesCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/runs/run-1/events" || r.URL.Query().Get("after") != "42" || r.URL.Query().Get("project_id") != "project-one" {
			t.Errorf("wrong events query: %s %s", r.Method, r.URL)
		}
		w.Write([]byte(`{"items":[{"sequence":43,"type":"node_started"}],"next_cursor":43}`))
	}))
	defer server.Close()
	code, out, stderr := invoke(t, server.URL, "run", "events", "run-1", "--after", "42")
	if code != 0 || !strings.Contains(out, "node_started") {
		t.Fatalf("code=%d out=%s err=%s", code, out, stderr)
	}
}

func TestMutationTimeoutDoesNotRetry(t *testing.T) {
	file := filepath.Join(t.TempDir(), "command.json")
	if err := os.WriteFile(file, []byte(`{"action":"cancel","expected_version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	calls := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(150 * time.Millisecond):
		}
	}))
	defer server.Close()
	var out, stderr bytes.Buffer
	code := Execute(context.Background(), []string{"--control", server.URL, "--project", "p", "--timeout", "20ms", "run", "command", "run-1", "--file", file}, strings.NewReader(""), &out, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "outcome may be unknown") || len(calls) != 1 {
		t.Fatalf("code=%d calls=%d err=%s", code, len(calls), stderr.String())
	}
}

func TestRedirectDoesNotForwardToken(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`{}`)) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	code, _, stderr := invoke(t, server.URL, "projects")
	if code != 1 || calls != 0 || strings.Contains(stderr, "test-secret") {
		t.Fatalf("code=%d calls=%d err=%s", code, calls, stderr)
	}
}

func TestCancelledContextExits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	var out, stderr bytes.Buffer
	code := Execute(ctx, []string{"--control", "http://127.0.0.1:1", "--project", "p", "runs"}, strings.NewReader(""), &out, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "cancelled") {
		t.Fatalf("code=%d err=%s", code, stderr.String())
	}
}

func TestTokenRedactionPreservesValidJSON(t *testing.T) {
	var out bytes.Buffer
	if err := writeJSON(&out, map[string]any{"message": "true secret", "enabled": true, "nested": json.RawMessage(`{"value":"true"}`)}, "true"); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("redaction broke JSON: %s", out.String())
	}
	if result["message"] != "[redacted] secret" || result["enabled"] != true || result["nested"].(map[string]any)["value"] != "[redacted]" {
		t.Fatalf("wrong redaction: %v", result)
	}
}
