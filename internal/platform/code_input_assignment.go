package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	codex "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
)

func codeInputAssignmentRef(a dag.Assignment) cc.AssignmentRef {
	if a.PoolID == "" {
		return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	}
	return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}

// Only platform-bound nodes can advertise a protected input mode. An
// ordinary published command/config cannot acquire cross-run code access.
func (s *Service) codeInputNodeEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, spec dag.Node) (bool, error) {
	if _, err := s.executionSkillsForNodeTx(ctx, tx, tenant, project, run, node, spec); err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Status >= 400 && api.Status < 500 {
			return false, nil
		}
		return false, err
	}
	if eligible, err := s.workspaceTerminalCandidateNodeEligibleTx(ctx, tx, tenant, project, run, node, spec); err != nil || !eligible {
		return eligible, err
	}
	if eligible, err := s.repositorySourceNodeEligibleTx(ctx, tx, tenant, project, run, node, spec); err != nil || !eligible {
		return eligible, err
	}
	declaredComposition := spec.Config["purpose"] == "candidate_composition" || len(spec.Command) == 1 && spec.Command[0] == ccmp.CompositionCommand
	declaredBaseline := spec.Config["code_baseline"] != nil
	declaredRepair := spec.Config["purpose"] == "candidate_conflict_repair" || spec.Config["candidate_conflict_repair"] != nil
	_, continuationIDDeclared := spec.Config["development_continuation_id"]
	_, continuationValueDeclared := spec.Config["development_continuation"]
	declaredContinuation := continuationIDDeclared || continuationValueDeclared
	if !declaredComposition && !declaredBaseline && !declaredRepair && !declaredContinuation {
		return true, nil
	}
	composition, err := s.readCandidateCompositionExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return false, err
	}
	baseline, err := s.readCodeBaselineExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return false, err
	}
	repair, err := s.readCandidateCompositionRepairExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return false, err
	}
	continuation, err := s.readDevelopmentContinuationExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return false, err
	}
	modes := 0
	for _, present := range []bool{composition != nil, baseline != nil, repair != nil, continuation != nil} {
		if present {
			modes++
		}
	}
	if declaredComposition != (composition != nil) || declaredBaseline != (baseline != nil) || declaredRepair != (repair != nil) || declaredContinuation != (continuation != nil) || modes > 1 {
		return false, nil
	}
	if composition != nil && (spec.Type != "command" || len(spec.Command) != 1 || spec.Command[0] != ccmp.CompositionCommand || spec.Config["candidate_from"] != nil) {
		return false, nil
	}
	if baseline != nil {
		eligible, e := s.codeBaselineNodeKindTx(ctx, tx, tenant, project, run, node, spec, *baseline)
		if e != nil || !eligible {
			return eligible, e
		}
	}
	if repair != nil && (spec.Type != "agent" || spec.Config["purpose"] != "candidate_conflict_repair" || spec.Config["candidate_from"] != nil) {
		return false, nil
	}
	if continuation != nil && !developmentContinuationNode(spec, continuation.ID) {
		return false, nil
	}
	return true, nil
}

func sameCodeBaselineValue(value any, expected ccmp.CodeBaselineBinding) bool {
	var actual ccmp.CodeBaselineBinding
	return json.Unmarshal([]byte(stringify(value)), &actual) == nil && actual == expected
}

func (s *Service) checkCodeInputExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, fresh bool) error {
	source, err := s.repositorySourceExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return err
	}
	if source != nil && fresh {
		if err = s.currentRepositorySourceTaskTx(ctx, tx, tenant, project, run, *source); err != nil {
			return err
		}
	}
	if err := s.checkWorkspaceTerminalCandidateExecutionTx(ctx, tx, tenant, project, run, node, fresh); err != nil {
		return err
	}
	if err := s.checkCandidateCompositionExecutionTx(ctx, tx, tenant, project, run, fresh); err != nil {
		return err
	}
	if err := s.checkCodeBaselineExecutionTx(ctx, tx, tenant, project, run, node, fresh); err != nil {
		return err
	}
	return s.checkDevelopmentContinuationExecutionTx(ctx, tx, tenant, project, run, node, fresh)
}

// Only a durable baseline can distinguish a selected development base from
// the bundle's administrator prerequisite; a manifest cannot grant inheritance.
func (s *Service) validateCandidateInputAnchorTx(ctx context.Context, tx *sql.Tx, tenant, project string, request cc.StartRequest) error {
	source, err := s.repositorySourceExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID)
	if err != nil {
		return err
	}
	if source != nil {
		if request.BaseCommit != source.BaseCommit || request.BundleBase() != source.BaseCommit || request.HeadCommit != source.HeadCommit ||
			request.BundleSHA256 != source.BundleSHA256 || request.SizeBytes != source.SizeBytes {
			return conflict("Candidate transfer differs from the approved repository source")
		}
		return s.currentRepositorySourceTaskTx(ctx, tx, tenant, project, request.Assignment.RunID, *source)
	}
	continuation, err := s.readDevelopmentContinuationExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID)
	if err != nil {
		return err
	}
	if continuation != nil {
		if err = s.checkDevelopmentContinuationExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID, true); err != nil {
			return err
		}
		if request.BaseCommit != continuation.Source.HeadCommit || request.BundleBase() != continuation.Source.BundleBase() {
			return continuationError()
		}
		return nil
	}
	baseline, err := s.readCodeBaselineExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID)
	if err != nil {
		return err
	}
	if baseline != nil {
		if request.BaseCommit != baseline.Source.HeadCommit || request.BundleBase() != baseline.Source.BundleBase() {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
		return nil
	}
	composition, err := s.readCandidateCompositionExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID)
	if err != nil {
		return err
	}
	if composition != nil && (request.BaseCommit != composition.BaseCommit || request.BundleBase() != composition.BaseCommit) {
		return compositionError(409, "changed", compositionChanged)
	}
	repair, err := s.readCandidateCompositionRepairExecutionTx(ctx, tx, tenant, project, request.Assignment.RunID, request.Assignment.NodeID)
	if err != nil {
		return err
	}
	if repair != nil && (request.BaseCommit != repair.Composition.BaseCommit || request.BundleBase() != repair.Composition.BaseCommit) {
		return compositionError(409, "changed", compositionChanged)
	}
	if request.BundleBase() != request.BaseCommit {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	return nil
}

// Decoration happens after the durable claim; recovery uses the same claim
// operation. No new run is created when this current-authority check fails.
func (s *Service) decorateCodeInputAssignment(ctx context.Context, subject candidateSubject, a *dag.Assignment) error {
	if a == nil {
		return nil
	}
	a.ExecutionSkills = nil
	a.DevelopmentContinuation = nil
	_, skillsDeclared := a.Config["execution_skills"]
	if !skillsDeclared && a.Config["purpose"] != "candidate_composition" && a.Config["purpose"] != "candidate_conflict_repair" && a.Config["code_baseline"] == nil && a.Config["development_continuation_id"] == nil && a.Config["development_continuation"] == nil {
		return nil
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	authority, err := s.candidateAuthorityTx(ctx, tx, subject, codeInputAssignmentRef(*a))
	if err != nil {
		return err
	}
	eligible, err := s.codeInputNodeEligibleTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID, authority.spec)
	if err != nil {
		return err
	}
	if !eligible {
		return forbidden()
	}
	if stringify(a.Config["execution_skills"]) != stringify(authority.spec.Config["execution_skills"]) {
		return forbidden()
	}
	a.ExecutionSkills, err = codex.DecodeExecutionSkillBundle(authority.spec.Config["execution_skills"])
	if err != nil {
		return invalid(codexSkillsInvalid)
	}
	if err = s.checkCandidateCompositionExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, true); err != nil {
		return err
	}
	if err = s.checkCodeBaselineExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID, true); err != nil {
		return err
	}
	if err = s.checkDevelopmentContinuationExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID, true); err != nil {
		return err
	}
	a.CandidateComposition, err = s.readCandidateCompositionExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	a.CandidateConflictRepair, err = s.readCandidateCompositionRepairExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	a.CodeBaseline, err = s.readCodeBaselineExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	a.DevelopmentContinuation, err = s.readDevelopmentContinuationExecutionTx(ctx, tx, subject.tenant, subject.project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	if a.DevelopmentContinuation != nil && !developmentContinuationNode(dag.Node{Type: a.Type, Config: a.Config}, a.DevelopmentContinuation.ID) {
		return forbidden()
	}
	return tx.Commit()
}

// Genuine completion binds the received workspace to its frozen baseline, not
// to a new current source. Revocation/source drift does not erase completed facts.
func (s *Service) validateCodeBaselineResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	if err := s.validateDevelopmentContinuationResultTx(ctx, tx, tenant, project, result); err != nil {
		return err
	}
	baseline, err := s.readCodeBaselineExecutionTx(ctx, tx, tenant, project, result.RunID, result.NodeID)
	if err != nil {
		return err
	}
	fact, present := result.WorkspaceEvidence["code_baseline"]
	if baseline == nil {
		if present {
			return forbidden()
		}
		return nil
	}
	// A terminal consumes the frozen baseline but does not produce formal code
	// evidence. Its original stop/effect guard remains the completion authority.
	if r, found, e := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, result.RunID); e != nil {
		return e
	} else if found {
		if result.NodeID != "terminal" || r.Session.CodeBaseline == nil || *r.Session.CodeBaseline != *baseline || result.WorkspaceEvidence != nil {
			return forbidden()
		}
		return nil
	}
	if present {
		var actual ccmp.CodeBaselineBinding
		if json.Unmarshal([]byte(stringify(fact)), &actual) != nil || actual != *baseline {
			return compositionError(409, "baseline_changed", compositionBaseline)
		}
	}
	if result.OutcomeUnknown || result.Stopped || result.ExitCode != 0 || result.Error != "" {
		return nil
	}
	proof := result.WorkspaceEvidence
	if !present || proof["base_commit"] != baseline.Source.HeadCommit || proof["admin_base_commit"] != baseline.Source.BundleBase() || proof["state"] != "completed" || proof["clean"] != true || proof["outcome_unknown"] != false {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	var reported cc.Binding
	if json.Unmarshal([]byte(stringify(proof["candidate_bundle"])), &reported) != nil || !reported.ValidSealed() || reported.HeadCommit != proof["head_commit"] || reported.BaseCommit != baseline.Source.HeadCommit || reported.BundleBase() != baseline.Source.BundleBase() || reported.EvidenceSHA256 != proof["evidence_sha256"] || reported.Source.RunID != result.RunID || reported.Source.NodeID != result.NodeID || reported.Source.Attempt != result.Attempt || reported.Source.Fence != result.Fence || reported.Source.WorkerID != result.WorkerID {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	stored, err := s.candidateTransferTx(ctx, tx, candidateSubject{tenant: tenant, project: project}, reported.TransferID)
	if err != nil {
		return err
	}
	if stored.binding != reported {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	return nil
}
