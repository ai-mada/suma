package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Revisions select protected domain facts, never caller-supplied task lineage,
// startup attestations or source bindings. Reads stay project-scoped and bounded.
func (s *Server) candidateRepairRevisionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	revision := len(parts) == 3 && parts[0] == "candidate-compositions" && (parts[2] == "repair-revision-context" || parts[2] == "repair-revisions")
	history := len(parts) == 3 && parts[0] == "tasks" && parts[2] == "run-history"
	if !revision && !history {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	for name, values := range query {
		if (name != "project_id" && !(history && (name == "limit" || name == "before"))) || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(compositionInvalid))
			return true
		}
	}
	if value := query.Get("project_id"); value != "" && value != project || r.Method == http.MethodGet && value == "" {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	switch {
	case history && r.Method == http.MethodGet:
		limit := 20
		if raw := query.Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 100 || strconv.Itoa(limit) != raw {
				writeError(w, invalid(compositionInvalid))
				return true
			}
		}
		out, err := s.Service.ListTaskRunProvenance(r.Context(), actor, project, parts[1], limit, query.Get("before"))
		respond(w, http.StatusOK, out, err)
	case revision && parts[2] == "repair-revision-context" && r.Method == http.MethodGet:
		out, err := s.Service.GetCandidateCompositionRepairRevisionContext(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case revision && parts[2] == "repair-revisions" && r.Method == http.MethodPost:
		allowed := map[string]bool{}
		for _, field := range []string{"project_id", "expected_version", "expected_task_version", "expected_conflict_sha256", "workflow_id", "workflow_version", "producer_node_id", "tool_profile_id", "expected_tool_version", "expected_recovery_sha256", "reason", "acknowledge", "idempotency_key"} {
			allowed[field] = true
		}
		for field := range body {
			if !allowed[field] {
				writeError(w, invalid(compositionInvalid))
				return true
			}
		}
		if body["project_id"] != project {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		for _, field := range []string{"expected_recovery_sha256"} {
			if value, exists := body[field]; exists {
				if _, typed := value.(string); !typed {
					writeError(w, invalid(compositionInvalid))
					return true
				}
			}
		}
		var req CandidateCompositionRepairRevisionRequest
		if err := projectKnowledgeRequired(body, "project_id", "expected_version", "expected_task_version", "expected_conflict_sha256", "workflow_id", "workflow_version", "producer_node_id", "tool_profile_id", "expected_tool_version", "reason", "acknowledge", "idempotency_key"); err != nil {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		if err := compositionBodyIntegers(body, "expected_version", "expected_task_version", "workflow_version", "expected_tool_version"); err != nil {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		if err := strictAutomation(body, &req); err != nil {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		out, err := s.Service.ReviseCandidateCompositionRepair(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusCreated, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
