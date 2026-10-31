package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func (s *Server) workBuddyTaskDispatchHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string, c *WorkBuddyConnections) bool {
	if len(parts) == 0 || parts[0] != "workbuddy-task-dispatch-plans" {
		return false
	}
	cloudHTTPHeaders(w)
	if !actor.IsBrowser() || scope != "" {
		writeError(w, forbidden())
		return true
	}
	if c == nil {
		writeError(w, &APIError{Status: 503, Code: workBuddyUnavailable, Message: workBuddyUnavailable})
		return true
	}
	bad := func() { writeError(w, invalid(workBuddyTaskDispatchInvalid)) }
	if len(parts) > 3 || len(parts) > 1 && !taskSubscriptionID(parts[1]) || len(parts) == 3 && parts[2] != "start" {
		bad()
		return true
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if len(r.URL.RawQuery) > 8192 || err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !taskSubscriptionID(query.Get("project_id")) {
			bad()
			return true
		}
		out, err := c.GetTaskDispatchPlan(r.Context(), s.Engine, actor, query.Get("project_id"), parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	if r.Method == http.MethodPost && (len(parts) == 1 || len(parts) == 3) {
		if r.URL.RawQuery != "" || r.Body == nil {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
		if err != nil || len(raw) > 64<<10 {
			bad()
			return true
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			bad()
			return true
		}
		required := []string{"project_id", "expected_version", "idempotency_key"}
		if len(parts) == 1 {
			required = []string{"project_id", "task_id", "expected_task_version", "connection_id", "expected_connection_version", "grant_generation", "idempotency_key"}
		}
		for _, key := range required {
			if _, ok := fields[key]; !ok {
				bad()
				return true
			}
		}
		if len(parts) == 1 {
			var req WorkBuddyTaskDispatchPreviewRequest
			if wc.Decode(raw, &req) != nil || !validWorkBuddyTaskDispatchPreview(req.ProjectID, req) || string(fields["knowledge"]) == "null" {
				bad()
				return true
			}
			out, err := c.CreateTaskDispatchPlan(r.Context(), s.Engine, actor, req.ProjectID, req)
			respond(w, http.StatusCreated, out, err)
			return true
		}
		var req WorkBuddyTaskDispatchStartRequest
		if wc.Decode(raw, &req) != nil {
			bad()
			return true
		}
		out, err := c.StartTaskDispatchPlan(r.Context(), s.Engine, actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusCreated, out, err)
		return true
	}
	w.Header().Set("Allow", http.MethodPost)
	if len(parts) == 2 {
		w.Header().Set("Allow", http.MethodGet)
	}
	writeError(w, &APIError{Status: 405, Code: "method_not_allowed", Message: "Method not allowed"})
	return true
}
