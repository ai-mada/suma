package platform

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

const designIntegrationProduct = `Reviewed product 原文 <scope> "quoted" must remain fixed.`

// These scenarios use actual browser/API credentials and the public HTTP
// commands. No actor, publication, task binding or acceptance is seeded in SQL.
type designIntegrationFixture struct {
	f           *fixture
	req         map[string]any
	request     map[string]any
	runnerToken string
}

func newDesignIntegrationFixture(t *testing.T, dispatch bool) *designIntegrationFixture {
	t.Helper()
	d := &designIntegrationFixture{}
	if dispatch {
		var task map[string]any
		d.f, d.runnerToken, d.request, task = taskDispatchFixture(t)
		d.req = d.f.call("GET", "/requirements/"+task["requirement_id"].(string)+"?project_id="+d.f.project, nil, 200)
	} else {
		d.f = newFixture(t)
		d.req = d.f.call("POST", "/requirements", d.f.body(map[string]any{"title": "Design-governed tenant boundary", "description": "Only current project members may read orders.", "acceptance_criteria": []string{"Cross-project access is rejected"}}), 201)
	}
	return d
}

func (d *designIntegrationFixture) path() string {
	return "/requirements/" + d.req["id"].(string) + "/design"
}

func designIntegrationContent() RequirementDesignContent {
	return RequirementDesignContent{Product: designIntegrationProduct, Experience: "Show a clear denial and preserve the edited draft. 交互约定。", Technical: "Enforce current project authority inside the transaction. 技术约定。"}
}

func designIntegrationTasks() []RequirementDesignTask {
	return []RequirementDesignTask{{Key: "access", Title: "Implement order isolation", Description: "Guard current project membership.", Priority: "high", DependsOn: []string{}, AcceptanceCriteria: []string{"Reject a foreign project"}, Covers: []int{1}}}
}

func (d *designIntegrationFixture) current() map[string]any {
	d.f.t.Helper()
	return d.f.call("GET", d.path()+"?project_id="+d.f.project, nil, 200)
}

func (d *designIntegrationFixture) save(view map[string]any, content RequirementDesignContent, tasks []RequirementDesignTask) map[string]any {
	d.f.t.Helper()
	return d.f.call("PUT", d.path(), d.f.body(map[string]any{"expected_version": view["version"], "expected_requirement_version": d.req["version"], "content": content, "tasks": tasks}), 200)
}

func (d *designIntegrationFixture) commandBody(view map[string]any, action string) map[string]any {
	return d.f.body(map[string]any{"action": action, "expected_version": view["version"], "expected_requirement_version": d.req["version"]})
}

func (d *designIntegrationFixture) command(view map[string]any, action string) map[string]any {
	d.f.t.Helper()
	return d.f.call("POST", d.path()+"/commands", d.commandBody(view, action), 200)
}

func (d *designIntegrationFixture) publish(content RequirementDesignContent, tasks []RequirementDesignTask) map[string]any {
	d.f.t.Helper()
	return d.command(d.save(d.current(), content, tasks), "publish")
}

func (d *designIntegrationFixture) preview(view map[string]any) map[string]any {
	d.f.t.Helper()
	published := view["published"].(map[string]any)
	return d.f.call("POST", d.path()+"/preview", map[string]any{"project_id": d.f.project, "expected_version": view["version"], "revision": published["revision"], "sha256": published["sha256"]}, 200)
}

func (d *designIntegrationFixture) adoptBody(preview map[string]any) map[string]any {
	return d.f.body(map[string]any{"expected_version": preview["version"], "revision": preview["revision"], "sha256": preview["sha256"], "plan_sha256": preview["plan_sha256"]})
}

func (d *designIntegrationFixture) adopt(view map[string]any) map[string]any {
	d.f.t.Helper()
	return d.f.call("POST", d.path()+"/adopt", d.adoptBody(d.preview(view)), 200)
}

func (d *designIntegrationFixture) task(adoption map[string]any, key string) map[string]any {
	d.f.t.Helper()
	for _, raw := range adoption["tasks"].([]any) {
		item := raw.(map[string]any)
		if item["key"] == key {
			return d.f.call("GET", "/tasks/"+item["task_id"].(string)+"?project_id="+d.f.project, nil, 200)
		}
	}
	d.f.t.Fatalf("adoption omitted stable task key %q", key)
	return nil
}

func (d *designIntegrationFixture) dispatch(task map[string]any, want int) map[string]any {
	d.f.t.Helper()
	body := map[string]any{}
	for k, v := range d.request {
		body[k] = v
	}
	body["task_id"], body["expected_task_version"], body["idempotency_key"] = task["id"], task["version"], newID()
	return d.f.call("POST", "/task-dispatch-plans", body, want)
}

func designIntegrationEffects(t *testing.T, d *designIntegrationFixture) string {
	t.Helper()
	effects := taskKnowledgeEffectCounts(t, d.f)
	rows, err := d.f.s.DB.Query(`SELECT task_key,task_id,active,revision,requirement_version,task_basis,adoption_id FROM suma_requirement_design_task_links WHERE tenant_id=? AND project_id=? AND requirement_id=? ORDER BY task_key`, d.f.user.TenantID, d.f.project, d.req["id"])
	if err != nil {
		t.Fatal(err)
	}
	links := []any{}
	for rows.Next() {
		var key, id, basis, adoption string
		var active, revision, requirementVersion int
		if err = rows.Scan(&key, &id, &active, &revision, &requirementVersion, &basis, &adoption); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		links = append(links, []any{key, id, active, revision, requirementVersion, basis, adoption})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	return stringify(map[string]any{"tasks": d.f.call("GET", "/tasks?project_id="+d.f.project, nil, 200), "design": d.current(), "adoptions": d.f.call("GET", d.path()+"/adoptions?project_id="+d.f.project, nil, 200), "links": links, "runs": effects.runs, "plans": effects.plans, "receipts": effects.receipts, "activities": effects.activities})
}

func TestRequirementDesignIntegrationFixedContextAndCombinedBudget(t *testing.T) {
	d := newDesignIntegrationFixture(t, true)
	f := d.f
	view := d.publish(designIntegrationContent(), designIntegrationTasks())
	adoption := d.adopt(view)
	task := d.task(adoption, "access")
	plan := d.dispatch(task, 201)
	design := plan["context"].(map[string]any)["design"].(map[string]any)
	if design["content"].(map[string]any)["product"] != designIntegrationProduct || design["task_key"] != "access" || design["adoption_id"] != adoption["id"] || design["requirement_id"] != d.req["id"] || design["requirement_version"] != d.req["version"] {
		t.Fatal("formal dispatch lost the reviewed design or its provenance", design)
	}
	goal, actual := taskKnowledgeGoal(t, plan["workflow"].(map[string]any))
	if stringify(actual["design"]) != stringify(design) || strings.Count(goal, stringify(designIntegrationProduct)) != 1 {
		t.Fatal("agent goal does not contain exactly one fixed reviewed body")
	}
	if _, repeated := actual["design"].(map[string]any)["tasks"]; repeated {
		t.Fatal("consumed design repeats the complete task proposal")
	}

	// Saving an unreviewed draft neither replaces the publication nor changes
	// the approved execution inputs held in an already prepared plan.
	draftContent := designIntegrationContent()
	draftContent.Product = "UNAPPROVED draft text must not enter execution."
	draft := d.save(d.current(), draftContent, designIntegrationTasks())
	if draft["has_draft"] != true || draft["published_revision"] != view["published_revision"] {
		t.Fatal("editing a draft replaced the reviewed publication", draft)
	}
	reread := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if reread["valid"] != true || stringify(reread["context"].(map[string]any)["design"]) != stringify(design) {
		t.Fatal("draft edit drifted the fixed execution basis", reread)
	}
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	workflowPath := "/workflows/" + run["workflow_id"].(string) + "?project_id=" + f.project + "&version=" + strconv.Itoa(int(run["workflow_version"].(float64)))
	runningGoal, _ := taskKnowledgeGoal(t, f.call("GET", workflowPath, nil, 200))
	if runningGoal != goal || strings.Contains(runningGoal, draftContent.Product) {
		t.Fatal("start used the unreviewed draft")
	}

	// A complete, valid 16k design remains usable by itself. Adding approved
	// knowledge must still obey the existing complete-goal budget, without any
	// truncation or a silent increase in the platform's 32,000-byte limit.
	large := RequirementDesignContent{Product: strings.Repeat("D", 15980), Experience: "Usable", Technical: "Isolated"}
	largeView := d.publish(large, designIntegrationTasks())
	largeTask := d.task(d.adopt(largeView), "access")
	largePlan := d.dispatch(largeTask, 201)
	largeGoal, largeContext := taskKnowledgeGoal(t, largePlan["workflow"].(map[string]any))
	if len(largeGoal) > 32000 || largeContext["design"].(map[string]any)["content"].(map[string]any)["product"] != large.Product || strings.Count(largeGoal, large.Product) != 1 {
		t.Fatal("single maximum design was truncated, repeated or over budget")
	}
	doc, knowledge := taskKnowledgePublished(t, f)
	knowledge = taskKnowledgeCommand(f, taskKnowledgeEdit(f, knowledge, doc, strings.Repeat("K", 16000)), "publish")
	d.request["knowledge"] = []any{taskKnowledgeReference(knowledge)}
	before := taskKnowledgeEffectCounts(t, f)
	d.dispatch(largeTask, 400)
	if after := taskKnowledgeEffectCounts(t, f); after != before {
		t.Fatal("over-budget combined context left a plan, run or receipt", before, after)
	}
	stillFixed, _ := taskKnowledgeGoal(t, f.call("GET", workflowPath, nil, 200))
	if stillFixed != runningGoal {
		t.Fatal("new publication rewrote the original running workflow")
	}
}

// Complete a scheduler/Runner protocol fixture through actual claim and result
// endpoints. Workspace/tool evidence is deliberately a contract fixture, not a
// claim that a vendor model or a real repository development task was exercised.
func designIntegrationCompleteProtocol(t *testing.T, d *designIntegrationFixture, runID string) {
	t.Helper()
	f, ctx := d.f, context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, d.request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		claimed := f.bearer("POST", "/runner/claim", d.runnerToken, f.body(map[string]any{"worker_id": "tool-worker", "capabilities": toolFixtureCapabilities(t, f)}), 200)
		var a dag.Assignment
		raw, _ := json.Marshal(claimed["assignment"])
		if err = json.Unmarshal(raw, &a); err != nil || a.RunID != runID {
			t.Fatalf("fixed design assignment was not claimable: %+v %v", a, err)
		}
		proof := reviewWorkspaceEvidence("c", a.NodeID == "test")
		if a.CandidateFrom != "" {
			proof["base_commit"], proof["head_commit"] = a.CandidateCommit, a.CandidateCommit
			proof["candidate_from"], proof["candidate_attempt"], proof["candidate_fence"], proof["candidate_evidence_sha256"] = a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256
		}
		result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
		if a.NodeID == "develop" {
			result.ToolEvidence = &toolcontract.Evidence{Profile: profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}
		}
		f.bearer("POST", "/runner/results", d.runnerToken, f.body(map[string]any{"result": result}), 200)
	}
	run := f.call("GET", "/runs/"+runID+"?project_id="+f.project, nil, 200)
	if run["state"] != "succeeded" {
		t.Fatal("protocol fixture did not produce a succeeded run", run)
	}
}

func TestRequirementDesignIntegrationNewPublicationRejectsOldRunAndPreview(t *testing.T) {
	d := newDesignIntegrationFixture(t, true)
	f := d.f
	tasks := append(designIntegrationTasks(), RequirementDesignTask{Key: "observe", Title: "Observe isolated behavior", Description: "Review the empty state.", Priority: "medium", DependsOn: []string{}, AcceptanceCriteria: []string{"State remains readable"}, Covers: []int{1}})
	view := d.publish(designIntegrationContent(), tasks)
	adoption := d.adopt(view)
	task := d.task(adoption, "access")
	plan := d.dispatch(task, 201)
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	designIntegrationCompleteProtocol(t, d, run["id"].(string))
	task = d.task(adoption, "access")
	review := f.body(map[string]any{"expected_version": task["version"], "run_id": run["id"], "decision": "accepted", "reason": "Explicit protocol fixture evidence reviewed"})
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", review, 200)
	stalePlan := d.dispatch(d.task(adoption, "observe"), 201)
	definitionPath := "/workflows/" + run["workflow_id"].(string) + "?project_id=" + f.project + "&version=" + strconv.Itoa(int(run["workflow_version"].(float64)))
	definition := stringify(f.call("GET", definitionPath, nil, 200))

	content := designIntegrationContent()
	content.Technical += " Newly approved architecture requires a fresh run."
	d.publish(content, tasks)
	before := designIntegrationEffects(t, d)
	f.call("POST", "/task-dispatch-plans/"+stalePlan["id"].(string)+"/start", f.body(map[string]any{"expected_version": stalePlan["version"]}), 409)
	if after := designIntegrationEffects(t, d); after != before {
		t.Fatal("stale design start produced partial effects")
	}
	adopted := d.adopt(d.current())
	current := d.task(adopted, "access")
	if current["status"] == "accepted" || current["run_id"] != run["id"] {
		t.Fatal("new adoption retained acceptance or lost historical run", current)
	}
	before = designIntegrationEffects(t, d)
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Old result must not approve the new design"}), 409)
	if after := designIntegrationEffects(t, d); after != before {
		t.Fatal("obsolete run acceptance produced a receipt or task change")
	}
	// The original successful fact and its exact definition remain inspectable.
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", review, 200)
	if d.task(adopted, "access")["status"] == "accepted" || stringify(f.call("GET", definitionPath, nil, 200)) != definition || f.call("GET", "/runs/"+run["id"].(string)+"?project_id="+f.project, nil, 200)["state"] != "succeeded" {
		t.Fatal("historical receipt re-applied acceptance or publication rewrote old execution")
	}
}

func TestRequirementDesignIntegrationDeltaReusesIdentityAndArchivesExcludedTasks(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	f := d.f
	tasks := append(designIntegrationTasks(), RequirementDesignTask{Key: "verify", Title: "Verify isolation", Description: "Independent verification.", Priority: "medium", DependsOn: []string{"access"}, AcceptanceCriteria: []string{"Verify the accepted candidate"}, Covers: []int{1}})
	view := d.publish(designIntegrationContent(), tasks)
	preview := d.preview(view)
	if preview["create_count"] != float64(2) || len(preview["plan_sha256"].(string)) != 64 {
		t.Fatal("creation preview is incomplete", preview)
	}
	body := d.adoptBody(preview)
	adoption := f.call("POST", d.path()+"/adopt", body, 200)
	a, b := d.task(adoption, "access"), d.task(adoption, "verify")
	deps := b["depends_on"].([]any)
	if len(deps) != 1 || deps[0] != a["id"] {
		t.Fatal("stable proposal keys were not mapped to actual task IDs", b)
	}
	if replay := f.call("POST", d.path()+"/adopt", body, 200); replay["id"] != adoption["id"] {
		t.Fatal("lost-response replay duplicated adoption")
	}
	noChange := d.preview(d.current())
	if noChange["has_changes"] != false || noChange["keep_count"] != float64(2) {
		t.Fatal("same publication does not preserve adopted tasks", noChange)
	}
	f.call("POST", d.path()+"/adopt", d.adoptBody(noChange), 200)
	if currentA, currentB := d.task(adoption, "access"), d.task(adoption, "verify"); currentA["version"] != a["version"] || currentB["version"] != b["version"] || currentA["id"] != a["id"] || currentB["id"] != b["id"] {
		t.Fatal("a new operation key rewrote or duplicated unchanged tasks")
	}

	// A user's separate task edit is visible in the next reviewed delta and
	// invalidates a prior complete-scope plan rather than being overwritten.
	stale := d.adoptBody(d.preview(d.current()))
	f.call("PATCH", "/tasks/"+a["id"].(string), f.body(map[string]any{"expected_version": a["version"], "title": "An independent user edit"}), 200)
	before := designIntegrationEffects(t, d)
	f.call("POST", d.path()+"/adopt", stale, 409)
	if designIntegrationEffects(t, d) != before {
		t.Fatal("stale task-scope adoption had partial effects")
	}
	corrected := d.preview(d.current())
	if corrected["update_count"] != float64(1) || corrected["keep_count"] != float64(1) {
		t.Fatal("independent edit was not exposed as a reviewed task update", corrected)
	}
	f.call("POST", d.path()+"/adopt", d.adoptBody(corrected), 200)

	view = d.publish(designIntegrationContent(), tasks[:1])
	excluded := d.preview(view)
	if excluded["exclude_count"] != float64(1) {
		t.Fatal("removing a stable key did not expose an archive", excluded)
	}
	f.call("POST", d.path()+"/adopt", d.adoptBody(excluded), 200)
	archived := f.call("GET", "/tasks/"+b["id"].(string)+"?project_id="+f.project, nil, 200)
	if archived["status"] != "archived" {
		t.Fatal("excluded task was deleted or reopened", archived)
	}
	view = d.publish(designIntegrationContent(), tasks)
	restored := d.task(d.adopt(view), "verify")
	if restored["id"] != b["id"] || restored["status"] != "todo" {
		t.Fatal("reintroduced key did not reuse its archived task", restored)
	}
	all := f.call("GET", "/tasks?project_id="+f.project, nil, 200)
	if all["total"] != float64(2) {
		t.Fatal("delta adoption duplicated existing task identities", all)
	}
}

func TestRequirementDesignIntegrationSourceScopeValidationAndAtomicRollback(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	f := d.f
	tasks := append(designIntegrationTasks(), RequirementDesignTask{Key: "verify", Title: "Verify isolated access", Description: "Verify current scope.", Priority: "medium", DependsOn: []string{}, AcceptanceCriteria: []string{"Current scope is enforced"}, Covers: []int{1}})
	view := d.publish(designIntegrationContent(), tasks)
	adoption := d.adopt(view)
	a, b := d.task(adoption, "access"), d.task(adoption, "verify")

	// Reject malformed dependency graphs before publication. A valid draft may
	// retain unfinished graph diagnostics without creating any formal tasks.
	cyclic := append([]RequirementDesignTask{}, tasks...)
	cyclic[0].DependsOn, cyclic[1].DependsOn = []string{"verify"}, []string{"access"}
	draft := d.save(d.current(), designIntegrationContent(), cyclic)
	before := designIntegrationEffects(t, d)
	f.call("POST", d.path()+"/commands", d.commandBody(draft, "publish"), 400)
	if designIntegrationEffects(t, d) != before {
		t.Fatal("invalid graph publication wrote formal state")
	}
	changed := designIntegrationContent()
	changed.Product += " A reviewed replacement."
	view = d.publish(changed, tasks)
	preview := d.preview(view)
	body := d.adoptBody(preview)
	before = designIntegrationEffects(t, d)

	// A deterministic database fault after the first stable-key task update
	// proves the real multi-task transaction rolls back. It does not fabricate
	// business data or approval and is confined to this temporary fixture DB.
	if _, err := f.s.DB.Exec(`CREATE TRIGGER requirement_design_integration_abort BEFORE UPDATE ON suma_tasks WHEN OLD.id='` + b["id"].(string) + `' BEGIN SELECT RAISE(ABORT,'design integration write fault'); END`); err != nil {
		t.Fatal(err)
	}
	f.call("POST", d.path()+"/adopt", body, 500)
	if _, err := f.s.DB.Exec(`DROP TRIGGER requirement_design_integration_abort`); err != nil {
		t.Fatal(err)
	}
	if designIntegrationEffects(t, d) != before {
		t.Fatal("mid-batch fault left changed tasks, bindings, adoption, receipt or audit")
	}
	f.call("POST", d.path()+"/adopt", body, 200)
	if d.task(adoption, "access")["id"] != a["id"] {
		t.Fatal("retry after rolled-back adoption changed task identity")
	}

	stale := d.adoptBody(d.preview(d.current()))
	d.req = f.call("PATCH", "/requirements/"+d.req["id"].(string), f.body(map[string]any{"expected_version": d.req["version"], "description": "The formal requirement changed after design review."}), 200)
	if d.current()["eligibility"] != "requirement_changed" {
		t.Fatal("changed source was presented as a current publication")
	}
	before = designIntegrationEffects(t, d)
	f.call("POST", d.path()+"/adopt", stale, 409)
	if designIntegrationEffects(t, d) != before {
		t.Fatal("changed requirement adoption produced partial effects")
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Different design project", "idempotency_key": newID()}, 201)
	f.call("GET", d.path()+"?project_id="+other["id"].(string), nil, 404)
	foreign := d.adoptBody(d.preview(d.publish(designIntegrationContent(), tasks)))
	foreign["project_id"] = other["id"]
	before = designIntegrationEffects(t, d)
	f.call("POST", d.path()+"/adopt", foreign, 404)
	if designIntegrationEffects(t, d) != before {
		t.Fatal("cross-project adoption touched the source project")
	}
}

func TestRequirementDesignIntegrationCurrentAuthorityBeforeSuccessfulReceipts(t *testing.T) {
	d := newDesignIntegrationFixture(t, false)
	f := d.f
	view := d.save(d.current(), designIntegrationContent(), designIntegrationTasks())
	api := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	f.bearer("GET", d.path()+"?project_id="+f.project, api, nil, 200)
	view = f.bearer("PUT", d.path(), api, f.body(map[string]any{"expected_version": view["version"], "expected_requirement_version": d.req["version"], "content": designIntegrationContent(), "tasks": designIntegrationTasks()}), 200)
	command := d.commandBody(view, "publish")
	f.bearer("POST", d.path()+"/commands", api, command, 403)
	view = f.call("POST", d.path()+"/commands", command, 200)
	adopt := d.adoptBody(d.preview(view))
	f.bearer("POST", d.path()+"/adopt", api, adopt, 403)
	f.call("POST", d.path()+"/adopt", adopt, 200)
	for _, kind := range []string{"agent", "runner"} {
		token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": kind}), 201)["token"].(string)
		f.bearer("GET", d.path()+"?project_id="+f.project, token, nil, 403)
		f.bearer("POST", d.path()+"/commands", token, command, 403)
		f.bearer("POST", d.path()+"/adopt", token, adopt, 403)
	}
	ownerCookie := f.cookie
	oldActor := f.actor
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.cookie = ownerCookie
	f.call("GET", d.path()+"?project_id="+f.project, nil, 401)
	f.call("POST", d.path()+"/commands", command, 401)
	f.call("POST", d.path()+"/adopt", adopt, 401)
	// Independent API authority survives the browser family logout, but its
	// scope cannot turn a historical browser receipt into approval authority.
	f.bearer("GET", d.path()+"?project_id="+f.project, api, nil, 200)
	f.bearer("POST", d.path()+"/commands", api, command, 403)
	f.bearer("POST", d.path()+"/adopt", api, adopt, 403)
	var dto RequirementDesignAdoptRequest
	raw, _ := json.Marshal(adopt)
	if err := json.Unmarshal(raw, &dto); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdoptRequirementDesign(context.Background(), oldActor, f.project, d.req["id"].(string), dto); err == nil {
		t.Fatal("detached revoked actor recovered an approval receipt")
	}
}
