package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

type PoolToolInventoryRequest struct {
	Preparation *tp.InventoryBinding           `json:"preparation,omitempty" wire:"nonnull"`
	RunnerID    string                         `json:"runner_id"`
	TenantID    string                         `json:"tenant_id"`
	ProjectID   string                         `json:"project_id"`
	OperationID string                         `json:"operation_id"`
	Items       []tc.Observation               `json:"items"`
	Inspection  *tic.Reference                 `json:"inspection,omitempty" wire:"nonnull"`
	Maintenance *tic.ToolMaintenanceCapability `json:"maintenance,omitempty" wire:"nonnull"`
}

type PoolRunnerAssignmentRequest struct {
	RunnerID   string         `json:"runner_id"`
	TenantID   string         `json:"tenant_id"`
	ProjectID  string         `json:"project_id"`
	Assignment dag.Assignment `json:"assignment"`
}

type PoolRunnerProgressRequest struct {
	PoolRunnerAssignmentRequest
	OperationID string    `json:"operation_id"`
	Message     string    `json:"message"`
	ToolEvent   *tc.Event `json:"tool_event,omitempty"`
}

type PoolRunnerResultRequest struct {
	PoolRunnerAssignmentRequest
	Result dag.Result `json:"result"`
}

type PoolRunnerClaimResponse struct {
	Assignment *dag.Assignment `json:"assignment"`
}
