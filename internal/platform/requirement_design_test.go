package platform

import (
	"context"
	"testing"
)

func designUnitDraft(f *fixture) RequirementDesignSaveRequest {
	return RequirementDesignSaveRequest{ProjectID: f.project, ExpectedRequirementVersion: 1, Content: RequirementDesignContent{Product: "Product outcome", Experience: "Accessible interaction", Technical: "Transactional implementation"}, Tasks: []RequirementDesignTask{{Key: "build", Title: "Build safely", Description: "Implement contract", Priority: "medium", DependsOn: []string{}, AcceptanceCriteria: []string{"Contract is implemented"}, Covers: []int{1}}}, IdempotencyKey: newID()}
}
func TestRequirementDesignPublicationDeltaAndMetadata(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	req := f.call("POST", "/requirements", f.body(map[string]any{"title": "Reviewed requirement", "description": "Stable source", "acceptance_criteria": []string{"Outcome is usable"}}), 201)
	id := req["id"].(string)
	empty, err := f.s.GetRequirementDesign(ctx, f.actor, f.project, id)
	if err != nil || empty.Version != 0 || empty.Requirement.Title != "Reviewed requirement" {
		t.Fatalf("actual source projection: %#v %v", empty, err)
	}
	draft := designUnitDraft(f)
	saved, err := f.s.SaveRequirementDesign(ctx, f.actor, f.project, id, draft)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := f.s.CommandRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: saved.Version, ExpectedRequirementVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if pub.Published.Requirement.Title != "Reviewed requirement" || pub.Published.Content.Product != draft.Content.Product {
		t.Fatalf("publication metadata missing: %#v", pub)
	}
	preview, err := f.s.PreviewRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignPreviewRequest{ProjectID: f.project, ExpectedVersion: pub.Version, Revision: pub.PublishedRevision, SHA256: pub.Published.SHA256})
	if err != nil || preview.CreateCount != 1 {
		t.Fatalf("preview: %#v %v", preview, err)
	}
	first, err := f.s.AdoptRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignAdoptRequest{ProjectID: f.project, ExpectedVersion: preview.Version, Revision: preview.Revision, SHA256: preview.SHA256, PlanSHA256: preview.PlanSHA256, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	taskID := first.Tasks[0].TaskID
	actual := f.call("GET", "/tasks/"+taskID+"?project_id="+f.project, nil, 200)
	if actual["title"] != "Build safely" || actual["requirement_id"] != id {
		t.Fatalf("formal task metadata: %v", actual)
	}
	view, err := f.s.GetRequirementDesign(ctx, f.actor, f.project, id)
	if err != nil {
		t.Fatal(err)
	}
	noop, err := f.s.PreviewRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignPreviewRequest{ProjectID: f.project, ExpectedVersion: view.Version, Revision: view.PublishedRevision, SHA256: view.Published.SHA256})
	if err != nil || noop.HasChanges || noop.KeepCount != 1 {
		t.Fatalf("noop: %#v %v", noop, err)
	}
	same, err := f.s.AdoptRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignAdoptRequest{ProjectID: f.project, ExpectedVersion: noop.Version, Revision: noop.Revision, SHA256: noop.SHA256, PlanSHA256: noop.PlanSHA256, IdempotencyKey: newID()})
	if err != nil || same.ID != first.ID {
		t.Fatalf("noop invented adoption: %#v %v", same, err)
	}
	draft.ExpectedVersion = view.Version
	draft.IdempotencyKey = newID()
	draft.Content.Product = "Improved product"
	edited, err := f.s.SaveRequirementDesign(ctx, f.actor, f.project, id, draft)
	if err != nil {
		t.Fatal(err)
	}
	if edited.Published.Content.Product != pub.Published.Content.Product || !edited.HasDraft {
		t.Fatal("draft changed approved baseline")
	}
	newer, err := f.s.CommandRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: edited.Version, ExpectedRequirementVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	delta, err := f.s.PreviewRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignPreviewRequest{ProjectID: f.project, ExpectedVersion: newer.Version, Revision: newer.PublishedRevision, SHA256: newer.Published.SHA256})
	if err != nil || delta.UpdateCount != 1 || delta.Changes[0].TaskID != taskID {
		t.Fatalf("new baseline must update same task: %#v %v", delta, err)
	}
	history, err := f.s.RequirementDesignRevisions(ctx, f.actor, f.project, id, 100, "")
	if err != nil || history.Total != 4 {
		t.Fatalf("history: %#v %v", history, err)
	}
	adoptions, err := f.s.RequirementDesignAdoptions(ctx, f.actor, f.project, id, 100, "")
	if err != nil || adoptions.Total != 1 {
		t.Fatalf("noop history: %#v %v", adoptions, err)
	}
}
func TestRequirementDesignDraftDiagnosticsAndStructure(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	req := f.call("POST", "/requirements", f.body(map[string]any{"title": "Draft requirement", "acceptance_criteria": []string{"Covered"}}), 201)
	id := req["id"].(string)
	draft := designUnitDraft(f)
	draft.Content.Experience = ""
	draft.Tasks[0].DependsOn = []string{"missing"}
	saved, err := f.s.SaveRequirementDesign(ctx, f.actor, f.project, id, draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Validation) < 2 {
		t.Fatalf("incomplete draft missing diagnostics: %#v", saved.Validation)
	}
	_, err = f.s.CommandRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: saved.Version, ExpectedRequirementVersion: 1, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 400)
	draft.ExpectedVersion = saved.Version
	draft.IdempotencyKey = newID()
	draft.Tasks = append(draft.Tasks, draft.Tasks[0])
	_, err = f.s.SaveRequirementDesign(ctx, f.actor, f.project, id, draft)
	requireAuthorityStatus(t, err, 400)
}
