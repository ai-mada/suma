package runner

import (
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// ACP end_turn, cancellation intent and process-tree quiescence are different
// facts. The Python carrier can report its child parent closed; only the Go
// controller can attest the whole tree. No ACP cancellation ACK is invented.
type octopStreamState struct {
	ready         bool
	parentStopped bool
	failed        bool
	proof         tc.OctopExecutionEvidence
}

func applyOctopProcessContainment(evidence *tc.Evidence, parentStopped, verified bool) {
	if evidence.OctopExecution != nil {
		evidence.OctopExecution.ProcessStopped = parentStopped && verified
		if !evidence.OctopExecution.ProcessStopped {
			evidence.Terminal = "unknown"
		}
	}
}

func (s *toolStream) octop(m map[string]any) {
	c := s.profile.OctopExecution
	if c == nil || !tc.SupportsOctopExecutionProfile(s.profile) {
		s.invalid("Octop Harness fixed configuration is missing")
		return
	}
	if s.octopState == nil {
		s.octopState = &octopStreamState{proof: tc.OctopExecutionEvidence{ConfigurationSHA256: c.ConfigurationSHA256, RequestedProvider: c.ProviderID, RequestedModel: c.Model}}
	}
	d := s.octopState
	v, valid := nativeInteger(m, "v")
	if !valid || v != 1 || s.expectedRequestID == "" || textAt(m, "request_id") != s.expectedRequestID {
		s.invalid("Octop frame version or original request identity is invalid")
		return
	}
	switch textAt(m, "type") {
	case "ready":
		if !closedOpenCodeFrame(m, "harness_version", "acp_sdk_version", "configuration_sha256", "provider_id", "model") || d.ready || s.sessionStarted || textAt(m, "harness_version") != c.HarnessVersion || textAt(m, "acp_sdk_version") != c.ACPSDKVersion || textAt(m, "configuration_sha256") != c.ConfigurationSHA256 || textAt(m, "provider_id") != c.ProviderID || textAt(m, "model") != c.Model {
			s.invalid("Octop readiness differs from the pinned configuration")
			return
		}
		d.ready = true
		d.proof.AcknowledgedProvider, d.proof.AcknowledgedModel = c.ProviderID, c.Model
	case "started":
		session := textAt(m, "session_id")
		if !closedOpenCodeFrame(m, "session_id") || !d.ready || s.sessionStarted || !s.openCodeID(session) {
			s.invalid("Octop ACP session identity is invalid")
			return
		}
		d.proof.SessionID, s.session = session, session
		s.sessionStarted, s.turnStarted = true, true
		s.event("session_started", "", "", "", nil)
		s.event("turn_started", "", "", "", nil)
	case "progress":
		if !closedOpenCodeFrame(m, "session_id", "kind", "status", "tool") || !s.sessionStarted || textAt(m, "session_id") != d.proof.SessionID {
			s.invalid("Octop progress has no matching ACP session")
			return
		}
		kind, status := textAt(m, "kind"), textAt(m, "status")
		if status != "started" && status != "completed" && status != "failed" {
			s.invalid("Octop progress status is invalid")
			return
		}
		item := "model"
		if kind == "tool" {
			tool := textAt(m, "tool")
			if !map[string]bool{"list": true, "read": true, "write": true, "edit": true, "remove": true, "check": true}[tool] {
				s.invalid("Octop reported an unapproved tool")
				return
			}
			item = "workspace." + tool
		} else if kind != "model" || m["tool"] != nil {
			s.invalid("Octop activity kind is invalid")
			return
		}
		event := "activity_started"
		if status == "completed" {
			event = "activity_completed"
		} else if status == "failed" {
			// Known host validation errors may be corrected within the turn.
			// Fatal or unknown effects require the runtime's failed terminal.
			event = "error"
		}
		s.event(event, item, item, status, nil)
	case "completed":
		if !closedOpenCodeFrame(m, "session_id", "provider_id", "model", "stop_reason", "completed_at", "agent_parent_stopped", "usage", "usage_state") || !d.ready || !s.sessionStarted || d.failed || textAt(m, "session_id") != d.proof.SessionID || textAt(m, "provider_id") != c.ProviderID || textAt(m, "model") != c.Model || textAt(m, "stop_reason") != "end_turn" || m["agent_parent_stopped"] != true {
			s.invalid("Octop completion lacks matching ACP and parent-close facts")
			return
		}
		completed, err := time.Parse(time.RFC3339Nano, textAt(m, "completed_at"))
		if err != nil || completed.Location() != time.UTC || completed.Format(time.RFC3339Nano) != textAt(m, "completed_at") {
			s.invalid("Octop completion time is invalid")
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
		d.proof.StopReason, d.proof.CompletedAt = "end_turn", completed.Format(time.RFC3339Nano)
		d.parentStopped = true
		s.setTerminal("completed")
		s.event("turn_completed", "", "", "", nil)
	case "failed":
		if !closedOpenCodeFrame(m, "code", "message", "agent_parent_stopped") || !s.openCodeID(textAt(m, "code")) || m["agent_parent_stopped"] != true {
			s.invalid("Octop failure has no confirmed parent-close fact")
			return
		}
		if _, present := m["message"]; present {
			if _, ok := m["message"].(string); !ok {
				s.invalid("Octop failure message is invalid")
				return
			}
		}
		d.parentStopped, d.failed = true, true
		s.setTerminal("failed")
		s.event("error", "", textAt(m, "message"), textAt(m, "code"), nil)
	case "cancelled":
		session := textAt(m, "session_id")
		if !closedOpenCodeFrame(m, "session_id", "cancel_requested", "agent_parent_stopped") || m["cancel_requested"] != true || m["agent_parent_stopped"] != true || s.sessionStarted && session != d.proof.SessionID || !s.sessionStarted && session != "" {
			s.invalid("Octop cancellation correlation or parent-close fact is invalid")
			return
		}
		d.parentStopped, d.proof.CancelRequested = true, true
		s.setTerminal("failed")
		s.event("turn_failed", "", "Octop execution cancellation requested", "cancelled", nil)
	default:
		s.invalid("unsupported Octop Harness frame")
	}
}
