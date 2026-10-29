package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func validWorkspaceTerminalExchange(r term.ExchangeRequest) bool {
	if !handoffID(r.SessionID, false) || r.RunID == "" || r.NodeID != "terminal" || r.Attempt != 1 || r.Fence < 1 || !planningKey(r.OperationID) || r.WorkerID == "" || len(r.OutputChunks) > 32 || len(r.InputReceipts) > 32 || r.ProcessID < 0 || !testPlanText(r.Directory, 4096, false) || !testPlanText(r.Error, 2000, false) {
		return false
	}
	switch r.State {
	case "starting", "running", "stop_requested", "stopped", "exited", "reconciling":
	default:
		return false
	}
	if r.TreeStopped && r.State != "stopped" && r.State != "exited" && r.State != "reconciling" {
		return false
	}
	return true
}

func (s *Service) ExchangeWorkspaceTerminal(ctx context.Context, actor Actor, project string, req term.ExchangeRequest) (out term.ExchangeResponse, err error) {
	if actor.kind != actorProjectRunner || !handoffID(project, false) || !validWorkspaceTerminalExchange(req) {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.runnerIdentityTx(ctx, tx, actor, project, req.WorkerID)
	if err != nil {
		return out, err
	}
	if err = s.projectRunnerExecutionAuthorityTx(ctx, tx, auth, req.WorkerID, req.RunID); err != nil {
		return out, err
	}
	var dispatched int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_dispatches WHERE id=? AND tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND worker_id=? AND attempt=? AND fence=?`), req.DispatchID, auth.target.tenantID, project, req.RunID, req.NodeID, req.WorkerID, req.Attempt, req.Fence).Scan(&dispatched); err != nil {
		return out, err
	}
	if dispatched != 1 {
		return out, forbidden()
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project); err != nil {
		return out, err
	}
	out, err = s.exchangeWorkspaceTerminalTx(ctx, tx, auth.target.tenantID, project, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) ExchangePoolWorkspaceTerminal(ctx context.Context, token string, req PoolWorkspaceTerminalExchangeRequest) (out term.ExchangeResponse, err error) {
	a := req.Assignment
	r := req.ExchangeRequest
	if !validWorkspaceTerminalExchange(r) || req.TenantID != a.TenantID || req.ProjectID != a.ProjectID || req.RunnerID != a.PoolRunnerID || a.RunID != r.RunID || a.NodeID != r.NodeID || a.Attempt != r.Attempt || a.Fence != r.Fence || a.WorkerID != r.WorkerID || a.DispatchID != r.DispatchID || a.PoolID == "" {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	proof, err := s.PoolRunnerAssignmentAuthorityTx(ctx, tx, token, a)
	if err != nil {
		return out, err
	}
	if err = proof.assertAssignment(tx, a); err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, a.TenantID, a.ProjectID); err != nil {
		return out, err
	}
	out, err = s.exchangeWorkspaceTerminalTx(ctx, tx, a.TenantID, a.ProjectID, r)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) exchangeWorkspaceTerminalTx(ctx context.Context, tx *sql.Tx, tenant, project string, req term.ExchangeRequest) (out term.ExchangeResponse, err error) {
	r, err := s.workspaceTerminalTx(ctx, tx, tenant, project, req.SessionID)
	if err != nil {
		return out, err
	}
	if r.Session.RunID != req.RunID || r.Session.WorkerID != req.WorkerID {
		return out, forbidden()
	}
	var worker, state string
	var attempt int
	var fence, lease int64
	if err = tx.QueryRowContext(ctx, s.q(`SELECT worker_id,attempt,fence,state,lease_until FROM dag_nodes WHERE run_id=? AND id=?`), req.RunID, req.NodeID).Scan(&worker, &attempt, &fence, &state, &lease); err != nil {
		return out, err
	}
	if worker != req.WorkerID || attempt != req.Attempt || fence != req.Fence {
		return out, forbidden()
	}
	if r.Attempt != 0 && (r.Attempt != req.Attempt || r.Fence != req.Fence || r.DispatchID != req.DispatchID) {
		return out, forbidden()
	}
	if r.ProcessID != 0 && req.ProcessID != r.ProcessID {
		return out, conflict(workspaceTerminalChanged)
	}
	if r.Session.Directory != "" && req.Directory != r.Session.Directory {
		return out, conflict(workspaceTerminalChanged)
	}
	if req.State == "running" && (req.ProcessID < 1 || req.Directory == "") {
		return out, invalid(workspaceTerminalInvalid)
	}
	if req.State == "starting" && req.ProcessID == 0 && (req.Directory != "" || len(req.OutputChunks) != 0 || len(req.InputReceipts) != 0) {
		return out, invalid(workspaceTerminalInvalid)
	}
	requestSHA := digest(stringify(req))
	var prior string
	err = tx.QueryRowContext(ctx, s.q(`SELECT sha256 FROM suma_workspace_terminal_exchanges WHERE session_id=? AND operation_id=?`), req.SessionID, req.OperationID).Scan(&prior)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	replay := err == nil
	if replay && prior != requestSHA {
		return out, conflict(workspaceTerminalChanged)
	}
	previous := r.Session.Version
	if !replay {
		if r.TreeStopped && !req.TreeStopped {
			return out, conflict(workspaceTerminalChanged)
		}
		r.Attempt, r.Fence, r.DispatchID = req.Attempt, req.Fence, req.DispatchID
		if r.ProcessID == 0 {
			r.ProcessID = req.ProcessID
		}
		if r.Session.Directory == "" {
			r.Session.Directory = req.Directory
		}
		for _, chunk := range req.OutputChunks {
			if err = s.workspaceTerminalOutputTx(ctx, tx, &r, chunk); err != nil {
				return out, err
			}
		}
		for _, receipt := range req.InputReceipts {
			if err = s.workspaceTerminalInputReceiptTx(ctx, tx, &r, receipt); err != nil {
				return out, err
			}
		}
		oldState := r.Session.State
		if req.TreeStopped {
			r.TreeStopped = true
			r.ExitCode = req.ExitCode
			if r.Session.Reason == workspaceTerminalInputUnknown {
				r.Session.State = "reconciling"
			} else {
				r.Session.State = req.State
				if r.Session.State != "stopped" && r.Session.State != "exited" {
					r.Session.State = "stopped"
				}
			}
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_terminal_inputs SET data_base64='',state='not_delivered' WHERE session_id=? AND state='accepted'`), r.Session.ID); err != nil {
				return out, err
			}
			// The slot remains occupied until the original SDK completion also
			// confirms its workspace observation. Tree stop alone does not permit
			// a replacement attempt while that final result is uncertain.
		} else if req.State == "stopped" || req.State == "exited" || req.State == "reconciling" {
			r.Session.State = "reconciling"
			if r.Session.Reason == "" {
				r.Session.Reason = workspaceTerminalStopped
			}
		} else if oldState != "stop_requested" && req.State != "starting" {
			r.Session.State = req.State
		}
		if r.Session.State != oldState {
			r.Session.Version++
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_terminal_exchanges(session_id,operation_id,sha256)VALUES(?,?,?)`), req.SessionID, req.OperationID, requestSHA); err != nil {
			return out, err
		}
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	live := lease > clock && (state == "running" || state == "cancelling") && !r.TreeStopped
	if live {
		if check := s.checkWorkspaceTerminalExecutionTx(ctx, tx, tenant, project, req.RunID, true); check != nil {
			var api *APIError
			if !errors.As(check, &api) {
				return out, check
			}
			live = false
			if r.Session.State != "stop_requested" {
				r.Session.State = "stop_requested"
				r.Session.Version++
			}
			if r.Session.Reason == "" {
				r.Session.Reason = workspaceTerminalAuthority
			}
		}
	}
	if !live && !r.TreeStopped && r.Session.State != "stop_requested" {
		r.Session.State = "stop_requested"
		r.Session.Version++
		if r.Session.Reason == "" {
			r.Session.Reason = workspaceTerminalAuthority
		}
	}
	if err = s.saveWorkspaceTerminalTx(ctx, tx, tenant, r, previous); err != nil {
		return out, err
	}
	out = term.ExchangeResponse{SessionID: r.Session.ID, ControlEpoch: r.Session.ControlEpoch, Inputs: []term.Input{}, Columns: r.Session.Columns, Rows: r.Session.Rows, StopRequested: !live || r.Session.State == "stop_requested" || r.TreeStopped, OutputAckSequence: r.Session.OutputLastSequence}
	if live && r.Session.State == "running" && r.Session.Reason == "" {
		rows, e := tx.QueryContext(ctx, s.q(`SELECT input_id,sequence,sha256,data_base64 FROM suma_workspace_terminal_inputs WHERE session_id=? AND state='accepted' ORDER BY sequence LIMIT 32`), r.Session.ID)
		if e != nil {
			return out, e
		}
		defer rows.Close()
		for rows.Next() {
			var in term.Input
			if e = rows.Scan(&in.InputID, &in.Sequence, &in.SHA256, &in.DataBase64); e != nil {
				return out, e
			}
			out.Inputs = append(out.Inputs, in)
		}
		if e = rows.Err(); e != nil {
			return out, e
		}
	}
	return out, nil
}

func (s *Service) workspaceTerminalInputReceiptTx(ctx context.Context, tx *sql.Tx, r *workspaceTerminalRecord, in term.InputReceipt) error {
	if !terminalInputID(in.InputID) || in.Sequence < 1 || in.Sequence >= r.Session.NextInputSequence || !contentDigest.MatchString(in.SHA256) || in.BytesWritten < 0 || in.State != "delivered" && in.State != "delivery_unknown" {
		return invalid(workspaceTerminalInvalid)
	}
	var sequence int64
	var sha, state string
	var size, written int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT sequence,sha256,bytes,state,bytes_written FROM suma_workspace_terminal_inputs WHERE session_id=? AND input_id=?`), r.Session.ID, in.InputID).Scan(&sequence, &sha, &size, &state, &written); err != nil {
		return err
	}
	if sequence != in.Sequence || sha != in.SHA256 || in.BytesWritten > size || in.State == "delivered" && in.BytesWritten != size {
		return conflict(workspaceTerminalChanged)
	}
	if state != "accepted" {
		if state == "delivery_unknown" && in.State == "delivered" && in.BytesWritten == size {
			// The original single PTY write may finish after its caller timed
			// out. This advances only its exact persisted identity; no input is
			// returned to the Runner or written a second time.
		} else if state != in.State || written != in.BytesWritten {
			return conflict(workspaceTerminalChanged)
		} else {
			return nil
		}
	}
	if _, err := tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_terminal_inputs SET state=?,bytes_written=?,data_base64='' WHERE session_id=? AND input_id=? AND state=?`), in.State, in.BytesWritten, r.Session.ID, in.InputID, state); err != nil {
		return err
	}
	if in.State == "delivery_unknown" {
		r.Session.Reason = workspaceTerminalInputUnknown
		r.Session.Version++
	} else if r.Session.Reason == workspaceTerminalInputUnknown {
		var unresolved int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_inputs WHERE session_id=? AND state='delivery_unknown'`), r.Session.ID).Scan(&unresolved); err != nil {
			return err
		}
		if unresolved == 0 {
			r.Session.Reason = ""
			r.Session.Version++
		}
	}
	return nil
}

func (s *Service) workspaceTerminalOutputTx(ctx context.Context, tx *sql.Tx, r *workspaceTerminalRecord, chunk term.OutputChunk) error {
	data, err := term.DecodeBytes(chunk.DataBase64, term.MaxOutputChunkBytes)
	if err != nil || base64.StdEncoding.EncodeToString(data) != chunk.DataBase64 || chunk.Sequence < 1 || chunk.Sequence > taskConversationMaxSeq {
		return invalid(workspaceTerminalInvalid)
	}
	sha := digest(string(data))
	if chunk.Sequence <= r.Session.OutputLastSequence {
		if chunk.Sequence < r.Session.OutputFirstSequence {
			return nil
		} // already committed, now outside the bounded replay window
		var prior string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT sha256 FROM suma_workspace_terminal_outputs WHERE session_id=? AND sequence=?`), r.Session.ID, chunk.Sequence).Scan(&prior); err != nil {
			return err
		}
		if prior != sha {
			return conflict(workspaceTerminalChanged)
		}
		return nil
	}
	if chunk.Sequence != r.Session.OutputLastSequence+1 {
		return conflict(workspaceTerminalChanged)
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_terminal_outputs(session_id,sequence,bytes,sha256,data_base64)VALUES(?,?,?,?,?)`), r.Session.ID, chunk.Sequence, len(data), sha, chunk.DataBase64); err != nil {
		return err
	}
	r.Session.OutputLastSequence = chunk.Sequence
	if r.Session.OutputFirstSequence == 0 {
		r.Session.OutputFirstSequence = chunk.Sequence
	}
	var retained int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(SUM(bytes),0) FROM suma_workspace_terminal_outputs WHERE session_id=?`), r.Session.ID).Scan(&retained); err != nil {
		return err
	}
	for retained > term.MaxOutputBytes || r.Session.OutputLastSequence-r.Session.OutputFirstSequence+1 > term.MaxOutputChunks {
		var sequence int64
		var size int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT sequence,bytes FROM suma_workspace_terminal_outputs WHERE session_id=? ORDER BY sequence LIMIT 1`), r.Session.ID).Scan(&sequence, &size); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_workspace_terminal_outputs WHERE session_id=? AND sequence=?`), r.Session.ID, sequence); err != nil {
			return err
		}
		retained -= size
		r.Session.OutputFirstSequence = sequence + 1
	}
	return nil
}
