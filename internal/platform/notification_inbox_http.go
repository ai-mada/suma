package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func (s *Server) notificationInboxHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "notification-inbox" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(notificationInboxInvalid)) }
	if len(parts) > 3 || len(parts) >= 2 && !handoffID(parts[1], false) {
		bad()
		return true
	}
	organization := len(parts) == 2 && parts[1] == "organization"
	if organization && (scope != "" || actor.kind != actorBrowser) {
		writeError(w, forbidden())
		return true
	}
	if r.Method == http.MethodGet && (len(parts) == 1 || organization) {
		values, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 8192 {
			bad()
			return true
		}
		for key, entries := range values {
			known := key == "filter" || key == "limit" || key == "cursor" || !organization && key == "project_id"
			if len(entries) != 1 || !known || entries[0] == "" {
				bad()
				return true
			}
		}
		project, filter, cursor := values.Get("project_id"), values.Get("filter"), values.Get("cursor")
		if !organization {
			if !handoffID(project, false) {
				bad()
				return true
			}
			if scope == "api" && project != boundProject {
				writeError(w, forbidden())
				return true
			}
		}
		limit := 20
		if values.Has("limit") {
			raw := values.Get("limit")
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 50 || strconv.Itoa(limit) != raw {
				bad()
				return true
			}
		}
		if organization {
			out, err := s.Service.NotificationOrganizationInbox(r.Context(), actor, filter, limit, cursor)
			respond(w, http.StatusOK, out, err)
		} else {
			out, err := s.Service.NotificationInbox(r.Context(), actor, project, filter, limit, cursor)
			respond(w, http.StatusOK, out, err)
		}
		return true
	}
	if r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "commands" && handoffID(parts[1], false) {
		if r.URL.RawQuery != "" {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
		if err != nil || len(raw) > 16<<10 {
			bad()
			return true
		}
		var req NotificationInboxCommand
		var fields map[string]json.RawMessage
		if woc.Decode(raw, &req) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 4 || !validNotificationInboxCommand(req.ProjectID, parts[1], req) {
			bad()
			return true
		}
		for _, key := range []string{"project_id", "expected_version", "action", "idempotency_key"} {
			if _, exists := fields[key]; !exists {
				bad()
				return true
			}
		}
		if scope == "api" && req.ProjectID != boundProject {
			writeError(w, forbidden())
			return true
		}
		out, err := s.Service.CommandNotificationInbox(r.Context(), actor, req.ProjectID, parts[1], req)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if organization || len(parts) == 1 || len(parts) == 3 && parts[2] == "commands" {
		allow := http.MethodGet
		if len(parts) == 3 {
			allow = http.MethodPost
		}
		w.Header().Set("Allow", allow)
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return true
	}
	writeError(w, missing())
	return true
}
