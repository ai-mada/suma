package platform

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const cloudTurnLeaseLifetime = 30 * time.Second

type cloudTurnRow struct {
	turn                                                                                         wc.CloudTurn
	tenant, project, owner, submission, requestHash, leaseOwner, leaseUntil, promptState, prompt string
	fence                                                                                        int64
	entries, bytes, toolsInvalid                                                                 int
}

func (cloudTurnRow) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "cloudTurnRow{redacted}")
}

type cloudTurnHead struct {
	version int
	latest  string
}

func turnInvalid() error     { return invalid(cloudTurnInvalid) }
func turnChanged() error     { return conflict(cloudTurnChanged) }
func turnUnavailable() error { return &APIError{503, cloudTurnUnavailable, cloudTurnUnavailable, nil} }
func turnLimited() error {
	return &APIError{409, "workbuddy_cloud_turn_limited", "workbuddy_cloud_turn_limited", nil}
}
func cloudTurnToken() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func cloudTurnActive(s string) bool {
	return s == "prepared" || s == "running" || s == "waiting_permission"
}
func cloudTurnSameSource(a, b wc.CloudTurnSource) bool {
	a.Collaboration.SubmissionVersion = 0
	b.Collaboration.SubmissionVersion = 0
	return a == b
}
func cloudTurnOverdue(row cloudTurnRow, now time.Time) wc.CloudTurn {
	t := row.turn
	d, de := time.Parse(time.RFC3339Nano, t.DeadlineAt)
	lease, le := time.Parse(time.RFC3339Nano, row.leaseUntil)
	t.Overdue = cloudTurnActive(t.State) && (de != nil || !now.Before(d) || le != nil || !now.Before(lease))
	return t
}

// Browser authority is real credential authority. Locate routing without locks,
// then take the normal organization -> pool -> project/run order.
func (w *WorkBuddyConnections) cloudTurnBrowser(ctx context.Context, actor Actor, project, id string, byTurn, write bool) (context.Context, *authorizedCommand, func(), error) {
	ctx, tx, cancel, e := w.s.beginAuthorityTx(ctx)
	if e != nil {
		return ctx, nil, func() {}, e
	}
	close := func() { tx.Rollback(); cancel() }
	purpose := purposeBrowserProjectRead
	if write {
		purpose = purposeBrowserProjectWrite
	}
	a, e := w.s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purpose)
	if e != nil {
		close()
		return ctx, nil, func() {}, e
	}
	query := `SELECT run_id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND id=?`
	if byTurn {
		query = `SELECT s.run_id FROM suma_workbuddy_cloud_submissions s JOIN suma_workbuddy_cloud_turns t ON t.submission_id=s.id AND t.tenant_id=s.tenant_id AND t.project_id=s.project_id WHERE t.tenant_id=? AND t.project_id=? AND t.id=? AND t.owner_id=?`
	}
	args := []any{a.target.tenantID, project, id}
	if byTurn {
		args = append(args, a.currentPrincipal.ID)
	}
	var run string
	e = tx.QueryRowContext(ctx, w.s.q(query), args...).Scan(&run)
	if errors.Is(e, sql.ErrNoRows) {
		e = missing()
	}
	if e == nil {
		e = dag.GuardExecutionScopeTx(ctx, tx, w.s.Dialect, a.target.tenantID, project, run)
	}
	if e != nil {
		close()
		return ctx, nil, func() {}, e
	}
	return ctx, a, close, nil
}

// This source proof grants no machine lease and does not renew the drained
// runner. It binds the human turn to its immutable original assignment.
func (w *WorkBuddyConnections) cloudTurnSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, owner, id string) (wc.CloudTurnSource, cloudDelegationRow, cloudNode, error) {
	var source wc.CloudTurnSource
	var d cloudDelegationRow
	var n cloudNode
	sub, e := w.cloudSubmissionTx(ctx, tx, tenant, project, id, true)
	if e != nil {
		return source, d, n, e
	}
	d, e = w.cloudDelegationTx(ctx, tx, tenant, project, sub.DelegationID, true)
	if e != nil {
		return source, d, n, e
	}
	if d.OwnerID != owner {
		return source, d, n, missing()
	}
	n, e = w.cloudNodeTx(ctx, tx, tenant, project, sub.RunID, sub.NodeID)
	if e != nil {
		return source, d, n, e
	}
	if sub.State != "task_known" || sub.Task == nil || d.RunID != sub.RunID || d.NodeID != sub.NodeID || n.specHash != d.NodeSpecSHA256 || n.requestHash != d.RequestSHA256 || sub.RequestSHA256 != d.RequestSHA256 || n.binding.ConnectionID != d.ConnectionID || n.binding.GrantGeneration != d.GrantGeneration {
		return source, d, n, turnChanged()
	}
	a, e := dag.ReadExternalAssignmentTx(ctx, tx, w.s.Dialect, tenant, project, sub.RunID, sub.NodeID, sub.Assignment.Attempt)
	if e != nil {
		return source, d, n, turnChanged()
	}
	var fence int64
	var worker string
	e = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT fence,worker_id FROM dag_nodes WHERE run_id=? AND id=?`)), sub.RunID, sub.NodeID).Scan(&fence, &worker)
	if e != nil {
		return source, d, n, e
	}
	binding, be := d.AssignmentBinding()
	if be != nil || !wc.SameAssignmentIdentity(a.Assignment, sub.Assignment) || a.Binding.DelegationID != d.ID || a.Binding.DelegationSHA256 != binding.DelegationSHA256 || n.attempt != a.Attempt || fence != a.Fence || worker != a.WorkerID {
		return source, d, n, turnChanged()
	}
	source = wc.CloudTurnSource{Collaboration: wc.CloudCollaborationSource{ProjectID: project, RunID: sub.RunID, NodeID: sub.NodeID, SubmissionID: id, SubmissionVersion: sub.Version, TaskID: sub.Task.ID, NodeSpecSHA256: d.NodeSpecSHA256, RequestSHA256: d.RequestSHA256}, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, AssignmentSHA256: a.AssignmentSHA256}
	if source.Validate() != nil {
		return source, d, n, turnChanged()
	}
	return source, d, n, nil
}
func (w *WorkBuddyConnections) cloudTurnQualifiedTx(ctx context.Context, tx *sql.Tx, tenant, project, owner string, d cloudDelegationRow, n cloudNode) error {
	if n.runState != "running" || n.nodeState != "waiting" {
		return turnChanged()
	}
	if _, e := w.s.currentProjectMembershipLockedTx(ctx, tx, tenant, owner, project, true, false); e != nil {
		return e
	}
	if _, e := w.cloudGrantMetadataTx(ctx, tx, tenant, project, owner, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256, true); e != nil {
		return e
	}
	return w.cloudBudgetTx(ctx, tx, tenant, project)
}
func (w *WorkBuddyConnections) cloudTurnHeadTx(ctx context.Context, tx *sql.Tx, tenant, project, owner, submission string) (h cloudTurnHead, e error) {
	e = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT version,latest_turn_id FROM suma_workbuddy_cloud_turn_heads WHERE tenant_id=? AND project_id=? AND owner_id=? AND submission_id=?`)), tenant, project, owner, submission).Scan(&h.version, &h.latest)
	if errors.Is(e, sql.ErrNoRows) {
		return h, nil
	}
	if e == nil && (h.version < 1 || h.version > wc.MaxVersion || !handoffID(h.latest, false)) {
		e = turnChanged()
	}
	return
}
func (w *WorkBuddyConnections) cloudTurnRowTx(ctx context.Context, tx *sql.Tx, tenant, project, owner, id string) (r cloudTurnRow, e error) {
	var raw, state, deadline, operation string
	var version int
	e = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT tenant_id,project_id,owner_id,submission_id,operation_id,request_sha256,version,state,lease_owner,lease_fence,lease_until,deadline_at,prompt_state,entry_count,text_bytes,tools_invalid,prompt,payload FROM suma_workbuddy_cloud_turns WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&r.tenant, &r.project, &r.owner, &r.submission, &operation, &r.requestHash, &version, &state, &r.leaseOwner, &r.fence, &r.leaseUntil, &deadline, &r.promptState, &r.entries, &r.bytes, &r.toolsInvalid, &r.prompt, &raw)
	if errors.Is(e, sql.ErrNoRows) {
		return r, missing()
	}
	if e != nil {
		return r, e
	}
	if r.owner != owner {
		return r, missing()
	}
	if json.Unmarshal([]byte(raw), &r.turn) != nil || r.turn.Validate() != nil || r.turn.ID != id || r.turn.Version != version || r.turn.State != state || r.turn.DeadlineAt != deadline || r.turn.OperationID != operation || r.turn.OwnerID != owner || r.turn.Source.Collaboration.ProjectID != project || r.turn.Source.Collaboration.SubmissionID != r.submission || r.turn.PromptSHA256 != digest(r.prompt) || r.entries < 0 || r.entries > wc.CloudTurnMaxEntries || r.bytes < 0 || r.bytes > wc.CloudTurnMaxTextBytes || r.fence < 1 {
		return r, turnChanged()
	}
	return r, nil
}
func (w *WorkBuddyConnections) cloudTurnSaveTx(ctx context.Context, tx *sql.Tx, r cloudTurnRow, before int) error {
	r.turn.Overdue = false
	if r.turn.Validate() != nil || r.turn.Version != before+1 {
		return turnInvalid()
	}
	if before == 0 {
		_, e := tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turns(id,tenant_id,project_id,owner_id,submission_id,operation_id,request_sha256,version,state,lease_owner,lease_fence,lease_until,deadline_at,prompt_state,entry_count,text_bytes,tools_invalid,prompt,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), r.turn.ID, r.tenant, r.project, r.owner, r.submission, r.turn.OperationID, r.requestHash, r.turn.Version, r.turn.State, r.leaseOwner, r.fence, r.leaseUntil, r.turn.DeadlineAt, r.promptState, r.entries, r.bytes, r.toolsInvalid, r.prompt, stringify(r.turn))
		return e
	}
	result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turns SET version=?,state=?,lease_fence=?,lease_until=?,prompt_state=?,entry_count=?,text_bytes=?,tools_invalid=?,payload=? WHERE tenant_id=? AND project_id=? AND owner_id=? AND id=? AND version=? AND lease_owner=?`), r.turn.Version, r.turn.State, r.fence, r.leaseUntil, r.promptState, r.entries, r.bytes, r.toolsInvalid, stringify(r.turn), r.tenant, r.project, r.owner, r.turn.ID, before, r.leaseOwner)
	return cloudCAS(result, e)
}
func (w *WorkBuddyConnections) cloudTurnPermissionTx(ctx context.Context, tx *sql.Tx, r cloudTurnRow, id string) (p wc.CloudTurnPermission, e error) {
	var raw, state string
	var version int
	e = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT payload,version,state FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=? AND id=?`)), r.tenant, r.project, r.turn.ID, id).Scan(&raw, &version, &state)
	if errors.Is(e, sql.ErrNoRows) {
		return p, missing()
	}
	if e != nil {
		return p, e
	}
	if json.Unmarshal([]byte(raw), &p) != nil || p.Validate() != nil || p.ID != id || p.TurnID != r.turn.ID || p.Version != version || p.State != state {
		return p, turnChanged()
	}
	return
}
func (w *WorkBuddyConnections) cloudTurnSavePermissionTx(ctx context.Context, tx *sql.Tx, r cloudTurnRow, p wc.CloudTurnPermission, before int) error {
	if p.Validate() != nil || p.TurnID != r.turn.ID || p.Version != before+1 {
		return turnInvalid()
	}
	if before == 0 {
		_, e := tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_permissions(id,tenant_id,project_id,turn_id,version,state,payload)VALUES(?,?,?,?,?,?,?)`), p.ID, r.tenant, r.project, r.turn.ID, p.Version, p.State, stringify(p))
		return e
	}
	result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_permissions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND turn_id=? AND id=? AND version=?`), p.Version, p.State, stringify(p), r.tenant, r.project, r.turn.ID, p.ID, before)
	return cloudCAS(result, e)
}
func (w *WorkBuddyConnections) cloudTurnCommandTx(ctx context.Context, tx *sql.Tx, r cloudTurnRow, id string) (c wc.CloudTurnCommand, nonce string, e error) {
	var raw, state, kind, operation string
	e = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT payload,state,kind,operation_id,send_nonce_sha256 FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND owner_id=? AND turn_id=? AND id=?`)), r.tenant, r.project, r.owner, r.turn.ID, id).Scan(&raw, &state, &kind, &operation, &nonce)
	if errors.Is(e, sql.ErrNoRows) {
		return c, "", missing()
	}
	if e != nil {
		return c, "", e
	}
	if json.Unmarshal([]byte(raw), &c) != nil || c.Validate() != nil || c.ID != id || c.TurnID != r.turn.ID || c.State != state || c.Kind != kind || c.OperationID != operation {
		return c, "", turnChanged()
	}
	return
}
func (w *WorkBuddyConnections) cloudTurnInsertCommandTx(ctx context.Context, tx *sql.Tx, r cloudTurnRow, c wc.CloudTurnCommand, hash string) error {
	if c.Validate() != nil || c.TurnID != r.turn.ID {
		return turnInvalid()
	}
	var count int
	if e := tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=?`), r.tenant, r.project, r.turn.ID).Scan(&count); e != nil {
		return e
	}
	if count >= 256 {
		return turnLimited()
	}
	_, e := tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_commands(id,tenant_id,project_id,owner_id,turn_id,operation_id,request_sha256,kind,state,send_nonce_sha256,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), c.ID, r.tenant, r.project, r.owner, r.turn.ID, c.OperationID, hash, c.Kind, c.State, "", stringify(c))
	return e
}
