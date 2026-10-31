package platform

import "github.com/ai-mada/suma/internal/dag"

const workBuddyTaskDispatchInvalid = "Invalid WorkBuddy task dispatch request"
const workBuddyTaskDispatchChanged = "The WorkBuddy task dispatch basis changed; preview again"
const workBuddyTaskDispatchBaseline = "WorkBuddy task dispatch does not support the bound code baseline"

type WorkBuddyTaskDispatchPreviewRequest struct {
	ProjectID                 string                      `json:"project_id"`
	TaskID                    string                      `json:"task_id"`
	ExpectedTaskVersion       int                         `json:"expected_task_version"`
	ConnectionID              string                      `json:"connection_id"`
	ExpectedConnectionVersion int                         `json:"expected_connection_version"`
	GrantGeneration           int                         `json:"grant_generation"`
	Knowledge                 []ProjectKnowledgeReference `json:"knowledge,omitempty"`
	IdempotencyKey            string                      `json:"idempotency_key"`
}

type WorkBuddyTaskDispatchPlan struct {
	ID                string                    `json:"id"`
	ProjectID         string                    `json:"project_id"`
	OwnerID           string                    `json:"owner_id"`
	Version           int                       `json:"version"`
	State             string                    `json:"state"`
	TaskID            string                    `json:"task_id"`
	TaskVersion       int                       `json:"task_version"`
	ConnectionID      string                    `json:"connection_id"`
	ConnectionVersion int                       `json:"connection_version"`
	GrantGeneration   int                       `json:"grant_generation"`
	RunID             string                    `json:"run_id"`
	NodeID            string                    `json:"node_id"`
	DelegationID      string                    `json:"delegation_id"`
	CreatedAt         string                    `json:"created_at"`
	ExpiresAt         string                    `json:"expires_at"`
	Context           TaskContext               `json:"context"`
	Workflow          dag.Workflow              `json:"workflow"`
	ExecutionRoute    dag.ProjectExecutionRoute `json:"execution_route"`
	NodeSpecSHA256    string                    `json:"node_spec_sha256"`
	RequestSHA256     string                    `json:"request_sha256"`
	Valid             bool                      `json:"valid"`
	Reason            string                    `json:"reason"`
}

type WorkBuddyTaskDispatchStartRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
