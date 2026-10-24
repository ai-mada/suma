package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// Exercise the authenticated HTTP result boundary, not only the protocol parser.
func TestToolHTTPStoppedZeroExitCannotSucceed(t *testing.T) {
	for _, terminal := range []string{"none", "failed", "completed", "unknown"} {
		t.Run(terminal, func(t *testing.T) {
			f, token, profile := toolFixture(t)
			ctx := context.Background()
			if _, err := f.s.ReportToolInventory(ctx, f.user, f.project, token, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
				t.Fatal(err)
			}
			var err error
			profile, err = f.s.CommandToolProfile(ctx, f.user, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Stopped zero exit", "nodes": []dag.Node{{ID: "agent", Type: "agent", Command: []string{profile.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}, {ID: "quality", Type: "test", Command: []string{"quality"}, DependsOn: []string{"agent"}}}}), 201)
			r := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
			claim := f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}, "idempotency_key": newID()}, 200)
			var a dag.Assignment
			raw, _ := json.Marshal(claim["assignment"])
			if json.Unmarshal(raw, &a) != nil || a.WorkerID != profile.WorkerID {
				t.Fatal("missing current matching assignment", claim)
			}
			result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Stopped: true, ExitCode: 0}
			if terminal != "none" {
				result.ToolEvidence = &toolcontract.Evidence{Profile: profile.Profile, Terminal: terminal, EventCount: 1, UsageState: toolcontract.UsageUnreported}
			}
			f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
			current := f.call("GET", "/runs/"+r["id"].(string)+"?project_id="+f.project, nil, 200)
			if current["state"] != "failed" {
				t.Fatal("confirmed stop did not fail active run", current)
			}
			for _, item := range current["nodes"].([]any) {
				node := item.(map[string]any)
				if node["id"] == "agent" && node["state"] != "failed" || node["id"] == "quality" && (node["state"] != "skipped" || node["attempt"] != float64(0)) {
					t.Fatal("stopped agent succeeded or dispatched tests", node)
				}
				if node["id"] == "agent" && terminal == "none" && node["tool_evidence"] != nil {
					t.Fatal("startup stop fabricated protocol proof", node)
				}
			}
			// Failure projection and receipt replay must retain one actual issue.
			f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
			var issues int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issues WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&issues); err != nil || issues != 1 {
				t.Fatal("stopped outcome did not produce one deduplicated issue", issues, err)
			}
			var issueData string
			if err = f.s.DB.QueryRow(`SELECT data FROM suma_issues WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&issueData); err != nil {
				t.Fatal(err)
			}
			var source map[string]any
			if json.Unmarshal([]byte(issueData), &source) != nil || source["run_id"] != a.RunID {
				t.Fatal("stopped failure issue lost its run identity", issueData)
			}
		})
	}
}
