package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestTaskConversationKnowledgeAppendHTTPNestedClosedPinsAndOriginalRecovery(t *testing.T) {
	f, ctx := knowledgeFixture(t), context.Background()
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Explicit publication discussion"}), 201)["id"].(string)
	published := publishedConversationKnowledge(t, f, "Approved HTTP source")
	req := taskConversationRequest(f, task, 1, "  Keep the selected publication and original body\n原文  ")
	req.Knowledge = []ProjectKnowledgeReference{conversationKnowledgePin(published)}
	posted := f.call("POST", "/task-conversations", req, 201)
	metadata, ok := posted["knowledge"].([]any)
	if !ok || len(metadata) != 1 || posted["body"] != req.Body || posted["actor_kind"] != "human" || posted["author_id"] != f.user.ID {
		t.Fatal("public append lost exact pins/body or server attribution", posted)
	}
	pin, ok := metadata[0].(map[string]any)
	if !ok || len(pin) != 5 || pin["id"] != published.ID || pin["revision"] != float64(published.PublishedRevision) || pin["sha256"] != published.PublishedSHA256 || pin["title_at_post"] != published.Published.Title || pin["kind_at_post"] != published.Kind {
		t.Fatal("public receipt did not return server-resolved immutable metadata", metadata)
	}
	if _, exists := pin["content"]; exists {
		t.Fatal("message receipt leaked the publication body")
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	pinRaw, err := json.Marshal(req.Knowledge[0])
	if err != nil {
		t.Fatal(err)
	}
	base, originalPin := string(raw), string(pinRaw)
	revisionToken := `"revision":` + strconv.Itoa(published.PublishedRevision)
	badBodies := []struct{ name, body string }{
		{"null array", strings.Replace(base, `"knowledge":[`+originalPin+`]`, `"knowledge":null`, 1)},
		{"duplicate ID", strings.Replace(base, `"knowledge":[`+originalPin+`]`, `"knowledge":[`+originalPin+`,`+originalPin+`]`, 1)},
		{"revision string", strings.Replace(base, revisionToken, `"revision":"`+strconv.Itoa(published.PublishedRevision)+`"`, 1)},
		{"revision fraction", strings.Replace(base, revisionToken, revisionToken+`.0`, 1)},
		{"revision exponent", strings.Replace(base, revisionToken, revisionToken+`e0`, 1)},
		{"bad digest", strings.Replace(base, published.PublishedSHA256, strings.ToUpper(published.PublishedSHA256), 1)},
		{"nested duplicate key", strings.Replace(base, originalPin, strings.TrimSuffix(originalPin, "}")+`,"revision":`+strconv.Itoa(published.PublishedRevision)+`}`, 1)},
	}
	for _, field := range []string{"title", "content", "role"} {
		badPin := strings.TrimSuffix(originalPin, "}") + `,"` + field + `":"caller-forged snapshot or role"}`
		badBodies = append(badBodies, struct{ name, body string }{"nested extra " + field, strings.Replace(base, originalPin, badPin, 1)})
	}
	// Use the existing fixture's raw HTTP pattern, not a pre-decoded map:
	// lexical numbers, duplicate keys and custom-Unmarshal nested fields must
	// all reach the same production woc.Decode boundary unchanged.
	for _, invalid := range badBodies {
		if invalid.body == base {
			t.Fatal("malformed fixture did not change its original pin", invalid.name)
		}
		r := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/task-conversations", bytes.NewBufferString(invalid.body))
		r.AddCookie(f.cookie)
		r.Header.Set("X-Suma-Expected-User", f.view.ID)
		r.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatal("closed nested knowledge pin was accepted", invalid.name, w.Code, w.Body.String())
		}
	}

	other := f.call("POST", "/projects", map[string]any{"title": "Other publication project", "idempotency_key": newID()}, 201)["id"].(string)
	draft := knowledgeDraft(f, "spec", "Foreign publication", "This exact content belongs to another project.")
	draft.ProjectID = other
	foreign, err := f.s.CreateProjectKnowledge(ctx, f.actor, other, draft)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err = f.s.CommandProjectKnowledge(ctx, f.actor, other, foreign.ID, ProjectKnowledgeCommandRequest{ProjectID: other, ExpectedVersion: foreign.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cross := req
	cross.IdempotencyKey, cross.Knowledge = newID(), []ProjectKnowledgeReference{conversationKnowledgePin(foreign)}
	f.call("POST", "/task-conversations", cross, 404)
	for _, scope := range []string{"runner", "agent", "api"} {
		issued, issueErr := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		body := req
		body.IdempotencyKey = newID()
		if scope == "api" {
			body.ProjectID = other
		}
		f.bearer("POST", "/task-conversations", issued.Token, body, 403)
	}
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: published.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	replayed := f.call("POST", "/task-conversations", req, 201)
	if stringify(replayed) != stringify(posted) || domainACount(t, f.s, "suma_task_conversation_messages") != 1 || domainACount(t, f.s, "suma_task_conversation_knowledge") != 1 {
		t.Fatal("correct original HTTP operation re-resolved retirement or duplicated its message", replayed)
	}
}
