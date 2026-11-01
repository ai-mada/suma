package runner

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

type codexNativeRequest struct{ method, paramsSHA string }

// Called under the stdout lock. Both independent domains share the native ID
// namespace; an identical repeat never repeats a human operation or response.
func (d *codexAppServer) registerNativeRequest(id cc.CodexRPCRequestID, method, sha string) (bool, error) {
	if d.nativeRequests == nil {
		d.nativeRequests = map[string]codexNativeRequest{}
	}
	key := id.Kind + ":" + id.Value
	if prior, ok := d.nativeRequests[key]; ok {
		if prior.method != method || prior.paramsSHA != sha {
			return false, errToolInput
		}
		return false, nil
	}
	if len(d.nativeRequests) >= 32 {
		return false, errToolInput
	}
	d.nativeRequests[key] = codexNativeRequest{method: method, paramsSHA: sha}
	return true, nil
}

type codexUserInputHandler struct {
	Open        func(context.Context, cc.CodexUserInputRequest) (string, error)
	Answer      func(context.Context, cc.CodexUserInputRequest, string) (*ic.ToolInputAnswer, error)
	BeforeWrite func(context.Context, cc.CodexUserInputRequest, string, ic.ToolInputAnswer, cc.CodexUserInputObservation) error
	Observe     func(context.Context, cc.CodexUserInputRequest, string, cc.CodexUserInputObservation) error
}
type codexUserInputRecord struct {
	ctx                              context.Context
	cancel                           context.CancelFunc
	key, inputID                     string
	wireID                           any
	request                          cc.CodexUserInputRequest
	answer                           *ic.ToolInputAnswer
	observation                      cc.CodexUserInputObservation
	persisted                        int64
	queued, started, writeDone       bool
	intentPending                    bool
	stagedResolved, stagedTurnClosed bool
	changed                          chan struct{}
	done                             chan struct{}
}
type codexUserInputSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	handler  codexUserInputHandler
	records  map[string]*codexUserInputRecord
	order    []*codexUserInputRecord
	terminal map[string]any
}

func (d *codexAppServer) configureUserInputs(ctx context.Context, h codexUserInputHandler) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx == nil || d.frames != 0 || d.userInputs != nil || d.binding == nil || d.spec.CodexAppServer == nil || !d.spec.CodexAppServer.EnableUserInput || d.stream == nil || d.stream.profile.UserInputProtocol != cc.UserInputProtocol || h.Open == nil || h.Answer == nil || h.BeforeWrite == nil || h.Observe == nil {
		return errToolInput
	}
	child, cancel := context.WithCancel(ctx)
	d.userInputs = &codexUserInputSession{ctx: child, cancel: cancel, handler: h, records: map[string]*codexUserInputRecord{}}
	return nil
}
func userInputShape(value map[string]any, required, optional []string) bool {
	allowed := map[string]bool{}
	for _, key := range required {
		if _, ok := value[key]; !ok {
			return false
		}
		allowed[key] = true
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range value {
		if !allowed[key] {
			return false
		}
	}
	return true
}
func (d *codexAppServer) projectUserInput(id cc.CodexRPCRequestID, p map[string]any) (cc.CodexUserInputRequest, error) {
	r := cc.CodexUserInputRequest{RequestID: id, Method: cc.UserInputMethod, ThreadID: textAt(p, "threadId"), TurnID: textAt(p, "turnId"), ItemID: textAt(p, "itemId"), BindingSHA256: d.binding.BindingSHA256, ParamsSHA256: codexApprovalSHA(p)}
	if !userInputShape(p, []string{"threadId", "turnId", "itemId", "isBlocking", "questions"}, []string{"autoResolutionMs"}) {
		return r, errToolInput
	}
	blocking, ok := p["isBlocking"].(bool)
	if !ok {
		return r, errToolInput
	}
	r.IsBlocking = blocking
	if value, present := p["autoResolutionMs"]; present && value != nil {
		number, ok := value.(json.Number)
		if !ok {
			return r, errToolInput
		}
		n, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != number.String() || n < 0 || n > cc.MaxSafeInteger {
			return r, errToolInput
		}
		r.AutoResolutionMS = &n
	}
	questions, ok := p["questions"].([]any)
	if !ok || len(questions) < 1 || len(questions) > 3 {
		return r, errToolInput
	}
	for _, value := range questions {
		q, ok := value.(map[string]any)
		if !ok || !userInputShape(q, []string{"id", "header", "question"}, []string{"isOther", "isSecret", "options"}) {
			return r, errToolInput
		}
		out := cc.CodexUserInputQuestion{Options: []cc.CodexUserInputOption{}}
		for _, field := range []struct {
			key    string
			target *string
		}{{"id", &out.ID}, {"header", &out.Header}, {"question", &out.Question}} {
			text, ok := q[field.key].(string)
			if !ok {
				return r, errToolInput
			}
			if _, unchanged := d.approvalText(text); !unchanged {
				return r, errToolInput
			}
			*field.target = text
		}
		for _, field := range []struct {
			key    string
			target *bool
		}{{"isOther", &out.IsOther}, {"isSecret", &out.IsSecret}} {
			if value, present := q[field.key]; present {
				flag, ok := value.(bool)
				if !ok {
					return r, errToolInput
				}
				*field.target = flag
			}
		}
		// Secret requests are rejected before any record, callback or HTTP body.
		if out.IsSecret {
			return r, errToolInput
		}
		if value, present := q["options"]; present && value != nil {
			options, ok := value.([]any)
			if !ok || len(options) > 12 {
				return r, errToolInput
			}
			for _, value := range options {
				option, ok := value.(map[string]any)
				if !ok || !userInputShape(option, []string{"label", "description"}, nil) {
					return r, errToolInput
				}
				label, lok := option["label"].(string)
				description, dok := option["description"].(string)
				if !lok || !dok {
					return r, errToolInput
				}
				if _, ok := d.approvalText(label); !ok {
					return r, errToolInput
				}
				if _, ok := d.approvalText(description); !ok {
					return r, errToolInput
				}
				out.Options = append(out.Options, cc.CodexUserInputOption{Label: label, Description: description})
			}
		}
		r.Questions = append(r.Questions, out)
	}
	if cc.ValidateUserInputRequest(r) != nil {
		return r, errToolInput
	}
	return r, nil
}
func (d *codexAppServer) receiveUserInput(wireID any, p map[string]any) {
	id, key, err := codexRPCIdentity(wireID)
	if err != nil || d.userInputs == nil || d.evidence == nil || d.stream == nil || d.stream.id(id.Value) != id.Value || d.stopping.Load() || d.terminalReceived || d.nativeTurnClosed || !d.evidence.ConfigurationAcknowledged || d.thread == "" || d.turn == "" || textAt(p, "threadId") != d.thread || textAt(p, "turnId") != d.turn {
		d.fail("user input has no enabled active native source")
		return
	}
	r, err := d.projectUserInput(id, p)
	if err != nil {
		d.fail("user input request is unsupported or unsafe")
		return
	}
	fresh, err := d.registerNativeRequest(id, cc.UserInputMethod, r.ParamsSHA256)
	if err != nil {
		d.fail("native server request identity changed or exceeded its bound")
		return
	}
	if !fresh {
		return
	}
	ctx, cancel := context.WithCancel(d.userInputs.ctx)
	record := &codexUserInputRecord{ctx: ctx, cancel: cancel, key: key, wireID: wireID, request: r, observation: cc.CodexUserInputObservation{ParamsSHA256: r.ParamsSHA256}, changed: make(chan struct{}, 1), done: make(chan struct{})}
	d.userInputs.records[key] = record
	d.userInputs.order = append(d.userInputs.order, record)
	go d.runUserInput(d.userInputs, record)
}
func (d *codexAppServer) userInputChanged(r *codexUserInputRecord) {
	r.observation.Sequence++
	d.syncUserInputEvidence()
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
func (d *codexAppServer) userInputActive(r *codexUserInputRecord) bool {
	return d.userInputs != nil && d.userInputs.records[r.key] == r && !r.observation.RequestResolved && !r.observation.TurnClosed && !r.stagedResolved && !r.stagedTurnClosed && d.bad == nil && !d.stopping.Load() && !d.terminalReceived && !d.nativeTurnClosed && d.thread == r.request.ThreadID && d.turn == r.request.TurnID
}
func (d *codexAppServer) runUserInput(s *codexUserInputSession, r *codexUserInputRecord) {
	defer close(r.done)
	defer r.cancel()
	id, err := s.handler.Open(r.ctx, r.request)
	d.mu.Lock()
	r.inputID = id
	closed := r.observation.RequestResolved || r.observation.TurnClosed
	d.mu.Unlock()
	if err == nil && !closed {
		answer, answerErr := s.handler.Answer(r.ctx, r.request, id)
		if answerErr == nil && answer == nil {
			answerErr = errToolInput
		}
		d.mu.Lock()
		closed = r.observation.RequestResolved || r.observation.TurnClosed
		if answerErr == nil && answer != nil {
			if sha, buildErr := cc.UserInputAnswerDigest(r.request, answer.Answers); buildErr != nil || sha != answer.AnswerSHA256 {
				answerErr = errToolInput
			} else {
				raw, _ := json.Marshal(answer)
				_ = json.Unmarshal(raw, &r.answer)
				r.observation.AnswerObserved = true
				r.observation.AnswerSHA256 = answer.AnswerSHA256
				d.userInputChanged(r)
			}
		}
		if answerErr != nil && !closed {
			err = answerErr
		}
		d.mu.Unlock()
	}
	d.mu.Lock()
	if err != nil || id == "" {
		r.observation.FailureCode = "user_input_source_unknown"
		d.userInputChanged(r)
		d.fail("user input source requires reconciliation")
	}
	if !d.userInputActive(r) && !(r.observation.RequestResolved || r.observation.TurnClosed) && r.observation.FailureCode == "" {
		r.observation.FailureCode = "user_input_cancelled"
		d.userInputChanged(r)
	}
	d.mu.Unlock()
	if !d.persistUserInput(s, r) {
		return
	}
	d.mu.Lock()
	shouldWrite := d.userInputActive(r) && r.answer != nil && r.observation.FailureCode == ""
	if shouldWrite {
		r.queued = true
	}
	d.mu.Unlock()
	if shouldWrite {
		response, buildErr := cc.BuildUserInputResponse(r.request, r.answer.Answers)
		if buildErr == nil {
			buildErr = d.input.enqueueReply(map[string]any{"id": r.wireID, "result": response}, func() error { return d.beforeUserInputWrite(s, r) }, func(err error) { d.afterUserInputWrite(r, err) })
		}
		if buildErr != nil {
			d.mu.Lock()
			r.writeDone = true
			r.observation.FailureCode = "user_input_reply_unavailable"
			d.userInputChanged(r)
			d.fail("user input reply unavailable")
			d.mu.Unlock()
		}
	}
	for {
		if !d.persistUserInput(s, r) {
			return
		}
		d.mu.Lock()
		settled := (!r.queued || r.writeDone) && r.persisted == r.observation.Sequence
		failed := r.observation.FailureCode != ""
		// A resolved request can precede the final native turn by a long time.
		// Keep its observer alive to persist that later independent turn fact.
		if settled && (r.observation.TurnClosed || failed) {
			if !d.userInputClosed(r) {
				d.fail("user input delivery requires reconciliation")
			}
			d.releaseUserInputTerminal()
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
		select {
		case <-r.changed:
		case <-s.ctx.Done():
			d.mu.Lock()
			if r.observation.FailureCode == "" && !(r.observation.RequestResolved || r.observation.TurnClosed) {
				r.observation.FailureCode = "user_input_cancelled"
				d.userInputChanged(r)
			}
			d.mu.Unlock()
			// Drain the actual writer callback before allowing the evidence to end.
			select {
			case <-r.changed:
			case <-time.After(1500 * time.Millisecond):
				d.mu.Lock()
				if r.queued && !r.writeDone {
					r.observation.FailureCode = "user_input_reply_unknown"
					d.userInputChanged(r)
					d.fail("user input writer did not settle")
				}
				d.mu.Unlock()
				_ = d.persistUserInput(s, r)
				return
			}
		}
	}
}
func (d *codexAppServer) persistUserInput(s *codexUserInputSession, r *codexUserInputRecord) bool {
	d.mu.Lock()
	o, persisted, intentPending := r.observation, r.persisted, r.intentPending
	id := r.inputID
	d.mu.Unlock()
	if intentPending || o.Sequence == 0 || o.Sequence == persisted {
		return true
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 3*time.Second)
	err := s.handler.Observe(ctx, r.request, id, o)
	cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		d.fail("user input durable observation unavailable")
		return false
	}
	if o.Sequence > r.persisted {
		r.persisted = o.Sequence
	}
	d.syncUserInputEvidence()
	d.releaseUserInputTerminal()
	return true
}
func (d *codexAppServer) beforeUserInputWrite(s *codexUserInputSession, r *codexUserInputRecord) error {
	d.mu.Lock()
	if !d.userInputActive(r) || r.started || r.answer == nil {
		d.mu.Unlock()
		return errToolInput
	}
	intent := r.observation
	intent.Sequence++
	intent.ReplyStarted = true
	r.intentPending = true
	answer := *r.answer
	d.mu.Unlock()
	err := s.handler.BeforeWrite(r.ctx, r.request, r.inputID, answer, intent)
	d.mu.Lock()
	defer d.mu.Unlock()
	r.intentPending = false
	lateFailure := r.observation.FailureCode
	if err == nil {
		r.observation = intent
		r.persisted = intent.Sequence
	}
	if err == nil && lateFailure != "" {
		r.observation.FailureCode = lateFailure
		d.userInputChanged(r)
	}
	// Native closure can race the slow source poll/save. Stage it until that
	// intent settles, so it cannot consume the same monotonic sequence number.
	if r.stagedResolved || r.stagedTurnClosed {
		r.observation.RequestResolved = r.observation.RequestResolved || r.stagedResolved
		r.observation.TurnClosed = r.observation.TurnClosed || r.stagedTurnClosed
		r.stagedResolved, r.stagedTurnClosed = false, false
		d.userInputChanged(r)
	}
	if err != nil {
		return err
	}
	if r.observation.RequestResolved || r.observation.TurnClosed {
		return errToolInput
	}
	if !d.userInputActive(r) {
		return errToolInput
	}
	r.started = true
	d.syncUserInputEvidence()
	return nil
}
func (d *codexAppServer) afterUserInputWrite(r *codexUserInputRecord, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r.writeDone = true
	if err == nil && r.started && r.observation.ReplyStarted {
		r.observation.ReplyWritten = true
	} else {
		if r.observation.FailureCode == "" {
			r.observation.FailureCode = "user_input_reply_unknown"
		}
		d.fail("user input reply requires reconciliation")
	}
	d.userInputChanged(r)
}
func (d *codexAppServer) resolveUserInput(p map[string]any) {
	_, key, err := codexRPCIdentity(p["requestId"])
	if err != nil || d.userInputs == nil || textAt(p, "threadId") != d.thread {
		d.fail("user input resolution scope differs")
		return
	}
	r := d.userInputs.records[key]
	if r == nil {
		d.fail("user input resolution identity differs")
		return
	}
	if !r.observation.RequestResolved && !r.stagedResolved {
		if r.intentPending {
			r.stagedResolved = true
		} else {
			r.observation.RequestResolved = true
			d.userInputChanged(r)
		}
		if !r.started {
			r.cancel()
		}
	}
}
func (d *codexAppServer) closeUserInputTurn() {
	if d.userInputs == nil {
		return
	}
	for _, r := range d.userInputs.order {
		if !r.observation.TurnClosed && !r.stagedTurnClosed {
			if r.intentPending {
				r.stagedTurnClosed = true
			} else {
				r.observation.TurnClosed = true
				d.userInputChanged(r)
			}
			if !r.started {
				r.cancel()
			}
		}
	}
}
func (d *codexAppServer) userInputClosed(r *codexUserInputRecord) bool {
	o := r.observation
	return r.inputID != "" && o.Sequence > 0 && r.persisted == o.Sequence && o.FailureCode == "" && (o.RequestResolved || o.TurnClosed) && (!o.ReplyStarted || o.ReplyWritten) && (!r.queued || r.writeDone)
}
func (d *codexAppServer) userInputsClosed() bool {
	if d.userInputs == nil {
		return true
	}
	for _, r := range d.userInputs.order {
		if !d.userInputClosed(r) {
			return false
		}
	}
	return true
}
func (d *codexAppServer) deferUserInputTerminal(p map[string]any) bool {
	if d.userInputsClosed() {
		return false
	}
	if d.userInputs.terminal != nil {
		d.fail("multiple turn terminals with pending user inputs")
		return true
	}
	d.userInputs.terminal = p
	return true
}
func (d *codexAppServer) releaseUserInputTerminal() {
	if d.userInputs != nil && d.userInputs.terminal != nil && d.userInputsClosed() && d.approvalsClosed() {
		p := d.userInputs.terminal
		d.userInputs.terminal = nil
		d.notification("turn/completed", p)
	}
}
func (d *codexAppServer) stopUserInputs() {
	if d.userInputs != nil {
		d.userInputs.cancel()
	}
}
func (d *codexAppServer) finishUserInputs() {
	if d.userInputs == nil {
		return
	}
	d.stopUserInputs()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for _, r := range d.userInputs.order {
		select {
		case <-r.done:
		case <-deadline.C:
			d.mu.Lock()
			d.fail("user input facts did not settle before process exit")
			d.mu.Unlock()
			return
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.userInputsClosed() {
		d.fail("user input facts are incomplete at process exit")
	}
}
func (d *codexAppServer) syncUserInputEvidence() {
	if d.userInputs == nil || d.evidence == nil {
		return
	}
	d.evidence.UserInputs = []cc.CodexUserInputEvidence{}
	for _, r := range d.userInputs.order {
		if r.inputID == "" {
			continue
		}
		var observation *cc.CodexUserInputObservation
		if r.observation.Sequence > 0 {
			copy := r.observation
			observation = &copy
		}
		d.evidence.UserInputs = append(d.evidence.UserInputs, cc.CodexUserInputEvidence{InputID: r.inputID, RequestID: r.request.RequestID, Method: r.request.Method, ThreadID: r.request.ThreadID, TurnID: r.request.TurnID, ItemID: r.request.ItemID, BindingSHA256: r.request.BindingSHA256, ParamsSHA256: r.request.ParamsSHA256, QuestionCount: len(r.request.Questions), IsBlocking: r.request.IsBlocking, Observation: observation})
	}
}
