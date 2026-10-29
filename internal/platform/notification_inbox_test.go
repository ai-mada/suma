package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func inboxPage(t *testing.T, f *fixture, actor Actor, filter string, limit int, cursor string) NotificationInboxPage {
	t.Helper()
	page, err := f.s.NotificationInbox(context.Background(), actor, f.project, filter, limit, cursor)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func inboxCreateTask(t *testing.T, h *handoffFixture, title, owner, key string) Record {
	t.Helper()
	out, err := h.f.s.Create(context.Background(), h.f.actor, h.f.project, "tasks", map[string]any{"title": title, "owner_id": owner, "idempotency_key": key})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNotificationInboxSourceAssignmentsReplayAndHydrateCurrentTask(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	before := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	task := inboxCreateTask(t, h, "Original assignment", h.owner.ID, "inbox-create-original")
	self := inboxPage(t, f, f.actor, "inbox", 20, "")
	if self.Total != before+1 || len(self.Items) == 0 || self.Items[0].CurrentTask == nil || self.Items[0].CurrentTask.Title != "Original assignment" || !self.Items[0].AssignmentCurrent || self.Items[0].Sequence == "" {
		t.Fatalf("missing real self assignment: %+v", self)
	}
	if replay := inboxCreateTask(t, h, "Original assignment", h.owner.ID, "inbox-create-original"); replay.ID != task.ID || inboxPage(t, f, f.actor, "inbox", 20, "").Total != self.Total {
		t.Fatal("source create replay duplicated event")
	}
	body := map[string]any{"project_id": f.project, "expected_version": task.Version, "owner_id": h.resource.ID, "idempotency_key": "inbox-reassign-original"}
	changed, err := f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, body)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, body); err != nil {
		t.Fatal(err)
	}
	receiver := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if receiver.Total != 1 || receiver.Items[0].TaskID != task.ID || receiver.Items[0].SourceTaskVersion != changed.Version || !receiver.Items[0].AssignmentCurrent {
		t.Fatalf("receiver event is not exact: %+v", receiver)
	}
	old := inboxPage(t, f, f.actor, "inbox", 20, "")
	if old.Total != self.Total || old.Items[0].AssignmentCurrent {
		t.Fatalf("old receipt was reassigned: %+v", old)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, map[string]any{"project_id": f.project, "expected_version": changed.Version, "title": "Current title only", "idempotency_key": "inbox-change-title-only"})
	if err != nil {
		t.Fatal(err)
	}
	receiver = inboxPage(t, f, h.receiver, "inbox", 20, "")
	if receiver.Total != 1 || receiver.Items[0].CurrentTask.Title != "Current title only" {
		t.Fatalf("title snapshot or duplicate: %+v", receiver)
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_tasks WHERE id=?`, task.ID); err != nil {
		t.Fatal(err)
	}
	unlinked := inboxPage(t, f, h.receiver, "inbox", 20, "").Items[0]
	if unlinked.SourceAvailable || unlinked.CurrentTask != nil || unlinked.AssignmentCurrent {
		t.Fatalf("unlinked source disclosed cached task: %+v", unlinked)
	}
}

func TestNotificationInboxProjectionFailureRollsBackSourceAndCounter(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	var tasks, beforeSeq, afterSeq int64
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tasks`).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&beforeSeq); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER inbox_reject_source BEFORE INSERT ON suma_notification_inbox BEGIN SELECT RAISE(ABORT,'owned projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.Create(ctx, f.actor, f.project, "tasks", map[string]any{"title": "Must roll back source", "owner_id": h.owner.ID, "idempotency_key": "inbox-failed-source"})
	if err == nil {
		t.Fatal("source succeeded without its atomic notification")
	}
	if _, err = f.s.DB.Exec(`DROP TRIGGER inbox_reject_source`); err != nil {
		t.Fatal(err)
	}
	var current, receipt int64
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tasks`).Scan(&current); err != nil || current != tasks {
		t.Fatal("failed projection retained task", current, err)
	}
	if err = f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&afterSeq); err != nil || afterSeq != beforeSeq {
		t.Fatal("failed projection advanced sequence", beforeSeq, afterSeq, err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE operation_id='inbox-failed-source'`).Scan(&receipt); err != nil || receipt != 0 {
		t.Fatal("failed source retained receipt", receipt, err)
	}
}

func TestNotificationInboxNonhumanUnboundAndUnchangedOwnersDoNotGuessRecipients(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	before := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	agent, err := f.s.Create(ctx, f.actor, f.project, "members", map[string]any{"title": "Agent source", "kind": "agent", "idempotency_key": "inbox-agent-resource"})
	if err != nil {
		t.Fatal(err)
	}
	inboxCreateTask(t, h, "Assigned to agent", agent.ID, "inbox-agent-task")
	inboxCreateTask(t, h, "Not assigned", "", "inbox-unassigned-task")
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET status='inactive' WHERE id=?`, h.owner.ID); err != nil {
		t.Fatal(err)
	}
	inboxCreateTask(t, h, "Inactive canonical human", h.owner.ID, "inbox-inactive-canonical-task")
	if inboxPage(t, f, f.actor, "inbox", 20, "").Total != before {
		t.Fatal("nonhuman or empty owner became current user")
	}
	for _, kind := range []string{"unbound", "inactive"} {
		bounded, tx, cleanup, e := f.s.beginAuthorityTx(ctx)
		if e != nil {
			t.Fatal(e)
		}
		auth, e := f.s.authorizeTx(bounded, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
		if e != nil {
			cleanup()
			t.Fatal(e)
		}
		member := recordFrom(f.user, f.project, map[string]any{"title": "Source resource", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}})
		if kind == "inactive" {
			member.Status = "inactive"
		}
		task := recordFrom(f.user, f.project, map[string]any{"title": "Bounded source", "owner_id": member.ID})
		if e = f.s.insertRecord(bounded, tx, member, "members"); e == nil {
			e = f.s.insertRecord(bounded, tx, task, "tasks")
		}
		if e == nil {
			e = f.s.projectTaskAssignmentNotificationTx(bounded, auth, "", task)
		}
		if e != nil {
			tx.Rollback()
			cleanup()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			cleanup()
			t.Fatal(e)
		}
		cleanup()
	}
	if inboxPage(t, f, f.actor, "inbox", 20, "").Total != before {
		t.Fatal("unbound resource guessed recipient")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='invalid-role' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	inboxCreateTask(t, h, "Invalid organization membership", h.resource.ID, "inbox-invalid-organization-role")
	var corruptRecipient int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_notification_inbox WHERE recipient_user_id=?`, h.receiver.credential.subjectID).Scan(&corruptRecipient); err != nil || corruptRecipient != 0 {
		t.Fatal("invalid organization role received notification", corruptRecipient, err)
	}
}

func TestNotificationInboxSequencePagingCountsAndCursorScope(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	for index := 0; index < 53; index++ {
		inboxCreateTask(t, h, fmt.Sprintf("Assigned %03d", index), h.resource.ID, fmt.Sprintf("inbox-paged-source-%03d", index))
	}
	first := inboxPage(t, f, h.receiver, "inbox", 7, "")
	if first.Total != 53 || first.Counts.Inbox != 53 || first.Counts.Unread != 53 || first.Counts.Archived != 0 || first.Limit != 7 {
		t.Fatalf("partial counts: %+v", first)
	}
	seen := map[string]bool{}
	previous := int64(1<<63 - 1)
	page := first
	for {
		for _, item := range page.Items {
			seq, err := strconv.ParseInt(item.Sequence, 10, 64)
			if err != nil || seq <= 0 || seq >= previous || seen[item.ID] {
				t.Fatalf("invalid sequence traversal: %+v", item)
			}
			seen[item.ID], previous = true, seq
		}
		if page.NextCursor == "" {
			break
		}
		page = inboxPage(t, f, h.receiver, "inbox", 7, page.NextCursor)
	}
	if len(seen) != 53 {
		t.Fatalf("lost notifications: %d", len(seen))
	}
	for _, change := range []struct {
		filter string
		limit  int
	}{{"unread", 7}, {"inbox", 8}, {"archived", 7}} {
		_, err := f.s.NotificationInbox(context.Background(), h.receiver, f.project, change.filter, change.limit, first.NextCursor)
		requireAuthorityStatus(t, err, 400)
	}
	_, err := f.s.NotificationInbox(context.Background(), f.actor, f.project, "inbox", 7, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, wire := range []string{" " + string(raw), string(raw[:len(raw)-1]) + `,"unknown":true}`} {
		_, err = f.s.NotificationInbox(context.Background(), h.receiver, f.project, "inbox", 7, base64.RawURLEncoding.EncodeToString([]byte(wire)))
		requireAuthorityStatus(t, err, 400)
	}
}

func TestNotificationInboxViewerPersonalCASReplayAndNoProjectWrite(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	task := inboxCreateTask(t, h, "Viewer notification", h.resource.ID, "inbox-viewer-source")
	activityBefore := domainACount(t, f.s, "suma_activity")
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	item := inboxPage(t, f, h.receiver, "inbox", 20, "").Items[0]
	read := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: "inbox-viewer-read"}
	one, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, read)
	if err != nil || !one.Read || one.Archived || one.Version != item.Version+1 || one.ReadAt == "" {
		t.Fatalf("viewer personal read: %+v %v", one, err)
	}
	archive := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: one.Version, Action: "archive", IdempotencyKey: "inbox-viewer-archive"}
	two, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, archive)
	if err != nil || !two.Archived || !two.Read || two.ArchivedAt == "" {
		t.Fatalf("independent archive: %+v %v", two, err)
	}
	again, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, read)
	if err != nil || again != one {
		t.Fatalf("original receipt not recovered: %+v %v", again, err)
	}
	noop := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: two.Version, Action: "archive", IdempotencyKey: "inbox-viewer-noop"}
	same, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, noop)
	if err != nil || same.Version != two.Version || same.UpdatedAt != two.UpdatedAt {
		t.Fatalf("no-op changed row: %+v %v", same, err)
	}
	wrong := archive
	wrong.IdempotencyKey = "inbox-viewer-stale"
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, wrong)
	requireAuthorityStatus(t, err, 409)
	restore := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: two.Version, Action: "restore", IdempotencyKey: "inbox-viewer-restore"}
	three, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, restore)
	if err != nil || three.Archived || !three.Read {
		t.Fatalf("restore reset read state: %+v %v", three, err)
	}
	unread := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: three.Version, Action: "unread", IdempotencyKey: "inbox-viewer-unread"}
	four, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, unread)
	if err != nil || four.Read || four.ReadAt != "" || four.Archived {
		t.Fatalf("unread lifecycle: %+v %v", four, err)
	}
	_, err = f.s.Update(ctx, h.receiver, f.project, "tasks", task.ID, map[string]any{"expected_version": task.Version, "title": "Forbidden project write", "idempotency_key": "inbox-viewer-project-write"})
	requireAuthorityStatus(t, err, 403)
	var taskVersion int
	if err = f.s.DB.QueryRow(`SELECT version FROM suma_tasks WHERE id=?`, task.ID).Scan(&taskVersion); err != nil || taskVersion != task.Version {
		t.Fatal("personal action changed task", taskVersion, err)
	}
	if domainACount(t, f.s, "suma_activity") != activityBefore {
		t.Fatal("personal notification state leaked into public project activity")
	}
	ownerItem := inboxPage(t, f, f.actor, "inbox", 20, "").Items[0]
	foreign := read
	foreign.ExpectedVersion = ownerItem.Version
	foreign.IdempotencyKey = "inbox-other-recipient"
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, f.project, ownerItem.ID, foreign)
	requireAuthorityStatus(t, err, 404)
}

func TestNotificationInboxHTTPClosedQueriesCommandsAndCredentialKinds(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	item := inboxPage(t, f, f.actor, "inbox", 20, "").Items[0]
	for _, suffix := range []string{"&user_id=other", "&recipient_user_id=other", "&filter=", "&filter=all", "&filter=inbox&filter=unread", "&limit=0", "&limit=51", "&limit=01", "&limit=1e1", "&cursor=", "&cursor=invalid", "&project_id=other"} {
		f.call("GET", "/notification-inbox?project_id="+f.project+suffix, nil, 400)
	}
	for _, version := range []string{"1e0", "1.0", "true", "null", "0", "-1", "2147483647", "2147483648"} {
		body := `{"project_id":"` + f.project + `","expected_version":` + version + `,"action":"read","idempotency_key":"inbox-raw-version"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/notification-inbox/"+item.ID+"/commands", strings.NewReader(body))
		req.AddCookie(f.cookie)
		req.Header.Set("X-Suma-Expected-User", f.user.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("ambiguous version %s => %d %s", version, w.Code, w.Body.String())
		}
	}
	for _, bad := range []NotificationInboxCommand{
		{ProjectID: f.project, ExpectedVersion: 2147483647, Action: "read", IdempotencyKey: "inbox-invalid-high-version"},
		{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: "        "},
		{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: "inbox-bad\nkey"},
	} {
		_, err := f.s.CommandNotificationInbox(ctx, f.actor, f.project, item.ID, bad)
		requireAuthorityStatus(t, err, 400)
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: "inbox-token-" + scope})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/notification-inbox?project_id="+f.project, nil)
		req.Header.Set("Authorization", "Bearer "+issued.Token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		want := 403
		if scope == "api" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s inbox read: %d %s", scope, w.Code, w.Body.String())
		}
		actor := domainAActor(t, f.s, issued.Token)
		_, err = f.s.NotificationInbox(ctx, actor, f.project, "inbox", 20, "")
		if scope == "api" {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			requireAuthorityStatus(t, err, 403)
		}
	}
	page := f.call("GET", "/notification-inbox?project_id="+f.project, nil, 200)
	encoded, _ := json.Marshal(page)
	if !strings.Contains(string(encoded), `"sequence":"`) {
		t.Fatal("sequence was exposed as imprecise JSON number")
	}
}

func TestNotificationInboxCurrentPermissionRevocationBeforePageOrReceiptReplay(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	inboxCreateTask(t, h, "Recipient revocation", h.resource.ID, "inbox-revocation-source")
	item := inboxPage(t, f, h.receiver, "inbox", 20, "").Items[0]
	command := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: "inbox-before-revocation"}
	if _, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, command); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.NotificationInbox(ctx, h.receiver, f.project, "inbox", 20, "")
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, f.project, item.ID, command)
	requireAuthorityStatus(t, err, 403)
	other := f.call("POST", "/projects", map[string]any{"name": "Other inbox project", "idempotency_key": "inbox-other-project"}, 201)["id"].(string)
	_, err = f.s.CommandNotificationInbox(ctx, f.actor, other, item.ID, command)
	requireAuthorityStatus(t, err, 400)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.NotificationInbox(ctx, f.actor, f.project, "inbox", 20, "")
	requireAuthorityStatus(t, err, 401)
}

func TestNotificationInboxProjectionVerifiesSavedSourceAndDeduplicatesWithinTransaction(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	before := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	bounded, tx, cleanup, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer tx.Rollback()
	auth, err := f.s.authorizeTx(bounded, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.projectTaskAssignmentNotificationTx(bounded, auth, "", h.task); err != nil {
		t.Fatal(err)
	}
	if err = f.s.projectTaskAssignmentNotificationTx(bounded, auth, "", h.task); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if inboxPage(t, f, f.actor, "inbox", 20, "").Total != before {
		t.Fatal("same source was projected twice")
	}
	bounded, tx, closeAgain, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeAgain()
	defer tx.Rollback()
	auth, err = f.s.authorizeTx(bounded, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	forged := h.task
	forged.Version++
	requireAuthorityStatus(t, f.s.projectTaskAssignmentNotificationTx(bounded, auth, "", forged), 409)
}
