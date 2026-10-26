package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Expert-team transports use the same current project and identity boundary as
// ordinary task dispatch. Draft/dispatch API writes do not grant publication.
func (s *Server) expertTeamHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "expert-teams" && parts[0] != "expert-team-plans" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, expertError("expert_team_invalid", expertInvalid)) }
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	listing := parts[0] == "expert-teams" && len(parts) == 1 && r.Method == http.MethodGet
	for key, values := range values {
		if len(values) != 1 || key != "project_id" && !(listing && (key == "limit" || key == "before")) {
			bad()
			return true
		}
	}
	if v, exists := values["project_id"]; exists && (v[0] == "" || v[0] != project) || r.Method == http.MethodGet && values.Get("project_id") == "" {
		bad()
		return true
	}
	if parts[0] == "expert-teams" {
		switch {
		case listing:
			limit := 20
			if text, exists := values["limit"]; exists {
				limit, err = expertTeamPositiveInteger(text[0], 100)
				if err != nil {
					bad()
					return true
				}
			}
			out, err := s.Service.ListExpertTeams(r.Context(), actor, project, limit, values.Get("before"))
			respond(w, http.StatusOK, out, err)
		case len(parts) == 2 && r.Method == http.MethodGet:
			out, err := s.Service.GetExpertTeam(r.Context(), actor, project, parts[1], 0)
			respond(w, http.StatusOK, out, err)
		case len(parts) == 4 && parts[2] == "revisions" && r.Method == http.MethodGet:
			revision, err := expertTeamPositiveInteger(parts[3], 2147483646)
			if err != nil {
				bad()
				return true
			}
			out, err := s.Service.GetExpertTeam(r.Context(), actor, project, parts[1], revision)
			respond(w, http.StatusOK, out, err)
		case len(parts) == 2 && r.Method == http.MethodPost:
			var req ExpertTeamSaveRequest
			if err := projectKnowledgeRequired(m, "project_id", "expected_version", "roles", "idempotency_key"); err != nil {
				bad()
				return true
			}
			if err := strictAutomation(m, &req); err != nil {
				writeError(w, err)
				return true
			}
			out, err := s.Service.SaveExpertTeam(r.Context(), s.Engine, actor, project, parts[1], req)
			respond(w, http.StatusOK, out, err)
		case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
			var req ExpertTeamCommandRequest
			if err := projectKnowledgeRequired(m, "project_id", "expected_version", "action", "reason", "idempotency_key"); err != nil {
				bad()
				return true
			}
			if err := strictAutomation(m, &req); err != nil {
				writeError(w, err)
				return true
			}
			out, err := s.Service.CommandExpertTeam(r.Context(), s.Engine, actor, project, parts[1], req)
			respond(w, http.StatusOK, out, err)
		default:
			writeError(w, missing())
		}
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req ExpertTeamPlanCreateRequest
		if err := projectKnowledgeRequired(m, "project_id", "team_id", "expected_configuration_version", "items", "idempotency_key"); err != nil {
			bad()
			return true
		}
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateExpertTeamPlan(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetExpertTeamPlan(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 3 && parts[2] == "start" && r.Method == http.MethodPost:
		var req ExpertTeamPlanStartRequest
		if err := projectKnowledgeRequired(m, "project_id", "expected_version", "acknowledge_dispatch", "idempotency_key"); err != nil {
			bad()
			return true
		}
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.StartExpertTeamPlan(r.Context(), s.Engine, actor, project, parts[1], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

// Query/path integers have one unambiguous spelling; JSON integer lexemes are
// separately enforced by the generated ingress schemas and typed requests.
func expertTeamPositiveInteger(value string, maximum int) (int, error) {
	if value == "" || value[0] == '0' {
		return 0, expertError("expert_team_invalid", expertInvalid)
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, expertError("expert_team_invalid", expertInvalid)
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > maximum {
		return 0, expertError("expert_team_invalid", expertInvalid)
	}
	return n, nil
}
