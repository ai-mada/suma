package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestProjectCommandClaimRemainsLocalAfterPoolMigration(t *testing.T) {
	f := newFixture(t)
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "route-local-fixture", "profiles": []any{}, "deployment": false}), 201)
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "原项目领取入口", "nodes": []dag.Node{{ID: "work", Type: "command", Command: []string{"fixture"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	reply := f.bearer("POST", "/runner/claim", credential["token"].(string), map[string]any{"project_id": f.project, "worker_id": "route-local-fixture", "capabilities": []string{"command:fixture"}, "idempotency_key": newID()}, 200)
	if reply["assignment"] == nil {
		snapshot, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, run["id"].(string))
		t.Fatal("local command disappeared during pool filtering", snapshot, err)
	}
}

func dispatchAuthorizedPool(t *testing.T, f *fixture) (ExecutionPool, ExecutionPoolGrant) {
	t.Helper()
	ctx := context.Background()
	pool, err := f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, ExecutionPoolCreateRequest{Name: "路由校验池", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := f.s.RequestExecutionPoolGrant(ctx, f.user, f.cookie.Value, ExecutionPoolGrantRequest{ProjectID: f.project, PoolID: pool.ID, ProjectWeight: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	grant, err = f.s.CommandOwnedPoolGrant(ctx, f.e, f.user, f.cookie.Value, pool.ID, grant.ID, ExecutionPoolGrantCommand{Action: "approve", ExpectedVersion: grant.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return pool, grant
}

func dispatchSetRoute(t *testing.T, f *fixture, expected int, grant *ExecutionPoolGrant) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, schedulingWriteOptions(f.s.Dialect))
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = f.s.authorizeProjectTx(ctx, tx, f.user, f.project, true, true); err != nil {
		t.Fatal(err)
	}
	in := dag.ProjectExecutionRouteInput{}
	if grant != nil {
		in = dag.ProjectExecutionRouteInput{PoolID: grant.PoolID, GrantID: grant.ID, GrantVersion: grant.Version}
	}
	if _, err = f.e.WriteProjectExecutionRouteTx(ctx, tx, f.user.TenantID, f.project, expected, in); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func dispatchPoolTool(t *testing.T, f *fixture, pool ExecutionPool) ToolProfile {
	t.Helper()
	ctx := context.Background()
	credential, err := f.s.RegisterExecutionPoolRunner(ctx, f.user, f.cookie.Value, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "路由匹配工具", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	profile := toolTestProfile()
	if _, err = f.s.ReportPoolToolInventory(ctx, credential.Token, PoolToolInventoryRequest{RunnerID: credential.Runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err := f.s.CreateToolProfile(ctx, f.user, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: credential.Runner.WorkerID, Name: "路由固定工具", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.user, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal(p, err)
	}
	return p
}

func TestTaskDispatchRouteRejectsUnreachableWorkerBeforePlan(t *testing.T) {
	for _, mode := range []string{"local-tool-pool-route", "pool-tool-local-route", "pool-a-tool-pool-b-route"} {
		t.Run(mode, func(t *testing.T) {
			f, _, request, task := taskDispatchFixture(t)
			pool, grant := dispatchAuthorizedPool(t, f)
			if mode != "local-tool-pool-route" {
				p := dispatchPoolTool(t, f, pool)
				request["tool_profile_id"], request["expected_tool_version"] = p.ID, p.Version
			}
			if mode == "pool-a-tool-pool-b-route" {
				_, other := dispatchAuthorizedPool(t, f)
				grant = other
			}
			if mode != "pool-tool-local-route" {
				dispatchSetRoute(t, f, 0, &grant)
			}
			f.call("POST", "/task-dispatch-plans", request, 409)
			var plans, runs int
			f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_task_dispatch_plans`).Scan(&plans)
			f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs)
			if plans != 0 || runs != 0 {
				t.Fatal("unreachable dispatch left durable side effects", plans, runs)
			}
			current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
			if current["version"] != task["version"] {
				t.Fatal("rejected preview changed task")
			}
		})
	}
}

func TestTaskDispatchRouteRoundTripInvalidatesOldPreviewWithoutBinding(t *testing.T) {
	f, _, request, task := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	dispatchSetRoute(t, f, 1, nil)
	view := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if view["valid"] != false {
		t.Fatal("route generation drift retained valid preview", view)
	}
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	var runs, events int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events`).Scan(&events)
	if runs != 0 || events != 0 {
		t.Fatal("stale route left run/events", runs, events)
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["version"] != task["version"] || current["run_id"] != nil {
		t.Fatal("stale route bound task", current)
	}
}

func TestTaskDispatchMatchingPoolRoutePersistsAndStartedReplayIgnoresNewDefault(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	pool, grant := dispatchAuthorizedPool(t, f)
	p := dispatchPoolTool(t, f, pool)
	dispatchSetRoute(t, f, 0, &grant)
	request["tool_profile_id"], request["expected_tool_version"] = p.ID, p.Version
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	body := f.body(map[string]any{"expected_version": plan["version"]})
	path := "/task-dispatch-plans/" + plan["id"].(string) + "/start"
	raw := f.call("POST", path, body, 201)
	var run dag.Run
	if err := json.Unmarshal([]byte(stringify(raw)), &run); err != nil {
		t.Fatal(err)
	}
	if run.ExecutionPoolID != pool.ID || run.ExecutionRouteVersion != 1 || run.ExecutionPoolGrantID != grant.ID {
		t.Fatal("actual run lost reviewed route", run)
	}
	dispatchSetRoute(t, f, 1, nil)
	if replay := f.call("POST", path, body, 201); replay["id"] != raw["id"] {
		t.Fatal("started replay followed new route")
	}
}

func TestTaskDispatchBoundRouteRecheckRollsBackPostAuthorityDrift(t *testing.T) {
	f, _, request, task := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	// Inject a change after the service's authority/preview check and before
	// Core freezes its new run. No unprotected production mutation path is added.
	trigger := `CREATE TRIGGER change_route_during_managed_insert AFTER INSERT ON dag_managed_workflows BEGIN INSERT INTO dag_project_execution_routes(tenant_id,project_id,version,pool_id,grant_id,grant_version)VALUES('` + f.user.TenantID + `','` + f.project + `',1,'','',0); END`
	if _, err := f.s.DB.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	for _, table := range []string{"dag_runs", "dag_managed_workflows", "dag_events", "dag_project_execution_routes"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("bound route conflict did not roll back", table, count, err)
		}
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["version"] != task["version"] || current["run_id"] != nil {
		t.Fatal("post-authority drift bound business task", current)
	}
}

func TestTaskDispatchLegacyLocalPlanKeepsUnchangedDefaultOnly(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	if _, err := f.s.DB.Exec(`DELETE FROM suma_task_dispatch_route_bases WHERE plan_id=?`, plan["id"]); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
}

func TestTaskDispatchPoolGrantRevokedAfterPreviewDoesNotCreateRun(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	pool, grant := dispatchAuthorizedPool(t, f)
	p := dispatchPoolTool(t, f, pool)
	dispatchSetRoute(t, f, 0, &grant)
	request["tool_profile_id"], request["expected_tool_version"] = p.ID, p.Version
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	if _, err := f.s.CommandOwnedPoolGrant(context.Background(), f.e, f.user, f.cookie.Value, pool.ID, grant.ID, ExecutionPoolGrantCommand{Action: "revoke", ExpectedVersion: grant.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	var count int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count)
	if count != 0 {
		t.Fatal("revoked pool task left run", count)
	}
}

func TestReleasePoolRouteRejectsPreviewAndExecuteWithoutStrandedRun(t *testing.T) {
	f, candidate, env, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	_, err := f.s.PreviewReleasePlan(context.Background(), f.user, f.project, f.cookie.Value, map[string]any{"project_id": f.project, "candidate_id": candidate.ID, "environment_id": env.ID, "action": "deploy", "expected_environment_generation": 0})
	releaseWantStatus(t, err, 422)
	_, err = f.s.ExecuteReleasePlan(context.Background(), f.user, f.project, plan.ID, f.cookie.Value, releaseExecuteBody(f, plan), f.e)
	releaseWantStatus(t, err, 422)
	var runs int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE id=?`, plan.Content.ReservedExecution.RunID).Scan(&runs)
	if runs != 0 {
		t.Fatal("unsupported production pool left managed run", runs)
	}
}

// This deliberately has no accepted candidate or deployment approval. It
// verifies the explicit unsupported-route boundary before those later gates;
// it is not evidence that a production deployment was accepted or executed.
func TestReleasePoolUnsupportedRouteGateBeforePlanBasis(t *testing.T) {
	f, _ := releaseTestSetup(t)
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	t.Run("preview", func(t *testing.T) {
		_, err := f.s.PreviewReleasePlan(context.Background(), f.user, f.project, f.cookie.Value, map[string]any{"project_id": f.project, "candidate_id": newID(), "environment_id": newID(), "action": "deploy", "expected_environment_generation": 0})
		releaseWantStatus(t, err, 422)
	})
	plan := ReleasePlan{ID: newID(), ProjectID: f.project, Version: 1, Revision: 1, State: "pending_approval", SHA256: strings.Repeat("0", 64), Content: rc.ReleasePlanContent{ProviderID: "local-static-test", ReservedExecution: rc.ReservedExecution{RunID: newID(), NodeID: "deploy"}}}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_release_plans(id,tenant_id,project_id,version,data)VALUES(?,?,?,?,?)`, plan.ID, f.user.TenantID, f.project, 1, stringify(plan)); err != nil {
		t.Fatal(err)
	}
	trace := dispatchRouteSQLTrace(t, f)
	t.Run("execute", func(t *testing.T) {
		_, err := f.s.ExecuteReleasePlan(context.Background(), f.user, f.project, plan.ID, f.cookie.Value, releaseExecuteBody(f, plan), f.e)
		releaseWantStatus(t, err, 422)
	})
	trace.mu.Lock()
	ordered := false
	for _, transaction := range trace.transactions {
		firstPool, firstProject := -1, -1
		for i, statement := range transaction.Statements {
			upper := strings.ToUpper(strings.TrimSpace(statement))
			if strings.HasPrefix(upper, "UPDATE DAG_EXECUTION_POOL_GUARDS ") && firstPool < 0 {
				firstPool = i
			}
			if strings.HasPrefix(upper, "UPDATE DAG_SCHEDULER_PROJECTS ") && firstProject < 0 {
				firstProject = i
			}
		}
		if firstPool >= 0 {
			if firstProject < firstPool || firstProject < 0 {
				trace.mu.Unlock()
				t.Fatal("production gate acquired project before pool", firstPool, firstProject)
			}
			ordered = true
		}
	}
	trace.mu.Unlock()
	if !ordered {
		t.Fatal("actual unsupported execution did not traverse ordered execution guard")
	}
	var runs, events int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs)
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events`).Scan(&events)
	if runs != 0 || events != 0 {
		t.Fatal("unsupported route created execution facts", runs, events)
	}
}

func dispatchRouteSQLTrace(t *testing.T, f *fixture) *schedulingSQLTrace {
	t.Helper()
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	trace := &schedulingSQLTrace{}
	name := "execution_route_" + newID()
	sql.Register(name, &schedulingTraceDriver{trace: trace})
	db, err := sql.Open(name, path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.s.DB, f.e = db, engine
	f.handler = (&Server{Service: f.s, Engine: engine, WebDir: t.TempDir()}).Handler()
	trace.mu.Lock()
	trace.transactions = nil
	trace.mu.Unlock()
	return trace
}

func TestReleaseFixedLocalGuardDoesNotFollowNewDefaultPool(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "已固定本地执行", Nodes: []dag.Node{{ID: "work", Type: "task"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	trace := dispatchRouteSQLTrace(t, f)
	tx, err := f.s.DB.BeginTx(ctx, schedulingWriteOptions(f.s.Dialect))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.authorizeProjectTx(ctx, tx, f.user, f.project, true, true); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = f.s.guardReleaseExecutionRouteTx(ctx, tx, f.user.TenantID, f.project, run.ID); err != nil {
		tx.Rollback()
		t.Fatal("existing local fact adopted new pool", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	for _, transaction := range trace.transactions {
		for _, statement := range transaction.Statements {
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statement)), "UPDATE DAG_EXECUTION_POOL_GUARDS ") {
				t.Fatal("old local receipt acquired current default pool")
			}
		}
	}
}

func TestReleaseFixedLocalActualAssignmentDrainsAfterDefaultPoolChange(t *testing.T) {
	f, _, _, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	ctx := context.Background()
	request := releaseExecuteBody(f, plan)
	deployment, err := f.s.ExecuteReleasePlan(ctx, f.user, f.project, plan.ID, f.cookie.Value, request, f.e)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, map[string]any{"worker_id": "route-local-deployer", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := &runner.Client{BaseURL: server.URL, Token: registration["token"].(string), ProjectID: f.project}
	a, err := client.Claim(ctx, "route-local-deployer", []string{"external_workflow:local-static-test"})
	if err != nil || a == nil || a.ReleaseExecution == nil {
		t.Fatal("actual protected local assignment unavailable", err)
	}
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	replayed, err := f.s.ExecuteReleasePlan(ctx, f.user, f.project, plan.ID, f.cookie.Value, request, f.e)
	if err != nil || replayed.OperationID != deployment.OperationID || replayed.RunID != deployment.RunID {
		t.Fatal("existing local deployment receipt adopted new pool", err)
	}
	if cancel, err := client.Heartbeat(ctx, *a); err != nil || cancel {
		t.Fatal("original local worker could not drain", err)
	}
	// Execute the real adapter's preflight rejection. No provider is configured,
	// no authorization ticket is issued and no production effect is fabricated.
	result := runner.Execute(ctx, *a, runner.ExecConfig{Artifacts: client})
	if result.ExitCode == 0 || result.OutcomeUnknown || result.Error != "release provider v2 is not configured" {
		t.Fatal("unexpected adapter preflight result", result.ExitCode, result.OutcomeUnknown, result.Error)
	}
	result.OperationID = newID()
	if err = client.Complete(ctx, result); err != nil {
		t.Fatal("original local rejection receipt failed after default route change", err)
	}
	if err = client.Complete(ctx, result); err != nil {
		t.Fatal("original local result replay failed", err)
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, deployment.RunID)
	if err != nil || current.ExecutionPoolID != "" || current.State != "failed" || current.Nodes[0].Attempt != 1 {
		t.Fatal("local failure was rerouted or promoted to success", current.State, err)
	}
	var poolDispatches int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_dispatches WHERE assignment LIKE ?`, "%"+deployment.RunID+"%").Scan(&poolDispatches); err != nil || poolDispatches != 0 {
		t.Fatal("local drain created shared dispatch", poolDispatches, err)
	}
}
