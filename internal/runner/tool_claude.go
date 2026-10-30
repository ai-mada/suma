package runner

import (
	"fmt"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
)

type claudeStreamState struct {
	session          string
	proof            tc.ClaudeExecutionEvidence
	pendingTools     map[string]bool
	usageMessages    map[string]bool
	assistantUsage   *tc.Usage
	sawError, denied bool
}

func (s *toolStream) claude(m map[string]any) {
	if s.claudeState == nil {
		c := s.profile.ClaudeExecution
		if c == nil {
			s.invalid("Claude fixed configuration is missing")
			return
		}
		s.claudeState = &claudeStreamState{pendingTools: map[string]bool{}, usageMessages: map[string]bool{}, proof: tc.ClaudeExecutionEvidence{ConfigurationSHA256: c.ConfigurationSHA256, RequestedModel: c.Model}}
	}
	state := s.claudeState
	if parent, present := m["parent_tool_use_id"]; present && parent != nil && parent != "" {
		s.invalid("Claude subagent output is outside the foreground contract")
		return
	}
	if raw, present := m["session_id"]; present {
		id, ok := raw.(string)
		if !ok || id == "" || state.session != "" && id != state.session {
			s.invalid("Claude session identity is missing or changed")
			return
		}
	}
	kind := textAt(m, "type")
	if kind == "control_request" || kind == "waiting_input" || kind == "waiting_for_input" {
		s.event("waiting_input", "", "Claude requires unsupported interactive authorization", "", nil)
		s.invalid("Claude permission control is not automatically approved")
		return
	}
	if kind == "system" && textAt(m, "subtype") == "init" {
		id := textAt(m, "session_id")
		model := textAt(m, "model")
		if s.sessionStarted || id == "" || model != state.proof.RequestedModel {
			s.invalid("Claude initialization does not acknowledge the fixed session and model")
			return
		}
		if mode, ok := m["permissionMode"]; ok && mode != "dontAsk" {
			s.invalid("Claude permission mode differs from the fixed policy")
			return
		}
		if version, ok := m["claude_code_version"]; ok {
			v, valid := version.(string)
			if !valid || tc.ValidateClaudeCLIVersion(v) != nil {
				s.invalid("Claude initialized an unsupported version")
				return
			}
		}
		for _, field := range []string{"mcp_servers", "plugins"} {
			if raw, present := m[field]; present {
				values, ok := raw.([]any)
				if !ok || len(values) != 0 {
					s.invalid("Claude initialized an unsupported external extension")
					return
				}
			}
		}
		if raw, present := m["tools"]; present {
			tools, ok := raw.([]any)
			if !ok {
				s.invalid("invalid Claude initial tool list")
				return
			}
			for _, rawTool := range tools {
				switch rawTool {
				case "Read", "Edit", "Write", "Glob", "Grep", "Bash", "EndConversation":
				default:
					s.invalid("Claude initialized an unsupported tool")
					return
				}
			}
		}
		state.session = id
		s.session = s.id(id)
		state.proof.SessionID = s.session
		state.proof.AcknowledgedModel = model
		s.sessionStarted, s.turnStarted = true, true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
		return
	}
	if !s.sessionStarted && kind != "error" {
		s.invalid("Claude activity before fixed initialization")
		return
	}
	switch kind {
	case "assistant", "user":
		s.claudeMessage(m, kind)
	case "result":
		s.claudeResult(m)
	case "system":
		switch textAt(m, "subtype") {
		case "permission_denied":
			state.denied = true
			s.event("error", "", "Claude denied a tool request", "", nil)
		case "task_started", "task_progress", "task_notification", "task_updated", "elicitation", "permission_request", "waiting_input":
			s.invalid("Claude background or interactive work is unsupported")
		case "status", "compact_boundary": // Bounded lifecycle metadata is not an execution result.
		default:
			s.invalid("unsupported Claude system event")
		}
	case "error":
		state.sawError = true
		s.event("error", "", "Claude reported an execution error", "", nil)
	case "stream_event", "keep_alive", "rate_limit_event": // No reasoning, raw delta, auth or billing payload is retained.
	default:
		s.invalid("unsupported Claude event type")
	}
}

func (s *toolStream) claudeMessage(m map[string]any, kind string) {
	state := s.claudeState
	message := objectAt(m, "message")
	content, ok := message["content"].([]any)
	if !ok || textAt(message, "role") != "" && textAt(message, "role") != kind {
		s.invalid("invalid Claude message content")
		return
	}
	if raw, present := message["usage"]; present {
		v, ok := raw.(map[string]any)
		if !ok {
			s.invalid("invalid Claude message usage")
			return
		}
		u, err := codebuddyUsage(v, false)
		if err != nil {
			s.invalid("invalid Claude message usage")
			return
		}
		id := textAt(message, "id")
		if kind == "assistant" && id != "" && !state.usageMessages[id] {
			state.usageMessages[id] = true
			if u != nil {
				u.OutputTokens = nil
				state.assistantUsage, err = codebuddyAddUsage(state.assistantUsage, u)
				if err != nil {
					s.invalid("Claude usage exceeds the safe integer range")
					return
				}
				s.usage = state.assistantUsage
				s.usagePartial = true
			}
		}
	}
	for _, raw := range content {
		block, ok := raw.(map[string]any)
		if !ok {
			s.invalid("invalid Claude content block")
			return
		}
		switch textAt(block, "type") {
		case "text":
			if kind == "assistant" {
				s.event("message", textAt(message, "id"), textAt(block, "text"), "", nil)
			}
		case "thinking", "redacted_thinking": // Private reasoning is never stored.
		case "tool_use":
			id, name := textAt(block, "id"), textAt(block, "name")
			if kind != "assistant" || id == "" || state.pendingTools[id] {
				s.invalid("invalid or duplicate Claude tool identity")
				return
			}
			switch name {
			case "Read", "Edit", "Write", "Glob", "Grep", "Bash":
			default:
				s.invalid("Claude tool is outside the explicit foreground allowlist")
				return
			}
			if background, _ := objectAt(block, "input")["run_in_background"].(bool); background {
				s.invalid("Claude background tool work is unsupported")
				return
			}
			state.pendingTools[id] = true
			s.event("activity_started", id, name, "", nil)
		case "tool_result":
			id := textAt(block, "tool_use_id")
			if kind != "user" || id == "" || !state.pendingTools[id] {
				s.invalid("Claude tool result has no pending call")
				return
			}
			if value, present := block["is_error"]; present {
				if _, ok := value.(bool); !ok {
					s.invalid("invalid Claude tool error flag")
					return
				}
			}
			delete(state.pendingTools, id)
			status := "completed"
			if failed, _ := block["is_error"].(bool); failed {
				status = "error"
			}
			s.event("activity_completed", id, "tool_result", status, nil)
		default:
			s.invalid("unsupported Claude content block")
			return
		}
		if s.bad != nil {
			return
		}
	}
}

func (s *toolStream) claudeResult(m map[string]any) {
	state := s.claudeState
	proof := &state.proof
	failed, ok := m["is_error"].(bool)
	if !ok || textAt(m, "session_id") != state.session {
		s.invalid("Claude result is missing its fixed session or error flag")
		return
	}
	proof.ResultSubtype = textAt(m, "subtype")
	proof.ResultIsError = failed
	switch proof.ResultSubtype {
	case "success":
	case "error_max_turns", "error_during_execution", "error_max_budget_usd", "error_max_structured_output_retries":
		failed = true
	default:
		s.invalid("unsupported Claude result subtype")
		return
	}
	if raw, present := m["terminal_reason"]; present {
		reason, ok := raw.(string)
		if !ok || len(reason) > 80 {
			s.invalid("invalid Claude terminal reason")
			return
		}
		proof.ResultTerminalReason = reason
	}
	if raw, present := m["deferred_tool_use"]; present && raw != nil {
		proof.DeferredToolUse = true
	}
	if _, present := m["queued_turn_count"]; present {
		count, ok := nativeInteger(m, "queued_turn_count")
		if !ok || count > 1000000 {
			s.invalid("invalid Claude queued turn count")
			return
		}
		proof.QueuedTurnCount = int(count)
	}
	denials, ok := m["permission_denials"].([]any)
	if !ok || len(denials) > 1000000 {
		s.invalid("Claude result has no valid permission-denial summary")
		return
	}
	proof.PermissionDenialCount = len(denials)
	if state.denied && proof.PermissionDenialCount == 0 {
		proof.PermissionDenialCount = 1
	}
	if proof.DeferredToolUse || proof.QueuedTurnCount > 0 || proof.ResultSubtype == "success" && proof.ResultTerminalReason != "" && proof.ResultTerminalReason != "completed" || len(state.pendingTools) > 0 {
		s.invalid("Claude foreground completion remains unconfirmed")
		return
	}
	total, err := claudeResultUsage(m)
	if err != nil {
		s.invalid(err.Error())
		return
	}
	// A crash can zero the final counters. Keep independently observed input/cache
	// subtotals, counted once per message; never invent missing output or cost.
	if proof.ResultSubtype == "error_during_execution" && claudeZeroUsage(total) {
		total = state.assistantUsage
	}
	if total == nil {
		total = state.assistantUsage
	}
	s.usage = total
	s.usagePartial = total != nil
	coverage := tc.UsageUnreported
	if total != nil {
		coverage = tc.UsagePartial
	}
	failed = failed || state.sawError || proof.PermissionDenialCount > 0
	if failed {
		s.setTerminal("failed")
		s.event("turn_failed", "", "Claude did not complete the approved foreground task", "", total, coverage)
		return
	}
	if text := textAt(m, "result"); strings.TrimSpace(text) != "" {
		s.event("message", "", text, "", nil)
	}
	s.setTerminal("completed")
	s.event("turn_completed", "", "", "", total, coverage)
}

func claudeResultUsage(m map[string]any) (*tc.Usage, error) {
	var total *tc.Usage
	if raw, present := m["usage"]; present {
		v, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Claude result usage")
		}
		u, err := codebuddyUsage(v, false)
		if err != nil {
			return nil, fmt.Errorf("invalid Claude result usage")
		}
		total = u
	}
	if raw, present := m["modelUsage"]; present {
		models, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Claude model usage")
		}
		var all *tc.Usage
		for _, raw := range models {
			v, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Claude model usage")
			}
			u, err := codebuddyUsage(v, true)
			if err != nil {
				return nil, fmt.Errorf("invalid Claude model usage")
			}
			all, err = codebuddyAddUsage(all, u)
			if err != nil {
				return nil, fmt.Errorf("Claude model usage exceeds the safe integer range")
			}
		}
		if all != nil {
			total = all
		}
	}
	return total, nil
}
func claudeZeroUsage(u *tc.Usage) bool {
	if u == nil {
		return true
	}
	for _, v := range usageCounts(u) {
		if v != nil && *v > 0 {
			return false
		}
	}
	return true
}
