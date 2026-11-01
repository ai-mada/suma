package platform

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

// Explicit database fixtures exercise the read model only. They are not
// scheduler dispatches, native process observations or proof of physical stop.
func insertImpactRun193(t *testing.T, tx *sql.Tx, tenant, project, id, state string) {
	t.Helper()
	_, err := tx.Exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at)VALUES(?,?,?,?,1,?,1,?,?,?)`, id, tenant, project, "owned-impact-flow193", "Owned impact source", state, "2026-11-01T10:00:00Z", "2026-11-01T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
}

func insertImpactNode193(t *testing.T, tx *sql.Tx, run, id, state string, p ToolProfile) {
	t.Helper()
	node := dag.Node{ID: id, Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256}}
	_, err := tx.Exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec)VALUES(?,?,?,'',1,1,?,'','','','',0,0,'',?)`, run, id, state, p.WorkerID, stringify(node))
	if err != nil {
		t.Fatal(err)
	}
}

func TestToolProfileImpactUsesCurrentProjectSourceAndKeepsStopSeparate(t *testing.T) {
	f, _, p := toolFixture(t)
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct{ run, runState, node, nodeState string }{{"impact-running193", "running", "work", "running"}, {"impact-paused193", "paused", "work", "waiting"}, {"impact-terminal193", "running", "done", "succeeded"}, {"impact-history193", "succeeded", "old", "running"}} {
		insertImpactRun193(t, tx, f.user.TenantID, f.project, x.run, x.runState)
		insertImpactNode193(t, tx, x.run, x.node, x.nodeState, p)
	}
	insertImpactRun193(t, tx, f.user.TenantID, "hidden-project193", "hidden-project-run193", "running")
	insertImpactNode193(t, tx, "hidden-project-run193", "private-project-node", "running", p)
	insertImpactRun193(t, tx, "hidden-tenant193", f.project, "hidden-tenant-run193", "running")
	insertImpactNode193(t, tx, "hidden-tenant-run193", "private-tenant-node", "running", p)
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	before := expertCount(t, f, "suma_operations")
	d, err := f.s.GetToolProfileDiagnostics(ctx, f.actor, f.project, p.ID)
	if err != nil || d.Impact == nil || d.Impact.Coverage != "complete" || d.Impact.Scope != "active_project_runs" || d.Impact.RunsExamined != 3 || d.Impact.NodesExamined != 3 || d.Impact.ObservedReferenceCount != 2 || len(d.Impact.Items) != 2 {
		t.Fatal("current active source scope wrong", d.Impact, err)
	}
	for _, item := range d.Impact.Items {
		if item.NodeID != "work" {
			t.Fatal("terminal/history/other scope leaked", item)
		}
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "revoke", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	d, err = f.s.GetToolProfileDiagnostics(ctx, f.actor, f.project, p.ID)
	if err != nil || d.Tool.State != "revoked" || d.Tool.Available || d.Impact == nil || d.Impact.ObservedReferenceCount != 2 {
		t.Fatal("revoke fabricated process termination", d, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET spec='unavailable native source' WHERE run_id='impact-running193'`); err != nil {
		t.Fatal(err)
	}
	d, err = f.s.GetToolProfileDiagnostics(ctx, f.actor, f.project, p.ID)
	if err != nil || d.Impact == nil || d.Impact.Coverage != "unknown" || d.Impact.Reason != "source_unavailable" || d.Impact.ObservedReferenceCount != 1 {
		t.Fatal("unavailable source became a complete zero", d.Impact, err)
	}
	if expertCount(t, f, "suma_operations") != before+1 {
		t.Fatal("diagnostic read created operations")
	}
}

func TestToolProfileImpactBoundsIndexedRunsAndNodes(t *testing.T) {
	for _, kind := range []string{"run_limit", "node_limit"} {
		t.Run(kind, func(t *testing.T) {
			f, _, p := toolFixture(t)
			ctx := context.Background()
			tx, err := f.s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "run_limit" {
				for i := 0; i < 201; i++ {
					run := fmt.Sprintf("impact-run-%03d", i)
					insertImpactRun193(t, tx, f.user.TenantID, f.project, run, "running")
					insertImpactNode193(t, tx, run, "work", "running", p)
				}
			} else {
				insertImpactRun193(t, tx, f.user.TenantID, f.project, "impact-large-run193", "running")
				for i := 0; i < 2001; i++ {
					insertImpactNode193(t, tx, "impact-large-run193", fmt.Sprintf("node-%04d", i), "running", p)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			d, err := f.s.GetToolProfileDiagnostics(ctx, f.actor, f.project, p.ID)
			if err != nil || d.Impact == nil || d.Impact.Coverage != "partial" || d.Impact.Reason != kind || !d.Impact.ItemsTruncated || len(d.Impact.Items) != 5 {
				t.Fatal("bounded source was presented as complete", d.Impact, err)
			}
			if kind == "run_limit" && (d.Impact.RunsExamined != 200 || d.Impact.NodesExamined != 200 || d.Impact.ObservedReferenceCount != 200) {
				t.Fatal(d.Impact)
			}
			if kind == "node_limit" && (d.Impact.RunsExamined != 1 || d.Impact.NodesExamined != 2000 || d.Impact.ObservedReferenceCount != 2000) {
				t.Fatal(d.Impact)
			}
		})
	}
}
