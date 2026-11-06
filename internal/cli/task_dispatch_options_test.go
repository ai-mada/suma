package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTaskDispatchOptionsCLIScopedRead(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Path != "/api/v1/tasks/task-one/dispatch-options" || q.Get("project_id") != "project-one" || q.Get("expected_task_version") != "2" || q.Get("workflow_id") != "flow-one" || q.Get("workflow_version") != "3" || q.Get("agent_node_id") != "develop" || q.Get("limit") != "50" {
			t.Error("scoped read changed", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"items":[],"recommended_tool_profile_id":"","total":0,"next_cursor":""}`))
	}))
	defer s.Close()
	base := []string{"--control", s.URL, "--project", "project-one", "tasks", "dispatch-options", "task-one"}
	valid := []string{"--expected-version", "2", "--workflow", "flow-one", "--workflow-version", "3", "--node", "develop"}
	var out, stderr strings.Builder
	if code := Execute(context.Background(), append(append([]string{}, base...), valid...), strings.NewReader(""), &out, &stderr); code != 0 || calls != 1 {
		t.Fatal(code, stderr.String(), calls)
	}
	for _, bad := range [][]string{{}, {"--expected-version", "1e0", "--workflow", "flow-one", "--workflow-version", "3", "--node", "develop"}, append(append([]string{}, valid...), "--limit", "201"), append(append([]string{}, valid...), "--confirm")} {
		out.Reset()
		stderr.Reset()
		if code := Execute(context.Background(), append(append([]string{}, base...), bad...), strings.NewReader(""), &out, &stderr); code != 2 {
			t.Fatal(code, stderr.String())
		}
	}
	if calls != 1 {
		t.Fatal("invalid read reached transport", calls)
	}
}

func TestTaskDispatchOptionsCLIExactTool(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/tasks/task-one/dispatch-options" ||
			q.Get("project_id") != "project-one" || q.Get("expected_task_version") != "2" ||
			q.Get("workflow_id") != "flow-one" || q.Get("workflow_version") != "3" ||
			q.Get("agent_node_id") != "develop" || q.Has("limit") || q.Has("before") || len(q["tool_profile_id"]) != 1 {
			t.Error("exact tool read changed", r.Method, r.URL)
		}
		switch q.Get("tool_profile_id") {
		case "Profile-One":
			_, _ = w.Write([]byte(`{"items":[{"tool":{"id":"Profile-One","version":4},"eligible":false,"reason_code":"route_incompatible"}],"recommended_tool_profile_id":"","total":1,"next_cursor":""}`))
		case "missing-tool":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"Tool profile not found"}}`))
		default:
			t.Error("unexpected tool", q.Get("tool_profile_id"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer s.Close()

	base := []string{"--control", s.URL, "--project", "project-one", "tasks", "dispatch-options", "task-one", "--expected-version", "2", "--workflow", "flow-one", "--workflow-version", "3", "--node", "develop"}
	execute := func(args ...string) (int, string, string) {
		var out, stderr strings.Builder
		code := Execute(context.Background(), append(append([]string{}, base...), args...), strings.NewReader(""), &out, &stderr)
		return code, out.String(), stderr.String()
	}
	t.Run("incompatible is an observation", func(t *testing.T) {
		code, out, stderr := execute("--tool", "Profile-One")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		var response struct {
			Items []struct {
				Tool struct {
					ID      string `json:"id"`
					Version int    `json:"version"`
				} `json:"tool"`
				Eligible   *bool  `json:"eligible"`
				ReasonCode string `json:"reason_code"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &response); err != nil || len(response.Items) != 1 {
			t.Fatalf("invalid exact response: %s (%v)", out, err)
		}
		item := response.Items[0]
		if item.Tool.ID != "Profile-One" || item.Tool.Version != 4 || item.Eligible == nil || *item.Eligible || item.ReasonCode != "route_incompatible" {
			t.Fatalf("observation changed: %s", out)
		}
	})
	t.Run("missing tool is an error", func(t *testing.T) {
		code, out, stderr := execute("--tool", "missing-tool")
		if code != 1 || out != "" || !strings.Contains(stderr, "not_found") {
			t.Fatalf("exit %d, output %q, error %q", code, out, stderr)
		}
	})
	for _, bad := range [][]string{
		{"--tool="},
		{"--tool", "../tool"},
		{"--tool", "profile one"},
		{"--tool", strings.Repeat("x", 65)},
		{"--tool", "Profile-One", "--limit", "50"},
		{"--tool", "Profile-One", "--before", "original-cursor"},
		{"--tool", "Profile-One", "--before="},
	} {
		code, _, stderr := execute(bad...)
		if code != 2 || !strings.Contains(stderr, "--tool ID") {
			t.Fatalf("invalid flags %v: exit %d, error %q", bad, code, stderr)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("expected only the two exact reads, got %d requests", calls.Load())
	}
}
