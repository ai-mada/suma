package platform

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func taskOwnerTargetQuery(r *http.Request, project string) (TaskOwnerTargetQuery, error) {
	q := TaskOwnerTargetQuery{Limit: 20}
	if len(r.URL.RawQuery) > 8192 {
		return q, invalid(taskOwnerTargetInvalid)
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, invalid(taskOwnerTargetInvalid)
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && key != "q" && key != "limit" && key != "cursor" || items[0] == "" && key != "q" && key != "cursor" {
			return q, invalid(taskOwnerTargetInvalid)
		}
	}
	if values.Get("project_id") != project || !handoffID(project, false) {
		return q, invalid(taskOwnerTargetInvalid)
	}
	if values.Has("limit") {
		q.Limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || q.Limit < 1 || q.Limit > 100 || strconv.Itoa(q.Limit) != values.Get("limit") {
			return q, invalid(taskOwnerTargetInvalid)
		}
	}
	q.Q, q.Cursor = values.Get("q"), values.Get("cursor")
	return q, nil
}

// The outer envelope preserves integer lexemes and rejects duplicate JSON keys;
// this second closed decode rejects aliases, null scalars and extra fields.
func decodeTaskOwnerChange(m map[string]any, out any) error {
	raw, err := json.Marshal(m)
	if err != nil || woc.Decode(raw, out) != nil {
		return invalid(taskOwnerChangeInvalid)
	}
	return nil
}

func (s *Server) taskOwnerChangesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "task-owner-targets" && parts[0] != "task-owner-changes" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if parts[0] == "task-owner-targets" {
		if len(parts) != 1 || r.Method != http.MethodGet {
			writeError(w, missing())
			return true
		}
		q, err := taskOwnerTargetQuery(r, project)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListTaskOwnerTargets(r.Context(), actor, project, q)
		respond(w, http.StatusOK, out, err)
		return true
	}
	if r.Method != http.MethodPost || len(parts) > 2 || len(parts) == 2 && parts[1] != "preview" {
		writeError(w, missing())
		return true
	}
	if r.URL.RawQuery != "" {
		writeError(w, invalid(taskOwnerChangeInvalid))
		return true
	}
	if len(parts) == 2 {
		var req TaskOwnerChangePreviewRequest
		if err := decodeTaskOwnerChange(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.PreviewTaskOwnerChanges(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	} else {
		var req TaskOwnerChangeRequest
		if err := decodeTaskOwnerChange(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ChangeTaskOwners(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	}
	return true
}
