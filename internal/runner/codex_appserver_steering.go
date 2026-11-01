package runner

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
)

type codexSteeringHandler struct {
	Open        func(context.Context, cc.CodexSteeringSource) (sc.SteeringSession, error)
	Poll        func(context.Context, sc.SteeringSession) (sc.SteeringPoll, error)
	BeforeWrite func(context.Context, sc.SteeringSession, sc.SteeringInstruction, cc.CodexSteeringObservation) error
	Observe     func(context.Context, sc.SteeringSession, sc.SteeringInstruction, cc.CodexSteeringObservation) error
	Close       func(context.Context, sc.SteeringSession) (sc.SteeringPoll, error)
}
type codexSteeringRecord struct {
	instruction                                  sc.SteeringInstruction
	requestID                                    int
	observation                                  cc.CodexSteeringObservation
	persisted                                    int64
	queued, intentPending, started, writeDone    bool
	stagedClosed, stagedAccepted, stagedRejected bool
	ackSHA                                       string
	changed                                      chan struct{}
}
type codexSteeringSession struct {
	ctx                                   context.Context
	cancel                                context.CancelFunc
	handler                               codexSteeringHandler
	source                                cc.CodexSteeringSource
	session                               sc.SteeringSession
	records                               map[string]*codexSteeringRecord
	order                                 []*codexSteeringRecord
	acks                                  map[string]*codexSteeringRecord
	started, closing, closed, unavailable bool
	nativeClosed                          chan struct{}
	done                                  chan struct{}
	terminal                              map[string]any
}

func (d *codexAppServer) configureSteering(ctx context.Context, h codexSteeringHandler) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx == nil || d.frames != 0 || d.steering != nil || d.binding == nil || d.stream == nil || d.stream.profile.ReadOnly || d.stream.profile.Protocol != cc.Protocol || h.Open == nil || h.Poll == nil || h.BeforeWrite == nil || h.Observe == nil || h.Close == nil {
		return errSteering
	}
	child, cancel := context.WithCancel(ctx)
	d.steering = &codexSteeringSession{ctx: child, cancel: cancel, handler: h, records: map[string]*codexSteeringRecord{}, acks: map[string]*codexSteeringRecord{}, nativeClosed: make(chan struct{}), done: make(chan struct{})}
	return nil
}

// Called only after the original turn/start ACK. It never allocates a thread,
// turn, model selection or another writer.
func (d *codexAppServer) startSteering() {
	s := d.steering
	if s == nil || s.started || d.stopping.Load() || d.nativeTurnClosed || !d.turnAcknowledged || d.evidence == nil || !d.evidence.ConfigurationAcknowledged || d.thread == "" || d.turn == "" {
		return
	}
	s.source = cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: d.thread, TurnID: d.turn, BindingSHA256: d.binding.BindingSHA256}
	s.started = true
	go d.runSteering(s)
}
func (d *codexAppServer) steeringActive(s *codexSteeringSession) bool {
	return d.steering == s && !s.closing && !d.stopping.Load() && !d.nativeTurnClosed && !d.terminalReceived && d.bad == nil && d.turnAcknowledged && d.thread == s.source.ThreadID && d.turn == s.source.TurnID
}
func (d *codexAppServer) runSteering(s *codexSteeringSession) {
	defer close(s.done)
	session, err := s.handler.Open(s.ctx, s.source)
	if errors.Is(err, errSteeringUnavailable) {
		d.mu.Lock()
		s.unavailable = true
		d.releaseSteeringTerminal()
		d.mu.Unlock()
		return
	}
	if err != nil {
		d.mu.Lock()
		d.fail("steering session registration requires reconciliation")
		d.mu.Unlock()
		d.finishSteeringSession(s)
		return
	}
	d.mu.Lock()
	s.session = session
	d.syncSteeringEvidence()
	d.mu.Unlock()
	for s.ctx.Err() == nil {
		poll, err := s.handler.Poll(s.ctx, session)
		d.mu.Lock()
		closing := s.closing
		d.mu.Unlock()
		if err != nil {
			if !closing {
				d.mu.Lock()
				d.fail("steering session poll requires reconciliation")
				d.mu.Unlock()
			}
			break
		}
		if poll.Session.State != "active" {
			break
		}
		for _, instruction := range poll.Instructions {
			d.mu.Lock()
			if !d.steeringActive(s) {
				d.mu.Unlock()
				break
			}
			r := s.records[instruction.ID]
			if r == nil {
				if len(s.records) >= 32 || !steeringInstructionSource(session, instruction) || !safeSteeringText(instruction.Message.Body, d.stream.secrets) || instruction.Observation != nil {
					d.fail("steering instruction source is unsafe or already observed")
					d.mu.Unlock()
					break
				}
				d.requestID++
				sha, err := cc.SteeringParamsDigest(s.source, instruction.ID, instruction.Message.Body)
				if err != nil {
					d.fail("steering native parameters are invalid")
					d.mu.Unlock()
					break
				}
				r = &codexSteeringRecord{instruction: instruction, requestID: d.requestID, observation: cc.CodexSteeringObservation{Sequence: 1, RequestID: cc.CodexRPCRequestID{Kind: "number", Value: strconv.Itoa(d.requestID)}, ParamsSHA256: sha, Observed: true}, changed: make(chan struct{}, 1)}
				s.records[instruction.ID], s.acks[strconv.Itoa(d.requestID)] = r, r
				s.order = append(s.order, r)
				d.syncSteeringEvidence()
			} else if !sameSteeringInstruction(r.instruction, instruction) {
				d.fail("steering instruction body changed")
				d.mu.Unlock()
				break
			} else {
				d.mu.Unlock()
				continue
			}
			d.mu.Unlock()
			if !d.deliverSteering(s, r) {
				break
			}
		}
		d.mu.Lock()
		active := d.steeringActive(s)
		d.mu.Unlock()
		if !active || !wait(s.ctx, 250*time.Millisecond) {
			break
		}
	}
	d.finishSteeringSession(s)
}
func (d *codexAppServer) steeringChanged(r *codexSteeringRecord) {
	r.observation.Sequence++
	d.syncSteeringEvidence()
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (d *codexAppServer) persistSteering(s *codexSteeringSession, r *codexSteeringRecord) bool {
	d.mu.Lock()
	o, persisted, pending := r.observation, r.persisted, r.intentPending
	session := s.session
	d.mu.Unlock()
	if pending || o.Sequence == 0 || o.Sequence == persisted {
		return true
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 3*time.Second)
	err := s.handler.Observe(ctx, session, r.instruction, o)
	cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.fail("steering durable observation unavailable")
		return false
	}
	if o.Sequence > r.persisted {
		r.persisted = o.Sequence
	}
	d.syncSteeringEvidence()
	return true
}
func (d *codexAppServer) deliverSteering(s *codexSteeringSession, r *codexSteeringRecord) bool {
	if !d.persistSteering(s, r) {
		return false
	}
	d.mu.Lock()
	active := d.steeringActive(s)
	if active {
		r.queued = true
	}
	d.mu.Unlock()
	if !active {
		return false
	}
	params, err := cc.BuildSteeringParams(s.source, r.instruction.ID, r.instruction.Message.Body)
	if err == nil {
		err = d.input.enqueueReply(map[string]any{"id": r.requestID, "method": "turn/steer", "params": params}, func() error { return d.beforeSteeringWrite(s, r) }, func(err error) { d.afterSteeringWrite(r, err) })
	}
	if err != nil {
		d.mu.Lock()
		r.writeDone = true
		r.observation.FailureCode = "steering_queue_unavailable"
		d.steeringChanged(r)
		d.fail("steering request queue unavailable")
		d.mu.Unlock()
		_ = d.persistSteering(s, r)
		return false
	}
	var grace <-chan time.Time
	for {
		if !d.persistSteering(s, r) {
			return false
		}
		d.mu.Lock()
		o := r.observation
		done := r.writeDone && (o.NativeAccepted || o.NativeRejected || o.FailureCode != "") && r.persisted == o.Sequence
		closing := s.closing
		d.mu.Unlock()
		if done {
			return o.FailureCode == ""
		}
		if closing && grace == nil {
			grace = time.After(1500 * time.Millisecond)
		}
		select {
		case <-r.changed:
		case <-s.ctx.Done():
			if grace == nil {
				grace = time.After(1500 * time.Millisecond)
			}
			select {
			case <-r.changed:
			case <-grace:
				d.mu.Lock()
				if r.observation.FailureCode == "" {
					if r.observation.WriteStarted {
						r.observation.FailureCode = "steering_ack_unknown"
						d.fail("steering native acknowledgement requires reconciliation")
					}
					d.steeringChanged(r)
				}
				d.mu.Unlock()
				_ = d.persistSteering(s, r)
				return false
			}
		case <-grace:
			d.mu.Lock()
			if r.observation.FailureCode == "" && r.observation.WriteStarted && !(r.observation.NativeAccepted || r.observation.NativeRejected) {
				r.observation.FailureCode = "steering_ack_unknown"
				d.steeringChanged(r)
				d.fail("steering native acknowledgement requires reconciliation")
			}
			d.mu.Unlock()
			_ = d.persistSteering(s, r)
			return false
		}
	}
}
func (d *codexAppServer) beforeSteeringWrite(s *codexSteeringSession, r *codexSteeringRecord) error {
	d.mu.Lock()
	if !d.steeringActive(s) || r.started || r.intentPending {
		d.mu.Unlock()
		return errSteering
	}
	intent := r.observation
	intent.Sequence++
	intent.WriteStarted = true
	r.intentPending = true
	session := s.session
	d.mu.Unlock()
	err := s.handler.BeforeWrite(s.ctx, session, r.instruction, intent)
	d.mu.Lock()
	defer d.mu.Unlock()
	r.intentPending = false
	// BeforeWrite durably records the exact intent before its current-source
	// HTTP check. Even an error cannot reconstruct another native write permit.
	r.observation = intent
	r.persisted = intent.Sequence
	if r.stagedClosed {
		r.observation.TurnClosed = true
		r.stagedClosed = false
		d.steeringChanged(r)
	}
	if err != nil || !d.steeringActive(s) {
		return errSteering
	}
	r.started = true
	d.syncSteeringEvidence()
	return nil
}
func (d *codexAppServer) afterSteeringWrite(r *codexSteeringRecord, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r.writeDone = true
	if err == nil && r.started && r.observation.WriteStarted {
		r.observation.WriteWritten = true
	} else if r.observation.FailureCode == "" {
		r.observation.FailureCode = "steering_write_unknown"
		d.fail("steering native write requires reconciliation")
	}
	r.observation.NativeAccepted = r.stagedAccepted
	r.observation.NativeRejected = r.stagedRejected
	d.steeringChanged(r)
}

// Returns false for a normal startup/interrupt response. Steering ACKs have a
// direction-specific map and never consume or replace startup waitingID.
func (d *codexAppServer) steeringResponse(id string, m map[string]any) bool {
	if d.steering == nil {
		return false
	}
	r := d.steering.acks[id]
	if r == nil {
		return false
	}
	sha := codexApprovalSHA(m)
	if r.ackSHA != "" {
		if r.ackSHA != sha {
			d.fail("steering acknowledgement changed")
		}
		return true
	}
	if !r.started || !r.observation.WriteStarted {
		d.fail("steering acknowledgement preceded its native write")
		return true
	}
	_, hasError := m["error"]
	result, hasResult := m["result"].(map[string]any)
	if hasError == hasResult {
		d.fail("steering acknowledgement shape is invalid")
		return true
	}
	if hasError {
		e, ok := m["error"].(map[string]any)
		code, codeOK := e["code"].(json.Number)
		_, messageOK := e["message"].(string)
		number, codeError := strconv.ParseInt(string(code), 10, 64)
		if !ok || !codeOK || !messageOK || codeError != nil || number < -cc.MaxSafeInteger || number > cc.MaxSafeInteger || strconv.FormatInt(number, 10) != string(code) {
			d.fail("steering RPC rejection is invalid")
			return true
		}
		r.stagedRejected = true
	} else if textAt(result, "turnId") != d.steering.source.TurnID {
		d.fail("steering acknowledgement turn differs")
		return true
	} else {
		r.stagedAccepted = true
	}
	r.ackSHA = sha
	if r.writeDone {
		r.observation.NativeAccepted = r.stagedAccepted
		r.observation.NativeRejected = r.stagedRejected
		d.steeringChanged(r)
	}
	return true
}
func (d *codexAppServer) closeSteering(nativeClosed bool) {
	s := d.steering
	if s == nil {
		return
	}
	s.closing = true
	s.cancel()
	if nativeClosed {
		select {
		case <-s.nativeClosed:
		default:
			close(s.nativeClosed)
		}
		for _, r := range s.order {
			if r.observation.Sequence == 0 || r.observation.TurnClosed || r.stagedClosed {
				continue
			}
			if r.intentPending {
				r.stagedClosed = true
			} else {
				r.observation.TurnClosed = true
				d.steeringChanged(r)
			}
		}
	}
}
func (d *codexAppServer) finishSteeringSession(s *codexSteeringSession) {
	// Closing the transport on a stop request is immediate; native TurnClosed
	// is only added once a terminal frame or actual process exit is observed.
	select {
	case <-s.nativeClosed:
	case <-time.After(5 * time.Second):
		d.mu.Lock()
		d.fail("steering native lifetime did not close")
		d.mu.Unlock()
	}
	for _, r := range s.order {
		deadline := time.Now().Add(5 * time.Second)
		for {
			d.mu.Lock()
			pending := r.intentPending || r.queued && !r.writeDone
			d.mu.Unlock()
			if !pending {
				break
			}
			if time.Now().After(deadline) {
				d.mu.Lock()
				d.fail("steering writer did not settle")
				d.mu.Unlock()
				return
			}
			select {
			case <-r.changed:
			case <-time.After(10 * time.Millisecond):
			}
		}
		if !d.persistSteering(s, r) {
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 5*time.Second)
	poll, err := s.handler.Close(ctx, s.session)
	cancel()
	if errors.Is(err, errSteeringUnavailable) {
		d.mu.Lock()
		s.unavailable = true
		d.releaseSteeringTerminal()
		d.mu.Unlock()
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.fail("steering session close requires reconciliation")
		return
	}
	if poll.Session.ID == "" || poll.Session.State != "closed" || s.session.ID != "" && poll.Session.ID != s.session.ID || poll.Session.Source != s.source {
		d.fail("steering closed session differs")
		return
	}
	s.session = poll.Session
	for _, instruction := range poll.Instructions {
		prior := s.records[instruction.ID]
		if prior != nil {
			if !sameSteeringInstruction(prior.instruction, instruction) || !samePermissionValue(prior.observation, instruction.Observation) {
				d.fail("steering final instruction facts differ")
				return
			}
			continue
		}
		if len(s.order) >= 32 || !steeringInstructionSource(s.session, instruction) || instruction.Observation != nil {
			d.fail("steering final unobserved instruction differs")
			return
		}
		r := &codexSteeringRecord{instruction: instruction}
		s.records[instruction.ID] = r
		s.order = append(s.order, r)
	}
	s.closed = true
	d.syncSteeringEvidence()
	d.releaseSteeringTerminal()
}
func (d *codexAppServer) finishSteering() {
	d.mu.Lock()
	s := d.steering
	if s == nil || !s.started {
		d.mu.Unlock()
		return
	}
	d.closeSteering(true)
	d.mu.Unlock()
	select {
	case <-s.done:
	case <-time.After(7 * time.Second):
		d.mu.Lock()
		d.fail("steering durable facts did not settle before process exit")
		d.mu.Unlock()
	}
}
func (d *codexAppServer) syncSteeringEvidence() {
	s := d.steering
	if s == nil || s.session.ID == "" || s.unavailable || d.evidence == nil {
		return
	}
	e := &cc.CodexSteeringEvidence{SessionID: s.session.ID, Source: s.source, Closed: s.closed, Instructions: []cc.CodexSteeringInstructionEvidence{}}
	for _, r := range s.order {
		var o *cc.CodexSteeringObservation
		if r.observation.Sequence > 0 {
			copy := r.observation
			o = &copy
		}
		e.Instructions = append(e.Instructions, cc.CodexSteeringInstructionEvidence{InstructionID: r.instruction.ID, MessageSHA256: r.instruction.Message.BodySHA256, Observation: o})
	}
	d.evidence.Steering = e
}

func (d *codexAppServer) deferSteeringTerminal(p map[string]any) bool {
	s := d.steering
	if s == nil || !s.started || s.closed || s.unavailable {
		return false
	}
	if s.terminal != nil {
		d.fail("multiple turn terminals with pending steering facts")
		return true
	}
	s.terminal = p
	return true
}
func (d *codexAppServer) releaseSteeringTerminal() {
	s := d.steering
	if s != nil && s.terminal != nil && (s.closed || s.unavailable) && d.approvalsClosed() && d.userInputsClosed() {
		p := s.terminal
		s.terminal = nil
		d.notification("turn/completed", p)
	}
}
