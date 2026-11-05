package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

const releaseWorkerUploadRecoveryPageSize = 20

type ReleaseWorkerUploadRecoveryView struct {
	ProjectID        string                            `json:"project_id"`
	WorkerRegistryID string                            `json:"worker_registry_id"`
	WorkerID         string                            `json:"worker_id"`
	WorkerVersion    int                               `json:"worker_version"`
	Items            []ReleaseWorkerUploadRecoveryItem `json:"items"`
	NextCursor       string                            `json:"next_cursor"`
}

type ReleaseWorkerUploadRecoveryItem struct {
	UploadID       string `json:"upload_id"`
	RunID          string `json:"run_id"`
	NodeID         string `json:"node_id"`
	Attempt        int    `json:"attempt"`
	Fence          string `json:"fence"`
	ManifestSHA256 string `json:"manifest_sha256"`
	CreatedAt      string `json:"created_at"`
	SizeBytes      int64  `json:"size_bytes"`
	AttemptState   string `json:"attempt_state"`
	Disposition    string `json:"disposition"`
}

type ReleaseWorkerUploadAbandonRequest struct {
	ProjectID              string `json:"project_id"`
	ExpectedWorkerVersion  int    `json:"expected_worker_version"`
	ExpectedManifestSHA256 string `json:"expected_manifest_sha256"`
	AcknowledgeIncomplete  bool   `json:"acknowledge_incomplete"`
	IdempotencyKey         string `json:"idempotency_key"`
}

type ReleaseWorkerUploadAbandonResult struct {
	ProjectID        string `json:"project_id"`
	WorkerRegistryID string `json:"worker_registry_id"`
	UploadID         string `json:"upload_id"`
	OperationID      string `json:"operation_id"`
	State            string `json:"state"`
}

// One bounded observation of the original attempt and current node. The
// reverse publication reference must also be absent before abandonment.
const manualWorkerUploadRecoveryQuery = `
	SELECT u.id,u.run_id,u.node_id,u.attempt,u.fence,u.manifest_sha256,u.created_at,u.size_bytes,
		u.artifact_id,EXISTS(SELECT 1 FROM suma_release_artifacts published WHERE published.upload_id=u.id),
		a.state,a.completed_at,a.result,n.state,n.attempt,n.fence,n.lease_until
	FROM suma_artifact_uploads u
	LEFT JOIN dag_runs r ON r.id=u.run_id AND r.tenant_id=u.tenant_id AND r.project_id=u.project_id
	LEFT JOIN dag_attempts a ON a.run_id=r.id AND a.node_id=u.node_id AND a.attempt=u.attempt
		AND a.fence=u.fence AND a.worker_id=u.worker_id
	LEFT JOIN dag_nodes n ON n.run_id=r.id AND n.id=u.node_id
	WHERE u.tenant_id=? AND u.project_id=? AND u.worker_id=? AND u.token_hash=? AND u.state='receiving'`

type uploadRecoveryScanner interface {
	Scan(...any) error
}

func readManualWorkerUploadRecovery(row uploadRecoveryScanner, worker string) (item ReleaseWorkerUploadRecoveryItem, err error) {
	var fence int64
	var artifactID string
	var published bool
	var attemptState, completedAt, raw, nodeState sql.NullString
	var nodeAttempt, nodeFence, lease sql.NullInt64
	if err = row.Scan(&item.UploadID, &item.RunID, &item.NodeID, &item.Attempt, &fence, &item.ManifestSHA256, &item.CreatedAt, &item.SizeBytes,
		&artifactID, &published, &attemptState, &completedAt, &raw, &nodeState, &nodeAttempt, &nodeFence, &lease); err != nil {
		return item, err
	}
	item.Fence = strconv.FormatInt(fence, 10)
	item.AttemptState = "unverified"
	item.Disposition = "unverified"
	original := rc.AssignmentRef{RunID: item.RunID, NodeID: item.NodeID, Attempt: item.Attempt, Fence: fence, WorkerID: worker}
	if !original.Valid() || !rc.ValidDigest(item.ManifestSHA256) {
		return item, nil
	}
	switch attemptState.String {
	case "running", "waiting", "reconciling":
		item.AttemptState = attemptState.String
		item.Disposition = "awaiting_attempt"
		return item, nil
	case "succeeded", "failed", "cancelled":
		item.AttemptState = attemptState.String
	default:
		return item, nil
	}
	if artifactID != "" || published || !nodeState.Valid || !nodeAttempt.Valid || !nodeFence.Valid || !lease.Valid ||
		nodeAttempt.Int64 < int64(item.Attempt) || nodeFence.Int64 < fence {
		return item, nil
	}
	if nodeAttempt.Int64 == int64(item.Attempt) {
		if nodeFence.Int64 != fence || lease.Int64 != 0 {
			return item, nil
		}
		switch nodeState.String {
		case "pending", "ready", "succeeded", "failed", "skipped", "cancelled":
		default:
			return item, nil
		}
	} else if nodeFence.Int64 <= fence {
		return item, nil
	}
	if _, err := time.Parse(time.RFC3339Nano, completedAt.String); err != nil {
		return item, nil
	}
	// The original persisted result must positively name this assignment and
	// include a known outcome. Missing outcome evidence is not a resolved run.
	var result struct {
		rc.AssignmentRef
		OperationID    string `json:"operation_id"`
		OutcomeUnknown *bool  `json:"outcome_unknown"`
	}
	if RejectDuplicateReleaseJSON([]byte(raw.String)) != nil || json.Unmarshal([]byte(raw.String), &result) != nil ||
		result.OutcomeUnknown == nil || *result.OutcomeUnknown || result.AssignmentRef != original || result.OperationID == "" {
		return item, nil
	}
	item.Disposition = "eligible"
	return item, nil
}

func (s *Service) ListReleaseWorkerUploadRecovery(ctx context.Context, actor Actor, project, id, cursor string) (out ReleaseWorkerUploadRecoveryView, err error) {
	if !releaseID(project) || !desktopWorkspaceHex(id, 32) || (cursor != "" && !desktopWorkspaceHex(cursor, 32)) {
		return out, invalid("无效的执行节点上传核对请求")
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
	rows, err := tx.QueryContext(ctx, s.q(manualWorkerUploadRecoveryQuery+` AND u.id>? ORDER BY u.id LIMIT 21`),
		auth.currentPrincipal.TenantID, project, registration.worker.WorkerID, registration.tokenDigest, cursor)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	out = ReleaseWorkerUploadRecoveryView{
		ProjectID: project, WorkerRegistryID: id, WorkerID: registration.worker.WorkerID, WorkerVersion: registration.worker.Version,
		Items: make([]ReleaseWorkerUploadRecoveryItem, 0),
	}
	for rows.Next() {
		item, readErr := readManualWorkerUploadRecovery(rows, registration.worker.WorkerID)
		if readErr != nil {
			return ReleaseWorkerUploadRecoveryView{}, readErr
		}
		if len(out.Items) == releaseWorkerUploadRecoveryPageSize {
			out.NextCursor = out.Items[len(out.Items)-1].UploadID
			break
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}

func (s *Service) AbandonReleaseWorkerUpload(ctx context.Context, actor Actor, project, id, uploadID string, req ReleaseWorkerUploadAbandonRequest) (out ReleaseWorkerUploadAbandonResult, err error) {
	if project != req.ProjectID || !releaseID(project) || !desktopWorkspaceHex(id, 32) || !desktopWorkspaceHex(uploadID, 32) ||
		req.ExpectedWorkerVersion < 1 || req.ExpectedWorkerVersion > math.MaxInt32 || !rc.ValidDigest(req.ExpectedManifestSHA256) ||
		!req.AcknowledgeIncomplete || !rc.ValidOperationID(req.IdempotencyKey) {
		return out, invalid("无效的执行节点上传核对请求")
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
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"abandon_release_worker_upload": uploadID, "worker_registry_id": id, "body": req})
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
	tenant := auth.currentPrincipal.TenantID
	worker := registration.worker.WorkerID
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, tenant, project); err != nil {
		return out, err
	}
	item, err := readManualWorkerUploadRecovery(tx.QueryRowContext(ctx, s.q(manualWorkerUploadRecoveryQuery+` AND u.id=?`),
		tenant, project, worker, registration.tokenDigest, uploadID), worker)
	if errors.Is(err, sql.ErrNoRows) {
		return out, conflict("原上传已不在当前节点的未封存范围，请刷新")
	}
	if err != nil {
		return out, err
	}
	if item.ManifestSHA256 != req.ExpectedManifestSHA256 {
		return out, conflict("原上传清单已变化，请刷新")
	}
	if item.Disposition != "eligible" {
		return out, releaseConflict("artifact_upload_attempt_unresolved", "原执行仍待核对，不能结束上传")
	}
	update, err := tx.ExecContext(ctx, s.q(`UPDATE suma_artifact_uploads SET state='abandoned'
		WHERE tenant_id=? AND project_id=? AND id=? AND worker_id=? AND token_hash=?
			AND manifest_sha256=? AND state='receiving' AND artifact_id=''`),
		tenant, project, uploadID, worker, registration.tokenDigest, req.ExpectedManifestSHA256)
	if err != nil {
		return out, err
	}
	if count, e := update.RowsAffected(); e != nil {
		return out, e
	} else if count != 1 {
		return out, conflict("原上传已变化，请刷新")
	}
	receipt := ReleaseWorkerUploadAbandonResult{ProjectID: project, WorkerRegistryID: id, UploadID: uploadID, OperationID: req.IdempotencyKey, State: "abandoned"}
	if err = s.commandActivity(ctx, auth, "release_worker.artifact_upload_abandoned", uploadID, worker); err != nil {
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
