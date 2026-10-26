package platform

import (
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// RepositoryProviderConfig is immutable startup configuration. Secrets and local
// target paths never enter public domain snapshots.
type RepositoryProviderConfig struct {
	ID             string   `json:"id"`
	TenantID       string   `json:"tenant_id"`
	ProjectID      string   `json:"project_id"`
	TargetAliases  []string `json:"target_aliases"`
	TokenHash      string   `json:"-"`
	Origin         string   `json:"-"`
	ReconcileToken string   `json:"-"`
}
type RepositoryIntegrationTarget struct {
	ID                string                          `json:"id"`
	ProjectID         string                          `json:"project_id"`
	Version           int                             `json:"version"`
	Name              string                          `json:"name"`
	ProviderID        string                          `json:"provider_id"`
	TargetAlias       string                          `json:"target_alias"`
	Ref               string                          `json:"ref"`
	ConfigSHA256      string                          `json:"config_sha256"`
	Status            string                          `json:"status"`
	Generation        int64                           `json:"generation"`
	ActiveOperationID string                          `json:"active_operation_id"`
	Observation       *ri.RepositoryTargetObservation `json:"observation,omitempty"`
	CanManage         bool                            `json:"can_manage"`
	Blockers          []string                        `json:"blockers"`
}
type RepositoryIntegrationBasis struct {
	TaskID            string              `json:"task_id"`
	TaskVersion       int                 `json:"task_version"`
	RunID             string              `json:"run_id"`
	SourceNodeID      string              `json:"source_node_id"`
	Source            cc.CandidateBinding `json:"source"`
	AcceptedTaskBasis ReleaseTaskBasis    `json:"accepted_task_basis"`
	BasisSHA256       string              `json:"basis_sha256"`
}
type RepositoryIntegrationApproval struct {
	ActorID        string `json:"actor_id"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	PlanVersion    int    `json:"plan_version"`
	PreparedSHA256 string `json:"prepared_sha256"`
	BasisSHA256    string `json:"basis_sha256"`
	At             string `json:"at"`
}
type RepositoryIntegrationPlan struct {
	ID             string                         `json:"id"`
	ProjectID      string                         `json:"project_id"`
	Version        int                            `json:"version"`
	TaskID         string                         `json:"task_id"`
	TargetID       string                         `json:"target_id"`
	State          string                         `json:"state"`
	Basis          RepositoryIntegrationBasis     `json:"basis"`
	Target         RepositoryIntegrationTarget    `json:"target"`
	PrepareRunID   string                         `json:"prepare_run_id"`
	ApplyRunID     string                         `json:"apply_run_id"`
	Preview        *ri.RepositoryPreview          `json:"preview,omitempty"`
	PreparedSHA256 string                         `json:"prepared_sha256"`
	Approval       *RepositoryIntegrationApproval `json:"approval,omitempty"`
	OperationID    string                         `json:"operation_id"`
	Observation    *ri.RepositoryObservation      `json:"observation,omitempty"`
	CreatedAt      string                         `json:"created_at"`
	ExpiresAt      string                         `json:"expires_at"`
	CanApprove     bool                           `json:"can_approve"`
	CanExecute     bool                           `json:"can_execute"`
	CanReconcile   bool                           `json:"can_reconcile"`
	Blockers       []string                       `json:"blockers"`
}
type RepositoryIntegrationSourceChoice struct {
	SourceNodeID string              `json:"source_node_id"`
	Source       cc.CandidateBinding `json:"source"`
	Eligible     bool                `json:"eligible"`
	Blockers     []string            `json:"blockers"`
}
type RepositoryIntegrationProviderChoice struct {
	ID            string   `json:"id"`
	TargetAliases []string `json:"target_aliases"`
}
type RepositoryIntegrationOptions struct {
	ProjectID   string                                `json:"project_id"`
	TaskID      string                                `json:"task_id"`
	TaskVersion int                                   `json:"task_version"`
	Sources     []RepositoryIntegrationSourceChoice   `json:"sources"`
	Targets     []RepositoryIntegrationTarget         `json:"targets"`
	Providers   []RepositoryIntegrationProviderChoice `json:"providers"`
	CanCreate   bool                                  `json:"can_create"`
	CanManage   bool                                  `json:"can_manage"`
	Blockers    []string                              `json:"blockers"`
}
type RepositoryIntegrationTargetCreateRequest struct {
	ProjectID      string `json:"project_id"`
	Name           string `json:"name"`
	ProviderID     string `json:"provider_id"`
	TargetAlias    string `json:"target_alias"`
	IdempotencyKey string `json:"idempotency_key"`
}
type RepositoryIntegrationTargetUpdateRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type RepositoryIntegrationPlanCreateRequest struct {
	ProjectID             string `json:"project_id"`
	TaskID                string `json:"task_id"`
	ExpectedTaskVersion   int    `json:"expected_task_version"`
	SourceNodeID          string `json:"source_node_id"`
	TargetID              string `json:"target_id"`
	ExpectedTargetVersion int    `json:"expected_target_version"`
	IdempotencyKey        string `json:"idempotency_key"`
}
type RepositoryIntegrationPlanCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	Acknowledge     bool   `json:"acknowledge"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type RepositoryIntegrationPlanPage struct {
	Items      []RepositoryIntegrationPlan `json:"items"`
	Total      int                         `json:"total"`
	NextCursor string                      `json:"next_cursor,omitempty"`
}

// Non-public state binds actual execution credentials to a fixed operation.
type repositoryOperation struct {
	Execution                            ri.RepositoryExecution
	RunID, NodeID, CreatedBy, ApprovedBy string
	CreatorCredential                    string
	Assignment                           *cc.CandidateAssignmentRef
	WorkerTokenHash                      string
	RequestSHA256                        string
	Observation                          *ri.RepositoryObservation
	Route                                dag.ProjectExecutionRoute
	Version                              int
}
