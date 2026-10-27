package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/modelcontract"
)

// A new preview/revision selects the current quote. A pending source check
// retains the reviewed binding and its exact historical quote, just as ordinary
// Start does. This path neither reprices a plan nor creates a usage reservation.
func (s *Service) taskDispatchSourceModelTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, request TaskDispatchPreviewRequest, source *TaskDispatchPlan) (*modelcontract.Binding, error) {
	if source == nil {
		return s.buildModelExecutionTx(ctx, tx, tenant, project, profile, request.ModelExecutionSelection)
	}
	if source.ProjectID != project || source.TaskID != request.TaskID || source.WorkflowID != request.WorkflowID || source.WorkflowVersion != request.WorkflowVersion || source.AgentNodeID != request.AgentNodeID || source.ToolProfileID != profile.ID || source.ToolProfileVersion != profile.Version || source.ToolProfileSHA256 != profile.Profile.ProfileSHA256 {
		return nil, conflict(modelExecutionChanged)
	}
	if err := s.validateModelExecutionPlanTx(ctx, tx, tenant, project, profile, *source); err != nil {
		return nil, err
	}
	return source.ModelExecution, nil
}
