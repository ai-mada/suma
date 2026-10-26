package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type intelligentFixture struct {
	f       *fixture
	token   string
	profile ToolProfile
	task    Record
	request PlanningAnalysisCreateRequest
}

func newIntelligentFixture(t *testing.T) *intelligentFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "planning-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	p := toolTestProfile()
	p.ReadOnly = true
	p.Planning = &toolcontract.PlanningCapability{Protocol: toolcontract.PlanningProtocol, Model: "local-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "planning-worker", Name: "Read-only planner fixture", Profile: p, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p, Available: true}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Existing task for model scheduling", "description": "Read this task without executing code"}))
	if err != nil {
		t.Fatal(err)
	}
	return &intelligentFixture{f: f, token: token, profile: profile, task: task, request: PlanningAnalysisCreateRequest{ProjectID: f.project, Goal: "Prioritize this task and explain schedule risk", Locale: "en-US", TaskIDs: []string{task.ID}, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
}
func (h *intelligentFixture) create(t *testing.T) PlanningAnalysis {
	t.Helper()
	out, err := h.f.s.CreatePlanningAnalysis(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func (h *intelligentFixture) claim(t *testing.T) dag.Assignment {
	t.Helper()
	claimed := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	b, _ := json.Marshal(claimed["assignment"])
	if json.Unmarshal(b, &a) != nil || a.RunID == "" {
		t.Fatal("missing planner assignment", claimed)
	}
	return a
}
func (h *intelligentFixture) output() string {
	return stringify(PlanningOutput{SchemaVersion: 1, Summary: "Schedule the selected task", Rationale: "Capacity is limited; keep explicit dates", Actions: []PlanningScheduleAction{{TaskID: h.task.ID, StartDate: "2026-10-26", DueDate: "2026-10-30"}}, Risks: []PlanningRisk{{Key: "capacity", Severity: "warning", Title: "Capacity needs review", Detail: "Model recommendation, not a confirmed incident", TaskIDs: []string{h.task.ID}}}})
}
func (h *intelligentFixture) result(a dag.Assignment, output string) dag.Result {
	return dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Output: output, ToolEvidence: &toolcontract.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}}
}
func (h *intelligentFixture) complete(t *testing.T, result dag.Result) {
	t.Helper()
	h.f.bearer("POST", "/runner/results", h.token, h.f.body(map[string]any{"result": result}), 200)
}
func (h *intelligentFixture) get(t *testing.T, id string) PlanningAnalysis {
	t.Helper()
	a, e := h.f.s.GetPlanningAnalysis(context.Background(), h.f.actor, h.f.project, id)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (h *intelligentFixture) command(a PlanningAnalysis, action string) PlanningAnalysisCommandRequest {
	return PlanningAnalysisCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: action, Reason: "Explicit fixture reviewer decision", AcknowledgeScheduleChanges: true, IdempotencyKey: newID()}
}

func TestIntelligentPlanningStrictModelSchema(t *testing.T) {
	source := PlanningSource{Tasks: []PlanningSourceTask{{Task: TaskContextRecord{ID: "task"}}}}
	valid := `{"schema_version":1,"summary":"plan","rationale":"reason","actions":[{"task_id":"task","iteration_id":"","milestone_id":"","start_date":"","due_date":"","estimated_hours":null}],"risks":[]}`
	if _, err := parsePlanningOutput(valid, source); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{"unknown": strings.Replace(valid, `"summary":"plan"`, `"summary":"plan","command":"shell"`, 1), "duplicate": strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), "foreign": strings.Replace(valid, `"task_id":"task"`, `"task_id":"other"`, 1), "missinghours": strings.Replace(valid, `,"estimated_hours":null`, "", 1), "nullarray": strings.Replace(valid, `"risks":[]`, `"risks":null`, 1), "fraction": strings.Replace(valid, `"schema_version":1`, `"schema_version":1.1`, 1), "trailing": valid + `{}`, "negative": strings.Replace(valid, `"estimated_hours":null`, `"estimated_hours":-1`, 1)} {
		t.Run(name, func(t *testing.T) {
			if _, err := parsePlanningOutput(bad, source); err == nil {
				t.Fatal("accepted malformed model output")
			}
		})
	}
}

func TestIntelligentPlanningCreateAuthorityBudgetAndReplay(t *testing.T) {
	h := newIntelligentFixture(t)
	a := h.create(t)
	again := h.create(t)
	if a.ID != again.ID || a.RunID != again.RunID {
		t.Fatal("create replay launched another model run")
	}
	var runs int
	if err := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE id=?`, a.RunID).Scan(&runs); err != nil || runs != 1 {
		t.Fatal(runs, err)
	}
	if a.MaxSteps != 3 || a.MaxOutputTokens != 4096 || a.State != "running" || len(a.AllowedActions) != 0 || !a.BasisCurrent {
		t.Fatal(a)
	}
	bad := h.request
	bad.IdempotencyKey = newID()
	bad.MaxSteps = 5
	if _, err := h.f.s.CreatePlanningAnalysis(context.Background(), h.f.e, h.f.actor, h.f.project, bad); err == nil {
		t.Fatal("unbounded planning accepted")
	}
	bad = h.request
	bad.IdempotencyKey = newID()
	bad.AcknowledgeModelUse = false
	if _, err := h.f.s.CreatePlanningAnalysis(context.Background(), h.f.e, h.f.actor, h.f.project, bad); err == nil {
		t.Fatal("missing model consent accepted")
	}
	actor := toolActorForTest(t, h.f, h.token)
	if _, err := h.f.s.CreatePlanningAnalysis(context.Background(), h.f.e, actor, h.f.project, h.request); err == nil {
		t.Fatal("runner impersonated browser")
	}
	if _, err := h.f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	bad = h.request
	bad.IdempotencyKey = newID()
	if _, err := h.f.s.CreatePlanningAnalysis(context.Background(), h.f.e, h.f.actor, h.f.project, bad); err == nil {
		t.Fatal("stale planner available")
	}
}

func TestIntelligentPlanningHTTPResultAlertsReviseAndAtomicApply(t *testing.T) {
	h := newIntelligentFixture(t)
	a := h.create(t)
	assignment := h.claim(t)
	if assignment.Config["purpose"] != "planning" || assignment.Idempotent || assignment.TimeoutSeconds != 120 {
		t.Fatal(assignment)
	}
	original := h.output()
	result := h.result(assignment, original)
	h.complete(t, result)
	h.complete(t, result)
	a = h.get(t, a.ID)
	if a.State != "ready" || a.Proposal == nil || a.OriginalOutput != original || a.OriginalOutputSHA256 != digest(original) {
		t.Fatal(a)
	}
	page, err := h.f.s.ListPlanningAlerts(context.Background(), h.f.actor, h.f.project, "", 20, "")
	if err != nil || len(page.Items) != 1 || page.OpenCount != 1 {
		t.Fatal(page, err)
	}
	ctx, auth, close, err := h.f.s.planningAuthority(context.Background(), h.f.actor, h.f.project, "", purposeBrowserProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	err = h.f.s.recordPlanningOutcomeTx(ctx, auth.tx, h.f.user.TenantID, h.f.project, result, func(tx *sql.Tx) error { return assertProjectCommand(tx, auth, h.f.project) })
	if err != nil {
		close()
		t.Fatal(err)
	}
	if err = auth.tx.Commit(); err != nil {
		close()
		t.Fatal(err)
	}
	close()
	req := h.command(a, "revise")
	proposal := *a.Proposal
	proposal.Summary = "Human revised summary"
	req.Proposal = &proposal
	revised, err := h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req)
	if err != nil || revised.OriginalOutput != original || revised.Proposal.Summary != proposal.Summary {
		t.Fatal(revised, err)
	}
	req = h.command(revised, "apply")
	req.AcknowledgeScheduleChanges = false
	if _, err = h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req); err == nil {
		t.Fatal("silent adoption")
	}
	req = h.command(revised, "apply")
	applied, err := h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req)
	if err != nil || applied.State != "applied" {
		t.Fatal(applied, err)
	}
	again, err := h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req)
	if err != nil || again.Version != applied.Version {
		t.Fatal("apply replay", again, err)
	}
	task, err := h.f.s.Get(context.Background(), h.f.actor, h.f.project, "tasks", h.task.ID)
	if err != nil || task.Status != h.task.Status || task.Version != h.task.Version {
		t.Fatal("planning changed business acceptance", task, err)
	}
	var version int
	var start string
	if err = h.f.s.DB.QueryRow(`SELECT version,start_date FROM suma_task_plans WHERE task_id=?`, h.task.ID).Scan(&version, &start); err != nil || version != 1 || start != "2026-10-26" {
		t.Fatal(version, start, err)
	}
}

func TestIntelligentPlanningSourceDriftKeepsRealRunResult(t *testing.T) {
	h := newIntelligentFixture(t)
	a := h.create(t)
	assignment := h.claim(t)
	if _, err := h.f.s.DB.Exec(`UPDATE suma_tasks SET version=version+1,title='Changed formal task' WHERE id=?`, h.task.ID); err != nil {
		t.Fatal(err)
	}
	h.complete(t, h.result(assignment, h.output()))
	a = h.get(t, a.ID)
	if a.State != "failed" || a.BasisCurrent || a.Proposal != nil {
		t.Fatal(a)
	}
	run := h.f.call("GET", "/runs/"+a.RunID+"?project_id="+h.f.project, nil, 200)
	if run["state"] != "succeeded" {
		t.Fatal("lost already occurred process fact", run)
	}
	alerts, err := h.f.s.ListPlanningAlerts(context.Background(), h.f.actor, h.f.project, "", 20, "")
	if err != nil || alerts.Total != 0 {
		t.Fatal(alerts, err)
	}
}

func TestIntelligentPlanningUnknownAndMalformedNeverAutoRetry(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "malformed", true: "unknown"}[unknown], func(t *testing.T) {
			h := newIntelligentFixture(t)
			a := h.create(t)
			assignment := h.claim(t)
			result := h.result(assignment, `not JSON`)
			if unknown {
				result.OutcomeUnknown = true
				result.ToolEvidence.Terminal = "unknown"
			}
			h.complete(t, result)
			a = h.get(t, a.ID)
			expected := "failed"
			if unknown {
				expected = "reconciling"
			}
			if a.State != expected || a.Proposal != nil {
				t.Fatal(a)
			}
			if unknown {
				req := h.command(a, "dismiss")
				if _, err := h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req); err == nil {
					t.Fatal("discarded unknown execution")
				}
			}
			var attempt int
			if err := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=?`, a.RunID).Scan(&attempt); err != nil || attempt != 1 {
				t.Fatal(attempt, err)
			}
		})
	}
}

func TestIntelligentPlanningAlertCASAndScopedPages(t *testing.T) {
	h := newIntelligentFixture(t)
	a := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output()))
	page, err := h.f.s.ListPlanningAlerts(context.Background(), h.f.actor, h.f.project, "open", 1, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	alert := page.Items[0]
	req := PlanningAlertCommandRequest{ProjectID: h.f.project, ExpectedVersion: alert.Version, Action: "acknowledge", Reason: "Human reviewed the model recommendation", IdempotencyKey: newID()}
	out, err := h.f.s.CommandPlanningAlert(context.Background(), h.f.actor, h.f.project, alert.ID, req)
	if err != nil || out.State != "acknowledged" {
		t.Fatal(out, err)
	}
	req.IdempotencyKey = newID()
	if _, err = h.f.s.CommandPlanningAlert(context.Background(), h.f.actor, h.f.project, alert.ID, req); err == nil {
		t.Fatal("stale alert CAS accepted")
	}
	req.ExpectedVersion = out.Version
	req.Action = "resolve"
	req.IdempotencyKey = newID()
	out, err = h.f.s.CommandPlanningAlert(context.Background(), h.f.actor, h.f.project, alert.ID, req)
	if err != nil || out.State != "resolved" {
		t.Fatal(out, err)
	}
	analyses, err := h.f.s.ListPlanningAnalyses(context.Background(), h.f.actor, h.f.project, 1, "")
	if err != nil || len(analyses.Items) != 1 || analyses.Items[0].ID != a.ID {
		t.Fatal(analyses, err)
	}
	if _, err = h.f.s.ListPlanningAnalyses(context.Background(), h.f.actor, h.f.project, 1, "foreign-cursor"); err == nil {
		t.Fatal("foreign cursor accepted")
	}
}

func TestIntelligentPlanningAdmissionCredentialAndApplyRollback(t *testing.T) {
	h := newIntelligentFixture(t)
	a := h.create(t)
	assignment := h.claim(t)
	h.complete(t, h.result(assignment, h.output()))
	a = h.get(t, a.ID)
	req := h.command(a, "revise")
	proposal := *a.Proposal
	proposal.Actions = append([]PlanningScheduleAction{}, proposal.Actions...)
	proposal.Actions[0].IterationID = "missing-period"
	req.Proposal = &proposal
	a, err := h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	req = h.command(a, "apply")
	if _, err = h.f.s.CommandPlanningAnalysis(context.Background(), h.f.actor, h.f.project, a.ID, req); err == nil {
		t.Fatal("invalid period applied")
	}
	if h.get(t, a.ID).Version != a.Version {
		t.Fatal("partial analysis CAS")
	}
	var count int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_task_plans WHERE task_id=?`, h.task.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial schedule", count, err)
	}
	h.request.IdempotencyKey = newID()
	b := h.create(t)
	if _, err = h.f.s.DB.Exec(`UPDATE suma_credential_slots SET revoked=1 WHERE current_digest=?`, h.f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	tx, err := h.f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = h.f.s.checkPlanningExecutionTx(context.Background(), tx, h.f.user.TenantID, h.f.project, b.RunID); err == nil {
		t.Fatal("revoked creating credential admitted")
	}
}
