package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestCodexSkillDispatchBuildSeparatesNativeAttachmentsFromText(t *testing.T) {
	w, profile, task := taskDispatchBuildFixture()
	profile.Protocol = cc.Protocol
	task.Knowledge = []ProjectKnowledgeContextItem{
		{ID: "reviewed-skill", Revision: 2, Kind: "skill", Title: "Exact reviewed skill", Summary: "Use the reviewed procedure", Content: "Unique approved native skill body 173", SHA256: strings.Repeat("c", 64), Sources: []ProjectKnowledgeContextSource{}},
		{ID: "reviewed-memory", Revision: 4, Kind: "memory", Title: "Original memory", Content: "Ordinary memory remains exact task text 173", SHA256: strings.Repeat("d", 64), Sources: []ProjectKnowledgeContextSource{}},
	}
	before := stringify(task)
	got, err := buildTaskDispatchWorkflow(w, "develop", "fixed-codex", profile, task)
	if err != nil {
		t.Fatal(err)
	}
	node := got.Nodes[0]
	if node.Config["execution_skills"] != nil {
		t.Fatal("an intermediate workflow acquired file authority before its final Codex binding")
	}
	goal, _ := node.Config["goal"].(string)
	if strings.Contains(goal, task.Knowledge[0].Content) || !strings.Contains(goal, task.Knowledge[1].Content) || !strings.Contains(goal, "reviewed-skill") || !strings.Contains(goal, "$suma-") {
		t.Fatal("native prompt repeated skill body, omitted memory, or lacked an explicit skill reference")
	}
	if stringify(task) != before || task.Knowledge[0].Content == "" {
		t.Fatal("native model projection changed the authoritative task snapshot")
	}
	// The original total fixed-input gate applies before the native projection.
	task.Knowledge[0].Content = strings.Repeat("x", 16000)
	task.Knowledge[1].Content = strings.Repeat("y", 16000)
	if _, err = buildTaskDispatchWorkflow(w, "develop", "fixed-codex", profile, task); err == nil {
		t.Fatal("native attachments bypassed the original 32000-byte fixed-input limit")
	}
}

func codexSkillPublished(t *testing.T, f *fixture, kind, content string) ProjectKnowledgeDetail {
	t.Helper()
	ctx := context.Background()
	draft, err := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, kind, "Reviewed "+kind, content))
	if err != nil {
		t.Fatal(err)
	}
	return codexSkillPublish(t, f, draft)
}

func codexSkillPublish(t *testing.T, f *fixture, draft ProjectKnowledgeDetail) ProjectKnowledgeDetail {
	t.Helper()
	got, err := f.s.CommandProjectKnowledge(context.Background(), f.actor, f.project, draft.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: draft.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func codexSkillReference(k ProjectKnowledgeDetail) ProjectKnowledgeReference {
	return ProjectKnowledgeReference{ID: k.ID, Revision: k.PublishedRevision, SHA256: k.PublishedSHA256}
}

func TestCodexSkillDispatchPlanClaimAndOriginalStartReceiptStayFixed(t *testing.T) {
	f, token, req, profile, _ := codexExecutionFixture(t)
	ctx := context.Background()
	skill := codexSkillPublished(t, f, "skill", "Approved original procedure 173\nKeep this exact native body.")
	memory := codexSkillPublished(t, f, "memory", "Memory stays in the original task input 173")
	req.Knowledge = []ProjectKnowledgeReference{codexSkillReference(memory), codexSkillReference(skill)}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExecutionSkills == nil || len(plan.ExecutionSkills.Documents) != 1 || plan.ExecutionSkills.Documents[0].ID != skill.ID || plan.Context.Knowledge[1].Content != skill.Published.Content {
		t.Fatal("plan lost the exact publication or included non-skill knowledge")
	}
	bound, err := cc.DecodeExecutionSkillBundle(codexPlanNode(t, plan).Config["execution_skills"])
	if err != nil || stringify(bound) != stringify(plan.ExecutionSkills) {
		t.Fatal("plan and protected workflow use different native skill bundles", err)
	}
	again, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil || again.ID != plan.ID || stringify(again.ExecutionSkills) != stringify(plan.ExecutionSkills) {
		t.Fatal("original preview operation replaced the native binding", err)
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	edit := knowledgeDraft(f, "skill", skill.Title, "A newer publication must not replace an existing run 173")
	edit.ExpectedVersion = skill.Version
	updated, err := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, skill.ID, edit)
	if err != nil {
		t.Fatal(err)
	}
	codexSkillPublish(t, f, updated)
	replayed, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || replayed.ID != run.ID {
		t.Fatal("current authorized original Start receipt did not recover its fixed run", err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.RunID != run.ID || stringify(assignment.ExecutionSkills) != stringify(plan.ExecutionSkills) {
		t.Fatal("actual private claim omitted or upgraded the original native bundle", claim)
	}
	forged := assignment
	forged.Fence++
	if err = f.s.decorateCodeInputAssignment(ctx, projectCandidateSubject(toolActorForTest(t, f, token), f.project), &forged); err == nil || forged.ExecutionSkills != nil {
		t.Fatal("stale attempt acquired native file authority", err)
	}
	req.IdempotencyKey = newID()
	if _, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req); err == nil {
		t.Fatal("fresh dispatch silently upgraded an explicitly selected old skill publication")
	}
}

func TestCodexSkillDispatchChangedPublicationOrSourceBlocksStart(t *testing.T) {
	for _, change := range []string{"publication", "source"} {
		t.Run(change, func(t *testing.T) {
			f, _, req, _, _ := codexExecutionFixture(t)
			ctx := context.Background()
			doc := f.call("POST", "/documents", f.body(map[string]any{"title": "Reviewed native procedure source", "description": taskKnowledgeQuote}), 201)
			draft := f.call("POST", "/project-knowledge/extractions", f.body(map[string]any{"kind": "skill", "title": "Reviewed native procedure", "sources": []any{taskKnowledgeSource(doc)}}), 201)
			published := taskKnowledgeCommand(f, draft, "publish")
			req.Knowledge = []ProjectKnowledgeReference{{ID: published["id"].(string), Revision: int(published["published_revision"].(float64)), SHA256: published["published_sha256"].(string)}}
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
			if err != nil || plan.ExecutionSkills == nil {
				t.Fatal("native skill preview missing", err)
			}
			if change == "source" {
				f.call("PATCH", "/documents/"+doc["id"].(string), f.body(map[string]any{"expected_version": doc["version"], "description": "Changed source basis 173"}), 200)
			} else {
				update := knowledgeDraft(f, "skill", "New reviewed native procedure", "Changed native procedure 173")
				update.ExpectedVersion = int(published["version"].(float64))
				edited, err := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published["id"].(string), update)
				if err != nil {
					t.Fatal(err)
				}
				codexSkillPublish(t, f, edited)
			}
			before := taskKnowledgeEffectCounts(t, f)
			_, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
			if err == nil || taskKnowledgeEffectCounts(t, f) != before {
				t.Fatal("stale skill Start wrote a run, receipt, or activity", err)
			}
		})
	}
}

func TestCodexSkillDispatchOrdinaryWorkflowCannotGrantNativeFiles(t *testing.T) {
	f, token, req, profile, _ := codexExecutionFixture(t)
	ctx := context.Background()
	skill := codexSkillPublished(t, f, "skill", "An exact skill bundle is still not run authority 173")
	req.Knowledge = []ProjectKnowledgeReference{codexSkillReference(skill)}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	copy := plan.Workflow
	copy.ID, copy.Version = newID(), 1
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, copy)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	if claim["assignment"] != nil {
		t.Fatal("a copied workflow bundle became file authority without its original started plan")
	}
	var attempts int
	if err = f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, run.ID, plan.AgentNodeID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("unauthorized bundle consumed an attempt", err)
	}
}

func TestCodexSkillDispatchPoolClaimUsesItsExactPlanAndMachineScope(t *testing.T) {
	p := newPoolMachineFixture(t)
	f, ctx := p.f, context.Background()
	runner := p.credential.Runner
	profile := tc.Profile{CommandID: "pool-codex-skills", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "Controlled native skill protocol 173"}
	catalog, err := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "owned-model", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/tool-inventory", p.credential.Token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true, CodexCatalog: &catalog}}}, 200)
	tool, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Explicit pool native skills", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	tool, err = f.s.CommandToolProfile(ctx, f.actor, f.project, tool.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: tool.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, _, _ := taskDispatchBuildFixture()
	w.Version = 1
	w, err = f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if err != nil {
		t.Fatal(err)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Scoped pool native skill delivery"}), 201)
	skill := codexSkillPublished(t, f, "skill", "Only this target project grants the reviewed file bytes 173")
	req := TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task["id"].(string), ExpectedTaskVersion: int(task["version"].(float64)), WorkflowID: w.ID, WorkflowVersion: w.Version, AgentNodeID: "develop", ToolProfileID: tool.ID, ExpectedToolVersion: tool.Version, CodexExecutionSelection: &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-model", ReasoningEffort: "low"}, Knowledge: []ProjectKnowledgeReference{codexSkillReference(skill)}, IdempotencyKey: newID()}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
	reply := f.bearer("POST", "/pool-runner/claim", p.credential.Token, claim, 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment) != nil || assignment.RunID != plan.RunID || stringify(assignment.ExecutionSkills) != stringify(plan.ExecutionSkills) {
		t.Fatal("actual pool claim lost or changed its protected native bundle", reply)
	}
	wrong := assignment
	wrong.ProjectID = p.scopes[1].ProjectID
	if err = f.s.decorateCodeInputAssignment(ctx, candidateSubject{pool: true, token: p.credential.Token, tenant: wrong.TenantID, project: wrong.ProjectID}, &wrong); err == nil || wrong.ExecutionSkills != nil {
		t.Fatal("another project scope obtained a native file grant", err)
	}
	wrong = assignment
	wrong.Fence++
	if err = f.s.decorateCodeInputAssignment(ctx, candidateSubject{pool: true, token: p.credential.Token, tenant: assignment.TenantID, project: assignment.ProjectID}, &wrong); err == nil || wrong.ExecutionSkills != nil {
		t.Fatal("changed pool fence obtained a native file grant", err)
	}
}

func TestCodexSkillDispatchResultReceiptBindsThePersistedPlan(t *testing.T) {
	f, token, req, profile, _ := codexExecutionFixture(t)
	ctx := context.Background()
	skill := codexSkillPublished(t, f, "skill", "Exact reviewed native result provenance 173")
	req.Knowledge = []ProjectKnowledgeReference{codexSkillReference(skill)}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.ExecutionSkills == nil {
		t.Fatal("native result fixture did not claim an authoritative bundle")
	}
	proof := &cc.ExecutionSkillEvidence{BundleSHA256: plan.ExecutionSkills.BundleSHA256, Documents: cc.ExecutionSkillReceipts(*plan.ExecutionSkills), Materialized: true, FilesVerified: true, FailureCode: "catalog_not_acknowledged"}
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Controlled catalog refusal", ToolEvidence: &tc.Evidence{Profile: profile.Profile, Terminal: "failed", UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: plan.CodexExecution.BindingSHA256, RequestedModel: plan.CodexExecution.Model, RequestedReasoningEffort: plan.CodexExecution.ReasoningEffort, Skills: proof}}}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, Spec: codexPlanNode(t, plan)}
	// A syntactically valid receipt for different bytes must fail even when
	// reaching the common accounting callback directly.
	forged := result
	var evidence tc.Evidence
	if err = json.Unmarshal([]byte(stringify(result.ToolEvidence)), &evidence); err != nil {
		t.Fatal(err)
	}
	evidence.CodexExecution.Skills.Documents[0].DocumentSHA256 = strings.Repeat("f", 64)
	forged.ToolEvidence = &evidence
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.RecordResultTx(ctx, tx, ref, forged)
	tx.Rollback()
	if err == nil {
		t.Fatal("a different document receipt entered the original plan ledger")
	}
	body := f.body(map[string]any{"result": result})
	f.bearer("POST", "/runner/results", token, body, 200)
	f.bearer("POST", "/runner/results", token, body, 200)
	var count int
	var raw string
	if err = f.s.DB.QueryRow(`SELECT COUNT(*),MIN(payload) FROM suma_usage_facts WHERE kind='result'`).Scan(&count, &raw); err != nil || count != 1 {
		t.Fatal("original completion receipt was duplicated or lost", count, err)
	}
	var stored dag.Result
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.ToolEvidence == nil || stored.ToolEvidence.CodexExecution == nil || stringify(stored.ToolEvidence.CodexExecution.Skills) != stringify(proof) {
		t.Fatal("durable result fact omitted native skill evidence")
	}
}

func TestCodexSkillDispatchSelectedSkillMissingBundleCannotClaim(t *testing.T) {
	f, token, req, profile, _ := codexExecutionFixture(t)
	ctx := context.Background()
	skill := codexSkillPublished(t, f, "skill", "Selected native skill cannot become an implicit text fallback 173")
	req.Knowledge = []ProjectKnowledgeReference{codexSkillReference(skill)}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	// Model a malformed persisted creator output: the approved skill remains in
	// Context and Codex is fixed, but every optional bundle field is absent.
	plan.ExecutionSkills = nil
	for i := range plan.Workflow.Nodes {
		delete(plan.Workflow.Nodes[i].Config, "execution_skills")
	}
	node := codexPlanNode(t, plan)
	if _, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(plan), plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?`, stringify(node), plan.RunID, plan.AgentNodeID); err != nil {
		t.Fatal(err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	if claim["assignment"] != nil {
		t.Fatal("selected native skill without its derived bundle became executable")
	}
	var attempt int
	if err = f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, plan.RunID, plan.AgentNodeID).Scan(&attempt); err != nil || attempt != 0 {
		t.Fatal("missing native bundle consumed an attempt", err)
	}
}

func TestCodexSkillDispatchExpertTeamUsesTheSameNativeBindingGate(t *testing.T) {
	h, profile, catalog := expertCodexFixture(t, false)
	f, ctx := h.f, context.Background()
	skill := codexSkillPublished(t, f, "skill", "One exact reviewed native skill for each independent team task 173")
	h.role.Knowledge = []ProjectKnowledgeReference{codexSkillReference(skill)}
	var err error
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Explicit native skill selection in the existing team", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, expertCodexRequest(h, catalog))
	if err != nil || len(preview.Items) != 2 || preview.Items[0].Plan.ExecutionSkills == nil || preview.Items[1].Plan.ExecutionSkills == nil {
		t.Fatal("team preview lost its per-task native skill bundle", err)
	}
	var malformed TaskDispatchPlan
	if err = json.Unmarshal([]byte(stringify(preview.Items[0].Plan)), &malformed); err != nil {
		t.Fatal(err)
	}
	malformed.ExecutionSkills = nil
	for i := range malformed.Workflow.Nodes {
		delete(malformed.Workflow.Nodes[i].Config, "execution_skills")
	}
	readCtx, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, preview.ID, purposeProjectRead)
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.checkExpertTeamPreviewReadTx(readCtx, auth, malformed)
	close()
	if err == nil {
		t.Fatal("team preview path bypassed the shared native binding validation")
	}
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview))
	if err != nil {
		t.Fatal(err)
	}
	claim := f.bearer("POST", "/runner/claim", h.runnerToken, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.ExecutionSkills == nil || assignment.ExecutionSkills.Documents[0].ID != skill.ID {
		t.Fatal("team task did not receive its original approved native file grant", claim)
	}
	found := false
	for _, item := range started.Items {
		if item.RunID == assignment.RunID {
			found = stringify(item.Plan.ExecutionSkills) == stringify(assignment.ExecutionSkills)
		}
	}
	if !found {
		t.Fatal("team assignment bundle differs from its exact persisted item")
	}
}

func TestCodexSkillDispatchBuildRejectsTemplateFileAuthority(t *testing.T) {
	w, profile, task := taskDispatchBuildFixture()
	w.Nodes[0].Config["execution_skills"] = map[string]any{"path": "../../unreviewed/SKILL.md", "content": "unreviewed"}
	task.Knowledge = []ProjectKnowledgeContextItem{{ID: "ordinary-text-skill", Revision: 3, Kind: "skill", Title: "Existing text skill", Content: "Other adapter exact published skill text 173", SHA256: strings.Repeat("a", 64)}}
	before, _ := json.Marshal(w)
	got, err := buildTaskDispatchWorkflow(w, "develop", "fixed-legacy", profile, task)
	if err != nil {
		t.Fatal(err)
	}
	if got.Nodes[0].Config["execution_skills"] != nil {
		t.Fatal("workflow configuration became native file authority")
	}
	if !strings.Contains(got.Nodes[0].Config["goal"].(string), task.Knowledge[0].Content) {
		t.Fatal("other adapter stopped receiving its existing text skill context")
	}
	after, _ := json.Marshal(w)
	if string(before) != string(after) {
		t.Fatal("dispatch mutated the original workflow")
	}
}
