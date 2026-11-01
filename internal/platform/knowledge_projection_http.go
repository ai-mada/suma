package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func knowledgeIndexHTTPQuery(r *http.Request, search bool) (out ProjectKnowledgeIndexQuery, err error) {
	if len(r.URL.RawQuery) > 2048 {
		return out, knowledgeIndexInvalid()
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return out, knowledgeIndexInvalid()
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && (!search || key != "q" && key != "kind" && key != "limit") {
			return out, knowledgeIndexInvalid()
		}
	}
	if !search {
		return out, nil
	}
	out.Query, out.Kind = values.Get("q"), values.Get("kind")
	if raw, ok := values["kind"]; ok && raw[0] == "" {
		return out, knowledgeIndexInvalid()
	}
	if raw, ok := values["limit"]; ok {
		out.Limit, e = strconv.Atoi(raw[0])
		if e != nil || out.Limit < 1 || out.Limit > 20 || strconv.Itoa(out.Limit) != raw[0] {
			return out, knowledgeIndexInvalid()
		}
	}
	return out, nil
}
func (s *Server) knowledgeProjectionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string, parts []string) bool {
	if len(parts) != 2 || parts[0] != "project-knowledge" || parts[1] != "index-search" && parts[1] != "index-status" {
		return false
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if parts[1] == "index-search" {
		out, err := s.Service.searchProjectKnowledgeIndex(r.Context(), actor, project, func() (ProjectKnowledgeIndexQuery, error) { return knowledgeIndexHTTPQuery(r, true) })
		respond(w, http.StatusOK, out, err)
		return true
	}
	var out ProjectKnowledgeIndexStatus
	err := s.Service.withCurrentRead(r.Context(), actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		if _, e := knowledgeIndexHTTPQuery(r, false); e != nil {
			return e
		}
		var e error
		out, e = s.Service.knowledgeIndexStatusTx(auth.ctx, auth, project)
		return e
	})
	respond(w, http.StatusOK, out, err)
	return true
}
