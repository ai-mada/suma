package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/buildinfo"
	"io"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// codexInput is a bounded host-owned request queue, not an interactive terminal.
// Closing it drains already queued protocol replies before delivering EOF.
type codexInput struct {
	mu      sync.Mutex
	changed *sync.Cond
	pending []byte
	frames  []codexInputFrame
	closed  bool
}

type codexInputFrame struct {
	raw    []byte
	before func() error
	after  func(error)
}

func newCodexInput() *codexInput { q := &codexInput{}; q.changed = sync.NewCond(&q.mu); return q }
func (q *codexInput) enqueue(value any) error {
	return q.enqueueReply(value, nil, nil)
}
func (q *codexInput) enqueueReply(value any, before func() error, after func(error)) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || len(raw)+len(q.pending) > 128<<10 {
		return fmt.Errorf("Codex request queue unavailable")
	}
	raw = append(raw, '\n')
	q.pending = append(q.pending, raw...)
	q.frames = append(q.frames, codexInputFrame{raw: raw, before: before, after: after})
	q.changed.Broadcast()
	return nil
}
func (q *codexInput) take() (codexInputFrame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.frames) == 0 && !q.closed {
		q.changed.Wait()
	}
	if len(q.frames) == 0 {
		return codexInputFrame{}, false
	}
	frame := q.frames[0]
	q.frames[0] = codexInputFrame{}
	q.frames = q.frames[1:]
	q.pending = q.pending[len(frame.raw):]
	return frame, true
}

// One writer owns stdin. Callbacks run without either queue or stdout locks;
// dequeueing bytes is never evidence that the underlying Write succeeded.
func (q *codexInput) copyTo(w io.Writer) (total int64, err error) {
	for {
		frame, ok := q.take()
		if !ok {
			return total, nil
		}
		if frame.before != nil {
			if err := frame.before(); err != nil {
				if frame.after != nil {
					frame.after(err)
				}
				continue
			}
		}
		n, err := w.Write(frame.raw)
		total += int64(n)
		if err == nil && n != len(frame.raw) {
			err = io.ErrShortWrite
		}
		if frame.after != nil {
			frame.after(err)
		}
		if err != nil {
			q.abort(err)
			return total, err
		}
	}
}
func (q *codexInput) abort(err error) {
	q.mu.Lock()
	frames := q.frames
	q.frames = nil
	q.pending = nil
	q.closed = true
	q.changed.Broadcast()
	q.mu.Unlock()
	for _, frame := range frames {
		if frame.after != nil {
			frame.after(err)
		}
	}
}
func (q *codexInput) close() { q.mu.Lock(); q.closed = true; q.changed.Broadcast(); q.mu.Unlock() }

// Each attempt owns exactly one app-server/thread/turn. Request IDs correlate
// replies only: a lost reply never causes another thread/start or turn/start.
type codexAppServer struct {
	mu                                              sync.Mutex
	input                                           *codexInput
	stream                                          *toolStream
	spec                                            CommandSpec
	home, root, goal                                string
	binding                                         *cc.Binding
	evidence                                        *cc.Evidence
	catalog                                         *cc.Catalog
	models                                          []cc.ModelOption
	cursors                                         map[string]bool
	pending                                         []byte
	requestID                                       int
	waiting, waitingID                              string
	thread, turn                                    string
	turnAcknowledged, turnStarted, terminalReceived bool
	interruptID                                     string
	frames                                          int
	usageTotal                                      int64
	hasUsage                                        bool
	bad                                             error
	stopping                                        atomic.Bool
	skills                                          *cc.ExecutionSkillBundle
	skillsConfigured                                bool
	approvals                                       *codexApprovalSession
	userInputs                                      *codexUserInputSession
	nativeRequests                                  map[string]codexNativeRequest
	nativeTurnClosed                                bool
	steering                                        *codexSteeringSession
}

func newCodexAppServer(profile tc.Profile, spec CommandSpec, home, root, goal string, binding *cc.Binding, stream *toolStream) *codexAppServer {
	d := &codexAppServer{input: newCodexInput(), stream: stream, spec: spec, home: home, root: root, goal: goal, binding: binding, cursors: map[string]bool{}}
	if binding != nil {
		d.evidence = &cc.Evidence{BindingSHA256: binding.BindingSHA256, RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort}
		stream.codexEvidence = d.evidence
	}
	d.request("initialize", map[string]any{"clientInfo": map[string]any{"name": "suma", "title": "suma Runner", "version": buildinfo.Version}, "capabilities": map[string]any{"experimentalApi": false, "requestAttestation": false, "explicitGatewayOauth": true, "optOutNotificationMethods": []string{"item/reasoning/textDelta", "item/reasoning/summaryTextDelta", "item/commandExecution/outputDelta", "item/agentMessage/delta", "rawResponse/completed"}}})
	return d
}
func (d *codexAppServer) CopyInput(w io.Writer) (int64, error) { return d.input.copyTo(w) }
func (d *codexAppServer) Close() error {
	d.stopping.Store(true)
	d.mu.Lock()
	d.closeSteering(false)
	d.mu.Unlock()
	d.stopApprovals()
	d.stopUserInputs()
	d.endApprovalInput()
	d.input.close()
	return nil
}

// exec.Cmd does not notify an io.Writer that stdout reached EOF. Validate its
// final fragment explicitly before deriving terminal evidence; a prior valid
// terminal cannot hide a truncated or malformed trailing protocol frame.
func (d *codexAppServer) Finish() {
	d.mu.Lock()
	if d.bad == nil && len(bytes.TrimSpace(d.pending)) != 0 {
		d.frame(bytes.TrimSpace(d.pending))
	}
	d.pending = nil
	d.mu.Unlock()
	d.finishSteering()
	d.finishApprovals()
	d.finishUserInputs()
}
func (d *codexAppServer) request(method string, params any) {
	if d.stopping.Load() || d.bad != nil || d.stream != nil && d.stream.bad != nil {
		d.stopping.Store(true)
		d.input.close()
		return
	}
	d.requestID++
	d.waiting, d.waitingID = method, strconv.Itoa(d.requestID)
	// Startup frames may wait behind stdin writes. Claim them only while the
	// attempt is active; closing the queue alone still drains queued frames.
	before := func() error {
		if d.stopping.Load() {
			return fmt.Errorf("Codex request cancelled before write")
		}
		return nil
	}
	if err := d.input.enqueueReply(map[string]any{"id": d.requestID, "method": method, "params": params}, before, nil); err != nil {
		d.fail("request queue unavailable")
	}
}
func (d *codexAppServer) fail(reason string) {
	if d.bad != nil {
		return
	}
	d.bad = fmt.Errorf("Codex app-server: %s", reason)
	if d.evidence != nil && d.evidence.Skills != nil && d.evidence.Skills.FailureCode == "" {
		d.evidence.Skills.FailureCode = "codex_skills_protocol_failed"
	}
	d.input.close()
	if d.stream != nil {
		d.stream.invalid("Codex app-server " + reason)
	}
}
func (d *codexAppServer) Write(raw []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(raw)
	for len(raw) > 0 && d.bad == nil {
		end := bytes.IndexByte(raw, '\n')
		part := raw
		if end >= 0 {
			part = raw[:end]
		}
		if len(d.pending)+len(part) > toolLineLimit {
			d.fail("frame exceeds 64 KiB")
			break
		}
		d.pending = append(d.pending, part...)
		if end < 0 {
			break
		}
		d.frame(bytes.TrimSpace(d.pending))
		d.pending = nil
		raw = raw[end+1:]
	}
	return n, nil
}
func (d *codexAppServer) frame(raw []byte) {
	if len(raw) == 0 {
		return
	}
	d.frames++
	if d.frames > 20000 {
		d.fail("frame limit exceeded")
		return
	}
	m, err := decodeToolObject(raw)
	if err != nil {
		d.fail("invalid bounded JSON frame")
		return
	}
	if method := textAt(m, "method"); method != "" {
		if id, present := m["id"]; present {
			if method == cc.UserInputMethod {
				d.receiveUserInput(id, objectAt(m, "params"))
				return
			}
			if method == "item/commandExecution/requestApproval" || method == "item/fileChange/requestApproval" {
				d.receiveApproval(id, method, objectAt(m, "params"))
				return
			}
			// No permissions, external tools, user input or authentication can be
			// delegated through a task. Reply with a protocol error and stop.
			_ = d.input.enqueue(map[string]any{"id": id, "error": map[string]any{"code": -32601, "message": "suma Runner does not grant server-requested capabilities"}})
			d.fail("unexpected server request denied")
			return
		}
		d.notification(method, objectAt(m, "params"))
		return
	}
	id, ok := m["id"].(json.Number)
	if !ok {
		d.fail("response has no host request identity")
		return
	}
	if d.steeringResponse(id.String(), m) {
		return
	}
	if id.String() == d.interruptID && d.interruptID != "" {
		if _, bad := m["error"]; bad {
			d.fail("interrupt rejected")
			return
		}
		if _, ok := m["result"].(map[string]any); !ok {
			d.fail("invalid interrupt acknowledgement")
			return
		}
		d.evidence.InterruptAcknowledged = true
		return
	}
	if d.waitingID == "" || id.String() != d.waitingID {
		d.fail("unexpected or repeated response identity")
		return
	}
	if _, bad := m["error"]; bad {
		d.fail("request rejected")
		return
	}
	result, ok := m["result"].(map[string]any)
	if !ok {
		d.fail("response result must be an object")
		return
	}
	method := d.waiting
	d.waiting, d.waitingID = "", ""
	switch method {
	case "initialize":
		if !sameCodexPath(textAt(result, "codexHome"), d.home) || textAt(result, "userAgent") == "" {
			d.fail("isolated home acknowledgement differs")
			return
		}
		if d.stopping.Load() {
			d.input.close()
			return
		}
		_ = d.input.enqueue(map[string]any{"method": "initialized"})
		d.request("model/list", map[string]any{"limit": 32, "includeHidden": false})
	case "model/list":
		d.modelList(result)
	case "thread/start":
		d.threadStart(result)
	case "skills/list":
		d.skillsList(result)
	case "turn/start":
		if !d.selectTurn(objectAt(result, "turn")) {
			return
		}
		d.turnAcknowledged = true
		d.startSteering()
		if d.evidence.Skills != nil {
			d.evidence.Skills.TurnAcknowledged = true
		}
		if d.terminalReceived {
			d.input.close()
		}
	default:
		d.fail("unsupported response phase")
	}
}

func sameCodexPath(a, b string) bool {
	return filepath.IsAbs(a) && filepath.Clean(a) == filepath.Clean(b)
}

func (d *codexAppServer) modelList(result map[string]any) {
	data, ok := result["data"].([]any)
	if !ok || len(data) > 256 || len(d.models)+len(data) > 256 {
		d.fail("model catalog is missing or oversized")
		return
	}
	for _, value := range data {
		m, ok := value.(map[string]any)
		if !ok {
			d.fail("invalid model catalog entry")
			return
		}
		option := cc.ModelOption{Model: textAt(m, "model"), DefaultReasoningEffort: textAt(m, "defaultReasoningEffort")}
		efforts, ok := m["supportedReasoningEfforts"].([]any)
		if !ok || len(efforts) > 32 {
			d.fail("invalid reasoning catalog")
			return
		}
		for _, effort := range efforts {
			e, ok := effort.(map[string]any)
			if !ok {
				d.fail("invalid reasoning entry")
				return
			}
			option.ReasoningEfforts = append(option.ReasoningEfforts, textAt(e, "reasoningEffort"))
		}
		d.models = append(d.models, option)
	}
	if cursor := textAt(result, "nextCursor"); cursor != "" {
		if len(cursor) > 2048 || d.cursors[cursor] || len(d.cursors) >= 16 {
			d.fail("model catalog pagination is invalid")
			return
		}
		d.cursors[cursor] = true
		d.request("model/list", map[string]any{"limit": 32, "includeHidden": false, "cursor": cursor})
		return
	}
	catalog, err := cc.NewCatalog(CodexConfigurationDigest(*d.spec.CodexAppServer), d.models)
	if err != nil {
		d.fail("invalid public model catalog")
		return
	}
	d.catalog = &catalog
	if d.binding == nil {
		d.input.close()
		return
	}
	if catalog.CatalogSHA256 != d.binding.CatalogSHA256 {
		d.fail("current catalog differs from fixed selection")
		return
	}
	if _, err = cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: d.binding.Model, ReasoningEffort: d.binding.ReasoningEffort, TokenNoticeLimit: d.binding.TokenNoticeLimit}); err != nil {
		d.fail("selected model and effort are unavailable")
		return
	}
	d.request("thread/start", d.threadStartParams())
}

func (d *codexAppServer) threadStartParams() map[string]any {
	params := map[string]any{"model": d.binding.Model, "modelProvider": d.spec.CodexAppServer.ProviderID, "cwd": d.root, "approvalPolicy": cc.ApprovalPolicy, "sandbox": "workspace-write", "ephemeral": true}
	if d.binding.ReasoningEffort != "" {
		params["config"] = map[string]any{"model_reasoning_effort": d.binding.ReasoningEffort}
	}
	return params
}

func (d *codexAppServer) policyMismatch(policy map[string]any, runtimeRoots any) string {
	if textAt(policy, "type") != "workspaceWrite" {
		return "sandbox.type"
	}
	for _, check := range []struct {
		field    string
		expected bool
	}{{"networkAccess", false}, {"excludeTmpdirEnvVar", true}, {"excludeSlashTmp", true}} {
		if policy[check.field] != check.expected {
			return "sandbox." + check.field
		}
	}
	// Current app-server versions separately acknowledge runtime workspace
	// roots. Verify that set before considering additional policy roots; an
	// empty additional set never supplies authority by itself.
	hasRuntimeRoot := runtimeRoots != nil
	if hasRuntimeRoot {
		roots, ok := runtimeRoots.([]any)
		if !ok || len(roots) != 1 {
			return "runtimeWorkspaceRoots"
		}
		root, ok := roots[0].(string)
		if !ok || !sameCodexPath(root, d.root) {
			return "runtimeWorkspaceRoots"
		}
	}
	roots, ok := policy["writableRoots"].([]any)
	if !ok || len(roots) > 1 || len(roots) == 0 && !hasRuntimeRoot {
		return "sandbox.writableRoots"
	}
	if len(roots) == 1 {
		root, ok := roots[0].(string)
		if !ok || !sameCodexPath(root, d.root) {
			return "sandbox.writableRoots"
		}
	}
	return ""
}

func (d *codexAppServer) configurationMismatch(result map[string]any, policyField, effortField string) string {
	for _, check := range []struct{ field, expected string }{{"model", d.binding.Model}, {effortField, d.binding.ReasoningEffort}, {"modelProvider", d.spec.CodexAppServer.ProviderID}, {"approvalPolicy", cc.ApprovalPolicy}} {
		if textAt(result, check.field) != check.expected {
			return check.field
		}
	}
	if !sameCodexPath(textAt(result, "cwd"), d.root) {
		return "cwd"
	}
	if value, present := result["runtimeWorkspaceRoots"]; present && value == nil {
		return "runtimeWorkspaceRoots"
	}
	return d.policyMismatch(objectAt(result, policyField), result["runtimeWorkspaceRoots"])
}
func (d *codexAppServer) threadStart(result map[string]any) {
	thread := textAt(objectAt(result, "thread"), "id")
	if thread == "" || d.stream.id(thread) != thread || (d.thread != "" && d.thread != thread) {
		d.fail("thread identity is missing or changed")
		return
	}
	d.thread, d.evidence.ThreadID, d.stream.session = thread, thread, thread
	// Validate all settings before sending any task text to turn/start. The
	// acknowledged provider is only a configuration label, not a vendor proof.
	// Preserve bounded, redacted acknowledgement identities even when another
	// field fails; false ConfigurationAcknowledged must not hide those facts.
	for _, field := range []struct {
		name        string
		destination *string
	}{{"model", &d.evidence.AcknowledgedModel}, {"reasoningEffort", &d.evidence.AcknowledgedReasoningEffort}, {"modelProvider", &d.evidence.AcknowledgedProvider}} {
		value := textAt(result, field.name)
		if value == "" || d.stream.id(value) == value {
			*field.destination = value
		}
	}
	if mismatch := d.configurationMismatch(result, "sandbox", "reasoningEffort"); mismatch != "" {
		d.fail("effective " + mismatch + " differs")
		return
	}
	d.evidence.ConfigurationAcknowledged = true
	d.stream.event("session_started", "", "", "", nil)
	if d.skills != nil {
		d.request("skills/list", map[string]any{"cwds": []string{d.root}, "forceReload": true, "perCwdExtraUserRoots": []any{map[string]any{"cwd": d.root, "extraUserRoots": []string{filepath.Join(d.home, "skills")}}}})
		return
	}
	d.startSkillAwareTurn()
}
func (d *codexAppServer) selectTurn(turn map[string]any) bool {
	id := textAt(turn, "id")
	if id == "" || d.stream.id(id) != id || (d.turn != "" && d.turn != id) {
		d.fail("turn identity is missing or changed")
		return false
	}
	d.turn, d.evidence.TurnID = id, id
	if !d.turnStarted {
		d.turnStarted = true
		d.stream.event("turn_started", "", "", "", nil)
	}
	return true
}
func (d *codexAppServer) scoped(p map[string]any, turn bool) bool {
	if d.binding == nil || d.thread == "" || textAt(p, "threadId") != d.thread || turn && (d.turn == "" || textAt(p, "turnId") != d.turn) {
		d.fail("notification scope differs from active attempt")
		return false
	}
	return true
}
func (d *codexAppServer) notification(method string, p map[string]any) {
	switch method {
	case "serverRequest/resolved":
		id, _, err := codexRPCIdentity(p["requestId"])
		if err == nil && d.nativeRequests[id.Kind+":"+id.Value].method == cc.UserInputMethod {
			d.resolveUserInput(p)
		} else {
			d.resolveApproval(p)
		}
	case "item/started":
		d.observeApprovalItem(p, false)
	case "thread/started":
		if d.binding == nil {
			return
		}
		id := textAt(objectAt(p, "thread"), "id")
		if id == "" || d.stream.id(id) != id || d.thread != "" && d.thread != id {
			d.fail("thread notification identity differs")
			return
		}
		d.thread = id
	case "turn/started":
		if !d.scoped(p, false) || !d.evidence.ConfigurationAcknowledged {
			return
		}
		d.selectTurn(objectAt(p, "turn"))
	case "turn/completed":
		if !d.scoped(p, false) || !d.turnStarted {
			return
		}
		turn := objectAt(p, "turn")
		if !d.selectTurn(turn) {
			return
		}
		terminal := textAt(turn, "status")
		if terminal != "completed" && terminal != "failed" && terminal != "interrupted" {
			d.fail("turn terminal status is invalid")
			return
		}
		if d.terminalReceived {
			d.fail("multiple turn terminals")
			return
		}
		d.nativeTurnClosed = true
		d.closeSteering(true)
		d.closeUserInputTurn()
		if d.deferApprovalTerminal(p) {
			return
		}
		if d.deferUserInputTerminal(p) {
			return
		}
		if d.deferSteeringTerminal(p) {
			return
		}
		d.terminalReceived = true
		if terminal == "interrupted" {
			terminal = "unknown"
		}
		d.stream.setTerminal(terminal)
		kind := "turn_completed"
		if terminal != "completed" {
			kind = "turn_failed"
		}
		d.stream.event(kind, "", "", "", nil)
		if d.turnAcknowledged {
			d.input.close()
		}
	case "thread/tokenUsage/updated":
		if !d.scoped(p, true) {
			return
		}
		d.usage(objectAt(p, "tokenUsage"))
	case "model/rerouted":
		if !d.scoped(p, true) {
			return
		}
		model := textAt(p, "toModel")
		if model == "" || d.stream.id(model) != model || len(d.evidence.ReroutedModels) >= 32 {
			d.fail("model reroute evidence is invalid")
			return
		}
		d.evidence.ReroutedModels = append(d.evidence.ReroutedModels, model)
	case "thread/settings/updated":
		if !d.scoped(p, false) {
			return
		}
		s := objectAt(p, "threadSettings")
		if mismatch := d.configurationMismatch(s, "sandboxPolicy", "effort"); mismatch != "" {
			d.fail("active " + mismatch + " changed")
			return
		}
	case "item/completed":
		if !d.scoped(p, true) {
			return
		}
		item := objectAt(p, "item")
		d.observeApprovalItem(p, true)
		if textAt(item, "type") == "agentMessage" {
			d.stream.event("message", textAt(item, "id"), textAt(item, "text"), "", nil)
		}
	case "error":
		if !d.scoped(p, true) {
			return
		}
		// An error may declare willRetry. Only a turn terminal ends work; the
		// adapter itself never retries a request or interprets this as success.
		d.stream.event("error", "", textAt(objectAt(p, "error"), "message"), "", nil)
	}
}
func (d *codexAppServer) usage(snapshot map[string]any) {
	total := objectAt(snapshot, "total")
	m := map[string]any{}
	for from, to := range map[string]string{"inputTokens": "input_tokens", "cachedInputTokens": "cached_input_tokens", "cacheWriteInputTokens": "cache_write_input_tokens", "outputTokens": "output_tokens", "reasoningOutputTokens": "reasoning_output_tokens"} {
		value, ok := total[from].(json.Number)
		if !ok {
			d.fail("usage snapshot counter missing")
			return
		}
		m[to] = value
	}
	u, usageErr := parseUsage(m)
	n, ok := total["totalTokens"].(json.Number)
	var count int64
	var err error
	if ok {
		count, err = n.Int64()
	}
	if usageErr != nil || err != nil || !ok || count < 0 || count > tc.MaxSafeUsage || u == nil || *u.InputTokens > tc.MaxSafeUsage-*u.OutputTokens || count != *u.InputTokens+*u.OutputTokens || d.hasUsage && count < d.usageTotal {
		d.fail("invalid usage snapshot")
		return
	}
	if d.stream.usage != nil {
		for i, previous := range usageCounts(d.stream.usage) {
			if previous != nil && *usageCounts(u)[i] < *previous {
				d.fail("usage snapshot decreased")
				return
			}
		}
	}
	d.stream.usage = u
	d.usageTotal, d.hasUsage = count, true
	d.stream.usagePartial = true
	d.stream.event("usage", "", "", "", u, tc.UsagePartial)
	if d.binding.TokenNoticeLimit != nil && count > *d.binding.TokenNoticeLimit && !d.evidence.TokenNoticeExceeded {
		d.evidence.TokenNoticeExceeded = true
		d.stream.event("activity_updated", "", "Codex token notice threshold exceeded; execution is observe-only.", "", nil)
	}
}

// Interrupt acknowledgement is a protocol receipt, not proof of process exit.
// Execute still owns Stop/Finish and the authoritative cancellation outcome.
func (d *codexAppServer) Cancel() error {
	// Publish the stop before waiting for a stdout callback holding mu. A
	// delayed acknowledgement must not start a new thread or turn meanwhile.
	d.stopping.Store(true)
	d.stopApprovals()
	d.stopUserInputs()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closeSteering(false)
	if d.evidence == nil || d.turn == "" || d.terminalReceived {
		d.input.close()
		return nil
	}
	if d.evidence.InterruptRequested {
		return nil
	}
	d.evidence.InterruptRequested = true
	d.requestID++
	d.interruptID = strconv.Itoa(d.requestID)
	return d.input.enqueue(map[string]any{"id": d.requestID, "method": "turn/interrupt", "params": map[string]any{"threadId": d.thread, "turnId": d.turn}})
}
