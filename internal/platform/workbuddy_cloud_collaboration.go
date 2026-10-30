package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/workbuddy"
	"github.com/ai-mada/suma/internal/workbuddyacp"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const cloudHistoryLifetime = 30 * time.Second

var errCloudHistoryLimit = errors.New("cloud history local limit")

// Current archive authority deliberately has no OAuth or machine lease check.
// Fresh external admission additionally uses the existing current human guard.
func (w *WorkBuddyConnections) cloudHistorySourceTx(ctx context.Context, a *authorizedCommand, id string, external bool) (wc.CloudCollaborationSource, *workbuddy.Client, error) {
	var row cloudSubmissionRow
	var client *workbuddy.Client
	var err error
	if external {
		row, client, err = w.cloudObserver(ctx, a, id)
	} else {
		row, err = w.cloudSubmissionTx(ctx, a.tx, a.target.tenantID, a.target.projectID, id, true)
	}
	if err != nil {
		return wc.CloudCollaborationSource{}, nil, err
	}
	d, err := w.cloudDelegationTx(ctx, a.tx, a.target.tenantID, a.target.projectID, row.DelegationID, true)
	if err != nil {
		return wc.CloudCollaborationSource{}, nil, err
	}
	if d.OwnerID != a.currentPrincipal.ID {
		return wc.CloudCollaborationSource{}, nil, missing()
	}
	if !external {
		n, e := w.cloudNodeTx(ctx, a.tx, a.target.tenantID, a.target.projectID, row.RunID, row.NodeID)
		if e != nil {
			return wc.CloudCollaborationSource{}, nil, e
		}
		if row.State != "task_known" || row.Task == nil || d.RunID != row.RunID || d.NodeID != row.NodeID || n.specHash != d.NodeSpecSHA256 || n.requestHash != d.RequestSHA256 || row.RequestSHA256 != d.RequestSHA256 || n.binding.ConnectionID != d.ConnectionID || n.binding.GrantGeneration != d.GrantGeneration {
			return wc.CloudCollaborationSource{}, nil, cloudConflict()
		}
	}
	source := wc.CloudCollaborationSource{ProjectID: a.target.projectID, RunID: row.RunID, NodeID: row.NodeID, SubmissionID: row.ID, SubmissionVersion: row.Version, TaskID: row.Task.ID, NodeSpecSHA256: d.NodeSpecSHA256, RequestSHA256: d.RequestSHA256}
	if source.Validate() != nil {
		return source, nil, cloudConflict()
	}
	return source, client, nil
}

func (w *WorkBuddyConnections) CloudCollaboration(ctx context.Context, actor Actor, project, id string) (wc.CloudCollaborationView, error) {
	if w == nil || w.s == nil || !handoffID(id, false) {
		return wc.CloudCollaborationView{}, cloudInvalid()
	}
	ctx, a, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectRead)
	if err != nil {
		return wc.CloudCollaborationView{}, err
	}
	defer close()
	source, _, err := w.cloudHistorySourceTx(ctx, a, id, false)
	if err != nil {
		return wc.CloudCollaborationView{}, err
	}
	head, err := w.cloudHistoryHeadTx(ctx, a, source)
	if err != nil {
		return wc.CloudCollaborationView{}, err
	}
	clock, err := w.s.authorityNow(ctx, a.tx)
	if err != nil {
		return wc.CloudCollaborationView{}, err
	}
	out := wc.CloudCollaborationView{Source: source, Version: head.version, AllowedActions: []string{}}
	for _, item := range []struct {
		id     string
		target **wc.CloudHistoryLoad
	}{{head.latest, &out.LatestLoad}, {head.last, &out.LastLoaded}} {
		if item.id == "" {
			continue
		}
		row, e := w.cloudHistoryLoadTx(ctx, a, item.id)
		if e != nil {
			return wc.CloudCollaborationView{}, e
		}
		if !cloudHistorySameSource(source, row.load.Source) {
			return wc.CloudCollaborationView{}, cloudConflict()
		}
		load := cloudHistoryOverdue(row.load, clock)
		*item.target = &load
	}
	if head.version <= wc.MaxVersion-2 && (out.LatestLoad == nil || out.LatestLoad.State != "loading" || out.LatestLoad.Overdue) {
		if _, e := w.s.currentProjectMembershipLockedTx(ctx, a.tx, a.target.tenantID, a.currentPrincipal.ID, project, true, false); e == nil {
			sub, e := w.cloudSubmissionTx(ctx, a.tx, a.target.tenantID, project, id, true)
			if e == nil {
				d, e := w.cloudDelegationTx(ctx, a.tx, a.target.tenantID, project, sub.DelegationID, true)
				if e == nil {
					if _, e = w.cloudGrantMetadataTx(ctx, a.tx, a.target.tenantID, project, d.OwnerID, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256, true); e == nil {
						out.AllowedActions = append(out.AllowedActions, "load_history")
					}
				}
			}
		}
	}
	return out, nil
}

func (w *WorkBuddyConnections) CloudHistory(ctx context.Context, actor Actor, project, id string, after, limit int) (wc.CloudHistoryPage, error) {
	if limit == 0 {
		limit = 50
	}
	if w == nil || w.s == nil || !handoffID(id, false) || after < 0 || after > wc.CloudHistoryMaxEntries || limit < 1 || limit > 100 {
		return wc.CloudHistoryPage{}, cloudInvalid()
	}
	ctx, a, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectRead)
	if err != nil {
		return wc.CloudHistoryPage{}, err
	}
	defer close()
	row, err := w.cloudHistoryLoadTx(ctx, a, id)
	if err != nil {
		return wc.CloudHistoryPage{}, err
	}
	source, _, err := w.cloudHistorySourceTx(ctx, a, row.load.Source.SubmissionID, false)
	if err != nil {
		return wc.CloudHistoryPage{}, err
	}
	if !cloudHistorySameSource(source, row.load.Source) {
		return wc.CloudHistoryPage{}, cloudConflict()
	}
	clock, err := w.s.authorityNow(ctx, a.tx)
	if err != nil {
		return wc.CloudHistoryPage{}, err
	}
	rows, err := a.tx.QueryContext(ctx, w.s.q(`SELECT sequence,kind,text FROM suma_workbuddy_cloud_history_entries WHERE tenant_id=? AND project_id=? AND load_id=? AND sequence>? ORDER BY sequence LIMIT ?`), a.target.tenantID, project, id, after, limit)
	if err != nil {
		return wc.CloudHistoryPage{}, err
	}
	defer rows.Close()
	out := wc.CloudHistoryPage{Load: cloudHistoryOverdue(row.load, clock), Items: []wc.CloudHistoryEntry{}, Total: row.load.EntryCount, NextAfter: after}
	for rows.Next() {
		var entry wc.CloudHistoryEntry
		if err = rows.Scan(&entry.Sequence, &entry.Kind, &entry.Text); err != nil {
			return wc.CloudHistoryPage{}, err
		}
		if entry.Validate() != nil || entry.Sequence > out.Total {
			return wc.CloudHistoryPage{}, cloudConflict()
		}
		out.Items = append(out.Items, entry)
		out.NextAfter = entry.Sequence
	}
	if err = rows.Err(); err != nil {
		return wc.CloudHistoryPage{}, err
	}
	out.HasMore = out.NextAfter < out.Total
	return out, nil
}

// LoadCloudHistory is synchronous only for this explicit browser command. A
// committed load/receipt grants one in-memory attachment; storage never grants
// a restarted caller permission to attach again.
func (w *WorkBuddyConnections) LoadCloudHistory(ctx context.Context, actor Actor, project, id string, req wc.CloudHistoryLoadRequest) (wc.CloudHistoryLoad, error) {
	if w == nil || w.s == nil || req.ProjectID != project || req.Validate() != nil || !handoffID(id, false) {
		return wc.CloudHistoryLoad{}, cloudInvalid()
	}
	var prepared cloudHistoryRow
	var client *workbuddy.Client
	var admittedAt time.Time
	replay := false
	body := struct {
		Kind, ID string
		Request  wc.CloudHistoryLoadRequest
	}{"workbuddy_cloud_history_load", id, req}
	err := func() error {
		bounded, a, close, e := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
		if e != nil {
			return e
		}
		defer close()
		source, c, e := w.cloudHistorySourceTx(bounded, a, id, true)
		if e != nil {
			return e
		}
		hash, done, e := w.s.commandReceipt(bounded, a, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if done {
			var loadID string
			if json.Unmarshal([]byte(hash), &loadID) != nil {
				return cloudConflict()
			}
			prepared, e = w.cloudHistoryLoadTx(bounded, a, loadID)
			if e != nil {
				return e
			}
			if !cloudHistorySameSource(source, prepared.load.Source) {
				return cloudConflict()
			}
			clock, e := w.s.authorityNow(bounded, a.tx)
			if e != nil {
				return e
			}
			prepared.load = cloudHistoryOverdue(prepared.load, clock)
			replay = true
			return nil
		}
		head, e := w.cloudHistoryHeadTx(bounded, a, source)
		if e != nil {
			return e
		}
		if source.SubmissionVersion != req.ExpectedSubmissionVersion || head.version != req.ExpectedVersion || head.version > wc.MaxVersion-2 {
			return cloudConflict()
		}
		clock, e := w.s.authorityNow(bounded, a.tx)
		if e != nil {
			return e
		}
		admittedAt = time.Now()
		if head.latest != "" {
			old, e := w.cloudHistoryLoadTx(bounded, a, head.latest)
			if e != nil {
				return e
			}
			if !cloudHistorySameSource(source, old.load.Source) {
				return cloudConflict()
			}
			if old.load.State == "loading" {
				if !cloudHistoryOverdue(old.load, clock).Overdue {
					return cloudConflict()
				}
				before := old.load.Version
				old.load.Version++
				old.load.State = "interrupted"
				old.load.FinishedAt = automationStamp(clock)
				old.load.ErrorCode = "workbuddy_cloud_history_interrupted"
				if e = w.cloudHistorySaveLoadTx(bounded, a, old, before); e != nil {
					return e
				}
			}
		}
		rows, e := a.tx.QueryContext(bounded, w.s.q(`SELECT deadline_at FROM suma_workbuddy_cloud_history_loads WHERE tenant_id=? AND project_id=? AND state='loading'`), a.target.tenantID, project)
		if e != nil {
			return e
		}
		active := 0
		for rows.Next() {
			var deadline string
			if e = rows.Scan(&deadline); e != nil {
				rows.Close()
				return e
			}
			end, e := time.Parse(time.RFC3339Nano, deadline)
			if e != nil {
				rows.Close()
				return cloudConflict()
			}
			if clock.Before(end) {
				active++
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if active >= 4 {
			return cloudConflict()
		}
		prepared = cloudHistoryRow{tenant: a.target.tenantID, owner: a.currentPrincipal.ID, operation: req.IdempotencyKey, requestHash: hash, load: wc.CloudHistoryLoad{ID: newID(), Version: 1, Source: source, OperationID: req.IdempotencyKey, HeadVersionAtStart: req.ExpectedVersion, State: "loading", Coverage: "unverified", StartedAt: automationStamp(clock), DeadlineAt: automationStamp(clock.Add(cloudHistoryLifetime))}}
		if e = w.cloudHistorySaveLoadTx(bounded, a, prepared, 0); e != nil {
			return e
		}
		before := head.version
		head.version++
		head.latest = prepared.load.ID
		if e = w.cloudHistorySaveHeadTx(bounded, a, source, head, before); e != nil {
			return e
		}
		if e = w.s.saveCommandReceipt(bounded, a, req.IdempotencyKey, hash, prepared.load.ID); e != nil {
			return e
		}
		if e = w.s.commandActivity(bounded, a, "workbuddy_cloud_history_started", prepared.load.ID, "WorkBuddy cloud history load started"); e != nil {
			return e
		}
		if e = a.tx.Commit(); e != nil {
			return e
		}
		client = c
		return nil
	}()
	if err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	if replay {
		return prepared.load, nil
	}
	// DB time decides the persisted deadline. A monotonic local timeout bounds
	// this network operation without comparing clocks from different hosts.
	bounded, cancel := context.WithTimeout(ctx, cloudHistoryLifetime-time.Since(admittedAt))
	defer cancel()
	access, providerErr := client.GetSessionAccess(bounded, prepared.load.Source.TaskID)
	if providerErr == nil && (!access.Available || access.Task.ID != prepared.load.Source.TaskID) {
		providerErr = errors.New("history unavailable")
	}
	var collectErr error
	if providerErr == nil {
		// Reauthorize between the ticket GET and connection creation as well.
		providerErr = w.cloudHistoryCheck(bounded, actor, project, prepared.load.ID, prepared.load.Source)
		if providerErr == nil {
			session, e := workbuddyacp.Attach(bounded, access, w.app.HTTPClient, workbuddyacp.Setup{InitializeID: newID(), LoadID: newID(), OnHistory: func(packet workbuddyacp.Packet) error {
				if collectErr != nil {
					return collectErr
				}
				entry := cloudHistoryProject(packet)
				collectErr = w.cloudHistoryAppend(bounded, actor, project, prepared.load.ID, prepared.load.Source, entry)
				return collectErr
			}})
			providerErr = e
			if session != nil {
				_ = session.Close()
			}
		}
	}
	state, code := "loaded", ""
	if providerErr != nil {
		state, code = "interrupted", "workbuddy_cloud_history_interrupted"
		if errors.Is(collectErr, errCloudHistoryLimit) {
			state, code = "limited", "workbuddy_cloud_history_limited"
		} else if access.Task.ID == "" || !access.Available {
			code = "workbuddy_cloud_history_unavailable"
		}
	}
	// A cancelled local waiter is not a remote cancellation. Current authority is
	// re-established for a short local status closure; revoked callers get no data.
	finish, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelFinish()
	return w.cloudHistoryFinish(finish, actor, project, prepared.load.ID, prepared.load.Source, state, code)
}

func (w *WorkBuddyConnections) cloudHistoryAuthorizedLoad(ctx context.Context, actor Actor, project, id string, source wc.CloudCollaborationSource) (context.Context, *authorizedCommand, func(), cloudHistoryRow, cloudHistoryHead, time.Time, error) {
	ctx, a, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return ctx, nil, func() {}, cloudHistoryRow{}, cloudHistoryHead{}, time.Time{}, err
	}
	fail := func(e error) (context.Context, *authorizedCommand, func(), cloudHistoryRow, cloudHistoryHead, time.Time, error) {
		close()
		return ctx, nil, func() {}, cloudHistoryRow{}, cloudHistoryHead{}, time.Time{}, e
	}
	current, _, err := w.cloudHistorySourceTx(ctx, a, source.SubmissionID, true)
	if err != nil {
		return fail(err)
	}
	if !cloudHistorySameSource(current, source) {
		return fail(cloudConflict())
	}
	row, err := w.cloudHistoryLoadTx(ctx, a, id)
	if err != nil {
		return fail(err)
	}
	if !cloudHistorySameSource(row.load.Source, source) {
		return fail(cloudConflict())
	}
	head, err := w.cloudHistoryHeadTx(ctx, a, current)
	if err != nil {
		return fail(err)
	}
	if head.latest != id || row.load.State != "loading" {
		return fail(cloudConflict())
	}
	clock, err := w.s.authorityNow(ctx, a.tx)
	if err != nil {
		return fail(err)
	}
	return ctx, a, close, row, head, clock, nil
}
func (w *WorkBuddyConnections) cloudHistoryCheck(ctx context.Context, actor Actor, project, id string, source wc.CloudCollaborationSource) error {
	bounded, _, close, row, _, clock, err := w.cloudHistoryAuthorizedLoad(ctx, actor, project, id, source)
	_ = bounded
	if err != nil {
		return err
	}
	defer close()
	if cloudHistoryOverdue(row.load, clock).Overdue {
		return context.DeadlineExceeded
	}
	return nil
}
func (w *WorkBuddyConnections) cloudHistoryAppend(ctx context.Context, actor Actor, project, id string, source wc.CloudCollaborationSource, entry wc.CloudHistoryEntry) error {
	ctx, a, close, row, _, clock, err := w.cloudHistoryAuthorizedLoad(ctx, actor, project, id, source)
	if err != nil {
		return err
	}
	defer close()
	if cloudHistoryOverdue(row.load, clock).Overdue {
		return context.DeadlineExceeded
	}
	if len(entry.Text) > wc.CloudHistoryMaxEntryBytes || row.load.EntryCount >= wc.CloudHistoryMaxEntries || row.load.TextBytes+len(entry.Text) > wc.CloudHistoryMaxTextBytes {
		return errCloudHistoryLimit
	}
	entry.Sequence = row.load.EntryCount + 1
	if entry.Validate() != nil {
		return errCloudHistoryLimit
	}
	if _, err = a.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_history_entries(tenant_id,project_id,load_id,sequence,kind,text)VALUES(?,?,?,?,?,?)`), a.target.tenantID, project, id, entry.Sequence, entry.Kind, entry.Text); err != nil {
		return err
	}
	before := row.load.Version
	row.load.Version++
	row.load.EntryCount++
	row.load.TextBytes += len(entry.Text)
	if entry.Kind == "unsupported" {
		row.load.UnsupportedCount++
	}
	if err = w.cloudHistorySaveLoadTx(ctx, a, row, before); err != nil {
		return err
	}
	return a.tx.Commit()
}
func (w *WorkBuddyConnections) cloudHistoryFinish(ctx context.Context, actor Actor, project, id string, source wc.CloudCollaborationSource, state, code string) (wc.CloudHistoryLoad, error) {
	ctx, a, close, row, head, clock, err := w.cloudHistoryAuthorizedLoad(ctx, actor, project, id, source)
	if err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	defer close()
	if cloudHistoryOverdue(row.load, clock).Overdue {
		state, code = "interrupted", "workbuddy_cloud_history_interrupted"
	}
	before := row.load.Version
	row.load.Version++
	row.load.State = state
	row.load.ErrorCode = code
	row.load.FinishedAt = automationStamp(clock)
	if err = w.cloudHistorySaveLoadTx(ctx, a, row, before); err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	previous := head.version
	head.version++
	if state == "loaded" {
		head.last = id
	}
	if err = w.cloudHistorySaveHeadTx(ctx, a, row.load.Source, head, previous); err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	if err = w.s.commandActivity(ctx, a, "workbuddy_cloud_history_finished", id, "WorkBuddy cloud history load finished"); err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	if err = a.tx.Commit(); err != nil {
		return wc.CloudHistoryLoad{}, err
	}
	return row.load, nil
}

// The projection reads only closed display fields after the protocol decoder.
// It never stores raw packets, thought text, tool content/arguments, or URIs.
func cloudHistoryProject(packet workbuddyacp.Packet) wc.CloudHistoryEntry {
	entry := wc.CloudHistoryEntry{Kind: "unsupported"}
	if packet.Kind != "session_update" || packet.Update == nil {
		return entry
	}
	var notification struct {
		Update struct {
			Tag           string                      `json:"sessionUpdate"`
			Content       struct{ Type, Text string } `json:"content"`
			Title, Status string
			Entries       []struct{ Content, Status string }
		} `json:"update"`
	}
	// Marshal the validated SDK value, then decode only the allowlisted display
	// projection; no original frame is copied into storage.
	raw, err := json.Marshal(packet.Update)
	if err != nil || json.Unmarshal(raw, &notification) != nil {
		return entry
	}
	update := notification.Update
	switch update.Tag {
	case "user_message_chunk", "agent_message_chunk":
		if update.Content.Type != "text" || !utf8.ValidString(update.Content.Text) || strings.ContainsRune(update.Content.Text, 0) {
			return entry
		}
		entry.Kind = "user_text"
		if update.Tag == "agent_message_chunk" {
			entry.Kind = "assistant_text"
		}
		entry.Text = update.Content.Text
	case "tool_call", "tool_call_update":
		entry.Kind = "tool_summary"
		entry.Text = strings.TrimSpace(update.Title + " " + update.Status)
	case "plan":
		entry.Kind = "plan_summary"
		var lines []string
		for _, item := range update.Entries {
			lines = append(lines, strings.TrimSpace(item.Content+" "+item.Status))
		}
		entry.Text = strings.Join(lines, "\n")
	}
	if len(entry.Text) <= wc.CloudHistoryMaxEntryBytes {
		check := entry
		check.Sequence = 1
		if check.Validate() != nil {
			return wc.CloudHistoryEntry{Kind: "unsupported"}
		}
	}
	return entry
}
