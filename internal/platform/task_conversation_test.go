package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

func taskConversationRequest(f *fixture, task string, version int, text string) TaskConversationAppendRequest {
	return TaskConversationAppendRequest{ProjectID: f.project, TaskID: task, ExpectedTaskVersion: version, Body: text, IdempotencyKey: newID()}
}

func TestTaskConversationImmutableContextAndOriginalReceipt(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Original shared requirement"}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Shared task", "requirement_id": requirement["id"]}), 201)
	taskID := task["id"].(string)
	counts := map[string]int{}
	for _, table := range []string{"suma_task_conversation_heads", "suma_task_conversation_messages", "dag_runs", "suma_reviews", "suma_operations", "suma_activity"} {
		counts[table] = domainACount(t, f.s, table)
	}
	empty, err := f.s.ListTaskConversation(ctx, f.actor, f.project, taskID, 0, 0)
	if err != nil || !empty.CanPost || empty.Total != 0 || empty.Items == nil || empty.Limit != 20 || empty.NextBefore != 0 {
		t.Fatal("first read did not remain an accurate empty page", empty, err)
	}
	for table, count := range counts {
		if domainACount(t, f.s, table) != count {
			t.Fatal("read created collaboration or execution state", table)
		}
	}
	req := taskConversationRequest(f, taskID, 1, "  Preserve exact 中文 / English\nline two  ")
	original, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || original.Sequence != 1 || original.Body != req.Body || original.AuthorID != f.user.ID || original.AuthorName != f.user.Name || original.ActorKind != "human" || original.RequirementID != requirement["id"] || original.RequirementVersion != 1 || original.TaskVersion != 1 || original.RunID != "" {
		t.Fatal("server-owned identity/context or exact body was lost", original, err)
	}
	// Simulate a committed response lost by the caller. Subsequent task/source
	// changes must not create a second message or rewrite the original context.
	f.call("PATCH", "/requirements/"+requirement["id"].(string), f.body(map[string]any{"expected_version": 1, "description": "New requirement source"}), 200)
	f.call("PATCH", "/tasks/"+taskID, f.body(map[string]any{"expected_version": 1, "title": "Later task context"}), 200)
	replayed, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil || stringify(replayed) != stringify(original) {
		t.Fatal("lost response did not recover original immutable receipt", replayed, err)
	}
	changed := req
	changed.Body = "different content on the same operation"
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, changed)
	requireAuthorityStatus(t, err, 409)
	changed.IdempotencyKey = newID()
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, changed)
	requireAuthorityStatus(t, err, 409)
	latest, err := f.s.ListTaskConversation(ctx, f.actor, f.project, taskID, 20, 0)
	if err != nil || latest.TaskVersion != 2 || latest.Total != 1 || stringify(latest.Items[0]) != stringify(original) {
		t.Fatal("current task was confused with original message context", latest, err)
	}
	if domainACount(t, f.s, "dag_runs") != counts["dag_runs"] || domainACount(t, f.s, "suma_reviews") != counts["suma_reviews"] {
		t.Fatal("posting collaboration changed execution or formal review")
	}
	var activity string
	if err = f.s.DB.QueryRow(`SELECT title FROM suma_activity WHERE kind='task_conversation.posted'`).Scan(&activity); err != nil || activity != "Task conversation message" || strings.Contains(activity, req.Body) {
		t.Fatal("general activity leaked message content", activity, err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 401)
}

func TestTaskConversationCurrentAuthorityAndAcceptedTask(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f, ctx := h.f, context.Background()
	a := h.run(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	current := f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+h.task.ID+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": a.RunID, "decision": "accepted", "reason": "Explicit acceptance before a follow-up collaboration note"}), 200)
	accepted, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, h.task.ID)
	if err != nil || !accepted.Delivery.FormalAcceptance {
		t.Fatal(err)
	}
	request := taskConversationRequest(f, h.task.ID, accepted.Task.Version, "Follow-up note on the accepted delivery")
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, request)
	if err != nil || message.RunID != a.RunID || message.TaskVersion != accepted.Task.Version {
		t.Fatal("accepted task notes were rejected or rebound the original run", message, err)
	}
	after, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, h.task.ID)
	if err != nil || stringify(after.Task) != stringify(accepted.Task) || stringify(after.Delivery) != stringify(accepted.Delivery) || stringify(after.Quality) != stringify(accepted.Quality) {
		t.Fatal("collaboration altered accepted delivery facts", after, err)
	}
	_, err = f.s.AppendTaskConversation(ctx, toolActorForTest(t, f, h.token), f.project, request)
	requireAuthorityStatus(t, err, 403)
	invitation := governanceInvite(f, "conversation-viewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invitation, "conversation-viewer@example.test")
	viewer := domainAActor(t, f.s, f.cookie.Value)
	page, err := f.s.ListTaskConversation(ctx, viewer, f.project, h.task.ID, 20, 0)
	if err != nil || page.CanPost || page.Total != 1 {
		t.Fatal("viewer read or can_post hint expanded write authority", page, err)
	}
	request.IdempotencyKey = newID()
	_, err = f.s.AppendTaskConversation(ctx, viewer, f.project, request)
	requireAuthorityStatus(t, err, 403)
}

func TestTaskConversationConcurrentAppendAndHistory(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Concurrent collaborators"}), 201)["id"].(string)
	type result struct {
		message TaskConversationMessage
		err     error
	}
	results, start := make(chan result, 8), make(chan struct{})
	var done sync.WaitGroup
	for i := 0; i < 8; i++ {
		req := taskConversationRequest(f, task, 1, fmt.Sprintf("Contributor note %d", i))
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
			results <- result{message, err}
		}()
	}
	close(start)
	done.Wait()
	close(results)
	sequences := []int{}
	for actual := range results {
		if actual.err != nil {
			t.Fatal(actual.err)
		}
		sequences = append(sequences, int(actual.message.Sequence))
	}
	sort.Ints(sequences)
	for i, seq := range sequences {
		if seq != i+1 {
			t.Fatal("concurrent append overwrote or reused a sequence", sequences)
		}
	}
	latest, err := f.s.ListTaskConversation(ctx, f.actor, f.project, task, 3, 0)
	if err != nil || latest.Total != 8 || len(latest.Items) != 3 || latest.Items[0].Sequence != 6 || latest.Items[2].Sequence != 8 || latest.NextBefore != 6 {
		t.Fatal("latest bounded window was not chronological", latest, err)
	}
	// A new append must not shift an older window or duplicate its boundary.
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, task, 1, "New note while reading history")); err != nil {
		t.Fatal(err)
	}
	older, err := f.s.ListTaskConversation(ctx, f.actor, f.project, task, 3, latest.NextBefore)
	if err != nil || older.Total != 9 || older.Items[0].Sequence != 3 || older.Items[2].Sequence != 5 || older.NextBefore != 3 {
		t.Fatal("append shifted exclusive history boundary", older, err)
	}
	first, err := f.s.ListTaskConversation(ctx, f.actor, f.project, task, 3, older.NextBefore)
	if err != nil || len(first.Items) != 2 || first.Items[0].Sequence != 1 || first.Items[1].Sequence != 2 || first.NextBefore != 0 {
		t.Fatal("oldest page lost messages or invented more history", first, err)
	}
}

func TestTaskConversationStrictHTTPAndScope(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "HTTP collaboration"}), 201)["id"].(string)
	req := taskConversationRequest(f, task, 1, "Human JSON note")
	posted := f.call("POST", "/task-conversations", req, 201)
	if posted["actor_kind"] != "human" || posted["author_id"] != f.user.ID || posted["sequence"] != float64(1) {
		t.Fatal("HTTP posting let caller own attribution", posted)
	}
	query := "/task-conversations?project_id=" + f.project + "&task_id=" + task
	f.call("GET", query, nil, 200)
	for _, suffix := range []string{"&before=0", "&before=01", "&before=1e1", "&before=9007199254740992", "&limit=0", "&limit=01", "&limit=51", "&limit=1&limit=2", "&task_id=" + task, "&unexpected=x"} {
		f.call("GET", query+suffix, nil, 400)
	}
	// Exercise the original raw decoder: duplicate, case alias, role forgery,
	// null, fractional numbers and malformed Unicode cannot be hidden by a map.
	raw, _ := json.Marshal(req)
	for _, body := range []string{
		strings.TrimSuffix(string(raw), "}") + `,"body":"duplicate"}`,
		strings.Replace(string(raw), `"body":`, `"Body":`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"author_id":"forged","actor_kind":"assistant"}`,
		strings.Replace(string(raw), `"body":"Human JSON note"`, `"body":null`, 1),
		strings.Replace(string(raw), `"expected_task_version":1`, `"expected_task_version":1.0`, 1),
		strings.Replace(string(raw), `"expected_task_version":1`, `"expected_task_version":2147483647`, 1),
		strings.Replace(string(raw), `Human JSON note`, `\ud800`, 1),
	} {
		r := httptest.NewRequest("POST", "http://suma.test/api/v1/task-conversations", bytes.NewBufferString(body))
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("closed body accepted an ambiguous/forged message", w.Code, w.Body.String(), body)
		}
	}
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	req.Body, req.IdempotencyKey = "API-attributed note", newID()
	viaAPI := f.bearer("POST", "/task-conversations", issued.Token, req, 201)
	if viaAPI["actor_kind"] != "api" || viaAPI["author_id"] != f.user.ID {
		t.Fatal("API credential impersonated human/assistant attribution", viaAPI)
	}
	f.bearer("GET", strings.Replace(query, f.project, newID(), 1), issued.Token, nil, 403)
	req.ProjectID, req.IdempotencyKey = newID(), newID()
	f.bearer("POST", "/task-conversations", issued.Token, req, 403)
	if domainACount(t, f.s, "suma_task_conversation_messages") != 2 {
		t.Fatal("rejected or duplicate operations changed the conversation")
	}
}
