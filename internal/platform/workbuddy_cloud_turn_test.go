package platform

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
	"testing"
)

func cloudTurnFixture(t *testing.T) (*cloudFixture, wc.CloudSubmission, wc.CloudTurnPromptRequest, *[]cloudTurnPermit) {
	t.Helper()
	h := newCloudFixture(t, nil)
	sub := createCloudFixture(t, h)
	a, r := cloudResultFixture(t, h)
	r.WorkBuddyCloudWait.SubmissionID = sub.ID
	r.WorkBuddyCloudWait.SubmissionVersion = sub.Version
	if e := completeCloudFixture(t, h, a, r); e != nil {
		t.Fatal(e)
	}
	if e := h.f.s.InitializeWorkBuddyCloudTurns(context.Background()); e != nil {
		t.Fatal(e)
	}
	var version int
	if e := h.f.s.DB.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, sub.RunID).Scan(&version); e != nil {
		t.Fatal(e)
	}
	permits := []cloudTurnPermit{}
	h.w.turnRuntime = func(p cloudTurnPermit) { permits = append(permits, p) }
	req := wc.CloudTurnPromptRequest{ProjectID: h.f.project, ExpectedSubmissionVersion: sub.Version, ExpectedRunVersion: version, ExpectedVersion: 0, Prompt: "explicit next goal", IdempotencyKey: newID()}
	return h, sub, req, &permits
}

func cloudTurnPrepared(t *testing.T) (*cloudFixture, wc.CloudSubmission, wc.CloudTurn, cloudTurnPermit) {
	t.Helper()
	h, sub, req, permits := cloudTurnFixture(t)
	turn, e := h.w.PromptCloudTurn(context.Background(), h.f.actor, h.f.project, sub.ID, req)
	if e != nil || len(*permits) != 1 {
		t.Fatal(e)
	}
	return h, sub, turn, (*permits)[0]
}
func cloudTurnStart(t *testing.T, h *cloudFixture, p cloudTurnPermit) {
	t.Helper()
	if e := h.w.cloudTurnBeginPrompt(context.Background(), p); e != nil {
		t.Fatal(e)
	}
	if e := h.w.cloudTurnAppend(context.Background(), p, cloudTurnRuntimeEvent{Kind: "prompt_sent"}); e != nil {
		t.Fatal(e)
	}
}
func cloudTurnPendingPermission(t *testing.T, h *cloudFixture, p cloudTurnPermit, approve bool) wc.CloudTurnPermission {
	t.Helper()
	details := "exact pending tool details"
	permission := wc.CloudTurnPermission{ID: newID(), TurnID: p.id, Version: 1, ToolCallID: "tool:权限", Title: "Tool review", Details: details, DetailsSHA256: digest(details), ApproveAllowed: approve, State: "pending", Options: []wc.CloudTurnPermissionOption{{ID: "option:一次", Name: "Allow once", Kind: "allow_once"}, {ID: "reject:一次", Name: "Reject once", Kind: "reject_once"}, {ID: "always", Name: "Allow always", Kind: "allow_always"}}}
	if e := h.w.cloudTurnAppend(context.Background(), p, cloudTurnRuntimeEvent{Kind: "permission", Permission: &permission}); e != nil {
		t.Fatal(e)
	}
	return permission
}

func TestCloudTurnCompletionRequiresOriginalACKAndCompleteLedger(t *testing.T) {
	for _, mode := range []string{"complete", "unsent", "open_tool", "orphan_tool", "permission", "cancel_scope_race", "permission_ack_continues"} {
		t.Run(mode, func(t *testing.T) {
			h, _, turn, p := cloudTurnPrepared(t)
			ctx := context.Background()
			if mode == "unsent" {
				if e := h.w.cloudTurnBeginPrompt(ctx, p); e != nil {
					t.Fatal(e)
				}
			} else {
				cloudTurnStart(t, h, p)
			}
			if mode == "permission" {
				cloudTurnPendingPermission(t, h, p, true)
			}
			if mode == "permission_ack_continues" {
				first := cloudTurnPendingPermission(t, h, p, true)
				second := first
				second.ID, second.ToolCallID = newID(), "tool:second"
				if e := h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "permission", Permission: &second}); e != nil {
					t.Fatal(e)
				}
				before, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
				if e != nil {
					t.Fatal(e)
				}
				answer := func(permission wc.CloudTurnPermission) wc.CloudTurn {
					t.Helper()
					if _, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, wc.CloudTurnPermissionReplyRequest{ProjectID: h.f.project, ExpectedVersion: permission.Version, OptionID: "option:一次", IdempotencyKey: newID()}); e != nil {
						t.Fatal(e)
					}
					commands, e := h.w.cloudTurnTakeCommands(ctx, p)
					if e != nil || len(commands) != 1 {
						t.Fatal("permission take", e)
					}
					if e = h.w.cloudTurnAckCommand(ctx, p, commands[0], "acknowledged", ""); e != nil {
						t.Fatal(e)
					}
					page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
					if e != nil {
						t.Fatal(e)
					}
					return page.Turn
				}
				partial := answer(first)
				if partial.State != "waiting_permission" || partial.Version != before.Turn.Version {
					t.Fatal("remaining permission was hidden")
				}
				resumed := answer(second)
				if resumed.State != "running" || resumed.Version != partial.Version+1 {
					t.Fatal("last exact permission ACK did not resume semantic state", resumed.State, resumed.Version)
				}
				for _, id := range []string{first.ToolCallID, second.ToolCallID} {
					if e := h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "tool", ToolCallID: id, ToolState: "completed"}); e != nil {
						t.Fatal(e)
					}
				}
			}
			if mode == "cancel_scope_race" {
				first := cloudTurnPendingPermission(t, h, p, true)
				page, err := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = h.w.CancelCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, wc.CloudTurnControlRequest{ProjectID: h.f.project, ExpectedVersion: page.Turn.Version, IdempotencyKey: newID()}); err != nil {
					t.Fatal(err)
				}
				commands, err := h.w.cloudTurnTakeCommands(ctx, p)
				if err != nil || len(commands) != 1 {
					t.Fatal("cancel take", err)
				}
				// Session.Cancel snapshots A before its HTTP ACK. Concurrent Next
				// then records B; a later cancel ACK cannot answer that new RPC.
				commands[0].cancelPermissions = []cloudTurnPermissionVersion{{ID: first.ID, Version: first.Version}}
				second := first
				second.ID, second.ToolCallID = newID(), "tool:later"
				if err = h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "permission", Permission: &second}); err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{first.ToolCallID, second.ToolCallID} {
					if err = h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "tool", ToolCallID: id, ToolState: "completed"}); err != nil {
						t.Fatal(err)
					}
				}
				if err = h.w.cloudTurnAckCommand(ctx, p, commands[0], "acknowledged", ""); err != nil {
					t.Fatal(err)
				}
				page, err = h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
				if err != nil {
					t.Fatal(err)
				}
				if page.Turn.State != "waiting_permission" {
					t.Fatal("cancel ACK inferred resumed state")
				}
				for _, permission := range page.Permissions {
					if permission.ID == first.ID && permission.State != "acknowledged" {
						t.Fatal("original cancel scope not retired")
					}
					if permission.ID == second.ID && permission.State != "pending" {
						t.Fatal("cancel ACK answered permission outside original snapshot")
					}
				}
			}
			if mode == "complete" || mode == "open_tool" || mode == "orphan_tool" {
				if e := h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "tool", ToolCallID: "tool:1", ToolState: "in_progress", ToolStart: mode != "orphan_tool"}); e != nil {
					t.Fatal(e)
				}
				if mode != "open_tool" {
					if e := h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "tool", ToolCallID: "tool:1", ToolState: "completed"}); e != nil {
						t.Fatal(e)
					}
				}
			}
			if e := h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "completed", StopReason: "end_turn", UsageState: "unreported"}); e != nil {
				t.Fatal(e)
			}
			page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
			if e != nil {
				t.Fatal(e)
			}
			want := "unknown"
			if mode == "complete" || mode == "permission_ack_continues" {
				want = "completed"
			}
			if page.Turn.State != want || page.Turn.Usage != nil || page.Turn.UsageState != "unreported" {
				t.Fatal("false completion", page.Turn.State)
			}
			run, e := h.f.e.GetRun(ctx, h.f.user.TenantID, h.f.project, turn.Source.Collaboration.RunID)
			if e != nil || run.State != "running" || run.Nodes[0].State != "waiting" {
				t.Fatal("conversation promoted DAG", e)
			}
		})
	}
}

func TestCloudTurnPermissionInboxIsCurrentExactOnceAndCrossReplica(t *testing.T) {
	h, _, turn, p := cloudTurnPrepared(t)
	ctx := context.Background()
	cloudTurnStart(t, h, p)
	permission := cloudTurnPendingPermission(t, h, p, true)
	req := wc.CloudTurnPermissionReplyRequest{ProjectID: h.f.project, ExpectedVersion: 1, OptionID: "always", IdempotencyKey: newID()}
	if _, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, req); e == nil {
		t.Fatal("always authorization accepted")
	}
	req.OptionID = "option:一次"
	req.IdempotencyKey = newID()
	replica := *h.w
	c, e := replica.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, req)
	if e != nil || c.State != "prepared" {
		t.Fatal(e)
	}
	again, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, req)
	if e != nil || again.ID != c.ID {
		t.Fatal("original permission operation lost", e)
	}
	commands, e := h.w.cloudTurnTakeCommands(ctx, p)
	if e != nil || len(commands) != 1 || commands[0].Command.ID != c.ID {
		t.Fatal("owner failed to take other replica inbox", e)
	}
	empty, e := h.w.cloudTurnTakeCommands(ctx, p)
	if e != nil || len(empty) != 0 {
		t.Fatal("sent command retaken", e)
	}
	forged := commands[0]
	forged.token = "different"
	if e = h.w.cloudTurnAckCommand(ctx, p, forged, "acknowledged", ""); e == nil {
		t.Fatal("forged send permit ACK accepted")
	}
	if e = h.w.cloudTurnAckCommand(ctx, p, commands[0], "unknown", ""); e != nil {
		t.Fatal(e)
	}
	statusPage, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
	if e != nil || statusPage.Turn.State != "waiting_permission" {
		t.Fatal("unknown permission inferred resumed state", e)
	}
	if e = h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "completed", StopReason: "end_turn"}); e != nil {
		t.Fatal(e)
	}
	page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
	if e != nil || page.Turn.State != "unknown" || page.Permissions[0].State != "unknown" {
		t.Fatal("unknown control erased", e)
	}
}

func TestCloudTurnCancelAndRejectRemainAvailableUnderEnforceAndPause(t *testing.T) {
	h, _, turn, p := cloudTurnPrepared(t)
	ctx := context.Background()
	cloudTurnStart(t, h, p)
	permission := cloudTurnPendingPermission(t, h, p, false)
	if _, e := h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: "project", ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "No new execution", IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	if _, e := h.f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, turn.Source.Collaboration.RunID); e != nil {
		t.Fatal(e)
	}
	if _, e := h.w.cloudTurnCheck(ctx, p); e != nil {
		t.Fatal("risk reducing channel blocked", e)
	}
	if _, e := h.w.cloudTurnCheckWork(ctx, p); e == nil {
		t.Fatal("paused/enforced new work allowed")
	}
	allow := wc.CloudTurnPermissionReplyRequest{ProjectID: h.f.project, ExpectedVersion: 1, OptionID: "option:一次", IdempotencyKey: newID()}
	if _, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, allow); e == nil {
		t.Fatal("redacted approval accepted")
	}
	allow.OptionID = "reject:一次"
	allow.IdempotencyKey = newID()
	if _, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, allow); e != nil {
		t.Fatal("reject blocked by budget", e)
	}
	page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.w.CancelCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, wc.CloudTurnControlRequest{ProjectID: h.f.project, ExpectedVersion: page.Turn.Version, IdempotencyKey: newID()}); e != nil {
		t.Fatal("cancel blocked by budget", e)
	}
	commands, e := h.w.cloudTurnTakeCommands(ctx, p)
	if e != nil || len(commands) != 2 {
		t.Fatal("risk reducing inbox blocked", e, len(commands))
	}
}

func TestCloudTurnRevocationStopsContentButOriginalPermitRecordsUnknown(t *testing.T) {
	h, _, turn, p := cloudTurnPrepared(t)
	ctx := context.Background()
	cloudTurnStart(t, h, p)
	if _, e := h.f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET state='disconnected' WHERE id=?`, h.delegation.ConnectionID); e != nil {
		t.Fatal(e)
	}
	if e := h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "entry", Entry: &wc.CloudTurnEntry{Kind: "assistant_text", Text: "must not publish"}}); e == nil {
		t.Fatal("revoked grant published")
	}
	if e := h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "completed", StopReason: "end_turn"}); e != nil {
		t.Fatal("lost incurred terminal fact", e)
	}
	page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
	if e != nil || page.Turn.State != "unknown" || len(page.Items) != 0 {
		t.Fatal("revocation fabricated completion", e)
	}
}

func TestCloudTurnExpiredLeaseCannotResurrectAndReconcileFencesLateWrites(t *testing.T) {
	h, sub, turn, p := cloudTurnPrepared(t)
	ctx := context.Background()
	if _, e := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_turns SET lease_until='2000-01-01T00:00:00Z' WHERE id=?`, turn.ID); e != nil {
		t.Fatal(e)
	}
	{
		view, e := h.w.CloudTurns(ctx, h.f.actor, h.f.project, sub.ID)
		if e != nil || view.LatestTurn == nil || !view.LatestTurn.Overdue {
			t.Fatal("expired lease missing from view projection", e)
		}
		page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
		if e != nil || !page.Turn.Overdue {
			t.Fatal("expired lease missing from page projection", e)
		}
		reconcile := false
		for _, action := range view.AllowedActions {
			if action == "cancel" {
				t.Fatal("expired lease offered cancel")
			}
			reconcile = reconcile || action == "reconcile"
		}
		if !reconcile {
			t.Fatal("expired lease omitted local reconciliation")
		}
	}
	if e := h.w.cloudTurnHeartbeat(ctx, p); e == nil {
		t.Fatal("expired lease revived")
	}
	req := wc.CloudTurnControlRequest{ProjectID: h.f.project, ExpectedVersion: turn.Version, IdempotencyKey: newID()}
	c, e := h.w.ReconcileCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, req)
	if e != nil || c.State != "acknowledged" {
		t.Fatal(e)
	}
	replay, e := h.w.ReconcileCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, req)
	if e != nil || replay.ID != c.ID {
		t.Fatal(e)
	}
	if e = h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "rejected", KnownNotSent: true}); e == nil {
		t.Fatal("fenced old permit mutated")
	}
	view, e := h.w.CloudTurns(ctx, h.f.actor, h.f.project, sub.ID)
	if e != nil || view.LatestTurn.State != "unknown" || view.LatestTurn.Overdue {
		t.Fatal(e)
	}
	for _, a := range view.AllowedActions {
		if a == "prompt" {
			t.Fatal("unknown head offered new input")
		}
	}
}

func TestCloudTurnCurrentOwnerAndCASRejectOtherActorsAndRollback(t *testing.T) {
	h, sub, req, permits := cloudTurnFixture(t)
	ctx := context.Background()
	other := cloudOtherWriter(t, h)
	if _, e := h.w.PromptCloudTurn(ctx, other, h.f.project, sub.ID, req); e == nil {
		t.Fatal("other writer used private grant")
	}
	if _, e := h.w.PromptCloudTurn(ctx, h.actor, h.f.project, sub.ID, req); e == nil {
		t.Fatal("runner invented human prompt")
	}
	bad := req
	bad.ExpectedRunVersion++
	if _, e := h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, bad); e == nil {
		t.Fatal("stale run accepted")
	}
	var count int
	if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workbuddy_cloud_turns`).Scan(&count); e != nil || count != 0 || len(*permits) != 0 {
		t.Fatal("failed CAS left authority", e)
	}
	if _, e := h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req); e != nil {
		t.Fatal("original key polluted by failed CAS", e)
	}
}

func TestCloudTurnConcurrentHeadsHaveOneWinner(t *testing.T) {
	h, sub, req, permits := cloudTurnFixture(t)
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			r := req
			r.IdempotencyKey = newID()
			_, e := h.w.PromptCloudTurn(context.Background(), h.f.actor, h.f.project, sub.ID, r)
			results <- e
		}()
	}
	wait.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 || len(*permits) != 1 {
		t.Fatal("head race", wins, len(*permits))
	}
}

func TestCloudTurnArchivePaginationAndHeartbeatDoNotChangeSemanticVersion(t *testing.T) {
	h, _, turn, p := cloudTurnPrepared(t)
	ctx := context.Background()
	cloudTurnStart(t, h, p)
	before, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{"first", "second"} {
		if e = h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "entry", Entry: &wc.CloudTurnEntry{Kind: "assistant_text", Text: text}}); e != nil {
			t.Fatal(e)
		}
	}
	if e = h.w.cloudTurnHeartbeat(ctx, p); e != nil {
		t.Fatal(e)
	}
	page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 1)
	if e != nil || page.Turn.Version != before.Turn.Version || len(page.Items) != 1 || !page.HasMore || page.NextAfter != 1 {
		t.Fatal("cursor/heartbeat changed semantic version", e)
	}
	next, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, page.NextAfter, 1)
	if e != nil || next.HasMore || next.Items[0].Text != "second" {
		t.Fatal(e)
	}
	// A single oversized entry is rejected before any partial database write.
	if e = h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "entry", Entry: &wc.CloudTurnEntry{Kind: "assistant_text", Text: strings.Repeat("x", wc.CloudTurnMaxEntryBytes+1)}}); e == nil {
		t.Fatal("entry limit ignored")
	}
}

func TestCloudTurnHTTPClosesBrowserRoutesAndExactBodies(t *testing.T) {
	h, sub, req, _ := cloudTurnFixture(t)
	server := &Server{Service: h.f.s, Engine: h.f.e}
	parts := []string{"workbuddy-cloud-submissions", sub.ID, "turns"}
	check := func(method, path, body, scope string, actor Actor, route []string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		out := httptest.NewRecorder()
		if !server.workBuddyCloudTurnsHTTP(out, r, actor, scope, "", route, h.w) || out.Code != want || out.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("closed route", out.Code, out.Body.String())
		}
		return out
	}
	path := "/unused?project_id=" + h.f.project
	check("GET", path, "", "", h.f.actor, parts, 200)
	check("GET", path+"&project_id="+h.f.project, "", "", h.f.actor, parts, 400)
	check("GET", path, "", "runner", h.actor, parts, 403)
	check("POST", "/unused?project_id="+h.f.project, stringify(req), "", h.f.actor, parts, 400)
	check("POST", "/unused", strings.Replace(stringify(req), `"expected_version":0,`, "", 1), "", h.f.actor, parts, 400)
	check("POST", "/unused", stringify(req), "", h.f.actor, parts, 202)
}

func TestCloudTurnSchemaRefusesPartialBeforeAnyDDL(t *testing.T) {
	db, e := store.Open("sqlite", filepath.Join(t.TempDir(), "partial-turns.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := &Service{DB: db, Dialect: "sqlite"}
	if _, e = db.Exec(`CREATE TABLE suma_workbuddy_cloud_turn_heads(id TEXT PRIMARY KEY,note TEXT);INSERT INTO suma_workbuddy_cloud_turn_heads VALUES('original','unique')`); e != nil {
		t.Fatal(e)
	}
	if e = s.InitializeWorkBuddyCloudTurns(context.Background()); !errors.Is(e, store.ErrInitialSchema) {
		t.Fatal("partial accepted", e)
	}
	var count int
	if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("DDL before validation", e)
	}
	var note string
	if e = db.QueryRow(`SELECT note FROM suma_workbuddy_cloud_turn_heads`).Scan(&note); e != nil || note != "unique" {
		t.Fatal("unique original changed", e)
	}
}

func TestCloudTurnPermissionImplicitOriginAndQueuedAllowAuthorizationChange(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "permission_before_tool", true: "queued_allow_policy_changed"}[changed], func(t *testing.T) {
			h, _, turn, p := cloudTurnPrepared(t)
			ctx := context.Background()
			cloudTurnStart(t, h, p)
			permission := cloudTurnPendingPermission(t, h, p, true)
			req := wc.CloudTurnPermissionReplyRequest{ProjectID: h.f.project, ExpectedVersion: 1, OptionID: "option:一次", IdempotencyKey: newID()}
			if _, e := h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, req); e != nil {
				t.Fatal(e)
			}
			if changed {
				if _, e := h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: "project", ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "No new execution", IdempotencyKey: newID()}); e != nil {
					t.Fatal(e)
				}
			}
			commands, e := h.w.cloudTurnTakeCommands(ctx, p)
			if e != nil {
				t.Fatal(e)
			}
			if changed {
				if len(commands) != 0 {
					t.Fatal("unsafe queued allow delivered")
				}
				page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
				if e != nil || page.Commands[0].State != "rejected" || page.Permissions[0].State != "pending" {
					t.Fatal("lost accurate rejection option", e)
				}
				req.ExpectedVersion = page.Permissions[0].Version
				req.OptionID = "reject:一次"
				req.IdempotencyKey = newID()
				if _, e = h.w.ReplyCloudTurnPermission(ctx, h.f.actor, h.f.project, turn.ID, permission.ID, req); e != nil {
					t.Fatal("risk reducing rejection lost", e)
				}
				return
			}
			if len(commands) != 1 {
				t.Fatal("missing original command")
			}
			if e = h.w.cloudTurnAckCommand(ctx, p, commands[0], "acknowledged", ""); e != nil {
				t.Fatal(e)
			}
			for _, event := range []cloudTurnRuntimeEvent{{Kind: "tool", ToolCallID: permission.ToolCallID, ToolStart: true, ToolState: "in_progress"}, {Kind: "tool", ToolCallID: permission.ToolCallID, ToolState: "completed"}} {
				if e = h.w.cloudTurnAppend(ctx, p, event); e != nil {
					t.Fatal(e)
				}
			}
			if e = h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "completed", StopReason: "end_turn"}); e != nil {
				t.Fatal(e)
			}
			page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
			if e != nil || page.Turn.State != "completed" {
				t.Fatal("legitimate permission-before-tool became orphan", e)
			}
		})
	}
}

func TestCloudTurnNotSentReleasesOnlyUnobservedHead(t *testing.T) {
	for _, observed := range []bool{false, true} {
		t.Run(map[bool]string{false: "known_not_sent", true: "observed_reply_cannot_be_not_sent"}[observed], func(t *testing.T) {
			h, sub, req, permits := cloudTurnFixture(t)
			ctx := context.Background()
			turn, e := h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req)
			if e != nil {
				t.Fatal(e)
			}
			p := (*permits)[0]
			if e = h.w.cloudTurnBeginPrompt(ctx, p); e != nil {
				t.Fatal(e)
			}
			if observed {
				if e = h.w.cloudTurnAppend(ctx, p, cloudTurnRuntimeEvent{Kind: "entry", Entry: &wc.CloudTurnEntry{Kind: "assistant_text", Text: "already observed\nreply"}}); e != nil {
					t.Fatal(e)
				}
			}
			if e = h.w.cloudTurnFinish(ctx, p, cloudTurnRuntimeEvent{Kind: "terminal", State: "rejected", KnownNotSent: true, ErrorCode: "workbuddy_cloud_turn_rejected"}); e != nil {
				t.Fatal(e)
			}
			page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, turn.ID, 0, 50)
			if e != nil {
				t.Fatal(e)
			}
			want := "rejected"
			if observed {
				want = "unknown"
			}
			if page.Turn.State != want {
				t.Fatal("unproved no-effect released head", page.Turn.State)
			}
			req.ExpectedVersion = 1
			req.IdempotencyKey = newID()
			_, e = h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req)
			if observed && e == nil || !observed && e != nil {
				t.Fatal("wrong next head eligibility", e)
			}
		})
	}
}
func TestCloudTurnPrepareReplayHasOneUnreconstructiblePermit(t *testing.T) {
	h, sub, req, permits := cloudTurnFixture(t)
	ctx := context.Background()
	view, e := h.w.CloudTurns(ctx, h.f.actor, h.f.project, sub.ID)
	if e != nil || view.Version != 0 || len(view.AllowedActions) != 1 || view.AllowedActions[0] != "prompt" {
		t.Fatal("empty current head", e, view)
	}
	first, e := h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req)
	if e != nil {
		t.Fatal(e)
	}
	again, e := h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req)
	if e != nil || first.ID != again.ID || len(*permits) != 1 {
		t.Fatal("replay launched another input", e, len(*permits))
	}
	req.IdempotencyKey = newID()
	req.ExpectedVersion = 1
	if _, e = h.w.PromptCloudTurn(ctx, h.f.actor, h.f.project, sub.ID, req); e == nil {
		t.Fatal("active head replaced")
	}
	page, e := h.w.GetCloudTurn(ctx, h.f.actor, h.f.project, first.ID, 0, 50)
	if e != nil || page.Turn.ID != first.ID || len(page.Items) != 0 || h.creates.Load() != 1 {
		t.Fatal("durable archive", e)
	}
}
