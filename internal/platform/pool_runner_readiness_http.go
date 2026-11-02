package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) poolRunnerReadinessHTTP(w http.ResponseWriter, r *http.Request, actor Actor, parts []string) bool {
	if len(parts) != 5 || parts[0] != "execution-pools" || parts[2] != "runners" || parts[4] != "readiness" {
		return false
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	out, e := s.Service.readPoolRunnerReadiness(r.Context(), actor, parts[1], parts[3], func() (string, string, error) {
		values, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(r.URL.RawQuery) > 1024 || len(values) != 2 || len(values["tenant_id"]) != 1 || len(values["project_id"]) != 1 {
			return "", "", invalid("输入无效")
		}
		return values.Get("tenant_id"), values.Get("project_id"), nil
	})
	respond(w, http.StatusOK, out, e)
	return true
}
