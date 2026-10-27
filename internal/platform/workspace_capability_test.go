package platform

import (
	"context"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func reportWorkspaceCapabilityForTest(t *testing.T, f *fixture, machine Actor, scope woc.Scope, read, write string) {
	t.Helper()
	profile, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, scope.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(context.Background(), machine, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: scope.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true, WorkspaceObservationProtocol: read, WorkspaceWriteProtocol: write}}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceCapabilityRequiresCurrentTransportDeclaration(t *testing.T) {
	f, machine, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	request := workspaceWriteRequest(f, scope)
	read := woc.CreateRequest{ProjectID: f.project, Scope: scope, Operation: "list", Arguments: woc.Arguments{}, IdempotencyKey: newID()}
	// Native MCP tools alone do not prove an outbound observation/save broker.
	reportWorkspaceCapabilityForTest(t, f, machine, scope, "", "")
	targets, err := f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 0 {
		t.Fatal("legacy worker was offered as a remote target", targets, err)
	}
	_, err = f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, read)
	expertStatus(t, err, 409)
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request)
	expertStatus(t, err, 409)
	// A read-capable worker remains useful, without implying save support.
	reportWorkspaceCapabilityForTest(t, f, machine, scope, woc.Protocol, "")
	targets, err = f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 1 || targets.Items[0].CanWrite {
		t.Fatal("read transport implied write support", targets, err)
	}
	if _, err = f.s.CreateWorkspaceObservation(ctx, f.actor, f.project, read); err != nil {
		t.Fatal("declared read transport was denied", err)
	}
	_, err = f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request)
	expertStatus(t, err, 409)
	// The same approved native profile can acquire a current transport report.
	reportWorkspaceCapabilityForTest(t, f, machine, scope, woc.Protocol, wcc.Protocol)
	targets, err = f.s.GetWorkspaceObservationTargets(ctx, f.actor, f.project)
	if err != nil || len(targets.Items) != 1 || !targets.Items[0].CanWrite {
		t.Fatal(targets, err)
	}
	created, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request)
	if err != nil || created.Scope.ProfileSHA256 != scope.ProfileSHA256 {
		t.Fatal(created, err)
	}
	// New execution must recheck the report after the command was created.
	reportWorkspaceCapabilityForTest(t, f, machine, scope, woc.Protocol, "")
	_, err = f.s.ClaimWorkspaceWrite(ctx, machine, f.project, wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, OperationID: newID()})
	expertStatus(t, err, 409)
}

func TestWorkspaceCapabilityWithdrawalKeepsOriginalWriteFacts(t *testing.T) {
	f, machine, scope := workspaceObservationFixture(t)
	ctx := context.Background()
	request := workspaceWriteRequest(f, scope)
	created, err := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	claim := wcc.ClaimRequest{ProjectID: f.project, WorkerID: scope.WorkerID, Scope: scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, OperationID: newID()}
	lease, err := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim)
	if err != nil || lease == nil {
		t.Fatal(lease, err)
	}
	reportWorkspaceCapabilityForTest(t, f, machine, scope, "", "")
	if replay, e := f.s.CreateWorkspaceWrite(ctx, f.actor, f.project, request); e != nil || replay.ID != created.ID {
		t.Fatal("capability withdrawal hid original create receipt", replay, e)
	}
	if page, e := f.s.ListWorkspaceWrites(ctx, f.actor, f.project); e != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	if _, e := f.s.GetWorkspaceWrite(ctx, f.actor, f.project, created.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.ClaimWorkspaceWrite(ctx, machine, f.project, claim); e == nil {
		t.Fatal("withdrawn capability redelivered write content")
	}
	effect := &wcc.Effect{NativeOperationID: wcc.NativeOperationID(created.ID), Path: request.Path, PreviousSHA256: request.ExpectedSHA256, SHA256: request.ContentSHA256, Generation: 1}
	result := wcc.ResultRequest{ProjectID: f.project, WorkerID: scope.WorkerID, CommandID: created.ID, Scope: scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, RequestSHA256: created.RequestSHA256, ExecutionFence: 1, ClaimID: lease.ClaimID, Mode: "execute", Outcome: "applied", Effect: effect, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ResultSHA256: wcc.ResultDigest("applied", effect, ""), OperationID: newID()}
	ack, err := f.s.CompleteWorkspaceWrite(ctx, machine, f.project, result)
	if err != nil || !ack.Acknowledged {
		t.Fatal("capability withdrawal discarded accepted effect", ack, err)
	}
	current, err := f.s.GetWorkspaceWrite(ctx, f.actor, f.project, created.ID)
	if err != nil || current.State != "applied" || current.Result == nil {
		t.Fatal(current, err)
	}
}
