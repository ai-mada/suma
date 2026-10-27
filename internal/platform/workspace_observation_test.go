package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func workspaceObservationFixture(t *testing.T) (*fixture, Actor, woc.Scope) {
	t.Helper()
	f, token, request, _ := taskDispatchFixture(t)
	ctx := context.Background()
	p, err := f.s.GetToolProfile(ctx, f.actor, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	p.Profile.WorkspaceToolsVersion = "attempt-workspace-v1"
	p.Profile.ProfileSHA256 = strings.Repeat("c", 64)
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_profiles SET profile=? WHERE id=?`, stringify(p.Profile), p.ID); err != nil {
		t.Fatal(err)
	}
	actor := toolActorForTest(t, f, token)
	if _, err = f.s.ReportToolInventory(ctx, actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a struct {
		RunID    string `json:"run_id"`
		NodeID   string `json:"node_id"`
		Attempt  int    `json:"attempt"`
		Fence    int64  `json:"fence"`
		WorkerID string `json:"worker_id"`
	}
	raw, _ := json.Marshal(claim["assignment"])
	if err = json.Unmarshal(raw, &a); err != nil || a.RunID == "" {
		t.Fatal(claim, err)
	}
	return f, actor, woc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: p.ID, ProfileSHA256: p.Profile.ProfileSHA256}
}

func TestWorkspaceObservationEmptyPollHasNoDurableReceipt(t *testing.T) {
	f, runner, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("d", 64), OperationID: newID()}
	before := dispatchOptionsCount(t, f, "suma_operations")
	for i := 0; i < 3; i++ {
		lease, err := f.s.ClaimWorkspaceObservation(ctx, runner, f.project, claim)
		if err != nil || lease != nil {
			t.Fatal(lease, err)
		}
	}
	if got := dispatchOptionsCount(t, f, "suma_operations"); got != before {
		t.Fatal("empty polling persisted effect-free receipts", before, got)
	}
	created, err := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "list", Arguments: woc.Arguments{}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.s.ClaimWorkspaceObservation(ctx, runner, f.project, claim)
	if err != nil || lease == nil || lease.Observation.ID != created.ID {
		t.Fatal("empty poll incorrectly froze a later lease", lease, err)
	}
	again, err := f.s.ClaimWorkspaceObservation(ctx, runner, f.project, claim)
	if err != nil || again == nil || again.Observation.ID != created.ID || again.LeaseFence != lease.LeaseFence {
		t.Fatal("nonempty lease lost exact recovery", again, err)
	}
	if got := dispatchOptionsCount(t, f, "suma_operations"); got != before+1 {
		t.Fatal("lease receipt was not recorded exactly once", before, got)
	}
}

func TestWorkspaceObservationLeaseFencingAndExpiry(t *testing.T) {
	f, runner, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	beforeRun, err := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
	if err != nil {
		t.Fatal(err)
	}
	targets := f.call("GET", "/workspace/targets?project_id="+f.project, nil, 200)
	if len(targets["items"].([]any)) != 1 {
		t.Fatal("exact running native target missing", targets)
	}
	f.call("GET", "/workspace/targets?project_id="+f.project+"&unknown=1", nil, 400)
	f.call("GET", "/workspace/targets?project_id="+f.project+"&project_id="+f.project, nil, 400)
	body := f.body(map[string]any{"scope": scope, "operation": "read", "arguments": woc.Arguments{Path: "source.txt"}})
	createdRaw := f.call("POST", "/workspace/observations", body, 201)
	var created woc.Observation
	if err := json.Unmarshal([]byte(stringify(createdRaw)), &created); err != nil {
		t.Fatal(err)
	}
	replayed := f.call("POST", "/workspace/observations", body, 201)
	if replayed["id"] != created.ID {
		t.Fatal("observation operation created another request")
	}
	f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+newID(), nil, 404)
	claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("d", 64), OperationID: newID()}
	lease, err := f.s.ClaimWorkspaceObservation(ctx, runner, f.project, claim)
	if err != nil || lease == nil || lease.LeaseFence != 1 {
		t.Fatal(lease, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_workspace_observations SET lease_until=0 WHERE id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	next, err := f.s.ClaimWorkspaceObservation(ctx, runner, f.project, claim)
	if err != nil || next == nil || next.LeaseFence != 2 {
		t.Fatal(next, err)
	}
	result := &woc.Result{Read: &woc.Read{Path: "source.txt", Content: "observed\n", SHA256: digest("observed\n"), Bytes: 9, StartLine: 1, Complete: true}}
	callback := woc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, RequestID: created.ID, Scope: scope, LeaseFence: 1, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: result, ResultSHA256: woc.ResultDigest(result, ""), OperationID: newID()}
	_, err = f.s.CompleteWorkspaceObservation(ctx, runner, f.project, callback)
	expertStatus(t, err, 409)
	callback.LeaseFence = 2
	ack, err := f.s.CompleteWorkspaceObservation(ctx, runner, f.project, callback)
	if err != nil || !ack.Acknowledged {
		t.Fatal(ack, err)
	}
	again, err := f.s.CompleteWorkspaceObservation(ctx, runner, f.project, callback)
	if err != nil || again != ack {
		t.Fatal("exact ACK replay changed", again, err)
	}
	current, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	if err != nil || current.Result == nil || current.Result.Read.Content != "observed\n" {
		t.Fatal(current, err)
	}
	metadata := f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=true", nil, 200)
	if metadata["state"] != "completed" || metadata["result"] != nil {
		t.Fatal("metadata reauthorization transmitted source", metadata)
	}
	f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=false", nil, 400)
	f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=true&metadata=true", nil, 400)
	originalCookie := f.cookie
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=true", nil, 403)
	f.cookie = originalCookie
	afterRun, err := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
	if err != nil || afterRun.Version != beforeRun.Version || afterRun.State != beforeRun.State || afterRun.Nodes[0].State != "running" || afterRun.Nodes[0].Attempt != scope.Attempt || afterRun.Nodes[0].Fence != scope.Fence {
		t.Fatal("read-only broker changed the execution", afterRun, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, scope.RunID, scope.NodeID); err != nil {
		t.Fatal(err)
	}
	closed, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	if err != nil || closed.State != "cancelled" || closed.Result != nil {
		t.Fatal("changed attempt disclosed old source", closed, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_workspace_observations SET expires_at=0 WHERE id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ExpireWorkspaceObservations(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	if err != nil || got.State != "expired" || got.Result != nil {
		t.Fatal(got, err)
	}
}

func TestWorkspaceObservationPoolRequiresExactCurrentGrantAndMachine(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	machine := p.credential.Runner
	profile := toolTestProfile()
	profile.WorkspaceToolsVersion = "attempt-workspace-v1"
	if _, err := f.s.ReportPoolToolInventory(ctx, p.credential.Token, PoolToolInventoryRequest{RunnerID: machine.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.WorkerID, Name: "Pool native observation", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Pool live native target", Nodes: []dag.Node{{ID: "code", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "observe bounded source only", "tool_profile_id": pr.ID, "tool_profile_sha256": profile.ProfileSHA256}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a := candidatePoolClaim(t, p, p.credential, profile.CommandID)
	scope := woc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: pr.ID, ProfileSHA256: profile.ProfileSHA256}
	created, err := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "list", Arguments: woc.Arguments{}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("e", 64), OperationID: newID()}
	ordinary := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	_, err = f.s.ClaimWorkspaceObservation(ctx, toolActorForTest(t, f, ordinary), f.project, claim)
	expertStatus(t, err, 403)
	claimed := f.bearer("POST", "/pool-runner/workspace-observations/claim", p.credential.Token, PoolWorkspaceObservationClaimRequest{ClaimRequest: claim, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a}, 200)
	var lease *woc.Lease
	if err = json.Unmarshal([]byte(stringify(claimed["lease"])), &lease); err != nil || lease == nil {
		t.Fatal(claimed, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	result := &woc.Result{List: &woc.List{Entries: []woc.Entry{{Path: "source.txt", Kind: "file"}}, Complete: true}}
	callback := woc.ResultRequest{ProjectID: f.project, WorkerID: a.WorkerID, RequestID: created.ID, Scope: scope, LeaseFence: lease.LeaseFence, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: result, ResultSHA256: woc.ResultDigest(result, ""), OperationID: newID()}
	_, err = f.s.CompletePoolWorkspaceObservation(ctx, p.credential.Token, PoolWorkspaceObservationResultRequest{ResultRequest: callback, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a})
	expertStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='approved',version=version+1 WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	_, err = f.s.ClaimPoolWorkspaceObservation(ctx, p.credential.Token, PoolWorkspaceObservationClaimRequest{ClaimRequest: claim, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a})
	expertStatus(t, err, 403)
	closed, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	if err != nil || closed.Result != nil || closed.State != "cancelled" {
		t.Fatal(closed, err)
	}
}
func TestWorkspaceObservationOriginalCallerCurrentAuthority(t *testing.T) {
	f, _, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	created, err := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "list", Arguments: woc.Arguments{}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	expertStatus(t, err, 403)
}
