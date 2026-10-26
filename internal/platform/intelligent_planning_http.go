package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// The planning routes deliberately share project authority and strict command
// decoding with the rest of the control plane. Model output is never an HTTP
// command or a source of permissions.
func (s *Server) intelligentPlanningHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope string, parts []string, project string, m map[string]any) bool {
	if len(parts) < 2 || parts[0] != "planning" || parts[1] != "analyses" && parts[1] != "alerts" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	limit := 20
	if len(r.URL.RawQuery) > 8192 {
		err = invalid("Invalid intelligent planning query")
	}
	for key, items := range values {
		allowed := key == "project_id" || r.Method == http.MethodGet && len(parts) == 2 && (key == "limit" || key == "before" || key == "state" && parts[1] == "alerts")
		if !allowed || len(items) != 1 {
			err = invalid("Invalid intelligent planning query")
		}
	}
	if v, ok := values["project_id"]; ok && (v[0] == "" || v[0] != project) {
		err = invalid("Invalid intelligent planning query")
	}
	if r.Method == http.MethodGet && values.Get("project_id") == "" {
		err = invalid("Invalid intelligent planning query")
	}
	if raw, ok := values["limit"]; ok {
		if raw[0] == "" || raw[0][0] == '0' {
			err = invalid("Invalid intelligent planning query")
		} else {
			for _, c := range raw[0] {
				if c < '0' || c > '9' {
					err = invalid("Invalid intelligent planning query")
				}
			}
			var parseErr error
			limit, parseErr = strconv.Atoi(raw[0])
			if parseErr != nil || limit < 1 || limit > 100 {
				err = invalid("Invalid intelligent planning query")
			}
		}
	}
	if len(values.Get("before")) > 64 {
		err = invalid("Invalid intelligent planning query")
	}
	if err != nil {
		writeError(w, invalid("Invalid intelligent planning query"))
		return true
	}
	decode := func(out any, required ...string) error {
		for _, key := range required {
			if v, ok := m[key]; !ok || v == nil {
				return invalid("Incomplete intelligent planning request")
			}
		}
		return strictAutomation(m, out)
	}
	switch {
	case parts[1] == "analyses" && len(parts) == 2 && r.Method == http.MethodPost:
		var req PlanningAnalysisCreateRequest
		if err := decode(&req, "project_id", "goal", "locale", "task_ids", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.CreatePlanningAnalysis(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case parts[1] == "analyses" && len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.ListPlanningAnalyses(r.Context(), actor, project, limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case parts[1] == "analyses" && len(parts) == 3 && r.Method == http.MethodGet:
		out, err := s.Service.GetPlanningAnalysis(r.Context(), actor, project, parts[2])
		respond(w, http.StatusOK, out, err)
	case parts[1] == "analyses" && len(parts) == 4 && parts[3] == "commands" && r.Method == http.MethodPost:
		var req PlanningAnalysisCommandRequest
		if err := decode(&req, "project_id", "expected_version", "action", "reason", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.CommandPlanningAnalysis(r.Context(), actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	case parts[1] == "alerts" && len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.ListPlanningAlerts(r.Context(), actor, project, values.Get("state"), limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case parts[1] == "alerts" && len(parts) == 4 && parts[3] == "commands" && r.Method == http.MethodPost:
		var req PlanningAlertCommandRequest
		if err := decode(&req, "project_id", "expected_version", "action", "reason", "idempotency_key"); err != nil {
			writeError(w, err)
			break
		}
		out, err := s.Service.CommandPlanningAlert(r.Context(), actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
