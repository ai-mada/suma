package runner

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

var errSteering = errors.New("steering requires its exact original live source")
var errSteeringUnavailable = errors.New("steering is unavailable for this execution")
var errSteeringReconciliation = errors.New("steering transport requires reconciliation; original journal retained")

type steeringObservationEvent struct {
	OperationID string                      `json:"operation_id"`
	Observation cc.CodexSteeringObservation `json:"observation"`
	Acked       bool                        `json:"acked"`
	Blocked     bool                        `json:"blocked,omitempty"`
}
type steeringInstructionCheckpoint struct {
	Instruction sc.SteeringInstruction       `json:"instruction"`
	Observation *cc.CodexSteeringObservation `json:"observation,omitempty"`
	Events      []steeringObservationEvent   `json:"events,omitempty"`
}
type steeringExecutionCheckpoint struct {
	Profile          tc.Profile                               `json:"profile"`
	Source           cc.CodexSteeringSource                   `json:"source"`
	OpenOperation    string                                   `json:"open_operation"`
	Session          *sc.SteeringSession                      `json:"session,omitempty"`
	NotTaskExecution bool                                     `json:"not_task_execution,omitempty"`
	CloseOperation   string                                   `json:"close_operation,omitempty"`
	CloseRequested   bool                                     `json:"close_requested"`
	CloseAcked       bool                                     `json:"close_acked"`
	Blocked          bool                                     `json:"blocked,omitempty"`
	Instructions     map[string]steeringInstructionCheckpoint `json:"instructions"`
}

func cloneSteeringCheckpoint(cp *steeringExecutionCheckpoint) *steeringExecutionCheckpoint {
	if cp == nil {
		return nil
	}
	raw, _ := json.Marshal(cp)
	var copy steeringExecutionCheckpoint
	_ = json.Unmarshal(raw, &copy)
	return &copy
}
func steeringSessionSource(a Assignment, p tc.Profile, source cc.CodexSteeringSource, session sc.SteeringSession, project string) bool {
	return sc.ValidateSession(session) == nil && session.ProjectID == project && session.RunID == a.RunID && session.NodeID == a.NodeID && session.Attempt == a.Attempt && session.Fence == a.Fence && session.WorkerID == a.WorkerID && session.DispatchID == a.DispatchID && session.ProfileSHA256 == p.ProfileSHA256 && session.Source == source
}
func steeringInstructionSource(session sc.SteeringSession, instruction sc.SteeringInstruction) bool {
	return sc.ValidateInstruction(instruction) == nil && instruction.SessionID == session.ID && instruction.ProjectID == session.ProjectID && instruction.TaskID == session.TaskID && instruction.RunID == session.RunID && instruction.NodeID == session.NodeID && instruction.Attempt == session.Attempt && instruction.Fence == session.Fence
}
func sameSteeringInstruction(first, next sc.SteeringInstruction) bool {
	first.Version, next.Version = 0, 0
	first.Observation, next.Observation = nil, nil
	return samePermissionValue(first, next)
}
func steeringObservationAdvances(old, next cc.CodexSteeringObservation) bool {
	return cc.ValidateSteeringObservation(next) == nil && next.Sequence > old.Sequence && old.RequestID == next.RequestID && old.ParamsSHA256 == next.ParamsSHA256 && (!old.Observed || next.Observed) && (!old.WriteStarted || next.WriteStarted) && (!old.WriteWritten || next.WriteWritten) && (!old.NativeAccepted || next.NativeAccepted) && (!old.NativeRejected || next.NativeRejected) && (!old.TurnClosed || next.TurnClosed) && (old.FailureCode == "" || old.FailureCode == next.FailureCode) && (!old.TurnClosed || old.WriteStarted || !next.WriteStarted)
}
func validSteeringCheckpoint(a Assignment, cp *steeringExecutionCheckpoint) error {
	if cp == nil {
		return nil
	}
	b, err := cc.DecodeBinding(a.Config["codex_execution"])
	if err != nil || a.Type != "agent" || a.WorkBuddyCloud != nil || len(a.Command) != 1 || cp.Profile.CommandID != a.Command[0] || cp.Profile.Adapter != "codex" || cp.Profile.Protocol != cc.Protocol || cp.Profile.ReadOnly || tc.ValidateProfile(cp.Profile, true) != nil || a.Config["tool_profile_sha256"] != cp.Profile.ProfileSHA256 || cc.ValidateSteeringSource(cp.Source) != nil || cp.Source.BindingSHA256 != b.BindingSHA256 || !acpLocalText(cp.OpenOperation, 128) || cp.Instructions == nil || len(cp.Instructions) > 32 || cp.CloseRequested != (cp.CloseOperation != "") || cp.CloseAcked && !cp.CloseRequested || cp.NotTaskExecution && cp.Session != nil {
		return errSteering
	}
	if cp.Session != nil && !steeringSessionSource(a, cp.Profile, cp.Source, *cp.Session, cp.Session.ProjectID) {
		return errSteering
	}
	seen := map[string]bool{}
	for id, entry := range cp.Instructions {
		if cp.Session == nil || id != entry.Instruction.ID || !steeringInstructionSource(*cp.Session, entry.Instruction) || seen[entry.Instruction.Message.ID] || len(entry.Events) > 128 {
			return errSteering
		}
		seen[entry.Instruction.Message.ID] = true
		sha, err := cc.SteeringParamsDigest(cp.Source, id, entry.Instruction.Message.Body)
		if err != nil {
			return errSteering
		}
		var previous *cc.CodexSteeringObservation
		keys := map[string]bool{}
		for _, event := range entry.Events {
			o := event.Observation
			if !acpLocalText(event.OperationID, 128) || keys[event.OperationID] || event.Blocked && event.Acked || cc.ValidateSteeringObservation(o) != nil || o.ParamsSHA256 != sha || previous != nil && !steeringObservationAdvances(*previous, o) {
				return errSteering
			}
			keys[event.OperationID] = true
			copy := o
			previous = &copy
		}
		if !samePermissionValue(previous, entry.Observation) {
			return errSteering
		}
	}
	return nil
}
func (j *Journal) updateSteering(a Assignment, executing bool, update func(*entry) error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	original, ok := j.entries[k]
	if !ok || executing && original.Result != nil || candidateRef(a) != candidateRef(original.Assignment) || a.DispatchID != original.Assignment.DispatchID {
		return errSteering
	}
	e := original
	e.Steering = cloneSteeringCheckpoint(e.Steering)
	if err := update(&e); err != nil {
		return err
	}
	if validSteeringCheckpoint(e.Assignment, e.Steering) != nil {
		return errSteering
	}
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = original
		return err
	}
	return nil
}
func (j *Journal) steeringCheckpoint(a Assignment) (*steeringExecutionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[key(a)]
	if !ok || e.Steering == nil || candidateRef(a) != candidateRef(e.Assignment) || a.DispatchID != e.Assignment.DispatchID {
		return nil, errSteering
	}
	return cloneSteeringCheckpoint(e.Steering), nil
}
func (j *Journal) prepareSteering(a Assignment, p tc.Profile, source cc.CodexSteeringSource) error {
	return j.updateSteering(a, true, func(e *entry) error {
		if e.Steering != nil {
			if e.Steering.Source != source || !tc.EqualProfile(e.Steering.Profile, p) {
				return errSteering
			}
			return nil
		}
		e.Steering = &steeringExecutionCheckpoint{Profile: tc.CloneProfile(p), Source: source, OpenOperation: operationID(), Instructions: map[string]steeringInstructionCheckpoint{}}
		return nil
	})
}
func (j *Journal) bindSteeringSession(a Assignment, session sc.SteeringSession) error {
	return j.updateSteering(a, false, func(e *entry) error {
		cp := e.Steering
		if cp == nil || cp.NotTaskExecution || !steeringSessionSource(a, cp.Profile, cp.Source, session, session.ProjectID) || cp.Session != nil && cp.Session.ID != session.ID {
			return errSteering
		}
		copy := session
		cp.Session = &copy
		return nil
	})
}
func (j *Journal) recordSteeringInstructions(a Assignment, poll sc.SteeringPoll, secrets []string) error {
	return j.updateSteering(a, true, func(e *entry) error {
		cp := e.Steering
		if cp == nil || cp.Session == nil || poll.Session.ID != cp.Session.ID || !steeringSessionSource(a, cp.Profile, cp.Source, poll.Session, cp.Session.ProjectID) || len(poll.Instructions) > 32 {
			return errSteering
		}
		for _, instruction := range poll.Instructions {
			if !steeringInstructionSource(*cp.Session, instruction) || !safeSteeringText(instruction.Message.Body, secrets) {
				return errSteering
			}
			if prior, exists := cp.Instructions[instruction.ID]; exists {
				if !sameSteeringInstruction(prior.Instruction, instruction) {
					return errSteering
				}
				continue
			}
			cp.Instructions[instruction.ID] = steeringInstructionCheckpoint{Instruction: instruction}
		}
		return nil
	})
}
func (j *Journal) recordSteeringObservation(a Assignment, instruction sc.SteeringInstruction, o cc.CodexSteeringObservation, begin bool) error {
	return j.updateSteering(a, true, func(e *entry) error {
		cp := e.Steering
		if cp == nil {
			return errSteering
		}
		r, ok := cp.Instructions[instruction.ID]
		if !ok || !sameSteeringInstruction(r.Instruction, instruction) || cc.ValidateSteeringObservation(o) != nil {
			return errSteering
		}
		sha, err := cc.SteeringParamsDigest(cp.Source, instruction.ID, instruction.Message.Body)
		if err != nil || sha != o.ParamsSHA256 {
			return errSteering
		}
		if begin && (!o.WriteStarted || o.WriteWritten || o.NativeAccepted || o.NativeRejected || o.TurnClosed || o.FailureCode != "" || cp.CloseRequested || r.Observation != nil && r.Observation.WriteStarted) {
			return errSteering
		}
		if r.Observation != nil {
			if !begin && samePermissionValue(*r.Observation, o) {
				return nil
			}
			if !steeringObservationAdvances(*r.Observation, o) {
				return errSteering
			}
		}
		if len(r.Events) >= 128 {
			return errSteering
		}
		copy := o
		r.Observation = &copy
		r.Events = append(r.Events, steeringObservationEvent{OperationID: operationID(), Observation: o})
		cp.Instructions[instruction.ID] = r
		return nil
	})
}
func (j *Journal) requestSteeringClose(a Assignment) error {
	return j.updateSteering(a, false, func(e *entry) error {
		if e.Steering == nil {
			return errSteering
		}
		if !e.Steering.CloseRequested {
			e.Steering.CloseRequested = true
			e.Steering.CloseOperation = operationID()
		}
		return nil
	})
}
func (c *Client) OpenSteeringSession(ctx context.Context, a Assignment, source cc.CodexSteeringSource, operation string) (sc.SteeringOpenResult, error) {
	var out sc.SteeringOpenResult
	if cc.ValidateSteeringSource(source) != nil || !acpLocalText(operation, 128) {
		return out, errSteering
	}
	err := c.post(ctx, "steering/sessions", map[string]any{"project_id": c.ProjectID, "assignment": a, "source": source, "idempotency_key": operation}, &out)
	p := tc.Profile{ProfileSHA256: stringAtConfig(a, "tool_profile_sha256")}
	if err == nil && !(out.Session == nil && out.Reason == "not_task_execution") && !(out.Session != nil && out.Reason == "" && steeringSessionSource(a, p, source, *out.Session, c.ProjectID)) {
		return sc.SteeringOpenResult{}, errSteering
	}
	return out, err
}
func stringAtConfig(a Assignment, field string) string {
	value, _ := a.Config[field].(string)
	return value
}
func (c *Client) PollSteering(ctx context.Context, a Assignment, session sc.SteeringSession) (sc.SteeringPoll, error) {
	var out sc.SteeringPoll
	if !acpLocalText(session.ID, 100) {
		return out, errSteering
	}
	err := c.post(ctx, "steering/poll", map[string]any{"project_id": c.ProjectID, "assignment": a, "session_id": session.ID}, &out)
	p := tc.Profile{ProfileSHA256: stringAtConfig(a, "tool_profile_sha256")}
	if err == nil {
		if !steeringSessionSource(a, p, session.Source, out.Session, c.ProjectID) || out.Session.ID != session.ID || out.Session.Version < session.Version || len(out.Instructions) > 32 {
			return sc.SteeringPoll{}, errSteering
		}
		seen := map[string]bool{}
		messages := map[string]bool{}
		for _, i := range out.Instructions {
			if !steeringInstructionSource(out.Session, i) || seen[i.ID] || messages[i.Message.ID] {
				return sc.SteeringPoll{}, errSteering
			}
			seen[i.ID] = true
			messages[i.Message.ID] = true
		}
	}
	return out, err
}
func (c *Client) ObserveSteering(ctx context.Context, a Assignment, session sc.SteeringSession, instruction sc.SteeringInstruction, operation string, o cc.CodexSteeringObservation) (sc.SteeringInstruction, error) {
	var out sc.SteeringInstruction
	sha, err := cc.SteeringParamsDigest(session.Source, instruction.ID, instruction.Message.Body)
	if err != nil || cc.ValidateSteeringObservation(o) != nil || sha != o.ParamsSHA256 || !steeringInstructionSource(session, instruction) || !acpLocalText(operation, 128) {
		return out, errSteering
	}
	err = c.post(ctx, "steering/observations", map[string]any{"project_id": c.ProjectID, "assignment": a, "session_id": session.ID, "instruction_id": instruction.ID, "idempotency_key": operation, "observation": o}, &out)
	if err == nil && (!sameSteeringInstruction(instruction, out) || out.Observation == nil || !samePermissionValue(*out.Observation, o)) {
		return sc.SteeringInstruction{}, errSteering
	}
	return out, err
}
func (c *Client) CloseSteeringSession(ctx context.Context, a Assignment, session sc.SteeringSession, operation string) (sc.SteeringSession, error) {
	var out sc.SteeringSession
	if !acpLocalText(operation, 128) {
		return out, errSteering
	}
	err := c.post(ctx, "steering/close", map[string]any{"project_id": c.ProjectID, "assignment": a, "session_id": session.ID, "idempotency_key": operation}, &out)
	p := tc.Profile{ProfileSHA256: stringAtConfig(a, "tool_profile_sha256")}
	if err == nil && (!steeringSessionSource(a, p, session.Source, out, c.ProjectID) || out.ID != session.ID || out.State != "closed") {
		return sc.SteeringSession{}, errSteering
	}
	return out, err
}
func ReplaySteering(ctx context.Context, c *Client, j *Journal) error {
	j.mu.Lock()
	assignments := []Assignment{}
	for _, e := range j.entries {
		if e.Steering != nil {
			assignments = append(assignments, e.Assignment)
		}
	}
	j.mu.Unlock()
	sort.Slice(assignments, func(i, j int) bool { return key(assignments[i]) < key(assignments[j]) })
	for _, a := range assignments {
		cp, err := j.steeringCheckpoint(a)
		if err != nil {
			return err
		}
		if cp.NotTaskExecution {
			continue
		}
		if cp.Blocked {
			return errSteeringReconciliation
		}
		scoped := c
		if c.Pool != nil {
			if c.Pool.Assignment != nil && !samePoolAssignmentIdentity(*c.Pool.Assignment, a) {
				return errSteering
			}
			scoped, err = c.forPoolAssignment(a)
			if err != nil {
				return err
			}
		} else if a.PoolID != "" {
			return errSteering
		}
		delivery := func(err error) error {
			var status *runnerHTTPStatusError
			if errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429 {
				if saveErr := j.updateSteering(a, false, func(e *entry) error { e.Steering.Blocked = true; return nil }); saveErr != nil {
					return saveErr
				}
				return errSteeringReconciliation
			}
			return err
		}
		if cp.Session == nil {
			result, err := scoped.OpenSteeringSession(ctx, a, cp.Source, cp.OpenOperation)
			if err != nil {
				return delivery(err)
			}
			if result.Session == nil {
				if err = j.updateSteering(a, false, func(e *entry) error { e.Steering.NotTaskExecution = true; return nil }); err != nil {
					return err
				}
				continue
			}
			if err = j.bindSteeringSession(a, *result.Session); err != nil {
				return err
			}
			cp, _ = j.steeringCheckpoint(a)
		}
		ids := make([]string, 0, len(cp.Instructions))
		for id := range cp.Instructions {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			r := cp.Instructions[id]
			for _, event := range r.Events {
				if event.Acked {
					continue
				}
				if event.Blocked {
					return errSteeringReconciliation
				}
				if _, err = scoped.ObserveSteering(ctx, a, *cp.Session, r.Instruction, event.OperationID, event.Observation); err != nil {
					return delivery(err)
				}
				operation := event.OperationID
				if err = j.updateSteering(a, false, func(e *entry) error {
					r := e.Steering.Instructions[id]
					for i := range r.Events {
						if r.Events[i].OperationID == operation {
							r.Events[i].Acked = true
						}
					}
					e.Steering.Instructions[id] = r
					return nil
				}); err != nil {
					return err
				}
			}
		}
		if cp.CloseRequested && !cp.CloseAcked {
			session, err := scoped.CloseSteeringSession(ctx, a, *cp.Session, cp.CloseOperation)
			if err != nil {
				return delivery(err)
			}
			if err = j.updateSteering(a, false, func(e *entry) error { e.Steering.CloseAcked = true; e.Steering.Session = &session; return nil }); err != nil {
				return err
			}
		}
	}
	return nil
}
func codexSteeringTransport(a Assignment, cfg ExecConfig, profile tc.Profile) codexSteeringHandler {
	return codexSteeringHandler{
		Open: func(ctx context.Context, source cc.CodexSteeringSource) (sc.SteeringSession, error) {
			if cfg.Journal == nil || cfg.SteeringClient == nil {
				return sc.SteeringSession{}, errSteeringUnavailable
			}
			if err := cfg.Journal.prepareSteering(a, profile, source); err != nil {
				return sc.SteeringSession{}, err
			}
			cp, _ := cfg.Journal.steeringCheckpoint(a)
			result, err := cfg.SteeringClient.OpenSteeringSession(ctx, a, source, cp.OpenOperation)
			if err != nil {
				return sc.SteeringSession{}, err
			}
			if result.Session == nil {
				if err = cfg.Journal.updateSteering(a, false, func(e *entry) error { e.Steering.NotTaskExecution = true; return nil }); err != nil {
					return sc.SteeringSession{}, err
				}
				return sc.SteeringSession{}, errSteeringUnavailable
			}
			if err = cfg.Journal.bindSteeringSession(a, *result.Session); err != nil {
				return sc.SteeringSession{}, err
			}
			return *result.Session, nil
		},
		Poll: func(ctx context.Context, session sc.SteeringSession) (sc.SteeringPoll, error) {
			poll, err := cfg.SteeringClient.PollSteering(ctx, a, session)
			if err == nil {
				err = cfg.Journal.recordSteeringInstructions(a, poll, cfg.Secrets)
			}
			return poll, err
		},
		Observe: func(_ context.Context, _ sc.SteeringSession, instruction sc.SteeringInstruction, o cc.CodexSteeringObservation) error {
			return cfg.Journal.recordSteeringObservation(a, instruction, o, false)
		},
		BeforeWrite: func(ctx context.Context, session sc.SteeringSession, instruction sc.SteeringInstruction, o cc.CodexSteeringObservation) error {
			if err := cfg.Journal.recordSteeringObservation(a, instruction, o, true); err != nil {
				return err
			}
			poll, err := cfg.SteeringClient.PollSteering(ctx, a, session)
			if err != nil || poll.Session.State != "active" {
				return errSteering
			}
			expires, err := time.Parse(time.RFC3339Nano, instruction.ExpiresAt)
			if err != nil || !expires.After(time.Now()) {
				return errSteering
			}
			for _, current := range poll.Instructions {
				if current.ID == instruction.ID && sameSteeringInstruction(current, instruction) {
					return nil
				}
			}
			return errSteering
		},
		Close: func(ctx context.Context, _ sc.SteeringSession) (sc.SteeringPoll, error) {
			if err := cfg.Journal.requestSteeringClose(a); err != nil {
				return sc.SteeringPoll{}, err
			}
			if err := ReplaySteering(ctx, cfg.SteeringClient, cfg.Journal); err != nil {
				return sc.SteeringPoll{}, err
			}
			cp, err := cfg.Journal.steeringCheckpoint(a)
			if err != nil {
				return sc.SteeringPoll{}, err
			}
			if cp.NotTaskExecution {
				return sc.SteeringPoll{}, errSteeringUnavailable
			}
			if cp.Session == nil || !cp.CloseAcked {
				return sc.SteeringPoll{}, errSteering
			}
			poll, err := cfg.SteeringClient.PollSteering(ctx, a, *cp.Session)
			if err == nil {
				err = cfg.Journal.recordSteeringInstructions(a, poll, cfg.Secrets)
			}
			return poll, err
		},
	}
}

func safeSteeringText(text string, secrets []string) bool {
	if !cc.ValidSteeringText(text) || credential.MatchString(text) {
		return false
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			return false
		}
	}
	return true
}
