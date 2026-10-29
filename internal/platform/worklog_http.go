package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func worklogDecode(raw []byte, out any, fields int) bool {
	if woc.Decode(raw, out) != nil {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && len(object) == fields
}

// Run after original-view and credential checks, before the generic map decoder
// so integer tokens, duplicate keys and canonical Unicode retain strict checks.
func (s *Server) handleWorklogs(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	if len(parts) == 0 || parts[0] != "worklogs" {
		return false
	}
	if !testPlanActor(actor) || scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	bad := func() { writeError(w, invalid(worklogInvalid)) }
	if len(parts) > 3 || len(parts) >= 2 && !handoffID(parts[1], false) {
		bad()
		return true
	}
	if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" {
			bad()
			return true
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (16<<10)+1))
		if err != nil || len(raw) > 16<<10 {
			bad()
			return true
		}
		var out Worklog
		project := ""
		status := http.StatusOK
		switch {
		case len(parts) == 1:
			var req WorklogCreateRequest
			if !worklogDecode(raw, &req, 9) || !validWorklogCreate(req) {
				bad()
				return true
			}
			project = req.ProjectID
			if scope == "api" && project != boundProject {
				writeError(w, forbidden())
				return true
			}
			out, err = s.Service.CreateWorklog(r.Context(), actor, project, req)
			status = http.StatusCreated
		case len(parts) == 3 && parts[2] == "corrections":
			var req WorklogCorrectionRequest
			if !worklogDecode(raw, &req, 10) || !validWorklogCorrection(req) {
				bad()
				return true
			}
			project = req.ProjectID
			if scope == "api" && project != boundProject {
				writeError(w, forbidden())
				return true
			}
			out, err = s.Service.CorrectWorklog(r.Context(), actor, project, parts[1], req)
		case len(parts) == 3 && parts[2] == "void":
			var req WorklogVoidRequest
			if !worklogDecode(raw, &req, 5) || !validWorklogVoid(req) {
				bad()
				return true
			}
			project = req.ProjectID
			if scope == "api" && project != boundProject {
				writeError(w, forbidden())
				return true
			}
			out, err = s.Service.VoidWorklog(r.Context(), actor, project, parts[1], req)
		default:
			writeError(w, missing())
			return true
		}
		respond(w, status, out, err)
		return true
	}
	if r.Method != http.MethodGet || len(parts) == 3 && parts[2] != "revisions" {
		writeError(w, missing())
		return true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		bad()
		return true
	}
	for key, values := range query {
		if len(values) != 1 || values[0] == "" {
			bad()
			return true
		}
		if key == "project_id" {
			continue
		}
		if key == "limit" || key == "cursor" {
			if len(parts) == 2 {
				bad()
				return true
			}
			continue
		}
		if len(parts) == 1 && (key == "task_id" || key == "member_id" || key == "from_date" || key == "to_date" || key == "status") {
			continue
		}
		bad()
		return true
	}
	project := query.Get("project_id")
	if !handoffID(project, false) {
		bad()
		return true
	}
	if scope == "api" && project != boundProject {
		writeError(w, forbidden())
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
	switch len(parts) {
	case 1:
		req := WorklogListRequest{TaskID: query.Get("task_id"), MemberID: query.Get("member_id"), FromDate: query.Get("from_date"), ToDate: query.Get("to_date"), Status: query.Get("status"), Limit: limit, Cursor: query.Get("cursor")}
		out, err := s.Service.ListWorklogs(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	case 2:
		out, err := s.Service.GetWorklog(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
	case 3:
		out, err := s.Service.WorklogRevisions(r.Context(), actor, project, parts[1], limit, query.Get("cursor"))
		respond(w, http.StatusOK, out, err)
	}
	return true
}
