package platform

import (
	"context"
	"testing"
)

func TestNotificationInboxAcceptedHandoffAndDecompositionSources(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	ownerBefore := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	if inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 {
		t.Fatal("receiver has no assignment yet")
	}
	proposal := h.send(t)
	if inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 || inboxPage(t, f, f.actor, "inbox", 20, "").Total != ownerBefore {
		t.Fatal("handoff proposal generated an assignment event")
	}
	command := handoffCommand(h, "accept", proposal.Version)
	accepted, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	current := h.current(t)
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if accepted.State != "accepted" || page.Total != 1 || len(page.Items) != 1 || current.OwnerID != h.resource.ID || current.Version != accepted.ResultTaskVersion {
		t.Fatalf("handoff did not commit its real assignment: %+v task=%+v inbox=%+v", accepted, current, page)
	}
	item := page.Items[0]
	if item.TaskID != current.ID || item.SourceResourceID != h.resource.ID || item.SourceTaskVersion != current.Version || !item.AssignmentCurrent || item.CurrentTask == nil || item.CurrentTask.Version != current.Version || item.CurrentTask.Title != current.Title {
		t.Fatalf("handoff notification differs from exact committed task: %+v task=%+v", item, current)
	}
	again, err := f.s.CommandTaskHandoff(ctx, h.receiver, f.project, proposal.ID, command)
	if err != nil || again.ID != accepted.ID || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 1 || h.current(t).Version != current.Version {
		t.Fatalf("accepted handoff replay duplicated source: %+v %v", again, err)
	}
	if old := inboxPage(t, f, f.actor, "inbox", 20, ""); old.Total != ownerBefore || old.Items[0].AssignmentCurrent {
		t.Fatalf("old owner event was transferred or rewritten: %+v", old)
	}

	// Ordinary decomposition is another real creation path through the same
	// source transaction, not a separate notification factory.
	body := map[string]any{"project_id": f.project, "expected_version": h.requirement.Version, "idempotency_key": "inbox-decompose-replay", "drafts": []any{map[string]any{"key": "followup", "title": "Decomposed receiving task", "owner_id": h.resource.ID, "acceptance_criteria": []any{"Use the exact assigned task"}}}}
	tasks, err := f.s.Decompose(ctx, f.actor, f.project, h.requirement.ID, body)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("decompose source: %+v %v", tasks, err)
	}
	actual, err := f.s.Get(ctx, f.actor, f.project, "tasks", tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	page = inboxPage(t, f, h.receiver, "inbox", 20, "")
	if page.Total != 2 || page.Items[0].TaskID != actual.ID || page.Items[0].SourceTaskVersion != actual.Version || page.Items[0].SourceResourceID != actual.OwnerID || actual.OwnerID != h.resource.ID {
		t.Fatalf("decomposition did not project exact source: %+v task=%+v", page, actual)
	}
	replayed, err := f.s.Decompose(ctx, f.actor, f.project, h.requirement.ID, body)
	if err != nil || len(replayed) != 1 || replayed[0].ID != actual.ID || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 2 {
		t.Fatalf("decomposition replay duplicated notification: %+v %v", replayed, err)
	}
}

func TestNotificationInboxDesignAdoptionCreatesAndReassignsExactRecipient(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	ownerBefore := inboxPage(t, f, f.actor, "inbox", 20, "").Total
	requirement, err := f.s.Create(ctx, f.actor, f.project, "requirements", map[string]any{"title": "Reviewed notification source", "acceptance_criteria": []any{"Assignment follows exact adopted task"}, "idempotency_key": "inbox-design-requirement"})
	if err != nil {
		t.Fatal(err)
	}
	publish := func(owner, title string, version int) RequirementDesignView {
		t.Helper()
		draft := designUnitDraft(f)
		draft.ExpectedVersion = version
		draft.ExpectedRequirementVersion = requirement.Version
		draft.Tasks[0].OwnerID = owner
		draft.Tasks[0].Title = title
		saved, e := f.s.SaveRequirementDesign(ctx, f.actor, f.project, requirement.ID, draft)
		if e != nil {
			t.Fatal(e)
		}
		published, e := f.s.CommandRequirementDesign(ctx, f.actor, f.project, requirement.ID, RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: saved.Version, ExpectedRequirementVersion: requirement.Version, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		return published
	}
	preview := func(view RequirementDesignView) RequirementDesignPreview {
		t.Helper()
		value, e := f.s.PreviewRequirementDesign(ctx, f.actor, f.project, requirement.ID, RequirementDesignPreviewRequest{ProjectID: f.project, ExpectedVersion: view.Version, Revision: view.PublishedRevision, SHA256: view.Published.SHA256})
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	request := func(plan RequirementDesignPreview, key string) RequirementDesignAdoptRequest {
		return RequirementDesignAdoptRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Revision: plan.Revision, SHA256: plan.SHA256, PlanSHA256: plan.PlanSHA256, IdempotencyKey: key}
	}
	currentView := func() RequirementDesignView {
		t.Helper()
		value, e := f.s.GetRequirementDesign(ctx, f.actor, f.project, requirement.ID)
		if e != nil {
			t.Fatal(e)
		}
		return value
	}

	published := publish(h.owner.ID, "Adopted assignment", 0)
	plan := preview(published)
	if plan.CreateCount != 1 || inboxPage(t, f, f.actor, "inbox", 20, "").Total != ownerBefore || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 {
		t.Fatal("design proposal/publication/preview generated an assignment")
	}
	firstRequest := request(plan, "inbox-design-first-adopt")
	first, err := f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, firstRequest)
	if err != nil || len(first.Tasks) != 1 {
		t.Fatalf("first adoption: %+v %v", first, err)
	}
	taskID := first.Tasks[0].TaskID
	actual, err := f.s.Get(ctx, f.actor, f.project, "tasks", taskID)
	if err != nil {
		t.Fatal(err)
	}
	ownerPage := inboxPage(t, f, f.actor, "inbox", 20, "")
	if ownerPage.Total != ownerBefore+1 || ownerPage.Items[0].TaskID != taskID || ownerPage.Items[0].SourceTaskVersion != actual.Version || actual.OwnerID != h.owner.ID {
		t.Fatalf("design creation notification is not exact: %+v task=%+v", ownerPage, actual)
	}
	if _, err = f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, firstRequest); err != nil || inboxPage(t, f, f.actor, "inbox", 20, "").Total != ownerBefore+1 {
		t.Fatal("initial design adoption replay duplicated event", err)
	}
	keep := preview(currentView())
	if keep.HasChanges || keep.KeepCount != 1 {
		t.Fatalf("expected keep plan: %+v", keep)
	}
	if _, err = f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, request(keep, "inbox-design-keep-adopt")); err != nil || inboxPage(t, f, f.actor, "inbox", 20, "").Total != ownerBefore+1 {
		t.Fatal("keep adoption generated notification", err)
	}

	unchangedOwner := publish(h.owner.ID, "Changed title with same owner", currentView().Version)
	samePlan := preview(unchangedOwner)
	if samePlan.UpdateCount != 1 {
		t.Fatalf("expected same-owner task update: %+v", samePlan)
	}
	if _, err = f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, request(samePlan, "inbox-design-same-owner")); err != nil {
		t.Fatal(err)
	}
	if inboxPage(t, f, f.actor, "inbox", 20, "").Total != ownerBefore+1 || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 {
		t.Fatal("owner-unchanged design update generated assignment")
	}

	reassigned := publish(h.resource.ID, "Reassigned by reviewed design", currentView().Version)
	changedPlan := preview(reassigned)
	if changedPlan.UpdateCount != 1 || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 0 {
		t.Fatalf("reassignment preview emitted event: %+v", changedPlan)
	}
	changedRequest := request(changedPlan, "inbox-design-reassignment")
	adopted, err := f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, changedRequest)
	if err != nil || len(adopted.Tasks) != 1 || adopted.Tasks[0].TaskID != taskID {
		t.Fatalf("owner update adoption: %+v %v", adopted, err)
	}
	actual, err = f.s.Get(ctx, f.actor, f.project, "tasks", taskID)
	if err != nil {
		t.Fatal(err)
	}
	receiverPage := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if receiverPage.Total != 1 || len(receiverPage.Items) != 1 {
		t.Fatalf("owner update did not generate exactly one event: %+v", receiverPage)
	}
	item := receiverPage.Items[0]
	if actual.OwnerID != h.resource.ID || actual.Version != adopted.Tasks[0].Version || item.TaskID != taskID || item.SourceResourceID != h.resource.ID || item.SourceTaskVersion != actual.Version || !item.AssignmentCurrent || item.CurrentTask == nil || item.CurrentTask.Version != actual.Version || item.CurrentTask.Title != actual.Title {
		t.Fatalf("design reassignment notification differs from committed version: %+v task=%+v", item, actual)
	}
	if _, err = f.s.AdoptRequirementDesign(ctx, f.actor, f.project, requirement.ID, changedRequest); err != nil || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != 1 {
		t.Fatal("owner update adoption replay duplicated notification", err)
	}
}
