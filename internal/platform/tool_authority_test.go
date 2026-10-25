package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolOnlyWorkerDoesNotGainReleaseAuthority(t *testing.T) {
	f, token, p := toolFixture(t)
	runnerActor := toolActorForTest(t, f, token)
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = f.s.releaseWorkerTx(ctx, tx, runnerActor, f.project, p.WorkerID, "", "", "identity"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"deployment", "build", "package_test"} {
		if _, err = f.s.releaseWorkerTx(ctx, tx, runnerActor, f.project, p.WorkerID, "not-granted", strings.Repeat("a", 64), kind); err == nil {
			t.Fatal("tool worker gained release", kind)
		}
	}
}

func TestToolEligibilityFiltersBeforeAttemptsAndRevokedEnabler(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	req := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: true}}}
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, req); err != nil {
		t.Fatal(err)
	}
	enable := ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()}
	p, err := f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, enable)
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Tool eligibility", Nodes: []dag.Node{{ID: "agent", Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256}}, {ID: "independent", Type: "command", Command: []string{"independent"}}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	// Cached role in f.user remains owner. SQL authority is now developer only.
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,'developer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, enable); err == nil {
		t.Fatal("revoked approval actor replayed cached management response")
	}
	runnerActor := toolActorForTest(t, f, token)
	var runnerProof *authorizedCommand
	claim, err := f.e.ClaimEligible(ctx, f.user.TenantID, f.project, p.WorkerID, []string{"command:" + p.Profile.CommandID, "command:independent"}, func(tx *sql.Tx) error {
		var e error
		runnerProof, e = f.s.runnerIdentityTx(ctx, tx, runnerActor, f.project, p.WorkerID)
		return e
	}, func(tx *sql.Tx, run, node string) (bool, error) {
		return f.s.toolClaimEligibleTx(ctx, tx, runnerProof, p.WorkerID, run, node)
	})
	if err != nil || claim == nil || claim.NodeID != "independent" {
		t.Fatal("blocked profile prevented independent candidate", claim, err)
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == "agent" && (n.State != "ready" || n.Attempt != 0) {
			t.Fatal("ineligible tool consumed attempt", n)
		}
	}
	p, err = f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if err != nil || p.Available || !strings.Contains(p.Reason, "审批权限") {
		t.Fatal("stale enabling authority", p, err)
	}
}

func TestToolHTTPScopesBoundWorkerAndUnknownFields(t *testing.T) {
	f, token, p := toolFixture(t)
	for _, scope := range []string{"api", "agent", "runner"} {
		key := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": scope}), 201)["token"].(string)
		f.bearer("GET", "/tool-profiles?project_id="+f.project, key, nil, 403)
	}
	body := map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "operation_id": newID(), "items": []toolcontract.Observation{{Profile: p.Profile, Available: true}}}
	f.bearer("POST", "/runner/tool-inventory", token, body, 200)
	wrong := map[string]any{"project_id": f.project, "worker_id": "other-worker", "operation_id": newID(), "items": []toolcontract.Observation{}}
	f.bearer("POST", "/runner/tool-inventory", token, wrong, 403)
	bad := map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "operation_id": newID(), "items": []any{map[string]any{"profile": p.Profile, "available": true, "raw_arguments": "must not persist"}}}
	f.bearer("POST", "/runner/tool-inventory", token, bad, 400)
}

func TestToolHTTPResultRequiresTerminalProofAndBoundLeaseIdentity(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	var err error
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Typed facts HTTP", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256}}}}), 201)
	r := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	claimed := f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}, "idempotency_key": newID()}, 200)
	var a dag.Assignment
	b, marshalErr := json.Marshal(claimed["assignment"])
	if marshalErr != nil || json.Unmarshal(b, &a) != nil || a.WorkerID != p.WorkerID {
		t.Fatal("no matching assignment", claimed, marshalErr)
	}
	other := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "other-tool-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	var before, after int64
	if err = f.s.DB.QueryRow(`SELECT lease_until FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, a.NodeID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/heartbeat", other, map[string]any{"project_id": f.project, "assignment": a}, 403)
	if err = f.s.DB.QueryRow(`SELECT lease_until FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, a.NodeID).Scan(&after); err != nil || before != after {
		t.Fatal("foreign heartbeat modified lease", before, after, err)
	}
	ev := toolcontract.Event{SchemaVersion: 1, CommandID: p.Profile.CommandID, ProfileSHA256: p.Profile.ProfileSHA256, Protocol: p.Profile.Protocol, Kind: "turn_started"}
	f.bearer("POST", "/runner/progress", other, map[string]any{"project_id": f.project, "assignment": a, "operation_id": newID(), "tool_event": ev}, 403)
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 0, OperationID: newID()}
	send := func(want int) {
		t.Helper()
		f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, want)
	}
	send(400)
	result.OperationID = newID()
	result.ToolEvidence = &toolcontract.Evidence{Profile: p.Profile, Terminal: "failed", EventCount: 1, UsageState: toolcontract.UsageUnreported}
	send(400)
	result.OperationID = newID()
	result.ToolEvidence.Terminal = "unknown"
	send(400)
	result.OperationID = newID()
	result.ToolEvidence.Terminal = "completed"
	result.ToolEvidence.Profile.ExecutableSHA256 = strings.Repeat("c", 64)
	send(400)
	result.OperationID = newID()
	result.ToolEvidence.Profile = p.Profile
	send(200)
	current := f.call("GET", "/runs/"+r["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["state"] != "succeeded" {
		t.Fatal("verified tool result not committed", current)
	}
}

func TestToolHTTPConfirmedStopReceiptCancelsWithoutProtocolSuccess(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	var err error
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Confirmed process stop", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256}}}}), 201)
	r := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	claimed := f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}, "idempotency_key": newID()}, 200)
	var a dag.Assignment
	b, _ := json.Marshal(claimed["assignment"])
	if json.Unmarshal(b, &a) != nil || a.WorkerID != p.WorkerID {
		t.Fatal("missing current assignment", claimed)
	}
	current := f.call("GET", "/runs/"+r["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/runs/"+r["id"].(string)+"/commands", f.body(map[string]any{"action": "cancel", "expected_version": current["version"], "payload": map[string]any{"reason": "verified stop test"}}), 200)
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Stopped: true, ToolEvidence: &toolcontract.Evidence{Profile: p.Profile, Terminal: "unknown", EventCount: 2, UsageState: toolcontract.UsageUnreported}}
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
	current = f.call("GET", "/runs/"+r["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["state"] != "cancelled" {
		t.Fatal("stop not committed", current)
	}
	node := current["nodes"].([]any)[0].(map[string]any)
	evidence := node["tool_evidence"].(map[string]any)
	if evidence["terminal"] != "unknown" || node["state"] != "cancelled" {
		t.Fatal("stop synthesized protocol success", node)
	}
}
