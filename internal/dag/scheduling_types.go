package dag

// SchedulingPolicyInput replaces the complete administrator admission policy.
// Nil capacities explicitly mean unlimited; zero is an enforced zero limit.
type SchedulingPolicyInput struct {
	ConcurrencyLimit int64             `json:"concurrency_limit"`
	ResourceLimits   map[string]*int64 `json:"resource_limits"`
	BudgetLimit      *int64            `json:"budget_limit"`
}

type SchedulingPolicy struct {
	ProjectID        string             `json:"project_id"`
	Version          int                `json:"version"`
	Default          bool               `json:"default"`
	ConcurrencyLimit int64              `json:"concurrency_limit"`
	ResourceLimits   map[string]*int64  `json:"resource_limits"`
	BudgetLimit      *int64             `json:"budget_limit"`
	BudgetSpent      int64              `json:"budget_spent"`
	Unit             string             `json:"unit"`
	BudgetHistory    string             `json:"budget_history"`
	AsOf             string             `json:"as_of"`
	Resources        []ResourceSnapshot `json:"resources"`
}

type ResourceSnapshot struct {
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	Capacity     *int64 `json:"capacity"`
	Used         int64  `json:"used"`
	Consumed     int64  `json:"consumed"`
	Mode         string `json:"mode"`
	ReleaseClass string `json:"release_class"`
}

type SchedulingBlocker struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	Resource  string `json:"resource,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Required  *int64 `json:"required,omitempty"`
	Capacity  *int64 `json:"capacity,omitempty"`
	Used      *int64 `json:"used,omitempty"`
	Available *int64 `json:"available,omitempty"`
	RetryAt   string `json:"retry_at,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
}

type ResourceRequest struct {
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	Amount       int64  `json:"amount"`
	ReleaseClass string `json:"release_class"`
}

type ResourceReservationSnapshot struct {
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	Amount       int64  `json:"amount"`
	ReleaseClass string `json:"release_class"`
	Attempt      int    `json:"attempt"`
	Fence        int64  `json:"fence"`
}

type NodeAdmissionSnapshot struct {
	RunID         string                        `json:"run_id"`
	NodeID        string                        `json:"node_id"`
	RunVersion    int                           `json:"run_version"`
	NodeState     string                        `json:"node_state"`
	Attempt       int                           `json:"attempt"`
	Fence         int64                         `json:"fence"`
	PolicyVersion int                           `json:"policy_version"`
	EvaluatedAt   string                        `json:"evaluated_at"`
	Status        string                        `json:"status"`
	Blockers      []SchedulingBlocker           `json:"blockers"`
	Requests      []ResourceRequest             `json:"requests"`
	Reservations  []ResourceReservationSnapshot `json:"reservations"`
}

type ReadyEntry struct {
	RunID        string                 `json:"run_id"`
	NodeID       string                 `json:"node_id"`
	NodeName     string                 `json:"node_name"`
	WorkflowName string                 `json:"workflow_name"`
	RunVersion   int                    `json:"run_version"`
	NodeState    string                 `json:"node_state"`
	Attempt      int                    `json:"attempt"`
	Fence        int64                  `json:"fence"`
	ReadyAt      string                 `json:"ready_at"`
	Sequence     int64                  `json:"sequence"`
	CommandID    string                 `json:"command_id,omitempty"`
	Type         string                 `json:"type"`
	Requests     []ResourceRequest      `json:"requests"`
	Admission    *NodeAdmissionSnapshot `json:"admission,omitempty"`
}

type ReadyPage struct {
	Items      []ReadyEntry `json:"items"`
	Total      int          `json:"total"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type ExecutionAttempt struct {
	RunID          string                 `json:"run_id"`
	NodeID         string                 `json:"node_id"`
	Attempt        int                    `json:"attempt"`
	Fence          int64                  `json:"fence"`
	WorkerID       string                 `json:"worker_id"`
	State          string                 `json:"state"`
	StartedAt      string                 `json:"started_at"`
	CompletedAt    string                 `json:"completed_at"`
	Result         *Result                `json:"result,omitempty"`
	ResultSource   string                 `json:"result_source"`
	Reconciliation *AttemptReconciliation `json:"reconciliation,omitempty"`
	ChildRunID     string                 `json:"child_run_id,omitempty"`
}

type AttemptReconciliation struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type AttemptPage struct {
	Items      []ExecutionAttempt `json:"items"`
	Total      int                `json:"total"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

type TickFailure struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	RunID     string `json:"run_id"`
	Message   string `json:"message"`
}

type TickReport struct {
	Examined int           `json:"examined"`
	Advanced int           `json:"advanced"`
	Failed   int           `json:"failed"`
	Failures []TickFailure `json:"failures"`
}
