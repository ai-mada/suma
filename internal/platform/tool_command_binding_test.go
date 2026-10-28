package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestBoundCommandClaimRejectsStructuredProfileAndDriftedReceipt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	worker := "bound-command-worker"
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	structured := toolTestProfile()
	text := tc.Profile{CommandID: "fixed-shell", Adapter: "command", Protocol: "text-v1", ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("d", 64), ToolVersion: "fixed text fixture"}
	profiles := []ToolProfile{}
	for _, definition := range []tc.Profile{structured, text} {
		p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: definition.CommandID, Profile: definition, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, p)
	}
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: structured, Available: true}, {Profile: text, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	runs := []string{}
	for i := range profiles {
		var err error
		profiles[i], err = f.s.CommandToolProfile(ctx, f.actor, f.project, profiles[i].ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profiles[i].Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		p := profiles[i]
		w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Fixed command qualification", "nodes": []any{map[string]any{"id": "produce", "type": "command", "command": []string{p.Profile.CommandID}, "config": map[string]any{"tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256}}}}), 201)
		run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
		runs = append(runs, run["id"].(string))
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:" + structured.CommandID, "command:" + text.CommandID}}), 200)
	var a dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claim["assignment"])), &a); err != nil || a.RunID != runs[1] {
		t.Fatal("structured profile was dispatchable as a command", claim, err)
	}
	blocked, err := f.e.GetRun(ctx, f.user.TenantID, f.project, runs[0])
	if err != nil || blocked.Nodes[0].Attempt != 0 {
		t.Fatal("ineligible profile consumed an attempt", blocked, err)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: text, Terminal: "completed", UsageState: tc.UsageUnreported}}
	result.ToolEvidence.Profile.ExecutableSHA256 = strings.Repeat("e", 64)
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 400)
	result.ToolEvidence.Profile = text
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
}
