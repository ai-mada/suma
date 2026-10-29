package platform

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func (s *Server) taskConversationReplyHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-conversation-replies" {
		return false
	}
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(conversationReplyInvalid)) }
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" || len(parts) != 1 && (len(parts) != 3 || parts[2] != "commands" || !handoffID(parts[1], false)) {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
		if err != nil || len(raw) > 128<<10 {
			bad()
			return true
		}
		if len(parts) == 1 {
			var request TaskConversationReplyCreateRequest
			if woc.Decode(raw, &request) != nil || !validTaskConversationReplyCreate(request) {
				bad()
				return true
			}
			if scope == "api" && (boundProject == "" || request.ProjectID != boundProject) {
				writeError(w, forbidden())
				return true
			}
			out, err := s.Service.CreateTaskConversationReply(r.Context(), s.Engine, actor, request.ProjectID, request)
			respond(w, http.StatusCreated, out, err)
			return true
		}
		var request TaskConversationReplyCommandRequest
		if woc.Decode(raw, &request) != nil {
			bad()
			return true
		}
		if scope == "api" && (boundProject == "" || request.ProjectID != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.CommandTaskConversationReply(r.Context(), s.Engine, actor, request.ProjectID, parts[1], request)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if r.Method != http.MethodGet || len(parts) > 2 || len(parts) == 2 && !handoffID(parts[1], false) {
		writeError(w, missing())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		bad()
		return true
	}
	list := len(parts) == 1
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && !(list && (key == "task_id" || key == "limit" || key == "cursor")) {
			bad()
			return true
		}
	}
	project := query.Get("project_id")
	if !handoffID(project, false) || list && (!handoffID(query.Get("task_id"), false) || !handoffID(query.Get("cursor"), true)) {
		bad()
		return true
	}
	if scope == "api" && (boundProject == "" || project != boundProject) {
		writeError(w, forbidden())
		return true
	}
	if !list {
		out, err := s.Service.GetTaskConversationReply(r.Context(), s.Engine, actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	limit := 20
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 50 || strconv.Itoa(limit) != query.Get("limit") {
			bad()
			return true
		}
	}
	out, err := s.Service.ListTaskConversationReplies(r.Context(), s.Engine, actor, project, query.Get("task_id"), limit, query.Get("cursor"))
	respond(w, http.StatusOK, out, err)
	return true
}
