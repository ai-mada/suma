package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const conflictRepairCLIRequest = `{"expected_composition_version":2,"expected_conflict_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repair":{"title":"保留两路业务意图","description":"Resolve only the fixed conflict and preserve both reviewed features.","acceptance_criteria":["Both source contracts pass independent tests"]},"workflow_id":"workflow-one","workflow_version":1,"producer_node_id":"code","tool_profile_id":"native-profile","expected_tool_version":2,"idempotency_key":"fixed-original-repair-create"}`

func TestCandidateConflictRepairCLIExactRoutesAndOriginalOperation(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == "GET" {
			if r.URL.Path != "/api/v1/candidate-compositions/original/conflict" || r.URL.Query().Get("project_id") != "project-one" {
				t.Error(r.URL)
			}
			_, _ = w.Write([]byte(`{"can_create":true}`))
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/candidate-compositions/original/repairs" {
			t.Error(r.Method, r.URL)
		}
		d := json.NewDecoder(r.Body)
		d.UseNumber()
		var body map[string]any
		if d.Decode(&body) != nil || body["project_id"] != "project-one" || body["idempotency_key"] != "fixed-original-repair-create" || body["expected_composition_version"] != json.Number("2") {
			t.Error(body)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"new-repair","task_id":"new-task","run_id":""}`))
	}))
	defer s.Close()
	code, _, err := contextInvoke(t, s.URL, "en-US", "", "candidate-compositions", "conflict", "original")
	if code != 0 || err != "" || calls != 1 {
		t.Fatal(code, err, calls)
	}
	code, out, err := contextInvoke(t, s.URL, "zh-CN", conflictRepairCLIRequest, "candidate-compositions", "repair", "original", "--file", "-", "--confirm")
	if code != 0 || err != "" || calls != 2 || !strings.Contains(out, "new-repair") {
		t.Fatal(code, out, err, calls)
	}
	for _, bad := range []string{
		strings.Replace(conflictRepairCLIRequest, `"expected_tool_version":2`, `"expected_tool_version":"2"`, 1),
		strings.Replace(conflictRepairCLIRequest, `"workflow_version":1`, `"workflow_version":1e0`, 1),
		strings.Replace(conflictRepairCLIRequest, `"title":"保留两路业务意图"`, `"title":null`, 1),
		strings.Replace(conflictRepairCLIRequest, `"repair":{`, `"repair":{"forged_input":true,`, 1),
		strings.Replace(conflictRepairCLIRequest, `"Both source contracts pass independent tests"`, `"`+strings.Repeat("x", 1001)+`"`, 1),
		strings.Replace(conflictRepairCLIRequest, `"Both source contracts pass independent tests"`, strings.TrimSuffix(strings.Repeat(`"criterion",`, 17), ","), 1),
	} {
		code, _, err := contextInvoke(t, s.URL, "zh-CN", bad, "candidate-compositions", "repair", "original", "--file", "-", "--confirm")
		if code != 2 || !strings.Contains(err, "repair_basis_required") {
			t.Fatal(code, err)
		}
	}
	if calls != 2 {
		t.Fatal("invalid requests were sent", calls)
	}
	code, _, _ = contextInvoke(t, s.URL, "en-US", conflictRepairCLIRequest, "candidate-compositions", "repair", "original", "--file", "-")
	if code != 2 || calls != 2 {
		t.Fatal("unconfirmed task creation", code, calls)
	}
}
