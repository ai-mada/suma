package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ti "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func toolPreparationInvalid() error {
	return &diagnosticAPIError{cause: &APIError{400, "tool_preparation_invalid", "Invalid managed tool preparation request", nil}, key: "validation.tool_preparation_invalid"}
}
func toolPreparationChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_preparation_source_changed", "Managed tool preparation source changed", nil}, key: "validation.tool_preparation_changed"}
}
func toolPreparationBusy() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_preparation_busy", "An original tool maintenance operation needs reconciliation", nil}, key: "validation.tool_preparation_busy"}
}
func toolPreparationUnavailable() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_preparation_unavailable", "The exact node has not allowed managed tool preparation", nil}, key: "validation.tool_preparation_unavailable"}
}

type ToolPreparationListQuery struct {
	WorkerID string
	Limit    int
	Cursor   string
}
type ToolPreparationPage struct {
	ProjectID   string                    `json:"project_id"`
	WorkerID    string                    `json:"worker_id"`
	AsOf        string                    `json:"as_of"`
	Maintenance tic.ToolMaintenanceStatus `json:"maintenance"`
	Items       []tp.Operation            `json:"items"`
	NextCursor  string                    `json:"next_cursor,omitempty"`
}

func preparationSupports(capability *tp.Capability, adapter string) bool {
	if capability == nil || tp.ValidateCapability(*capability) != nil {
		return false
	}
	for _, id := range capability.AdapterIDs {
		if id == adapter {
			return true
		}
	}
	return false
}

func toolPreparationEffective(value tp.Operation, clock time.Time) tp.Operation {
	if value.State == "pending" {
		until, err := time.Parse(time.RFC3339Nano, value.ExpiresAt)
		if err == nil && !clock.Before(until) {
			value.State, value.ResultCode = "expired", "request_expired"
		}
	}
	if value.State == "claimed" {
		until, err := time.Parse(time.RFC3339Nano, value.LeaseUntil)
		if err == nil && !clock.Before(until) {
			value.State, value.ResultCode = "unknown", "preparation_timeout"
		}
	}
	return value
}
func toolPreparationActive(state string) bool {
	return state == "pending" || state == "claimed" || state == "awaiting_inventory" || state == "unknown"
}

// A prepared receipt permits only this operation's own accepted generation
// transition. A different current generation is drift, even on the same node.
func (s *Service) toolPreparationCurrentTx(ctx context.Context, tx *sql.Tx, row toolPreparationRow, fresh, accepted bool) error {
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, fresh)
	if err != nil {
		return err
	}
	v := row.Value
	if identity != row.IdentitySHA256 || status.InstanceID != v.InstanceID || !preparationSupports(status.ManagedPreparation, v.AdapterID) {
		return toolPreparationChanged()
	}
	generation := status.ManagedPreparation.GenerationSHA256
	if generation != v.ExpectedGenerationSHA256 && (!accepted || v.Receipt == nil || generation != v.Receipt.GenerationSHA256) {
		return toolPreparationChanged()
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, row.Tenant, v.RequestedBy, v.ProjectID, true, true); err != nil {
		return err
	}
	return s.toolInstallationCurrentTx(ctx, tx, toolInstallationRow{Value: row.Scan, Tenant: row.Tenant, IdentitySHA256: row.IdentitySHA256}, false)
}

// Reconciliation records an already claimed effect. Turning off a future
// preparation opt-in cannot strand that original receipt, but a new identity,
// instance, route, unrelated generation or withdrawn human grant still blocks
// it. No missing capability is reconstructed or returned as current authority.
func (s *Service) toolPreparationReconcileTx(ctx context.Context, tx *sql.Tx, row toolPreparationRow, accepted bool) error {
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, false)
	if err != nil {
		return err
	}
	v := row.Value
	if identity != row.IdentitySHA256 || status.InstanceID != v.InstanceID {
		return toolPreparationChanged()
	}
	if status.ManagedPreparation != nil {
		if !preparationSupports(status.ManagedPreparation, v.AdapterID) {
			return toolPreparationChanged()
		}
		generation := status.ManagedPreparation.GenerationSHA256
		if generation != v.ExpectedGenerationSHA256 && (!accepted || v.Receipt == nil || generation != v.Receipt.GenerationSHA256) {
			return toolPreparationChanged()
		}
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, row.Tenant, v.RequestedBy, v.ProjectID, true, true); err != nil {
		return err
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, row.Tenant, row.Scan.RequestedBy, v.ProjectID, true, true); err != nil {
		return err
	}
	return nil
}

func (s *Service) toolPreparationBusyTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, clock time.Time) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','awaiting_inventory','unknown') ORDER BY created_at,id LIMIT 10001`), tenant, project, worker)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > tp.MaxProjectRequests {
		return toolPreparationChanged()
	}
	for _, id := range ids {
		row, err := s.toolPreparationReadTx(ctx, tx, tenant, project, id)
		if err != nil {
			return err
		}
		effective := toolPreparationEffective(row.Value, clock)
		if toolPreparationActive(effective.State) {
			return toolPreparationBusy()
		}
		old := row.Value.Version
		row.Value = effective
		row.Value.Version++
		if err = s.toolPreparationSaveTx(ctx, tx, row, old); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateToolPreparation(ctx context.Context, actor Actor, project string, request tp.CreateRequest) (out tp.Operation, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, request.WorkerID, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	if request.ProjectID != project || tp.ValidateCreateRequest(request) != nil {
		return out, toolPreparationInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"tool_preparation": request})
	if err != nil {
		return out, err
	}
	// The original browser receipt does not depend on today's catalogue, source
	// generation, or TTL. It cannot create a second configuration operation.
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, auth.target.tenantID, project, request.WorkerID, true)
	if err != nil {
		return out, err
	}
	if !preparationSupports(status.ManagedPreparation, request.AdapterID) {
		return out, toolPreparationUnavailable()
	}
	if status.ManagedPreparation.GenerationSHA256 != request.ExpectedGenerationSHA256 {
		return out, toolPreparationChanged()
	}
	scan, err := s.toolInstallationReadTx(ctx, tx, auth.target.tenantID, project, request.InstallationScanID)
	if err != nil {
		return out, err
	}
	if scan.Value.WorkerID != request.WorkerID || scan.Value.Version != request.ExpectedScanVersion || scan.Value.State != "completed" || scan.IdentitySHA256 != identity || scan.Value.InstanceID != status.InstanceID || scan.Value.Result == nil || scan.Value.Result.StopProof == nil {
		return out, toolPreparationChanged()
	}
	if err = s.toolInstallationCurrentTx(ctx, tx, scan, false); err != nil {
		return out, err
	}
	var selected *ti.Item
	for index := range scan.Value.Result.Items {
		item := scan.Value.Result.Items[index]
		if item.AdapterID == request.AdapterID {
			copy := item
			selected = &copy
		}
	}
	if selected == nil || !tp.CanPrepareItem(*selected) {
		return out, toolPreparationChanged()
	}
	if err = s.toolInstallationBusyTx(ctx, tx, auth.target.tenantID, project, request.WorkerID, clock); err != nil {
		return out, err
	}
	if err = s.toolInstallationInspectionBusyTx(ctx, tx, auth.target.tenantID, project, request.WorkerID, clock); err != nil {
		return out, err
	}
	if err = s.toolPreparationBusyTx(ctx, tx, auth.target.tenantID, project, request.WorkerID, clock); err != nil {
		return out, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_preparations WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&count); err != nil {
		return out, err
	}
	if count >= tp.MaxProjectRequests {
		return out, &diagnosticAPIError{cause: &APIError{409, "tool_preparation_limit", "Managed tool preparation history is full", nil}, key: "validation.tool_preparation_limit"}
	}
	out = tp.Operation{ID: newID(), TenantID: auth.target.tenantID, ProjectID: project, WorkerID: request.WorkerID, Version: 1, State: "pending", AdapterID: request.AdapterID, Configuration: request.Configuration, InstanceID: status.InstanceID,
		InstallationScanID: scan.Value.ID, InstallationScanVersion: scan.Value.Version, InstallationReference: ti.Reference{ScanID: scan.Value.ID, ClaimID: scan.Value.ClaimID, Fence: scan.Value.Fence}, InstallationSourceSHA256: scan.Value.SourceSHA256, InstallationItem: *selected, InstallationStopProof: scan.Value.Result.StopProof,
		ExpectedGenerationSHA256: request.ExpectedGenerationSHA256, RequestedBy: auth.currentPrincipal.ID, CreatedAt: clock.UTC().Format(time.RFC3339Nano), ExpiresAt: clock.Add(tp.PendingTTLSeconds * time.Second).UTC().Format(time.RFC3339Nano)}
	out.SourceSHA256 = tp.SourceSHA256(out)
	if tp.ValidateOperation(out) != nil {
		return out, toolPreparationInvalid()
	}
	row := toolPreparationRow{Value: out, Tenant: auth.target.tenantID, IdentitySHA256: identity, Scan: scan.Value}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_preparations(id,tenant_id,project_id,worker_id,state,version,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, row.Tenant, project, out.WorkerID, out.State, out.Version, out.CreatedAt, stringify(row)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) toolPreparationPublicTx(ctx context.Context, tx *sql.Tx, row toolPreparationRow, clock time.Time) (tp.Operation, error) {
	value := toolPreparationEffective(row.Value, clock)
	if err := s.toolPreparationCurrentTx(ctx, tx, row, false, true); err != nil {
		var api *APIError
		if automationAuthorityFailure(err) || errors.As(err, &api) && (api.Code == "tool_preparation_source_changed" || api.Code == "tool_installation_source_changed" || api.Code == "tool_maintenance_unavailable") {
			value.SourceChanged = true
			if value.State == "pending" {
				value.State, value.ResultCode = "superseded", "source_changed"
			}
		} else {
			return value, err
		}
	}
	return value, nil
}

func (s *Service) GetToolPreparation(ctx context.Context, actor Actor, project, id string) (out tp.Operation, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	var worker string
	lookup := tx.QueryRowContext(ctx, s.q(`SELECT worker_id FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND id=?`), actor.credential.tenantID, project, id).Scan(&worker)
	if lookup != nil && !errors.Is(lookup, sql.ErrNoRows) {
		return out, lookup
	}
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, worker, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if !ti.ID(id) {
		return out, toolPreparationInvalid()
	}
	row, err := s.toolPreparationReadTx(ctx, tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out, err = s.toolPreparationPublicTx(ctx, tx, row, clock)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) ListToolPreparations(ctx context.Context, actor Actor, project string, query ToolPreparationListQuery) (out ToolPreparationPage, err error) {
	out.Items = []tp.Operation{}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, query.WorkerID, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if !ti.ID(query.WorkerID) || query.Limit < 1 || query.Limit > 100 || len(query.Cursor) > 2048 {
		return out, toolPreparationInvalid()
	}
	scope := "tool-preparation-history/" + auth.currentPrincipal.ID + "/" + query.WorkerID + "/" + fmt.Sprint(query.Limit)
	position, err := automationDecodeCursor(auth.currentPrincipal, project, scope, query.Cursor)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.ProjectID, out.WorkerID, out.AsOf = project, query.WorkerID, clock.UTC().Format(time.RFC3339Nano)
	out.Maintenance, _, err = s.toolMaintenanceStatusTx(ctx, tx, auth.target.tenantID, project, query.WorkerID, clock)
	if err != nil {
		return out, err
	}
	where, args := "", []any{auth.target.tenantID, project, query.WorkerID}
	if position != "" {
		var created string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=? AND id=?`), auth.target.tenantID, project, query.WorkerID, position).Scan(&created); errors.Is(err, sql.ErrNoRows) {
			return out, toolPreparationInvalid()
		} else if err != nil {
			return out, err
		}
		where = " AND (created_at<? OR (created_at=? AND id<?))"
		args = append(args, created, created, position)
	}
	args = append(args, query.Limit+1)
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=?`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > query.Limit {
		ids = ids[:query.Limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, scope, ids[len(ids)-1])
	}
	for _, id := range ids {
		row, e := s.toolPreparationReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		value, e := s.toolPreparationPublicTx(ctx, tx, row, clock)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, value)
	}
	return out, tx.Commit()
}

func preparationProfilesEqual(a, b tc.Profile) bool { return tc.EqualProfile(a, b) }
