package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolReplacementInvalid() error {
	return invalidDiagnostic("validation.tool_profile_replacement", "工具配置换新请求或分页参数无效", nil)
}
func toolReplacementChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_profile_replacement_changed", "工具配置换新依据已变化，请重新核对当前配置", nil}, key: "execution.tool_profile.replacement_changed"}
}
func toolReplacementInventory() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_profile_replacement_inventory", "目标节点尚未报告新鲜且完整可用的匹配配置", nil}, key: "execution.tool_profile.replacement_inventory"}
}
func toolReplacementSnapshotLimit() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_profile_replacement_snapshot_limit", "配置快照超过2MiB限制，未提交换新", nil}, key: "execution.tool_profile.replacement_snapshot_limit"}
}

func (s *Service) ReplaceToolProfile(ctx context.Context, actor Actor, project, id string, request ToolProfileReplacementRequest) (ToolProfileReplacement, error) {
	return s.replaceToolProfile(ctx, actor, project, id, func() (ToolProfileReplacementRequest, error) { return request, nil })
}

// The parser executes only after current authorization. Receipt recovery then
// precedes new source CAS and inventory checks, so an uncertain successful POST
// cannot silently become another configuration change after a later edit.
func (s *Service) replaceToolProfile(ctx context.Context, actor Actor, project, id string, parse func() (ToolProfileReplacementRequest, error)) (out ToolProfileReplacement, err error) {
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
	request, err := parse()
	if err != nil {
		return out, err
	}
	if request.ProjectID != project || !ValidToolProfileReplacementRequest(request, id) {
		return out, toolReplacementInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"tool_profile_replacement": id, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		out, err = decodeToolProfileReplacement(hash)
		if err == nil && (!ToolProfileReplacementMatches(out, request, id) || out.CreatedBy != auth.currentPrincipal.ID) {
			err = toolReplacementChanged()
		}
		return out, err
	}
	original, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, id)
	if err != nil {
		return out, err
	}
	if original.Version != request.ExpectedVersion || original.State == "revoked" {
		return out, toolReplacementChanged()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	reason, err := s.toolWorkerReasonTx(ctx, tx, auth.currentPrincipal, project, request.WorkerID, clock)
	if err != nil {
		return out, err
	}
	if reason.source != "" {
		return out, toolReplacementInventory()
	}
	if err = s.replacementInventoryTx(ctx, tx, auth.target.tenantID, project, request.WorkerID, request.Profile, clock); err != nil {
		return out, err
	}
	var target ToolProfile
	disposition := "reused"
	if request.Reuse != nil {
		target, err = s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, request.Reuse.ProfileID)
		if err != nil {
			return out, err
		}
		if target.ID == original.ID || target.State == "revoked" || target.Version != request.Reuse.ExpectedVersion || target.WorkerID != request.WorkerID || !toolcontract.EqualProfile(target.Profile, request.Profile) {
			return out, toolReplacementChanged()
		}
	} else {
		matches, e := s.toolProfileExactMatchesTx(auth, project, request.WorkerID, request.Profile)
		if e != nil {
			return out, e
		}
		for _, match := range matches {
			if match.State != "revoked" {
				return out, toolProfileMatchExists()
			}
		}
		var count int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_profiles WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&count); err != nil {
			return out, err
		}
		if count >= 1000 {
			return out, conflict("项目工具档案已达 1000 项限制")
		}
		at := automationStamp(clock)
		target = ToolProfile{ID: newID(), ProjectID: project, WorkerID: request.WorkerID, Name: request.Name, Version: 1, State: "paused", Profile: toolcontract.CloneProfile(request.Profile), CreatedAt: at, UpdatedAt: at}
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_profiles(id,tenant_id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), target.ID, auth.target.tenantID, project, target.WorkerID, target.Name, target.Version, target.State, stringify(target.Profile), "", at, at)
		if err != nil {
			return out, err
		}
		disposition = "created"
	}
	if request.PauseOriginal && original.State != "paused" {
		if !ingressVersion(original.Version + 1) {
			return out, toolReplacementChanged()
		}
		result, e := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_profiles SET state='paused',version=version+1,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), automationStamp(clock), auth.target.tenantID, project, original.ID, original.Version)
		if e != nil {
			return out, e
		}
		changed, e := result.RowsAffected()
		if e != nil {
			return out, e
		}
		if changed != 1 {
			return out, toolReplacementChanged()
		}
		original.State = "paused"
		original.Version++
		original.UpdatedAt = automationStamp(clock)
	}
	// Save the observations exactly once with the committed relation; replay and
	// history return these snapshots without treating them as today's readiness.
	original, err = s.toolProfileObservationTx(ctx, tx, auth.currentPrincipal, project, original)
	if err != nil {
		return out, err
	}
	target, err = s.toolProfileObservationTx(ctx, tx, auth.currentPrincipal, project, target)
	if err != nil {
		return out, err
	}
	out = ToolProfileReplacement{ID: newID(), ProjectID: project, Original: original, Replacement: target, Disposition: disposition, PauseOriginal: request.PauseOriginal, CreatedBy: auth.currentPrincipal.ID, CreatedAt: clock.UTC().Format(replacementStamp)}
	if !ToolProfileReplacementMatches(out, request, id) {
		return out, toolReplacementChanged()
	}
	// The successful write must be recoverable by the same bounded reader.
	// Two individually legal inventories can form an oversized dual snapshot;
	// reject it inside this transaction before any relation/receipt is committed.
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if len(raw) > maxToolProfileReplacementBytes {
		return out, toolReplacementSnapshotLimit()
	}
	if _, err = decodeToolProfileReplacement(string(raw)); err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_profile_replacements(tenant_id,project_id,id,original_id,replacement_id,created_at,payload)VALUES(?,?,?,?,?,?,?)`), auth.target.tenantID, project, out.ID, original.ID, target.ID, out.CreatedAt, string(raw))
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_profile.replaced", out.ID, original.Name+" → "+target.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, json.RawMessage(raw)); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) replacementInventoryTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, profile toolcontract.Profile, clock time.Time) error {
	var observed, raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, worker).Scan(&observed, &raw); err != nil {
		if err == sql.ErrNoRows {
			return toolReplacementInventory()
		}
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, observed)
	if err != nil || at.After(clock) || !clock.Before(at.Add(time.Minute)) {
		return toolReplacementInventory()
	}
	items, err := decodeToolDiscoveryItems(raw)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Available && toolcontract.EqualProfile(item.Profile, profile) {
			return nil
		}
	}
	return toolReplacementInventory()
}
