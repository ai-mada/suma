package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

type workBuddyTaskDispatchFixture struct {
	f                *fixture
	w                *WorkBuddyConnections
	req              WorkBuddyTaskDispatchPreviewRequest
	task             Record
	creates, queries *atomic.Int32
}

func workBuddyTaskPreview(t *testing.T, h workBuddyTaskDispatchFixture) WorkBuddyTaskDispatchPlan {
	t.Helper()
	plan, err := h.w.CreateTaskDispatchPlan(context.Background(), h.f.e, h.f.actor, h.f.project, h.req)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestWorkBuddyTaskDispatchWorkflowSurvivesTypedJSONRoundTrip(t *testing.T) {
	workflow, specSHA, _, err := workBuddyTaskWorkflow(TaskContext{Task: TaskContextRecord{ID: "task", Version: 1, Title: "Fixed goal", AcceptanceCriteria: []string{"Keep evidence"}}}, "connection", 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	var reloaded dag.Workflow
	if err = json.Unmarshal([]byte(stringify(workflow)), &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Nodes[0].Command == nil || reloaded.Nodes[0].DependsOn == nil {
		t.Fatal("new plan arrays are null")
	}
	normalizedNode := reloaded.Nodes[0]
	normalizedNode.DependsOn = nil
	if digest(stringify(normalizedNode)) != specSHA || stringify(reloaded) != stringify(workflow) {
		t.Fatal("generated node spec changes after its normal durable typed JSON reload")
	}
}
func workBuddyTaskStart(t *testing.T, h workBuddyTaskDispatchFixture, plan WorkBuddyTaskDispatchPlan, key string) dag.Run {
	t.Helper()
	run, err := h.w.StartTaskDispatchPlan(context.Background(), h.f.e, h.f.actor, h.f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: h.f.project, ExpectedVersion: plan.Version, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestWorkBuddyTaskDispatchStartAtomicOriginalReplayAndProvenance(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	ctx := context.Background()
	// Metadata-only preview must succeed without decrypting this nonempty but
	// invalid ciphertext. Restore it before any actual Runner provider operation.
	var ciphertext string
	if err := f.s.DB.QueryRow(`SELECT ciphertext FROM suma_workbuddy_connections WHERE id=?`, h.req.ConnectionID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET ciphertext='not-decryptable' WHERE id=?`, h.req.ConnectionID); err != nil {
		t.Fatal(err)
	}
	plan := workBuddyTaskPreview(t, h)
	if _, err := f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET ciphertext=? WHERE id=?`, ciphertext, h.req.ConnectionID); err != nil {
		t.Fatal(err)
	}
	key := newID()
	run := workBuddyTaskStart(t, h, plan, key)
	if run.ID != plan.RunID || len(run.Nodes) != 1 || run.Nodes[0].Attempt != 0 || run.Nodes[0].State != "ready" {
		t.Fatal("run was not activated after atomic binding", run)
	}
	view, err := h.w.GetCloudNode(ctx, f.actor, f.project, run.ID, plan.NodeID)
	if err != nil || view.Delegation == nil || view.Delegation.ID != plan.DelegationID || view.Delegation.CreatesConsumed != 0 {
		t.Fatal("exact reserved delegation missing", err)
	}
	provenance, err := f.s.ListTaskRunProvenance(ctx, f.actor, f.project, h.task.ID, 20, "")
	if err != nil || len(provenance.Items) != 1 || provenance.Items[0].PlanID != plan.ID || provenance.Items[0].TaskVersion == nil || *provenance.Items[0].TaskVersion != h.task.Version+1 {
		t.Fatal("formal task provenance missing", provenance, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET state='disconnected' WHERE id=?`, h.req.ConnectionID); err != nil {
		t.Fatal(err)
	}
	replay := workBuddyTaskStart(t, h, plan, key)
	if replay.ID != run.ID || domainACount(t, f.s, "dag_runs") != 1 || domainACount(t, f.s, "suma_workbuddy_cloud_delegations") != 1 || h.creates.Load() != 0 {
		t.Fatal("old receipt recreated work")
	}
	_, err = h.w.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 2, IdempotencyKey: key})
	requireAuthorityStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = h.w.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: key})
	requireAuthorityStatus(t, err, 403)
}

func TestWorkBuddyTaskDispatchRejectsChangedInputsBeforeRun(t *testing.T) {
	for _, kind := range []string{"task", "connection", "grant", "route", "expired", "organization_budget", "project_budget", "pool_grant"} {
		t.Run(kind, func(t *testing.T) {
			h := newWorkBuddyTaskDispatchFixture(t)
			f := h.f
			var poolGrantID string
			if kind == "pool_grant" {
				_, grant := dispatchAuthorizedPool(t, f)
				dispatchSetRoute(t, f, 0, &grant)
				poolGrantID = grant.ID
			}
			plan := workBuddyTaskPreview(t, h)
			ctx := context.Background()
			var err error
			switch kind {
			case "pool_grant":
				_, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, poolGrantID)
			case "task":
				_, err = f.s.DB.Exec(`UPDATE suma_tasks SET version=version+1 WHERE id=?`, h.task.ID)
			case "connection":
				_, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET version=version+1 WHERE id=?`, h.req.ConnectionID)
			case "grant":
				_, err = f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET grant_generation=grant_generation+1 WHERE id=?`, h.req.ConnectionID)
			case "route":
				_, grant := dispatchAuthorizedPool(t, f)
				dispatchSetRoute(t, f, 0, &grant)
			case "expired":
				var raw string
				if err = f.s.DB.QueryRow(`SELECT payload FROM suma_workbuddy_task_dispatch_plans WHERE id=?`, plan.ID).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var snapshot workBuddyTaskDispatchSnapshot
				if err = json.Unmarshal([]byte(raw), &snapshot); err != nil {
					t.Fatal(err)
				}
				snapshot.Plan.ExpiresAt = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
				_, err = f.s.DB.Exec(`UPDATE suma_workbuddy_task_dispatch_plans SET expires_at=?,payload=? WHERE id=?`, snapshot.Plan.ExpiresAt, stringify(snapshot), plan.ID)
			default:
				scope := "project"
				if kind == "organization_budget" {
					scope = "organization"
				}
				_, err = f.s.WriteUsagePolicy(ctx, f.actor, f.project, UsagePolicyWriteRequest{ProjectID: f.project, Scope: scope, ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "Explicit fixed dispatch policy", IdempotencyKey: newID()})
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = h.w.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
			requireAuthorityStatus(t, err, 409)
			if domainACount(t, f.s, "dag_runs") != 0 || domainACount(t, f.s, "suma_workbuddy_cloud_delegations") != 0 || h.creates.Load() != 0 {
				t.Fatal("stale plan produced work")
			}
		})
	}
}

func TestWorkBuddyTaskDispatchRejectsUnsupportedTaskBoundaries(t *testing.T) {
	if _, _, _, err := workBuddyTaskWorkflow(TaskContext{CodeBaseline: &ccmp.CodeBaselineBinding{}}, "connection", 1, "run"); err == nil {
		t.Fatal("code baseline silently dropped")
	}
	for _, kind := range []string{"accepted", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			h := newWorkBuddyTaskDispatchFixture(t)
			var err error
			if kind == "accepted" {
				_, err = h.f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted' WHERE id=?`, h.task.ID)
			} else {
				_, err = h.f.s.DB.Exec(`UPDATE suma_tasks SET description=? WHERE id=?`, strings.Repeat("界", 11000), h.task.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = h.w.CreateTaskDispatchPlan(context.Background(), h.f.e, h.f.actor, h.f.project, h.req); err == nil {
				t.Fatal("unsupported task accepted")
			}
			if domainACount(t, h.f.s, workBuddyTaskDispatchTable) != 0 || h.creates.Load() != 0 {
				t.Fatal("rejected task left plan/effect")
			}
		})
	}
}

func TestWorkBuddyTaskDispatchOwnerAndCredentialBoundaries(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	ctx := context.Background()
	plan := workBuddyTaskPreview(t, h)
	other := cloudOtherWriter(t, &cloudFixture{f: f, w: h.w})
	_, err := h.w.GetTaskDispatchPlan(ctx, f.e, other, f.project, plan.ID)
	requireAuthorityStatus(t, err, 404)
	_, err = h.w.StartTaskDispatchPlan(ctx, f.e, other, f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 404)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		actor, e := f.s.AuthenticateActor(ctx, issued.Token)
		if e != nil {
			t.Fatal(e)
		}
		_, e = h.w.CreateTaskDispatchPlan(ctx, f.e, actor, f.project, h.req)
		requireAuthorityStatus(t, e, 403)
		_, e = h.w.GetTaskDispatchPlan(ctx, f.e, actor, f.project, plan.ID)
		requireAuthorityStatus(t, e, 403)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, other.Projection().ID); err != nil {
		t.Fatal(err)
	}
	_, err = h.w.CreateTaskDispatchPlan(ctx, f.e, other, f.project, h.req)
	requireAuthorityStatus(t, err, 403)
}

func TestWorkBuddyTaskDispatchStartRollbackLeavesNoPartialRun(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	plan := workBuddyTaskPreview(t, h)
	key := "workbuddy-task-rollback168"
	before := map[string]int{}
	for _, table := range []string{"suma_operations", "suma_tasks", "suma_activity", "dag_workflows", "dag_managed_workflows", "dag_runs", "suma_task_run_provenance", "suma_workbuddy_cloud_delegations", "suma_test_plan_runs"} {
		before[table] = domainACount(t, f.s, table)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER workbuddy_dispatch_reject BEFORE INSERT ON suma_operations WHEN NEW.operation_id='workbuddy-task-rollback168' BEGIN SELECT RAISE(ABORT,'original start receipt rejected'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := h.w.StartTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: key})
	if err == nil {
		t.Fatal("receipt failure committed start")
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("partial original start", table)
		}
	}
	var version int
	if err = f.s.DB.QueryRow(`SELECT version FROM suma_tasks WHERE id=?`, h.task.ID).Scan(&version); err != nil || version != h.task.Version {
		t.Fatal("task binding survived rollback", err)
	}
	if _, err = f.s.DB.Exec(`DROP TRIGGER workbuddy_dispatch_reject`); err != nil {
		t.Fatal(err)
	}
	workBuddyTaskStart(t, h, plan, key)
}

func TestWorkBuddyTaskDispatchConcurrentStartsCreateOneRun(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	plan := workBuddyTaskPreview(t, h)
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := h.w.StartTaskDispatchPlan(context.Background(), h.f.e, h.f.actor, h.f.project, plan.ID, WorkBuddyTaskDispatchStartRequest{ProjectID: h.f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireAuthorityStatus(t, err, 409)
		}
	}
	if success != 1 || domainACount(t, h.f.s, "dag_runs") != 1 || domainACount(t, h.f.s, "suma_workbuddy_cloud_delegations") != 1 {
		t.Fatal("concurrent starts duplicated work", success)
	}
}

func TestWorkBuddyTaskDispatchPublishedTestPlanIsSnapshotOnly(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	ctx := context.Background()
	// Use the original real design publication/adoption commands; the generated
	// task must bind this same protected design, not an editable config claim.
	design := &designIntegrationFixture{f: f, req: f.call("POST", "/requirements", f.body(map[string]any{"title": "Cloud design objective", "acceptance_criteria": []string{"Reject a foreign project"}}), 201)}
	adopted := design.adopt(design.publish(designIntegrationContent(), designIntegrationTasks()))
	if err := json.Unmarshal([]byte(stringify(design.task(adopted, "access"))), &h.task); err != nil {
		t.Fatal(err)
	}
	h.req.TaskID, h.req.ExpectedTaskVersion = h.task.ID, h.task.Version
	requirement := f.call("GET", "/requirements/"+h.task.RequirementID+"?project_id="+f.project, nil, 200)
	quality, err := f.s.CreateTestPlan(ctx, f.actor, f.project, TestPlanCreateRequest{ProjectID: f.project, TaskID: h.task.ID, ExpectedTaskVersion: h.task.Version, ExpectedRequirementVersion: int(requirement["version"].(float64)), Title: "Cloud delivery still requires independent evidence", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	quality, err = f.s.CommandTestPlan(ctx, f.actor, f.project, quality.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: quality.Version, Action: "publish", Reason: "Reviewed quality basis", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	plan := workBuddyTaskPreview(t, h)
	if plan.Context.TestPlan == nil || plan.Context.Design == nil || plan.Context.Requirement == nil {
		t.Fatal("published quality omitted")
	}
	run := workBuddyTaskStart(t, h, plan, newID())
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	fixed, found, err := f.s.testPlanRunTx(ctx, tx, f.user.TenantID, f.project, run.ID)
	if err != nil || !found || fixed.Revision == nil || fixed.Revision.SHA256 != plan.Context.TestPlan.SHA256 || len(fixed.PackageTests) != 0 {
		t.Fatal("cloud quality snapshot invented mapping or lost revision", err)
	}
	requireAuthorityStatus(t, f.s.checkTestPlanRunEvidenceTx(ctx, tx, f.user.TenantID, f.project, run.ID), 409)
}

func newWorkBuddyTaskDispatchFixture(t *testing.T) workBuddyTaskDispatchFixture {
	t.Helper()
	creates, queries := &atomic.Int32{}, &atomic.Int32{}
	f, w := workBuddyCommandFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/openapi/v2/tasks" {
			creates.Add(1)
			w.WriteHeader(201)
			fmt.Fprint(w, `{"task_id":"task-dispatch-cloud","status":"completed","name":"Cloud task"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/openapi/v2/tasks/") {
			queries.Add(1)
			fmt.Fprint(w, `{"task_id":"task-dispatch-cloud","status":"completed","name":"Cloud task"}`)
			return
		}
		workBuddyCommandResponse(w, "open-one", "user.task.readable user.task.invokable", "private-refresh")
	})
	if err := f.s.InitializeWorkBuddyTaskDispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	auth, _ := workBuddyCommandStart(t, f, w)
	connection, err := w.CompleteAuthorization(context.Background(), f.actor, f.project, auth.ID, workBuddyCommandComplete(auth))
	if err != nil {
		t.Fatal(err)
	}
	created := f.call("POST", "/tasks", f.body(map[string]any{"title": "Explicit WorkBuddy objective", "description": "Preserve fixed task content", "acceptance_criteria": []string{"Independent evidence remains required"}}), 201)
	var task Record
	if err = json.Unmarshal([]byte(stringify(created)), &task); err != nil {
		t.Fatal(err)
	}
	return workBuddyTaskDispatchFixture{f: f, w: w, task: task, creates: creates, queries: queries, req: WorkBuddyTaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ConnectionID: connection.ID, ExpectedConnectionVersion: connection.Version, GrantGeneration: connection.GrantGeneration, IdempotencyKey: newID()}}
}

func TestWorkBuddyTaskDispatchPreviewFreezesTaskWithoutExternalEffect(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	before := map[string]int{}
	for _, table := range []string{"dag_runs", "suma_workbuddy_cloud_delegations", "suma_workbuddy_cloud_submissions", "suma_tasks"} {
		before[table] = domainACount(t, f.s, table)
	}
	plan, err := h.w.CreateTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, h.req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TaskID != h.task.ID || plan.TaskVersion != h.task.Version || plan.OwnerID != f.user.ID || plan.Context.Task.Title != h.task.Title || plan.State != "ready" || !plan.Valid || plan.DelegationID == "" || len(plan.Workflow.Nodes) != 1 || plan.RunID == "" {
		t.Fatal("preview did not freeze exact task", plan)
	}
	if h.creates.Load() != 0 || h.queries.Load() != 0 {
		t.Fatal("preview called provider")
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("preview changed business fact", table)
		}
	}
	var version int
	if err = f.s.DB.QueryRow(`SELECT version FROM suma_tasks WHERE id=?`, h.task.ID).Scan(&version); err != nil || version != h.task.Version {
		t.Fatal("preview changed task version", err)
	}
}
