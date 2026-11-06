package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

const failureIssueKeyPrefix = "failure-"

func failureIssueKey(run string) string {
	return failureIssueKeyPrefix + run
}

// Raw project operation keys share a storage key with the failure projector.
// OceanBase VARCHAR collations can equate Unicode aliases with ASCII keys, so
// accept only visible ASCII at these raw-key boundaries on that dialect.
func (s *Service) checkFailureIssueKeyReservation(key string) error {
	if s.Dialect == "oceanbase" {
		for i := 0; i < len(key); i++ {
			b := key[i]
			if b < '!' || b > '~' {
				return invalid("Operation key must use visible ASCII characters on OceanBase")
			}
		}
	}
	if len(key) >= len(failureIssueKeyPrefix) && strings.EqualFold(key[:len(failureIssueKeyPrefix)], failureIssueKeyPrefix) {
		return invalid(reservedRecordOperationKey)
	}
	return nil
}

// Keep the original system command and its receipt identity in one place so
// the projector, task delivery, and diagnosis prove the same lineage.
func failureIssueBody(run, workflow string) map[string]any {
	return map[string]any{
		"title":           "执行失败：" + workflow,
		"description":     "运行 " + run + " 失败，请查看节点证据并分诊。",
		"status":          "open",
		"priority":        "high",
		"idempotency_key": failureIssueKey(run),
		"data":            map[string]any{"run_id": run, "origin": "execution"},
	}
}

func failureIssueHash(body map[string]any) string {
	return digest("system" + stringify(map[string]any{"kind": "issues", "body": body}))
}

func failureIssueReceiptID(raw, stored, expected, tenant, project, run string) (string, error) {
	if stored != expected {
		return "", &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
	}
	var original Record
	// The response contains the immutable creation snapshot, not the issue's
	// editable current metadata. Only the receipt may identify the original.
	if json.Unmarshal([]byte(raw), &original) != nil || !handoffID(original.ID, false) || original.Version != 1 ||
		original.TenantID != tenant || original.ProjectID != project || original.Data["run_id"] != run || original.Data["origin"] != "execution" {
		return "", conflict(diagnosisIssue)
	}
	return original.ID, nil
}

// failureIssueIDTx reads the canonical issue association only after the exact
// run has a durable failure event. A manual issue or a reused operation key
// cannot establish execution provenance.
func (s *Service) failureIssueIDTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (string, bool, error) {
	var workflow, stored, raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT r.workflow_name,o.request_hash,o.response
		FROM dag_runs r JOIN suma_operations o ON o.tenant_id=r.tenant_id AND o.project_id=r.project_id AND o.operation_id=?
		WHERE r.tenant_id=? AND r.project_id=? AND r.id=?
		AND EXISTS(SELECT 1 FROM dag_events e WHERE e.tenant_id=r.tenant_id AND e.project_id=r.project_id AND e.run_id=r.id AND e.event_type='run_failed')`),
		failureIssueKey(run), tenant, project, run).Scan(&workflow, &stored, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	id, err := failureIssueReceiptID(raw, stored, failureIssueHash(failureIssueBody(run, workflow)), tenant, project, run)
	return id, err == nil, err
}
