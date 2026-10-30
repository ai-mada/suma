package workbuddyacp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/ai-mada/suma/internal/toolcontract"
	acp "github.com/coder/acp-go-sdk"
)

// Packet is an internal, untrusted observation. No response means completion
// until the session owner has correlated its exact request and typed result.
// SDK payloads and original bytes are deliberately excluded from diagnostics.
type Packet struct {
	Kind       string                        `json:"kind"`
	SessionID  string                        `json:"session_id,omitempty"`
	RPCID      json.RawMessage               `json:"-"`
	Method     string                        `json:"-"`
	Update     *acp.SessionNotification      `json:"-"`
	Permission *acp.RequestPermissionRequest `json:"-"`
	Artifact   *ArtifactNotice               `json:"-"`
	Result     json.RawMessage               `json:"-"`
	ErrorCode  *int32                        `json:"error_code,omitempty"`
	Raw        Frame                         `json:"-"`
}

func (Packet) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.Packet{payload:redacted}")
}

// ArtifactNotice records identity and provider change only; URI is not a
// trusted download endpoint, local filename or permission to access either.
type ArtifactNotice struct {
	SessionID string          `json:"-"`
	Event     string          `json:"-"`
	Type      string          `json:"-"`
	URI       string          `json:"-"`
	Body      json.RawMessage `json:"-"`
}

func (ArtifactNotice) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddyacp.ArtifactNotice{payload:redacted}")
}

type codecError struct{}

func (codecError) Error() string { return "WorkBuddy ACP frame is invalid" }
func (codecError) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "WorkBuddy ACP frame is invalid")
}

var codecUnsupported = errors.New("unsupported ACP observation")
var codecInteger = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
var codecSessionID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

func DecodeFrame(frame Frame, expectedSessionID string) (Packet, error) {
	bad := func() (Packet, error) { return Packet{}, codecError{} }
	if !codecSessionID.MatchString(expectedSessionID) || len(frame.Data) > 1<<20 || !toolcontract.ValidJSONUnicode(frame.Data) {
		return bad()
	}
	decoder := json.NewDecoder(bytes.NewReader(frame.Data))
	decoder.UseNumber()
	remaining := 65536
	if !codecUniqueValue(decoder, 0, &remaining) {
		return bad()
	}
	if _, err := decoder.Token(); err != io.EOF {
		return bad()
	}
	envelope := codecObject(frame.Data)
	if envelope == nil || codecString(envelope, "jsonrpc", 8) != "2.0" {
		return bad()
	}
	for key := range envelope {
		if !codecOneOf(key, "jsonrpc", "id", "method", "params", "result", "error") {
			return bad()
		}
	}
	frame.Data = bytes.Clone(frame.Data)
	packet := Packet{Raw: frame}
	id, hasID := envelope["id"]
	if hasID {
		if !codecRPCID(id) {
			return bad()
		}
		packet.RPCID = bytes.Clone(id)
	}
	_, hasMethod := envelope["method"]
	result, hasResult := envelope["result"]
	wireError, hasError := envelope["error"]
	if !hasMethod {
		if !hasID || hasResult == hasError || envelope["params"] != nil {
			return bad()
		}
		packet.Kind = "response"
		if hasResult {
			packet.Result = bytes.Clone(result)
		} else {
			body := codecObject(wireError)
			var code int32
			if body == nil || body["code"] == nil || bytes.Equal(body["code"], []byte("null")) || json.Unmarshal(body["code"], &code) != nil || !codecHasString(body, "message", 4096, true) {
				return bad()
			}
			for key := range body {
				if !codecOneOf(key, "code", "message", "data") {
					return bad()
				}
			}
			packet.ErrorCode = &code
		}
		return packet, nil
	}
	method := codecString(envelope, "method", 128)
	if method == "" || hasResult || hasError {
		return bad()
	}
	packet.Method = method
	params := codecObject(envelope["params"])
	if envelope["params"] != nil && params == nil {
		return bad()
	}
	if params["sessionId"] != nil {
		packet.SessionID = codecString(params, "sessionId", 200)
		if packet.SessionID != expectedSessionID {
			return bad()
		}
	}
	switch method {
	case "session/update":
		if hasID || packet.SessionID != expectedSessionID {
			return bad()
		}
		update := codecObject(params["update"])
		tag := codecString(update, "sessionUpdate", 80)
		if tag == "" {
			return bad()
		}
		if !codecOneOf(tag, "user_message_chunk", "agent_message_chunk", "agent_thought_chunk", "tool_call", "tool_call_update", "plan", "plan_update", "plan_removed", "available_commands_update", "current_mode_update", "config_option_update", "session_info_update", "usage_update") {
			packet.Kind = "unsupported"
			return packet, nil
		}
		if err := codecValidateUpdate(update, tag); err != nil {
			if errors.Is(err, codecUnsupported) {
				packet.Kind = "unsupported"
				return packet, nil
			}
			return bad()
		}
		var notification acp.SessionNotification
		if json.Unmarshal(envelope["params"], &notification) != nil || notification.Validate() != nil || notification.Update.Validate() != nil || !codecUnionMatches(notification.Update, tag) {
			return bad()
		}
		packet.Kind, packet.Update = "session_update", &notification
	case "session/request_permission":
		if !hasID || packet.SessionID != expectedSessionID || codecValidatePermission(params) != nil {
			return bad()
		}
		var request acp.RequestPermissionRequest
		if json.Unmarshal(envelope["params"], &request) != nil || request.Validate() != nil {
			return bad()
		}
		packet.Kind, packet.Permission = "permission_request", &request
	case "_codebuddy.ai/artifact":
		if hasID || packet.SessionID != expectedSessionID {
			return bad()
		}
		body := codecObject(params["artifact"])
		event := codecString(params, "event", 32)
		kind := codecString(body, "type", 32)
		uri := codecString(body, "uri", 2048)
		if body == nil || event == "" || kind == "" || uri == "" {
			return bad()
		}
		if !codecOneOf(event, "created", "updated", "deleted") || !codecOneOf(kind, "plan", "tasks", "media", "overview") {
			packet.Kind = "unsupported"
			return packet, nil
		}
		packet.Kind = "artifact"
		packet.Artifact = &ArtifactNotice{SessionID: expectedSessionID, Event: event, Type: kind, URI: uri, Body: bytes.Clone(params["artifact"])}
	default:
		packet.Kind = "unsupported"
	}
	return packet, nil
}

// Validate recursively before SDK maps can collapse duplicate names or replace
// invalid unions. Byte, token and depth bounds are local resource constraints.
func codecUniqueValue(d *json.Decoder, depth int, remaining *int) bool {
	*remaining--
	if depth > 64 || *remaining < 0 {
		return false
	}
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, container := token.(json.Delim)
	if !container {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			seen[name] = true
			if !codecUniqueValue(d, depth+1, remaining) {
				return false
			}
		}
		close, err := d.Token()
		return err == nil && close == json.Delim('}')
	case '[':
		for d.More() {
			if !codecUniqueValue(d, depth+1, remaining) {
				return false
			}
		}
		close, err := d.Token()
		return err == nil && close == json.Delim(']')
	default:
		return false
	}
}
func codecObject(raw []byte) map[string]json.RawMessage {
	raw = bytes.TrimSpace(raw)
	var fields map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}
func codecOneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
func codecHasString(fields map[string]json.RawMessage, key string, max int, empty bool) bool {
	var value string
	raw := fields[key]
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil || len(value) > max || !empty && strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func codecString(fields map[string]json.RawMessage, key string, max int) string {
	if !codecHasString(fields, key, max, false) {
		return ""
	}
	var value string
	_ = json.Unmarshal(fields[key], &value)
	return value
}
func codecRPCID(raw []byte) bool {
	if len(raw) == 0 || len(raw) > 256 {
		return false
	}
	if raw[0] == '"' {
		return codecHasString(map[string]json.RawMessage{"id": raw}, "id", 200, false)
	}
	return codecInteger.Match(raw)
}
func codecArray(raw []byte, max int) ([]json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	var list []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &list) != nil || len(list) > max {
		return nil, false
	}
	return list, true
}
func codecRequiredText(fields map[string]json.RawMessage, key string) bool {
	var value string
	raw := fields[key]
	return len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &value) == nil && len(value) <= 1<<20
}
func codecNumber(fields map[string]json.RawMessage, key string) bool {
	var value int64
	raw := fields[key]
	return len(raw) > 0 && codecInteger.Match(raw) && json.Unmarshal(raw, &value) == nil && value >= 0 && value <= 9007199254740991
}

func codecContent(raw []byte) error {
	body := codecObject(raw)
	kind := codecString(body, "type", 40)
	if body == nil || kind == "" {
		return codecError{}
	}
	switch kind {
	case "text":
		if !codecRequiredText(body, "text") {
			return codecError{}
		}
	case "image", "audio":
		if !codecRequiredText(body, "data") || codecString(body, "mimeType", 256) == "" {
			return codecError{}
		}
	case "resource_link":
		if codecString(body, "uri", 2048) == "" || codecString(body, "name", 512) == "" {
			return codecError{}
		}
	case "resource":
		resource := codecObject(body["resource"])
		if codecString(resource, "uri", 2048) == "" || (resource["text"] == nil) == (resource["blob"] == nil) {
			return codecError{}
		}
		if resource["text"] != nil && !codecRequiredText(resource, "text") || resource["blob"] != nil && !codecRequiredText(resource, "blob") {
			return codecError{}
		}
	default:
		return codecUnsupported
	}
	return nil
}
func codecToolContent(fields map[string]json.RawMessage) error {
	if fields["content"] == nil {
		return nil
	}
	list, ok := codecArray(fields["content"], 256)
	if !ok {
		return codecError{}
	}
	for _, raw := range list {
		body := codecObject(raw)
		switch codecString(body, "type", 40) {
		case "content":
			if err := codecContent(body["content"]); err != nil {
				return err
			}
		case "diff":
			if codecString(body, "path", 4096) == "" || !codecRequiredText(body, "newText") {
				return codecError{}
			}
			if body["oldText"] != nil && !bytes.Equal(body["oldText"], []byte("null")) && !codecRequiredText(body, "oldText") {
				return codecError{}
			}
		case "terminal":
			if codecString(body, "terminalId", 128) == "" {
				return codecError{}
			}
		default:
			return codecUnsupported
		}
	}
	return nil
}
func codecEntries(raw []byte) error {
	list, ok := codecArray(raw, 512)
	if !ok {
		return codecError{}
	}
	for _, item := range list {
		entry := codecObject(item)
		if !codecRequiredText(entry, "content") || !codecOneOf(codecString(entry, "priority", 32), "high", "medium", "low") || !codecOneOf(codecString(entry, "status", 32), "pending", "in_progress", "completed") {
			return codecError{}
		}
	}
	return nil
}
func codecValidateUpdate(update map[string]json.RawMessage, tag string) error {
	switch tag {
	case "user_message_chunk", "agent_message_chunk", "agent_thought_chunk":
		return codecContent(update["content"])
	case "tool_call", "tool_call_update":
		if codecString(update, "toolCallId", 128) == "" || tag == "tool_call" && codecString(update, "title", 4096) == "" {
			return codecError{}
		}
		if update["status"] != nil && !codecOneOf(codecString(update, "status", 32), "pending", "in_progress", "completed", "failed") {
			return codecError{}
		}
		return codecToolContent(update)
	case "plan":
		return codecEntries(update["entries"])
	case "plan_update":
		plan := codecObject(update["plan"])
		if codecString(plan, "id", 128) == "" {
			return codecError{}
		}
		switch codecString(plan, "type", 32) {
		case "items":
			return codecEntries(plan["entries"])
		case "file":
			if codecString(plan, "uri", 2048) == "" {
				return codecError{}
			}
		case "markdown":
			if !codecRequiredText(plan, "content") {
				return codecError{}
			}
		default:
			return codecUnsupported
		}
	case "plan_removed":
		if codecString(update, "id", 128) == "" {
			return codecError{}
		}
	case "available_commands_update":
		commands, ok := codecArray(update["availableCommands"], 256)
		if !ok {
			return codecError{}
		}
		seen := map[string]bool{}
		for _, raw := range commands {
			command := codecObject(raw)
			name := codecString(command, "name", 128)
			if name == "" || seen[name] || !codecRequiredText(command, "description") {
				return codecError{}
			}
			seen[name] = true
		}
	case "current_mode_update":
		if codecString(update, "currentModeId", 128) == "" {
			return codecError{}
		}
	case "config_option_update":
		options, ok := codecArray(update["configOptions"], 128)
		if !ok {
			return codecError{}
		}
		seen := map[string]bool{}
		for _, raw := range options {
			option := codecObject(raw)
			id := codecString(option, "id", 128)
			if id == "" || seen[id] || codecString(option, "name", 512) == "" {
				return codecError{}
			}
			seen[id] = true
			switch codecString(option, "type", 32) {
			case "boolean":
				if !bytes.Equal(option["currentValue"], []byte("true")) && !bytes.Equal(option["currentValue"], []byte("false")) {
					return codecError{}
				}
			case "select":
				if codecString(option, "currentValue", 128) == "" {
					return codecError{}
				}
				if !codecSelectOptions(option["options"]) {
					return codecError{}
				}
			default:
				return codecUnsupported
			}
		}
	case "session_info_update":
		for _, key := range []string{"title", "updatedAt"} {
			if update[key] != nil && !bytes.Equal(update[key], []byte("null")) && !codecHasString(update, key, 4096, true) {
				return codecError{}
			}
		}
	case "usage_update":
		if !codecNumber(update, "size") || !codecNumber(update, "used") {
			return codecError{}
		}
		if update["cost"] != nil && !bytes.Equal(update["cost"], []byte("null")) {
			cost := codecObject(update["cost"])
			var amount float64
			if len(cost["amount"]) == 0 || bytes.Equal(cost["amount"], []byte("null")) || json.Unmarshal(cost["amount"], &amount) != nil || amount < 0 || math.IsInf(amount, 0) || math.IsNaN(amount) || codecString(cost, "currency", 16) == "" {
				return codecError{}
			}
		}
	}
	return nil
}

func codecSelectOptions(raw []byte) bool {
	list, ok := codecArray(raw, 256)
	if !ok {
		return false
	}
	values, groups := map[string]bool{}, map[string]bool{}
	grouped := len(list) > 0 && codecObject(list[0])["group"] != nil
	checkValue := func(raw []byte) bool {
		option := codecObject(raw)
		value := codecString(option, "value", 128)
		if value == "" || values[value] || len(values) >= 512 || codecString(option, "name", 512) == "" || option["group"] != nil || option["options"] != nil {
			return false
		}
		values[value] = true
		return true
	}
	for _, raw := range list {
		if !grouped {
			if !checkValue(raw) {
				return false
			}
			continue
		}
		group := codecObject(raw)
		id := codecString(group, "group", 128)
		options, ok := codecArray(group["options"], 256)
		if !ok || id == "" || groups[id] || codecString(group, "name", 512) == "" || group["value"] != nil {
			return false
		}
		groups[id] = true
		for _, option := range options {
			if !checkValue(option) {
				return false
			}
		}
	}
	return true
}
func codecUnionMatches(update acp.SessionUpdate, tag string) bool {
	// The generated union marshal writes its selected variant's canonical tag.
	// Compare it to the incoming tag rather than trusting shape-based fallback.
	raw, err := json.Marshal(update)
	return err == nil && codecString(codecObject(raw), "sessionUpdate", 80) == tag
}
func codecValidatePermission(params map[string]json.RawMessage) error {
	call := codecObject(params["toolCall"])
	if codecString(call, "toolCallId", 128) == "" {
		return codecError{}
	}
	if err := codecToolContent(call); err != nil {
		return err
	}
	options, ok := codecArray(params["options"], 32)
	if !ok || len(options) == 0 {
		return codecError{}
	}
	seen := map[string]bool{}
	for _, raw := range options {
		option := codecObject(raw)
		id := codecString(option, "optionId", 128)
		if id == "" || seen[id] || codecString(option, "name", 512) == "" || !codecOneOf(codecString(option, "kind", 32), "allow_once", "allow_always", "reject_once", "reject_always") {
			return codecError{}
		}
		seen[id] = true
	}
	return nil
}
