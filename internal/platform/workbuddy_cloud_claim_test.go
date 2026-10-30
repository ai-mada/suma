package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudHTTPClaim(t *testing.T, h *cloudFixture, pooled bool, operation string, want int) *dag.Assignment {
	t.Helper()
	h.f.handler = (&Server{Service: h.f.s, Engine: h.f.e, WorkBuddy: h.w}).Handler()
	var reply map[string]any
	if pooled {
		req := h.poolClaim
		req.OperationID = operation
		reply = h.f.bearer("POST", "/pool-runner/claim", h.poolToken, req, want)
	} else {
		token := h.claimToken
		reply = h.f.bearer("POST", "/runner/claim", token, RunnerClaimRequest{ProjectID: h.f.project, WorkerID: "cloud-worker", Capabilities: []string{"external_workflow:workbuddy-cloud"}, IdempotencyKey: operation}, want)
	}
	if reply["assignment"] == nil {
		return nil
	}
	raw, _ := json.Marshal(reply["assignment"])
	var a dag.Assignment
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	return &a
}

func TestWorkBuddyCloudClaimHTTPPrivateAndPoolFrozenConsumedReplay(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		name := "private"
		if pooled {
			name = "pool"
		}
		t.Run(name, func(t *testing.T) {
			h := newCloudFixtureMode(t, nil, pooled, true)
			op := newID()
			var before, after int
			if err := h.f.s.DB.QueryRow(`SELECT sequence FROM suma_identity_authority_locks WHERE user_id=?`, h.delegation.OwnerID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			a := cloudHTTPClaim(t, h, pooled, op, 200)
			if a == nil || a.WorkBuddyCloud == nil || a.WorkBuddyCloud.DelegationID != h.delegation.ID || a.WorkBuddyCloud.DelegationVersion != 1 {
				t.Fatal("formal HTTP claim omitted fixed grant")
			}
			if err := h.f.s.DB.QueryRow(`SELECT sequence FROM suma_identity_authority_locks WHERE user_id=?`, h.delegation.OwnerID).Scan(&after); err != nil || after <= before || pooled && after != before+1 {
				t.Fatal("claim did not coordinate original owner", before, after, err)
			}
			if pooled {
				for _, strip := range []bool{true, false} {
					bounded, tx, done, err := h.f.s.beginAuthorityTx(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					bad := *a
					if strip {
						bad.WorkBuddyCloud = nil
					} else {
						b := *a.WorkBuddyCloud
						b.DelegationVersion++
						bad.WorkBuddyCloud = &b
					}
					_, err = h.f.s.PoolRunnerAssignmentAuthorityTx(bounded, tx, h.poolToken, bad)
					tx.Rollback()
					done()
					if err == nil {
						t.Fatal("full machine envelope omitted or replaced protected binding")
					}
				}
			}
			h.request.Assignment = repositoryTestRef(*a)
			if pooled {
				if _, err := h.w.CreatePoolCloudTask(context.Background(), h.poolToken, h.request); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := h.w.CreateCloudTask(context.Background(), h.actor, h.f.project, h.request); err != nil {
					t.Fatal(err)
				}
			}
			// A current lease observation can move without changing the original grant.
			if _, err := h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=lease_until+1 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); err != nil {
				t.Fatal(err)
			}
			replay := cloudHTTPClaim(t, h, pooled, op, 200)
			if replay == nil || replay.DispatchID != a.DispatchID || *replay.WorkBuddyCloud != *a.WorkBuddyCloud || h.creates.Load() != 1 {
				t.Fatal("consumed original claim changed binding or resent")
			}
			var attempts, snapshots int
			h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts`).Scan(&attempts)
			h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_external_assignments`).Scan(&snapshots)
			if attempts != 1 || snapshots != 1 {
				t.Fatal("replay created another execution", attempts, snapshots)
			}
		})
	}
}

func TestWorkBuddyCloudClaimCurrentAdmissionBeforeAttempt(t *testing.T) {
	for _, kind := range []string{"missing_delegation", "owner_removed", "organization_enforce", "project_enforce", "max_version"} {
		t.Run(kind, func(t *testing.T) {
			h := newCloudFixtureMode(t, nil, false, true)
			want := 200
			switch kind {
			case "missing_delegation":
				if _, err := h.f.s.DB.Exec(`DELETE FROM suma_workbuddy_cloud_delegations`); err != nil {
					t.Fatal(err)
				}
			case "owner_removed":
				// Issue the machine before removal; keep its original bytes in the helper.
				want = 403
				if _, err := h.f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.delegation.OwnerID); err != nil {
					t.Fatal(err)
				}
			case "max_version":
				d := h.delegation
				d.Version = wc.MaxVersion
				if d.Validate() != nil {
					t.Fatal("invalid saturation fixture")
				}
				if _, err := h.f.s.DB.Exec(`UPDATE suma_workbuddy_cloud_delegations SET version=?,payload=? WHERE id=?`, d.Version, stringify(d), d.ID); err != nil {
					t.Fatal(err)
				}
			default:
				scope := "project"
				if kind == "organization_enforce" {
					scope = "organization"
				}
				_, err := h.f.s.WriteUsagePolicy(context.Background(), h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: scope, ExpectedVersion: 0, Mode: "enforce", Currency: "USD", AcknowledgeEnforcement: true, Reason: "Explicit cloud admission policy", IdempotencyKey: newID()})
				if err != nil {
					t.Fatal(err)
				}
				want = 409
			}
			if a := cloudHTTPClaim(t, h, false, newID(), want); a != nil {
				t.Fatal("ineligible cloud node dispatched")
			}
			var attempts, snapshots int
			h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts`).Scan(&attempts)
			h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_external_assignments`).Scan(&snapshots)
			if attempts != 0 || snapshots != 0 || h.creates.Load() != 0 {
				t.Fatal("rejection consumed execution or provider", attempts, snapshots, h.creates.Load())
			}
		})
	}
}

func TestWorkBuddyCloudClaimUndiscoveredOwnerRollsBack(t *testing.T) {
	h := newCloudFixtureMode(t, nil, false, true)
	cloud := h.f.s.newCloudClaimAuthority(h.w)
	authority := h.f.s.engineAuthority(context.Background(), h.actor, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project}, purposeRunnerWrite)
	defer authority.close()
	operation := newID()
	caps := []string{"external_workflow:workbuddy-cloud"}
	_, err := h.f.e.ClaimWithOperation(authority.ctx, h.f.user.TenantID, h.f.project, "cloud-worker", operation, caps, func(tx *sql.Tx) error {
		var before, after int
		if e := tx.QueryRow(`SELECT sequence FROM suma_identity_authority_locks WHERE user_id=?`, h.delegation.OwnerID).Scan(&before); e != nil {
			return e
		}
		if e := cloud.beforePrivate(authority, tx, "cloud-worker", operation, caps); e != nil {
			return e
		}
		if e := tx.QueryRow(`SELECT sequence FROM suma_identity_authority_locks WHERE user_id=?`, h.delegation.OwnerID).Scan(&after); e != nil {
			return e
		}
		if after != before+1 {
			t.Errorf("claim transaction coordinated owner more than once: %d -> %d", before, after)
		}
		delete(cloud.owners, h.delegation.OwnerID)
		return nil
	}, func(tx *sql.Tx, r, n string) (bool, error) {
		_, ok, e := cloud.eligibleTx(authority.ctx, tx, h.f.user.TenantID, h.f.project, r, n)
		return ok, e
	}, cloud.runnableHook())
	var api *APIError
	if !errors.As(err, &api) || api.Code != "authority_retry" {
		t.Fatal("new owner did not force bounded authority retry", err)
	}
	var attempt, dispatch int
	h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, h.delegation.RunID, h.delegation.NodeID).Scan(&attempt)
	h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_dispatches`).Scan(&dispatch)
	if attempt != 0 || dispatch != 0 || h.creates.Load() != 0 {
		t.Fatal("authority retry committed execution", attempt, dispatch)
	}
}
