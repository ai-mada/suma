package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func projectApplicationObject(value any, keys ...string) bool {
	m, ok := value.(map[string]any)
	if !ok || len(m) != len(keys) {
		return false
	}
	for _, key := range keys {
		v, exists := m[key]
		if !exists || v == nil {
			return false
		}
	}
	return true
}
func applicationWriteShape(body map[string]any) bool {
	if !projectApplicationObject(body, "project_id", "expected_version", "key", "name", "description", "kind", "parent_application_id", "owner_team_id", "state", "code_scopes", "reason", "idempotency_key") {
		return false
	}
	scopes, ok := body["code_scopes"].([]any)
	if !ok {
		return false
	}
	for _, scope := range scopes {
		if !projectApplicationObject(scope, "id", "target_id", "expected_target_version", "relative_root", "shared", "reason") {
			return false
		}
	}
	return true
}
func applicationVersionShape(body map[string]any, register bool) bool {
	keys := []string{"project_id", "expected_application_version", "label", "sources", "dependencies", "acknowledge_dependency_gaps", "reason"}
	if register {
		keys = append(keys, "expected_preview_sha256", "idempotency_key")
	}
	if !projectApplicationObject(body, keys...) {
		return false
	}
	sources, ok := body["sources"].([]any)
	if !ok {
		return false
	}
	for _, source := range sources {
		if !projectApplicationObject(source, "scope_id", "integration_plan_id", "expected_integration_version") {
			return false
		}
	}
	deps, ok := body["dependencies"].([]any)
	if !ok {
		return false
	}
	for _, d := range deps {
		if !projectApplicationObject(d, "target_application_id", "target_version_id", "target_version_sha256", "observation_id", "declared_range", "reason") {
			return false
		}
	}
	return true
}

// Source previews and impact queries are pure reads of fixed evidence. Other
// POSTs are explicit browser commands; no route dispatches or deploys a task.
func (s *Server) projectApplicationsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "applications" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) {
		writeError(w, forbidden())
		return true
	}
	preview := len(parts) == 4 && parts[2] == "versions" && parts[3] == "preview" && r.Method == http.MethodPost
	impact := len(parts) == 2 && parts[1] == "impact" && r.Method == http.MethodPost
	if r.Method != http.MethodGet && !preview && !impact && actor.kind != actorBrowser {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(applicationInvalid))
		return true
	}
	list := r.Method == http.MethodGet && (len(parts) == 1 || len(parts) == 3 && parts[2] == "versions")
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(list && (key == "limit" || key == "before" || len(parts) == 1 && key == "state")) {
			writeError(w, invalid(applicationInvalid))
			return true
		}
	}
	if (r.Method == http.MethodGet && query.Get("project_id") == "") || (query.Get("project_id") != "" && query.Get("project_id") != project) {
		writeError(w, invalid(applicationInvalid))
		return true
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		if raw[0] == '0' {
			writeError(w, invalid(applicationInvalid))
			return true
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				writeError(w, invalid(applicationInvalid))
				return true
			}
		}
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, invalid(applicationInvalid))
			return true
		}
	}
	respond := func(out any, err error, status int) {
		if err != nil {
			writeError(w, err)
		} else {
			writeJSON(w, status, out)
		}
	}
	decode := func(out any, shape bool) bool {
		if !shape {
			writeError(w, invalid(applicationInvalid))
			return false
		}
		if err := strictAutomation(body, out); err != nil {
			writeError(w, err)
			return false
		}
		return true
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.ListProjectApplications(r.Context(), actor, project, query.Get("state"), limit, query.Get("before"))
		respond(out, err, 200)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var q ApplicationWriteRequest
		if decode(&q, applicationWriteShape(body)) {
			out, err := s.Service.CreateProjectApplication(r.Context(), actor, project, q)
			respond(out, err, 201)
		}
	case len(parts) == 2 && parts[1] == "options" && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationOptions(r.Context(), actor, project)
		respond(out, err, 200)
	case len(parts) == 2 && parts[1] == "graph" && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationGraph(r.Context(), actor, project)
		respond(out, err, 200)
	case impact:
		var q ApplicationImpactRequest
		if decode(&q, projectApplicationObject(body, "project_id", "application_id", "expected_graph_sha256")) {
			out, err := s.Service.GetApplicationImpact(r.Context(), actor, project, q)
			respond(out, err, 200)
		}
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetProjectApplication(r.Context(), actor, project, parts[1])
		respond(out, err, 200)
	case len(parts) == 2 && r.Method == http.MethodPatch:
		var q ApplicationWriteRequest
		if decode(&q, applicationWriteShape(body)) {
			out, err := s.Service.SaveProjectApplication(r.Context(), actor, project, parts[1], q)
			respond(out, err, 200)
		}
	case len(parts) == 4 && parts[2] == "versions" && parts[3] == "options" && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationVersionOptions(r.Context(), actor, project, parts[1])
		respond(out, err, 200)
	case preview:
		var q ApplicationVersionPreviewRequest
		if decode(&q, applicationVersionShape(body, false)) {
			out, err := s.Service.PreviewApplicationVersion(r.Context(), actor, project, parts[1], q)
			respond(out, err, 200)
		}
	case len(parts) == 3 && parts[2] == "versions" && r.Method == http.MethodPost:
		var q ApplicationVersionRegisterRequest
		if decode(&q, applicationVersionShape(body, true)) {
			out, err := s.Service.RegisterApplicationVersion(r.Context(), actor, project, parts[1], q)
			respond(out, err, 201)
		}
	case len(parts) == 3 && parts[2] == "versions" && r.Method == http.MethodGet:
		out, err := s.Service.ListApplicationVersions(r.Context(), actor, project, parts[1], limit, query.Get("before"))
		respond(out, err, 200)
	case len(parts) == 4 && parts[2] == "versions" && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationVersion(r.Context(), actor, project, parts[1], parts[3])
		respond(out, err, 200)
	default:
		writeError(w, missing())
	}
	return true
}
