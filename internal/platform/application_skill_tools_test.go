package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSkillCatalogueRealMCPReadsExactReviewWithoutExpandingAgentAuthority(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "skill", "Release review", "Verify independent test evidence before accepting a release."))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: draft.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	req := SkillCatalogWriteRequest{ProjectID: f.project, KnowledgeID: published.ID, ExpectedKnowledgeVersion: published.Version, PublishedRevision: published.PublishedRevision, PublishedSHA256: published.PublishedSHA256, Domain: "engineering", Scenarios: []string{"release-review"}, Tags: []string{"quality"}, Reason: "Share reviewed release practice", AcknowledgeSharing: true, IdempotencyKey: newID()}
	shared, err := f.s.CreateSkillCatalog(ctx, f.actor, req)
	if err != nil {
		t.Fatal(err)
	}
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	server := httptest.NewServer((&Server{Service: f.s}).NewMCPHandler())
	t.Cleanup(server.Close)
	client := mcpTestClient(t, server.URL, f.project, token, f.user, "2025-11-25")
	page, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_skill_catalog_list", Arguments: map[string]any{"domain": "engineering", "scenario": "release-review", "limit": 5}})
	if err != nil || page.IsError || len(mcpTestResult(t, page)["items"].([]any)) != 1 {
		t.Fatalf("discovery: %+v %v", page, err)
	}
	pin := map[string]any{"id": shared.ID, "version": shared.Version, "origin_sha256": shared.Origin.SHA256}
	read, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_skill_catalog_get", Arguments: pin})
	if err != nil || read.IsError {
		t.Fatalf("exact read: %+v %v", read, err)
	}
	result := mcpTestResult(t, read)
	if result["content"] != shared.Content || result["sources"] != nil || result["can_manage"] != false || result["can_adopt"] != false {
		t.Fatalf("read leaked originals or browser authority: %+v", result)
	}
	req.ExpectedVersion, req.Domain, req.IdempotencyKey = shared.Version, "delivery", newID()
	if _, err = f.s.UpdateSkillCatalog(ctx, f.actor, shared.ID, req); err != nil {
		t.Fatal(err)
	}
	stale, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_skill_catalog_get", Arguments: pin})
	if err != nil || !stale.IsError || mcpTestResult(t, stale)["error"].(map[string]any)["status"] != float64(409) {
		t.Fatalf("old pin silently changed: %+v %v", stale, err)
	}
	agent := applicationActor(t, f, "agent")
	args, _ := json.Marshal(pin)
	if _, err = applicationCall(t, f, agent, "project.skill_catalog_get", string(args)); applicationStatus(err) != 403 {
		t.Fatalf("Agent authority expanded: %v", err)
	}
	api := applicationActor(t, f, "api")
	for _, raw := range []string{`{"id":"` + shared.ID + `","version":1.0,"origin_sha256":"` + shared.Origin.SHA256 + `"}`, `{"project_id":"foreign"}`, `{"limit":0}`, `{"limit":1e0}`} {
		name := "project.skill_catalog_list"
		if strings.Contains(raw, "origin_sha256") {
			name = "project.skill_catalog_get"
		}
		if _, err = applicationCall(t, f, api, name, raw); applicationStatus(err) != 422 {
			t.Fatalf("inexact or overridden input accepted: %s %v", raw, err)
		}
	}
}
