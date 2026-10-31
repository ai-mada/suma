package toolcontract

import (
	"encoding/json"
	ac "github.com/ai-mada/suma/internal/acpcontract"
	"strings"
	"testing"
)

func acpTestProfile(t *testing.T) Profile {
	t.Helper()
	var p Profile
	raw := `{"command_id":"qoder-local","adapter":"qoder","protocol":"acp-stdio-v1","profile_sha256":"` + strings.Repeat("a", 64) + `","executable_sha256":"` + strings.Repeat("b", 64) + `","tool_version":"1.0.0-reviewed","read_only":false,"acp_execution":{"runtime":"qoder","protocol_version":1,"sdk_version":"0.13.5","configuration_sha256":"` + strings.Repeat("c", 64) + `","model":"performance","model_source":"administrator_session_config","budget_mode":"observe_only","identity_evidence":"configuration_ack_only","auth_strategy":"existing_cli_login","permission_mode":"human_once"}}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestACPNodeCannotReplaceFrozenPolicy(t *testing.T) {
	p := acpTestProfile(t)
	configuration := map[string]any{"acp_execution": p.ACPExecution, "goal": "Implement current task"}
	fixed, err := DecodeACPNodeConfiguration(configuration)
	if err != nil || fixed == nil || *fixed != *p.ACPExecution {
		t.Fatal("fixed current node rejected", err)
	}
	for _, key := range []string{"model", "model_execution", "args", "env", "permissions", "mcp_servers", "resume", "budget", "acp"} {
		t.Run(key, func(t *testing.T) {
			c := map[string]any{"acp_execution": p.ACPExecution, key: nil}
			if _, err := DecodeACPNodeConfiguration(c); err == nil {
				t.Fatal("presence, including null, cannot replace fixed execution")
			}
		})
	}
	if _, err := DecodeACPNodeConfiguration(map[string]any{"acp_execution": nil}); err == nil {
		t.Fatal("null capability accepted")
	}
}

func TestACPCompletionRequiresOriginalConfigurationAndStop(t *testing.T) {
	p := acpTestProfile(t)
	proof := ac.Evidence{ConfigurationSHA256: p.ACPExecution.ConfigurationSHA256, RequestedModel: p.ACPExecution.Model, SelectedModel: p.ACPExecution.Model, ModelSelectionAcknowledged: true, SessionID: "original-session", ProtocolVersion: 1, PromptStarted: true, PromptCompleted: true, StopReason: "end_turn", ToolsComplete: true, HostProcessStopped: true}
	e := Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", EventCount: 3, UsageState: UsageUnreported, ACPExecution: &proof}
	if err := ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"no_model_ack", "wrong_model", "wrong_protocol", "cancelled", "unfinished_tools", "permission_denied", "host_live", "wrong_source", "complete_usage"} {
		t.Run(mode, func(t *testing.T) {
			q := proof
			changed := e
			changed.ACPExecution = &q
			switch mode {
			case "no_model_ack":
				q.ModelSelectionAcknowledged = false
			case "wrong_model":
				q.SelectedModel = "another-model"
			case "wrong_protocol":
				q.ProtocolVersion = 0
			case "cancelled":
				q.StopReason = "cancelled"
			case "unfinished_tools":
				q.ToolsComplete = false
			case "permission_denied":
				q.PermissionsRequested = 1
				q.PermissionsDenied = 1
			case "host_live":
				q.HostProcessStopped = false
			case "wrong_source":
				q.ConfigurationSHA256 = strings.Repeat("d", 64)
			case "complete_usage":
				changed.UsageState = UsageComplete
			}
			if ValidateEvidence(changed) == nil {
				t.Fatal("unproven completion accepted")
			}
		})
	}
	unknown := e
	incomplete := ac.Evidence{ConfigurationSHA256: proof.ConfigurationSHA256, RequestedModel: proof.RequestedModel}
	unknown.ACPExecution = &incomplete
	unknown.SessionID = ""
	unknown.Terminal = "unknown"
	if err := ValidateEvidence(unknown); err != nil {
		t.Fatal("early unknown must remain representable", err)
	}
}

func TestACPFixedProfileAndIsolation(t *testing.T) {
	p := acpTestProfile(t)
	if err := ValidateProfile(p, true); err != nil {
		t.Fatalf("fixed ACP profile rejected: %v", err)
	}
	if err := ValidateEvent(Event{SchemaVersion: 1, CommandID: p.CommandID, ProfileSHA256: p.ProfileSHA256, Protocol: p.Protocol, Kind: "session_started", SessionID: "original-session"}); err != nil {
		t.Fatal("valid registered profile must also accept its durable first event", err)
	}
	copy := CloneProfile(p)
	if !EqualProfile(p, copy) || p.ACPExecution == copy.ACPExecution {
		t.Fatal("capability clone must preserve values without sharing mutation")
	}
	copy.ACPExecution.Model = "auto"
	if EqualProfile(p, copy) || p.ACPExecution.Model != "performance" {
		t.Fatal("changed fixed model must not retain original profile identity")
	}
	for _, mode := range []string{"readonly", "gateway", "workspace", "other_adapter", "always", "other_capability"} {
		t.Run(mode, func(t *testing.T) {
			q := CloneProfile(p)
			switch mode {
			case "readonly":
				q.ReadOnly = true
			case "gateway":
				q.ModelExecutionProtocol = "suma-model-execution-v1"
			case "workspace":
				q.WorkspaceToolsVersion = "attempt-workspace-v1"
			case "other_adapter":
				q.Adapter = "suma"
			case "always":
				q.ACPExecution.PermissionMode = "allow_always"
			case "other_capability":
				q.ClaudeExecution = &ClaudeExecutionCapability{}
			}
			if ValidateProfile(q, true) == nil {
				t.Fatal("mixed or broader capability accepted")
			}
		})
	}
}
