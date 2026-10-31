package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

var errCodexApproval = errors.New("Codex approval requires reconciliation")

// Native callbacks deliberately do not impersonate ACP session/options. Runtime
// owns the durable platform permission and current human decision checks.
type codexApprovalDecision struct {
	PermissionID string
	Choice       cc.CodexApprovalChoice
}
type codexApprovalHandler struct {
	Decide      func(context.Context, cc.CodexApprovalRequest) (codexApprovalDecision, error)
	BeforeWrite func(context.Context, cc.CodexApprovalRequest, codexApprovalDecision, cc.CodexApprovalObservation) error
	Observe     func(context.Context, cc.CodexApprovalRequest, codexApprovalDecision, cc.CodexApprovalObservation) error
}
type codexApprovalItem struct {
	raw    map[string]any
	sha    string
	kind   string
	status string
}
type codexApprovalRecord struct {
	ctx                    context.Context
	cancel                 context.CancelFunc
	key, method, paramsSHA string
	id                     cc.CodexRPCRequestID
	wireID                 any
	params                 map[string]any
	item                   *codexApprovalItem
	request                cc.CodexApprovalRequest
	decision               codexApprovalDecision
	observation            cc.CodexApprovalObservation
	persisted              int64
	started, written       bool
	queued, writeDone      bool
	stagedResolved         bool
	stagedItemStatus       string
	updates                chan cc.CodexApprovalObservation
	done                   chan struct{}
}
type codexApprovalSession struct {
	ctx        context.Context
	cancel     context.CancelFunc
	handler    codexApprovalHandler
	records    map[string]*codexApprovalRecord
	order      []*codexApprovalRecord
	items      map[string]*codexApprovalItem
	terminal   map[string]any
	changed    chan struct{}
	finishing  chan struct{}
	finishOnce sync.Once
}

func (d *codexAppServer) configureApprovals(ctx context.Context, handler codexApprovalHandler) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ctx == nil || d.approvals != nil || d.frames != 0 || d.waiting != "initialize" || d.binding == nil || handler.Decide == nil || handler.BeforeWrite == nil || handler.Observe == nil {
		return errCodexApproval
	}
	child, cancel := context.WithCancel(ctx)
	d.approvals = &codexApprovalSession{ctx: child, cancel: cancel, handler: handler, records: map[string]*codexApprovalRecord{}, items: map[string]*codexApprovalItem{}, changed: make(chan struct{}, 1), finishing: make(chan struct{})}
	return nil
}

func (d *codexAppServer) stopApprovals() {
	if d.approvals != nil {
		d.approvals.cancel()
	}
}
func (d *codexAppServer) endApprovalInput() {
	if d.approvals != nil {
		d.approvals.finishOnce.Do(func() { close(d.approvals.finishing) })
	}
}

func codexRPCIdentity(value any) (cc.CodexRPCRequestID, string, error) {
	var id cc.CodexRPCRequestID
	switch value := value.(type) {
	case string:
		if value == "" || !utf8.ValidString(value) || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n") {
			return id, "", errCodexApproval
		}
		id.Kind, id.Value = "string", value
	case json.Number:
		n, err := strconv.ParseInt(value.String(), 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != value.String() {
			return id, "", errCodexApproval
		}
		id.Kind, id.Value = "number", value.String()
	default:
		return id, "", errCodexApproval
	}
	if cc.ValidateRPCRequestID(id) != nil {
		return id, "", errCodexApproval
	}
	return id, id.Kind + ":" + id.Value, nil
}

func codexApprovalSHA(value any) string { raw, _ := json.Marshal(value); return digest(raw) }

// Caller holds d.mu. Only immutable bounded data crosses to the async worker.
func (d *codexAppServer) receiveApproval(wireID any, method string, params map[string]any) {
	s := d.approvals
	id, key, err := codexRPCIdentity(wireID)
	if err != nil || s == nil || d.evidence == nil || d.stream == nil || d.stream.id(id.Value) != id.Value || d.stopping.Load() || d.terminalReceived || !d.evidence.ConfigurationAcknowledged || d.thread == "" || d.turn == "" || textAt(params, "threadId") != d.thread || textAt(params, "turnId") != d.turn {
		d.fail("approval request has no active native source")
		return
	}
	paramsSHA := codexApprovalSHA(params)
	if existing := s.records[key]; existing != nil {
		if existing.method != method || existing.paramsSHA != paramsSHA {
			d.fail("approval request identity changed")
		}
		return // An identical callback never repeats a human command or reply.
	}
	item := s.items[textAt(params, "itemId")]
	started, ok := params["startedAtMs"].(json.Number)
	ms, msErr := strconv.ParseInt(string(started), 10, 64)
	if len(s.records) >= 32 || item == nil || item.status != "inProgress" || !ok || msErr != nil || ms <= 0 || ms > cc.MaxSafeInteger || strconv.FormatInt(ms, 10) != string(started) || !validApprovalParamsShape(method, params) || (method == "item/commandExecution/requestApproval") != (item.kind == "commandExecution") {
		d.fail("approval callback item or bounded timestamp is invalid")
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	r := &codexApprovalRecord{ctx: ctx, cancel: cancel, key: key, method: method, paramsSHA: paramsSHA, id: id, wireID: wireID, params: params, item: item, updates: make(chan cc.CodexApprovalObservation, 8), done: make(chan struct{})}
	s.records[key], s.order = r, append(s.order, r)
	d.syncApprovalEvidence()
	go d.runApproval(s, r)
}

func (d *codexAppServer) runApproval(s *codexApprovalSession, r *codexApprovalRecord) {
	defer r.cancel()
	defer func() {
		close(r.done)
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}()
	request, err := d.projectApproval(r)
	if err != nil {
		d.approvalFailure(r, "approval_request_invalid")
		return
	}
	d.mu.Lock()
	r.request = request
	d.syncApprovalEvidence()
	active := !d.stopping.Load() && d.bad == nil && !d.terminalReceived
	d.mu.Unlock()
	if !active {
		d.approvalWithoutDecision(s, r, "approval_cancelled")
		d.publishApprovalUpdates(s, r)
		return
	}
	decision, err := s.handler.Decide(r.ctx, request)
	if err != nil || decision.PermissionID == "" || !cc.ApprovalChoiceMatchesRequest(request, decision.Choice) {
		d.approvalWithoutDecision(s, r, "approval_decision_unknown")
		d.publishApprovalUpdates(s, r)
		return
	}
	d.mu.Lock()
	r.decision = decision
	r.observation = cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.paramsSHA, Decision: decision.Choice.Decision, DecisionObserved: true}
	first := r.observation
	d.syncApprovalEvidence()
	d.mu.Unlock()
	if err = s.handler.Observe(r.ctx, request, decision, first); err != nil {
		d.approvalFailure(r, "approval_observation_unknown")
		return
	}
	d.mu.Lock()
	r.persisted = first.Sequence
	d.mu.Unlock()
	d.mu.Lock()
	r.queued = true
	d.mu.Unlock()
	err = d.input.enqueueReply(map[string]any{"id": r.wireID, "result": map[string]any{"decision": decision.Choice.Decision}}, func() error { return d.beforeApprovalWrite(s, r) }, func(err error) { d.afterApprovalWrite(r, err) })
	if err != nil {
		d.approvalFailure(r, "approval_reply_unavailable")
		return
	}
	d.publishApprovalUpdates(s, r)
}

func (d *codexAppServer) publishApprovalUpdates(s *codexApprovalSession, r *codexApprovalRecord) {
	cancelled, finishing := s.ctx.Done(), s.finishing
	finishSeen := false
	for {
		if finishSeen {
			d.mu.Lock()
			settled := !r.queued || r.writeDone
			d.mu.Unlock()
			if settled && len(r.updates) == 0 {
				return
			}
		}
		select {
		case observation := <-r.updates:
			if observation.Sequence == 0 {
				continue
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 3*time.Second)
			err := s.handler.Observe(ctx, r.request, r.decision, observation)
			cancel()
			if err != nil {
				d.approvalFailure(r, "approval_observation_unknown")
				return
			}
			d.mu.Lock()
			r.persisted = observation.Sequence
			d.syncApprovalEvidence()
			d.releaseApprovalTerminal()
			if observation.FailureCode != "" {
				d.fail("native approval transport or source is unconfirmed")
			}
			done := observation.FailureCode == "" && observation.RequestResolved && observation.ItemStatus != "" && r.persisted == r.observation.Sequence
			d.mu.Unlock()
			if done {
				return
			}
		case <-cancelled:
			cancelled = nil
			d.mu.Lock()
			if r.writeDone && (!r.written || !r.stagedResolved || r.stagedItemStatus == "") {
				if r.observation.FailureCode == "" {
					r.observation.FailureCode = "approval_cancelled"
					d.queueApprovalObservation(r)
				}
			}
			d.mu.Unlock()
		case <-finishing:
			finishSeen = true
			finishing = nil
		}
	}
}

func (d *codexAppServer) approvalActive(r *codexApprovalRecord) bool {
	return d.approvals != nil && d.approvals.records[r.key] == r && d.approvals.terminal == nil && !r.stagedResolved && d.bad == nil && !d.terminalReceived && !d.stopping.Load() && d.thread == r.request.ThreadID && d.turn == r.request.TurnID && r.item.status == "inProgress"
}

func (d *codexAppServer) beforeApprovalWrite(s *codexApprovalSession, r *codexApprovalRecord) error {
	d.mu.Lock()
	if !d.approvalActive(r) || r.started {
		d.mu.Unlock()
		return errCodexApproval
	}
	intent := r.observation
	intent.Sequence++
	intent.ReplyStarted = true // Durable start intent, not bytes written.
	d.mu.Unlock()
	if err := s.handler.BeforeWrite(r.ctx, r.request, r.decision, intent); err != nil {
		return errCodexApproval
	}
	d.mu.Lock()
	r.observation = intent
	r.persisted = intent.Sequence
	d.syncApprovalEvidence()
	d.mu.Unlock()
	// Filesystem checks stay outside the stdout lock, including the final
	// pre-write boundary after a potentially slow human/source round trip.
	fresh, err := d.projectApproval(r)
	if err != nil || codexApprovalSHA(fresh) != codexApprovalSHA(r.request) {
		return errCodexApproval
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.approvalActive(r) {
		return errCodexApproval
	}
	r.started = true
	return nil
}

func (d *codexAppServer) afterApprovalWrite(r *codexApprovalRecord, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r.writeDone = true
	if !r.observation.ReplyStarted {
		d.fail("native approval reply did not obtain durable current-source intent")
		select {
		case r.updates <- cc.CodexApprovalObservation{}:
		default:
		}
		return
	}
	if err != nil || !r.started {
		r.observation.FailureCode = "approval_reply_unknown"
	} else {
		r.written, r.observation.ReplyWritten = true, true
		r.observation.RequestResolved, r.observation.ItemStatus = r.stagedResolved, r.stagedItemStatus
	}
	if d.approvals.ctx.Err() != nil && r.observation.FailureCode == "" {
		r.observation.FailureCode = "approval_cancelled"
	}
	d.queueApprovalObservation(r)
}

func (d *codexAppServer) queueApprovalObservation(r *codexApprovalRecord) {
	r.observation.Sequence++
	d.syncApprovalEvidence()
	select {
	case r.updates <- r.observation:
	default:
		d.approvalFailureLocked(r, "approval_observation_overflow")
	}
}

func (d *codexAppServer) approvalFailure(r *codexApprovalRecord, code string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.approvalFailureLocked(r, code)
}
func (d *codexAppServer) approvalFailureLocked(r *codexApprovalRecord, code string) {
	if r.observation.Sequence == 0 {
		r.observation.ParamsSHA256 = r.paramsSHA
	}
	if r.observation.FailureCode == "" {
		r.observation.Sequence++
		r.observation.FailureCode = code
	}
	d.syncApprovalEvidence()
	d.fail("native approval outcome requires reconciliation")
}

func (d *codexAppServer) approvalWithoutDecision(s *codexApprovalSession, r *codexApprovalRecord, code string) {
	d.mu.Lock()
	r.observation = cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.paramsSHA, RequestResolved: r.stagedResolved, ItemStatus: r.stagedItemStatus, FailureCode: code}
	observation := r.observation
	d.syncApprovalEvidence()
	d.mu.Unlock()
	// A cleanup observation is not a human decision. Runtime can recover only
	// the original Open operation and persist it without waiting for a reply.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 3*time.Second)
	err := s.handler.Observe(ctx, r.request, codexApprovalDecision{}, observation)
	cancel()
	d.mu.Lock()
	if err == nil {
		r.persisted = observation.Sequence
	}
	d.fail("native approval has no confirmed human decision")
	d.syncApprovalEvidence()
	d.mu.Unlock()
}

func (d *codexAppServer) resolveApproval(params map[string]any) {
	if d.approvals == nil {
		d.fail("unexpected server request resolution")
		return
	}
	_, key, err := codexRPCIdentity(params["requestId"])
	r := d.approvals.records[key]
	if err != nil || r == nil || textAt(params, "threadId") != d.thread {
		d.fail("approval resolution identity differs")
		return
	}
	if !r.stagedResolved {
		r.stagedResolved = true
		if !r.started {
			r.cancel()
		}
		if r.writeDone || !r.queued && r.observation.Sequence > 0 {
			r.observation.RequestResolved = true
			d.queueApprovalObservation(r)
		}
	}
}

func (d *codexAppServer) observeApprovalItem(params map[string]any, completed bool) {
	if d.approvals == nil {
		return
	}
	item := objectAt(params, "item")
	kind, id := textAt(item, "type"), textAt(item, "id")
	if kind != "commandExecution" && kind != "fileChange" {
		return
	}
	if !d.scoped(params, false) || textAt(params, "turnId") != d.turn || id == "" || len(id) > 256 || d.stream.id(id) != id {
		d.fail("approval item scope is invalid")
		return
	}
	status := textAt(item, "status")
	timestamp := "startedAtMs"
	if completed {
		timestamp = "completedAtMs"
	}
	number, numberOK := params[timestamp].(json.Number)
	ms, msErr := strconv.ParseInt(string(number), 10, 64)
	if !numberOK || msErr != nil || ms <= 0 || ms > cc.MaxSafeInteger || strconv.FormatInt(ms, 10) != string(number) {
		d.fail("approval item timestamp is invalid")
		return
	}
	if !completed {
		if status != "inProgress" || d.approvals.items[id] != nil || len(d.approvals.items) >= 256 {
			d.fail("approval item start is repeated or invalid")
			return
		}
		d.approvals.items[id] = &codexApprovalItem{raw: item, sha: codexApprovalSHA(item), kind: kind, status: status}
		return
	}
	prior := d.approvals.items[id]
	if prior == nil || prior.status != "inProgress" || prior.kind != kind || status != "completed" && status != "failed" && status != "declined" || !sameApprovalItemSource(prior.raw, item) {
		d.fail("approval item completion differs")
		return
	}
	prior.status = status
	for _, r := range d.approvals.order {
		if r.item != prior {
			continue
		}
		r.stagedItemStatus = status
		if !r.started {
			r.cancel()
		}
		if r.writeDone || !r.queued && r.observation.Sequence > 0 {
			r.observation.ItemStatus = status
			d.queueApprovalObservation(r)
		}
	}
}

func sameApprovalItemSource(a, b map[string]any) bool {
	if textAt(a, "type") != textAt(b, "type") || textAt(a, "id") != textAt(b, "id") {
		return false
	}
	if textAt(a, "type") == "commandExecution" {
		return textAt(a, "command") == textAt(b, "command") && textAt(a, "cwd") == textAt(b, "cwd")
	}
	return codexApprovalSHA(a["changes"]) == codexApprovalSHA(b["changes"])
}

func (d *codexAppServer) approvalsClosed() bool {
	if d.approvals == nil {
		return true
	}
	for _, r := range d.approvals.order {
		o := r.observation
		if r.decision.PermissionID == "" || !o.DecisionObserved || !o.ReplyStarted || !o.ReplyWritten || !o.RequestResolved || o.ItemStatus == "" || o.FailureCode != "" || r.persisted != o.Sequence {
			return false
		}
	}
	return true
}
func (d *codexAppServer) releaseApprovalTerminal() {
	if d.approvals != nil && d.approvals.terminal != nil && d.approvalsClosed() {
		params := d.approvals.terminal
		d.approvals.terminal = nil
		d.notification("turn/completed", params)
	}
}
func (d *codexAppServer) deferApprovalTerminal(params map[string]any) bool {
	if d.approvalsClosed() {
		return false
	}
	if d.approvals.terminal != nil {
		d.fail("multiple turn terminals with pending approvals")
		return true
	}
	d.approvals.terminal = params
	for _, r := range d.approvals.order {
		if !r.started {
			r.cancel()
		}
	}
	return true
}
func (d *codexAppServer) finishApprovals() {
	if d.approvals == nil {
		return
	}
	d.endApprovalInput()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	grace := time.NewTimer(1500 * time.Millisecond)
	defer grace.Stop()
	graceElapsed := false
	for {
		d.mu.Lock()
		allDone := true
		for _, r := range d.approvals.order {
			select {
			case <-r.done:
			default:
				allDone = false
			}
		}
		if allDone {
			if !d.approvalsClosed() {
				d.fail("native approval facts are incomplete at process exit")
			}
			d.mu.Unlock()
			return
		}
		canSettle := d.bad == nil
		for _, r := range d.approvals.order {
			if !r.started || r.observation.FailureCode != "" {
				canSettle = false
			}
		}
		changed := d.approvals.changed
		d.mu.Unlock()
		if !canSettle || graceElapsed {
			d.stopApprovals()
		}
		select {
		case <-changed:
		case <-grace.C:
			graceElapsed = true
		case <-deadline.C:
			d.mu.Lock()
			d.fail("native approval facts did not settle before process exit")
			d.mu.Unlock()
			d.stopApprovals()
			return
		}
	}
}

func (d *codexAppServer) syncApprovalEvidence() {
	if d.approvals == nil || d.evidence == nil {
		return
	}
	d.evidence.Approvals = make([]cc.CodexApprovalEvidence, 0, len(d.approvals.order))
	for _, r := range d.approvals.order {
		var observation *cc.CodexApprovalObservation
		if r.observation.Sequence > 0 {
			copy := r.observation
			observation = &copy
		}
		d.evidence.Approvals = append(d.evidence.Approvals, cc.CodexApprovalEvidence{PermissionID: r.decision.PermissionID, RequestID: r.id, Method: r.method, ThreadID: d.thread, TurnID: d.turn, ItemID: textAt(r.params, "itemId"), ApprovalID: textAt(r.params, "approvalId"), ParamsSHA256: r.paramsSHA, Observation: observation})
	}
	select {
	case d.approvals.changed <- struct{}{}:
	default:
	}
}

func (d *codexAppServer) approvalPath(raw string, directory bool) (string, bool) {
	if raw == "" || strings.ContainsRune(raw, 0) {
		return "", false
	}
	raw = filepath.FromSlash(raw)
	if filepath.Clean(raw) != raw {
		return "", false
	}
	rootInfo, rootErr := os.Lstat(d.root)
	if rootErr != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	full := raw
	if !filepath.IsAbs(full) {
		if filepath.VolumeName(full) != "" {
			return "", false
		}
		full = filepath.Join(d.root, full)
	}
	full = filepath.Clean(full)
	if !within(d.root, full) {
		return "", false
	}
	rel, err := filepath.Rel(d.root, full)
	if err != nil {
		return "", false
	}
	current := d.root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) && !directory {
			break
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 || directory) && !info.IsDir() {
			return "", false
		}
		if !directory && i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", false
		}
	}
	return filepath.ToSlash(rel), true
}

func (d *codexAppServer) approvalText(value string) (string, bool) {
	out := value
	for _, secret := range d.stream.secrets {
		if secret != "" {
			out = strings.ReplaceAll(out, secret, "[REDACTED]")
		}
	}
	out = ansi.ReplaceAllString(out, "")
	out = credential.ReplaceAllString(out, "[REDACTED]")
	out = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, out)
	if len(out) > 32768 {
		out = out[:32768]
		for !utf8.ValidString(out) && len(out) > 0 {
			out = out[:len(out)-1]
		}
	}
	return out, out == value && utf8.ValidString(value)
}

func (d *codexAppServer) projectApproval(r *codexApprovalRecord) (cc.CodexApprovalRequest, error) {
	p := r.params
	ms, _ := strconv.ParseInt(string(p["startedAtMs"].(json.Number)), 10, 64)
	request := cc.CodexApprovalRequest{RequestID: r.id, Method: r.method, ThreadID: textAt(p, "threadId"), TurnID: textAt(p, "turnId"), ItemID: textAt(p, "itemId"), StartedAtMS: ms, ApprovalID: textAt(p, "approvalId"), BindingSHA256: d.binding.BindingSHA256, ParamsSHA256: r.paramsSHA, ItemSHA256: r.item.sha, DetailsComplete: true, ScopeSupported: true}
	unsupported := func(reason string) {
		request.ScopeSupported = false
		if request.UnsupportedReason == "" {
			request.UnsupportedReason = reason
		}
	}
	itemKeys := map[string]bool{"id": true, "type": true, "status": true}
	allowed := map[string]bool{"threadId": true, "turnId": true, "itemId": true, "startedAtMs": true, "reason": true}
	if r.item.kind == "commandExecution" {
		request.Title = "Command execution approval"
		for _, key := range []string{"command", "cwd", "commandActions", "aggregatedOutput", "exitCode", "durationMs", "processId", "source", "pluginId", "scriptPath"} {
			itemKeys[key] = true
		}
		for _, key := range []string{"approvalId", "command", "cwd", "kind", "environmentId", "commandActions", "networkApprovalContext", "proposedExecpolicyAmendment", "proposedNetworkPolicyAmendments"} {
			allowed[key] = true
		}
		for _, key := range []string{"networkApprovalContext", "proposedExecpolicyAmendment", "proposedNetworkPolicyAmendments"} {
			if value := p[key]; value != nil {
				if list, ok := value.([]any); !ok || len(list) > 0 {
					unsupported("permission_extension_unsupported")
				}
			}
		}
		if kind := textAt(p, "kind"); kind != "" && kind != "command" {
			unsupported("stdin_approval_unsupported")
		}
		if textAt(p, "environmentId") != "" {
			unsupported("environment_unsupported")
		}
		command, cwd := textAt(r.item.raw, "command"), textAt(r.item.raw, "cwd")
		if textAt(p, "command") != "" && textAt(p, "command") != command || textAt(p, "cwd") != "" && textAt(p, "cwd") != cwd || command == "" {
			return request, errCodexApproval
		}
		var complete bool
		request.Command, complete = d.approvalText(command)
		request.DetailsComplete = request.DetailsComplete && complete
		var ok bool
		request.CWD, ok = d.approvalPath(cwd, true)
		if !ok {
			unsupported("workspace_scope_unsupported")
			request.DetailsComplete = false
		}
	} else {
		request.Title = "File change approval"
		allowed["grantRoot"] = true
		itemKeys["changes"] = true
		if textAt(p, "grantRoot") != "" {
			unsupported("session_write_grant_unsupported")
		}
		changes, ok := r.item.raw["changes"].([]any)
		if !ok || len(changes) > 128 {
			return request, errCodexApproval
		}
		for _, value := range changes {
			change, ok := value.(map[string]any)
			if !ok {
				return request, errCodexApproval
			}
			for key := range change {
				if key != "path" && key != "kind" && key != "diff" {
					unsupported("unknown_item_extension")
				}
			}
			path, pathOK := d.approvalPath(textAt(change, "path"), false)
			if !pathOK {
				unsupported("workspace_scope_unsupported")
				request.DetailsComplete = false
				continue
			}
			kind := objectAt(change, "kind")
			tag := textAt(kind, "type")
			if tag != "add" && tag != "delete" && tag != "update" {
				return request, errCodexApproval
			}
			for key := range kind {
				if key != "type" && key != "move_path" {
					unsupported("unknown_item_extension")
				}
			}
			if _, ok := change["diff"].(string); !ok {
				return request, errCodexApproval
			}
			diff, complete := d.approvalText(textAt(change, "diff"))
			request.DetailsComplete = request.DetailsComplete && complete
			projected := cc.CodexApprovalFileChange{Path: path, Kind: tag, Diff: diff}
			if textAt(kind, "move_path") != "" {
				move, moveOK := d.approvalPath(textAt(kind, "move_path"), false)
				projected.MovePath = move
				if !moveOK {
					unsupported("workspace_scope_unsupported")
					request.DetailsComplete = false
					continue
				}
			}
			request.Changes = append(request.Changes, projected)
		}
	}
	for key := range p {
		if !allowed[key] {
			unsupported("unknown_permission_extension")
		}
	}
	for key := range r.item.raw {
		if !itemKeys[key] {
			unsupported("unknown_item_extension")
		}
	}
	reason, complete := d.approvalText(textAt(p, "reason"))
	request.Reason = reason
	request.DetailsComplete = request.DetailsComplete && complete
	if len(request.Reason) > 4096 {
		request.Reason = request.Reason[:4096]
		for !utf8.ValidString(request.Reason) {
			request.Reason = request.Reason[:len(request.Reason)-1]
		}
		request.DetailsComplete = false
	}
	for _, value := range []string{request.ApprovalID, request.ItemID} {
		if d.stream.id(value) != value {
			return request, errCodexApproval
		}
	}
	details, _ := json.Marshal(struct {
		Command string                       `json:"command,omitempty"`
		Cwd     string                       `json:"cwd,omitempty"`
		Changes []cc.CodexApprovalFileChange `json:"changes,omitempty"`
		Reason  string                       `json:"reason,omitempty"`
	}{request.Command, request.CWD, request.Changes, request.Reason})
	request.Details, complete = d.approvalText(string(details))
	request.DetailsComplete = request.DetailsComplete && complete
	return request, cc.ValidateApprovalRequest(request)
}

func validApprovalParamsShape(method string, p map[string]any) bool {
	stringsFields := []string{"reason"}
	if method == "item/commandExecution/requestApproval" {
		stringsFields = append(stringsFields, "approvalId", "command", "cwd", "environmentId", "kind")
	} else {
		stringsFields = append(stringsFields, "grantRoot")
	}
	for _, key := range stringsFields {
		if value, exists := p[key]; exists && value != nil {
			if _, ok := value.(string); !ok {
				return false
			}
		}
	}
	if method == "item/commandExecution/requestApproval" {
		for _, key := range []string{"commandActions", "proposedExecpolicyAmendment", "proposedNetworkPolicyAmendments"} {
			if value, exists := p[key]; exists && value != nil {
				if _, ok := value.([]any); !ok {
					return false
				}
			}
		}
	}
	return true
}
