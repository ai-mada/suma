package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func codexWorkspaceObservationFixture(t *testing.T) (*fixture, Actor, woc.Scope, ToolProfile, cc.Catalog) {
	t.Helper()
	f, token, request, profile, catalog := codexExecutionFixture(t)
	ctx := context.Background()
	machine := toolActorForTest(t, f, token)
	reportCodexWorkspaceObservation(t, f, machine, profile, catalog, woc.Protocol)
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	if err = json.Unmarshal([]byte(stringify(claimed["assignment"])), &a); err != nil || a.RunID != run.ID || a.Config["codex_execution"] == nil {
		t.Fatal("missing current bound Codex attempt", claimed, err)
	}
	return f, machine, woc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: profile.ID, ProfileSHA256: profile.Profile.ProfileSHA256}, profile, catalog
}

func reportCodexWorkspaceObservation(t *testing.T, f *fixture, machine Actor, profile ToolProfile, catalog cc.Catalog, protocol string) {
	t.Helper()
	_, err := f.s.ReportToolInventory(context.Background(), machine, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true, CodexCatalog: &catalog, WorkspaceObservationProtocol: protocol}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCodexWorkspaceObservationReadProtocolAndWriteBoundary(t *testing.T) {
	f, machine, scope, _, _ := codexWorkspaceObservationFixture(t)
	ctx := context.Background()
	before, err := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 1 || targets.Items[0].WorkspaceObservationScope != scope || targets.Items[0].CanWrite {
		t.Fatal("current Codex read target missing or writable", targets, err)
	}
	for _, operation := range []string{"list", "read"} {
		args := woc.Arguments{}
		result := &woc.Result{List: &woc.List{Entries: []woc.Entry{{Path: "source.txt", Kind: "file"}}, Complete: true}}
		if operation == "read" {
			args = woc.Arguments{Path: "source.txt", FullFile: true}
			result = &woc.Result{Read: &woc.Read{Path: "source.txt", Content: "observed\n", SHA256: digest("observed\n"), Bytes: 9, StartLine: 1, Complete: true}}
		}
		request := woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: operation, Arguments: args, IdempotencyKey: newID()}
		created, e := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, request)
		if e != nil {
			t.Fatal(operation, e)
		}
		if replay, e := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, request); e != nil || replay.ID != created.ID {
			t.Fatal("Codex observation create receipt changed", replay, e)
		}
		claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("e", 64), OperationID: newID()}
		lease, e := f.s.ClaimWorkspaceObservation(ctx, machine, f.project, claim)
		if e != nil || lease == nil || lease.Observation.ID != created.ID || lease.LeaseFence != 1 {
			t.Fatal(operation, lease, e)
		}
		callback := woc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, RequestID: created.ID, Scope: scope, LeaseFence: lease.LeaseFence, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: result, ResultSHA256: woc.ResultDigest(result, ""), OperationID: newID()}
		if operation == "read" {
			if _, e = f.s.DB.Exec(`UPDATE suma_workspace_observations SET lease_until=0 WHERE id=?`, created.ID); e != nil {
				t.Fatal(e)
			}
			claim.OperationID = newID()
			next, e := f.s.ClaimWorkspaceObservation(ctx, machine, f.project, claim)
			if e != nil || next == nil || next.LeaseFence != 2 {
				t.Fatal("Codex read lease was not fenced", next, e)
			}
			_, e = f.s.CompleteWorkspaceObservation(ctx, machine, f.project, callback)
			expertStatus(t, e, 409)
			callback.LeaseFence, callback.OperationID = next.LeaseFence, newID()
		}
		ack, e := f.s.CompleteWorkspaceObservation(ctx, machine, f.project, callback)
		if e != nil || !ack.Acknowledged {
			t.Fatal(operation, ack, e)
		}
		if replay, e := f.s.CompleteWorkspaceObservation(ctx, machine, f.project, callback); e != nil || replay != ack {
			t.Fatal("Codex exact result ACK changed", replay, e)
		}
		current, e := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
		if e != nil || current.State != "completed" || stringify(current.Result) != stringify(result) {
			t.Fatal(operation, current, e)
		}
		metadata := f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=true", nil, 200)
		if metadata["state"] != "completed" || metadata["result"] != nil {
			t.Fatal("Codex metadata disclosed source", metadata)
		}
		if operation == "read" {
			originalCookie := f.cookie
			f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
			f.call("GET", "/workspace/observations/"+created.ID+"?project_id="+f.project+"&metadata=true", nil, 403)
			f.cookie = originalCookie
		}
		if _, e = f.s.DB.Exec(`UPDATE suma_workspace_observations SET expires_at=0 WHERE id=?`, created.ID); e != nil {
			t.Fatal(e)
		}
		expired, e := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
		if e != nil || expired.State != "expired" || expired.Result != nil {
			t.Fatal("expired Codex observation disclosed source", expired, e)
		}
	}
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, workspaceWriteRequest(f, scope))
	expertStatus(t, err, 409)
	_, err = f.s.ClaimWorkspaceWrite(ctx, machine, f.project, wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("e", 64), OperationID: newID()})
	expertStatus(t, err, 409)
	after, err := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
	if err != nil || after.Version != before.Version || after.State != before.State || stringify(after.Nodes) != stringify(before.Nodes) {
		t.Fatal("Codex observation or denied write changed the DAG", after, err)
	}
}

func TestCodexWorkspaceObservationCurrentInventoryAndAuthority(t *testing.T) {
	for _, scenario := range []string{"withdrawn", "stale-inventory", "old-attempt-fence", "caller-credential", "machine-credential"} {
		t.Run(scenario, func(t *testing.T) {
			f, machine, scope, profile, catalog := codexWorkspaceObservationFixture(t)
			ctx := context.Background()
			request := woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "read", Arguments: woc.Arguments{Path: "source.txt"}, IdempotencyKey: newID()}
			created, err := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("e", 64), OperationID: newID()}
			lease, err := f.s.ClaimWorkspaceObservation(ctx, machine, f.project, claim)
			if err != nil || lease == nil {
				t.Fatal(lease, err)
			}
			switch scenario {
			case "withdrawn":
				reportCodexWorkspaceObservation(t, f, machine, profile, catalog, "")
			case "stale-inventory":
				_, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE worker_id=?`, scope.WorkerID)
			case "old-attempt-fence":
				_, err = f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, scope.RunID, scope.NodeID)
			case "caller-credential":
				_, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, f.actor.credential.digest)
			case "machine-credential":
				_, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, machine.credential.digest)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := &woc.Result{Read: &woc.Read{Path: "source.txt", Content: "source\n", SHA256: digest("source\n"), Bytes: 7, StartLine: 1, Complete: true}}
			callback := woc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, RequestID: created.ID, Scope: scope, LeaseFence: lease.LeaseFence, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: result, ResultSHA256: woc.ResultDigest(result, ""), OperationID: newID()}
			if _, err = f.s.CompleteWorkspaceObservation(ctx, machine, f.project, callback); err == nil {
				t.Fatal("stale Codex authority accepted a source result")
			}
			if _, err = f.s.ClaimWorkspaceObservation(ctx, machine, f.project, claim); err == nil {
				t.Fatal("stale Codex authority recovered a lease")
			}
			if scenario == "machine-credential" {
				return
			}
			request.IdempotencyKey = newID()
			if _, err = f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, request); err == nil {
				t.Fatal("stale Codex authority created a new observation")
			}
			current, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
			if scenario == "caller-credential" {
				if err == nil {
					t.Fatal("expired original caller read an observation")
				}
				return
			}
			if err != nil || current.State != "cancelled" || current.Result != nil {
				t.Fatal("stale Codex target did not close source access", current, err)
			}
			targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
			if scenario == "old-attempt-fence" {
				if err != nil || len(targets.Items) != 1 || targets.Items[0].Fence != scope.Fence+1 {
					t.Fatal("discovery did not retain only the current Codex fence", targets, err)
				}
			} else if err != nil || len(targets.Items) != 0 {
				t.Fatal("stale Codex target remained discoverable", targets, err)
			}
		})
	}
}

func TestCodexWorkspaceObservationPoolExactAssignmentAndCurrentGrant(t *testing.T) {
	p := newPoolMachineFixture(t)
	f, ctx := p.f, context.Background()
	machine := p.credential.Runner
	profile := tc.Profile{CommandID: "pool-codex", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "owned Codex workspace protocol fixture"}
	catalog, err := cc.NewCatalog(strings.Repeat("d", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"low"}, DefaultReasoningEffort: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportPoolToolInventory(ctx, p.credential.Token, PoolToolInventoryRequest{RunnerID: machine.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true, CodexCatalog: &catalog, WorkspaceObservationProtocol: woc.Protocol}}})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.WorkerID, Name: "Pool Codex read observation", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	approved, err = f.s.CommandToolProfile(ctx, f.actor, f.project, approved.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: approved.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Pool current Codex workspace", Nodes: []dag.Node{{ID: "code", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "observe bounded source only", "tool_profile_id": approved.ID, "tool_profile_sha256": profile.ProfileSHA256, "codex_execution": binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	f.e.AttemptAccounting = f.s
	a := candidatePoolClaim(t, p, p.credential, profile.CommandID)
	scope := woc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: approved.ID, ProfileSHA256: profile.ProfileSHA256}
	targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 1 || targets.Items[0].PoolID != p.pool.ID || targets.Items[0].CanWrite {
		t.Fatal(targets, err)
	}
	created, err := f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "read", Arguments: woc.Arguments{Path: "source.txt", FullFile: true}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claim := woc.ClaimRequest{ProjectID: f.project, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("e", 64), OperationID: newID()}
	ordinary := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	_, err = f.s.ClaimWorkspaceObservation(ctx, toolActorForTest(t, f, ordinary), f.project, claim)
	expertStatus(t, err, 403)
	poolClaim := PoolWorkspaceObservationClaimRequest{ClaimRequest: claim, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a}
	wrong := poolClaim
	wrong.Assignment.DispatchID = newID()
	_, err = f.s.ClaimPoolWorkspaceObservation(ctx, p.credential.Token, wrong)
	expertStatus(t, err, 403)
	lease, err := f.s.ClaimPoolWorkspaceObservation(ctx, p.credential.Token, poolClaim)
	if err != nil || lease == nil || lease.Observation.ID != created.ID {
		t.Fatal(lease, err)
	}
	result := &woc.Result{Read: &woc.Read{Path: "source.txt", Content: "source\n", SHA256: digest("source\n"), Bytes: 7, StartLine: 1, Complete: true}}
	callback := woc.ResultRequest{ProjectID: f.project, WorkerID: a.WorkerID, RequestID: created.ID, Scope: scope, LeaseFence: lease.LeaseFence, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: result, ResultSHA256: woc.ResultDigest(result, ""), OperationID: newID()}
	poolResult := PoolWorkspaceObservationResultRequest{ResultRequest: callback, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a}
	if ack, e := f.s.CompletePoolWorkspaceObservation(ctx, p.credential.Token, poolResult); e != nil || !ack.Acknowledged {
		t.Fatal(ack, e)
	}
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, workspaceWriteRequest(f, scope))
	expertStatus(t, err, 409)
	_, err = f.s.ClaimPoolWorkspaceWrite(ctx, p.credential.Token, PoolWorkspaceWriteClaimRequest{ClaimRequest: wcc.ClaimRequest{ProjectID: f.project, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: claim.WorkspacePolicySHA256, OperationID: newID()}, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a})
	expertStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET status='revoked' WHERE id=?`, a.PoolCredentialID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ClaimPoolWorkspaceObservation(ctx, p.credential.Token, poolClaim)
	expertStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET status='active' WHERE id=?`, a.PoolCredentialID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CompletePoolWorkspaceObservation(ctx, p.credential.Token, poolResult)
	expertStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='approved',version=version+1 WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ClaimPoolWorkspaceObservation(ctx, p.credential.Token, poolClaim)
	expertStatus(t, err, 403)
	closed, err := f.s.GetWorkspaceObservation(ctx, f.actor, f.project, created.ID)
	if err != nil || closed.State != "cancelled" || closed.Result != nil {
		t.Fatal("changed Pool authority disclosed Codex source", closed, err)
	}
}
