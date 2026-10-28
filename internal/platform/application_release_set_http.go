package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func applicationReleaseSetShape(body map[string]any, register bool) bool {
	keys := []string{"project_id", "label", "reason", "members", "acknowledge_unverified_compatibility"}
	if register {
		keys = append(keys, "expected_preview_sha256", "idempotency_key")
	}
	if !projectApplicationObject(body, keys...) {
		return false
	}
	members, ok := body["members"].([]any)
	if !ok {
		return false
	}
	for _, m := range members {
		if !projectApplicationObject(m, "application_id", "application_version_id", "application_version_sha256", "candidate_id") {
			return false
		}
	}
	return true
}
func applicationReleaseSetDeploymentShape(body map[string]any, prepare bool) bool {
	keys := []string{"project_id", "expected_set_sha256", "environments", "reason"}
	if prepare {
		keys = append(keys, "expected_preview_sha256", "idempotency_key")
	}
	if !projectApplicationObject(body, keys...) {
		return false
	}
	items, ok := body["environments"].([]any)
	if !ok {
		return false
	}
	for _, e := range items {
		if !projectApplicationObject(e, "application_id", "environment_id", "expected_environment_generation") {
			return false
		}
	}
	return true
}
func (s *Server) applicationReleaseSetHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "application-release-sets" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) {
		writeError(w, forbidden())
		return true
	}
	preview := r.Method == http.MethodPost && (len(parts) == 2 && parts[1] == "preview" || len(parts) == 3 && parts[2] == "deployment-preview")
	if r.Method != http.MethodGet && !preview && actor.kind != actorBrowser {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(applicationReleaseSetInvalid))
		return true
	}
	list := r.Method == http.MethodGet && len(parts) == 1
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(list && (key == "limit" || key == "before")) {
			writeError(w, invalid(applicationReleaseSetInvalid))
			return true
		}
	}
	if (r.Method == http.MethodGet && query.Get("project_id") == "") || (query.Get("project_id") != "" && query.Get("project_id") != project) {
		writeError(w, invalid(applicationReleaseSetInvalid))
		return true
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		if raw[0] == '0' {
			writeError(w, invalid(applicationReleaseSetInvalid))
			return true
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				writeError(w, invalid(applicationReleaseSetInvalid))
				return true
			}
		}
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, invalid(applicationReleaseSetInvalid))
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
			writeError(w, invalid(applicationReleaseSetInvalid))
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
		out, err := s.Service.ListApplicationReleaseSets(r.Context(), actor, project, limit, query.Get("before"))
		respond(out, err, 200)
	case len(parts) == 2 && parts[1] == "options" && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationReleaseSetOptions(r.Context(), actor, project)
		respond(out, err, 200)
	case len(parts) == 2 && parts[1] == "preview" && r.Method == http.MethodPost:
		var q ApplicationReleaseSetPreviewRequest
		if decode(&q, applicationReleaseSetShape(body, false)) {
			out, err := s.Service.PreviewApplicationReleaseSet(r.Context(), actor, project, q)
			respond(out, err, 200)
		}
	case len(parts) == 1 && r.Method == http.MethodPost:
		var q ApplicationReleaseSetRegisterRequest
		if decode(&q, applicationReleaseSetShape(body, true)) {
			out, err := s.Service.RegisterApplicationReleaseSet(r.Context(), actor, project, q)
			respond(out, err, 201)
		}
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetApplicationReleaseSet(r.Context(), actor, project, parts[1])
		respond(out, err, 200)
	case len(parts) == 3 && parts[2] == "deployment-preview" && r.Method == http.MethodPost:
		var q ApplicationReleaseSetDeploymentPreviewRequest
		if decode(&q, applicationReleaseSetDeploymentShape(body, false)) {
			out, err := s.Service.PreviewApplicationReleaseSetDeployment(r.Context(), actor, project, parts[1], q)
			respond(out, err, 200)
		}
	case len(parts) == 3 && parts[2] == "deployment-plans" && r.Method == http.MethodPost:
		var q ApplicationReleaseSetDeploymentPrepareRequest
		if decode(&q, applicationReleaseSetDeploymentShape(body, true)) {
			out, err := s.Service.PrepareApplicationReleaseSetDeployment(r.Context(), actor, project, parts[1], q)
			respond(out, err, 201)
		}
	default:
		writeError(w, missing())
	}
	return true
}
