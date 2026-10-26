package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type skillFixture struct {
	f         *fixture
	target    string
	source    Record
	knowledge ProjectKnowledgeDetail
}

func newSkillFixture(t *testing.T) *skillFixture {
	t.Helper()
	f := knowledgeFixture(t)
	ctx := context.Background()
	target, e := f.s.CreateProject(ctx, f.actor, map[string]any{"title": "Skill receiving project", "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	source, e := f.s.Create(ctx, f.actor, f.project, "documents", f.body(map[string]any{"title": "PRIVATE_SOURCE_TITLE", "description": "PRIVATE_SOURCE_QUOTE never goes into the catalog response"}))
	if e != nil {
		t.Fatal(e)
	}
	req := knowledgeDraft(f, "skill", "Reviewed shared workflow", "Reviewed reusable instructions. 原文 is stable.")
	req.Summary = "A reusable organization skill"
	req.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: source.ID, Version: source.Version, Quote: "PRIVATE_SOURCE_QUOTE"}}
	draft, e := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, req)
	if e != nil {
		t.Fatal(e)
	}
	published, e := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: draft.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return &skillFixture{f: f, target: target.ID, source: source, knowledge: published}
}
func (h *skillFixture) request() SkillCatalogWriteRequest {
	return SkillCatalogWriteRequest{ProjectID: h.f.project, KnowledgeID: h.knowledge.ID, ExpectedKnowledgeVersion: h.knowledge.Version, PublishedRevision: h.knowledge.PublishedRevision, PublishedSHA256: h.knowledge.PublishedSHA256, Domain: "engineering", Scenarios: []string{"review"}, Tags: []string{"safe"}, Reason: "Explicit organization sharing review", AcknowledgeSharing: true, IdempotencyKey: newID()}
}
func (h *skillFixture) share(t *testing.T) SkillCatalogDetail {
	t.Helper()
	out, e := h.f.s.CreateSkillCatalog(context.Background(), h.f.actor, h.request())
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func (h *skillFixture) adopt(c SkillCatalogDetail) SkillCatalogAdoptRequest {
	return SkillCatalogAdoptRequest{ProjectID: h.target, ExpectedVersion: c.Version, OriginSHA256: c.Origin.SHA256, Reason: "Adopt reviewed text as an independently reviewed project draft", AcknowledgeDraft: true, IdempotencyKey: newID()}
}
func skillHasAction(actions []string, key string) bool {
	for _, v := range actions {
		if v == key {
			return true
		}
	}
	return false
}

func TestSkillCatalogPublishedSnapshotAndSourcePrivacy(t *testing.T) {
	h := newSkillFixture(t)
	ctx := context.Background()
	req := h.request()
	req.AcknowledgeSharing = false
	if _, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, req); e == nil {
		t.Fatal("sharing without explicit confirmation")
	}
	req = h.request()
	shared, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, req)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, req)
	if e != nil || replay.ID != shared.ID || replay.Version != 1 {
		t.Fatal(replay, e)
	}
	edit := knowledgeDraft(h.f, "skill", "Unreviewed draft title", "UNREVIEWED_DRAFT_PRIVATE")
	edit.ExpectedVersion = h.knowledge.Version
	edit.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: h.source.ID, Version: h.source.Version, Quote: "PRIVATE_SOURCE_QUOTE"}}
	if _, e = h.f.s.UpdateProjectKnowledge(ctx, h.f.actor, h.f.project, h.knowledge.ID, edit); e != nil {
		t.Fatal(e)
	}
	got, e := h.f.s.GetSkillCatalog(ctx, h.f.actor, h.target, shared.ID)
	if e != nil || !got.SourceCurrent || got.Content != h.knowledge.Published.Content || got.Title != h.knowledge.Published.Title {
		t.Fatal("source draft replaced shared publication", got, e)
	}
	raw := stringify(got)
	for _, private := range []string{"PRIVATE_SOURCE_QUOTE", "PRIVATE_SOURCE_TITLE", "UNREVIEWED_DRAFT_PRIVATE", `"sources"`} {
		if strings.Contains(raw, private) {
			t.Fatal("catalog leaked nonshared source material", private)
		}
	}
	if e = checkSkillCatalogReference(got, got.Version+1, got.Origin.SHA256); e == nil {
		t.Fatal("fixed get substituted a newer version")
	}
}

func TestSkillCatalogTargetReviewerScopedAPIAndReplayAuthority(t *testing.T) {
	h := newSkillFixture(t)
	ctx := context.Background()
	shared := h.share(t)
	ownerCookie := h.f.cookie
	invitation := governanceInvite(h.f, "skill-reviewer@example.test", "member", []any{map[string]any{"project_id": h.target, "role": "reviewer"}})
	governanceAccept(h.f, invitation, "skill-reviewer@example.test")
	reviewer, e := h.f.s.AuthenticateActor(ctx, h.f.cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.GetProjectKnowledge(ctx, reviewer, h.f.project, h.knowledge.ID); e == nil {
		t.Fatal("target reviewer gained source project permission")
	}
	got, e := h.f.s.GetSkillCatalog(ctx, reviewer, h.target, shared.ID)
	if e != nil || got.CanManage || !got.CanAdopt || got.Content != shared.Content {
		t.Fatal(got, e)
	}
	if _, e = h.f.s.CreateSkillCatalog(ctx, reviewer, h.request()); e == nil {
		t.Fatal("ordinary member promoted organization content")
	}
	req := h.adopt(shared)
	adopted, e := h.f.s.AdoptSkillCatalog(ctx, reviewer, shared.ID, req)
	if e != nil || adopted.State != "draft" || adopted.Published != nil || adopted.Latest.Method != "catalog_adoption" || len(adopted.Latest.Sources) != 0 || adopted.CatalogAdoption == nil || adopted.CatalogAdoption.CatalogVersion != shared.Version {
		t.Fatal(adopted, e)
	}
	again, e := h.f.s.AdoptSkillCatalog(ctx, reviewer, shared.ID, req)
	if e != nil || again.ID != adopted.ID {
		t.Fatal("adoption replay duplicated draft", again, e)
	}
	api, e := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.target, Scope: "api", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	apiActor, e := h.f.s.AuthenticateActor(ctx, api.Token)
	if e != nil {
		t.Fatal(e)
	}
	page, e := h.f.s.ListSkillCatalog(ctx, apiActor, SkillCatalogListQuery{ProjectID: h.target})
	if e != nil || page.Total != 1 || page.CanManage || page.CanAdopt {
		t.Fatal("API scoped read/capabilities", page, e)
	}
	if _, e = h.f.s.GetSkillCatalog(ctx, apiActor, h.f.project, shared.ID); e == nil {
		t.Fatal("API crossed fixed project scope")
	}
	if _, e = h.f.s.AdoptSkillCatalog(ctx, apiActor, shared.ID, req); e == nil {
		t.Fatal("API acted as browser approver")
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, h.target, reviewer.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.AdoptSkillCatalog(ctx, reviewer, shared.ID, req); e == nil {
		t.Fatal("revoked target approver replayed privileged adoption")
	}
	h.f.cookie = ownerCookie
}

func TestSkillCatalogSourceChangeRefreshRetireAndImmutableHistory(t *testing.T) {
	h := newSkillFixture(t)
	ctx := context.Background()
	shareRequest := h.request()
	shared, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, shareRequest)
	if e != nil {
		t.Fatal(e)
	}
	req := h.adopt(shared)
	adopted, e := h.f.s.AdoptSkillCatalog(ctx, h.f.actor, shared.ID, req)
	if e != nil {
		t.Fatal(e)
	}
	edit := knowledgeDraft(h.f, "skill", "Reviewed workflow revision two", "New explicitly published reusable body.")
	edit.ExpectedVersion = h.knowledge.Version
	draft, e := h.f.s.UpdateProjectKnowledge(ctx, h.f.actor, h.f.project, h.knowledge.ID, edit)
	if e != nil {
		t.Fatal(e)
	}
	h.knowledge, e = h.f.s.CommandProjectKnowledge(ctx, h.f.actor, h.f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: h.f.project, Action: "publish", ExpectedVersion: draft.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	stale, e := h.f.s.GetSkillCatalog(ctx, h.f.actor, h.target, shared.ID)
	if e != nil || stale.SourceCurrent || stale.CanAdopt || !skillHasAction(stale.AllowedActions, "update") || stale.Content != shared.Content {
		t.Fatal("new publication broke refresh or old shared snapshot", stale, e)
	}
	if _, e = h.f.s.AdoptSkillCatalog(ctx, h.f.actor, shared.ID, h.adopt(shared)); e == nil {
		t.Fatal("stale origin adopted")
	}
	update := h.request()
	update.ExpectedVersion = shared.Version
	updateReceipt := update
	updated, e := h.f.s.UpdateSkillCatalog(ctx, h.f.actor, shared.ID, update)
	if e != nil || updated.Version != 2 || !updated.SourceCurrent || updated.Content == shared.Content {
		t.Fatal(updated, e)
	}
	oldReceipt, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, shareRequest)
	if e != nil || oldReceipt.Version != shared.Version || oldReceipt.Content != shared.Content || oldReceipt.Eligibility != "publication_changed" || oldReceipt.CanAdopt || len(oldReceipt.AllowedActions) != 0 {
		t.Fatal("historical share receipt claimed current adoption authority", oldReceipt, e)
	}
	update.IdempotencyKey = newID()
	if _, e = h.f.s.UpdateSkillCatalog(ctx, h.f.actor, shared.ID, update); e == nil {
		t.Fatal("stale catalog CAS")
	}
	retireRequest := SkillCatalogCommandRequest{ProjectID: h.target, Action: "retire", ExpectedVersion: updated.Version, Reason: "Explicit catalog retirement", IdempotencyKey: newID()}
	retired, e := h.f.s.CommandSkillCatalog(ctx, h.f.actor, shared.ID, retireRequest)
	if e != nil || retired.State != "retired" || retired.CanAdopt || len(retired.AllowedActions) != 0 {
		t.Fatal(retired, e)
	}
	resurrect := h.request()
	resurrect.ExpectedVersion = retired.Version
	if _, e = h.f.s.UpdateSkillCatalog(ctx, h.f.actor, shared.ID, resurrect); e == nil {
		t.Fatal("PATCH resurrected a retired catalog entry")
	}
	priorUpdate, e := h.f.s.UpdateSkillCatalog(ctx, h.f.actor, shared.ID, updateReceipt)
	if e != nil || priorUpdate.Version != updated.Version || priorUpdate.Content != updated.Content || !priorUpdate.SourceCurrent || priorUpdate.Eligibility != "retired" || priorUpdate.CanAdopt || len(priorUpdate.AllowedActions) != 0 {
		t.Fatal("old update receipt lost its original source facts or regained adoption authority", priorUpdate, e)
	}
	oldReceipt, e = h.f.s.CreateSkillCatalog(ctx, h.f.actor, shareRequest)
	if e != nil || oldReceipt.Version != shared.Version || oldReceipt.Content != shared.Content || oldReceipt.Eligibility != "retired" || oldReceipt.CanAdopt || len(oldReceipt.AllowedActions) != 0 {
		t.Fatal("old share receipt advertised adoption after retirement", oldReceipt, e)
	}
	retireReplay, e := h.f.s.CommandSkillCatalog(ctx, h.f.actor, shared.ID, retireRequest)
	if e != nil || retireReplay.Version != retired.Version || retireReplay.CanAdopt || len(retireReplay.AllowedActions) != 0 {
		t.Fatal(retireReplay, e)
	}
	history, e := h.f.s.SkillCatalogRevisions(ctx, h.f.actor, h.target, shared.ID, 1, "")
	if e != nil || history.Total != 3 || len(history.Items) != 1 || history.NextCursor == "" {
		t.Fatal(history, e)
	}
	second, e := h.f.s.SkillCatalogRevisions(ctx, h.f.actor, h.target, shared.ID, 1, history.NextCursor)
	if e != nil || second.Items[0].Version != 2 {
		t.Fatal(second, e)
	}
	third, e := h.f.s.SkillCatalogRevisions(ctx, h.f.actor, h.target, shared.ID, 1, second.NextCursor)
	if e != nil || third.Items[0].Content != shared.Content || third.Items[0].Origin != shared.Origin {
		t.Fatal("history was rewritten", third, e)
	}
	old, e := h.f.s.GetProjectKnowledge(ctx, h.f.actor, h.target, adopted.ID)
	if e != nil || old.Latest.Content != shared.Content || old.CatalogAdoption == nil || old.CatalogAdoption.Origin.SHA256 != shared.Origin.SHA256 {
		t.Fatal("catalog changes rewrote independent adopted draft", old, e)
	}
}

func TestSkillCatalogHiddenSourceTenantIsolationAndCurrentAdmin(t *testing.T) {
	h := newSkillFixture(t)
	ctx := context.Background()
	request := h.request()
	shared, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, request)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_projects SET status='archived' WHERE tenant_id=? AND id=?`, h.f.user.TenantID, h.f.project); e != nil {
		t.Fatal(e)
	}
	archivedRequest := h.request()
	if _, e = h.f.s.CreateSkillCatalog(ctx, h.f.actor, archivedRequest); e == nil {
		t.Fatal("archived source project was promoted")
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_projects SET status='active' WHERE tenant_id=? AND id=?`, h.f.user.TenantID, h.f.project); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_documents SET status='archived' WHERE tenant_id=? AND project_id=? AND id=?`, h.f.user.TenantID, h.f.project, h.source.ID); e != nil {
		t.Fatal(e)
	}
	readable, e := h.f.s.GetSkillCatalog(ctx, h.f.actor, h.target, shared.ID)
	if e != nil || readable.SourceCurrent || readable.Content != shared.Content {
		t.Fatal("source hiding erased shared text or stayed eligible", readable, e)
	}
	if _, e = h.f.s.AdoptSkillCatalog(ctx, h.f.actor, shared.ID, h.adopt(shared)); e == nil {
		t.Fatal("hidden source adopted")
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'reviewer')`, h.f.project, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.CreateSkillCatalog(ctx, h.f.actor, request); e == nil {
		t.Fatal("revoked organization admin replayed sharing")
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_org_members SET role='owner' WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	org, e := h.f.s.CreateOrganization(ctx, h.f.actor, map[string]any{"name": "Another catalog tenant", "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	_, token, e := h.f.s.SwitchOrganization(ctx, h.f.actor, map[string]any{"organization_id": org["id"], "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	other, e := h.f.s.AuthenticateActor(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	project, e := h.f.s.CreateProject(ctx, other, map[string]any{"title": "Foreign catalog scope", "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.GetSkillCatalog(ctx, other, project.ID, shared.ID); e == nil {
		t.Fatal("cross-tenant catalog read")
	}
	page, e := h.f.s.ListSkillCatalog(ctx, other, SkillCatalogListQuery{ProjectID: project.ID})
	if e != nil || page.Total != 0 {
		t.Fatal("cross-tenant catalog listing", page, e)
	}
}

func TestSkillCatalogFilteredPaginationAndAtomicAdoptionRollback(t *testing.T) {
	h := newSkillFixture(t)
	ctx := context.Background()
	first := h.share(t)
	request := h.request()
	request.Tags = []string{"safe", "50%_literal"}
	second, e := h.f.s.CreateSkillCatalog(ctx, h.f.actor, request)
	if e != nil {
		t.Fatal(e)
	}
	page, e := h.f.s.ListSkillCatalog(ctx, h.f.actor, SkillCatalogListQuery{ProjectID: h.target, Domain: "engineering", Scenario: "review", Tag: "safe", Limit: 1})
	if e != nil || page.Total != 2 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page, e)
	}
	next, e := h.f.s.ListSkillCatalog(ctx, h.f.actor, SkillCatalogListQuery{ProjectID: h.target, Domain: "engineering", Scenario: "review", Tag: "safe", Limit: 1, Cursor: page.NextCursor})
	if e != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal(next, e)
	}
	if _, e = h.f.s.ListSkillCatalog(ctx, h.f.actor, SkillCatalogListQuery{ProjectID: h.target, Tag: "other", Cursor: page.NextCursor}); e == nil {
		t.Fatal("cursor reused across filter")
	}
	literal, e := h.f.s.ListSkillCatalog(ctx, h.f.actor, SkillCatalogListQuery{ProjectID: h.target, Tag: "50%_literal"})
	if e != nil || literal.Total != 1 || literal.Items[0].ID != second.ID {
		t.Fatal("wildcard metadata broadened exact tag filter", literal, e)
	}
	if _, e = h.f.s.DB.Exec(`CREATE TRIGGER skill_adoption_failure BEFORE INSERT ON suma_skill_catalog_adoptions BEGIN SELECT RAISE(ABORT,'fixture adoption persistence failure'); END`); e != nil {
		t.Fatal(e)
	}
	var before, after int
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_knowledge WHERE project_id=?`, h.target).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.AdoptSkillCatalog(ctx, h.f.actor, first.ID, h.adopt(first)); e == nil {
		t.Fatal("fixture failed lineage accepted")
	}
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_project_knowledge WHERE project_id=?`, h.target).Scan(&after); e != nil || before != after {
		t.Fatal("adoption failed after leaking project draft", before, after, e)
	}
}

func TestSkillCatalogHTTPRejectsAmbiguousAndIncompleteCommands(t *testing.T) {
	h := newSkillFixture(t)
	request := h.request()
	raw, _ := json.Marshal(request)
	var body map[string]any
	json.Unmarshal(raw, &body)
	body["expected_version"] = 0.5
	h.f.call("POST", "/skill-catalog", body, 400)
	body["expected_version"] = 0
	body["tags"] = nil
	h.f.call("POST", "/skill-catalog", body, 400)
	body["tags"] = []any{}
	body["content"] = "Caller-supplied shared body"
	h.f.call("POST", "/skill-catalog", body, 400)
	delete(body, "content")
	created := h.f.call("POST", "/skill-catalog", body, 201)
	if created["origin"] == nil {
		t.Fatal(created)
	}
	h.f.call("GET", "/skill-catalog?project_id="+h.target+"&project_id="+h.target, nil, 400)
	h.f.call("GET", "/skill-catalog?project_id="+h.target+"&limit=01", nil, 400)
	h.f.call("GET", "/skill-catalog/"+created["id"].(string)+"?project_id="+h.target+"&version=1", nil, 400)
}
