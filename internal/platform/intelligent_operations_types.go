package platform

// The closed proposal language describes local drafts only. It never carries
// credentials, arbitrary HTTP requests, approval, execution or acceptance.
type IntelligentOperationRequirement struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type IntelligentOperationDesign struct {
	RequirementID      string                   `json:"requirement_id"`
	RequirementStepKey string                   `json:"requirement_step_key"`
	Content            RequirementDesignContent `json:"content"`
	Tasks              []RequirementDesignTask  `json:"tasks"`
}
type IntelligentOperationKnowledge struct {
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Content       string `json:"content"`
	SourceIndices []int  `json:"source_indices"`
}
type IntelligentOperationStep struct {
	Key         string                           `json:"key"`
	Action      string                           `json:"action"`
	DependsOn   []string                         `json:"depends_on"`
	Requirement *IntelligentOperationRequirement `json:"requirement,omitempty"`
	Design      *IntelligentOperationDesign      `json:"design,omitempty"`
	Knowledge   *IntelligentOperationKnowledge   `json:"knowledge,omitempty"`
}
type IntelligentOperationOutput struct {
	SchemaVersion int                        `json:"schema_version"`
	Summary       string                     `json:"summary"`
	Rationale     string                     `json:"rationale"`
	Questions     []string                   `json:"questions"`
	Steps         []IntelligentOperationStep `json:"steps"`
}
type IntelligentOperationSource struct {
	SHA256  string                   `json:"sha256"`
	Sources []ProjectKnowledgeSource `json:"sources"`
}

// Results are immutable adoption evidence, separate from subsequent target edits.
type IntelligentOperationStepResult struct {
	Key                  string `json:"key"`
	Action               string `json:"action"`
	ResourceKind         string `json:"resource_kind"`
	ResourceID           string `json:"resource_id"`
	Version              int    `json:"version"`
	Revision             int    `json:"revision"`
	SHA256               string `json:"sha256"`
	SourceSHA256         string `json:"source_sha256"`
	ProposalSHA256       string `json:"proposal_sha256"`
	OriginalOutputSHA256 string `json:"original_output_sha256"`
	RunID                string `json:"run_id"`
}
type IntelligentOperationAnalysis struct {
	ID                   string                           `json:"id"`
	ProjectID            string                           `json:"project_id"`
	Version              int                              `json:"version"`
	State                string                           `json:"state"`
	Goal                 string                           `json:"goal"`
	Locale               string                           `json:"locale"`
	CreatedBy            string                           `json:"created_by"`
	CreatedAt            string                           `json:"created_at"`
	UpdatedAt            string                           `json:"updated_at"`
	ToolProfileID        string                           `json:"tool_profile_id"`
	ToolProfileVersion   int                              `json:"tool_profile_version"`
	ToolProfileSHA256    string                           `json:"tool_profile_sha256"`
	RunID                string                           `json:"run_id"`
	MaxSteps             int                              `json:"max_steps"`
	MaxOutputTokens      int                              `json:"max_output_tokens"`
	Source               IntelligentOperationSource       `json:"source"`
	Proposal             *IntelligentOperationOutput      `json:"proposal,omitempty"`
	OriginalOutput       string                           `json:"original_output"`
	OriginalOutputSHA256 string                           `json:"original_output_sha256"`
	Reason               string                           `json:"reason"`
	ReasonKey            string                           `json:"reason_key,omitempty"`
	Results              []IntelligentOperationStepResult `json:"results"`
	AppliedBy            string                           `json:"applied_by"`
	AppliedAt            string                           `json:"applied_at"`
	AppliedPreviewSHA256 string                           `json:"applied_preview_sha256"`
	AllowedActions       []string                         `json:"allowed_actions"`
	BasisCurrent         bool                             `json:"basis_current"`
}
type IntelligentOperationCreateRequest struct {
	ProjectID           string                        `json:"project_id"`
	Goal                string                        `json:"goal"`
	Locale              string                        `json:"locale"`
	Sources             []ProjectKnowledgeSourceInput `json:"sources"`
	ToolProfileID       string                        `json:"tool_profile_id"`
	ExpectedToolVersion int                           `json:"expected_tool_version"`
	MaxOutputTokens     int                           `json:"max_output_tokens"`
	AcknowledgeModelUse bool                          `json:"acknowledge_model_use"`
	IdempotencyKey      string                        `json:"idempotency_key"`
}
type IntelligentOperationCommandRequest struct {
	ProjectID         string                      `json:"project_id"`
	ExpectedVersion   int                         `json:"expected_version"`
	Action            string                      `json:"action"`
	Proposal          *IntelligentOperationOutput `json:"proposal,omitempty"`
	PreviewSHA256     string                      `json:"preview_sha256"`
	AcknowledgeDrafts bool                        `json:"acknowledge_drafts"`
	Reason            string                      `json:"reason"`
	IdempotencyKey    string                      `json:"idempotency_key"`
}
type IntelligentOperationPreviewRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
}
type IntelligentOperationDefault struct {
	StepKey string `json:"step_key"`
	Field   string `json:"field"`
	Value   string `json:"value"`
	Source  string `json:"source"`
}
type IntelligentOperationImpact struct {
	Key                string   `json:"key"`
	Action             string   `json:"action"`
	TargetKind         string   `json:"target_kind"`
	TargetID           string   `json:"target_id"`
	TargetStepKey      string   `json:"target_step_key"`
	CurrentVersion     int      `json:"current_version"`
	RequirementVersion int      `json:"requirement_version"`
	Title              string   `json:"title"`
	DependsOn          []string `json:"depends_on"`
	RequiredPermission string   `json:"required_permission"`
	Risk               string   `json:"risk"`
}
type IntelligentOperationPreview struct {
	ID             string                        `json:"id"`
	ProjectID      string                        `json:"project_id"`
	Version        int                           `json:"version"`
	SourceSHA256   string                        `json:"source_sha256"`
	ProposalSHA256 string                        `json:"proposal_sha256"`
	PreviewSHA256  string                        `json:"preview_sha256"`
	Policy         string                        `json:"policy"`
	CanApply       bool                          `json:"can_apply"`
	BasisCurrent   bool                          `json:"basis_current"`
	Questions      []string                      `json:"questions"`
	Defaults       []IntelligentOperationDefault `json:"defaults"`
	Steps          []IntelligentOperationImpact  `json:"steps"`
}
type IntelligentOperationPage struct {
	Items      []IntelligentOperationAnalysis `json:"items"`
	Total      int                            `json:"total"`
	NextCursor string                         `json:"next_cursor"`
}
