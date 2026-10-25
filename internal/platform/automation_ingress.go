package platform

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/store"
)

func (s *Service) MigrateAutomationIngresses(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_automation_ingresses(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,rule_id VARCHAR(64) NOT NULL,rule_revision INTEGER NOT NULL,version INTEGER NOT NULL,revision INTEGER NOT NULL,state VARCHAR(20) NOT NULL,activated_by VARCHAR(64) NOT NULL,key_generation INTEGER NOT NULL,key_fingerprint VARCHAR(64) NOT NULL,key_ciphertext TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,last_error TEXT NOT NULL,spec TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_automation_ingress_revisions(ingress_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,rule_revision INTEGER NOT NULL,spec TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(ingress_id,revision))`,
		`CREATE TABLE IF NOT EXISTS suma_automation_ingress_deliveries(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,ingress_id VARCHAR(64) NOT NULL,ingress_revision INTEGER NOT NULL,rule_id VARCHAR(64) NOT NULL,rule_revision INTEGER NOT NULL,delivery_id VARCHAR(128) NOT NULL,provider VARCHAR(20) NOT NULL,body_sha256 VARCHAR(64) NOT NULL,key_generation INTEGER NOT NULL,received_at VARCHAR(40) NOT NULL,last_received_at VARCHAR(40) NOT NULL,duplicate_count INTEGER NOT NULL,state VARCHAR(20) NOT NULL,reason TEXT NOT NULL,occurrence_id VARCHAR(64) NOT NULL,event TEXT NOT NULL,UNIQUE(ingress_id,delivery_id))`,
		`CREATE TABLE IF NOT EXISTS suma_automation_ingress_github_bodies(ingress_id VARCHAR(64) NOT NULL,body_sha256 VARCHAR(64) NOT NULL,delivery_id VARCHAR(64) NOT NULL,PRIMARY KEY(ingress_id,body_sha256))`,
		`CREATE TABLE IF NOT EXISTS suma_automation_ingress_delivery_ids(ingress_id VARCHAR(64) NOT NULL,delivery_id VARCHAR(128) NOT NULL,body_sha256 VARCHAR(64) NOT NULL,receipt_id VARCHAR(64) NOT NULL,received_at VARCHAR(40) NOT NULL,PRIMARY KEY(ingress_id,delivery_id))`,
		`CREATE TABLE IF NOT EXISTS suma_automation_ingress_key_claims(fingerprint VARCHAR(64) PRIMARY KEY,ingress_id VARCHAR(64) NOT NULL,key_generation INTEGER NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return fmt.Errorf("automation ingress migration: %w", err)
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_ingress_rule_scope", "suma_automation_ingresses", []string{"tenant_id", "project_id", "rule_id", "id"}},
		{"suma_ingress_delivery_history", "suma_automation_ingress_deliveries", []string{"ingress_id", "received_at", "id"}},
		{"suma_ingress_delivery_occurrence", "suma_automation_ingress_deliveries", []string{"tenant_id", "project_id", "occurrence_id"}},
		{"suma_ingress_identity_rate", "suma_automation_ingress_delivery_ids", []string{"ingress_id", "received_at"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); err != nil {
			return err
		}
	}
	return nil
}

func ingressText(value string, max int) bool {
	if len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func ingressIdentifier(value string, min, max int) bool {
	if len(value) < min || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-", r)) {
			return false
		}
	}
	return true
}
func ingressDigits(value string, positive bool) bool {
	if len(value) == 0 || len(value) > 30 || positive && value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func ingressLowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func ingressRef(value string) bool {
	if !ingressText(value, 250) {
		return false
	}
	if value == "" {
		return true
	}
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if strings.HasPrefix(value, prefix) && len(value) > len(prefix) {
			return true
		}
	}
	return false
}
func ingressConclusion(value string) bool {
	if len(value) < 1 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
func ingressEventAllowed(provider, event string) bool {
	if provider == "github" {
		return event == "push" || event == "workflow_run.completed" || event == "ping"
	}
	return provider == "suma" && (event == "git.push" || event == "ci.completed" || event == "monitor.firing" || event == "monitor.resolved")
}
func validateIngressSpec(spec *AutomationIngressSpec) error {
	if spec.Name == "" || spec.Name != strings.TrimSpace(spec.Name) || !ingressText(spec.Name, 200) {
		return invalid("入口名称无效")
	}
	if spec.Provider != "github" && spec.Provider != "suma" {
		return invalid("provider 必须为 github 或 suma")
	}
	if len(spec.Events) < 1 || len(spec.Events) > 8 {
		return invalid("events 必须为 1..8 个白名单事件")
	}
	seen := map[string]bool{}
	for _, event := range spec.Events {
		if !ingressEventAllowed(spec.Provider, event) || seen[event] {
			return invalid("events 含无效或重复事件")
		}
		seen[event] = true
	}
	if !ingressText(spec.Repository, 201) || spec.RepositoryID != "" && !ingressDigits(spec.RepositoryID, true) {
		return invalid("仓库名称或数字 ID 无效")
	}
	if spec.Provider == "github" {
		parts := strings.Split(spec.Repository, "/")
		if len(parts) != 2 || !ingressIdentifier(parts[0], 1, 100) || !ingressIdentifier(parts[1], 1, 100) || spec.RepositoryID == "" {
			return invalid("GitHub 必须固定 owner/repo 和正十进制 repository_id")
		}
	}
	if !ingressRef(spec.Ref) {
		return invalid("ref 必须为完整 refs/heads 分支或 refs/tags 标签")
	}
	if len(spec.Conclusions) > 16 {
		return invalid("conclusions 最多 16 项")
	}
	seen = map[string]bool{}
	for _, value := range spec.Conclusions {
		if !ingressConclusion(value) || seen[value] {
			return invalid("conclusions 含无效或重复状态")
		}
		seen[value] = true
	}
	if spec.Conclusions == nil {
		spec.Conclusions = []string{}
	}
	return nil
}
func ingressVersion(value int) bool { return value > 0 && value <= math.MaxInt32 }

type automationIngressRecord struct {
	AutomationIngress
	tenant, ciphertext string
}

const ingressColumns = `id,project_id,rule_id,rule_revision,version,revision,state,activated_by,key_generation,key_fingerprint,created_at,updated_at,last_error,spec,tenant_id,key_ciphertext`

func scanIngress(row automationScanner) (automationIngressRecord, error) {
	var out automationIngressRecord
	var raw string
	err := row.Scan(&out.ID, &out.ProjectID, &out.RuleID, &out.RuleRevision, &out.Version, &out.Revision, &out.State, &out.ActivatedBy, &out.KeyGeneration, &out.KeyFingerprint, &out.CreatedAt, &out.UpdatedAt, &out.LastError, &raw, &out.tenant, &out.ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out.AutomationIngressSpec); err != nil {
		return out, err
	}
	out.KeyConfigured = out.ciphertext != ""
	out.EndpointPath = "/api/v1/hooks/" + out.ID
	return out, nil
}
func (s *Service) ingressTx(ctx context.Context, tx governanceQuery, p Principal, project, id string) (automationIngressRecord, error) {
	return scanIngress(tx.QueryRowContext(ctx, s.q(`SELECT `+ingressColumns+` FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
}
func (s *Service) saveIngress(ctx context.Context, auth *authorizedCommand, in automationIngressRecord) error {
	if auth == nil {
		return forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, purposeBrowserProjectApprove); err != nil {
		return err
	}
	if auth.actor.kind != actorBrowser || auth.target.tenantID != in.tenant || auth.target.projectID != in.ProjectID || auth.target.resourceID != in.ID {
		return forbidden()
	}
	tx := auth.tx
	_, err := tx.ExecContext(ctx, s.q(`UPDATE suma_automation_ingresses SET rule_revision=?,version=?,revision=?,state=?,activated_by=?,key_generation=?,key_fingerprint=?,key_ciphertext=?,updated_at=?,last_error=?,spec=? WHERE tenant_id=? AND project_id=? AND id=?`), in.RuleRevision, in.Version, in.Revision, in.State, in.ActivatedBy, in.KeyGeneration, in.KeyFingerprint, in.ciphertext, in.UpdatedAt, in.LastError, stringify(in.AutomationIngressSpec), in.tenant, in.ProjectID, in.ID)
	return err
}
func (s *Service) CreateAutomationIngress(ctx context.Context, actor Actor, project, rule string, req AutomationIngressCreateRequest) (AutomationIngress, error) {
	var out AutomationIngress
	if !ingressVersion(req.ExpectedRuleVersion) || !ingressVersion(req.ExpectedRuleRevision) {
		return out, invalid("必须确认当前规则版本和修订")
	}
	if err := validateIngressSpec(&req.Spec); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: rule}, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"ingress_create": rule, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	a, err := s.automationRuleTx(ctx, tx, p, project, rule)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = missing()
		}
		return out, err
	}
	if a.Version != req.ExpectedRuleVersion || a.Revision != req.ExpectedRuleRevision || a.State == "archived" {
		return out, conflict("自动化规则版本已变化或已归档")
	}
	var total int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=?`), p.TenantID, project).Scan(&total); err != nil {
		return out, err
	}
	if total >= 50 {
		return out, conflict("项目最多允许 50 个事件入口")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	out = AutomationIngress{AutomationIngressSpec: req.Spec, ID: newID(), ProjectID: project, RuleID: rule, RuleRevision: a.Revision, Version: 1, Revision: 1, State: "paused", CreatedAt: at, UpdatedAt: at}
	out.EndpointPath = "/api/v1/hooks/" + out.ID
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingresses(`+ingressColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), out.ID, project, rule, a.Revision, 1, 1, "paused", "", 0, "", at, at, "", stringify(req.Spec), p.TenantID, "")
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_revisions(ingress_id,revision,rule_revision,spec,created_at)VALUES(?,?,?,?,?)`), out.ID, 1, a.Revision, stringify(req.Spec), at); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "automation_ingress.created", out.ID, out.Name); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ingressAEAD() (cipher.AEAD, error) {
	if len(s.AutomationIngressKey) != 32 {
		return nil, fmt.Errorf("automation ingress encryption is unavailable")
	}
	block, err := aes.NewCipher(s.AutomationIngressKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func ingressAAD(in automationIngressRecord) []byte {
	return []byte(in.tenant + "\n" + in.ProjectID + "\n" + in.ID + "\n" + strconv.Itoa(in.KeyGeneration))
}
func (s *Service) encryptIngressSecret(in automationIngressRecord, secret string) (string, error) {
	aead, err := s.ingressAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(secret), ingressAAD(in))), nil
}
func (s *Service) decryptIngressSecret(in automationIngressRecord) ([]byte, error) {
	aead, err := s.ingressAEAD()
	if err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(in.ciphertext)
	if err != nil || len(raw) < aead.NonceSize() {
		return nil, fmt.Errorf("automation ingress credential unavailable")
	}
	secret, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], ingressAAD(in))
	if err != nil {
		return nil, fmt.Errorf("automation ingress credential unavailable")
	}
	return secret, nil
}
func (s *Service) blockIngressQueued(ctx context.Context, tx *sql.Tx, in automationIngressRecord, reason, at string) error {
	_, err := tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state='blocked',reason=?,updated_at=? WHERE tenant_id=? AND project_id=? AND state='queued' AND id IN(SELECT occurrence_id FROM suma_automation_ingress_deliveries WHERE ingress_id=?)`), reason, at, in.tenant, in.ProjectID, in.ID)
	return err
}
func (s *Service) CommandAutomationIngress(ctx context.Context, actor Actor, project, id string, req AutomationIngressCommandRequest) (AutomationIngress, error) {
	var out AutomationIngress
	if !ingressVersion(req.ExpectedVersion) {
		return out, invalid("expected_version 必须为正整数")
	}
	if req.Action != "revise" && req.Action != "enable" && req.Action != "pause" && req.Action != "revoke" && req.Action != "rotate_key" {
		return out, invalid("入口命令无效")
	}
	bind := req.Action == "revise" || req.Action == "enable"
	if bind && (!ingressVersion(req.ExpectedRuleVersion) || !ingressVersion(req.ExpectedRuleRevision)) || !bind && (req.ExpectedRuleVersion != 0 || req.ExpectedRuleRevision != 0) {
		return out, invalid("规则版本确认字段与命令不匹配")
	}
	if (req.Action == "revise") != (req.Spec != nil) {
		return out, invalid("仅 revise 必须携带 spec")
	}
	if req.Spec != nil {
		if err := validateIngressSpec(req.Spec); err != nil {
			return out, err
		}
	}
	if req.Action == "rotate_key" {
		if !ingressLowerHex(req.Secret, 64) {
			return out, invalid("secret 必须为 64 位小写十六进制原文")
		}
	} else if req.Secret != "" {
		return out, invalid("仅 rotate_key 接受 secret")
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
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"ingress_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	in, err := s.ingressTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if in.Version != req.ExpectedVersion || in.State == "revoked" {
		return out, conflict("入口版本已变化或已撤销")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	if bind {
		a, err := s.automationRuleTx(ctx, tx, p, project, in.RuleID)
		if err != nil {
			return out, err
		}
		if a.Version != req.ExpectedRuleVersion || a.Revision != req.ExpectedRuleRevision || a.State == "archived" {
			return out, conflict("自动化规则版本已变化")
		}
		if req.Action == "enable" {
			if in.RuleRevision != a.Revision || a.State != "enabled" || !in.KeyConfigured {
				return out, conflict("启用需要密钥及当前已启用规则修订")
			}
			if _, err = s.currentProjectMembershipLockedTx(ctx, tx, p.TenantID, a.ActivatedBy, project, true, true); err != nil {
				return out, err
			}
			if err = s.automationWorkflow(ctx, tx, p, project, a.AutomationSpec); err != nil {
				return out, err
			}
			if _, err = s.decryptIngressSecret(in); err != nil {
				return out, err
			}
			in.State = "enabled"
			in.ActivatedBy = p.ID
		} else {
			in.AutomationIngressSpec = *req.Spec
			in.RuleRevision = a.Revision
			in.Revision++
			in.State = "paused"
			if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_revisions(ingress_id,revision,rule_revision,spec,created_at)VALUES(?,?,?,?,?)`), in.ID, in.Revision, in.RuleRevision, stringify(in.AutomationIngressSpec), at); err != nil {
				return out, err
			}
		}
	} else if req.Action == "rotate_key" {
		fingerprint := digest(req.Secret)
		var occupied string
		err = tx.QueryRowContext(ctx, s.q(`SELECT fingerprint FROM suma_automation_ingress_key_claims WHERE fingerprint=?`), fingerprint).Scan(&occupied)
		if err == nil {
			return out, conflict("该来源密钥不能重新使用")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		in.KeyGeneration++
		in.KeyFingerprint = fingerprint
		in.ciphertext, err = s.encryptIngressSecret(in, req.Secret)
		if err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_key_claims(fingerprint,ingress_id,key_generation)VALUES(?,?,?)`), fingerprint, in.ID, in.KeyGeneration); err != nil {
			return out, conflict("该来源密钥不能重新使用")
		}
		in.KeyConfigured = true
		in.State = "paused"
	} else if req.Action == "revoke" {
		in.State = "revoked"
		in.ciphertext = ""
		in.KeyConfigured = false
	} else {
		in.State = "paused"
	}
	if req.Action != "enable" {
		if err = s.blockIngressQueued(ctx, tx, in, "ingress_"+req.Action, at); err != nil {
			return out, err
		}
	}
	in.Version++
	in.UpdatedAt = at
	in.LastError = ""
	out = in.AutomationIngress
	if err = s.saveIngress(ctx, auth, in); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "automation_ingress."+req.Action, in.ID, in.Name); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) getAutomationIngressCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string) (AutomationIngress, error) {
	if auth == nil {
		return AutomationIngress{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationIngress{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	in, err := s.ingressTx(ctx, tx, p, project, id)
	return in.AutomationIngress, err
}
func (s *Service) listAutomationIngressesCurrentTx(ctx context.Context, auth *authorizedCommand, project, rule string, limit int, cursor string) (AutomationIngressList, error) {
	if auth == nil {
		return AutomationIngressList{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: rule}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationIngressList{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationIngressList{Items: []AutomationIngress{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	last, err := automationDecodeCursor(p, project, "ingresses:"+rule, cursor)
	if err != nil {
		return out, err
	}
	if _, err = s.automationRuleTx(ctx, tx, p, project, rule); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = missing()
		}
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=? AND rule_id=?`), p.TenantID, project, rule).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+ingressColumns+` FROM suma_automation_ingresses WHERE tenant_id=? AND project_id=? AND rule_id=? AND id>? ORDER BY id LIMIT ?`), p.TenantID, project, rule, last, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		in, err := scanIngress(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, in.AutomationIngress)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, project, "ingresses:"+rule, out.Items[limit-1].ID)
	}
	return out, rows.Err()
}
