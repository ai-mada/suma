package platform

import (
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func toolInputDecodeHTTP(r *http.Request, out any, fields ...string) error {
	if _, answer := out.(*ic.ToolInputAnswerRequest); answer {
		if r.URL.RawQuery != "" {
			return invalid(toolInputInvalid)
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		var object map[string]json.RawMessage
		if err != nil || len(raw) > 1<<20 || json.Unmarshal(raw, &object) != nil {
			return invalid(toolInputInvalid)
		}
		selected := ""
		for _, branch := range []string{"answers", "cursor", "claude"} {
			if _, present := object[branch]; present {
				if selected != "" {
					return invalid(toolInputInvalid)
				}
				selected = branch
			}
		}
		if len(object) != len(fields)+1 || selected == "" {
			return invalid(toolInputInvalid)
		}
		for _, field := range append(fields, selected) {
			if value, exists := object[field]; !exists || string(value) == "null" {
				return invalid(toolInputInvalid)
			}
		}
		if woc.Decode(raw, out) != nil {
			return invalid(toolInputInvalid)
		}
		return nil
	}
	if toolPermissionDecodeHTTP(r, out, fields...) != nil {
		return invalid(toolInputInvalid)
	}
	return nil
}

func (s *Server) toolInputsHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	path := strings.Join(parts, "/")
	machine := path == "runner/tool-inputs" || path == "runner/tool-inputs/poll" || path == "runner/tool-inputs/observations"
	list := len(parts) == 5 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "tool-inputs"
	answer := len(parts) == 3 && parts[0] == "tool-inputs" && parts[2] == "answer"
	if !machine && !list && !answer {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(e error) bool { writeError(w, e); return true }
	if machine {
		if actor.kind != actorProjectRunner || scope != "runner" {
			return fail(forbidden())
		}
		if r.Method != http.MethodPost {
			return fail(invalid(toolInputInvalid))
		}
		if strings.HasSuffix(path, "/observations") {
			var q ToolInputObservationRequest
			if toolInputDecodeHTTP(r, &q, "project_id", "assignment", "input_id", "idempotency_key", "observation") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(toolInputInvalid))
			}
			out, err := s.Service.ObserveToolInput(r.Context(), actor, q.ProjectID, q.Assignment, q.InputID, q.IdempotencyKey, q.Observation)
			respond(w, 200, out, err)
		} else if strings.HasSuffix(path, "/poll") {
			var q ToolInputPollRequest
			if toolInputDecodeHTTP(r, &q, "project_id", "assignment", "input_id") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(toolInputInvalid))
			}
			out, err := s.Service.PollToolInput(r.Context(), actor, q.ProjectID, q.Assignment, q.InputID)
			respond(w, 200, out, err)
		} else {
			var q ToolInputOpenRequest
			if toolInputDecodeHTTP(r, &q, "project_id", "assignment", "request") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(toolInputInvalid))
			}
			out, err := s.Service.OpenToolInput(r.Context(), actor, q.ProjectID, q.Assignment, q.Request)
			respond(w, 200, out, err)
		}
		return true
	}
	if !actor.IsBrowser() || scope != "" {
		return fail(forbidden())
	}
	if list {
		if r.Method != http.MethodGet || len(r.URL.RawQuery) > 2048 {
			return fail(invalid(toolInputInvalid))
		}
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(q) != 3 {
			return fail(invalid(toolInputInvalid))
		}
		for _, key := range []string{"project_id", "attempt", "fence"} {
			if len(q[key]) != 1 {
				return fail(invalid(toolInputInvalid))
			}
		}
		attempt, e := strconv.Atoi(q.Get("attempt"))
		if e != nil {
			return fail(invalid(toolInputInvalid))
		}
		fence, e := strconv.ParseInt(q.Get("fence"), 10, 64)
		if e != nil {
			return fail(invalid(toolInputInvalid))
		}
		out, err := s.Service.ListToolInputs(r.Context(), actor, q.Get("project_id"), parts[1], parts[3], attempt, fence)
		respond(w, 200, out, err)
		return true
	}
	if r.Method != http.MethodPost || !handoffID(parts[1], false) {
		return fail(invalid(toolInputInvalid))
	}
	var q ic.ToolInputAnswerRequest
	if toolInputDecodeHTTP(r, &q, "project_id", "expected_version", "idempotency_key") != nil {
		return fail(invalid(toolInputInvalid))
	}
	out, err := s.Service.AnswerToolInput(r.Context(), actor, parts[1], q)
	respond(w, 200, out, err)
	return true
}

func (s *Server) poolToolInputsHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/tool-inputs" && path != "pool-runner/tool-inputs/poll" && path != "pool-runner/tool-inputs/observations" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(e error) bool { writeError(w, e); return true }
	if r.Method != http.MethodPost {
		return fail(invalid(toolInputInvalid))
	}
	if strings.HasSuffix(path, "/observations") {
		var q PoolToolInputObservationRequest
		if toolInputDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "input_id", "idempotency_key", "observation") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(toolInputInvalid))
		}
		out, err := s.Service.ObservePoolToolInput(r.Context(), token, q.ProjectID, q.Assignment, q.InputID, q.IdempotencyKey, q.Observation)
		respond(w, 200, out, err)
	} else if strings.HasSuffix(path, "/poll") {
		var q PoolToolInputPollRequest
		if toolInputDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "input_id") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(toolInputInvalid))
		}
		out, err := s.Service.PollPoolToolInput(r.Context(), token, q.ProjectID, q.Assignment, q.InputID)
		respond(w, 200, out, err)
	} else {
		var q PoolToolInputOpenRequest
		if toolInputDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "request") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(toolInputInvalid))
		}
		out, err := s.Service.OpenPoolToolInput(r.Context(), token, q.ProjectID, q.Assignment, q.Request)
		respond(w, 200, out, err)
	}
	return true
}

type ToolInputOpenRequest struct {
	ProjectID  string              `json:"project_id"`
	Assignment dag.Assignment      `json:"assignment"`
	Request    ic.ToolInputRequest `json:"request"`
}
type ToolInputPollRequest struct {
	ProjectID  string         `json:"project_id"`
	Assignment dag.Assignment `json:"assignment"`
	InputID    string         `json:"input_id"`
}
type ToolInputObservationRequest struct {
	ProjectID      string               `json:"project_id"`
	Assignment     dag.Assignment       `json:"assignment"`
	InputID        string               `json:"input_id"`
	IdempotencyKey string               `json:"idempotency_key"`
	Observation    delivery.Observation `json:"observation"`
}
type PoolToolInputOpenRequest struct {
	PoolRunnerAssignmentRequest
	Request ic.ToolInputRequest `json:"request"`
}
type PoolToolInputPollRequest struct {
	PoolRunnerAssignmentRequest
	InputID string `json:"input_id"`
}
type PoolToolInputObservationRequest struct {
	PoolRunnerAssignmentRequest
	InputID        string               `json:"input_id"`
	IdempotencyKey string               `json:"idempotency_key"`
	Observation    delivery.Observation `json:"observation"`
}
