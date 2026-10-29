package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
)

// UnknownInputs identify delivery facts the reviewer explicitly accepts as
// unresolved. They never assert delivery or authorize replay of shell bytes.
type WorkspaceTerminalRecoveryInput struct {
	InputID  string `json:"input_id"`
	Sequence int64  `json:"sequence"`
	SHA256   string `json:"sha256"`
}
type WorkspaceTerminalRecoveryRequest struct {
	ProjectID          string                           `json:"project_id"`
	ExpectedVersion    int                              `json:"expected_version"`
	ExpectedRunVersion int                              `json:"expected_run_version"`
	RunID              string                           `json:"run_id"`
	Attempt            int                              `json:"attempt"`
	Fence              int64                            `json:"fence"`
	DispatchID         string                           `json:"dispatch_id"`
	UnknownInputs      []WorkspaceTerminalRecoveryInput `json:"unknown_inputs"`
	Reason             string                           `json:"reason"`
	IdempotencyKey     string                           `json:"idempotency_key"`
}
type WorkspaceTerminalRecoveryView struct {
	SessionID     string                           `json:"session_id"`
	RunID         string                           `json:"run_id"`
	RunVersion    int                              `json:"run_version"`
	RunState      string                           `json:"run_state"`
	Attempt       int                              `json:"attempt"`
	Fence         int64                            `json:"fence"`
	DispatchID    string                           `json:"dispatch_id"`
	TreeStopped   bool                             `json:"tree_stopped"`
	UnknownInputs []WorkspaceTerminalRecoveryInput `json:"unknown_inputs"`
	CanRecover    bool                             `json:"can_recover"`
	Reason        string                           `json:"reason"`
}
type WorkspaceTerminalRecoveryReceipt struct {
	Session  WorkspaceTerminalSession      `json:"session"`
	Recovery WorkspaceTerminalRecoveryView `json:"recovery"`
}

func (s *Service) workspaceTerminalRecoveryViewTx(ctx context.Context, auth *authorizedCommand, r workspaceTerminalRecord) (out WorkspaceTerminalRecoveryView, err error) {
	out = WorkspaceTerminalRecoveryView{SessionID: r.Session.ID, RunID: r.Session.RunID, Attempt: r.Attempt, Fence: r.Fence, DispatchID: r.DispatchID, TreeStopped: r.TreeStopped, UnknownInputs: []WorkspaceTerminalRecoveryInput{}}
	var state, worker string
	var attempt int
	var fence int64
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT r.version,r.state,n.state,n.worker_id,n.attempt,n.fence FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id AND n.id='terminal' WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), auth.target.tenantID, r.Session.ProjectID, r.Session.RunID).Scan(&out.RunVersion, &out.RunState, &state, &worker, &attempt, &fence); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT input_id,sequence,sha256 FROM suma_workspace_terminal_inputs WHERE session_id=? AND state='delivery_unknown' ORDER BY sequence`), r.Session.ID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var input WorkspaceTerminalRecoveryInput
		if err = rows.Scan(&input.InputID, &input.Sequence, &input.SHA256); err != nil {
			rows.Close()
			return out, err
		}
		out.UnknownInputs = append(out.UnknownInputs, input)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if r.RecoveryOperationID != "" {
		out.Reason = "The original terminal was reviewed and closed without replay"
		return out, nil
	}
	if !r.TreeStopped {
		out.Reason = workspaceTerminalStopped
		return out, nil
	}
	if state != "reconciling" || attempt != 1 || attempt != r.Attempt || fence != r.Fence || worker != r.Session.WorkerID || r.DispatchID == "" {
		out.Reason = "Wait for the original terminal attempt to require reconciliation"
		return out, nil
	}
	_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, r.Session.ProjectID, false, true)
	if err != nil {
		var api *APIError
		if !errors.As(err, &api) || api.Status != 403 {
			return out, err
		}
		out.Reason = "Current project approval permission is required to review terminal recovery"
		return out, nil
	}
	out.CanRecover = true
	return out, nil
}

func (s *Service) GetWorkspaceTerminalRecovery(ctx context.Context, actor Actor, project, id string) (out WorkspaceTerminalRecoveryView, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.workspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if _, err = s.workspaceTerminalProjectionTx(ctx, auth, r); err != nil {
		return out, err
	}
	return s.workspaceTerminalRecoveryViewTx(ctx, auth, r)
}

// RecoverWorkspaceTerminal closes only the original failed attempt. The Engine
// owns accounting, resource release, state transitions and its original receipt.
func (s *Service) RecoverWorkspaceTerminal(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req WorkspaceTerminalRecoveryRequest) (out WorkspaceTerminalRecoveryReceipt, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if engine == nil || project != req.ProjectID || !handoffID(id, false) || !handoffID(req.RunID, false) || req.ExpectedVersion < 1 || req.ExpectedRunVersion < 1 || req.Attempt != 1 || req.Fence < 1 || !planningKey(req.DispatchID) || !planningKey(req.IdempotencyKey) || len(req.UnknownInputs) > 65536 || strings.TrimSpace(req.Reason) == "" || !testPlanText(req.Reason, 2000, true) {
		return out, invalid(workspaceTerminalInvalid)
	}
	seen := map[string]bool{}
	for _, in := range req.UnknownInputs {
		if !terminalInputID(in.InputID) || in.Sequence < 1 || !contentDigest.MatchString(in.SHA256) || seen[in.InputID] {
			return out, invalid(workspaceTerminalInvalid)
		}
		seen[in.InputID] = true
	}
	body := map[string]any{"workspace_terminal_recovery": id, "request": req}
	var auth *authorizedCommand
	var record workspaceTerminalRecord
	var hash string
	var replay bool
	// Resolve to failed even when the shell exited zero: a reviewed ambiguous
	// effect is not a successful execution or a code/test/acceptance fact.
	payload := map[string]any{"node_id": "terminal", "outcome": "failed", "reason": req.Reason, "terminal_session_id": id, "attempt": req.Attempt, "fence": req.Fence, "dispatch_id": req.DispatchID, "unknown_inputs": req.UnknownInputs}
	_, err = engine.CommandWithAuthority(ctx, actor.credential.tenantID, project, req.RunID, "reconcile", "terminal-recovery-"+digest(req.IdempotencyKey), req.ExpectedRunVersion, payload, func(tx *sql.Tx) error {
		var e error
		auth, e = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectApprove)
		if e != nil {
			return e
		}
		if e = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, auth.target.tenantID, project, req.RunID); e != nil {
			return e
		}
		hash, replay, e = s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if replay {
			return json.Unmarshal([]byte(hash), &out)
		}
		record, e = s.workspaceTerminalTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return e
		}
		if _, e = s.workspaceTerminalProjectionTx(ctx, auth, record); e != nil {
			return e
		}
		view, e := s.workspaceTerminalRecoveryViewTx(ctx, auth, record)
		if e != nil {
			return e
		}
		if !view.CanRecover || record.Session.Version != req.ExpectedVersion || view.RunVersion != req.ExpectedRunVersion || view.RunID != req.RunID || view.Attempt != req.Attempt || view.Fence != req.Fence || view.DispatchID != req.DispatchID {
			return conflict(workspaceTerminalChanged)
		}
		if len(req.UnknownInputs) != len(view.UnknownInputs) {
			return conflict("Review every original unknown terminal input before closing its failed attempt")
		}
		for _, input := range view.UnknownInputs {
			matched := false
			for _, reviewed := range req.UnknownInputs {
				if reviewed == input {
					matched = true
					break
				}
			}
			if !matched {
				return conflict("Review every original unknown terminal input before closing its failed attempt")
			}
		}
		return nil
	}, func(tx *sql.Tx, run dag.Run) error {
		if replay {
			return nil
		}
		previous := record.Session.Version
		record.RecoveryOperationID = req.IdempotencyKey
		record.ReviewedUnknownInputs = append([]WorkspaceTerminalRecoveryInput{}, req.UnknownInputs...)
		record.Session.State = "stopped"
		record.Session.Reason = "The original terminal was reviewed and closed without replay"
		record.Session.Version++
		if e := s.saveWorkspaceTerminalTx(ctx, tx, auth.target.tenantID, record, previous); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, s.q(`DELETE FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=? AND session_id=?`), auth.target.tenantID, project, record.Session.TaskID, id); e != nil {
			return e
		}
		var e error
		out.Session, e = s.workspaceTerminalProjectionTx(ctx, auth, record)
		if e != nil {
			return e
		}
		out.Recovery, e = s.workspaceTerminalRecoveryViewTx(ctx, auth, record)
		if e != nil {
			return e
		}
		if e = s.commandActivity(ctx, auth, "workspace_terminal.recover", id, req.Reason); e != nil {
			return e
		}
		return s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out)
	})
	return out, err
}
