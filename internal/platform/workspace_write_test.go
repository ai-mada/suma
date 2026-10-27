package platform

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func workspaceWriteRequest(f *fixture, scope wcc.Scope) wcc.CreateRequest {
	return wcc.CreateRequest{ProjectID: f.project, Scope: scope, WorkspacePolicySHA256: strings.Repeat("d", 64), Path: "source.txt", ExpectedSHA256: digest("before\n"), Content: "after\n", ContentSHA256: wcc.ContentDigest("after\n"), IdempotencyKey: newID()}
}
func TestWorkspaceWriteCurrentAuthorityAndImmutableCommand(t *testing.T) {
	f, machine, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	claim := wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: strings.Repeat("d", 64), OperationID: newID()}
	before := dispatchOptionsCount(t, f, "suma_operations")
	for i := 0; i < 3; i++ {
		if lease, e := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim); e != nil || lease != nil {
			t.Fatal(lease, e)
		}
	}
	if dispatchOptionsCount(t, f, "suma_operations") != before {
		t.Fatal("empty save poll persisted a receipt")
	}
	targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 1 || !targets.Items[0].CanWrite {
		t.Fatal("current writer was not eligible", targets, err)
	}
	req := workspaceWriteRequest(f, scope)
	command, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	if err != nil || replayed.ID != command.ID {
		t.Fatal(replayed, err)
	}
	changed := req
	changed.Content = "different\n"
	changed.ContentSHA256 = wcc.ContentDigest(changed.Content)
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, changed)
	expertStatus(t, err, 409)
	page := f.call("GET", "/workspace/writes?project_id="+f.project, nil, 200)
	if page["total"] != float64(1) || len(page["items"].([]any)) != 1 || strings.Contains(stringify(page), `"content":`) {
		t.Fatal("metadata list lost or disclosed source", page)
	}
	f.call("GET", "/workspace/writes?project_id="+f.project+"&project_id="+f.project, nil, 400)
	originalCookie := f.cookie
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	other := f.call("GET", "/workspace/writes?project_id="+f.project, nil, 200)
	if other["total"] != float64(0) || len(other["items"].([]any)) != 0 {
		t.Fatal("another credential saw original save metadata", other)
	}
	f.call("GET", "/workspace/writes/"+command.ID+"?project_id="+f.project, nil, 403)
	f.cookie = originalCookie
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'viewer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	if err != nil || recovered.ID != command.ID || recovered.ExecutionFence != 0 || strings.Contains(stringify(recovered), `"content":`) {
		t.Fatal("viewer could not recover the existing metadata receipt", recovered, err)
	}
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, changed)
	expertStatus(t, err, 409)
	req.IdempotencyKey = newID()
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	expertStatus(t, err, 403)
	if got, e := f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID); e != nil || got.ID != command.ID {
		t.Fatal("viewer lost read-only effect metadata", got, e)
	}
	targets, err = f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) > 0 && targets.Items[0].CanWrite {
		t.Fatal("viewer received new write eligibility", targets, err)
	}
}

func TestWorkspaceWriteOnePermitInspectionAndLateEffect(t *testing.T) {
	f, machine, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	request := workspaceWriteRequest(f, scope)
	command, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	claim := wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, OperationID: newID()}
	lease, err := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim)
	if err != nil || lease == nil || lease.Mode != "execute" || lease.Command.ExecutionFence != 1 || lease.Content == nil || *lease.Content != request.Content {
		t.Fatal(lease, err)
	}
	againLease, err := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim)
	if err != nil || againLease == nil || againLease.ClaimID != lease.ClaimID || againLease.LeaseUntil != lease.LeaseUntil || againLease.Content == nil || *againLease.Content != request.Content {
		t.Fatal("exact permit could not recover", againLease, err)
	}
	var persisted string
	subject := workspaceObservationSubject{Actor: machine, Tenant: f.user.TenantID, Project: f.project}
	if err = f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, workspaceWriteOperationKey(subject, "claim", claim.OperationID)).Scan(&persisted); err != nil || strings.Contains(persisted, `"content":`) {
		t.Fatal("claim receipt retained source text", err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_workspace_writes SET execute_before=1,next_expiry=1 WHERE id=?`, command.ID); err != nil {
		t.Fatal(err)
	}
	if expired, e := f.s.ExpireWorkspaceWrites(ctx); e != nil || expired != 1 {
		t.Fatal("permit timeout was not swept", expired, e)
	}
	current, err := f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID)
	if err != nil || current.State != "reconciling" {
		t.Fatal(current, err)
	}
	claim.OperationID = newID()
	if next, e := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim); e != nil || next != nil {
		t.Fatal("timed-out write was dispatched twice", next, e)
	}
	inspectionRequest := wcc.InspectionRequest{ProjectID: f.project, ExpectedVersion: current.Version, IdempotencyKey: newID()}
	inspected, err := f.s.RequestWorkspaceWriteInspection(ctx, f.actor, f.project, command.ID, inspectionRequest)
	if err != nil {
		t.Fatal(err)
	}
	restoredInspection, err := f.s.RequestWorkspaceWriteInspection(ctx, f.actor, f.project, command.ID, inspectionRequest)
	if err != nil || restoredInspection.InspectionID != inspected.InspectionID || restoredInspection.Version != inspected.Version {
		t.Fatal("lost inspection response could not recover before CAS", restoredInspection, err)
	}
	claim.OperationID = newID()
	inspection, err := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim)
	if err != nil || inspection == nil || inspection.Mode != "inspect" || inspection.Content != nil || inspection.ClaimID != inspected.InspectionID {
		t.Fatal(inspection, err)
	}
	effect := &wcc.Effect{NativeOperationID: wcc.NativeOperationID(command.ID), Path: request.Path, PreviousSHA256: request.ExpectedSHA256, SHA256: request.ContentSHA256, Generation: 1}
	result := wcc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, CommandID: command.ID, Scope: scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, RequestSHA256: command.RequestSHA256, ExecutionFence: 1, ClaimID: lease.ClaimID, Mode: "execute", Outcome: "applied", Effect: effect, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ResultSHA256: wcc.ResultDigest("applied", effect, ""), OperationID: newID()}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='succeeded' WHERE run_id=? AND id=?`, scope.RunID, scope.NodeID); err != nil {
		t.Fatal(err)
	}
	ack, err := f.s.CompleteWorkspaceWrite(ctx, machine, f.project, result)
	if err != nil || !ack.Acknowledged {
		t.Fatal("late original effect was lost", ack, err)
	}
	again, err := f.s.CompleteWorkspaceWrite(ctx, machine, f.project, result)
	if err != nil || again != ack {
		t.Fatal(again, err)
	}
	current, err = f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID)
	if err != nil || current.State != "applied" || current.Result == nil || current.Result.SHA256 != request.ContentSHA256 {
		t.Fatal(current, err)
	}
	result.Mode, result.ClaimID, result.Outcome, result.Effect, result.ErrorCode, result.OperationID = "inspect", inspection.ClaimID, "unknown", nil, "operation_unknown", newID()
	result.ResultSHA256 = wcc.ResultDigest(result.Outcome, result.Effect, result.ErrorCode)
	if _, err = f.s.CompleteWorkspaceWrite(ctx, machine, f.project, result); err != nil {
		t.Fatal(err)
	}
	current, err = f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID)
	if err != nil || current.State != "applied" || current.InspectionState != "completed" {
		t.Fatal("late unknown inspection downgraded applied fact", current, err)
	}
	if err = f.s.DB.QueryRow(`SELECT content FROM suma_workspace_writes WHERE id=?`, command.ID).Scan(&persisted); err != nil || persisted != "" {
		t.Fatal("delivery source remained after result", err)
	}
}

func TestWorkspaceWriteTaskBasisAndFactDrain(t *testing.T) {
	for _, mutation := range []string{`UPDATE suma_tasks SET description='changed' WHERE run_id=?`, `UPDATE suma_tasks SET status='accepted' WHERE run_id=?`, `UPDATE suma_tasks SET run_id='another-run' WHERE run_id=?`} {
		t.Run(mutation, func(t *testing.T) {
			f, machine, scope := workspaceObservationFixture(t)
			ctx := context.Background()
			req := workspaceWriteRequest(f, scope)
			if _, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.DB.Exec(mutation, scope.RunID); err != nil {
				t.Fatal(err)
			}
			req.IdempotencyKey = newID()
			_, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
			expertStatus(t, err, 409)
			_, err = f.s.ClaimWorkspaceWrite(ctx, machine, f.project, wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, OperationID: newID()})
			expertStatus(t, err, 409)
			targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
			if err != nil || len(targets.Items) != 1 || targets.Items[0].CanWrite {
				t.Fatal("changed task retained write eligibility", targets, err)
			}
		})
	}
	f, machine, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	req := workspaceWriteRequest(f, scope)
	command, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, OperationID: newID()})
	if err != nil || lease == nil {
		t.Fatal(lease, err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	result := wcc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, CommandID: command.ID, Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, RequestSHA256: command.RequestSHA256, ExecutionFence: 1, ClaimID: lease.ClaimID, Mode: "execute", Outcome: "not_applied", ErrorCode: "file_changed", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ResultSHA256: wcc.ResultDigest("not_applied", nil, "file_changed"), OperationID: newID()}
	if ack, e := f.s.CompleteWorkspaceWrite(ctx, machine, f.project, result); e != nil || !ack.Acknowledged {
		t.Fatal("revoked caller suppressed accepted no-effect receipt", ack, e)
	}
	_, err = f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID)
	expertStatus(t, err, 401)
}

func TestWorkspaceWritePoolWithdrawalBlocksNewWriteButDrainsFact(t *testing.T) {
	p := newPoolMachineFixture(t)
	f, ctx := p.f, context.Background()
	machine, profile := p.credential.Runner, toolTestProfile()
	profile.WorkspaceToolsVersion = "attempt-workspace-v1"
	if _, err := f.s.ReportPoolToolInventory(ctx, p.credential.Token, PoolToolInventoryRequest{RunnerID: machine.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.WorkerID, Name: "Pool native save", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Pool native save", Nodes: []dag.Node{{ID: "code", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "save bounded text", "tool_profile_id": pr.ID, "tool_profile_sha256": profile.ProfileSHA256}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a := candidatePoolClaim(t, p, p.credential, profile.CommandID)
	scope := wcc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: pr.ID, ProfileSHA256: profile.ProfileSHA256}
	req := workspaceWriteRequest(f, scope)
	command, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	claim := PoolWorkspaceWriteClaimRequest{ClaimRequest: wcc.ClaimRequest{ProjectID: f.project, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, OperationID: newID()}, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a}
	lease, err := f.s.ClaimPoolWorkspaceWrite(ctx, p.credential.Token, claim)
	if err != nil || lease == nil {
		t.Fatal(lease, err)
	}
	req.IdempotencyKey = newID()
	if _, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, req); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	_, err = f.s.ClaimPoolWorkspaceWrite(ctx, p.credential.Token, claim)
	expertStatus(t, err, 403)
	effect := &wcc.Effect{NativeOperationID: wcc.NativeOperationID(command.ID), Path: req.Path, PreviousSHA256: req.ExpectedSHA256, SHA256: req.ContentSHA256, Generation: 1}
	result := wcc.ResultRequest{ProjectID: f.project, WorkerID: a.WorkerID, CommandID: command.ID, Scope: scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, RequestSHA256: command.RequestSHA256, ExecutionFence: 1, ClaimID: lease.ClaimID, Mode: "execute", Outcome: "applied", Effect: effect, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ResultSHA256: wcc.ResultDigest("applied", effect, ""), OperationID: newID()}
	if ack, e := f.s.CompletePoolWorkspaceWrite(ctx, p.credential.Token, PoolWorkspaceWriteResultRequest{ResultRequest: result, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: a}); e != nil || !ack.Acknowledged {
		t.Fatal("withdrawn grant suppressed fixed effect fact", ack, e)
	}
	got, err := f.s.GetWorkspaceWrite(ctx, f.actor, f.project, command.ID)
	if err != nil || got.State != "applied" {
		t.Fatal(got, err)
	}
}
