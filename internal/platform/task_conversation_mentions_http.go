package platform

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The shared server invokes this only after current authentication, origin and
// original-view validation. Both endpoints observe current project/task data.
func (s *Server) taskConversationMentionHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-conversation-mention-targets" && parts[0] != "task-conversation-messages" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	directory := parts[0] == "task-conversation-mention-targets"
	if r.Method != http.MethodGet || directory && len(parts) != 1 || !directory && len(parts) != 2 {
		writeError(w, missing())
		return true
	}
	bad := func() { writeError(w, invalid(taskConversationMentionInvalid)) }
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	for key, values := range query {
		allowed := key == "project_id" || key == "task_id" || directory && (key == "q" || key == "limit" || key == "cursor" || key == "resource_ids")
		if !allowed || len(values) != 1 || values[0] == "" {
			bad()
			return true
		}
	}
	project, task := query.Get("project_id"), query.Get("task_id")
	if !handoffID(project, false) || !handoffID(task, false) {
		bad()
		return true
	}
	if scope == "api" && (boundProject == "" || project != boundProject) {
		writeError(w, forbidden())
		return true
	}
	if !directory {
		if !handoffID(parts[1], false) {
			bad()
			return true
		}
		out, err := s.Service.TaskConversationMessageDetail(r.Context(), actor, project, task, parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	q := TaskConversationMentionTargetQuery{Q: query.Get("q"), Cursor: query.Get("cursor")}
	if query.Has("limit") {
		raw := query.Get("limit")
		value, parseErr := strconv.Atoi(raw)
		if parseErr != nil || value < 1 || value > 50 || strconv.Itoa(value) != raw {
			bad()
			return true
		}
		q.Limit = value
	}
	if query.Has("resource_ids") {
		if query.Has("q") || query.Has("cursor") {
			bad()
			return true
		}
		q.ResourceIDs = strings.Split(query.Get("resource_ids"), ",")
	}
	var valid bool
	q, valid = normalizeTaskConversationMentionTargetQuery(q)
	if !valid {
		bad()
		return true
	}
	out, err := s.Service.ListTaskConversationMentionTargets(r.Context(), actor, project, task, q)
	respond(w, http.StatusOK, out, err)
	return true
}
