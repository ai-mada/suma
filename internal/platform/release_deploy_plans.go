package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func releaseGeneration(m map[string]any, key string) error {
	v, ok := m[key]
	if !ok || v == nil {
		return invalid("缺少" + key)
	}
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	default:
		return invalid(key + "必须为整数")
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 9007199254740990 || math.Trunc(n) != n {
		return invalid(key + "必须为安全非负整数")
	}
	return nil
}
func (s *Service) releasePreviewTx(ctx context.Context, tx *sql.Tx, p Principal, project string, in ReleasePlanInput) (ReleasePlanPreview, rc.ReleasePlanContent, error) {
	var view ReleasePlanPreview
	var content rc.ReleasePlanContent
	if in.ProjectID != project || in.CandidateID == "" || in.EnvironmentID == "" || in.Action != "deploy" && in.Action != "rollback" || in.ExpectedEnvironmentGeneration < 0 {
		return view, content, invalid("候选、环境和发布动作无效")
	}
	if err := s.checkNewReleaseRouteTx(ctx, tx, p.TenantID, project); err != nil {
		return view, content, err
	}
	env, err := s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, in.EnvironmentID, true)
	if err != nil {
		return view, content, err
	}
	if env.Status != "active" {
		return view, content, releaseConflict("environment_archived", "环境已归档")
	}
	if env.BarrierOperationID != "" {
		return view, content, releaseConflict("environment_reconciling", "环境仍被原操作占用，必须先核对")
	}
	if env.Generation != in.ExpectedEnvironmentGeneration {
		return view, content, releaseConflict("environment_generation_conflict", "环境代数已变化")
	}
	if env.Generation >= 9007199254740990 {
		return view, content, invalid("环境代数已到限制")
	}
	if _, err = s.releaseProviderSite(p.TenantID, project, env.ProviderID, env.SiteID); err != nil {
		return view, content, err
	}
	candidate, err := s.validateCurrentReleaseCandidateTx(ctx, tx, p, project, in.CandidateID)
	if err != nil {
		return view, content, err
	}
	if in.Action == "rollback" {
		if !rc.ValidOperationID(in.RollbackSourceOperationID) {
			return view, content, invalid("回退必须选择历史成功操作")
		}
		prior, e := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, in.RollbackSourceOperationID)
		if e != nil {
			return view, content, e
		}
		priorPlan, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, prior.PlanID)
		if e != nil {
			return view, content, e
		}
		if prior.EnvironmentID != env.ID || prior.State != "deployed_healthy" || prior.Observation == nil || priorPlan.Content.CandidateID != candidate.ID || prior.ArtifactID != candidate.ArtifactID || prior.ArtifactSHA256 != candidate.ArtifactSHA256 {
			return view, content, releaseConflict("rollback_basis_invalid", "回退目标必须是同环境真实健康部署的原候选")
		}
	} else if in.RollbackSourceOperationID != "" {
		return view, content, invalid("普通部署不能传回退来源")
	}
	manifest, err := s.releaseArtifactManifestTx(ctx, tx, p, project, candidate.ArtifactID)
	if err != nil {
		return view, content, err
	}
	health := env.Health
	health.Checks = append([]rc.HealthCheck{}, env.Health.Checks...)
	for i := range health.Checks {
		found := false
		for _, file := range manifest.Files {
			if file.Path == health.Checks[i].ExpectedFile {
				health.Checks[i].ExpectedSHA256 = file.SHA256
				found = true
				break
			}
		}
		if !found {
			return view, content, releaseConflict("health_file_missing", "健康策略要求的文件不在实际封存制品中")
		}
	}
	if err = rc.ValidateHealthPolicy(health, true); err != nil {
		return view, content, invalid(err.Error())
	}
	content = rc.ReleasePlanContent{CandidateID: candidate.ID, Action: in.Action, EnvironmentID: env.ID, ProviderID: env.ProviderID, SiteID: env.SiteID, EnvironmentPolicyVersion: env.PolicyVersion, ExpectedGeneration: env.Generation, Generation: env.Generation + 1, ArtifactID: candidate.ArtifactID, ArtifactSHA256: candidate.ArtifactSHA256, SourceCommit: candidate.SourceCommit, BuildAttestationID: candidate.BuildAttestationID, PackageTestAttestationID: candidate.PackageTestAttestationID, AcceptedTaskBasisSHA256: candidate.AcceptedTaskBasisSHA256, Health: health, RollbackSourceOperationID: in.RollbackSourceOperationID}
	view = ReleasePlanPreview{PreviewSHA256: releasePlanHash(content), Environment: env, CandidateID: candidate.ID, Action: in.Action, Health: health, Eligibility: true, Blockers: []ReleaseBlocker{}, AllowedActions: []string{"create"}}
	return view, content, nil
}
func (s *Service) PreviewReleasePlan(ctx context.Context, p Principal, project, token string, m map[string]any) (ReleasePlanPreview, error) {
	var in ReleasePlanInput
	if err := releaseDecode(m, &in); err != nil {
		return ReleasePlanPreview{}, err
	}
	if err := releaseGeneration(m, "expected_environment_generation"); err != nil {
		return ReleasePlanPreview{}, err
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return ReleasePlanPreview{}, err
	}
	defer tx.Rollback()
	if err = s.releaseBrowserTx(ctx, tx, p, project, token, false); err != nil {
		return ReleasePlanPreview{}, err
	}
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return ReleasePlanPreview{}, err
	}
	view, _, err := s.releasePreviewTx(ctx, tx, p, project, in)
	return view, err
}
func (s *Service) CreateReleasePlan(ctx context.Context, p Principal, project, token string, m map[string]any) (ReleasePlan, error) {
	var in ReleasePlanInput
	if err := releaseDecode(m, &in); err != nil {
		return ReleasePlan{}, err
	}
	if err := releaseGeneration(m, "expected_environment_generation"); err != nil {
		return ReleasePlan{}, err
	}
	if !rc.ValidDigest(in.PreviewSHA256) {
		return ReleasePlan{}, invalid("必须确认最新计划预览摘要")
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return ReleasePlan{}, err
	}
	defer tx.Rollback()
	if err = s.releaseBrowserTx(ctx, tx, p, project, token, false); err != nil {
		return ReleasePlan{}, err
	}
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return ReleasePlan{}, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, in.IdempotencyKey, map[string]any{"release_plan_create": m})
	if err != nil {
		return ReleasePlan{}, err
	}
	var out ReleasePlan
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	preview, content, err := s.releasePreviewTx(ctx, tx, p, project, in)
	if err != nil {
		return out, err
	}
	if preview.PreviewSHA256 != in.PreviewSHA256 {
		return out, releaseConflict("release_basis_changed", "预览依据已变化，请重新预览")
	}
	content.ReservedExecution = rc.ReservedExecution{RunID: newID(), NodeID: "deploy"}
	out = ReleasePlan{ID: newID(), ProjectID: project, Version: 1, Revision: 1, State: "pending_approval", Content: content, CreatedBy: p.ID, CreatedAt: now(), Eligibility: true, Blockers: []ReleaseBlocker{}, AllowedActions: []string{}}
	if s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil {
		out.AllowedActions = []string{"approve", "reject"}
	}
	out.SHA256 = releasePlanHash(content)
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_plans(id,tenant_id,project_id,version,data)VALUES(?,?,?,?,?)`), out.ID, p.TenantID, project, 1, stringify(out)); err != nil {
		return out, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, in.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) releasePlanBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan ReleasePlan, operation string) error {
	return s.releasePlanBasisReadTx(ctx, tx, p, project, plan, operation, false)
}

// Execution gates call this only after the organization and project guards.
func (s *Service) currentReleasePlanBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan ReleasePlan, operation string) error {
	return s.releasePlanBasisReadTx(ctx, tx, p, project, plan, operation, true)
}
func (s *Service) releasePlanBasisReadTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan ReleasePlan, operation string, current bool) error {
	if plan.SHA256 != releasePlanHash(plan.Content) {
		return releaseConflict("release_basis_changed", "计划摘要损坏")
	}
	env, err := s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, plan.Content.EnvironmentID, current)
	if err != nil {
		return err
	}
	if env.Status != "active" || env.PolicyVersion != plan.Content.EnvironmentPolicyVersion || env.ProviderID != plan.Content.ProviderID || env.SiteID != plan.Content.SiteID {
		return releaseConflict("release_basis_changed", "环境策略或绑定已变化")
	}
	if operation == "" {
		if env.Generation != plan.Content.ExpectedGeneration || env.BarrierOperationID != "" {
			return releaseConflict("environment_generation_conflict", "环境代数或占用已变化")
		}
	} else {
		if env.Generation != plan.Content.Generation || env.BarrierOperationID != operation {
			return releaseConflict("environment_reconciling", "环境屏障不属于此操作")
		}
	}
	if _, err = s.releaseProviderSite(p.TenantID, project, plan.Content.ProviderID, plan.Content.SiteID); err != nil {
		return err
	}
	candidate, err := s.validateReleaseCandidateReadTx(ctx, tx, p, project, plan.Content.CandidateID, current)
	if err != nil {
		return err
	}
	if candidate.ArtifactID != plan.Content.ArtifactID || candidate.ArtifactSHA256 != plan.Content.ArtifactSHA256 || candidate.SourceCommit != plan.Content.SourceCommit || candidate.BuildAttestationID != plan.Content.BuildAttestationID || candidate.PackageTestAttestationID != plan.Content.PackageTestAttestationID || candidate.AcceptedTaskBasisSHA256 != plan.Content.AcceptedTaskBasisSHA256 {
		return releaseConflict("release_basis_changed", "候选发布依据已变化")
	}
	return nil
}
func (s *Service) releaseActorAdminTx(ctx context.Context, tx *sql.Tx, tenant, project, actor string) error {
	return s.releaseActorAdminReadTx(ctx, tx, tenant, project, actor, true)
}
func (s *Service) releaseActorSnapshotAdminTx(ctx context.Context, tx *sql.Tx, tenant, project, actor string) error {
	return s.releaseActorAdminReadTx(ctx, tx, tenant, project, actor, false)
}
func (s *Service) releaseActorAdminReadTx(ctx context.Context, tx *sql.Tx, tenant, project, actor string, current bool) error {
	var owner string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT tenant_id FROM suma_projects WHERE id=?`, current)), project).Scan(&owner); err != nil || owner != tenant {
		return missing()
	}
	var organizationRole string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE m.tenant_id=? AND m.user_id=?`, current)), tenant, actor).Scan(&organizationRole); err != nil {
		if err == sql.ErrNoRows {
			return forbidden()
		}
		return err
	}
	if organizationRole == "owner" || organizationRole == "admin" {
		return nil
	}
	var role string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`, current)), project, actor).Scan(&role); err != nil || role != "admin" {
		return forbidden()
	}
	return nil
}
func (s *Service) ApproveReleasePlan(ctx context.Context, p Principal, project, id, token string, m map[string]any) (ReleasePlan, error) {
	var req struct {
		ProjectID       string `json:"project_id"`
		ExpectedVersion int    `json:"expected_version"`
		IdempotencyKey  string `json:"idempotency_key"`
		PlanRevision    int    `json:"plan_revision"`
		PlanSHA256      string `json:"plan_sha256"`
		Decision        string `json:"decision"`
		Reason          string `json:"reason"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return ReleasePlan{}, err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" || len(req.Reason) > 2000 || req.Decision != "approved" && req.Decision != "rejected" && req.Decision != "revoked" {
		return ReleasePlan{}, invalid("审批结论或理由无效")
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return ReleasePlan{}, err
	}
	defer tx.Rollback()
	if err = s.releaseBrowserTx(ctx, tx, p, project, token, true); err != nil {
		return ReleasePlan{}, err
	}
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return ReleasePlan{}, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, req.IdempotencyKey, map[string]any{"release_approve": id, "body": m})
	if err != nil {
		return ReleasePlan{}, err
	}
	var out ReleasePlan
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err = s.currentReleasePlanTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if err = releaseVersionCheck(m, out.Version, out); err != nil {
		return out, err
	}
	if out.ExecutionOperationID != "" && req.Decision != "revoked" {
		return out, conflict("已派发计划仅能撤销后续授权，不能覆盖历史审批")
	}
	if req.Decision == "approved" {
		if err = s.currentReleasePlanBasisTx(ctx, tx, p, project, out, ""); err != nil {
			return out, err
		}
	}
	if req.Decision == "revoked" && out.Approval == nil {
		return out, conflict("计划尚无可撤销审批")
	}
	old := out.Version
	out.Version++
	out.State = req.Decision
	out.Approval = &ReleaseApproval{ActorID: p.ID, Decision: req.Decision, Reason: req.Reason, At: now(), PlanSHA256: out.SHA256}
	out.AllowedActions = []string{}
	if out.ExecutionOperationID == "" {
		if req.Decision == "approved" {
			out.AllowedActions = []string{"execute", "revoke"}
		} else {
			out.AllowedActions = []string{"approve", "reject"}
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_approvals(id,tenant_id,project_id,plan_id,version,data)VALUES(?,?,?,?,?,?)`), newID(), p.TenantID, project, id, out.Version, stringify(out.Approval)); err != nil {
		return out, err
	}
	if err = s.saveReleaseJSON(ctx, tx, "plans", p.TenantID, project, id, old, out); err != nil {
		return out, err
	}
	if req.Decision == "revoked" {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_authorizations SET state='revoked' WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='issued'`), p.TenantID, project, out.ExecutionOperationID); err != nil {
			return out, err
		}
	}
	if err = s.saveReceipt(ctx, tx, p, project, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ExecuteReleasePlan(ctx context.Context, p Principal, project, id, token string, m map[string]any, engine *dag.Engine) (ReleaseDeployment, error) {
	var req struct {
		ProjectID       string `json:"project_id"`
		ExpectedVersion int    `json:"expected_version"`
		IdempotencyKey  string `json:"idempotency_key"`
		PlanRevision    int    `json:"plan_revision"`
		PlanSHA256      string `json:"plan_sha256"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return ReleaseDeployment{}, err
	}
	for _, k := range []string{"expected_version", "plan_revision"} {
		if err := exactVersion(m, k, 1); err != nil {
			return ReleaseDeployment{}, err
		}
	}
	if engine == nil {
		return ReleaseDeployment{}, invalid("发布调度器未配置")
	}
	// This initial read only locates the immutable reserved identity. All authority,
	// receipts and state are checked again by hooks inside the managed transaction.
	read, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseDeployment{}, err
	}
	plan, err := s.releasePlanTx(ctx, read, p.TenantID, project, id)
	read.Rollback()
	if err != nil {
		return ReleaseDeployment{}, err
	}
	var out ReleaseDeployment
	var receiptHash string
	var replay bool
	workflow := dag.Workflow{Name: "受控发布 " + id, Nodes: []dag.Node{{ID: plan.Content.ReservedExecution.NodeID, Name: "发布与健康检查", Type: "external_workflow", Idempotent: false, TimeoutSeconds: 300, MaxRetries: 0, Config: map[string]any{"plugin": plan.Content.ProviderID, "workflow": rc.ReleaseProtocol, "input": map[string]any{}}}}}
	_, err = engine.CreateManagedRunBound(ctx, p.TenantID, project, plan.Content.ReservedExecution.RunID, "release-"+id, workflow, func(tx *sql.Tx) error {
		if e := s.releaseBrowserTx(ctx, tx, p, project, token, true); e != nil {
			return e
		}
		if e := s.guardReleaseExecutionRouteTx(ctx, tx, p.TenantID, project, plan.Content.ReservedExecution.RunID); e != nil {
			return e
		}
		var e error
		receiptHash, replay, e = s.receipt(ctx, tx, p, project, req.IdempotencyKey, map[string]any{"release_execute": id, "body": m})
		if e != nil {
			return e
		}
		if replay {
			if e = json.Unmarshal([]byte(receiptHash), &out); e != nil {
				return e
			}
			if out.PlanID != id {
				return conflict("执行回执不属于此计划")
			}
			return nil
		}
		current, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, id)
		if e != nil {
			return e
		}
		if current.ExecutionOperationID != "" {
			return conflict("计划已派发，请读取已有部署")
		}
		return releaseVersionCheck(m, current.Version, current)
	}, func(tx *sql.Tx, run string) error {
		if replay {
			return conflict("回执与管理执行状态不一致")
		}
		current, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, id)
		if e != nil {
			return e
		}
		if current.State != "approved" || current.Approval == nil || current.Approval.Decision != "approved" || current.Approval.PlanSHA256 != current.SHA256 {
			return releaseConflict("release_approval_required", "此固定计划尚未批准")
		}
		if e = s.releaseActorAdminTx(ctx, tx, p.TenantID, project, current.Approval.ActorID); e != nil {
			return e
		}
		if e = s.currentReleasePlanBasisTx(ctx, tx, p, project, current, ""); e != nil {
			return e
		}
		env, e := s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, current.Content.EnvironmentID, true)
		if e != nil {
			return e
		}
		op := "deploy-" + newID()
		oldEnv := env.Version
		env.Version++
		env.Generation = current.Content.Generation
		env.BarrierOperationID = op
		env.UpdatedAt = now()
		if e = s.saveReleaseJSON(ctx, tx, "environments", p.TenantID, project, env.ID, oldEnv, env); e != nil {
			return e
		}
		out = ReleaseDeployment{OperationID: op, ProjectID: project, Version: 1, PlanID: id, PlanRevision: current.Revision, PlanSHA256: current.SHA256, EnvironmentID: env.ID, ProviderID: env.ProviderID, Generation: env.Generation, ArtifactID: current.Content.ArtifactID, ArtifactSHA256: current.Content.ArtifactSHA256, RunID: run, NodeID: current.Content.ReservedExecution.NodeID, State: "queued", EffectState: "not_started", HealthState: "not_checked", ApprovedBy: current.Approval.ActorID, ExecutedBy: p.ID, CreatedAt: now(), UpdatedAt: now(), AllowedActions: []string{"reconcile"}, Blockers: []ReleaseBlocker{}}
		if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_deployments(operation_id,tenant_id,project_id,version,run_id,node_id,approved_by,executed_by,data)VALUES(?,?,?,?,?,?,?,?,?)`), op, p.TenantID, project, 1, run, out.NodeID, out.ApprovedBy, out.ExecutedBy, stringify(out)); e != nil {
			return e
		}
		oldPlan := current.Version
		current.Version++
		current.State = "executing"
		current.ExecutionOperationID = op
		current.AllowedActions = []string{"revoke"}
		if e = s.saveReleaseJSON(ctx, tx, "plans", p.TenantID, project, id, oldPlan, current); e != nil {
			return e
		}
		return s.saveReceipt(ctx, tx, p, project, req.IdempotencyKey, receiptHash, out)
	})
	return out, err
}
func (s *Service) ReleasePlans(ctx context.Context, p Principal, project string) ([]ReleasePlan, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT id FROM suma_release_plans WHERE tenant_id=? AND project_id=? ORDER BY id`), p.TenantID, project)
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
	out := []ReleasePlan{}
	for _, id := range ids {
		plan, e := s.ReleasePlan(ctx, p, project, id)
		if e != nil {
			return nil, e
		}
		out = append(out, plan)
	}
	return out, nil
}
func (s *Service) ReleasePlan(ctx context.Context, p Principal, project, id string) (ReleasePlan, error) {
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return ReleasePlan{}, err
	}
	tx, err := s.DB.BeginTx(ctx, s.evidenceSnapshotOptions())
	if err != nil {
		return ReleasePlan{}, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectSnapshotTx(ctx, tx, p, project); err != nil {
		return ReleasePlan{}, err
	}
	plan, err := s.releasePlanTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return plan, err
	}
	plan.Blockers = []ReleaseBlocker{}
	plan.AllowedActions = []string{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT data FROM suma_release_approvals WHERE tenant_id=? AND project_id=? AND plan_id=? ORDER BY version`), p.TenantID, project, id)
	if err != nil {
		return plan, err
	}
	plan.ApprovalHistory = []ReleaseApproval{}
	for rows.Next() {
		var raw string
		var approval ReleaseApproval
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return plan, err
		}
		if err = json.Unmarshal([]byte(raw), &approval); err != nil {
			rows.Close()
			return plan, err
		}
		plan.ApprovalHistory = append(plan.ApprovalHistory, approval)
	}
	if err = rows.Close(); err != nil {
		return plan, err
	}
	if plan.ExecutionOperationID != "" {
		deployment, e := s.releaseDeploymentTx(ctx, tx, p.TenantID, project, plan.ExecutionOperationID)
		if e != nil {
			return plan, e
		}
		if deployment.State == "deployed_healthy" || deployment.State == "deployed_unhealthy" || deployment.State == "failed" || deployment.State == "cancelled" {
			plan.Eligibility = false
			plan.Blockers = append(plan.Blockers, ReleaseBlocker{"plan_already_executed", "计划已执行；查看实际部署，后续变更需要新计划"})
			if _, e = s.validateReleaseCandidateTx(ctx, tx, p, project, plan.Content.CandidateID); e != nil {
				plan.Blockers = append(plan.Blockers, ReleaseBlocker{"release_basis_changed", e.Error()})
			}
			if plan.Approval != nil && plan.Approval.Decision == "approved" && s.releaseActorSnapshotAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil {
				plan.AllowedActions = append(plan.AllowedActions, "revoke")
			}
			return plan, nil
		}
	}
	basis := s.releasePlanBasisTx(ctx, tx, p, project, plan, plan.ExecutionOperationID)
	if basis != nil {
		code := "release_basis_changed"
		if e, ok := basis.(*APIError); ok {
			code = e.Code
		}
		plan.Blockers = append(plan.Blockers, ReleaseBlocker{code, basis.Error()})
	}
	plan.Eligibility = basis == nil
	admin := s.releaseActorSnapshotAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil
	if admin && plan.ExecutionOperationID == "" && basis == nil {
		if plan.State == "approved" {
			plan.AllowedActions = append(plan.AllowedActions, "execute", "revoke")
		} else if plan.State == "pending_approval" || plan.State == "rejected" || plan.State == "revoked" {
			plan.AllowedActions = append(plan.AllowedActions, "approve", "reject")
		}
	}
	if admin && plan.ExecutionOperationID != "" && plan.Approval != nil && plan.Approval.Decision == "approved" {
		plan.AllowedActions = append(plan.AllowedActions, "revoke")
	}
	return plan, nil
}
