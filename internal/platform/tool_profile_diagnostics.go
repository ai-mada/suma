package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// GetToolProfileDiagnostics reads only current, exact-scope facts. The original
// browser credential authorizes this same transaction; no probe or receipt runs.
func (s *Service) GetToolProfileDiagnostics(ctx context.Context, actor Actor, project, id string) (ToolProfileDiagnostics, error) {
	var out ToolProfileDiagnostics
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		profile, err := s.toolProfileTx(auth.ctx, auth.tx, auth.currentPrincipal, project, id)
		if err != nil {
			return err
		}
		out, err = s.collectToolProfileObservationLockedTx(auth.ctx, auth.tx, auth.currentPrincipal.TenantID, project, profile, true)
		return err
	})
	return out, err
}

// Ordinary observations stop at their original first blocker. Diagnostics alone
// continue through independent facts, while preserving that same first blocker
// and availability result. Both views use this single predicate sequence.
func (s *Service) collectToolProfileObservationLockedTx(ctx context.Context, tx *sql.Tx, tenant, project string, out ToolProfile, diagnostics bool) (ToolProfileDiagnostics, error) {
	report := ToolProfileDiagnostics{NextAction: "none"}
	finish := func(err error) (ToolProfileDiagnostics, error) {
		report.Tool = out
		return report, err
	}
	p := Principal{TenantID: tenant} // projection/scope only; never an authority source
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return finish(err)
	}
	report.CheckedAt = clock.UTC().Format(time.RFC3339Nano)
	expires := clock.Add(60 * time.Second)
	report.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	out.Available = false
	out.ReasonDiagnostic = nil
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, out.WorkerID).Scan(&out.LastSeenAt, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return finish(err)
	}
	if err == nil {
		var items []toolcontract.Observation
		if err = json.Unmarshal([]byte(raw), &items); err != nil {
			return finish(err)
		}
		for _, item := range items {
			if item.Profile.CommandID == out.Profile.CommandID {
				copy := item
				out.Observation = &copy
				break
			}
		}
	}
	blocked := false
	record := func(code, status string, reason toolReason, action string) bool {
		if diagnostics {
			report.Checks = append(report.Checks, ToolProfileDiagnosticCheck{Code: code, Status: status, Reason: reason.source, ReasonDiagnostic: reason.diagnostic, NextAction: action})
		}
		if status == "blocked" {
			if !blocked {
				out.Reason, out.ReasonDiagnostic, report.NextAction = reason.source, reason.diagnostic, action
			}
			blocked = true
			return !diagnostics
		}
		return false
	}
	authorization := newToolReason("配置启用授权当前有效", "authorization_current", nil)
	authorizationStatus, authorizationAction := "pass", "none"
	switch out.State {
	case "revoked":
		authorization = newToolReason("工具档案已永久撤销", "profile_revoked", nil)
		authorizationStatus, authorizationAction = "blocked", "register_profile"
	case "paused":
		authorization = newToolReason("工具档案已暂停，尚未授权派发", "profile_paused", nil)
		authorizationStatus, authorizationAction = "blocked", "review_profile"
	case "enabled":
		if err = s.toolActivationLockedTx(ctx, tx, tenant, project, out); err != nil {
			if !automationAuthorityFailure(err) {
				return finish(err)
			}
			authorization = newToolReason("工具档案启用人的当前审批权限已失效", "activation_revoked", nil)
			authorizationStatus, authorizationAction = "blocked", "review_profile"
		}
	default:
		return finish(invalid("工具档案状态无效"))
	}
	if record("authorization", authorizationStatus, authorization, authorizationAction) {
		return finish(nil)
	}
	worker, err := s.toolWorkerReasonTx(ctx, tx, p, project, out.WorkerID, clock)
	if err != nil {
		return finish(err)
	}
	if worker.source != "" {
		if record("worker", "blocked", worker, "open_worker") {
			return finish(nil)
		}
	} else {
		record("worker", "pass", newToolReason("执行节点身份当前有效", "worker_current", nil), "none")
	}
	observed, parseErr := time.Parse(time.RFC3339, out.LastSeenAt)
	inventory := newToolReason("工具清单当前新鲜", "inventory_fresh", nil)
	inventoryStatus, inventoryAction := "pass", "none"
	if out.LastSeenAt == "" {
		inventory = newToolReason("执行节点尚未上报工具清单", "inventory_missing", nil)
		inventoryStatus, inventoryAction = "blocked", "open_worker"
	} else if parseErr != nil || observed.After(clock) || !clock.Before(observed.Add(60*time.Second)) {
		inventory = newToolReason("工具清单已过期，等待执行节点重新探测", "inventory_stale", nil)
		inventoryStatus, inventoryAction = "blocked", "open_worker"
	} else if until := observed.Add(60 * time.Second); until.Before(expires) {
		report.ExpiresAt = until.UTC().Format(time.RFC3339Nano)
	}
	if record("inventory", inventoryStatus, inventory, inventoryAction) {
		return finish(nil)
	}
	if inventoryStatus != "pass" {
		record("profile", "not_checked", inventory, "open_worker")
		return finish(nil)
	}
	profile := newToolReason("节点已报告完整相同配置可用", "profile_matches", nil)
	profileStatus, profileAction := "pass", "none"
	switch {
	case out.Observation == nil:
		profile = newToolReason("当前执行节点未声明此命令", "command_missing", nil)
		profileStatus, profileAction = "blocked", "open_worker"
	case !toolcontract.EqualProfile(out.Observation.Profile, out.Profile):
		profile = newToolReason("当前工具配置、版本或程序摘要与固定档案不匹配", "profile_mismatch", nil)
		profileStatus, profileAction = "blocked", "register_profile"
	case !out.Observation.Available:
		profile = newToolReason("执行节点报告工具不可用："+out.Observation.Reason, "reported_unavailable", map[string]any{"reason": out.Observation.Reason})
		profileStatus, profileAction = "blocked", "open_worker"
	}
	if record("profile", profileStatus, profile, profileAction) {
		return finish(nil)
	}
	if !blocked {
		out.Available = true
		out = withToolReason(out, "执行节点已报告可领取；模型认证、执行与验收结果需分别核验", "dispatchable", nil)
	}
	return finish(nil)
}
