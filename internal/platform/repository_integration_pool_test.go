package platform

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// This is a same-organization Control/Core/Pool HTTP authority fixture. The
// provider facts are typed protocol fixtures; the separate provider tests prove
// actual Git effects. Neither cross-organization nor vendor execution is claimed.
func TestRepositoryIntegrationPoolCurrentArmAndGrantedFactRecovery(t *testing.T) {
	for _, afterGrant := range []bool{false, true} {
		name := "revoked_before_arm"
		if afterGrant {
			name = "granted_fact_after_revocation"
		}
		t.Run(name, func(t *testing.T) {
			h := newRepositoryFixture(t)
			f, ctx := h.f, context.Background()
			pool, poolGrant := dispatchAuthorizedPool(t, f)
			dispatchSetRoute(t, f, 0, &poolGrant)
			credential, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Repository Pool protocol fixture", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			claim := func(phase string) dag.Assignment {
				t.Helper()
				req := dag.PoolClaimRequest{PoolID: pool.ID, RunnerID: credential.Runner.ID, WorkerID: credential.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"external_workflow:repo-fixture"}}}}
				reply := f.bearer("POST", "/pool-runner/claim", credential.Token, req, 200)
				var a dag.Assignment
				if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RepositoryIntegration == nil || a.RepositoryIntegration.Phase != phase || a.PoolGrantID != poolGrant.ID || a.PoolRunnerID != credential.Runner.ID || a.PoolCredentialVersion != credential.Runner.CredentialVersion || a.Attempt != 1 {
					t.Fatal("missing exact Pool repository assignment", reply)
				}
				return a
			}
			consume := func(a dag.Assignment) ri.RepositoryAuthorizationGrant {
				t.Helper()
				ref := repositoryTestRef(a)
				req := PoolRepositoryAuthorizationRequest{RepositoryAuthorizationRequest: ri.RepositoryAuthorizationRequest{ProjectID: f.project, Assignment: ref, OperationID: a.RepositoryIntegration.OperationID}, TenantID: f.user.TenantID, RunnerID: credential.Runner.ID}
				forged := req
				forged.Assignment.PoolCredentialVersion++
				f.bearer("POST", "/pool-runner/repository-integrations/authorize", credential.Token, forged, 403)
				reply := f.bearer("POST", "/pool-runner/repository-integrations/authorize", credential.Token, req, 200)
				var ticket ri.RepositoryAuthorization
				if json.Unmarshal([]byte(stringify(reply)), &ticket) != nil || ticket.AuthorizationTicket == "" {
					t.Fatal("Pool did not issue bound authorization", reply)
				}
				var workerHash string
				if err := f.s.DB.QueryRow(`SELECT worker_token_hash FROM suma_repository_authorizations WHERE id=?`, ticket.AuthorizationID).Scan(&workerHash); err != nil || workerHash != digest(credential.Token) {
					t.Fatal("Pool current machine hash not durably pinned", err)
				}
				grant, err := f.s.ConsumeRepositoryAuthorization(ctx, h.secret, ri.RepositoryConsumeAuthorization{ProviderID: "repo-fixture", AuthorizationTicket: ticket.AuthorizationTicket, OperationID: ticket.OperationID, Assignment: ref, RequestSHA256: ticket.RequestSHA256})
				if err != nil || grant.Assignment.PoolGrantID != poolGrant.ID || grant.Assignment.PoolRunnerID != credential.Runner.ID {
					t.Fatal("provider lost complete Pool identity", grant, err)
				}
				return grant
			}
			complete := func(a dag.Assignment, o ri.RepositoryObservation, unknown bool) {
				t.Helper()
				result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), OutcomeUnknown: unknown}
				if unknown {
					result.ExitCode = -1
					result.Error = "provider completion not yet received"
				} else {
					if _, err := f.s.RecordRepositoryObservation(ctx, h.secret, o); err != nil {
						t.Fatal(err)
					}
					result.IntegrationEvidence = &ri.RepositoryResult{OperationID: o.OperationID, RequestSHA256: o.RequestSHA256, ObservationSHA256: o.ObservationSHA256, Version: o.Version}
				}
				body := poolAssignmentBody(a)
				body["result"] = result
				f.bearer("POST", "/pool-runner/results", credential.Token, body, 200)
				f.bearer("POST", "/pool-runner/results", credential.Token, body, 200)
			}
			h.create(t)
			prepare := claim("prepare")
			prepareGrant := consume(prepare)
			complete(prepare, h.observation(t, prepareGrant, "prepared", ""), false)
			if h.current(t).State != "ready" {
				t.Fatal("Pool prepare failed")
			}
			h.command(t, "approve")
			h.command(t, "execute")
			apply := claim("apply")
			grant := consume(apply)
			effectRequest := ri.RepositoryBeginEffect{AuthorizationID: grant.AuthorizationID, OperationID: grant.OperationID, Assignment: grant.Assignment, RequestSHA256: grant.RequestSHA256, Generation: grant.Execution.Generation}
			var effect ri.RepositoryEffectGrant
			if afterGrant {
				effect, err = f.s.BeginRepositoryEffect(ctx, h.secret, effectRequest)
				if err != nil || effect.EffectGrantID == "" {
					t.Fatal("current stored Pool token hash could not arm effect", effect, err)
				}
				complete(apply, ri.RepositoryObservation{}, true)
			}
			poolGrant, err = f.s.CommandOwnedPoolGrant(ctx, f.e, f.actor, pool.ID, poolGrant.ID, ExecutionPoolGrantCommand{Action: "revoke", ExpectedVersion: poolGrant.Version, IdempotencyKey: newID()})
			if err != nil || poolGrant.Status != "revoked" {
				t.Fatal(err)
			}
			_, err = f.s.BeginRepositoryEffect(ctx, h.secret, effectRequest)
			var denied *APIError
			if !errors.As(err, &denied) || denied.Status != 403 {
				t.Fatal("revoked Pool authority could arm effect", err)
			}
			if !afterGrant {
				var effects int
				if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_repository_authorizations WHERE operation_id=? AND effect_grant_id<>''`, grant.OperationID).Scan(&effects); err != nil || effects != 0 {
					t.Fatal("revocation created an effect grant", effects, err)
				}
				return
			}
			// Current execution authority is gone, but an already granted provider effect
			// is still a genuine fact. It cannot be erased or fabricated as a worker ACK.
			observed := h.observation(t, grant, "applied", effect.EffectGrantID)
			for range 2 {
				if _, err = f.s.RecordRepositoryObservation(ctx, h.secret, observed); err != nil {
					t.Fatal("revocation erased authentic granted effect", err)
				}
			}
			h.mock.mu.Lock()
			h.mock.Observations[observed.OperationID] = observed
			h.mock.mu.Unlock()
			var facts int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_repository_observations WHERE operation_id=?`, observed.OperationID).Scan(&facts); err != nil || facts != 1 {
				t.Fatal("callback replay duplicated Pool fact", facts, err)
			}
			if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, apply.RunID, apply.NodeID); err != nil {
				t.Fatal(err)
			}
			plan := h.command(t, "reconcile")
			if plan.State != "integrated" {
				t.Fatal("expired original Pool tuple did not recover provider fact", plan)
			}
			proof, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, apply.RunID, apply.NodeID, apply.Attempt)
			if err != nil || proof.ResultSource != "managed_observation" || proof.Result == nil || proof.Result.IntegrationEvidence == nil || proof.Result.WorkerID != credential.Runner.WorkerID {
				t.Fatal("Pool proof did not retain original machine tuple", proof, err)
			}
		})
	}
}
