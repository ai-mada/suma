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

// Result facts do not depend on the creator or source still being current.
// Core's exact live assignment and transport proof remain mandatory. This
// check binds a successful host fact to the original durable composition.
func (s *Service) validateCandidateCompositionResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if err != nil {
		return err
	}
	fact, hasFact := result.WorkspaceEvidence["candidate_composition"]
	if found && r.RepairExecution != nil {
		if hasFact {
			return forbidden()
		}
		return nil
	}
	if !found {
		if hasFact {
			return forbidden()
		}
		return nil
	}
	if result.NodeID != r.Plan.ProducerNodeID {
		if hasFact {
			return forbidden()
		}
		return nil
	}
	if r.Execution == nil || ccmp.ValidateCandidateCompositionExecution(*r.Execution) != nil {
		return compositionError(409, "changed", compositionChanged)
	}
	if !hasFact {
		if !result.OutcomeUnknown && !result.Stopped && result.ExitCode == 0 && result.Error == "" {
			return compositionError(409, "changed", compositionChanged)
		}
		return nil
	}
	var evidence ccmp.CandidateCompositionEvidence
	if json.Unmarshal([]byte(stringify(fact)), &evidence) != nil || evidence.Protocol != ccmp.CandidateCompositionProtocol || evidence.PlanID != r.Plan.ID || evidence.PlanVersion != r.Plan.Version || evidence.SourcesSHA256 != r.Plan.SourcesSHA256 || evidence.OperationID != r.Plan.OperationID || evidence.BaseCommit != r.Plan.AdminBaseCommit {
		return compositionError(409, "changed", compositionChanged)
	}
	if result.OutcomeUnknown {
		return nil
	}
	success := !result.Stopped && result.ExitCode == 0 && result.Error == ""
	if !success {
		if evidence.State == "completed" {
			return compositionError(409, "changed", compositionChanged)
		}
		if evidence.State == "conflicted" && (len(evidence.Conflicts) == 0 || evidence.ResultCommit != "" || evidence.ResultTree != "") {
			return compositionError(409, "changed", compositionChanged)
		}
		return nil
	}
	if evidence.State != "completed" || !cc.ValidCommit(evidence.ResultCommit) || !cc.ValidCommit(evidence.ResultTree) || len(evidence.Conflicts) != 0 || result.WorkspaceEvidence["head_commit"] != evidence.ResultCommit || result.WorkspaceEvidence["base_commit"] != r.Plan.AdminBaseCommit || result.WorkspaceEvidence["clean"] != true {
		return compositionError(409, "changed", compositionChanged)
	}
	expectedHeads := []string{}
	for _, b := range r.Plan.Sources {
		expectedHeads = append(expectedHeads, b.Source.HeadCommit)
	}
	if stringify(evidence.SourceHeads) != stringify(expectedHeads) || len(evidence.Parents) < 1 || len(evidence.Parents) > 2 || len(evidence.MergeCommits) != len(expectedHeads)-1 || evidence.MergeCommits[len(evidence.MergeCommits)-1] != evidence.ResultCommit {
		return compositionError(409, "changed", compositionChanged)
	}
	for _, parent := range evidence.Parents {
		if !cc.ValidCommit(parent) {
			return compositionError(409, "changed", compositionChanged)
		}
	}
	for _, commit := range evidence.MergeCommits {
		if !cc.ValidCommit(commit) {
			return compositionError(409, "changed", compositionChanged)
		}
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT binding FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return compositionError(409, "changed", compositionChanged)
	}
	if err != nil {
		return err
	}
	var b cc.Binding
	if json.Unmarshal([]byte(raw), &b) != nil || !b.ValidSealed() || b.HeadCommit != evidence.ResultCommit || b.BaseCommit != r.Plan.AdminBaseCommit || b.BundleBase() != r.Plan.AdminBaseCommit || b.EvidenceSHA256 != result.WorkspaceEvidence["evidence_sha256"] {
		return compositionError(409, "changed", compositionChanged)
	}
	return nil
}
