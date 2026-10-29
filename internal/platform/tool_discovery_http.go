package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) toolDiscoveryHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "tool-discovery" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	if len(parts) != 1 || r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		writeError(w, invalid("工具检测请求无效"))
		return true
	}
	for key, values := range q {
		if (key != "project_id" && key != "worker_id") || len(values) != 1 || values[0] == "" {
			writeError(w, invalid("工具检测请求无效"))
			return true
		}
	}
	if q.Get("project_id") == "" || q.Get("project_id") != project || q.Get("worker_id") == "" {
		writeError(w, invalid("工具检测请求无效"))
		return true
	}
	out, err := s.Service.DiscoverTools(r.Context(), actor, project, q.Get("worker_id"))
	respond(w, http.StatusOK, out, err)
	return true
}
