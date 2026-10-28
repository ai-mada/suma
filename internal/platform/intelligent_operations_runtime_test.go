package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type operationFixture struct {
	f       *fixture
	token   string
	profile ToolProfile
	request IntelligentOperationCreateRequest
}

func newOperationFixture(t *testing.T) *operationFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.s.InitializeIntelligentOperations(ctx); err != nil {
		t.Fatal(err)
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "operation-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	p := toolTestProfile()
	p.ReadOnly = true
	p.IntelligentOperations = &toolcontract.IntelligentOperationsCapability{Protocol: toolcontract.IntelligentOperationsProtocol, Model: "owned-operation-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "operation-worker", Name: "Fixed local draft planner", Profile: p, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return &operationFixture{f: f, token: token, profile: profile, request: IntelligentOperationCreateRequest{ProjectID: f.project, Goal: "Propose a feature requirement, draft design and review guidance", Locale: "en-US", Sources: []ProjectKnowledgeSourceInput{}, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
}

func (h *operationFixture) create(t *testing.T) IntelligentOperationAnalysis {
	t.Helper()
	out, err := h.f.s.CreateIntelligentOperation(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func (h *operationFixture) claim(t *testing.T) dag.Assignment {
	t.Helper()
	wire := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if err := json.Unmarshal([]byte(stringify(wire["assignment"])), &assignment); err != nil || assignment.RunID == "" || assignment.NodeID != "analyze" || assignment.Config["purpose"] != "intelligent_operations" || stringify(assignment.Config["max_steps"]) != "1" {
		t.Fatalf("expected the intelligent-operation assignment for worker %q; claim response=%s, decode_error=%v", h.profile.WorkerID, stringify(wire), err)
	}
	return assignment
}
func (h *operationFixture) result(a dag.Assignment, raw string) dag.Result {
	return dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Output: raw, ToolEvidence: &toolcontract.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}}
}
func (h *operationFixture) complete(result dag.Result) {
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": result}), 200)
}
func (h *operationFixture) get(t *testing.T, id string) IntelligentOperationAnalysis {
	t.Helper()
	out, err := h.f.s.GetIntelligentOperation(context.Background(), h.f.actor, h.f.project, id)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestIntelligentOperationsActualDAGResultQuestionsRevisionAndBatch(t *testing.T) {
	h := newOperationFixture(t)
	ctx := context.Background()
	a := h.create(t)
	replay := h.create(t)
	if replay.ID != a.ID || replay.RunID != a.RunID || a.MaxSteps != 1 || a.MaxOutputTokens != 4096 {
		t.Fatal(a, replay)
	}
	assignment := h.claim(t)
	proposal := operationTestOutput()
	proposal.Questions = []string{"Which release should receive this draft?"}
	h.complete(h.result(assignment, stringify(proposal)))
	a = h.get(t, a.ID)
	if a.State != "ready" || a.OriginalOutput != stringify(proposal) || !a.BasisCurrent {
		t.Fatal(a)
	}
	preview, err := h.f.s.PreviewIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: h.f.project, ExpectedVersion: a.Version})
	if err != nil || preview.CanApply {
		t.Fatal(preview, err)
	}
	command := IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "apply", PreviewSHA256: preview.PreviewSHA256, AcknowledgeDrafts: true, Reason: "Reviewed the fixed source and plan", IdempotencyKey: newID()}
	if _, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, command); err == nil {
		t.Fatal("unresolved questions applied")
	}
	proposal.Questions = []string{}
	a, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "revise", Proposal: &proposal, Reason: "The target release was clarified for this local draft", IdempotencyKey: newID()})
	if err != nil || a.OriginalOutput == stringify(proposal) {
		t.Fatal("original model output changed", a, err)
	}
	preview, err = h.f.s.PreviewIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: h.f.project, ExpectedVersion: a.Version})
	if err != nil || !preview.CanApply {
		t.Fatal(preview, err)
	}
	command.ExpectedVersion, command.PreviewSHA256, command.IdempotencyKey = a.Version, preview.PreviewSHA256, newID()
	a, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, command)
	if err != nil || a.State != "applied" || len(a.Results) != 3 {
		t.Fatal(a, err)
	}
	var runs int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil || runs != 1 {
		t.Fatal("batch created another dispatcher/run", runs, err)
	}
	h.f.call("POST", "/intelligent-operations/analyses/"+a.ID+"/commands", command, 200)
}

func TestIntelligentOperationsSourceAndTargetDriftBlockWholeBatch(t *testing.T) {
	h := newOperationFixture(t)
	ctx := context.Background()
	r, err := h.f.s.Create(ctx, h.f.actor, h.f.project, "requirements", h.f.body(map[string]any{"title": "Fixed requirement", "description": "Read a fixed requirement", "acceptance_criteria": []string{"Preserve the draft gate"}}))
	if err != nil {
		t.Fatal(err)
	}
	h.request.Sources = []ProjectKnowledgeSourceInput{{Kind: "requirements", ID: r.ID, Version: r.Version}}
	a := h.create(t)
	assignment := h.claim(t)
	proposal := operationTestOutput()
	proposal.Steps[1].Design.RequirementID = r.ID
	proposal.Steps[1].Design.RequirementStepKey = ""
	h.complete(h.result(assignment, stringify(proposal)))
	a = h.get(t, a.ID)
	preview, err := h.f.s.PreviewIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: h.f.project, ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.f.s.SaveRequirementDesign(ctx, h.f.actor, h.f.project, r.ID, RequirementDesignSaveRequest{ProjectID: h.f.project, ExpectedVersion: 0, ExpectedRequirementVersion: r.Version, Content: RequirementDesignContent{Product: "A newer independent draft"}, Tasks: []RequirementDesignTask{}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	command := IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "apply", PreviewSHA256: preview.PreviewSHA256, AcknowledgeDrafts: true, Reason: "Review the current impact", IdempotencyKey: newID()}
	if _, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, command); err == nil {
		t.Fatal("stale design preview applied")
	}
	var count int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirements`).Scan(&count); err != nil || count != 1 {
		t.Fatal("stale preview partially applied", count, err)
	}
	_, err = h.f.s.Update(ctx, h.f.actor, h.f.project, "requirements", r.ID, h.f.body(map[string]any{"expected_version": r.Version, "title": "Changed source requirement"}))
	if err != nil {
		t.Fatal(err)
	}
	a = h.get(t, a.ID)
	if a.BasisCurrent || a.Proposal == nil || a.OriginalOutput == "" || a.ReasonKey != "basis_changed" {
		t.Fatal("source drift erased the proposal/original or system attribution", a)
	}
	if _, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, command); err == nil {
		t.Fatal("stale source applied")
	}
	revised, err := h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "revise", Proposal: a.Proposal, Reason: operationSource, IdempotencyKey: newID()})
	if err != nil || revised.Reason != operationSource || revised.ReasonKey != "" {
		t.Fatal("human reason text was reclassified as a host translation", revised, err)
	}
}

func TestIntelligentOperationsSafeContextAndCurrentAuthority(t *testing.T) {
	h := newOperationFixture(t)
	ctx := context.Background()
	secret := "sk-proj-" + strings.Repeat("x", 25)
	doc, err := h.f.s.Create(ctx, h.f.actor, h.f.project, "documents", h.f.body(map[string]any{"title": "Safe description", "description": "Only this visible safe text is selected", "data": map[string]any{"api_key": secret, "content": secret}}))
	if err != nil {
		t.Fatal(err)
	}
	h.request.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc.ID, Version: doc.Version}}
	a := h.create(t)
	if !knowledgeSourceCurrent(a.Source.Sources[0], map[string]Record{knowledgeSourceKey("documents", doc.ID): doc}) {
		t.Fatal("operation source cannot be adopted as a current ordinary knowledge source")
	}
	var spec string
	if err = h.f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=?`, a.RunID).Scan(&spec); err != nil || strings.Contains(spec, secret) || strings.Contains(spec, "api_key") {
		t.Fatal("private metadata entered model context", err)
	}
	h.request.Goal = "Inspect https://username:password@example.test/source"
	h.request.IdempotencyKey = newID()
	if _, err = h.f.s.CreateIntelligentOperation(ctx, h.f.e, h.f.actor, h.f.project, h.request); err == nil {
		t.Fatal("URL credentials entered the model")
	}
	assignment := h.claim(t)
	h.complete(h.result(assignment, stringify(operationTestOutput())))
	a = h.get(t, a.ID)
	preview, err := h.f.s.PreviewIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: h.f.project, ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, h.f.project, h.f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = h.f.s.CommandIntelligentOperation(ctx, h.f.actor, h.f.project, a.ID, IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "apply", PreviewSHA256: preview.PreviewSHA256, AcknowledgeDrafts: true, Reason: "Cached role must not authorize", IdempotencyKey: newID()})
	if err == nil {
		t.Fatal("stale owner projection authorized a batch")
	}
}
