package platform

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	"net/http"
	"net/url"
	"strings"
)

func steeringDecodeHTTP(r *http.Request, out any, fields ...string) error {
	if toolPermissionDecodeHTTP(r, out, fields...) != nil {
		return invalid(steeringInvalid)
	}
	return nil
}

func (s *Server) turnSteeringHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, boundProject string, parts []string) bool {
	path := strings.Join(parts, "/")
	machine := path == "runner/steering/sessions" || path == "runner/steering/poll" || path == "runner/steering/observations" || path == "runner/steering/close"
	context := len(parts) == 3 && parts[0] == "tasks" && parts[2] == "execution-steering"
	create := path == "task-execution-steering"
	if !machine && !context && !create {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(e error) bool { writeError(w, e); return true }
	if machine {
		if actor.kind != actorProjectRunner || scope != "runner" {
			return fail(forbidden())
		}
		if r.Method != http.MethodPost {
			return fail(invalid(steeringInvalid))
		}
		switch parts[2] {
		case "sessions":
			var q SteeringSessionOpenRequest
			if steeringDecodeHTTP(r, &q, "project_id", "assignment", "source", "idempotency_key") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(steeringInvalid))
			}
			out, e := s.Service.OpenSteeringSession(r.Context(), actor, q.ProjectID, q.Assignment, q.Source, q.IdempotencyKey)
			respond(w, 200, out, e)
		case "poll":
			var q SteeringPollRequest
			if steeringDecodeHTTP(r, &q, "project_id", "assignment", "session_id") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(steeringInvalid))
			}
			out, e := s.Service.PollSteering(r.Context(), actor, q.ProjectID, q.Assignment, q.SessionID)
			respond(w, 200, out, e)
		case "observations":
			var q SteeringObservationRequest
			if steeringDecodeHTTP(r, &q, "project_id", "assignment", "session_id", "instruction_id", "idempotency_key", "observation") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(steeringInvalid))
			}
			out, e := s.Service.ObserveSteering(r.Context(), actor, q.ProjectID, q.Assignment, q.SessionID, q.InstructionID, q.IdempotencyKey, q.Observation)
			respond(w, 200, out, e)
		case "close":
			var q SteeringCloseRequest
			if steeringDecodeHTTP(r, &q, "project_id", "assignment", "session_id", "idempotency_key") != nil || boundProject != "" && boundProject != q.ProjectID {
				return fail(invalid(steeringInvalid))
			}
			out, e := s.Service.CloseSteeringSession(r.Context(), actor, q.ProjectID, q.Assignment, q.SessionID, q.IdempotencyKey)
			respond(w, 200, out, e)
		}
		return true
	}
	if !actor.IsBrowser() || scope != "" {
		return fail(forbidden())
	}
	if context {
		if r.Method != http.MethodGet || !handoffID(parts[1], false) || len(r.URL.RawQuery) > 2048 {
			return fail(invalid(steeringInvalid))
		}
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil || len(q) != 2 || len(q["project_id"]) != 1 || len(q["message_id"]) != 1 {
			return fail(invalid(steeringInvalid))
		}
		out, e := s.Service.GetTaskSteeringContext(r.Context(), actor, q.Get("project_id"), parts[1], q.Get("message_id"))
		respond(w, 200, out, e)
		return true
	}
	if r.Method != http.MethodPost {
		return fail(invalid(steeringInvalid))
	}
	var q sc.SteeringCreateRequest
	if steeringDecodeHTTP(r, &q, "project_id", "task_id", "message_id", "expected_task_version", "session_id", "expected_session_version", "idempotency_key") != nil {
		return fail(invalid(steeringInvalid))
	}
	out, e := s.Service.CreateSteeringInstruction(r.Context(), actor, q)
	respond(w, 200, out, e)
	return true
}

func (s *Server) poolTurnSteeringHTTP(w http.ResponseWriter, r *http.Request, token, path string) bool {
	if path != "pool-runner/steering/sessions" && path != "pool-runner/steering/poll" && path != "pool-runner/steering/observations" && path != "pool-runner/steering/close" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	fail := func(e error) bool { writeError(w, e); return true }
	if r.Method != http.MethodPost {
		return fail(invalid(steeringInvalid))
	}
	switch strings.TrimPrefix(path, "pool-runner/steering/") {
	case "sessions":
		var q PoolSteeringSessionOpenRequest
		if steeringDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "source", "idempotency_key") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(steeringInvalid))
		}
		out, e := s.Service.OpenPoolSteeringSession(r.Context(), token, q.ProjectID, q.Assignment, q.Source, q.IdempotencyKey)
		respond(w, 200, out, e)
	case "poll":
		var q PoolSteeringPollRequest
		if steeringDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "session_id") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(steeringInvalid))
		}
		out, e := s.Service.PollPoolSteering(r.Context(), token, q.ProjectID, q.Assignment, q.SessionID)
		respond(w, 200, out, e)
	case "observations":
		var q PoolSteeringObservationRequest
		if steeringDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "session_id", "instruction_id", "idempotency_key", "observation") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(steeringInvalid))
		}
		out, e := s.Service.ObservePoolSteering(r.Context(), token, q.ProjectID, q.Assignment, q.SessionID, q.InstructionID, q.IdempotencyKey, q.Observation)
		respond(w, 200, out, e)
	case "close":
		var q PoolSteeringCloseRequest
		if steeringDecodeHTTP(r, &q, "runner_id", "tenant_id", "project_id", "assignment", "session_id", "idempotency_key") != nil || validatePoolEnvelope(q.PoolRunnerAssignmentRequest) != nil {
			return fail(invalid(steeringInvalid))
		}
		out, e := s.Service.ClosePoolSteeringSession(r.Context(), token, q.ProjectID, q.Assignment, q.SessionID, q.IdempotencyKey)
		respond(w, 200, out, e)
	}
	return true
}

type SteeringSessionOpenRequest struct {
	ProjectID      string                 `json:"project_id"`
	Assignment     dag.Assignment         `json:"assignment"`
	Source         cc.CodexSteeringSource `json:"source"`
	IdempotencyKey string                 `json:"idempotency_key"`
}
type SteeringPollRequest struct {
	ProjectID  string         `json:"project_id"`
	Assignment dag.Assignment `json:"assignment"`
	SessionID  string         `json:"session_id"`
}
type SteeringObservationRequest struct {
	ProjectID      string                      `json:"project_id"`
	Assignment     dag.Assignment              `json:"assignment"`
	SessionID      string                      `json:"session_id"`
	InstructionID  string                      `json:"instruction_id"`
	IdempotencyKey string                      `json:"idempotency_key"`
	Observation    cc.CodexSteeringObservation `json:"observation"`
}
type SteeringCloseRequest struct {
	ProjectID      string         `json:"project_id"`
	Assignment     dag.Assignment `json:"assignment"`
	SessionID      string         `json:"session_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type PoolSteeringSessionOpenRequest struct {
	PoolRunnerAssignmentRequest
	Source         cc.CodexSteeringSource `json:"source"`
	IdempotencyKey string                 `json:"idempotency_key"`
}
type PoolSteeringPollRequest struct {
	PoolRunnerAssignmentRequest
	SessionID string `json:"session_id"`
}
type PoolSteeringObservationRequest struct {
	PoolRunnerAssignmentRequest
	SessionID      string                      `json:"session_id"`
	InstructionID  string                      `json:"instruction_id"`
	IdempotencyKey string                      `json:"idempotency_key"`
	Observation    cc.CodexSteeringObservation `json:"observation"`
}
type PoolSteeringCloseRequest struct {
	PoolRunnerAssignmentRequest
	SessionID      string `json:"session_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
