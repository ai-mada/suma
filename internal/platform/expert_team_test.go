package platform

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

type expertFixture struct {
	f             *fixture
	runnerToken   string
	team          string
	resource      Record
	role          ExpertTeamRoleRequest
	configuration ExpertTeamConfiguration
	tasks         []Record
}

func newExpertFixture(t *testing.T, pool bool) *expertFixture {
	t.Helper()
	f, runnerToken, request, _ := taskDispatchFixture(t)
	ctx := context.Background()
	team, err := f.s.SaveTeam(ctx, f.actor, "", map[string]any{"name": "Actual expert team", "status": "active", "expected_version": float64(0), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Development expert", "kind": "agent", "status": "active"}))
	if err != nil {
		t.Fatal(err)
	}
	teamID := team["id"].(string)
	if _, err = f.s.SaveTeamMember(ctx, f.actor, teamID, map[string]any{"resource_kind": "agent", "resource_id": resource.ID, "project_id": f.project, "role": "member", "expected_version": float64(0), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	var role ExpertTeamRoleRequest
	encoded, _ := json.Marshal(request)
	if err = json.Unmarshal(encoded, &role); err != nil {
		t.Fatal(err)
	}
	role.Key, role.Title, role.ResourceID, role.ExpectedResourceVersion, role.Knowledge = "developer", "Development", resource.ID, resource.Version, []ProjectKnowledgeReference{}
	if pool {
		p, g := dispatchAuthorizedPool(t, f)
		dispatchSetRoute(t, f, 0, &g)
		profile := dispatchPoolTool(t, f, p)
		role.ToolProfileID, role.ExpectedToolVersion = profile.ID, profile.Version
	}
	h := &expertFixture{f: f, runnerToken: runnerToken, team: teamID, resource: resource, role: role}
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamSaveRequest{ProjectID: f.project, Roles: []ExpertTeamRoleRequest{role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Review fixed resource, tool, workflow and route", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"Independent task A", "Independent task B"} {
		task, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": title, "acceptance_criteria": []any{"Formal tests and independent human acceptance"}}))
		if e != nil {
			t.Fatal(e)
		}
		h.tasks = append(h.tasks, task)
	}
	return h
}

func (h *expertFixture) request() ExpertTeamPlanCreateRequest {
	r := ExpertTeamPlanCreateRequest{ProjectID: h.f.project, TeamID: h.team, ExpectedConfigurationVersion: h.configuration.Version, Items: []ExpertTeamPlanItemRequest{}, IdempotencyKey: newID()}
	for _, task := range h.tasks {
		r.Items = append(r.Items, ExpertTeamPlanItemRequest{TaskID: task.ID, ExpectedTaskVersion: task.Version, RoleKey: h.role.Key})
	}
	return r
}
func (h *expertFixture) preview(t *testing.T) ExpertTeamPlan {
	t.Helper()
	p, e := h.f.s.CreateExpertTeamPlan(context.Background(), h.f.e, h.f.actor, h.f.project, h.request())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func (h *expertFixture) start(p ExpertTeamPlan) ExpertTeamPlanStartRequest {
	return ExpertTeamPlanStartRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, AcknowledgeDispatch: true, IdempotencyKey: newID()}
}
func expertStatus(t *testing.T, err error, status int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("expected HTTP %d, got %v", status, err)
	}
}
func expertCount(t *testing.T, f *fixture, table string) int {
	t.Helper()
	var n int
	if e := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestExpertTeamConfigurationRevisionsRetirementAndAPIPrivileges(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	first, err := f.s.GetExpertTeam(ctx, f.actor, f.project, h.team, 1)
	if err != nil || first.State != "draft" || first.CanEdit || first.CanPublish || first.CanRetire {
		t.Fatal(first, err)
	}
	if h.configuration.State != "published" || !h.configuration.Valid || h.configuration.CanPublish {
		t.Fatal(h.configuration)
	}
	issued := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "expert-api"}), 201)
	api := toolActorForTest(t, f, issued["token"].(string))
	if _, err = f.s.CommandExpertTeam(ctx, f.e, api, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: 2, Action: "retire", Reason: "API cannot approve", IdempotencyKey: newID()}); err == nil {
		t.Fatal("API acquired formal approval")
	}
	save := ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: 2, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()}
	draft, err := f.s.SaveExpertTeam(ctx, f.e, api, f.project, h.team, save)
	if err != nil || draft.Version != 3 || draft.State != "draft" || draft.CanPublish {
		t.Fatal(draft, err)
	}
	retired, err := f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: 3, Action: "retire", Reason: "Retire the execution template", IdempotencyKey: newID()})
	if err != nil || retired.State != "retired" || retired.CanEdit {
		t.Fatal(retired, err)
	}
	replayed, err := f.s.SaveExpertTeam(ctx, f.e, api, f.project, h.team, save)
	if err != nil || replayed.Version != 3 || replayed.CanEdit || replayed.CanPublish || replayed.Valid {
		t.Fatal("old receipt regained eligibility", replayed, err)
	}
	save.ExpectedVersion = 4
	save.IdempotencyKey = newID()
	_, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, save)
	expertStatus(t, err, 409)
	list, err := f.s.ListExpertTeams(ctx, api, f.project, 20, "")
	if err != nil || list.Total != 1 || len(list.Items) != 1 || list.Items[0].State != "retired" {
		t.Fatal(list, err)
	}
}

func TestExpertTeamPreviewAtomicRollbackAndAcceptedDependencyGate(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	r := h.request()
	r.Items[1].ExpectedTaskVersion = 99
	_, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, r)
	expertStatus(t, err, 409)
	for _, table := range []string{"suma_task_dispatch_plans", "suma_expert_team_plans", "suma_expert_team_plan_items", "dag_runs"} {
		if expertCount(t, f, table) != 0 {
			t.Fatal("partial preview persisted", table)
		}
	}
	dependent, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": 1, "depends_on": []any{h.tasks[0].ID}}))
	if err != nil {
		t.Fatal(err)
	}
	h.tasks[1] = dependent
	dependentPlan, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, h.request())
	if err != nil || dependentPlan.Items[1].State != "waiting_dependencies" {
		t.Fatal(dependentPlan, err)
	}
	if expertCount(t, f, "suma_task_dispatch_plans") != 0 {
		t.Fatal("waiting dependency created a premature dispatch plan")
	}
	r = h.request()
	r.Items[1] = r.Items[0]
	_, err = f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, r)
	expertStatus(t, err, 400)
}

func TestExpertTeamStartIndependentPrivateAndPoolRunsReplay(t *testing.T) {
	for _, mode := range []string{"private", "pool"} {
		t.Run(mode, func(t *testing.T) {
			h := newExpertFixture(t, mode == "pool")
			f, ctx := h.f, context.Background()
			request := h.request()
			plan, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			repeat, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil || repeat.ID != plan.ID || len(plan.Items) != 2 {
				t.Fatal(repeat, err)
			}
			for _, task := range h.tasks {
				current, e := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
				if e != nil || current.RunID != "" || current.Version != task.Version {
					t.Fatal(current, e)
				}
			}
			start := h.start(plan)
			started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
			if err != nil || replay.ID != started.ID || replay.Version != 2 || expertCount(t, f, "dag_runs") != 2 {
				t.Fatal(replay, err)
			}
			for i, item := range started.Items {
				if item.Plan.RunID == "" || plan.Items[i].Plan.RunID != "" || item.RunState == "" {
					t.Fatal("actual run identity was missing or exposed prematurely", item)
				}
				var root string
				if e := f.s.DB.QueryRow(`SELECT root_run_id FROM dag_run_families WHERE run_id=?`, item.Plan.RunID).Scan(&root); e != nil || root != item.Plan.RunID {
					t.Fatal("roots shared a family", root, e)
				}
				run, e := f.e.GetRun(ctx, f.user.TenantID, f.project, item.Plan.RunID)
				if e != nil || mode == "pool" && run.ExecutionPoolID == "" || mode == "private" && run.ExecutionPoolID != "" {
					t.Fatal(run, e)
				}
				task, e := f.s.Get(ctx, f.actor, f.project, "tasks", item.Plan.TaskID)
				if e != nil || task.Status != "in_progress" || task.RunID != run.ID || task.Version != h.tasks[i].Version+1 {
					t.Fatal(task, e)
				}
				if _, e = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": run.ID, "decision": "accepted", "reason": "No test evidence yet"}), f.e); e == nil {
					t.Fatal("dispatch fabricated acceptance")
				}
			}
		})
	}
}

func TestExpertTeamStartSourceAndConfigurationDriftRejectsWholeBatch(t *testing.T) {
	for _, mode := range []string{"task", "configuration", "membership", "bind-rollback"} {
		t.Run(mode, func(t *testing.T) {
			h := newExpertFixture(t, false)
			f, ctx := h.f, context.Background()
			plan := h.preview(t)
			switch mode {
			case "task":
				_, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": 1, "description": "Changed final task source"}))
				if err != nil {
					t.Fatal(err)
				}
			case "configuration":
				_, err := f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: 2, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
			case "membership":
				_, err := f.s.SaveTeamMember(ctx, f.actor, h.team, map[string]any{"resource_kind": "agent", "resource_id": h.resource.ID, "project_id": f.project, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()})
				if err != nil {
					t.Fatal(err)
				}
			case "bind-rollback":
				// Fault injection after the first actual run binding exercises the
				// platform transaction rather than faking a task or run state.
				if _, err := f.s.DB.Exec(`CREATE TRIGGER expert_test_bind_failure BEFORE UPDATE OF run_id ON suma_tasks WHEN NEW.title='Independent task B' BEGIN SELECT RAISE(ABORT,'fixture binding failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, h.start(plan))
			if mode == "bind-rollback" {
				if err == nil {
					t.Fatal("injected binding failure was ignored")
				}
			} else {
				expertStatus(t, err, 409)
			}
			if expertCount(t, f, "dag_runs") != 0 || expertCount(t, f, "dag_managed_workflows") != 0 {
				t.Fatal("source drift partially started work")
			}
			for _, task := range h.tasks {
				r, e := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
				if e != nil || r.RunID != "" {
					t.Fatal("task rebound", r, e)
				}
			}
			current, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID)
			if err != nil || mode != "bind-rollback" && (current.Valid || current.CanStart || len(current.Blockers) == 0) || mode == "bind-rollback" && (!current.Valid || !current.CanStart || current.Version != 1) {
				t.Fatal(current, err)
			}
		})
	}
}

func TestExpertTeamOwnedTaskPlanCannotStartDirectly(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	plan := h.preview(t)
	var err error
	plan, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, h.start(plan))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		_, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, item.Plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
		expertStatus(t, err, 409)
	}
	if expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("direct start escaped batch")
	}
	ack := h.start(plan)
	ack.AcknowledgeDispatch = false
	_, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, ack)
	expertStatus(t, err, 400)
}

func TestExpertTeamConcurrentStartAndRevokedReplay(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	plan := h.preview(t)
	start := h.start(plan)
	var seq int
	var name, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	other := &Service{DB: db, Dialect: "sqlite"}
	var group sync.WaitGroup
	failures := make(chan error, 2)
	results := make(chan ExpertTeamPlan, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			s, e := f.s, f.e
			if i == 1 {
				s, e = other, engine
			}
			out, err := s.StartExpertTeamPlan(ctx, e, f.actor, f.project, plan.ID, start)
			if err == nil && (out.Version != 2 || len(out.Items) != 2 || out.Items[0].RunID == "" || out.Items[0].DispatchPlanID == "") {
				err = errors.New("original runs changed")
			}
			failures <- err
			results <- out
		}(i)
	}
	group.Wait()
	close(failures)
	close(results)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first *ExpertTeamPlan
	for out := range results {
		if first == nil {
			copy := out
			first = &copy
			continue
		}
		for i, item := range out.Items {
			if item.RunID == "" || item.DispatchPlanID == "" || item.RunID != first.Items[i].RunID || item.DispatchPlanID != first.Items[i].DispatchPlanID || item.Plan.RunID != item.RunID || item.Plan.ID != item.DispatchPlanID {
				t.Fatal("concurrent replies changed original actual bindings", *first, out)
			}
		}
	}
	if expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("concurrent start duplicated runs")
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	_, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err == nil {
		t.Fatal("cached receipt bypassed revoked credential")
	}
}

func TestExpertTeamRoleIdentityAndScopeAreNotExecutionAuthority(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	for _, mode := range []string{"human", "wrong-node", "missing-membership", "wrong-version", "missing-workflow"} {
		t.Run(mode, func(t *testing.T) {
			role := h.role
			switch mode {
			case "human":
				human, e := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Real human", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}}))
				if e != nil {
					t.Fatal(e)
				}
				role.ResourceID = human.ID
			case "wrong-node":
				role.AgentNodeID = "test"
			case "missing-membership":
				r, e := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Unbound agent", "kind": "agent", "status": "active"}))
				if e != nil {
					t.Fatal(e)
				}
				role.ResourceID = r.ID
			case "wrong-version":
				role.ExpectedResourceVersion++
			case "missing-workflow":
				role.WorkflowVersion = 999
			}
			_, err := f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: 2, Roles: []ExpertTeamRoleRequest{role}, IdempotencyKey: newID()})
			if err == nil {
				t.Fatal("invalid role binding accepted")
			}
		})
	}
	if expertCount(t, f, "suma_expert_team_revisions") != 2 {
		t.Fatal("rejected roles persisted revisions")
	}
}
