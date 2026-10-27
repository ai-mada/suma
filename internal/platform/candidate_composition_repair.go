package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func validCompositionRepairDraft(r CandidateCompositionRepairDraft) bool {
	if !planningText(r.Title, 200) || !planningText(r.Description, 16000) || r.Priority != "low" && r.Priority != "medium" && r.Priority != "high" || !handoffID(r.OwnerID, true) || len(r.AcceptanceCriteria) < 1 || len(r.AcceptanceCriteria) > 16 {
		return false
	}
	for _, v := range r.AcceptanceCriteria {
		if !planningText(v, 1000) {
			return false
		}
	}
	return true
}

func candidateConflictRepairProfile(p tc.Profile) bool {
	return p.Adapter == "suma" && p.Protocol == "suma-jsonl-v1" && p.WorkspaceToolsVersion == "attempt-workspace-v1" && p.ConflictRepairProtocol == ccmp.CandidateConflictRepairProtocol && !p.ReadOnly && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil
}

func buildCandidateCompositionRepairWorkflow(source dag.Workflow, producer, profileID string, profile tc.Profile, task TaskContext) (dag.Workflow, error) {
	var w dag.Workflow
	if json.Unmarshal([]byte(stringify(source)), &w) != nil || !candidateConflictRepairProfile(profile) || task.CodeBaseline != nil {
		return w, compositionError(400, "repair_profile", compositionRepairProfile)
	}
	found, tests := false, 0
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.ID == producer {
			capability, _ := n.Config["capability"].(string)
			if found || n.Type != "agent" && n.Type != "command" || len(n.DependsOn) != 0 || n.Condition != "" || n.Config["candidate_from"] != nil || capability != "" && capability != "command:"+profile.CommandID {
				return w, compositionError(400, "template", compositionTemplate)
			}
			found = true
			n.Type = "agent"
			if n.Config == nil {
				n.Config = map[string]any{}
			}
			if instructions, _ := n.Config["goal"].(string); strings.TrimSpace(instructions) == "" {
				n.Config["goal"] = "Resolve every frozen source conflict in the isolated workspace. Preserve each reviewed source intent, finish all source merges, run the fixed development checks, and retain independent tests and explicit human acceptance."
			}
			n.MaxRetries = 0
		} else {
			if n.Type != "test" || len(n.Command) != 1 || n.Config["candidate_from"] != producer || n.Condition != "" {
				return w, compositionError(400, "template", compositionTemplate)
			}
			tests++
		}
	}
	if !found || tests == 0 {
		return w, compositionError(400, "template", compositionTemplate)
	}
	w, err := buildTaskDispatchWorkflow(w, producer, profileID, profile, task)
	if err != nil {
		return w, err
	}
	for i := range w.Nodes {
		if w.Nodes[i].ID == producer {
			delete(w.Nodes[i].Config, "candidate_composition")
			delete(w.Nodes[i].Config, "code_baseline")
			w.Nodes[i].Config["purpose"] = "candidate_conflict_repair"
		}
	}
	return w, dag.Compile(w)
}

func (s *Service) compositionRepairWorkflowTx(ctx context.Context, auth *authorizedCommand, project string, p CandidateCompositionPlan) (dag.Workflow, ToolProfile, error) {
	profile, err := s.validateToolDispatchTx(ctx, auth.tx, auth, project, p.ToolProfileID, p.ToolProfileVersion, p.ToolProfileSHA256)
	if err != nil {
		return dag.Workflow{}, profile, err
	}
	if !candidateConflictRepairProfile(profile.Profile) {
		return dag.Workflow{}, profile, compositionError(409, "repair_profile", compositionRepairProfile)
	}
	if err = s.checkDispatchProfileRouteTx(ctx, auth.tx, auth.target.tenantID, project, profile, p.ExecutionRoute); err != nil {
		return dag.Workflow{}, profile, err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), auth.target.tenantID, project, p.WorkflowID, p.WorkflowVersion).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return dag.Workflow{}, profile, missing()
	}
	if err != nil {
		return dag.Workflow{}, profile, err
	}
	var template dag.Workflow
	if err = json.Unmarshal([]byte(raw), &template); err != nil {
		return template, profile, err
	}
	w, err := buildCandidateCompositionRepairWorkflow(template, p.ProducerNodeID, p.ToolProfileID, profile.Profile, p.Context)
	return w, profile, err
}

func (s *Service) CreateCandidateCompositionRepair(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req CandidateCompositionRepairCreateRequest) (out CandidateCompositionPlan, err error) {
	if req.Repair.Priority == "" {
		req.Repair.Priority = "medium"
	}
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedCompositionVersion < 1 || !cc.ValidHash(req.ExpectedConflictSHA256) || !validCompositionRepairDraft(req.Repair) || !diagnosisID(req.WorkflowID, 100) || req.WorkflowVersion < 1 || !diagnosisID(req.ProducerNodeID, 150) || !handoffID(req.ToolProfileID, false) || req.ExpectedToolVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"composition_repair": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		r, e := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, out.ID)
		if e != nil {
			return out, e
		}
		return s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
	}
	original, err := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if original.Plan.Version != req.ExpectedCompositionVersion {
		return out, compositionError(409, "changed", compositionChanged)
	}
	basis, err := s.compositionConflictBasisTx(ctx, auth.tx, auth.target.tenantID, project, original)
	if err != nil {
		return out, err
	}
	if basis.SHA256 != req.ExpectedConflictSHA256 {
		return out, compositionError(409, "conflict_changed", compositionConflict)
	}
	existing, err := s.candidateRepairPlanForConflictTx(ctx, auth.tx, auth.target.tenantID, project, basis)
	if err != nil {
		return out, err
	}
	if existing != "" {
		r, e := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, project, existing)
		if e != nil {
			return out, e
		}
		out, e = s.candidateCompositionProjectionTx(ctx, auth, r.Plan)
		if e != nil {
			return out, e
		}
		if e = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); e != nil {
			return out, e
		}
		return out, auth.tx.Commit()
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, original.Plan); err != nil {
		return out, err
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, req.ToolProfileID)
	if err != nil {
		return out, err
	}
	if !candidateConflictRepairProfile(profile.Profile) {
		return out, compositionError(409, "repair_profile", compositionRepairProfile)
	}
	owner := req.Repair.OwnerID
	if owner == "" {
		rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT id,data FROM suma_members WHERE tenant_id=? AND project_id=? AND kind='human' AND status NOT IN ('archived','deleted') ORDER BY id`), auth.target.tenantID, project)
		if e != nil {
			return out, e
		}
		matches := []string{}
		for rows.Next() {
			var member, raw string
			if e = rows.Scan(&member, &raw); e != nil {
				break
			}
			var data map[string]any
			if json.Unmarshal([]byte(raw), &data) == nil && data["user_id"] == auth.currentPrincipal.ID {
				matches = append(matches, member)
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return out, e
		}
		if len(matches) == 1 {
			owner = matches[0]
		}
	}
	criteria := make([]any, len(req.Repair.AcceptanceCriteria))
	for i, v := range req.Repair.AcceptanceCriteria {
		criteria[i] = v
	}
	task, err := s.createCommandRecordTx(ctx, auth.tx, auth, project, "tasks", map[string]any{"title": req.Repair.Title, "description": req.Repair.Description, "priority": req.Repair.Priority, "owner_id": owner, "acceptance_criteria": criteria, "idempotency_key": "composition-repair-task-" + basis.SHA256})
	if err != nil {
		return out, err
	}
	context, err := s.taskContextForDispatchTx(ctx, auth, project, task.ID, nil)
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = CandidateCompositionPlan{ID: newID(), ProjectID: project, Version: 1, State: "ready", TaskID: task.ID, TaskVersion: task.Version, TaskBasisSHA256: digest(stringify(context)), WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, Strategy: "conflict_repair", AdminBaseCommit: original.Plan.AdminBaseCommit, SourcesSHA256: original.Plan.SourcesSHA256, Sources: original.Plan.Sources, Context: context, ConflictBasis: &basis, ToolProfileID: profile.ID, ToolProfileVersion: req.ExpectedToolVersion, ToolProfileSHA256: profile.Profile.ProfileSHA256, CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), FixedTimestamp: clock.UTC().Format(time.RFC3339Nano), Valid: true, Conflicts: []string{}, Blockers: []string{}, AllowedActions: []string{}}
	out.OperationID = "composition-repair-" + out.ID
	out.ExecutionRoute, err = engine.ReadProjectExecutionRouteTx(ctx, auth.tx, auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	out.Workflow, _, err = s.compositionRepairWorkflowTx(ctx, auth, project, out)
	if err != nil {
		return out, err
	}
	r := candidateCompositionRecord{Plan: out, Creator: expertCredentialSnapshot(actor)}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_compositions(id,tenant_id,project_id,task_id,version,state,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, task.ID, 1, "ready", out.CreatedAt, stringify(r)); err != nil {
		return out, err
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_candidate_composition_repair_sources(tenant_id,project_id,composition_id,conflict_sha256,repair_plan_id,task_id)VALUES(?,?,?,?,?,?)`), auth.target.tenantID, project, id, compositionConflictIdentityDigest(basis), out.ID, task.ID); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "candidate.composition_repair_created", out.ID, task.Title); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if out, err = s.candidateCompositionProjectionTx(ctx, auth, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) validateCandidateCompositionRepairTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, p CandidateCompositionPlan) error {
	if p.ConflictBasis == nil || p.State != "ready" || p.RunID != "" {
		return compositionError(409, "changed", compositionChanged)
	}
	if err := s.requireCurrentCandidateRepairTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p); err != nil {
		return err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= p.ExpiresAt {
		return compositionError(409, "expired", compositionExpired)
	}
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.TaskID, p.ID); err != nil {
		return err
	}
	if err = s.checkCompositionConflictBasisTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, *p.ConflictBasis); err != nil {
		return err
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, auth.currentPrincipal, p); err != nil {
		return err
	}
	current, err := s.taskContextForDispatchTx(ctx, auth, p.ProjectID, p.TaskID, nil)
	if err != nil {
		return err
	}
	var run string
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, p.ProjectID, p.TaskID).Scan(&run); err != nil {
		return err
	}
	if digest(stringify(current)) != p.TaskBasisSHA256 {
		return compositionError(409, "changed", compositionChanged)
	}
	lineage, found, err := s.candidateRepairRevisionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.ID)
	if err != nil {
		return err
	}
	if found && lineage.RecoveryBasis != nil {
		previous, e := s.candidateCompositionTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, lineage.RecoveryBasis.PlanID)
		if e != nil {
			return e
		}
		basis, e := s.candidateRepairRecoveryBasisTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, previous)
		if e != nil {
			return e
		}
		if basis.SHA256 != lineage.RecoveryBasis.SHA256 || run != basis.RunID {
			return compositionError(409, "repair_recovery", compositionRepairRecovery)
		}
	} else if run != "" {
		return compositionError(409, "changed", compositionChanged)
	}
	w, _, err := s.compositionRepairWorkflowTx(ctx, auth, p.ProjectID, p)
	if err != nil {
		return err
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, auth.target.tenantID, p.ProjectID)
	if err != nil {
		return err
	}
	if stringify(w) != stringify(p.Workflow) || route != p.ExecutionRoute {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}

// Renewal is an explicit command over the same frozen definition. Only expiry
// is reconsidered; a changed task, source, tool or route requires a future
// governed revision instead of silently adopting new execution inputs.
func (s *Service) refreshedCandidateCompositionRepairTx(ctx context.Context, auth *authorizedCommand, p CandidateCompositionPlan) (CandidateCompositionPlan, error) {
	if p.ConflictBasis == nil || p.State != "ready" || p.RunID != "" {
		return p, compositionError(409, "changed", compositionChanged)
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return p, err
	}
	p.ExpiresAt = automationStamp(clock.Add(30 * time.Minute))
	if err = s.validateCandidateCompositionRepairTx(ctx, nil, auth, p); err != nil {
		return p, err
	}
	return p, nil
}

func (s *Service) bindCandidateCompositionRepairExecutionTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, r *candidateCompositionRecord) error {
	p := r.Plan
	if p.ConflictBasis == nil {
		return compositionError(409, "conflict", compositionConflict)
	}
	original, err := s.candidateCompositionTx(ctx, tx, auth.target.tenantID, p.ProjectID, p.ConflictBasis.CompositionID)
	if err != nil {
		return err
	}
	if original.Execution == nil {
		return compositionError(409, "conflict", compositionConflict)
	}
	instructions := ""
	for _, n := range p.Workflow.Nodes {
		if n.ID == p.ProducerNodeID {
			instructions, _ = n.Config["goal"].(string)
		}
	}
	r.RepairExecution = &ccmp.CandidateConflictRepairExecution{Protocol: ccmp.CandidateConflictRepairProtocol, PlanID: p.ID, PlanVersion: p.Version, ProjectID: p.ProjectID, CompositionID: p.ConflictBasis.CompositionID, CompositionVersion: p.ConflictBasis.CompositionVersion, OriginalAssignment: p.ConflictBasis.Assignment, OriginalEvidenceSHA256: p.ConflictBasis.EvidenceSHA256, Composition: *original.Execution, Conflict: p.ConflictBasis.Evidence, Instructions: instructions, FixedTimestamp: p.FixedTimestamp, OperationID: p.OperationID}
	return ccmp.ValidateCandidateConflictRepairExecution(*r.RepairExecution)
}
