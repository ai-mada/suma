package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func (s *Server) projectNotificationPreferencesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "notification-preferences" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(projectNotificationPreferencesInvalid)) }
	if len(parts) > 2 || len(parts) == 2 && parts[1] != "commands" || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["project_id"]) != 1 || !taskSubscriptionID(query.Get("project_id")) {
		bad()
		return true
	}
	project := query.Get("project_id")
	if scope == "api" && project != boundProject {
		writeError(w, forbidden())
		return true
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		out, e := s.Service.GetProjectNotificationPreferences(r.Context(), actor, project)
		respond(w, http.StatusOK, out, e)
		return true
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		raw, e := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
		if e != nil || len(raw) > 16<<10 {
			bad()
			return true
		}
		var req ProjectNotificationPreferencesCommand
		var fields map[string]json.RawMessage
		if woc.Decode(raw, &req) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 4 || !validProjectNotificationPreferencesCommand(project, req) {
			bad()
			return true
		}
		for _, key := range []string{"project_id", "expected_version", "followed_discussions_muted", "idempotency_key"} {
			if _, ok := fields[key]; !ok {
				bad()
				return true
			}
		}
		out, e := s.Service.CommandProjectNotificationPreferences(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, e)
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	if len(parts) == 2 {
		w.Header().Set("Allow", http.MethodPost)
	}
	writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
	return true
}
