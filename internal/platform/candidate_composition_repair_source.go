package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func conflictBasisDigest(b CandidateCompositionConflictBasis) string {
	b.SHA256 = ""
	return digest(stringify(b))
}

// Identity names the original immutable failed execution. Current task/run
// versions remain in the separate basis digest used for freshness and CAS.
func compositionConflictIdentityDigest(b CandidateCompositionConflictBasis) string {
	return digest(stringify([]any{b.CompositionID, b.CompositionVersion, b.RunID, b.NodeID, b.Attempt, b.Fence, b.WorkerID, b.Assignment, b.ResultSHA256, b.EvidenceSHA256}))
}

func (s *Service) compositionConflictBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, r candidateCompositionRecord) (out CandidateCompositionConflictBasis, err error) {
	p := r.Plan
	if p.RunID == "" || p.ConflictBasis != nil || !validCandidateCompositionExecutionRecord(r) {
		return out, compositionError(409, "conflict", compositionConflict)
	}
	var runState, nodeState, attemptState, resultRaw, attemptWorker string
	var attemptFence int64
	out = CandidateCompositionConflictBasis{CompositionID: p.ID, CompositionVersion: p.Version, TaskID: p.TaskID, RunID: p.RunID, NodeID: p.ProducerNodeID}
	err = tx.QueryRowContext(ctx, s.q(`SELECT r.version,r.state,n.state,n.attempt,n.fence,n.worker_id,a.state,a.fence,a.worker_id,a.result FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), tenant, project, p.RunID, p.ProducerNodeID).Scan(&out.RunVersion, &runState, &nodeState, &out.Attempt, &out.Fence, &out.WorkerID, &attemptState, &attemptFence, &attemptWorker, &resultRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, compositionError(409, "conflict", compositionConflict)
	}
	if err != nil {
		return out, err
	}
	var result dag.Result
	if runState != "failed" || nodeState != "failed" || attemptState != "failed" || attemptFence != out.Fence || attemptWorker != out.WorkerID || json.Unmarshal([]byte(resultRaw), &result) != nil || result.OutcomeUnknown || result.Stopped || result.ExitCode == 0 || result.RunID != p.RunID || result.NodeID != p.ProducerNodeID || result.Attempt != out.Attempt || result.Fence != out.Fence || result.WorkerID != out.WorkerID {
		return out, compositionError(409, "conflict", compositionConflict)
	}
	var unknown, sealed int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND state='reconciling'`), p.RunID).Scan(&unknown); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), tenant, project, p.RunID, p.ProducerNodeID, out.Attempt, out.Fence, out.WorkerID).Scan(&sealed); err != nil {
		return out, err
	}
	var taskRun string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT version,run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, p.TaskID).Scan(&out.TaskVersion, &taskRun); err != nil {
		return out, err
	}
	if unknown != 0 || sealed != 0 || taskRun != p.RunID || json.Unmarshal([]byte(stringify(result.WorkspaceEvidence["candidate_composition"])), &out.Evidence) != nil {
		return out, compositionError(409, "conflict", compositionConflict)
	}
	out.Assignment = cc.AssignmentRef{RunID: p.RunID, NodeID: p.ProducerNodeID, Attempt: out.Attempt, Fence: out.Fence, WorkerID: out.WorkerID}
	out.ResultSHA256 = digest(resultRaw)
	out.EvidenceSHA256 = digest(stringify(out.Evidence))
	probe := ccmp.CandidateConflictRepairExecution{Protocol: ccmp.CandidateConflictRepairProtocol, PlanID: "conflict-eligibility", PlanVersion: 1, ProjectID: project, CompositionID: p.ID, CompositionVersion: p.Version, OriginalAssignment: out.Assignment, OriginalEvidenceSHA256: out.EvidenceSHA256, Composition: *r.Execution, Conflict: out.Evidence, Instructions: "Validate complete original conflict evidence", FixedTimestamp: p.FixedTimestamp, OperationID: "conflict-eligibility"}
	if ccmp.ValidateCandidateConflictRepairExecution(probe) != nil {
		return out, compositionError(409, "conflict", compositionConflict)
	}
	out.SHA256 = conflictBasisDigest(out)
	return out, nil
}

func (s *Service) checkCompositionConflictBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, b CandidateCompositionConflictBasis) error {
	r, err := s.candidateCompositionTx(ctx, tx, tenant, project, b.CompositionID)
	if err != nil {
		return err
	}
	current, err := s.compositionConflictBasisTx(ctx, tx, tenant, project, r)
	if err != nil {
		return err
	}
	if current.SHA256 != b.SHA256 || conflictBasisDigest(b) != b.SHA256 {
		return compositionError(409, "conflict_changed", compositionConflict)
	}
	return nil
}

func (s *Service) candidateRepairPlanForConflictTx(ctx context.Context, tx *sql.Tx, tenant, project string, basis CandidateCompositionConflictBasis) (string, error) {
	// The protected child basis is authoritative; a lookup key never changes
	// which failed assignment/result owns a repair. Bound retained links and
	// fail closed on corruption instead of creating another task.
	rows, err := tx.QueryContext(ctx, s.q(`SELECT c.repair_plan_id,c.task_id,p.payload FROM suma_candidate_composition_repair_sources c LEFT JOIN suma_candidate_compositions p ON p.tenant_id=c.tenant_id AND p.project_id=c.project_id AND p.id=c.repair_plan_id WHERE c.tenant_id=? AND c.project_id=? AND c.composition_id=? ORDER BY p.created_at,c.repair_plan_id LIMIT 18`), tenant, project, basis.CompositionID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	identity, out, count := compositionConflictIdentityDigest(basis), "", 0
	for rows.Next() {
		count++
		if count > 17 {
			return "", compositionError(409, "changed", compositionChanged)
		}
		var plan, task string
		var raw *string
		if err = rows.Scan(&plan, &task, &raw); err != nil {
			return "", err
		}
		var child candidateCompositionRecord
		if raw == nil || json.Unmarshal([]byte(*raw), &child) != nil || child.Plan.ID != plan || child.Plan.TaskID != task || child.Plan.ProjectID != project || child.Plan.Version < 1 || child.Plan.ConflictBasis == nil || child.Plan.ConflictBasis.CompositionID != basis.CompositionID || conflictBasisDigest(*child.Plan.ConflictBasis) != child.Plan.ConflictBasis.SHA256 {
			return "", compositionError(409, "changed", compositionChanged)
		}
		if compositionConflictIdentityDigest(*child.Plan.ConflictBasis) == identity {
			if out != "" && out != plan {
				return "", compositionError(409, "changed", compositionChanged)
			}
			out = plan
		}
	}
	return out, rows.Err()
}

func (s *Service) rejectReservedCandidateRepairTaskTx(ctx context.Context, tx governanceQuery, tenant, project, task, allowedPlan string) error {
	var plan string
	err := tx.QueryRowContext(ctx, s.q(`SELECT repair_plan_id FROM suma_candidate_composition_repair_sources WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if plan != allowedPlan {
		return compositionError(409, "repair_task", compositionRepairTask)
	}
	return nil
}

// Every business run-binding path passes this gate, including older AttachRun
// callers. Only the exact protected repair execution inserted by Start grants
// its reserved task; generic records and workflow configuration cannot do so.
func (s *Service) checkReservedCandidateRepairRunTx(ctx context.Context, tx *sql.Tx, tenant, project, task, run string) error {
	var plan string
	err := tx.QueryRowContext(ctx, s.q(`SELECT repair_plan_id FROM suma_candidate_composition_repair_sources WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil {
		return err
	}
	if !found || r.Plan.ID != plan || r.Plan.TaskID != task || r.Plan.RunID != run || r.RepairExecution == nil || !validCandidateCompositionExecutionRecord(r) {
		return compositionError(409, "repair_task", compositionRepairTask)
	}
	return nil
}

func (s *Service) GetCandidateCompositionConflict(ctx context.Context, actor Actor, project, id string) (out CandidateCompositionConflictContext, err error) {
	out = CandidateCompositionConflictContext{ProjectID: project, CompositionID: id, Blockers: []string{}}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out.CompositionVersion = r.Plan.Version
	basis, e := s.compositionConflictBasisTx(ctx, auth.tx, auth.target.tenantID, project, r)
	if e != nil {
		if !repositoryKnownError(e) {
			return out, e
		}
		out.Blockers = append(out.Blockers, compositionConflict)
		return out, nil
	}
	out.Basis = &basis
	out.RepairPlanID, err = s.candidateRepairPlanForConflictTx(ctx, auth.tx, auth.target.tenantID, project, basis)
	if err != nil {
		return out, err
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	out.CanCreate = write && out.RepairPlanID == ""
	if e = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, r.Plan); e != nil {
		if !repositoryKnownError(e) {
			return out, e
		}
		out.CanCreate = false
		out.Blockers = append(out.Blockers, compositionSource)
	}
	return out, nil
}
