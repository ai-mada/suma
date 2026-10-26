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

func compositionSources(p CandidateCompositionPlan) []cc.Binding {
	out := make([]cc.Binding, 0, len(p.Sources))
	for _, b := range p.Sources {
		out = append(out, b.Source)
	}
	return out
}
func (s *Service) checkCompositionSourcesTx(ctx context.Context, tx *sql.Tx, p Principal, plan CandidateCompositionPlan) error {
	ctx, _ = compositionAcceptanceContext(ctx)
	for _, b := range plan.Sources {
		current, err := s.repositoryBasisReadTx(ctx, tx, p, plan.ProjectID, b.TaskID, b.SourceNodeID)
		if err != nil {
			return err
		}
		if current != b {
			return compositionError(409, "source_changed", compositionSource)
		}
		baseline, _, err := s.taskCodeBaselineTx(ctx, tx, p.TenantID, plan.ProjectID, b.TaskID)
		if err != nil {
			return err
		}
		if baseline != nil && b.Source.BaseCommit != baseline.Source.HeadCommit {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
	}
	return nil
}
func (s *Service) compositionCreatorTx(ctx context.Context, tx *sql.Tx, tenant, project string, ref expertTeamCredentialReference) (Principal, error) {
	c, err := s.readCredential(ctx, tx, ref.Digest, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return Principal{}, compositionError(409, "authority", compositionAuthority)
		}
		return Principal{}, err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return Principal{}, err
	}
	if c.kind != actorBrowser && c.kind != actorProjectAPI || c.kind != ref.Kind || c.ref != ref.ref() || c.ref.tenantID != tenant || c.kind == actorProjectAPI && c.ref.projectID != project || !c.expires.After(clock) {
		return Principal{}, compositionError(409, "authority", compositionAuthority)
	}
	return s.currentProjectMembershipLockedTx(ctx, tx, tenant, c.ref.subjectID, project, true, false)
}
func (s *Service) checkCandidateCompositionExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, requireCurrentSource bool) error {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	if r.RepairExecution != nil {
		return s.checkCandidateCompositionRepairExecutionTx(ctx, tx, tenant, project, run, r.Plan.ProducerNodeID, requireCurrentSource)
	}
	p, err := s.compositionCreatorTx(ctx, tx, tenant, project, r.Creator)
	if err != nil {
		return err
	}
	if r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
		return compositionError(409, "changed", compositionChanged)
	}
	if requireCurrentSource {
		var taskRun string
		var taskVersion int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT run_id,version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, r.Plan.TaskID).Scan(&taskRun, &taskVersion); err != nil {
			return err
		}
		if taskRun != run || taskVersion != r.Plan.TaskVersion+1 {
			return compositionError(409, "changed", compositionChanged)
		}
		if err = s.checkCompositionSourcesTx(ctx, tx, p, r.Plan); err != nil {
			return err
		}
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	if route != r.Plan.ExecutionRoute || bound != r.Plan.ExecutionRoute {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}

// The HTTP transport proves the current full lease before invoking this helper.
// A transfer ID alone is never an authorization to read another task's code.
func (s *Service) codeInputBindingTx(ctx context.Context, tx *sql.Tx, a *candidateAuthority, req ccmp.CodeInputRequest) (out candidateTransfer, err error) {
	if err = ccmp.ValidateCodeInputRequest(req); err != nil {
		return out, compositionError(400, "invalid", compositionInvalid)
	}
	if a == nil || a.tx != tx || a.assert() != nil || req.ProjectID != a.subject.project || stringify(req.Assignment) != stringify(a.assignment) {
		return out, forbidden()
	}
	var binding cc.Binding
	switch req.InputKind {
	case "composition":
		r, found, e := s.candidateCompositionExecutionRecordTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID)
		if e != nil {
			return out, e
		}
		if !found || r.Execution == nil || r.Plan.ID != req.BindingID || r.Plan.ProducerNodeID != a.assignment.NodeID || a.spec.Type != "command" || len(a.spec.Command) != 1 || a.spec.Command[0] != ccmp.CompositionCommand || a.spec.Config["purpose"] != "candidate_composition" {
			return out, forbidden()
		}
		if e = s.checkCandidateCompositionExecutionTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID, true); e != nil {
			return out, e
		}
		for _, source := range r.Execution.Sources {
			if source.TransferID == req.TransferID {
				binding = source
			}
		}
	case "code_baseline":
		// Only the platform's immutable run/node baseline record may grant this mode.
		return s.codeBaselineInputBindingTx(ctx, tx, a, req)
	case "conflict_repair":
		return s.conflictRepairInputBindingTx(ctx, tx, a, req)
	default:
		return out, forbidden()
	}
	if !binding.ValidSealed() {
		return out, forbidden()
	}
	out, err = s.candidateTransferTx(ctx, tx, a.subject, binding.TransferID)
	if err != nil {
		return out, err
	}
	if out.binding != binding {
		return out, compositionError(409, "source_changed", compositionSource)
	}
	return out, s.candidateSourceTx(ctx, tx, a.subject, binding)
}
func (s *Service) compositionSnapshotTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, req CandidateCompositionCreateRequest) (out CandidateCompositionPlan, err error) {
	p := auth.currentPrincipal
	project := req.ProjectID
	if err = assertProjectCommand(auth.tx, auth, project); err != nil {
		return out, err
	}
	context, err := s.taskContextForDispatchTx(ctx, auth, project, req.TaskID, nil)
	if err != nil {
		return out, err
	}
	if context.Task.Version != req.ExpectedTaskVersion {
		return out, compositionError(409, "changed", compositionChanged)
	}
	var run string
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, req.TaskID).Scan(&run); err != nil {
		return out, err
	}
	if run != "" {
		return out, compositionError(409, "started", compositionStarted)
	}
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, p.TenantID, project, req.TaskID, ""); err != nil {
		return out, err
	}
	baseline, _, err := s.taskCodeBaselineTx(ctx, auth.tx, p.TenantID, project, req.TaskID)
	if err != nil {
		return out, err
	}
	if baseline != nil {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	out = CandidateCompositionPlan{ProjectID: project, Version: 1, State: "ready", TaskID: req.TaskID, TaskVersion: req.ExpectedTaskVersion, Context: context, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, Strategy: "merge_commit", Sources: []RepositoryIntegrationBasis{}, Conflicts: []string{}, Blockers: []string{}, AllowedActions: []string{}, Valid: true}
	out.TaskBasisSHA256 = digest(stringify(context))
	seen := map[string]bool{}
	seenTransfers := map[string]bool{}
	for _, source := range req.Sources {
		if source.TaskID == req.TaskID || seen[source.TaskID] || source.ExpectedTaskVersion < 1 || !handoffID(source.TaskID, false) || !diagnosisID(source.SourceNodeID, 150) {
			return out, compositionError(400, "invalid", compositionInvalid)
		}
		seen[source.TaskID] = true
		b, e := s.repositoryBasisTx(ctx, auth, source.TaskID, source.SourceNodeID)
		if e != nil {
			return out, e
		}
		if b.TaskVersion != source.ExpectedTaskVersion {
			return out, compositionError(409, "source_changed", compositionSource)
		}
		if seenTransfers[b.Source.TransferID] {
			return out, compositionError(400, "invalid", compositionInvalid)
		}
		seenTransfers[b.Source.TransferID] = true
		if out.AdminBaseCommit == "" {
			out.AdminBaseCommit = b.Source.BundleBase()
		} else if out.AdminBaseCommit != b.Source.BundleBase() {
			return out, compositionError(409, "base", compositionBase)
		}
		out.Sources = append(out.Sources, b)
	}
	out.SourcesSHA256, err = ccmp.CandidateCompositionSourcesDigest(compositionSources(out))
	if err != nil {
		return out, err
	}
	if err = s.checkCompositionSourcesTx(ctx, auth.tx, p, out); err != nil {
		return out, err
	}
	out.ExecutionRoute, err = engine.ReadProjectExecutionRouteTx(ctx, auth.tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), p.TenantID, project, req.WorkflowID, req.WorkflowVersion).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	var w dag.Workflow
	if err = json.Unmarshal([]byte(raw), &w); err != nil {
		return out, err
	}
	out.Workflow, err = buildCandidateCompositionWorkflow(w, req.ProducerNodeID)
	return
}
func buildCandidateCompositionWorkflow(source dag.Workflow, producer string) (w dag.Workflow, err error) {
	if json.Unmarshal([]byte(stringify(source)), &w) != nil {
		return w, compositionError(400, "template", compositionTemplate)
	}
	found, tests := false, 0
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.ID == producer {
			capability, _ := n.Config["capability"].(string)
			// Either root code-producer form is replaced by the same trusted
			// command; the copied independent quality nodes remain unchanged.
			if (n.Type != "agent" && n.Type != "command") || found || len(n.DependsOn) != 0 || n.Condition != "" || capability != "" && capability != "command:"+ccmp.CompositionCommand {
				return w, compositionError(400, "template", compositionTemplate)
			}
			found = true
			n.Type = "command"
			n.Command = []string{ccmp.CompositionCommand}
			n.Config = map[string]any{"purpose": "candidate_composition"}
			n.MaxRetries = 0
			n.Idempotent = false
			n.TimeoutSeconds = 300
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
	return w, nil
}
