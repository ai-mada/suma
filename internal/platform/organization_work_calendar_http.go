package platform

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func decodeOrganizationWorkCalendarSave(r *http.Request) (OrganizationWorkCalendarSaveRequest, error) {
	var out OrganizationWorkCalendarSaveRequest
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || len(raw) > limit || !toolcontract.ValidJSONUnicode(raw) || RejectDuplicateReleaseJSON(raw) != nil {
		return out, invalid(organizationWorkCalendarInvalid)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 ||
		bytes.Equal(bytes.TrimSpace(fields["expected_version"]), []byte("null")) ||
		json.Unmarshal(fields["expected_version"], &out.ExpectedVersion) != nil ||
		json.Unmarshal(fields["idempotency_key"], &out.IdempotencyKey) != nil {
		return out, invalid(organizationWorkCalendarInvalid)
	}
	// Preserve the integer lexeme above and reuse the project calendar's
	// explicit weekday/exception presence and null checks below.
	var schedule map[string]any
	if json.Unmarshal(fields["schedule"], &schedule) != nil {
		return out, invalid(organizationWorkCalendarInvalid)
	}
	checked, err := decodeResourceCalendarSave(map[string]any{
		"expected_version": out.ExpectedVersion,
		"idempotency_key":  out.IdempotencyKey,
		"schedule":         schedule,
	})
	if err != nil {
		return out, err
	}
	out.Schedule = checked.Schedule
	return out, nil
}

func (s *Server) organizationWorkCalendarHTTP(w http.ResponseWriter, r *http.Request, actor Actor, path string) bool {
	user, found := strings.CutPrefix(path, "organization/work-calendars/")
	if !found && path != "organization/work-calendars" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if !actor.IsBrowser() {
		writeError(w, forbidden())
		return true
	}
	if !handoffID(user, false) {
		writeError(w, missing())
		return true
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, invalid(organizationWorkCalendarInvalid))
		return true
	}
	switch r.Method {
	case http.MethodGet:
		out, err := s.Service.GetOrganizationWorkCalendar(r.Context(), actor, user)
		respond(w, http.StatusOK, out, err)
	case http.MethodPut:
		req, err := decodeOrganizationWorkCalendarSave(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.SaveOrganizationWorkCalendar(r.Context(), actor, user, req)
		respond(w, http.StatusOK, out, err)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, &APIError{405, "method_not_allowed", "Organization work calendars support GET and PUT", nil})
	}
	return true
}
