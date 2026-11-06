package dag

import (
	"database/sql"
	"errors"
	"fmt"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/releasecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// ErrRunnerResultClosed proves an unacknowledged ordinary process result can
// no longer apply to its original completed attempt.
var ErrRunnerResultClosed = fmt.Errorf("runner result attempt closed: %w", ErrStale)

func (t *transaction) closedRunnerResult(r *runState, n *nodeState, result Result) error {
	if r.ID != result.RunID || n == nil || n.ID != result.NodeID || n.Attempt < result.Attempt ||
		result.OutcomeUnknown || result.Stopped || result.WorkBuddyCloudWait != nil || result.WorkspaceEvidence != nil ||
		result.ReleaseEvidence != nil || result.IntegrationEvidence != nil || result.ArtifactEvidence != nil ||
		result.ToolEvidence != nil || result.TestReport != nil || result.StartupFailure != nil || protectedRunnerResultNode(n.spec) {
		return ErrStale
	}
	switch n.Type {
	case "task", "script", "command", "agent", "test":
	default:
		return ErrStale
	}
	// The durable attempt is the authority. Pending history with a completion
	// timestamp represents manual not_started reconciliation. Managed and
	// compensation families have independent evidence and cannot use this path.
	var runID, nodeID, worker, state, completed string
	err := t.row(`SELECT a.run_id,a.node_id,a.worker_id,a.state,a.completed_at FROM dag_attempts a
		LEFT JOIN dag_run_families f ON f.run_id=a.run_id
		WHERE a.run_id=? AND a.node_id=? AND a.attempt=? AND a.fence=? AND a.worker_id=?
		AND NOT EXISTS (SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=? AND m.project_id=? AND (m.run_id=a.run_id OR m.run_id=f.root_run_id))
		AND NOT EXISTS (SELECT 1 FROM dag_compensations c WHERE c.compensation_run_id=a.run_id OR c.compensation_run_id=f.root_run_id)`,
		r.ID, n.ID, result.Attempt, result.Fence, result.WorkerID, r.TenantID, r.ProjectID).Scan(&runID, &nodeID, &worker, &state, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStale
	}
	if err != nil {
		return err
	}
	if runID != result.RunID || nodeID != result.NodeID || worker != result.WorkerID || completed == "" || !terminal(state) && state != "pending" {
		return ErrStale
	}
	return ErrRunnerResultClosed
}

func protectedRunnerResultNode(n Node) bool {
	if n.Config["plugin"] == wbc.Plugin || n.Config["purpose"] == "candidate_composition" || n.Config["purpose"] == "candidate_conflict_repair" ||
		len(n.Command) == 1 && n.Command[0] == ccmp.CompositionCommand {
		return true
	}
	for _, key := range []string{
		releasecontract.ApplicationBuildConfigKey, ri.RepositorySourceExecutionKey, term.CandidateConfigKey,
		"candidate_from", "artifact_from", "test_from", "candidate_composition", "candidate_conflict_repair", "code_baseline",
		"development_continuation_id", "development_continuation", "execution_skills", "model_execution", "codex_execution", "opencode_execution", "octop_execution",
	} {
		if _, present := n.Config[key]; present {
			return true
		}
	}
	return false
}
