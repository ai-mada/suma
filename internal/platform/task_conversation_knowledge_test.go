package platform

import (
	"context"
	"strings"
	"testing"
)

func publishedConversationKnowledge(t *testing.T, f *fixture, title string) ProjectKnowledgeDetail {
	t.Helper()
	ctx := context.Background()
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "spec", title, "Use the exact approved revision and preserve its source."))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func conversationKnowledgePin(value ProjectKnowledgeDetail) ProjectKnowledgeReference {
	return ProjectKnowledgeReference{ID: value.ID, Revision: value.PublishedRevision, SHA256: value.PublishedSHA256}
}

func TestTaskConversationKnowledgeFixedMetadataAndOriginalReceiptAfterRetirement(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	published := publishedConversationKnowledge(t, f, "Original approved title")
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "Discuss the selected publication explicitly")
	req.Knowledge = []ProjectKnowledgeReference{conversationKnowledgePin(published)}
	original, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || len(original.Knowledge) != 1 || original.Knowledge[0].TitleAtPost != published.Published.Title || original.Knowledge[0].SHA256 != published.PublishedSHA256 {
		t.Fatalf("exact approved metadata: %+v %v", original, err)
	}
	edit := knowledgeDraft(f, "spec", "Unpublished replacement title", "This draft must not replace the original message pin.")
	edit.ExpectedVersion = published.Version
	updated, err := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	fresh := req
	fresh.IdempotencyKey = newID()
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, fresh); err != nil {
		t.Fatal("a new draft hid the still-current publication", err)
	}
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: updated.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || stringify(replay) != stringify(original) {
		t.Fatal("original receipt re-resolved retired knowledge", replay, err)
	}
	detail, err := f.s.TaskConversationMessageDetail(ctx, f.actor, f.project, h.task.ID, original.ID)
	if err != nil || stringify(detail.Message) != stringify(original) {
		t.Fatal("historical metadata changed with the current publication", detail, err)
	}
	fresh.IdempotencyKey = newID()
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, fresh)
	requireAuthorityStatus(t, err, 409)
	plain, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.task.ID, h.task.Version, "No automatic knowledge selection"))
	if err != nil || strings.Contains(stringify(plain), `"knowledge"`) {
		t.Fatal("plain message acquired a new field", plain, err)
	}
}

func TestTaskConversationKnowledgeInsertFailureRollsBackMessageSequenceAndReceipt(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	published := publishedConversationKnowledge(t, f, "Atomic reference")
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "This message and its reference must commit together")
	req.Knowledge = []ProjectKnowledgeReference{conversationKnowledgePin(published)}
	before := map[string]int{}
	for _, table := range []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "suma_task_conversation_knowledge", "suma_operations", "suma_activity"} {
		before[table] = domainACount(t, f.s, table)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER reject_conversation_knowledge BEFORE INSERT ON suma_task_conversation_knowledge BEGIN SELECT RAISE(ABORT,'owned knowledge insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("message committed without its selected publication")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER reject_conversation_knowledge`); err != nil {
		t.Fatal(err)
	}
	for table, count := range before {
		if actual := domainACount(t, f.s, table); actual != count {
			t.Fatal("partial knowledge/message/receipt persisted", table, count, actual)
		}
	}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || message.Sequence != 1 || len(message.Knowledge) != 1 {
		t.Fatal("rollback consumed the message sequence or original operation", message, err)
	}
}

func TestTaskConversationKnowledgeUnionIsOrderedBoundedAndNeverChoosesAnotherRevision(t *testing.T) {
	a := TaskConversationKnowledgeReference{ID: "knowledge-a", Revision: 2, SHA256: strings.Repeat("a", 64), TitleAtPost: "A", KindAtPost: "spec"}
	b := TaskConversationKnowledgeReference{ID: "knowledge-b", Revision: 4, SHA256: strings.Repeat("b", 64), TitleAtPost: "B", KindAtPost: "memory"}
	refs, err := taskConversationKnowledgeReferences([]TaskConversationMessage{{Knowledge: []TaskConversationKnowledgeReference{b, a}}, {Knowledge: []TaskConversationKnowledgeReference{a}}})
	if err != nil || len(refs) != 2 || refs[0].ID != b.ID || refs[1].ID != a.ID {
		t.Fatal("knowledge union lost first-seen message order", refs, err)
	}
	changed := a
	changed.Revision++
	if _, err = taskConversationKnowledgeReferences([]TaskConversationMessage{{Knowledge: []TaskConversationKnowledgeReference{a}}, {Knowledge: []TaskConversationKnowledgeReference{changed}}}); err == nil {
		t.Fatal("a newer revision silently replaced an older message pin")
	}
	if validTaskConversationKnowledgeReferences([]ProjectKnowledgeReference{{ID: a.ID, Revision: 2, SHA256: strings.Repeat("A", 64)}}) || validTaskConversationKnowledgeReferences([]ProjectKnowledgeReference{{ID: a.ID, Revision: 0, SHA256: a.SHA256}}) || validTaskConversationKnowledgeReferences([]ProjectKnowledgeReference{{ID: a.ID, Revision: 2, SHA256: a.SHA256}, {ID: a.ID, Revision: 2, SHA256: a.SHA256}}) {
		t.Fatal("noncanonical/duplicate pins accepted")
	}
	if validTaskConversationKnowledgeReferences(make([]ProjectKnowledgeReference, 17)) {
		t.Fatal("unbounded selection accepted")
	}
	messages := []TaskConversationMessage{{Knowledge: []TaskConversationKnowledgeReference{b, a}}}
	snapshot := []ProjectKnowledgeContextItem{{ID: b.ID, Revision: b.Revision, SHA256: b.SHA256}, {ID: a.ID, Revision: a.Revision, SHA256: a.SHA256}}
	if err = checkTaskConversationKnowledgeSnapshot(messages, snapshot); err != nil {
		t.Fatal("matching ordered snapshot rejected", err)
	}
	if err = checkTaskConversationKnowledgeSnapshot(nil, nil); err != nil {
		t.Fatal("plain historical source changed", err)
	}
	if checkTaskConversationKnowledgeSnapshot(messages, nil) == nil || checkTaskConversationKnowledgeSnapshot(messages, []ProjectKnowledgeContextItem{snapshot[1], snapshot[0]}) == nil || checkTaskConversationKnowledgeSnapshot(nil, snapshot) == nil {
		t.Fatal("missing, reordered or invented source knowledge was accepted")
	}
}

func TestTaskConversationKnowledgeAssistantInheritsOnlyItsSealedCompletedReply(t *testing.T) {
	h, ctx := newTaskReplyFixture(t), context.Background()
	f := h.f
	published := publishedConversationKnowledge(t, f, "Fixed knowledge used by the real reply")
	question := taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "Use the selected publication for this answer")
	question.Knowledge = []ProjectKnowledgeReference{conversationKnowledgePin(published)}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, question)
	if err != nil {
		t.Fatal(err)
	}
	h.request.ExpectedLastSequence = message.Sequence
	h.request.ReplyToSequence = message.Sequence
	h.request.ContextMessageLimit = 1
	reply, result := h.start(t)
	if len(reply.Source.Knowledge) != 1 || reply.Source.Knowledge[0].ID != published.ID {
		t.Fatal("the explicit reply did not seal its publication", reply.Source)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	var assistantID string
	if err = f.s.DB.QueryRow(`SELECT message_id FROM suma_task_conversation_reply_messages WHERE reply_id=?`, reply.ID).Scan(&assistantID); err != nil {
		t.Fatal(err)
	}
	original, err := f.s.TaskConversationMessageDetail(ctx, f.actor, f.project, h.request.TaskID, assistantID)
	if err != nil || len(original.Message.Knowledge) != 1 || original.Message.Knowledge[0].ID != published.ID {
		t.Fatal("assistant-only history lost the sealed source", original, err)
	}
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: published.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.TaskConversationMessageDetail(ctx, f.actor, f.project, h.request.TaskID, assistantID)
	if err != nil || stringify(after.Message) != stringify(original.Message) {
		t.Fatal("retirement changed historical assistant provenance", after, err)
	}
	backing, err := f.s.taskConversationReplyTx(ctx, f.s.DB, f.user.TenantID, f.project, reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range []string{"hash", "snapshot_cleared", "snapshot_replaced", "message_identity", "not_completed", "execution_relation", "human_relation"} {
		t.Run(shape, func(t *testing.T) {
			changed := backing
			switch shape {
			case "hash":
				changed.Reply.Source.SHA256 = strings.Repeat("0", 64)
			case "snapshot_cleared", "snapshot_replaced":
				if shape == "snapshot_cleared" {
					changed.Reply.Source.Knowledge = nil
				} else {
					changed.Reply.Source.Knowledge = append([]ProjectKnowledgeContextItem{}, backing.Reply.Source.Knowledge...)
					changed.Reply.Source.Knowledge[0].ID = "different-knowledge"
				}
				// The altered source has a self-consistent hash. Its message refs
				// still carry the real publication, so the pure union must reject it.
				source := changed.Reply.Source
				source.SHA256 = ""
				changed.Reply.Source.SHA256 = digest(stringify(source))
			case "message_identity":
				changed.Reply.MessageID = "another-message"
			case "not_completed":
				changed.Reply.State = "running"
			case "execution_relation":
				if _, err = f.s.DB.Exec(`UPDATE suma_task_conversation_reply_messages SET execution_run_id='another-run' WHERE message_id=?`, assistantID); err != nil {
					t.Fatal(err)
				}
			case "human_relation":
				if _, err = f.s.DB.Exec(`INSERT INTO suma_task_conversation_knowledge(tenant_id,project_id,task_id,message_id,ordinal,knowledge_id,revision,sha256,title_at_post,kind_at_post) VALUES(?,?,?,?,0,?,?,?,?,?)`, f.user.TenantID, f.project, h.request.TaskID, assistantID, published.ID, published.PublishedRevision, published.PublishedSHA256, published.Published.Title, published.Kind); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_conversation_replies SET state=?,payload=? WHERE id=?`, changed.Reply.State, stringify(changed), reply.ID); err != nil {
				t.Fatal(err)
			}
			if _, readErr := f.s.TaskConversationMessageDetail(ctx, f.actor, f.project, h.request.TaskID, assistantID); readErr == nil {
				t.Fatal("unbacked assistant knowledge was treated as trusted history", shape)
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_conversation_replies SET state=?,payload=? WHERE id=?`, backing.Reply.State, stringify(backing), reply.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_conversation_reply_messages SET execution_run_id=? WHERE message_id=?`, reply.RunID, assistantID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.DB.Exec(`DELETE FROM suma_task_conversation_knowledge WHERE message_id=?`, assistantID); err != nil {
				t.Fatal(err)
			}
		})
	}
}
