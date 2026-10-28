package toolcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	mc "github.com/ai-mada/suma/internal/modelcontract"
)

const OctopProtocol = "octop-harness-acp-v1"
const OctopHarnessVersion = "1.0.5"
const OctopACPSDKVersion = "0.12.1"

// OctopExecutionCapability identifies only the SDK execution adapter. Model,
// parameters, credentials and budget have one authority: ModelBinding + Go host.
type OctopExecutionCapability struct {
	HarnessVersion string `json:"harness_version"`
	ACPSDKVersion  string `json:"acp_sdk_version"`
}

// OctopExecutionEvidence retains the actual ACP session and independent Go
// process-stop proof. ACP cancellation is a notification, not an abort ACK.
// Octop end_turn by itself cannot prove a successful turn or code delivery.
type OctopExecutionEvidence struct {
	BindingSHA256   string `json:"binding_sha256"`
	SessionID       string `json:"session_id"`
	StopReason      string `json:"stop_reason"`
	CompletedAt     string `json:"completed_at"`
	ProcessStopped  bool   `json:"process_stopped"`
	CancelRequested bool   `json:"cancel_requested"`
}

func ValidateOctopExecutionCapability(c OctopExecutionCapability) error {
	if c.HarnessVersion != OctopHarnessVersion || c.ACPSDKVersion != OctopACPSDKVersion {
		return fmt.Errorf("invalid Octop ACP runtime capability")
	}
	return nil
}

// The protocol is enabled only with the implemented host workspace. A profile
// cannot acquire native model limits, repair, tests or another model purpose.
func SupportsOctopExecutionProfile(p Profile) bool {
	return p.Adapter == "octop" && p.Protocol == OctopProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "attempt-workspace-v1" && p.ModelExecutionProtocol == mc.Protocol && p.ConflictRepairProtocol == "" && p.TestReportProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution != nil && ValidateOctopExecutionCapability(*p.OctopExecution) == nil
}

func validateOctopProfile(p Profile, complete bool) error {
	if p.Adapter != "octop" && p.Protocol != OctopProtocol && p.OctopExecution == nil {
		return nil
	}
	if SupportsOctopExecutionProfile(p) {
		return nil
	}
	if !complete && p.Adapter == "octop" && p.Protocol == OctopProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == mc.Protocol && p.ConflictRepairProtocol == "" && p.TestReportProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid Octop ACP host-workspace capability")
}

// The only model input is the reviewed ModelBinding. Process and SDK state are
// administrator-owned; old Octop snapshots are rejected rather than migrated.
func ValidateOctopNodeConfiguration(config map[string]any) error {
	for _, field := range []string{"octop_execution", "codex_execution", "opencode_execution", "model", "model_id", "provider", "provider_id", "base_url", "model_base_url", "endpoint", "endpoint_url", "api_key", "env", "env_from", "cwd", "max_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "session_id", "tools", "mcp_servers", "agent", "agent_id", "expert", "parameters", "model_parameters", "home", "octop_home", "workspace", "python", "executable", "args", "api_key_env_ref"} {
		if _, exists := config[field]; exists {
			return fmt.Errorf("invalid Octop node override")
		}
	}
	return nil
}

func validateOctopEvidence(e Evidence) error {
	if e.OctopExecution == nil {
		if e.Profile.Protocol == OctopProtocol {
			return fmt.Errorf("Octop execution evidence is required")
		}
		return nil
	}
	proof := e.OctopExecution
	if !SupportsOctopExecutionProfile(e.Profile) || e.OpenCodeExecution != nil || e.CodexExecution != nil || e.ModelExecution == nil || !ValidDigest(proof.BindingSHA256) || proof.BindingSHA256 != e.ModelExecution.BindingSHA256 || e.SessionID != proof.SessionID {
		return fmt.Errorf("invalid Octop execution configuration evidence")
	}
	if proof.SessionID != "" && !safeID.MatchString(proof.SessionID) {
		return fmt.Errorf("invalid Octop execution session")
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
	if e.Terminal == "completed" && (proof.SessionID == "" || proof.StopReason != "end_turn" || proof.CompletedAt == "" || !proof.ProcessStopped || proof.CancelRequested || !e.ModelExecution.GatewayClosed) {
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
	if err := decodeOctopObject(raw, &value, []string{"harness_version", "acp_sdk_version"}); err != nil {
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
	if err := decodeOctopObject(raw, &value, []string{"binding_sha256", "session_id", "stop_reason", "completed_at", "process_stopped", "cancel_requested"}); err != nil {
		return err
	}
	*e = OctopExecutionEvidence(value)
	return nil
}
