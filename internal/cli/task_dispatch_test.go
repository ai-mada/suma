package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskDispatchCLIScopedJourney(t *testing.T) {
	var calls atomic.Int32
	expectedUser := strings.Repeat("a", 32)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := calls.Add(1)
		q := r.URL.Query()
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" ||
			r.Header.Get("X-Suma-Expected-User") != expectedUser ||
			r.Header.Get("Accept-Language") != "en-US" || q.Get("project_id") != "project-one" {
			t.Error("original credential, view, language or project missing")
		}
		var body map[string]any
		switch step {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tasks/task-one/dispatch-test-option" ||
				len(q) != 10 || q.Get("expected_task_version") != "2" || q.Get("workflow_id") != "flow-one" ||
				q.Get("workflow_version") != "3" || q.Get("agent_node_id") != "develop" ||
				q.Get("development_tool_profile_id") != "dev-tool" || q.Get("development_tool_profile_version") != "4" ||
				q.Get("test_node_id") != "verify" || q.Get("test_tool_profile_id") != "test-tool" || q.Get("test_tool_profile_version") != "5" {
				t.Error("wrong fixed preflight request", r.Method, r.URL)
			}
		case 2:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/task-dispatch-plans" ||
				json.NewDecoder(r.Body).Decode(&body) != nil || body["project_id"] != "project-one" ||
				body["idempotency_key"] != "fixed-preview-001" || body["expected_task_version"] != float64(2) ||
				body["expected_tool_version"] != float64(4) {
				t.Error("wrong fixed preview request", r.Method, r.URL, body)
			}
			got, _ := json.Marshal(body["test_bindings"])
			if string(got) != `[{"expected_tool_version":5,"node_id":"verify","tool_profile_id":"test-tool"}]` {
				t.Error("preview lost test bindings", string(got))
			}
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/task-dispatch-plans/plan-one" {
				t.Error("wrong plan read", r.Method, r.URL)
			}
		case 4, 5:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/task-dispatch-plans/plan-one/start" ||
				json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 3 ||
				body["expected_version"] != float64(1) || body["project_id"] != "project-one" ||
				body["idempotency_key"] != "fixed-start-001" {
				t.Error("wrong fixed start request or replay", r.Method, r.URL, body)
			}
		default:
			t.Error("unexpected request", r.Method, r.URL)
		}
		_, _ = fmt.Fprint(w, `{"id":"exact-server-result"}`)
	}))
	defer s.Close()
	base := []string{"--control", s.URL, "--project", "project-one", "--language", "en-US", "--token", "fixture-api-token", "--expected-user", expectedUser, "tasks"}
	start := []string{"dispatch-start", "plan-one", "--expected-version", "1", "--operation", "fixed-start-001"}
	for i, tc := range []struct {
		args  []string
		input string
	}{
		{[]string{"dispatch-test-option", "task-one", "--expected-version", "2", "--workflow", "flow-one", "--workflow-version", "3", "--node", "develop", "--tool", "dev-tool", "--expected-tool-version", "4", "--test-node", "verify", "--test-tool", "test-tool", "--expected-test-tool-version", "5"}, ""},
		{[]string{"dispatch-preview", "--file", "-"}, `{"task_id":"task-one","expected_task_version":2,"workflow_id":"flow-one","workflow_version":3,"agent_node_id":"develop","tool_profile_id":"dev-tool","expected_tool_version":4,"test_bindings":[{"node_id":"verify","tool_profile_id":"test-tool","expected_tool_version":5}],"idempotency_key":"fixed-preview-001"}`},
		{[]string{"dispatch-plan", "plan-one"}, ""},
		{start, ""},
		{start, ""},
	} {
		var stdout, stderr strings.Builder
		code := Execute(context.Background(), append(append([]string{}, base...), tc.args...), strings.NewReader(tc.input), &stdout, &stderr)
		if code != 0 || calls.Load() != int32(i+1) || !strings.Contains(stdout.String(), "exact-server-result") {
			t.Fatalf("%s did not perform one exact request: exit=%d calls=%d stderr=%s", tc.args[0], code, calls.Load(), stderr.String())
		}
	}
}

func TestTaskDispatchCLIRejectsAmbiguousInputsBeforeTransport(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		t.Error("invalid request reached transport", r.Method, r.URL)
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer s.Close()
	base := []string{"--control", s.URL, "--project", "project-one", "--language", "en-US", "tasks"}
	for _, tc := range []struct {
		args  []string
		input string
	}{
		{[]string{"dispatch-test-option", "task-one", "--expected-version", "01"}, ""},
		{[]string{"dispatch-plan", ".."}, ""},
		{[]string{"dispatch-start", "plan-one", "--expected-version", "1"}, ""},
		{[]string{"dispatch-start", "plan-one", "--expected-version", "01", "--operation", "fixed-start-001"}, ""},
		{[]string{"dispatch-preview", "--file", "-"}, `{"task_id":"task-one"}`},
		{[]string{"dispatch-preview", "--file", "-"}, `{"idempotency_key":"fixed-preview-001","test_bindings":[{"node_id":"a","node_id":"b"}]}`},
		{[]string{"dispatch-preview", "--file", "-"}, `{"project_id":"other","idempotency_key":"fixed-preview-001"}`},
	} {
		var stdout, stderr strings.Builder
		code := Execute(context.Background(), append(append([]string{}, base...), tc.args...), strings.NewReader(tc.input), &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 {
			t.Errorf("%v: exit=%d stdout=%s stderr=%s", tc.args, code, stdout.String(), stderr.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid CLI input made requests", calls.Load())
	}
}
