package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudNodeLocatesFormalTaskDelivery(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f := h.f
	ctx := context.Background()
	plan := workBuddyTaskPreview(t, h)
	run := workBuddyTaskStart(t, h, plan, newID())
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w, WebDir: t.TempDir()}).Handler()
	other := cloudOtherWriter(t, &cloudFixture{f: f})
	foreignProject := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)["id"].(string)
	path := func(project string) string {
		query := url.Values{"project_id": {project}, "run_id": {run.ID}, "node_id": {plan.NodeID}}
		return "/workbuddy-cloud-nodes?" + query.Encode()
	}
	counts := map[string]int{}
	for _, table := range []string{"suma_operations", "suma_activity", "dag_runs", "suma_workbuddy_cloud_delegations", "suma_workbuddy_cloud_submissions"} {
		counts[table] = domainACount(t, f.s, table)
	}
	assertLink := func(relation string, version int) {
		t.Helper()
		view := f.call("GET", path(f.project), nil, 200)
		want := map[string]any{"task_id": h.task.ID, "task_version": float64(version), "run_relation": relation}
		if stringify(view["task_delivery"]) != stringify(want) {
			t.Fatalf("expected exact %s locator at version %d, got %v", relation, version, view["task_delivery"])
		}
	}
	assertLink("current", h.task.Version+1)
	shared, err := h.w.GetCloudNode(ctx, other, f.project, run.ID, plan.NodeID)
	if err != nil || shared.Delegation != nil || shared.TaskDelivery == nil || shared.TaskDelivery.TaskID != h.task.ID {
		t.Fatal("authorized teammate could not read locator independently of private delegation", err)
	}
	f.call("GET", path(foreignProject), nil, 404)

	// The collaborator cannot see the delegation, so the protected dispatch
	// source must independently reject a node with a different specification.
	var originalSpec string
	if err = f.s.DB.QueryRow("SELECT spec FROM dag_nodes WHERE run_id=? AND id=?", run.ID, plan.NodeID).Scan(&originalSpec); err != nil {
		t.Fatal(err)
	}
	var changedSpec dag.Node
	if err = json.Unmarshal([]byte(originalSpec), &changedSpec); err != nil {
		t.Fatal(err)
	}
	changedSpec.Name = "Changed after dispatch"
	if _, err = f.s.DB.Exec("UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?", stringify(changedSpec), run.ID, plan.NodeID); err != nil {
		t.Fatal(err)
	}
	_, err = h.w.GetCloudNode(ctx, other, f.project, run.ID, plan.NodeID)
	requireAuthorityStatus(t, err, 409)
	if _, err = f.s.DB.Exec("UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?", originalSpec, run.ID, plan.NodeID); err != nil {
		t.Fatal(err)
	}

	for i, binding := range []struct{ run, relation string }{{newID(), "changed"}, {"", "unbound"}} {
		if _, err = f.s.DB.Exec("UPDATE suma_tasks SET run_id=?,version=version+1 WHERE tenant_id=? AND project_id=? AND id=?", binding.run, f.user.TenantID, f.project, h.task.ID); err != nil {
			t.Fatal(err)
		}
		assertLink(binding.relation, h.task.Version+2+i)
	}

	if _, err = f.s.DB.Exec("UPDATE suma_workbuddy_task_dispatch_plans SET state='ready',version=1 WHERE id=?", plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, present := f.call("GET", path(f.project), nil, 200)["task_delivery"]; present {
		t.Fatal("a preview acquired a formal delivery locator")
	}
	if _, err = f.s.DB.Exec("UPDATE suma_workbuddy_task_dispatch_plans SET state='started',version=2 WHERE id=?", plan.ID); err != nil {
		t.Fatal(err)
	}
	// A task identity must resolve in the original authorized project.
	if _, err = f.s.DB.Exec("UPDATE suma_tasks SET project_id=? WHERE tenant_id=? AND project_id=? AND id=?", foreignProject, f.user.TenantID, f.project, h.task.ID); err != nil {
		t.Fatal(err)
	}
	if _, present := f.call("GET", path(f.project), nil, 200)["task_delivery"]; present {
		t.Fatal("locator exposed a task outside the exact project")
	}
	if _, err = f.s.DB.Exec("DELETE FROM suma_project_members WHERE project_id=? AND user_id=?", f.project, other.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = h.w.GetCloudNode(ctx, other, f.project, run.ID, plan.NodeID)
	if err == nil {
		t.Fatal("revoked teammate retained task locator access")
	}
	for table, count := range counts {
		if domainACount(t, f.s, table) != count {
			t.Fatal("node read changed durable state", table)
		}
	}
	if h.creates.Load() != 0 || h.queries.Load() != 0 {
		t.Fatal("node read contacted provider")
	}
}

func cloudOtherWriter(t *testing.T, h *cloudFixture) Actor {
	t.Helper()
	f := h.f
	original := f.cookie
	inv := governanceInvite(f, "cloud-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	governanceAccept(f, inv, "cloud-reader@example.test")
	actor, err := f.s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = original
	f.call("GET", "/auth/session", nil, 200)
	return actor
}

func TestWorkBuddyCloudNodeReopensExactFactsAndProtectsPrivateDelegation(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	sub, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	read := func(actor Actor) (wc.CloudNodeView, error) {
		return h.w.GetCloudNode(ctx, actor, h.f.project, h.delegation.RunID, h.delegation.NodeID)
	}
	view, err := read(h.f.actor)
	if err != nil || view.Validate() != nil || view.TaskDelivery != nil || view.Submission == nil || view.Submission.ID != sub.ID || view.Delegation == nil || view.Delegation.ID != h.delegation.ID || view.RequestSHA256 != h.delegation.RequestSHA256 || !slices.Contains(view.AllowedActions, "observe") || slices.Contains(view.AllowedActions, "delegate") {
		t.Fatal("cannot reopen original node", err)
	}
	other := cloudOtherWriter(t, h)
	view, err = read(other)
	if err != nil || view.Submission == nil || view.Delegation != nil || len(view.AllowedActions) != 0 || h.queries.Load() != 0 {
		t.Fatal("read leaked private delegation or performed provider GET", err)
	}
	if _, err = h.f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, h.f.project, other.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	view, err = read(other)
	if err == nil || view.Submission != nil || view.Binding.ConnectionID != "" {
		t.Fatal("revoked read returned protected facts")
	}
}

func TestWorkBuddyCloudOwnerObservationSurvivesRetiredMachineWithoutRestoringGrant(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	sub, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	other := cloudOtherWriter(t, h)
	req := wc.CloudObserveRequest{ProjectID: h.f.project, ExpectedVersion: sub.Version, IdempotencyKey: newID()}
	if _, err = h.w.ObserveCloudTask(ctx, other, h.f.project, sub.ID, req); err == nil || h.queries.Load() != 0 {
		t.Fatal("other writer observed private cloud grant")
	}
	if _, err = h.w.RevokeCloudDelegation(ctx, h.f.actor, h.f.project, h.delegation.ID, wc.CloudRevokeRequest{ProjectID: h.f.project, ExpectedVersion: 2, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until='2000-01-01T00:00:00Z' WHERE run_id=? AND id=?`, h.delegation.RunID, h.delegation.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: "project", ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "No new execution", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	observed, err := h.w.ObserveCloudTask(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || observed.Version != sub.Version+1 || observed.Task.Status != "provider_future_state" || h.queries.Load() != 1 {
		t.Fatal("explicit observation blocked by retired machine", err)
	}
	replay, err := h.w.ObserveCloudTask(ctx, h.f.actor, h.f.project, sub.ID, req)
	if err != nil || replay.Version != observed.Version || h.queries.Load() != 1 {
		t.Fatal("observation replay performed GET", err)
	}
	stale := req
	stale.IdempotencyKey = newID()
	if _, err = h.w.ObserveCloudTask(ctx, h.f.actor, h.f.project, sub.ID, stale); err == nil || h.queries.Load() != 1 {
		t.Fatal("stale observation performed GET")
	}
	d, err := h.w.GetCloudDelegation(ctx, h.f.actor, h.f.project, h.delegation.ID)
	if err != nil || d.State != "revoked" || d.CreatesConsumed != 1 {
		t.Fatal("observation revived delegation", err)
	}
	c, err := h.w.Get(ctx, h.f.actor, h.f.project, d.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.Disconnect(ctx, h.f.actor, h.f.project, c.ID, WorkBuddyDisconnectRequest{ProjectID: h.f.project, ExpectedVersion: c.Version, Reason: "Stop grant", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if _, err = h.w.ObserveCloudTask(ctx, h.f.actor, h.f.project, sub.ID, req); err == nil || h.queries.Load() != 1 {
		t.Fatal("disconnected grant replay bypassed current authorization")
	}
}

func TestWorkBuddyCloudNodeAcceptsActualLongNodeIdentity(t *testing.T) {
	h := newCloudFixture(t, nil)
	ctx := context.Background()
	nodeID := strings.Repeat("n", 100)
	n, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, h.delegation.RunID, h.delegation.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := h.f.e.Publish(ctx, h.f.user.TenantID, h.f.project, dag.Workflow{Name: "Long node identity", Nodes: []dag.Node{{ID: nodeID, Type: wc.NodeType, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol, "workbuddy_cloud": n.Binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.f.e.CreateRun(ctx, h.f.user.TenantID, h.f.project, flow.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	view, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, run.ID, nodeID)
	if err != nil || view.NodeID != nodeID || !slices.Contains(view.AllowedActions, "delegate") || h.queries.Load() != 0 {
		t.Fatal("actual schema-valid node cannot be read", err)
	}
}

func TestWorkBuddyCloudNodeHidesDelegateAtSourceVersionLimit(t *testing.T) {
	for _, source := range []string{"connection", "run"} {
		t.Run(source, func(t *testing.T) {
			h := newCloudFixture(t, nil)
			ctx := context.Background()
			original, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, h.delegation.RunID, h.delegation.NodeID)
			if err != nil {
				t.Fatal(err)
			}
			flow, err := h.f.e.Publish(ctx, h.f.user.TenantID, h.f.project, dag.Workflow{Name: "Version limit source", Nodes: []dag.Node{{ID: "cloud-limit", Type: wc.NodeType, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol, "workbuddy_cloud": original.Binding}}}})
			if err != nil {
				t.Fatal(err)
			}
			run, err := h.f.e.CreateRun(ctx, h.f.user.TenantID, h.f.project, flow.ID, newID())
			if err != nil {
				t.Fatal(err)
			}
			before, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, run.ID, "cloud-limit")
			if err != nil || !slices.Contains(before.AllowedActions, "delegate") {
				t.Fatal("fixture not initially actionable", err)
			}
			if source == "connection" {
				_, err = h.f.s.DB.Exec(`UPDATE suma_workbuddy_connections SET version=? WHERE id=?`, wc.MaxVersion, original.Binding.ConnectionID)
			} else {
				_, err = h.f.s.DB.Exec(`UPDATE dag_runs SET version=? WHERE id=?`, wc.MaxVersion, run.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			view, err := h.w.GetCloudNode(ctx, h.f.actor, h.f.project, run.ID, "cloud-limit")
			if err != nil || view.Validate() != nil || view.NodeID != "cloud-limit" || view.RequestSHA256 != before.RequestSHA256 || slices.Contains(view.AllowedActions, "delegate") || h.queries.Load() != 0 || h.creates.Load() != 0 {
				t.Fatal("version-exhausted source still offers impossible delegate", err)
			}
			var version int
			if source == "connection" {
				err = h.f.s.DB.QueryRow(`SELECT version FROM suma_workbuddy_connections WHERE id=?`, original.Binding.ConnectionID).Scan(&version)
			} else {
				err = h.f.s.DB.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, run.ID).Scan(&version)
			}
			if err != nil || version != wc.MaxVersion {
				t.Fatal("read changed exhausted source", err)
			}
		})
	}
}
