package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

const claudeSDKLineLimit = 512 << 10

type claudeSDKCallback struct {
	permission        *cl.PermissionSource
	input             *cl.InputRequest
	permissionRequest pc.Request
	inputRequest      ic.ToolInputRequest
	id                string
	choice            *cl.PermissionChoice
	answer            *ic.ToolInputAnswer
	observation       delivery.Observation
}

// claudeSDKDriver owns the private SDK-host carrier only. Worker identity,
// current permission, durable IO, the process tree and candidate remain Go facts.
type claudeSDKDriver struct {
	mu                                      sync.Mutex
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	abort                                   func()
	a                                       Assignment
	cfg                                     ExecConfig
	profile                                 tc.Profile
	requestID                               string
	queue                                   *codexInput
	output                                  *output
	pending                                 []byte
	bad                                     error
	closed, receivedTerminal, blockedResult bool
	terminal                                string
	proof                                   tc.ClaudeSDKExecutionEvidence
	usage                                   *tc.Usage
	events                                  int
	callbacks                               map[string]*claudeSDKCallback
	requestIDs                              map[string]string
	order                                   []*claudeSDKCallback
	activities                              map[string]string
	work                                    sync.WaitGroup
}

func newClaudeSDKDriver(ctx context.Context, a Assignment, cfg ExecConfig, p tc.Profile, abort func()) (*claudeSDKDriver, error) {
	if !tc.SupportsClaudeSDKExecutionProfile(p) || p.ClaudeSDKExecution == nil || cfg.Journal == nil || cfg.ToolPermission == nil || cfg.ToolPermissionPoll == nil || cfg.ToolInputClient == nil {
		return nil, fmt.Errorf("Claude SDK requires its original durable worker and human-input clients")
	}
	if err := cfg.Journal.beginToolPermissionExecution(a, p); err != nil {
		return nil, err
	}
	if err := cfg.Journal.beginToolInputExecution(a, p); err != nil {
		return nil, err
	}
	child, cancel := context.WithCancel(ctx)
	d := &claudeSDKDriver{ctx: child, cancel: cancel, abort: abort, a: a, cfg: cfg, profile: tc.CloneProfile(p), requestID: digest([]byte(key(a))), queue: newBoundedHostInput(claudeSDKLineLimit, false), output: newOutput(cfg.OutputLimit, cfg.Secrets), callbacks: map[string]*claudeSDKCallback{}, requestIDs: map[string]string{}, activities: map[string]string{}}
	c := p.ClaudeSDKExecution
	d.proof = tc.ClaudeSDKExecutionEvidence{SDKVersion: tc.ClaudeSDKVersion, CLIVersion: tc.ClaudeSDKCLIVersion, ConfigurationSHA256: c.ConfigurationSHA256, RequestedModel: c.Model}
	goal, _ := a.Config["goal"].(string)
	if err := d.queue.enqueue(map[string]any{"v": 1, "type": "execute", "request_id": d.requestID, "goal": goal, "configuration_sha256": c.ConfigurationSHA256}); err != nil {
		cancel()
		return nil, err
	}
	return d, nil
}

func (d *claudeSDKDriver) failLocked(reason string) {
	if d.bad == nil {
		d.bad = fmt.Errorf("Claude SDK carrier: %s", reason)
		d.cancel()
		d.queue.close()
		if d.abort != nil {
			d.abort()
		}
	}
}

func (d *claudeSDKDriver) emitLocked(kind, item, message, status string) {
	d.events++
	if d.cfg.ToolProgress == nil {
		return
	}
	e := tc.Event{SchemaVersion: 1, CommandID: d.profile.CommandID, ProfileSHA256: d.profile.ProfileSHA256, Protocol: d.profile.Protocol, Kind: kind, SessionID: d.proof.SessionID, ItemID: item, Message: progressText(message, d.cfg.Secrets), Status: status}
	if err := d.cfg.ToolProgress(e); err != nil {
		d.failLocked("durable progress unavailable")
	}
}

func claudeSDKFrame(m map[string]any, fields ...string) bool {
	allowed := map[string]bool{"v": true, "type": true, "request_id": true}
	for _, f := range fields {
		allowed[f] = true
	}
	for k, v := range m {
		if !allowed[k] || v == nil {
			return false
		}
	}
	return true
}

func decodeClaudeSDKValue(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (d *claudeSDKDriver) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return 0, io.ErrClosedPipe
	}
	d.pending = append(d.pending, data...)
	for {
		i := bytes.IndexByte(d.pending, '\n')
		if i < 0 {
			break
		}
		if i > claudeSDKLineLimit {
			d.failLocked("oversized frame")
			return len(data), d.bad
		}
		raw := append([]byte(nil), d.pending[:i]...)
		d.pending = d.pending[i+1:]
		d.lineLocked(raw)
		if d.bad != nil {
			return len(data), d.bad
		}
	}
	if len(d.pending) > claudeSDKLineLimit {
		d.failLocked("oversized pending frame")
	}
	return len(data), d.bad
}

func (d *claudeSDKDriver) lineLocked(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	if d.receivedTerminal {
		d.failLocked("frame after terminal")
		return
	}
	if !tc.ValidJSONUnicode(raw) {
		d.failLocked("invalid Unicode")
		return
	}
	m, err := decodeToolObjectBounded(raw, 1<<18)
	if err != nil {
		d.failLocked("invalid or duplicate JSON fields")
		return
	}
	v, ok := nativeInteger(m, "v")
	if !ok || v != 1 || textAt(m, "request_id") != d.requestID {
		d.failLocked("wrong carrier identity")
		return
	}
	kind := textAt(m, "type")
	if kind == "initialized" {
		if !claudeSDKFrame(m, "session_id", "model", "sdk_version", "cli_version", "configuration_sha256") || d.proof.SessionID != "" || !cl.ID(textAt(m, "session_id")) || textAt(m, "model") != d.proof.RequestedModel || textAt(m, "configuration_sha256") != d.proof.ConfigurationSHA256 || textAt(m, "sdk_version") != tc.ClaudeSDKVersion || textAt(m, "cli_version") != tc.ClaudeSDKCLIVersion {
			d.failLocked("invalid SDK initialization")
			return
		}
		d.proof.SessionID = textAt(m, "session_id")
		d.proof.AcknowledgedModel = textAt(m, "model")
		d.emitLocked("session_started", "", "", "")
		d.emitLocked("turn_started", "", "", "")
		return
	}
	if kind == "permission_request" || kind == "input_request" {
		if d.proof.SessionID == "" || !claudeSDKFrame(m, "source") {
			d.failLocked("callback before exact initialization")
			return
		}
		r := &claudeSDKCallback{}
		var basis cl.Basis
		if kind == "permission_request" {
			var source cl.PermissionSource
			if decodeClaudeSDKValue(m["source"], &source) != nil || cl.ValidatePermissionSource(source) != nil {
				d.failLocked("invalid permission source")
				return
			}
			r.permission = &source
			basis = source.Basis
		} else {
			var source cl.InputRequest
			if decodeClaudeSDKValue(m["source"], &source) != nil || cl.ValidateInputRequest(source) != nil {
				d.failLocked("invalid input source")
				return
			}
			r.input = &source
			basis = source.Basis
		}
		if basis.SessionID != d.proof.SessionID || basis.ConfigurationSHA256 != d.proof.ConfigurationSHA256 {
			d.failLocked("callback source changed")
			return
		}
		if previous := d.callbacks[basis.CallbackID]; previous != nil {
			if !samePermissionValue(previous.permission, r.permission) || !samePermissionValue(previous.input, r.input) {
				d.failLocked("callback ID reused")
			}
			return
		}
		if old, exists := d.requestIDs[basis.RequestID]; exists && old != basis.CallbackID {
			d.failLocked("native request ID reused")
			return
		}
		if len(d.callbacks) >= cl.MaxCallbacks {
			d.failLocked("callback limit exceeded")
			return
		}
		d.callbacks[basis.CallbackID] = r
		d.requestIDs[basis.RequestID] = basis.CallbackID
		d.order = append(d.order, r)
		if r.permission != nil {
			d.proof.PermissionRequests++
		}
		d.emitLocked("waiting_input", basis.ToolUseID, "Claude SDK input requires an explicit current decision", "")
		d.work.Add(1)
		go func() {
			defer d.work.Done()
			if r.permission != nil {
				d.handlePermission(r)
			} else {
				d.handleInput(r)
			}
		}()
		return
	}
	if textAt(m, "session_id") != d.proof.SessionID || d.proof.SessionID == "" && kind != "failed" && kind != "cancelled" {
		d.failLocked("session identity changed")
		return
	}
	switch kind {
	case "message":
		if !claudeSDKFrame(m, "session_id", "text") {
			d.failLocked("invalid message frame")
			return
		}
		text, ok := m["text"].(string)
		if !ok || len(text) > 32000 {
			d.failLocked("invalid message")
			return
		}
		_, _ = d.output.Write([]byte(text))
		d.emitLocked("message", "", text, "")
	case "activity_started":
		if !claudeSDKFrame(m, "session_id", "tool_use_id", "tool_name") || !cl.ID(textAt(m, "tool_use_id")) {
			d.failLocked("invalid activity")
			return
		}
		id, name := textAt(m, "tool_use_id"), textAt(m, "tool_name")
		switch name {
		case "Read", "Edit", "Write", "Glob", "Grep", "Bash", "AskUserQuestion":
		default:
			d.failLocked("unexpected tool")
			return
		}
		if _, exists := d.activities[id]; exists {
			d.failLocked("duplicate tool activity")
			return
		}
		d.activities[id] = name
		d.emitLocked(kind, id, "", "in_progress")
	case "activity_completed":
		if !claudeSDKFrame(m, "session_id", "tool_use_id", "status") || !cl.ID(textAt(m, "tool_use_id")) {
			d.failLocked("invalid tool result frame")
			return
		}
		id := textAt(m, "tool_use_id")
		if _, exists := d.activities[id]; !exists {
			d.failLocked("tool result source mismatch")
			return
		}
		status := textAt(m, "status")
		if status != "completed" && status != "error" {
			d.failLocked("invalid tool result")
			return
		}
		delete(d.activities, id)
		if status == "error" {
			status = "failed"
		}
		d.emitLocked(kind, id, "", status)
	case "completed", "failed", "cancelled":
		if !claudeSDKFrame(m, "session_id", "code", "evidence", "usage") {
			d.failLocked("invalid terminal frame")
			return
		}
		facts := objectAt(m, "evidence")
		if facts == nil {
			d.failLocked("missing SDK result facts")
			return
		}
		allowed := map[string]bool{"result_subtype": true, "result_is_error": true, "turn_ended": true, "tools_completed": true, "callback_count": true, "returned_count": true, "pending_callbacks": true, "callback_denial_count": true, "native_permission_denial_count": true}
		if len(facts) < 8 || len(facts) > len(allowed) {
			d.failLocked("incomplete SDK result facts")
			return
		}
		for key, value := range facts {
			if !allowed[key] || value == nil {
				d.failLocked("unexpected SDK result fact")
				return
			}
		}
		booleans := []*bool{&d.proof.ResultIsError, &d.proof.TurnEnded, &d.proof.ToolsCompleted}
		for i, key := range []string{"result_is_error", "turn_ended", "tools_completed"} {
			value, ok := facts[key].(bool)
			if !ok {
				d.failLocked("invalid SDK result flag")
				return
			}
			*booleans[i] = value
		}
		d.proof.ResultSubtype = textAt(facts, "result_subtype")
		counts := make([]int64, 4)
		for i, key := range []string{"callback_count", "returned_count", "pending_callbacks", "callback_denial_count"} {
			n, ok := nativeInteger(facts, key)
			if !ok || n < 0 || n > cl.MaxCallbacks {
				d.failLocked("invalid SDK callback count")
				return
			}
			counts[i] = n
		}
		if counts[0] != int64(len(d.callbacks)) || counts[1]+counts[2] != counts[0] {
			d.failLocked("SDK callback accounting mismatch")
			return
		}
		d.proof.DeniedCallbacks = int(counts[3])
		if _, present := facts["native_permission_denial_count"]; present {
			n, ok := nativeInteger(facts, "native_permission_denial_count")
			if !ok || n < 0 || n > 1000000 {
				d.failLocked("invalid native permission-denial fact")
				return
			}
			count := int(n)
			d.proof.NativePermissionDenials = &count
		}
		d.terminal = "failed"
		if kind == "completed" {
			if d.proof.ResultSubtype != "success" || d.proof.ResultIsError || !d.proof.TurnEnded || !d.proof.ToolsCompleted || len(d.activities) != 0 || counts[2] != 0 || counts[3] != 0 || d.proof.NativePermissionDenials == nil || *d.proof.NativePermissionDenials != 0 {
				d.failLocked("success without complete foreground result")
				return
			}
			d.terminal = "completed"
		}
		if kind == "cancelled" {
			d.proof.CancelRequested = true
			d.terminal = "unknown"
		}
		if value, present := m["usage"]; present {
			counters, ok := value.(map[string]any)
			if !ok {
				d.failLocked("invalid usage object")
				return
			}
			for key := range counters {
				switch key {
				case "input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens":
				default:
					d.failLocked("unknown usage counter")
					return
				}
			}
			usage, err := parseUsage(counters)
			if err != nil {
				d.failLocked("invalid usage")
				return
			}
			d.usage = usage
		}
		d.receivedTerminal = true
		d.cancel()
		d.queue.close()
	default:
		d.failLocked("unsupported SDK frame")
	}
}

func (d *claudeSDKDriver) CopyInput(writer io.Writer) (int64, error) { return d.queue.copyTo(writer) }
func (d *claudeSDKDriver) Cancel() error {
	d.mu.Lock()
	d.proof.CancelRequested = true
	d.mu.Unlock()
	return d.queue.enqueue(map[string]any{"v": 1, "type": "cancel", "request_id": d.requestID})
}
func (d *claudeSDKDriver) Close() error {
	d.mu.Lock()
	d.closed = true
	d.cancel()
	d.mu.Unlock()
	d.queue.abort(io.ErrClosedPipe)
	return nil
}

func (d *claudeSDKDriver) finish(hostStopped, treeStopped bool) *tc.Evidence {
	d.mu.Lock()
	if len(d.pending) > 0 && d.bad == nil {
		d.lineLocked(d.pending)
		d.pending = nil
	}
	d.closed = true
	d.cancel()
	d.queue.close()
	d.mu.Unlock()
	d.work.Wait()
	for _, r := range d.order {
		d.closeCallback(r)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.proof.HostProcessStopped = hostStopped
	d.proof.HostProcessTreeStopped = hostStopped && treeStopped
	for _, r := range d.order {
		if r.id == "" {
			continue
		}
		copy := r.observation
		if r.permission != nil {
			d.proof.Permissions = append(d.proof.Permissions, cl.PermissionEvidence{PermissionID: r.id, Basis: r.permission.Basis, ToolName: r.permission.ToolName, Observation: &copy})
		} else {
			d.proof.UserInputs = append(d.proof.UserInputs, cl.InputEvidence{InputID: r.id, Basis: r.input.Basis, QuestionCount: len(r.input.Questions), Observation: &copy})
		}
	}
	terminal := d.terminal
	if d.bad != nil || !d.receivedTerminal || !hostStopped || !treeStopped {
		terminal = "unknown"
	}
	if terminal == "completed" && d.blockedResult {
		terminal = "failed"
	}
	state := tc.UsageUnreported
	if d.usage != nil {
		state = tc.UsagePartial
	}
	return &tc.Evidence{Profile: tc.CloneProfile(d.profile), SessionID: d.proof.SessionID, Terminal: terminal, EventCount: d.events, Usage: d.usage, UsageState: state, ClaudeSDKExecution: func() *tc.ClaudeSDKExecutionEvidence { v := tc.CloneClaudeSDKExecutionEvidence(d.proof); return &v }()}
}
