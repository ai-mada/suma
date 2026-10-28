package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type dispatchRecommendationFixture struct {
	f              *fixture
	developer      ToolProfile
	developerToken string
	analysis       ToolProfile
	analysisToken  string
	deployments    []mc.Deployment
	request        DispatchRecommendationCreateRequest
}

func newDispatchRecommendationFixture(t *testing.T) *dispatchRecommendationFixture {
	t.Helper()
	ctx := context.Background()
	f, developerToken, preview, developer, deployments := modelExecutionFixture(t)
	worker := "recommendation-worker-122"
	analysisToken := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := toolTestProfile()
	definition.CommandID = "recommend122"
	definition.ProfileSHA256 = strings.Repeat("e", 64)
	definition.ReadOnly = true
	definition.ModelExecutionProtocol = mc.Protocol
	definition.DispatchRecommendation = &tc.DispatchRecommendationCapability{Protocol: tc.DispatchRecommendationProtocol}
	analysis, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Read-only recommendation protocol fixture", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, analysisToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true, ModelDeployments: deployments[:1]}}}); err != nil {
		t.Fatal(err)
	}
	analysis, err = f.s.CommandToolProfile(ctx, f.actor, f.project, analysis.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: analysis.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var original TaskDispatchPreviewRequest
	if err = json.Unmarshal([]byte(stringify(preview)), &original); err != nil {
		t.Fatal(err)
	}
	options := DispatchRecommendationOptionsRequest{ProjectID: f.project, TaskID: original.TaskID, ExpectedTaskVersion: original.ExpectedTaskVersion, WorkflowID: original.WorkflowID, WorkflowVersion: original.WorkflowVersion, AgentNodeID: original.AgentNodeID, Knowledge: []ProjectKnowledgeReference{}, Constraints: DispatchRecommendationConstraints{MaxSteps: 8, MaxOutputTokens: 4096}}
	source, err := f.s.GetDispatchRecommendationOptions(ctx, f.e, f.actor, f.project, options)
	if err != nil || len(source.Candidates) != 2 {
		t.Fatal("real model inventory was not frozen", source, err)
	}
	selection := mc.Selection{DeploymentID: deployments[0].ID, DeploymentRevision: deployments[0].Revision, DeploymentSHA256: deployments[0].SHA256, Parameters: mc.Parameters{MaxSteps: 1, MaxOutputTokens: 128}}
	request := DispatchRecommendationCreateRequest{DispatchRecommendationOptionsRequest: options, SourceSHA256: source.SHA256, Goal: "Compare these fixed model candidates without starting development", Locale: "en-US", AnalysisToolProfileID: analysis.ID, ExpectedAnalysisToolVersion: analysis.Version, AnalysisModelExecutionSelection: &selection, AcknowledgeModelUse: true, IdempotencyKey: newID()}
	return &dispatchRecommendationFixture{f: f, developer: developer, developerToken: developerToken, analysis: analysis, analysisToken: analysisToken, deployments: deployments, request: request}
}

func (h *dispatchRecommendationFixture) create(t *testing.T) DispatchRecommendation {
	t.Helper()
	out, err := h.f.s.CreateDispatchRecommendation(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A protocol fixture verifies platform receipt/authority behavior. It is not a
// provider invocation or evidence of an actual supplier charge; real SDK/HTTP
// gateway execution is verified separately by the parent integration journey.
func (h *dispatchRecommendationFixture) start(t *testing.T) (DispatchRecommendation, dag.Assignment, dag.Result, DispatchRecommendationChoice) {
	t.Helper()
	f := h.f
	a := h.create(t)
	guardCtx, guard, closeGuard, guardErr := f.s.planningAuthority(context.Background(), f.actor, f.project, h.request.TaskID, purposeBrowserProjectWrite)
	if guardErr != nil {
		t.Fatal(guardErr)
	}
	guardErr = f.s.checkDispatchRecommendationExecutionTx(guardCtx, guard.tx, f.user.TenantID, f.project, a.RunID)
	closeGuard()
	if guardErr != nil {
		t.Fatal("original recommendation admission", guardErr)
	}
	claimed := f.bearer("POST", "/runner/claim", h.analysisToken, f.body(map[string]any{"worker_id": h.analysis.WorkerID, "capabilities": []string{"command:" + h.analysis.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claimed["assignment"])), &assignment); err != nil || assignment.RunID != a.RunID || assignment.NodeID != "recommend" {
		t.Fatal("missing original analysis assignment", claimed, err)
	}
	candidate := a.Source.Candidates[0]
	steps, output := 1, int64(64)
	choice := DispatchRecommendationChoice{CandidateID: candidate.ID, MaxSteps: &steps, MaxOutputTokens: &output, Reason: "The frozen candidate has a verified context window; historical speed is unknown"}
	proposal := DispatchRecommendationProposal{SchemaVersion: 1, Summary: "Review this bounded model alternative", Rationale: "Uses the current profile and fixed deployment; this is a recommendation only", Recommendations: []DispatchRecommendationChoice{choice}, Questions: []string{"Do you prefer a different context window?"}}
	b := a.AnalysisModelExecution
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), Output: stringify(proposal), ToolEvidence: &tc.Evidence{Profile: h.analysis.Profile, Terminal: "completed", EventCount: 3, UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: usageInt(2), OutputTokens: usageInt(3)}, ModelExecution: &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: b.Deployment.Model, OutputAllowance: b.Parameters.MaxOutputTokens, UsageState: tc.UsageComplete, Usage: &mc.RequestUsage{InputTokens: usageInt(2), OutputTokens: usageInt(3)}}}}}}
	return a, assignment, result, choice
}

func (h *dispatchRecommendationFixture) ready(t *testing.T) (DispatchRecommendation, DispatchRecommendationChoice) {
	t.Helper()
	f := h.f
	a, _, result, choice := h.start(t)
	f.bearer("POST", "/runner/results", h.analysisToken, f.body(map[string]any{"result": result}), 200)
	out, err := f.s.GetDispatchRecommendation(context.Background(), f.e, f.actor, f.project, a.ID)
	if err != nil || out.State != "ready" || out.Proposal == nil || !out.BasisCurrent {
		t.Fatal("valid original recommendation was not projected", out, err)
	}
	return out, choice
}

func TestDispatchRecommendationSingleAnalysisReceiptAndSourceDrift(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f := h.f
	ctx := context.Background()
	a := h.create(t)
	again := h.create(t)
	privateSource := a.Source
	privateSource.Workflow.Nodes = []dag.Node{{ID: "safe-node", Type: "agent", Command: []string{"administrator-command-never-transmit"}, Config: map[string]any{"token": "administrator-secret-never-transmit", "endpoint": "https://administrator-endpoint.invalid"}}}
	promptSource := stringify(dispatchRecommendationPromptSource(privateSource))
	for _, private := range []string{"administrator-command-never-transmit", "administrator-secret-never-transmit", "administrator-endpoint.invalid"} {
		if strings.Contains(promptSource, private) {
			t.Fatal("private workflow configuration entered the model prompt")
		}
	}
	if a.ID != again.ID || a.RunID != again.RunID || a.AnalysisModelExecution.Parameters.MaxSteps != 1 || a.AnalysisModelExecution.Parameters.MaxOutputTokens != 128 || a.State != "running" {
		t.Fatal("analysis receipt changed its operation", a, again)
	}
	if n := dispatchOptionsCount(t, f, "dag_runs"); n != 1 {
		t.Fatal("another model run was created", n)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil || task.RunID != "" || task.Version != h.request.ExpectedTaskVersion {
		t.Fatal("analysis dispatched the development task", task, err)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, f.body(map[string]any{"expected_version": task.Version, "title": "Changed engineering objective"}))
	if err != nil {
		t.Fatal(err)
	}
	recovered := h.create(t)
	if recovered.RunID != a.RunID || recovered.BasisCurrent {
		t.Fatal("old create operation repeated or hid drift", recovered)
	}
	request := h.request
	request.IdempotencyKey = newID()
	_, err = f.s.CreateDispatchRecommendation(ctx, f.e, f.actor, f.project, request)
	requireAuthorityStatus(t, err, 409)
	if n := dispatchOptionsCount(t, f, "dag_runs"); n != 1 {
		t.Fatal("rejected source created model effects", n)
	}
	_, err = f.s.CreateDispatchRecommendation(ctx, f.e, toolActorForTest(t, f, h.analysisToken), f.project, h.request)
	requireAuthorityStatus(t, err, 403)
}

func TestDispatchRecommendationHumanAdoptionPreservesOriginalPlanAndStart(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f := h.f
	ctx := context.Background()
	a, choice := h.ready(t)
	steps := 2
	choice.MaxSteps = &steps
	choice.Reason = "Human reviewer explicitly chooses two bounded development steps"
	request := DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: a.Version, SourceSHA256: a.Source.SHA256, Choice: choice, Reason: "Review the proposal and prepare the ordinary dispatch plan", IdempotencyKey: newID()}
	out, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	if err != nil || out.Plan.ID == "" || out.Plan.Recommendation == nil || out.Plan.Recommendation.RecommendationVersion != a.Version || out.Recommendation.State != "adopted" || out.Recommendation.AdoptedChoice == nil || out.Plan.ModelExecution.Parameters.MaxSteps != 2 {
		t.Fatal(out, err)
	}
	replay, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	if err != nil || replay.Plan.ID != out.Plan.ID || dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 1 || dispatchOptionsCount(t, f, "dag_runs") != 1 {
		t.Fatal("adoption repeated a plan or started development", replay, err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil || task.RunID != "" {
		t.Fatal(task, err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, out.Plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: out.Plan.Version, IdempotencyKey: newID()})
	if err != nil || run.ID != out.Plan.RunID || run.ID == a.RunID || dispatchOptionsCount(t, f, "dag_runs") != 2 {
		t.Fatal("original start authority was not reused", run, err)
	}
}

func TestDispatchRecommendationForeignChoiceAndCurrentInventoryCannotBeAdopted(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f := h.f
	ctx := context.Background()
	a, choice := h.ready(t)
	request := DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: a.Version, SourceSHA256: a.Source.SHA256, Choice: choice, Reason: "Explicit bounded human choice", IdempotencyKey: newID()}
	foreign := request
	foreign.Choice.CandidateID = strings.Repeat("f", 64)
	_, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, foreign)
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, h.developerToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: h.developer.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: h.developer.Profile, Available: true, ModelDeployments: h.deployments[1:]}}}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	requireAuthorityStatus(t, err, 409)
	if dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 0 || dispatchOptionsCount(t, f, "dag_runs") != 1 {
		t.Fatal("invalid selection caused dispatch effects")
	}
}

func TestDispatchRecommendationReaderReauthorizesHistoricalKnowledgeBeforeTaskDrift(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f := h.f
	ctx := context.Background()
	created, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "spec", "Protected current project context", "Historical reviewed engineering context"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: created.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.request.Knowledge = []ProjectKnowledgeReference{{ID: published.ID, Revision: published.PublishedRevision, SHA256: published.PublishedSHA256}}
	source, err := f.s.GetDispatchRecommendationOptions(ctx, f.e, f.actor, f.project, h.request.DispatchRecommendationOptionsRequest)
	if err != nil {
		t.Fatal(err)
	}
	h.request.SourceSHA256 = source.SHA256
	a := h.create(t)
	ownerCookie := f.cookie
	invitation := governanceInvite(f, "recommendation-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	accepted := governanceAccept(f, invitation, "recommendation-reader@example.test")
	viewer, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	read, err := f.s.GetDispatchRecommendation(ctx, f.e, viewer, f.project, a.ID)
	if err != nil || !read.BasisCurrent || len(read.AllowedActions) != 0 || len(read.Source.Context.Knowledge) != 1 {
		t.Fatal("read-only membership was treated as stale execution configuration", read, err)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", h.request.TaskID, f.body(map[string]any{"expected_version": h.request.ExpectedTaskVersion, "title": "A newer engineering objective"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: published.Version, Action: "retire", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	read, err = f.s.GetDispatchRecommendation(ctx, f.e, viewer, f.project, a.ID)
	if err != nil || read.BasisCurrent || read.Source.Context.Knowledge[0].Content != "Historical reviewed engineering context" {
		t.Fatal("ordinary version drift erased authorized historical context", read, err)
	}
	other, err := f.s.CreateProject(ctx, f.actor, f.body(map[string]any{"title": "Foreign recommendation knowledge"}))
	if err != nil {
		t.Fatal(err)
	}
	foreignRequest := knowledgeDraft(f, "spec", "Foreign private material", "Must never enter the current project reader response")
	foreignRequest.ProjectID = other.ID
	foreign, err := f.s.CreateProjectKnowledge(ctx, f.actor, other.ID, foreignRequest)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err = f.s.CommandProjectKnowledge(ctx, f.actor, other.ID, foreign.ID, ProjectKnowledgeCommandRequest{ProjectID: other.ID, ExpectedVersion: foreign.Version, Action: "publish", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	// Negative stored-reference fixture: neither an old aggregate payload nor
	// an earlier task-version conflict can grant access to a foreign asset.
	stored, err := f.s.dispatchRecommendationTx(ctx, f.s.DB, f.user.TenantID, f.project, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := foreign.Published
	stored.Recommendation.Source.Context.Knowledge = []ProjectKnowledgeContextItem{{ID: foreign.ID, Revision: r.Revision, SHA256: r.SHA256, Kind: r.Kind, Title: r.Title, Summary: r.Summary, Content: r.Content, Sources: knowledgeContextSources(r.Sources)}}
	if _, err = f.s.DB.Exec(`UPDATE suma_dispatch_recommendations SET payload=? WHERE id=?`, stringify(stored), a.ID); err != nil {
		t.Fatal(err)
	}
	blocked, err := f.s.GetDispatchRecommendation(ctx, f.e, viewer, f.project, a.ID)
	requireAuthorityStatus(t, err, 403)
	if blocked.ID != "" || blocked.OriginalOutput != "" || len(blocked.Source.Context.Knowledge) != 0 {
		t.Fatal("authorization failure retained a sensitive response", blocked)
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": accepted["user"].(map[string]any)["id"], "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	_, err = f.s.GetDispatchRecommendation(ctx, f.e, viewer, f.project, a.ID)
	requireAuthorityStatus(t, err, 403)
}
