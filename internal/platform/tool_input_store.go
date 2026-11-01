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
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/dag"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const toolInputInvalid = "Invalid tool input request"
const toolInputChanged = "The tool input source changed; reload before answering"
const toolInputColumns = "id,tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,profile_id,profile_version,profile_payload,operation_id,request_sha256,source_session_sha256,source_request_sha256,version,state,expires_at,assignment,payload"
const toolInputDDL = `CREATE TABLE IF NOT EXISTS suma_tool_inputs(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,dispatch_id VARCHAR(128) NOT NULL,profile_id VARCHAR(64) NOT NULL,profile_version INTEGER NOT NULL,profile_payload TEXT NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,source_session_sha256 VARCHAR(64) NOT NULL,source_request_sha256 VARCHAR(64) NOT NULL,version BIGINT NOT NULL,state VARCHAR(32) NOT NULL,expires_at VARCHAR(40) NOT NULL,assignment TEXT NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,worker_id,operation_id),UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence,source_session_sha256,source_request_sha256))`

func (s *Service) validateToolInputsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND lower(name)='suma_tool_inputs'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_tool_inputs'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_tool_inputs'`
	default:
		return fmt.Errorf("%w: tool input dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, query).Scan(&count); err != nil || count == 0 {
		return err
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered tool input schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_inputs WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: tool input columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want := strings.Split(toolInputColumns, ",")
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact tool input columns required", store.ErrInitialSchema)
	}
	for _, column := range want {
		if !seen[column] {
			return fmt.Errorf("%w: tool input column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, "suma_tool_inputs")
	if err != nil {
		return err
	}
	if primary != "id" || !unique["tenant_id,project_id,worker_id,operation_id"] || !unique["tenant_id,project_id,run_id,node_id,attempt,fence,source_session_sha256,source_request_sha256"] {
		return fmt.Errorf("%w: tool input keys required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeToolInputs(ctx context.Context) error {
	if err := s.validateToolInputsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(toolInputDDL))
	return err
}

type toolInputRow struct {
	value                          ic.ToolInput
	tenant, profileID, requestHash string
	profileVersion                 int
	profile                        tc.Profile
	assignment                     dag.Assignment
}

func (toolInputRow) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "toolInputRow{redacted}") }

func toolInputSourceDigests(r ic.ToolInputRequest) (string, string) {
	if r.Cursor != nil && r.Codex == nil {
		// ACP tool-call IDs and client Prompt IDs are not server RPC IDs.
		// The two Cursor methods share one server-request namespace.
		return digest(stringify(struct {
				Protocol, Runtime, Direction, Session string
				Prompt                                cu.CursorRPCRequestID
			}{r.Protocol, "cursor", "server_request", r.Cursor.SessionID, r.Cursor.PromptID})), digest(stringify(struct {
				Protocol, Runtime, Direction string
				ID                           cu.CursorRPCRequestID
			}{r.Protocol, "cursor", "server_request", r.Cursor.RequestID}))
	}
	if r.Codex == nil {
		return "", ""
	}
	return digest(stringify([]string{r.Protocol, r.Codex.ThreadID})), digest(stringify(struct {
		Protocol string               `json:"protocol"`
		ID       cc.CodexRPCRequestID `json:"id"`
	}{r.Protocol, r.Codex.RequestID}))
}
func toolInputRequestHash(a dag.Assignment, r ic.ToolInputRequest) string {
	return digest(stringify(struct {
		Assignment dag.Assignment
		Request    ic.ToolInputRequest
	}{a, r}))
}
func toolInputSameSource(v ic.ToolInput, a dag.Assignment) bool {
	return v.RunID == a.RunID && v.NodeID == a.NodeID && v.Attempt == a.Attempt && v.Fence == a.Fence && v.WorkerID == a.WorkerID && v.DispatchID == a.DispatchID
}

// Storage and immutable receipts preserve each real protocol's observation.
// The API projects the native Codex turn closure into the shared domain;
// Cursor stores only the domain facts its ACP execution can actually observe.
type toolInputStored struct {
	ic.ToolInput
	Observation json.RawMessage `json:"observation,omitempty"`
}

func toolInputNativeObservation(request ic.ToolInputRequest, observation *delivery.Observation) any {
	if observation == nil {
		return nil
	}
	if request.Codex != nil && request.Cursor == nil {
		return ic.ToCodexObservation(*observation)
	}
	return *observation
}

func toolInputStorageValue(value ic.ToolInput) toolInputStored {
	stored := toolInputStored{ToolInput: value}
	if value.Observation != nil {
		stored.Observation, _ = json.Marshal(toolInputNativeObservation(value.Request, value.Observation))
	}
	return stored
}

func decodeToolInputStorage(raw string, value *ic.ToolInput) error {
	var stored toolInputStored
	if json.Unmarshal([]byte(raw), &stored) != nil || ic.ValidateRequest(stored.Request) != nil {
		return conflict(toolInputChanged)
	}
	*value = stored.ToolInput
	if len(stored.Observation) != 0 {
		if value.Request.Codex != nil {
			var native cc.CodexUserInputObservation
			if woc.Decode(stored.Observation, &native) != nil || cc.ValidateUserInputObservation(native) != nil {
				return conflict(toolInputChanged)
			}
			projected := ic.FromCodexObservation(native)
			value.Observation = &projected
		} else {
			var observed delivery.Observation
			if woc.Decode(stored.Observation, &observed) != nil {
				return conflict(toolInputChanged)
			}
			value.Observation = &observed
		}
	}
	if ic.ValidateInput(*value) != nil {
		return conflict(toolInputChanged)
	}
	return nil
}
func toolInputMatchesBasis(r ic.ToolInputRequest, b toolPermissionBasis) bool {
	if ic.ValidateRequest(r) != nil {
		return false
	}
	if r.Cursor != nil {
		return toolInputProfileProtocol(b.node, b.profile) == cu.InputProtocol && b.profile.Profile.ACPExecution != nil && r.Cursor.ConfigurationSHA256 == b.profile.Profile.ACPExecution.ConfigurationSHA256
	}
	binding, err := dag.CodexBinding(b.node)
	return r.Codex != nil && toolInputProfileProtocol(b.node, b.profile) == cc.UserInputProtocol && err == nil && binding != nil && binding.BindingSHA256 == r.Codex.BindingSHA256
}

func (s *Service) toolInputReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (r toolInputRow, err error) {
	var raw, assignment, profile, operation, sessionSHA, requestSHA, state, expires string
	var version int64
	var source ic.ToolInput
	err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,profile_id,profile_version,profile_payload,operation_id,request_sha256,source_session_sha256,source_request_sha256,version,state,expires_at,assignment,payload,run_id,node_id,attempt,fence,worker_id,dispatch_id FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&r.tenant, &r.profileID, &r.profileVersion, &profile, &operation, &r.requestHash, &sessionSHA, &requestSHA, &version, &state, &expires, &assignment, &raw, &source.RunID, &source.NodeID, &source.Attempt, &source.Fence, &source.WorkerID, &source.DispatchID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	if decodeToolInputStorage(raw, &r.value) != nil || json.Unmarshal([]byte(assignment), &r.assignment) != nil || json.Unmarshal([]byte(profile), &r.profile) != nil || r.value.ID != id || r.value.ProjectID != project || r.value.Version != version || r.value.State != state || r.value.ExpiresAt != expires || r.value.Request.IdempotencyKey != operation || r.value.ProfileSHA256 != r.profile.ProfileSHA256 || !toolInputSameSource(r.value, r.assignment) || !toolInputSameSource(source, r.assignment) || r.requestHash != toolInputRequestHash(r.assignment, r.value.Request) {
		return r, conflict(toolInputChanged)
	}
	a, b := toolInputSourceDigests(r.value.Request)
	if a != sessionSHA || b != requestSHA {
		return r, conflict(toolInputChanged)
	}
	return r, nil
}

func (s *Service) toolInputSaveTx(ctx context.Context, tx *sql.Tx, r toolInputRow, before int64) error {
	r.value.AllowedActions = []string{}
	if ic.ValidateInput(r.value) != nil || r.value.Version != before+1 {
		return invalid(toolInputInvalid)
	}
	if before == 0 {
		sessionSHA, requestSHA := toolInputSourceDigests(r.value.Request)
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_inputs(`+toolInputColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), r.value.ID, r.tenant, r.value.ProjectID, r.value.RunID, r.value.NodeID, r.value.Attempt, r.value.Fence, r.value.WorkerID, r.value.DispatchID, r.profileID, r.profileVersion, stringify(r.profile), r.value.Request.IdempotencyKey, r.requestHash, sessionSHA, requestSHA, r.value.Version, r.value.State, r.value.ExpiresAt, stringify(r.assignment), stringify(toolInputStorageValue(r.value)))
		return err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_inputs SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.value.Version, r.value.State, stringify(toolInputStorageValue(r.value)), r.tenant, r.value.ProjectID, r.value.ID, before)
	return cloudCAS(result, err)
}

func toolInputEnded(o *delivery.Observation) bool {
	return o != nil && (o.RequestResolved || o.ExecutionClosed)
}
func toolInputEffective(r toolInputRow, b toolPermissionBasis, now time.Time) ic.ToolInput {
	v := r.value
	v.AllowedActions = []string{}
	expires, err := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if b.attempt != v.Attempt || b.fence != v.Fence || b.worker != v.WorkerID || b.state != "running" || b.runState != "running" && b.runState != "paused" || toolInputEnded(v.Observation) {
		v.State = "closed"
	} else if err != nil || !now.Before(expires) || b.lease <= now.Unix() || b.deadline <= now.Unix() {
		v.State = "expired"
	}
	return v
}

// Cross-kind admission shares the original execution guard lock. Neither a
// second table nor a new HTTP operation can reset the native attempt limit.
func (s *Service) toolInteractionAdmitTx(ctx context.Context, tx *sql.Tx, a dag.Assignment, sessionSHA, requestSHA string) error {
	var count int
	err := tx.QueryRowContext(ctx, s.q(`SELECT (SELECT COUNT(*) FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=?)+(SELECT COUNT(*) FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=?)`), a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt).Scan(&count)
	if err != nil {
		return err
	}
	if count >= 32 {
		return conflict(toolInputChanged)
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT (SELECT COUNT(*) FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND source_session_sha256=? AND source_request_sha256=?)+(SELECT COUNT(*) FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND source_session_sha256=? AND source_request_sha256=?)`), a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt, a.Fence, sessionSHA, requestSHA, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt, a.Fence, sessionSHA, requestSHA).Scan(&count)
	if err != nil {
		return err
	}
	if count != 0 {
		return conflict(toolInputChanged)
	}
	return nil
}
