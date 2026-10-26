package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func repositoryKnownObservation(o ri.RepositoryObservation) bool {
	return o.Phase == "prepare" && (o.State == "prepared" || o.State == "blocked") || o.Phase == "apply" && (o.State == "applied" || o.State == "not_applied") && o.Receipt != nil
}

func (s *Service) validateRepositoryObservationTx(ctx context.Context, tx *sql.Tx, cfg RepositoryProviderConfig, op repositoryOperation, o ri.RepositoryObservation) error {
	if op.Execution.ProviderID != cfg.ID || op.Assignment == nil || op.RequestSHA256 == "" || !validReleaseTime(o.ObservedAt) || len(o.Reason) > 2000 || !utf8.ValidString(o.Reason) || ri.RepositoryValidateObservation(o, op.Execution, op.RequestSHA256) != nil {
		return repositoryConflict("observation", repositoryObservation)
	}
	if o.Preview != nil && (!utf8.ValidString(o.Preview.Diff) || len(o.Preview.Diff) > ri.RepositoryMaxDiffBytes) {
		return repositoryConflict("observation", repositoryObservation)
	}
	if o.State != "applied" && o.State != "not_applied" {
		var authorized int
		if o.Phase == "prepare" && o.State == "blocked" && o.EffectGrantID != "" {
			if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_reconciliation_grants WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND state='consumed' AND id=?`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.RequestSHA256, o.EffectGrantID).Scan(&authorized); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND state='consumed'`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.RequestSHA256).Scan(&authorized); err != nil {
				return err
			}
		}
		if authorized != 1 {
			return repositoryConflict("observation", repositoryObservation)
		}
	}
	if o.State == "prepared" && o.Phase != "prepare" {
		return repositoryConflict("observation", repositoryObservation)
	}
	if o.State == "applied" || o.State == "not_applied" {
		var count int
		if o.State == "applied" {
			if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND state='consumed' AND effect_grant_id=?`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.RequestSHA256, o.EffectGrantID).Scan(&count); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_reconciliation_grants WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND state='consumed' AND id=?`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.RequestSHA256, o.EffectGrantID).Scan(&count); err != nil {
				return err
			}
		}
		if count != 1 {
			return repositoryConflict("observation", repositoryObservation)
		}
	}
	return nil
}
func (s *Service) storeRepositoryObservationTx(ctx context.Context, tx *sql.Tx, cfg RepositoryProviderConfig, o ri.RepositoryObservation) (op repositoryOperation, err error) {
	op, err = s.repositoryOperationTx(ctx, tx, cfg.TenantID, cfg.ProjectID, o.OperationID)
	if err != nil {
		return op, err
	}
	if err = s.validateRepositoryObservationTx(ctx, tx, cfg, op, o); err != nil {
		return op, err
	}
	var existing string
	err = tx.QueryRowContext(ctx, s.q(`SELECT sha256 FROM suma_repository_observations WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=?`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.Version).Scan(&existing)
	if err == nil {
		if existing != o.ObservationSHA256 {
			return op, repositoryConflict("observation", repositoryObservation)
		}
		return op, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return op, err
	}
	if op.Observation != nil && o.Version > op.Observation.Version && repositoryKnownObservation(*op.Observation) {
		if !repositoryKnownObservation(o) || o.State != op.Observation.State || o.ReceiptSHA256 != op.Observation.ReceiptSHA256 || o.Preview != nil && op.Observation.Preview != nil && o.Preview.PreparedSHA256 != op.Observation.Preview.PreparedSHA256 {
			return op, repositoryConflict("observation", repositoryObservation)
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_observations(tenant_id,project_id,operation_id,version,sha256,data)VALUES(?,?,?,?,?,?)`), cfg.TenantID, cfg.ProjectID, o.OperationID, o.Version, o.ObservationSHA256, stringify(o)); err != nil {
		return op, err
	}
	if op.Observation == nil || o.Version > op.Observation.Version {
		old := op.Version
		op.Version++
		op.Observation = &o
		if err = s.saveRepositoryOperationTx(ctx, tx, cfg.TenantID, cfg.ProjectID, old, op); err != nil {
			return op, err
		}
		plan, e := s.repositoryPlanTx(ctx, tx, cfg.TenantID, cfg.ProjectID, op.Execution.PlanID)
		if e != nil {
			return op, e
		}
		if plan.OperationID == o.OperationID {
			old := plan.Version
			plan.Version++
			plan.Observation = &o
			if e = s.saveRepositoryPlanTx(ctx, tx, cfg.TenantID, old, plan); e != nil {
				return op, e
			}
		}
	}
	return op, nil
}
func (s *Service) RecordRepositoryObservation(ctx context.Context, token string, o ri.RepositoryObservation) (out RepositoryIntegrationPlan, err error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, cfg.TenantID, cfg.ProjectID); err != nil {
		return out, err
	}
	op, err := s.storeRepositoryObservationTx(ctx, tx, cfg, o)
	if err != nil {
		return out, err
	}
	out, err = s.repositoryPlanTx(ctx, tx, cfg.TenantID, cfg.ProjectID, op.Execution.PlanID)
	if err != nil {
		return out, err
	}
	out.CanApprove, out.CanExecute, out.CanReconcile = false, false, false
	return out, tx.Commit()
}

func (s *Service) repositoryResultObservationTx(ctx context.Context, tx *sql.Tx, tenant, project string, op repositoryOperation, result dag.Result) (out *ri.RepositoryObservation, err error) {
	if op.Assignment != nil && (op.Assignment.RunID != result.RunID || op.Assignment.NodeID != result.NodeID || op.Assignment.Attempt != result.Attempt || op.Assignment.Fence != result.Fence || op.Assignment.WorkerID != result.WorkerID) {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	if result.WorkspaceEvidence != nil || result.ToolEvidence != nil || result.ArtifactEvidence != nil || result.ReleaseEvidence != nil {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	e := result.IntegrationEvidence
	if e == nil {
		if result.OutcomeUnknown || op.Execution.Phase == "prepare" && result.ExitCode != 0 {
			return nil, nil
		}
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	if e.OperationID != op.Execution.OperationID || e.RequestSHA256 != op.RequestSHA256 || e.Version < 1 {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT data FROM suma_repository_observations WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=? AND sha256=?`), tenant, project, e.OperationID, e.Version, e.ObservationSHA256).Scan(&raw)
	if err != nil {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	var o ri.RepositoryObservation
	if json.Unmarshal([]byte(raw), &o) != nil {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	if ri.RepositoryValidateObservation(o, op.Execution, op.RequestSHA256) != nil {
		return nil, repositoryConflict("observation", repositoryObservation)
	}
	if result.OutcomeUnknown {
		if repositoryKnownObservation(o) {
			return nil, repositoryConflict("observation", repositoryObservation)
		}
	} else {
		success := o.State == "prepared" || o.State == "applied"
		if !repositoryKnownObservation(o) || success && (result.ExitCode != 0 || result.Stopped || result.Error != "") || !success && result.ExitCode == 0 {
			return nil, repositoryConflict("observation", repositoryObservation)
		}
	}
	return &o, nil
}
func (s *Service) repositoryOperationForResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) (out repositoryOperation, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_repository_operations WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), tenant, project, result.RunID, result.NodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		if result.IntegrationEvidence != nil {
			return out, false, forbidden()
		}
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out, err = s.repositoryOperationTx(ctx, tx, tenant, project, id)
	return out, true, err
}
func (s *Service) validateRepositoryIntegrationResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	op, found, err := s.repositoryOperationForResultTx(ctx, tx, tenant, project, result)
	if err != nil || !found {
		return err
	}
	_, err = s.repositoryResultObservationTx(ctx, tx, tenant, project, op, result)
	return err
}
func (s *Service) recordRepositoryIntegrationOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	op, found, err := s.repositoryOperationForResultTx(ctx, tx, tenant, project, result)
	if err != nil || !found {
		return err
	}
	o, err := s.repositoryResultObservationTx(ctx, tx, tenant, project, op, result)
	if err != nil {
		return err
	}
	plan, err := s.repositoryPlanTx(ctx, tx, tenant, project, op.Execution.PlanID)
	if err != nil {
		return err
	}
	if plan.OperationID != op.Execution.OperationID {
		return nil
	}
	old := plan.Version
	if result.OutcomeUnknown || op.Execution.Phase == "apply" && o == nil {
		plan.State = "reconciling"
	} else if op.Execution.Phase == "prepare" {
		if o != nil && o.State == "prepared" {
			plan.State = "ready"
			plan.Preview = o.Preview
			plan.PreparedSHA256 = o.Preview.PreparedSHA256
		} else {
			plan.State = "blocked"
		}
	} else if o.State == "applied" {
		plan.State = "integrated"
	} else {
		plan.State = "blocked"
	}
	if o != nil {
		plan.Observation = o
	}
	plan.CanApprove, plan.CanExecute, plan.CanReconcile = false, false, false
	if op.Execution.Phase == "apply" && o != nil && repositoryKnownObservation(*o) {
		target, e := s.repositoryTargetTx(ctx, tx, tenant, project, op.Execution.TargetID)
		if e != nil {
			return e
		}
		if target.ActiveOperationID == op.Execution.OperationID && target.Generation == op.Execution.Generation {
			version := target.Version
			target.Version++
			target.ActiveOperationID = ""
			target.Observation = nil
			if e = s.saveRepositoryTargetTx(ctx, tx, tenant, version, target); e != nil {
				return e
			}
		}
	}
	plan.Version++
	return s.saveRepositoryPlanTx(ctx, tx, tenant, old, plan)
}
