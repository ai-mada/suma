package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type issueRepairNodeResult struct {
	ID, State, Worker string
	Attempt           int
	Fence             int64
	Spec              dag.Node
	Result            dag.Result
}

// Read the result for the current node attempt/fence/worker only. A report or
// editable issue field cannot substitute an earlier producer or test attempt.
func (s *Service) issueRepairNodeResultTx(ctx context.Context, auth *authorizedCommand, project, run, node string) (out issueRepairNodeResult, found bool, err error) {
	var spec, result string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT n.id,n.state,n.attempt,n.fence,n.worker_id,n.spec,COALESCE(a.result,'') FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id LEFT JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), auth.target.tenantID, project, run, node).Scan(&out.ID, &out.State, &out.Attempt, &out.Fence, &out.Worker, &spec, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if err = json.Unmarshal([]byte(spec), &out.Spec); err != nil {
		return out, false, err
	}
	if result == "" {
		return out, false, nil
	}
	if err = json.Unmarshal([]byte(result), &out.Result); err != nil {
		return out, false, err
	}
	return out, true, nil
}

func issueRepairResultIdentity(n issueRepairNodeResult, run string) bool {
	r := n.Result
	return n.State == "succeeded" && n.Attempt > 0 && n.Fence > 0 && n.Worker != "" && r.RunID == run && r.NodeID == n.ID && r.Attempt == n.Attempt && r.Fence == n.Fence && r.WorkerID == n.Worker && r.ExitCode == 0 && r.Error == "" && !r.Stopped && !r.OutcomeUnknown
}

func issueRepairProofInteger(v any, expected int64) bool {
	switch n := v.(type) {
	case int:
		return int64(n) == expected
	case int64:
		return n == expected
	case float64:
		return n == float64(expected)
	}
	return false
}

// Issue closure is narrower than generic test-only task acceptance: every
// selected regression report must verify a real repair producer through the
// scheduler's immutable candidate pin, on an independent execution identity.
// Complex/test_from sources remain blocked until they have equivalent proof.
func (s *Service) issueRepairCaseCandidateTx(ctx context.Context, auth *authorizedCommand, project, run string, evidence TestCaseEvidence) (bool, error) {
	test, found, err := s.issueRepairNodeResultTx(ctx, auth, project, run, evidence.NodeID)
	if err != nil || !found {
		return false, err
	}
	if test.Spec.Type != "test" || !issueRepairResultIdentity(test, run) || test.Attempt != evidence.Attempt || test.Fence != evidence.Fence || test.Worker != evidence.WorkerID {
		return false, nil
	}
	source, ok := test.Spec.Config["candidate_from"].(string)
	if !ok || source == "" || source == test.ID {
		return false, nil
	}
	var pinnedHead, pinnedSource, pinnedSHA string
	var sourceAttempt int
	var sourceFence int64
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT commit_id,source_node,source_attempt,source_fence,evidence_sha256 FROM dag_candidate_revisions WHERE run_id=? AND node_id=? AND attempt=?`), run, test.ID, test.Attempt).Scan(&pinnedHead, &pinnedSource, &sourceAttempt, &sourceFence, &pinnedSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pinnedSource != source || sourceAttempt < 1 || sourceFence < 1 || !gitRevision.MatchString(pinnedHead) || !contentDigest.MatchString(pinnedSHA) || evidence.SourceCommit != pinnedHead {
		return false, nil
	}
	head, valid := testPlanReportWorkspace(test.Result)
	if !valid || head != pinnedHead {
		return false, nil
	}
	proof := test.Result.WorkspaceEvidence
	if proof["candidate_from"] != source || !issueRepairProofInteger(proof["candidate_attempt"], int64(sourceAttempt)) || !issueRepairProofInteger(proof["candidate_fence"], sourceFence) || proof["candidate_evidence_sha256"] != pinnedSHA {
		return false, nil
	}
	producer, found, err := s.issueRepairNodeResultTx(ctx, auth, project, run, source)
	if err != nil || !found {
		return false, err
	}
	if producer.Spec.Type != "command" && producer.Spec.Type != "agent" || !issueRepairResultIdentity(producer, run) || producer.Worker == test.Worker || producer.Attempt != sourceAttempt || producer.Fence != sourceFence {
		return false, nil
	}
	profileID, profileSHA, bound, err := dag.ToolBinding(producer.Spec)
	if err != nil || !bound || producer.Result.ToolEvidence == nil {
		return false, nil
	}
	tool := producer.Result.ToolEvidence
	if toolcontract.ValidateEvidence(*tool) != nil || tool.Terminal != "completed" || tool.Profile.ProfileSHA256 != profileSHA || tool.Profile.CommandID != producer.Spec.Command[0] || producer.Spec.Type == "command" && (tool.Profile.Adapter != "command" || tool.Profile.Protocol != "text-v1") {
		return false, nil
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, profileID)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	if profile.WorkerID != producer.Worker || profile.Profile.ProfileSHA256 != profileSHA || !toolcontract.EqualProfile(profile.Profile, tool.Profile) {
		return false, nil
	}
	head, err = workspaceRevision(producer.Result.WorkspaceEvidence)
	if err != nil {
		return false, nil
	}
	if head != pinnedHead || producer.Result.WorkspaceEvidence["evidence_sha256"] != pinnedSHA {
		return false, nil
	}
	if err = validateNativeWorkspaceEvidence(producer.Result.WorkspaceEvidence, producer.Result.ToolEvidence); err != nil {
		return false, nil
	}
	return true, nil
}
