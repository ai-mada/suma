package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func knowledgeDiscoveryHTTPQuery(r *http.Request) (out ProjectKnowledgeDiscoveryQuery, err error) {
	if len(r.URL.RawQuery) > 4096 {
		return out, knowledgeDiscoveryInvalid()
	}
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		return out, knowledgeDiscoveryInvalid()
	}
	for key, items := range values {
		if len(items) != 1 {
			return out, knowledgeDiscoveryInvalid()
		}
		switch key {
		case "project_id", "q", "kind", "limit", "cursor":
		default:
			return out, knowledgeDiscoveryInvalid()
		}
	}
	out.Query, out.Kind, out.Cursor = values.Get("q"), values.Get("kind"), values.Get("cursor")
	if raw, exists := values["kind"]; exists && raw[0] == "" {
		return out, knowledgeDiscoveryInvalid()
	}
	if raw, exists := values["limit"]; exists {
		out.Limit, e = strconv.Atoi(raw[0])
		if e != nil || out.Limit < 1 || out.Limit > 20 || strconv.Itoa(out.Limit) != raw[0] {
			return out, knowledgeDiscoveryInvalid()
		}
	}
	return out, nil
}
func (s *Server) knowledgeDiscoveryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string, parts []string) bool {
	if len(parts) != 2 || parts[0] != "project-knowledge" || parts[1] != "discovery" {
		return false
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	var out ProjectKnowledgeDiscovery
	// The transport's closed query shape is checked in the same current
	// project-read transaction as candidate resolution. A still-authenticated
	// but revoked member cannot mask that revocation with invalid filters.
	err := s.Service.withCurrentRead(r.Context(), actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, knowledgeReadPurpose(actor), func(auth *authorizedCommand) error {
		query, e := knowledgeDiscoveryHTTPQuery(r)
		if e != nil {
			return e
		}
		out, e = s.Service.discoverProjectKnowledgeTx(auth.ctx, auth, project, query)
		return e
	})
	respond(w, http.StatusOK, out, err)
	return true
}
