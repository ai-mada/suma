package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) taskDesignViewHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string, parts []string) bool {
	if len(parts) != 3 || parts[0] != "tasks" || parts[2] != "design-context" {
		return false
	}
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 || len(values) != 1 || len(values["project_id"]) != 1 || values.Get("project_id") == "" || values.Get("project_id") != project {
		writeError(w, invalid("任务设计依据只接受唯一的当前项目参数"))
		return true
	}
	out, err := s.Service.GetTaskDesignContext(r.Context(), actor, project, parts[1])
	respond(w, 200, out, err)
	return true
}
