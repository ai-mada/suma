package platform

import (
	"context"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// Claim has already committed its durable assignment. This short, current
// machine-authority transaction adds the typed envelope and stores that exact
// identity for recovery before the runner can request a provider ticket. A
// failed decoration is retried through the original claim operation.
func (s *Service) decorateRepositoryAssignment(ctx context.Context, actor Actor, project string, a *dag.Assignment) error {
	if a == nil || a.Type != "external_workflow" {
		return nil
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeRunnerWrite)
	if err != nil {
		return err
	}
	if err = s.projectRunnerExecutionAuthorityTx(ctx, tx, auth, a.WorkerID, a.RunID); err != nil {
		return err
	}
	if err = s.bindRepositoryAssignmentTx(ctx, tx, auth.currentPrincipal.TenantID, project, *a); err != nil {
		return err
	}
	a.RepositoryIntegration, err = s.readRepositoryExecutionTx(ctx, tx, auth.currentPrincipal.TenantID, project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) decoratePoolRepositoryAssignment(ctx context.Context, token string, a *dag.Assignment) error {
	if a == nil || a.Type != "external_workflow" {
		return nil
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	if _, err = s.PoolRunnerAssignmentAuthorityTx(ctx, tx, token, *a); err != nil {
		return err
	}
	if err = s.bindRepositoryAssignmentTx(ctx, tx, a.TenantID, a.ProjectID, *a); err != nil {
		return err
	}
	a.RepositoryIntegration, err = s.readRepositoryExecutionTx(ctx, tx, a.TenantID, a.ProjectID, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// The pool transport wrapper must agree with the complete original assignment;
// it never supplies a project principal or a new execution capability.
type PoolRepositoryAuthorizationRequest struct {
	ri.RepositoryAuthorizationRequest
	TenantID string `json:"tenant_id"`
	RunnerID string `json:"runner_id"`
}
