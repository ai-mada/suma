package platform

import (
	"context"
	"encoding/json"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
	"time"
)

const cloudTurnInvalid = "Invalid WorkBuddy cloud turn request"
const cloudTurnChanged = "The WorkBuddy cloud turn changed; reload before deciding"
const cloudTurnUnavailable = "WorkBuddy cloud turn is unavailable"

func (w *WorkBuddyConnections) CloudTurns(ctx context.Context, actor Actor, project, id string) (wc.CloudTurnView, error) {
	if w == nil || w.s == nil || !handoffID(project, false) || !handoffID(id, false) {
		return wc.CloudTurnView{}, turnInvalid()
	}
	ctx, a, close, e := w.cloudTurnBrowser(ctx, actor, project, id, false, false)
	if e != nil {
		return wc.CloudTurnView{}, e
	}
	defer close()
	source, d, n, e := w.cloudTurnSourceTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
	if e != nil {
		return wc.CloudTurnView{}, e
	}
	h, e := w.cloudTurnHeadTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
	if e != nil {
		return wc.CloudTurnView{}, e
	}
	now, e := w.s.authorityNow(ctx, a.tx)
	if e != nil {
		return wc.CloudTurnView{}, e
	}
	out := wc.CloudTurnView{Source: source, Version: h.version, AllowedActions: []string{}}
	var row cloudTurnRow
	if h.latest != "" {
		row, e = w.cloudTurnRowTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, h.latest)
		if e != nil {
			return out, e
		}
		if !cloudTurnSameSource(source, row.turn.Source) {
			return out, turnChanged()
		}
		t := cloudTurnOverdue(row, now)
		out.LatestTurn = &t
	}
	writable := false
	if _, e = w.s.currentProjectMembershipLockedTx(ctx, a.tx, a.target.tenantID, a.currentPrincipal.ID, project, true, false); e == nil {
		writable = true
	}
	grantOK := false
	if writable {
		_, e = w.cloudGrantMetadataTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256, true)
		grantOK = e == nil
	}
	if out.LatestTurn == nil || out.LatestTurn.State == "completed" || out.LatestTurn.State == "rejected" {
		if h.version <= wc.MaxVersion-2 && w.turnRuntime != nil && grantOK && w.cloudTurnQualifiedTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, d, n) == nil {
			out.AllowedActions = append(out.AllowedActions, "prompt")
		}
	}
	if out.LatestTurn != nil && writable {
		lease, le := time.Parse(time.RFC3339Nano, row.leaseUntil)
		live := le == nil && now.Before(lease) && !out.LatestTurn.Overdue
		if cloudTurnActive(row.turn.State) && live && grantOK {
			out.AllowedActions = append(out.AllowedActions, "cancel")
		}
		if row.turn.State == "unknown" || cloudTurnActive(row.turn.State) && !live {
			out.AllowedActions = append(out.AllowedActions, "reconcile")
		}
	}
	if out.Validate() != nil {
		return wc.CloudTurnView{}, turnChanged()
	}
	return out, nil
}
func (w *WorkBuddyConnections) PromptCloudTurn(ctx context.Context, actor Actor, project, id string, req wc.CloudTurnPromptRequest) (wc.CloudTurn, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil || !handoffID(id, false) {
		return wc.CloudTurn{}, turnInvalid()
	}
	var out wc.CloudTurn
	var permit cloudTurnPermit
	replay := false
	e := func() error {
		ctx, a, close, e := w.cloudTurnBrowser(ctx, actor, project, id, false, true)
		if e != nil {
			return e
		}
		defer close()
		source, d, n, e := w.cloudTurnSourceTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
		if e != nil {
			return e
		}
		body := struct {
			Kind, ID string
			Request  wc.CloudTurnPromptRequest
		}{"workbuddy_cloud_turn_prompt", id, req}
		hash, done, e := w.s.commandReceipt(ctx, a, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if done {
			var original string
			if json.Unmarshal([]byte(hash), &original) != nil {
				return turnChanged()
			}
			row, e := w.cloudTurnRowTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, original)
			if e != nil {
				return e
			}
			if !cloudTurnSameSource(source, row.turn.Source) {
				return turnChanged()
			}
			now, e := w.s.authorityNow(ctx, a.tx)
			out = cloudTurnOverdue(row, now)
			replay = true
			return e
		}
		if w.turnRuntime == nil {
			return turnUnavailable()
		}
		if e = w.cloudTurnQualifiedTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, d, n); e != nil {
			return e
		}
		h, e := w.cloudTurnHeadTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
		if e != nil {
			return e
		}
		if h.version != req.ExpectedVersion || h.version > wc.MaxVersion-2 || source.Collaboration.SubmissionVersion != req.ExpectedSubmissionVersion || n.runVersion != req.ExpectedRunVersion {
			return turnChanged()
		}
		if h.latest != "" {
			old, e := w.cloudTurnRowTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, h.latest)
			if e != nil {
				return e
			}
			if old.turn.State != "completed" && old.turn.State != "rejected" {
				return turnChanged()
			}
		}
		var active int
		if e = a.tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turns WHERE tenant_id=? AND project_id=? AND state IN ('prepared','running','waiting_permission')`), a.target.tenantID, project).Scan(&active); e != nil {
			return e
		}
		if active >= 4 {
			return turnChanged()
		}
		now, e := w.s.authorityNow(ctx, a.tx)
		if e != nil {
			return e
		}
		token, e := cloudTurnToken()
		if e != nil {
			return e
		}
		out = wc.CloudTurn{ID: newID(), Version: 1, OperationID: req.IdempotencyKey, HeadVersionAtStart: h.version, Source: source, OwnerID: a.currentPrincipal.ID, State: "prepared", PromptSHA256: digest(req.Prompt), StartedAt: now.Format(time.RFC3339Nano), DeadlineAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano), UsageState: "unreported"}
		row := cloudTurnRow{turn: out, tenant: a.target.tenantID, project: project, owner: a.currentPrincipal.ID, submission: id, requestHash: hash, leaseOwner: digest(token), fence: 1, leaseUntil: now.Add(cloudTurnLeaseLifetime).Format(time.RFC3339Nano), promptState: "not_started", prompt: req.Prompt}
		if e = w.cloudTurnSaveTx(ctx, a.tx, row, 0); e != nil {
			return e
		}
		if h.version == 0 {
			_, e = a.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_heads(tenant_id,project_id,owner_id,submission_id,version,latest_turn_id)VALUES(?,?,?,?,?,?)`), row.tenant, project, row.owner, id, 1, out.ID)
		} else {
			result, err := a.tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_heads SET version=?,latest_turn_id=? WHERE tenant_id=? AND project_id=? AND owner_id=? AND submission_id=? AND version=?`), h.version+1, out.ID, row.tenant, project, row.owner, id, h.version)
			e = cloudCAS(result, err)
		}
		if e != nil {
			return e
		}
		if e = w.s.saveCommandReceipt(ctx, a, req.IdempotencyKey, hash, out.ID); e != nil {
			return e
		}
		if e = a.tx.Commit(); e != nil {
			return e
		}
		permit = cloudTurnPermit{tenant: row.tenant, project: project, owner: row.owner, id: out.ID, token: token, fence: 1}
		return nil
	}()
	if e != nil {
		return wc.CloudTurn{}, e
	}
	if !replay {
		w.turnRuntime(permit)
	}
	return out, nil
}
func (w *WorkBuddyConnections) GetCloudTurn(ctx context.Context, actor Actor, project, id string, after, limit int) (wc.CloudTurnPage, error) {
	if limit == 0 {
		limit = 50
	}
	if w == nil || w.s == nil || !handoffID(id, false) || after < 0 || after > wc.CloudTurnMaxEntries || limit < 1 || limit > 200 {
		return wc.CloudTurnPage{}, turnInvalid()
	}
	ctx, a, close, e := w.cloudTurnBrowser(ctx, actor, project, id, true, false)
	if e != nil {
		return wc.CloudTurnPage{}, e
	}
	defer close()
	row, e := w.cloudTurnRowTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
	if e != nil {
		return wc.CloudTurnPage{}, e
	}
	source, _, _, e := w.cloudTurnSourceTx(ctx, a.tx, row.tenant, project, row.owner, row.submission)
	if e != nil {
		return wc.CloudTurnPage{}, e
	}
	if !cloudTurnSameSource(source, row.turn.Source) {
		return wc.CloudTurnPage{}, turnChanged()
	}
	now, e := w.s.authorityNow(ctx, a.tx)
	if e != nil {
		return wc.CloudTurnPage{}, e
	}
	out := wc.CloudTurnPage{Turn: cloudTurnOverdue(row, now), Items: []wc.CloudTurnEntry{}, Permissions: []wc.CloudTurnPermission{}, Commands: []wc.CloudTurnCommand{}, NextAfter: after}
	rows, e := a.tx.QueryContext(ctx, w.s.q(`SELECT payload FROM suma_workbuddy_cloud_turn_entries WHERE tenant_id=? AND project_id=? AND turn_id=? AND sequence>? ORDER BY sequence LIMIT ?`), row.tenant, project, id, after, limit)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var raw string
		var entry wc.CloudTurnEntry
		if e = rows.Scan(&raw); e != nil {
			break
		}
		if json.Unmarshal([]byte(raw), &entry) != nil || entry.Validate() != nil || entry.Sequence <= out.NextAfter {
			e = turnChanged()
			break
		}
		out.Items = append(out.Items, entry)
		out.NextAfter = entry.Sequence
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	out.HasMore = out.NextAfter < row.entries
	rows, e = a.tx.QueryContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=? ORDER BY id`), row.tenant, project, id)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			break
		}
		ids = append(ids, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, v := range ids {
		p, err := w.cloudTurnPermissionTx(ctx, a.tx, row, v)
		if err != nil {
			return out, err
		}
		out.Permissions = append(out.Permissions, p)
	}
	rows, e = a.tx.QueryContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=? ORDER BY id`), row.tenant, project, id)
	if e != nil {
		return out, e
	}
	ids = []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			break
		}
		ids = append(ids, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, v := range ids {
		c, _, err := w.cloudTurnCommandTx(ctx, a.tx, row, v)
		if err != nil {
			return out, err
		}
		out.Commands = append(out.Commands, c)
	}
	if out.Validate() != nil {
		return wc.CloudTurnPage{}, turnChanged()
	}
	return out, nil
}
func (w *WorkBuddyConnections) ReplyCloudTurnPermission(ctx context.Context, actor Actor, project, id, permission string, req wc.CloudTurnPermissionReplyRequest) (wc.CloudTurnCommand, error) {
	if req.Validate() != nil || req.ProjectID != project || !handoffID(permission, false) {
		return wc.CloudTurnCommand{}, turnInvalid()
	}
	return w.cloudTurnControl(ctx, actor, project, id, "permission", permission, req.OptionID, req.ExpectedVersion, req.IdempotencyKey, req)
}
func (w *WorkBuddyConnections) CancelCloudTurn(ctx context.Context, actor Actor, project, id string, req wc.CloudTurnControlRequest) (wc.CloudTurnCommand, error) {
	if req.Validate() != nil || req.ProjectID != project {
		return wc.CloudTurnCommand{}, turnInvalid()
	}
	return w.cloudTurnControl(ctx, actor, project, id, "cancel", "", "", req.ExpectedVersion, req.IdempotencyKey, req)
}
func (w *WorkBuddyConnections) ReconcileCloudTurn(ctx context.Context, actor Actor, project, id string, req wc.CloudTurnControlRequest) (wc.CloudTurnCommand, error) {
	if req.Validate() != nil || req.ProjectID != project {
		return wc.CloudTurnCommand{}, turnInvalid()
	}
	return w.cloudTurnControl(ctx, actor, project, id, "reconcile", "", "", req.ExpectedVersion, req.IdempotencyKey, req)
}

func (w *WorkBuddyConnections) cloudTurnControl(ctx context.Context, actor Actor, project, id, kind, permission, option string, expected int, key string, request any) (wc.CloudTurnCommand, error) {
	if w == nil || w.s == nil || !handoffID(id, false) {
		return wc.CloudTurnCommand{}, turnInvalid()
	}
	ctx, a, close, e := w.cloudTurnBrowser(ctx, actor, project, id, true, true)
	if e != nil {
		return wc.CloudTurnCommand{}, e
	}
	defer close()
	row, e := w.cloudTurnRowTx(ctx, a.tx, a.target.tenantID, project, a.currentPrincipal.ID, id)
	if e != nil {
		return wc.CloudTurnCommand{}, e
	}
	source, d, n, e := w.cloudTurnSourceTx(ctx, a.tx, row.tenant, project, row.owner, row.submission)
	if e != nil {
		return wc.CloudTurnCommand{}, e
	}
	if !cloudTurnSameSource(source, row.turn.Source) {
		return wc.CloudTurnCommand{}, turnChanged()
	}
	body := struct {
		Kind, ID, Permission string
		Request              any
	}{"workbuddy_cloud_turn_" + kind, id, permission, request}
	hash, done, e := w.s.commandReceipt(ctx, a, key, body)
	if e != nil {
		return wc.CloudTurnCommand{}, e
	}
	if done {
		var original string
		if json.Unmarshal([]byte(hash), &original) != nil {
			return wc.CloudTurnCommand{}, turnChanged()
		}
		c, _, e := w.cloudTurnCommandTx(ctx, a.tx, row, original)
		return c, e
	}
	now, e := w.s.authorityNow(ctx, a.tx)
	if e != nil {
		return wc.CloudTurnCommand{}, e
	}
	lease, le := time.Parse(time.RFC3339Nano, row.leaseUntil)
	deadline, de := time.Parse(time.RFC3339Nano, row.turn.DeadlineAt)
	live := le == nil && de == nil && now.Before(lease) && now.Before(deadline)
	c := wc.CloudTurnCommand{ID: newID(), TurnID: id, OperationID: key, Kind: kind, State: "prepared", PermissionID: permission, OptionID: option}
	if kind == "reconcile" {
		if expected != row.turn.Version || row.turn.State != "unknown" && (!cloudTurnActive(row.turn.State) || live) {
			return c, turnChanged()
		}
		before := row.turn.Version
		row.turn.Version++
		row.turn.State = "unknown"
		row.turn.ErrorCode = "workbuddy_cloud_turn_unknown"
		row.turn.FinishedAt = now.Format(time.RFC3339Nano)
		row.turn.Overdue = false
		row.fence++
		row.leaseUntil = now.Format(time.RFC3339Nano)
		if e = w.cloudTurnSaveTx(ctx, a.tx, row, before); e != nil {
			return c, e
		}
		c.State = "acknowledged"
	} else {
		if !cloudTurnActive(row.turn.State) || !live {
			return c, turnChanged()
		}
		if _, e = w.cloudGrantMetadataTx(ctx, a.tx, row.tenant, project, row.owner, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256, true); e != nil {
			return c, e
		}
		if kind == "permission" {
			p, e := w.cloudTurnPermissionTx(ctx, a.tx, row, permission)
			if e != nil {
				return c, e
			}
			if p.State != "pending" || p.Version != expected {
				return c, turnChanged()
			}
			var selected string
			for _, o := range p.Options {
				if o.ID == option {
					selected = o.Kind
				}
			}
			if selected != "allow_once" && selected != "reject_once" || selected == "allow_once" && !p.ApproveAllowed {
				return c, turnInvalid()
			}
			if selected == "allow_once" {
				if e = w.cloudTurnQualifiedTx(ctx, a.tx, row.tenant, project, row.owner, d, n); e != nil {
					return c, e
				}
			}
			before := p.Version
			p.Version++
			p.State = "prepared"
			if e = w.cloudTurnSavePermissionTx(ctx, a.tx, row, p, before); e != nil {
				return c, e
			}
		} else {
			if expected != row.turn.Version {
				return c, turnChanged()
			}
			var count int
			if e = a.tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=? AND kind='cancel' AND state IN ('prepared','sent','unknown')`), row.tenant, project, id).Scan(&count); e != nil {
				return c, e
			}
			if count > 0 {
				return c, turnChanged()
			}
		}
	}
	if e = w.cloudTurnInsertCommandTx(ctx, a.tx, row, c, hash); e != nil {
		return c, e
	}
	if e = w.s.saveCommandReceipt(ctx, a, key, hash, c.ID); e != nil {
		return c, e
	}
	if e = a.tx.Commit(); e != nil {
		return c, e
	}
	return c, nil
}
