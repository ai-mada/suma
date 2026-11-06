package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func validReleaseTime(v string) bool { _, err := time.Parse(time.RFC3339Nano, v); return err == nil }
func releaseObservationTerminal(o rc.DeploymentObservation) bool {
	return o.State == "succeeded" || o.State == "failed" || o.State == "cancelled"
}
func releaseObservationKnownTerminal(o rc.DeploymentObservation) bool {
	return releaseObservationTerminal(o) && o.Effect.State != "unknown" && o.FenceGeneration >= o.Generation && (o.Effect.State == "not_started" || o.Health.State == "passed" || o.Health.State == "failed")
}
func validateReleaseObservation(d ReleaseDeployment, p ReleasePlan, o rc.DeploymentObservation) error {
	if o.Protocol != rc.ReleaseProtocol || o.OperationID != d.OperationID || o.RequestSHA256 != d.RequestSHA256 || !rc.ValidDigest(o.RequestSHA256) || o.ProviderID != d.ProviderID || o.PlanID != d.PlanID || o.PlanRevision != d.PlanRevision || o.PlanSHA256 != d.PlanSHA256 || o.EnvironmentID != d.EnvironmentID || o.Generation != d.Generation || o.FenceGeneration < 0 || o.FenceGeneration > 9007199254740991 || o.Version < 1 {
		return releaseConflict("observation_binding_invalid", "提供方观测与固定操作不匹配")
	}
	hash, err := rc.ObservationDigest(o)
	if err != nil || !rc.ValidDigest(o.EvidenceSHA256) || hash != o.EvidenceSHA256 {
		return releaseConflict("observation_digest_mismatch", "提供方观测摘要不匹配")
	}
	switch o.State {
	case "received", "authorized", "staging", "applying", "checking_health", "succeeded", "failed", "cancelled", "reconciling":
	default:
		return invalid("提供方操作状态无效")
	}
	switch o.Effect.State {
	case "not_started", "applied", "unknown":
	default:
		return invalid("必须显式提供实际副作用状态")
	}
	switch o.Health.State {
	case "not_checked", "passed", "failed", "unknown":
	default:
		return invalid("必须显式提供实际健康状态")
	}
	if o.Error != nil && (len(o.Error.Code) > 100 || len(o.Error.Message) > 2000) {
		return invalid("提供方错误信息过长")
	}
	if o.Effect.State == "applied" {
		if o.Effect.ActiveGeneration != d.Generation || o.Effect.ActiveArtifactSHA256 != d.ArtifactSHA256 || o.Effect.PreviousActiveGeneration < 0 || o.Effect.PreviousActiveGeneration > p.Content.ExpectedGeneration || !validReleaseTime(o.Effect.ObservedAt) {
			return releaseConflict("observation_effect_invalid", "实际 active 指针不匹配本代制品")
		}
	}
	if o.Effect.State == "not_started" && (o.Effect.ActiveGeneration < 0 || o.Effect.ActiveGeneration >= d.Generation || o.Effect.ActiveArtifactSHA256 != "" && !rc.ValidDigest(o.Effect.ActiveArtifactSHA256)) {
		return releaseConflict("observation_effect_invalid", "未开始观测与实际 active 指针矛盾")
	}
	if o.Health.State == "passed" {
		if o.Effect.State != "applied" || o.Health.Generation != d.Generation || o.Health.ArtifactSHA256 != d.ArtifactSHA256 || len(o.Health.Checks) != len(p.Content.Health.Checks) {
			return releaseConflict("observation_health_invalid", "健康观测不属于本代实际制品")
		}
		seen := map[string]bool{}
		for _, actual := range o.Health.Checks {
			if seen[actual.Path] || !validReleaseTime(actual.ObservedAt) {
				return invalid("健康检查重复或缺少实际时间")
			}
			seen[actual.Path] = true
			found := false
			for _, expected := range p.Content.Health.Checks {
				if actual.Path == expected.Path && actual.StatusCode == expected.ExpectedStatus && actual.BodySHA256 == expected.ExpectedSHA256 {
					found = true
					break
				}
			}
			if !found {
				return releaseConflict("observation_health_invalid", "实际 HTTP 内容或状态不匹配固定健康策略")
			}
		}
	}
	if o.Health.State == "failed" {
		if o.Effect.State != "applied" || o.Health.Generation != d.Generation || o.Health.ArtifactSHA256 != d.ArtifactSHA256 || len(o.Health.Checks) == 0 {
			return releaseConflict("observation_health_invalid", "失败健康观测也须绑定实际 active 制品")
		}
		for _, check := range o.Health.Checks {
			if !validReleaseTime(check.ObservedAt) || len(check.Path) > 200 {
				return invalid("失败健康观测不完整")
			}
			known := false
			for _, expected := range p.Content.Health.Checks {
				if expected.Path == check.Path {
					known = true
				}
			}
			if !known || check.BodySHA256 != "" && !rc.ValidDigest(check.BodySHA256) {
				return invalid("失败健康观测不属于固定策略")
			}
		}
	}
	if o.State == "succeeded" && (o.Effect.State != "applied" || o.Health.State != "passed" || o.FenceGeneration < d.Generation) {
		return releaseConflict("observation_success_unproven", "部署成功需要实际激活、本代 HTTP 健康与持久 fencing ACK")
	}
	if o.State == "cancelled" && o.Effect.State != "not_started" {
		return releaseConflict("observation_cancel_unproven", "已发生或未知的副作用不能标为取消成功")
	}
	return nil
}
func (s *Service) releaseFailureIssueTx(ctx context.Context, tx *sql.Tx, tenant, project string, d *ReleaseDeployment, o rc.DeploymentObservation) error {
	if d.IssueID != "" || o.State != "failed" && o.Effect.State != "unknown" && o.Health.State != "failed" && o.State != "reconciling" {
		return nil
	}
	return s.ensureReleaseIssueTx(ctx, tx, tenant, project, d, &o)
}

func (s *Service) ensureReleaseIssueTx(ctx context.Context, tx *sql.Tx, tenant, project string, d *ReleaseDeployment, o *rc.DeploymentObservation) error {
	if d.IssueID != "" {
		return nil
	}
	description := "发布执行已失败；提供方实际效果尚待核对，请查看原操作和运行证据。"
	source := "execution"
	if o != nil {
		description = "提供方已返回失败、健康失败或未知结果；请查看固定操作的结构化观测。"
		source = "provider"
	}
	r := recordFrom(Principal{ID: "release-projector", TenantID: tenant}, project, map[string]any{"title": "发布操作需要处理 " + d.OperationID, "description": description, "status": "open", "kind": "deployment", "priority": "high"})
	r.RunID = d.RunID
	r.Data["origin"] = "release"
	r.Data["failure_source"] = source
	r.Data["release_operation_id"] = d.OperationID
	r.Data["release_plan_id"] = d.PlanID
	if o != nil {
		r.Data["provider_evidence_sha256"] = o.EvidenceSHA256
		r.Data["effect_state"] = o.Effect.State
		r.Data["health_state"] = o.Health.State
	}
	if err := s.insertRecord(ctx, tx, r, "issues"); err != nil {
		return err
	}
	d.IssueID = r.ID
	return nil
}
func (s *Service) applyReleaseObservationTx(ctx context.Context, tx *sql.Tx, cfg ReleaseProviderConfig, o rc.DeploymentObservation) (ReleaseDeployment, error) {
	d, err := s.currentReleaseDeploymentTx(ctx, tx, cfg.TenantID, cfg.ProjectID, o.OperationID)
	if err != nil {
		return d, err
	}
	if d.ProviderID != cfg.ID {
		return d, forbidden()
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, cfg.TenantID, cfg.ProjectID, d.PlanID)
	if err != nil {
		return d, err
	}
	if err = validateReleaseObservation(d, plan, o); err != nil {
		return d, err
	}
	var count int
	query := `SELECT COUNT(*) FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='consumed' AND request_sha256=?`
	args := []any{cfg.TenantID, cfg.ProjectID, d.OperationID, o.RequestSHA256}
	if o.Effect.State == "applied" {
		query += ` AND effect_grant_id<>''`
	}
	if err = tx.QueryRowContext(ctx, s.q(query), args...).Scan(&count); err != nil {
		return d, err
	}
	if count < 1 {
		return d, forbidden()
	}
	var priorHash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT evidence_sha256 FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=?`), cfg.TenantID, cfg.ProjectID, d.OperationID, o.Version).Scan(&priorHash)
	if err == nil {
		if priorHash != o.EvidenceSHA256 {
			return d, releaseConflict("idempotency_payload_conflict", "同观测版本已保存不同内容")
		}
		return d, nil
	}
	if err != sql.ErrNoRows {
		return d, err
	}
	if d.Observation != nil {
		if o.Version < d.Observation.Version {
			// Outbox/network ordering may deliver an older valid observation later.
			// Preserve it as history without changing current projection or version.
			_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_observations(tenant_id,project_id,operation_id,version,evidence_sha256,data)VALUES(?,?,?,?,?,?)`), cfg.TenantID, cfg.ProjectID, d.OperationID, o.Version, o.EvidenceSHA256, stringify(o))
			return d, err
		}
		if d.State != "reconciling" && releaseObservationTerminal(*d.Observation) {
			return d, releaseConflict("terminal_observation_immutable", "终态观测不可覆盖")
		}
	}
	env, err := s.releaseEnvironmentTx(ctx, tx, cfg.TenantID, cfg.ProjectID, d.EnvironmentID, true)
	if err != nil {
		return d, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_observations(tenant_id,project_id,operation_id,version,evidence_sha256,data)VALUES(?,?,?,?,?,?)`), cfg.TenantID, cfg.ProjectID, d.OperationID, o.Version, o.EvidenceSHA256, stringify(o)); err != nil {
		return d, err
	}
	old := d.Version
	d.Version++
	d.Observation = &o
	d.Observations = nil
	d.State = releaseProjection(o)
	d.EffectState = o.Effect.State
	d.HealthState = o.Health.State
	d.UpdatedAt = now()
	if err = s.releaseFailureIssueTx(ctx, tx, cfg.TenantID, cfg.ProjectID, &d, o); err != nil {
		return d, err
	}
	// A stale generation never changes current active state, health or barriers.
	if env.Generation == d.Generation && env.BarrierOperationID == d.OperationID {
		oldEnv := env.Version
		env.Version++
		env.UpdatedAt = now()
		if o.Effect.State == "applied" {
			env.ActiveGeneration = d.Generation
			env.ActiveArtifactID = d.ArtifactID
			env.ActiveArtifactSHA256 = d.ArtifactSHA256
		}
		knownTerminal := releaseObservationKnownTerminal(o)
		if knownTerminal {
			env.BarrierOperationID = ""
			if d.State == "deployed_healthy" {
				env.CapabilityState = "verified"
			}
			oldPlan := plan.Version
			plan.Version++
			if plan.State != "revoked" {
				plan.State = "completed"
			}
			plan.AllowedActions = []string{}
			if err = s.saveReleaseJSON(ctx, tx, "plans", cfg.TenantID, cfg.ProjectID, plan.ID, oldPlan, plan); err != nil {
				return d, err
			}
		} else if releaseObservationTerminal(o) {
			d.State = "reconciling"
		}
		if err = s.saveReleaseJSON(ctx, tx, "environments", cfg.TenantID, cfg.ProjectID, env.ID, oldEnv, env); err != nil {
			return d, err
		}
	}
	if err = s.saveReleaseJSON(ctx, tx, "deployments", cfg.TenantID, cfg.ProjectID, d.OperationID, old, d); err != nil {
		return d, err
	}
	return d, nil
}
func (s *Service) ObserveReleaseDeployment(ctx context.Context, token string, o rc.DeploymentObservation) (ReleaseDeployment, error) {
	cfg, err := s.authenticateReleaseProvider(token, o.ProviderID)
	if err != nil {
		return ReleaseDeployment{}, err
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return ReleaseDeployment{}, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return ReleaseDeployment{}, err
	}
	if err = s.releaseSchedulerLock(ctx, tx, cfg.TenantID, cfg.ProjectID); err != nil {
		return ReleaseDeployment{}, err
	}
	d, err := s.applyReleaseObservationTx(ctx, tx, cfg, o)
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *Service) ReleaseDeployments(ctx context.Context, actor Actor, project string) ([]ReleaseDeployment, error) {
	var result0 []ReleaseDeployment
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseDeploymentsTx(bounded, auth, project)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseDeploymentsTx(ctx context.Context, auth *authorizedCommand, project string) ([]ReleaseDeployment, error) {
	if auth == nil {
		return nil, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return nil, e
	}
	if auth.target.projectID != project {
		return nil, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	rows, err := tx.QueryContext(ctx, s.q(`SELECT operation_id FROM suma_release_deployments WHERE tenant_id=? AND project_id=? ORDER BY operation_id`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	out := []ReleaseDeployment{}
	for _, id := range ids {
		d, e := s.readReleaseDeploymentTx(ctx, auth, project, id)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, nil

}

func (s *Service) ReleaseDeployment(ctx context.Context, actor Actor, project, id string) (ReleaseDeployment, error) {
	var result0 ReleaseDeployment
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseDeploymentTx(bounded, auth, project, id)
		if e == nil {
			result0.RepairSource, e = s.releaseDeploymentRepairSourceReadTx(bounded, auth.tx, auth.target.tenantID, project, result0)
		}
		return e
	})
	return result0, err
}
func (s *Service) readReleaseDeploymentTx(ctx context.Context, auth *authorizedCommand, project, id string) (ReleaseDeployment, error) {
	if auth == nil {
		return ReleaseDeployment{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ReleaseDeployment{}, e
	}
	if auth.target.projectID != project {
		return ReleaseDeployment{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	d, err := s.releaseDeploymentTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return d, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT data FROM suma_release_observations WHERE tenant_id=? AND project_id=? AND operation_id=? ORDER BY version`), p.TenantID, project, id)
	if err != nil {
		return d, err
	}
	d.Observations = []rc.DeploymentObservation{}
	for rows.Next() {
		var raw string
		var o rc.DeploymentObservation
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return d, err
		}
		if err = json.Unmarshal([]byte(raw), &o); err != nil {
			rows.Close()
			return d, err
		}
		d.Observations = append(d.Observations, o)
	}
	if err = rows.Close(); err != nil {
		return d, err
	}
	d.AllowedActions = []string{}
	d.Blockers = []ReleaseBlocker{}
	if auth.actor.IsBrowser() && s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil {
		d.AllowedActions = append(d.AllowedActions, "reconcile")
		if d.State == "deployed_healthy" {
			d.AllowedActions = append(d.AllowedActions, "rollback_preview")
		}
	}
	if d.State == "reconciling" {
		d.Blockers = append(d.Blockers, ReleaseBlocker{"environment_reconciling", "原操作结果尚未核清，环境屏障保持占用"})
	}
	return d, nil

}

func (s *Service) ReconcileReleaseDeployment(ctx context.Context, actor Actor, project, id string, m map[string]any, engine *dag.Engine) (ReleaseDeployment, error) {
	p := actor.Projection()

	var req struct {
		ProjectID       string `json:"project_id"`
		ExpectedVersion int    `json:"expected_version"`
		IdempotencyKey  string `json:"idempotency_key"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return ReleaseDeployment{}, err
	}
	if err := exactVersion(m, "expected_version", 1); err != nil {
		return ReleaseDeployment{}, err
	}
	readCtx, read, readCancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ReleaseDeployment{}, err
	}
	defer readCancel()
	auth, err := s.releaseBrowserTx(readCtx, read, actor, project, true)
	if err != nil {
		read.Rollback()
		readCancel()
		return ReleaseDeployment{}, err
	}
	p = auth.currentPrincipal
	hash, replay, err := s.commandReceipt(readCtx, auth, req.IdempotencyKey, map[string]any{"release_reconcile": id, "body": m})
	if err != nil {
		read.Rollback()
		readCancel()
		return ReleaseDeployment{}, err
	}
	if replay {
		var saved ReleaseDeployment
		err = json.Unmarshal([]byte(hash), &saved)
		read.Rollback()
		readCancel()
		return saved, err
	}
	d, err := s.releaseDeploymentTx(readCtx, read, p.TenantID, project, id)
	read.Rollback()
	readCancel()
	if err != nil {
		return d, err
	}
	if d.Version != req.ExpectedVersion {
		return d, conflict("部署版本已变化")
	}
	cfg, err := s.releaseProvider(p.TenantID, project, d.ProviderID)
	if err != nil {
		return d, err
	}
	o, err := inspectReleaseProvider(ctx, cfg, id)
	if err != nil {
		return d, err
	}
	if releaseObservationKnownTerminal(o) && d.Assignment != nil {
		probe, e := s.DB.BeginTx(ctx, s.executionWriteOptions())
		if e != nil {
			return d, e
		}
		var nodeState string
		var lease int64
		e = probe.QueryRowContext(ctx, s.q(`SELECT state,lease_until FROM dag_nodes WHERE run_id=? AND id=?`), d.RunID, d.NodeID).Scan(&nodeState, &lease)
		clock, clockErr := s.artifactClockTx(ctx, probe)
		probe.Rollback()
		if e != nil {
			return d, e
		}
		if clockErr != nil {
			return d, clockErr
		}
		if nodeState == "reconciling" || nodeState == "running" && lease <= clock {
			if engine == nil {
				return d, invalid("核对管理执行需要发布调度器")
			}
			return s.resolveObservedRelease(ctx, actor, project, id, m, req.ExpectedVersion, req.IdempotencyKey, cfg, o, d, engine)
		}
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return d, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err = s.releaseBrowserTx(ctx, tx, actor, project, true)
	if err != nil {
		return d, err
	}
	p = auth.currentPrincipal
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return d, err
	}
	hash, replay, err = s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"release_reconcile": id, "body": m})
	if err != nil {
		return d, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &d)
		return d, err
	}
	current, err := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return d, err
	}
	if current.Version != req.ExpectedVersion {
		return current, conflict("核对期间部署记录已变化，请刷新")
	}
	d, err = s.applyReleaseObservationTx(ctx, tx, cfg, o)
	if err != nil {
		return d, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, d); err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *Service) resolveObservedRelease(ctx context.Context, actor Actor, project, id string, m map[string]any, expected int, key string, cfg ReleaseProviderConfig, o rc.DeploymentObservation, snapshot ReleaseDeployment, engine *dag.Engine) (ReleaseDeployment, error) {
	p := actor.Projection()

	if snapshot.Assignment == nil {
		return snapshot, releaseConflict("observation_binding_invalid", "缺少原执行身份")
	}
	a := *snapshot.Assignment
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "observed-" + id + "-" + strconv.Itoa(o.Version), ReleaseEvidence: &rc.ReleaseResult{OperationID: id, RequestSHA256: o.RequestSHA256, EvidenceSHA256: o.EvidenceSHA256, Version: o.Version}}
	if o.State != "succeeded" {
		result.ExitCode = 1
		result.Error = "提供方确认原操作未成功"
	}
	var out ReleaseDeployment
	var receiptHash string
	var replay bool
	var auth *authorizedCommand
	err := engine.CompleteManagedObservedBound(ctx, p.TenantID, project, result, func(tx *sql.Tx) error {
		var e error
		auth, e = s.releaseBrowserTx(ctx, tx, actor, project, true)
		if e != nil {
			return e
		}
		p = auth.currentPrincipal
		if e := dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, p.TenantID, project, a.RunID); e != nil {
			return e
		}
		receiptHash, replay, e = s.commandReceipt(ctx, auth, key, map[string]any{"release_reconcile": id, "body": m})
		if e != nil {
			return e
		}
		if replay {
			return json.Unmarshal([]byte(receiptHash), &out)
		}
		current, e := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, id)
		if e != nil {
			return e
		}
		if current.Version != expected {
			return conflict("核对期间部署记录已变化，请刷新")
		}
		if current.Assignment == nil || !releaseSame(*current.Assignment, a) {
			return releaseConflict("stale_assignment", "原执行身份已变化")
		}
		return nil
	}, func(tx *sql.Tx, state string) error {
		if replay {
			return nil
		}
		var e error
		out, e = s.applyReleaseObservationTx(ctx, tx, cfg, o)
		if e != nil {
			return e
		}
		if e = s.ValidateObservedManagedResultTx(ctx, tx, auth, result, state); e != nil {
			return e
		}
		return s.saveCommandReceipt(ctx, auth, key, receiptHash, out)
	})
	return out, err
}
func inspectReleaseProvider(ctx context.Context, cfg ReleaseProviderConfig, operation string) (rc.DeploymentObservation, error) {
	var out rc.DeploymentObservation
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || cfg.ReconcileToken == "" {
		return out, releaseError(503, "provider_unavailable", "未配置固定提供方核对连接")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return out, releaseError(503, "provider_unavailable", "首轮核对仅允许管理员固定回环提供方")
	}
	u.Path = "/v2/operations/" + url.PathEscape(operation)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, err
	}
	r.Header.Set("Authorization", "Bearer "+cfg.ReconcileToken)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(r)
	if err != nil {
		return out, releaseError(503, "provider_unavailable", "提供方不可达，原操作保持占用")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, releaseError(503, "provider_unavailable", "提供方尚未返回可核对的原操作记录")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(b) > 1<<20 {
		return out, invalid("提供方观测超限或不可读")
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&out); err != nil {
		return out, invalid("提供方返回非结构化观测")
	}
	return out, nil
}
func (s *Service) RollbackReleasePreview(ctx context.Context, actor Actor, project, id string, m map[string]any) (map[string]any, error) {
	p := actor.Projection()

	var req struct {
		ProjectID                     string `json:"project_id"`
		ExpectedEnvironmentGeneration int64  `json:"expected_environment_generation"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return nil, err
	}
	if err := releaseGeneration(m, "expected_environment_generation"); err != nil {
		return nil, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseBrowserTx(ctx, tx, actor, project, true)
	if err != nil {
		return nil, err
	}
	p = auth.currentPrincipal
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return nil, err
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return nil, err
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, d.PlanID)
	if err != nil {
		return nil, err
	}
	input := ReleasePlanInput{ProjectID: project, CandidateID: plan.Content.CandidateID, EnvironmentID: d.EnvironmentID, Action: "rollback", ExpectedEnvironmentGeneration: req.ExpectedEnvironmentGeneration, RollbackSourceOperationID: id}
	preview, _, err := s.releasePreviewTx(ctx, tx, p, project, input)
	if err != nil {
		return nil, err
	}
	return map[string]any{"input": input, "preview": preview}, nil
}

// Invoked inside CompleteBound after its current attempt write, before advance.
// The scheduler's state is not evidence; only a stored authenticated observation
// can substantiate a successful managed deployment result.
func (s *Service) ValidateReleaseResultTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, r dag.Result, state string) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, purposeRunnerWrite); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	var operation string
	err := tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), p.TenantID, project, r.RunID, r.NodeID).Scan(&operation)
	if err == sql.ErrNoRows {
		if r.ReleaseEvidence != nil {
			return forbidden()
		}
		return nil
	}
	if err != nil {
		return err
	}
	if state != "succeeded" {
		return nil
	}
	if r.ExitCode != 0 || r.Stopped || r.OutcomeUnknown {
		return releaseConflict("deployment_success_unproven", "发布结果缺少真实提供方成功证据")
	}
	return s.ValidateObservedManagedResultTx(ctx, tx, auth, r, state)
}

// An already granted effect is a historical external fact. Later revocation or
// task edits block new authorization, but cannot erase that authenticated fact.
func (s *Service) ValidateObservedManagedResultTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, r dag.Result, state string) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return err
	}
	if auth.purpose != purposeRunnerWrite && auth.purpose != purposeBrowserProjectWrite {
		return forbidden()
	}
	p, project := auth.currentPrincipal, auth.target.projectID
	var operation string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), p.TenantID, project, r.RunID, r.NodeID).Scan(&operation); err != nil {
		return missing()
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, operation)
	if err != nil {
		return err
	}
	if r.ReleaseEvidence == nil || r.OutcomeUnknown || d.Assignment == nil || !releaseSame(*d.Assignment, artifactRef(r)) {
		return releaseConflict("deployment_success_unproven", "缺少原操作的完整执行证据")
	}
	proof := r.ReleaseEvidence
	if d.Observation == nil || proof.OperationID != operation || proof.RequestSHA256 != d.RequestSHA256 || proof.EvidenceSHA256 != d.Observation.EvidenceSHA256 || proof.Version != d.Observation.Version {
		return releaseConflict("deployment_success_unproven", "发布结果不能代替当前持久提供方观测")
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, d.PlanID)
	if err != nil {
		return err
	}
	if err = validateReleaseObservation(d, plan, *d.Observation); err != nil {
		return err
	}
	if !releaseObservationKnownTerminal(*d.Observation) {
		return releaseConflict("deployment_success_unproven", "提供方尚未证明已知且持久fencing的终态")
	}
	if state == "succeeded" && (d.State != "deployed_healthy" || d.Observation.State != "succeeded") {
		return releaseConflict("deployment_success_unproven", "未知或失败不能提升为成功")
	}
	if state == "failed" && d.Observation.State != "failed" && d.Observation.State != "cancelled" {
		return releaseConflict("deployment_success_unproven", "失败核对与提供方事实不一致")
	}
	var grants int
	query := `SELECT COUNT(*) FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND assignment=? AND state='consumed'`
	if d.Observation.Effect.State == "applied" {
		query += ` AND effect_grant_id<>''`
	}
	if err = tx.QueryRowContext(ctx, s.q(query), p.TenantID, project, operation, d.RequestSHA256, stringify(*d.Assignment)).Scan(&grants); err != nil {
		return err
	}
	if grants < 1 {
		return forbidden()
	}
	return nil
}
