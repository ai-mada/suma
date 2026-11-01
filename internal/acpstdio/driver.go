package acpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	sdk "github.com/coder/acp-go-sdk"
)

const frameLimit = 64 << 10
const textLimit = 1 << 20

type outbound struct {
	raw    []byte
	reply  string
	prompt bool
}
type pendingPermission struct {
	wireID, toolID string
	raw            json.RawMessage
	decided        bool
	denied         bool
	replyOK        bool
	writeDone      chan struct{}
}
type Driver struct {
	mu, writeMu                             sync.Mutex
	options                                 Options
	ctx                                     context.Context
	cancel                                  context.CancelFunc
	conn                                    *sdk.ClientSideConnection
	input                                   *io.PipeWriter
	output                                  chan outbound
	outEnd, done, promptSent                chan struct{}
	finishOnce, closeOnce, promptSentOnce   sync.Once
	copyStarted                             bool
	localClosed                             bool
	result                                  Result
	bad                                     error
	pending                                 []byte
	requests                                map[string]string
	remote                                  map[string]*pendingPermission
	permissionTools                         map[string]*pendingPermission
	tools                                   map[string]string
	toolInvalid, incomplete, promptResponse bool
	frames, wireBytes, textBytes            int
	filter                                  *textFilter
	modelConfig                             string
	discovery                               *discoveryState
}
type sdkOutput struct{ d *Driver }

func newDriver(parent context.Context, o Options) (*Driver, error) {
	if !text(o.Goal, 32000, true) || strings.TrimSpace(o.Goal) == "" || !token(o.Model, 256) || !tc.ValidDigest(o.ConfigSHA) {
		return nil, errors.New("acp_options_invalid")
	}
	return newTransport(parent, o, nil)
}
func newTransport(parent context.Context, o Options, discovery *discoveryState) (*Driver, error) {
	if parent == nil || !filepath.IsAbs(o.Cwd) || len(o.Secrets) > 128 || o.Runtime != "" && o.Runtime != "qoder" && o.Runtime != "cursor" {
		return nil, errors.New("acp_options_invalid")
	}
	total := 0
	for _, s := range o.Secrets {
		total += len(s)
		if len(s) > 32<<10 || total > 1<<20 {
			return nil, errors.New("acp_options_invalid")
		}
	}
	ctx, cancel := context.WithCancel(parent)
	reader, writer := io.Pipe()
	d := &Driver{options: o, ctx: ctx, cancel: cancel, input: writer, output: make(chan outbound, 64), outEnd: make(chan struct{}), done: make(chan struct{}), promptSent: make(chan struct{}), requests: map[string]string{}, remote: map[string]*pendingPermission{}, permissionTools: map[string]*pendingPermission{}, tools: map[string]string{}, filter: newTextFilter(o.Secrets), result: Result{Terminal: "unknown", UsageState: "unreported", Evidence: ac.Evidence{ConfigurationSHA256: o.ConfigSHA, RequestedModel: o.Model}}, discovery: discovery}
	d.conn = sdk.NewClientSideConnection(d, sdkOutput{d}, reader)
	d.conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() {
		<-ctx.Done()
		d.mu.Lock()
		complete := d.result.Evidence.PromptCompleted
		if d.discovery != nil {
			complete = d.discovery.finalized
		}
		d.mu.Unlock()
		if !complete {
			d.fail("acp_interrupted")
		}
		_ = writer.CloseWithError(errors.New("acp_closed"))
		_ = reader.Close()
	}()
	if discovery != nil {
		go d.runModelDiscovery()
	} else {
		go d.run()
	}
	return d, nil
}
func (d *Driver) fail(code string) {
	d.mu.Lock()
	if d.bad == nil {
		d.bad = errors.New(code)
	}
	d.mu.Unlock()
	d.cancel()
}
func (d *Driver) run() {
	defer close(d.done)
	defer close(d.outEnd)
	fail := func() { d.fail("acp_session_unknown") }
	initialized, e := d.conn.Initialize(d.ctx, sdk.InitializeRequest{ProtocolVersion: 1, ClientInfo: &sdk.Implementation{Name: "suma-runner", Version: "1"}, ClientCapabilities: sdk.ClientCapabilities{}})
	if e != nil || initialized.ProtocolVersion != 1 {
		fail()
		return
	}
	d.mu.Lock()
	d.result.Evidence.ProtocolVersion = 1
	if initialized.AgentInfo != nil {
		d.result.Evidence.AgentName, _ = safeText(initialized.AgentInfo.Name, d.options.Secrets)
		d.result.Evidence.AgentVersion, _ = safeText(initialized.AgentInfo.Version, d.options.Secrets)
	}
	if initialized.AgentInfo == nil && d.options.Runtime != "cursor" || initialized.AgentInfo != nil && (!text(d.result.Evidence.AgentName, 256, false) || !text(d.result.Evidence.AgentVersion, 256, false)) {
		d.result.Evidence.AgentName = ""
		d.result.Evidence.AgentVersion = ""
		d.mu.Unlock()
		fail()
		return
	}
	d.mu.Unlock()
	session, e := d.conn.NewSession(d.ctx, sdk.NewSessionRequest{Cwd: d.options.Cwd, McpServers: []sdk.McpServer{}})
	if e != nil || !token(string(session.SessionId), 128) {
		fail()
		return
	}
	d.mu.Lock()
	if d.result.Evidence.SessionID != "" && d.result.Evidence.SessionID != string(session.SessionId) {
		d.mu.Unlock()
		fail()
		return
	}
	d.result.Evidence.SessionID = string(session.SessionId)
	d.mu.Unlock()
	option, e := modelOption(session.ConfigOptions, d.options.Model)
	if e != nil {
		d.fail("acp_model_unacknowledged")
		return
	}
	d.mu.Lock()
	d.modelConfig = string(option.Id)
	d.mu.Unlock()
	changed, e := d.conn.SetSessionConfigOption(d.ctx, sdk.SetSessionConfigOptionRequest{ValueId: &sdk.SetSessionConfigOptionValueId{SessionId: session.SessionId, ConfigId: option.Id, Value: sdk.SessionConfigValueId(d.options.Model)}})
	if e != nil {
		d.fail("acp_model_unacknowledged")
		return
	}
	ack, e := modelOption(changed.ConfigOptions, d.options.Model)
	if e != nil || ack.Id != option.Id || string(ack.CurrentValue) != d.options.Model {
		d.fail("acp_model_unacknowledged")
		return
	}
	d.mu.Lock()
	d.result.Evidence.SelectedModel = d.options.Model
	d.result.Evidence.ModelSelectionAcknowledged = true
	d.mu.Unlock()
	if d.emit(Event{Kind: "session", SessionID: string(session.SessionId)}) != nil {
		return
	}
	d.mu.Lock()
	d.result.Evidence.PromptStarted = true
	d.mu.Unlock()
	response, e := d.conn.Prompt(d.ctx, sdk.PromptRequest{SessionId: session.SessionId, Prompt: []sdk.ContentBlock{sdk.TextBlock(d.options.Goal)}})
	if e != nil {
		fail()
		return
	}
	select {
	case <-d.promptSent:
	case <-d.ctx.Done():
		fail()
		return
	}
	d.waitReplyWrites()
	tail, _ := d.filter.flush()
	if tail != "" {
		if d.emit(Event{Kind: "message", SessionID: string(session.SessionId), Text: tail}) != nil {
			return
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result.Evidence.PromptCompleted = true
	d.result.Evidence.StopReason = string(response.StopReason)
	d.result.Evidence.ToolsComplete = !d.toolInvalid
	for _, state := range d.tools {
		if state != "completed" && state != "failed" {
			d.result.Evidence.ToolsComplete = false
		}
	}
	pending := false
	for _, p := range d.remote {
		pending = pending || !p.decided
	}
	switch string(response.StopReason) {
	case "end_turn":
		if d.bad == nil && !d.incomplete && d.result.Evidence.ToolsComplete && !pending && !d.result.Evidence.CancelRequested && d.result.Evidence.PermissionsDenied == 0 {
			d.result.Terminal = "completed"
		}
	case "refusal", "max_tokens", "max_turn_requests":
		if d.bad == nil && !pending {
			d.result.Terminal = "failed"
		}
	case "cancelled":
		d.result.Terminal = "unknown"
	default:
		d.result.Evidence.StopReason = ""
		if d.bad == nil {
			d.bad = errors.New("acp_stop_unknown")
		}
	}
	if d.result.Terminal == "unknown" && d.bad == nil {
		d.bad = errors.New("acp_outcome_unknown")
	}
}
func modelOption(options []sdk.SessionConfigOption, model string) (sdk.SessionConfigOptionSelect, error) {
	option, values, err := modelSelection(options)
	if err != nil {
		return sdk.SessionConfigOptionSelect{}, err
	}
	for _, v := range values {
		if string(v.Value) == model {
			return option, nil
		}
	}
	return sdk.SessionConfigOptionSelect{}, errors.New("acp_model_unacknowledged")
}
func modelSelection(options []sdk.SessionConfigOption) (sdk.SessionConfigOptionSelect, []sdk.SessionConfigSelectOption, error) {
	var found *sdk.SessionConfigOptionSelect
	if len(options) > 128 {
		return sdk.SessionConfigOptionSelect{}, nil, errors.New("acp_model_unacknowledged")
	}
	for _, o := range options {
		if o.Select == nil || o.Select.Category == nil || *o.Select.Category != sdk.SessionConfigOptionCategoryModel {
			continue
		}
		if found != nil || o.Select.Type != "select" || !token(string(o.Select.Id), 128) {
			return sdk.SessionConfigOptionSelect{}, nil, errors.New("acp_model_unacknowledged")
		}
		found = o.Select
	}
	if found == nil {
		return sdk.SessionConfigOptionSelect{}, nil, errors.New("acp_model_unacknowledged")
	}
	values := []sdk.SessionConfigSelectOption{}
	if found.Options.Ungrouped != nil {
		values = append(values, (*found.Options.Ungrouped)...)
	}
	if found.Options.Grouped != nil {
		for _, g := range *found.Options.Grouped {
			values = append(values, g.Options...)
		}
	}
	if len(values) == 0 || len(values) > 256 {
		return sdk.SessionConfigOptionSelect{}, nil, errors.New("acp_model_unacknowledged")
	}
	seen := map[string]bool{}
	for _, v := range values {
		key := string(v.Value)
		if !token(key, 256) || seen[key] {
			return sdk.SessionConfigOptionSelect{}, nil, errors.New("acp_model_unacknowledged")
		}
		seen[key] = true
	}
	return *found, values, nil
}
func (d *Driver) CopyInput(w io.Writer) (int64, error) {
	d.mu.Lock()
	if d.copyStarted {
		d.mu.Unlock()
		return 0, errors.New("acp_input_already_owned")
	}
	d.copyStarted = true
	d.mu.Unlock()
	var total int64
	write := func(item outbound) error {
		// A peer may consume these bytes and return Prompt before Write returns.
		// Track the in-flight write without treating enqueue/delivery as success.
		var written chan struct{}
		d.mu.Lock()
		if p := d.remote[item.reply]; item.reply != "" && p != nil {
			written = make(chan struct{})
			p.writeDone = written
		}
		d.mu.Unlock()
		if written != nil {
			defer close(written)
		}
		n, e := w.Write(item.raw)
		total += int64(n)
		if e != nil || n != len(item.raw) {
			d.fail("acp_stdin_unknown")
			return errors.New("acp_stdin_unknown")
		}
		d.mu.Lock()
		if p := d.remote[item.reply]; item.reply != "" && p != nil && (p.replyOK || p.toolID == "") {
			p.decided = true
			if p.denied {
				d.result.Evidence.PermissionsDenied++
			}
		}
		d.mu.Unlock()
		if item.prompt {
			d.promptSentOnce.Do(func() { close(d.promptSent) })
		}
		return nil
	}
	for {
		select {
		case item := <-d.output:
			if e := write(item); e != nil {
				return total, e
			}
		case <-d.outEnd:
			for {
				select {
				case item := <-d.output:
					if e := write(item); e != nil {
						return total, e
					}
				default:
					return total, nil
				}
			}
		case <-d.ctx.Done():
			return total, errors.New("acp_closed")
		}
	}
}
func (d *Driver) waitReplyWrites() {
	d.mu.Lock()
	writes := []chan struct{}{}
	for _, p := range d.remote {
		if !p.decided && p.writeDone != nil {
			writes = append(writes, p.writeDone)
		}
	}
	d.mu.Unlock()
	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	for _, written := range writes {
		select {
		case <-written:
		case <-d.ctx.Done():
			return
		case <-timer.C:
			d.fail("acp_stdin_unknown")
			return
		}
	}
}
func (d *Driver) Cancel() error {
	d.mu.Lock()
	d.result.Evidence.CancelRequested = true
	session := d.result.Evidence.SessionID
	d.mu.Unlock()
	if session == "" {
		return errors.New("acp_cancel_unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if e := d.conn.Cancel(ctx, sdk.CancelNotification{SessionId: sdk.SessionId(session)}); e != nil {
		return errors.New("acp_cancel_unknown")
	}
	return nil
}
func (d *Driver) Close() error {
	d.closeOnce.Do(func() { d.mu.Lock(); d.localClosed = true; d.mu.Unlock(); d.cancel(); _ = d.input.Close() })
	return nil
}
func (d *Driver) Finish() Result {
	d.finishOnce.Do(func() {
		d.writeMu.Lock()
		if len(strings.TrimSpace(string(d.pending))) != 0 {
			d.fail("acp_truncated_frame")
		}
		d.pending = nil
		_ = d.input.Close()
		d.writeMu.Unlock()
		select {
		case <-d.done:
		case <-time.After(2 * time.Second):
			d.fail("acp_finish_unknown")
		}
		d.cancel()
	})
	return d.Evidence()
}
func (d *Driver) Evidence() Result {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.result
	if d.bad != nil {
		r.Terminal = "unknown"
		r.Error = d.bad
	}
	if r.Usage != nil {
		raw, _ := json.Marshal(r.Usage)
		var clone tc.Usage
		_ = json.Unmarshal(raw, &clone)
		r.Usage = &clone
	}
	return r
}
func (d *Driver) emit(event Event) error {
	d.mu.Lock()
	d.textBytes += len(event.Text)
	over := d.textBytes > textLimit
	d.mu.Unlock()
	if over {
		d.fail("acp_output_limited")
		return errors.New("acp_output_limited")
	}
	if d.options.Emit != nil {
		if e := d.options.Emit(event); e != nil {
			d.fail("acp_progress_unavailable")
			return errors.New("acp_progress_unavailable")
		}
	}
	return nil
}
