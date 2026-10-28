package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) testPlansHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "test-plans" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(testPlanInvalid))
		return true
	}
	paged := r.Method == http.MethodGet && (len(parts) == 1 || len(parts) == 3 && parts[2] == "executions")
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(paged && (key == "limit" || key == "before")) && !(r.Method == http.MethodGet && len(parts) == 1 && key == "task_id") {
			writeError(w, invalid(testPlanInvalid))
			return true
		}
	}
	if values, ok := query["project_id"]; ok && values[0] != project || r.Method == http.MethodGet && query.Get("project_id") == "" {
		writeError(w, invalid(testPlanInvalid))
		return true
	}
	limit := 20
	if paged && query.Get("limit") != "" {
		raw := query.Get("limit")
		if raw[0] == '0' {
			writeError(w, invalid(testPlanInvalid))
			return true
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				writeError(w, invalid(testPlanInvalid))
				return true
			}
		}
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, invalid(testPlanInvalid))
			return true
		}
	}
	decode := func(out any, required ...string) bool {
		if projectKnowledgeRequired(body, required...) != nil {
			writeError(w, invalid(testPlanInvalid))
			return false
		}
		if !testPlanDecodeBody(body, out) {
			writeError(w, invalid(testPlanInvalid))
			return false
		}
		if _, hasCases := body["cases"]; hasCases && !testPlanHTTPCaseFields(body) {
			writeError(w, invalid(testPlanInvalid))
			return false
		}
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.ListTestPlans(r.Context(), actor, project, query.Get("task_id"), limit, query.Get("before"))
		respond(w, 200, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var request TestPlanCreateRequest
		if !decode(&request, "project_id", "task_id", "expected_task_version", "expected_requirement_version", "title", "description", "cases", "idempotency_key") {
			return true
		}
		out, err := s.Service.CreateTestPlan(r.Context(), actor, project, request)
		respond(w, 201, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetTestPlan(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodPut:
		var request TestPlanSaveRequest
		if !decode(&request, "project_id", "expected_version", "expected_task_version", "expected_requirement_version", "title", "description", "cases", "idempotency_key") {
			return true
		}
		out, err := s.Service.SaveTestPlan(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var request TestPlanCommandRequest
		if !decode(&request, "project_id", "expected_version", "action", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.CommandTestPlan(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "executions" && r.Method == http.MethodGet:
		out, err := s.Service.ListTestPlanExecutions(r.Context(), actor, project, parts[1], limit, query.Get("before"))
		respond(w, 200, out, err)
	case len(parts) == 4 && parts[2] == "executions" && r.Method == http.MethodGet:
		out, err := s.Service.GetTestPlanExecution(r.Context(), actor, project, parts[1], parts[3])
		respond(w, 200, out, err)
	case len(parts) == 5 && parts[2] == "executions" && parts[4] == "defects" && r.Method == http.MethodPost:
		var request TestPlanDefectRequest
		if !decode(&request, "project_id", "expected_version", "case_key", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.CreateTestPlanDefect(r.Context(), actor, project, parts[1], parts[3], request)
		respond(w, 201, out, err)
	case len(parts) == 4 && parts[2] == "reports" && r.Method == http.MethodGet:
		out, err := s.Service.GetTestPlanReport(r.Context(), actor, project, parts[1], parts[3])
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

// Required nested members match the generated closed DTO. Empty draft values
// are permitted; missing or null arrays never become silently invented data.
func testPlanHTTPCaseFields(body map[string]any) bool {
	cases, ok := body["cases"].([]any)
	if !ok {
		return false
	}
	for _, raw := range cases {
		c, ok := raw.(map[string]any)
		if !ok || projectKnowledgeRequired(c, "key", "title", "description", "preconditions", "steps", "expected_result", "priority", "covers", "selector") != nil {
			return false
		}
		selector, ok := c["selector"].(map[string]any)
		if !ok || projectKnowledgeRequired(selector, "class_name", "name") != nil {
			return false
		}
	}
	return true
}

func testPlanDecodeBody(body map[string]any, out any) bool {
	var hasNull func(any, int) bool
	hasNull = func(value any, depth int) bool {
		if depth > 40 || value == nil {
			return true
		}
		switch v := value.(type) {
		case map[string]any:
			for _, item := range v {
				if hasNull(item, depth+1) {
					return true
				}
			}
		case []any:
			for _, item := range v {
				if hasNull(item, depth+1) {
					return true
				}
			}
		}
		return false
	}
	if hasNull(body, 0) {
		return false
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 256*1024 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out) == nil
}
