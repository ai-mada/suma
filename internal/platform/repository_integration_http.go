package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// Repository integration is project-scoped governance, separate from IDE file
// access and release artifacts. Provider callbacks use their own authenticated
// transport before the ordinary browser/API router.
func (s *Server) repositoryIntegrationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "repository-integrations" && parts[0] != "repository-integration-targets" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)) }
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	listPlans := len(parts) == 2 && parts[0] == "repository-integrations" && parts[1] == "plans" && r.Method == http.MethodGet
	options := len(parts) == 2 && parts[0] == "repository-integrations" && parts[1] == "options" && r.Method == http.MethodGet
	for key, entries := range values {
		if len(entries) != 1 || key != "project_id" && !(key == "task_id" && (listPlans || options)) && !(listPlans && (key == "limit" || key == "before")) {
			bad()
			return true
		}
	}
	if value, exists := values["project_id"]; exists && (value[0] == "" || value[0] != project) || r.Method == http.MethodGet && values.Get("project_id") == "" {
		bad()
		return true
	}
	if parts[0] == "repository-integration-targets" {
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			items, err := s.Service.ListRepositoryIntegrationTargets(r.Context(), actor, project)
			respondList(w, items, err)
		case len(parts) == 1 && r.Method == http.MethodPost:
			var req RepositoryIntegrationTargetCreateRequest
			if err := repositoryDecodeBody(m, &req, "project_id", "name", "provider_id", "target_alias", "idempotency_key"); err != nil {
				writeError(w, err)
				return true
			}
			out, err := s.Service.CreateRepositoryIntegrationTarget(r.Context(), actor, project, req)
			respond(w, http.StatusCreated, out, err)
		case len(parts) == 2 && r.Method == http.MethodPatch:
			var req RepositoryIntegrationTargetUpdateRequest
			if err := repositoryDecodeBody(m, &req, "project_id", "expected_version", "name", "status", "idempotency_key"); err != nil {
				writeError(w, err)
				return true
			}
			out, err := s.Service.UpdateRepositoryIntegrationTarget(r.Context(), actor, project, parts[1], req)
			respond(w, http.StatusOK, out, err)
		default:
			writeError(w, missing())
		}
		return true
	}
	switch {
	case options:
		if values.Get("task_id") == "" {
			bad()
			return true
		}
		out, err := s.Service.RepositoryIntegrationOptions(r.Context(), actor, project, values.Get("task_id"))
		respond(w, http.StatusOK, out, err)
	case listPlans:
		limit := 20
		if raw, exists := values["limit"]; exists {
			limit, err = repositoryPageLimit(raw[0])
			if err != nil {
				bad()
				return true
			}
		}
		out, err := s.Service.ListRepositoryIntegrationPlans(r.Context(), s.Engine, actor, project, values.Get("task_id"), limit, values.Get("before"))
		respond(w, http.StatusOK, out, err)
	case len(parts) == 2 && parts[1] == "plans" && r.Method == http.MethodPost:
		var req RepositoryIntegrationPlanCreateRequest
		if err := repositoryDecodeBody(m, &req, "project_id", "task_id", "expected_task_version", "source_node_id", "target_id", "expected_target_version", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateRepositoryIntegrationPlan(r.Context(), s.Engine, actor, project, req)
		respond(w, http.StatusCreated, out, err)
	case len(parts) == 3 && parts[1] == "plans" && r.Method == http.MethodGet:
		out, err := s.Service.GetRepositoryIntegrationPlan(r.Context(), s.Engine, actor, project, parts[2])
		respond(w, http.StatusOK, out, err)
	case len(parts) == 4 && parts[1] == "plans" && parts[3] == "commands" && r.Method == http.MethodPost:
		var req RepositoryIntegrationPlanCommandRequest
		if err := repositoryDecodeBody(m, &req, "project_id", "expected_version", "action", "reason", "acknowledge", "idempotency_key"); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandRepositoryIntegrationPlan(r.Context(), s.Engine, actor, project, parts[2], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}

func repositoryDecodeBody(m map[string]any, out any, fields ...string) error {
	if err := projectKnowledgeRequired(m, fields...); err != nil {
		return repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)
	}
	return strictAutomation(m, out)
}

func repositoryPageLimit(value string) (int, error) {
	if value == "" || value[0] == '0' {
		return 0, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 100 {
		return 0, repositoryError(http.StatusBadRequest, "invalid", repositoryInvalid)
	}
	return n, nil
}
