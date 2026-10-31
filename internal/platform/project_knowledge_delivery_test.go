package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestDeliveryKnowledgeOrdinarySourceRejectsDeliveryProof(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	doc, err := f.s.Create(ctx, f.actor, f.project, "documents", f.body(map[string]any{"title": "Ordinary source", "description": "Review this exact ordinary text."}))
	if err != nil {
		t.Fatal(err)
	}
	request := ProjectKnowledgeWriteRequest{ProjectID: f.project, Kind: "memory", Title: "Ordinary source draft", Content: "Reviewed draft", Sources: []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc.ID, Version: doc.Version, BasisSHA256: strings.Repeat("a", 64)}}, IdempotencyKey: newID()}
	if _, err = f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request); err == nil {
		t.Fatal("ordinary source accepted an unrelated delivery basis")
	}
	request.IdempotencyKey = newID()
	request.Sources[0].BasisSHA256 = ""
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	source := draft.Latest.Sources[0]
	source.Delivery = &DeliveryKnowledgeBasis{Protocol: "suma-accepted-delivery-v1", SHA256: strings.Repeat("b", 64)}
	if knowledgeSourceCurrent(source, map[string]Record{knowledgeSourceKey("documents", doc.ID): doc}) {
		t.Fatal("ordinary source snapshot acquired a forged typed delivery proof")
	}
}

func acceptedKnowledgeExtractionFixture(t *testing.T, f *fixture, source ProjectKnowledgeSource) *intelligenceFixture {
	t.Helper()
	ctx := context.Background()
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "delivery-extraction-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.KnowledgeExtraction = &toolcontract.KnowledgeExtractionCapability{Protocol: toolcontract.KnowledgeExtractionProtocol, Model: "controlled-delivery-extraction", EndpointOrigin: "http://127.0.0.1:45678"}
	registered, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "delivery-extraction-worker", Name: "Reviewed delivery extraction", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: registered.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	registered, err = f.s.CommandToolProfile(ctx, f.actor, f.project, registered.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: registered.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return &intelligenceFixture{f: f, token: token, profile: registered, request: KnowledgeIntelligenceCreateRequest{ProjectID: f.project, Kind: "skill", Goal: "Propose reusable lessons from this exact accepted delivery", Locale: "en-US", Sources: []ProjectKnowledgeSourceInput{{Kind: source.Kind, ID: source.ID, Version: source.Version, BasisSHA256: source.Delivery.SHA256}}, ToolProfileID: registered.ID, ExpectedToolVersion: registered.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
}

func acceptedPublishedContext(f *fixture, task string, ref ProjectKnowledgeReference) (TaskContext, error) {
	ctx, auth, close, err := f.s.planningAuthority(context.Background(), f.actor, f.project, task, purposeProjectRead)
	if err != nil {
		return TaskContext{}, err
	}
	defer close()
	return f.s.taskContextForDispatchTx(ctx, auth, f.project, task, []ProjectKnowledgeReference{ref})
}

func TestKnowledgeAcceptedDeliveryInputAndCompleteSnapshot(t *testing.T) {
	h, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := h.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	source := view.Source
	input := ProjectKnowledgeSourceInput{Kind: source.Kind, ID: source.ID, Version: source.Version, BasisSHA256: source.Delivery.SHA256}
	for _, scenario := range []struct {
		name string
		edit func(*ProjectKnowledgeSourceInput)
		code int
	}{
		{"missing-basis", func(v *ProjectKnowledgeSourceInput) { v.BasisSHA256 = "" }, 400},
		{"uppercase-basis", func(v *ProjectKnowledgeSourceInput) { v.BasisSHA256 = strings.Repeat("A", 64) }, 400},
		{"changed-basis", func(v *ProjectKnowledgeSourceInput) { v.BasisSHA256 = strings.Repeat("a", 64) }, 409},
		{"changed-version", func(v *ProjectKnowledgeSourceInput) { v.Version-- }, 409},
		{"invented-quote", func(v *ProjectKnowledgeSourceInput) { v.Quote = "This sentence was never accepted." }, 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ref := input
			scenario.edit(&ref)
			request := knowledgeDraft(f, "memory", "Reviewed lesson", "A reviewed lesson")
			request.Sources = []ProjectKnowledgeSourceInput{ref}
			_, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request)
			requireAuthorityStatus(t, err, scenario.code)
		})
	}
	f.call("POST", "/project-knowledge", f.body(map[string]any{"kind": "memory", "title": "Injected proof", "content": "A reviewed lesson", "expected_version": 0, "sources": []any{map[string]any{"kind": source.Kind, "id": source.ID, "version": source.Version, "basis_sha256": source.Delivery.SHA256, "delivery": source.Delivery}}}), 400)
	request := knowledgeDraft(f, "memory", "Reviewed lesson", "A reviewed lesson")
	input.Quote = "Explicit review of the published case"
	request.Sources = []ProjectKnowledgeSourceInput{input}
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request)
	if err != nil || len(draft.Latest.Sources) != 1 {
		t.Fatal(draft, err)
	}
	stored := draft.Latest.Sources[0]
	if stored.Quote != input.Quote || stored.SHA256 != source.SHA256 || stringify(stored.Delivery) != stringify(source.Delivery) {
		t.Fatal("source resolution trusted a client proof or changed the full source digest", stored)
	}
	var altered ProjectKnowledgeSource
	if err = json.Unmarshal([]byte(stringify(stored)), &altered); err != nil {
		t.Fatal(err)
	}
	altered.Delivery.Tests[0].ResultSHA256 = strings.Repeat("f", 64)
	ctx, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, accepted.ID, purposeProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	states, err := f.s.knowledgeSourcesCurrentTx(ctx, auth.tx, f.user.TenantID, f.project, []ProjectKnowledgeSource{stored, altered, stored}, nil, true)
	if err != nil || len(states) != 3 || !states[0] || states[1] || !states[2] {
		t.Fatal("same-task batch weakened complete typed source comparison", states, err)
	}
}

func TestKnowledgeAcceptedDeliveryModelAdoptionPublicationAndNextContext(t *testing.T) {
	quality, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := quality.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := acceptedKnowledgeExtractionFixture(t, f, view.Source)
	analysis := h.create(t)
	assignment := h.claim(t)
	goal, _ := assignment.Config["goal"].(string)
	if !strings.Contains(goal, `"delivery_summary"`) || strings.Contains(goal, view.Source.Delivery.Tests[0].ResultSHA256) || stringify(analysis.Source.Sources[0].Delivery) != stringify(view.Source.Delivery) {
		t.Fatal("model prompt copied complete evidence or analysis lost its original typed basis")
	}
	original := h.output()
	h.complete(t, h.result(assignment, original))
	analysis = h.get(t, analysis.ID)
	adopt := h.command(analysis, "adopt")
	adopted, err := f.s.CommandKnowledgeIntelligence(ctx, f.actor, f.project, analysis.ID, adopt)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.s.GetProjectKnowledge(ctx, f.actor, f.project, adopted.AdoptedKnowledgeID)
	if err != nil || draft.State != "draft" || draft.Published != nil || draft.Latest.Method != "model_assisted" || stringify(draft.Latest.Sources[0].Delivery) != stringify(view.Source.Delivery) {
		t.Fatal("adoption changed provenance or bypassed draft publication", draft, err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Next explicit consumer", "description": "Use only the selected published revision"}))
	if err != nil {
		t.Fatal(err)
	}
	ref := ProjectKnowledgeReference{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.PublishedSHA256}
	context, err := acceptedPublishedContext(f, next.ID, ref)
	if err != nil || len(context.Knowledge) != 1 || stringify(context.Knowledge[0].Sources[0].Delivery) != stringify(view.Source.Delivery) {
		t.Fatal("next task context lost the exact published delivery basis", context, err)
	}
	f.call("PATCH", "/tasks/"+accepted.ID, f.body(map[string]any{"expected_version": accepted.Version, "description": "Changed delivery scope needs another review"}), 200)
	_, err = acceptedPublishedContext(f, next.ID, ref)
	requireAuthorityStatus(t, err, 409)
	replayed, err := f.s.CommandKnowledgeIntelligence(ctx, f.actor, f.project, analysis.ID, adopt)
	if err != nil || replayed.AdoptedKnowledgeID != draft.ID || replayed.BasisCurrent || replayed.OriginalOutput != original {
		t.Fatal("historical adoption replay lost its original fact or gained new authority", replayed, err)
	}
	history, err := f.s.GetProjectKnowledgeRevision(ctx, f.actor, f.project, published.ID, published.PublishedRevision)
	if err != nil || history.Snapshot.SHA256 != ref.SHA256 || stringify(history.Snapshot.Sources[0].Delivery) != stringify(view.Source.Delivery) {
		t.Fatal("source drift erased an immutable published revision", history, err)
	}
}

func TestKnowledgeAcceptedDeliveryDriftKeepsOutputButBlocksNewActions(t *testing.T) {
	quality, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := quality.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := acceptedKnowledgeExtractionFixture(t, f, view.Source)
	analysis := h.create(t)
	manual := knowledgeDraft(f, "memory", "Reviewed delivery draft", "Human reviewed text")
	manual.Sources = h.request.Sources
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, manual)
	if err != nil {
		t.Fatal(err)
	}
	assignment := h.claim(t)
	f.call("PATCH", "/tasks/"+accepted.ID, f.body(map[string]any{"expected_version": accepted.Version, "description": "This accepted source changed while extraction was in flight"}), 200)
	original := h.output()
	h.complete(t, h.result(assignment, original))
	analysis = h.get(t, analysis.ID)
	if analysis.State != "ready" || analysis.BasisCurrent || analysis.OriginalOutput != original || analysis.OriginalOutputSHA256 != digest(original) {
		t.Fatal("a changed source discarded the already-observed model original", analysis)
	}
	_, err = f.s.CommandKnowledgeIntelligence(ctx, f.actor, f.project, analysis.ID, h.command(analysis, "adopt"))
	requireAuthorityStatus(t, err, 409)
	_, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	replay, err := f.s.CreateKnowledgeIntelligence(ctx, f.e, f.actor, f.project, h.request)
	if err != nil || replay.ID != analysis.ID || replay.BasisCurrent || replay.OriginalOutputSHA256 != digest(original) {
		t.Fatal("same create operation repeated a model run or lost the original", replay, err)
	}
	var n int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE id=?`, analysis.RunID).Scan(&n); err != nil || n != 1 {
		t.Fatal("drift replay created another run", n, err)
	}
}

func TestKnowledgeAcceptedDeliveryClaimRechecksSource(t *testing.T) {
	quality, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := quality.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := acceptedKnowledgeExtractionFixture(t, f, view.Source)
	analysis := h.create(t)
	f.call("PATCH", "/tasks/"+accepted.ID, f.body(map[string]any{"expected_version": accepted.Version, "description": "Changed before model admission"}), 200)
	claimed := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if claimed["assignment"] != nil {
		t.Fatal("a stale accepted delivery was admitted as a new model execution", claimed)
	}
	var attempt int
	if err = f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id='extract'`, analysis.RunID).Scan(&attempt); err != nil || attempt != 0 {
		t.Fatal("ineligible source acquired an execution attempt", attempt, err)
	}
}

func TestKnowledgeAcceptedDeliverySkillCatalogUsesOriginalMetadataBoundary(t *testing.T) {
	quality, _, accepted := deliveryKnowledgeAcceptedQualityFixture(t)
	f, ctx := quality.f, context.Background()
	view, err := f.s.GetDeliveryKnowledgeSource(ctx, f.actor, f.project, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := knowledgeDraft(f, "skill", "Reviewed reusable delivery skill", "Review exact delivery evidence before using a lesson.")
	request.Sources = []ProjectKnowledgeSourceInput{{Kind: view.Source.Kind, ID: view.Source.ID, Version: view.Source.Version, BasisSHA256: view.Source.Delivery.SHA256}}
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	target, err := f.s.CreateProject(ctx, f.actor, map[string]any{"title": "Receiving project", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	h := &skillFixture{f: f, target: target.ID, knowledge: published}
	shared := h.share(t)
	if !shared.SourceCurrent || strings.Contains(stringify(shared), view.Source.Quote) || strings.Contains(stringify(shared), view.Source.Delivery.Task.ReviewID) {
		t.Fatal("catalog source metadata either became stale or disclosed original review proof", shared)
	}
	adopted, err := f.s.AdoptSkillCatalog(ctx, f.actor, shared.ID, h.adopt(shared))
	if err != nil || adopted.ProjectID != target.ID || adopted.State != "draft" || len(adopted.Latest.Sources) != 0 || adopted.CatalogAdoption == nil {
		t.Fatal("catalog adoption inherited a source-project delivery grant", adopted, err)
	}
	f.call("PATCH", "/tasks/"+accepted.ID, f.body(map[string]any{"expected_version": accepted.Version, "description": "Changed after explicit catalog sharing"}), 200)
	stale, err := f.s.GetSkillCatalog(ctx, f.actor, target.ID, shared.ID)
	if err != nil || stale.SourceCurrent || stale.Content != shared.Content {
		t.Fatal("catalog lost history or kept stale accepted-delivery eligibility", stale, err)
	}
}

func TestDeliveryKnowledgeContextPreservesTypedSourceBasis(t *testing.T) {
	basis := &DeliveryKnowledgeBasis{Protocol: "suma-accepted-delivery-v1", RunVersion: 3, Tests: []DeliveryKnowledgeTestEvidence{{RunID: "original-run", NodeID: "test", Attempt: 1, Fence: 2, WorkerID: "independent-worker", ResultSHA256: strings.Repeat("a", 64)}}, Candidates: []DeliveryKnowledgeCandidateEvidence{}, SHA256: strings.Repeat("b", 64)}
	source := ProjectKnowledgeSource{Kind: "accepted_delivery", ID: "original-task", Version: 4, Title: "Accepted delivery", Quote: "Private review quote remains in the revision only", SHA256: strings.Repeat("c", 64), Delivery: basis}
	out := knowledgeContextSources([]ProjectKnowledgeSource{source})
	if len(out) != 1 || out[0].Delivery == nil || stringify(out[0].Delivery) != stringify(basis) || out[0].SHA256 != source.SHA256 {
		t.Fatal("published context dropped its exact typed accepted-delivery basis")
	}
	if strings.Contains(stringify(out), source.Quote) {
		t.Fatal("published source metadata duplicated the original source quote")
	}
}
