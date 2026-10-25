package platform

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

func ingressHistoryCursor(p Principal, project, collection, last, at string) string {
	raw, _ := json.Marshal(automationCursor{Tenant: p.TenantID, Project: project, Collection: collection, Last: last, At: at})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func readIngressHistoryCursor(p Principal, project, collection, value string) (automationCursor, error) {
	var out automationCursor
	if value == "" {
		return out, nil
	}
	if len(value) > 1024 {
		return out, invalid("无效分页游标")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || RejectDuplicateReleaseJSON(raw) != nil {
		return out, invalid("无效分页游标")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || out.Tenant != p.TenantID || out.Project != project || out.Collection != collection {
		return out, invalid("分页游标不属于当前入口范围")
	}
	return out, nil
}
func (s *Service) automationIngressRevisionsCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string, limit int, cursor string) (AutomationIngressRevisions, error) {
	if auth == nil {
		return AutomationIngressRevisions{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationIngressRevisions{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationIngressRevisions{Items: []AutomationIngressRevision{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	c, err := readIngressHistoryCursor(p, project, "ingress_revisions:"+id, cursor)
	if err != nil {
		return out, err
	}
	last := int64(1 << 32)
	if cursor != "" {
		last, err = strconv.ParseInt(c.Last, 10, 32)
		if err != nil || last < 1 || c.At != "" {
			return out, invalid("无效修订分页游标")
		}
	}
	if _, err = s.ingressTx(ctx, tx, p, project, id); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_ingress_revisions WHERE ingress_id=?`), id).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT revision,rule_revision,spec,created_at FROM suma_automation_ingress_revisions WHERE ingress_id=? AND revision<? ORDER BY revision DESC LIMIT ?`), id, last, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var rev AutomationIngressRevision
		var raw string
		if err = rows.Scan(&rev.Revision, &rev.RuleRevision, &raw, &rev.CreatedAt); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &rev.Spec); err != nil {
			return out, err
		}
		out.Items = append(out.Items, rev)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = ingressHistoryCursor(p, project, "ingress_revisions:"+id, strconv.Itoa(out.Items[limit-1].Revision), "")
	}
	return out, rows.Err()
}

const ingressDeliveryColumns = `d.id,d.ingress_id,d.ingress_revision,d.rule_id,d.rule_revision,d.delivery_id,d.provider,d.body_sha256,d.key_generation,d.received_at,d.last_received_at,d.duplicate_count,d.state,d.reason,d.occurrence_id,d.event,COALESCE(o.state,''),COALESCE(o.run_id,''),COALESCE(r.state,'')`
const ingressDeliveryJoins = ` FROM suma_automation_ingress_deliveries d LEFT JOIN suma_automation_occurrences o ON o.id=d.occurrence_id AND o.tenant_id=d.tenant_id AND o.project_id=d.project_id LEFT JOIN dag_runs r ON r.id=o.run_id AND r.tenant_id=d.tenant_id AND r.project_id=d.project_id `

func scanIngressDelivery(row automationScanner) (AutomationIngressDelivery, error) {
	var out AutomationIngressDelivery
	var raw string
	err := row.Scan(&out.ID, &out.IngressID, &out.IngressRevision, &out.RuleID, &out.RuleRevision, &out.DeliveryID, &out.Provider, &out.BodySHA256, &out.KeyGeneration, &out.ReceivedAt, &out.LastReceivedAt, &out.DuplicateCount, &out.State, &out.Reason, &out.OccurrenceID, &raw, &out.OccurrenceState, &out.RunID, &out.RunState)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out.Event)
	return out, err
}
func (s *Service) automationIngressDeliveriesCurrentTx(ctx context.Context, auth *authorizedCommand, project, id string, limit int, cursor string) (AutomationIngressDeliveries, error) {
	if auth == nil {
		return AutomationIngressDeliveries{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationIngressDeliveries{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	out := AutomationIngressDeliveries{Items: []AutomationIngressDelivery{}}
	if limit < 1 || limit > 200 {
		return out, invalid("limit 必须为 1..200")
	}
	c, err := readIngressHistoryCursor(p, project, "ingress_deliveries:"+id, cursor)
	if err != nil {
		return out, err
	}
	if cursor != "" {
		at, e := time.Parse(time.RFC3339, c.At)
		if e != nil || automationStamp(at) != c.At || !ingressLowerHex(c.Last, 32) {
			return out, invalid("无效交付分页游标")
		}
	}
	if _, err = s.ingressTx(ctx, tx, p, project, id); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_ingress_deliveries WHERE tenant_id=? AND project_id=? AND ingress_id=?`), p.TenantID, project, id).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT ` + ingressDeliveryColumns + ingressDeliveryJoins + `WHERE d.tenant_id=? AND d.project_id=? AND d.ingress_id=?`
	args := []any{p.TenantID, project, id}
	if cursor != "" {
		query += ` AND (d.received_at<? OR (d.received_at=? AND d.id<?))`
		args = append(args, c.At, c.At, c.Last)
	}
	query += ` ORDER BY d.received_at DESC,d.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		delivery, e := scanIngressDelivery(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, delivery)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		out.NextCursor = ingressHistoryCursor(p, project, "ingress_deliveries:"+id, last.ID, last.ReceivedAt)
	}
	return out, rows.Err()
}
func (s *Service) getAutomationIngressDeliveryCurrentTx(ctx context.Context, auth *authorizedCommand, project, id, delivery string) (AutomationIngressDelivery, error) {
	if auth == nil {
		return AutomationIngressDelivery{}, forbidden()
	}
	expected := commandTarget{tenantID: auth.target.tenantID, projectID: project, resourceID: id}
	if e := auth.assertTx(auth.tx, expected, purposeBrowserProjectRead); e != nil {
		return AutomationIngressDelivery{}, e
	}
	ctx = auth.ctx
	tx, p := auth.tx, auth.currentPrincipal
	var err error

	if _, err = s.ingressTx(ctx, tx, p, project, id); err != nil {
		return AutomationIngressDelivery{}, err
	}
	return scanIngressDelivery(tx.QueryRowContext(ctx, s.q(`SELECT `+ingressDeliveryColumns+ingressDeliveryJoins+`WHERE d.tenant_id=? AND d.project_id=? AND d.ingress_id=? AND d.id=?`), p.TenantID, project, id, delivery))
}
