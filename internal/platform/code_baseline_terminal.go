package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// A baseline belongs to its protected input grant. A node kind alone cannot
// turn a reusable command or terminal definition into a cross-run code reader.
func (s *Service) codeBaselineNodeKindTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, spec dag.Node, b ccmp.CodeBaselineBinding) (bool, error) {
	if spec.Config["candidate_from"] != nil || !codeBaselineConfigMatches(spec.Config["code_baseline"], b) {
		return false, nil
	}
	if spec.Type == "agent" {
		return true, nil
	}
	if spec.Type == "terminal_session" {
		r, found, err := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, run)
		if err != nil || !found {
			return false, err
		}
		return node == "terminal" && r.Session.TaskID == b.TaskID && r.Session.CodeBaseline != nil && *r.Session.CodeBaseline == b && spec.Config["terminal_session_id"] == r.Session.ID, nil
	}
	if spec.Type != "command" {
		return false, nil
	}
	actual, err := term.DecodeCandidateSource(spec.Config[term.CandidateConfigKey])
	if err != nil || actual == nil || actual.CodeBaseline == nil || *actual.CodeBaseline != b {
		return false, nil
	}
	frozen, err := s.workspaceTerminalCandidateSourceTx(ctx, tx, tenant, project, actual.PlanID)
	if err != nil || frozen == nil {
		return false, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_candidate_plans WHERE tenant_id=? AND project_id=? AND plan_id=? AND run_id=? AND node_id=?`), tenant, project, actual.PlanID, run, node).Scan(&count); err != nil {
		return false, err
	}
	return count == 1 && stringify(actual) == stringify(frozen), nil
}

func (s *Service) codeBaselineTaskRelationTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, b ccmp.CodeBaselineBinding, fresh bool) (dag.Node, error) {
	var spec dag.Node
	var taskRun, raw string
	var version int
	err := tx.QueryRowContext(ctx, s.q(`SELECT t.run_id,t.version,n.spec FROM suma_tasks t JOIN dag_runs r ON r.tenant_id=t.tenant_id AND r.project_id=t.project_id AND r.id=? JOIN dag_nodes n ON n.run_id=r.id AND n.id=? WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), run, node, tenant, project, b.TaskID).Scan(&taskRun, &version, &raw)
	if err != nil {
		return spec, err
	}
	if json.Unmarshal([]byte(raw), &spec) != nil {
		return spec, compositionError(409, "baseline_changed", compositionBaseline)
	}
	allowed, err := s.codeBaselineNodeKindTx(ctx, tx, tenant, project, run, node, spec, b)
	if err != nil {
		return spec, err
	}
	if !allowed {
		return spec, compositionError(409, "baseline_changed", compositionBaseline)
	}
	if spec.Type == "terminal_session" {
		r, found, e := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, run)
		if e != nil {
			return spec, e
		}
		if !found || r.Task.Version != b.TaskVersion || fresh && (taskRun != r.TaskRunID || version != r.Task.Version) {
			return spec, compositionError(409, "baseline_changed", compositionBaseline)
		}
		return spec, nil
	}
	if taskRun != run || version <= b.TaskVersion || fresh && version != b.TaskVersion+1 {
		return spec, compositionError(409, "baseline_changed", compositionBaseline)
	}
	return spec, nil
}
