package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTaskConversationMentionTargetsPagedCurrentScopeAndClosedHTTP(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	page, err := f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, TaskConversationMentionTargetQuery{Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("complete canonical directory with bounded first page: %+v %v", page, err)
	}
	next, err := f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, TaskConversationMentionTargetQuery{Limit: 1, Cursor: page.NextCursor})
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ResourceID == page.Items[0].ResourceID || next.NextCursor != "" {
		t.Fatalf("directory did not advance deterministically: %+v %v", next, err)
	}
	_, err = f.s.ListTaskConversationMentionTargets(ctx, h.receiver, f.project, h.task.ID, TaskConversationMentionTargetQuery{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	_, err = f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, TaskConversationMentionTargetQuery{Limit: 1, Q: "changed", Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", h.owner.ID, map[string]any{"title": "Literal %_! specialist", "expected_version": h.owner.Version, "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	literal, err := f.s.ListTaskConversationMentionTargets(ctx, f.actor, f.project, h.task.ID, TaskConversationMentionTargetQuery{Q: "%_!"})
	if err != nil || literal.Total != 1 || len(literal.Items) != 1 || literal.Items[0].ResourceID != h.owner.ID {
		t.Fatalf("search treated literal punctuation as wildcard: %+v %v", literal, err)
	}
	base := "/task-conversation-mention-targets?project_id=" + f.project + "&task_id=" + h.task.ID
	for _, suffix := range []string{"&user_id=other", "&limit=0", "&limit=51", "&limit=01", "&q=a&q=b", "&q=" + url.QueryEscape("bad\u0085query"), "&resource_ids=" + h.owner.ID + "," + h.owner.ID, "&resource_ids=" + h.owner.ID + "&q=Literal", "&resource_ids=" + h.owner.ID + "&cursor=token"} {
		f.call("GET", base+suffix, nil, 400)
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1"+base, nil)
		request.Header.Set("Authorization", "Bearer "+issued.Token)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		want := 403
		if scope == "api" {
			want = 200
		}
		if response.Code != want {
			t.Fatalf("%s mention target scope: %d %s", scope, response.Code, response.Body.String())
		}
	}
}

func TestTaskConversationMentionsHTTPClosedInputAndExactDetailPrivacy(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	base := map[string]any{"project_id": f.project, "task_id": h.task.ID, "expected_task_version": h.task.Version, "body": "Author-controlled plain text", "idempotency_key": newID()}
	for _, field := range []string{"mentions", "mention_user_ids", "MentionResourceIDs", "author_id", "actor_kind"} {
		body := map[string]any{}
		for key, value := range base {
			body[key] = value
		}
		body[field] = []string{h.resource.ID}
		f.call("POST", "/task-conversations", body, 400)
	}
	for _, value := range []any{nil, "person", []any{h.resource.ID, h.resource.ID}, []any{map[string]any{"resource_id": h.resource.ID}}, []any{h.resource.ID, 3}} {
		body := map[string]any{}
		for key, original := range base {
			body[key] = original
		}
		body["mention_resource_ids"] = value
		f.call("POST", "/task-conversations", body, 400)
	}
	req := taskConversationRequest(f, h.task.ID, h.task.Version, "Private exact discussion anchor")
	req.MentionResourceIDs = []string{h.resource.ID}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	path := "/task-conversation-messages/" + message.ID + "?project_id=" + f.project + "&task_id=" + h.task.ID
	detail := f.call("GET", path, nil, 200)
	if detail["message"].(map[string]any)["id"] != message.ID {
		t.Fatal("exact message route returned another source", detail)
	}
	f.call("GET", path+"&message_id=another", nil, 400)
	other, err := f.s.Create(ctx, f.actor, f.project, "tasks", map[string]any{"title": "Unrelated task", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/task-conversation-messages/"+message.ID+"?project_id="+f.project+"&task_id="+other.ID, nil)
	request.AddCookie(f.cookie)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != 404 || strings.Contains(response.Body.String(), req.Body) || strings.Contains(response.Body.String(), h.resource.Title) {
		t.Fatal("foreign task anchor leaked message or recipient", response.Code, response.Body.String())
	}
	for _, scope := range []string{"runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		request = httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		request.Header.Set("Authorization", "Bearer "+issued.Token)
		response = httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		if response.Code != 403 {
			t.Fatal("nonhuman identity read a human discussion anchor", scope, response.Code)
		}
	}
}

func TestTaskConversationMentionInboxCorruptionRetainsPrivateEventWithoutSource(t *testing.T) {
	for _, corruption := range []string{"missing_relation", "missing_message", "assistant_origin", "wrong_receiver"} {
		t.Run(corruption, func(t *testing.T) {
			h, ctx := newHandoffFixture(t), context.Background()
			f := h.f
			req := taskConversationRequest(f, h.task.ID, h.task.Version, "Original body must not be cached in the notification")
			req.MentionResourceIDs = []string{h.resource.ID}
			message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, req)
			if err != nil {
				t.Fatal(err)
			}
			page := inboxPage(t, f, h.receiver, "inbox", 20, "")
			if page.Total != 1 || len(page.Items) != 1 || !page.Items[0].SourceAvailable || page.Items[0].MessageID != message.ID {
				t.Fatal("test did not establish real mention source", page)
			}
			item := page.Items[0]
			switch corruption {
			case "missing_relation":
				_, err = f.s.DB.Exec(`DELETE FROM suma_task_conversation_mentions WHERE notification_id=?`, item.ID)
			case "missing_message":
				_, err = f.s.DB.Exec(`DELETE FROM suma_task_conversation_messages WHERE id=?`, message.ID)
			case "assistant_origin":
				_, err = f.s.DB.Exec(`UPDATE suma_task_conversation_messages SET actor_kind='assistant' WHERE id=?`, message.ID)
			case "wrong_receiver":
				_, err = f.s.DB.Exec(`UPDATE suma_task_conversation_mentions SET recipient_user_id=? WHERE notification_id=?`, f.user.ID, item.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			page = inboxPage(t, f, h.receiver, "inbox", 20, "")
			if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != item.ID || page.Items[0].SourceAvailable || page.Items[0].MessageID != "" || page.Items[0].CurrentTask != nil || page.Items[0].AssignmentCurrent {
				t.Fatalf("damaged source was actionable or private event was discarded: %+v", page)
			}
			raw, _ := json.Marshal(page)
			if strings.Contains(string(raw), req.Body) {
				t.Fatal("inbox leaked a historical discussion body")
			}
		})
	}
}
