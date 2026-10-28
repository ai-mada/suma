package runner

import (
	"fmt"
	"strings"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type openCodeStreamState struct {
	ready bool
	proof tc.OpenCodeExecutionEvidence
}

// The wrapper observes its private server; only the host process controller
// can confirm the surrounding execution tree. Never publish the former as a
// successful stop when the latter requires reconciliation.
func applyOpenCodeProcessContainment(evidence *tc.Evidence, verified bool) {
	if evidence.OpenCodeExecution != nil && !verified {
		evidence.OpenCodeExecution.ServerStopped = false
		evidence.Terminal = "unknown"
	}
}

func closedOpenCodeFrame(m map[string]any, fields ...string) bool {
	allowed := map[string]bool{"v": true, "type": true, "request_id": true}
	for _, field := range fields {
		allowed[field] = true
	}
	for field, value := range m {
		if !allowed[field] || value == nil {
			return false
		}
	}
	return true
}

func (s *toolStream) openCodeID(value string) bool {
	return value != "" && len(value) <= 128 && s.id(value) == value && tc.ValidateEvent(tc.Event{SchemaVersion: 1, CommandID: s.profile.CommandID, ProfileSHA256: s.profile.ProfileSHA256, Protocol: s.profile.Protocol, Kind: "message", ItemID: value}) == nil
}

func (s *toolStream) openCode(m map[string]any) {
	c := s.profile.OpenCodeExecution
	if c == nil || !tc.SupportsOpenCodeExecutionProfile(s.profile) {
		s.invalid("OpenCode SDK fixed configuration is missing")
		return
	}
	if s.openCodeState == nil {
		s.openCodeState = &openCodeStreamState{proof: tc.OpenCodeExecutionEvidence{ConfigurationSHA256: c.ConfigurationSHA256, RequestedProvider: c.ProviderID, RequestedModel: c.Model}}
	}
	d := s.openCodeState
	v, valid := nativeInteger(m, "v")
	if !valid || v != 1 || s.expectedRequestID == "" || textAt(m, "request_id") != s.expectedRequestID {
		s.invalid("OpenCode SDK frame version or request identity is invalid")
		return
	}
	kind := textAt(m, "type")
	switch kind {
	case "ready":
		if !closedOpenCodeFrame(m, "sdk_version", "server_version", "configuration_sha256", "provider_id", "model") || d.ready || s.sessionStarted || textAt(m, "sdk_version") != c.SDKVersion || textAt(m, "server_version") != c.ServerVersion || textAt(m, "configuration_sha256") != c.ConfigurationSHA256 || textAt(m, "provider_id") != c.ProviderID || textAt(m, "model") != c.Model {
			s.invalid("OpenCode SDK readiness differs from the pinned configuration")
			return
		}
		d.ready = true
		d.proof.AcknowledgedProvider, d.proof.AcknowledgedModel = c.ProviderID, c.Model
	case "started":
		session, user := textAt(m, "session_id"), textAt(m, "user_message_id")
		if !closedOpenCodeFrame(m, "session_id", "user_message_id") || !d.ready || s.sessionStarted || !s.openCodeID(session) || !s.openCodeID(user) {
			s.invalid("OpenCode SDK session or prompt identity is invalid")
			return
		}
		d.proof.SessionID, d.proof.UserMessageID = session, user
		s.session = session
		s.sessionStarted, s.turnStarted = true, true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
	case "progress":
		if !closedOpenCodeFrame(m, "session_id", "kind", "status", "tool") || !s.sessionStarted || textAt(m, "session_id") != d.proof.SessionID {
			s.invalid("OpenCode SDK progress has no matching session")
			return
		}
		activity, status := textAt(m, "kind"), textAt(m, "status")
		if status != "started" && status != "completed" {
			s.invalid("OpenCode SDK progress status is invalid")
			return
		}
		item := "model"
		if activity == "tool" {
			tool := textAt(m, "tool")
			if !map[string]bool{"list": true, "read": true, "write": true, "edit": true, "remove": true, "check": true}[tool] {
				s.invalid("OpenCode SDK reported an unapproved tool")
				return
			}
			item = "workspace." + tool
		} else if activity != "model" || m["tool"] != nil {
			s.invalid("OpenCode SDK activity kind is invalid")
			return
		}
		event := "activity_started"
		if status == "completed" {
			event = "activity_completed"
		}
		s.event(event, item, item, status, nil)
	case "completed":
		if !closedOpenCodeFrame(m, "session_id", "user_message_id", "assistant_message_id", "provider_id", "model", "finish", "completed_at", "server_stopped", "usage", "usage_state") || !d.ready || !s.sessionStarted || textAt(m, "session_id") != d.proof.SessionID || textAt(m, "user_message_id") != d.proof.UserMessageID || !s.openCodeID(textAt(m, "assistant_message_id")) || textAt(m, "assistant_message_id") == d.proof.UserMessageID || textAt(m, "provider_id") != c.ProviderID || textAt(m, "model") != c.Model || textAt(m, "finish") != "stop" || m["server_stopped"] != true {
			s.invalid("OpenCode SDK completion lacks matching final assistant and server stop")
			return
		}
		completed, err := time.Parse(time.RFC3339Nano, textAt(m, "completed_at"))
		if err != nil || !strings.HasSuffix(textAt(m, "completed_at"), "Z") {
			s.invalid("OpenCode SDK completion time is invalid")
			return
		}
		usage, state, err := openCodeUsage(m)
		if err != nil {
			s.invalid(err.Error())
			return
		}
		s.usage, s.usagePartial = usage, usage != nil
		if usage != nil {
			s.event("usage", "", "", "", usage, state)
		}
		d.proof.AssistantMessageID, d.proof.Finish = textAt(m, "assistant_message_id"), "stop"
		d.proof.CompletedAt, d.proof.ServerStopped = completed.UTC().Format(time.RFC3339Nano), true
		s.setTerminal("completed")
		s.event("turn_completed", "", "", "", nil)
	case "failed":
		if !closedOpenCodeFrame(m, "code", "message", "server_stopped") || !s.openCodeID(textAt(m, "code")) || m["server_stopped"] != true || textAt(m, "code") == "cancel_unconfirmed" {
			s.invalid("OpenCode SDK failure does not prove private server shutdown")
			return
		}
		if _, present := m["message"]; present {
			if _, ok := m["message"].(string); !ok {
				s.invalid("OpenCode SDK failure message is invalid")
				return
			}
		}
		d.proof.ServerStopped = true
		s.setTerminal("failed")
		s.event("error", "", textAt(m, "message"), textAt(m, "code"), nil)
	case "cancelled":
		acknowledged, ok := m["abort_acknowledged"].(bool)
		session := textAt(m, "session_id")
		if !closedOpenCodeFrame(m, "session_id", "abort_acknowledged", "server_stopped") || !ok || m["server_stopped"] != true || s.sessionStarted && (session != d.proof.SessionID || !acknowledged) || !s.sessionStarted && (session != "" || acknowledged) {
			s.invalid("OpenCode SDK cancellation identity or acknowledgement is unverified")
			return
		}
		d.proof.ServerStopped, d.proof.AbortAcknowledged = true, acknowledged
		s.setTerminal("failed")
		s.event("turn_failed", "", "OpenCode SDK execution cancelled", "cancelled", nil)
	default:
		s.invalid("unsupported OpenCode SDK frame")
	}
}

func openCodeUsage(m map[string]any) (*tc.Usage, string, error) {
	state := textAt(m, "usage_state")
	raw, present := m["usage"]
	if state == tc.UsageUnreported && !present {
		return nil, state, nil
	}
	values, ok := raw.(map[string]any)
	if state != tc.UsagePartial || !ok || !present || len(values) == 0 {
		return nil, "", fmt.Errorf("OpenCode SDK usage must preserve partial or unreported coverage")
	}
	for name := range values {
		if !map[string]bool{"input_tokens": true, "output_tokens": true, "cached_input_tokens": true, "cache_write_input_tokens": true, "reasoning_output_tokens": true}[name] {
			return nil, "", fmt.Errorf("OpenCode SDK usage has an unknown counter")
		}
	}
	usage, err := parseUsage(values)
	if err != nil || usage == nil {
		return nil, "", fmt.Errorf("OpenCode SDK usage counter is invalid")
	}
	return usage, state, nil
}
