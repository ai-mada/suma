package acpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	sdk "github.com/coder/acp-go-sdk"
)

var _ sdk.Client = (*Driver)(nil)

func (d *Driver) SessionUpdate(context.Context, sdk.SessionNotification) error { return nil } // Strict raw projection occurs before SDK decoding.
func deny() error                                                              { return sdk.NewMethodNotFound("host capability disabled") }
func (d *Driver) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, deny()
}
func (d *Driver) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, deny()
}
func (d *Driver) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, deny()
}
func (d *Driver) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, deny()
}
func (d *Driver) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, deny()
}
func (d *Driver) ReleaseTerminal(context.Context, sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, deny()
}
func (d *Driver) WaitForTerminalExit(context.Context, sdk.WaitForTerminalExitRequest) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, deny()
}

func (d *Driver) RequestPermission(ctx context.Context, p sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	tool := string(p.ToolCall.ToolCallId)
	d.mu.Lock()
	pending := d.permissionTools[tool]
	var raw []byte
	if pending != nil {
		raw = append([]byte{}, pending.raw...)
	}
	session := d.result.Evidence.SessionID
	d.mu.Unlock()
	if pending == nil || string(p.SessionId) != session {
		return sdk.RequestPermissionResponse{}, errors.New("acp_permission_invalid")
	}
	if d.discovery != nil {
		d.mu.Lock()
		pending.denied = true
		pending.replyOK = true
		d.mu.Unlock()
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	req, e := permissionRequest(raw, d.options.Secrets)
	if e != nil {
		d.fail("acp_permission_invalid")
		return sdk.RequestPermissionResponse{}, errors.New("acp_permission_invalid")
	}
	if d.emit(Event{Kind: "permission", SessionID: session, ToolCallID: tool, Text: req.Title}) != nil {
		return sdk.RequestPermissionResponse{}, errors.New("acp_permission_unavailable")
	}
	cancelled := func() sdk.RequestPermissionResponse {
		d.mu.Lock()
		pending.denied = true
		pending.replyOK = true
		d.mu.Unlock()
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}
	}
	if d.options.Permission == nil {
		return cancelled(), nil
	}
	bounded, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(d.ctx, cancel)
	defer func() { stop(); cancel() }()
	decision, e := d.options.Permission(bounded, req)
	if e != nil {
		d.mu.Lock()
		d.incomplete = true
		d.mu.Unlock()
		return sdk.RequestPermissionResponse{}, errors.New("acp_permission_unavailable")
	}
	selected := ""
	for _, option := range req.Options {
		if option.ID == decision.OptionID {
			selected = option.Kind
		}
	}
	if selected != "allow_once" && selected != "reject_once" || decision.Kind != selected || selected == "allow_once" && !req.DetailsComplete || ac.ValidatePermissionChoice(decision) != nil {
		d.fail("acp_permission_decision_invalid")
		return sdk.RequestPermissionResponse{}, errors.New("acp_permission_decision_invalid")
	}
	d.mu.Lock()
	pending.denied = selected == "reject_once"
	pending.replyOK = true
	d.mu.Unlock()
	return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{Selected: &sdk.RequestPermissionOutcomeSelected{OptionId: sdk.PermissionOptionId(decision.OptionID)}}}, nil
}
func permissionRequest(raw []byte, secrets []string) (ac.NativeSource, error) {
	fail := func() (ac.NativeSource, error) {
		return ac.NativeSource{}, errors.New("acp_permission_invalid")
	}
	params := object(raw)
	call := object(params["toolCall"])
	id := stringField(call, "toolCallId")
	session := stringField(params, "sessionId")
	if !token(id, 128) || !token(session, 128) || hasSecret(id, secrets) || hasSecret(session, secrets) {
		return fail()
	}
	title := stringField(call, "title")
	kind := stringField(call, "kind")
	complete := true
	if title == "" {
		title = "Permission request"
		complete = false
	}
	if !text(title, 4096, true) {
		return fail()
	}
	title, changed := safeText(title, secrets)
	complete = complete && !changed
	kind, changed = safeText(kind, secrets)
	if changed || !token(kind, 64) {
		kind = "other"
		complete = false
	}
	shown := map[string]any{}
	for key, rawValue := range call {
		switch key {
		case "toolCallId", "title", "kind", "status", "rawInput":
		default:
			complete = false
			continue
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(rawValue)))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return fail()
		}
		safe, redacted := safeValue(value, secrets)
		complete = complete && !redacted
		shown[key] = safe
	}
	if call["rawInput"] == nil || string(call["rawInput"]) == "null" {
		complete = false
	}
	details, e := json.MarshalIndent(shown, "", "  ")
	if e != nil {
		return fail()
	}
	if len(details) > 32<<10 {
		complete = false
		details = details[:32<<10]
		for !utf8.Valid(details) {
			details = details[:len(details)-1]
		}
	}
	var options []map[string]json.RawMessage
	if json.Unmarshal(params["options"], &options) != nil || len(options) == 0 || len(options) > 16 {
		return fail()
	}
	out := ac.NativeSource{SessionID: session, ToolCallID: id, Title: title, Kind: kind, Details: string(details), DetailsComplete: complete, Options: []ac.PermissionOption{}}
	seen := map[string]bool{}
	for _, option := range options {
		if !closed(option, "_meta", "optionId", "name", "kind") {
			return fail()
		}
		oid, name, okind := stringField(option, "optionId"), stringField(option, "name"), stringField(option, "kind")
		if !token(oid, 128) || hasSecret(oid, secrets) || seen[oid] || !text(name, 512, false) || strings.TrimSpace(name) == "" {
			return fail()
		}
		seen[oid] = true
		switch okind {
		case "allow_once", "reject_once", "allow_always", "reject_always":
		default:
			return fail()
		}
		name, changed := safeText(name, secrets)
		out.DetailsComplete = out.DetailsComplete && !changed
		out.Options = append(out.Options, ac.PermissionOption{ID: oid, Name: name, Kind: okind})
	}
	for {
		encoded, err := json.Marshal(out)
		if err != nil {
			return fail()
		}
		if len(encoded) <= 64<<10 {
			break
		}
		out.DetailsComplete = false
		if len(out.Details) > 0 {
			out.Details = trimText(out.Details, len(out.Details)/2)
		} else if len(out.Title) > 1 {
			out.Title = trimText(out.Title, len(out.Title)/2)
		} else {
			return fail()
		}
	}
	if ac.ValidatePermissionSource(out) != nil {
		return fail()
	}
	return out, nil
}
func trimText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
func safeValue(v any, secrets []string) (any, bool) {
	switch value := v.(type) {
	case string:
		return safeText(value, secrets)
	case []any:
		out := make([]any, len(value))
		changed := false
		for i, x := range value {
			safe, c := safeValue(x, secrets)
			out[i] = safe
			changed = changed || c
		}
		return out, changed
	case map[string]any:
		out := map[string]any{}
		changed := false
		for k, x := range value {
			safeKey, c := safeText(k, secrets)
			safe, d := safeValue(x, secrets)
			out[safeKey] = safe
			changed = changed || c || d
		}
		return out, changed
	case json.Number, float64, bool, nil:
		// Raw JSON primitives may themselves be a credential (for example a
		// numeric PIN). Preserve their type only when their wire value is safe.
		raw, err := json.Marshal(value)
		if err != nil {
			return "[REDACTED]", true
		}
		if _, changed := safeText(string(raw), secrets); changed {
			return "[REDACTED]", true
		}
	}
	return v, false
}
