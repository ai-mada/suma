package platform

import (
	"net/http"
	"net/url"
)

func (s *Server) deliveryKnowledgeSourceHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project, taskID string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeError(w, missing())
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if len(r.URL.RawQuery) > 8192 || err != nil || len(values) != 1 || len(values["project_id"]) != 1 || values.Get("project_id") != project || project == "" {
		writeError(w, invalid("知识详情只接受唯一项目参数"))
		return
	}
	out, err := s.Service.GetDeliveryKnowledgeSource(r.Context(), actor, project, taskID)
	respond(w, http.StatusOK, out, err)
}
