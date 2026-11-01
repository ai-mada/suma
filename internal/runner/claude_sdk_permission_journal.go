package runner

import (
	"context"
	"errors"
	"sort"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// Claude retains its own observation encoding. The shared delivery fields
// describe Go-to-host IO, not the SDK's private native control-response pipe.
type claudeToolPermissionObservation struct {
	OperationID  string                   `json:"operation_id"`
	PermissionID string                   `json:"permission_id"`
	Observation  cl.PermissionObservation `json:"observation"`
	Acked        bool                     `json:"acked"`
	Blocked      bool                     `json:"reconciliation_required,omitempty"`
}

type pendingClaudeToolPermissionObservation struct {
	Assignment Assignment
	Request    pc.Request
	claudeToolPermissionObservation
}

func validClaudeToolPermissionObservations(r toolPermissionCheckpoint) error {
	if r.Request.Claude == nil || r.Observation != nil || len(r.Observations) != 0 || len(r.ClaudeObservations) > 128 {
		return errToolPermission
	}
	var previous *cl.PermissionObservation
	keys := map[string]bool{}
	for _, event := range r.ClaudeObservations {
		o := event.Observation
		if !acpLocalText(event.OperationID, 128) || keys[event.OperationID] || event.PermissionID != r.PermissionID || event.Blocked && event.Acked || cl.ValidatePermissionObservation(o) != nil || o.ParamsSHA256 != r.Request.Claude.ParamsSHA256 || event.PermissionID == "" && (o.AnswerObserved || o.ReplyStarted || event.Acked) || previous != nil && !inputObservationAdvances(*previous, o) {
			return errToolPermission
		}
		if o.AnswerObserved {
			if r.Decision == nil || r.Decision.Claude == nil {
				return errToolPermission
			}
			sha, err := cl.PermissionDecisionDigest(*r.Request.Claude, *r.Decision.Claude)
			if err != nil || sha != o.AnswerSHA256 {
				return errToolPermission
			}
		}
		keys[event.OperationID] = true
		copy := o
		previous = &copy
	}
	if !samePermissionValue(previous, r.ClaudeObservation) {
		return errToolPermission
	}
	return nil
}

func (j *Journal) recordClaudeToolPermissionObservation(a Assignment, request pc.Request, id string, o cl.PermissionObservation, beginWrite bool) error {
	if request.Claude == nil || pc.ValidateRequest(request) != nil || cl.ValidatePermissionObservation(o) != nil || o.ParamsSHA256 != request.Claude.ParamsSHA256 {
		return errToolPermission
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolPermissions == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolPermission
	}
	r, ok := e.ToolPermissions.Requests[request.IdempotencyKey]
	if !ok || !samePermissionValue(r.Request, request) || r.PermissionID != id || id == "" && (o.AnswerObserved || o.ReplyStarted) || len(r.ClaudeObservations) >= 128 {
		return errToolPermission
	}
	if beginWrite && (!o.AnswerObserved || !o.ReplyStarted || o.ReplyWritten || o.ExecutionClosed || o.FailureCode != "" || r.ClaudeObservation != nil && r.ClaudeObservation.ReplyStarted) {
		return errToolPermission
	}
	if r.ClaudeObservation != nil {
		if o.Sequence == r.ClaudeObservation.Sequence && !beginWrite && samePermissionValue(*r.ClaudeObservation, o) {
			return nil
		}
		if !inputObservationAdvances(*r.ClaudeObservation, o) {
			return errToolPermission
		}
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	r = e.ToolPermissions.Requests[request.IdempotencyKey]
	copy := o
	r.ClaudeObservation = &copy
	r.ClaudeObservations = append(r.ClaudeObservations, claudeToolPermissionObservation{OperationID: operationID(), PermissionID: id, Observation: o})
	if validClaudeToolPermissionObservations(r) != nil {
		return errToolPermission
	}
	e.ToolPermissions.Requests[request.IdempotencyKey] = r
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}

func (j *Journal) pendingClaudeToolPermissionObservations() []pendingClaudeToolPermissionObservation {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := make([]string, 0, len(j.entries))
	for key := range j.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []pendingClaudeToolPermissionObservation
	for _, key := range keys {
		e := j.entries[key]
		if e.ToolPermissions == nil {
			continue
		}
		requestKeys := make([]string, 0, len(e.ToolPermissions.Requests))
		for k := range e.ToolPermissions.Requests {
			requestKeys = append(requestKeys, k)
		}
		sort.Strings(requestKeys)
		for _, k := range requestKeys {
			r := e.ToolPermissions.Requests[k]
			for _, event := range r.ClaudeObservations {
				if !event.Acked {
					out = append(out, pendingClaudeToolPermissionObservation{Assignment: e.Assignment, Request: r.Request, claudeToolPermissionObservation: event})
				}
			}
		}
	}
	return out
}

func (j *Journal) settleClaudeToolPermissionObservation(a Assignment, requestKey, operation string, blocked bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.ToolPermissions == nil {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	r, ok := e.ToolPermissions.Requests[requestKey]
	if !ok {
		return errToolPermission
	}
	for i := range r.ClaudeObservations {
		v := &r.ClaudeObservations[i]
		if v.OperationID != operation {
			continue
		}
		if blocked && v.Acked || !blocked && v.Blocked {
			return errToolPermission
		}
		v.Blocked, v.Acked = blocked, !blocked
		e.ToolPermissions.Requests[requestKey] = r
		j.entries[k] = e
		if err := j.save(); err != nil {
			j.entries[k] = before
			return err
		}
		return nil
	}
	return errToolPermission
}

func (c *Client) ObserveClaudeToolPermission(ctx context.Context, a Assignment, request pc.Request, id, operation string, o cl.PermissionObservation) (pc.Permission, error) {
	var result pc.Permission
	if request.Claude == nil || pc.ValidateRequest(request) != nil || cl.ValidatePermissionObservation(o) != nil || o.ParamsSHA256 != request.Claude.ParamsSHA256 || !acpLocalText(id, 64) || !acpLocalText(operation, 128) {
		return result, errToolPermission
	}
	err := c.post(ctx, "tool-permissions/observations", map[string]any{"project_id": c.ProjectID, "assignment": a, "permission_id": id, "idempotency_key": operation, "claude_observation": o}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if result.ID != id || !toolPermissionSource(a, profile, result, request, c.ProjectID) || result.ClaudeObservation == nil || !samePermissionValue(*result.ClaudeObservation, o) {
			return pc.Permission{}, errToolPermission
		}
	}
	return result, err
}

func replayClaudeToolPermissionObservations(ctx context.Context, c *Client, j *Journal) error {
	for _, event := range j.pendingClaudeToolPermissionObservations() {
		if event.Blocked {
			return errToolPermissionReconciliation
		}
		deliveryError := func(err error) error {
			var status *runnerHTTPStatusError
			if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429 {
				if saveErr := j.settleClaudeToolPermissionObservation(event.Assignment, event.Request.IdempotencyKey, event.OperationID, true); saveErr != nil {
					return saveErr
				}
				return errToolPermissionReconciliation
			}
			return err
		}
		if event.PermissionID == "" {
			cp, err := j.toolPermission(event.Assignment, event.Request)
			if err != nil {
				return err
			}
			event.PermissionID = cp.PermissionID
		}
		if event.PermissionID == "" {
			v, err := c.OpenToolPermission(ctx, event.Assignment, event.Request)
			if err != nil {
				return deliveryError(err)
			}
			if err = j.bindToolPermissionObservation(event.Assignment, event.Request, v); err != nil {
				return err
			}
			event.PermissionID = v.ID
		}
		if _, err := c.ObserveClaudeToolPermission(ctx, event.Assignment, event.Request, event.PermissionID, event.OperationID, event.Observation); err != nil {
			return deliveryError(err)
		}
		if err := j.settleClaudeToolPermissionObservation(event.Assignment, event.Request.IdempotencyKey, event.OperationID, false); err != nil {
			return err
		}
	}
	return nil
}
