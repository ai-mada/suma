package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

type ToolInstallationScanPage struct {
	ProjectID   string                    `json:"project_id"`
	WorkerID    string                    `json:"worker_id"`
	AsOf        string                    `json:"as_of"`
	Maintenance tic.ToolMaintenanceStatus `json:"maintenance"`
	Items       []tii.Scan                `json:"items"`
	NextCursor  string                    `json:"next_cursor,omitempty"`
}
type ToolInstallationScanListQuery struct {
	WorkerID string `json:"worker_id"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

func toolInstallationInvalid() error {
	return invalidDiagnostic("validation.tool_installation_scan", "工具安装检测请求无效", nil)
}
func toolInstallationChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_installation_source_changed", "工具安装检测依据已变化，请核对原请求", nil}, key: "validation.tool_installation_changed"}
}
func toolInstallationBusy() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_maintenance_in_progress", "执行节点已有未完成的维护操作，请先核对原请求", nil}, key: "validation.tool_installation_busy"}
}
func toolInstallationUnavailable() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_installation_unavailable", "执行节点尚未声明当前工具安装检测能力", nil}, key: "validation.tool_installation_unavailable"}
}
func (s *Service) toolInstallationBrowserAuthTx(ctx context.Context, tx *sql.Tx, actor Actor, project, worker string, purpose commandPurpose) (*authorizedCommand, error) {
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, err
	}
	var owner string
	err := tx.QueryRowContext(ctx, s.q(`SELECT p.owner_tenant_id FROM suma_execution_pool_runners r JOIN suma_execution_pools p ON p.id=r.pool_id WHERE r.worker_id=?`), worker).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: worker}, purpose, []string{owner})
}
func installationSupports(cap *tii.Capability, ids []string) bool {
	if cap == nil || tii.ValidateCapability(*cap) != nil {
		return false
	}
	for _, id := range ids {
		found := false
		for _, candidate := range cap.AdapterIDs {
			if id == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func installationEffective(v tii.Scan, clock time.Time) tii.Scan {
	if v.State == "pending" {
		until, e := time.Parse(time.RFC3339Nano, v.ExpiresAt)
		if e == nil && !clock.Before(until) {
			v.State = "expired"
			v.ResultCode = "request_expired"
		}
	}
	if v.State == "claimed" {
		until, e := time.Parse(time.RFC3339Nano, v.LeaseUntil)
		if e == nil && !clock.Before(until) {
			v.State = "unknown"
			v.ResultCode = "probe_timeout"
		}
	}
	return v
}
func installationActive(state string) bool {
	return state == "pending" || state == "claimed" || state == "unknown"
}
func (s *Service) toolInstallationCurrentTx(ctx context.Context, tx *sql.Tx, row toolInstallationRow, fresh bool) error {
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, fresh)
	if err != nil {
		return err
	}
	if identity != row.IdentitySHA256 || status.InstanceID != row.Value.InstanceID || !installationSupports(status.InstalledDiscovery, row.Value.AdapterIDs) {
		return toolInstallationChanged()
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, row.Tenant, row.Value.RequestedBy, row.Value.ProjectID, true, true); err != nil {
		return err
	}
	return nil
}

// This bounded indexed check is shared with profile-inspection creation. An
// expired pending request can be finalized; an already-started one cannot.
func (s *Service) toolInstallationBusyTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, clock time.Time) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') ORDER BY created_at,id LIMIT 10001`), tenant, project, worker)
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
	if len(ids) > tii.MaxProjectRequests {
		return toolInstallationChanged()
	}
	for _, id := range ids {
		row, e := s.toolInstallationReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		v := installationEffective(row.Value, clock)
		if installationActive(v.State) {
			return toolInstallationBusy()
		}
		old := row.Value.Version
		row.Value = v
		row.Value.Version++
		if e = s.toolInstallationSaveTx(ctx, tx, row, old); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) toolInstallationInspectionBusyTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, clock time.Time) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') ORDER BY created_at,id LIMIT 10001`), tenant, project, worker)
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
	if len(ids) > tic.MaxProjectRequests {
		return toolInstallationChanged()
	}
	for _, id := range ids {
		row, e := s.toolInspectionReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		v := toolInspectionEffective(row.Value, clock)
		if toolInspectionUnfinished(v.State) {
			return toolInstallationBusy()
		}
		old := row.Value.Version
		row.Value = v
		row.Value.Version++
		if e = s.toolInspectionSaveTx(ctx, tx, row, old); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) CreateToolInstallationScan(ctx context.Context, actor Actor, project string, req tii.CreateRequest) (out tii.Scan, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, req.WorkerID, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	if req.ProjectID != project || tii.ValidateCreateRequest(req) != nil {
		return out, toolInstallationInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"tool_installation_scan": req})
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(hash), &out) != nil || tii.ValidateScan(out) != nil {
			return out, toolInstallationChanged()
		}
		return out, nil
	}
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, auth.target.tenantID, project, req.WorkerID, true)
	if err != nil {
		return out, err
	}
	if !installationSupports(status.InstalledDiscovery, req.AdapterIDs) {
		return out, toolInstallationUnavailable()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = s.toolInstallationBusyTx(ctx, tx, auth.target.tenantID, project, req.WorkerID, clock); err != nil {
		return out, err
	}
	if err = s.toolInstallationInspectionBusyTx(ctx, tx, auth.target.tenantID, project, req.WorkerID, clock); err != nil {
		return out, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&count); err != nil {
		return out, err
	}
	if count >= tii.MaxProjectRequests {
		return out, toolInstallationBusy()
	}
	out = tii.Scan{ID: newID(), ProjectID: project, WorkerID: req.WorkerID, AdapterIDs: append([]string{}, req.AdapterIDs...), InstanceID: status.InstanceID, Version: 1, State: "pending", RequestedBy: auth.currentPrincipal.ID, CreatedAt: clock.UTC().Format(time.RFC3339Nano), ExpiresAt: clock.Add(tii.PendingTTLSeconds * time.Second).UTC().Format(time.RFC3339Nano)}
	out.SourceSHA256 = tii.ScanSourceSHA256(out)
	if err = s.toolInstallationSaveTx(ctx, tx, toolInstallationRow{Value: out, Tenant: auth.target.tenantID, IdentitySHA256: identity}, 0); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_installation.requested", out.ID, req.WorkerID); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) toolInstallationPublicTx(ctx context.Context, tx *sql.Tx, row toolInstallationRow, clock time.Time) (tii.Scan, error) {
	v := installationEffective(row.Value, clock)
	// Terminal history remains readable to the current project reader, but it
	// must not be presented as a current installation after identity drift.
	if err := s.toolInstallationCurrentTx(ctx, tx, row, false); err != nil {
		var api *APIError
		if automationAuthorityFailure(err) || errors.As(err, &api) && (api.Code == "tool_installation_source_changed" || api.Code == "tool_maintenance_unavailable") {
			v.SourceChanged = true
			if v.State == "pending" {
				v.State = "superseded"
				v.ResultCode = "source_changed"
			}
		} else {
			return v, err
		}
	}
	return v, nil
}
func (s *Service) GetToolInstallationScan(ctx context.Context, actor Actor, project, id string) (out tii.Scan, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	// Resolve only the lock participant before authorization, never public facts.
	var worker string
	lookup := tx.QueryRowContext(ctx, s.q(`SELECT worker_id FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND id=?`), actor.credential.tenantID, project, id).Scan(&worker)
	if lookup != nil && !errors.Is(lookup, sql.ErrNoRows) {
		return out, lookup
	}
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, worker, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if !tii.ID(id) {
		return out, toolInstallationInvalid()
	}
	row, err := s.toolInstallationReadTx(ctx, tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out, err = s.toolInstallationPublicTx(ctx, tx, row, clock)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ListToolInstallationScans(ctx context.Context, actor Actor, project string, q ToolInstallationScanListQuery) (out ToolInstallationScanPage, err error) {
	out.Items = []tii.Scan{}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInstallationBrowserAuthTx(ctx, tx, actor, project, q.WorkerID, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if !tii.ID(q.WorkerID) || q.Limit < 1 || q.Limit > 100 || len(q.Cursor) > 2048 {
		return out, toolInstallationInvalid()
	}
	scope := "tool-installation-history/" + auth.currentPrincipal.ID + "/" + q.WorkerID + "/" + fmt.Sprint(q.Limit)
	position, err := automationDecodeCursor(auth.currentPrincipal, project, scope, q.Cursor)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.ProjectID, out.WorkerID, out.AsOf = project, q.WorkerID, clock.UTC().Format(time.RFC3339Nano)
	out.Maintenance, _, err = s.toolMaintenanceStatusTx(ctx, tx, auth.target.tenantID, project, q.WorkerID, clock)
	if err != nil {
		return out, err
	}
	where := ""
	args := []any{auth.target.tenantID, project, q.WorkerID}
	if position != "" {
		var created string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND worker_id=? AND id=?`), auth.target.tenantID, project, q.WorkerID, position).Scan(&created); errors.Is(err, sql.ErrNoRows) {
			return out, toolInstallationInvalid()
		} else if err != nil {
			return out, err
		}
		where = " AND (created_at<? OR (created_at=? AND id<?))"
		args = append(args, created, created, position)
	}
	args = append(args, q.Limit+1)
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND worker_id=?`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), args...)
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
	for i, id := range ids {
		if i == q.Limit {
			last := out.Items[len(out.Items)-1]
			out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, scope, last.ID)
			break
		}
		row, e := s.toolInstallationReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		value, e := s.toolInstallationPublicTx(ctx, tx, row, clock)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, value)
	}
	return out, tx.Commit()
}
