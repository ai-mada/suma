package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkspaceTerminalCandidateWorkBuddyOrigin(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f, ctx := h.f, context.Background()
	original := terminalCandidateWaitingWorkBuddy(t, h)
	assertOriginalWaiting := func() dag.Run {
		t.Helper()
		run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, original.RunID)
		if err != nil || run.State != "running" || len(run.Nodes) != 1 {
			t.Fatalf("missing original cloud run: %+v %v", run, err)
		}
		node := run.Nodes[0]
		if node.ID != original.NodeID || node.Attempt != original.Attempt || node.Fence != original.Fence || node.State != "waiting" || node.Reason != "cloud.delivery_required" {
			t.Fatalf("original cloud attempt lost its delivery wait: %+v", node)
		}
		return run
	}
	cloudRun := assertOriginalWaiting()
	var originalAssignment string
	if err := f.s.DB.QueryRow(`SELECT assignment FROM dag_external_assignments WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=?`, f.user.TenantID, f.project, original.RunID, original.NodeID, original.Attempt).Scan(&originalAssignment); err != nil {
		t.Fatal(err)
	}
	assignmentSHA := digest(originalAssignment)
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil || task.RunID != original.RunID {
		t.Fatalf("task is missing its original cloud run: %+v %v", task, err)
	}
	before := workspaceTerminalTaskSQL(t, f, task.ID)
	session, preview := terminalCandidateStoppedWorkBuddy(t, f, task)
	beforeRuns := domainACount(t, f.s, "dag_runs")
	beforeCreates, beforeQueries := h.creates.Load(), h.queries.Load()
	previewPath := "/workspace/terminal-sessions/" + session.ID + "/candidate-plans"
	planWire := f.call("POST", previewPath, preview, 201)
	plan := issueRepairRuntimeDecode[WorkspaceTerminalCandidatePlan](t, planWire)
	if !plan.Valid || plan.Source.SnapshotSHA256 != session.SourceSnapshotSHA256 || workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 {
		t.Fatal("preview changed the formal task binding or lost the stopped source", plan)
	}
	assertOrigin := func(wire map[string]any) {
		t.Helper()
		source, _ := wire["source"].(map[string]any)
		origin, _ := source["workbuddy_origin"].(map[string]any)
		if len(origin) != 6 || origin["run_id"] != original.RunID || origin["run_version"] != float64(cloudRun.Version) || origin["node_id"] != original.NodeID || origin["attempt"] != float64(original.Attempt) || origin["fence"] != float64(original.Fence) || origin["assignment_sha256"] != assignmentSHA {
			t.Fatalf("expected exact original waiting cloud attempt provenance: %#v", origin)
		}
	}
	assertOrigin(planWire)
	planPath := "/workspace/terminal-candidate-plans/" + plan.ID
	assertOrigin(f.call("GET", planPath+"?project_id="+f.project, nil, 200))
	if domainACount(t, f.s, "dag_runs") != beforeRuns {
		t.Fatal("candidate preview created a run")
	}

	changeFixture := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	assertUnbound := func(change string) {
		t.Helper()
		if workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 || domainACount(t, f.s, "dag_runs") != beforeRuns {
			t.Fatalf("%s changed the task binding or created a run", change)
		}
	}
	assertBlocked := func(change string) {
		t.Helper()
		stale := f.call("GET", planPath+"?project_id="+f.project, nil, 200)
		if stale["valid"] != false {
			t.Fatalf("candidate plan remained valid after %s", change)
		}
		start.IdempotencyKey = newID()
		f.call("POST", planPath+"/start", start, 409)
		assertUnbound(change)
	}
	const setWaitReason = `UPDATE dag_nodes SET reason=? WHERE run_id=? AND id=?`
	changeFixture(setWaitReason, "cloud.waiting_input", original.RunID, original.NodeID)
	assertBlocked("original wait reason changed")
	changeFixture(setWaitReason, "cloud.delivery_required", original.RunID, original.NodeID)
	const setRunVersion = `UPDATE dag_runs SET version=? WHERE id=?`
	changeFixture(setRunVersion, cloudRun.Version+1, original.RunID)
	assertBlocked("original run version changed")
	changeFixture(setRunVersion, cloudRun.Version, original.RunID)
	// Inject drift after the provisional run insert, between the preflight and
	// binding. Both the drift and provisional run must roll back on conflict.
	changeFixture(fmt.Sprintf(`CREATE TRIGGER drift_workbuddy_origin AFTER INSERT ON dag_runs
		WHEN NEW.id='%s' BEGIN
			UPDATE dag_nodes SET reason='cloud.waiting_input' WHERE run_id='%s' AND id='%s';
		END`, plan.RunID, original.RunID, original.NodeID))
	if ready := f.call("GET", planPath+"?project_id="+f.project, nil, 200); ready["valid"] != true {
		t.Fatal("fixture must pass preflight before the trigger fires")
	}
	start.IdempotencyKey = newID()
	f.call("POST", planPath+"/start", start, 409)
	assertUnbound("origin drift during the candidate start")
	assertOriginalWaiting()
	changeFixture(`DROP TRIGGER drift_workbuddy_origin`)

	start.IdempotencyKey = newID()
	started := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", planPath+"/start", start, 201))
	replay := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", planPath+"/start", start, 201))
	if started.ID != plan.RunID || replay.ID != started.ID || domainACount(t, f.s, "dag_runs") != beforeRuns+1 {
		t.Fatalf("candidate start did not bind exactly one original reserved run: %q %q %q", plan.RunID, started.ID, replay.ID)
	}
	bound, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil || bound.RunID != started.ID || bound.Version != task.Version+1 || bound.Status != "in_progress" {
		t.Fatalf("explicit start failed to bind the local run exactly once: %+v %v", bound, err)
	}
	delivery := issueRepairRuntimeDecode[TaskDelivery](t, f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200))
	if delivery.FormalAcceptance || delivery.Run == nil || delivery.Run.RunID != started.ID || delivery.Run.WorkBuddyDelivery != nil {
		t.Fatalf("local run inherited cloud wait or acceptance: %+v", delivery)
	}
	assertOrigin(f.call("GET", planPath+"?project_id="+f.project, nil, 200))
	assertOriginalWaiting()
	if h.creates.Load() != beforeCreates || h.queries.Load() != beforeQueries {
		t.Fatal("local candidate operations contacted the cloud supplier")
	}
}

func terminalCandidateWaitingWorkBuddy(t *testing.T, h workBuddyTaskDispatchFixture) dag.Assignment {
	t.Helper()
	ctx, f := context.Background(), h.f
	cloud := &cloudFixture{f: f, w: h.w}
	cloud.claimToken = f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "cloud-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	cloud.actor = toolActorForTest(t, f, cloud.claimToken)
	plan := workBuddyTaskPreview(t, h)
	run := workBuddyTaskStart(t, h, plan, newID())
	view, err := h.w.GetCloudNode(ctx, f.actor, f.project, run.ID, plan.NodeID)
	if err != nil || view.Delegation == nil {
		t.Fatal("missing original WorkBuddy delegation", err)
	}
	cloud.delegation = *view.Delegation
	claim := cloudHTTPClaim(t, cloud, false, newID(), 200)
	if claim == nil || claim.RunID != run.ID || claim.NodeID != plan.NodeID {
		t.Fatal("missing original WorkBuddy assignment")
	}
	cloud.request = wc.CloudCreateRequest{ProjectID: f.project, DelegationID: plan.DelegationID, ExpectedVersion: 1, Assignment: repositoryTestRef(*claim), IdempotencyKey: newID()}
	submission := createCloudFixture(t, cloud)
	if submission.Task == nil || submission.Task.Status != "completed" {
		t.Fatal("fixture did not observe completed supplier work")
	}
	assignment, result := cloudResultFixture(t, cloud)
	result.WorkBuddyCloudWait.SubmissionID, result.WorkBuddyCloudWait.SubmissionVersion = submission.ID, submission.Version
	if err = completeCloudFixture(t, cloud, assignment, result); err != nil {
		t.Fatal(err)
	}
	return assignment
}

func terminalCandidateStoppedWorkBuddy(t *testing.T, f *fixture, task Record) (WorkspaceTerminalSession, WorkspaceTerminalCandidatePreviewRequest) {
	t.Helper()
	ctx := context.Background()
	const localWorker = "workbuddy-local-terminal"
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": localWorker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	worker := toolActorForTest(t, f, token)
	profiles := make([]ToolProfile, 0, 2)
	var observations []tc.Observation
	for _, name := range []string{"cloud-local-shell", "cloud-local-check"} {
		definition := toolTestProfile()
		definition.Adapter, definition.Protocol, definition.CommandID = "command", "text-v1", name
		definition.ProfileSHA256 = digest(name)
		if name == "cloud-local-shell" {
			definition.TerminalSessionProtocol = term.Protocol
		}
		p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: localWorker, Name: name, Profile: definition, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, p)
		observations = append(observations, tc.Observation{Profile: definition, Available: true})
	}
	if _, err := f.s.ReportToolInventory(ctx, worker, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: localWorker, OperationID: newID(), Items: observations}); err != nil {
		t.Fatal(err)
	}
	for i, p := range profiles {
		var err error
		profiles[i], err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
	}
	shell, producer := profiles[0], profiles[1]
	terminal := terminalPlatformFixture{f: f, token: token, profile: shell, request: WorkspaceTerminalCreateRequest{
		ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ToolProfileID: shell.ID, ExpectedToolVersion: shell.Version,
		Columns: 100, Rows: 30, TimeoutSeconds: 120, AcknowledgeShell: true, IdempotencyKey: newID(),
	}}
	session, assignment := terminal.start(t)
	stop := terminalExchangeForTest(session, assignment)
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, stop); err != nil {
		t.Fatal(err)
	}
	stop.OperationID, stop.State, stop.TreeStopped = newID(), "stopped", true
	stop.SourceSnapshotSHA256, stop.BaseCommit = strings.Repeat("e", 64), strings.Repeat("f", 40)
	stop.AdminBaseCommit = stop.BaseCommit
	if _, err := f.s.ExchangeWorkspaceTerminal(ctx, worker, f.project, stop); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": dag.Result{
		RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence,
		WorkerID: assignment.WorkerID, OperationID: newID(), Stopped: true, ExitCode: 0,
		ToolEvidence: &tc.Evidence{Profile: shell.Profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported},
	}}), 200)
	session, err := f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || session.SourceSnapshotSHA256 != stop.SourceSnapshotSHA256 {
		t.Fatalf("missing original stopped source: %+v %v", session, err)
	}
	verifier := dispatchTestBindingsProfile(t, f, "workbuddy-local-independent", "cloud-local-test", tr.Protocol)
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Local candidate with independent verification", "nodes": []any{
		map[string]any{"id": "capture", "type": "command", "command": []string{producer.Profile.CommandID}, "config": map[string]any{"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256}},
		map[string]any{"id": "independent", "type": "test", "command": []string{verifier.Profile.CommandID}, "depends_on": []string{"capture"}, "config": map[string]any{"candidate_from": "capture", "tool_profile_id": verifier.ID, "tool_profile_sha256": verifier.Profile.ProfileSHA256}},
	}}), 201)
	return session, WorkspaceTerminalCandidatePreviewRequest{
		ProjectID: f.project, ExpectedSessionVersion: session.Version, ExpectedTaskVersion: task.Version,
		ToolProfileID: producer.ID, ExpectedToolVersion: producer.Version, WorkflowID: workflow["id"].(string), WorkflowVersion: int(workflow["version"].(float64)),
		ProducerNodeID: "capture", AcknowledgeStoppedChanges: true, IdempotencyKey: newID(),
	}
}
