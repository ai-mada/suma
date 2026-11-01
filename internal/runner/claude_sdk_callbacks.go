package runner

import (
	"encoding/json"
	"fmt"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func (d *claudeSDKDriver) callbackKey(source any) string {
	raw, _ := json.Marshal(struct {
		Assignment string `json:"assignment"`
		Source     any    `json:"source"`
	}{key(d.a), source})
	return "claude-sdk-" + digest(raw)
}

func (d *claudeSDKDriver) observeCallback(r *claudeSDKCallback, o delivery.Observation, begin bool) error {
	var err error
	if r.permission != nil {
		err = d.cfg.Journal.recordClaudeToolPermissionObservation(d.a, r.permissionRequest, r.id, o, begin)
	} else {
		err = d.cfg.Journal.recordToolInputObservation(d.a, r.inputRequest, r.id, o, begin)
	}
	if err == nil {
		d.mu.Lock()
		r.observation = o
		d.mu.Unlock()
	}
	return err
}

func (d *claudeSDKDriver) callbackError(r *claudeSDKCallback, err error) {
	d.mu.Lock()
	o := r.observation
	d.mu.Unlock()
	if o.Sequence == 0 {
		var sha string
		if r.permission != nil {
			sha = r.permission.ParamsSHA256
		} else {
			sha = r.input.ParamsSHA256
		}
		o = delivery.Observation{ParamsSHA256: sha}
	}
	if o.FailureCode == "" {
		o.Sequence++
		o.FailureCode = "delivery_failed"
		if saveErr := d.observeCallback(r, o, false); saveErr != nil {
			err = saveErr
		}
	}
	d.mu.Lock()
	d.failLocked("original callback requires reconciliation")
	d.mu.Unlock()
	_ = err // Details may contain host paths or credentials; public text is fixed.
}

func (d *claudeSDKDriver) handlePermission(r *claudeSDKCallback) {
	if d.ctx.Err() != nil {
		return
	}
	r.permissionRequest = pc.Request{IdempotencyKey: d.callbackKey(r.permission), Protocol: cl.Protocol, Claude: r.permission}
	if _, err := safeToolPermission(r.permissionRequest, d.cfg.Secrets); err != nil {
		d.callbackError(r, err)
		return
	}
	if err := d.cfg.Journal.recordToolPermission(d.a, r.permissionRequest); err != nil {
		d.callbackError(r, err)
		return
	}
	decision, err := d.cfg.ToolPermission(d.ctx, r.permissionRequest)
	if err != nil || decision.Claude == nil || !pc.DecisionMatchesRequest(r.permissionRequest, decision) {
		d.callbackError(r, err)
		return
	}
	cp, err := d.cfg.Journal.toolPermission(d.a, r.permissionRequest)
	if err != nil || cp.PermissionID == "" || cp.Decision == nil || !samePermissionValue(cp.Decision, &decision) {
		d.callbackError(r, errToolPermission)
		return
	}
	sha, err := cl.PermissionDecisionDigest(*r.permission, *decision.Claude)
	if err != nil {
		d.callbackError(r, err)
		return
	}
	d.mu.Lock()
	r.id = cp.PermissionID
	choice := *decision.Claude
	r.choice = &choice
	if choice.Outcome != "allow_once" {
		d.blockedResult = true
	}
	d.mu.Unlock()
	o := delivery.Observation{Sequence: 1, ParamsSHA256: r.permission.ParamsSHA256, AnswerSHA256: sha, AnswerObserved: true}
	if err = d.observeCallback(r, o, false); err != nil {
		d.callbackError(r, err)
		return
	}
	reply := map[string]any{"v": 1, "type": "permission_reply", "request_id": d.requestID, "source": r.permission, "decision": choice}
	if err = d.queue.enqueueReply(reply, func() error { return d.beforeCallbackWrite(r) }, func(err error) { d.afterCallbackWrite(r, err) }); err != nil {
		d.callbackError(r, err)
	}
}

func (d *claudeSDKDriver) handleInput(r *claudeSDKCallback) {
	if d.ctx.Err() != nil {
		return
	}
	r.inputRequest = ic.ToolInputRequest{IdempotencyKey: d.callbackKey(r.input), Protocol: cl.Protocol, Claude: r.input}
	cfg := d.cfg
	cfg.ToolInputProfile = d.profile.ProfileSHA256
	h := newToolInputHandler(d.a, cfg)
	id, err := h.Open(d.ctx, r.inputRequest)
	if err != nil {
		d.callbackError(r, err)
		return
	}
	d.mu.Lock()
	r.id = id
	d.mu.Unlock()
	answer, err := h.Answer(d.ctx, r.inputRequest, id)
	if err != nil || answer == nil || answer.Claude == nil || ic.ValidateAnswer(r.inputRequest, *answer) != nil {
		d.callbackError(r, errToolInput)
		return
	}
	d.mu.Lock()
	copy := *answer
	r.answer = &copy
	if answer.Claude.Outcome != "answered" {
		d.blockedResult = true
	}
	d.mu.Unlock()
	o := delivery.Observation{Sequence: 1, ParamsSHA256: r.input.ParamsSHA256, AnswerSHA256: answer.AnswerSHA256, AnswerObserved: true}
	if err = d.observeCallback(r, o, false); err != nil {
		d.callbackError(r, err)
		return
	}
	reply := map[string]any{"v": 1, "type": "input_reply", "request_id": d.requestID, "source": r.input, "answer": answer.Claude}
	if err = d.queue.enqueueReply(reply, func() error { return d.beforeCallbackWrite(r) }, func(err error) { d.afterCallbackWrite(r, err) }); err != nil {
		d.callbackError(r, err)
	}
}

// The only stdin writer re-reads current authority immediately before saving
// its write intent. A terminal observed during that IO invalidates the send;
// the already-saved intent remains unknown rather than being silently replayed.
func (d *claudeSDKDriver) beforeCallbackWrite(r *claudeSDKCallback) error {
	d.mu.Lock()
	if d.bad != nil || d.closed || d.receivedTerminal || d.ctx.Err() != nil || r.observation.ReplyStarted {
		d.mu.Unlock()
		return fmt.Errorf("Claude SDK callback closed before write")
	}
	o := r.observation
	o.Sequence++
	o.ReplyStarted = true
	d.mu.Unlock()
	if r.permission != nil {
		fresh, err := d.cfg.ToolPermissionPoll(d.ctx, r.id)
		profile, _ := d.a.Config["tool_profile_sha256"].(string)
		if err != nil || fresh.State != "decided" || fresh.Decision == nil || fresh.Decision.Claude == nil || !samePermissionValue(fresh.Decision.Claude, r.choice) || !toolPermissionSource(d.a, profile, fresh, r.permissionRequest, fresh.ProjectID) || fresh.ClaudeObservation != nil && (fresh.ClaudeObservation.ReplyStarted || fresh.ClaudeObservation.ExecutionClosed) {
			return errToolPermission
		}
		if err = d.cfg.Journal.observeToolPermission(d.a, fresh); err != nil {
			return err
		}
		if err = d.observeCallback(r, o, true); err != nil {
			return err
		}
	} else {
		cfg := d.cfg
		cfg.ToolInputProfile = d.profile.ProfileSHA256
		if r.answer == nil {
			return errToolInput
		}
		if err := newToolInputHandler(d.a, cfg).BeforeWrite(d.ctx, r.inputRequest, r.id, *r.answer, o); err != nil {
			return err
		}
		d.mu.Lock()
		r.observation = o
		d.mu.Unlock()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bad != nil || d.closed || d.receivedTerminal || d.ctx.Err() != nil {
		return fmt.Errorf("Claude SDK callback closed while saving write intent")
	}
	return nil
}

func (d *claudeSDKDriver) afterCallbackWrite(r *claudeSDKCallback, writeErr error) {
	d.mu.Lock()
	o := r.observation
	d.mu.Unlock()
	if writeErr != nil {
		d.callbackError(r, writeErr)
		return
	}
	if !o.ReplyStarted {
		d.callbackError(r, errToolInput)
		return
	}
	o.Sequence++
	o.ReplyWritten = true
	if err := d.observeCallback(r, o, false); err != nil {
		d.callbackError(r, err)
	}
}

func (d *claudeSDKDriver) closeCallback(r *claudeSDKCallback) {
	d.mu.Lock()
	o := r.observation
	d.mu.Unlock()
	if o.ExecutionClosed {
		return
	}
	if o.Sequence == 0 {
		if r.permission != nil {
			o.ParamsSHA256 = r.permission.ParamsSHA256
		} else {
			o.ParamsSHA256 = r.input.ParamsSHA256
		}
	}
	o.Sequence++
	o.ExecutionClosed = true
	if err := d.observeCallback(r, o, false); err != nil {
		d.mu.Lock()
		d.failLocked("callback closure could not be persisted")
		d.mu.Unlock()
	}
}

func claudeSDKExecutionRequest(a Assignment, spec CommandSpec, p tc.Profile) error {
	fixed, err := tc.DecodeClaudeSDKNodeConfiguration(a.Config)
	if err != nil || (fixed != nil) != (spec.ClaudeSDK != nil) {
		return fmt.Errorf("Claude SDK requires its exact frozen node configuration")
	}
	if spec.ClaudeSDK == nil {
		return nil
	}
	if a.Type != "agent" || fixed == nil || p.ClaudeSDKExecution == nil || !samePermissionValue(fixed, p.ClaudeSDKExecution) {
		return fmt.Errorf("Claude SDK assignment differs from its current profile")
	}
	current, err := localClaudeSDKCapability(spec)
	if err != nil || !samePermissionValue(current, fixed) {
		return fmt.Errorf("Claude SDK local source changed")
	}
	return nil
}
