package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// The transport owns strict request shape only. Current identity, role, source
// versions, mitigation evidence and every write decision remain in the domain.
func (s *Server) projectRisksHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, body map[string]any) bool {
	if len(parts) == 0 || parts[0] != "risks" {
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
		writeError(w, invalid(projectRiskInvalid))
		return true
	}
	paged := len(parts) == 1 && r.Method == http.MethodGet
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(paged && (key == "state" || key == "owner_id" || key == "limit" || key == "before")) {
			writeError(w, invalid(projectRiskInvalid))
			return true
		}
	}
	if values, ok := query["project_id"]; ok && values[0] != project || r.Method == http.MethodGet && query.Get("project_id") == "" {
		writeError(w, invalid(projectRiskInvalid))
		return true
	}
	limit := 20
	if raw := query.Get("limit"); raw != "" {
		if raw[0] == '0' {
			writeError(w, invalid(projectRiskInvalid))
			return true
		}
		for _, c := range raw {
			if c < '0' || c > '9' {
				writeError(w, invalid(projectRiskInvalid))
				return true
			}
		}
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeError(w, invalid(projectRiskInvalid))
			return true
		}
	}
	decode := func(out any, assessment bool, required ...string) bool {
		allowed := map[string]bool{}
		for _, key := range required {
			allowed[key] = true
		}
		switch out.(type) {
		case *ProjectRiskCreateRequest:
			allowed["source_alert_id"], allowed["expected_alert_version"] = true, true
		case *ProjectRiskSaveRequest:
		case *ProjectRiskCommandRequest:
			allowed["review"], allowed["expected_review_id"] = true, true
		default:
			writeError(w, invalid(projectRiskInvalid))
			return false
		}
		// encoding/json intentionally folds case for struct fields. This API
		// does not: canonical keys must be checked before typed decoding.
		for key := range body {
			if !allowed[key] {
				writeError(w, invalid(projectRiskInvalid))
				return false
			}
		}
		for _, key := range required {
			value, ok := body[key]
			if !ok || value == nil && key != "probability" && key != "impact" {
				writeError(w, invalid(projectRiskInvalid))
				return false
			}
		}
		for key, value := range body {
			if value == nil && !(assessment && (key == "probability" || key == "impact")) {
				writeError(w, invalid(projectRiskInvalid))
				return false
			}
		}
		if raw, ok := body["links"]; ok {
			links, ok := raw.([]any)
			if !ok {
				writeError(w, invalid(projectRiskInvalid))
				return false
			}
			for _, raw := range links {
				link, ok := raw.(map[string]any)
				if !ok || len(link) != 2 || link["kind"] == nil || link["id"] == nil {
					writeError(w, invalid(projectRiskInvalid))
					return false
				}
			}
		}
		if raw, ok := body["review"]; ok {
			review, ok := raw.(map[string]any)
			if !ok || len(review) != 5 {
				writeError(w, invalid(projectRiskInvalid))
				return false
			}
			for _, key := range []string{"disposition", "residual_probability", "residual_impact", "rationale", "evidence"} {
				value, present := review[key]
				if !present || value == nil && key != "residual_probability" && key != "residual_impact" {
					writeError(w, invalid(projectRiskInvalid))
					return false
				}
			}
		}
		if strictAutomation(body, out) != nil {
			writeError(w, invalid(projectRiskInvalid))
			return false
		}
		return true
	}
	fields := []string{"project_id", "title", "detail", "owner_id", "review_date", "strategy", "plan", "probability", "impact", "links", "reason", "idempotency_key"}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		out, err := s.Service.ListProjectRisks(r.Context(), actor, project, query.Get("state"), query.Get("owner_id"), limit, query.Get("before"))
		respond(w, 200, out, err)
	case len(parts) == 2 && parts[1] == "summary" && r.Method == http.MethodGet:
		out, err := s.Service.GetProjectRiskSummary(r.Context(), actor, project)
		respond(w, 200, out, err)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var request ProjectRiskCreateRequest
		if !decode(&request, true, fields...) {
			return true
		}
		out, err := s.Service.CreateProjectRisk(r.Context(), actor, project, request)
		status := http.StatusCreated
		if out.Created != nil && !*out.Created {
			status = http.StatusOK
		}
		respond(w, status, out, err)
	case len(parts) == 2 && r.Method == http.MethodGet:
		out, err := s.Service.GetProjectRisk(r.Context(), actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == http.MethodPatch:
		var request ProjectRiskSaveRequest
		if !decode(&request, true, append(fields, "expected_version")...) {
			return true
		}
		out, err := s.Service.SaveProjectRisk(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var request ProjectRiskCommandRequest
		if !decode(&request, false, "project_id", "expected_version", "action", "reason", "idempotency_key") {
			return true
		}
		out, err := s.Service.CommandProjectRisk(r.Context(), actor, project, parts[1], request)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
