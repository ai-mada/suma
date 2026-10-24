package platform

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func taskDispatchFixture(t *testing.T) (*fixture, string, map[string]any, map[string]any) {
	t.Helper()
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	_, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.user, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, _, _ := taskDispatchBuildFixture()
	w.Version = 1
	published, err := f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if err != nil {
		t.Fatal(err)
	}
	req := f.call("POST", "/requirements", f.body(map[string]any{"title": "冻结的订单需求", "description": "本轮租户隔离", "acceptance_criteria": []string{"明确权限依据"}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "实际派发任务", "description": "实现成员权限校验", "requirement_id": req["id"], "acceptance_criteria": []string{"独立测试不得跳过"}}), 201)
	request := f.body(map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "workflow_id": published.ID, "workflow_version": published.Version, "agent_node_id": "develop", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version})
	return f, token, request, task
}

func TestTaskDispatchMaximumOperationRecoversOneRunAcrossConnections(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	var seq int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	e2, err := dag.New(db2, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	s2 := &Service{DB: db2, Dialect: "sqlite"}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: strings.Repeat("x", 128)}
	results := make(chan dag.Run, 2)
	failures := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			service, engine := f.s, f.e
			if index == 1 {
				service, engine = s2, e2
			}
			run, e := service.StartTaskDispatchPlan(context.Background(), engine, f.user, f.project, plan["id"].(string), start)
			if e != nil {
				failures <- e
				return
			}
			results <- run
		}(i)
	}
	group.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	count := 0
	for r := range results {
		count++
		if r.ID != plan["run_id"] {
			t.Fatal("concurrent start changed identity")
		}
	}
	if count != 2 {
		t.Fatal("did not recover both receipts")
	}
	var runs int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil || runs != 1 {
		t.Fatal("multiple effects", runs, err)
	}
}

func TestTaskDispatchDirectAgentAssociationIsRejectedAtomically(t *testing.T) {
	f, _, request, task := taskDispatchFixture(t)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": request["workflow_id"], "workflow_version": request["workflow_version"], "task_id": task["id"], "expected_version": task["version"]}), 400)
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("rejected direct association left a run", count, err)
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["version"] != task["version"] || current["run_id"] != nil {
		t.Fatal("rejected direct association changed task", current)
	}
}

func TestTaskDispatchRevokedCurrentAuthorityRejectsCachedStart(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	start := f.body(map[string]any{"expected_version": plan["version"]})
	path := "/task-dispatch-plans/" + plan["id"].(string) + "/start"
	f.call("POST", path, start, 201)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call("POST", path, start, 403)
}

func TestTaskDispatchPreviewStartAtomicBindingAndReceiptRecovery(t *testing.T) {
	f, token, request, task := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	if plan["state"] != "ready" || plan["valid"] != true || plan["task_version"] != task["version"] {
		t.Fatal(plan)
	}
	snapshot := plan["context"].(map[string]any)
	if snapshot["task"].(map[string]any)["description"] != "实现成员权限校验" || snapshot["requirement"].(map[string]any)["title"] != "冻结的订单需求" {
		t.Fatal("actual business context missing", snapshot)
	}
	replay := f.call("POST", "/task-dispatch-plans", request, 201)
	if replay["id"] != plan["id"] {
		t.Fatal("preview duplicated")
	}
	start := f.body(map[string]any{"expected_version": plan["version"]})
	path := "/task-dispatch-plans/" + plan["id"].(string) + "/start"
	run := f.call("POST", path, start, 201)
	if run["id"] != plan["run_id"] {
		t.Fatal("reserved run changed")
	}
	bound := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	if bound["run_id"] != run["id"] || bound["version"].(float64) != task["version"].(float64)+1 {
		t.Fatal("task not atomically bound", bound)
	}
	pr, err := f.s.GetToolProfile(context.Background(), f.user, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandToolProfile(context.Background(), f.user, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "pause", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	// Recover the original fact, without reapplying the now-stale plan basis.
	if recovered := f.call("POST", path, start, 201); recovered["id"] != run["id"] {
		t.Fatal("start replay created another run")
	}
	f.call("POST", path, f.body(map[string]any{"expected_version": plan["version"]}), 409)
	changed := map[string]any{}
	for k, v := range start {
		changed[k] = v
	}
	changed["expected_version"] = 2
	f.call("POST", path, changed, 409)
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate run", count, err)
	}
	f.bearer("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, token, nil, 403)
	read := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if read["state"] != "started" || read["valid"] != false {
		t.Fatal(read)
	}
}

func TestTaskDispatchChangedBasisExpiryAndMissingQualityDoNotStart(t *testing.T) {
	for _, kind := range []string{"task", "requirement", "profile", "inventory_expired", "plan_expired"} {
		t.Run(kind, func(t *testing.T) {
			f, _, request, task := taskDispatchFixture(t)
			plan := f.call("POST", "/task-dispatch-plans", request, 201)
			var err error
			switch kind {
			case "task":
				_, err = f.s.DB.Exec(`UPDATE suma_tasks SET version=version+1,description='changed' WHERE id=?`, task["id"])
			case "requirement":
				_, err = f.s.DB.Exec(`UPDATE suma_requirements SET version=version+1 WHERE id=?`, task["requirement_id"])
			case "profile":
				_, err = f.s.DB.Exec(`UPDATE suma_tool_profiles SET state='paused',version=version+1 WHERE id=?`, request["tool_profile_id"])
			case "inventory_expired":
				_, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z'`)
			case "plan_expired":
				_, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET expires_at='2000-01-01T00:00:00Z',payload=replace(payload,?,?) WHERE id=?`, plan["expires_at"], "2000-01-01T00:00:00Z", plan["id"])
			}
			if err != nil {
				t.Fatal(err)
			}
			f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
			current := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
			if current["valid"] != false {
				t.Fatal("stale preview still marked dispatchable", current)
			}
			var count int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 0 {
				t.Fatal("invalid preview produced a run", count, err)
			}
		})
	}
}

func TestTaskDispatchManagedRunHasNormalLifecycleButNoDeploymentAuthority(t *testing.T) {
	f, token, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "tool-worker", "capabilities": []string{"command:native-agent"}}), 200)
	if claimed["assignment"] == nil {
		t.Fatal("pure execution worker blocked as if deployment worker")
	}
	a := claimed["assignment"].(map[string]any)
	goal := a["config"].(map[string]any)["goal"].(string)
	if !strings.Contains(goal, "实际派发任务") || a["command"].([]any)[0] != "native-agent" {
		t.Fatal("task was linked but not sent as fixed input")
	}
	current := f.call("GET", "/runs/"+run["id"].(string)+"?project_id="+f.project, nil, 200)
	paused := f.call("POST", "/runs/"+run["id"].(string)+"/commands", f.body(map[string]any{"action": "pause", "expected_version": current["version"], "payload": map[string]any{}}), 200)
	if paused["state"] != "paused" {
		t.Fatal("task managed run lost normal pause lifecycle")
	}
}
