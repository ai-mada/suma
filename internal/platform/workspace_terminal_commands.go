package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"

	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func terminalInputID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validWorkspaceTerminalCommand(r term.CommandRequest) bool {
	if !handoffID(r.ProjectID, false) || r.ExpectedVersion < 1 || r.ExpectedVersion > 2147483646 || r.ControlEpoch != 1 || !planningKey(r.IdempotencyKey) {
		return false
	}
	switch r.Action {
	case "input":
		data, err := term.DecodeBytes(r.DataBase64, term.MaxInputBytes)
		return err == nil && base64.StdEncoding.EncodeToString(data) == r.DataBase64 && terminalInputID(r.InputID) && r.InputSequence > 0 && r.InputSequence <= taskConversationMaxSeq && r.Columns == 0 && r.Rows == 0 && r.OutputSequence == 0
	case "resize":
		return r.Columns >= 20 && r.Columns <= 500 && r.Rows >= 5 && r.Rows <= 200 && r.InputID == "" && r.InputSequence == 0 && r.DataBase64 == "" && r.OutputSequence == 0
	case "ack":
		return r.OutputSequence >= 0 && r.OutputSequence <= taskConversationMaxSeq && r.InputID == "" && r.InputSequence == 0 && r.DataBase64 == "" && r.Columns == 0 && r.Rows == 0
	case "stop":
		return r.OutputSequence == 0 && r.InputID == "" && r.InputSequence == 0 && r.DataBase64 == "" && r.Columns == 0 && r.Rows == 0
	}
	return false
}

func (s *Service) CommandWorkspaceTerminal(ctx context.Context, actor Actor, project, id string, request WorkspaceTerminalCommandRequest) (out WorkspaceTerminalCommandReceipt, err error) {
	req := term.CommandRequest(request)
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID || !handoffID(id, false) || !validWorkspaceTerminalCommand(req) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"workspace_terminal_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	r, err := s.workspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", r.Session.TaskID); err != nil {
		return out, err
	}
	if r.Session.Version != req.ExpectedVersion || r.Session.ControlEpoch != req.ControlEpoch {
		return out, conflict(workspaceTerminalChanged)
	}
	if req.Action != "stop" && r.Creator != auth.currentPrincipal.ID {
		return out, forbidden()
	}
	if req.Action == "input" || req.Action == "resize" {
		if r.Session.State != "running" || r.Session.Reason != "" || r.TreeStopped {
			return out, conflict(workspaceTerminalChanged)
		}
		if err = s.checkWorkspaceTerminalExecutionTx(ctx, auth.tx, auth.target.tenantID, project, r.Session.RunID, true); err != nil {
			return out, err
		}
	}
	previous := r.Session.Version
	switch req.Action {
	case "input":
		if req.InputSequence != r.Session.NextInputSequence || r.Session.NextInputSequence >= taskConversationMaxSeq {
			return out, conflict(workspaceTerminalChanged)
		}
		var pending int
		if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_inputs WHERE session_id=? AND state='accepted'`), id).Scan(&pending); err != nil {
			return out, err
		}
		if pending >= 32 {
			return out, conflict("Terminal input queue is full; wait for original delivery receipts")
		}
		data, _ := term.DecodeBytes(req.DataBase64, term.MaxInputBytes)
		sha := digest(string(data))
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_terminal_inputs(session_id,input_id,sequence,sha256,bytes,data_base64,state,bytes_written)VALUES(?,?,?,?,?,?,'accepted',0)`), id, req.InputID, req.InputSequence, sha, len(data), req.DataBase64); err != nil {
			return out, conflict(workspaceTerminalChanged)
		}
		r.Session.NextInputSequence++
		out.Input = &WorkspaceTerminalInputReceipt{InputID: req.InputID, Sequence: req.InputSequence, SHA256: sha, State: "accepted", BytesWritten: 0}
	case "resize":
		r.Session.Columns, r.Session.Rows = req.Columns, req.Rows
	case "ack":
		if req.OutputSequence < r.Session.OutputAckSequence || req.OutputSequence > r.Session.OutputLastSequence {
			return out, conflict(workspaceTerminalChanged)
		}
		r.Session.OutputAckSequence = req.OutputSequence
	case "stop":
		if r.TreeStopped || r.Session.State == "exited" || r.Session.State == "stopped" {
			return out, conflict(workspaceTerminalChanged)
		}
		r.Session.State = "stop_requested"
	}
	r.Session.Version++
	if err = s.saveWorkspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, r, previous); err != nil {
		return out, err
	}
	out.Session, err = s.workspaceTerminalProjectionTx(ctx, auth, r)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "workspace_terminal."+req.Action, id, "Terminal control accepted; shell execution and delivery are separate facts"); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) WorkspaceTerminalOutputs(ctx context.Context, actor Actor, project, id string, after int64) (out WorkspaceTerminalOutputPage, err error) {
	out.Chunks = []WorkspaceTerminalOutputChunk{}
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) || after < 0 || after > taskConversationMaxSeq {
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
	out.SessionID, out.ControlEpoch, out.FirstSequence, out.LastSequence, out.State = id, r.Session.ControlEpoch, r.Session.OutputFirstSequence, r.Session.OutputLastSequence, r.Session.State
	if after > r.Session.OutputLastSequence {
		return out, conflict(workspaceTerminalChanged)
	}
	out.Gap = out.FirstSequence > 0 && after < out.FirstSequence-1
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT sequence,data_base64 FROM suma_workspace_terminal_outputs WHERE session_id=? AND sequence>? ORDER BY sequence LIMIT 64`), id, after)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var chunk WorkspaceTerminalOutputChunk
		if err = rows.Scan(&chunk.Sequence, &chunk.DataBase64); err != nil {
			return out, err
		}
		out.Chunks = append(out.Chunks, chunk)
	}
	return out, rows.Err()
}
