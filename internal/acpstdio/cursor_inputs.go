package acpstdio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

type CursorInputAnswer struct {
	Decision     cu.CursorInputDecision
	AnswerSHA256 string
}

// The Runner owns authority and durable IO. This transport only reconstructs
// the typed result and reports what its single stdin writer actually did.
type CursorInputHandler struct {
	Open        func(context.Context, cu.CursorInputRequest) (string, error)
	Answer      func(context.Context, cu.CursorInputRequest, string) (*CursorInputAnswer, error)
	BeforeWrite func(context.Context, cu.CursorInputRequest, string, CursorInputAnswer, delivery.Observation) error
	Observe     func(context.Context, cu.CursorInputRequest, string, delivery.Observation) error
}
type cursorNativeRequest struct{ method, sha string }
type cursorInputRecord struct {
	ctx                                                     context.Context
	cancel                                                  context.CancelFunc
	key, inputID                                            string
	wireID                                                  json.RawMessage
	request                                                 cu.CursorInputRequest
	answer                                                  *CursorInputAnswer
	observation                                             delivery.Observation
	persisted                                               int64
	queued, started, writeDone, intentPending, stagedClosed bool
	changed                                                 chan struct{}
}
type cursorInputState struct {
	handler CursorInputHandler
	prompt  cu.CursorRPCRequestID
	seen    map[string]cursorNativeRequest
	records map[string]*cursorInputRecord
	order   []*cursorInputRecord
	wait    sync.WaitGroup
}

func rawDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Both permission and input requests share the server's ID direction. Client
// Prompt IDs are separate and never enter this bounded original-ID registry.
func (d *Driver) registerCursorRequest(key, method, sha string) (bool, error) {
	if d.cursor == nil {
		return true, nil
	}
	if old, ok := d.cursor.seen[key]; ok {
		if old.method != method || old.sha != sha {
			return false, errors.New("cursor_input_identity_changed")
		}
		return false, nil
	}
	if key == "" || len(d.cursor.seen) >= 32 {
		return false, errors.New("cursor_input_limit")
	}
	d.cursor.seen[key] = cursorNativeRequest{method, sha}
	return true, nil
}
func (d *Driver) acceptCursorInput(m map[string]json.RawMessage) error {
	method := stringField(m, "method")
	wireID := m["id"]
	key := idKey(wireID)
	q, p, err := cursorPayload(method, m["params"])
	if err != nil {
		return err
	}
	var value any
	if json.Unmarshal(m["params"], &value) != nil {
		return errors.New("cursor_input_invalid")
	}
	if _, secret := safeValue(value, d.options.Secrets); secret {
		return errors.New("cursor_input_secret")
	}
	id, err := cursorRPCID(wireID)
	if err != nil || hasSecret(id.Value, d.options.Secrets) {
		return errors.New("cursor_input_invalid")
	}
	d.mu.Lock()
	if d.cursor == nil || d.discovery != nil || d.cursor.handler.Open == nil || !d.result.Evidence.PromptStarted || !d.result.Evidence.ModelSelectionAcknowledged || d.promptResponse || d.bad != nil || m["result"] != nil || m["error"] != nil {
		d.mu.Unlock()
		return errors.New("cursor_input_unavailable")
	}
	r := cu.CursorInputRequest{RequestID: id, SessionID: d.result.Evidence.SessionID, PromptID: d.cursor.prompt, Method: method, ConfigurationSHA256: d.options.ConfigSHA, Questions: q, Plan: p}
	r.ParamsSHA256, err = cu.ParamsDigest(r)
	if err != nil || cu.ValidateRequest(r) != nil {
		d.mu.Unlock()
		return errors.New("cursor_input_invalid")
	}
	fresh, err := d.registerCursorRequest(key, method, r.ParamsSHA256)
	if err != nil || !fresh {
		d.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithCancel(d.ctx)
	record := &cursorInputRecord{ctx: ctx, cancel: cancel, key: key, wireID: append(json.RawMessage(nil), wireID...), request: r, observation: delivery.Observation{Sequence: 1, ParamsSHA256: r.ParamsSHA256}, changed: make(chan struct{}, 1)}
	d.cursor.records[key] = record
	d.cursor.order = append(d.cursor.order, record)
	d.cursor.wait.Add(1)
	d.mu.Unlock()
	go d.runCursorInput(record)
	return nil
}
func cursorChanged(r *cursorInputRecord) {
	r.observation.Sequence++
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (d *Driver) persistCursorInput(r *cursorInputRecord) bool {
	d.mu.Lock()
	o, old, pending, id := r.observation, r.persisted, r.intentPending, r.inputID
	d.mu.Unlock()
	if pending || o.Sequence == old {
		return true
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.ctx), 3*time.Second)
	err := d.cursor.handler.Observe(ctx, r.request, id, o)
	cancel()
	d.mu.Lock()
	if err == nil && o.Sequence > r.persisted {
		r.persisted = o.Sequence
	}
	d.syncCursorEvidence()
	d.mu.Unlock()
	if err != nil {
		d.fail("cursor_input_observation_unknown")
		return false
	}
	return true
}
func (d *Driver) runCursorInput(r *cursorInputRecord) {
	defer d.cursor.wait.Done()
	select {
	case <-d.promptSent:
	case <-d.ctx.Done():
		return
	}
	h := d.cursor.handler
	openCtx, openCancel := context.WithTimeout(context.WithoutCancel(d.ctx), 3*time.Second)
	id, err := h.Open(openCtx, r.request)
	openCancel()
	d.mu.Lock()
	r.inputID = id
	closed := r.observation.ExecutionClosed
	d.mu.Unlock()
	if err != nil || id == "" {
		d.mu.Lock()
		r.observation.FailureCode = "cursor_input_open_unknown"
		cursorChanged(r)
		d.mu.Unlock()
		_ = d.persistCursorInput(r)
		d.fail("cursor_input_open_unknown")
		return
	}
	if !d.persistCursorInput(r) {
		return
	}
	if !closed {
		toolID := ""
		if r.request.Questions != nil {
			toolID = r.request.Questions.ToolCallID
		} else {
			toolID = r.request.Plan.ToolCallID
		}
		if d.emit(Event{Kind: "input", SessionID: r.request.SessionID, ToolCallID: toolID}) != nil {
			return
		}
		answer, e := h.Answer(r.ctx, r.request, id)
		d.mu.Lock()
		closed = r.observation.ExecutionClosed
		d.mu.Unlock()
		if !closed {
			if e != nil || answer == nil {
				d.cursorInputFailure(r, "cursor_input_answer_unavailable")
				return
			}
			sha, e := cu.AnswerDigest(r.request, answer.Decision)
			if e != nil || sha != answer.AnswerSHA256 {
				d.cursorInputFailure(r, "cursor_input_answer_invalid")
				return
			}
			response, e := cu.BuildResponse(r.request, answer.Decision)
			if e != nil {
				d.cursorInputFailure(r, "cursor_input_answer_invalid")
				return
			}
			d.mu.Lock()
			copy := *answer
			r.answer = &copy
			r.observation.AnswerObserved = true
			r.observation.AnswerSHA256 = sha
			cursorChanged(r)
			d.mu.Unlock()
			if !d.persistCursorInput(r) {
				return
			}
			raw, _ := json.Marshal(struct {
				JSONRPC string                     `json:"jsonrpc"`
				ID      json.RawMessage            `json:"id"`
				Result  cu.CursorInputNativeResult `json:"result"`
			}{"2.0", r.wireID, response})
			d.mu.Lock()
			closed = r.observation.ExecutionClosed
			if !closed {
				r.queued = true
			}
			d.mu.Unlock()
			if !closed {
				select {
				case d.output <- outbound{raw: append(raw, '\n'), cursorReply: r.key}:
				case <-r.ctx.Done():
					d.mu.Lock()
					r.queued = false
					r.writeDone = true
					cursorChanged(r)
					d.mu.Unlock()
				}
			}
		}
	}
	for {
		if !d.persistCursorInput(r) {
			return
		}
		d.mu.Lock()
		settled := (!r.queued || r.writeDone) && r.persisted == r.observation.Sequence
		closed = r.observation.ExecutionClosed
		failed := r.observation.FailureCode != ""
		d.mu.Unlock()
		if settled && (closed || failed) {
			return
		}
		select {
		case <-r.changed:
		case <-r.ctx.Done():
			d.mu.Lock()
			if !r.observation.ExecutionClosed && r.observation.FailureCode == "" {
				r.observation.FailureCode = "cursor_input_cancelled"
				cursorChanged(r)
			}
			pending := r.queued && !r.writeDone
			d.mu.Unlock()
			if pending {
				select {
				case <-r.changed:
				case <-time.After(1500 * time.Millisecond):
					d.mu.Lock()
					if !r.writeDone {
						r.observation.FailureCode = "cursor_input_reply_unknown"
						r.writeDone = true
						cursorChanged(r)
					}
					d.mu.Unlock()
				}
			}
		}
	}
}
func (d *Driver) cursorInputFailure(r *cursorInputRecord, code string) {
	d.mu.Lock()
	r.observation.FailureCode = code
	cursorChanged(r)
	d.mu.Unlock()
	_ = d.persistCursorInput(r)
	d.fail(code)
}
func (d *Driver) beforeCursorWrite(key string) error {
	d.mu.Lock()
	r := d.cursor.records[key]
	if r == nil || r.answer == nil || r.started || r.observation.ExecutionClosed || d.bad != nil || d.localClosed || d.ctx.Err() != nil || r.ctx.Err() != nil {
		if r != nil {
			r.writeDone = true
			cursorChanged(r)
		}
		d.mu.Unlock()
		return errors.New("cursor_input_closed")
	}
	intent := r.observation
	intent.Sequence++
	intent.ReplyStarted = true
	r.intentPending = true
	answer := *r.answer
	d.mu.Unlock()
	err := d.cursor.handler.BeforeWrite(r.ctx, r.request, r.inputID, answer, intent)
	d.mu.Lock()
	r.intentPending = false
	if err == nil {
		r.observation = intent
		r.persisted = intent.Sequence
		r.started = true
	}
	if r.stagedClosed {
		r.observation.ExecutionClosed = true
		r.stagedClosed = false
		cursorChanged(r)
	}
	// Slow durable IO may race native closure or a local stop. A saved intent
	// remains a fact, but it never permits a new write after either has won.
	if err != nil || r.observation.ExecutionClosed || d.bad != nil || d.localClosed || d.ctx.Err() != nil || r.ctx.Err() != nil {
		r.writeDone = true
		if r.started {
			r.observation.FailureCode = "cursor_input_reply_unknown"
		}
		cursorChanged(r)
		d.mu.Unlock()
		return errors.New("cursor_input_write_refused")
	}
	d.mu.Unlock()
	return nil
}
func (d *Driver) afterCursorWrite(key string, complete bool) {
	d.mu.Lock()
	r := d.cursor.records[key]
	if r != nil {
		r.writeDone = true
		if complete {
			r.observation.ReplyWritten = true
		} else {
			r.observation.FailureCode = "cursor_input_reply_unknown"
		}
		cursorChanged(r)
	}
	d.mu.Unlock()
}

// Execution closure is independent of a reply ACK, which Cursor does not send.
func (d *Driver) closeCursorInputsLocked() {
	if d.cursor == nil {
		return
	}
	for _, r := range d.cursor.order {
		if r.intentPending {
			r.stagedClosed = true
		} else if !r.observation.ExecutionClosed {
			r.observation.ExecutionClosed = true
			cursorChanged(r)
		}
		r.cancel()
	}
}
func (d *Driver) finishCursorInputs() {
	if d.cursor == nil {
		return
	}
	d.mu.Lock()
	d.closeCursorInputsLocked()
	d.mu.Unlock()
	d.cursor.wait.Wait()
	for _, r := range d.cursor.order {
		_ = d.persistCursorInput(r)
	}
}
func (d *Driver) syncCursorEvidence() {
	if d.cursor == nil {
		return
	}
	out := []cu.CursorInputEvidence{}
	for _, r := range d.cursor.order {
		if r.inputID == "" || r.persisted == 0 {
			continue
		}
		kind, count := "plan", 0
		if r.request.Questions != nil {
			kind = "questions"
			count = len(r.request.Questions.Questions)
		}
		o := r.observation
		if r.persisted != o.Sequence {
			continue
		}
		out = append(out, cu.CursorInputEvidence{InputID: r.inputID, RequestID: r.request.RequestID, PromptID: r.request.PromptID, SessionID: r.request.SessionID, Method: r.request.Method, ConfigurationSHA256: r.request.ConfigurationSHA256, ParamsSHA256: r.request.ParamsSHA256, Kind: kind, QuestionCount: count, Observation: &o})
	}
	d.result.Evidence.UserInputs = out
}
func (d *Driver) cursorInputsCompleteLocked() bool {
	if d.cursor == nil {
		return true
	}
	for _, r := range d.cursor.order {
		o := r.observation
		if r.inputID == "" || r.persisted != o.Sequence || !o.AnswerObserved || !o.ReplyWritten || !o.ExecutionClosed || o.FailureCode != "" {
			return false
		}
	}
	return true
}
