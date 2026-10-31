package platform

import (
	"context"
	"encoding/json"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Only the task-dispatch bind callback calls this helper, after Core has inserted
// its reserved immutable run/node and before activation. Public DelegateCloud
// still requires a positive expected run version and owns its separate receipt.
func (w *WorkBuddyConnections) issueCloudTaskDispatchDelegationTx(ctx context.Context, auth *authorizedCommand, record workBuddyTaskDispatchSnapshot) (wc.CloudDelegation, error) {
	p := record.Plan
	if auth == nil || !auth.actor.IsBrowser() || p.OwnerID != auth.currentPrincipal.ID || p.ProjectID != auth.target.projectID {
		return wc.CloudDelegation{}, forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, purposeBrowserProjectWrite); err != nil {
		return wc.CloudDelegation{}, err
	}
	stored, err := w.s.workBuddyTaskDispatchPlanTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.ID)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if stored.Plan.State != "ready" || stringify(stored) != stringify(record) {
		return wc.CloudDelegation{}, cloudConflict()
	}
	node, err := w.cloudNodeTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.RunID, p.NodeID)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if node.runVersion != 0 || node.runState != "running" || node.nodeState != "pending" || node.attempt != 0 || node.specHash != p.NodeSpecSHA256 || node.requestHash != p.RequestSHA256 || node.binding.ConnectionID != p.ConnectionID || node.binding.GrantGeneration != p.GrantGeneration {
		return wc.CloudDelegation{}, cloudConflict()
	}
	row, err := w.cloudGrantMetadataTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID, p.OwnerID, p.ConnectionID, p.GrantGeneration, record.ClientBindingSHA256, true)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if row.Version != p.ConnectionVersion {
		return wc.CloudDelegation{}, cloudConflict()
	}
	if err = w.cloudBudgetTx(ctx, auth.tx, auth.target.tenantID, p.ProjectID); err != nil {
		return wc.CloudDelegation{}, err
	}
	var count int
	if err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_delegations WHERE tenant_id=? AND project_id=? AND (id=? OR (run_id=? AND node_id=?))`), auth.target.tenantID, p.ProjectID, p.DelegationID, p.RunID, p.NodeID).Scan(&count); err != nil {
		return wc.CloudDelegation{}, err
	}
	if count != 0 {
		return wc.CloudDelegation{}, cloudConflict()
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	stamp := automationStamp(clock)
	d := cloudDelegationRow{CloudDelegation: wc.CloudDelegation{ID: p.DelegationID, ProjectID: p.ProjectID, OwnerID: p.OwnerID, ConnectionID: p.ConnectionID, Version: 1, RunID: p.RunID, NodeID: p.NodeID, NodeSpecSHA256: p.NodeSpecSHA256, ConnectionVersionAtIssue: row.Version, GrantGeneration: row.GrantGeneration, ClientBindingSHA256: record.ClientBindingSHA256, RequestSHA256: p.RequestSHA256, ExpiresAt: automationStamp(clock.Add(wc.MaxDelegationLifetime)), MaxCreates: 1, State: "active", CreatedAt: stamp, UpdatedAt: stamp}, tenant: auth.target.tenantID}
	if err = w.saveCloudDelegationTx(ctx, auth.tx, d, 0); err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_cloud_delegated", d.ID, "WorkBuddy cloud node delegated"); err != nil {
		return wc.CloudDelegation{}, err
	}
	return d.CloudDelegation, nil
}

func (w *WorkBuddyConnections) DelegateCloud(ctx context.Context, actor Actor, project string, req wc.CloudDelegateRequest) (wc.CloudDelegation, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil {
		return wc.CloudDelegation{}, cloudInvalid()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, req.RunID, purposeBrowserProjectWrite)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	defer close()
	row, _, err := w.cloudGrantTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID, req.ConnectionID, req.GrantGeneration, "")
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.cloudBudgetTx(ctx, auth.tx, auth.target.tenantID, project); err != nil {
		return wc.CloudDelegation{}, err
	}
	node, err := w.cloudNodeTx(ctx, auth.tx, auth.target.tenantID, project, req.RunID, req.NodeID)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if node.specHash != req.NodeSpecSHA256 || node.requestHash != req.RequestSHA256 || node.binding.ConnectionID != req.ConnectionID || node.binding.GrantGeneration != req.GrantGeneration {
		return wc.CloudDelegation{}, cloudConflict()
	}
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, struct {
		Kind    string
		Request wc.CloudDelegateRequest
	}{"workbuddy_cloud_delegate", req})
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if replay {
		var id string
		if json.Unmarshal([]byte(hash), &id) != nil {
			return wc.CloudDelegation{}, cloudConflict()
		}
		d, err := w.cloudDelegationTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
		if err != nil {
			return wc.CloudDelegation{}, err
		}
		if d.OwnerID != auth.currentPrincipal.ID {
			return wc.CloudDelegation{}, missing()
		}
		return d.CloudDelegation, nil
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if req.ValidateAt(clock) != nil {
		return wc.CloudDelegation{}, cloudInvalid()
	}
	if row.Version != req.ExpectedConnectionVersion || node.runVersion != req.ExpectedRunVersion || node.attempt != 0 || node.runState != "running" && node.runState != "paused" {
		return wc.CloudDelegation{}, cloudConflict()
	}
	var count int
	if err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_delegations WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), auth.target.tenantID, project, req.RunID, req.NodeID).Scan(&count); err != nil {
		return wc.CloudDelegation{}, err
	}
	if count != 0 {
		return wc.CloudDelegation{}, cloudConflict()
	}
	stamp := automationStamp(clock)
	d := cloudDelegationRow{CloudDelegation: wc.CloudDelegation{ID: newID(), ProjectID: project, OwnerID: auth.currentPrincipal.ID, ConnectionID: row.ID, Version: 1, RunID: req.RunID, NodeID: req.NodeID, NodeSpecSHA256: node.specHash, ConnectionVersionAtIssue: row.Version, GrantGeneration: row.GrantGeneration, ClientBindingSHA256: cloudClientBinding(row), RequestSHA256: node.requestHash, ExpiresAt: req.ExpiresAt, MaxCreates: 1, CreatesConsumed: 0, State: "active", CreatedAt: stamp, UpdatedAt: stamp}, tenant: auth.target.tenantID}
	if err = w.saveCloudDelegationTx(ctx, auth.tx, d, 0); err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, d.ID); err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_cloud_delegated", d.ID, "WorkBuddy cloud node delegated"); err != nil {
		return wc.CloudDelegation{}, err
	}
	return d.CloudDelegation, auth.tx.Commit()
}

func (w *WorkBuddyConnections) RevokeCloudDelegation(ctx context.Context, actor Actor, project, id string, req wc.CloudRevokeRequest) (wc.CloudDelegation, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil || !handoffID(id, false) {
		return wc.CloudDelegation{}, cloudInvalid()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	defer close()
	d, err := w.cloudDelegationTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if d.OwnerID != auth.currentPrincipal.ID {
		return wc.CloudDelegation{}, missing()
	}
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, struct {
		Kind, ID string
		Request  wc.CloudRevokeRequest
	}{"workbuddy_cloud_revoke", id, req})
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	if replay {
		return d.CloudDelegation, nil
	}
	if d.Version != req.ExpectedVersion || d.State != "active" || d.Version >= wc.MaxVersion {
		return wc.CloudDelegation{}, cloudConflict()
	}
	before := d.Version
	d.Version++
	d.State = "revoked"
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	d.UpdatedAt = automationStamp(clock)
	if err = w.saveCloudDelegationTx(ctx, auth.tx, d, before); err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, d.ID); err != nil {
		return wc.CloudDelegation{}, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_cloud_revoked", id, "WorkBuddy cloud delegation revoked"); err != nil {
		return wc.CloudDelegation{}, err
	}
	return d.CloudDelegation, auth.tx.Commit()
}
func (w *WorkBuddyConnections) GetCloudDelegation(ctx context.Context, actor Actor, project, id string) (wc.CloudDelegation, error) {
	if w == nil || w.s == nil || !handoffID(id, false) {
		return wc.CloudDelegation{}, cloudInvalid()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	defer close()
	d, err := w.cloudDelegationTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
	if err == nil && d.OwnerID != auth.currentPrincipal.ID {
		err = missing()
	}
	if err != nil {
		return wc.CloudDelegation{}, err
	}
	return d.CloudDelegation, err
}
func (w *WorkBuddyConnections) GetCloudSubmission(ctx context.Context, actor Actor, project, id string) (wc.CloudSubmission, error) {
	if w == nil || w.s == nil || !handoffID(id, false) {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	defer close()
	d, err := w.cloudSubmissionTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	var count int
	err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), auth.target.tenantID, project, d.RunID, d.NodeID).Scan(&count)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if count != 1 {
		return wc.CloudSubmission{}, missing()
	}
	return d.CloudSubmission, nil
}
