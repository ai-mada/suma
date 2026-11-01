package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type ToolPermissionOpenRequest struct {
	ProjectID  string         `json:"project_id"`
	Assignment dag.Assignment `json:"assignment"`
	Request    pc.Request     `json:"request"`
}
type ToolPermissionPollRequest struct {
	ProjectID    string         `json:"project_id"`
	Assignment   dag.Assignment `json:"assignment"`
	PermissionID string         `json:"permission_id"`
}
type PoolToolPermissionOpenRequest struct {
	PoolRunnerAssignmentRequest
	Request pc.Request `json:"request"`
}
type PoolToolPermissionPollRequest struct {
	PoolRunnerAssignmentRequest
	PermissionID string `json:"permission_id"`
}

type ToolPermissionObservationRequest struct {
	ProjectID         string                       `json:"project_id"`
	Assignment        dag.Assignment               `json:"assignment"`
	PermissionID      string                       `json:"permission_id"`
	IdempotencyKey    string                       `json:"idempotency_key"`
	Observation       *cc.CodexApprovalObservation `json:"observation,omitempty"`
	ClaudeObservation *cl.PermissionObservation    `json:"claude_observation,omitempty"`
}

type PoolToolPermissionObservationRequest struct {
	PoolRunnerAssignmentRequest
	PermissionID      string                       `json:"permission_id"`
	IdempotencyKey    string                       `json:"idempotency_key"`
	Observation       *cc.CodexApprovalObservation `json:"observation,omitempty"`
	ClaudeObservation *cl.PermissionObservation    `json:"claude_observation,omitempty"`
}

func toolPermissionDecodeHTTP(r *http.Request, out any, fields ...string) error {
	if r.URL.RawQuery != "" {
		return invalid(toolPermissionInvalid)
	}
	// The transport includes the authoritative assignment and bounded native
	// attachments. The typed Request retains its independent 64 KiB limit.
	raw, e := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if e != nil || len(raw) > 1<<20 {
		return invalid(toolPermissionInvalid)
	}
	var object map[string]json.RawMessage
	if !json.Valid(raw) || json.Unmarshal(raw, &object) != nil {
		return invalid(toolPermissionInvalid)
	}
	var branches []string
	switch out.(type) {
	case *pc.DecisionRequest:
		branches = []string{"acp", "codex", "claude"}
	case *ToolPermissionObservationRequest, *PoolToolPermissionObservationRequest:
		branches = []string{"observation", "claude_observation"}
	}
	if branches != nil {
		selected := ""
		for _, branch := range branches {
			if _, present := object[branch]; present {
				if selected != "" {
					return invalid(toolPermissionInvalid)
				}
				selected = branch
			}
		}
		if selected == "" || len(object) != len(fields)+1 {
			return invalid(toolPermissionInvalid)
		}
		fields = append(fields, selected)
	} else if len(object) != len(fields) {
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
	machine := path == "runner/tool-permissions" || path == "runner/tool-permissions/poll" || path == "runner/tool-permissions/observations"
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
		if strings.HasSuffix(path, "/observations") {
			var q ToolPermissionObservationRequest
			if toolPermissionDecodeHTTP(r, &q, "project_id", "assignment", "permission_id", "idempotency_key") != nil || boundProject != "" && q.ProjectID != boundProject {
				return fail(invalid(toolPermissionInvalid))
			}
			var out pc.Permission
			var e error
			if q.ClaudeObservation != nil {
				out, e = s.Service.ObserveClaudeToolPermission(r.Context(), actor, q.ProjectID, q.Assignment, q.PermissionID, q.IdempotencyKey, *q.ClaudeObservation)
			} else {
				out, e = s.Service.ObserveToolPermission(r.Context(), actor, q.ProjectID, q.Assignment, q.PermissionID, q.IdempotencyKey, *q.Observation)
			}
			respond(w, 200, out, e)
		} else if strings.HasSuffix(path, "/poll") {
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
	var q pc.DecisionRequest
	if toolPermissionDecodeHTTP(r, &q, "project_id", "expected_version", "idempotency_key", "protocol") != nil || boundProject != "" && q.ProjectID != boundProject {
		return fail(invalid(toolPermissionInvalid))
	}
	out, e := s.Service.DecideToolPermission(r.Context(), actor, parts[1], q)
	respond(w, 200, out, e)
	return true
}
func (s *Server) poolToolPermissionsHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-permissions" && path != "pool-runner/tool-permissions/poll" && path != "pool-runner/tool-permissions/observations" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeError(w, invalid(toolPermissionInvalid))
		return true
	}
	if strings.HasSuffix(path, "/observations") {
		var q PoolToolPermissionObservationRequest
		if e := toolPermissionDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "permission_id", "idempotency_key"); e != nil {
			writeError(w, e)
			return true
		}
		if validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			writeError(w, invalid(toolPermissionInvalid))
			return true
		}
		var out pc.Permission
		var e error
		if q.ClaudeObservation != nil {
			out, e = s.Service.ObservePoolClaudeToolPermission(r.Context(), token, q.ProjectID, q.Assignment, q.PermissionID, q.IdempotencyKey, *q.ClaudeObservation)
		} else {
			out, e = s.Service.ObservePoolToolPermission(r.Context(), token, q.ProjectID, q.Assignment, q.PermissionID, q.IdempotencyKey, *q.Observation)
		}
		respond(w, 200, out, e)
	} else if strings.HasSuffix(path, "/poll") {
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
