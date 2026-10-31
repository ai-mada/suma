package runner

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// Each native observation has its own immutable delivery key. Retrying HTTP
// recovers this receipt; it never writes another native approval response.
type toolPermissionObservation struct {
	OperationID            string                      `json:"operation_id"`
	PermissionID           string                      `json:"permission_id"`
	Observation            cc.CodexApprovalObservation `json:"observation"`
	Acked                  bool                        `json:"acked"`
	ReconciliationRequired bool                        `json:"reconciliation_required,omitempty"`
	FailureCode            string                      `json:"failure_code,omitempty"`
}
type pendingToolPermissionObservation struct {
	Assignment Assignment
	Request    pc.Request
	toolPermissionObservation
}

func observationAdvances(old, next cc.CodexApprovalObservation) bool {
	if cc.ValidateApprovalObservation(next) != nil || next.Sequence <= old.Sequence || next.ParamsSHA256 != old.ParamsSHA256 || old.DecisionObserved && !next.DecisionObserved || old.Decision != "" && old.Decision != next.Decision || old.ReplyStarted && !next.ReplyStarted || old.ReplyWritten && !next.ReplyWritten || old.RequestResolved && !next.RequestResolved || old.FailureCode != "" && old.FailureCode != next.FailureCode {
		return false
	}
	return old.ItemStatus == "" || old.ItemStatus == "inProgress" && next.ItemStatus != "" || old.ItemStatus == next.ItemStatus
}
func validToolPermissionObservations(r toolPermissionCheckpoint) error {
	if r.Request.Codex == nil {
		if r.Observation != nil || len(r.Observations) != 0 {
			return errToolPermission
		}
		return nil
	}
	if len(r.Observations) > 128 {
		return errToolPermission
	}
	var previous *cc.CodexApprovalObservation
	keys := map[string]bool{}
	for _, event := range r.Observations {
		o := event.Observation
		if event.ReconciliationRequired != (event.FailureCode == "source_reconciliation_required") || event.FailureCode != "" && !event.ReconciliationRequired || event.ReconciliationRequired && event.Acked {
			return errToolPermission
		}
		if !acpLocalText(event.OperationID, 128) || keys[event.OperationID] || event.PermissionID != r.PermissionID || event.PermissionID == "" && (o.DecisionObserved || o.ReplyStarted || event.Acked) || cc.ValidateApprovalObservation(o) != nil || o.ParamsSHA256 != r.Request.Codex.ParamsSHA256 || previous != nil && !observationAdvances(*previous, o) || o.DecisionObserved && (r.Decision == nil || r.Decision.Codex == nil || r.Decision.Codex.Decision != o.Decision) {
			return errToolPermission
		}
		keys[event.OperationID] = true
		copy := o
		previous = &copy
	}
	if !samePermissionValue(previous, r.Observation) {
		return errToolPermission
	}
	return nil
}
func (j *Journal) recordToolPermissionObservation(a Assignment, request pc.Request, id string, observation cc.CodexApprovalObservation, beginWrite bool) error {
	if request.Codex == nil || cc.ValidateApprovalObservation(observation) != nil || observation.ParamsSHA256 != request.Codex.ParamsSHA256 {
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
	if !ok || !samePermissionValue(r.Request, request) || id != r.PermissionID || id == "" && (observation.DecisionObserved || observation.ReplyStarted) {
		return errToolPermission
	}
	if beginWrite && (!observation.DecisionObserved || !observation.ReplyStarted || observation.ReplyWritten || observation.RequestResolved || observation.ItemStatus != "" && observation.ItemStatus != "inProgress" || r.Observation != nil && r.Observation.ReplyStarted) {
		return errToolPermission
	}
	if r.Observation != nil {
		if r.Observation.Sequence == observation.Sequence {
			if !beginWrite && samePermissionValue(*r.Observation, observation) {
				return nil
			}
			return errToolPermission
		}
		if !observationAdvances(*r.Observation, observation) {
			return errToolPermission
		}
	}
	if observation.DecisionObserved && (r.Decision == nil || r.Decision.Codex == nil || r.Decision.Codex.Decision != observation.Decision) {
		return errToolPermission
	}
	if len(r.Observations) >= 128 {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	r = e.ToolPermissions.Requests[request.IdempotencyKey]
	copy := observation
	r.Observation = &copy
	r.Observations = append(r.Observations, toolPermissionObservation{OperationID: operationID(), PermissionID: id, Observation: observation})
	e.ToolPermissions.Requests[request.IdempotencyKey] = r
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) pendingToolPermissionObservations() []pendingToolPermissionObservation {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []pendingToolPermissionObservation{}
	keys := make([]string, 0, len(j.entries))
	for k := range j.entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		e := j.entries[k]
		if e.ToolPermissions == nil {
			continue
		}
		cp := cloneToolPermissionCheckpoint(e.ToolPermissions)
		requestKeys := make([]string, 0, len(cp.Requests))
		for rk := range cp.Requests {
			requestKeys = append(requestKeys, rk)
		}
		sort.Strings(requestKeys)
		for _, rk := range requestKeys {
			r := cp.Requests[rk]
			for _, event := range r.Observations {
				if !event.Acked {
					out = append(out, pendingToolPermissionObservation{Assignment: e.Assignment, Request: r.Request, toolPermissionObservation: event})
				}
			}
		}
	}
	return out
}
func (j *Journal) ackToolPermissionObservation(a Assignment, requestKey, operation string) error {
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
	for i := range r.Observations {
		if r.Observations[i].OperationID == operation {
			r.Observations[i].Acked = true
			e.ToolPermissions.Requests[requestKey] = r
			j.entries[k] = e
			if err := j.save(); err != nil {
				j.entries[k] = before
				return err
			}
			return nil
		}
	}
	return errToolPermission
}
func (c *Client) ObserveToolPermission(ctx context.Context, a Assignment, request pc.Request, id, operation string, observation cc.CodexApprovalObservation) (pc.Permission, error) {
	var result pc.Permission
	if request.Codex == nil || pc.ValidateRequest(request) != nil || cc.ValidateApprovalObservation(observation) != nil || observation.ParamsSHA256 != request.Codex.ParamsSHA256 || !acpLocalText(id, 64) || !acpLocalText(operation, 128) {
		return result, errToolPermission
	}
	err := c.post(ctx, "tool-permissions/observations", map[string]any{"project_id": c.ProjectID, "assignment": a, "permission_id": id, "idempotency_key": operation, "observation": observation}, &result)
	if err == nil {
		profile, _ := a.Config["tool_profile_sha256"].(string)
		if result.ID != id || !toolPermissionSource(a, profile, result, request, c.ProjectID) || result.Observation == nil || !samePermissionValue(*result.Observation, observation) {
			return pc.Permission{}, errToolPermission
		}
	}
	return result, err
}
func ReplayToolPermissionObservations(ctx context.Context, c *Client, j *Journal) error {
	for _, event := range j.pendingToolPermissionObservations() {
		if event.ReconciliationRequired {
			return errToolPermissionReconciliation
		}
		deliveryError := func(err error) error {
			var status *runnerHTTPStatusError
			if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429 {
				if saveErr := j.blockToolPermissionObservation(event); saveErr != nil {
					return saveErr
				}
				return errToolPermissionReconciliation
			}
			return err
		}
		if event.PermissionID == "" {
			checkpoint, err := j.toolPermission(event.Assignment, event.Request)
			if err != nil {
				return err
			}
			event.PermissionID = checkpoint.PermissionID
		}
		if event.PermissionID == "" {
			// Only recover the original registration operation. A late human
			// decision here cannot reconstruct a native response/send permit.
			permission, err := c.OpenToolPermission(ctx, event.Assignment, event.Request)
			if err != nil {
				return deliveryError(err)
			}
			if err = j.bindToolPermissionObservation(event.Assignment, event.Request, permission); err != nil {
				return err
			}
			event.PermissionID = permission.ID
		}
		if _, err := c.ObserveToolPermission(ctx, event.Assignment, event.Request, event.PermissionID, event.OperationID, event.Observation); err != nil {
			return deliveryError(err)
		}
		if err := j.ackToolPermissionObservation(event.Assignment, event.Request.IdempotencyKey, event.OperationID); err != nil {
			return err
		}
	}
	return nil
}

func (j *Journal) blockToolPermissionObservation(event pendingToolPermissionObservation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(event.Assignment)
	e, ok := j.entries[k]
	if !ok || e.ToolPermissions == nil {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	r, ok := e.ToolPermissions.Requests[event.Request.IdempotencyKey]
	if !ok {
		return errToolPermission
	}
	for i := range r.Observations {
		if r.Observations[i].OperationID == event.OperationID {
			if r.Observations[i].Acked {
				return errToolPermission
			}
			r.Observations[i].ReconciliationRequired = true
			r.Observations[i].FailureCode = "source_reconciliation_required"
			e.ToolPermissions.Requests[event.Request.IdempotencyKey] = r
			j.entries[k] = e
			if err := j.save(); err != nil {
				j.entries[k] = before
				return err
			}
			return nil
		}
	}
	return errToolPermission
}

func (j *Journal) bindToolPermissionObservation(a Assignment, request pc.Request, permission pc.Permission) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.ToolPermissions == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolPermission
	}
	r, ok := e.ToolPermissions.Requests[request.IdempotencyKey]
	if !ok || !toolPermissionSource(a, e.ToolPermissions.Profile.ProfileSHA256, permission, r.Request, permission.ProjectID) || r.PermissionID != "" && r.PermissionID != permission.ID {
		return errToolPermission
	}
	before := e
	e.ToolPermissions = cloneToolPermissionCheckpoint(e.ToolPermissions)
	r = e.ToolPermissions.Requests[request.IdempotencyKey]
	r.PermissionID = permission.ID
	for i := range r.Observations {
		if r.Observations[i].PermissionID == "" {
			r.Observations[i].PermissionID = permission.ID
		}
	}
	e.ToolPermissions.Requests[request.IdempotencyKey] = r
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}

// This closure owns durable transport only. The driver owns native correlation,
// generation/cancellation checks and the single stdin writer.
func codexPermissionHandler(a Assignment, cfg ExecConfig) codexApprovalHandler {
	requestFor := func(native cc.CodexApprovalRequest) pc.Request {
		body, _ := json.Marshal(struct {
			Assignment string
			Request    cc.CodexApprovalRequest
		}{key(a), native})
		return pc.Request{IdempotencyKey: "codex-" + digest(body), Protocol: cc.Protocol, Codex: &native}
	}
	return codexApprovalHandler{
		Decide: func(ctx context.Context, native cc.CodexApprovalRequest) (codexApprovalDecision, error) {
			r := requestFor(native)
			if cfg.Journal == nil || cfg.ToolPermission == nil {
				return codexApprovalDecision{}, errToolPermission
			}
			if _, err := safeToolPermission(r, cfg.Secrets); err != nil {
				return codexApprovalDecision{}, err
			}
			if err := cfg.Journal.recordToolPermission(a, r); err != nil {
				return codexApprovalDecision{}, err
			}
			d, err := cfg.ToolPermission(ctx, r)
			if err != nil || !pc.DecisionMatchesRequest(r, d) || d.Codex == nil {
				return codexApprovalDecision{}, errToolPermission
			}
			cp, err := cfg.Journal.toolPermission(a, r)
			if err != nil || cp.PermissionID == "" || cp.Decision == nil || !samePermissionValue(cp.Decision, &d) {
				return codexApprovalDecision{}, errToolPermission
			}
			return codexApprovalDecision{PermissionID: cp.PermissionID, Choice: *d.Codex}, nil
		},
		BeforeWrite: func(ctx context.Context, native cc.CodexApprovalRequest, decision codexApprovalDecision, observation cc.CodexApprovalObservation) error {
			r := requestFor(native)
			if cfg.Journal == nil || cfg.ToolPermissionPoll == nil {
				return errToolPermission
			}
			fresh, err := cfg.ToolPermissionPoll(ctx, decision.PermissionID)
			profile, _ := a.Config["tool_profile_sha256"].(string)
			if err != nil || fresh.ID != decision.PermissionID || !toolPermissionSource(a, profile, fresh, r, fresh.ProjectID) || fresh.State != "decided" || fresh.Decision == nil || fresh.Decision.Codex == nil || !samePermissionValue(*fresh.Decision.Codex, decision.Choice) {
				return errToolPermission
			}
			if err = cfg.Journal.observeToolPermission(a, fresh); err != nil {
				return err
			}
			return cfg.Journal.recordToolPermissionObservation(a, r, decision.PermissionID, observation, true)
		},
		Observe: func(ctx context.Context, native cc.CodexApprovalRequest, decision codexApprovalDecision, observation cc.CodexApprovalObservation) error {
			if cfg.Journal == nil {
				return errToolPermission
			}
			r := requestFor(native)
			if _, err := safeToolPermission(r, cfg.Secrets); err != nil {
				return err
			}
			if err := cfg.Journal.recordToolPermission(a, r); err != nil {
				return err
			}
			cp, err := cfg.Journal.toolPermission(a, r)
			if err != nil {
				return err
			}
			id := decision.PermissionID
			if id == "" {
				id = cp.PermissionID
			}
			return cfg.Journal.recordToolPermissionObservation(a, r, id, observation, false)
		},
	}
}
