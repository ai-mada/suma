package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestSchedulingToolBlockersScopedRead(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	foreignProject := f.call("POST", "/projects", map[string]any{"title": "Foreign scheduling fixture", "idempotency_key": newID()}, 201)["id"].(string)
	const foreignWorker = "foreign-tool-worker"
	f.call("POST", "/release-workers", map[string]any{"project_id": foreignProject, "worker_id": foreignWorker, "profiles": []any{}, "deployment": false, "idempotency_key": newID()}, 201)
	foreign, err := f.s.CreateToolProfile(ctx, f.actor, foreignProject, ToolProfileCreateRequest{
		ProjectID: foreignProject, WorkerID: foreignWorker, Name: "Foreign configuration", Profile: profile.Profile, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	makeNode := func(id, profileID, hash, command string) dag.Node {
		return dag.Node{ID: id, Type: "agent", Command: []string{command}, Config: map[string]any{
			"tool_profile_id": profileID, "tool_profile_sha256": hash,
		}}
	}
	hash, command := profile.Profile.ProfileSHA256, profile.Profile.CommandID
	pending := makeNode("pending", profile.ID, hash, command)
	pending.DependsOn = []string{"original"}
	nodes := []dag.Node{
		makeNode("original", profile.ID, hash, command),
		makeNode("changed-digest", profile.ID, strings.Repeat("c", 64), command),
		makeNode("changed-command", profile.ID, hash, "other-command"),
		makeNode("same-profile", profile.ID, hash, command),
		makeNode("missing", "absent-profile", hash, command),
		makeNode("foreign", foreign.ID, hash, command),
		{ID: "unbound", Type: "command", Command: []string{"unbound"}},
		pending,
	}
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Fixed tool blocker fixture", "nodes": nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "workflow_version": 1}), 201)["id"].(string)
	queuePath := "/scheduling/queue?project_id=" + f.project + "&run_id=" + run
	readNode := func(node string) dag.NodeAdmissionSnapshot {
		t.Helper()
		return issueRepairRuntimeDecode[dag.NodeAdmissionSnapshot](t, f.call("GET", "/runs/"+run+"/nodes/"+node+"/scheduling?project_id="+f.project, nil, 200))
	}
	assertBlocker := func(snapshot dag.NodeAdmissionSnapshot, node dag.Node, want string) {
		t.Helper()
		if snapshot.RunID != run || snapshot.NodeID != node.ID {
			t.Fatalf("wrong admission identity: %+v", snapshot)
		}
		var code string
		for _, blocker := range snapshot.Blockers {
			if blocker.Source == "tool" {
				if code != "" || blocker.RunID != run || blocker.NodeID != node.ID || blocker.Message == "" {
					t.Fatalf("wrong tool blocker scope: %+v", blocker)
				}
				code = blocker.Code
			}
		}
		if code != want || want != "" && snapshot.Status != "blocked" || want == "" && snapshot.NodeState == "ready" && snapshot.Status != "unknown" {
			t.Fatalf("wanted blocker %q, got %+v", want, snapshot)
		}
		if node.Config == nil {
			if snapshot.ToolBinding != nil {
				t.Fatalf("unbound command returned a tool binding: %+v", snapshot)
			}
		} else {
			expected := dag.SchedulingToolBinding{ProfileID: node.Config["tool_profile_id"].(string), ProfileSHA256: node.Config["tool_profile_sha256"].(string), CommandID: node.Command[0]}
			if snapshot.ToolBinding == nil || *snapshot.ToolBinding != expected {
				t.Fatalf("wrong fixed binding: got %+v, want %+v", snapshot.ToolBinding, expected)
			}
		}
	}
	checkRead := func(want string) {
		t.Helper()
		before := historyDatabaseHash(t, f.s.DB)
		page := issueRepairRuntimeDecode[dag.ReadyPage](t, f.call("GET", queuePath, nil, 200))
		if page.Total != len(nodes)-1 || len(page.Items) != len(nodes)-1 {
			t.Fatalf("wrong ready scope: %+v", page)
		}
		rows := make(map[string]*dag.NodeAdmissionSnapshot)
		for _, item := range page.Items {
			rows[item.NodeID] = item.Admission
		}
		for _, node := range nodes[:len(nodes)-1] {
			expected := want
			switch node.ID {
			case "changed-digest", "changed-command":
				expected = "tool_profile_changed"
			case "missing", "foreign":
				expected = "tool_profile_missing"
			case "unbound":
				expected = ""
			}
			if rows[node.ID] == nil {
				t.Fatalf("missing admission for %s", node.ID)
			}
			assertBlocker(*rows[node.ID], node, expected)
		}
		assertBlocker(readNode("original"), nodes[0], want)
		assertBlocker(readNode("pending"), pending, want)
		if after := historyDatabaseHash(t, f.s.DB); before != after {
			t.Fatal("scheduling reads mutated durable state")
		}
	}

	checkRead("tool_profile_paused")
	toolDiagnosticsReportInventory(t, f, token, profile, toolcontract.Observation{Profile: profile.Profile, Available: true})
	profile = toolDiagnosticsEnable(t, f, profile)
	checkRead("")
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_profiles SET enabled_by=? WHERE id=?`, "missing-approver", profile.ID); err != nil {
		t.Fatal(err)
	}
	checkRead("tool_activation_revoked")
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{
		ProjectID: f.project, Action: "revoke", ExpectedVersion: profile.Version, IdempotencyKey: newID(),
	})
	if err != nil || profile.State != "revoked" {
		t.Fatal(profile, err)
	}
	checkRead("tool_profile_revoked")
}
