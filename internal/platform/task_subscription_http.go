package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func (s *Server) taskSubscriptionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) < 3 || parts[0] != "tasks" || parts[2] != "subscription" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(taskSubscriptionInvalid)) }
	if len(parts) > 4 || !taskSubscriptionID(parts[1]) || len(parts) == 4 && parts[3] != "commands" || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(q) != 1 || len(q["project_id"]) != 1 || !taskSubscriptionID(q.Get("project_id")) {
		bad()
		return true
	}
	project := q.Get("project_id")
	if scope == "api" && project != boundProject {
		writeError(w, forbidden())
		return true
	}
	if len(parts) == 3 && r.Method == http.MethodGet {
		out, e := s.Service.GetTaskSubscription(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, e)
		return true
	}
	if len(parts) == 4 && r.Method == http.MethodPost {
		raw, e := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
		if e != nil || len(raw) > 16<<10 {
			bad()
			return true
		}
		var req TaskSubscriptionCommand
		var fields map[string]json.RawMessage
		if woc.Decode(raw, &req) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 5 || !validTaskSubscriptionCommand(project, parts[1], req) {
			bad()
			return true
		}
		for _, key := range []string{"project_id", "task_id", "expected_version", "action", "idempotency_key"} {
			if _, ok := fields[key]; !ok {
				bad()
				return true
			}
		}
		out, e := s.Service.CommandTaskSubscription(r.Context(), actor, project, parts[1], req)
		respond(w, http.StatusOK, out, e)
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	if len(parts) == 4 {
		w.Header().Set("Allow", http.MethodPost)
	}
	writeError(w, &APIError{Status: 405, Code: "method_not_allowed", Message: "Method not allowed"})
	return true
}
