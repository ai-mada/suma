package toolcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

const OctopProtocol = "octop-harness-acp-v1"
const OctopHarnessVersion = "1.0.5"
const OctopACPSDKVersion = "0.12.1"

// OctopExecutionCapability fixes the administrator's local harness and model.
// It is configuration acknowledgement, never account health or a hard budget.
type OctopExecutionCapability struct {
	HarnessVersion      string `json:"harness_version"`
	ACPSDKVersion       string `json:"acp_sdk_version"`
	ProviderID          string `json:"provider_id"`
	Model               string `json:"model"`
	EndpointOrigin      string `json:"endpoint_origin"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
}

// OctopExecutionEvidence retains the actual ACP session and independent Go
// process-stop proof. ACP cancellation is a notification, not an abort ACK.
// Octop end_turn by itself cannot prove a successful turn or code delivery.
type OctopExecutionEvidence struct {
	ConfigurationSHA256  string `json:"configuration_sha256"`
	SessionID            string `json:"session_id"`
	RequestedProvider    string `json:"requested_provider"`
	RequestedModel       string `json:"requested_model"`
	AcknowledgedProvider string `json:"acknowledged_provider"`
	AcknowledgedModel    string `json:"acknowledged_model"`
	StopReason           string `json:"stop_reason"`
	CompletedAt          string `json:"completed_at"`
	ProcessStopped       bool   `json:"process_stopped"`
	CancelRequested      bool   `json:"cancel_requested"`
}

func ValidateOctopExecutionCapability(c OctopExecutionCapability) error {
	if c.HarnessVersion != OctopHarnessVersion || c.ACPSDKVersion != OctopACPSDKVersion || !ValidCommandID(c.ProviderID) || !openCodeModel.MatchString(c.Model) || !validFixedModel(c.Model, c.EndpointOrigin) || !ValidDigest(c.ConfigurationSHA256) || c.BudgetMode != "observe_only" || c.IdentityEvidence != "configuration_ack_only" {
		return fmt.Errorf("invalid fixed Octop ACP configuration")
	}
	return nil
}

// The protocol is enabled only with the implemented host workspace. A profile
// cannot acquire native model limits, repair, tests or another model purpose.
func SupportsOctopExecutionProfile(p Profile) bool {
	return p.Adapter == "octop" && p.Protocol == OctopProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "attempt-workspace-v1" && p.ModelExecutionProtocol == "" && p.ConflictRepairProtocol == "" && p.TestReportProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution != nil && ValidateOctopExecutionCapability(*p.OctopExecution) == nil
}

func validateOctopProfile(p Profile, complete bool) error {
	if p.Adapter != "octop" && p.Protocol != OctopProtocol && p.OctopExecution == nil {
		return nil
	}
	if SupportsOctopExecutionProfile(p) {
		return nil
	}
	if !complete && p.Adapter == "octop" && p.Protocol == OctopProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == "" && p.ConflictRepairProtocol == "" && p.TestReportProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid Octop ACP host-workspace capability")
}

// Node input can retain only the reviewed public configuration. Process,
// experts, provider credentials, roots and limits belong to the local profile.
func DecodeOctopNodeConfiguration(config map[string]any) (*OctopExecutionCapability, error) {
	value, present := config["octop_execution"]
	if !present {
		return nil, nil
	}
	if cc.HasNodeOverrides(config) {
		return nil, fmt.Errorf("invalid Octop node override")
	}
	for _, field := range []string{"codex_execution", "opencode_execution", "max_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "session_id", "tools", "mcp_servers", "agent", "agent_id", "expert", "provider_id", "parameters", "model_parameters", "home", "octop_home", "workspace", "python", "executable", "args", "api_key_env_ref"} {
		if _, exists := config[field]; exists {
			return nil, fmt.Errorf("invalid Octop node override")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fixed OctopExecutionCapability
	if err := json.Unmarshal(raw, &fixed); err != nil {
		return nil, err
	}
	return &fixed, nil
}

func validateOctopEvidence(e Evidence) error {
	if e.OctopExecution == nil {
		if e.Profile.Protocol == OctopProtocol {
			return fmt.Errorf("Octop execution evidence is required")
		}
		return nil
	}
	c, proof := e.Profile.OctopExecution, e.OctopExecution
	if !SupportsOctopExecutionProfile(e.Profile) || e.OpenCodeExecution != nil || e.CodexExecution != nil || e.ModelExecution != nil || e.UsageState == UsageComplete || c == nil || proof.ConfigurationSHA256 != c.ConfigurationSHA256 || proof.RequestedProvider != c.ProviderID || proof.RequestedModel != c.Model || e.SessionID != proof.SessionID {
		return fmt.Errorf("invalid Octop execution configuration evidence")
	}
	if proof.SessionID != "" && !safeID.MatchString(proof.SessionID) || proof.AcknowledgedProvider != "" && proof.AcknowledgedProvider != c.ProviderID || proof.AcknowledgedModel != "" && proof.AcknowledgedModel != c.Model {
		return fmt.Errorf("invalid Octop execution acknowledgement")
	}
	switch proof.StopReason {
	case "", "end_turn", "max_tokens", "max_turn_requests", "refusal", "cancelled":
	default:
		return fmt.Errorf("invalid Octop stop reason")
	}
	if proof.CompletedAt != "" {
		completed, err := time.Parse(time.RFC3339Nano, proof.CompletedAt)
		if err != nil || completed.Location() != time.UTC || completed.Format(time.RFC3339Nano) != proof.CompletedAt {
			return fmt.Errorf("invalid Octop completion time")
		}
	}
	if e.Terminal == "completed" && (proof.SessionID == "" || proof.AcknowledgedProvider != c.ProviderID || proof.AcknowledgedModel != c.Model || proof.StopReason != "end_turn" || proof.CompletedAt == "" || !proof.ProcessStopped || proof.CancelRequested) {
		return fmt.Errorf("Octop completion requires the original successful turn and stopped process scope")
	}
	return nil
}

// Only the supervisor can derive ProcessStopped. A cancel notification is not
// acknowledged by ACP, so this protocol never invents an abort-acknowledgement.
func ValidateOctopResultStop(e Evidence, stopped, outcomeUnknown bool) error {
	if e.Profile.Protocol != OctopProtocol && e.OctopExecution == nil {
		return nil
	}
	if err := ValidateEvidence(e); err != nil {
		return err
	}
	proof := e.OctopExecution
	if proof == nil || !outcomeUnknown && !proof.ProcessStopped || stopped && !proof.CancelRequested {
		return fmt.Errorf("Octop result requires process-stop and original cancellation facts")
	}
	return nil
}

// These objects are small closed public facts. Preserve original JSON while
// rejecting duplicate names, aliases, nulls, unknown and omitted fields.
func decodeOctopObject(raw []byte, target any, fields []string) error {
	if len(raw) > 8192 || !ValidJSONUnicode(raw) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("invalid Octop object encoding")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	if err := scanValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("trailing Octop object")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil || len(values) != len(fields) {
		return fmt.Errorf("invalid Octop object fields")
	}
	for _, name := range fields {
		value, present := values[name]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("invalid Octop object fields")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func (c *OctopExecutionCapability) UnmarshalJSON(raw []byte) error {
	type wire OctopExecutionCapability
	var value wire
	if err := decodeOctopObject(raw, &value, []string{"harness_version", "acp_sdk_version", "provider_id", "model", "endpoint_origin", "configuration_sha256", "budget_mode", "identity_evidence"}); err != nil {
		return err
	}
	if err := ValidateOctopExecutionCapability(OctopExecutionCapability(value)); err != nil {
		return err
	}
	*c = OctopExecutionCapability(value)
	return nil
}

func (e *OctopExecutionEvidence) UnmarshalJSON(raw []byte) error {
	type wire OctopExecutionEvidence
	var value wire
	if err := decodeOctopObject(raw, &value, []string{"configuration_sha256", "session_id", "requested_provider", "requested_model", "acknowledged_provider", "acknowledged_model", "stop_reason", "completed_at", "process_stopped", "cancel_requested"}); err != nil {
		return err
	}
	*e = OctopExecutionEvidence(value)
	return nil
}
