package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTaskConversationMentionsCanonicalRecipientsAndImmutableReplay(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	// A project viewer is a valid mention recipient, unlike a write handoff.
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	query := TaskConversationMentionTargetQuery{ResourceIDs: []string{h.resource.ID, h.owner.ID}}
	targets, err := f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, query)
	if err != nil || targets.Total != 2 || len(targets.Items) != 2 || targets.Items[0].ResourceID != h.resource.ID || targets.Items[1].ResourceID != h.owner.ID || targets.NextCursor != "" {
		t.Fatalf("canonical ordered viewer/self targets: %+v %v", targets, err)
	}
	ownerBefore := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "Please review this exact discussion. @plain-text-name remains ordinary text.")
	req.MentionResourceIDs = []string{h.resource.ID, h.owner.ID}
	original, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || len(original.Mentions) != 2 || original.Mentions[0].ResourceID != h.resource.ID || original.Mentions[0].UserID != h.receiver.credential.subjectID || original.Mentions[0].NameAtPost != h.resource.Title || original.Mentions[1].ResourceID != h.owner.ID {
		t.Fatalf("immutable explicit recipient resolution: %+v %v", original, err)
	}
	self := inboxPage(t, f, f.actor, "inbox", 20, "")
	received := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if self.Total != ownerBefore+1 || received.Total != 1 || received.Items[0].Kind != "task_mentioned" || received.Items[0].TaskID != h.task.ID || !received.Items[0].SourceAvailable {
		t.Fatalf("actual receiver/self inbox: self=%+v recipient=%+v", self, received)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", h.resource.ID, map[string]any{"expected_version": h.resource.Version, "title": "Renamed current person", "idempotency_key": "mentions-rename-recipient"}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || stringify(replayed) != stringify(original) || inboxPage(t, f, f.actor, "inbox", 20, "").Total != self.Total {
		t.Fatalf("original receipt changed after target rename/revocation: %+v %v", replayed, err)
	}
	page, err := f.s.ListTaskConversation(ctx, f.actor, f.project, h.task.ID, 20, 0)
	if err != nil || len(page.Items) != 1 || stringify(page.Items[0]) != stringify(original) {
		t.Fatalf("history changed fixed mention snapshot: %+v %v", page, err)
	}
	visible, err := f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, query)
	if err != nil || visible.Total != 1 || len(visible.Items) != 1 || visible.Items[0].ResourceID != h.owner.ID {
		t.Fatalf("draft target hydration returned revoked person: %+v %v", visible, err)
	}
	changed := req
	changed.MentionResourceIDs = []string{h.owner.ID, h.resource.ID}
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, changed)
	requireAuthorityStatus(t, err, 409)
	req.IdempotencyKey = newID()
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 400)
	_, err = f.s.NotificationInbox(ctx, h.receiver, f.project, "inbox", 20, "")
	requireAuthorityStatus(t, err, 403)
}

func TestTaskConversationMentionsFailureRollsBackMessageReceiptAndInboxSequence(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	counts := map[string]int{}
	for _, table := range []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "suma_task_conversation_mentions", "suma_notification_inbox", "suma_operations", "suma_activity"} {
		counts[table] = domainACount(t, f.s, table)
	}
	var sequenceBefore, sequenceAfter int64
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequenceBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER mentions_reject_delivery BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_mentioned' BEGIN SELECT RAISE(ABORT,'owned mention projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "This message must share the notification transaction")
	req.MentionResourceIDs = []string{h.resource.ID}
	if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("message committed while its explicit mention failed")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER mentions_reject_delivery`); err != nil {
		t.Fatal(err)
	}
	for table, count := range counts {
		if actual := domainACount(t, f.s, table); actual != count {
			t.Fatal("failed mention retained partial facts", table, count, actual)
		}
	}
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequenceAfter); err != nil || sequenceAfter != sequenceBefore {
		t.Fatal("failed mention consumed inbox sequence", sequenceBefore, sequenceAfter, err)
	}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || message.Sequence != 1 || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 1 {
		t.Fatalf("same original operation could not commit after rollback: %+v %v", message, err)
	}
	if err = f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequenceAfter); err != nil || sequenceAfter != sequenceBefore+1 {
		t.Fatal("successful mention did not consume exactly one sequence", sequenceBefore, sequenceAfter, err)
	}
}

func TestTaskConversationMentionsRejectInvalidRecipientsAsOneAtomicRequest(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	agent, err := f.s.Create(ctx, f.actor, f.project, "members", map[string]any{"title": "Agent is not a human mention", "kind": "agent", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	otherProject := f.call("POST", "/projects", map[string]any{"name": "Other mention project", "idempotency_key": newID()}, 201)["id"].(string)
	foreign, err := f.s.Create(ctx, f.actor, otherProject, "members", map[string]any{"title": "Foreign canonical human", "kind": "human", "data": map[string]any{"user_id": f.user.ID}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	forged := recordFrom(f.user, f.project, map[string]any{"title": "Noncanonical duplicate label", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}})
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.insertRecord(ctx, tx, forged, "members"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	beforeMessages, beforeInbox := domainACount(t, f.s, "suma_task_conversation_messages"), domainACount(t, f.s, "suma_notification_inbox")
	for _, ids := range [][]string{{h.resource.ID, agent.ID}, {h.resource.ID, foreign.ID}, {h.resource.ID, forged.ID}, {h.resource.ID, h.resource.ID}, {h.resource.ID, "missing-person"}} {
		req := taskConversationRequest(f, h.task.ID, h.task.Version, "Every explicitly selected target must be eligible")
		req.MentionResourceIDs = ids
		_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
		requireAuthorityStatus(t, err, 400)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_members SET status='inactive' WHERE id=?`, h.resource.ID); err != nil {
		t.Fatal(err)
	}
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "Inactive resource is not a recipient")
	req.MentionResourceIDs = []string{h.resource.ID}
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 400)
	if domainACount(t, f.s, "suma_task_conversation_messages") != beforeMessages || domainACount(t, f.s, "suma_notification_inbox") != beforeInbox {
		t.Fatal("invalid target batch retained a message or partial delivery")
	}
}

func TestTaskConversationMentionsPlainJSONAndExactOldMessageDetail(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	before := domainACount(t, f.s, "suma_notification_inbox")
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "@Receiving human is plain text unless a resource is explicitly selected")
	first, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || strings.Contains(stringify(first), `"mentions"`) || strings.Contains(stringify(req), `"mention_resource_ids"`) {
		t.Fatal("zero mention JSON or ordinary plain text changed", first, err)
	}
	if err = f.s.InitializeTaskConversationMentions(ctx); err != nil {
		t.Fatal(err)
	}
	if domainACount(t, f.s, "suma_task_conversation_mentions") != 0 || domainACount(t, f.s, "suma_notification_inbox") != before {
		t.Fatal("initialization or plain @text inferred mention history")
	}
	for index := 0; index < 24; index++ {
		if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, fmt.Sprintf("Later note %02d", index))); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := f.s.ListTaskConversation(ctx, f.actor, f.project, h.task.ID, 20, 0)
	if err != nil || len(latest.Items) != 20 || latest.Items[0].Sequence <= first.Sequence {
		t.Fatal("test did not move the target outside the latest history window", latest, err)
	}
	counts := map[string]int{}
	for _, table := range []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "suma_operations", "suma_activity", "dag_runs", "suma_notification_inbox"} {
		counts[table] = domainACount(t, f.s, table)
	}
	detail, err := f.s.TaskConversationMessageDetail(ctx, f.actor, f.project, h.task.ID, first.ID)
	if err != nil || detail.ProjectID != f.project || detail.TaskID != h.task.ID || stringify(detail.Message) != stringify(first) || !detail.CanPost {
		t.Fatalf("exact old anchor did not resolve directly: %+v %v", detail, err)
	}
	for table, count := range counts {
		if domainACount(t, f.s, table) != count {
			t.Fatal("reading exact message changed business or private state", table)
		}
	}
}

func TestTaskConversationMentionsFrozenReplyAndDevelopmentSource(t *testing.T) {
	h, ctx := newTaskReplyFixture(t), context.Background()
	f := h.f
	resource, err := f.s.Create(ctx, f.actor, f.project, "members", map[string]any{"title": "Fixed mentioned specialist", "kind": "human", "data": map[string]any{"user_id": f.user.ID}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	req := taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "Explain this exact mention without granting tools or dispatch")
	req.MentionResourceIDs = []string{resource.ID}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	h.request.ExpectedLastSequence, h.request.ReplyToSequence = message.Sequence, message.Sequence
	reply, result := h.start(t)
	if len(reply.Source.Messages) != 2 || len(reply.Source.Messages[1].Mentions) != 1 || reply.Source.Messages[1].Mentions[0].NameAtPost != resource.Title {
		t.Fatal("protected model source lost immutable selected mention", reply.Source)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", resource.ID, map[string]any{"title": "Changed current specialist label", "expected_version": resource.Version, "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	completed, err := f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || completed.State != "completed" || completed.Source.SHA256 != reply.Source.SHA256 {
		t.Fatal("member rename invalidated an immutable protected source", completed, err)
	}
	readCtx, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, h.request.TaskID, purposeProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	_, assistant, err := f.s.taskDiscussionDevelopmentSourceTx(readCtx, auth, f.project, TaskDiscussionDevelopmentCreateRequest{ProjectID: f.project, TaskID: h.request.TaskID, ExpectedTaskVersion: h.request.ExpectedTaskVersion, ReplyID: completed.ID, ExpectedReplyVersion: completed.Version})
	if err != nil || assistant.ActorKind != "assistant" || assistant.ReplyID != completed.ID || len(assistant.Mentions) != 0 {
		t.Fatal("reviewed development source lost reply proof or gained assistant mention authority", assistant, err)
	}
	// Release the authorized read transaction before using the fixture's
	// single database connection for the independent persisted-row count.
	close()
	if domainACount(t, f.s, "suma_task_conversation_mentions") != 1 {
		t.Fatal("protected assistant output manufactured mention relations")
	}
}
