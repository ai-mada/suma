package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/displaycontract"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func (s *Service) InitializeToolProfiles(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_tool_profiles(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,name VARCHAR(200) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,profile TEXT NOT NULL,enabled_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_tool_inventories(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,observed_at VARCHAR(40) NOT NULL,items TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,worker_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return fmt.Errorf("tool profile migration: %w", err)
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_profile_scope", "suma_tool_profiles", "tenant_id", "project_id", "id")
}

const toolProfileColumns = `id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at`

func scanToolProfile(row automationScanner) (ToolProfile, error) {
	var p ToolProfile
	var raw string
	err := row.Scan(&p.ID, &p.ProjectID, &p.WorkerID, &p.Name, &p.Version, &p.State, &raw, &p.EnabledBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, missing()
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &p.Profile)
	}
	return p, err
}
func (s *Service) toolProfileTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string) (ToolProfile, error) {
	return scanToolProfile(tx.QueryRowContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
}

// toolReason carries producer identity through project and shared-pool checks.
// It never derives authority or display identity from arbitrary stored prose.
type toolReason struct {
	source     string
	diagnostic *displaycontract.DisplayDiagnostic
}

func newToolReason(source, suffix string, params map[string]any) toolReason {
	return toolReason{source: source, diagnostic: displaycontract.BindToolReason(source, "display.tool."+suffix, params)}
}
func withToolReason(out ToolProfile, source, suffix string, params map[string]any) ToolProfile {
	reason := newToolReason(source, suffix, params)
	out.Reason, out.ReasonDiagnostic = reason.source, reason.diagnostic
	return out
}

// decodeToolProfileReceipt isolates optional presentation from the authoritative
// saved response. Unsupported hints cannot poison replay, while business JSON
// errors still propagate. Nothing is recomputed or written back to persistence.
func decodeToolProfileReceipt(raw string) (ToolProfile, error) {
	type response ToolProfile
	var saved struct {
		response
		ReasonDiagnostic json.RawMessage `json:"reason_diagnostic"`
	}
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return ToolProfile{}, err
	}
	out := ToolProfile(saved.response)
	if len(saved.ReasonDiagnostic) != 0 {
		var hint displaycontract.DisplayDiagnostic
		if json.Unmarshal(saved.ReasonDiagnostic, &hint) == nil {
			out.ReasonDiagnostic = displaycontract.ValidateToolReason(out.Reason, &hint)
		}
	}
	return out, nil
}

// toolWorkerReasonTx checks the same registered execution identity used by
// release operations, without granting a tool-only worker release capabilities.
func (s *Service) toolWorkerReasonTx(ctx context.Context, tx *sql.Tx, p Principal, project, worker string, clock time.Time) (toolReason, error) {
	var state, expires, creator, tokenHash string
	err := tx.QueryRowContext(ctx, s.q(`SELECT w.status,x.expires_at,w.created_by,w.token_hash FROM suma_release_workers w JOIN suma_sessions x ON x.token_hash=w.token_hash AND x.tenant_id=w.tenant_id JOIN suma_token_scopes t ON t.token_hash=w.token_hash AND t.kind='runner' AND t.project_id=w.project_id WHERE w.tenant_id=? AND w.project_id=? AND w.worker_id=?`), p.TenantID, project, worker).Scan(&state, &expires, &creator, &tokenHash)
	if errors.Is(err, sql.ErrNoRows) {
		return s.poolToolWorkerReasonTx(ctx, tx, p.TenantID, project, worker, clock)
	}
	if err != nil {
		return toolReason{}, err
	}
	until, err := time.Parse(time.RFC3339Nano, expires)
	if state != "active" || err != nil || !until.After(clock) {
		return newToolReason("执行节点凭证已过期或撤销", "worker_expired", nil), nil
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, p.TenantID, creator, project, true, false); err != nil {
		if automationAuthorityFailure(err) {
			return newToolReason("执行节点登记人的当前项目权限已失效", "worker_creator_revoked", nil), nil
		}
		return toolReason{}, err
	}
	credential, credentialErr := s.readCredential(ctx, tx, tokenHash, true)
	if credentialErr != nil {
		var api *APIError
		if errors.As(credentialErr, &api) && (api.Status == 401 || api.Status == 403 || api.Status == 404) {
			return newToolReason("执行节点凭证已过期或撤销", "worker_expired", nil), nil
		}
		return toolReason{}, credentialErr
	}
	if credential.kind != actorProjectRunner || credential.ref.subjectID != creator || credential.ref.tenantID != p.TenantID || credential.ref.projectID != project || credential.expires.Unix() <= clock.Unix() {
		return newToolReason("执行节点凭证已过期或撤销", "worker_expired", nil), nil
	}

	return toolReason{}, nil
}

func (s *Service) toolProfileObservationTx(ctx context.Context, tx *sql.Tx, p Principal, project string, out ToolProfile) (ToolProfile, error) {
	return s.toolProfileObservationLockedTx(ctx, tx, p.TenantID, project, out)
}

func (s *Service) toolProfileObservationLockedTx(ctx context.Context, tx *sql.Tx, tenant, project string, out ToolProfile) (ToolProfile, error) {
	p := Principal{TenantID: tenant} // projection/scope only; never an authority source
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Available = false
	out.ReasonDiagnostic = nil
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), p.TenantID, project, out.WorkerID).Scan(&out.LastSeenAt, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	var items []toolcontract.Observation
	if err == nil {
		if err = json.Unmarshal([]byte(raw), &items); err != nil {
			return out, err
		}
		for _, o := range items {
			if o.Profile.CommandID == out.Profile.CommandID {
				copy := o
				out.Observation = &copy
				break
			}
		}
	}
	switch out.State {
	case "revoked":
		out = withToolReason(out, "工具档案已永久撤销", "profile_revoked", nil)
		return out, nil
	case "paused":
		out = withToolReason(out, "工具档案已暂停，尚未授权派发", "profile_paused", nil)
		return out, nil
	case "enabled":
	default:
		return out, invalid("工具档案状态无效")
	}
	if err = s.toolActivationLockedTx(ctx, tx, tenant, project, out); err != nil {
		if automationAuthorityFailure(err) {
			out = withToolReason(out, "工具档案启用人的当前审批权限已失效", "activation_revoked", nil)
			return out, nil
		}
		return out, err
	}
	reason, err := s.toolWorkerReasonTx(ctx, tx, p, project, out.WorkerID, clock)
	if err != nil {
		return out, err
	}
	if reason.source != "" {
		out.Reason, out.ReasonDiagnostic = reason.source, reason.diagnostic
		return out, nil
	}
	observed, parseErr := time.Parse(time.RFC3339, out.LastSeenAt)
	if out.LastSeenAt == "" {
		out = withToolReason(out, "执行节点尚未上报工具清单", "inventory_missing", nil)
		return out, nil
	}
	if parseErr != nil || observed.After(clock) || !clock.Before(observed.Add(60*time.Second)) {
		out = withToolReason(out, "工具清单已过期，等待执行节点重新探测", "inventory_stale", nil)
		return out, nil
	}
	if out.Observation == nil {
		out = withToolReason(out, "当前执行节点未声明此命令", "command_missing", nil)
		return out, nil
	}
	if !toolcontract.EqualProfile(out.Observation.Profile, out.Profile) {
		out = withToolReason(out, "当前工具配置、版本或程序摘要与固定档案不匹配", "profile_mismatch", nil)
		return out, nil
	}
	if !out.Observation.Available {
		out = withToolReason(out, "执行节点报告工具不可用："+out.Observation.Reason, "reported_unavailable", map[string]any{"reason": out.Observation.Reason})
		return out, nil
	}
	out.Available = true
	out = withToolReason(out, "执行节点已报告可领取；模型认证、执行与验收结果需分别核验", "dispatchable", nil)
	return out, nil
}

func (s *Service) getToolProfileCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string) (ToolProfile, error) {
	if auth == nil {
		return ToolProfile{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return ToolProfile{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	var out ToolProfile
	out, err = s.toolProfileTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	return s.toolProfileObservationTx(ctx, tx, p, project, out)
}
func (s *Service) listToolProfilesCurrentTx(ctx context.Context, auth *authorizedCommand, project string, limit int, cursor string) (ToolProfileList, error) {
	if auth == nil {
		return ToolProfileList{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: ""}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return ToolProfileList{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := ToolProfileList{Items: []ToolProfile{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	last, err := automationDecodeCursor(p, project, "tool_profiles", cursor)
	if err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_profiles WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id>? ORDER BY id LIMIT ?`), p.TenantID, project, last, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		pr, e := scanToolProfile(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, pr)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, project, "tool_profiles", out.Items[len(out.Items)-1].ID)
	}
	for i, pr := range out.Items {
		out.Items[i], err = s.toolProfileObservationTx(ctx, tx, p, project, pr)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Service) CreateToolProfile(ctx context.Context, actor Actor, project string, req ToolProfileCreateRequest) (ToolProfile, error) {
	var out ToolProfile
	if req.ProjectID != project || !toolcontract.ValidCommandID(req.WorkerID) || req.Name == "" || strings.TrimSpace(req.Name) != req.Name || !ingressText(req.Name, 200) {
		return out, invalid("工具名称、执行节点或项目无效")
	}
	if err := toolcontract.ValidateProfile(req.Profile, true); err != nil {
		return out, invalidDiagnostic("validation.tool_profile_detail", "固定工具档案无效："+err.Error(), map[string]any{"detail": err.Error()})
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"tool_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		return decodeToolProfileReceipt(hash)
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	reason, err := s.toolWorkerReasonTx(ctx, tx, p, project, req.WorkerID, clock)
	if err != nil {
		return out, err
	}
	if reason.source != "" {
		return out, conflict(reason.source)
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_profiles WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&count); err != nil {
		return out, err
	}
	if count >= 1000 {
		return out, conflict("项目工具档案已达 1000 项限制")
	}
	at := automationStamp(clock)
	out = ToolProfile{ID: newID(), ProjectID: project, WorkerID: req.WorkerID, Name: req.Name, Version: 1, State: "paused", Profile: req.Profile, CreatedAt: at, UpdatedAt: at, Reason: "工具档案已暂停，尚未授权派发"}
	out = withToolReason(out, out.Reason, "profile_paused", nil)
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_profiles(id,tenant_id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), out.ID, p.TenantID, project, out.WorkerID, out.Name, 1, out.State, stringify(out.Profile), "", at, at)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_profile.created", out.ID, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) CommandToolProfile(ctx context.Context, actor Actor, project, id string, req ToolProfileCommandRequest) (ToolProfile, error) {
	var out ToolProfile
	if req.ProjectID != project || !ingressVersion(req.ExpectedVersion) || (req.Action != "enable" && req.Action != "pause" && req.Action != "revoke") {
		return out, invalid("工具操作、项目或版本无效")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"tool_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		return decodeToolProfileReceipt(hash)
	}
	out, err = s.toolProfileTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.State == "revoked" {
		return out, conflict("工具档案版本已变化或已撤销")
	}
	switch req.Action {
	case "enable":
		out.State = "enabled"
		out.EnabledBy = p.ID
	case "pause":
		out.State = "paused"
	case "revoke":
		out.State = "revoked"
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Version++
	out.UpdatedAt = automationStamp(clock)
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_profiles SET version=?,state=?,enabled_by=?,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.State, out.EnabledBy, out.UpdatedAt, p.TenantID, project, id, req.ExpectedVersion)
	if err != nil {
		return out, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return out, err
	}
	if changed != 1 {
		return out, conflict("工具档案版本已变化")
	}
	out, err = s.toolProfileObservationTx(ctx, tx, p, project, out)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_profile."+req.Action, id, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// validateToolDispatchTx is also used by immutable task preview/start. The user
// needs write authority; approval belongs to the current profile enabling actor.
func (s *Service) validateToolDispatchTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, id string, version int, sha string) (ToolProfile, error) {
	var out ToolProfile
	if auth == nil {
		return out, forbidden()
	}
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return out, err
	}
	if auth.target.projectID != project {
		return out, forbidden()
	}
	p := auth.currentPrincipal
	if _, err := s.currentProjectMembershipLockedTx(ctx, tx, p.TenantID, p.ID, project, true, false); err != nil {
		return out, err
	}
	out, err := s.toolProfileTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != version || out.Profile.ProfileSHA256 != sha {
		return out, conflict("工具档案依据已变化，请重新预览")
	}
	out, err = s.toolProfileObservationTx(ctx, tx, p, project, out)
	if err != nil {
		return out, err
	}
	if !out.Available {
		return out, conflict(out.Reason)
	}
	if err := s.checkOpenCodeBudgetTx(ctx, tx, p.TenantID, project, out); err != nil {
		return out, err
	}
	return out, nil
}
