package toolcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

const OpenCodeSDKProtocol = "opencode-sdk-v1"
const OpenCodeSDKVersion = "1.19.4"

// OpenCodeExecutionCapability is a fixed local configuration acknowledgement.
// It does not attest provider identity, provider health, billing or a hard cap.
type OpenCodeExecutionCapability struct {
	SDKVersion          string `json:"sdk_version"`
	ServerVersion       string `json:"server_version"`
	ProviderID          string `json:"provider_id"`
	Model               string `json:"model"`
	EndpointOrigin      string `json:"endpoint_origin"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
}

// OpenCodeExecutionEvidence retains the exact original prompt correlation and
// the supervisor's process-stop fact. An abort ACK is not process quiescence.
type OpenCodeExecutionEvidence struct {
	ConfigurationSHA256  string `json:"configuration_sha256"`
	SessionID            string `json:"session_id"`
	UserMessageID        string `json:"user_message_id"`
	AssistantMessageID   string `json:"assistant_message_id"`
	RequestedProvider    string `json:"requested_provider"`
	RequestedModel       string `json:"requested_model"`
	AcknowledgedProvider string `json:"acknowledged_provider"`
	AcknowledgedModel    string `json:"acknowledged_model"`
	Finish               string `json:"finish"`
	CompletedAt          string `json:"completed_at"`
	ServerStopped        bool   `json:"server_stopped"`
	AbortAcknowledged    bool   `json:"abort_acknowledged"`
}

var openCodeModel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$`)

func ValidateOpenCodeExecutionCapability(c OpenCodeExecutionCapability) error {
	if c.SDKVersion != OpenCodeSDKVersion || c.ServerVersion != OpenCodeSDKVersion || !ValidCommandID(c.ProviderID) || !openCodeModel.MatchString(c.Model) || !validFixedModel(c.Model, c.EndpointOrigin) || !ValidDigest(c.ConfigurationSHA256) || c.BudgetMode != "observe_only" || c.IdentityEvidence != "configuration_ack_only" {
		return fmt.Errorf("invalid fixed OpenCode SDK configuration")
	}
	return nil
}

// SupportsOpenCodeExecutionProfile names the implemented host-tool bridge. A
// current inventory must separately advertise each actual outbound transport.
func SupportsOpenCodeExecutionProfile(p Profile) bool {
	return p.Adapter == "opencode" && p.Protocol == OpenCodeSDKProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "attempt-workspace-v1" && p.ModelExecutionProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.OpenCodeExecution != nil && ValidateOpenCodeExecutionCapability(*p.OpenCodeExecution) == nil
}

// DecodeOpenCodeNodeConfiguration admits only the fixed reviewed model fact.
// Per-node provider/process/tool permissions and native hard limits are not
// OpenCode SDK inputs, including when their JSON value is null.
func DecodeOpenCodeNodeConfiguration(config map[string]any) (*OpenCodeExecutionCapability, error) {
	value, present := config["opencode_execution"]
	if !present {
		return nil, nil
	}
	if cc.HasNodeOverrides(config) {
		return nil, fmt.Errorf("invalid OpenCode node override")
	}
	for _, field := range []string{"codex_execution", "max_tokens", "max_cost", "max_cost_micros", "hard_budget", "token_budget", "budget", "resume", "session_id", "tools", "mcp_servers"} {
		if _, exists := config[field]; exists {
			return nil, fmt.Errorf("invalid OpenCode node override")
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fixed OpenCodeExecutionCapability
	if err := json.Unmarshal(raw, &fixed); err != nil {
		return nil, err
	}
	return &fixed, nil
}

func validateOpenCodeProfile(p Profile, complete bool) error {
	if p.Adapter != "opencode" && p.Protocol != OpenCodeSDKProtocol && p.OpenCodeExecution == nil {
		return nil
	}
	if SupportsOpenCodeExecutionProfile(p) {
		return nil
	}
	// A missing/failed local probe can report an unavailable configured command
	// without inventing an acknowledged SDK model or host-tool capability.
	if !complete && p.Adapter == "opencode" && p.Protocol == OpenCodeSDKProtocol && !p.ReadOnly && p.WorkspaceToolsVersion == "" && p.ModelExecutionProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.OpenCodeExecution == nil {
		return nil
	}
	return fmt.Errorf("invalid OpenCode SDK host-workspace capability")
}

func validateOpenCodeEvidence(e Evidence) error {
	if e.OpenCodeExecution == nil {
		if e.Profile.Protocol == OpenCodeSDKProtocol {
			return fmt.Errorf("OpenCode execution evidence is required")
		}
		return nil
	}
	c, proof := e.Profile.OpenCodeExecution, e.OpenCodeExecution
	if !SupportsOpenCodeExecutionProfile(e.Profile) || e.CodexExecution != nil || e.ModelExecution != nil || e.UsageState == UsageComplete || c == nil || proof.ConfigurationSHA256 != c.ConfigurationSHA256 || proof.RequestedProvider != c.ProviderID || proof.RequestedModel != c.Model || e.SessionID != proof.SessionID {
		return fmt.Errorf("invalid OpenCode execution configuration evidence")
	}
	for _, id := range []string{proof.SessionID, proof.UserMessageID, proof.AssistantMessageID} {
		if id != "" && !safeID.MatchString(id) {
			return fmt.Errorf("invalid OpenCode execution identifier")
		}
	}
	if proof.AssistantMessageID != "" && (proof.SessionID == "" || proof.UserMessageID == "" || proof.AssistantMessageID == proof.UserMessageID) || proof.UserMessageID != "" && proof.SessionID == "" || proof.AbortAcknowledged && proof.SessionID == "" {
		return fmt.Errorf("invalid OpenCode prompt correlation")
	}
	if proof.AcknowledgedProvider != "" && !ValidCommandID(proof.AcknowledgedProvider) || proof.AcknowledgedModel != "" && !openCodeModel.MatchString(proof.AcknowledgedModel) || !validText(proof.Finish, 64, false) {
		return fmt.Errorf("invalid OpenCode acknowledgement")
	}
	if proof.CompletedAt != "" {
		completed, err := time.Parse(time.RFC3339Nano, proof.CompletedAt)
		if err != nil || completed.Location() != time.UTC || completed.Format(time.RFC3339Nano) != proof.CompletedAt {
			return fmt.Errorf("invalid OpenCode completion time")
		}
	}
	if e.Terminal == "completed" && (proof.SessionID == "" || proof.UserMessageID == "" || proof.AssistantMessageID == "" || proof.AcknowledgedProvider != proof.RequestedProvider || proof.AcknowledgedModel != proof.RequestedModel || proof.Finish != "stop" || proof.CompletedAt == "" || !proof.ServerStopped || proof.AbortAcknowledged) {
		return fmt.Errorf("OpenCode whole-turn completion and server stop are required")
	}
	return nil
}

// ValidateOpenCodeResultStop checks the supervisor receipt in the result's
// context. Normal private-server cleanup does not set the DAG's stopped flag.
func ValidateOpenCodeResultStop(e Evidence, stopped, outcomeUnknown bool) error {
	if e.Profile.Protocol != OpenCodeSDKProtocol && e.OpenCodeExecution == nil {
		return nil
	}
	if err := ValidateEvidence(e); err != nil {
		return err
	}
	proof := e.OpenCodeExecution
	if proof == nil || stopped && !proof.ServerStopped || !stopped && !outcomeUnknown && !proof.ServerStopped || stopped && !outcomeUnknown && proof.SessionID != "" && !proof.AbortAcknowledged {
		return fmt.Errorf("OpenCode result requires original abort and private server stop facts")
	}
	return nil
}

// Decode exact nested objects even when callers use ordinary encoding/json:
// case aliases, duplicate keys, null facts and unknown properties are rejected.
func decodeOpenCodeObject(raw []byte, target any, fields []string) error {
	if len(raw) > 8192 || !ValidJSONUnicode(raw) || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("invalid OpenCode object encoding")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	if err := scanValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("trailing OpenCode object")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil || values == nil {
		return fmt.Errorf("invalid OpenCode object")
	}
	for key, value := range values {
		known := false
		for _, field := range fields {
			known = known || key == field
		}
		if !known || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("invalid OpenCode field")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func (c *OpenCodeExecutionCapability) UnmarshalJSON(raw []byte) error {
	type wire OpenCodeExecutionCapability
	var value wire
	if err := decodeOpenCodeObject(raw, &value, []string{"sdk_version", "server_version", "provider_id", "model", "endpoint_origin", "configuration_sha256", "budget_mode", "identity_evidence"}); err != nil {
		return err
	}
	if err := ValidateOpenCodeExecutionCapability(OpenCodeExecutionCapability(value)); err != nil {
		return err
	}
	*c = OpenCodeExecutionCapability(value)
	return nil
}

func (e *OpenCodeExecutionEvidence) UnmarshalJSON(raw []byte) error {
	type wire OpenCodeExecutionEvidence
	var value wire
	if err := decodeOpenCodeObject(raw, &value, []string{"configuration_sha256", "session_id", "user_message_id", "assistant_message_id", "requested_provider", "requested_model", "acknowledged_provider", "acknowledged_model", "finish", "completed_at", "server_stopped", "abort_acknowledged"}); err != nil {
		return err
	}
	*e = OpenCodeExecutionEvidence(value)
	return nil
}
