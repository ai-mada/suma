package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

func workspaceHostEventFixture(a dag.Assignment, sha string) wo.Event {
	e := wo.Event{Protocol: wo.Protocol, Source: wo.Source, SchemaVersion: 1, Sequence: 1, OperationID: "go-read-invocation", Revision: 1, Action: "read", Phase: "started", StartedAt: "2026-10-29T01:00:00Z", RecordedAt: "2026-10-29T01:00:00Z", RequestSHA256: strings.Repeat("d", 64), PolicySHA256: sha, ProfileSHA256: sha, Path: "src/current.go"}
	e.EventID = wo.CanonicalEventID(dag.WorkspaceOperationIdentity(a.TenantID, a.ProjectID, a), e)
	return e
}

func TestWorkspaceHostProgressPrivateClosedEnvelopeAndCurrentAuthority(t *testing.T) {
	f, token, request, profile, _ := modelExecutionFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claimed["assignment"])), &a); err != nil || a.TenantID != f.user.TenantID || a.ProjectID != f.project {
		t.Fatal("private original scope missing", claimed, err)
	}
	event := workspaceHostEventFixture(a, profile.Profile.ProfileSHA256)
	body := HostOperationProgressRequest{ProjectID: f.project, Assignment: a, OperationID: event.EventID, HostOperation: event}
	for range 2 {
		if ack := f.bearer("POST", "/runner/progress", token, body, 200); ack["acknowledged"] != true {
			t.Fatal(ack)
		}
	}
	bad := map[string]any{"project_id": f.project, "assignment": a, "operation_id": event.EventID, "host_operation": event, "message": ""}
	f.bearer("POST", "/runner/progress", token, bad, 400)
	fakeSDK := map[string]any{"schema_version": 1, "command_id": profile.Profile.CommandID, "profile_sha256": profile.Profile.ProfileSHA256, "protocol": profile.Profile.Protocol, "kind": "activity_completed", "message": "SDK says read finished", "host_operation": event}
	f.bearer("POST", "/runner/progress", token, map[string]any{"project_id": f.project, "assignment": a, "operation_id": newID(), "tool_event": fakeSDK}, 400)
	foreign := a
	foreign.ProjectID = "another-project"
	wrong := workspaceHostEventFixture(foreign, profile.Profile.ProfileSHA256)
	f.bearer("POST", "/runner/progress", token, HostOperationProgressRequest{ProjectID: f.project, Assignment: foreign, OperationID: wrong.EventID, HostOperation: wrong}, 400)
	page := f.call("GET", "/runs/"+a.RunID+"/nodes/"+a.NodeID+"/attempts/1/tool-events?project_id="+f.project, nil, 200)
	if len(page["items"].([]any)) != 1 {
		t.Fatal("host replay duplicated history", page)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Protocol fixture finished with original failure", ToolEvidence: &tc.Evidence{Profile: profile.Profile, Terminal: "failed", EventCount: 1, UsageState: tc.UsageUnreported}}
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
	late := event
	late.Sequence, late.Revision, late.Phase, late.ErrorCode = 2, 2, "failed", "native_workspace_missing"
	late.RecordedAt, late.CompletedAt = "2026-10-29T01:00:01Z", "2026-10-29T01:00:01Z"
	late.EventID = wo.CanonicalEventID(dag.WorkspaceOperationIdentity(a.TenantID, a.ProjectID, a), late)
	f.bearer("POST", "/runner/progress", token, HostOperationProgressRequest{ProjectID: f.project, Assignment: a, OperationID: late.EventID, HostOperation: late}, 200)
	state, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, a.RunID)
	if err != nil || state.State != "failed" || state.Nodes[0].Attempt != a.Attempt {
		t.Fatal("late HTTP metadata changed original result", state, err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/progress", token, body, 403)
}

func TestWorkspaceHostProgressPoolOriginalMachineAndFrozenDispatch(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	machine := p.credential.Runner
	definition := toolTestProfile()
	definition.WorkspaceToolsVersion, definition.ModelExecutionProtocol = "attempt-workspace-v1", modelcontract.Protocol
	model := nativeWorkspaceModelForTest(t)
	if _, err := f.s.ReportPoolToolInventory(ctx, p.credential.Token, PoolToolInventoryRequest{RunnerID: machine.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true, ModelDeployments: []modelcontract.Deployment{model.Deployment}}}}); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.WorkerID, Name: "Pool host operation fixture", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Pool host metadata protocol", Nodes: []dag.Node{{ID: "code", Type: "agent", Command: []string{definition.CommandID}, Config: map[string]any{"goal": "Record only actual bounded host facts", "tool_profile_id": profile.ID, "tool_profile_sha256": definition.ProfileSHA256, "model_execution": model, "max_steps": model.Parameters.MaxSteps, "max_output_tokens": model.Parameters.MaxOutputTokens}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a := candidatePoolClaim(t, p, p.credential, definition.CommandID)
	event := workspaceHostEventFixture(a, definition.ProfileSHA256)
	body := PoolHostOperationProgressRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: a.PoolRunnerID, TenantID: a.TenantID, ProjectID: a.ProjectID, Assignment: a}, OperationID: event.EventID, HostOperation: event}
	f.bearer("POST", "/pool-runner/progress", p.credential.Token, body, 200)
	ordinary := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	f.bearer("POST", "/runner/progress", ordinary, HostOperationProgressRequest{ProjectID: f.project, Assignment: a, OperationID: event.EventID, HostOperation: event}, 403)
	wrong := body
	wrong.Assignment.PoolCredentialVersion++
	f.bearer("POST", "/pool-runner/progress", p.credential.Token, wrong, 403)
	// Withdrawing new project work does not discard historical metadata for
	// this already frozen dispatch. Revoking the machine still rejects replay.
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/progress", p.credential.Token, body, 200)
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Protocol fixture finished before new work", ToolEvidence: &tc.Evidence{Profile: definition, Terminal: "failed", EventCount: 1, UsageState: tc.UsageUnreported}}
	f.bearer("POST", "/pool-runner/results", p.credential.Token, PoolRunnerResultRequest{PoolRunnerAssignmentRequest: body.PoolRunnerAssignmentRequest, Result: result}, 200)
	late := event
	late.Sequence, late.Revision, late.Phase, late.ErrorCode = 2, 2, "failed", "native_workspace_missing"
	late.RecordedAt, late.CompletedAt = "2026-10-29T01:00:01Z", "2026-10-29T01:00:01Z"
	late.EventID = wo.CanonicalEventID(dag.WorkspaceOperationIdentity(a.TenantID, a.ProjectID, a), late)
	lateBody := body
	lateBody.OperationID, lateBody.HostOperation = late.EventID, late
	f.bearer("POST", "/pool-runner/progress", p.credential.Token, lateBody, 200)
	state, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
	if err != nil || state.State != "failed" {
		t.Fatal("late Pool metadata replaced original failure", state, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET status='revoked' WHERE runner_id=?`, a.PoolRunnerID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/progress", p.credential.Token, body, 403)
}
