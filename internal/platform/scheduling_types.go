package platform

import "github.com/ai-mada/suma/internal/dag"

type RunnerClaimRequest struct {
	ProjectID      string   `json:"project_id"`
	WorkerID       string   `json:"worker_id"`
	Capabilities   []string `json:"capabilities"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type RunnerClaimResponse struct {
	Assignment *dag.Assignment `json:"assignment"`
}

type SchedulingFairness struct {
	Mode  string `json:"mode"`
	Scope string `json:"scope"`
}

type SchedulingPolicyView struct {
	dag.SchedulingPolicy
	AllowedActions []string           `json:"allowed_actions"`
	Fairness       SchedulingFairness `json:"fairness"`
}

type SchedulingPolicyPreviewRequest struct {
	ProjectID       string                    `json:"project_id"`
	ExpectedVersion int                       `json:"expected_version"`
	Policy          dag.SchedulingPolicyInput `json:"policy"`
	IdempotencyKey  string                    `json:"idempotency_key"`
}

type SchedulingPolicyCommitRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type SchedulingPolicyChange struct {
	Resource string `json:"resource"`
	Unit     string `json:"unit"`
	Before   *int64 `json:"before"`
	After    *int64 `json:"after"`
}

type SchedulingPolicyImpact struct {
	ObservedAt             string                 `json:"observed_at"`
	ReadyTotal             int                    `json:"ready_total"`
	ReservedExecutionSlots int64                  `json:"reserved_execution_slots"`
	BudgetSpent            int64                  `json:"budget_spent"`
	BudgetHistory          string                 `json:"budget_history"`
	Resources              []dag.ResourceSnapshot `json:"resources"`
	Notes                  []string               `json:"notes"`
}

type SchedulingPolicyPreview struct {
	ID                   string                    `json:"id"`
	ProjectID            string                    `json:"project_id"`
	Version              int                       `json:"version"`
	State                string                    `json:"state"`
	PolicyVersion        int                       `json:"policy_version"`
	AppliedPolicyVersion int                       `json:"applied_policy_version,omitempty"`
	Policy               dag.SchedulingPolicyInput `json:"policy"`
	PreviousPolicy       dag.SchedulingPolicyInput `json:"previous_policy"`
	Changes              []SchedulingPolicyChange  `json:"changes"`
	Impact               SchedulingPolicyImpact    `json:"impact"`
	PolicySHA256         string                    `json:"policy_sha256"`
	CreatedAt            string                    `json:"created_at"`
	ExpiresAt            string                    `json:"expires_at"`
	Valid                bool                      `json:"valid"`
	Reason               string                    `json:"reason,omitempty"`
}
