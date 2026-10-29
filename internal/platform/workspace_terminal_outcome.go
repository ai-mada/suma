package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The existing Engine verifies the original result operation and live machine
// scope. A result cannot replace the exact exchange's process-tree stop fact,
// manufacture a code candidate, or clear an unknown input effect.
func (s *Service) recordWorkspaceTerminalOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	r, found, err := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found {
		return err
	}
	if authorize == nil {
		return forbidden()
	}
	if err = authorize(tx); err != nil {
		return err
	}
	if result.NodeID != "terminal" || result.Attempt != 1 || result.WorkerID != r.Session.WorkerID || result.Fence < 1 || r.Attempt != 0 && (r.Attempt != result.Attempt || r.Fence != result.Fence) {
		return forbidden()
	}
	if result.WorkspaceEvidence != nil || result.TestReport != nil || result.ArtifactEvidence != nil || result.ReleaseEvidence != nil || result.IntegrationEvidence != nil {
		return forbidden()
	}
	if evidence := result.ToolEvidence; evidence != nil {
		if tc.ValidateEvidence(*evidence) != nil || !tc.SupportsTerminalSessionProfile(evidence.Profile) || evidence.Profile.ProfileSHA256 != r.ToolProfileSHA256 || evidence.ModelExecution != nil || evidence.CodexExecution != nil || evidence.OpenCodeExecution != nil || evidence.OctopExecution != nil || evidence.Usage != nil {
			return forbidden()
		}
	}
	previous := r.Session.Version
	if !result.OutcomeUnknown {
		if result.ToolEvidence == nil || !r.TreeStopped || r.DispatchID == "" || r.Attempt != result.Attempt || r.Fence != result.Fence || result.ExitCode != r.ExitCode || r.Session.Reason == workspaceTerminalInputUnknown || r.Session.State != "stopped" && r.Session.State != "exited" || r.Session.State == "stopped" && !result.Stopped {
			return conflict(workspaceTerminalStopped)
		}
		_, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=? AND session_id=?`), tenant, project, r.Session.TaskID, r.Session.ID)
		return err
	}
	// Unknown means the original facts remain unsettled, even if the process
	// itself stopped. Retain the task slot until a separate explicit review.
	r.Session.State = "reconciling"
	if r.Session.Reason == "" {
		r.Session.Reason = workspaceTerminalStopped
	}
	r.Session.Version++
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, r.Session.TaskID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_terminal_active(tenant_id,project_id,task_id,session_id)VALUES(?,?,?,?)`), tenant, project, r.Session.TaskID, r.Session.ID); err != nil {
			return err
		}
	}
	return s.saveWorkspaceTerminalTx(ctx, tx, tenant, r, previous)
}
