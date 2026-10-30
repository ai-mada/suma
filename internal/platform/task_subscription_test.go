package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestTaskSubscriptionHTTPInitialReadHasNoEffects(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	before := map[string]int{}
	for _, table := range []string{"suma_operations", "suma_notification_inbox", "suma_activity"} {
		before[table] = domainACount(t, f.s, table)
	}
	out := f.call("GET", "/tasks/"+h.task.ID+"/subscription?project_id="+f.project, nil, 200)
	if out["version"] != float64(0) || out["following"] != false || out["updated_at"] != "" || out["user_id"] != f.user.ID {
		t.Fatal("unconfigured subscription is not a pure personal observation", out)
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("GET created business facts", table)
		}
	}
}

func subscriptionHTTP(t *testing.T, f *fixture, actor Actor, token, task string, body any, want int) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/tasks/"+task+"/subscription/commands?project_id="+f.project, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	} else {
		r.AddCookie(f.cookie)
	}
	r.Header.Set("X-Suma-Expected-User", actor.Projection().ID)
	r.Header.Set("X-Suma-Expected-Organization", actor.Projection().TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("subscription HTTP => %d want %d: %s", w.Code, want, w.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal("non JSON reply")
	}
	return out
}

func subscriptionCommand(t *testing.T, f *fixture, actor Actor, task string, version int, action, key string) TaskSubscriptionReceipt {
	t.Helper()
	out, err := f.s.CommandTaskSubscription(context.Background(), actor, f.project, task, TaskSubscriptionCommand{ProjectID: f.project, TaskID: task, ExpectedVersion: version, Action: action, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func subscriptionNotificationCount(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_notification_inbox WHERE kind='task_followed'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTaskSubscriptionViewerCASNoopAndOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	empty := subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "unfollow", "subscription-empty154")
	if empty.Subscription.Version != 0 || domainACount(t, f.s, "suma_task_subscriptions") != 0 {
		t.Fatal("initial unfollow created preference")
	}
	first := subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", "subscription-follow154")
	if first.Subscription.Version != 1 || !first.Subscription.Following {
		t.Fatal(first)
	}
	noop := subscriptionCommand(t, f, h.receiver, h.task.ID, 1, "follow", "subscription-noop154")
	if stringify(first.Subscription) != stringify(noop.Subscription) {
		t.Fatal("already-following command changed version/time")
	}
	unfollow := subscriptionCommand(t, f, h.receiver, h.task.ID, 1, "unfollow", "subscription-unfollow154")
	replayed := subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", "subscription-follow154")
	if stringify(replayed) != stringify(first) || unfollow.Subscription.Version != 2 {
		t.Fatal("original receipt changed")
	}
	current, err := f.s.GetTaskSubscription(ctx, h.receiver, f.project, h.task.ID)
	if err != nil || current.Following || current.Version != 2 {
		t.Fatal("old acknowledgement reset current preference", err)
	}
	_, err = f.s.CommandTaskSubscription(ctx, h.receiver, f.project, h.task.ID, TaskSubscriptionCommand{ProjectID: f.project, TaskID: h.task.ID, ExpectedVersion: 0, Action: "unfollow", IdempotencyKey: "subscription-follow154"})
	requireAuthorityStatus(t, err, 409)
	_, err = f.s.CommandTaskSubscription(ctx, h.receiver, f.project, h.task.ID, TaskSubscriptionCommand{ProjectID: f.project, TaskID: h.task.ID, ExpectedVersion: 1, Action: "follow", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandTaskSubscription(ctx, h.receiver, f.project, h.task.ID, TaskSubscriptionCommand{ProjectID: f.project, TaskID: h.task.ID, ExpectedVersion: 0, Action: "follow", IdempotencyKey: "subscription-follow154"})
	requireAuthorityStatus(t, err, 403)
}

func TestTaskSubscriptionHumanProjectionMentionPriorityAndPrivateSource(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	subscriptionCommand(t, f, f.actor, h.task.ID, 0, "follow", newID())
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "new discussion")
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("self notification or missing subscriber")
	}
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if len(page.Items) != 1 || page.Items[0].Kind != "task_followed" || !page.Items[0].SourceAvailable || page.Items[0].MessageID != message.ID {
		t.Fatal("private source did not hydrate", page)
	}
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err != nil || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("original message replay repeated notifications", err)
	}
	req = taskConversationRequest(f, h.task.ID, h.task.Version, "precise mention wins")
	req.MentionResourceIDs = []string{h.resource.ID}
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err != nil || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("mention/follow duplication", err)
	}
	subscriptionCommand(t, f, h.receiver, h.task.ID, 1, "unfollow", newID())
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "after cancellation")); err != nil || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("unfollow did not affect subsequent messages", err)
	}
	if !inboxPage(t, f, h.receiver, "inbox", 20, "").Items[1].SourceAvailable {
		t.Fatal("unfollow hid immutable original source")
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_task_subscription_messages WHERE message_id=?`, message.ID); err != nil {
		t.Fatal(err)
	}
	page = inboxPage(t, f, h.receiver, "inbox", 20, "")
	for _, item := range page.Items {
		if item.Kind == "task_followed" && (item.SourceAvailable || item.MessageID != "" || item.CurrentTask != nil) {
			t.Fatal("missing mapping exposed task/message", item)
		}
	}
}

func TestTaskSubscriptionDuplicateProjectionAndRecipientWithdrawal(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "exact notification event"))
	if err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, h.task.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = f.s.projectTaskSubscriptionMessageTx(bounded, auth, f.project, h.task.ID, message.ID); err != nil {
			close()
			t.Fatal(err)
		}
	}
	if err = auth.tx.Commit(); err != nil {
		close()
		t.Fatal(err)
	}
	close()
	if subscriptionNotificationCount(t, f) != 1 || domainACount(t, f.s, "suma_task_subscription_messages") != 1 {
		t.Fatal("same persisted event was delivered twice")
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "after recipient withdrawal")); err != nil || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("revoked recipient retained new delivery eligibility", err)
	}
	_, err = f.s.NotificationInbox(ctx, h.receiver, f.project, "inbox", 20, "")
	requireAuthorityStatus(t, err, 403)
}

func TestTaskSubscriptionProjectionFailureRollsBackMessageAndOriginalOperation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	before := map[string]int{}
	for _, table := range []string{"suma_task_conversation_messages", "suma_operations", "suma_activity", "suma_notification_inbox"} {
		before[table] = domainACount(t, f.s, table)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER subscription_reject BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_followed' BEGIN SELECT RAISE(ABORT,'follow projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "atomic follower delivery")
	if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("message survived failed follower delivery")
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("partial facts", table)
		}
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER subscription_reject`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err != nil || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("original retry after rollback failed", err)
	}
}

func TestTaskSubscriptionAllCurrentFollowersWithoutHumanResources(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	for i := 0; i < 27; i++ {
		user := newID()
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO suma_users(id,name,email,password_hash,created_at)VALUES(?,?,?,'fixture',?)`, []any{user, "Follower", fmt.Sprintf("follow154-%d@example.test", i), now()}},
			{`INSERT INTO suma_org_members(tenant_id,user_id,role)VALUES(?,?,'member')`, []any{f.user.TenantID, user}},
			{`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'viewer')`, []any{f.project, user}},
			{`INSERT INTO suma_task_subscriptions(id,tenant_id,project_id,task_id,user_id,version,following,updated_at)VALUES(?,?,?,?,?,1,1,?)`, []any{newID(), f.user.TenantID, f.project, h.task.ID, user, now()}},
		} {
			if _, err := f.s.DB.Exec(q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "Notify every current follower")); err != nil {
		t.Fatal(err)
	}
	if subscriptionNotificationCount(t, f) != 27 {
		t.Fatal("valid follower set was truncated or required a human resource")
	}
}

func TestTaskSubscriptionAPIAndCredentialScope(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	api, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(ctx, api.Token)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionCommand(t, f, actor, h.task.ID, 0, "follow", newID())
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	message, err := f.s.AppendTaskConversation(ctx, actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "Actual current-user API author"))
	if err != nil || message.ActorKind != "api" || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("API self-notification or wrong actual author", err)
	}
	for _, scope := range []string{"runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		machine, e := f.s.AuthenticateActor(ctx, issued.Token)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.GetTaskSubscription(ctx, machine, f.project, h.task.ID)
		requireAuthorityStatus(t, e, 403)
		_, e = f.s.CommandTaskSubscription(ctx, machine, f.project, h.task.ID, TaskSubscriptionCommand{ProjectID: f.project, TaskID: h.task.ID, ExpectedVersion: 1, Action: "unfollow", IdempotencyKey: newID()})
		requireAuthorityStatus(t, e, 403)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Other subscription scope", "idempotency_key": newID()}, 201)["id"].(string)
	_, err = f.s.GetTaskSubscription(ctx, f.actor, other, h.task.ID)
	requireAuthorityStatus(t, err, 404)
	_, err = f.s.GetTaskSubscription(ctx, actor, other, h.task.ID)
	requireAuthorityStatus(t, err, 403)
}

func TestTaskSubscriptionHTTPRejectsUnreviewedBodyAndAmbiguousScope(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	body := map[string]any{"project_id": f.project, "task_id": h.task.ID, "expected_version": 0, "action": "follow", "idempotency_key": "subscription-http154"}
	raw := stringify(body)
	for _, candidate := range []string{
		strings.TrimSuffix(raw, "}") + `,"user_id":"someone-else"}`,
		strings.TrimSuffix(raw, "}") + `,"action":"unfollow"}`,
		strings.Replace(raw, `"expected_version":0`, `"expected_version":null`, 1),
		strings.Replace(raw, `"expected_version":0`, `"expected_version":0.0`, 1),
		strings.Replace(raw, `"project_id"`, `"Project_ID"`, 1),
		strings.Replace(raw, `"task_id":"`+h.task.ID+`"`, `"task_id":".."`, 1),
	} {
		r := httptest.NewRequest("POST", "http://suma.test/api/v1/tasks/"+h.task.ID+"/subscription/commands?project_id="+f.project, io.NopCloser(strings.NewReader(candidate)))
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Expected-User", f.user.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("unreviewed payload accepted: %d %s", w.Code, w.Body.String())
		}
	}
	f.call("GET", "/tasks/"+h.task.ID+"/subscription?project_id="+f.project+"&project_id="+f.project, nil, 400)
	subscriptionHTTP(t, f, f.actor, "", h.task.ID, body, 200)
}

func TestTaskSubscriptionVersionLimitAllowsNoopOnly(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	subscriptionCommand(t, f, f.actor, h.task.ID, 0, "follow", newID())
	if _, err := f.s.DB.Exec(`UPDATE suma_task_subscriptions SET version=?`, taskSubscriptionMaxVersion); err != nil {
		t.Fatal(err)
	}
	current := subscriptionCommand(t, f, f.actor, h.task.ID, taskSubscriptionMaxVersion, "follow", newID())
	if current.Subscription.Version != taskSubscriptionMaxVersion {
		t.Fatal("no-op overflow")
	}
	_, err := f.s.CommandTaskSubscription(context.Background(), f.actor, f.project, h.task.ID, TaskSubscriptionCommand{ProjectID: f.project, TaskID: h.task.ID, ExpectedVersion: taskSubscriptionMaxVersion, Action: "unfollow", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
}

func TestTaskSubscriptionAssistantFinalBindingAndAtomicDelivery(t *testing.T) {
	h := newTaskReplyFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, f.actor, h.request.TaskID, 0, "follow", newID())
	a, result := h.start(t)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER subscription_assistant_reject BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_followed' BEGIN SELECT RAISE(ABORT,'atomic final assistant'); END`); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 500)
	if subscriptionNotificationCount(t, f) != 0 || domainACount(t, f.s, "suma_task_conversation_reply_messages") != 0 {
		t.Fatal("partial assistant source survived rollback")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER subscription_assistant_reject`); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	if subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("assistant requester notification omitted or duplicated")
	}
	page := inboxPage(t, f, f.actor, "inbox", 20, "")
	var followed NotificationInboxItem
	for _, item := range page.Items {
		if item.Kind == "task_followed" {
			followed = item
		}
	}
	if !followed.SourceAvailable || followed.MessageID == "" {
		t.Fatal("completed assistant backing not hydrated", followed)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_task_conversation_reply_messages SET execution_run_id='invalid-backing' WHERE reply_id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.NotificationInbox(ctx, f.actor, f.project, "inbox", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Kind == "task_followed" && (item.SourceAvailable || item.MessageID != "" || item.CurrentTask != nil) {
			t.Fatal("invalid assistant backing exposed a link")
		}
	}
}

func TestTaskSubscriptionSharedInitializationRejectsPartialDomainBeforeWrites(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "partial-subscription.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE suma_task_subscriptions(id TEXT PRIMARY KEY,note TEXT); INSERT INTO suma_task_subscriptions VALUES('unique','preserve')`); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("partial subscription domain accepted", err)
	}
	var count int
	var note string
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("rejected schema initialization wrote tables", count, err)
	}
	if err = db.QueryRow(`SELECT note FROM suma_task_subscriptions WHERE id='unique'`).Scan(&note); err != nil || note != "preserve" {
		t.Fatal("unique source was repaired", err)
	}
}
