package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProjectKnowledgeRevisionDetailIsExactAfterDraftAndRetirement(t *testing.T) {
	f, ctx := knowledgeFixture(t), context.Background()
	published := publishedConversationKnowledge(t, f, "Original title")
	edit := knowledgeDraft(f, "spec", "New draft title", "A later draft is not the original publication.")
	edit.ExpectedVersion = published.Version
	updated, err := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: updated.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	before := domainACount(t, f.s, "suma_operations")
	exact, err := f.s.GetProjectKnowledgeRevision(ctx, f.actor, f.project, published.ID, published.PublishedRevision)
	if err != nil || exact.ProjectID != f.project || exact.KnowledgeID != published.ID || stringify(exact.Snapshot) != stringify(*published.Published) {
		t.Fatal("accurate revision was replaced with latest or hidden by retirement", exact, err)
	}
	if domainACount(t, f.s, "suma_operations") != before {
		t.Fatal("revision read created an operation")
	}
	_, err = f.s.GetProjectKnowledgeRevision(ctx, f.actor, f.project, published.ID, 9999)
	requireAuthorityStatus(t, err, 404)
	bad := *published.Published
	bad.Content = "Modified without the original digest"
	if _, err = f.s.DB.Exec(`UPDATE suma_project_knowledge_revisions SET snapshot=? WHERE tenant_id=? AND project_id=? AND knowledge_id=? AND revision=?`, stringify(bad), f.user.TenantID, f.project, published.ID, published.PublishedRevision); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetProjectKnowledgeRevision(ctx, f.actor, f.project, published.ID, published.PublishedRevision)
	requireAuthorityStatus(t, err, 409)
}

func TestProjectKnowledgeRevisionDetailHTTPClosedScopeAndCurrentAuthority(t *testing.T) {
	h, ctx := newHandoffFixture(t), context.Background()
	f := h.f
	published := publishedConversationKnowledge(t, f, "Private exact source")
	path := fmt.Sprintf("/project-knowledge/%s/revisions/%d?project_id=%s", published.ID, published.PublishedRevision, f.project)
	value := f.call("GET", path, nil, 200)
	if value["knowledge_id"] != published.ID || value["snapshot"].(map[string]any)["sha256"] != published.PublishedSHA256 {
		t.Fatal("wrong deep source", value)
	}
	for _, suffix := range []string{"&limit=20", "&project_id=" + f.project, "&revision=2"} {
		f.call("GET", path+suffix, nil, 400)
	}
	for _, revision := range []string{"0", "01", "1e0", "-1", "2147483648"} {
		f.call("GET", "/project-knowledge/"+published.ID+"/revisions/"+revision+"?project_id="+f.project, nil, 400)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Unrelated exact source project", "idempotency_key": newID()}, 201)["id"].(string)
	_, err := f.s.GetProjectKnowledgeRevision(ctx, f.actor, other, published.ID, published.PublishedRevision)
	requireAuthorityStatus(t, err, 404)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetProjectKnowledgeRevision(ctx, h.receiver, f.project, published.ID, published.PublishedRevision)
	requireAuthorityStatus(t, err, 403)
	for _, scope := range []string{"runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		r.Header.Set("Authorization", "Bearer "+issued.Token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 403 || strings.Contains(w.Body.String(), published.Published.Content) {
			t.Fatal("nonhuman scope read historical knowledge through public namespace", scope, w.Code, w.Body.String())
		}
	}
}
