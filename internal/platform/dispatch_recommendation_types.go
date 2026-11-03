package platform

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	drc "github.com/ai-mada/suma/internal/dispatchrecommendationcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

// Constraints are execution requirements, not instructions that can grant a
// model access to an executable, provider endpoint or credential.
type DispatchRecommendationConstraints struct {
	RequireHardBudget bool       `json:"require_hard_budget"`
	Budget            *mc.Budget `json:"budget,omitempty" wire:"nonnull"`
	MaxSteps          int        `json:"max_steps"`
	MaxOutputTokens   int64      `json:"max_output_tokens"`
}

type DispatchRecommendationOptionsRequest struct {
	ProjectID           string                            `json:"project_id"`
	TaskID              string                            `json:"task_id"`
	ExpectedTaskVersion int                               `json:"expected_task_version"`
	WorkflowID          string                            `json:"workflow_id"`
	WorkflowVersion     int                               `json:"workflow_version"`
	AgentNodeID         string                            `json:"agent_node_id"`
	Knowledge           []ProjectKnowledgeReference       `json:"knowledge"`
	Constraints         DispatchRecommendationConstraints `json:"constraints"`
}

// Each candidate is one real current profile/model alternative. Performance
// has no inferred score; prices are configuration quotes, never provider bills.
type DispatchRecommendationCandidate struct {
	ID                       string           `json:"id"`
	Kind                     string           `json:"kind"`
	ToolProfileID            string           `json:"tool_profile_id"`
	ToolProfileVersion       int              `json:"tool_profile_version"`
	ToolProfileSHA256        string           `json:"tool_profile_sha256"`
	WorkerID                 string           `json:"worker_id"`
	ToolName                 string           `json:"tool_name"`
	Adapter                  string           `json:"adapter"`
	Protocol                 string           `json:"protocol"`
	InventoryObservedAt      string           `json:"inventory_observed_at"`
	Model                    string           `json:"model"`
	ContextWindow            int64            `json:"context_window"`
	ModelState               string           `json:"model_state"`
	CostState                string           `json:"cost_state"`
	PerformanceState         string           `json:"performance_state"`
	BudgetMode               string           `json:"budget_mode"`
	ModelSelection           *mc.Selection    `json:"model_selection,omitempty" wire:"nonnull"`
	ModelRate                *mc.RateSnapshot `json:"model_rate,omitempty" wire:"nonnull"`
	CodexSelection           *cc.Selection    `json:"codex_selection,omitempty" wire:"nonnull"`
	ReasoningEfforts         []string         `json:"reasoning_efforts"`
	FixedConfigurationSHA256 string           `json:"fixed_configuration_sha256,omitempty"`
}

type DispatchRecommendationCoverage struct {
	ProfileTotal     int    `json:"profile_total"`
	ProfilesExamined int    `json:"profiles_examined"`
	CandidateCount   int    `json:"candidate_count"`
	ExcludedCount    int    `json:"excluded_count"`
	MaxProfiles      int    `json:"max_profiles"`
	MaxCandidates    int    `json:"max_candidates"`
	Complete         bool   `json:"complete"`
	Reason           string `json:"reason"`
}

type DispatchRecommendationSource struct {
	SHA256         string                            `json:"sha256"`
	PolicySHA256   string                            `json:"policy_sha256"`
	ObservedAt     string                            `json:"observed_at"`
	Context        TaskContext                       `json:"context"`
	Workflow       dag.Workflow                      `json:"workflow"`
	AgentNodeID    string                            `json:"agent_node_id"`
	ExecutionRoute dag.ProjectExecutionRoute         `json:"execution_route"`
	Constraints    DispatchRecommendationConstraints `json:"constraints"`
	Candidates     []DispatchRecommendationCandidate `json:"candidates"`
	Coverage       DispatchRecommendationCoverage    `json:"coverage"`
}

type DispatchRecommendationChoice = drc.DispatchRecommendationChoice
type DispatchRecommendationProposal = drc.DispatchRecommendationProposal

type DispatchRecommendation struct {
	// Frozen by the original dispatch builder at explicit adoption, never by the model.
	AdoptedTestBindings        []TaskDispatchTestBinding       `json:"adopted_test_bindings,omitempty" wire:"nonnull"`
	ID                         string                          `json:"id"`
	ProjectID                  string                          `json:"project_id"`
	Version                    int                             `json:"version"`
	State                      string                          `json:"state"`
	Goal                       string                          `json:"goal"`
	Locale                     string                          `json:"locale"`
	RunID                      string                          `json:"run_id"`
	Source                     DispatchRecommendationSource    `json:"source"`
	AnalysisToolProfileID      string                          `json:"analysis_tool_profile_id"`
	AnalysisToolProfileVersion int                             `json:"analysis_tool_profile_version"`
	AnalysisToolProfileSHA256  string                          `json:"analysis_tool_profile_sha256"`
	AnalysisModelExecution     *mc.Binding                     `json:"analysis_model_execution,omitempty" wire:"nonnull"`
	MaxOutputTokens            int64                           `json:"max_output_tokens"`
	Proposal                   *DispatchRecommendationProposal `json:"proposal,omitempty" wire:"nonnull"`
	OriginalOutputSHA256       string                          `json:"original_output_sha256"`
	OriginalOutput             string                          `json:"original_output"`
	CreatedBy                  string                          `json:"created_by"`
	CreatedAt                  string                          `json:"created_at"`
	UpdatedAt                  string                          `json:"updated_at"`
	ExpiresAt                  string                          `json:"expires_at"`
	BasisCurrent               bool                            `json:"basis_current"`
	Reason                     string                          `json:"reason"`
	AllowedActions             []string                        `json:"allowed_actions"`
	PlanID                     string                          `json:"plan_id"`
	AdoptedChoice              *DispatchRecommendationChoice   `json:"adopted_choice,omitempty" wire:"nonnull"`
}

type DispatchRecommendationCreateRequest struct {
	DispatchRecommendationOptionsRequest
	SourceSHA256                    string        `json:"source_sha256"`
	Goal                            string        `json:"goal"`
	Locale                          string        `json:"locale"`
	AnalysisToolProfileID           string        `json:"analysis_tool_profile_id"`
	ExpectedAnalysisToolVersion     int           `json:"expected_analysis_tool_version"`
	AnalysisModelExecutionSelection *mc.Selection `json:"analysis_model_execution_selection" wire:"nonnull"`
	AcknowledgeModelUse             bool          `json:"acknowledge_model_use"`
	IdempotencyKey                  string        `json:"idempotency_key"`
}

type DispatchRecommendationAdoptRequest struct {
	TestBindings    []TaskDispatchTestSelection  `json:"test_bindings,omitempty" wire:"nonnull"`
	ProjectID       string                       `json:"project_id"`
	ExpectedVersion int                          `json:"expected_version"`
	SourceSHA256    string                       `json:"source_sha256"`
	Choice          DispatchRecommendationChoice `json:"choice"`
	Reason          string                       `json:"reason"`
	IdempotencyKey  string                       `json:"idempotency_key"`
}

type DispatchRecommendationAdoption struct {
	Recommendation DispatchRecommendation `json:"recommendation"`
	Plan           TaskDispatchPlan       `json:"plan"`
}

type DispatchRecommendationPage struct {
	Items      []DispatchRecommendation `json:"items"`
	Total      int                      `json:"total"`
	NextCursor string                   `json:"next_cursor"`
}

// Provenance is stored in the original plan payload by its authenticated
// creator. Public direct-preview inputs do not accept this proof.
type DispatchRecommendationProvenance struct {
	RecommendationID      string `json:"recommendation_id"`
	RecommendationVersion int    `json:"recommendation_version"`
	SourceSHA256          string `json:"source_sha256"`
	OriginalOutputSHA256  string `json:"original_output_sha256"`
	CandidateID           string `json:"candidate_id"`
	ChoiceSHA256          string `json:"choice_sha256"`
	AdoptedBy             string `json:"adopted_by"`
	AdoptedAt             string `json:"adopted_at"`
	Reason                string `json:"reason"`
}
