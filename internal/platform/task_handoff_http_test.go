package platform

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTaskHandoffHTTPRejectsAmbiguousScopeAndVersions(t *testing.T) {
	for _, query := range []string{
		"project_id=p&task_id=t&project_id=other", "project_id=p&task_id=t&task_id=other",
		"project_id=p&task_id=t&limit=1e1", "project_id=p&task_id=t&limit=01",
		"project_id=p&task_id=t&limit=101", "project_id=p&task_id=t&limit=0",
		"project_id=p&task_id=t&extra=x", "project_id=p&task_id=",
	} {
		if _, _, err := taskHandoffQuery(httptest.NewRequest("GET", "/api/v1/task-handoffs?"+query, nil)); err == nil {
			t.Fatal("ambiguous or unbounded handoff query accepted", query)
		}
	}
	for _, version := range []string{"1.0", "1e0", "1.5", "true", "null"} {
		r := httptest.NewRequest("POST", "/api/v1/task-handoffs", strings.NewReader(`{"project_id":"p","task_id":"t","expected_task_version":`+version+`,"to_resource_id":"r","note":"handoff","idempotency_key":"handoff-test-001"}`))
		body, err := decode(r)
		if err != nil {
			continue
		}
		if taskHandoffRequired(body, "expected_task_version") != nil {
			continue
		}
		var request TaskHandoffCreateRequest
		if strictAutomation(body, &request) == nil {
			t.Fatal("handoff CAS lost the original integer lexeme", version)
		}
	}
	// A scoped Agent/Runner cannot turn an application read credential into a
	// responsibility mutation, even before the service is consulted.
	for _, scope := range []string{"agent", "runner"} {
		response := httptest.NewRecorder()
		if !(&Server{}).taskHandoffHTTP(response, httptest.NewRequest("POST", "/api/v1/task-handoffs", nil), Actor{}, scope, []string{"task-handoffs"}, "p", nil) || response.Code != 403 {
			t.Fatal("scoped execution credential entered handoff service", scope)
		}
	}
}
