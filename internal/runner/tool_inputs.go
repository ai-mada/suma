package runner

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

var errToolInput = errors.New("tool input requires its original current source and answer")
var errToolInputReconciliation = errors.New("tool input source requires reconciliation; original journal retained")

type toolInputEvent struct {
	OperationID string                       `json:"operation_id"`
	InputID     string                       `json:"input_id,omitempty"`
	Observation cc.CodexUserInputObservation `json:"observation"`
	Acked       bool                         `json:"acked"`
	Blocked     bool                         `json:"reconciliation_required,omitempty"`
}
type toolInputCheckpoint struct {
	Request      ic.ToolInputRequest           `json:"request"`
	InputID      string                        `json:"input_id,omitempty"`
	Version      int64                         `json:"version"`
	Answer       *ic.ToolInputAnswer           `json:"answer,omitempty"`
	Observation  *cc.CodexUserInputObservation `json:"observation,omitempty"`
	Observations []toolInputEvent              `json:"observations,omitempty"`
}
type toolInputExecutionCheckpoint struct {
	Profile  tc.Profile                     `json:"profile"`
	Requests map[string]toolInputCheckpoint `json:"requests"`
}
type pendingToolInputEvent struct {
	Assignment Assignment
	Request    ic.ToolInputRequest
	toolInputEvent
}

func cloneToolInputCheckpoint(v *toolInputExecutionCheckpoint) *toolInputExecutionCheckpoint {
	if v == nil {
		return nil
	}
	raw, _ := json.Marshal(v)
	var copy toolInputExecutionCheckpoint
	_ = json.Unmarshal(raw, &copy)
	return &copy
}
func inputObservationAdvances(old, next cc.CodexUserInputObservation) bool {
	return cc.ValidateUserInputObservation(next) == nil && next.Sequence > old.Sequence && old.ParamsSHA256 == next.ParamsSHA256 && (!old.AnswerObserved || next.AnswerObserved && old.AnswerSHA256 == next.AnswerSHA256) && (!old.ReplyStarted || next.ReplyStarted) && (!old.ReplyWritten || next.ReplyWritten) && (!old.RequestResolved || next.RequestResolved) && (!old.TurnClosed || next.TurnClosed) && (old.FailureCode == "" || old.FailureCode == next.FailureCode) && (!(old.RequestResolved || old.TurnClosed) || old.ReplyStarted || !next.ReplyStarted)
}
func validToolInputCheckpoint(a Assignment, cp *toolInputExecutionCheckpoint) error {
	if cp == nil {
		return nil
	}
	p := cp.Profile
	b, err := cc.DecodeBinding(a.Config["codex_execution"])
	if err != nil || a.Type != "agent" || a.WorkBuddyCloud != nil || len(a.Command) != 1 || p.CommandID != a.Command[0] || p.Adapter != "codex" || p.Protocol != cc.Protocol || p.UserInputProtocol != cc.UserInputProtocol || tc.ValidateProfile(p, true) != nil || a.Config["tool_profile_sha256"] != p.ProfileSHA256 || cp.Requests == nil || len(cp.Requests) > 32 {
		return errToolInput
	}
	for key, r := range cp.Requests {
		if key != r.Request.IdempotencyKey || ic.ValidateRequest(r.Request) != nil || r.Request.Codex.BindingSHA256 != b.BindingSHA256 || r.Version < 0 || r.Answer != nil && (r.InputID == "" || ic.ValidateAnswer(r.Request, *r.Answer) != nil) || len(r.Observations) > 128 {
			return errToolInput
		}
		var previous *cc.CodexUserInputObservation
		keys := map[string]bool{}
		for _, event := range r.Observations {
			o := event.Observation
			if !acpLocalText(event.OperationID, 128) || keys[event.OperationID] || event.InputID != r.InputID || event.Blocked && event.Acked || cc.ValidateUserInputObservation(o) != nil || o.ParamsSHA256 != r.Request.Codex.ParamsSHA256 || previous != nil && !inputObservationAdvances(*previous, o) || o.AnswerObserved && (r.Answer == nil || r.Answer.AnswerSHA256 != o.AnswerSHA256) || event.InputID == "" && (o.AnswerObserved || o.ReplyStarted || event.Acked) {
				return errToolInput
			}
			keys[event.OperationID] = true
			copy := o
			previous = &copy
		}
		if !samePermissionValue(previous, r.Observation) {
			return errToolInput
		}
	}
	return nil
}

// The two independent domains share only native identity and the attempt cap.
// Neither a question nor its answer can become a permission choice.
func validNativeRequestCheckpoints(e entry) error {
	seen := map[string]string{}
	add := func(id cc.CodexRPCRequestID, method, sha string) bool {
		key := id.Kind + ":" + id.Value
		value := method + ":" + sha
		if old, ok := seen[key]; ok {
			return old == value
		}
		seen[key] = value
		return len(seen) <= 32
	}
	if e.ToolPermissions != nil {
		for _, r := range e.ToolPermissions.Requests {
			if r.Request.Codex != nil && !add(r.Request.Codex.RequestID, r.Request.Codex.Method, r.Request.Codex.ParamsSHA256) {
				return errToolInput
			}
		}
	}
	if e.ToolInputs != nil {
		for _, r := range e.ToolInputs.Requests {
			if !add(r.Request.Codex.RequestID, r.Request.Codex.Method, r.Request.Codex.ParamsSHA256) {
				return errToolInput
			}
		}
	}
	return nil
}
func (j *Journal) beginToolInputExecution(a Assignment, p tc.Profile) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolInputs != nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolInput
	}
	cp := &toolInputExecutionCheckpoint{Profile: tc.CloneProfile(p), Requests: map[string]toolInputCheckpoint{}}
	if validToolInputCheckpoint(e.Assignment, cp) != nil {
		return errToolInput
	}
	before := e
	e.ToolInputs = cp
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func safeToolInput(r ic.ToolInputRequest, secrets []string) error {
	if ic.ValidateRequest(r) != nil {
		return errToolInput
	}
	check := func(text string) bool { return progressText(text, secrets) == text && !ansi.MatchString(text) }
	for _, q := range r.Codex.Questions {
		if !check(q.ID) || !check(q.Header) || !check(q.Question) {
			return errToolInput
		}
		for _, option := range q.Options {
			if !check(option.Label) || !check(option.Description) {
				return errToolInput
			}
		}
	}
	return nil
}
func safeToolInputAnswer(r ic.ToolInputRequest, a ic.ToolInputAnswer, secrets []string) error {
	if ic.ValidateAnswer(r, a) != nil {
		return errToolInput
	}
	for _, s := range a.Answers {
		for _, value := range []string{s.QuestionID, s.Option, s.Text} {
			if progressText(value, secrets) != value || ansi.MatchString(value) {
				return errToolInput
			}
		}
	}
	return nil
}
func (j *Journal) recordToolInput(a Assignment, r ic.ToolInputRequest) error {
	if ic.ValidateRequest(r) != nil {
		return errToolInput
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolInputs == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolInput
	}
	if original, exists := e.ToolInputs.Requests[r.IdempotencyKey]; exists {
		if samePermissionValue(original.Request, r) {
			return nil
		}
		return errToolInput
	}
	before := e
	e.ToolInputs = cloneToolInputCheckpoint(e.ToolInputs)
	raw, _ := json.Marshal(r)
	var copy ic.ToolInputRequest
	_ = json.Unmarshal(raw, &copy)
	e.ToolInputs.Requests[r.IdempotencyKey] = toolInputCheckpoint{Request: copy}
	if validToolInputCheckpoint(e.Assignment, e.ToolInputs) != nil || validNativeRequestCheckpoints(e) != nil {
		return errToolInput
	}
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func toolInputSource(a Assignment, profile string, v ic.ToolInput, r ic.ToolInputRequest, project string) bool {
	return ic.ValidateInput(v) == nil && v.ProjectID == project && v.RunID == a.RunID && v.NodeID == a.NodeID && v.Attempt == a.Attempt && v.Fence == a.Fence && v.DispatchID == a.DispatchID && v.WorkerID == a.WorkerID && v.ProfileSHA256 == profile && samePermissionValue(v.Request, r)
}
func (j *Journal) observeToolInput(a Assignment, v ic.ToolInput, bindOnly bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.ToolInputs == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolInput
	}
	r, ok := e.ToolInputs.Requests[v.Request.IdempotencyKey]
	if !ok || !toolInputSource(a, e.ToolInputs.Profile.ProfileSHA256, v, r.Request, v.ProjectID) || r.InputID != "" && r.InputID != v.ID || !bindOnly && v.Version < r.Version || r.Answer != nil && v.Answer != nil && !samePermissionValue(r.Answer, v.Answer) {
		return errToolInput
	}
	before := e
	e.ToolInputs = cloneToolInputCheckpoint(e.ToolInputs)
	r = e.ToolInputs.Requests[v.Request.IdempotencyKey]
	r.InputID = v.ID
	for i := range r.Observations {
		if r.Observations[i].InputID == "" {
			r.Observations[i].InputID = v.ID
		}
	}
	if !bindOnly {
		r.Version = v.Version
		if v.Answer != nil {
			raw, _ := json.Marshal(v.Answer)
			_ = json.Unmarshal(raw, &r.Answer)
		}
	}
	e.ToolInputs.Requests[v.Request.IdempotencyKey] = r
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (j *Journal) toolInput(a Assignment, r ic.ToolInputRequest) (toolInputCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[key(a)]
	if !ok || e.ToolInputs == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return toolInputCheckpoint{}, errToolInput
	}
	v, ok := cloneToolInputCheckpoint(e.ToolInputs).Requests[r.IdempotencyKey]
	if !ok || !samePermissionValue(v.Request, r) {
		return toolInputCheckpoint{}, errToolInput
	}
	return v, nil
}
func (j *Journal) recordToolInputObservation(a Assignment, r ic.ToolInputRequest, id string, o cc.CodexUserInputObservation, beginWrite bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.ToolInputs == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return errToolInput
	}
	v, ok := e.ToolInputs.Requests[r.IdempotencyKey]
	if !ok || !samePermissionValue(v.Request, r) || v.InputID != id || cc.ValidateUserInputObservation(o) != nil || o.ParamsSHA256 != r.Codex.ParamsSHA256 || id == "" && (o.AnswerObserved || o.ReplyStarted) || o.AnswerObserved && (v.Answer == nil || v.Answer.AnswerSHA256 != o.AnswerSHA256) {
		return errToolInput
	}
	if beginWrite && (!o.AnswerObserved || !o.ReplyStarted || o.ReplyWritten || o.RequestResolved || o.TurnClosed || o.FailureCode != "" || v.Observation != nil && v.Observation.ReplyStarted) {
		return errToolInput
	}
	if v.Observation != nil {
		if samePermissionValue(*v.Observation, o) && !beginWrite {
			return nil
		}
		if !inputObservationAdvances(*v.Observation, o) {
			return errToolInput
		}
	}
	if len(v.Observations) >= 128 {
		return errToolInput
	}
	before := e
	e.ToolInputs = cloneToolInputCheckpoint(e.ToolInputs)
	v = e.ToolInputs.Requests[r.IdempotencyKey]
	copy := o
	v.Observation = &copy
	v.Observations = append(v.Observations, toolInputEvent{OperationID: operationID(), InputID: id, Observation: o})
	e.ToolInputs.Requests[r.IdempotencyKey] = v
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = before
		return err
	}
	return nil
}
func (c *Client) OpenToolInput(ctx context.Context, a Assignment, r ic.ToolInputRequest) (ic.ToolInput, error) {
	var v ic.ToolInput
	if ic.ValidateRequest(r) != nil {
		return v, errToolInput
	}
	err := c.post(ctx, "tool-inputs", map[string]any{"project_id": c.ProjectID, "assignment": a, "request": r}, &v)
	profile, _ := a.Config["tool_profile_sha256"].(string)
	if err == nil && !toolInputSource(a, profile, v, r, c.ProjectID) {
		return ic.ToolInput{}, errToolInput
	}
	return v, err
}
func (c *Client) PollToolInput(ctx context.Context, a Assignment, id string) (ic.ToolInput, error) {
	var v ic.ToolInput
	if !acpLocalText(id, 100) || strings.ContainsAny(id, "/\\") {
		return v, errToolInput
	}
	err := c.post(ctx, "tool-inputs/poll", map[string]any{"project_id": c.ProjectID, "assignment": a, "input_id": id}, &v)
	profile, _ := a.Config["tool_profile_sha256"].(string)
	if err == nil && (v.ID != id || !toolInputSource(a, profile, v, v.Request, c.ProjectID)) {
		return ic.ToolInput{}, errToolInput
	}
	return v, err
}
func (c *Client) ObserveToolInput(ctx context.Context, a Assignment, r ic.ToolInputRequest, id, operation string, observation cc.CodexUserInputObservation) (ic.ToolInput, error) {
	var v ic.ToolInput
	if ic.ValidateRequest(r) != nil || cc.ValidateUserInputObservation(observation) != nil || observation.ParamsSHA256 != r.Codex.ParamsSHA256 || !acpLocalText(id, 100) || !acpLocalText(operation, 128) {
		return v, errToolInput
	}
	err := c.post(ctx, "tool-inputs/observations", map[string]any{"project_id": c.ProjectID, "assignment": a, "input_id": id, "idempotency_key": operation, "observation": observation}, &v)
	profile, _ := a.Config["tool_profile_sha256"].(string)
	if err == nil && (v.ID != id || !toolInputSource(a, profile, v, r, c.ProjectID) || v.Observation == nil || !samePermissionValue(*v.Observation, observation)) {
		return ic.ToolInput{}, errToolInput
	}
	return v, err
}
func (j *Journal) pendingToolInputEvents() []pendingToolInputEvent {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []pendingToolInputEvent{}
	keys := make([]string, 0, len(j.entries))
	for key := range j.entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		e := j.entries[key]
		if e.ToolInputs == nil {
			continue
		}
		cp := cloneToolInputCheckpoint(e.ToolInputs)
		rks := make([]string, 0, len(cp.Requests))
		for rk := range cp.Requests {
			rks = append(rks, rk)
		}
		sort.Strings(rks)
		for _, rk := range rks {
			r := cp.Requests[rk]
			for _, event := range r.Observations {
				if !event.Acked {
					out = append(out, pendingToolInputEvent{Assignment: e.Assignment, Request: r.Request, toolInputEvent: event})
				}
			}
		}
	}
	return out
}
func (j *Journal) settleToolInputEvent(event pendingToolInputEvent, blocked bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(event.Assignment)
	e, ok := j.entries[k]
	if !ok || e.ToolInputs == nil {
		return errToolInput
	}
	before := e
	e.ToolInputs = cloneToolInputCheckpoint(e.ToolInputs)
	r, ok := e.ToolInputs.Requests[event.Request.IdempotencyKey]
	if !ok {
		return errToolInput
	}
	for i := range r.Observations {
		if r.Observations[i].OperationID == event.OperationID {
			r.Observations[i].Blocked = blocked
			r.Observations[i].Acked = !blocked
			e.ToolInputs.Requests[event.Request.IdempotencyKey] = r
			j.entries[k] = e
			if err := j.save(); err != nil {
				j.entries[k] = before
				return err
			}
			return nil
		}
	}
	return errToolInput
}
func ReplayToolInputObservations(ctx context.Context, c *Client, j *Journal) error {
	for _, event := range j.pendingToolInputEvents() {
		scoped := c
		if c.Pool != nil {
			if c.Pool.Assignment != nil && !samePoolAssignmentIdentity(*c.Pool.Assignment, event.Assignment) {
				return errToolInput
			}
			var err error
			scoped, err = c.forPoolAssignment(event.Assignment)
			if err != nil {
				return err
			}
		}
		if event.Blocked {
			return errToolInputReconciliation
		}
		delivery := func(err error) error {
			var status *runnerHTTPStatusError
			if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429 {
				if e := j.settleToolInputEvent(event, true); e != nil {
					return e
				}
				return errToolInputReconciliation
			}
			return err
		}
		if event.InputID == "" {
			v, err := scoped.OpenToolInput(ctx, event.Assignment, event.Request)
			if err != nil {
				return delivery(err)
			}
			if err = j.observeToolInput(event.Assignment, v, true); err != nil {
				return err
			}
			event.InputID = v.ID
		}
		if _, err := scoped.ObserveToolInput(ctx, event.Assignment, event.Request, event.InputID, event.OperationID, event.Observation); err != nil {
			return delivery(err)
		}
		if err := j.settleToolInputEvent(event, false); err != nil {
			return err
		}
	}
	return nil
}

func codexToolInputHandler(a Assignment, cfg ExecConfig) codexUserInputHandler {
	requestFor := func(source cc.CodexUserInputRequest) ic.ToolInputRequest {
		raw, _ := json.Marshal(struct {
			Assignment string
			Source     cc.CodexUserInputRequest
		}{key(a), source})
		return ic.ToolInputRequest{IdempotencyKey: "codex-input-" + digest(raw), Protocol: cc.Protocol, Codex: source}
	}
	return codexUserInputHandler{
		Open: func(ctx context.Context, source cc.CodexUserInputRequest) (string, error) {
			r := requestFor(source)
			if cfg.Journal == nil || cfg.ToolInputClient == nil || safeToolInput(r, cfg.Secrets) != nil {
				return "", errToolInput
			}
			if err := cfg.Journal.recordToolInput(a, r); err != nil {
				return "", err
			}
			var v ic.ToolInput
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				v, err = cfg.ToolInputClient.OpenToolInput(ctx, a, r)
				if err == nil {
					break
				}
				var status *runnerHTTPStatusError
				if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429 {
					return "", err
				}
				if attempt == 2 || !wait(ctx, 100*time.Millisecond) {
					return "", err
				}
			}
			if v.Answer != nil && safeToolInputAnswer(r, *v.Answer, cfg.Secrets) != nil {
				return "", errToolInput
			}
			if err = cfg.Journal.observeToolInput(a, v, false); err != nil {
				return "", err
			}
			return v.ID, nil
		},
		Answer: func(ctx context.Context, source cc.CodexUserInputRequest, id string) (*ic.ToolInputAnswer, error) {
			r := requestFor(source)
			for {
				v, err := cfg.ToolInputClient.PollToolInput(ctx, a, id)
				if err != nil || !toolInputSource(a, cfg.ToolInputProfile, v, r, cfg.ToolInputClient.ProjectID) {
					return nil, errToolInput
				}
				if v.Answer != nil && safeToolInputAnswer(r, *v.Answer, cfg.Secrets) != nil {
					return nil, errToolInput
				}
				if err = cfg.Journal.observeToolInput(a, v, false); err != nil {
					return nil, err
				}
				if v.State == "answered" && v.Answer != nil {
					if safeToolInputAnswer(r, *v.Answer, cfg.Secrets) != nil {
						return nil, errToolInput
					}
					return v.Answer, nil
				}
				if v.State != "pending" || !wait(ctx, 250*time.Millisecond) {
					return nil, errToolInput
				}
			}
		},
		BeforeWrite: func(ctx context.Context, source cc.CodexUserInputRequest, id string, answer ic.ToolInputAnswer, o cc.CodexUserInputObservation) error {
			r := requestFor(source)
			v, err := cfg.ToolInputClient.PollToolInput(ctx, a, id)
			if err != nil || v.State != "answered" || v.Answer == nil || !samePermissionValue(*v.Answer, answer) || !toolInputSource(a, cfg.ToolInputProfile, v, r, cfg.ToolInputClient.ProjectID) || v.Observation != nil && (v.Observation.RequestResolved || v.Observation.TurnClosed || v.Observation.ReplyStarted) {
				return errToolInput
			}
			if err = cfg.Journal.observeToolInput(a, v, false); err != nil {
				return err
			}
			return cfg.Journal.recordToolInputObservation(a, r, id, o, true)
		},
		Observe: func(ctx context.Context, source cc.CodexUserInputRequest, id string, o cc.CodexUserInputObservation) error {
			return cfg.Journal.recordToolInputObservation(a, requestFor(source), id, o, false)
		},
	}
}
