package platform

type KnowledgeIntelligenceOutput struct {
	SchemaVersion int    `json:"schema_version"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Content       string `json:"content"`
	SourceIndices []int  `json:"source_indices"`
	Rationale     string `json:"rationale"`
}
type KnowledgeIntelligenceSource struct {
	SHA256  string                   `json:"sha256"`
	Sources []ProjectKnowledgeSource `json:"sources"`
}
type KnowledgeIntelligenceAnalysis struct {
	ID                   string                       `json:"id"`
	ProjectID            string                       `json:"project_id"`
	Version              int                          `json:"version"`
	State                string                       `json:"state"`
	Kind                 string                       `json:"kind"`
	Goal                 string                       `json:"goal"`
	Locale               string                       `json:"locale"`
	CreatedBy            string                       `json:"created_by"`
	CreatedAt            string                       `json:"created_at"`
	UpdatedAt            string                       `json:"updated_at"`
	ToolProfileID        string                       `json:"tool_profile_id"`
	ToolProfileVersion   int                          `json:"tool_profile_version"`
	ToolProfileSHA256    string                       `json:"tool_profile_sha256"`
	RunID                string                       `json:"run_id"`
	MaxSteps             int                          `json:"max_steps"`
	MaxOutputTokens      int                          `json:"max_output_tokens"`
	Source               KnowledgeIntelligenceSource  `json:"source"`
	Proposal             *KnowledgeIntelligenceOutput `json:"proposal,omitempty"`
	OriginalOutput       string                       `json:"original_output"`
	OriginalOutputSHA256 string                       `json:"original_output_sha256"`
	AdoptedKnowledgeID   string                       `json:"adopted_knowledge_id"`
	Reason               string                       `json:"reason"`
	AllowedActions       []string                     `json:"allowed_actions"`
	BasisCurrent         bool                         `json:"basis_current"`
}
type KnowledgeIntelligenceCreateRequest struct {
	ProjectID           string                        `json:"project_id"`
	Kind                string                        `json:"kind"`
	Goal                string                        `json:"goal"`
	Locale              string                        `json:"locale"`
	Sources             []ProjectKnowledgeSourceInput `json:"sources"`
	ToolProfileID       string                        `json:"tool_profile_id"`
	ExpectedToolVersion int                           `json:"expected_tool_version"`
	MaxSteps            int                           `json:"max_steps,omitempty"`
	MaxOutputTokens     int                           `json:"max_output_tokens,omitempty"`
	AcknowledgeModelUse bool                          `json:"acknowledge_model_use"`
	IdempotencyKey      string                        `json:"idempotency_key"`
}
type KnowledgeIntelligenceCommandRequest struct {
	ProjectID        string                       `json:"project_id"`
	ExpectedVersion  int                          `json:"expected_version"`
	Action           string                       `json:"action"`
	Proposal         *KnowledgeIntelligenceOutput `json:"proposal,omitempty"`
	Reason           string                       `json:"reason"`
	AcknowledgeDraft bool                         `json:"acknowledge_draft"`
	IdempotencyKey   string                       `json:"idempotency_key"`
}
type KnowledgeIntelligencePage struct {
	Items      []KnowledgeIntelligenceAnalysis `json:"items"`
	Total      int                             `json:"total"`
	NextCursor string                          `json:"next_cursor"`
}

// This immutable link describes the model proposal adopted into the first
// project draft. Later human edits/publications keep their own revision chain.
type KnowledgeIntelligenceAdoption struct {
	AnalysisID           string `json:"analysis_id"`
	AnalysisVersion      int    `json:"analysis_version"`
	RunID                string `json:"run_id"`
	ToolProfileSHA256    string `json:"tool_profile_sha256"`
	SourceSHA256         string `json:"source_sha256"`
	OriginalOutputSHA256 string `json:"original_output_sha256"`
	SourceIndices        []int  `json:"source_indices"`
	ProjectID            string `json:"project_id"`
	KnowledgeID          string `json:"knowledge_id"`
	Revision             int    `json:"revision"`
	SHA256               string `json:"sha256"`
	AdoptedBy            string `json:"adopted_by"`
	AdoptedAt            string `json:"adopted_at"`
	Reason               string `json:"reason"`
}
