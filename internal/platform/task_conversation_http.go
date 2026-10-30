package platform

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// taskConversationHTTP runs after authentication, origin and original-view
// validation but before the generic map decoder. It preserves canonical JSON
// keys, integer tokens, Unicode and the original message body for recovery.
func (s *Server) taskConversationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "task-conversations" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !taskConversationActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if len(parts) != 1 || r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, missing())
		return true
	}
	bad := func() { writeError(w, invalid(taskConversationInvalid)) }
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (128<<10)+1))
		var req TaskConversationAppendRequest
		// The existing shared closed decoder rejects duplicates, case aliases,
		// null scalars and malformed Unicode; it confers no workspace authority.
		if err != nil || len(raw) > 128<<10 || woc.Decode(raw, &req) != nil || !validTaskConversationAppend(req) {
			bad()
			return true
		}
		if scope == "api" && (boundProject == "" || req.ProjectID != boundProject) {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.AppendTaskConversation(r.Context(), actor, req.ProjectID, req)
		respond(w, http.StatusCreated, out, err)
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		bad()
		return true
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" || key != "project_id" && key != "task_id" && key != "limit" && key != "before" {
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
	positive := func(raw string, max int64) (int64, bool) {
		if raw == "" || raw[0] == '0' {
			return 0, false
		}
		for _, digit := range raw {
			if digit < '0' || digit > '9' {
				return 0, false
			}
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		return value, err == nil && value > 0 && value <= max
	}
	limit, before := int64(20), int64(0)
	if query.Has("limit") {
		var valid bool
		if limit, valid = positive(query.Get("limit"), 50); !valid {
			bad()
			return true
		}
	}
	if query.Has("before") {
		var valid bool
		if before, valid = positive(query.Get("before"), taskConversationMaxSeq); !valid {
			bad()
			return true
		}
	}
	out, err := s.Service.ListTaskConversation(r.Context(), actor, project, task, int(limit), before)
	respond(w, http.StatusOK, out, err)
	return true
}
