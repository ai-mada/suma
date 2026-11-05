package platform

import (
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// Decode before the generic map handler, retaining duplicate/null/type checks.
func (s *Server) handleSharedTaskViews(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-views" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	bad := func() { writeError(w, invalid(sharedTaskViewInvalid)) }
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	isSync := len(parts) >= 2 && parts[1] == "sync"
	if isSync {
		if r.URL.Path != "/api/v1/task-views/sync" {
			writeError(w, missing())
			return true
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
			return true
		}
	}
	if len(parts) > 3 || len(parts) > 1 && !handoffID(parts[1], false) {
		bad()
		return true
	}
	if r.Method == http.MethodGet {
		if len(parts) > 2 || len(r.URL.RawQuery) > 8192 {
			bad()
			return true
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			bad()
			return true
		}
		for key, values := range query {
			if len(values) != 1 || values[0] == "" || key != "project_id" && (len(parts) != 1 || key != "limit" && key != "cursor") {
				bad()
				return true
			}
		}
		project := query.Get("project_id")
		if !handoffID(project, false) {
			bad()
			return true
		}
		if scope == "api" && project != boundProject {
			writeError(w, forbidden())
			return true
		}
		if isSync {
			out, e := s.Service.SyncSharedTaskViews(r.Context(), actor, project)
			respond(w, http.StatusOK, out, e)
			return true
		}
		if len(parts) == 2 {
			out, e := s.Service.GetSharedTaskView(r.Context(), actor, project, parts[1])
			respond(w, http.StatusOK, out, e)
			return true
		}
		limit := 20
		if query.Has("limit") {
			limit, err = strconv.Atoi(query.Get("limit"))
			if err != nil || limit < 1 || limit > 100 || strconv.Itoa(limit) != query.Get("limit") {
				bad()
				return true
			}
		}
		out, e := s.Service.ListSharedTaskViews(r.Context(), actor, project, limit, query.Get("cursor"))
		respond(w, http.StatusOK, out, e)
		return true
	}
	if r.URL.RawQuery != "" {
		bad()
		return true
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
	if err != nil || len(raw) > 16<<10 {
		bad()
		return true
	}
	var out SharedTaskView
	switch {
	case r.Method == http.MethodPost && len(parts) == 1:
		var req SharedTaskViewCreateRequest
		if woc.Decode(raw, &req) != nil {
			bad()
			return true
		}
		if scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		out, err = s.Service.CreateSharedTaskView(r.Context(), actor, req.ProjectID, req)
		respond(w, http.StatusCreated, out, err)
	case r.Method == http.MethodPatch && len(parts) == 2:
		var req SharedTaskViewUpdateRequest
		if woc.Decode(raw, &req) != nil {
			bad()
			return true
		}
		if scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		out, err = s.Service.UpdateSharedTaskView(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
	case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "archive":
		var req SharedTaskViewArchiveRequest
		if woc.Decode(raw, &req) != nil {
			bad()
			return true
		}
		if scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		out, err = s.Service.ArchiveSharedTaskView(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
