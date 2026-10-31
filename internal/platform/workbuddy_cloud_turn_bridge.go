package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ai-mada/suma/internal/dag"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Only the original committed prepare stack holds token. Persistence stores its
// digest; a restarted runtime cannot reconstruct a Prompt send permit.
type cloudTurnPermit struct {
	tenant, project, owner, id, token string
	fence                             int64
}

func (cloudTurnPermit) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "cloudTurnPermit{redacted}")
}

type cloudTurnLease struct {
	Turn      wc.CloudTurn
	Prompt    string
	Client    *workbuddy.Client
	Secrets   []string
	Remaining time.Duration
}

func (cloudTurnLease) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "cloudTurnLease{redacted}")
}

type cloudTurnRuntimeEvent struct {
	Kind                                     string // entry, permission, tool, state, prompt_sent, terminal
	Entry                                    *wc.CloudTurnEntry
	Permission                               *wc.CloudTurnPermission
	ToolCallID, ToolState                    string
	ToolStart                                bool
	State, StopReason, ErrorCode, UsageState string
	Usage                                    *tc.Usage
	KnownNotSent                             bool
}

func (cloudTurnRuntimeEvent) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "cloudTurnRuntimeEvent{redacted}")
}

type cloudTurnRuntimeCommand struct {
	Command    wc.CloudTurnCommand
	Permission wc.CloudTurnPermission
	token      string
	// The once-only Cancel call may retire only this observed version snapshot.
	// This private send scope is never accepted from a browser or reconstructed.
	cancelPermissions []cloudTurnPermissionVersion
}

type cloudTurnPermissionVersion struct {
	ID      string
	Version int
}

func (cloudTurnRuntimeCommand) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "cloudTurnRuntimeCommand{redacted}")
}

// A private permit is a capability of the original committed call stack, not a
// reconstructed Actor. Current human membership is independently re-read.
func (w *WorkBuddyConnections) cloudTurnPermitTx(ctx context.Context, p cloudTurnPermit, current, work bool) (context.Context, *sql.Tx, cloudTurnRow, func(), error) {
	var empty cloudTurnRow
	if w == nil || w.s == nil || p.token == "" || p.fence < 1 {
		return ctx, nil, empty, func() {}, forbidden()
	}
	ctx, tx, cancel, e := w.s.beginAuthorityTx(ctx)
	if e != nil {
		return ctx, nil, empty, func() {}, e
	}
	close := func() { tx.Rollback(); cancel() }
	fail := func(err error) (context.Context, *sql.Tx, cloudTurnRow, func(), error) {
		close()
		return ctx, nil, empty, func() {}, err
	}
	if e = w.s.authorityWriter(ctx, tx); e != nil {
		return fail(e)
	}
	orgs, e := w.s.authorityOrganizations(ctx, tx, p.owner, p.tenant)
	if e != nil {
		return fail(e)
	}
	if e = w.s.lockHumanAuthorities(ctx, tx, []string{p.owner}, orgs, []string{"workbuddy-cloud-turn:" + p.id}); e != nil {
		return fail(e)
	}
	var run string
	e = tx.QueryRowContext(ctx, w.s.q(`SELECT s.run_id FROM suma_workbuddy_cloud_submissions s JOIN suma_workbuddy_cloud_turns t ON t.submission_id=s.id AND t.tenant_id=s.tenant_id AND t.project_id=s.project_id WHERE t.tenant_id=? AND t.project_id=? AND t.owner_id=? AND t.id=?`), p.tenant, p.project, p.owner, p.id).Scan(&run)
	if e != nil {
		return fail(turnChanged())
	}
	if e = dag.GuardExecutionScopeTx(ctx, tx, w.s.Dialect, p.tenant, p.project, run); e != nil {
		return fail(e)
	}
	row, e := w.cloudTurnRowTx(ctx, tx, p.tenant, p.project, p.owner, p.id)
	if e != nil {
		return fail(e)
	}
	if row.fence != p.fence || row.leaseOwner != digest(p.token) {
		return fail(turnChanged())
	}
	if current {
		now, e := w.s.authorityNow(ctx, tx)
		if e != nil {
			return fail(e)
		}
		until, ue := time.Parse(time.RFC3339Nano, row.leaseUntil)
		deadline, de := time.Parse(time.RFC3339Nano, row.turn.DeadlineAt)
		if !cloudTurnActive(row.turn.State) || ue != nil || de != nil || !now.Before(until) || !now.Before(deadline) {
			return fail(turnChanged())
		}
		if _, e = w.s.currentProjectMembershipLockedTx(ctx, tx, p.tenant, p.owner, p.project, true, false); e != nil {
			return fail(e)
		}
		source, d, n, e := w.cloudTurnSourceTx(ctx, tx, p.tenant, p.project, p.owner, row.submission)
		if e != nil {
			return fail(e)
		}
		if !cloudTurnSameSource(source, row.turn.Source) {
			return fail(turnChanged())
		}
		if _, e = w.cloudGrantMetadataTx(ctx, tx, p.tenant, p.project, p.owner, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256, true); e != nil {
			return fail(e)
		}
		if work {
			if e = w.cloudTurnQualifiedTx(ctx, tx, p.tenant, p.project, p.owner, d, n); e != nil {
				return fail(e)
			}
		}
	}
	return ctx, tx, row, close, nil
}
func (w *WorkBuddyConnections) cloudTurnLeaseRead(ctx context.Context, p cloudTurnPermit, work bool) (cloudTurnLease, error) {
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, true, work)
	if e != nil {
		return cloudTurnLease{}, e
	}
	defer close()
	_, d, _, e := w.cloudTurnSourceTx(ctx, tx, p.tenant, p.project, p.owner, row.submission)
	if e != nil {
		return cloudTurnLease{}, e
	}
	connection, client, e := w.cloudGrantTx(ctx, tx, p.tenant, p.project, p.owner, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256)
	if e != nil {
		return cloudTurnLease{}, e
	}
	grant, e := w.open(connection)
	if e != nil {
		return cloudTurnLease{}, turnUnavailable()
	}
	now, e := w.s.authorityNow(ctx, tx)
	if e != nil {
		return cloudTurnLease{}, e
	}
	deadline, _ := time.Parse(time.RFC3339Nano, row.turn.DeadlineAt)
	return cloudTurnLease{Turn: row.turn, Prompt: row.prompt, Client: client, Secrets: []string{grant.AccessToken, grant.RefreshToken, w.app.ClientSecret}, Remaining: deadline.Sub(now)}, nil
}
func (w *WorkBuddyConnections) cloudTurnCheck(ctx context.Context, p cloudTurnPermit) (cloudTurnLease, error) {
	return w.cloudTurnLeaseRead(ctx, p, false)
}
func (w *WorkBuddyConnections) cloudTurnCheckWork(ctx context.Context, p cloudTurnPermit) (cloudTurnLease, error) {
	return w.cloudTurnLeaseRead(ctx, p, true)
}
func (w *WorkBuddyConnections) cloudTurnBeginPrompt(ctx context.Context, p cloudTurnPermit) error {
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, true, true)
	if e != nil {
		return e
	}
	defer close()
	if row.turn.State != "prepared" || row.promptState != "not_started" {
		return turnChanged()
	}
	before := row.turn.Version
	row.turn.Version++
	row.turn.State = "running"
	row.promptState = "started"
	if e = w.cloudTurnSaveTx(ctx, tx, row, before); e != nil {
		return e
	}
	return tx.Commit()
}
func (w *WorkBuddyConnections) cloudTurnAppend(ctx context.Context, p cloudTurnPermit, event cloudTurnRuntimeEvent) error {
	// New content requires current running/budget authority. Sending an ACK fact
	// does not: it must survive a concurrent pause/revocation in Finish below.
	if event.Kind == "terminal" {
		return w.cloudTurnFinish(ctx, p, event)
	}
	strict := event.Kind != "prompt_sent"
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, true, strict)
	if e != nil {
		return e
	}
	defer close()
	before := row.turn.Version
	if row.promptState == "not_started" {
		return turnChanged()
	}
	switch event.Kind {
	case "entry":
		if event.Entry == nil {
			return turnInvalid()
		}
		entry := *event.Entry
		entry.Sequence = row.entries + 1
		if len(entry.Text) > wc.CloudTurnMaxEntryBytes || row.entries >= wc.CloudTurnMaxEntries || row.bytes+len(entry.Text) > wc.CloudTurnMaxTextBytes {
			return turnLimited()
		}
		if entry.Validate() != nil {
			return turnInvalid()
		}
		if _, e = tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_entries(tenant_id,project_id,turn_id,sequence,payload)VALUES(?,?,?,?,?)`), p.tenant, p.project, p.id, entry.Sequence, stringify(entry)); e != nil {
			return e
		}
		row.entries++
		row.bytes += len(entry.Text)
	case "permission":
		if event.Permission == nil {
			return turnInvalid()
		}
		permission := *event.Permission
		if permission.Version != 1 || permission.State != "pending" || permission.TurnID != p.id || permission.Validate() != nil {
			return turnInvalid()
		}
		var count int
		if e = tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=?`), p.tenant, p.project, p.id).Scan(&count); e != nil {
			return e
		}
		if count >= wc.CloudTurnMaxPermissions {
			return turnLimited()
		}
		if e = w.cloudTurnSavePermissionTx(ctx, tx, row, permission, 0); e != nil {
			return e
		}
		// A permission's tool operation is not completed merely because the
		// user answers it. Keep an otherwise unseen tool pending until its
		// explicit corresponding terminal update is observed.
		var toolCount int
		if e = tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_tools WHERE tenant_id=? AND project_id=? AND turn_id=? AND tool_call_id=?`), p.tenant, p.project, p.id, permission.ToolCallID).Scan(&toolCount); e != nil {
			return e
		}
		if toolCount == 0 {
			if _, e = tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_tools(tenant_id,project_id,turn_id,tool_call_id,state)VALUES(?,?,?,?,?)`), p.tenant, p.project, p.id, permission.ToolCallID, "permission_pending"); e != nil {
				return e
			}
		}
		if row.turn.State != "waiting_permission" {
			row.turn.State = "waiting_permission"
			row.turn.Version++
		}
	case "tool":
		check := wc.CloudTurnEntry{Sequence: 1, Kind: "tool_state", ToolCallID: event.ToolCallID, ToolState: "pending"}
		if check.Validate() != nil {
			return turnInvalid()
		}
		state := event.ToolState
		if state != "pending" && state != "in_progress" && state != "completed" && state != "failed" && state != "unknown" {
			state = "unknown"
		}
		var old string
		found := true
		e = tx.QueryRowContext(ctx, w.s.q(`SELECT state FROM suma_workbuddy_cloud_turn_tools WHERE tenant_id=? AND project_id=? AND turn_id=? AND tool_call_id=?`), p.tenant, p.project, p.id, event.ToolCallID).Scan(&old)
		if errors.Is(e, sql.ErrNoRows) {
			found = false
		} else if e != nil {
			return e
		}
		if !found {
			var count int
			if e = tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_tools WHERE tenant_id=? AND project_id=? AND turn_id=?`), p.tenant, p.project, p.id).Scan(&count); e != nil {
				return e
			}
			if count >= wc.CloudTurnMaxEntries {
				return turnLimited()
			}
		}
		if !event.ToolStart && !found || event.ToolStart && found && old != "permission_pending" || found && (old == "completed" || old == "failed") && state != old {
			state = "unknown"
		}
		if state == "unknown" {
			row.toolsInvalid = 1
		}
		if found {
			_, e = tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_tools SET state=? WHERE tenant_id=? AND project_id=? AND turn_id=? AND tool_call_id=?`), state, p.tenant, p.project, p.id, event.ToolCallID)
		} else {
			_, e = tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_turn_tools(tenant_id,project_id,turn_id,tool_call_id,state)VALUES(?,?,?,?,?)`), p.tenant, p.project, p.id, event.ToolCallID, state)
		}
		if e != nil {
			return e
		}
	case "prompt_sent":
		if row.promptState != "started" && row.promptState != "sent" {
			return turnChanged()
		}
		row.promptState = "sent"
	case "state":
		if event.State != "running" && event.State != "waiting_permission" {
			return turnInvalid()
		}
		if event.State != row.turn.State {
			row.turn.State = event.State
			row.turn.Version++
		}
	default:
		return turnInvalid()
	}
	if row.turn.Version != before {
		e = w.cloudTurnSaveTx(ctx, tx, row, before)
	} else {
		result, err := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turns SET prompt_state=?,entry_count=?,text_bytes=?,tools_invalid=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND lease_owner=? AND lease_fence=?`), row.promptState, row.entries, row.bytes, row.toolsInvalid, p.tenant, p.project, p.id, before, row.leaseOwner, p.fence)
		e = cloudCAS(result, err)
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (w *WorkBuddyConnections) cloudTurnFinish(ctx context.Context, p cloudTurnPermit, event cloudTurnRuntimeEvent) error {
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, false, false)
	if e != nil {
		return e
	}
	defer close()
	if !cloudTurnActive(row.turn.State) {
		return nil
	}
	now, e := w.s.authorityNow(ctx, tx)
	if e != nil {
		return e
	}
	state := event.State
	if state != "completed" && state != "unknown" && state != "rejected" {
		return turnInvalid()
	}
	if state == "rejected" && (!event.KnownNotSent || row.promptState == "sent") {
		state = "unknown"
	}
	if state == "rejected" {
		var count int
		for _, query := range []string{`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_tools WHERE tenant_id=? AND project_id=? AND turn_id=?`, `SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=?`} {
			var n int
			if e = tx.QueryRowContext(ctx, w.s.q(query), p.tenant, p.project, p.id).Scan(&n); e != nil {
				return e
			}
			count += n
		}
		if row.entries != 0 || count != 0 {
			state = "unknown"
		}
	}
	if state == "completed" {
		count := 0
		for _, query := range []string{`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_tools WHERE tenant_id=? AND project_id=? AND turn_id=? AND state NOT IN ('completed','failed')`, `SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=? AND state IN ('pending','prepared','sent','unknown')`, `SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=? AND state IN ('prepared','sent','unknown')`} {
			var n int
			if e = tx.QueryRowContext(ctx, w.s.q(query), p.tenant, p.project, p.id).Scan(&n); e != nil {
				return e
			}
			count += n
		}
		deadline, de := time.Parse(time.RFC3339Nano, row.turn.DeadlineAt)
		until, ue := time.Parse(time.RFC3339Nano, row.leaseUntil)
		source, d, n, se := w.cloudTurnSourceTx(ctx, tx, p.tenant, p.project, p.owner, row.submission)
		if count != 0 || row.toolsInvalid != 0 || row.promptState != "sent" || de != nil || ue != nil || !now.Before(deadline) || !now.Before(until) || se != nil || !cloudTurnSameSource(source, row.turn.Source) || w.cloudTurnQualifiedTx(ctx, tx, p.tenant, p.project, p.owner, d, n) != nil {
			state = "unknown"
		}
	}
	before := row.turn.Version
	row.turn.Version++
	row.turn.State = state
	row.turn.FinishedAt = now.Format(time.RFC3339Nano)
	row.turn.Overdue = false
	row.turn.StopReason = event.StopReason
	row.turn.UsageState = event.UsageState
	row.turn.Usage = event.Usage
	if row.turn.UsageState == "" {
		row.turn.UsageState = "unreported"
		row.turn.Usage = nil
	}
	if state != "completed" {
		row.turn.ErrorCode = event.ErrorCode
		if row.turn.ErrorCode == "" {
			row.turn.ErrorCode = "workbuddy_cloud_turn_unknown"
		}
	} else {
		row.turn.ErrorCode = ""
	}
	row.leaseUntil = now.Format(time.RFC3339Nano)
	if e = w.cloudTurnSaveTx(ctx, tx, row, before); e != nil {
		return e
	}
	return tx.Commit()
}
func (w *WorkBuddyConnections) cloudTurnHeartbeat(ctx context.Context, p cloudTurnPermit) error {
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, true, false)
	if e != nil {
		return e
	}
	defer close()
	now, e := w.s.authorityNow(ctx, tx)
	if e != nil {
		return e
	}
	deadline, _ := time.Parse(time.RFC3339Nano, row.turn.DeadlineAt)
	until := now.Add(cloudTurnLeaseLifetime)
	if until.After(deadline) {
		until = deadline
	}
	result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turns SET lease_until=? WHERE tenant_id=? AND project_id=? AND id=? AND lease_owner=? AND lease_fence=?`), until.Format(time.RFC3339Nano), p.tenant, p.project, p.id, row.leaseOwner, p.fence)
	if e = cloudCAS(result, e); e != nil {
		return e
	}
	return tx.Commit()
}
func (w *WorkBuddyConnections) cloudTurnTakeCommands(ctx context.Context, p cloudTurnPermit) ([]cloudTurnRuntimeCommand, error) {
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, true, false)
	if e != nil {
		return nil, e
	}
	defer close()
	rows, e := tx.QueryContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=? AND state='prepared' ORDER BY id LIMIT 16`), p.tenant, p.project, p.id)
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []cloudTurnRuntimeCommand{}
	for _, id := range ids {
		c, _, e := w.cloudTurnCommandTx(ctx, tx, row, id)
		if e != nil {
			return nil, e
		}
		var permission wc.CloudTurnPermission
		allowed := true
		if c.Kind == "permission" {
			permission, e = w.cloudTurnPermissionTx(ctx, tx, row, c.PermissionID)
			if e != nil {
				return nil, e
			}
			if permission.State != "prepared" {
				return nil, turnChanged()
			}
			for _, option := range permission.Options {
				if option.ID == c.OptionID && option.Kind == "allow_once" {
					source, d, n, se := w.cloudTurnSourceTx(ctx, tx, p.tenant, p.project, p.owner, row.submission)
					allowed = se == nil && cloudTurnSameSource(source, row.turn.Source) && w.cloudTurnQualifiedTx(ctx, tx, p.tenant, p.project, p.owner, d, n) == nil
				}
			}
		}
		nonce, e := cloudTurnToken()
		if e != nil {
			return nil, e
		}
		c.State = "sent"
		if !allowed {
			c.State = "rejected"
			c.ErrorCode = "workbuddy_cloud_turn_authorization_changed"
			nonce = ""
		}
		hash := ""
		if nonce != "" {
			hash = digest(nonce)
		}
		result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_commands SET state=?,send_nonce_sha256=?,payload=? WHERE tenant_id=? AND project_id=? AND turn_id=? AND id=? AND state='prepared'`), c.State, hash, stringify(c), p.tenant, p.project, p.id, id)
		if e = cloudCAS(result, e); e != nil {
			return nil, e
		}
		if c.Kind == "permission" {
			before := permission.Version
			permission.Version++
			permission.State = c.State
			if !allowed {
				permission.State = "pending"
			}
			if e = w.cloudTurnSavePermissionTx(ctx, tx, row, permission, before); e != nil {
				return nil, e
			}
		}
		if allowed {
			command := cloudTurnRuntimeCommand{Command: c, Permission: permission, token: nonce}
			if c.Kind == "cancel" {
				pending, err := tx.QueryContext(ctx, w.s.q(`SELECT id,version FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=? AND state IN ('pending','prepared') ORDER BY id`), p.tenant, p.project, p.id)
				if err != nil {
					return nil, err
				}
				for pending.Next() {
					var permission cloudTurnPermissionVersion
					if err = pending.Scan(&permission.ID, &permission.Version); err != nil {
						break
					}
					command.cancelPermissions = append(command.cancelPermissions, permission)
				}
				if err == nil {
					err = pending.Err()
				}
				pending.Close()
				if err != nil {
					return nil, err
				}
				if len(command.cancelPermissions) > wc.CloudTurnMaxPermissions {
					return nil, turnChanged()
				}
			}
			out = append(out, command)
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}
func (w *WorkBuddyConnections) cloudTurnAckCommand(ctx context.Context, p cloudTurnPermit, command cloudTurnRuntimeCommand, state, code string) error {
	if state != "acknowledged" && state != "unknown" && state != "rejected" {
		return turnInvalid()
	}
	// An already sent command can record its receipt after authorization changes,
	// but this private token grants no further HTTP operation or public content.
	ctx, tx, row, close, e := w.cloudTurnPermitTx(ctx, p, false, false)
	if e != nil {
		return e
	}
	defer close()
	c, nonce, e := w.cloudTurnCommandTx(ctx, tx, row, command.Command.ID)
	if e != nil {
		return e
	}
	if command.token == "" || digest(command.token) != nonce || c.Kind != command.Command.Kind || c.PermissionID != command.Command.PermissionID || c.OptionID != command.Command.OptionID {
		return turnChanged()
	}
	if c.State != "sent" {
		if c.State == state {
			return nil
		}
		return turnChanged()
	}
	c.State = state
	c.ErrorCode = code
	if state == "unknown" && code == "" {
		c.ErrorCode = "workbuddy_cloud_turn_unknown"
	}
	if c.Validate() != nil {
		return turnInvalid()
	}
	result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_commands SET state=?,payload=? WHERE tenant_id=? AND project_id=? AND turn_id=? AND id=? AND state='sent' AND send_nonce_sha256=?`), state, stringify(c), p.tenant, p.project, p.id, c.ID, nonce)
	if e = cloudCAS(result, e); e != nil {
		return e
	}
	if c.Kind == "permission" {
		permission, e := w.cloudTurnPermissionTx(ctx, tx, row, c.PermissionID)
		if e != nil {
			return e
		}
		before := permission.Version
		permission.Version++
		permission.State = state
		if e = w.cloudTurnSavePermissionTx(ctx, tx, row, permission, before); e != nil {
			return e
		}
		if state == "acknowledged" && row.turn.State == "waiting_permission" {
			var pending int
			if e = tx.QueryRowContext(ctx, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turn_permissions WHERE tenant_id=? AND project_id=? AND turn_id=? AND state IN ('pending','prepared','sent','unknown')`), p.tenant, p.project, p.id).Scan(&pending); e != nil {
				return e
			}
			if pending == 0 {
				before := row.turn.Version
				row.turn.State = "running"
				row.turn.Version++
				if e = w.cloudTurnSaveTx(ctx, tx, row, before); e != nil {
					return e
				}
			}
		}
	}
	if c.Kind == "cancel" && state == "acknowledged" {
		if e = w.cloudTurnRetireCancelledPermissionsTx(ctx, tx, row, command.cancelPermissions); e != nil {
			return e
		}
	}
	return tx.Commit()
}

// The SDK snapshots permission refs before the Cancel HTTP request. Its ACK
// proves only that original subset, never a permission arriving through Next
// while the request was in flight. A changed version is left unconfirmed.
func (w *WorkBuddyConnections) cloudTurnRetireCancelledPermissionsTx(ctx context.Context, tx *sql.Tx, row cloudTurnRow, scope []cloudTurnPermissionVersion) error {
	if len(scope) > wc.CloudTurnMaxPermissions {
		return turnInvalid()
	}
	covered := map[string]bool{}
	seen := map[string]bool{}
	for _, item := range scope {
		if !handoffID(item.ID, false) || item.Version < 1 || item.Version > wc.MaxVersion || seen[item.ID] {
			return turnInvalid()
		}
		seen[item.ID] = true
		covered[item.ID] = true
		permission, e := w.cloudTurnPermissionTx(ctx, tx, row, item.ID)
		if e != nil {
			return e
		}
		if permission.Version != item.Version || permission.State != "pending" && permission.State != "prepared" {
			delete(covered, item.ID)
			continue
		}
		before := permission.Version
		permission.Version++
		permission.State = "acknowledged"
		if e = w.cloudTurnSavePermissionTx(ctx, tx, row, permission, before); e != nil {
			return e
		}
	}
	if len(covered) == 0 {
		return nil
	}
	// Commands carry their exact permission ID in the closed public payload.
	// Read the bounded prepared inbox, but retire only the verified scope above;
	// sent/unknown commands and new permissions are never swept by cancel ACK.
	rows, e := tx.QueryContext(ctx, w.s.q(`SELECT id FROM suma_workbuddy_cloud_turn_commands WHERE tenant_id=? AND project_id=? AND turn_id=? AND kind='permission' AND state='prepared'`), row.tenant, row.project, row.turn.ID)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		ids = append(ids, id)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		c, _, e := w.cloudTurnCommandTx(ctx, tx, row, id)
		if e != nil {
			return e
		}
		if !covered[c.PermissionID] {
			continue
		}
		c.State = "rejected"
		c.ErrorCode = "workbuddy_cloud_turn_interrupted"
		result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_turn_commands SET state='rejected',payload=? WHERE tenant_id=? AND project_id=? AND turn_id=? AND id=? AND state='prepared'`), stringify(c), row.tenant, row.project, row.turn.ID, c.ID)
		if e = cloudCAS(result, e); e != nil {
			return e
		}
	}
	return nil
}
