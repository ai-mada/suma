package toolcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

func acpProfileBase(p Profile) bool {
	input := p.Adapter == "qoder" && p.UserInputProtocol == "" || p.Adapter == "cursor" && p.UserInputProtocol == cu.InputProtocol
	return input && p.Protocol == ac.Protocol && !p.ReadOnly && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == "" && p.TerminalSessionProtocol == "" && p.TestReportProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.DispatchRecommendation == nil && p.TaskConversationReply == nil && p.ClaudeExecution == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil
}

func SupportsACPExecutionProfile(p Profile) bool {
	return acpProfileBase(p) && p.ACPExecution != nil && p.ACPExecution.Runtime == p.Adapter && ac.ValidateCapability(*p.ACPExecution) == nil && ac.ValidFixedText(p.ToolVersion, 256)
}

func validateACPProfile(p Profile, complete bool) error {
	if p.Adapter != "qoder" && p.Adapter != "cursor" && p.Protocol != ac.Protocol && p.ACPExecution == nil {
		return nil
	}
	if SupportsACPExecutionProfile(p) || !complete && acpProfileBase(p) && p.ACPExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid ACP execution capability")
}

// Tasks select the fixed current profile. No node-supplied executable, model,
// permission policy or environment can replace administrator configuration.
func DecodeACPNodeConfiguration(config map[string]any) (*ac.Capability, error) {
	value, present := config["acp_execution"]
	if !present {
		return nil, nil
	}
	if cc.HasNodeOverrides(config) {
		return nil, fmt.Errorf("invalid ACP node override")
	}
	for _, key := range []string{"model_execution", "octop_execution", "opencode_execution", "codex_execution", "claude_execution", "model", "max_steps", "temperature", "top_p", "reasoning_effort", "max_tokens", "max_output_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "continue", "session_id", "tools", "mcp_servers", "mcp_config", "permissions", "permission_mode", "permission_prompts", "settings", "settings_sources", "allowed_tools", "disallowed_tools", "agent", "agents", "home", "executable", "args", "env", "env_from", "api_key_env_ref", "parameters", "model_parameters", "acp"} {
		if _, ok := config[key]; ok {
			return nil, fmt.Errorf("invalid ACP node override")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 4096 || !ValidJSONUnicode(raw) {
		return nil, fmt.Errorf("invalid ACP node configuration")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err = scanValue(d, 0); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("invalid ACP node configuration")
	}
	var fixed ac.Capability
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&fixed); err != nil || ac.ValidateCapability(fixed) != nil {
		return nil, fmt.Errorf("invalid ACP node configuration")
	}
	return &fixed, nil
}

func validateACPEvidence(e Evidence) error {
	if e.ACPExecution == nil {
		if e.Profile.Adapter == "qoder" || e.Profile.Adapter == "cursor" || e.Profile.Protocol == ac.Protocol {
			return fmt.Errorf("ACP execution evidence is required")
		}
		return nil
	}
	p, c := e.ACPExecution, e.Profile.ACPExecution
	if !SupportsACPExecutionProfile(e.Profile) || c == nil || p.ConfigurationSHA256 != c.ConfigurationSHA256 || p.RequestedModel != c.Model || p.SessionID != e.SessionID || e.ModelExecution != nil || e.CodexExecution != nil || e.ClaudeExecution != nil || e.OpenCodeExecution != nil || e.OctopExecution != nil || e.UsageState == UsageComplete {
		return fmt.Errorf("invalid ACP execution source evidence")
	}
	if p.PermissionsRequested < 0 || p.PermissionsRequested > 32 || p.PermissionsDenied < 0 || p.PermissionsDenied > p.PermissionsRequested || p.ProtocolVersion < 0 || p.ProtocolVersion > 1 || !validText(p.AgentName, 256, false) || !validText(p.AgentVersion, 256, false) || p.SelectedModel != "" && !ac.ValidFixedText(p.SelectedModel, 256) {
		return fmt.Errorf("invalid ACP observed execution facts")
	}
	if p.ModelSelectionAcknowledged && p.SelectedModel != c.Model || p.SelectedModel != "" && !p.ModelSelectionAcknowledged || p.PromptStarted && (!p.ModelSelectionAcknowledged || p.SessionID == "" || p.ProtocolVersion != 1) || p.PromptCompleted && !p.PromptStarted || p.HostProcessTreeStopped && !p.HostProcessStopped {
		return fmt.Errorf("inconsistent ACP execution facts")
	}
	if !ac.ValidHostExitCode(p.HostProcessExitCode) || p.HostProcessExitCode != nil && !p.HostProcessStopped || p.HostStoppedAfterRPC && (e.Profile.Adapter != "cursor" || p.HostProcessExitCode == nil || !p.PromptCompleted || p.StopReason != "end_turn" || !p.ToolsComplete || p.PermissionsDenied != 0 || p.CancelRequested || !p.HostProcessStopped || !p.HostProcessTreeStopped) {
		return fmt.Errorf("invalid ACP RPC and host termination relationship")
	}
	if len(p.UserInputs) > 0 && e.Profile.Adapter != "cursor" || len(p.UserInputs)+p.PermissionsRequested > 32 {
		return fmt.Errorf("ACP input capability or interaction limit differs")
	}
	seen, ids := map[string]bool{}, map[string]bool{}
	for _, input := range p.UserInputs {
		key := input.RequestID.Kind + ":" + input.RequestID.Value
		if cu.ValidateEvidence(input) != nil || input.ConfigurationSHA256 != c.ConfigurationSHA256 || input.SessionID != p.SessionID || !p.PromptStarted || seen[key] || ids[input.InputID] {
			return fmt.Errorf("ACP input evidence differs from its original execution")
		}
		seen[key], ids[input.InputID] = true, true
		if (e.Terminal == "completed" || p.HostStoppedAfterRPC) && (input.Observation == nil || !input.Observation.AnswerObserved || !input.Observation.ReplyWritten || !input.Observation.ExecutionClosed || input.Observation.FailureCode != "") {
			return fmt.Errorf("ACP completion requires closed original input delivery")
		}
	}
	if p.StopReason != "" {
		switch p.StopReason {
		case "end_turn", "cancelled", "max_tokens", "refusal", "max_turn_requests":
		default:
			return fmt.Errorf("unknown ACP stop reason")
		}
	}
	if e.Terminal == "completed" && (!p.PromptCompleted || p.StopReason != "end_turn" || !p.ToolsComplete || p.PermissionsDenied != 0 || p.CancelRequested || !p.HostProcessStopped || p.HostProcessExitCode != nil && *p.HostProcessExitCode != 0 && !p.HostStoppedAfterRPC) {
		return fmt.Errorf("ACP completion requires exact successful RPC and host termination facts")
	}
	return nil
}
