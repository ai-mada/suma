package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) MigrateScheduling(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_scheduling_previews(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,applied_policy_version INTEGER NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`))
	return err
}

var schedulingResourceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

func schedulingWriteOptions(dialect string) *sql.TxOptions {
	if dialect == "postgres" || dialect == "oceanbase" {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}

func validateSchedulingPolicy(input dag.SchedulingPolicyInput) error {
	const max = int64(9007199254740991)
	valid := func(v int64) bool { return v >= 0 && v <= max }
	if !valid(input.ConcurrencyLimit) || len(input.ResourceLimits) > 64 || input.BudgetLimit != nil && !valid(*input.BudgetLimit) {
		return invalid("执行容量必须为 0–9007199254740991 的整数，最多配置 64 类资源")
	}
	for name, capacity := range input.ResourceLimits {
		if !schedulingResourceName.MatchString(name) || name == "execution_slots" || name == "budget" || capacity != nil && !valid(*capacity) {
			return invalid("资源名称或容量无效；执行槽位及预算请使用独立字段")
		}
	}
	return nil
}

func schedulingInput(policy dag.SchedulingPolicy) dag.SchedulingPolicyInput {
	return dag.SchedulingPolicyInput{ConcurrencyLimit: policy.ConcurrencyLimit, ResourceLimits: policy.ResourceLimits, BudgetLimit: policy.BudgetLimit}
}

func schedulingView(policy dag.SchedulingPolicy, canChange bool) SchedulingPolicyView {
	actions := []string{}
	if canChange {
		actions = []string{"preview", "commit"}
	}
	return SchedulingPolicyView{SchedulingPolicy: policy, AllowedActions: actions, Fairness: SchedulingFairness{Mode: "project_run_round_robin", Scope: "project"}}
}

func (s *Service) SchedulingPolicy(ctx context.Context, engine *dag.Engine, p Principal, project string, human bool) (SchedulingPolicyView, error) {
	var out SchedulingPolicyView
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return out, err
	}
	canChange := human && s.Authorize(ctx, p, project, true, true) == nil
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	policy, err := engine.ReadSchedulingPolicyTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	return schedulingView(policy, canChange), tx.Commit()
}

func schedulingUnit(name string) string {
	switch name {
	case "execution_slots", "gpu_slots":
		return "slots"
	case "cpu_millis":
		return "millicores"
	case "memory_bytes":
		return "bytes"
	case "model_concurrency":
		return "requests"
	case "budget":
		return "admission_units"
	default:
		return "units"
	}
}

func schedulingChanges(before, after dag.SchedulingPolicyInput) []SchedulingPolicyChange {
	beforeSlots, afterSlots := before.ConcurrencyLimit, after.ConcurrencyLimit
	b := map[string]*int64{"execution_slots": &beforeSlots, "budget": before.BudgetLimit}
	a := map[string]*int64{"execution_slots": &afterSlots, "budget": after.BudgetLimit}
	for name, cap := range before.ResourceLimits {
		b[name] = cap
	}
	for name, cap := range after.ResourceLimits {
		a[name] = cap
	}
	names := map[string]bool{}
	for name := range b {
		names[name] = true
	}
	for name := range a {
		names[name] = true
	}
	ordered := []string{}
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	out := []SchedulingPolicyChange{}
	for _, name := range ordered {
		x, y := b[name], a[name]
		if x == nil && y == nil || x != nil && y != nil && *x == *y {
			continue
		}
		out = append(out, SchedulingPolicyChange{Resource: name, Unit: schedulingUnit(name), Before: x, After: y})
	}
	return out
}

func (s *Service) schedulingPreviewTx(ctx context.Context, q governanceQuery, p Principal, project, id string) (SchedulingPolicyPreview, error) {
	var out SchedulingPolicyPreview
	var raw, state string
	var version, applied int
	err := q.QueryRowContext(ctx, s.q(`SELECT version,state,applied_policy_version,payload FROM suma_scheduling_previews WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&version, &state, &applied, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	out.Version, out.State, out.AppliedPolicyVersion = version, state, applied
	return out, nil
}

func (s *Service) validateSchedulingPreviewTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, p Principal, project string, preview SchedulingPolicyPreview) error {
	if preview.State != "ready" {
		return conflict("该预览已提交，请查看当前调度策略")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= preview.ExpiresAt {
		return conflict("调度预览已过期，请重新预览")
	}
	policy, err := engine.ReadSchedulingPolicyTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return err
	}
	if policy.Version != preview.PolicyVersion {
		return conflict("调度策略依据已变化，请重新预览")
	}
	return nil
}

func (s *Service) CreateSchedulingPreview(ctx context.Context, engine *dag.Engine, p Principal, project string, request SchedulingPolicyPreviewRequest, sessionToken string) (SchedulingPolicyPreview, error) {
	var out SchedulingPolicyPreview
	if request.ProjectID != project || request.ExpectedVersion < 0 {
		return out, invalid("请提供项目及当前策略版本")
	}
	if err := validateSchedulingPolicy(request.Policy); err != nil {
		return out, err
	}
	if request.Policy.ResourceLimits == nil {
		request.Policy.ResourceLimits = map[string]*int64{}
	}
	tx, err := s.DB.BeginTx(ctx, schedulingWriteOptions(s.Dialect))
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// Authority is rechecked before looking up even a successful operation receipt.
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, true); err != nil {
		return out, err
	}
	if err = s.currentSchedulingSessionTx(ctx, tx, p, sessionToken); err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, project); err != nil {
		return out, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, request.IdempotencyKey, map[string]any{"scheduling_preview": request})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt SchedulingPolicyPreview
		if err = json.Unmarshal([]byte(hash), &receipt); err != nil {
			return out, err
		}
		out, err = s.schedulingPreviewTx(ctx, tx, p, project, receipt.ID)
		if err != nil {
			return out, err
		}
		out.Valid, out.Reason = true, ""
		if check := s.validateSchedulingPreviewTx(ctx, tx, engine, p, project, out); check != nil {
			out.Valid, out.Reason = false, check.Error()
		}
		return out, tx.Commit()
	}
	policy, err := engine.ReadSchedulingPolicyTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	if policy.Version != request.ExpectedVersion {
		return out, conflict("调度策略已经变更，请刷新后重新预览")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	impact := SchedulingPolicyImpact{ObservedAt: policy.AsOf, BudgetSpent: policy.BudgetSpent, BudgetHistory: policy.BudgetHistory, Resources: policy.Resources, Notes: []string{"容量为项目执行准入额度，并非实测硬件或操作系统硬限制", "降低容量只阻止新派发，已运行尝试继续；累计准入单位不退款", "影响数据为预览时快照；提交时重新核验权限与策略版本"}}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_ready_v2 q JOIN dag_nodes n ON n.run_id=q.run_id AND n.id=q.node_id WHERE q.tenant_id=? AND q.project_id=? AND n.state='ready'`), p.TenantID, project).Scan(&impact.ReadyTotal); err != nil {
		return out, err
	}
	for _, resource := range policy.Resources {
		if resource.Name == "execution_slots" {
			impact.ReservedExecutionSlots = resource.Used
		}
	}
	out = SchedulingPolicyPreview{ID: newID(), ProjectID: project, Version: 1, State: "ready", PolicyVersion: policy.Version, Policy: request.Policy, PreviousPolicy: schedulingInput(policy), Changes: schedulingChanges(schedulingInput(policy), request.Policy), Impact: impact, PolicySHA256: digest(stringify(request.Policy)), CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), Valid: true}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_scheduling_previews(id,tenant_id,project_id,version,state,applied_policy_version,created_by,created_at,expires_at,payload)VALUES(?,?,?,1,'ready',0,?,?,?,?)`), out.ID, p.TenantID, project, p.ID, out.CreatedAt, out.ExpiresAt, stringify(out))
	if err != nil {
		return out, err
	}
	if err = s.saveReceipt(ctx, tx, p, project, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.activity(ctx, tx, p, project, "scheduling.preview", out.ID, "审阅项目执行准入策略"); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) GetSchedulingPreview(ctx context.Context, engine *dag.Engine, p Principal, project, id string) (SchedulingPolicyPreview, error) {
	var out SchedulingPolicyPreview
	if err := s.Authorize(ctx, p, project, false, false); err != nil {
		return out, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out, err = s.schedulingPreviewTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	out.Valid, out.Reason = true, ""
	if check := s.validateSchedulingPreviewTx(ctx, tx, engine, p, project, out); check != nil {
		out.Valid, out.Reason = false, check.Error()
	}
	return out, tx.Commit()
}

func (s *Service) CommitSchedulingPreview(ctx context.Context, engine *dag.Engine, p Principal, project, id string, request SchedulingPolicyCommitRequest, sessionToken string) (SchedulingPolicyView, error) {
	var out SchedulingPolicyView
	if request.ProjectID != project || request.ExpectedVersion < 1 {
		return out, invalid("请提供项目及当前预览版本")
	}
	tx, err := s.DB.BeginTx(ctx, schedulingWriteOptions(s.Dialect))
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, true); err != nil {
		return out, err
	}
	if err = s.currentSchedulingSessionTx(ctx, tx, p, sessionToken); err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, project); err != nil {
		return out, err
	}
	hash, replay, err := s.receipt(ctx, tx, p, project, request.IdempotencyKey, map[string]any{"scheduling_commit": id, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	preview, err := s.schedulingPreviewTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if preview.Version != request.ExpectedVersion {
		return out, conflict("调度预览版本已经变化，请刷新核对")
	}
	if err = s.validateSchedulingPreviewTx(ctx, tx, engine, p, project, preview); err != nil {
		return out, err
	}
	policy, err := engine.WriteSchedulingPolicyTx(ctx, tx, p.TenantID, project, preview.PolicyVersion, preview.Policy)
	if err != nil {
		return out, err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_scheduling_previews SET state='committed',version=version+1,applied_policy_version=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND state='ready'`), policy.Version, p.TenantID, project, id, request.ExpectedVersion)
	if err != nil {
		return out, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return out, err
	}
	if n != 1 {
		return out, conflict("调度预览已由另一操作提交")
	}
	out = schedulingView(policy, true)
	if err = s.saveReceipt(ctx, tx, p, project, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.activity(ctx, tx, p, project, "scheduling.commit", id, "应用项目执行准入策略"); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// Current human credential and authority are checked in the mutation transaction,
// including receipt recovery. Session row locking follows the organization lock
// and precedes the scheduler project/family locks. Scoped credentials are never
// upgraded to human policy approval by a cached outer Principal.
func (s *Service) currentSchedulingSessionTx(ctx context.Context, tx *sql.Tx, p Principal, token string) error {
	query := `SELECT user_id,tenant_id,expires_at FROM suma_sessions WHERE token_hash=?`
	if s.Dialect == "oceanbase" || s.Dialect == "postgres" {
		query += ` FOR UPDATE`
	}
	var user, tenant, expires string
	err := tx.QueryRowContext(ctx, s.q(query), digest(token)).Scan(&user, &tenant, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return forbidden()
	}
	if err != nil {
		return err
	}
	if user != p.ID || tenant != p.TenantID {
		return forbidden()
	}
	var scoped int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_token_scopes WHERE token_hash=?`), digest(token)).Scan(&scoped); err != nil {
		return err
	}
	if scoped != 0 {
		return forbidden()
	}
	expiry, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return forbidden()
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if expiry.Unix() <= clock.Unix() {
		return forbidden()
	}
	return nil
}
