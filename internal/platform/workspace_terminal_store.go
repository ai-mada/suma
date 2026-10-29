package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	workspaceTerminalInvalid       = "Invalid workspace terminal request"
	workspaceTerminalChanged       = "The terminal basis or control version changed; refresh the original session"
	workspaceTerminalProfileReason = "Select a current enabled interactive terminal profile"
	workspaceTerminalActive        = "This task already has an active or uncertain terminal session"
	workspaceTerminalAuthority     = "The terminal creator no longer has current workspace execution authority"
	workspaceTerminalInputUnknown  = "Terminal input delivery is unknown; inspect the original session without replaying input"
	workspaceTerminalStopped       = "The original terminal process tree has not been confirmed stopped"
	workspaceTerminalControls      = "Interactive terminal sessions must use their dedicated controls"
)

func (s *Service) InitializeWorkspaceTerminals(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_sessions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,version INTEGER NOT NULL,state VARCHAR(30) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_active(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,session_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_inputs(session_id VARCHAR(64) NOT NULL,input_id VARCHAR(128) NOT NULL,sequence BIGINT NOT NULL,sha256 VARCHAR(64) NOT NULL,bytes INTEGER NOT NULL,data_base64 TEXT NOT NULL,state VARCHAR(30) NOT NULL,bytes_written INTEGER NOT NULL,PRIMARY KEY(session_id,input_id),UNIQUE(session_id,sequence))`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_outputs(session_id VARCHAR(64) NOT NULL,sequence BIGINT NOT NULL,bytes INTEGER NOT NULL,sha256 VARCHAR(64) NOT NULL,data_base64 TEXT NOT NULL,PRIMARY KEY(session_id,sequence))`,
		`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_exchanges(session_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,sha256 VARCHAR(64) NOT NULL,PRIMARY KEY(session_id,operation_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_terminal_input_pending", "suma_workspace_terminal_inputs", "session_id", "state", "sequence"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_terminal_task_page", "suma_workspace_terminal_sessions", "tenant_id", "project_id", "task_id", "created_at", "id")
}

func workspaceTerminalProfile(p ToolProfile) bool {
	return p.State == "enabled" && tc.SupportsTerminalSessionProfile(p.Profile)
}

func (s *Service) workspaceTerminalTx(ctx context.Context, tx governanceQuery, tenant, project, id string) (out workspaceTerminalRecord, err error) {
	var raw, task, run, state string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT task_id,run_id,version,state,payload FROM suma_workspace_terminal_sessions WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&task, &run, &version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	a := out.Session
	if a.ID != id || a.ProjectID != project || a.TaskID != task || a.RunID != run || a.Version != version || a.State != state {
		return out, conflict(workspaceTerminalChanged)
	}
	return out, nil
}

func (s *Service) saveWorkspaceTerminalTx(ctx context.Context, tx *sql.Tx, tenant string, r workspaceTerminalRecord, previous int) error {
	a := r.Session
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_terminal_sessions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(r), tenant, a.ProjectID, a.ID, previous)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return conflict(workspaceTerminalChanged)
	}
	return err
}

func (s *Service) workspaceTerminalForRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (workspaceTerminalRecord, bool, error) {
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_workspace_terminal_sessions WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		rows, e := tx.QueryContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), tenant, project, run)
		if e != nil {
			return workspaceTerminalRecord{}, false, e
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			if e = rows.Scan(&raw); e != nil {
				return workspaceTerminalRecord{}, false, e
			}
			var n dag.Node
			if json.Unmarshal([]byte(raw), &n) != nil {
				return workspaceTerminalRecord{}, false, conflict(workspaceTerminalInvalid)
			}
			if n.Type == "terminal_session" {
				return workspaceTerminalRecord{}, false, forbidden()
			}
		}
		return workspaceTerminalRecord{}, false, rows.Err()
	}
	if err != nil {
		return workspaceTerminalRecord{}, false, err
	}
	out, err := s.workspaceTerminalTx(ctx, tx, tenant, project, id)
	return out, true, err
}

func (s *Service) WorkspaceTerminalRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (bool, error) {
	_, found, err := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, run)
	return found, err
}

func (s *Service) authorizeWorkspaceTerminalRunCommandTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run, action string) error {
	if auth == nil || auth.target.projectID != project {
		return forbidden()
	}
	tenant := auth.target.tenantID
	_, found, err := s.workspaceTerminalForRunTx(ctx, tx, tenant, project, run)
	if err != nil {
		return err
	}
	if found {
		return conflict(workspaceTerminalControls)
	}
	return nil
}
