package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
