package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/dag"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type ToolPermissionOpenRequest struct {
	ProjectID  string               `json:"project_id"`
	Assignment dag.Assignment       `json:"assignment"`
	Request    ac.PermissionRequest `json:"request"`
}
type ToolPermissionPollRequest struct {
	ProjectID    string         `json:"project_id"`
	Assignment   dag.Assignment `json:"assignment"`
	PermissionID string         `json:"permission_id"`
}
type PoolToolPermissionOpenRequest struct {
	PoolRunnerAssignmentRequest
	Request ac.PermissionRequest `json:"request"`
}
type PoolToolPermissionPollRequest struct {
	PoolRunnerAssignmentRequest
	PermissionID string `json:"permission_id"`
}

func toolPermissionDecodeHTTP(r *http.Request, out any, fields ...string) error {
	if r.URL.RawQuery != "" {
		return invalid(toolPermissionInvalid)
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if e != nil || len(raw) > 64<<10 {
		return invalid(toolPermissionInvalid)
	}
	var object map[string]json.RawMessage
	if !json.Valid(raw) || json.Unmarshal(raw, &object) != nil || len(object) != len(fields) {
		return invalid(toolPermissionInvalid)
	}
	for _, field := range fields {
		v, ok := object[field]
		if !ok || string(v) == "null" {
			return invalid(toolPermissionInvalid)
		}
	}
	if woc.Decode(raw, out) != nil {
		return invalid(toolPermissionInvalid)
	}
	return nil
}
func (s *Server) toolPermissionsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	path := strings.Join(parts, "/")
	machine := path == "runner/tool-permissions" || path == "runner/tool-permissions/poll"
	list := len(parts) == 5 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "tool-permissions"
	decide := len(parts) == 3 && parts[0] == "tool-permissions" && parts[2] == "decision"
	if !machine && !list && !decide {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(e error) bool { writeError(w, e); return true }
	if machine {
		if actor.kind != actorProjectRunner || scope != "runner" {
			return fail(forbidden())
		}
		if r.Method != http.MethodPost {
			return fail(invalid(toolPermissionInvalid))
		}
		if strings.HasSuffix(path, "/poll") {
			var q ToolPermissionPollRequest
			if toolPermissionDecodeHTTP(r, &q, "project_id", "assignment", "permission_id") != nil || boundProject != "" && q.ProjectID != boundProject {
				return fail(invalid(toolPermissionInvalid))
			}
			out, e := s.Service.PollToolPermission(r.Context(), actor, q.ProjectID, q.Assignment, q.PermissionID)
			respond(w, 200, out, e)
		} else {
			var q ToolPermissionOpenRequest
			if toolPermissionDecodeHTTP(r, &q, "project_id", "assignment", "request") != nil || boundProject != "" && q.ProjectID != boundProject {
				return fail(invalid(toolPermissionInvalid))
			}
			out, e := s.Service.OpenToolPermission(r.Context(), actor, q.ProjectID, q.Assignment, q.Request)
			respond(w, 200, out, e)
		}
		return true
	}
	if !actor.IsBrowser() || scope != "" {
		return fail(forbidden())
	}
	if list {
		if r.Method != http.MethodGet || len(r.URL.RawQuery) > 1024 {
			return fail(invalid(toolPermissionInvalid))
		}
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(q["project_id"]) != 1 || !handoffID(q.Get("project_id"), false) || boundProject != "" && boundProject != q.Get("project_id") {
			return fail(invalid(toolPermissionInvalid))
		}
		var locator []string
		for key, values := range q {
			if len(values) != 1 || values[0] == "" || key != "project_id" && key != "permission_id" {
				return fail(invalid(toolPermissionInvalid))
			}
			if key == "permission_id" {
				if !handoffID(values[0], false) || len(values[0]) > 64 {
					return fail(invalid(toolPermissionInvalid))
				}
				locator = []string{values[0]}
			}
		}
		out, e := s.Service.ListToolPermissions(r.Context(), actor, q.Get("project_id"), parts[1], parts[3], locator...)
		respond(w, 200, out, e)
		return true
	}
	if r.Method != http.MethodPost {
		return fail(invalid(toolPermissionInvalid))
	}
	var q ac.DecisionRequest
	if toolPermissionDecodeHTTP(r, &q, "project_id", "expected_version", "idempotency_key", "option_id") != nil || boundProject != "" && q.ProjectID != boundProject {
		return fail(invalid(toolPermissionInvalid))
	}
	out, e := s.Service.DecideToolPermission(r.Context(), actor, parts[1], q)
	respond(w, 200, out, e)
	return true
}
func (s *Server) poolToolPermissionsHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-permissions" && path != "pool-runner/tool-permissions/poll" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, invalid(toolPermissionInvalid))
		return true
	}
	if strings.HasSuffix(path, "/poll") {
		var q PoolToolPermissionPollRequest
		if e := toolPermissionDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "permission_id"); e != nil {
			writeError(w, e)
			return true
		}
		if validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			writeError(w, invalid(toolPermissionInvalid))
			return true
		}
		out, e := s.Service.PollPoolToolPermission(r.Context(), token, q.ProjectID, q.Assignment, q.PermissionID)
		respond(w, 200, out, e)
	} else {
		var q PoolToolPermissionOpenRequest
		if e := toolPermissionDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "request"); e != nil {
			writeError(w, e)
			return true
		}
		if validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			writeError(w, invalid(toolPermissionInvalid))
			return true
		}
		out, e := s.Service.OpenPoolToolPermission(r.Context(), token, q.ProjectID, q.Assignment, q.Request)
		respond(w, 200, out, e)
	}
	return true
}
