package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const steeringInvalid = "Invalid turn steering request"
const steeringChanged = "The live execution or discussion source changed; refresh before sending"
const steeringSessionColumns = "id,tenant_id,project_id,task_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,operation_id,request_sha256,version,state,expires_at,payload"
const steeringInstructionColumns = "id,tenant_id,project_id,task_id,session_id,message_id,actor_id,version,payload"
const steeringSessionDDL = `CREATE TABLE IF NOT EXISTS suma_steering_sessions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,dispatch_id VARCHAR(128) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,version BIGINT NOT NULL,state VARCHAR(20) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,worker_id,operation_id),UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence))`
const steeringInstructionDDL = `CREATE TABLE IF NOT EXISTS suma_steering_instructions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,session_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL,actor_id VARCHAR(64) NOT NULL,version BIGINT NOT NULL,payload TEXT NOT NULL,UNIQUE(session_id,message_id))`

func (s *Service) validateTurnSteeringInitialSchema(ctx context.Context) error {
	for _, def := range []struct {
		table, columns string
		keys           []string
	}{{"suma_steering_sessions", steeringSessionColumns, []string{"tenant_id,project_id,worker_id,operation_id", "tenant_id,project_id,run_id,node_id,attempt,fence"}}, {"suma_steering_instructions", steeringInstructionColumns, []string{"session_id,message_id"}}} {
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND lower(name)=?`
		switch s.Dialect {
		case "sqlite":
		case "postgres":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
		case "oceanbase":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
		default:
			return fmt.Errorf("%w: steering dialect", store.ErrInitialSchema)
		}
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(query), def.table).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			continue
		}
		var version string
		if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&version); err != nil || version != store.InitialDataContract {
			return fmt.Errorf("%w: steering baseline", store.ErrInitialSchema)
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+def.table+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: steering columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		want := strings.Split(def.columns, ",")
		seen := map[string]bool{}
		for _, c := range columns {
			seen[c] = true
		}
		if len(columns) != len(want) {
			return fmt.Errorf("%w: exact steering columns", store.ErrInitialSchema)
		}
		for _, c := range want {
			if !seen[c] {
				return fmt.Errorf("%w: steering column", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, def.table)
		if err != nil {
			return err
		}
		if primary != "id" {
			return fmt.Errorf("%w: steering primary key", store.ErrInitialSchema)
		}
		for _, key := range def.keys {
			if !unique[key] {
				return fmt.Errorf("%w: steering uniqueness", store.ErrInitialSchema)
			}
		}
	}
	return nil
}
func (s *Service) InitializeTurnSteering(ctx context.Context) error {
	if err := s.validateTurnSteeringInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, ddl := range []string{steeringSessionDDL, steeringInstructionDDL} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_steering_sessions_task", "suma_steering_sessions", "tenant_id", "project_id", "task_id", "run_id", "state"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_steering_instructions_message", "suma_steering_instructions", "tenant_id", "project_id", "task_id", "message_id", "id")
}

type steeringSessionRecord struct {
	Session        sc.SteeringSession `json:"session"`
	Assignment     dag.Assignment     `json:"assignment"`
	ProfileID      string             `json:"profile_id"`
	ProfileVersion int                `json:"profile_version"`
	Profile        tc.Profile         `json:"profile"`
	RequestHash    string             `json:"request_hash"`
	OperationID    string             `json:"operation_id"`
}
type steeringInstructionRecord struct {
	Instruction sc.SteeringInstruction        `json:"instruction"`
	Caller      expertTeamCredentialReference `json:"caller"`
	TaskVersion int                           `json:"task_version"`
}
type steeringQuery interface {
	governanceQuery
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (steeringSessionRecord) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, "steeringSessionRecord{redacted}")
}
func (steeringInstructionRecord) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, "steeringInstructionRecord{redacted}")
}

func (s *Service) steeringSessionTx(ctx context.Context, q governanceQuery, tenant, project, id string) (r steeringSessionRecord, err error) {
	var raw, task, run, node, worker, dispatch, key, hash, state, expires string
	var version, fence int64
	var attempt int
	err = q.QueryRowContext(ctx, s.q(`SELECT payload,task_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,operation_id,request_sha256,version,state,expires_at FROM suma_steering_sessions WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw, &task, &run, &node, &attempt, &fence, &worker, &dispatch, &key, &hash, &version, &state, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	if json.Unmarshal([]byte(raw), &r) != nil || sc.ValidateSession(r.Session) != nil || r.Session.ID != id || r.Session.ProjectID != project || r.Session.TaskID != task || r.Session.RunID != run || r.Session.NodeID != node || r.Session.Attempt != attempt || r.Session.Fence != fence || r.Session.WorkerID != worker || r.Session.DispatchID != dispatch || r.Session.Version != version || r.Session.State != state || r.Session.ExpiresAt != expires || r.OperationID != key || r.RequestHash != hash || r.Profile.ProfileSHA256 != r.Session.ProfileSHA256 || !steeringSameAssignment(r.Session, r.Assignment) {
		return r, conflict(steeringChanged)
	}
	return r, nil
}
func (s *Service) steeringInstructionTx(ctx context.Context, q governanceQuery, tenant, project, id string) (r steeringInstructionRecord, err error) {
	var raw, task, session, message, actor string
	var version int64
	err = q.QueryRowContext(ctx, s.q(`SELECT payload,task_id,session_id,message_id,actor_id,version FROM suma_steering_instructions WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw, &task, &session, &message, &actor, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	v := &r.Instruction
	if json.Unmarshal([]byte(raw), &r) != nil || sc.ValidateInstruction(*v) != nil || v.ID != id || v.ProjectID != project || v.TaskID != task || v.SessionID != session || v.Message.ID != message || v.ActorID != actor || v.Version != version || r.Caller.Kind != actorBrowser || r.Caller.SubjectID != actor || r.TaskVersion < 1 {
		return r, conflict(steeringChanged)
	}
	return r, nil
}
func (s *Service) saveSteeringSessionTx(ctx context.Context, tx *sql.Tx, tenant string, r steeringSessionRecord, before int64) error {
	v := r.Session
	v.AllowedActions = []string{}
	r.Session = v
	if sc.ValidateSession(v) != nil || v.Version != before+1 {
		return invalid(steeringInvalid)
	}
	if before == 0 {
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_steering_sessions(`+steeringSessionColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), v.ID, tenant, v.ProjectID, v.TaskID, v.RunID, v.NodeID, v.Attempt, v.Fence, v.WorkerID, v.DispatchID, r.OperationID, r.RequestHash, v.Version, v.State, v.ExpiresAt, stringify(r))
		return err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_steering_sessions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), v.Version, v.State, stringify(r), tenant, v.ProjectID, v.ID, before)
	return cloudCAS(result, err)
}
func (s *Service) saveSteeringInstructionTx(ctx context.Context, tx *sql.Tx, tenant string, r steeringInstructionRecord, before int64) error {
	v := r.Instruction
	if sc.ValidateInstruction(v) != nil || v.Version != before+1 {
		return invalid(steeringInvalid)
	}
	if before == 0 {
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_steering_instructions(`+steeringInstructionColumns+`)VALUES(?,?,?,?,?,?,?,?,?)`), v.ID, tenant, v.ProjectID, v.TaskID, v.SessionID, v.Message.ID, v.ActorID, v.Version, stringify(r))
		return err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_steering_instructions SET version=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), v.Version, stringify(r), tenant, v.ProjectID, v.ID, before)
	return cloudCAS(result, err)
}
func steeringSameAssignment(v sc.SteeringSession, a dag.Assignment) bool {
	return v.RunID == a.RunID && v.NodeID == a.NodeID && v.Attempt == a.Attempt && v.Fence == a.Fence && v.WorkerID == a.WorkerID && v.DispatchID == a.DispatchID
}
func steeringSourceMatches(v sc.SteeringSession, b toolPermissionBasis) bool {
	binding, err := dag.CodexBinding(b.node)
	return err == nil && binding != nil && b.profile.Profile.Protocol == cc.Protocol && supportsCodexExecution(b.profile) && v.ProfileSHA256 == b.profile.Profile.ProfileSHA256 && v.Source.BindingSHA256 == binding.BindingSHA256
}

func (s *Service) steeringTaskTx(ctx context.Context, tx *sql.Tx, tenant, project, id, run string, version int) error {
	var current, status string
	var actual int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT run_id,status,version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&current, &status, &actual); err != nil {
		return err
	}
	if current != run || status == "accepted" || version > 0 && actual != version {
		return conflict(steeringChanged)
	}
	_, bound, err := s.taskRunSourceTx(ctx, tx, tenant, project, id, run)
	if err != nil {
		return err
	}
	if bound == nil {
		_, err = s.continuationProvenanceTx(ctx, tx, tenant, project, id, run)
	}
	return err
}
func (s *Service) steeringFreshTx(ctx context.Context, tx *sql.Tx, tenant, project string, r steeringSessionRecord, b toolPermissionBasis, now time.Time) error {
	v := r.Session
	expires, err := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if err != nil || v.State != "active" || !now.Before(expires) || b.state != "running" || b.runState != "running" || b.attempt != v.Attempt || b.fence != v.Fence || b.worker != v.WorkerID || b.lease <= now.Unix() || b.deadline <= now.Unix() || r.ProfileVersion != b.profile.Version || !tc.EqualProfile(r.Profile, b.profile.Profile) || !steeringSourceMatches(v, b) {
		return conflict(steeringChanged)
	}
	if err = s.toolPermissionCurrentProfileTx(ctx, tx, tenant, project, b, r.Assignment); err != nil {
		return err
	}
	if err = s.steeringTaskTx(ctx, tx, tenant, project, v.TaskID, v.RunID, 0); err != nil {
		return err
	}
	allowed, err := s.toolPermissionWorkAllowedTx(ctx, tx, tenant, project, b)
	if err != nil {
		return err
	}
	if !allowed {
		return forbidden()
	}
	return nil
}

func (s *Service) steeringInstructionsTx(ctx context.Context, q steeringQuery, tenant, project, session string) (out []steeringInstructionRecord, err error) {
	out = []steeringInstructionRecord{}
	rows, err := q.QueryContext(ctx, s.q(`SELECT id FROM suma_steering_instructions WHERE tenant_id=? AND project_id=? AND session_id=? ORDER BY id LIMIT 33`), tenant, project, session)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > 32 {
		return out, conflict(steeringChanged)
	}
	for _, id := range ids {
		r, e := s.steeringInstructionTx(ctx, q, tenant, project, id)
		if e != nil {
			return out, e
		}
		out = append(out, r)
	}
	return out, nil
}
