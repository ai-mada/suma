package platform

import (
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// Runs after the shared authentication/origin/original-view guard and before
// generic map decoding. Commands retain their complete typed original bytes.
func (s *Server) taskReviewRequestsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-review-targets" && parts[0] != "task-review-requests" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	targets := parts[0] == "task-review-targets"
	bad := func() { writeError(w, invalid(taskReviewRequestInvalid)) }
	bound := func(project string) bool {
		if scope == "api" && (boundProject == "" || project != boundProject) {
			writeError(w, forbidden())
			return false
		}
		return true
	}
	if r.Method == http.MethodPost && !targets && (len(parts) == 1 || len(parts) == 3 && parts[2] == "commands") {
		if r.URL.RawQuery != "" {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
		if err != nil || len(raw) > 128<<10 {
			bad()
			return true
		}
		if len(parts) == 1 {
			var req TaskReviewRequestCreate
			if woc.Decode(raw, &req) != nil || !validTaskReviewRequestCreate(req.ProjectID, req) {
				bad()
				return true
			}
			if !bound(req.ProjectID) {
				return true
			}
			out, err := s.Service.CreateTaskReviewRequest(r.Context(), actor, req.ProjectID, req)
			respond(w, http.StatusCreated, out, err)
			return true
		}
		var req TaskReviewRequestCommand
		if woc.Decode(raw, &req) != nil || !validTaskReviewRequestCommand(req.ProjectID, parts[1], req) {
			bad()
			return true
		}
		if !bound(req.ProjectID) {
			return true
		}
		out, err := s.Service.CommandTaskReviewRequest(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if r.Method != http.MethodGet || targets && len(parts) != 1 || !targets && len(parts) != 1 && len(parts) != 2 {
		writeError(w, missing())
		return true
	}
	detail := !targets && len(parts) == 2
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	for key, values := range query {
		allowed := key == "project_id" || !detail && (key == "task_id" || key == "limit" || key == "cursor") || targets && (key == "q" || key == "resource_ids")
		if !allowed || len(values) != 1 || values[0] == "" {
			bad()
			return true
		}
	}
	project, task := query.Get("project_id"), query.Get("task_id")
	if !handoffID(project, false) || !detail && !handoffID(task, false) || detail && !handoffID(parts[1], false) {
		bad()
		return true
	}
	if !bound(project) {
		return true
	}
	if detail {
		out, err := s.Service.GetTaskReviewRequest(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	limit := 20
	if query.Has("limit") {
		raw := query.Get("limit")
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 50 || strconv.Itoa(value) != raw {
			bad()
			return true
		}
		limit = value
	}
	if !targets {
		out, err := s.Service.ListTaskReviewRequests(r.Context(), actor, project, task, limit, query.Get("cursor"))
		respond(w, http.StatusOK, out, err)
		return true
	}
	q := TaskReviewTargetQuery{Q: query.Get("q"), Limit: limit, Cursor: query.Get("cursor")}
	if query.Has("resource_ids") {
		if query.Has("q") || query.Has("cursor") {
			bad()
			return true
		}
		q.ResourceIDs = strings.Split(query.Get("resource_ids"), ",")
	}
	out, err := s.Service.ListTaskReviewTargets(r.Context(), actor, project, task, q)
	respond(w, http.StatusOK, out, err)
	return true
}
