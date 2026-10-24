package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type poolMachineFixture struct {
	f          *fixture
	pool       ExecutionPool
	credential ExecutionPoolCredentialResponse
	scopes     []dag.PoolCapabilityScope
	grants     []ExecutionPoolGrant
}

func TestPoolMachineToolInventoryKeepsProjectApprovalAndTTL(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	token := p.credential.Token
	runner := p.credential.Runner
	profile := toolTestProfile()
	request := PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}
	f.bearer("POST", "/pool-runner/tool-inventory", token, request, 200)
	wrong := request
	wrong.ProjectID = p.scopes[1].ProjectID
	wrong.OperationID = newID()
	f.bearer("POST", "/pool-runner/tool-inventory", token, wrong, 403)
	pr, err := f.s.CreateToolProfile(ctx, f.user, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "已授权机器的项目工具", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.user, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal("pool registration replaced or lost project tool approval", err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "固定工具范围协议", Nodes: []dag.Node{{ID: "agent", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "fixture protocol only", "tool_profile_id": pr.ID, "tool_profile_sha256": profile.ProfileSHA256}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, runner.WorkerID); err != nil {
		t.Fatal(err)
	}
	if reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200); reply["assignment"] != nil {
		t.Fatal("expired inventory consumed an attempt")
	}
	request.OperationID = newID()
	f.bearer("POST", "/pool-runner/tool-inventory", token, request, 200)
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	if reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200); reply["assignment"] != nil {
		t.Fatal("revoked human tool approval was replaced by machine consent")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='owner' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var a dag.Assignment
	json.Unmarshal([]byte(stringify(reply["assignment"])), &a)
	if a.RunID != run.ID || a.Attempt != 1 {
		t.Fatal("valid project tool did not become eligible without prior attempt consumption")
	}
	event := tc.Event{SchemaVersion: 1, CommandID: profile.CommandID, ProfileSHA256: profile.ProfileSHA256, Protocol: profile.Protocol, Kind: "session_started", SessionID: "fixture-session"}
	body := poolAssignmentBody(a)
	body["operation_id"] = newID()
	body["message"] = ""
	body["tool_event"] = event
	f.bearer("POST", "/pool-runner/progress", token, body, 200)
	event.ProfileSHA256 = toolTestProfile().ExecutableSHA256
	body["operation_id"] = newID()
	body["tool_event"] = event
	f.bearer("POST", "/pool-runner/progress", token, body, 400)
}

func newPoolMachineFixture(t *testing.T) *poolMachineFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	pool, err := f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, ExecutionPoolCreateRequest{Name: "独立共享机器协议", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	out := &poolMachineFixture{f: f, pool: pool}
	actors := []Principal{f.user}
	tokens := []string{f.cookie.Value}
	projects := []string{f.project}
	org, err := f.s.CreateOrganization(ctx, f.user, map[string]any{"name": "机器协议第二组织", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, token, err := f.s.SwitchOrganization(ctx, f.user, map[string]any{"organization_id": org["id"], "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.s.CreateProject(ctx, actor, map[string]any{"title": "机器协议第二项目", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actors = append(actors, actor)
	tokens = append(tokens, token)
	projects = append(projects, project.ID)
	for i, p := range actors {
		grant, err := f.s.RequestExecutionPoolGrant(ctx, p, tokens[i], ExecutionPoolGrantRequest{ProjectID: projects[i], PoolID: pool.ID, ProjectWeight: 1, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		grant, err = f.s.CommandOwnedPoolGrant(ctx, f.e, f.user, f.cookie.Value, pool.ID, grant.ID, ExecutionPoolGrantCommand{Action: "approve", ExpectedVersion: grant.Version, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		out.grants = append(out.grants, grant)
		tx, err := f.s.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.authorizeProjectTx(ctx, tx, p, projects[i], true, true); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if _, err = f.e.WriteProjectExecutionRouteTx(ctx, tx, p.TenantID, projects[i], 0, dag.ProjectExecutionRouteInput{PoolID: pool.ID, GrantID: grant.ID, GrantVersion: grant.Version}); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		workflow, err := f.e.Publish(ctx, p.TenantID, projects[i], dag.Workflow{Name: "实际范围协议夹具", Nodes: []dag.Node{{ID: "inspect", Type: "command", Command: []string{"inspect"}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.e.CreateRun(ctx, p.TenantID, projects[i], workflow.ID, newID()); err != nil {
			t.Fatal(err)
		}
		out.scopes = append(out.scopes, dag.PoolCapabilityScope{TenantID: p.TenantID, ProjectID: projects[i], Capabilities: []string{"command:inspect"}})
	}
	out.credential, err = f.s.RegisterExecutionPoolRunner(ctx, f.user, f.cookie.Value, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "机器HTTP协议", MaxSlots: 2, TTLSeconds: 3600, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func poolAssignmentBody(a dag.Assignment) map[string]any {
	return map[string]any{"runner_id": a.PoolRunnerID, "tenant_id": a.TenantID, "project_id": a.ProjectID, "assignment": a}
}

func TestPoolMachineHTTPIndependentIdentityAndFrozenScope(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	id := p.credential.Runner.ID
	token := p.credential.Token
	legacy := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	f.bearer("GET", "/pool-runner/session?runner_id="+id, legacy, nil, 403)
	f.bearer("GET", "/pool-runner/session?runner_id="+id, token, nil, 200)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pool-runner/session?runner_id="+id, nil)
	req.AddCookie(f.cookie)
	res := httptest.NewRecorder()
	f.handler.ServeHTTP(res, req)
	if res.Code != 401 {
		t.Fatal("browser cookie became machine credential", res.Code)
	}
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: id, WorkerID: p.credential.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: p.scopes}
	originalOperation := claim.OperationID
	decode := func(reply map[string]any) dag.Assignment {
		t.Helper()
		var a dag.Assignment
		if err := json.Unmarshal([]byte(stringify(reply["assignment"])), &a); err != nil || a.RunID == "" {
			t.Fatal("missing actual pool assignment", err)
		}
		return a
	}
	a := decode(f.bearer("POST", "/pool-runner/claim", token, claim, 200))
	again := decode(f.bearer("POST", "/pool-runner/claim", token, claim, 200))
	if a.DispatchID != again.DispatchID || a.Fence != again.Fence || a.PoolGrantID == "" {
		t.Fatal("claim replay created another attempt or dropped grant")
	}
	claim.SlotID = "1"
	claim.OperationID = newID()
	b := decode(f.bearer("POST", "/pool-runner/claim", token, claim, 200))
	if a.ProjectID == b.ProjectID || a.TenantID == b.TenantID {
		t.Fatal("two project/organization scopes were not independently dispatched")
	}
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(a), 200)
	forged := a
	forged.PoolGrantID = b.PoolGrantID
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(forged), 409)
	wrong := poolAssignmentBody(a)
	wrong["project_id"] = b.ProjectID
	f.bearer("POST", "/pool-runner/heartbeat", token, wrong, 400)
	for _, assignment := range []dag.Assignment{a, b} {
		body := poolAssignmentBody(assignment)
		body["result"] = dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID()}
		f.bearer("POST", "/pool-runner/results", token, body, 200)
		run, err := f.e.GetRun(ctx, assignment.TenantID, assignment.ProjectID, assignment.RunID)
		if err != nil || run.State != "succeeded" {
			t.Fatal("scoped protocol result did not persist", err)
		}
	}
	closed := decode(f.bearer("POST", "/pool-runner/claim", token, dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: id, WorkerID: p.credential.Runner.WorkerID, SlotID: "0", OperationID: originalOperation, Scopes: p.scopes}, 200))
	if closed.DispatchID != a.DispatchID || closed.DispatchState != "closed" {
		t.Fatal("completed claim became executable again")
	}
}

func TestPoolMachineFailureProjectsOneIssueInFrozenProject(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	req := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: p.credential.Runner.ID, WorkerID: p.credential.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: p.scopes}
	reply := f.bearer("POST", "/pool-runner/claim", p.credential.Token, req, 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID == "" {
		t.Fatal("missing pool assignment")
	}
	body := poolAssignmentBody(a)
	body["result"] = dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, Error: "explicit local protocol fixture failure", OperationID: newID()}
	f.bearer("POST", "/pool-runner/results", p.credential.Token, body, 200)
	f.bearer("POST", "/pool-runner/results", p.credential.Token, body, 200)
	var count, foreign int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issues WHERE tenant_id=? AND project_id=?`, a.TenantID, a.ProjectID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issues WHERE tenant_id<>? OR project_id<>?`, a.TenantID, a.ProjectID).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	if count != 1 || foreign != 0 {
		t.Fatalf("failure projection lost frozen scope or deduplication: scoped=%d foreign=%d", count, foreign)
	}
	var raw string
	if err := f.s.DB.QueryRow(`SELECT data FROM suma_issues WHERE tenant_id=? AND project_id=?`, a.TenantID, a.ProjectID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if json.Unmarshal([]byte(raw), &data) != nil || data["run_id"] != a.RunID {
		t.Fatal("issue lost actual failed execution origin")
	}
}
