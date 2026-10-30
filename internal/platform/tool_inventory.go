package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// Inventory observations need the database's actual subsecond time so a new
// local worker report can be distinguished from a report earlier in the same
// second. Keep this clock local to tool freshness; authority policy and other
// domains retain their existing database clock and timestamp contracts.
func (s *Service) toolInventoryClock(ctx context.Context, q governanceQuery) (time.Time, error) {
	query := `SELECT strftime('%Y-%m-%dT%H:%M:%fZ','now')`
	if s.Dialect == "postgres" {
		query = `SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
	} else if s.Dialect == "oceanbase" {
		query = `SELECT DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%dT%H:%i:%s.%fZ')`
	}
	var value string
	if err := q.QueryRowContext(ctx, query).Scan(&value); err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, value)
}

func (s *Service) ReportToolInventory(ctx context.Context, actor Actor, project string, req ToolInventoryRequest) (ToolInventoryReceipt, error) {
	var out ToolInventoryReceipt
	if req.ProjectID != project || !toolcontract.ValidCommandID(req.WorkerID) || len(req.Items) > 128 {
		return out, invalid("项目、执行节点或工具清单大小无效")
	}
	seen := map[string]bool{}
	for _, observation := range req.Items {
		if err := toolcontract.ValidateObservation(observation); err != nil {
			return out, invalidDiagnostic("validation.tool_inventory_detail", "工具探测数据无效："+err.Error(), map[string]any{"detail": err.Error()})
		}
		if seen[observation.Profile.CommandID] {
			return out, invalid("工具命令标识不能重复")
		}
		seen[observation.Profile.CommandID] = true
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, req.WorkerID, "", "", "identity")
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.OperationID, map[string]any{"tool_inventory": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out = ToolInventoryReceipt{Acknowledged: true, ObservedAt: clock.UTC().Format(time.RFC3339Nano)}
	var old string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), p.TenantID, project, req.WorkerID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,?,?)`), p.TenantID, project, req.WorkerID, out.ObservedAt, stringify(req.Items))
	} else if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_tool_inventories SET observed_at=?,items=? WHERE tenant_id=? AND project_id=? AND worker_id=?`), out.ObservedAt, stringify(req.Items), p.TenantID, project, req.WorkerID)
	}
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// toolClaimEligibleTx only examines bound candidates. It runs before any
// attempt/resource acquisition, and cannot turn a self-declared capability into
// an approved execution profile or revive expired observations.
func (s *Service) toolClaimEligibleTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, worker, run, node string) (bool, error) {
	if err := assertRunnerCommand(tx, auth); err != nil {
		return false, err
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), p.TenantID, project, run, node).Scan(&raw)
	if err != nil {
		return false, err
	}
	var spec dag.Node
	if err = json.Unmarshal([]byte(raw), &spec); err != nil {
		return false, err
	}
	if eligible, err := s.codeInputNodeEligibleTx(ctx, tx, p.TenantID, project, run, node, spec); err != nil || !eligible {
		return false, err
	}
	id, sha, bound, err := dag.ToolBinding(spec)
	if err != nil {
		return false, nil
	}
	if !bound {
		if _, present := spec.Config["octop_execution"]; present {
			return false, nil
		}
		if _, present := spec.Config["opencode_execution"]; present {
			return false, nil
		}
		return true, nil
	}
	if err = s.releaseWorkerBindingTx(ctx, tx, auth, worker, "", "", "identity"); err != nil {
		if automationAuthorityFailure(err) {
			return false, nil
		}
		return false, err
	}
	profile, err := s.toolProfileTx(ctx, tx, p, project, id)
	if err != nil {
		if automationAuthorityFailure(err) {
			return false, nil
		}
		return false, err
	}
	if profile.WorkerID != worker || profile.Profile.CommandID != spec.Command[0] || profile.Profile.ProfileSHA256 != sha {
		return false, nil
	}
	if spec.Type == "command" && (profile.Profile.Adapter != "command" || profile.Profile.Protocol != "text-v1") {
		return false, nil
	}
	profile, err = s.toolProfileObservationTx(ctx, tx, p, project, profile)
	if err != nil || !profile.Available {
		return false, err
	}
	return s.modelExecutionClaimEligibleTx(ctx, tx, p.TenantID, project, profile, spec)
}

func (s *Service) toolBoundNodeTx(ctx context.Context, tx *sql.Tx, p Principal, project, run, node string) (dag.Node, ToolProfile, error) {
	var n dag.Node
	var pr ToolProfile
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), p.TenantID, project, run, node).Scan(&raw)
	if err != nil {
		return n, pr, err
	}
	if err = json.Unmarshal([]byte(raw), &n); err != nil {
		return n, pr, err
	}
	id, _, bound, err := dag.ToolBinding(n)
	if err != nil {
		return n, pr, err
	}
	if !bound {
		return n, pr, nil
	}
	pr, err = s.toolProfileTx(ctx, tx, p, project, id)
	return n, pr, err
}

func (s *Service) validateToolProgressTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, a dag.Assignment, event *toolcontract.Event) error {
	if err := assertRunnerCommand(tx, auth); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	if event == nil {
		return nil
	}
	n, pr, err := s.toolBoundNodeTx(ctx, tx, p, project, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	_, sha, bound, err := dag.ToolBinding(n)
	if err != nil {
		return err
	}
	if !bound || pr.WorkerID != a.WorkerID || pr.Profile.ProfileSHA256 != sha || pr.Profile.CommandID != event.CommandID || pr.Profile.Protocol != event.Protocol || event.ProfileSHA256 != sha || n.Type == "command" && (pr.Profile.Adapter != "command" || pr.Profile.Protocol != "text-v1") {
		return invalid("工具进度与固定档案不匹配")
	}
	return nil
}

func (s *Service) validateToolResultTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, result dag.Result) error {
	if err := assertRunnerCommand(tx, auth); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	n, pr, err := s.toolBoundNodeTx(ctx, tx, p, project, result.RunID, result.NodeID)
	if err != nil {
		return err
	}
	if dag.WorkBuddyCloudNode(n) {
		if result.WorkBuddyCloudWait == nil || result.ToolEvidence != nil || result.Stopped {
			return cloudInvalid()
		}
		return nil
	}
	if result.ToolEvidence == nil {
		if result.Stopped && pr.Profile.Protocol == toolcontract.OpenCodeSDKProtocol {
			return invalid(opencodeExecutionInvalid)
		}
		return nil
	}
	_, sha, bound, err := dag.ToolBinding(n)
	if err != nil {
		return err
	}
	if !bound || pr.WorkerID != result.WorkerID || pr.Profile.ProfileSHA256 != sha || !toolcontract.EqualProfile(result.ToolEvidence.Profile, pr.Profile) || n.Type == "command" && (pr.Profile.Adapter != "command" || pr.Profile.Protocol != "text-v1") {
		return invalid("工具执行证据与固定档案不匹配")
	}
	// A vendor abort acknowledgement and operating-system shutdown are distinct.
	// Validate them against this result before the scheduler accepts a stop.
	if err := toolcontract.ValidateOpenCodeResultStop(*result.ToolEvidence, result.Stopped, result.OutcomeUnknown); err != nil {
		return invalid(opencodeExecutionInvalid)
	}
	return nil
}

// Machine callbacks only consume the proof accepted before scheduler/attempt
// locks. Identity, fence, lease and ACK remain the engine's current protocol.
func assertRunnerCommand(tx *sql.Tx, auth *authorizedCommand) error {
	if auth == nil || auth.actor.kind != actorProjectRunner || auth.purpose != purposeRunnerWrite {
		return forbidden()
	}
	return auth.assertTx(tx, auth.target, purposeRunnerWrite)
}
