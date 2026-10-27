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

func validCandidateCompositionExecutionRecord(r candidateCompositionRecord) bool {
	p := r.Plan
	if p.ConflictBasis == nil {
		e := r.Execution
		return e != nil && r.RepairExecution == nil && ccmp.ValidateCandidateCompositionExecution(*e) == nil && e.PlanID == p.ID && e.PlanVersion == p.Version && e.ProjectID == p.ProjectID && e.BaseCommit == p.AdminBaseCommit && e.SourcesSHA256 == p.SourcesSHA256 && e.OperationID == p.OperationID && e.FixedTimestamp == p.FixedTimestamp && stringify(e.Sources) == stringify(compositionSources(p))
	}
	e := r.RepairExecution
	b := p.ConflictBasis
	return r.Execution == nil && e != nil && ccmp.ValidateCandidateConflictRepairExecution(*e) == nil && e.PlanID == p.ID && e.PlanVersion == p.Version && e.ProjectID == p.ProjectID && e.CompositionID == b.CompositionID && e.CompositionVersion == b.CompositionVersion && e.OriginalAssignment == b.Assignment && e.OriginalEvidenceSHA256 == b.EvidenceSHA256 && stringify(e.Conflict) == stringify(b.Evidence) && e.Composition.SourcesSHA256 == p.SourcesSHA256 && e.Composition.BaseCommit == p.AdminBaseCommit && stringify(e.Composition.Sources) == stringify(compositionSources(p)) && e.FixedTimestamp == p.FixedTimestamp && e.OperationID == p.OperationID && b.SHA256 == conflictBasisDigest(*b)
}

func (s *Service) readCandidateCompositionRepairExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*ccmp.CandidateConflictRepairExecution, error) {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return nil, err
	}
	if r.Plan.ProducerNodeID != node || r.RepairExecution == nil {
		return nil, nil
	}
	if r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
		return nil, compositionError(409, "changed", compositionChanged)
	}
	return r.RepairExecution, nil
}

func (s *Service) checkCandidateCompositionRepairExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, fresh bool) error {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	if r.RepairExecution == nil {
		return nil
	}
	if r.Plan.ProducerNodeID != node {
		return nil
	}
	if r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
		return compositionError(409, "changed", compositionChanged)
	}
	p, err := s.compositionCreatorTx(ctx, tx, tenant, project, r.Creator)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	if bound != r.Plan.ExecutionRoute || route != r.Plan.ExecutionRoute {
		return compositionError(409, "changed", compositionChanged)
	}
	var specRaw, taskRun string
	var version int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT t.run_id,t.version,n.spec FROM suma_tasks t JOIN dag_nodes n ON n.run_id=? AND n.id=? WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), run, node, tenant, project, r.Plan.TaskID).Scan(&taskRun, &version, &specRaw); err != nil {
		return err
	}
	var spec dag.Node
	if taskRun != run || json.Unmarshal([]byte(specRaw), &spec) != nil || spec.Type != "agent" || spec.Config["purpose"] != "candidate_conflict_repair" || spec.Config["candidate_from"] != nil || spec.Config["code_baseline"] != nil || spec.Config["candidate_composition"] != nil || spec.Config["tool_profile_id"] != r.Plan.ToolProfileID || spec.Config["tool_profile_sha256"] != r.Plan.ToolProfileSHA256 {
		return compositionError(409, "changed", compositionChanged)
	}
	if fresh {
		if err = s.requireCurrentCandidateRepairTx(ctx, tx, tenant, project, r.Plan); err != nil {
			return err
		}
		if version != r.Plan.TaskVersion+1 {
			return compositionError(409, "changed", compositionChanged)
		}
		if err = s.checkCompositionConflictBasisTx(ctx, tx, tenant, project, *r.Plan.ConflictBasis); err != nil {
			return err
		}
		if err = s.checkCompositionSourcesTx(ctx, tx, p, r.Plan); err != nil {
			return err
		}
		profile, e := s.toolProfileTx(ctx, tx, p, project, r.Plan.ToolProfileID)
		if e != nil {
			return e
		}
		if profile.Version != r.Plan.ToolProfileVersion || profile.Profile.ProfileSHA256 != r.Plan.ToolProfileSHA256 || !candidateConflictRepairProfile(profile.Profile) {
			return compositionError(409, "repair_profile", compositionRepairProfile)
		}
		profile, e = s.toolProfileObservationTx(ctx, tx, p, project, profile)
		if e != nil {
			return e
		}
		if !profile.Available {
			return compositionError(409, "repair_profile", compositionRepairProfile)
		}
		if err = s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, profile, route); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) conflictRepairInputBindingTx(ctx context.Context, tx *sql.Tx, a *candidateAuthority, req ccmp.CodeInputRequest) (out candidateTransfer, err error) {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID)
	if err != nil {
		return out, err
	}
	if !found || r.RepairExecution == nil || r.Plan.ID != req.BindingID || r.Plan.ProducerNodeID != a.assignment.NodeID || a.spec.Type != "agent" || a.spec.Config["purpose"] != "candidate_conflict_repair" || a.spec.Config["candidate_from"] != nil {
		return out, forbidden()
	}
	if err = s.checkCandidateCompositionRepairExecutionTx(ctx, tx, a.subject.tenant, a.subject.project, a.assignment.RunID, a.assignment.NodeID, true); err != nil {
		return out, err
	}
	var binding cc.Binding
	for _, b := range r.RepairExecution.Composition.Sources {
		if b.TransferID == req.TransferID {
			binding = b
		}
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

// Result validation records the genuine host fact. Current creator/source
// eligibility is deliberately checked by fresh work and Review, not here.
func (s *Service) validateCandidateCompositionRepairResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if err != nil {
		return err
	}
	fact, present := result.WorkspaceEvidence["candidate_conflict_repair"]
	if !found || r.RepairExecution == nil || result.NodeID != r.Plan.ProducerNodeID {
		if present {
			return forbidden()
		}
		return nil
	}
	if !validCandidateCompositionExecutionRecord(r) {
		return compositionError(409, "changed", compositionChanged)
	}
	success := !result.OutcomeUnknown && !result.Stopped && result.ExitCode == 0 && result.Error == ""
	if !present {
		if success {
			return compositionError(409, "changed", compositionChanged)
		}
		return nil
	}
	var evidence ccmp.CandidateConflictRepairEvidence
	if json.Unmarshal([]byte(stringify(fact)), &evidence) != nil || evidence.Protocol != ccmp.CandidateConflictRepairProtocol || evidence.PlanID != r.Plan.ID || evidence.PlanVersion != r.Plan.Version || evidence.CompositionID != r.RepairExecution.CompositionID || evidence.OriginalAssignment != r.RepairExecution.OriginalAssignment || evidence.OriginalEvidenceSHA256 != r.RepairExecution.OriginalEvidenceSHA256 || evidence.SourcesSHA256 != r.Plan.SourcesSHA256 || evidence.OperationID != r.Plan.OperationID {
		return compositionError(409, "changed", compositionChanged)
	}
	if result.OutcomeUnknown {
		return nil
	}
	if !success {
		if evidence.State == "completed" {
			return compositionError(409, "changed", compositionChanged)
		}
		return nil
	}
	if ccmp.ValidateCandidateConflictRepairEvidence(evidence, *r.RepairExecution) != nil || result.WorkspaceEvidence["head_commit"] != evidence.ResultCommit || result.WorkspaceEvidence["base_commit"] != r.Plan.AdminBaseCommit || result.WorkspaceEvidence["clean"] != true {
		return compositionError(409, "changed", compositionChanged)
	}
	var raw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return compositionError(409, "changed", compositionChanged)
		}
		return err
	}
	var b cc.Binding
	if json.Unmarshal([]byte(raw), &b) != nil || !b.ValidSealed() || b.HeadCommit != evidence.ResultCommit || b.BaseCommit != r.Plan.AdminBaseCommit || b.BundleBase() != r.Plan.AdminBaseCommit || b.EvidenceSHA256 != result.WorkspaceEvidence["evidence_sha256"] {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}
