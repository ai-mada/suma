package runner

import (
	"fmt"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// CodeBuddy's print/stream-json protocol is a foreground, single-turn contract.
// Assistant output is progress, never a replacement for the final result. The
// CLI's permission bridge and asynchronous task protocol are not auto-approved.
type codebuddyStreamState struct {
	session        string
	pendingTools   map[string]bool
	usageMessages  map[string]bool
	assistantUsage *tc.Usage
	sawError       bool
}

func (s *toolStream) codebuddy(m map[string]any) {
	if s.codebuddyState == nil {
		s.codebuddyState = &codebuddyStreamState{pendingTools: map[string]bool{}, usageMessages: map[string]bool{}}
	}
	state := s.codebuddyState
	kind := textAt(m, "type")
	if value, exists := m["session_id"]; exists {
		id, ok := value.(string)
		if !ok || id == "" || state.session != "" && state.session != id {
			s.invalid("missing or changed CodeBuddy session identity")
			return
		}
	}
	if kind == "control_request" || kind == "waiting_input" || kind == "waiting_for_input" {
		s.event("waiting_input", "", "CodeBuddy requires interactive input or authorization", "", nil)
		s.invalid("CodeBuddy interactive input is unsupported")
		return
	}
	if kind == "system" && textAt(m, "subtype") == "init" {
		id := textAt(m, "session_id")
		if s.sessionStarted || id == "" {
			s.invalid("missing or duplicate CodeBuddy session initialization")
			return
		}
		state.session, s.session = id, s.id(id)
		s.sessionStarted, s.turnStarted = true, true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
		return
	}
	if !s.sessionStarted && kind != "error" && kind != "log" {
		s.invalid("CodeBuddy activity before session initialization")
		return
	}
	switch kind {
	case "system":
		switch textAt(m, "subtype") {
		case "task_started", "task_progress", "task_updated", "task_notification":
			s.invalid("CodeBuddy background tasks are unsupported by the foreground contract")
		case "waiting_input", "waiting_for_input", "permission_request", "elicitation":
			s.event("waiting_input", "", "CodeBuddy requires interactive input or authorization", "", nil)
			s.invalid("CodeBuddy interactive input is unsupported")
		}
	case "assistant", "user":
		s.codebuddyMessage(m, kind)
	case "result":
		s.codebuddyResult(m)
	case "error":
		state.sawError = true
		// The final result remains the only whole-run terminal. Startup errors
		// without a result therefore retain an unknown outcome at EOF.
		s.event("error", "", textAt(m, "error"), "", nil)
	case "log":
		// Logs are deliberately not copied: verbose logs can include private
		// configuration. stderr already uses the existing bounded redactor.
	case "stream_event", "keep_alive":
		// Partial-token events are never counted as complete assistant messages.
	default:
		s.invalid("unsupported CodeBuddy event type")
	}
}

func (s *toolStream) codebuddyMessage(m map[string]any, kind string) {
	message := objectAt(m, "message")
	content, ok := message["content"].([]any)
	if !ok || textAt(message, "role") != "" && textAt(message, "role") != kind {
		s.invalid("invalid CodeBuddy message content or role")
		return
	}
	if raw, present := message["usage"]; present {
		usage, valid := raw.(map[string]any)
		if !valid {
			s.invalid("invalid CodeBuddy assistant usage")
			return
		}
		u, err := codebuddyUsage(usage, false)
		if err != nil {
			s.invalid("invalid CodeBuddy assistant usage")
			return
		}
		id := textAt(message, "id")
		// Assistant output_tokens is an intermediate placeholder. Only main
		// assistant input/cache counters with a stable message ID are subtotaled;
		// the final modelUsage/usage, when supplied, replaces this partial view.
		if kind == "assistant" && textAt(m, "parent_tool_use_id") == "" && id != "" && !s.codebuddyState.usageMessages[id] {
			s.codebuddyState.usageMessages[id] = true
			if u != nil {
				u.OutputTokens = nil
				if u.InputTokens != nil || u.CachedInputTokens != nil || u.CacheWriteInputTokens != nil {
					s.codebuddyState.assistantUsage, err = codebuddyAddUsage(s.codebuddyState.assistantUsage, u)
					if err != nil {
						s.invalid("CodeBuddy usage exceeds safe integer range")
						return
					}
				}
			}
		}
	}
	for _, raw := range content {
		block, ok := raw.(map[string]any)
		if !ok {
			s.invalid("invalid CodeBuddy content block")
			return
		}
		switch textAt(block, "type") {
		case "text":
			if kind == "assistant" {
				s.event("message", textAt(message, "id"), textAt(block, "text"), "", nil)
			}
		case "thinking", "redacted_thinking":
			// Private reasoning is not retained.
		case "tool_use":
			id := textAt(block, "id")
			if kind != "assistant" || id == "" || s.codebuddyState.pendingTools[id] {
				s.invalid("invalid or duplicate CodeBuddy tool identity")
				return
			}
			input := objectAt(block, "input")
			if background, _ := input["run_in_background"].(bool); background {
				s.invalid("CodeBuddy background tasks are unsupported by the foreground contract")
				return
			}
			switch textAt(block, "name") {
			case "AskUserQuestion", "EnterPlanMode", "ExitPlanMode":
				s.event("waiting_input", id, "CodeBuddy requires interactive input or authorization", "", nil)
				s.invalid("CodeBuddy interactive input is unsupported")
				return
			}
			s.codebuddyState.pendingTools[id] = true
			s.event("activity_started", id, textAt(block, "name"), "", nil)
		case "tool_result":
			id := textAt(block, "tool_use_id")
			if kind != "user" || id == "" || !s.codebuddyState.pendingTools[id] {
				s.invalid("CodeBuddy tool result has no pending call")
				return
			}
			if raw, exists := block["is_error"]; exists {
				if _, ok := raw.(bool); !ok {
					s.invalid("invalid CodeBuddy tool result error flag")
					return
				}
			}
			delete(s.codebuddyState.pendingTools, id)
			status := "completed"
			if failed, _ := block["is_error"].(bool); failed {
				status = "error"
			}
			s.event("activity_completed", id, "tool_result", status, nil)
		default:
			s.invalid("unsupported CodeBuddy content block")
			return
		}
		if s.bad != nil {
			return
		}
	}
}

func (s *toolStream) codebuddyResult(m map[string]any) {
	failed, ok := m["is_error"].(bool)
	if !ok || textAt(m, "session_id") != s.codebuddyState.session {
		s.invalid("CodeBuddy result is missing its error flag or session identity")
		return
	}
	switch textAt(m, "subtype") {
	case "success":
	case "error_max_turns", "error_during_execution", "error_max_budget_usd", "error_max_structured_output_retries":
		failed = true
	default:
		s.invalid("unsupported CodeBuddy whole-run result subtype")
		return
	}
	if len(s.codebuddyState.pendingTools) != 0 {
		s.invalid("CodeBuddy result arrived with unfinished tool calls")
		return
	}
	if raw, present := m["permission_denials"]; present {
		denials, ok := raw.([]any)
		if !ok {
			s.invalid("invalid CodeBuddy permission denial summary")
			return
		}
		failed = failed || len(denials) > 0
	}
	var total *tc.Usage
	partialModels := false
	if raw, present := m["usage"]; present {
		usage, ok := raw.(map[string]any)
		if !ok {
			s.invalid("invalid CodeBuddy result usage")
			return
		}
		var err error
		total, err = codebuddyUsage(usage, false)
		if err != nil {
			s.invalid("invalid CodeBuddy result usage")
			return
		}
	}
	if raw, present := m["modelUsage"]; present {
		models, ok := raw.(map[string]any)
		if !ok {
			s.invalid("invalid CodeBuddy model usage")
			return
		}
		var modelTotal *tc.Usage
		for _, raw := range models {
			usage, ok := raw.(map[string]any)
			if !ok {
				s.invalid("invalid CodeBuddy model usage")
				return
			}
			u, err := codebuddyUsage(usage, true)
			if err != nil {
				s.invalid("invalid CodeBuddy model usage")
				return
			}
			partialModels = partialModels || u == nil || u.InputTokens == nil || u.OutputTokens == nil
			modelTotal, err = codebuddyAddUsage(modelTotal, u)
			if err != nil {
				s.invalid("CodeBuddy usage exceeds safe integer range")
				return
			}
		}
		if modelTotal != nil {
			total = modelTotal
		}
	}
	coverage := tc.UsageUnreported
	if total != nil {
		coverage = tc.UsagePartial
		if total.InputTokens != nil && total.OutputTokens != nil && !partialModels {
			coverage = tc.UsageComplete
		}
	} else if s.codebuddyState.assistantUsage != nil {
		total, coverage = s.codebuddyState.assistantUsage, tc.UsagePartial
	}
	s.usage, s.usagePartial = total, coverage == tc.UsagePartial
	failed = failed || s.codebuddyState.sawError
	if failed {
		s.setTerminal("failed")
		s.event("turn_failed", "", textAt(m, "result"), "", total, coverage)
		return
	}
	if text := textAt(m, "result"); strings.TrimSpace(text) != "" {
		s.event("message", "", text, "", nil)
	}
	s.setTerminal("completed")
	s.event("turn_completed", "", "", "", total, coverage)
}

func codebuddyUsage(v map[string]any, camel bool) (*tc.Usage, error) {
	fields := map[string]string{"input_tokens": "input_tokens", "output_tokens": "output_tokens", "cache_read_input_tokens": "cached_input_tokens", "cache_creation_input_tokens": "cache_write_input_tokens"}
	if camel {
		fields = map[string]string{"inputTokens": "input_tokens", "outputTokens": "output_tokens", "cacheReadInputTokens": "cached_input_tokens", "cacheCreationInputTokens": "cache_write_input_tokens"}
	}
	selected := map[string]any{}
	for source, target := range fields {
		if value, present := v[source]; present {
			selected[target] = value
		}
	}
	return parseUsage(selected)
}

func codebuddyAddUsage(a, b *tc.Usage) (*tc.Usage, error) {
	if b == nil {
		return a, nil
	}
	if a == nil {
		a = &tc.Usage{}
	}
	sources := usageCounts(b)
	targets := []**int64{&a.InputTokens, &a.CachedInputTokens, &a.CacheWriteInputTokens, &a.OutputTokens, &a.ReasoningOutputTokens}
	for i, source := range sources {
		if source == nil {
			continue
		}
		if *targets[i] == nil {
			zero := int64(0)
			*targets[i] = &zero
		}
		if **targets[i] > tc.MaxSafeUsage-*source {
			return nil, fmt.Errorf("usage overflow")
		}
		**targets[i] += *source
	}
	return a, nil
}
