package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudResultFixture(t *testing.T, h *cloudFixture) (dag.Assignment, dag.Result) {
	t.Helper()
	tx, err := h.f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := dag.ReadExternalAssignmentTx(context.Background(), tx, h.f.s.Dialect, h.f.user.TenantID, h.f.project, h.request.Assignment.RunID, h.request.Assignment.NodeID, h.request.Assignment.Attempt)
	tx.Rollback()
	if err != nil {
		t.Fatal(err)
	}
	query := "SELECT assignment FROM dag_dispatches WHERE id=?"
	if h.poolToken != "" {
		query = "SELECT assignment FROM dag_execution_pool_dispatches WHERE dispatch_id=?"
	}
	var raw string
	if err = h.f.s.DB.QueryRow(query, snapshot.DispatchID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var a dag.Assignment
	if err = json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatal(err)
	}
	r := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: -1, OutcomeUnknown: true, OperationID: newID(), WorkBuddyCloudWait: &wc.CloudWaitEvidence{Protocol: wc.Protocol, ClaimID: a.DispatchID, Assignment: h.request.Assignment, Binding: *a.WorkBuddyCloud, CreateOperationID: h.request.IdempotencyKey, Cause: "delivery_required"}}
	return a, r
}

func completeCloudFixture(t *testing.T, h *cloudFixture, a dag.Assignment, r dag.Result) error {
	t.Helper()
	ctx := context.Background()
	if h.poolToken != "" {
		var proof *poolMachineProof
		return h.f.e.CompleteBound(ctx, h.f.user.TenantID, h.f.project, r, func(tx *sql.Tx) error {
			var err error
			proof, err = h.f.s.PoolRunnerAssignmentAuthorityTx(ctx, tx, h.poolToken, a)
			return err
		}, func(tx *sql.Tx, _ string) error { return h.f.s.ValidatePoolToolResultTx(ctx, tx, proof, a, r) }, h.w.cloudResultResolver(candidateSubject{pool: true, token: h.poolToken, tenant: h.f.user.TenantID, project: h.f.project}))
	}
	authority := h.f.s.engineAuthority(ctx, h.actor, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project, resourceID: a.WorkerID}, purposeRunnerWrite)
	defer authority.close()
	return h.f.e.CompleteBound(authority.ctx, h.f.user.TenantID, h.f.project, r, func(tx *sql.Tx) error {
		if err := authority.beforeRunner(tx, a.WorkerID); err != nil {
			return err
		}
		return h.f.s.projectRunnerExecutionAuthorityTx(authority.ctx, tx, authority.proof, a.WorkerID, a.RunID)
	}, func(tx *sql.Tx, _ string) error {
		return h.f.s.validateToolResultTx(authority.ctx, tx, authority.proof, r)
	}, h.w.cloudResultResolver(projectCandidateSubject(h.actor, h.f.project)))
}

func createCloudFixture(t *testing.T, h *cloudFixture) wc.CloudSubmission {
	t.Helper()
	var sub wc.CloudSubmission
	var err error
	if h.poolToken != "" {
		sub, err = h.w.CreatePoolCloudTask(context.Background(), h.poolToken, h.request)
	} else {
		sub, err = h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request)
	}
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestCloudResultUsesStoredEffectNotProviderSuccess(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint("pool=", pooled), func(t *testing.T) {
			h := newCloudFixtureMode(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(201)
				fmt.Fprint(w, `{"task_id":"cloud-task","status":"completed"}`)
			}, pooled)
			a, r := cloudResultFixture(t, h)
			sub := createCloudFixture(t, h)
			r.WorkBuddyCloudWait.SubmissionID, r.WorkBuddyCloudWait.SubmissionVersion = sub.ID, sub.Version
			if err := completeCloudFixture(t, h, a, r); err != nil {
				t.Fatal(err)
			}
			if err := completeCloudFixture(t, h, a, r); err != nil {
				t.Fatal("original result ACK replay", err)
			}
			current, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, a.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Nodes[0].State != "waiting" || current.Nodes[0].Reason != "cloud.delivery_required" || current.Nodes[0].CompletedAt != "" || h.creates.Load() != 1 {
				t.Fatal("provider completion became platform success", current.Nodes[0])
			}
		})
	}
}

func TestCloudResultPreservesFactAfterDelegationRevocation(t *testing.T) {
	h := newCloudFixture(t, nil)
	a, r := cloudResultFixture(t, h)
	createCloudFixture(t, h)
	if _, err := h.w.RevokeCloudDelegation(context.Background(), h.f.actor, h.f.project, h.delegation.ID, wc.CloudRevokeRequest{ProjectID: h.f.project, ExpectedVersion: h.delegation.Version + 1, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if err := completeCloudFixture(t, h, a, r); err != nil {
		t.Fatal("revocation erased already incurred effect", err)
	}
}

func TestCloudResultRejectsReplacementCreateOperation(t *testing.T) {
	h := newCloudFixture(t, nil)
	a, r := cloudResultFixture(t, h)
	sub := createCloudFixture(t, h)
	for _, change := range []func(*wc.CloudWaitEvidence){func(e *wc.CloudWaitEvidence) { e.CreateOperationID = "another-create-operation" }, func(e *wc.CloudWaitEvidence) { e.SubmissionID = "another-submission"; e.SubmissionVersion = 1 }, func(e *wc.CloudWaitEvidence) { e.SubmissionID = sub.ID; e.SubmissionVersion = sub.Version + 1 }} {
		bad := r
		proof := *r.WorkBuddyCloudWait
		bad.WorkBuddyCloudWait = &proof
		change(&proof)
		if err := completeCloudFixture(t, h, a, bad); err == nil {
			t.Fatal("replacement original operation accepted")
		}
	}
	if h.creates.Load() != 1 {
		t.Fatal("reconciliation created another provider task")
	}
}

func TestCloudResultNoCreateRequiresUnconsumedAndAbsentSubmission(t *testing.T) {
	t.Run("not_created", func(t *testing.T) {
		h := newCloudFixture(t, nil)
		a, r := cloudResultFixture(t, h)
		if err := completeCloudFixture(t, h, a, r); err != nil {
			t.Fatal(err)
		}
		current, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, a.RunID)
		if err != nil || current.Nodes[0].State != "failed" || current.Nodes[0].Reason != "cloud.not_created" || h.creates.Load() != 0 {
			t.Fatal(current, err)
		}
	})
	t.Run("consumed_without_record", func(t *testing.T) {
		h := newCloudFixture(t, nil)
		a, r := cloudResultFixture(t, h)
		createCloudFixture(t, h)
		if _, err := h.f.s.DB.Exec(`DELETE FROM suma_workbuddy_cloud_submissions`); err != nil {
			t.Fatal(err)
		}
		if err := completeCloudFixture(t, h, a, r); err == nil {
			t.Fatal("corrupt consumed grant treated as no effect")
		}
	})
}
