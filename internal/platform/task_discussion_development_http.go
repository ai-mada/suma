package platform

import (
	"io"
	"net/http"
	"net/url"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// This endpoint is intentionally decoded before the generic record API. The
// closed schema prevents callers from supplying a frozen goal, model binding,
// raw shell, source proof or approval result on behalf of the platform.
func (s *Server) taskDiscussionDevelopmentHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-discussion-development-plans" {
		return false
	}
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(discussionDevelopmentInvalid)) }
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" || len(parts) != 1 && (len(parts) != 3 || parts[2] != "start" || !handoffID(parts[1], false)) {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
		if err != nil || len(raw) > 64<<10 {
			bad()
			return true
		}
		if len(parts) == 1 {
			var request TaskDiscussionDevelopmentCreateRequest
			if woc.Decode(raw, &request) != nil || !validTaskDiscussionDevelopmentCreate(request) {
				bad()
				return true
			}
			if scope == "api" && (boundProject == "" || request.ProjectID != boundProject) {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.CreateTaskDiscussionDevelopmentPlan(r.Context(), s.Engine, actor, request.ProjectID, request)
			respond(w, http.StatusCreated, out, err)
			return true
		}
		var request TaskDispatchStartRequest
		if woc.Decode(raw, &request) != nil || !handoffID(request.ProjectID, false) || request.ExpectedVersion < 1 || !planningKey(request.IdempotencyKey) {
			bad()
			return true
		}
		if scope == "api" && (boundProject == "" || request.ProjectID != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.StartTaskDiscussionDevelopmentPlan(r.Context(), s.Engine, actor, request.ProjectID, parts[1], request)
		respond(w, http.StatusCreated, out, err)
		return true
	}
	if r.Method != http.MethodGet || len(parts) != 2 || !handoffID(parts[1], false) || len(r.URL.RawQuery) > 4096 {
		writeError(w, missing())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !handoffID(query.Get("project_id"), false) {
		bad()
		return true
	}
	project := query.Get("project_id")
	if scope == "api" && (boundProject == "" || project != boundProject) {
		writeError(w, forbidden())
		return true
	}
	out, err := s.Service.GetTaskDiscussionDevelopmentPlan(r.Context(), s.Engine, actor, project, parts[1])
	respond(w, http.StatusOK, out, err)
	return true
}
