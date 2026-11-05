package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

type ReleaseWorkerRotationRequest struct {
	ProjectID          string `json:"project_id"`
	ExpectedVersion    int    `json:"expected_version"`
	AcknowledgeStopped bool   `json:"acknowledge_stopped"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type ReleaseWorkerRotationResult struct {
	OperationID string        `json:"operation_id"`
	Worker      ReleaseWorker `json:"worker"`
	Token       string        `json:"token,omitempty"`
}

// Registered, paired and desktop Workers share a registry. Match the original
// issuance operation domain so a name, activity or registry row cannot assert
// that a machine-managed credential supports manual replacement.
const manualRunnerIssuanceJoins = `
	JOIN suma_sessions x ON x.token_hash=w.token_hash AND x.tenant_id=w.tenant_id AND x.user_id=w.created_by
	JOIN suma_credential_authority a ON a.token_hash=w.token_hash AND a.family_id=''
	JOIN suma_credential_slots c ON c.id=a.slot_id AND c.user_id=x.user_id AND c.tenant_id=w.tenant_id
		AND c.project_id=w.project_id AND c.credential_kind=a.credential_kind AND c.family_id=a.family_id
		AND c.grant_id=a.grant_id AND c.source_id=a.source_id
	JOIN suma_credential_issuance i ON i.token_hash=w.token_hash AND i.slot_id=c.id AND i.grant_id=c.grant_id AND i.family_id=c.family_id
	JOIN suma_credential_grants g ON g.id=c.grant_id AND g.user_id=c.user_id AND g.tenant_id=c.tenant_id
		AND g.project_id=c.project_id AND g.credential_kind=c.credential_kind AND g.approved_by=c.user_id
		AND g.source_id=a.source_id AND g.source_generation=a.source_generation AND g.scope_kind='runner'
	JOIN suma_auth_sources z ON z.id=a.source_id AND z.user_id=c.user_id
	JOIN suma_token_scopes b ON b.token_hash=w.token_hash AND b.project_id=w.project_id AND b.kind='runner'`

const manualRunnerCredentialCurrent = `
	a.revoked=0 AND c.revoked=0 AND g.revoked=0 AND z.revoked=0
	AND z.generation=a.source_generation AND z.authenticated_at<>'' AND z.source_kind IN ('setup','password','invitation')
	AND c.current_digest=w.token_hash
	AND c.expires_at=x.expires_at AND g.expires_at=x.expires_at AND w.expires_at=x.expires_at`

func manualRunnerSlotMatches(tenant, project, slotOperation, issuanceOperation string) bool {
	return issuanceOperation != "" && slotOperation == digest("registered-runner:"+tenant+":"+project+":"+issuanceOperation)
}

func (s *Service) manualRunnerRotationAllowedTx(ctx context.Context, auth *authorizedCommand, project string) (map[string]bool, error) {
	p := auth.currentPrincipal
	if auth.actor.kind != actorBrowser {
		return nil, nil
	}
	allowed, err := s.releaseAdministratorTx(ctx, auth.tx, p, project)
	if err != nil || !allowed {
		return nil, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT w.id,w.worker_id,x.label,c.operation_id,i.operation_id
		FROM suma_release_workers w`+manualRunnerIssuanceJoins+`
		WHERE w.tenant_id=? AND w.project_id=? AND w.created_by=? AND w.status='active'
			AND w.version>0 AND w.version<? AND a.credential_kind=? AND `+manualRunnerCredentialCurrent),
		p.TenantID, project, p.ID, math.MaxInt32, kindName(actorProjectRunner))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id, worker, label, slotOperation, issuanceOperation string
		if err = rows.Scan(&id, &worker, &label, &slotOperation, &issuanceOperation); err != nil {
			return nil, err
		}
		if label == "release-worker:"+worker && manualRunnerSlotMatches(p.TenantID, project, slotOperation, issuanceOperation) {
			out[id] = true
		}
	}
	return out, rows.Err()
}

type manualRunnerRegistration struct {
	worker                           ReleaseWorker
	rawProfiles, tokenDigest, slotID string
}

func (s *Service) manualRunnerRegistrationTx(ctx context.Context, auth *authorizedCommand, project, id string) (out manualRunnerRegistration, err error) {
	p := auth.currentPrincipal
	worker := &out.worker
	var slotOperation, issuanceOperation string
	query := `SELECT w.id,w.project_id,w.worker_id,w.version,w.status,w.profiles,w.deployment,w.created_at,w.expires_at,
		w.token_hash,c.id,c.operation_id,i.operation_id FROM suma_release_workers w` + manualRunnerIssuanceJoins + `
		WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.created_by=? AND a.credential_kind=?`
	err = auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(query)), p.TenantID, project, id, p.ID, kindName(actorProjectRunner)).
		Scan(&worker.ID, &worker.ProjectID, &worker.WorkerID, &worker.Version, &worker.Status, &out.rawProfiles, &worker.Deployment,
			&worker.CreatedAt, &worker.ExpiresAt, &out.tokenDigest, &out.slotID, &slotOperation, &issuanceOperation)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	if err != nil {
		return out, err
	}
	if !manualRunnerSlotMatches(p.TenantID, project, slotOperation, issuanceOperation) {
		return out, forbidden()
	}

	return out, nil
}

func (s *Service) checkCurrentManualRunnerTx(ctx context.Context, auth *authorizedCommand, registration manualRunnerRegistration) error {
	worker, p := registration.worker, auth.currentPrincipal
	if worker.Status != "active" || worker.Version < 1 || worker.Version > math.MaxInt32 {
		return forbidden()
	}
	// readCredential checks the current Grant, slot and source. Its separate
	// expiry decision belongs to command authorization and is intentionally
	// omitted here: the browser authorizes rotation of an expired credential.
	old, err := s.readCredential(ctx, auth.tx, registration.tokenDigest, true)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 401 {
			return forbidden()
		}
		return err
	}
	registeredExpiry, err := time.Parse(time.RFC3339Nano, worker.ExpiresAt)
	if err != nil || !old.expires.Equal(registeredExpiry) || old.kind != actorProjectRunner ||
		old.ref.tenantID != p.TenantID || old.ref.subjectID != p.ID || old.ref.projectID != worker.ProjectID ||
		old.ref.slotID != registration.slotID || old.label != "release-worker:"+worker.WorkerID {
		return forbidden()
	}
	return nil
}

func (s *Service) RotateReleaseWorkerCredential(ctx context.Context, actor Actor, project, id string, req ReleaseWorkerRotationRequest) (out ReleaseWorkerRotationResult, err error) {
	if !desktopWorkspaceHex(id, 32) || project != req.ProjectID || !releaseID(project) ||
		req.ExpectedVersion < 1 || req.ExpectedVersion >= math.MaxInt32 ||
		!req.AcknowledgeStopped || !rc.ValidOperationID(req.IdempotencyKey) {
		return out, invalid("执行节点凭证轮换请求无效")
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
	p := auth.currentPrincipal
	registration, err := s.manualRunnerRegistrationTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	worker := registration.worker
	oldDigest := registration.tokenDigest
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"rotate_release_worker": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if worker.Status != "active" || worker.Version != req.ExpectedVersion {
		return out, conflict("执行节点凭证已变化，请刷新")
	}
	if err = s.checkCurrentManualRunnerTx(ctx, auth, registration); err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(registration.rawProfiles), &worker.Profiles); err != nil {
		return out, err
	}
	if worker.Profiles == nil {
		worker.Profiles = []ReleaseProfileRegistration{}
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, p.TenantID, project); err != nil {
		return out, err
	}
	if err = s.manualRunnerRotationDrainedTx(ctx, tx, auth, worker.WorkerID, oldDigest); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=? AND revoked=0`), oldDigest); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_slots SET revoked=1 WHERE id=? AND current_digest=? AND revoked=0`), registration.slotID, oldDigest); err != nil {
		return out, err
	}
	issued, err := s.issueRegisteredRunnerTx(ctx, tx, auth, "release-worker:"+worker.WorkerID, req.IdempotencyKey)
	if err != nil {
		return out, err
	}
	update, err := tx.ExecContext(ctx, s.q(`UPDATE suma_release_workers SET token_hash=?,version=version+1,expires_at=?
		WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND token_hash=? AND status='active'`),
		digest(issued.Token), issued.ExpiresAt, p.TenantID, project, id, worker.Version, oldDigest)
	if err != nil {
		return out, err
	}
	if rows, e := update.RowsAffected(); e != nil {
		return out, e
	} else if rows != 1 {
		return out, conflict("执行节点凭证已变化，请刷新")
	}
	// The inventory is a current, replaceable observation keyed only by
	// Worker ID. Require a fresh report after changing the credential binding.
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), p.TenantID, project, worker.WorkerID); err != nil {
		return out, err
	}
	worker.Version++
	worker.ExpiresAt = issued.ExpiresAt
	worker.ManualRotationAllowed = worker.Version < math.MaxInt32
	receipt := ReleaseWorkerRotationResult{OperationID: req.IdempotencyKey, Worker: worker}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, receipt); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "release_worker.credential_rotated", id, worker.WorkerID); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	receipt.Token = issued.Token
	return receipt, nil
}

func workerRotationPending() error {
	return releaseConflict("worker_rotation_pending", "节点仍有在途操作或待核对结果，请先处理后再轮换")
}

func (s *Service) manualRunnerRotationDrainedTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, worker, oldDigest string) error {
	if err := s.desktopRunnerRenewalDrainedTx(ctx, tx, auth, worker, oldDigest); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == "desktop_runner_renewal_pending" {
			return workerRotationPending()
		}
		return err
	}
	tenant, project := auth.target.tenantID, auth.target.projectID
	for _, query := range []string{
		`SELECT COUNT(*) FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('claimed','awaiting_inventory','unknown')`,
		`SELECT COUNT(*) FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('claimed','unknown')`,
		`SELECT COUNT(*) FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('claimed','unknown')`,
	} {
		var count int
		if err := tx.QueryRowContext(ctx, s.q(query), tenant, project, worker).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return workerRotationPending()
		}
	}
	return s.manualRunnerProviderEffectsDrainedTx(ctx, tx, tenant, project, oldDigest)
}

// An issued ticket is covered by the shared drain. Consuming a provider
// effect grant can outlive its ticket or DAG lease. Preserve the old credential
// until the durable record proves the exact provider effect is terminal.
func (s *Service) manualRunnerProviderEffectsDrainedTx(ctx context.Context, tx *sql.Tx, tenant, project, oldDigest string) error {
	queries := []struct {
		sql   string
		known func(raw, operation, requestSHA256, grant string) bool
	}{
		{
			`SELECT COALESCE(d.data,''),a.operation_id,a.request_sha256,a.effect_grant_id
			 FROM suma_release_authorizations a LEFT JOIN suma_release_deployments d
			   ON d.tenant_id=a.tenant_id AND d.project_id=a.project_id AND d.operation_id=a.operation_id
			 WHERE a.tenant_id=? AND a.project_id=? AND a.worker_token_hash=? AND a.state='consumed' AND a.effect_grant_id<>''`,
			func(raw, operation, requestSHA256, _ string) bool {
				var release ReleaseDeployment
				return json.Unmarshal([]byte(raw), &release) == nil && release.Observation != nil &&
					release.OperationID == operation && release.RequestSHA256 == requestSHA256 &&
					release.Assignment != nil && release.Generation > 0 && release.Generation == release.Observation.Generation &&
					release.Observation.OperationID == operation && release.Observation.RequestSHA256 == requestSHA256 &&
					releaseObservationKnownTerminal(*release.Observation)
			},
		},
		{
			`SELECT COALESCE(o.data,''),a.operation_id,a.request_sha256,a.effect_grant_id
			 FROM suma_repository_authorizations a LEFT JOIN suma_repository_operations o
			   ON o.tenant_id=a.tenant_id AND o.project_id=a.project_id AND o.operation_id=a.operation_id
			 WHERE a.tenant_id=? AND a.project_id=? AND a.worker_token_hash=? AND a.state='consumed' AND a.effect_grant_id<>''`,
			func(raw, operation, requestSHA256, grant string) bool {
				var repository repositoryOperation
				if json.Unmarshal([]byte(raw), &repository) != nil || repository.Observation == nil {
					return false
				}
				observed := *repository.Observation
				return repository.Execution.OperationID == operation && repository.RequestSHA256 == requestSHA256 &&
					repository.Assignment != nil && repository.WorkerTokenHash == oldDigest && repository.Execution.Generation > 0 &&
					repository.Execution.Generation == observed.Execution.Generation && observed.OperationID == operation &&
					observed.RequestSHA256 == requestSHA256 && observed.Phase == "apply" && repositoryKnownObservation(observed) &&
					(observed.State == "not_applied" || observed.EffectGrantID == grant)
			},
		},
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, s.q(query.sql), tenant, project, oldDigest)
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw, operation, requestSHA256, grant string
			if err = rows.Scan(&raw, &operation, &requestSHA256, &grant); err != nil {
				rows.Close()
				return err
			}
			if !query.known(raw, operation, requestSHA256, grant) {
				rows.Close()
				return workerRotationPending()
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
