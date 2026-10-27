package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

// A lineage row is immutable. The only movable head is the protected conflict
// reservation; neither its prior plan nor any per-run execution is rewritten.
type candidateRepairRevisionRecord struct {
	PlanID        string                                    `json:"plan_id"`
	TaskID        string                                    `json:"task_id"`
	Lineage       CandidateCompositionRepairRevisionLineage `json:"lineage"`
	Definition    CandidateCompositionPlan                  `json:"definition"`
	RecoveryBasis *CandidateCompositionRepairRecoveryBasis  `json:"recovery_basis,omitempty"`
}

func repairDefinitionDigest(p CandidateCompositionPlan) string {
	return digest(stringify(map[string]any{"id": p.ID, "project_id": p.ProjectID, "task_id": p.TaskID, "task_version": p.TaskVersion, "task_basis": p.TaskBasisSHA256, "context": p.Context, "workflow_id": p.WorkflowID, "workflow_version": p.WorkflowVersion, "producer": p.ProducerNodeID, "workflow": p.Workflow, "model_execution": p.ModelExecution, "route": p.ExecutionRoute, "strategy": p.Strategy, "base": p.AdminBaseCommit, "sources_sha256": p.SourcesSHA256, "sources": p.Sources, "conflict": p.ConflictBasis, "tool_id": p.ToolProfileID, "tool_version": p.ToolProfileVersion, "tool_sha256": p.ToolProfileSHA256, "fixed_timestamp": p.FixedTimestamp, "operation_id": p.OperationID}))
}

func (s *Service) candidateRepairRevisionTx(ctx context.Context, tx governanceQuery, tenant, project, id string) (out candidateRepairRevisionRecord, found bool, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_candidate_composition_repair_revisions WHERE tenant_id=? AND project_id=? AND plan_id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.PlanID != id || out.Definition.ID != id || out.Definition.ProjectID != project || out.TaskID != out.Definition.TaskID || out.Lineage.Ordinal < 1 || out.Lineage.Ordinal > 32 || out.Lineage.DefinitionSHA256 != repairDefinitionDigest(out.Definition) || out.Lineage.RootPlanID == "" || out.Lineage.Ordinal == 1 && (out.Lineage.ParentPlanID != "" || out.Lineage.RootPlanID != id) || out.Lineage.Ordinal > 1 && out.Lineage.ParentPlanID == "" {
		return out, false, compositionError(409, "changed", compositionChanged)
	}
	return out, true, nil
}

func (s *Service) candidateRepairHeadTx(ctx context.Context, tx governanceQuery, tenant, project string, p CandidateCompositionPlan) (head string, err error) {
	if p.ConflictBasis == nil || p.Strategy != "conflict_repair" || conflictBasisDigest(*p.ConflictBasis) != p.ConflictBasis.SHA256 {
		return "", compositionError(409, "changed", compositionChanged)
	}
	var original string
	err = tx.QueryRowContext(ctx, s.q(`SELECT repair_plan_id,composition_id FROM suma_candidate_composition_repair_sources WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, p.TaskID).Scan(&head, &original)
	if errors.Is(err, sql.ErrNoRows) {
		return "", compositionError(409, "changed", compositionChanged)
	}
	if err != nil {
		return "", err
	}
	if original != p.ConflictBasis.CompositionID {
		return "", compositionError(409, "changed", compositionChanged)
	}
	// The lookup key is opaque. The immutable current child's complete basis
	// owns the business identity, as it does for same-conflict Create recovery.
	current, err := s.candidateCompositionTx(ctx, tx, tenant, project, head)
	if err != nil {
		return "", err
	}
	if current.Plan.TaskID != p.TaskID || current.Plan.ConflictBasis == nil || conflictBasisDigest(*current.Plan.ConflictBasis) != current.Plan.ConflictBasis.SHA256 || compositionConflictIdentityDigest(*current.Plan.ConflictBasis) != compositionConflictIdentityDigest(*p.ConflictBasis) {
		return "", compositionError(409, "changed", compositionChanged)
	}
	return head, nil
}

func (s *Service) requireCurrentCandidateRepairTx(ctx context.Context, tx governanceQuery, tenant, project string, p CandidateCompositionPlan) error {
	head, err := s.candidateRepairHeadTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if head != p.ID {
		return compositionError(409, "repair_historical", compositionRepairHistorical)
	}
	return nil
}

func (s *Service) projectCandidateRepairLineageTx(ctx context.Context, auth *authorizedCommand, p *CandidateCompositionPlan) (bool, error) {
	head, err := s.candidateRepairHeadTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, *p)
	if err != nil {
		return false, err
	}
	r, found, err := s.candidateRepairRevisionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.ID)
	if err != nil {
		return false, err
	}
	if found {
		if r.Lineage.DefinitionSHA256 != repairDefinitionDigest(*p) {
			return false, compositionError(409, "changed", compositionChanged)
		}
		lineage := r.Lineage
		lineage.CurrentHeadPlanID, lineage.IsCurrent = head, head == p.ID
		p.RepairRevision = &lineage
	}
	return head == p.ID, nil
}

func repairRecoveryDigest(b CandidateCompositionRepairRecoveryBasis) string {
	b.SHA256 = ""
	return digest(stringify(b))
}

// Certification examines the complete bounded attempt set, not merely the last
// failure. A later startup failure cannot hide an earlier model/workspace run.
func (s *Service) candidateRepairRecoveryBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, r candidateCompositionRecord) (out CandidateCompositionRepairRecoveryBasis, err error) {
	p := r.Plan
	deny := func() (CandidateCompositionRepairRecoveryBasis, error) {
		return out, compositionError(409, "repair_recovery", compositionRepairRecovery)
	}
	if p.RunID == "" || r.RepairExecution == nil || !validCandidateCompositionExecutionRecord(r) {
		return deny()
	}
	out = CandidateCompositionRepairRecoveryBasis{PlanID: p.ID, PlanVersion: p.Version, TaskID: p.TaskID, RunID: p.RunID, Attempts: []CandidateCompositionRepairRecoveryAttempt{}}
	var state, taskRun, taskState, nodeState string
	var nodeFence int64
	var nodeWorker string
	var attempt int
	err = tx.QueryRowContext(ctx, s.q(`SELECT r.version,r.state,n.state,n.attempt,n.fence,n.worker_id,t.version,t.run_id,t.status FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id AND n.id=? JOIN suma_tasks t ON t.tenant_id=r.tenant_id AND t.project_id=r.project_id AND t.id=? WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), p.ProducerNodeID, p.TaskID, tenant, project, p.RunID).Scan(&out.RunVersion, &state, &nodeState, &attempt, &nodeFence, &nodeWorker, &out.TaskVersion, &taskRun, &taskState)
	if err != nil {
		return out, err
	}
	if state != "failed" || nodeState != "failed" || attempt < 1 || attempt > 256 || taskRun != p.RunID || taskState == "accepted" || taskState == "archived" || taskState == "deleted" {
		return deny()
	}
	for _, check := range []struct {
		q    string
		args []any
	}{
		{`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND (state NOT IN ('failed','pending','skipped') OR (id<>? AND attempt>0))`, []any{p.RunID, p.ProducerNodeID}},
		{`SELECT COUNT(*) FROM dag_attempts WHERE run_id=? AND node_id<>?`, []any{p.RunID, p.ProducerNodeID}},
		{`SELECT COUNT(*) FROM dag_compensations WHERE original_run_id=? OR compensation_run_id=?`, []any{p.RunID, p.RunID}},
		{`SELECT COUNT(*) FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=?`, []any{tenant, project, p.RunID}},
		{`SELECT COUNT(*) FROM suma_artifact_uploads WHERE tenant_id=? AND project_id=? AND run_id=?`, []any{tenant, project, p.RunID}},
		{`SELECT COUNT(*) FROM suma_artifact_bindings WHERE tenant_id=? AND project_id=? AND run_id=?`, []any{tenant, project, p.RunID}},
		{`SELECT COUNT(*) FROM suma_artifact_proofs WHERE tenant_id=? AND project_id=? AND run_id=?`, []any{tenant, project, p.RunID}},
	} {
		var count int
		if err = tx.QueryRowContext(ctx, s.q(check.q), check.args...).Scan(&count); err != nil {
			return out, err
		}
		if count != 0 {
			return deny()
		}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT attempt,fence,worker_id,state,result FROM dag_attempts WHERE run_id=? AND node_id=? ORDER BY attempt LIMIT 257`), p.RunID, p.ProducerNodeID)
	if err != nil {
		return out, err
	}
	type stored struct {
		attempt            int
		fence              int64
		worker, state, raw string
	}
	all := []stored{}
	for rows.Next() {
		var a stored
		if err = rows.Scan(&a.attempt, &a.fence, &a.worker, &a.state, &a.raw); err != nil {
			break
		}
		all = append(all, a)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(all) != attempt {
		return deny()
	}
	if all[len(all)-1].fence != nodeFence || all[len(all)-1].worker != nodeWorker {
		return deny()
	}
	for i, a := range all {
		var result dag.Result
		if a.attempt != i+1 || a.state != "failed" || json.Unmarshal([]byte(a.raw), &result) != nil || result.RunID != p.RunID || result.NodeID != p.ProducerNodeID || result.Attempt != a.attempt || result.Fence != a.fence || result.WorkerID != a.worker {
			return deny()
		}
		certified, e := s.CertifiedCandidateRepairStartupFailureTx(ctx, tx, tenant, project, result)
		if e != nil {
			return out, e
		}
		if !certified {
			return deny()
		}
		out.Attempts = append(out.Attempts, CandidateCompositionRepairRecoveryAttempt{Assignment: result.StartupFailure.Assignment, ResultSHA256: digest(a.raw), StartupFailureSHA256: digest(stringify(result.StartupFailure))})
	}
	out.SHA256 = repairRecoveryDigest(out)
	return out, nil
}

func (s *Service) candidateRepairRevisionEligibilityTx(ctx context.Context, auth *authorizedCommand, r candidateCompositionRecord) (out CandidateCompositionRepairRevisionContext, err error) {
	p := r.Plan
	out = CandidateCompositionRepairRevisionContext{ProjectID: p.ProjectID, PlanID: p.ID, PlanVersion: p.Version, Blockers: []string{}}
	if err = s.requireCurrentCandidateRepairTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p); err != nil {
		return out, err
	}
	lineage, found, err := s.candidateRepairRevisionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.ID)
	if err != nil {
		return out, err
	}
	if found && lineage.Lineage.Ordinal >= 32 {
		return out, compositionError(409, "repair_revision_limit", compositionRepairLimit)
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, p); err != nil {
		return out, err
	}
	original, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.ConflictBasis.CompositionID)
	if err != nil {
		return out, err
	}
	basis, err := s.compositionConflictBasisTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, original)
	if err != nil {
		return out, err
	}
	if compositionConflictIdentityDigest(basis) != compositionConflictIdentityDigest(*p.ConflictBasis) {
		return out, compositionError(409, "conflict_changed", compositionConflict)
	}
	out.ConflictBasis = &basis
	var taskRun, status string
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,run_id,status FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, p.ProjectID, p.TaskID).Scan(&out.TaskVersion, &taskRun, &status); err != nil {
		return out, err
	}
	if status == "accepted" || status == "archived" || status == "deleted" {
		return out, compositionError(409, "repair_revision", compositionRepairRevision)
	}
	if p.RunID != "" {
		recovery, e := s.candidateRepairRecoveryBasisTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, r)
		if e != nil {
			return out, e
		}
		out.RecoveryBasis = &recovery
		out.EligibilityMode = "certified_startup_failure"
	} else {
		if p.State != "ready" {
			return out, compositionError(409, "repair_revision", compositionRepairRevision)
		}
		out.EligibilityMode = "ready"
		if found && lineage.RecoveryBasis != nil {
			previous, e := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, lineage.RecoveryBasis.PlanID)
			if e != nil {
				return out, e
			}
			recovery, e := s.candidateRepairRecoveryBasisTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, previous)
			if e != nil {
				return out, e
			}
			if recovery.SHA256 != lineage.RecoveryBasis.SHA256 {
				return out, compositionError(409, "repair_recovery", compositionRepairRecovery)
			}
			out.RecoveryBasis = &recovery
		} else if taskRun != "" {
			return out, compositionError(409, "repair_revision", compositionRepairRevision)
		}
	}
	out.CanRevise = true
	return out, nil
}

func (s *Service) GetCandidateCompositionRepairRevisionContext(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out CandidateCompositionRepairRevisionContext, err error) {
	out = CandidateCompositionRepairRevisionContext{ProjectID: project, PlanID: id, Blockers: []string{}}
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) {
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
	out, e := s.candidateRepairRevisionEligibilityTx(ctx, auth, r)
	if e != nil {
		if !repositoryKnownError(e) {
			return out, e
		}
		out.CanRevise = false
		out.EligibilityMode = ""
		out.Blockers = append(out.Blockers, e.Error())
		return out, nil
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	out.CanRevise = write
	return out, nil
}

func safeRepairCreator(ref expertTeamCredentialReference) (string, string) {
	kind := ""
	if ref.Kind == actorBrowser {
		kind = "browser"
	} else if ref.Kind == actorProjectAPI {
		kind = "project_api"
	}
	return kind, ref.SubjectID
}

func (s *Service) saveCandidateRepairRevisionTx(ctx context.Context, auth *authorizedCommand, r candidateRepairRevisionRecord) error {
	var parent any
	if r.Lineage.ParentPlanID != "" {
		parent = r.Lineage.ParentPlanID
	}
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_composition_repair_revisions(tenant_id,project_id,root_plan_id,plan_id,parent_plan_id,ordinal,task_id,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?)`), auth.target.tenantID, r.Definition.ProjectID, r.Lineage.RootPlanID, r.PlanID, parent, r.Lineage.Ordinal, r.TaskID, r.Lineage.CreatedAt, stringify(r))
	return err
}

func (s *Service) ReviseCandidateCompositionRepair(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req CandidateCompositionRepairRevisionRequest) (out CandidateCompositionPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.ExpectedTaskVersion < 1 || !cc.ValidHash(req.ExpectedConflictSHA256) || !diagnosisID(req.WorkflowID, 100) || req.WorkflowVersion < 1 || !diagnosisID(req.ProducerNodeID, 150) || !handoffID(req.ToolProfileID, false) || req.ExpectedToolVersion < 1 || req.ExpectedRecoverySHA256 != "" && !cc.ValidHash(req.ExpectedRecoverySHA256) || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	if !req.Acknowledge {
		return out, compositionError(400, "ack", compositionAck)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"composition_repair_revision": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	parent, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if parent.Plan.Version != req.ExpectedVersion {
		return out, compositionError(409, "changed", compositionChanged)
	}
	view, err := s.candidateRepairRevisionEligibilityTx(ctx, auth, parent)
	if err != nil {
		return out, err
	}
	if view.TaskVersion != req.ExpectedTaskVersion || view.ConflictBasis == nil || view.ConflictBasis.SHA256 != req.ExpectedConflictSHA256 {
		return out, compositionError(409, "changed", compositionChanged)
	}
	if view.RecoveryBasis == nil && req.ExpectedRecoverySHA256 != "" || view.RecoveryBasis != nil && view.RecoveryBasis.SHA256 != req.ExpectedRecoverySHA256 {
		return out, compositionError(409, "repair_recovery", compositionRepairRecovery)
	}
	taskContext, err := s.taskContextForDispatchTx(ctx, auth, project, parent.Plan.TaskID, nil)
	if err != nil {
		return out, err
	}
	if taskContext.Task.Version != req.ExpectedTaskVersion || taskContext.CodeBaseline != nil {
		return out, compositionError(409, "changed", compositionChanged)
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, req.ToolProfileID)
	if err != nil {
		return out, err
	}
	if !candidateConflictRepairProfile(profile.Profile) {
		return out, compositionError(409, "repair_profile", compositionRepairProfile)
	}
	modelBinding, err := s.buildModelExecutionTx(ctx, auth.tx, auth.target.tenantID, project, profile, req.ModelExecutionSelection)
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = parent.Plan
	out.ID = newID()
	out.Version = 1
	out.State = "ready"
	out.RunID = ""
	out.RunState = ""
	out.TaskState = ""
	out.Candidate = nil
	out.RepairRevision = nil
	out.Context = taskContext
	out.TaskVersion = taskContext.Task.Version
	out.TaskBasisSHA256 = digest(stringify(taskContext))
	out.ConflictBasis = view.ConflictBasis
	out.WorkflowID = req.WorkflowID
	out.WorkflowVersion = req.WorkflowVersion
	out.ProducerNodeID = req.ProducerNodeID
	out.ToolProfileID = req.ToolProfileID
	out.ToolProfileVersion = req.ExpectedToolVersion
	out.ToolProfileSHA256 = profile.Profile.ProfileSHA256
	out.ModelExecution = modelBinding
	out.CreatedAt = automationStamp(clock)
	out.ExpiresAt = automationStamp(clock.Add(30 * time.Minute))
	out.FixedTimestamp = clock.UTC().Format(time.RFC3339Nano)
	out.OperationID = "composition-repair-" + out.ID
	out.Valid = true
	out.Blockers = []string{}
	out.Conflicts = []string{}
	out.AllowedActions = []string{}
	out.ExecutionRoute, err = engine.ReadProjectExecutionRouteTx(ctx, auth.tx, auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	out.Workflow, _, err = s.compositionRepairWorkflowTx(ctx, auth, project, out)
	if err != nil {
		return out, err
	}
	lineage, found, err := s.candidateRepairRevisionTx(ctx, auth.tx, auth.target.tenantID, project, parent.Plan.ID)
	if err != nil {
		return out, err
	}
	if !found {
		kind, principal := safeRepairCreator(parent.Creator)
		lineage = candidateRepairRevisionRecord{PlanID: parent.Plan.ID, TaskID: parent.Plan.TaskID, Definition: parent.Plan, Lineage: CandidateCompositionRepairRevisionLineage{RootPlanID: parent.Plan.ID, Ordinal: 1, Reason: "Initial repair plan observed before explicit revision", CreatedAt: automationStamp(clock), DefinitionSHA256: repairDefinitionDigest(parent.Plan), CreatorKind: kind, CreatorPrincipalID: principal, Observed: true}}
		if err = s.saveCandidateRepairRevisionTx(ctx, auth, lineage); err != nil {
			return out, err
		}
	}
	kind, principal := safeRepairCreator(expertCredentialSnapshot(actor))
	child := candidateRepairRevisionRecord{PlanID: out.ID, TaskID: out.TaskID, Definition: out, RecoveryBasis: view.RecoveryBasis, Lineage: CandidateCompositionRepairRevisionLineage{RootPlanID: lineage.Lineage.RootPlanID, ParentPlanID: parent.Plan.ID, Ordinal: lineage.Lineage.Ordinal + 1, Reason: req.Reason, CreatedAt: out.CreatedAt, DefinitionSHA256: repairDefinitionDigest(out), CreatorKind: kind, CreatorPrincipalID: principal}}
	if err = s.saveCandidateRepairRevisionTx(ctx, auth, child); err != nil {
		return out, err
	}
	r := candidateCompositionRecord{Plan: out, Creator: expertCredentialSnapshot(actor)}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_compositions(id,tenant_id,project_id,task_id,version,state,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.TaskID, out.Version, out.State, out.CreatedAt, stringify(r)); err != nil {
		return out, err
	}
	res, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_candidate_composition_repair_sources SET repair_plan_id=? WHERE tenant_id=? AND project_id=? AND task_id=? AND repair_plan_id=? AND composition_id=?`), out.ID, auth.target.tenantID, project, out.TaskID, parent.Plan.ID, view.ConflictBasis.CompositionID)
	if err != nil {
		return out, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return out, compositionError(409, "changed", compositionChanged)
	}
	if err = s.commandActivity(ctx, auth, "candidate.composition_repair_revised", out.ID, req.Reason); err != nil {
		return out, err
	}
	if out, err = s.candidateCompositionProjectionTx(ctx, auth, out); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
