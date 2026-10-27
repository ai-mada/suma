package platform

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type UsagePolicy struct {
	ID              string   `json:"id"`
	Scope           string   `json:"scope"`
	ProjectID       string   `json:"project_id"`
	Version         int      `json:"version"`
	Mode            string   `json:"mode"`
	Currency        string   `json:"currency"`
	TokenLimit      *int64   `json:"token_limit"`
	CostLimitMicros *int64   `json:"cost_limit_micros"`
	UpdatedBy       string   `json:"updated_by"`
	UpdatedAt       string   `json:"updated_at"`
	AllowedActions  []string `json:"allowed_actions"`
}
type UsagePolicyWriteRequest struct {
	ProjectID              string `json:"project_id"`
	Scope                  string `json:"scope"`
	ExpectedVersion        int    `json:"expected_version"`
	Mode                   string `json:"mode"`
	Currency               string `json:"currency"`
	TokenLimit             *int64 `json:"token_limit"`
	CostLimitMicros        *int64 `json:"cost_limit_micros"`
	Reason                 string `json:"reason"`
	AcknowledgeEnforcement bool   `json:"acknowledge_enforcement"`
	IdempotencyKey         string `json:"idempotency_key"`
}
type UsageRateRevision struct {
	ModelDeploymentID          string `json:"model_deployment_id,omitempty"`
	ModelDeploymentRevision    int    `json:"model_deployment_revision,omitempty"`
	ModelDeploymentSHA256      string `json:"model_deployment_sha256,omitempty"`
	ID                         string `json:"id"`
	ProjectID                  string `json:"project_id"`
	ToolProfileID              string `json:"tool_profile_id"`
	Version                    int    `json:"version"`
	ProfileSHA256              string `json:"profile_sha256"`
	Model                      string `json:"model"`
	EndpointOrigin             string `json:"endpoint_origin"`
	Currency                   string `json:"currency"`
	InputRateMicrosPerMillion  int64  `json:"input_rate_micros_per_million"`
	OutputRateMicrosPerMillion int64  `json:"output_rate_micros_per_million"`
	InputTokenReservation      int64  `json:"input_token_reservation"`
	OutputTokenReservation     int64  `json:"output_token_reservation"`
	Reason                     string `json:"reason"`
	CreatedBy                  string `json:"created_by"`
	CreatedAt                  string `json:"created_at"`
}
type UsageRateWriteRequest struct {
	ModelDeploymentID          string `json:"model_deployment_id,omitempty"`
	ModelDeploymentRevision    int    `json:"model_deployment_revision,omitempty"`
	ModelDeploymentSHA256      string `json:"model_deployment_sha256,omitempty"`
	ProjectID                  string `json:"project_id"`
	ToolProfileID              string `json:"tool_profile_id"`
	ExpectedToolVersion        int    `json:"expected_tool_version"`
	ExpectedVersion            int    `json:"expected_version"`
	Currency                   string `json:"currency"`
	InputRateMicrosPerMillion  int64  `json:"input_rate_micros_per_million"`
	OutputRateMicrosPerMillion int64  `json:"output_rate_micros_per_million"`
	InputTokenReservation      int64  `json:"input_token_reservation"`
	OutputTokenReservation     int64  `json:"output_token_reservation"`
	Reason                     string `json:"reason"`
	IdempotencyKey             string `json:"idempotency_key"`
}
type UsageMoneyBalance struct {
	Currency       string `json:"currency"`
	SpentMicros    *int64 `json:"spent_micros"`
	ReservedMicros *int64 `json:"reserved_micros"`
}
type UsageBalance struct {
	Scope              string              `json:"scope"`
	ProjectID          string              `json:"project_id"`
	PeriodStart        string              `json:"period_start"`
	ReportedTokens     *int64              `json:"reported_tokens"`
	ReservedTokens     *int64              `json:"reserved_tokens"`
	UnresolvedAttempts int                 `json:"unresolved_attempts"`
	Costs              []UsageMoneyBalance `json:"costs"`
}
type UsageDashboard struct {
	ProjectID             string              `json:"project_id"`
	PeriodStart           string              `json:"period_start"`
	AsOf                  string              `json:"as_of"`
	ProjectPolicy         UsagePolicy         `json:"project_policy"`
	OrganizationPolicy    *UsagePolicy        `json:"organization_policy,omitempty"`
	OrganizationEnforced  bool                `json:"organization_enforced"`
	Balances              []UsageBalance      `json:"balances"`
	Rates                 []UsageRateRevision `json:"rates"`
	RateTotal             int                 `json:"rate_total"`
	CanManageProject      bool                `json:"can_manage_project"`
	CanManageOrganization bool                `json:"can_manage_organization"`
	CanManageRates        bool                `json:"can_manage_rates"`
}
type UsageAttempt struct {
	CodexExecution      *cc.Binding            `json:"codex_execution,omitempty"`
	ModelExecution      *modelcontract.Binding `json:"model_execution,omitempty"`
	ID                  string                 `json:"id"`
	ProjectID           string                 `json:"project_id"`
	Version             int                    `json:"version"`
	RunID               string                 `json:"run_id"`
	NodeID              string                 `json:"node_id"`
	Attempt             int                    `json:"attempt"`
	Fence               int64                  `json:"fence"`
	WorkerID            string                 `json:"worker_id"`
	ToolProfileID       string                 `json:"tool_profile_id"`
	ProfileSHA256       string                 `json:"profile_sha256"`
	Model               string                 `json:"model"`
	EndpointOrigin      string                 `json:"endpoint_origin"`
	Currency            string                 `json:"currency"`
	Rate                *UsageRateRevision     `json:"rate,omitempty"`
	State               string                 `json:"state"`
	Usage               *toolcontract.Usage    `json:"usage,omitempty"`
	UsageState          string                 `json:"usage_state"`
	ReportedTokens      *int64                 `json:"reported_tokens"`
	EstimatedCostMicros *int64                 `json:"estimated_cost_micros"`
	ReservedTokens      int64                  `json:"reserved_tokens"`
	ReservedCostMicros  *int64                 `json:"reserved_cost_micros"`
	HeldTokens          int64                  `json:"held_tokens"`
	HeldCostMicros      *int64                 `json:"held_cost_micros"`
	UsageSource         string                 `json:"usage_source"`
	DecisionBy          string                 `json:"decision_by"`
	DecisionAt          string                 `json:"decision_at"`
	DecisionReason      string                 `json:"decision_reason"`
	SourceRef           string                 `json:"source_ref"`
	Overrun             bool                   `json:"overrun"`
	SourceResultSHA256  string                 `json:"source_result_sha256"`
	CreatedAt           string                 `json:"created_at"`
	UpdatedAt           string                 `json:"updated_at"`
	AllowedActions      []string               `json:"allowed_actions"`
}
type UsageAttemptPage struct {
	Items      []UsageAttempt `json:"items"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor"`
}
type UsageConfirmRequest struct {
	ProjectID        string             `json:"project_id"`
	ExpectedVersion  int                `json:"expected_version"`
	Usage            toolcontract.Usage `json:"usage"`
	SourceRef        string             `json:"source_ref"`
	Reason           string             `json:"reason"`
	AcknowledgeUsage bool               `json:"acknowledge_usage"`
	IdempotencyKey   string             `json:"idempotency_key"`
}
