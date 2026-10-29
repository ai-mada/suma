package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

func (s *Server) myWorkHTTP(w http.ResponseWriter, r *http.Request, actor Actor, project string) {
	if !myWorkHuman(actor) {
		writeError(w, forbidden())
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return
	}
	options, err := myWorkQuery(r, project)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.Service.MyWork(r.Context(), actor, project, options)
	respond(w, http.StatusOK, out, err)
}

func myWorkQuery(r *http.Request, project string) (MyWorkOptions, error) {
	options := MyWorkOptions{Limit: 20}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		return options, invalid(myWorkInvalid)
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "project_id" && key != "q" && key != "status" && key != "limit" && key != "cursor") {
			return options, invalid(myWorkInvalid)
		}
	}
	if !handoffID(project, false) || values.Get("project_id") != project || values.Has("cursor") && values.Get("cursor") == "" {
		return options, invalid(myWorkInvalid)
	}
	options.Q, options.Status, options.Cursor = values.Get("q"), values.Get("status"), values.Get("cursor")
	if values.Has("limit") {
		value := values.Get("limit")
		options.Limit, err = strconv.Atoi(value)
		if err != nil || options.Limit < 1 || options.Limit > 50 || strconv.Itoa(options.Limit) != value {
			return options, invalid(myWorkInvalid)
		}
	}
	return normalizeMyWorkOptions(options)
}
