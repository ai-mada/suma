package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const repairRevisionCLIRequest = `{"expected_version":2,"expected_task_version":3,"expected_conflict_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","workflow_id":"repair-workflow","workflow_version":2,"producer_node_id":"code","tool_profile_id":"native-profile","expected_tool_version":4,"reason":"Choose the explicitly reviewed healthy tool; retain the previous run.","acknowledge":true,"idempotency_key":"original-repair-revision"}`

// Exercise the real CLI parser/transport rather than reproducing its validator.
func TestCandidateRepairRevisionCLIOriginalRequestAndTaskHistory(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("project_id") != "project-one" && r.Method == "GET" {
			t.Error("project scope missing", r.URL)
		}
		if r.Method == "GET" {
			if r.URL.Path != "/api/v1/candidate-compositions/parent-plan/repair-revision-context" && r.URL.Path != "/api/v1/tasks/repair-task/run-history" {
				t.Error("unexpected read", r.URL)
			}
			_, _ = w.Write([]byte(`{"items":[],"can_revise":true}`))
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/candidate-compositions/parent-plan/repair-revisions" {
			t.Error("unexpected mutation", r.Method, r.URL)
		}
		var body map[string]any
		d := json.NewDecoder(r.Body)
		d.UseNumber()
		if d.Decode(&body) != nil || body["project_id"] != "project-one" || body["expected_version"] != json.Number("2") || body["idempotency_key"] != "original-repair-revision" {
			t.Error("original identity or numeric token changed", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"new-plan","task_id":"repair-task","run_id":""}`))
	}))
	defer s.Close()
	for _, command := range [][]string{{"candidate-compositions", "revision-context", "parent-plan"}, {"tasks", "run-history", "repair-task", "--limit", "20"}} {
		code, _, err := contextInvoke(t, s.URL, "en-US", "", command...)
		if code != 0 || err != "" {
			t.Fatal(code, err)
		}
	}
	code, out, err := contextInvoke(t, s.URL, "zh-CN", repairRevisionCLIRequest, "candidate-compositions", "revise-repair", "parent-plan", "--file", "-", "--confirm")
	if code != 0 || err != "" || !strings.Contains(out, "new-plan") || calls != 3 {
		t.Fatal(code, out, err, calls)
	}
	for _, bad := range []string{
		strings.Replace(repairRevisionCLIRequest, `"expected_version":2`, `"expected_version":2e0`, 1),
		strings.Replace(repairRevisionCLIRequest, `"expected_tool_version":4`, `"expected_tool_version":"4"`, 1),
		strings.Replace(repairRevisionCLIRequest, `"reason":`, `"task_id":"forged", "reason":`, 1),
		strings.Replace(repairRevisionCLIRequest, `"reason":`, `"expected_recovery_sha256":null, "reason":`, 1),
		strings.Replace(repairRevisionCLIRequest, `"acknowledge":true`, `"acknowledge":false`, 1),
		strings.Replace(repairRevisionCLIRequest, `"expected_version":2`, `"expected_version":2,"expected_version":2`, 1),
	} {
		code, _, _ := contextInvoke(t, s.URL, "zh-CN", bad, "candidate-compositions", "revise-repair", "parent-plan", "--file", "-", "--confirm")
		if code != 2 || calls != 3 {
			t.Fatal("invalid revision reached server", code, calls)
		}
	}
	code, _, _ = contextInvoke(t, s.URL, "en-US", repairRevisionCLIRequest, "candidate-compositions", "revise-repair", "parent-plan", "--file", "-")
	if code != 2 || calls != 3 {
		t.Fatal("unconfirmed revision sent", code, calls)
	}
}
