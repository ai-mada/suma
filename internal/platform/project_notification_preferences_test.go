package platform

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

func notificationPreferencesCommand(t *testing.T, f *fixture, actor Actor, version int, muted bool, key string) ProjectNotificationPreferencesReceipt {
	t.Helper()
	out, err := f.s.CommandProjectNotificationPreferences(context.Background(), actor, f.project, ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: version, FollowedDiscussionsMuted: muted, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestProjectNotificationPreferencesCurrentIdentityAndProjectIsolation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	first := notificationPreferencesCommand(t, f, api, 0, true, newID())
	current, err := f.s.GetProjectNotificationPreferences(ctx, f.actor, f.project)
	if err != nil || current != first.Preferences {
		t.Fatal("API preference did not belong to current account", err)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Independent notification preferences", "idempotency_key": newID()}, 201)["id"].(string)
	current, err = f.s.GetProjectNotificationPreferences(ctx, f.actor, other)
	if err != nil || current.Version != 0 || current.FollowedDiscussionsMuted {
		t.Fatal("project preference leaked", err)
	}
	_, err = f.s.GetProjectNotificationPreferences(ctx, api, other)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetProjectNotificationPreferences(ctx, h.receiver, other)
	requireAuthorityStatus(t, err, 403)
	for _, scope := range []string{"runner", "agent"} {
		credential, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		machine, e := f.s.AuthenticateActor(ctx, credential.Token)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.GetProjectNotificationPreferences(ctx, machine, f.project)
		requireAuthorityStatus(t, e, 403)
		_, e = f.s.CommandProjectNotificationPreferences(ctx, machine, f.project, ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
		requireAuthorityStatus(t, e, 403)
	}
}

func TestProjectNotificationPreferencesRollbackAndVersionLimit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	before := domainACount(t, f.s, "suma_operations")
	if _, err := f.s.DB.Exec(`CREATE TRIGGER preferences_reject BEFORE INSERT ON suma_operations WHEN NEW.operation_id='preferences-rollback167' BEGIN SELECT RAISE(ABORT,'reject preference receipt'); END`); err != nil {
		t.Fatal(err)
	}
	req := ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: 0, FollowedDiscussionsMuted: true, IdempotencyKey: "preferences-rollback167"}
	if _, err := f.s.CommandProjectNotificationPreferences(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("preference survived receipt failure")
	}
	if domainACount(t, f.s, projectNotificationPreferencesTable) != 0 || domainACount(t, f.s, "suma_operations") != before {
		t.Fatal("partial preference or receipt survived rollback")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER preferences_reject`); err != nil {
		t.Fatal(err)
	}
	notificationPreferencesCommand(t, f, f.actor, 0, true, req.IdempotencyKey)
	if _, err := f.s.DB.Exec(`UPDATE suma_project_notification_preferences SET version=?`, taskSubscriptionMaxVersion); err != nil {
		t.Fatal(err)
	}
	current := notificationPreferencesCommand(t, f, f.actor, taskSubscriptionMaxVersion, true, newID())
	if current.Preferences.Version != taskSubscriptionMaxVersion {
		t.Fatal("noop version overflow")
	}
	req.ExpectedVersion, req.FollowedDiscussionsMuted, req.IdempotencyKey = taskSubscriptionMaxVersion, false, newID()
	_, err := f.s.CommandProjectNotificationPreferences(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 409)
}

func TestProjectNotificationPreferencesMuteOnlyFollowedDiscussions(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	appendMessage := func(actor Actor, text string, mention bool) {
		t.Helper()
		req := taskConversationRequest(f, h.task.ID, h.task.Version, text)
		if mention {
			req.MentionResourceIDs = []string{h.resource.ID}
		}
		if _, err := f.s.AppendTaskConversation(ctx, actor, f.project, req); err != nil {
			t.Fatal(err)
		}
	}
	appendMessage(f.actor, "before mute", false)
	before := inboxPage(t, f, h.receiver, "inbox", 20, "")
	notificationPreferencesCommand(t, f, h.receiver, 0, true, newID())
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	appendMessage(api, "muted API discussion", false)
	if subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("muted API discussion delivered")
	}
	appendMessage(f.actor, "mention remains precise", true)
	after := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if len(after.Items) != len(before.Items)+1 || subscriptionNotificationCount(t, f) != 1 {
		t.Fatal("mute changed mention/history delivery", after)
	}
	var mentions int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_notification_inbox WHERE kind='task_mentioned' AND recipient_user_id=?`, h.receiver.Projection().ID).Scan(&mentions); err != nil {
		t.Fatal(err)
	}
	if mentions != 1 {
		t.Fatal("precise mention suppressed", mentions)
	}
	current, err := f.s.GetTaskSubscription(ctx, h.receiver, f.project, h.task.ID)
	if err != nil || !current.Following || current.Version != 1 {
		t.Fatal("mute changed following", err)
	}
	notificationPreferencesCommand(t, f, h.receiver, 1, false, newID())
	appendMessage(f.actor, "after explicit unmute", false)
	if subscriptionNotificationCount(t, f) != 2 {
		t.Fatal("unmute did not affect only subsequent messages")
	}
	// A preference from another project must not suppress this project's recipient.
	other := f.call("POST", "/projects", map[string]any{"name": "Other muted project", "idempotency_key": newID()}, 201)["id"].(string)
	if _, err = f.s.CommandProjectNotificationPreferences(ctx, f.actor, other, ProjectNotificationPreferencesCommand{ProjectID: other, FollowedDiscussionsMuted: true, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	subscriptionCommand(t, f, f.actor, h.task.ID, 0, "follow", newID())
	appendMessage(h.receiver, "original project still not muted", false)
	if subscriptionNotificationCount(t, f) != 3 {
		t.Fatal("other project's mute leaked into projection")
	}
}

func TestProjectNotificationPreferencesFinalAssistantKeepsMessageWithoutFollowNotice(t *testing.T) {
	h := newTaskReplyFixture(t)
	f := h.f
	subscriptionCommand(t, f, f.actor, h.request.TaskID, 0, "follow", newID())
	notificationPreferencesCommand(t, f, f.actor, 0, true, newID())
	_, result := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	if subscriptionNotificationCount(t, f) != 0 || domainACount(t, f.s, "suma_task_conversation_reply_messages") != 1 {
		t.Fatal("muted final assistant lost its message or delivered follow notice")
	}
}

func TestProjectNotificationPreferencesConcurrentCASAndMessageSerialization(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	notificationPreferencesCommand(t, f, h.receiver, 0, true, newID())
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := f.s.CommandProjectNotificationPreferences(ctx, h.receiver, f.project, ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: 1, FollowedDiscussionsMuted: false, IdempotencyKey: newID()})
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireAuthorityStatus(t, err, 409)
			conflicts++
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("concurrent CAS lost update", success, conflicts)
	}
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	var file string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&file); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	writer, err := other.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err = dag.GuardProjectTx(ctx, writer, "sqlite", f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Exec(`UPDATE suma_project_notification_preferences SET version=3,followed_discussions_muted=1,updated_at=? WHERE tenant_id=? AND project_id=? AND user_id=?`, now(), f.user.TenantID, f.project, h.receiver.Projection().ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "message after serialized preference"))
		done <- e
	}()
	select {
	case err := <-done:
		t.Fatal("message escaped current preference transaction", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = writer.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("message did not resume")
	}
	if subscriptionNotificationCount(t, f) != 0 {
		t.Fatal("message used stale preference")
	}
}

func TestProjectNotificationPreferencesInitialSchemaRejectsBeforeWrites(t *testing.T) {
	for _, kind := range []string{"unregistered", "columns", "identity"} {
		t.Run(kind, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "preferences.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if kind != "unregistered" {
				if err = store.InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
					t.Fatal(err)
				}
			}
			ddl := projectNotificationPreferencesDDL
			if kind == "columns" {
				ddl = `CREATE TABLE suma_project_notification_preferences(id TEXT PRIMARY KEY,note TEXT)`
			}
			if kind == "identity" {
				ddl = `CREATE TABLE suma_project_notification_preferences(id TEXT PRIMARY KEY,tenant_id TEXT,project_id TEXT,user_id TEXT,version INTEGER,followed_discussions_muted INTEGER,updated_at TEXT)`
			}
			if _, err = db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			var before int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if kind == "columns" {
				if _, err = db.Exec(`INSERT INTO suma_project_notification_preferences VALUES('unique','preserve')`); err != nil {
					t.Fatal(err)
				}
			}
			s := &Service{DB: db, Dialect: "sqlite"}
			if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("partial preferences accepted", err)
			}
			var after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); err != nil || after != before {
				t.Fatal("rejection wrote schema", after, before, err)
			}
			if kind == "columns" {
				var note string
				if err = db.QueryRow(`SELECT note FROM suma_project_notification_preferences WHERE id='unique'`).Scan(&note); err != nil || note != "preserve" {
					t.Fatal("unique source changed", err)
				}
			}
		})
	}
}

func TestProjectNotificationPreferencesDefaultViewerCASAndOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	before := map[string]int{}
	for _, table := range []string{projectNotificationPreferencesTable, "suma_operations", "suma_activity", "suma_notification_inbox"} {
		before[table] = domainACount(t, f.s, table)
	}
	current, err := f.s.GetProjectNotificationPreferences(ctx, h.receiver, f.project)
	if err != nil || current.ProjectID != f.project || current.UserID != h.receiver.Projection().ID || current.Version != 0 || current.FollowedDiscussionsMuted || current.UpdatedAt != "" {
		t.Fatal("default read", current, err)
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("GET wrote", table)
		}
	}
	empty := notificationPreferencesCommand(t, f, h.receiver, 0, false, "preferences-empty167")
	if empty.Preferences != current || domainACount(t, f.s, projectNotificationPreferencesTable) != 0 {
		t.Fatal("default noop created preference")
	}
	first := notificationPreferencesCommand(t, f, h.receiver, 0, true, "preferences-mute167")
	if first.Preferences.Version != 1 || !first.Preferences.FollowedDiscussionsMuted || first.Preferences.UpdatedAt == "" {
		t.Fatal(first)
	}
	noop := notificationPreferencesCommand(t, f, h.receiver, 1, true, "preferences-noop167")
	if noop.Preferences != first.Preferences {
		t.Fatal("noop changed version/time")
	}
	second := notificationPreferencesCommand(t, f, h.receiver, 1, false, "preferences-unmute167")
	replay := notificationPreferencesCommand(t, f, h.receiver, 0, true, "preferences-mute167")
	if replay != first || second.Preferences.Version != 2 {
		t.Fatal("original receipt changed")
	}
	current, err = f.s.GetProjectNotificationPreferences(ctx, h.receiver, f.project)
	if err != nil || current != second.Preferences {
		t.Fatal("replay overwrote current preference", err)
	}
	for _, req := range []ProjectNotificationPreferencesCommand{
		{ProjectID: f.project, ExpectedVersion: 0, FollowedDiscussionsMuted: false, IdempotencyKey: "preferences-mute167"},
		{ProjectID: f.project, ExpectedVersion: 1, FollowedDiscussionsMuted: true, IdempotencyKey: newID()},
	} {
		_, err = f.s.CommandProjectNotificationPreferences(ctx, h.receiver, f.project, req)
		requireAuthorityStatus(t, err, 409)
	}
	for _, table := range []string{"suma_activity", "suma_notification_inbox"} {
		if domainACount(t, f.s, table) != before[table] {
			t.Fatal("preference created business facts", table)
		}
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandProjectNotificationPreferences(ctx, h.receiver, f.project, ProjectNotificationPreferencesCommand{ProjectID: f.project, ExpectedVersion: 0, FollowedDiscussionsMuted: true, IdempotencyKey: "preferences-mute167"})
	requireAuthorityStatus(t, err, 403)
}
