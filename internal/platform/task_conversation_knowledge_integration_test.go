package platform

import (
	"context"
	"strings"
	"testing"
)

func conversationKnowledgeForReplyTest(t *testing.T, f *fixture, title, content string) ProjectKnowledgeDetail {
	t.Helper()
	ctx := context.Background()
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "spec", title, content))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func TestTaskConversationKnowledgeReplyDevelopmentAndRetiredStart(t *testing.T) {
	h := newTaskReplyFixture(t)
	f, ctx := h.f, context.Background()
	a := conversationKnowledgeForReplyTest(t, f, "Review source A", "Preserve citation145-A and the existing approval authority.")
	b := conversationKnowledgeForReplyTest(t, f, "Review source B", "Preserve citation145-B and the independent exact-commit test.")
	ref := func(v ProjectKnowledgeDetail) ProjectKnowledgeReference {
		return ProjectKnowledgeReference{ID: v.ID, Revision: v.PublishedRevision, SHA256: v.PublishedSHA256}
	}
	post := taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "Use these explicitly published specifications in the development advice.")
	post.Knowledge = []ProjectKnowledgeReference{ref(a), ref(b)}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, post)
	if err != nil {
		t.Fatal(err)
	}
	h.request.ExpectedLastSequence, h.request.ReplyToSequence = message.Sequence, message.Sequence
	reply, result := h.start(t) // Protocol fixture; no provider-call claim.
	if len(reply.Source.Knowledge) != 2 || reply.Source.Knowledge[0].ID != a.ID || reply.Source.Knowledge[1].SHA256 != b.PublishedSHA256 {
		t.Fatal("reply lost exact ordered knowledge", reply.Source.Knowledge)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	reply, err = f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || reply.State != "completed" || reply.MessageID == "" {
		t.Fatal("original exact protocol result was not published", reply, err)
	}
	// Both source hash corruption and a resealed but incomplete dependency set
	// must fail the same gate used by admission, heartbeat and publication.
	for _, clear := range []bool{false, true} {
		broken := reply
		if clear {
			broken.Source.Knowledge = nil
			broken.Source.SHA256 = ""
			broken.Source.SHA256 = digest(stringify(broken.Source))
		} else {
			broken.Source.SHA256 = strings.Repeat("0", 64)
		}
		err = f.s.withCurrentRead(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
			return f.s.checkTaskConversationReplySourceTx(auth.ctx, auth, broken)
		})
		requireAuthorityStatus(t, err, 409)
	}
	request := taskDiscussionDevelopmentRequestForReply(t, h, reply)
	plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil || len(plan.DispatchPlan.Context.Knowledge) != 2 {
		t.Fatal("development must use the original knowledge channel", plan, err)
	}
	goal := ""
	for _, node := range plan.DispatchPlan.Workflow.Nodes {
		if node.ID == request.AgentNodeID {
			goal, _ = node.Config["goal"].(string)
		}
	}
	if strings.Count(goal, a.Published.Content) != 1 || strings.Count(goal, b.Published.Content) != 1 {
		t.Fatal("approved text must reach the native development goal exactly once")
	}
	before, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, a.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: a.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	after, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil || after.Version != before.Version || after.RunID != before.RunID {
		t.Fatal("retired input must not start a development run or mutate its task", after, err)
	}
	// The referenced human turn is outside the next two-message window. Its
	// verified assistant plus a new human question must retain the dependency.
	question, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, before.ID, before.Version, "Explain the next step without replacing old knowledge."))
	if err != nil {
		t.Fatal(err)
	}
	next := h.request
	next.ExpectedLastSequence, next.ReplyToSequence, next.ContextMessageLimit, next.IdempotencyKey = question.Sequence, question.Sequence, 2, newID()
	_, err = f.s.CreateTaskConversationReply(ctx, f.e, f.actor, f.project, next)
	requireAuthorityStatus(t, err, 409)
	recovered, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, post)
	if err != nil || recovered.ID != message.ID || stringify(recovered.Knowledge) != stringify(message.Knowledge) {
		t.Fatal("recovering the original discussion does not consume retired knowledge", recovered, err)
	}
}

func TestTaskConversationKnowledgeReplyContextLimitRejectsWithoutRun(t *testing.T) {
	h := newTaskReplyFixture(t)
	f, ctx := h.f, context.Background()
	post := taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "Do not silently trim the reviewed sources.")
	for _, title := range []string{"A", "B", "C"} {
		v := conversationKnowledgeForReplyTest(t, f, title, strings.Repeat(title, 11000))
		post.Knowledge = append(post.Knowledge, ProjectKnowledgeReference{ID: v.ID, Revision: v.PublishedRevision, SHA256: v.PublishedSHA256})
	}
	m, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, post)
	if err != nil {
		t.Fatal(err)
	}
	h.request.ExpectedLastSequence, h.request.ReplyToSequence = m.Sequence, m.Sequence
	_, err = f.s.CreateTaskConversationReply(ctx, f.e, f.actor, f.project, h.request)
	requireAuthorityStatus(t, err, 400)
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND task_id=?`, f.user.TenantID, f.project, h.request.TaskID).Scan(&count); err != nil || count != 0 {
		t.Fatal("oversize context left a model reply/run request", count, err)
	}
}
