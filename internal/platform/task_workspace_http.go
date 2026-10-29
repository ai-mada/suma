package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) taskWorkspaceHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &APIError{http.StatusMethodNotAllowed, "method_not_allowed", "不支持此请求方法", nil})
		return
	}
	if len(r.URL.RawQuery) > 8192 {
		writeError(w, invalid(taskWorkspaceInvalid))
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, invalid(taskWorkspaceInvalid))
		return
	}
	for key, entries := range values {
		if len(entries) != 1 {
			writeError(w, invalid(taskWorkspaceInvalid))
			return
		}
		switch key {
		case "project_id", "status", "owner_id", "priority", "sort", "limit", "cursor":
			if entries[0] == "" {
				writeError(w, invalid(taskWorkspaceInvalid))
				return
			}
		case "q":
		default:
			writeError(w, invalid(taskWorkspaceInvalid))
			return
		}
	}
	if !handoffID(project, false) || values.Get("project_id") != project {
		writeError(w, invalid(taskWorkspaceInvalid))
		return
	}
	q := TaskWorkspaceQuery{Q: values.Get("q"), Status: values.Get("status"), OwnerID: values.Get("owner_id"), Priority: values.Get("priority"), Sort: values.Get("sort"), Cursor: values.Get("cursor"), Limit: 100}
	if values.Has("limit") {
		q.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || q.Limit < 1 || q.Limit > 100 || strconv.Itoa(q.Limit) != values.Get("limit") {
			writeError(w, invalid(taskWorkspaceInvalid))
			return
		}
	}
	page, err := s.Service.ListTaskWorkspace(r.Context(), actor, project, q)
	respond(w, http.StatusOK, page, err)
}
