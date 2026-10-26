package runner

import (
	"fmt"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This decoder projects the public Multica DSH v1 frames, not CLI prose. Model
// reasoning, arbitrary tool arguments and tool result bodies are not retained.
type dshStreamState struct {
	ready    bool
	session  string
	lastText string
	calls    map[string]string
}

func dshString(m map[string]any, key string, nonempty bool) bool {
	v, ok := m[key].(string)
	return ok && (!nonempty || strings.TrimSpace(v) != "")
}

func (s *toolStream) dsh(m map[string]any) {
	if s.dshState == nil {
		s.dshState = &dshStreamState{calls: map[string]string{}}
	}
	d := s.dshState
	v, valid := nativeInteger(m, "v")
	if !valid || v != 1 {
		s.invalid("unsupported dsh protocol version")
		return
	}
	kind := textAt(m, "type")
	if kind == "protocol_error" {
		s.invalid("dsh runtime reported a protocol error")
		return
	}
	if kind == "ready" {
		if d.ready || s.sessionStarted || textAt(m, "runtime") != "dsh" || !dshString(m, "plugin_version", true) || objectAt(m, "capabilities") == nil {
			s.invalid("invalid or duplicate dsh readiness handshake")
			return
		}
		// The official ready frame has no protocol_version; probe does. Do not
		// require a field absent from the wire, but reject a contradictory one.
		if _, present := m["protocol_version"]; present {
			pv, ok := nativeInteger(m, "protocol_version")
			if !ok || pv != 1 {
				s.invalid("unsupported dsh readiness version")
				return
			}
		}
		if id, present := m["request_id"]; present && id != s.expectedRequestID {
			s.invalid("foreign dsh readiness identity")
			return
		}
		d.ready = true
		return
	}
	if !d.ready || s.expectedRequestID == "" || textAt(m, "request_id") != s.expectedRequestID {
		s.invalid("dsh frame precedes readiness or has a foreign request identity")
		return
	}
	if kind != "session" && kind != "result" && !s.sessionStarted {
		s.invalid("dsh activity before session")
		return
	}
	if raw, present := m["session_id"]; present {
		id, ok := raw.(string)
		if !ok || strings.TrimSpace(id) == "" || d.session != "" && d.session != id {
			s.invalid("missing or changed dsh session identity")
			return
		}
	}
	switch kind {
	case "session":
		resumed, ok := m["resumed"].(bool)
		if s.sessionStarted || !dshString(m, "session_id", true) || !ok || resumed {
			s.invalid("invalid, duplicate or unsolicited resumed dsh session")
			return
		}
		d.session = textAt(m, "session_id")
		s.session = s.id(d.session)
		s.sessionStarted, s.turnStarted = true, true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
	case "text", "thinking":
		if !dshString(m, "content", false) {
			s.invalid("invalid dsh message")
			return
		}
		if kind == "text" {
			d.lastText = textAt(m, "content")
			s.event("message", "", d.lastText, "", nil)
		}
	case "tool_call":
		id, name := textAt(m, "call_id"), textAt(m, "name")
		if !dshString(m, "call_id", true) || !dshString(m, "name", true) || !dshString(m, "arguments", false) || d.calls[id] != "" {
			s.invalid("invalid or duplicate dsh tool call")
			return
		}
		d.calls[id] = name
		s.event("activity_started", id, name, "", nil)
	case "tool_result":
		id, name := textAt(m, "call_id"), textAt(m, "name")
		isError, ok := m["is_error"].(bool)
		if !ok || !dshString(m, "output", false) || id == "" || d.calls[id] == "" || d.calls[id] != name {
			s.invalid("dsh tool result does not match its call")
			return
		}
		delete(d.calls, id)
		status := "completed"
		if isError {
			status = "error"
		}
		s.event("activity_completed", id, name, status, nil)
	case "usage":
		selected := map[string]any{}
		for source, target := range map[string]string{"input_tokens": "input_tokens", "output_tokens": "output_tokens", "cache_read_tokens": "cached_input_tokens", "cache_write_tokens": "cache_write_input_tokens", "reasoning_tokens": "reasoning_output_tokens"} {
			if value, present := m[source]; present {
				selected[target] = value
			}
		}
		u, err := parseUsage(selected)
		if err != nil {
			s.invalid("invalid dsh token usage")
			return
		}
		state := tc.UsageComplete
		if u == nil {
			state = tc.UsageUnreported
		} else if u.InputTokens == nil || u.OutputTokens == nil {
			state = tc.UsagePartial
		}
		if err = s.accumulateUsage(u); err != nil {
			s.invalid(err.Error())
			return
		}
		s.usageSteps++
		s.usagePartial = s.usagePartial || state == tc.UsagePartial
		s.event("usage", "", "", "", u, state)
	case "result":
		status := textAt(m, "status")
		resumeRejected, ok := m["resume_rejected"].(bool)
		if !ok || !dshString(m, "output", false) {
			s.invalid("invalid dsh terminal fields")
			return
		}
		message := ""
		if raw, present := m["error"]; present {
			errorFields, ok := raw.(map[string]any)
			if !ok || !dshString(errorFields, "code", true) || !dshString(errorFields, "message", false) {
				s.invalid("invalid dsh terminal error")
				return
			}
			message = fmt.Sprintf("%s: %s", textAt(errorFields, "code"), textAt(errorFields, "message"))
		}
		switch status {
		case "completed":
			if !s.sessionStarted || resumeRejected || message != "" || len(d.calls) != 0 {
				s.invalid("dsh success contradicts its execution facts")
				return
			}
			if textAt(m, "output") != "" && textAt(m, "output") != d.lastText {
				s.event("message", "", textAt(m, "output"), "", nil)
			}
			if s.bad != nil {
				return
			}
			s.setTerminal("completed")
			s.event("turn_completed", "", "", status, nil)
		case "failed", "aborted", "cancelled":
			if message == "" {
				message = "dsh execution " + status
			}
			s.setTerminal("failed")
			s.event("turn_failed", "", message, status, nil)
		default:
			s.invalid("unsupported dsh whole-run terminal status")
		}
	default:
		s.invalid("unsupported dsh frame type")
	}
}
