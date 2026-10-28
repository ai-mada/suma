package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// This adapter closes transport shapes before Go's case-insensitive struct
// decoding. Source authority, candidate feasibility and adoption remain domain
// decisions; neither an options request nor adoption starts the target run.
func (s *Server) dispatchRecommendationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "dispatch-recommendations" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) || r.Method != http.MethodGet && actor.kind != actorBrowser {
		writeError(w, forbidden())
		return true
	}
	bad := func() bool { writeError(w, invalid(dispatchRecommendationInvalid)); return true }
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		return bad()
	}
	list := len(parts) == 1 && r.Method == http.MethodGet
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(list && (key == "task_id" || key == "limit" || key == "before")) {
			return bad()
		}
	}
	if values, ok := query["project_id"]; ok && values[0] != project || r.Method == http.MethodGet && query.Get("project_id") == "" || len(query.Get("before")) > 64 {
		return bad()
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		if raw[0] == '0' {
			return bad()
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				return bad()
			}
		}
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return bad()
		}
	}
	// Required names are canonical and non-null. Optional fields may be absent,
	// but never null; only the choice's three explicit nullable scalars differ.
	closed := func(value any, required, optional []string) bool {
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		allowed := map[string]bool{}
		for _, key := range required {
			allowed[key] = true
			if object[key] == nil {
				return false
			}
		}
		for _, key := range optional {
			allowed[key] = true
		}
		for key, value := range object {
			if !allowed[key] || value == nil {
				return false
			}
		}
		return true
	}
	budget := func(value any) bool {
		return closed(value, nil, []string{"token_limit", "cost_limit_micros", "currency"})
	}
	optionsFields := []string{"project_id", "task_id", "expected_task_version", "workflow_id", "workflow_version", "agent_node_id", "knowledge", "constraints"}
	checkOptions := func() bool {
		if !closed(body["constraints"], []string{"require_hard_budget", "max_steps", "max_output_tokens"}, []string{"budget"}) {
			return false
		}
		constraints := body["constraints"].(map[string]any)
		if value, ok := constraints["budget"]; ok && !budget(value) {
			return false
		}
		knowledge, ok := body["knowledge"].([]any)
		if !ok {
			return false
		}
		for _, reference := range knowledge {
			if !closed(reference, []string{"id", "revision", "sha256"}, nil) {
				return false
			}
		}
		return true
	}
	switch {
	case len(parts) == 2 && parts[1] == "options" && r.Method == http.MethodPost:
		var req DispatchRecommendationOptionsRequest
		if !closed(body, optionsFields, nil) || !checkOptions() || strictAutomation(body, &req) != nil {
			return bad()
		}
		out, err := s.Service.GetDispatchRecommendationOptions(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusOK, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req DispatchRecommendationCreateRequest
		fields := append(append([]string{}, optionsFields...), "source_sha256", "goal", "locale", "analysis_tool_profile_id", "expected_analysis_tool_version", "analysis_model_execution_selection", "acknowledge_model_use", "idempotency_key")
		if !closed(body, fields, nil) || !checkOptions() || !closed(body["analysis_model_execution_selection"], []string{"deployment_id", "deployment_revision", "deployment_sha256", "parameters"}, []string{"budget"}) {
			return bad()
		}
		selection := body["analysis_model_execution_selection"].(map[string]any)
		if !closed(selection["parameters"], []string{"max_steps", "max_output_tokens"}, nil) {
			return bad()
		}
		if value, ok := selection["budget"]; ok && !budget(value) {
			return bad()
		}
		if strictAutomation(body, &req) != nil {
			return bad()
		}
		out, err := s.Service.CreateDispatchRecommendation(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.ListDispatchRecommendations(r.Context(), s.Engine, actor, project, query.Get("task_id"), limit, query.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetDispatchRecommendation(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 3 && parts[2] == "adopt" && r.Method == http.MethodPost:
		var req DispatchRecommendationAdoptRequest
		if !closed(body, []string{"project_id", "expected_version", "source_sha256", "choice", "reason", "idempotency_key"}, nil) || strictAutomation(body, &req) != nil {
			return bad()
		}
		// DispatchRecommendationChoice.UnmarshalJSON also requires every exact
		// key, including null scalars, and rejects aliases, duplicates and bounds.
		out, err := s.Service.AdoptDispatchRecommendation(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusCreated, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
