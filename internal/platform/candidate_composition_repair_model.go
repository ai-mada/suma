package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
)

// Repair shares the native deployment, frozen quote, parameters, budget and
// admission contract. Its final workflow already contains the complete fixed
// task context and repair instructions before the binding is validated.
func (s *Service) validateCandidateRepairModelTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, p CandidateCompositionPlan, workflow dag.Workflow) error {
	return s.validateModelExecutionPlanTx(ctx, tx, tenant, project, profile, TaskDispatchPlan{AgentNodeID: p.ProducerNodeID, Workflow: workflow, ModelExecution: p.ModelExecution})
}

func candidateRepairProducer(p CandidateCompositionPlan) *dag.Node {
	for i := range p.Workflow.Nodes {
		if p.Workflow.Nodes[i].ID == p.ProducerNodeID {
			return &p.Workflow.Nodes[i]
		}
	}
	return nil
}

func validCandidateRepairModelDefinition(p CandidateCompositionPlan) bool {
	node := candidateRepairProducer(p)
	if node == nil {
		return false
	}
	binding, err := dag.ModelBinding(*node)
	return err == nil && stringify(binding) == stringify(p.ModelExecution)
}

func candidateRepairModelSelectionMatches(binding *modelcontract.Binding, selection *modelcontract.Selection) bool {
	if binding == nil || selection == nil {
		return binding == nil && selection == nil
	}
	fixed := modelcontract.Selection{DeploymentID: binding.Deployment.ID, DeploymentRevision: binding.Deployment.Revision, DeploymentSHA256: binding.Deployment.SHA256, Parameters: binding.Parameters, Budget: binding.Budget}
	return stringify(fixed) == stringify(selection)
}
