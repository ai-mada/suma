package platform

import (
	"net/http"
	"net/url"
	"strconv"
)

// workspaceTaskContextHTTP is a read-only browser/project-API composer. It is
// deliberately separate from file access, Runner and MCP operation endpoints.
func (s *Server) workspaceTaskContextHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) < 2 || parts[0] != "workspace" || parts[1] != "task-context" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if r.Method != http.MethodGet || len(parts) > 3 || len(parts) == 3 && parts[2] != "tasks" {
		writeError(w, missing())
		return true
	}
	picker := len(parts) == 3
	bad := func() { writeError(w, invalid(workspaceTaskContextInvalid)) }
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	for key, entries := range values {
		if len(entries) != 1 || key != "project_id" && !(key == "task_id" && !picker) && !(picker && (key == "search" || key == "limit" || key == "cursor")) {
			bad()
			return true
		}
	}
	project := values.Get("project_id")
	if !handoffID(project, false) {
		bad()
		return true
	}
	// Browser sessions select a currently authorized project through the query;
	// a project API credential retains its original immutable project scope.
	if scope == "api" && (boundProject == "" || project != boundProject) {
		writeError(w, forbidden())
		return true
	}
	if !picker {
		if !handoffID(values.Get("task_id"), false) {
			bad()
			return true
		}
		out, e := s.Service.GetWorkspaceTaskContext(r.Context(), actor, project, values.Get("task_id"))
		respond(w, http.StatusOK, out, e)
		return true
	}
	limit := 20
	if raw, present := values["limit"]; present {
		value := raw[0]
		if value == "" || value[0] == '0' {
			bad()
			return true
		}
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				bad()
				return true
			}
		}
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 50 {
			bad()
			return true
		}
	}
	if values.Has("cursor") && values.Get("cursor") == "" {
		bad()
		return true
	}
	out, err := s.Service.ListWorkspaceTaskOptions(r.Context(), actor, project, values.Get("search"), limit, values.Get("cursor"))
	respond(w, http.StatusOK, out, err)
	return true
}
