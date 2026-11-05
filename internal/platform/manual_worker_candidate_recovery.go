package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

const releaseWorkerCandidateRecoveryPageSize = 20

type ReleaseWorkerCandidateRecoveryView struct {
	ProjectID        string                               `json:"project_id"`
	WorkerRegistryID string                               `json:"worker_registry_id"`
	WorkerID         string                               `json:"worker_id"`
	WorkerVersion    int                                  `json:"worker_version"`
	Items            []ReleaseWorkerCandidateRecoveryItem `json:"items"`
	NextCursor       string                               `json:"next_cursor"`
}

type ReleaseWorkerCandidateRecoveryItem struct {
	TransferID   string `json:"transfer_id"`
	RunID        string `json:"run_id"`
	NodeID       string `json:"node_id"`
	Attempt      int    `json:"attempt"`
	Fence        string `json:"fence"`
	BundleSHA256 string `json:"bundle_sha256"`
	BaseCommit   string `json:"base_commit"`
	HeadCommit   string `json:"head_commit"`
	CreatedAt    string `json:"created_at"`
	SizeBytes    int64  `json:"size_bytes"`
	AttemptState string `json:"attempt_state"`
	Disposition  string `json:"disposition"`
}

type ReleaseWorkerCandidateAbandonRequest struct {
	ProjectID             string `json:"project_id"`
	ExpectedWorkerVersion int    `json:"expected_worker_version"`
	ExpectedBundleSHA256  string `json:"expected_bundle_sha256"`
	AcknowledgeIncomplete bool   `json:"acknowledge_incomplete"`
	IdempotencyKey        string `json:"idempotency_key"`
}

type ReleaseWorkerCandidateAbandonResult struct {
	ProjectID        string `json:"project_id"`
	WorkerRegistryID string `json:"worker_registry_id"`
	TransferID       string `json:"transfer_id"`
	OperationID      string `json:"operation_id"`
	State            string `json:"state"`
}

const manualWorkerCandidateRecoveryQuery = `
	SELECT c.id,c.run_id,c.node_id,c.attempt,c.fence,c.created_at,c.binding,c.storage_path,
		a.state,a.completed_at,a.result,n.state,n.attempt,n.fence,n.lease_until
	FROM suma_candidate_transfers c
	LEFT JOIN dag_runs r ON r.id=c.run_id AND r.tenant_id=c.tenant_id AND r.project_id=c.project_id
	LEFT JOIN dag_attempts a ON a.run_id=r.id AND a.node_id=c.node_id AND a.attempt=c.attempt
		AND a.fence=c.fence AND a.worker_id=c.worker_id
	LEFT JOIN dag_nodes n ON n.run_id=r.id AND n.id=c.node_id
	WHERE c.tenant_id=? AND c.project_id=? AND c.worker_id=? AND c.subject_hash=?`

type manualWorkerCandidateRecovery struct {
	item         ReleaseWorkerCandidateRecoveryItem
	source       cc.AssignmentRef
	binding      cc.Binding
	raw, storage string
}

func readManualWorkerCandidateRecovery(row manualTransferRecoveryScanner, project, worker string) (out manualWorkerCandidateRecovery, err error) {
	var fence int64
	var proof manualTransferAttemptProof
	item := &out.item
	targets := []any{&item.TransferID, &item.RunID, &item.NodeID, &item.Attempt, &fence, &item.CreatedAt, &out.raw, &out.storage}
	if err = row.Scan(append(targets, proof.scanTargets()...)...); err != nil {
		return out, err
	}
	item.Fence = strconv.FormatInt(fence, 10)
	out.source = cc.AssignmentRef{RunID: item.RunID, NodeID: item.NodeID, Attempt: item.Attempt, Fence: fence, WorkerID: worker}
	original := rc.AssignmentRef{RunID: item.RunID, NodeID: item.NodeID, Attempt: item.Attempt, Fence: fence, WorkerID: worker}
	item.AttemptState, item.Disposition = proof.disposition(original)
	out.binding = exactManualCandidateBinding(out.raw, project, item.TransferID, out.source)
	if out.storage != "" && !storageBlobPath.MatchString(out.storage) {
		out.binding = cc.Binding{}
	}
	if out.binding.State == "" {
		item.Disposition = "unverified"
		return out, nil
	}
	item.BundleSHA256, item.BaseCommit, item.HeadCommit, item.SizeBytes =
		out.binding.BundleSHA256, out.binding.BaseCommit, out.binding.HeadCommit, out.binding.SizeBytes
	if item.Disposition == "eligible" && !candidateResultAllowsAbandonment(item.AttemptState, proof.result.String) {
		item.Disposition = "unverified"
	}
	return out, nil
}

// The persisted manifest is checked against the row's identity. Validating a
// copy as sealed reuses the candidate protocol's immutable manifest rules.
func exactManualCandidateBinding(raw, project, id string, source cc.AssignmentRef) cc.Binding {
	var binding cc.Binding
	if strictjson.Decode([]byte(raw), &binding, 8192) != nil ||
		!desktopWorkspaceHex(id, 32) || binding.TransferID != id || binding.ProjectID != project ||
		candidateHasPoolFields(source) || binding.Source != source {
		return cc.Binding{}
	}
	switch binding.State {
	case "uploading", "sealed", "abandoned":
	default:
		return cc.Binding{}
	}
	manifest := binding
	manifest.State = "sealed"
	if !manifest.ValidSealed() {
		return cc.Binding{}
	}
	return binding
}

func candidateResultAllowsAbandonment(state, raw string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return false
	}
	// The partial shared proof decoder is intentionally tolerant of other DAG
	// result fields. Its authority fields must still use their exact names.
	for name := range fields {
		for _, exact := range []string{"run_id", "node_id", "attempt", "fence", "worker_id", "operation_id", "outcome_unknown",
			"exit_code", "error", "stopped", "workspace_evidence"} {
			if name != exact && strings.EqualFold(name, exact) {
				return false
			}
		}
	}
	var result struct {
		ExitCode          *int                       `json:"exit_code"`
		Error             *string                    `json:"error"`
		Stopped           *bool                      `json:"stopped"`
		WorkspaceEvidence map[string]json.RawMessage `json:"workspace_evidence"`
	}
	if json.Unmarshal([]byte(raw), &result) != nil || result.ExitCode == nil || result.Error == nil || result.Stopped == nil {
		return false
	}
	// Any recorded bundle claim requires reconciliation of its own evidence.
	if _, claimed := result.WorkspaceEvidence["candidate_bundle"]; claimed {
		return false
	}
	switch state {
	case "failed":
		return *result.ExitCode != 0 || *result.Error != "" || *result.Stopped
	case "cancelled":
		return *result.Stopped
	default:
		return false
	}
}
func (s *Service) ListReleaseWorkerCandidateRecovery(ctx context.Context, actor Actor, project, id, cursor string) (out ReleaseWorkerCandidateRecoveryView, err error) {
	if !releaseID(project) || !desktopWorkspaceHex(id, 32) || (cursor != "" && !desktopWorkspaceHex(cursor, 32)) {
		return out, invalid("无效的执行节点候选传输核对请求")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, project)
	if err != nil {
		return out, err
	}
	registration, err := s.manualRunnerRegistrationTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if err = s.checkCurrentManualRunnerTx(ctx, auth, registration); err != nil {
		return out, err
	}
	worker := registration.worker.WorkerID
	rows, err := tx.QueryContext(ctx, s.q(manualWorkerCandidateRecoveryQuery+` AND c.id>? ORDER BY c.id LIMIT 21`),
		auth.currentPrincipal.TenantID, project, worker, registration.tokenDigest, cursor)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out = ReleaseWorkerCandidateRecoveryView{
		ProjectID: project, WorkerRegistryID: id, WorkerID: worker, WorkerVersion: registration.worker.Version,
		Items: make([]ReleaseWorkerCandidateRecoveryItem, 0),
	}
	// State lives in a typed binding rather than an SQL column. Page by the
	// scanned key so filtering historic bindings does not hide later transfers.
	lastCursor := ""
	for scanned := 0; rows.Next(); scanned++ {
		if scanned == releaseWorkerCandidateRecoveryPageSize && lastCursor != "" {
			out.NextCursor = lastCursor
			break
		}
		row, readErr := readManualWorkerCandidateRecovery(rows, project, worker)
		if readErr != nil {
			return ReleaseWorkerCandidateRecoveryView{}, readErr
		}
		item := row.item
		if !desktopWorkspaceHex(item.TransferID, 32) {
			continue
		}
		lastCursor = item.TransferID
		if row.source.Valid() && row.binding.State != "sealed" && row.binding.State != "abandoned" {
			out.Items = append(out.Items, item)
		}
	}
	return out, rows.Err()
}

func (s *Service) AbandonReleaseWorkerCandidateTransfer(ctx context.Context, actor Actor, project, id, transferID string, req ReleaseWorkerCandidateAbandonRequest) (out ReleaseWorkerCandidateAbandonResult, err error) {
	if project != req.ProjectID || !releaseID(project) || !desktopWorkspaceHex(id, 32) || !desktopWorkspaceHex(transferID, 32) ||
		req.ExpectedWorkerVersion < 1 || req.ExpectedWorkerVersion > math.MaxInt32 || !cc.ValidHash(req.ExpectedBundleSHA256) ||
		!req.AcknowledgeIncomplete || !rc.ValidOperationID(req.IdempotencyKey) {
		return out, invalid("无效的执行节点候选传输核对请求")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, project)
	if err != nil {
		return out, err
	}
	registration, err := s.manualRunnerRegistrationTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if err = s.checkCurrentManualRunnerTx(ctx, auth, registration); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"abandon_release_worker_candidate_transfer": transferID, "worker_registry_id": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if registration.worker.Version != req.ExpectedWorkerVersion {
		return out, conflict("执行节点凭证已变化，请刷新")
	}
	tenant, worker := auth.currentPrincipal.TenantID, registration.worker.WorkerID
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, tenant, project); err != nil {
		return out, err
	}
	row, err := readManualWorkerCandidateRecovery(tx.QueryRowContext(ctx, s.q(manualWorkerCandidateRecoveryQuery+` AND c.id=?`),
		tenant, project, worker, registration.tokenDigest, transferID), project, worker)
	if errors.Is(err, sql.ErrNoRows) {
		return out, conflict("原传输已不在当前节点的未封存范围，请刷新")
	}
	if err != nil {
		return out, err
	}
	if row.binding.State != "uploading" || row.item.BundleSHA256 != req.ExpectedBundleSHA256 {
		return out, conflict("原传输清单已变化，请刷新")
	}
	if row.item.Disposition != "eligible" {
		return out, releaseConflict("candidate_transfer_attempt_unresolved", "原执行仍待核对，不能结束候选传输")
	}
	row.binding.State = "abandoned"
	update, err := tx.ExecContext(ctx, s.q(`UPDATE suma_candidate_transfers SET binding=?
		WHERE tenant_id=? AND project_id=? AND id=? AND worker_id=? AND subject_hash=? AND binding=? AND storage_path=?`),
		stringify(row.binding), tenant, project, transferID, worker, registration.tokenDigest, row.raw, row.storage)
	if err != nil {
		return out, err
	}
	if count, e := update.RowsAffected(); e != nil {
		return out, e
	} else if count != 1 {
		return out, conflict("原传输已变化，请刷新")
	}
	receipt := ReleaseWorkerCandidateAbandonResult{ProjectID: project, WorkerRegistryID: id, TransferID: transferID, OperationID: req.IdempotencyKey, State: "abandoned"}
	if err = s.commandActivity(ctx, auth, "release_worker.candidate_transfer_abandoned", transferID, worker); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, receipt); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	return receipt, nil
}
