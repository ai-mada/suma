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

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

const toolPermissionInvalid = "Invalid tool permission request"
const toolPermissionChanged = "The tool permission source changed; reload before deciding"
const toolPermissionColumns = "id,tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,profile_id,profile_version,profile_payload,operation_id,request_sha256,source_session_sha256,source_request_sha256,version,state,expires_at,assignment,payload"
const toolPermissionDDL = `CREATE TABLE IF NOT EXISTS suma_tool_permissions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,dispatch_id VARCHAR(128) NOT NULL,profile_id VARCHAR(64) NOT NULL,profile_version INTEGER NOT NULL,profile_payload TEXT NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,source_session_sha256 VARCHAR(64) NOT NULL,source_request_sha256 VARCHAR(64) NOT NULL,version BIGINT NOT NULL,state VARCHAR(32) NOT NULL,expires_at VARCHAR(40) NOT NULL,assignment TEXT NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,worker_id,operation_id),UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence,source_session_sha256,source_request_sha256))`

func (s *Service) validateToolPermissionsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name='suma_tool_permissions'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_tool_permissions'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_tool_permissions'`
	default:
		return fmt.Errorf("%w: tool permission dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered tool permission schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_permissions WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: tool permission columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want := strings.Split(toolPermissionColumns, ",")
	seen := map[string]bool{}
	for _, c := range columns {
		seen[c] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact tool permission columns required", store.ErrInitialSchema)
	}
	for _, c := range want {
		if !seen[c] {
			return fmt.Errorf("%w: tool permission column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, "suma_tool_permissions")
	if err != nil {
		return err
	}
	if primary != "id" || !unique["tenant_id,project_id,worker_id,operation_id"] || !unique["tenant_id,project_id,run_id,node_id,attempt,fence,source_session_sha256,source_request_sha256"] {
		return fmt.Errorf("%w: tool permission keys required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializeToolPermissions(ctx context.Context) error {
	if err := s.validateToolPermissionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(toolPermissionDDL))
	return err
}

type toolPermissionRow struct {
	value                          pc.Permission
	tenant, profileID, requestHash string
	profileVersion                 int
	profile                        tc.Profile
	assignment                     dag.Assignment
}

func (toolPermissionRow) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "toolPermissionRow{redacted}")
}
func (s *Service) toolPermissionReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (r toolPermissionRow, err error) {
	var raw, assignment, profile, requestKey, sessionSHA, requestSHA, state, expires string
	var version int64
	var source pc.Permission
	err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,profile_id,profile_version,profile_payload,operation_id,request_sha256,source_session_sha256,source_request_sha256,version,state,expires_at,assignment,payload,run_id,node_id,attempt,fence,worker_id,dispatch_id FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&r.tenant, &r.profileID, &r.profileVersion, &profile, &requestKey, &r.requestHash, &sessionSHA, &requestSHA, &version, &state, &expires, &assignment, &raw, &source.RunID, &source.NodeID, &source.Attempt, &source.Fence, &source.WorkerID, &source.DispatchID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	if json.Unmarshal([]byte(raw), &r.value) != nil || pc.ValidatePermission(r.value) != nil || json.Unmarshal([]byte(assignment), &r.assignment) != nil || json.Unmarshal([]byte(profile), &r.profile) != nil || r.value.ID != id || r.value.ProjectID != project || r.value.Version != version || r.value.State != state || r.value.ExpiresAt != expires || r.value.Request.IdempotencyKey != requestKey || r.value.ProfileSHA256 != r.profile.ProfileSHA256 || !toolPermissionSameSource(r.value, r.assignment) || !toolPermissionSameSource(source, r.assignment) || r.requestHash != toolPermissionRequestHash(r.assignment, r.value.Request) {
		return r, conflict(toolPermissionChanged)
	}
	actualSession, actualRequest, e := pc.RequestSourceDigests(r.value.Request)
	if e != nil || actualSession != sessionSHA || actualRequest != requestSHA {
		return r, conflict(toolPermissionChanged)
	}
	return r, nil
}
func toolPermissionRequestHash(a dag.Assignment, r pc.Request) string {
	return digest(stringify(struct {
		Assignment dag.Assignment
		Request    pc.Request
	}{a, r}))
}
func toolPermissionSameSource(p pc.Permission, a dag.Assignment) bool {
	return p.RunID == a.RunID && p.NodeID == a.NodeID && p.Attempt == a.Attempt && p.Fence == a.Fence && p.WorkerID == a.WorkerID && p.DispatchID == a.DispatchID
}

func toolPermissionRequestMatchesBasis(request pc.Request, basis toolPermissionBasis) bool {
	if pc.ValidateRequest(request) != nil || request.Protocol != basis.profile.Profile.Protocol {
		return false
	}
	if request.Protocol == cc.Protocol {
		binding, err := dag.CodexBinding(basis.node)
		return err == nil && binding != nil && request.Codex != nil && request.Codex.BindingSHA256 == binding.BindingSHA256
	}
	if request.Protocol == cl.Protocol {
		return request.Claude != nil && supportsClaudeSDKExecution(basis.profile) && request.Claude.ConfigurationSHA256 == basis.profile.Profile.ClaudeSDKExecution.ConfigurationSHA256
	}
	return request.Protocol == ac.Protocol && request.ACP != nil
}

func toolPermissionChoiceAction(choice pc.Choice) string {
	if choice.ACP != nil {
		return choice.ACP.Kind
	}
	if choice.Codex != nil {
		return choice.Codex.Decision
	}
	if choice.Claude != nil {
		return choice.Claude.Outcome
	}
	return ""
}

func toolPermissionObservationEnded(observation *cc.CodexApprovalObservation) bool {
	return observation != nil && (observation.RequestResolved || observation.ItemStatus == "completed" || observation.ItemStatus == "failed" || observation.ItemStatus == "declined")
}

// Each native protocol owns its closure facts. Claude's callback return is not
// a Codex request acknowledgement, so it only uses the explicit execution end.
func toolPermissionHasEnded(value pc.Permission) bool {
	return toolPermissionObservationEnded(value.Observation) || value.ClaudeObservation != nil && value.ClaudeObservation.ExecutionClosed
}
func (s *Service) toolPermissionSaveTx(ctx context.Context, tx *sql.Tx, r toolPermissionRow, before int64) error {
	r.value.AllowedActions = []string{}
	if pc.ValidatePermission(r.value) != nil || r.value.Version != before+1 {
		return invalid(toolPermissionInvalid)
	}
	if before == 0 {
		sessionSHA, requestSHA, err := pc.RequestSourceDigests(r.value.Request)
		if err != nil {
			return invalid(toolPermissionInvalid)
		}
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_permissions(`+toolPermissionColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), r.value.ID, r.tenant, r.value.ProjectID, r.value.RunID, r.value.NodeID, r.value.Attempt, r.value.Fence, r.value.WorkerID, r.value.DispatchID, r.profileID, r.profileVersion, stringify(r.profile), r.value.Request.IdempotencyKey, r.requestHash, sessionSHA, requestSHA, r.value.Version, r.value.State, r.value.ExpiresAt, stringify(r.assignment), stringify(r.value))
		return err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_permissions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.value.Version, r.value.State, stringify(r.value), r.tenant, r.value.ProjectID, r.value.ID, before)
	return cloudCAS(result, err)
}

type toolPermissionBasis struct {
	node                   dag.Node
	profile                ToolProfile
	poolOwner              string
	runState, state        string
	attempt                int
	fence, lease, deadline int64
	worker                 string
}

func (s *Service) toolPermissionBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, a dag.Assignment) (b toolPermissionBasis, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT r.state,n.state,n.attempt,n.fence,n.worker_id,n.lease_until,n.spec FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), tenant, project, a.RunID, a.NodeID).Scan(&b.runState, &b.state, &b.attempt, &b.fence, &b.worker, &b.lease, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return b, missing()
	}
	if err != nil {
		return b, err
	}
	if json.Unmarshal([]byte(raw), &b.node) != nil {
		return b, conflict(toolPermissionChanged)
	}
	id, sha, bound, e := dag.ToolBinding(b.node)
	if e != nil || !bound || b.node.Type != "agent" || len(b.node.Command) != 1 {
		return b, conflict(toolPermissionChanged)
	}
	b.profile, err = s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		return b, err
	}
	qualified := false
	switch b.profile.Profile.Protocol {
	case ac.Protocol:
		qualified = acpNodeMatchesProfile(b.node, b.profile) && tc.SupportsACPExecutionProfile(b.profile.Profile)
	case cc.Protocol:
		binding, e := dag.CodexBinding(b.node)
		qualified = e == nil && binding != nil && supportsCodexExecution(b.profile)
	case cl.Protocol:
		qualified = supportsClaudeSDKExecution(b.profile) && claudeSDKNodeMatchesProfile(b.node, b.profile)
	}
	if !qualified || b.profile.Profile.ProfileSHA256 != sha || b.profile.WorkerID != a.WorkerID {
		return b, conflict(toolPermissionChanged)
	}
	// Pool callbacks deliberately carry only the original execution identity.
	// Their command and configuration authority comes from the protected node.
	if a.PoolID == "" || a.Type != "" || a.Command != nil || a.Config != nil {
		if a.Type != b.node.Type || stringify(a.Command) != stringify(b.node.Command) || stringify(a.Config) != stringify(b.node.Config) || a.Idempotent != b.node.Idempotent {
			return b, conflict(toolPermissionChanged)
		}
	}
	if a.PoolID != "" {
		if err = tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), a.PoolID).Scan(&b.poolOwner); err != nil {
			return b, forbidden()
		}
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT w.deadline_at FROM dag_retry_windows w JOIN dag_node_dependencies d ON d.run_id=w.run_id AND d.node_id=w.node_id AND d.generation=w.generation WHERE w.run_id=? AND w.node_id=?`), a.RunID, a.NodeID).Scan(&b.deadline)
	if err != nil {
		return b, conflict(toolPermissionChanged)
	}
	return b, nil
}
func (s *Service) toolPermissionDispatchTx(ctx context.Context, tx *sql.Tx, tenant, project string, a dag.Assignment) error {
	if a.DispatchID == "" || a.TenantID != tenant || a.ProjectID != project || a.Attempt < 1 || a.Fence < 1 {
		return conflict(toolPermissionChanged)
	}
	if a.PoolID != "" {
		return dag.ValidatePoolAssignmentEnvelopeTx(ctx, tx, s.Dialect, a)
	}
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_dispatches WHERE id=? AND tenant_id=? AND project_id=? AND worker_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), a.DispatchID, tenant, project, a.WorkerID, a.RunID, a.NodeID, a.Attempt, a.Fence).Scan(&raw); err != nil {
		return conflict(toolPermissionChanged)
	}
	var stored dag.Assignment
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.DispatchID != a.DispatchID || stored.WorkerID != a.WorkerID || stored.RunID != a.RunID || stored.NodeID != a.NodeID || stored.Attempt != a.Attempt || stored.Fence != a.Fence {
		return conflict(toolPermissionChanged)
	}
	return nil
}
func (s *Service) toolPermissionCurrentProfileTx(ctx context.Context, tx *sql.Tx, tenant, project string, b toolPermissionBasis, a dag.Assignment) error {
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, b.profile)
	if err != nil {
		return err
	}
	if !current.Available {
		return forbidden()
	}
	if a.PoolID != "" {
		scope, e := s.currentPoolActiveScopeTx(ctx, tx, a.PoolID, tenant, project)
		if e != nil {
			return e
		}
		if scope.GrantID != a.PoolGrantID || scope.GrantVersion != a.PoolGrantVersion {
			return forbidden()
		}
		var status string
		if e = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_execution_pools WHERE id=?`)), a.PoolID).Scan(&status); e != nil || status != "active" {
			return forbidden()
		}
	}
	return nil
}
func toolPermissionEffective(r toolPermissionRow, b toolPermissionBasis, now time.Time) pc.Permission {
	p := r.value
	p.AllowedActions = []string{}
	if b.attempt != p.Attempt || b.fence != p.Fence || b.worker != p.WorkerID || b.state != "running" || (b.runState != "running" && b.runState != "paused") {
		p.State = "superseded"
	} else if b.lease <= now.Unix() || b.deadline <= now.Unix() {
		p.State = "expired"
	} else if p.State == "pending" && toolPermissionHasEnded(p) {
		p.State = "superseded"
	}
	return p
}
func (s *Service) toolPermissionWorkAllowedTx(ctx context.Context, tx *sql.Tx, tenant, project string, b toolPermissionBasis) (bool, error) {
	allow := b.runState == "running"
	for _, scope := range []string{"organization", "project"} {
		policy, e := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if e != nil {
			return false, e
		}
		if policy.Mode == "enforce" {
			allow = false
		}
	}
	return allow, nil
}
func (s *Service) toolPermissionDeliveryTx(ctx context.Context, tx *sql.Tx, r toolPermissionRow, b toolPermissionBasis, now time.Time) (pc.Permission, error) {
	p := toolPermissionEffective(r, b, now)
	if p.State == "decided" && toolPermissionHasEnded(p) {
		p.State = "superseded"
	}
	if p.State == "decided" && p.Decision != nil && pc.IsGrant(p.Decision.Choice) {
		allowed, e := s.toolPermissionWorkAllowedTx(ctx, tx, r.tenant, p.ProjectID, b)
		if e != nil {
			return pc.Permission{}, e
		}
		if !allowed {
			p.State = "superseded"
		}
	}
	return p, nil
}
func (s *Service) toolPermissionAllowsTx(ctx context.Context, tx *sql.Tx, tenant, project string, p pc.Permission, b toolPermissionBasis) ([]string, error) {
	out := []string{}
	if p.State != "pending" {
		return out, nil
	}
	allow, err := s.toolPermissionWorkAllowedTx(ctx, tx, tenant, project, b)
	if err != nil {
		return nil, err
	}
	if toolPermissionHasEnded(p) {
		return out, nil
	}
	for _, action := range pc.AllowedActions(p.Request) {
		if !allow && (action == "allow_once" || action == "accept") {
			continue
		}
		out = append(out, action)
	}
	return out, nil
}
