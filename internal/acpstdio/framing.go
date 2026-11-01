package acpstdio

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func object(raw []byte) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m == nil {
		return nil
	}
	return m
}
func stringField(m map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}
func closed(m map[string]json.RawMessage, keys ...string) bool {
	if m == nil {
		return false
	}
	for k := range m {
		found := false
		for _, key := range keys {
			found = found || k == key
		}
		if !found {
			return false
		}
	}
	return true
}

// Validate canonical names before the SDK's case-insensitive struct decoder.
// Every option is checked: an aliased category must never turn an unrelated
// option into the model authority, nor may a nested value replace its ACK.
func configOptionsValid(raw []byte) bool {
	return configOptionsValidMode(raw, false)
}
func configOptionsValidMode(raw []byte, discovery bool) bool {
	var options []json.RawMessage
	if json.Unmarshal(raw, &options) != nil || options == nil || len(options) > 128 {
		return false
	}
	optionalString := func(m map[string]json.RawMessage, key string) bool {
		r, present := m[key]
		if !present || string(r) == "null" {
			return true
		}
		var s string
		return json.Unmarshal(r, &s) == nil && text(s, 4096, true)
	}
	for _, rawOption := range options {
		o := object(rawOption)
		if !closed(o, "_meta", "id", "name", "category", "type", "currentValue", "description", "options") || !token(stringField(o, "id"), 128) || !token(stringField(o, "name"), 4096) || !optionalString(o, "description") {
			return false
		}
		if c, present := o["category"]; present && string(c) != "null" && !token(stringField(o, "category"), 128) {
			return false
		}
		switch stringField(o, "type") {
		case "select":
			currentOK := token(stringField(o, "currentValue"), 256)
			if discovery {
				var current string
				value, present := o["currentValue"]
				currentOK = !present || string(value) == "null" || json.Unmarshal(value, &current) == nil && (current == "" || token(current, 256))
			}
			if !currentOK || !configValuesValid(o["options"], true) {
				return false
			}
		case "boolean":
			if o["options"] != nil || string(o["currentValue"]) != "true" && string(o["currentValue"]) != "false" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func configValuesValid(raw []byte, groups bool) bool {
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 || len(entries) > 256 {
		return false
	}
	grouped := groups && object(entries[0])["group"] != nil
	for _, rawEntry := range entries {
		e := object(rawEntry)
		if !token(stringField(e, "name"), 4096) {
			return false
		}
		if grouped {
			if !closed(e, "_meta", "group", "name", "options") || !token(stringField(e, "group"), 128) || !configValuesValid(e["options"], false) {
				return false
			}
		} else {
			if !closed(e, "_meta", "value", "name", "description") || !token(stringField(e, "value"), 256) {
				return false
			}
			if r, present := e["description"]; present && string(r) != "null" {
				var s string
				if json.Unmarshal(r, &s) != nil || !text(s, 4096, true) {
					return false
				}
			}
		}
	}
	return true
}
func strictJSON(raw []byte) bool {
	return strictJSONLimit(raw, frameLimit)
}
func strictJSONLimit(raw []byte, limit int) bool {
	if len(raw) > limit || !tc.ValidJSONUnicode(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	budget := 16384
	var value func(int) bool
	value = func(depth int) bool {
		budget--
		if depth > 32 || budget < 0 {
			return false
		}
		t, e := d.Token()
		if e != nil {
			return false
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				s, ok := k.(string)
				if e != nil || !ok || seen[s] {
					return false
				}
				seen[s] = true
				if !value(depth + 1) {
					return false
				}
			}
			end, e := d.Token()
			return e == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, e := d.Token()
			return e == nil && end == json.Delim(']')
		}
		return false
	}
	if !value(0) {
		return false
	}
	_, e := d.Token()
	return e == io.EOF
}
func idKey(raw []byte) string {
	if len(raw) == 0 || len(raw) > 256 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if !token(s, 128) {
			return ""
		}
		b, _ := json.Marshal(s)
		return string(b)
	}
	s = string(raw)
	if s == "" {
		return ""
	}
	for i, r := range s {
		if r == '-' && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return ""
		}
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil {
		return ""
	}
	return strconv.FormatInt(n, 10)
}
func envelope(raw []byte) (map[string]json.RawMessage, bool) {
	return envelopeLimit(raw, frameLimit)
}
func envelopeLimit(raw []byte, limit int) (map[string]json.RawMessage, bool) {
	if !strictJSONLimit(raw, limit) {
		return nil, false
	}
	m := object(raw)
	if stringField(m, "jsonrpc") != "2.0" {
		return nil, false
	}
	for k := range m {
		switch k {
		case "jsonrpc", "id", "method", "params", "result", "error":
		default:
			return nil, false
		}
	}
	return m, true
}

func (d *Driver) Write(raw []byte) (int, error) {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	original := len(raw)
	for len(raw) > 0 {
		at := bytes.IndexByte(raw, '\n')
		part := raw
		if at >= 0 {
			part = raw[:at]
		}
		limit := frameLimit
		if d.options.Runtime == "cursor" && d.discovery == nil {
			limit = 1 << 20
		}
		if len(d.pending)+len(part) > limit {
			d.fail("acp_frame_limited")
			return original, nil
		}
		d.pending = append(d.pending, part...)
		if at < 0 {
			break
		}
		frame := bytes.TrimSuffix(d.pending, []byte{'\r'})
		d.pending = nil
		if len(bytes.TrimSpace(frame)) != 0 {
			if e := d.frame(frame); e != nil {
				d.fail("acp_protocol_invalid")
				return original, nil
			}
		}
		raw = raw[at+1:]
	}
	return original, nil
}
func (d *Driver) frame(raw []byte) error {
	limit := frameLimit
	if d.options.Runtime == "cursor" && d.discovery == nil {
		limit = 1 << 20
	}
	m, ok := envelopeLimit(raw, limit)
	if !ok {
		return errors.New("acp_protocol_invalid")
	}
	key := idKey(m["id"])
	method := stringField(m, "method")
	if len(raw) > frameLimit && method != "cursor/ask_question" && method != "cursor/create_plan" {
		return errors.New("acp_frame_limited")
	}
	_, hasID := m["id"]
	params := object(m["params"])
	cursorCatalogue := d.options.Runtime == "cursor" && !hasID && method == "session/update" && stringField(object(params["update"]), "sessionUpdate") == "available_commands_update"
	d.mu.Lock()
	d.frames++
	d.wireBytes += len(raw)
	if d.bad != nil || d.frames > 4096 || d.wireBytes > 8<<20 || d.promptResponse || d.discovery != nil && d.discovery.complete && !cursorCatalogue {
		d.mu.Unlock()
		return errors.New("acp_protocol_invalid")
	}
	if d.options.Runtime == "cursor" && !hasID && cursorNotification(method) {
		session := d.result.Evidence.SessionID
		if m["result"] != nil || m["error"] != nil || params == nil || session == "" || !d.result.Evidence.PromptStarted || d.discovery != nil {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
		if rawSession, present := params["sessionId"]; present {
			var supplied string
			if json.Unmarshal(rawSession, &supplied) != nil || supplied != session {
				d.mu.Unlock()
				return errors.New("acp_protocol_invalid")
			}
		}
		d.mu.Unlock()
		return d.cursorObservation(method, session, m["params"])
	}
	if d.options.Runtime == "cursor" && hasID && (method == "cursor/ask_question" || method == "cursor/create_plan" || method == "cursor/task") {
		if method != "cursor/task" && d.cursor != nil && d.cursor.handler.Open != nil {
			d.mu.Unlock()
			return d.acceptCursorInput(m)
		}
		session := d.result.Evidence.SessionID
		started := d.result.Evidence.PromptStarted
		d.mu.Unlock()
		if session != "" && started {
			_ = d.emit(Event{Kind: "notice", SessionID: session, Text: "Unsupported Cursor request: " + method})
		}
		return errors.New("acp_cursor_request_unsupported")
	}
	if method == "" {
		original, exists := d.requests[key]
		if !hasID || key == "" || !exists || m["params"] != nil {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
		delete(d.requests, key)
		if (m["result"] == nil) == (m["error"] == nil) || m["result"] != nil && object(m["result"]) == nil || m["error"] != nil && object(m["error"]) == nil {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
		if m["result"] != nil {
			r := object(m["result"])
			switch original {
			case "initialize":
				if !closed(r, "_meta", "protocolVersion", "agentCapabilities", "agentInfo", "authMethods") || string(r["protocolVersion"]) != "1" {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
			case "session/new":
				session := stringField(r, "sessionId")
				keys := []string{"_meta", "sessionId", "configOptions", "modes"}
				if d.options.Runtime == "cursor" {
					keys = append(keys, "models")
				}
				if !closed(r, keys...) || r["models"] != nil && !cursorModelsValid(r["models"]) || !configOptionsValidMode(r["configOptions"], d.discovery != nil || d.options.Runtime == "cursor") || !token(session, 128) || hasSecret(session, d.options.Secrets) || d.result.Evidence.SessionID != "" {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				d.result.Evidence.SessionID = session
				if d.discovery != nil {
					d.discovery.evidence.SessionCreated = true
				}
			case "session/set_config_option":
				if !closed(r, "_meta", "configOptions") || !configOptionsValid(r["configOptions"]) {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
			case "session/prompt":
				if !closed(r, "_meta", "stopReason", "usage", "userMessageId") || !token(stringField(r, "stopReason"), 128) {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				d.promptResponse = true
				d.closeCursorInputsLocked()
				if rawUsage, present := r["usage"]; present {
					value, e := projectUsage(rawUsage)
					if e != nil {
						d.mu.Unlock()
						return e
					}
					d.result.Usage = value
					if value != nil {
						d.result.UsageState = "partial"
					}
				}
			}
		}
	} else {
		if m["result"] != nil || m["error"] != nil || hasID && key == "" {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
		if d.discovery != nil && !cursorCatalogue {
			d.discovery.evidence.UnexpectedActivity++
			if method == "session/request_permission" {
				d.discovery.evidence.PermissionRequests++
			}
		}
		if params == nil || stringField(params, "sessionId") != d.result.Evidence.SessionID || d.result.Evidence.SessionID == "" {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
		if hasID {
			if d.options.Runtime == "cursor" {
				fresh, e := d.registerCursorRequest(key, method, rawDigest(m["params"]))
				if e != nil || !fresh {
					d.mu.Unlock()
					return e
				}
			}
			if _, exists := d.remote[key]; exists || len(d.remote) >= 64 {
				d.mu.Unlock()
				return errors.New("acp_protocol_invalid")
			}
			p := &pendingPermission{wireID: key, raw: append([]byte{}, m["params"]...)}
			if method == "session/request_permission" {
				if !closed(params, "_meta", "sessionId", "toolCall", "options") {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				if d.discovery == nil && !d.result.Evidence.PromptStarted || d.result.Evidence.PermissionsRequested >= 32 {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				tool := stringField(object(params["toolCall"]), "toolCallId")
				if !token(tool, 128) || hasSecret(tool, d.options.Secrets) {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				if old := d.permissionTools[tool]; old != nil && !old.decided {
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
				p.toolID = tool
				d.permissionTools[tool] = p
				d.result.Evidence.PermissionsRequested++
				if prior, exists := d.tools[tool]; exists && (prior == "completed" || prior == "failed") {
					d.toolInvalid = true
					d.tools[tool] = "permission_pending"
				}
				if _, exists := d.tools[tool]; !exists {
					d.tools[tool] = "permission_pending"
				}
			} else {
				switch method {
				case "fs/read_text_file", "fs/write_text_file", "terminal/create", "terminal/output", "terminal/release", "terminal/wait_for_exit", "terminal/kill":
					d.incomplete = true
				default:
					d.mu.Unlock()
					return errors.New("acp_protocol_invalid")
				}
			}
			d.remote[key] = p
		} else if method != "session/update" || d.discovery != nil && !cursorCatalogue {
			d.mu.Unlock()
			return errors.New("acp_protocol_invalid")
		}
	}
	d.mu.Unlock()
	if method == "session/update" {
		if e := d.projectUpdate(object(m["params"])); e != nil {
			return e
		}
	}
	_, e := d.input.Write(append(append([]byte{}, raw...), '\n'))
	return e
}

// SDK transport stays authoritative. This bounded writer records correlation
// before transmission, and explicitly advertises the disabled host callbacks.
func (w sdkOutput) Write(raw []byte) (int, error) {
	d := w.d
	m, ok := envelope(bytes.TrimSpace(raw))
	if !ok {
		return 0, errors.New("acp_outbound_invalid")
	}
	method := stringField(m, "method")
	key := idKey(m["id"])
	item := outbound{raw: append([]byte{}, raw...)}
	d.mu.Lock()
	if d.localClosed {
		d.mu.Unlock()
		return 0, errors.New("acp_closed")
	}
	if method != "" {
		if d.discovery != nil {
			if method == "session/prompt" {
				d.discovery.evidence.PromptRequests++
			}
			if method == "session/set_config_option" {
				d.discovery.evidence.ConfigurationWrites++
			}
			if method != "initialize" && method != "session/new" && method != "session/cancel" || method == "session/cancel" && (!d.discovery.evidence.SessionCreated || key != "") {
				d.mu.Unlock()
				return 0, errors.New("acp_outbound_invalid")
			}
		}
		switch method {
		case "initialize", "session/new", "session/set_config_option", "session/prompt":
			if key == "" || d.requests[key] != "" {
				d.mu.Unlock()
				return 0, errors.New("acp_outbound_invalid")
			}
			d.requests[key] = method
			if method == "session/prompt" && d.cursor != nil {
				prompt, e := cursorRPCID(m["id"])
				if e != nil {
					d.mu.Unlock()
					return 0, e
				}
				d.cursor.prompt = prompt
			}
			item.prompt = method == "session/prompt"
		case "session/cancel", "$/cancel_request":
			if d.discovery != nil {
				d.discovery.evidence.SessionCancelRequested = true
				item.discoveryCancel = true
			} else {
				d.result.Evidence.CancelRequested = true
			}
		default:
			d.mu.Unlock()
			return 0, errors.New("acp_outbound_invalid")
		}
	} else if p := d.remote[key]; p != nil {
		item.reply = key
	} else {
		d.mu.Unlock()
		return 0, errors.New("acp_outbound_invalid")
	}
	d.mu.Unlock()
	if method == "initialize" {
		params := object(m["params"])
		capabilities, _ := json.Marshal(map[string]any{"fs": map[string]bool{"readTextFile": false, "writeTextFile": false}, "terminal": false, "auth": map[string]bool{"terminal": false}})
		params["clientCapabilities"] = capabilities
		m["params"], _ = json.Marshal(params)
		item.raw, _ = json.Marshal(m)
		item.raw = append(item.raw, '\n')
	}
	timer := time.NewTimer(1500 * time.Millisecond)
	defer timer.Stop()
	select {
	case d.output <- item:
		return len(raw), nil
	case <-d.ctx.Done():
		return 0, errors.New("acp_closed")
	case <-d.outEnd:
		return 0, errors.New("acp_closed")
	case <-timer.C:
		d.fail("acp_stdin_limited")
		return 0, errors.New("acp_stdin_limited")
	}
}
