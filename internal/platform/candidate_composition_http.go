package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// Locations select current scoped records; client payloads never supply source
// acceptance, candidate bytes, protected baseline data or execution authority.
func (s *Server) candidateCompositionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	composition := len(parts) > 0 && parts[0] == "candidate-compositions"
	baseline := len(parts) == 3 && parts[0] == "tasks" && parts[2] == "code-baseline"
	if !composition && !baseline {
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
	list := composition && len(parts) == 1 && r.Method == http.MethodGet
	options := composition && len(parts) == 2 && parts[1] == "options" && r.Method == http.MethodGet
	for name, values := range query {
		allowed := name == "project_id" || (list || options) && name == "task_id" || list && (name == "limit" || name == "before")
		if !allowed || len(values) != 1 || values[0] == "" {
			writeError(w, invalid(compositionInvalid))
			return true
		}
	}
	if value := query.Get("project_id"); value != "" && value != project || r.Method == http.MethodGet && value == "" {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	if r.Method != http.MethodGet && body["project_id"] != project {
		writeError(w, invalid(compositionInvalid))
		return true
	}
	reject := func(err error) bool {
		if err == nil {
			return false
		}
		writeError(w, invalid(compositionInvalid))
		return true
	}
	switch {
	case baseline && r.Method == http.MethodGet:
		out, err := s.Service.GetTaskCodeBaseline(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case baseline && r.Method == http.MethodPost:
		var req TaskCodeBaselineBindRequest
		if reject(projectKnowledgeRequired(body, "project_id", "expected_task_version", "expected_binding_version", "reason", "acknowledge", "idempotency_key")) || reject(compositionBodyIntegers(body, "expected_task_version", "expected_binding_version")) {
			return true
		}
		// Count presence, not zero-values: partial, null or mixed origins cannot
		// be disguised as another source before the strict DTO decoder runs.
		origins := [][]string{{"composition_id", "expected_composition_version"}, {"terminal_candidate_plan_id", "expected_terminal_candidate_plan_version"}, {"source_task_id", "expected_source_task_version", "source_node_id"}}
		selected := -1
		for index, fields := range origins {
			for _, field := range fields {
				if _, present := body[field]; present {
					if selected >= 0 && selected != index {
						writeError(w, invalid(compositionInvalid))
						return true
					}
					selected = index
				}
			}
		}
		if selected < 0 {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		if reject(projectKnowledgeRequired(body, origins[selected]...)) || reject(compositionBodyIntegers(body, origins[selected][1])) {
			return true
		}

		if reject(strictAutomation(body, &req)) {
			return true
		}
		out, err := s.Service.BindTaskCodeBaseline(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusOK, out, err)
	case options:
		if query.Get("task_id") == "" {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		out, err := s.Service.CandidateCompositionOptions(r.Context(), s.Engine, actor, project, query.Get("task_id"))
		respond(w, http.StatusOK, out, err)
	case list:
		limit := 20
		if value := query.Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil || strconv.Itoa(limit) != value || limit < 1 || limit > 100 {
				writeError(w, invalid(compositionInvalid))
				return true
			}
		}
		if len(query.Get("before")) > 4096 {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		out, err := s.Service.ListCandidateCompositions(r.Context(), s.Engine, actor, project, query.Get("task_id"), limit, query.Get("before"))
		respond(w, http.StatusOK, out, err)
	case composition && len(parts) == 1 && r.Method == http.MethodPost:
		var req CandidateCompositionCreateRequest
		if reject(projectKnowledgeRequired(body, "project_id", "task_id", "expected_task_version", "workflow_id", "workflow_version", "producer_node_id", "sources", "idempotency_key")) || reject(compositionBodyIntegers(body, "expected_task_version", "workflow_version")) || reject(strictAutomation(body, &req)) {
			return true
		}
		entries, _ := body["sources"].([]any)
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok || reject(projectKnowledgeRequired(entry, "task_id", "expected_task_version", "source_node_id")) || reject(compositionBodyIntegers(entry, "expected_task_version")) {
				if !ok {
					writeError(w, invalid(compositionInvalid))
				}
				return true
			}
		}
		out, err := s.Service.CreateCandidateComposition(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case composition && len(parts) == 3 && parts[2] == "conflict" && r.Method == http.MethodGet:
		// A current conflict projection is read from durable attempt evidence. A
		// client-supplied path or prose error can never become repair authority.
		out, err := s.Service.GetCandidateCompositionConflict(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case composition && len(parts) == 3 && parts[2] == "repairs" && r.Method == http.MethodPost:
		var req CandidateCompositionRepairCreateRequest
		if reject(validateModelSelectionFields(body)) {
			return true
		}
		if reject(projectKnowledgeRequired(body, "project_id", "expected_composition_version", "expected_conflict_sha256", "repair", "workflow_id", "workflow_version", "producer_node_id", "tool_profile_id", "expected_tool_version", "idempotency_key")) || reject(compositionBodyIntegers(body, "expected_composition_version", "workflow_version", "expected_tool_version")) || reject(strictAutomation(body, &req)) {
			return true
		}
		repair, ok := body["repair"].(map[string]any)
		if !ok {
			writeError(w, invalid(compositionInvalid))
			return true
		}
		// Optional means omitted, not an untyped/null value. Keep the public
		// string contract intact before Go's decoder can turn null into "".
		for _, field := range []string{"priority", "owner_id"} {
			if value, exists := repair[field]; exists {
				if _, typed := value.(string); !typed {
					writeError(w, invalid(compositionInvalid))
					return true
				}
			}
		}
		if reject(projectKnowledgeRequired(repair, "title", "description", "acceptance_criteria")) {
			return true
		}
		out, err := s.Service.CreateCandidateCompositionRepair(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusCreated, out, err)
	case composition && len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetCandidateComposition(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case composition && len(parts) == 3 && parts[2] == "start" && r.Method == http.MethodPost:
		var req CandidateCompositionStartRequest
		if reject(projectKnowledgeRequired(body, "project_id", "expected_version", "acknowledge", "idempotency_key")) || reject(compositionBodyIntegers(body, "expected_version")) || reject(strictAutomation(body, &req)) {
			return true
		}
		out, err := s.Service.StartCandidateComposition(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusOK, out, err)
	case composition && len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var req CandidateCompositionCommandRequest
		if reject(projectKnowledgeRequired(body, "project_id", "expected_version", "action", "reason", "idempotency_key")) || reject(compositionBodyIntegers(body, "expected_version")) || reject(strictAutomation(body, &req)) {
			return true
		}
		out, err := s.Service.CommandCandidateComposition(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

// Numbers remain exact from the first ingress decode; exponent/fraction forms
// and null never pass through float conversion into authority or CAS versions.
func compositionBodyIntegers(body map[string]any, fields ...string) error {
	for _, name := range fields {
		number, ok := body[name].(json.Number)
		if !ok {
			return invalid(compositionInvalid)
		}
		raw := number.String()
		value, err := strconv.ParseInt(raw, 10, 32)
		minimum := int64(1)
		if name == "expected_binding_version" {
			minimum = 0
		}
		if err != nil || value < minimum || value > 2147483646 || strconv.FormatInt(value, 10) != raw {
			return invalid(compositionInvalid)
		}
	}
	return nil
}
