package toolcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

const ClaudeProtocol = "claude-jsonl-v1"
const ClaudeCLIVersion = "2.1.314"

// ClaudeExecutionCapability is an administrator-fixed foreground CLI
// configuration, not a gateway deployment, account health or billing identity.
type ClaudeExecutionCapability struct {
	Model               string `json:"model"`
	ModelSource         string `json:"model_source"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
	AuthStrategy        string `json:"auth_strategy"`
}

type ClaudeExecutionEvidence struct {
	ConfigurationSHA256   string `json:"configuration_sha256"`
	SessionID             string `json:"session_id"`
	RequestedModel        string `json:"requested_model"`
	AcknowledgedModel     string `json:"acknowledged_model"`
	ResultSubtype         string `json:"result_subtype"`
	ResultTerminalReason  string `json:"result_terminal_reason,omitempty"`
	ResultIsError         bool   `json:"result_is_error"`
	PermissionDenialCount int    `json:"permission_denial_count"`
	DeferredToolUse       bool   `json:"deferred_tool_use,omitempty"`
	QueuedTurnCount       int    `json:"queued_turn_count,omitempty"`
}

var claudeModel = regexp.MustCompile(`^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$`)

// The reviewed release is explicit. A later version must be reviewed and
// probed before changing this contract; a semantic version alone proves no flags.
func ValidateClaudeCLIVersion(v string) error {
	if v != ClaudeCLIVersion && v != ClaudeCLIVersion+" (Claude Code)" {
		return fmt.Errorf("unsupported Claude Code CLI version")
	}
	return nil
}
func ValidateClaudeExecutionCapability(c ClaudeExecutionCapability) error {
	if len(c.Model) > 256 || !claudeModel.MatchString(c.Model) || c.ModelSource != "administrator_cli_argument" || !ValidDigest(c.ConfigurationSHA256) || c.BudgetMode != "observe_only" || c.IdentityEvidence != "configuration_ack_only" || c.AuthStrategy != "existing_cli_login" {
		return fmt.Errorf("invalid fixed Claude CLI configuration")
	}
	return nil
}
func claudeProfileBase(p Profile) bool {
	return p.Adapter == "claude" && p.Protocol == ClaudeProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == "" && p.TerminalSessionProtocol == "" && p.ConflictRepairProtocol == "" && p.TestReportProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.DispatchRecommendation == nil && p.TaskConversationReply == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil
}
func SupportsClaudeExecutionProfile(p Profile) bool {
	return claudeProfileBase(p) && p.ClaudeExecution != nil && ValidateClaudeExecutionCapability(*p.ClaudeExecution) == nil && ValidateClaudeCLIVersion(p.ToolVersion) == nil
}
func validateClaudeProfile(p Profile, complete bool) error {
	if p.Adapter != "claude" && p.Protocol != ClaudeProtocol && p.ClaudeExecution == nil {
		return nil
	}
	if SupportsClaudeExecutionProfile(p) {
		return nil
	}
	if !complete && claudeProfileBase(p) && p.ClaudeExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid Claude foreground CLI capability")
}

// Node data cannot replace the reviewed CLI model, permissions or environment.
// Presence is significant, including null values. Configuration identity is
// checked again against the currently authorized tool profile before dispatch.
func DecodeClaudeNodeConfiguration(config map[string]any) (*ClaudeExecutionCapability, error) {
	value, present := config["claude_execution"]
	if !present {
		return nil, nil
	}
	if cc.HasNodeOverrides(config) {
		return nil, fmt.Errorf("invalid Claude node override")
	}
	for _, key := range []string{"octop_execution", "opencode_execution", "codex_execution", "max_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "continue", "session_id", "tools", "mcp_servers", "mcp_config", "permissions", "permission_mode", "permission_prompts", "settings", "settings_sources", "allowed_tools", "disallowed_tools", "agent", "agents", "home", "executable", "args", "api_key_env_ref", "parameters", "model_parameters"} {
		if _, ok := config[key]; ok {
			return nil, fmt.Errorf("invalid Claude node override")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 4096 || !ValidJSONUnicode(raw) {
		return nil, fmt.Errorf("invalid Claude node configuration")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err = scanValue(d, 0); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, fmt.Errorf("invalid Claude node configuration")
	}
	var fixed ClaudeExecutionCapability
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&fixed); err != nil || ValidateClaudeExecutionCapability(fixed) != nil {
		return nil, fmt.Errorf("invalid Claude node configuration")
	}
	return &fixed, nil
}

func validateClaudeEvidence(e Evidence) error {
	if e.ClaudeExecution == nil {
		if e.Profile.Adapter == "claude" || e.Profile.Protocol == ClaudeProtocol {
			return fmt.Errorf("Claude execution evidence is required")
		}
		return nil
	}
	c, proof := e.Profile.ClaudeExecution, e.ClaudeExecution
	if !SupportsClaudeExecutionProfile(e.Profile) || c == nil || proof.ConfigurationSHA256 != c.ConfigurationSHA256 || proof.RequestedModel != c.Model || proof.SessionID != e.SessionID || e.OpenCodeExecution != nil || e.OctopExecution != nil || e.CodexExecution != nil || e.ModelExecution != nil || e.UsageState == UsageComplete {
		return fmt.Errorf("invalid Claude execution configuration evidence")
	}
	if proof.SessionID != "" && !safeID.MatchString(proof.SessionID) || proof.AcknowledgedModel != "" && !claudeModel.MatchString(proof.AcknowledgedModel) || !validText(proof.ResultSubtype, 80, false) || !validText(proof.ResultTerminalReason, 80, false) || proof.PermissionDenialCount < 0 || proof.PermissionDenialCount > 1000000 || proof.QueuedTurnCount < 0 || proof.QueuedTurnCount > 1000000 {
		return fmt.Errorf("invalid Claude result correlation")
	}
	if e.Terminal == "completed" && (proof.SessionID == "" || proof.AcknowledgedModel != c.Model || proof.ResultSubtype != "success" || proof.ResultIsError || proof.PermissionDenialCount != 0 || proof.DeferredToolUse || proof.QueuedTurnCount != 0 || proof.ResultTerminalReason != "" && proof.ResultTerminalReason != "completed") {
		return fmt.Errorf("Claude requires an exact completed foreground result")
	}
	return nil
}
