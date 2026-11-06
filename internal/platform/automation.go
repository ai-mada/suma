package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cronengine "github.com/ai-mada/suma/internal/automation"
	"github.com/ai-mada/suma/internal/store"
)

type AutomationSpec struct {
	Name            string `json:"name"`
	WorkflowID      string `json:"workflow_id"`
	WorkflowVersion int    `json:"workflow_version"`
	Trigger         string `json:"trigger"`
	Cron            string `json:"cron"`
	Timezone        string `json:"timezone"`
	Misfire         string `json:"misfire"`
	CatchUpLimit    int    `json:"catch_up_limit"`
	Overlap         string `json:"overlap"`
}
type AutomationRevision struct {
	Revision  int            `json:"revision"`
	Spec      AutomationSpec `json:"spec"`
	CreatedAt string         `json:"created_at"`
}
type AutomationRule struct {
	AutomationSpec
	ID          string               `json:"id"`
	ProjectID   string               `json:"project_id"`
	Version     int                  `json:"version"`
	Revision    int                  `json:"revision"`
	State       string               `json:"state"`
	ActivatedBy string               `json:"activated_by"`
	NextAt      string               `json:"next_at"`
	CreatedAt   string               `json:"created_at"`
	UpdatedAt   string               `json:"updated_at"`
	LastError   string               `json:"last_error"`
	Revisions   []AutomationRevision `json:"revisions,omitempty"`
}
type AutomationOccurrence struct {
	ID                     string `json:"id"`
	RuleID                 string `json:"rule_id"`
	Revision               int    `json:"revision"`
	Source                 string `json:"source"`
	ScheduledAt            string `json:"scheduled_at"`
	State                  string `json:"state"`
	Reason                 string `json:"reason"`
	RunID                  string `json:"run_id"`
	RunState               string `json:"run_state,omitempty"`
	RunNeedsReconciliation bool   `json:"run_needs_reconciliation,omitempty"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}
type AutomationCreateRequest struct {
	ProjectID      string         `json:"project_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	Spec           AutomationSpec `json:"spec"`
}
type AutomationPreviewRequest struct {
	ProjectID string         `json:"project_id"`
	Spec      AutomationSpec `json:"spec"`
}
type AutomationCommandRequest struct {
	ProjectID       string          `json:"project_id"`
	Action          string          `json:"action"`
	ExpectedVersion int             `json:"expected_version"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Spec            *AutomationSpec `json:"spec,omitempty"`
}
type AutomationCommandResult struct {
	Rule       AutomationRule        `json:"rule"`
	Occurrence *AutomationOccurrence `json:"occurrence,omitempty"`
}
type AutomationPreviewTime struct {
	UTC   string `json:"utc"`
	Local string `json:"local"`
}
type AutomationPreview struct {
	Items []AutomationPreviewTime `json:"items"`
}
type AutomationList struct {
	Items      []AutomationRule `json:"items"`
	Total      int              `json:"total"`
	NextCursor string           `json:"next_cursor"`
}
type AutomationOccurrences struct {
	Items      []AutomationOccurrence `json:"items"`
	Total      int                    `json:"total"`
	NextCursor string                 `json:"next_cursor"`
}
type AutomationRevisions struct {
	Items      []AutomationRevision `json:"items"`
	Total      int                  `json:"total"`
	NextCursor string               `json:"next_cursor"`
}

func (s *Service) InitializeAutomations(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS suma_automations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,revision INTEGER NOT NULL,state VARCHAR(20) NOT NULL,activated_by VARCHAR(64) NOT NULL,next_at VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,last_error TEXT NOT NULL,spec TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_automation_revisions(rule_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,spec TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(rule_id,revision))`,
		`CREATE TABLE IF NOT EXISTS suma_automation_occurrences(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,rule_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,source VARCHAR(20) NOT NULL,scheduled_at VARCHAR(40) NOT NULL,state VARCHAR(20) NOT NULL,reason TEXT NOT NULL,run_id VARCHAR(64) NOT NULL,dedupe_key VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,rule_id,revision,dedupe_key))`,
		automationManualInvocationsDDL,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(q)); err != nil {
			return fmt.Errorf("automation migration: %w", err)
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_automations_due", "suma_automations", []string{"state", "next_at", "id"}},
		{"suma_automations_scope", "suma_automations", []string{"tenant_id", "project_id", "id"}},
		{"suma_automation_occ_dispatch", "suma_automation_occurrences", []string{"state", "updated_at", "created_at", "id"}},
		{"suma_automation_occ_frontier", "suma_automation_occurrences", []string{"rule_id", "state", "scheduled_at", "created_at", "id"}},
		{"suma_automation_occ_scope", "suma_automation_occurrences", []string{"tenant_id", "project_id", "rule_id", "created_at", "id"}},
		{"suma_automation_occ_active", "suma_automation_occurrences", []string{"tenant_id", "project_id", "rule_id", "run_id"}},
		{"suma_automation_occ_run", "suma_automation_occurrences", []string{"tenant_id", "project_id", "run_id"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); err != nil {
			return err
		}
	}
	return nil
}
func strictAutomation(m map[string]any, out any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return invalid("无效自动化请求")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return invalidDiagnostic("validation.automation_detail", "自动化字段或类型无效: "+err.Error(), map[string]any{"detail": err.Error()})
	}
	return nil
}
func (s *Service) automationClock(ctx context.Context, q governanceQuery) (time.Time, error) {
	return s.authorityNow(ctx, q)
}
func automationStamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func validateAutomationSpec(a AutomationSpec) (*cronengine.Schedule, error) {
	if strings.TrimSpace(a.Name) == "" || len(a.Name) > 200 || a.Name != strings.TrimSpace(a.Name) || len(a.WorkflowID) > 100 || a.WorkflowID == "" || a.WorkflowVersion < 1 || a.WorkflowVersion > math.MaxInt32 {
		return nil, invalid("规则名称或固定流程版本无效")
	}
	if a.Trigger != "cron" && a.Trigger != "manual" {
		return nil, invalid("trigger 必须为 cron 或 manual")
	}
	if a.Misfire != "skip" && a.Misfire != "latest" && a.Misfire != "catch_up" {
		return nil, invalid("misfire 无效")
	}
	if a.Overlap != "allow" && a.Overlap != "forbid" && a.Overlap != "queue" {
		return nil, invalid("overlap 无效")
	}
	if a.CatchUpLimit < 1 || a.CatchUpLimit > 100 {
		return nil, invalid("catch_up_limit 必须为 1..100")
	}
	expr := a.Cron
	if a.Trigger == "manual" {
		if a.Cron != "" {
			return nil, invalid("人工规则不接受 cron")
		}
		expr = "* * * * *"
	}
	c, err := cronengine.Parse(expr, a.Timezone)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return c, nil
}
func (s *Service) automationWorkflow(ctx context.Context, tx *sql.Tx, p Principal, project string, a AutomationSpec) error {
	var count int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND workflow_id=?`), p.TenantID, project, a.WorkflowID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return invalid("受保护发布流程不能被自动化引用")
	}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_compensations c JOIN dag_runs r ON r.id=c.compensation_run_id WHERE r.tenant_id=? AND r.project_id=? AND r.workflow_id=?`), p.TenantID, project, a.WorkflowID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return invalid("生成的补偿流程只作历史证据，不能作为自动化模板")
	}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), p.TenantID, project, a.WorkflowID, a.WorkflowVersion).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return invalid("固定流程版本不存在或不属于当前项目")
	}
	return nil
}

const automationColumns = `id,project_id,version,revision,state,activated_by,next_at,created_at,updated_at,last_error,spec`

type automationScanner interface{ Scan(...any) error }

func scanAutomation(row automationScanner) (AutomationRule, error) {
	var a AutomationRule
	var raw string
	err := row.Scan(&a.ID, &a.ProjectID, &a.Version, &a.Revision, &a.State, &a.ActivatedBy, &a.NextAt, &a.CreatedAt, &a.UpdatedAt, &a.LastError, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &a.AutomationSpec)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = missing()
	}
	return a, err
}
func (s *Service) automationRuleTx(ctx context.Context, tx governanceQuery, p Principal, project, id string) (AutomationRule, error) {
	return scanAutomation(tx.QueryRowContext(ctx, s.q(`SELECT `+automationColumns+` FROM suma_automations WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
}
func (s *Service) saveAutomation(ctx context.Context, auth *authorizedCommand, a AutomationRule) error {
	if auth == nil {
		return forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, auth.purpose); err != nil {
		return err
	}
	if auth.actor.kind != actorBrowser || (auth.purpose != purposeBrowserProjectWrite && auth.purpose != purposeBrowserProjectApprove) {
		return forbidden()
	}
	if auth.target.projectID != a.ProjectID || auth.target.resourceID != a.ID {
		return forbidden()
	}
	tx, p := auth.tx, auth.currentPrincipal
	_, err := tx.ExecContext(ctx, s.q(`UPDATE suma_automations SET version=?,revision=?,state=?,activated_by=?,next_at=?,updated_at=?,last_error=?,spec=? WHERE tenant_id=? AND project_id=? AND id=?`), a.Version, a.Revision, a.State, a.ActivatedBy, a.NextAt, a.UpdatedAt, a.LastError, stringify(a.AutomationSpec), p.TenantID, a.ProjectID, a.ID)
	return err
}
func (s *Service) CreateAutomation(ctx context.Context, actor Actor, project string, req AutomationCreateRequest) (AutomationRule, error) {
	var out AutomationRule
	if _, err := validateAutomationSpec(req.Spec); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"automation_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if err = s.automationWorkflow(ctx, tx, p, project, req.Spec); err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	out = AutomationRule{AutomationSpec: req.Spec, ID: newID(), ProjectID: project, Version: 1, Revision: 1, State: "draft", CreatedAt: at, UpdatedAt: at}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automations(id,tenant_id,project_id,version,revision,state,activated_by,next_at,created_at,updated_at,last_error,spec)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), out.ID, p.TenantID, project, 1, 1, "draft", "", "", at, at, "", stringify(req.Spec))
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_revisions(rule_id,revision,spec,created_at)VALUES(?,?,?,?)`), out.ID, 1, stringify(req.Spec), at); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "automation.created", out.ID, out.Name); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) previewAutomationCurrentTx(ctx context.Context, auth *authorizedCommand, project string, a AutomationSpec) (AutomationPreview, error) {
	if auth == nil {
		return AutomationPreview{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: ""}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationPreview{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationPreview{Items: []AutomationPreviewTime{}}
	c, err := validateAutomationSpec(a)
	if err != nil {
		return out, err
	}
	if err = s.automationWorkflow(ctx, tx, p, project, a); err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if a.Trigger == "manual" {
		return out, nil
	}
	for i := 0; i < 5; i++ {
		clock, err = c.NextContext(ctx, clock)
		if err != nil {
			return out, invalid(err.Error())
		}
		out.Items = append(out.Items, AutomationPreviewTime{automationStamp(clock), c.Local(clock)})
	}
	return out, nil
}
func (s *Service) getAutomationCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string) (AutomationRule, error) {
	if auth == nil {
		return AutomationRule{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationRule{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	var a AutomationRule
	a, err = s.automationRuleTx(ctx, tx, p, project, id)
	if err != nil {
		return a, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT revision,spec,created_at FROM suma_automation_revisions WHERE rule_id=? ORDER BY revision DESC LIMIT 100`), id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	a.Revisions = []AutomationRevision{}
	for rows.Next() {
		var rev AutomationRevision
		var raw string
		if err = rows.Scan(&rev.Revision, &raw, &rev.CreatedAt); err != nil {
			return a, err
		}
		if err = json.Unmarshal([]byte(raw), &rev.Spec); err != nil {
			return a, err
		}
		a.Revisions = append(a.Revisions, rev)
	}
	return a, rows.Err()
}
func automationPage(r *http.Request) (int, string, error) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 200 {
			return 0, "", invalid("limit 必须为 1..200")
		}
		limit = n
	}
	cursor := r.URL.Query().Get("cursor")
	if len(cursor) > 1024 {
		return 0, "", invalid("无效分页游标")
	}
	return limit, cursor, nil
}

type automationCursor struct {
	Tenant     string `json:"tenant"`
	Project    string `json:"project"`
	Collection string `json:"collection"`
	Last       string `json:"last"`
	At         string `json:"at,omitempty"`
}

func automationEncodeCursor(p Principal, project, collection, last string) string {
	raw, _ := json.Marshal(automationCursor{Tenant: p.TenantID, Project: project, Collection: collection, Last: last})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func automationDecodeCursor(p Principal, project, collection, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if len(encoded) > 1024 {
		return "", invalid("无效分页游标")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", invalid("无效分页游标")
	}
	var c automationCursor
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil || c.Tenant != p.TenantID || c.Project != project || c.Collection != collection || len(c.Last) != 32 {
		return "", invalid("分页游标不属于当前范围")
	}
	for _, r := range c.Last {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", invalid("无效分页游标")
		}
	}
	return c.Last, nil
}

func (s *Service) listAutomationsCurrentTx(ctx context.Context, auth *authorizedCommand, project string, limit int, cursor string) (AutomationList, error) {
	if auth == nil {
		return AutomationList{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: ""}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationList{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationList{Items: []AutomationRule{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	last, err := automationDecodeCursor(p, project, "rules", cursor)
	if err != nil {
		return out, err
	}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automations WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+automationColumns+` FROM suma_automations WHERE tenant_id=? AND project_id=? AND id>? ORDER BY id LIMIT ?`), p.TenantID, project, last, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		a, e := scanAutomation(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, a)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, project, "rules", out.Items[limit-1].ID)
	}
	return out, rows.Err()
}
func (s *Service) insertAutomationOccurrence(ctx context.Context, auth *authorizedCommand, a AutomationRule, source, scheduled, state, reason, key string) (AutomationOccurrence, error) {
	if auth == nil {
		return AutomationOccurrence{}, forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, auth.purpose); err != nil {
		return AutomationOccurrence{}, err
	}
	if auth.target.projectID != a.ProjectID {
		return AutomationOccurrence{}, forbidden()
	}
	switch auth.purpose {
	case purposeBrowserProjectWrite:
		if source != "manual" || auth.actor.kind != actorBrowser || auth.target.resourceID != a.ID {
			return AutomationOccurrence{}, forbidden()
		}
	case purposeAutomationMaterialize:
		if source != "cron" || auth.actor.delegation.recordID != a.ID || auth.actor.delegation.revision != a.Revision {
			return AutomationOccurrence{}, forbidden()
		}
	case purposeAutomationEvaluate:
		if source != "cron" || state != "blocked" || auth.target.resourceID != a.ID {
			return AutomationOccurrence{}, forbidden()
		}
	case purposeIngressDelivery:
		if source != "webhook" || auth.actor.kind != actorSignedIngress {
			return AutomationOccurrence{}, forbidden()
		}
		in, e := s.ingressTx(ctx, auth.tx, auth.currentPrincipal, a.ProjectID, auth.actor.delegation.recordID)
		if e != nil {
			return AutomationOccurrence{}, e
		}
		if in.RuleID != a.ID || in.RuleRevision != a.Revision {
			return AutomationOccurrence{}, forbidden()
		}
		reason, e := s.ingressAdmission(ctx, auth.tx, in, a)
		if e != nil {
			return AutomationOccurrence{}, e
		}
		if reason != "" {
			return AutomationOccurrence{}, forbidden()
		}
	default:
		return AutomationOccurrence{}, forbidden()
	}
	tx, p := auth.tx, auth.currentPrincipal
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return AutomationOccurrence{}, err
	}
	at := automationStamp(clock)
	o := AutomationOccurrence{ID: newID(), RuleID: a.ID, Revision: a.Revision, Source: source, ScheduledAt: scheduled, State: state, Reason: reason, CreatedAt: at, UpdatedAt: at}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_occurrences(id,tenant_id,project_id,rule_id,revision,source,scheduled_at,state,reason,run_id,dedupe_key,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), o.ID, p.TenantID, a.ProjectID, a.ID, a.Revision, source, scheduled, state, reason, "", digest(key), at, at)
	if err == nil && source == "manual" {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_manual_invocations(tenant_id,project_id,occurrence_id,rule_id,invoker_user_id) VALUES(?,?,?,?,?)`), p.TenantID, a.ProjectID, o.ID, a.ID, p.ID)
	}
	return o, err
}
func (s *Service) CommandAutomation(ctx context.Context, actor Actor, project, id string, req AutomationCommandRequest) (AutomationCommandResult, error) {
	var out AutomationCommandResult
	if req.ExpectedVersion < 1 || req.ExpectedVersion > math.MaxInt32 {
		return out, invalid("expected_version 必须为正整数")
	}
	if req.Action != "revise" && req.Action != "enable" && req.Action != "pause" && req.Action != "archive" && req.Action != "trigger" {
		return out, invalid("无效自动化命令")
	}
	if (req.Action == "revise") != (req.Spec != nil) {
		return out, invalid("仅 revise 必须携带完整 spec")
	}
	if req.Spec != nil {
		if _, err := validateAutomationSpec(*req.Spec); err != nil {
			return out, err
		}
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	purpose := purposeBrowserProjectWrite
	if req.Action == "enable" {
		purpose = purposeBrowserProjectApprove
	}
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purpose)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"automation_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	a, err := s.automationRuleTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if a.Version != req.ExpectedVersion {
		return out, conflict("自动化规则版本已变化")
	}
	if a.State == "archived" {
		return out, conflict("已归档规则不能修改或触发")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	switch req.Action {
	case "revise":
		if err = s.automationWorkflow(ctx, tx, p, project, *req.Spec); err != nil {
			return out, err
		}
		a.AutomationSpec = *req.Spec
		a.Revision++
		a.State = "paused"
		a.NextAt = ""
		a.LastError = ""
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_revisions(rule_id,revision,spec,created_at)VALUES(?,?,?,?)`), a.ID, a.Revision, stringify(a.AutomationSpec), at); err != nil {
			return out, err
		}
	case "enable":
		if a.State == "enabled" {
			return out, conflict("规则已经启用")
		}
		if err = s.automationWorkflow(ctx, tx, p, project, a.AutomationSpec); err != nil {
			return out, err
		}
		a.State = "enabled"
		a.ActivatedBy = p.ID
		a.LastError = ""
		a.NextAt = ""
		if a.Trigger == "cron" {
			c, e := validateAutomationSpec(a.AutomationSpec)
			if e != nil {
				return out, e
			}
			next, e := c.NextContext(ctx, clock)
			if e != nil {
				return out, invalid(e.Error())
			}
			a.NextAt = automationStamp(next)
		}
	case "pause":
		a.State = "paused"
		a.NextAt = ""
	case "archive":
		a.State = "archived"
		a.NextAt = ""
	case "trigger":
		if a.State != "enabled" {
			return out, conflict("仅已启用规则可人工触发")
		}
		o, e := s.insertAutomationOccurrence(ctx, auth, a, "manual", at, "queued", "", p.ID+":"+req.IdempotencyKey)
		if e != nil {
			return out, e
		}
		out.Occurrence = &o
	}
	if req.Action == "revise" || req.Action == "pause" || req.Action == "archive" {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state='blocked',reason=?,updated_at=? WHERE tenant_id=? AND project_id=? AND rule_id=? AND state='queued'`), "rule_"+req.Action, at, p.TenantID, project, id); err != nil {
			return out, err
		}
	}
	a.Version++
	a.UpdatedAt = at
	out.Rule = a
	if err = s.saveAutomation(ctx, auth, a); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "automation."+req.Action, id, a.Name); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

const (
	automationOccurrenceAll       = "all"
	automationOccurrenceAttention = "attention"
)

func validAutomationOccurrenceFilter(filter string) bool {
	return filter == automationOccurrenceAll || filter == automationOccurrenceAttention
}

func (s *Service) automationOccurrencesCurrentTx(ctx context.Context, auth *authorizedCommand, project, id, filter string, limit int, cursor string) (AutomationOccurrences, error) {
	if auth == nil {
		return AutomationOccurrences{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationOccurrences{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationOccurrences{Items: []AutomationOccurrence{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	if !validAutomationOccurrenceFilter(filter) {
		return out, invalid("无效自动化请求")
	}
	collection := "occurrences:" + id + ":" + filter
	last, err := automationDecodeCursor(p, project, collection, cursor)
	if err != nil {
		return out, err
	}
	at := ""
	if cursor != "" {
		raw, _ := base64.RawURLEncoding.DecodeString(cursor)
		var c automationCursor
		_ = json.Unmarshal(raw, &c)
		parsed, e := time.Parse(time.RFC3339, c.At)
		if e != nil || automationStamp(parsed) != c.At {
			return out, invalid("无效时间分页游标")
		}
		at = c.At
	}
	if _, err := s.automationRuleTx(ctx, tx, p, project, id); err != nil {
		return out, err
	}
	const source = ` FROM suma_automation_occurrences o`
	const linkedRun = ` LEFT JOIN dag_runs r ON r.id=o.run_id AND r.tenant_id=o.tenant_id AND r.project_id=o.project_id`
	// A DAG node is scoped by its run. Use the joined run ID so an
	// occurrence with a missing or foreign run never inherits its facts.
	const reconciliation = `EXISTS (SELECT 1 FROM dag_nodes n WHERE n.run_id=r.id AND n.state='reconciling')`
	countSource := source
	where := ` WHERE o.tenant_id=? AND o.project_id=? AND o.rule_id=?`
	if filter == automationOccurrenceAttention {
		countSource += linkedRun
		where += ` AND (o.state='blocked' OR r.state='failed' OR ` + reconciliation + `)`
	}
	args := []any{p.TenantID, project, id}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+countSource+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	columns := `SELECT o.id,o.rule_id,o.revision,o.source,o.scheduled_at,o.state,o.reason,COALESCE(r.id,''),o.created_at,o.updated_at,COALESCE(r.state,''),CASE WHEN ` +
		reconciliation + ` THEN 1 ELSE 0 END`
	page := ` AND (?='' OR o.created_at<? OR (o.created_at=? AND o.id<?)) ORDER BY o.created_at DESC,o.id DESC LIMIT ?`
	rows, err := tx.QueryContext(ctx, s.q(columns+source+linkedRun+where+page), append(args, at, at, at, last, limit+1)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var o AutomationOccurrence
		if err = rows.Scan(&o.ID, &o.RuleID, &o.Revision, &o.Source, &o.ScheduledAt, &o.State, &o.Reason, &o.RunID, &o.CreatedAt, &o.UpdatedAt, &o.RunState, &o.RunNeedsReconciliation); err != nil {
			return out, err
		}
		out.Items = append(out.Items, o)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		lastItem := out.Items[limit-1]
		raw, _ := json.Marshal(automationCursor{Tenant: p.TenantID, Project: project, Collection: collection, Last: lastItem.ID, At: lastItem.CreatedAt})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, rows.Err()
}
func (s *Service) automationRevisionsCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string, limit int, cursor string) (AutomationRevisions, error) {
	if auth == nil {
		return AutomationRevisions{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationRevisions{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationRevisions{Items: []AutomationRevision{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	last, err := automationDecodeCursor(p, project, "revisions:"+id, cursor)
	if err != nil {
		return out, err
	}
	before := int64(math.MaxInt32)
	if last != "" {
		before, err = strconv.ParseInt(last, 16, 32)
		if err != nil || before < 1 {
			return out, invalid("无效修订游标")
		}
	}
	a, err := s.automationRuleTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	out.Total = a.Revision
	rows, err := tx.QueryContext(ctx, s.q(`SELECT revision,spec,created_at FROM suma_automation_revisions WHERE rule_id=? AND revision<? ORDER BY revision DESC LIMIT ?`), id, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var rev AutomationRevision
		var raw string
		if err = rows.Scan(&rev.Revision, &raw, &rev.CreatedAt); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &rev.Spec); err != nil {
			return out, err
		}
		out.Items = append(out.Items, rev)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, project, "revisions:"+id, fmt.Sprintf("%032x", out.Items[limit-1].Revision))
	}
	return out, rows.Err()
}
func (s *Server) automationHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, path string, parts []string, project string, m map[string]any) bool {
	if len(parts) == 3 && parts[0] == "runs" && parts[2] == "automation-origin" {
		w.Header().Set("Cache-Control", "no-store")
		if scope != "" {
			writeError(w, forbidden())
			return true
		}
		if r.Method != http.MethodGet {
			writeError(w, missing())
			return true
		}
		out, err := s.Service.AutomationRunOrigin(r.Context(), actor, project, parts[1])
		respond(w, http.StatusOK, out, err)
		return true
	}
	if parts[0] != "automations" {
		return false
	}
	if scope != "" {
		writeError(w, forbidden())
		return true
	}
	ctx := r.Context()
	switch {
	case path == "automations" && r.Method == "POST":
		var req AutomationCreateRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CreateAutomation(ctx, actor, project, req)
		respond(w, 201, out, err)
	case path == "automations/preview" && r.Method == "POST":
		var req AutomationPreviewRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.PreviewAutomation(ctx, actor, project, req.Spec)
		respond(w, 200, out, err)
	case path == "automations" && r.Method == "GET":
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ListAutomations(ctx, actor, project, limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 2 && r.Method == "GET":
		out, err := s.Service.GetAutomation(ctx, actor, project, parts[1])
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "commands" && r.Method == "POST":
		var req AutomationCommandRequest
		if err := strictAutomation(m, &req); err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.CommandAutomation(ctx, actor, project, parts[1], req)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "occurrences" && r.Method == "GET":
		query, err := url.ParseQuery(r.URL.RawQuery)
		filter := automationOccurrenceAll
		if values, present := query["filter"]; present {
			filter = ""
			if len(values) == 1 {
				filter = values[0]
			}
		}
		if err != nil || !validAutomationOccurrenceFilter(filter) {
			writeError(w, invalid("无效自动化请求"))
			return true
		}
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.AutomationOccurrences(ctx, actor, project, parts[1], filter, limit, cursor)
		respond(w, 200, out, err)
	case len(parts) == 3 && parts[2] == "revisions" && r.Method == "GET":
		limit, cursor, err := automationPage(r)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.AutomationRevisions(ctx, actor, project, parts[1], limit, cursor)
		respond(w, 200, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
