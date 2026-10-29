package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) applicationReleaseRolloutHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) < 3 || parts[0] != "application-release-sets" || parts[2] != "rollouts" {
		return false
	}
	if scope != "" && scope != "api" || !testPlanActor(actor) {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet && actor.kind != actorBrowser {
		writeError(w, forbidden())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(applicationRolloutInvalid))
		return true
	}
	list := r.Method == http.MethodGet && len(parts) == 3
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(list && (key == "plan_group_id" || key == "limit" || key == "before")) {
			writeError(w, invalid(applicationRolloutInvalid))
			return true
		}
	}
	if r.Method == http.MethodGet && query.Get("project_id") == "" || query.Get("project_id") != "" && query.Get("project_id") != project {
		writeError(w, invalid(applicationRolloutInvalid))
		return true
	}
	if !handoffID(parts[1], false) || len(parts) >= 4 && parts[3] != "preview" && !handoffID(parts[3], false) {
		writeError(w, invalid(applicationRolloutInvalid))
		return true
	}
	respond := func(out any, err error, status int) {
		if err != nil {
			writeError(w, err)
		} else {
			writeJSON(w, status, out)
		}
	}
	decode := func(out any, keys ...string) bool {
		if !projectApplicationObject(body, keys...) {
			writeError(w, invalid(applicationRolloutInvalid))
			return false
		}
		if e := strictAutomation(body, out); e != nil {
			writeError(w, e)
			return false
		}
		return true
	}
	startKeys := []string{"project_id", "plan_group_id", "expected_set_sha256", "expected_plan_group_sha256", "reason"}
	switch {
	case len(parts) == 3 && r.Method == http.MethodGet:
		limit := 20
		if raw := query.Get("limit"); raw != "" {
			if raw[0] == '0' {
				writeError(w, invalid(applicationRolloutInvalid))
				return true
			}
			for _, c := range raw {
				if c < '0' || c > '9' {
					writeError(w, invalid(applicationRolloutInvalid))
					return true
				}
			}
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 100 {
				writeError(w, invalid(applicationRolloutInvalid))
				return true
			}
		}
		out, e := s.Service.ListApplicationReleaseRollouts(r.Context(), actor, project, parts[1], query.Get("plan_group_id"), limit, query.Get("before"))
		respond(out, e, 200)
	case len(parts) == 4 && parts[3] == "preview" && r.Method == http.MethodPost:
		var q ApplicationReleaseRolloutPreviewRequest
		if decode(&q, startKeys...) {
			out, e := s.Service.PreviewApplicationReleaseRollout(r.Context(), actor, project, parts[1], q)
			respond(out, e, 200)
		}
	case len(parts) == 3 && r.Method == http.MethodPost:
		var q ApplicationReleaseRolloutStartRequest
		keys := append(append([]string{}, startKeys...), "expected_preview_sha256", "idempotency_key")
		if decode(&q, keys...) {
			out, e := s.Service.StartApplicationReleaseRollout(r.Context(), actor, project, parts[1], q, s.Engine)
			respond(out, e, 201)
		}
	case len(parts) == 4 && r.Method == http.MethodGet:
		out, e := s.Service.GetApplicationReleaseRollout(r.Context(), actor, project, parts[1], parts[3])
		respond(out, e, 200)
	case len(parts) == 5 && parts[4] == "resume-preview" && r.Method == http.MethodPost:
		var q ApplicationReleaseRolloutResumePreviewRequest
		if decode(&q, "project_id", "expected_run_version", "expected_rollout_sha256") {
			out, e := s.Service.PreviewApplicationReleaseRolloutResume(r.Context(), actor, project, parts[1], parts[3], q)
			respond(out, e, 200)
		}
	case len(parts) == 5 && parts[4] == "commands" && r.Method == http.MethodPost:
		var q ApplicationReleaseRolloutCommandRequest
		if decode(&q, "project_id", "expected_run_version", "expected_rollout_sha256", "action", "expected_preview_sha256", "reason", "idempotency_key") {
			out, e := s.Service.CommandApplicationReleaseRollout(r.Context(), actor, project, parts[1], parts[3], q, s.Engine)
			respond(out, e, 200)
		}
	default:
		writeError(w, missing())
	}
	return true
}
