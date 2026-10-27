package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func knowledgeFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	if err := f.s.InitializeProjectKnowledge(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f
}
func knowledgeDraft(f *fixture, kind, title, content string) ProjectKnowledgeWriteRequest {
	return ProjectKnowledgeWriteRequest{ProjectID: f.project, Kind: kind, Title: title, Content: content, Sources: []ProjectKnowledgeSourceInput{}, IdempotencyKey: newID()}
}
func TestProjectKnowledgePublicationPreservesApprovedRevisionDuringDraftEdit(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "skill", "Safe migration", "Review a migration before executing it."))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(published.Published)
	edit := knowledgeDraft(f, "skill", "Safer migration", "Also review the rollback evidence.")
	edit.ExpectedVersion = published.Version
	updated, err := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := json.Marshal(updated.Published)
	if updated.PublishedTitle != published.Published.Title || updated.PublishedSummary != published.Published.Summary || string(now) != string(original) || !updated.HasDraft || updated.PublishedRevision != published.PublishedRevision || updated.Latest.Revision <= published.Latest.Revision {
		t.Fatalf("draft rewrote approval: %#v", updated)
	}
	ref := ProjectKnowledgeReference{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.Published.SHA256}
	var items []ProjectKnowledgeContextItem
	err = f.s.withCurrentRead(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		items, e = f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{ref})
		return e
	})
	if err != nil || len(items) != 1 || items[0].Content != "Review a migration before executing it." {
		t.Fatalf("approved context changed: %v %v", items, err)
	}
	retired, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "retire", ExpectedVersion: updated.Version, IdempotencyKey: newID()})
	if err != nil || retired.PublishedRevision != 0 {
		t.Fatalf("retire: %v %v", retired, err)
	}
	err = f.s.withCurrentRead(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		_, e := f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{ref})
		return e
	})
	requireAuthorityStatus(t, err, 409)
	history, err := f.s.ProjectKnowledgeRevisions(ctx, f.actor, f.project, published.ID, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if history.Total != 4 || len(history.Items) != 4 {
		t.Fatalf("immutable history missing: %v", history)
	}
	found := false
	for _, item := range history.Items {
		if item.Revision == published.PublishedRevision {
			stored, _ := json.Marshal(item)
			if string(stored) != string(original) {
				t.Fatal("retirement rewrote historical approved revision")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("published revision is not accessible in history")
	}

}

func TestProjectKnowledgeExtractionChecksQuoteSourceVersionAndPublication(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	doc := f.call("POST", "/documents", f.body(map[string]any{"title": "Migration notes", "description": "Use a staged rollout. Verify rollback first."}), 201)
	source := ProjectKnowledgeSourceInput{Kind: "documents", ID: doc["id"].(string), Version: 1, Quote: "Verify rollback first."}
	req := ProjectKnowledgeExtractionRequest{ProjectID: f.project, Kind: "memory", Sources: []ProjectKnowledgeSourceInput{source}, IdempotencyKey: newID()}
	created, err := f.s.ExtractProjectKnowledge(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if created.Latest.Method != "extractive" || !strings.Contains(created.Latest.Content, source.Quote) || len(created.Latest.Sources) != 1 {
		t.Fatalf("extractive provenance missing: %#v", created)
	}
	bad := req
	bad.IdempotencyKey = newID()
	bad.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: source.ID, Version: 1, Quote: "Invented output"}}
	_, err = f.s.ExtractProjectKnowledge(ctx, f.actor, f.project, bad)
	requireAuthorityStatus(t, err, 400)
	f.call("PATCH", "/documents/"+source.ID, f.body(map[string]any{"expected_version": 1, "description": "The rollout policy changed."}), 200)
	_, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
}

func TestProjectKnowledgeCurrentActorPrecedesFreshAndReplay(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	req := knowledgeDraft(f, "spec", "Required checks", "Independent tests and review.")
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("idempotency: %v %v", replay, err)
	}
	api, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	apiActor, err := f.s.AuthenticateActor(ctx, api.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateProjectKnowledge(ctx, apiActor, f.project, knowledgeDraft(f, "memory", "API draft", "Scoped API write."))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandProjectKnowledge(ctx, apiActor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 401)
	req.IdempotencyKey = newID()
	_, err = f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 401)
	_, err = f.s.GetProjectKnowledge(ctx, f.actor, f.project, created.ID)
	requireAuthorityStatus(t, err, 401)
}

func TestProjectKnowledgeListCursorAndAgentPublicationBoundary(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	first, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "memory", "Published choice", "Approved instructions."))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, first.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "memory", "Draft choice", "Unapproved instructions."))
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeListQuery{Kind: "memory", Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("page: %v %v", page, err)
	}
	_, err = f.s.ListProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeListQuery{Kind: "skill", Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "agent", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetProjectKnowledge(ctx, agent, f.project, draft.ID)
	requireAuthorityStatus(t, err, 403)
	err = f.s.withCurrentRead(ctx, agent, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeAgentRead, func(auth *authorizedCommand) error {
		_, e := f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.Published.SHA256}})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.withCurrentRead(ctx, agent, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeAgentRead, func(auth *authorizedCommand) error {
		_, e := f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{{ID: draft.ID, Revision: draft.LatestRevision, SHA256: draft.Latest.SHA256}})
		return e
	})
	requireAuthorityStatus(t, err, 409)
}

func TestProjectKnowledgeHistoryAndReceiptEligibilityRemainVersionBound(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	doc := f.call("POST", "/documents", f.body(map[string]any{"title": "Source", "description": "Original policy."}), 201)
	req := knowledgeDraft(f, "spec", "Policy", "Apply original policy.")
	req.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc["id"].(string), Version: 1}}
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	command := ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: 1, IdempotencyKey: newID()}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	history, err := f.s.ProjectKnowledgeRevisions(ctx, f.actor, f.project, created.ID, 1, "")
	if err != nil || history.Total != 2 || len(history.Items) != 1 || history.NextCursor == "" {
		t.Fatalf("history paging: %v %v", history, err)
	}
	older, err := f.s.ProjectKnowledgeRevisions(ctx, f.actor, f.project, created.ID, 1, history.NextCursor)
	if err != nil || len(older.Items) != 1 || older.Items[0].State != "draft" {
		t.Fatalf("old history: %v %v", older, err)
	}
	another, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "memory", "Other", "Other body."))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ProjectKnowledgeRevisions(ctx, f.actor, f.project, another.ID, 1, history.NextCursor)
	requireAuthorityStatus(t, err, 400)
	f.call("PATCH", "/documents/"+doc["id"].(string), f.body(map[string]any{"expected_version": 1, "description": "New policy."}), 200)
	replay, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Latest.SHA256 != published.Latest.SHA256 || replay.PublishedEligible || replay.Eligibility != "source_changed" {
		t.Fatalf("historical receipt claimed current source: %#v", replay)
	}
	ref := ProjectKnowledgeReference{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.Published.SHA256}
	err = f.s.withCurrentRead(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		_, e := f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{ref})
		return e
	})
	requireAuthorityStatus(t, err, 409)
}

func TestProjectKnowledgeHTTPRejectsUntrustedFieldsAndNonIntegerVersions(t *testing.T) {
	f := knowledgeFixture(t)
	request := knowledgeDraft(f, "memory", "HTTP memory", "Keep reviewed provenance.")
	raw, _ := json.Marshal(request)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	created := f.call("POST", "/project-knowledge", body, 201)
	bad := func() map[string]any {
		copy := map[string]any{}
		for key, value := range body {
			copy[key] = value
		}
		copy["idempotency_key"] = newID()
		return copy
	}
	extra := bad()
	extra["approved_by"] = f.user.ID
	f.call("POST", "/project-knowledge", extra, 400)
	missing := bad()
	delete(missing, "expected_version")
	f.call("POST", "/project-knowledge", missing, 400)
	exponent := bad()
	exponent["expected_version"] = json.Number("1e0")
	f.call("PATCH", "/project-knowledge/"+created["id"].(string), exponent, 400)
	fraction := bad()
	fraction["sources"] = []any{map[string]any{"kind": "documents", "id": newID(), "version": json.Number("1.0000000000000001")}}
	f.call("POST", "/project-knowledge", fraction, 400)
	fields := bad()
	fields["sources"] = []any{map[string]any{"kind": "documents", "id": newID(), "version": 1, "sha256": strings.Repeat("a", 64)}}
	f.call("POST", "/project-knowledge", fields, 400)
	f.call("GET", "/project-knowledge/"+created["id"].(string)+"/revisions?project_id="+f.project+"&kind=", nil, 400)
	f.call("GET", "/project-knowledge?project_id="+f.project+"&limit=0", nil, 400)
	f.call("GET", "/project-knowledge?project_id="+f.project+"&limit=101", nil, 400)
	f.call("GET", "/project-knowledge?project_id="+f.project+"&kind=memory&kind=skill", nil, 400)
	f.call("GET", "/project-knowledge/"+created["id"].(string)+"?project_id="+f.project+"&q=private", nil, 400)
}

func TestProjectKnowledgeApprovalReceiptRechecksCurrentReviewerRole(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	reviewer := invitedReviewer(t, f)
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "spec", "Review standard", "Require independent evidence."))
	if err != nil {
		t.Fatal(err)
	}
	command := ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: 1, IdempotencyKey: newID()}
	published, err := f.s.CommandProjectKnowledge(ctx, reviewer, f.project, created.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": reviewer.Projection().ID, "role": "viewer", "expected_version": float64(1), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandProjectKnowledge(ctx, reviewer, f.project, created.ID, command)
	requireAuthorityStatus(t, err, 403)
	command.ExpectedVersion = published.Version
	command.IdempotencyKey = newID()
	command.Action = "retire"
	_, err = f.s.CommandProjectKnowledge(ctx, reviewer, f.project, created.ID, command)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetProjectKnowledge(ctx, reviewer, f.project, created.ID)
	if err != nil {
		t.Fatal("viewer lost permitted read", err)
	}
	_, err = f.s.CreateProjectKnowledge(ctx, reviewer, f.project, knowledgeDraft(f, "memory", "Forbidden", "No write grant."))
	requireAuthorityStatus(t, err, 403)
}

func TestProjectKnowledgeContextKeepsProvenanceWithoutDuplicateQuotes(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	body := strings.Repeat("x", 16000)
	source := f.call("POST", "/documents", f.body(map[string]any{"title": "Large source", "description": body}), 201)
	draft, err := f.s.ExtractProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeExtractionRequest{ProjectID: f.project, Kind: "memory", Sources: []ProjectKnowledgeSourceInput{{Kind: "documents", ID: source["id"].(string), Version: 1}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if published.Published.Sources[0].Quote != body {
		t.Fatal("review evidence lost full original quote")
	}
	var resolved []ProjectKnowledgeContextItem
	err = f.s.withCurrentRead(ctx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		resolved, e = f.s.resolvePublishedKnowledgeTx(auth.ctx, auth, f.project, []ProjectKnowledgeReference{{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.Published.SHA256}})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), body) != 1 || strings.Contains(string(encoded), `"quote":`) || len(encoded) > 18000 {
		t.Fatalf("consumption duplicated source or dropped body: %d bytes", len(encoded))
	}
	if len(resolved) != 1 || len(resolved[0].Sources) != 1 || resolved[0].Sources[0].SHA256 != published.Published.Sources[0].SHA256 || resolved[0].SHA256 != published.Published.SHA256 {
		t.Fatal("consumption provenance changed")
	}
}

func TestProjectKnowledgeLatestAndPublishedMetadataProjection(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	request := knowledgeDraft(f, "memory", "Approved title", "Approved content.")
	request.Summary = "Approved summary"
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	check := func(stage, title, summary, publishedTitle, publishedSummary string) {
		t.Helper()
		detail, e := f.s.GetProjectKnowledge(ctx, f.actor, f.project, created.ID)
		if e != nil {
			t.Fatal(e)
		}
		page, e := f.s.ListProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeListQuery{Query: title})
		if e != nil || len(page.Items) != 1 {
			t.Fatalf("%s query: %v %v", stage, page, e)
		}
		for _, item := range []ProjectKnowledgeSummary{detail.ProjectKnowledgeSummary, page.Items[0]} {
			if item.Title != title || item.Summary != summary || item.PublishedTitle != publishedTitle || item.PublishedSummary != publishedSummary {
				t.Errorf("%s latest/published metadata lost: %#v", stage, item)
			}
		}
	}
	check("create", "Approved title", "Approved summary", "", "")
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	check("publish", "Approved title", "Approved summary", "Approved title", "Approved summary")
	request.Title = "New draft title"
	request.Summary = "New draft summary"
	request.Content = "Unapproved draft content."
	request.ExpectedVersion = published.Version
	request.IdempotencyKey = newID()
	_, err = f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, created.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	check("edit", "New draft title", "New draft summary", "Approved title", "Approved summary")
}
