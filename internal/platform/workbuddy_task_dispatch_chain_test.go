package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyTaskDispatchFormalPrivateAndPoolClaimKeepsCompletedWaiting(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		name := "private"
		if pooled {
			name = "pool"
		}
		t.Run(name, func(t *testing.T) {
			h := newWorkBuddyTaskDispatchFixture(t)
			f := h.f
			ctx := context.Background()
			cloud := &cloudFixture{f: f, w: h.w}
			if pooled {
				pool, grant := dispatchAuthorizedPool(t, f)
				dispatchSetRoute(t, f, 0, &grant)
				issued, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Task cloud runner", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
				cloud.poolToken = issued.Token
				cloud.poolClaim = dag.PoolClaimRequest{PoolID: pool.ID, RunnerID: issued.Runner.ID, WorkerID: issued.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"external_workflow:workbuddy-cloud"}}}}
			} else {
				cloud.claimToken = f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "cloud-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
				var err error
				cloud.actor, err = f.s.AuthenticateActor(ctx, cloud.claimToken)
				if err != nil {
					t.Fatal(err)
				}
			}
			plan := workBuddyTaskPreview(t, h)
			startKey := newID()
			run := workBuddyTaskStart(t, h, plan, startKey)
			view, err := h.w.GetCloudNode(ctx, f.actor, f.project, run.ID, plan.NodeID)
			if err != nil || view.Delegation == nil {
				t.Fatal("missing original delegation", err)
			}
			cloud.delegation = *view.Delegation
			a := cloudHTTPClaim(t, cloud, pooled, newID(), 200)
			if a == nil || a.RunID != run.ID || a.NodeID != plan.NodeID || a.WorkBuddyCloud == nil || a.WorkBuddyCloud.DelegationID != plan.DelegationID {
				t.Fatal("formal claim missed task binding")
			}
			cloud.request = wc.CloudCreateRequest{ProjectID: f.project, DelegationID: plan.DelegationID, ExpectedVersion: 1, Assignment: repositoryTestRef(*a), IdempotencyKey: newID()}
			sub := createCloudFixture(t, cloud)
			if sub.Task == nil || sub.Task.Status != "completed" || h.creates.Load() != 1 {
				t.Fatal("fixed supplier create missing")
			}
			assignment, result := cloudResultFixture(t, cloud)
			result.WorkBuddyCloudWait.SubmissionID, result.WorkBuddyCloudWait.SubmissionVersion = sub.ID, sub.Version
			if err = completeCloudFixture(t, cloud, assignment, result); err != nil {
				t.Fatal(err)
			}
			current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
			if err != nil || current.State != "running" || current.Nodes[0].State != "waiting" || current.Nodes[0].Reason != "cloud.delivery_required" || current.Nodes[0].Attempt != 1 {
				t.Fatal("supplier completed became platform success", current, err)
			}
			replay := workBuddyTaskStart(t, h, plan, startKey)
			if replay.ID != run.ID || h.creates.Load() != 1 || domainACount(t, f.s, "suma_workbuddy_cloud_delegations") != 1 || domainACount(t, f.s, "dag_attempts") != 1 {
				t.Fatal("consumed start replay created another execution")
			}
		})
	}
}
