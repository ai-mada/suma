package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/dag"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

// ReleaseProviders is startup administrator configuration, never browser input.
// TokenHash authenticates the provider's independent control-plane connection.
type ReleaseProviderConfig struct {
	ID             string   `json:"id"`
	TenantID       string   `json:"tenant_id"`
	ProjectID      string   `json:"project_id"`
	TokenHash      string   `json:"-"`
	SiteIDs        []string `json:"site_ids"`
	Origin         string   `json:"-"`
	ReconcileToken string   `json:"-"`
}
type ReleaseBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ReleaseEnvironment struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"project_id"`
	Version              int             `json:"version"`
	Name                 string          `json:"name"`
	Status               string          `json:"status"`
	ProviderID           string          `json:"provider_id"`
	SiteID               string          `json:"site_id"`
	PolicyVersion        int             `json:"policy_version"`
	Health               rc.HealthPolicy `json:"health"`
	Generation           int64           `json:"generation"`
	ActiveGeneration     int64           `json:"active_generation"`
	ActiveArtifactID     string          `json:"active_artifact_id"`
	ActiveArtifactSHA256 string          `json:"active_artifact_sha256"`
	BarrierOperationID   string          `json:"barrier_operation_id"`
	CapabilityState      string          `json:"capability_state"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	AllowedActions       []string        `json:"allowed_actions"`
}
type ReleaseApproval struct {
	ActorID    string `json:"actor_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
	At         string `json:"at"`
	PlanSHA256 string `json:"plan_sha256"`
}
type ReleasePlan struct {
	ID                   string                `json:"id"`
	ProjectID            string                `json:"project_id"`
	Version              int                   `json:"version"`
	State                string                `json:"state"`
	Revision             int                   `json:"revision"`
	SHA256               string                `json:"sha256"`
	Content              rc.ReleasePlanContent `json:"content"`
	CreatedBy            string                `json:"created_by"`
	CreatedAt            string                `json:"created_at"`
	Approval             *ReleaseApproval      `json:"approval"`
	ApprovalHistory      []ReleaseApproval     `json:"approval_history,omitempty"`
	ExecutionOperationID string                `json:"execution_operation_id"`
	Eligibility          bool                  `json:"eligibility"`
	Blockers             []ReleaseBlocker      `json:"blockers"`
	AllowedActions       []string              `json:"allowed_actions"`
}
type ReleasePlanInput struct {
	ProjectID                     string `json:"project_id"`
	CandidateID                   string `json:"candidate_id"`
	EnvironmentID                 string `json:"environment_id"`
	Action                        string `json:"action"`
	ExpectedEnvironmentGeneration int64  `json:"expected_environment_generation"`
	RollbackSourceOperationID     string `json:"rollback_source_operation_id,omitempty"`
	PreviewSHA256                 string `json:"preview_sha256,omitempty"`
	IdempotencyKey                string `json:"idempotency_key,omitempty"`
}
type ReleasePlanPreview struct {
	PreviewSHA256  string             `json:"preview_sha256"`
	Environment    ReleaseEnvironment `json:"environment"`
	CandidateID    string             `json:"candidate_id"`
	Action         string             `json:"action"`
	Health         rc.HealthPolicy    `json:"health"`
	Eligibility    bool               `json:"eligibility"`
	Blockers       []ReleaseBlocker   `json:"blockers"`
	AllowedActions []string           `json:"allowed_actions"`
}
type ReleaseDeployment struct {
	OperationID    string                     `json:"operation_id"`
	ProjectID      string                     `json:"project_id"`
	Version        int                        `json:"version"`
	PlanID         string                     `json:"plan_id"`
	PlanRevision   int                        `json:"plan_revision"`
	PlanSHA256     string                     `json:"plan_sha256"`
	EnvironmentID  string                     `json:"environment_id"`
	ProviderID     string                     `json:"provider_id"`
	Generation     int64                      `json:"generation"`
	ArtifactID     string                     `json:"artifact_id"`
	ArtifactSHA256 string                     `json:"artifact_sha256"`
	RunID          string                     `json:"run_id"`
	NodeID         string                     `json:"node_id"`
	State          string                     `json:"state"`
	EffectState    string                     `json:"effect_state"`
	HealthState    string                     `json:"health_state"`
	Assignment     *rc.AssignmentRef          `json:"assignment,omitempty"`
	RequestSHA256  string                     `json:"request_sha256"`
	Observation    *rc.DeploymentObservation  `json:"observation"`
	Observations   []rc.DeploymentObservation `json:"observations,omitempty"`
	IssueID        string                     `json:"issue_id"`
	CreatedAt      string                     `json:"created_at"`
	UpdatedAt      string                     `json:"updated_at"`
	ApprovedBy     string                     `json:"-"`
	ExecutedBy     string                     `json:"-"`
	AllowedActions []string                   `json:"allowed_actions"`
	Blockers       []ReleaseBlocker           `json:"blockers"`
}

func (s *Service) InitializeReleaseDeployment(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_release_environments(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_release_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_release_approvals(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL,UNIQUE(tenant_id,project_id,plan_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_release_deployments(operation_id VARCHAR(128) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,node_id VARCHAR(150) NOT NULL,approved_by VARCHAR(64) NOT NULL,executed_by VARCHAR(64) NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_release_authorizations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,ticket_hash VARCHAR(64) NOT NULL UNIQUE,expires_at VARCHAR(40) NOT NULL,state VARCHAR(30) NOT NULL,assignment TEXT NOT NULL,request_sha256 VARCHAR(64) NOT NULL,effect_grant_id VARCHAR(64) NOT NULL,grant_data TEXT NOT NULL,actor_id VARCHAR(64) NOT NULL,worker_token_hash VARCHAR(64) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_release_observations(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,version INTEGER NOT NULL,evidence_sha256 VARCHAR(64) NOT NULL,data TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,operation_id,version))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return nil
}
func releaseError(status int, code, message string) error {
	return &APIError{status, code, message, nil}
}
func releaseConflict(code, message string) error { return releaseError(409, code, message) }
func releaseDecode(m map[string]any, out any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return invalid("发布请求格式错误")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return invalidDiagnostic("validation.release_detail", "发布请求存在未知字段或错误类型: "+err.Error(), map[string]any{"detail": err.Error()})
	}
	return nil
}
func releaseID(id string) bool { return len(id) > 0 && len(id) <= 100 && rc.ValidProfileID(id) }
func (s *Service) releaseProvider(tenant, project, provider string) (ReleaseProviderConfig, error) {
	for _, p := range s.ReleaseProviders {
		if p.ID == provider && p.TenantID == tenant && p.ProjectID == project && rc.ValidDigest(p.TokenHash) {
			return p, nil
		}
	}
	return ReleaseProviderConfig{}, releaseError(503, "provider_unavailable", "该项目尚未登记此发布提供方")
}
func (s *Service) releaseProviderSite(tenant, project, provider, site string) (ReleaseProviderConfig, error) {
	cfg, err := s.releaseProvider(tenant, project, provider)
	if err != nil {
		return cfg, err
	}
	for _, id := range cfg.SiteIDs {
		if id == site {
			return cfg, nil
		}
	}
	return ReleaseProviderConfig{}, releaseError(503, "provider_unavailable", "该站点已不在管理员当前发布登记中")
}
func (s *Service) authenticateReleaseProvider(token, provider string) (ReleaseProviderConfig, error) {
	hash := digest(token)
	if token != "" {
		for _, p := range s.ReleaseProviders {
			if (provider == "" || p.ID == provider) && rc.ValidDigest(p.TokenHash) && subtle.ConstantTimeCompare([]byte(hash), []byte(p.TokenHash)) == 1 {
				return p, nil
			}
		}
	}
	return ReleaseProviderConfig{}, forbidden()
}
func (s *Service) ReleaseProviderOptions(ctx context.Context, actor Actor, project string) ([]map[string]any, error) {
	var result0 []map[string]any
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseProviderOptionsTx(bounded, auth, project)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseProviderOptionsTx(ctx context.Context, auth *authorizedCommand, project string) ([]map[string]any, error) {
	if auth == nil {
		return nil, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return nil, e
	}
	if auth.target.projectID != project {
		return nil, forbidden()
	}

	p := auth.currentPrincipal

	out := []map[string]any{}
	for _, v := range s.ReleaseProviders {
		if v.TenantID == p.TenantID && v.ProjectID == project {
			out = append(out, map[string]any{"id": v.ID, "site_ids": v.SiteIDs, "kind": "local_static_site_v1", "capability_state": "configured"})
		}
	}
	return out, nil

}

func (s *Service) releaseBrowserTx(ctx context.Context, tx *sql.Tx, actor Actor, project string, admin bool) (*authorizedCommand, error) {
	if admin {
		return s.artifactAdminTx(ctx, tx, actor, project)
	}
	return s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectWrite)
}
func (s *Service) releaseSchedulerLock(ctx context.Context, tx *sql.Tx, tenant, project string) error {
	return dag.GuardProjectTx(ctx, tx, s.Dialect, tenant, project)
}
func (s *Service) releaseEnvironmentTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, lock bool) (ReleaseEnvironment, error) {
	var out ReleaseEnvironment
	var raw string
	q := `SELECT data FROM suma_release_environments WHERE tenant_id=? AND project_id=? AND id=?`
	if lock && s.Dialect != "sqlite" {
		q += ` FOR UPDATE`
	}
	if err := tx.QueryRowContext(ctx, s.q(q), tenant, project, id).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return out, missing()
		}
		return out, err
	}
	err := json.Unmarshal([]byte(raw), &out)
	out.AllowedActions = []string{}
	return out, err
}
func (s *Service) releasePlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (ReleasePlan, error) {
	return s.releasePlanReadTx(ctx, tx, tenant, project, id, false)
}
func (s *Service) currentReleasePlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (ReleasePlan, error) {
	return s.releasePlanReadTx(ctx, tx, tenant, project, id, true)
}
func (s *Service) releasePlanReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, current bool) (ReleasePlan, error) {
	var out ReleasePlan
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT data FROM suma_release_plans WHERE tenant_id=? AND project_id=? AND id=?`, current)), tenant, project, id).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return out, missing()
		}
		return out, err
	}
	err := json.Unmarshal([]byte(raw), &out)
	return out, err
}
func (s *Service) releaseDeploymentTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (ReleaseDeployment, error) {
	return s.releaseDeploymentReadTx(ctx, tx, tenant, project, id, false)
}
func (s *Service) currentReleaseDeploymentTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (ReleaseDeployment, error) {
	return s.releaseDeploymentReadTx(ctx, tx, tenant, project, id, true)
}
func (s *Service) releaseDeploymentReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, current bool) (ReleaseDeployment, error) {
	var out ReleaseDeployment
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT data,approved_by,executed_by FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND operation_id=?`, current)), tenant, project, id).Scan(&raw, &out.ApprovedBy, &out.ExecutedBy); err != nil {
		if err == sql.ErrNoRows {
			return out, missing()
		}
		return out, err
	}
	approved, executed := out.ApprovedBy, out.ExecutedBy
	err := json.Unmarshal([]byte(raw), &out)
	out.ApprovedBy, out.ExecutedBy = approved, executed
	return out, err
}
func (s *Service) saveReleaseJSON(ctx context.Context, tx *sql.Tx, table, tenant, project, id string, version int, out any) error {
	idCol := "id"
	if table == "deployments" {
		idCol = "operation_id"
	}
	r, err := tx.ExecContext(ctx, s.q(`UPDATE suma_release_`+table+` SET version=?,data=? WHERE tenant_id=? AND project_id=? AND `+idCol+`=? AND version=?`), version+1, stringify(out), tenant, project, id, version)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict("发布记录已变化，请刷新")
	}
	return nil
}
func (s *Service) ReleaseEnvironments(ctx context.Context, actor Actor, project string) ([]ReleaseEnvironment, error) {
	var result0 []ReleaseEnvironment
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseEnvironmentsTx(bounded, auth, project)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseEnvironmentsTx(ctx context.Context, auth *authorizedCommand, project string) ([]ReleaseEnvironment, error) {
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

	admin := auth.actor.IsBrowser() && s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil
	rows, err := tx.QueryContext(ctx, s.q(`SELECT data FROM suma_release_environments WHERE tenant_id=? AND project_id=? ORDER BY id`), p.TenantID, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseEnvironment{}
	for rows.Next() {
		var raw string
		var e ReleaseEnvironment
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, err
		}
		e.AllowedActions = []string{}
		if admin && e.BarrierOperationID == "" {
			e.AllowedActions = append(e.AllowedActions, "edit")
			if e.Status == "active" {
				e.AllowedActions = append(e.AllowedActions, "archive")
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()

}

func (s *Service) ReleaseEnvironment(ctx context.Context, actor Actor, project, id string) (ReleaseEnvironment, error) {
	var result0 ReleaseEnvironment
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseEnvironmentTx(bounded, auth, project, id)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseEnvironmentTx(ctx context.Context, auth *authorizedCommand, project, id string) (ReleaseEnvironment, error) {
	if auth == nil {
		return ReleaseEnvironment{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ReleaseEnvironment{}, e
	}
	if auth.target.projectID != project {
		return ReleaseEnvironment{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	admin := auth.actor.IsBrowser() && s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil
	out, err := s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, id, false)
	out.AllowedActions = []string{}
	if admin && out.BarrierOperationID == "" {
		out.AllowedActions = append(out.AllowedActions, "edit")
		if out.Status == "active" {
			out.AllowedActions = append(out.AllowedActions, "archive")
		}
	}
	return out, err

}

func (s *Service) ReleaseEnvironmentActions(ctx context.Context, actor Actor, project string) ([]string, error) {
	var result0 []string
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readReleaseEnvironmentActionsTx(bounded, auth, project)
		return e
	})
	return result0, err
}
func (s *Service) readReleaseEnvironmentActionsTx(ctx context.Context, auth *authorizedCommand, project string) ([]string, error) {
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

	out := []string{}
	if auth.actor.IsBrowser() && s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID) == nil {
		out = append(out, "create")
	}
	return out, nil

}

func (s *Service) SaveReleaseEnvironment(ctx context.Context, actor Actor, project, id string, m map[string]any) (ReleaseEnvironment, error) {
	p := actor.Projection()

	var req struct {
		ProjectID       string           `json:"project_id"`
		Name            *string          `json:"name"`
		ProviderID      string           `json:"provider_id"`
		SiteID          string           `json:"site_id"`
		Health          *rc.HealthPolicy `json:"health"`
		Status          *string          `json:"status"`
		ExpectedVersion int              `json:"expected_version"`
		IdempotencyKey  string           `json:"idempotency_key"`
	}
	if err := releaseDecode(m, &req); err != nil {
		return ReleaseEnvironment{}, err
	}
	if id != "" {
		if err := exactVersion(m, "expected_version", 1); err != nil {
			return ReleaseEnvironment{}, err
		}
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ReleaseEnvironment{}, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.releaseBrowserTx(ctx, tx, actor, project, true)
	if err != nil {
		return ReleaseEnvironment{}, err
	}
	p = auth.currentPrincipal
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return ReleaseEnvironment{}, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"release_environment": id, "body": m})
	if err != nil {
		return ReleaseEnvironment{}, err
	}
	var out ReleaseEnvironment
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if id == "" {
		if req.Name == nil || req.ProviderID == "" || req.SiteID == "" || req.Status != nil {
			return out, invalid("环境必须提供名称和管理员登记的提供方/站点")
		}
		cfg, e := s.releaseProvider(p.TenantID, project, req.ProviderID)
		if e != nil {
			return out, e
		}
		found := false
		for _, site := range cfg.SiteIDs {
			if site == req.SiteID {
				found = true
			}
		}
		if !found {
			return out, invalid("站点未登记")
		}
		rows, e := tx.QueryContext(ctx, s.q(`SELECT data FROM suma_release_environments WHERE tenant_id=? AND project_id=?`), p.TenantID, project)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var raw string
			var existing ReleaseEnvironment
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return out, e
			}
			if e = json.Unmarshal([]byte(raw), &existing); e != nil {
				rows.Close()
				return out, e
			}
			if existing.ProviderID == req.ProviderID && existing.SiteID == req.SiteID {
				rows.Close()
				return out, releaseConflict("environment_identity_exists", "站点已绑定环境，归档也不复用身份")
			}
		}
		if e = rows.Close(); e != nil {
			return out, e
		}
		out = ReleaseEnvironment{ID: newID(), ProjectID: project, Version: 1, Name: *req.Name, Status: "active", ProviderID: req.ProviderID, SiteID: req.SiteID, PolicyVersion: 1, CapabilityState: "configured", CreatedAt: now(), UpdatedAt: now(), Health: rc.HealthPolicy{Checks: []rc.HealthCheck{{Path: "/", ExpectedStatus: 200, ExpectedFile: "index.html"}}, TimeoutMS: 2000, Attempts: 3}}
	} else {
		out, err = s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, id, true)
		if err != nil {
			return out, err
		}
		if out.Version != req.ExpectedVersion {
			return out, conflict("环境版本已变化")
		}
		if out.BarrierOperationID != "" {
			return out, releaseConflict("environment_reconciling", "环境仍有占用操作，须先核对")
		}
		if req.ProviderID != "" || req.SiteID != "" {
			return out, invalid("已创建环境不能换绑提供方或站点")
		}
		out.Version++
		out.PolicyVersion++
		out.UpdatedAt = now()
		if req.Name != nil {
			out.Name = *req.Name
		}
		if req.Status != nil {
			out.Status = *req.Status
		}
	}
	if req.Health != nil {
		out.Health = *req.Health
	}
	out.Name = strings.TrimSpace(out.Name)
	if out.Name == "" || len(out.Name) > 200 || out.Status != "active" && out.Status != "archived" {
		return out, invalid("环境名称或状态无效")
	}
	if err = rc.ValidateHealthPolicy(out.Health, false); err != nil {
		return out, invalid(err.Error())
	}
	out.AllowedActions = []string{"edit"}
	if out.Status == "active" {
		out.AllowedActions = append(out.AllowedActions, "archive")
	}
	if id == "" {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_environments(id,tenant_id,project_id,version,data)VALUES(?,?,?,?,?)`), out.ID, p.TenantID, project, out.Version, stringify(out))
	} else {
		err = s.saveReleaseJSON(ctx, tx, "environments", p.TenantID, project, id, req.ExpectedVersion, out)
	}
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func releaseSame(a, b any) bool { return stringify(a) == stringify(b) }
func releaseProjection(o rc.DeploymentObservation) string {
	if o.Effect.State == "unknown" || o.State == "reconciling" {
		return "reconciling"
	}
	if o.Effect.State == "applied" {
		if o.Health.State == "passed" && o.State == "succeeded" {
			return "deployed_healthy"
		}
		if o.Health.State == "failed" {
			return "deployed_unhealthy"
		}
		return "reconciling"
	}
	if o.State == "failed" || o.State == "cancelled" {
		return o.State
	}
	return o.State
}
func releaseVersionCheck(m map[string]any, version int, plan ReleasePlan) error {
	for _, key := range []string{"expected_version", "plan_revision"} {
		if err := exactVersion(m, key, 1); err != nil {
			return err
		}
	}
	if releaseInteger(m, "expected_version") != version || releaseInteger(m, "plan_revision") != plan.Revision || str(m, "plan_sha256") != plan.SHA256 {
		return conflict("计划版本或内容已变化，请刷新后重新审核")
	}
	return nil
}
func releaseInteger(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return -1
}
func releasePlanHash(c rc.ReleasePlanContent) string {
	h, err := rc.CanonicalDigest(c)
	if err != nil {
		panic(fmt.Sprintf("release plan encoding: %v", err))
	}
	return h
}
