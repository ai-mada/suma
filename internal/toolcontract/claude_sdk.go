package toolcontract

import (
	"encoding/json"
	"fmt"
	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/strictjson"
	"strings"
)

const ClaudeSDKProtocol = cl.Protocol
const ClaudeSDKVersion = cl.SDKVersion
const ClaudeSDKCLIVersion = cl.CLIVersion

// The fixed SDK model is a configuration fact, never a gateway or billing grant.
type ClaudeSDKExecutionCapability struct {
	SDKVersion          string `json:"sdk_version"`
	CLIVersion          string `json:"cli_version"`
	Model               string `json:"model"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	ModelSource         string `json:"model_source"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
	AuthStrategy        string `json:"auth_strategy"`
}
type ClaudeSDKExecutionEvidence struct {
	SDKVersion             string `json:"sdk_version"`
	CLIVersion             string `json:"cli_version"`
	ConfigurationSHA256    string `json:"configuration_sha256"`
	SessionID              string `json:"session_id"`
	RequestedModel         string `json:"requested_model"`
	AcknowledgedModel      string `json:"acknowledged_model"`
	ResultSubtype          string `json:"result_subtype"`
	ResultIsError          bool   `json:"result_is_error"`
	TurnEnded              bool   `json:"turn_ended"`
	ToolsCompleted         bool   `json:"tools_completed"`
	CancelRequested        bool   `json:"cancel_requested"`
	HostProcessStopped     bool   `json:"host_process_stopped"`
	HostProcessTreeStopped bool   `json:"host_process_tree_stopped"`
	PermissionRequests     int    `json:"permission_requests"`
	// Callback denials and the native result summary are independent facts.
	DeniedCallbacks         int                     `json:"denied_callbacks"`
	NativePermissionDenials *int                    `json:"native_permission_denials,omitempty" wire:"nonnull"`
	Permissions             []cl.PermissionEvidence `json:"permissions,omitempty"`
	UserInputs              []cl.InputEvidence      `json:"user_inputs,omitempty"`
}

// ValidClaudeSDKModel matches the pinned SDK host's fixed model grammar. Local
// preparation and published capabilities must reject the same identifiers.
func ValidClaudeSDKModel(model string) bool {
	return len(model) <= 256 && claudeModel.MatchString(model)
}

// ValidClaudeSDKCredentialEnvRef permits only a separate credential variable
// name. OS process mechanics, locale and dynamic-loader names are unavailable
// because the isolated probe/runtime owns their values independently.
func ValidClaudeSDKCredentialEnvRef(ref string) bool {
	if len(ref) < 1 || len(ref) > 128 || ref[0] < 'A' || ref[0] > 'Z' {
		return false
	}
	for _, character := range ref {
		if !(character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_') {
			return false
		}
	}
	for _, prefix := range []string{"SUMA_", "NODE_", "NPM_", "CLAUDE_", "LC_", "LD_", "DYLD_"} {
		if strings.HasPrefix(ref, prefix) {
			return false
		}
	}
	switch ref {
	case "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PROGRAMDATA", "TEMP", "TMP", "TMPDIR", "ANTHROPIC_AUTH_TOKEN",
		"LANG", "LC_ALL", "PATHEXT", "SYSTEMDRIVE", "ALLUSERSPROFILE", "PROGRAMFILES", "PROGRAMW6432", "COMMONPROGRAMFILES", "COMMONPROGRAMW6432", "HOMEDRIVE", "HOMEPATH", "PUBLIC", "OS":
		return false
	}
	return true
}

func ValidateClaudeSDKExecutionCapability(c ClaudeSDKExecutionCapability) error {
	if c.SDKVersion != cl.SDKVersion || c.CLIVersion != cl.CLIVersion || !ValidClaudeSDKModel(c.Model) || !ValidDigest(c.ConfigurationSHA256) || c.ModelSource != "administrator_sdk_option" || c.BudgetMode != "observe_only" || c.IdentityEvidence != "sdk_init_model_ack_only" || c.AuthStrategy != "api_key_env_ref" {
		return fmt.Errorf("invalid fixed Claude SDK configuration")
	}
	return nil
}
func claudeSDKProfileBase(p Profile) bool {
	return p.Adapter == "claude" && p.Protocol == cl.Protocol && !p.ReadOnly && p.UserInputProtocol == cl.InputProtocol && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == "" && p.TerminalSessionProtocol == "" && p.TestReportProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.DispatchRecommendation == nil && p.TaskConversationReply == nil && p.ClaudeExecution == nil && p.ACPExecution == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil
}
func SupportsClaudeSDKExecutionProfile(p Profile) bool {
	return claudeSDKProfileBase(p) && p.ClaudeSDKExecution != nil && ValidateClaudeSDKExecutionCapability(*p.ClaudeSDKExecution) == nil && p.ToolVersion == cl.CLIVersion
}
func validateClaudeSDKProfile(p Profile, complete bool) error {
	if p.Protocol != cl.Protocol && p.ClaudeSDKExecution == nil {
		return nil
	}
	if SupportsClaudeSDKExecutionProfile(p) || !complete && claudeSDKProfileBase(p) && p.ClaudeSDKExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid Claude SDK profile")
}
func DecodeClaudeSDKNodeConfiguration(config map[string]any) (*ClaudeSDKExecutionCapability, error) {
	value, present := config["claude_sdk_execution"]
	if !present {
		return nil, nil
	}
	if cc.HasNodeOverrides(config) {
		return nil, fmt.Errorf("Claude SDK node override denied")
	}
	for _, key := range []string{"claude_execution", "codex_execution", "opencode_execution", "octop_execution", "acp_execution", "model_execution", "model", "max_steps", "temperature", "top_p", "reasoning_effort", "max_tokens", "max_output_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "continue", "session_id", "tools", "mcp_servers", "mcp_config", "permissions", "permission_mode", "permission_prompts", "settings", "settings_sources", "allowed_tools", "disallowed_tools", "agent", "agents", "home", "executable", "args", "env", "env_from", "api_key_env_ref", "credential_env_ref", "parameters", "model_parameters", "claude_sdk"} {
		if _, ok := config[key]; ok {
			return nil, fmt.Errorf("Claude SDK node override denied")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fixed ClaudeSDKExecutionCapability
	if strictjson.Decode(raw, &fixed, 4096) != nil || ValidateClaudeSDKExecutionCapability(fixed) != nil {
		return nil, fmt.Errorf("invalid Claude SDK node configuration")
	}
	return &fixed, nil
}
func validateClaudeSDKEvidence(e Evidence) error {
	proof := e.ClaudeSDKExecution
	if proof == nil {
		if e.Profile.Protocol == cl.Protocol {
			return fmt.Errorf("Claude SDK evidence required")
		}
		return nil
	}
	c := e.Profile.ClaudeSDKExecution
	if !SupportsClaudeSDKExecutionProfile(e.Profile) || c == nil || proof.SDKVersion != cl.SDKVersion || proof.CLIVersion != cl.CLIVersion || proof.ConfigurationSHA256 != c.ConfigurationSHA256 || proof.RequestedModel != c.Model || proof.SessionID != e.SessionID || e.ClaudeExecution != nil || e.CodexExecution != nil || e.OpenCodeExecution != nil || e.OctopExecution != nil || e.ACPExecution != nil || e.ModelExecution != nil || e.UsageState == UsageComplete {
		return fmt.Errorf("invalid Claude SDK evidence source")
	}
	if proof.SessionID != "" && !cl.ID(proof.SessionID) || proof.AcknowledgedModel != "" && !claudeModel.MatchString(proof.AcknowledgedModel) || !validText(proof.ResultSubtype, 80, false) || proof.PermissionRequests < 0 || proof.PermissionRequests > cl.MaxCallbacks || proof.DeniedCallbacks < 0 || proof.DeniedCallbacks > cl.MaxCallbacks || proof.NativePermissionDenials != nil && (*proof.NativePermissionDenials < 0 || *proof.NativePermissionDenials > 1000000) || len(proof.Permissions) > proof.PermissionRequests || len(proof.Permissions)+len(proof.UserInputs) > cl.MaxCallbacks || proof.HostProcessTreeStopped && !proof.HostProcessStopped {
		return fmt.Errorf("invalid Claude SDK execution facts")
	}
	complete := e.Terminal == "completed"
	requests := map[string]bool{}
	callbacks := map[string]bool{}
	ids := map[string]bool{}
	check := func(id string, b cl.Basis, o *cl.PermissionObservation) bool {
		if !cl.ID(id) || ids[id] || cl.ValidateBasis(b) != nil || b.SessionID != proof.SessionID || b.ConfigurationSHA256 != c.ConfigurationSHA256 || requests[b.RequestID] || callbacks[b.CallbackID] {
			return false
		}
		ids[id] = true
		requests[b.RequestID] = true
		callbacks[b.CallbackID] = true
		if o != nil && (cl.ValidatePermissionObservation(*o) != nil || o.ParamsSHA256 != b.ParamsSHA256) {
			return false
		}
		return !complete || o != nil && o.AnswerObserved && o.ReplyWritten && o.ExecutionClosed && o.FailureCode == ""
	}
	for _, p := range proof.Permissions {
		if !check(p.PermissionID, p.Basis, p.Observation) {
			return fmt.Errorf("invalid Claude SDK permission evidence")
		}
		switch p.ToolName {
		case "Read", "Edit", "Write", "Glob", "Grep", "Bash":
		default:
			return fmt.Errorf("invalid Claude SDK permission tool")
		}
	}
	for _, input := range proof.UserInputs {
		if input.QuestionCount < 1 || input.QuestionCount > 4 || !check(input.InputID, input.Basis, input.Observation) {
			return fmt.Errorf("invalid Claude SDK question evidence")
		}
	}
	if complete && (proof.SessionID == "" || proof.AcknowledgedModel != c.Model || proof.ResultSubtype != "success" || proof.ResultIsError || !proof.TurnEnded || !proof.ToolsCompleted || proof.CancelRequested || !proof.HostProcessStopped || !proof.HostProcessTreeStopped || proof.DeniedCallbacks != 0 || proof.NativePermissionDenials == nil || *proof.NativePermissionDenials != 0 || len(proof.Permissions) != proof.PermissionRequests) {
		return fmt.Errorf("Claude SDK requires its exact completed foreground result")
	}
	return nil
}
func CloneClaudeSDKExecutionEvidence(e ClaudeSDKExecutionEvidence) ClaudeSDKExecutionEvidence {
	if e.NativePermissionDenials != nil {
		v := *e.NativePermissionDenials
		e.NativePermissionDenials = &v
	}
	e.Permissions = append([]cl.PermissionEvidence(nil), e.Permissions...)
	e.UserInputs = append([]cl.InputEvidence(nil), e.UserInputs...)
	for i := range e.Permissions {
		if e.Permissions[i].Observation != nil {
			v := *e.Permissions[i].Observation
			e.Permissions[i].Observation = &v
		}
	}
	for i := range e.UserInputs {
		if e.UserInputs[i].Observation != nil {
			v := *e.UserInputs[i].Observation
			e.UserInputs[i].Observation = &v
		}
	}
	return e
}
