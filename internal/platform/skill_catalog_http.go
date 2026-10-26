package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func skillCatalogHTTPQuery(r *http.Request, mode string) (out SkillCatalogListQuery, err error) {
	if len(r.URL.RawQuery) > 8192 {
		return out, invalid(skillInvalid)
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return out, invalid(skillInvalid)
	}
	for key, items := range values {
		allowed := key == "project_id" || mode == "list" && (key == "domain" || key == "scenario" || key == "tag" || key == "state" || key == "q") || mode != "detail" && (key == "limit" || key == "cursor")
		if !allowed || len(items) != 1 {
			return out, invalid(skillInvalid)
		}
	}
	out = SkillCatalogListQuery{ProjectID: values.Get("project_id"), Domain: values.Get("domain"), Scenario: values.Get("scenario"), Tag: values.Get("tag"), State: values.Get("state"), Query: values.Get("q"), Cursor: values.Get("cursor")}
	if !handoffID(out.ProjectID, false) {
		return out, invalid(skillInvalid)
	}
	if raw, ok := values["limit"]; ok {
		if raw[0] == "" || raw[0][0] == '0' {
			return out, invalid(skillInvalid)
		}
		for _, c := range raw[0] {
			if c < '0' || c > '9' {
				return out, invalid(skillInvalid)
			}
		}
		out.Limit, e = strconv.Atoi(raw[0])
		if e != nil || out.Limit < 1 || out.Limit > 100 {
			return out, invalid(skillInvalid)
		}
	}
	return out, nil
}
func skillCatalogRequired(m map[string]any, out any, fields ...string) error {
	for _, field := range fields {
		if value, ok := m[field]; !ok || value == nil {
			return invalid(skillInvalid)
		}
	}
	return strictAutomation(m, out)
}
func (s *Server) skillCatalogHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string, parts []string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "skill-catalog" {
		return false
	}
	if r.Method != http.MethodGet {
		values, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(values) != 0 {
			writeError(w, invalid(skillInvalid))
			return true
		}
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		query, e := skillCatalogHTTPQuery(r, "list")
		if e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.ListSkillCatalog(r.Context(), actor, query)
		respond(w, 200, out, e)
	case len(parts) == 1 && r.Method == http.MethodPost:
		var req SkillCatalogWriteRequest
		if e := skillCatalogRequired(m, &req, "project_id", "knowledge_id", "expected_knowledge_version", "published_revision", "published_sha256", "expected_version", "domain", "scenarios", "tags", "reason", "acknowledge_sharing", "idempotency_key"); e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.CreateSkillCatalog(r.Context(), actor, req)
		respond(w, 201, out, e)
	case len(parts) == 2 && r.Method == http.MethodGet:
		query, e := skillCatalogHTTPQuery(r, "detail")
		if e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.GetSkillCatalog(r.Context(), actor, query.ProjectID, parts[1])
		respond(w, 200, out, e)
	case len(parts) == 2 && r.Method == http.MethodPatch:
		var req SkillCatalogWriteRequest
		if e := skillCatalogRequired(m, &req, "project_id", "knowledge_id", "expected_knowledge_version", "published_revision", "published_sha256", "expected_version", "domain", "scenarios", "tags", "reason", "acknowledge_sharing", "idempotency_key"); e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.UpdateSkillCatalog(r.Context(), actor, parts[1], req)
		respond(w, 200, out, e)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == http.MethodPost:
		var req SkillCatalogCommandRequest
		if e := skillCatalogRequired(m, &req, "project_id", "action", "expected_version", "reason", "idempotency_key"); e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.CommandSkillCatalog(r.Context(), actor, parts[1], req)
		respond(w, 200, out, e)
	case len(parts) == 3 && parts[2] == "adoptions" && r.Method == http.MethodPost:
		var req SkillCatalogAdoptRequest
		if e := skillCatalogRequired(m, &req, "project_id", "expected_version", "origin_sha256", "reason", "acknowledge_draft", "idempotency_key"); e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.AdoptSkillCatalog(r.Context(), actor, parts[1], req)
		respond(w, 201, out, e)
	case len(parts) == 3 && parts[2] == "revisions" && r.Method == http.MethodGet:
		query, e := skillCatalogHTTPQuery(r, "revisions")
		if e != nil {
			writeError(w, e)
			break
		}
		out, e := s.Service.SkillCatalogRevisions(r.Context(), actor, query.ProjectID, parts[1], query.Limit, query.Cursor)
		respond(w, 200, out, e)
	default:
		writeError(w, missing())
	}
	return true
}
