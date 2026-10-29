// Package toolcontract defines the bounded, non-secret wire contract shared by
// the control plane, scheduler and execution nodes.
package toolcontract

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/modelcontract"
)

type ToolExecutionProfile struct {
	CommandID              string `json:"command_id"`
	Adapter                string `json:"adapter"`
	Protocol               string `json:"protocol"`
	ProfileSHA256          string `json:"profile_sha256"`
	ExecutableSHA256       string `json:"executable_sha256"`
	ToolVersion            string `json:"tool_version"`
	ReadOnly               bool   `json:"read_only"`
	WorkspaceToolsVersion  string `json:"workspace_tools_version,omitempty"`
	ModelExecutionProtocol string `json:"model_execution_protocol,omitempty"`
	// TestReportProtocol is a process-collected report capability of an isolated,
	// administrator-designated test command. It grants no case acceptance.
	TestReportProtocol string `json:"test_report_protocol,omitempty"`
	// ConflictRepairProtocol is an explicitly enabled host-native capability, not a grant of Git or acceptance authority.
	ConflictRepairProtocol string                            `json:"conflict_repair_protocol,omitempty"`
	Planning               *PlanningCapability               `json:"planning,omitempty"`
	KnowledgeExtraction    *KnowledgeExtractionCapability    `json:"knowledge_extraction,omitempty"`
	IntelligentOperations  *IntelligentOperationsCapability  `json:"intelligent_operations,omitempty"`
	DispatchRecommendation *DispatchRecommendationCapability `json:"dispatch_recommendation,omitempty"`
	TaskConversationReply  *TaskConversationReplyCapability  `json:"task_conversation_reply,omitempty"`
	FailureDiagnosis       *FailureDiagnosisCapability       `json:"failure_diagnosis,omitempty"`
	// OpenCodeExecution is the immutable administrator configuration acknowledged
	// by the pinned SDK/server probe, not an independently selectable deployment.
	OpenCodeExecution *OpenCodeExecutionCapability `json:"opencode_execution,omitempty"`
	OctopExecution    *OctopExecutionCapability    `json:"octop_execution,omitempty"`
}

// PlanningCapability names a probed, fixed model configuration, not account health.
type PlanningCapability struct {
	Protocol       string `json:"protocol"`
	Model          string `json:"model"`
	EndpointOrigin string `json:"endpoint_origin"`
}

const PlanningProtocol = "suma-schedule-plan-v1"

// KnowledgeExtractionCapability fixes a text-only model purpose; it grants no tools.
type KnowledgeExtractionCapability struct {
	Protocol       string `json:"protocol"`
	Model          string `json:"model"`
	EndpointOrigin string `json:"endpoint_origin"`
}

const KnowledgeExtractionProtocol = "suma-knowledge-extraction-v1"

// FailureDiagnosisCapability fixes a single-call, text-only diagnostic purpose.
// It grants no tools, repair execution, retries or task acceptance authority.
type FailureDiagnosisCapability struct {
	Protocol       string `json:"protocol"`
	Model          string `json:"model"`
	EndpointOrigin string `json:"endpoint_origin"`
}

const FailureDiagnosisProtocol = "suma-failure-diagnosis-v1"

// EqualProfile compares capability values even after independent wire decoding.
func EqualProfile(a, b Profile) bool {
	ap, bp := a.Planning, b.Planning
	ak, bk := a.KnowledgeExtraction, b.KnowledgeExtraction
	ai, bi := a.IntelligentOperations, b.IntelligentOperations
	af, bf := a.FailureDiagnosis, b.FailureDiagnosis
	ao, bo := a.OpenCodeExecution, b.OpenCodeExecution
	at, bt := a.OctopExecution, b.OctopExecution
	ad, bd := a.DispatchRecommendation, b.DispatchRecommendation
	ac, bc := a.TaskConversationReply, b.TaskConversationReply
	a.Planning, b.Planning = nil, nil
	a.KnowledgeExtraction, b.KnowledgeExtraction = nil, nil
	a.IntelligentOperations, b.IntelligentOperations = nil, nil
	a.FailureDiagnosis, b.FailureDiagnosis = nil, nil
	a.OpenCodeExecution, b.OpenCodeExecution = nil, nil
	a.OctopExecution, b.OctopExecution = nil, nil
	a.DispatchRecommendation, b.DispatchRecommendation = nil, nil
	a.TaskConversationReply, b.TaskConversationReply = nil, nil
	if !(ac == nil && bc == nil || ac != nil && bc != nil && *ac == *bc) {
		return false
	}
	if !(ad == nil && bd == nil || ad != nil && bd != nil && *ad == *bd) {
		return false
	}
	return a == b && (ap == nil && bp == nil || ap != nil && bp != nil && *ap == *bp) && (ak == nil && bk == nil || ak != nil && bk != nil && *ak == *bk) && (ai == nil && bi == nil || ai != nil && bi != nil && *ai == *bi) && (af == nil && bf == nil || af != nil && bf != nil && *af == *bf) && (ao == nil && bo == nil || ao != nil && bo != nil && *ao == *bo) && (at == nil && bt == nil || at != nil && bt != nil && *at == *bt)
}

func CloneProfile(p Profile) Profile {
	if p.TaskConversationReply != nil {
		value := *p.TaskConversationReply
		p.TaskConversationReply = &value
	}
	if p.DispatchRecommendation != nil {
		value := *p.DispatchRecommendation
		p.DispatchRecommendation = &value
	}
	if p.OctopExecution != nil {
		value := *p.OctopExecution
		p.OctopExecution = &value
	}
	if p.IntelligentOperations != nil {
		value := *p.IntelligentOperations
		p.IntelligentOperations = &value
	}
	if p.Planning != nil {
		value := *p.Planning
		p.Planning = &value
	}
	if p.KnowledgeExtraction != nil {
		value := *p.KnowledgeExtraction
		p.KnowledgeExtraction = &value
	}
	if p.FailureDiagnosis != nil {
		value := *p.FailureDiagnosis
		p.FailureDiagnosis = &value
	}
	if p.OpenCodeExecution != nil {
		value := *p.OpenCodeExecution
		p.OpenCodeExecution = &value
	}
	return p
}

type ToolObservation struct {
	Profile          Profile                    `json:"profile"`
	Available        bool                       `json:"available"`
	Reason           string                     `json:"reason"`
	ModelDeployments []modelcontract.Deployment `json:"model_deployments,omitempty"`
	// CodexCatalog is from actual model/list under the pinned local configuration.
	CodexCatalog *cc.Catalog `json:"codex_catalog,omitempty"`
	// These are the current Runner's outbound transport implementations, not
	// approved command inputs or proof that a remote host is connected/healthy.
	WorkspaceObservationProtocol string `json:"workspace_observation_protocol,omitempty"`
	WorkspaceWriteProtocol       string `json:"workspace_write_protocol,omitempty"`
}

type ToolUsage struct {
	InputTokens           *int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
	OutputTokens          *int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
}

// MaxSafeUsage is the largest exact integer shared by the Go and browser wire.
const MaxSafeUsage int64 = 9007199254740991

const (
	UsageComplete   = "complete"
	UsagePartial    = "partial"
	UsageUnreported = "unreported"
)

type ToolEvent struct {
	SchemaVersion int             `json:"schema_version"`
	CommandID     string          `json:"command_id"`
	ProfileSHA256 string          `json:"profile_sha256"`
	Protocol      string          `json:"protocol"`
	Kind          string          `json:"kind"`
	SessionID     string          `json:"session_id,omitempty"`
	ItemID        string          `json:"item_id,omitempty"`
	Message       string          `json:"message,omitempty"`
	Status        string          `json:"status,omitempty"`
	Usage         *Usage          `json:"usage,omitempty"`
	UsageState    string          `json:"usage_state,omitempty"`
	Diagnostic    *ToolDiagnostic `json:"diagnostic,omitempty"`
}

// ToolDiagnostic is display identity, not authority, terminal evidence or the
// native process's rendered prose. Its source is always ToolEvent.Message.
type ToolDiagnostic struct {
	SchemaVersion int            `json:"schema_version"`
	MessageKey    string         `json:"message_key"`
	MessageParams map[string]any `json:"message_params,omitempty"`
}

type ToolEvidence struct {
	Profile        Profile                 `json:"profile"`
	SessionID      string                  `json:"session_id,omitempty"`
	Terminal       string                  `json:"terminal"`
	EventCount     int                     `json:"event_count"`
	Usage          *Usage                  `json:"usage,omitempty"`
	UsageState     string                  `json:"usage_state"`
	ModelExecution *modelcontract.Evidence `json:"model_execution,omitempty"`
	// CodexExecution records requested/acknowledged configuration, never provider billing identity.
	CodexExecution    *cc.Evidence               `json:"codex_execution,omitempty"`
	OpenCodeExecution *OpenCodeExecutionEvidence `json:"opencode_execution,omitempty"`
	OctopExecution    *OctopExecutionEvidence    `json:"octop_execution,omitempty"`
}

type Profile = ToolExecutionProfile
type Observation = ToolObservation
type Usage = ToolUsage
type Event = ToolEvent
type Evidence = ToolEvidence
