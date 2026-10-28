package platform

import (
	"net/http"
	"net/url"
)

// Issue lifecycle commands never use the generic record PATCH path. In
// particular, transport validation does not grant approval: the domain reloads
// current authority, the protected repair cycle and exact test evidence.
func (s *Server) issueRepairHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) < 3 || parts[0] != "issues" || parts[2] != "repair" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(issueRepairInvalid))
		return true
	}
	for key, values := range query {
		if key != "project_id" || len(values) != 1 || values[0] == "" || values[0] != project {
			writeError(w, invalid(issueRepairInvalid))
			return true
		}
	}
	if !handoffID(parts[1], false) || r.Method == http.MethodGet && query.Get("project_id") == "" {
		writeError(w, invalid(issueRepairInvalid))
		return true
	}
	decode := func(out any, required ...string) bool {
		// Every member in these current commands is explicit. Exact key count
		// also rejects encoding/json's case-insensitive aliases alongside the
		// required canonical keys; aliases never overwrite reviewed inputs.
		if len(body) != len(required) || projectKnowledgeRequired(body, required...) != nil || !testPlanDecodeBody(body, out) {
			writeError(w, invalid(issueRepairInvalid))
			return false
		}
		return true
	}
	if len(parts) == 3 && r.Method == http.MethodGet {
		out, err := s.Service.GetIssueRepair(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	if len(parts) != 4 || r.Method != http.MethodPost {
		writeError(w, missing())
		return true
	}
	switch parts[3] {
	case "prepare":
		var request IssueRepairPrepareRequest
		if !decode(&request, "project_id", "expected_issue_version", "title", "description", "owner_id", "acceptance_criteria", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.PrepareIssueRepair(r.Context(), actor, project, parts[1], request)
		respond(w, http.StatusCreated, out, err)
	case "regression":
		var request IssueRepairBindRequest
		if !decode(&request, "project_id", "expected_issue_version", "expected_repair_version", "expected_plan_version", "plan_id", "case_keys", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.BindIssueRegression(r.Context(), actor, project, parts[1], request)
		respond(w, http.StatusOK, out, err)
	case "close":
		var request IssueRepairCloseRequest
		if !decode(&request, "project_id", "expected_issue_version", "expected_repair_version", "run_id", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.CloseIssueRepair(r.Context(), actor, project, parts[1], request)
		respond(w, http.StatusOK, out, err)
	case "reopen":
		var request IssueRepairReopenRequest
		if !decode(&request, "project_id", "expected_issue_version", "expected_repair_version", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.ReopenIssueRepair(r.Context(), actor, project, parts[1], request)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
