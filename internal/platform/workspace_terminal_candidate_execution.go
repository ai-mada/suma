package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func (s *Service) workspaceTerminalCandidateExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*term.TerminalCandidateSource, error) {
	var plan string
	err := tx.QueryRowContext(ctx, s.q(`SELECT c.plan_id FROM suma_workspace_terminal_candidate_plans c JOIN suma_task_dispatch_plans p ON p.id=c.plan_id AND p.tenant_id=c.tenant_id AND p.project_id=c.project_id AND p.run_id=c.run_id WHERE c.tenant_id=? AND c.project_id=? AND c.run_id=? AND c.node_id=? AND p.state='started'`), tenant, project, run, node).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.workspaceTerminalCandidateSourceTx(ctx, tx, tenant, project, plan)
}

func (s *Service) workspaceTerminalCandidateNodeEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, spec dag.Node) (bool, error) {
	source, err := s.workspaceTerminalCandidateExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return false, err
	}
	raw, present := spec.Config[term.CandidateConfigKey]
	if source == nil {
		return !present, nil
	}
	actual, err := term.DecodeCandidateSource(raw)
	if err != nil || actual == nil || !present || stringify(actual) != stringify(source) || spec.Type != "command" || len(spec.Command) != 1 || spec.MaxRetries != 0 || spec.Idempotent || spec.Config["candidate_from"] != nil || spec.Config["code_baseline"] != nil || spec.Config["candidate_composition"] != nil || spec.Config["candidate_conflict_repair"] != nil || spec.Config["model_execution"] != nil {
		return false, nil
	}
	var worker string
	profileID, _, bound, err := dag.ToolBinding(spec)
	if err != nil || !bound {
		return false, nil
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT worker_id FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, profileID).Scan(&worker); err != nil {
		return false, err
	}
	return worker == source.Original.WorkerID, nil
}

func (s *Service) checkWorkspaceTerminalCandidateExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, fresh bool) error {
	source, err := s.workspaceTerminalCandidateExecutionTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return err
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`), run, node).Scan(&raw)
	if err != nil {
		return err
	}
	var spec dag.Node
	if json.Unmarshal([]byte(raw), &spec) != nil {
		return forbidden()
	}
	eligible, err := s.workspaceTerminalCandidateNodeEligibleTx(ctx, tx, tenant, project, run, node, spec)
	if err != nil {
		return err
	}
	if !eligible {
		return forbidden()
	}
	if source != nil && fresh {
		return s.workspaceTerminalCandidateStoppedTx(ctx, tx, tenant, project, *source)
	}
	return nil
}

func validateTerminalCandidateEvidence(proof map[string]any, tool *tc.Evidence, expected *term.TerminalCandidateSource) error {
	raw, present := proof["terminal_candidate"]
	if expected == nil {
		if present {
			return forbidden()
		}
		return nil
	}
	if !present || tool == nil || tool.Profile.Adapter != "command" || tool.Profile.Protocol != "text-v1" || tool.Profile.ReadOnly || tool.Profile.TerminalSessionProtocol != "" {
		return forbidden()
	}
	var actual term.TerminalCandidateEvidence
	encoded, err := json.Marshal(raw)
	if err != nil || len(encoded) > 16384 || json.Unmarshal(encoded, &actual) != nil || actual.SchemaVersion != 1 || stringify(actual.Source) != stringify(expected) || actual.SnapshotSHA256 != expected.SnapshotSHA256 || actual.FileCount < 1 || actual.FileCount > 2000 {
		return conflict(workspaceTerminalCandidateChanged)
	}
	seal := actual.Seal
	if seal.SchemaVersion != 1 || seal.OperationID == "" || seal.State != "sealed" || seal.PolicySHA256 != tool.Profile.ProfileSHA256 || seal.PreviousCommit != expected.BaseCommit || seal.Commit != proof["head_commit"] || seal.Commit == seal.PreviousCommit || !gitRevision.MatchString(seal.Commit) || !gitRevision.MatchString(seal.Tree) || seal.ChangedPaths != actual.FileCount || proof["base_commit"] != expected.BaseCommit || proof["admin_base_commit"] != expected.BaseCommit {
		return conflict(workspaceTerminalCandidateChanged)
	}
	return nil
}

func (s *Service) validateWorkspaceTerminalCandidateResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	source, err := s.workspaceTerminalCandidateExecutionTx(ctx, tx, tenant, project, result.RunID, result.NodeID)
	if err != nil {
		return err
	}
	if source == nil {
		if _, exists := result.WorkspaceEvidence["terminal_candidate"]; exists {
			return forbidden()
		}
		return nil
	}
	if result.WorkerID != source.Original.WorkerID || result.Attempt != 1 {
		return forbidden()
	}
	if result.OutcomeUnknown || result.Stopped || result.ExitCode != 0 || result.Error != "" {
		return nil
	}
	return validateTerminalCandidateEvidence(result.WorkspaceEvidence, result.ToolEvidence, source)
}
