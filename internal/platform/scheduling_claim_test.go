package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestRunnerClaimHTTPRestoresOriginalDispatchAndClosedAttempt(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "领取回执恢复", "nodes": []any{map[string]any{"id": "work", "type": "command", "command": []string{"inspect"}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"]}), 201)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	request := f.body(map[string]any{"worker_id": "receipt-worker", "capabilities": []string{"command:inspect"}})
	first := f.bearer("POST", "/runner/claim", token, request, 200)
	recovered := f.bearer("POST", "/runner/claim", token, request, 200)
	var a, b dag.Assignment
	raw, _ := json.Marshal(first["assignment"])
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(recovered["assignment"])
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	if a.DispatchID == "" || a.DispatchID != b.DispatchID || a.Attempt != 1 || b.Attempt != 1 || a.Fence != b.Fence || b.DispatchState != "active" {
		t.Fatalf("lost ACK produced a new attempt: %+v %+v", a, b)
	}
	changed := map[string]any{}
	for key, value := range request {
		changed[key] = value
	}
	changed["capabilities"] = []string{"command:other"}
	f.bearer("POST", "/runner/claim", token, changed, 409)
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID()}
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
	closed := f.bearer("POST", "/runner/claim", token, request, 200)["assignment"].(map[string]any)
	if closed["dispatch_state"] != "closed" || closed["dispatch_id"] != a.DispatchID {
		t.Fatal("completed receipt offered another execution", closed)
	}
	var attempts, spent int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, a.RunID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT budget_spent FROM dag_scheduling_policies WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&spent); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || spent != 1 {
		t.Fatalf("receipt replay consumed additional attempt/budget: %d %d", attempts, spent)
	}
	missing := map[string]any{"project_id": f.project, "worker_id": a.WorkerID, "capabilities": []string{"command:inspect"}}
	f.bearer("POST", "/runner/claim", token, missing, 400)
}

func TestRunnerClaimActiveReplayRechecksPausedToolProfile(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "回执恢复仍需当前工具授权", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{profile.Profile.CommandID}, Config: map[string]any{"goal": "local protocol fixture", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	request := f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}})
	original := f.bearer("POST", "/runner/claim", token, request, 200)["assignment"].(map[string]any)
	// The first response may have been lost before a process starts. Pausing the
	// immutable profile must block recovering an executable active assignment.
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "pause", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/claim", token, request, 409)
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	recovered := f.bearer("POST", "/runner/claim", token, request, 200)["assignment"].(map[string]any)
	if recovered["dispatch_id"] != original["dispatch_id"] || recovered["attempt"] != original["attempt"] {
		t.Fatal("authorization recovery dispatched another attempt")
	}
}

func TestRunnerUsageRawFractionIsRejectedBeforeItCanBeStored(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "精确用量事实", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{profile.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	a := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)["assignment"]
	base := map[string]any{"project_id": f.project, "assignment": a, "operation_id": newID(), "tool_event": map[string]any{"schema_version": 1, "command_id": profile.Profile.CommandID, "profile_sha256": profile.Profile.ProfileSHA256, "protocol": profile.Profile.Protocol, "kind": "usage", "usage_state": "complete", "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}}
	f.bearer("POST", "/runner/progress", token, base, 200)
	raw, _ := json.Marshal(base)
	bad := strings.Replace(string(raw), `"input_tokens":0`, `"input_tokens":9007199254740991.1`, 1)
	req := httptest.NewRequest("POST", "http://suma.test/api/v1/runner/progress", bytes.NewBufferString(bad))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	written := httptest.NewRecorder()
	f.handler.ServeHTTP(written, req)
	if written.Code != 400 {
		t.Fatalf("fraction reached a rounded typed usage: %d %s", written.Code, written.Body.String())
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE event_type='tool_progress'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("invalid usage altered durable evidence: count=%d %v", count, err)
	}
}

func TestSchedulingScopedAPIReadCannotChangePolicy(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	policy := f.bearer("GET", "/scheduling/policy?project_id="+f.project, token, nil, 200)
	if len(policy["allowed_actions"].([]any)) != 0 {
		t.Fatal("API credential advertised human policy approval")
	}
	request := f.body(map[string]any{"expected_version": policy["version"], "policy": map[string]any{"concurrency_limit": 5, "resource_limits": map[string]any{}, "budget_limit": nil}})
	f.bearer("POST", "/scheduling/policy-previews", token, request, 403)
}
