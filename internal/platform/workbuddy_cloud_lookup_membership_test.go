package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// The OAuth owner is an invited member, while the unchanged machine credential
// belongs to the organization administrator. Neither Actor is fabricated.
func newCloudLookupMemberFixture(t *testing.T) (*cloudFixture, Actor) {
	t.Helper()
	ctx := context.Background()
	h := &cloudFixture{}
	h.f, h.w = workBuddyCommandFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/v2/tasks":
			h.creates.Add(1)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"task_id":"member-cloud-task","status":"working"}`)
		case r.URL.Path == "/openapi/v2/token":
			workBuddyCommandResponse(w, "member-cloud-account", "user.task.readable user.task.invokable", "private-refresh")
		default:
			h.queries.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err := h.f.s.InitializeWorkBuddyCloud(ctx); err != nil {
		t.Fatal(err)
	}
	admin := h.f.actor
	token := h.f.call("POST", "/auth/tokens", h.f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	h.actor = domainCActor(t, h.f.s, token)
	invitation := governanceInvite(h.f, "cloud-lookup-member@example.test", "member", []any{map[string]any{"project_id": h.f.project, "role": "developer"}})
	governanceAccept(h.f, invitation, "cloud-lookup-member@example.test")
	member, err := h.f.s.AuthenticateActor(ctx, h.f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	h.f.actor, h.f.user = member, member.Projection()
	if h.f.user.ID == admin.Projection().ID || h.actor.Projection().ID != admin.Projection().ID || h.f.user.Role != "member" {
		t.Fatal("fixture did not establish independent real subjects")
	}
	authorization, _ := workBuddyCommandStart(t, h.f, h.w)
	connection, err := h.w.CompleteAuthorization(ctx, member, h.f.project, authorization.ID, workBuddyCommandComplete(authorization))
	if err != nil {
		t.Fatal(err)
	}
	binding := wc.NodeBinding{ConnectionID: connection.ID, GrantGeneration: connection.GrantGeneration, Request: workbuddy.CreateRequest{Prompt: "fixed member-owned task"}}
	flow, err := h.f.e.Publish(ctx, h.f.user.TenantID, h.f.project, dag.Workflow{Name: "Member cloud lookup", Nodes: []dag.Node{{ID: "cloud", Type: wc.NodeType, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol, "workbuddy_cloud": binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.f.e.CreateRun(ctx, h.f.user.TenantID, h.f.project, flow.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	node, err := h.w.GetCloudNode(ctx, member, h.f.project, run.ID, "cloud")
	if err != nil {
		t.Fatal(err)
	}
	h.delegation, err = h.w.DelegateCloud(ctx, member, h.f.project, wc.CloudDelegateRequest{ProjectID: h.f.project, ConnectionID: connection.ID, ExpectedConnectionVersion: connection.Version, GrantGeneration: connection.GrantGeneration, RunID: run.ID, NodeID: "cloud", ExpectedRunVersion: node.RunVersion, NodeSpecSHA256: node.NodeSpecSHA256, RequestSHA256: node.RequestSHA256, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339), MaxCreates: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	worker := "member-cloud-worker"
	authority := h.f.s.engineAuthority(ctx, h.actor, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project, resourceID: worker}, purposeRunnerWrite)
	cloud := h.f.s.newCloudClaimAuthority(h.w)
	operation, capabilities := newID(), []string{"external_workflow:workbuddy-cloud"}
	assignment, err := h.f.e.ClaimWithOperation(authority.ctx, h.f.user.TenantID, h.f.project, worker, operation, capabilities, func(tx *sql.Tx) error {
		return cloud.beforePrivate(authority, tx, worker, operation, capabilities)
	}, func(tx *sql.Tx, r, n string) (bool, error) {
		_, allowed, e := cloud.eligibleTx(authority.ctx, tx, h.f.user.TenantID, h.f.project, r, n)
		return allowed, e
	}, cloud.runnableHook())
	authority.close()
	if err != nil || assignment == nil {
		t.Fatal("exact authorized fixture claim failed", err)
	}
	h.request = wc.CloudCreateRequest{ProjectID: h.f.project, DelegationID: h.delegation.ID, ExpectedVersion: h.delegation.Version, Assignment: repositoryTestRef(*assignment), IdempotencyKey: newID()}
	return h, admin
}

func TestWorkBuddyCloudLookupRechecksIndependentDelegationOwnerMembership(t *testing.T) {
	h, admin := newCloudLookupMemberFixture(t)
	ctx := context.Background()
	created, err := h.w.CreateCloudTask(ctx, h.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal("baseline create", err)
	}
	req := cloudLookupRequest(h)
	read := func(t *testing.T) {
		t.Helper()
		out, e := h.w.LookupCloudSubmission(ctx, h.actor, h.f.project, req)
		if e != nil || out.State != "task_known" || out.Submission == nil || out.Submission.ID != created.ID {
			t.Fatal("original stored fact unavailable", e)
		}
	}
	read(t)
	setRole := func(t *testing.T, role string) {
		t.Helper()
		var version int
		if e := h.f.s.DB.QueryRowContext(ctx, `SELECT version FROM suma_project_grant_versions WHERE tenant_id=? AND project_id=? AND user_id=?`, h.f.user.TenantID, h.f.project, h.f.user.ID).Scan(&version); e != nil {
			t.Fatal(e)
		}
		input := map[string]any{"project_id": h.f.project, "user_id": h.f.user.ID, "expected_version": float64(version), "idempotency_key": newID(), "role": role}
		if role == "" {
			input["action"] = "remove"
		}
		if _, e := h.f.s.SaveProjectAccess(ctx, admin, input); e != nil {
			t.Fatal("formal grant change", e)
		}
	}
	for _, role := range []string{"viewer", ""} {
		t.Run(map[string]string{"viewer": "downgrade", "": "remove"}[role], func(t *testing.T) {
			setRole(t, role)
			// Check the machine's own current project proof, separately from the
			// delegate owner check, so a revoked machine cannot make this pass.
			bounded, tx, done, e := h.f.s.beginAuthorityTx(ctx)
			if e != nil {
				t.Fatal(e)
			}
			proof, e := h.f.s.runnerIdentityTx(bounded, tx, h.actor, h.f.project, h.request.Assignment.WorkerID)
			if e == nil {
				e = h.f.s.projectRunnerExecutionAuthorityTx(bounded, tx, proof, h.request.Assignment.WorkerID, h.request.Assignment.RunID)
			}
			_ = tx.Rollback()
			done()
			if e != nil {
				t.Fatal("independent machine lost authority", e)
			}
			before := cloudLookupStoredFingerprint(t, h)
			out, e := h.w.LookupCloudSubmission(ctx, h.actor, h.f.project, req)
			var api *APIError
			if !errors.As(e, &api) || api.Status != 403 || out.Submission != nil {
				t.Fatal("changed owner permission did not refuse lookup", e)
			}
			if cloudLookupStoredFingerprint(t, h) != before || h.creates.Load() != 1 || h.queries.Load() != 0 {
				t.Fatal("denied lookup altered original records or contacted provider")
			}
			setRole(t, "developer")
			before = cloudLookupStoredFingerprint(t, h)
			read(t)
			if cloudLookupStoredFingerprint(t, h) != before {
				t.Fatal("restored read mutated facts")
			}
			delegation, e := h.w.GetCloudDelegation(ctx, h.f.actor, h.f.project, h.delegation.ID)
			if e != nil {
				t.Fatal(e)
			}
			newCreate := h.request
			newCreate.ExpectedVersion = delegation.Version
			newCreate.IdempotencyKey = newID()
			if _, e = h.w.CreateCloudTask(ctx, h.actor, h.f.project, newCreate); e == nil {
				t.Fatal("restored membership reset consumed create allowance")
			}
			if h.creates.Load() != 1 || h.queries.Load() != 0 {
				t.Fatal("membership recovery produced provider effects")
			}
		})
	}
}
