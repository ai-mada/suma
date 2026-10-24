package platform

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func ingressProtocolHeader(headers http.Header, name string) (string, error) {
	values := headers.Values(name)
	if len(values) != 1 || values[0] == "" {
		return "", ingressSignatureError()
	}
	return values[0], nil
}
func verifyIngressMAC(secret []byte, message []byte, value string) bool {
	if !strings.HasPrefix(value, "sha256=") || len(value) != 71 {
		return false
	}
	want, err := hex.DecodeString(value[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(message)
	return hmac.Equal(mac.Sum(nil), want)
}
func (s *Service) verifyIngressRequest(ctx context.Context, in automationIngressRecord, headers http.Header, raw []byte) (string, string, error) {
	secret, err := s.decryptIngressSecret(in)
	if err != nil {
		return "", "", ingressSignatureError()
	}
	if in.Provider == "github" {
		id, err := ingressProtocolHeader(headers, "X-GitHub-Delivery")
		if err != nil || !ingressIdentifier(id, 8, 128) {
			return "", "", ingressSignatureError()
		}
		event, err := ingressProtocolHeader(headers, "X-GitHub-Event")
		if err != nil || !ingressIdentifier(event, 1, 40) {
			return "", "", ingressSignatureError()
		}
		sig, err := ingressProtocolHeader(headers, "X-Hub-Signature-256")
		if err != nil || !verifyIngressMAC(secret, raw, sig) {
			return "", "", ingressSignatureError()
		}
		return id, event, nil
	}
	id, err := ingressProtocolHeader(headers, "X-Suma-Delivery")
	if err != nil || !ingressIdentifier(id, 8, 128) {
		return "", "", ingressSignatureError()
	}
	stamp, err := ingressProtocolHeader(headers, "X-Suma-Timestamp")
	if err != nil {
		return "", "", ingressSignatureError()
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || seconds < 0 || strconv.FormatInt(seconds, 10) != stamp {
		return "", "", ingressSignatureError()
	}
	clock, err := s.automationClock(ctx, s.DB)
	if err != nil {
		return "", "", err
	}
	if seconds < clock.Unix()-300 || seconds > clock.Unix()+300 {
		return "", "", ingressSignatureError()
	}
	sig, err := ingressProtocolHeader(headers, "X-Suma-Signature-256")
	message := append([]byte("suma-v1\n"+in.ID+"\n"+stamp+"\n"+id+"\n"), raw...)
	if err != nil || !verifyIngressMAC(secret, message, sig) {
		return "", "", ingressSignatureError()
	}
	return id, "", nil
}
func validateIngressEvent(event AutomationIngressEvent, provider string) error {
	if event.SchemaVersion != 1 || !ingressEventAllowed(provider, event.Event) || !ingressText(event.Repository, 201) || event.RepositoryID != "" && !ingressDigits(event.RepositoryID, true) || !ingressRef(event.Ref) || event.Commit != "" && !ingressLowerHex(event.Commit, 40) && !ingressLowerHex(event.Commit, 64) || event.Conclusion != "" && !ingressConclusion(event.Conclusion) || event.ExternalID != "" && !ingressIdentifier(event.ExternalID, 1, 128) || provider == "suma" && event.ExternalID == "" {
		return invalid("事件受限上下文字段无效")
	}
	return nil
}
func ingressJSONNumber(raw json.RawMessage) (string, bool) {
	value := string(raw)
	return value, ingressDigits(value, true)
}
func normalizeIngressEvent(provider, eventName string, raw []byte) (AutomationIngressEvent, string, error) {
	out := AutomationIngressEvent{SchemaVersion: 1}
	if !utf8.Valid(raw) || validateIngressJSON(raw) != nil {
		return out, "", invalid("事件 JSON 无效、重复或层级超限")
	}
	if provider == "suma" {
		out = AutomationIngressEvent{}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(&out); err != nil {
			return out, "", invalid("Suma 事件字段或类型无效")
		}
		return out, "", validateIngressEvent(out, "suma")
	}
	var payload struct {
		Zen    string          `json:"zen"`
		HookID json.RawMessage `json:"hook_id"`
		Hook   *struct {
			ID json.RawMessage `json:"id"`
		} `json:"hook"`
		Repository *struct {
			ID       json.RawMessage `json:"id"`
			FullName string          `json:"full_name"`
		} `json:"repository"`
		Ref         string `json:"ref"`
		After       string `json:"after"`
		Deleted     *bool  `json:"deleted"`
		Action      string `json:"action"`
		WorkflowRun *struct {
			ID         json.RawMessage `json:"id"`
			HeadBranch string          `json:"head_branch"`
			HeadSHA    string          `json:"head_sha"`
			Conclusion *string         `json:"conclusion"`
		} `json:"workflow_run"`
	}
	trimmed := bytes.TrimSpace(raw)
	if err := json.Unmarshal(raw, &payload); err != nil || len(trimmed) == 0 || trimmed[0] != '{' {
		return out, "", invalid("GitHub 事件必须是 JSON 对象")
	}
	if payload.Repository != nil {
		var ok bool
		out.RepositoryID, ok = ingressJSONNumber(payload.Repository.ID)
		if !ok {
			return out, "", invalid("GitHub repository.id 无效")
		}
		out.Repository = payload.Repository.FullName
	}
	switch eventName {
	case "ping":
		hookID, valid := ingressJSONNumber(payload.HookID)
		if !valid || payload.Hook == nil || string(payload.Hook.ID) != hookID || payload.Zen == "" || !ingressText(payload.Zen, 500) {
			return out, "", invalid("GitHub ping 正文结构无效")
		}
		out.Event = "ping"
		return out, "connection_check", validateIngressEvent(out, "github")
	case "push":
		if payload.Repository == nil || out.Repository == "" || payload.Ref == "" || payload.After == "" || payload.Deleted == nil {
			return out, "", invalid("GitHub push 缺少必要字段")
		}
		out.Event = "push"
		out.Ref = payload.Ref
		out.Commit = payload.After
		if err := validateIngressEvent(out, "github"); err != nil {
			return out, "", err
		}
		if *payload.Deleted {
			return out, "branch_deleted", nil
		}
		return out, "", nil
	case "workflow_run":
		if payload.Repository == nil || out.Repository == "" || payload.WorkflowRun == nil || payload.Action == "" {
			return out, "", invalid("GitHub workflow_run 缺少必要字段")
		}
		run := payload.WorkflowRun
		external, ok := ingressJSONNumber(run.ID)
		if !ok || run.HeadBranch == "" || run.HeadSHA == "" {
			return out, "", invalid("GitHub workflow_run 上下文无效")
		}
		out.Event = "workflow_run.completed"
		out.Ref = "refs/heads/" + run.HeadBranch
		out.Commit = run.HeadSHA
		out.ExternalID = external
		if run.Conclusion != nil {
			out.Conclusion = *run.Conclusion
		}
		if err := validateIngressEvent(out, "github"); err != nil {
			return out, "", err
		}
		if payload.Action != "completed" {
			return out, "action_not_completed", nil
		}
		if out.Conclusion == "" {
			return out, "", invalid("completed workflow_run 缺少 conclusion")
		}
		return out, "", nil
	default:
		return out, "", invalid("不支持的 GitHub 事件")
	}
}
func ingressMatches(in AutomationIngress, event AutomationIngressEvent) bool {
	contains := func(values []string, value string) bool {
		for _, v := range values {
			if v == value {
				return true
			}
		}
		return false
	}
	if !contains(in.Events, event.Event) {
		return false
	}
	if in.Repository != "" {
		if in.Provider == "github" {
			if !strings.EqualFold(in.Repository, event.Repository) {
				return false
			}
		} else if in.Repository != event.Repository {
			return false
		}
	}
	return (in.RepositoryID == "" || in.RepositoryID == event.RepositoryID) && (in.Ref == "" || in.Ref == event.Ref) && (len(in.Conclusions) == 0 || contains(in.Conclusions, event.Conclusion))
}
func (s *Service) ingressAdmission(ctx context.Context, tx *sql.Tx, in automationIngressRecord, a AutomationRule) (string, error) {
	if in.State != "enabled" || !in.KeyConfigured {
		return "ingress_inactive", nil
	}
	if a.State != "enabled" || in.RuleRevision != a.Revision {
		return "rule_inactive_or_revised", nil
	}
	for _, actor := range []string{in.ActivatedBy, a.ActivatedBy} {
		if err := s.authorizeProjectTx(ctx, tx, Principal{ID: actor, TenantID: in.tenant}, in.ProjectID, true, true); err != nil {
			if automationAuthorityFailure(err) {
				return "activation_authority_revoked", nil
			}
			return "", err
		}
	}
	if err := s.automationWorkflow(ctx, tx, Principal{TenantID: in.tenant}, in.ProjectID, a.AutomationSpec); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return "workflow_unavailable", nil
		}
		return "", err
	}
	return "", nil
}
func ingressReceipt(delivery AutomationIngressDelivery, duplicate bool) AutomationIngressReceipt {
	return AutomationIngressReceipt{ID: delivery.ID, State: delivery.State, Duplicate: duplicate, OccurrenceID: delivery.OccurrenceID}
}
func (s *Service) ReceiveAutomationIngress(ctx context.Context, id string, headers http.Header, raw []byte) (AutomationIngressReceipt, error) {
	var out AutomationIngressReceipt
	snapshot, err := scanIngress(s.DB.QueryRowContext(ctx, s.q(`SELECT `+ingressColumns+` FROM suma_automation_ingresses WHERE id=?`), id))
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Status == 404 {
			return out, ingressSignatureError()
		}
		return out, err
	}
	deliveryID, eventName, err := s.verifyIngressRequest(ctx, snapshot, headers, raw)
	if err != nil {
		return out, err
	}
	event, ignore, err := normalizeIngressEvent(snapshot.Provider, eventName, raw)
	if err != nil {
		return out, err
	}
	bodyHash := digest(string(raw))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, snapshot.tenant); err != nil {
		return out, err
	}
	p := Principal{TenantID: snapshot.tenant}
	in, err := s.ingressTx(ctx, tx, p, snapshot.ProjectID, id)
	if err != nil {
		return out, err
	}
	if in.KeyGeneration != snapshot.KeyGeneration || in.ciphertext != snapshot.ciphertext || in.Revision != snapshot.Revision || in.Provider != snapshot.Provider {
		return out, conflict("入口凭据或配置已变化，请重新核对")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	if in.Provider == "suma" {
		stamp, _ := strconv.ParseInt(headers.Get("X-Suma-Timestamp"), 10, 64)
		if stamp < clock.Unix()-300 || stamp > clock.Unix()+300 {
			return out, ingressSignatureError()
		}
	}
	var existingID, existingHash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT receipt_id,body_sha256 FROM suma_automation_ingress_delivery_ids WHERE ingress_id=? AND delivery_id=?`), id, deliveryID).Scan(&existingID, &existingHash)
	aliasKnown := err == nil
	if err == nil && existingHash != bodyHash {
		return out, &APIError{409, "delivery_conflict", "同一交付标识已用于不同正文", nil}
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if !aliasKnown {
		var count int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_ingress_delivery_ids WHERE ingress_id=? AND received_at>=? AND received_at<?`), id, automationStamp(clock.Truncate(time.Minute)), automationStamp(clock.Truncate(time.Minute).Add(time.Minute))).Scan(&count); err != nil {
			return out, err
		}
		if count >= 120 {
			return out, &APIError{429, "ingress_rate_limit", "入口每分钟最多接收 120 个新交付身份", nil}
		}
	}
	if existingID == "" && in.Provider == "github" {
		err = tx.QueryRowContext(ctx, s.q(`SELECT delivery_id FROM suma_automation_ingress_github_bodies WHERE ingress_id=? AND body_sha256=?`), id, bodyHash).Scan(&existingID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
	}
	if existingID != "" {
		existing, err := scanIngressDelivery(tx.QueryRowContext(ctx, s.q(`SELECT `+ingressDeliveryColumns+ingressDeliveryJoins+`WHERE d.id=? AND d.ingress_id=?`), existingID, id))
		if err != nil {
			return out, err
		}
		if !aliasKnown {
			if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_delivery_ids(ingress_id,delivery_id,body_sha256,receipt_id,received_at)VALUES(?,?,?,?,?)`), id, deliveryID, bodyHash, existingID, at); err != nil {
				return out, err
			}
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automation_ingress_deliveries SET duplicate_count=duplicate_count+1,last_received_at=? WHERE id=? AND ingress_id=?`), at, existingID, id); err != nil {
			return out, err
		}
		out = ingressReceipt(existing, true)
		return out, tx.Commit()
	}
	a, err := s.automationRuleTx(ctx, tx, p, in.ProjectID, in.RuleID)
	if err != nil {
		return out, err
	}
	state := "accepted"
	reason, err := s.ingressAdmission(ctx, tx, in, a)
	if err != nil {
		return out, err
	}
	if reason != "" {
		state = "blocked"
	} else if ignore != "" {
		state = "ignored"
		reason = ignore
	} else if !ingressMatches(in.AutomationIngress, event) {
		state = "ignored"
		reason = "filter_mismatch"
	}
	delivery := AutomationIngressDelivery{ID: newID(), IngressID: id, IngressRevision: in.Revision, RuleID: in.RuleID, RuleRevision: in.RuleRevision, DeliveryID: deliveryID, Provider: in.Provider, BodySHA256: bodyHash, KeyGeneration: in.KeyGeneration, ReceivedAt: at, LastReceivedAt: at, State: state, Reason: reason, Event: event}
	if state == "accepted" {
		o, e := s.insertAutomationOccurrence(ctx, tx, p, a, "webhook", at, "queued", "", "ingress:"+id+":"+deliveryID)
		if e != nil {
			return out, e
		}
		delivery.OccurrenceID = o.ID
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_deliveries(id,tenant_id,project_id,ingress_id,ingress_revision,rule_id,rule_revision,delivery_id,provider,body_sha256,key_generation,received_at,last_received_at,duplicate_count,state,reason,occurrence_id,event)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), delivery.ID, in.tenant, in.ProjectID, id, in.Revision, in.RuleID, in.RuleRevision, deliveryID, in.Provider, bodyHash, in.KeyGeneration, at, at, 0, state, reason, delivery.OccurrenceID, stringify(event))
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_delivery_ids(ingress_id,delivery_id,body_sha256,receipt_id,received_at)VALUES(?,?,?,?,?)`), id, deliveryID, bodyHash, delivery.ID, at); err != nil {
		return out, err
	}
	if in.Provider == "github" {
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_automation_ingress_github_bodies(ingress_id,body_sha256,delivery_id)VALUES(?,?,?)`), id, bodyHash, delivery.ID); err != nil {
			return out, err
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automation_ingresses SET last_error=? WHERE id=?`), reason, id); err != nil {
		return out, err
	}
	out = ingressReceipt(delivery, false)
	return out, tx.Commit()
}

func (s *Service) automationIngressDispatchAdmission(ctx context.Context, tx *sql.Tx, ref automationRef, a AutomationRule, o AutomationOccurrence) error {
	if o.Source != "webhook" {
		return nil
	}
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT ingress_id FROM suma_automation_ingress_deliveries WHERE tenant_id=? AND project_id=? AND occurrence_id=?`), ref.tenant, ref.project, o.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return &automationDeferred{"blocked", "ingress_delivery_unavailable"}
	}
	if err != nil {
		return err
	}
	in, err := s.ingressTx(ctx, tx, Principal{TenantID: ref.tenant}, ref.project, id)
	if err != nil {
		return err
	}
	var ingressRevision, keyGeneration int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT ingress_revision,key_generation FROM suma_automation_ingress_deliveries WHERE ingress_id=? AND occurrence_id=?`), id, o.ID).Scan(&ingressRevision, &keyGeneration); err != nil {
		return err
	}
	if ingressRevision != in.Revision || keyGeneration != in.KeyGeneration {
		return &automationDeferred{"blocked", "ingress_revised_or_rotated"}
	}
	reason, err := s.ingressAdmission(ctx, tx, in, a)
	if err != nil {
		return err
	}
	if reason != "" {
		return &automationDeferred{"blocked", reason}
	}
	return nil
}
