package platform

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestTaskSubscriptionPageHydratesAtMost51SourcesWithOneBatchRead(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	for i := 0; i < 55; i++ {
		if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "Actual bounded source page")); err != nil {
			t.Fatal(err)
		}
	}
	trace := &schedulingSQLTrace{}
	name := "subscription_batch_" + newID()
	sql.Register(name, &schedulingTraceDriver{trace: trace})
	db, err := sql.Open(name, authorityDatabasePath(t, f.s))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	f.s.DB = db
	page, err := f.s.NotificationInbox(ctx, h.receiver, f.project, "inbox", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 50 || page.NextCursor == "" {
		t.Fatal("the page lost its 50-row bound or continuation")
	}
	for _, item := range page.Items {
		if !item.SourceAvailable || item.MessageID == "" {
			t.Fatal("batch source was not hydrated", item)
		}
	}
	sourceReads, messageReads := 0, 0
	trace.mu.Lock()
	defer trace.mu.Unlock()
	for _, tx := range trace.transactions {
		for _, query := range tx.Statements {
			if strings.Contains(query, "FROM suma_task_subscription_messages") {
				sourceReads++
				if strings.Count(query, "?") > 54 {
					t.Fatal("source IN exceeded the 51-row page plus fixed scope")
				}
			}
			if strings.Contains(query, "FROM suma_task_conversation_messages") {
				messageReads++
			}
		}
	}
	if sourceReads != 1 || messageReads != 0 {
		t.Fatalf("page hydration made per-row SQL calls: reverse=%d message=%d", sourceReads, messageReads)
	}
}

func TestTaskSubscriptionMixedPageDoesNotCrossHydrateDamagedSource(t *testing.T) {
	h := newTaskReplyFixture(t)
	f := h.f
	ctx := context.Background()
	ownerCookie := f.cookie
	inv := governanceInvite(f, "follow-batch154@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, inv, "follow-batch154@example.test")
	recipient, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	second, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "A separate followed task"}))
	if err != nil {
		t.Fatal(err)
	}
	subscriptionCommand(t, f, recipient, h.request.TaskID, 0, "follow", newID())
	subscriptionCommand(t, f, recipient, second.ID, 0, "follow", newID())
	human, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, second.ID, second.Version, "Human from second task"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	apiMessage, err := f.s.AppendTaskConversation(ctx, api, f.project, taskConversationRequest(f, second.ID, second.Version, "API from second task"))
	if err != nil {
		t.Fatal(err)
	}
	firstReply, result := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	var firstAssistant string
	if err = f.s.DB.QueryRow(`SELECT message_id FROM suma_task_conversation_reply_messages WHERE reply_id=?`, firstReply.ID).Scan(&firstAssistant); err != nil {
		t.Fatal(err)
	}
	question, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "A second exact assistant turn"))
	if err != nil {
		t.Fatal(err)
	}
	h.request.IdempotencyKey = newID()
	h.request.ExpectedLastSequence = question.Sequence
	h.request.ReplyToSequence = question.Sequence
	_, secondResult := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": secondResult}), 200)
	page := inboxPage(t, f, recipient, "inbox", 50, "")
	if len(page.Items) != 5 {
		t.Fatal("mixed fixture did not create its exact source set", len(page.Items))
	}
	var badID, badAssistantID string
	for _, item := range page.Items {
		if !item.SourceAvailable || item.MessageID == "" {
			t.Fatal("valid mixed source lost", item)
		}
		if item.MessageID == human.ID {
			badID = item.ID
		}
		if item.MessageID == firstAssistant {
			badAssistantID = item.ID
		}
	}
	if badID == "" || badAssistantID == "" {
		t.Fatal("human notification missing")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_task_subscription_messages SET task_id=? WHERE notification_id=?`, h.request.TaskID, badID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_task_conversation_reply_messages SET execution_run_id='damaged-batch-backing' WHERE reply_id=?`, firstReply.ID); err != nil {
		t.Fatal(err)
	}
	page = inboxPage(t, f, recipient, "inbox", 50, "")
	valid := 0
	for _, item := range page.Items {
		if item.ID == badID || item.ID == badAssistantID {
			if item.SourceAvailable || item.MessageID != "" || item.CurrentTask != nil {
				t.Fatal("damaged mapping borrowed another task/message")
			}
			continue
		}
		if !item.SourceAvailable || item.CurrentTask == nil {
			t.Fatal("one damaged source retired an unrelated source", item)
		}
		if item.MessageID == apiMessage.ID && item.TaskID != second.ID {
			t.Fatal("API message crossed task scope")
		}
		valid++
	}
	if valid != 3 {
		t.Fatal("mixed intact sources not preserved")
	}
}
