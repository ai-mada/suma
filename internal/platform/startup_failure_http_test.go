package platform

import (
	"context"
	"encoding/json"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type startupHTTPFixture struct {
	f          *fixture
	token      string
	assignment dag.Assignment
	result     dag.Result
	pooled     bool
}

func newStartupHTTPFixture(t *testing.T, pooled bool) startupHTTPFixture {
	t.Helper()
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	var request CandidateCompositionRepairCreateRequest
	var profile ToolProfile
	var token string
	var machine ExecutionPoolCredentialResponse
	if !pooled {
		request, profile, token = compositionRepairRequestFixture(t, h, original)
	} else {
		pool, grant := dispatchAuthorizedPool(t, f)
		dispatchSetRoute(t, f, 0, &grant)
		var err error
		machine, err = f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "HTTP startup-proof Pool", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		token = machine.Token
		fixed := toolTestProfile()
		fixed.WorkspaceToolsVersion, fixed.ConflictRepairProtocol = nwc.Version, ccc.CandidateConflictRepairProtocol
		f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: machine.Runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: fixed, Available: true}}}, 200)
		profile, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.Runner.WorkerID, Name: "HTTP fixed startup-proof native", Profile: fixed, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, original.ID)
		if err != nil {
			t.Fatal(err)
		}
		request = CandidateCompositionRepairCreateRequest{ProjectID: f.project, ExpectedCompositionVersion: original.Version, ExpectedConflictSHA256: view.Basis.SHA256, Repair: CandidateCompositionRepairDraft{Title: "HTTP protected repair", Description: "Preserve exact source and original conflict facts.", AcceptanceCriteria: []string{"Independent exact-candidate tests and Review"}}, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, ProducerNodeID: h.req.ProducerNodeID, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, IdempotencyKey: newID()}
	}
	f.e.AttemptAccounting = f.s
	plan, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, plan.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Acknowledge: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var reply map[string]any
	if pooled {
		reply = f.bearer("POST", "/pool-runner/claim", token, dag.PoolClaimRequest{PoolID: machine.Runner.PoolID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.Profile.CommandID}}}}, 200)
	} else {
		reply = f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	}
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.RunID != plan.RunID || assignment.CandidateConflictRepair == nil {
		t.Fatal("HTTP claim lost fixed repair execution", reply, err)
	}
	proof := &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: repositoryTestRef(assignment), Profile: profile.Profile, Purpose: "candidate_conflict_repair"}
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), ExitCode: -1, Error: "Owned missing workspace root", StartupFailure: proof}
	return startupHTTPFixture{f: f, token: token, assignment: assignment, result: result, pooled: pooled}
}

func (h startupHTTPFixture) ledger(t *testing.T) string {
	t.Helper()
	var payload string
	if err := h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE run_id=?`, h.result.RunID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	rows, err := h.f.s.DB.Query(`SELECT scope,project_id,period,resource,version,spent,reserved,unresolved FROM suma_usage_accounts ORDER BY scope,project_id,period,resource`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	accounts := []any{}
	for rows.Next() {
		var scope, project, period, resource, spent, reserved string
		var version, unresolved int
		if err = rows.Scan(&scope, &project, &period, &resource, &version, &spent, &reserved, &unresolved); err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, []any{scope, project, period, resource, version, spent, reserved, unresolved})
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	var facts int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id IN(SELECT id FROM suma_usage_attempts WHERE run_id=?)`, h.result.RunID).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	return stringify(map[string]any{"attempt": json.RawMessage(payload), "accounts": accounts, "facts": facts})
}

func (h startupHTTPFixture) post(result map[string]any, want int) map[string]any {
	if !h.pooled {
		return h.f.bearer("POST", "/runner/results", h.token, map[string]any{"project_id": h.f.project, "result": result}, want)
	}
	return h.f.bearer("POST", "/pool-runner/results", h.token, map[string]any{"runner_id": h.assignment.PoolRunnerID, "tenant_id": h.assignment.TenantID, "project_id": h.assignment.ProjectID, "assignment": h.assignment, "result": result}, want)
}

func TestStartupFailureHTTPPrivatePoolRejectInvalidThenSettleOriginalOnce(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "Pool"}[pooled], func(t *testing.T) {
			h := newStartupHTTPFixture(t, pooled)
			before := h.ledger(t)
			object := func() map[string]any {
				var value map[string]any
				if err := json.Unmarshal([]byte(stringify(h.result)), &value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			missing := object()
			delete(missing["startup_failure"].(map[string]any), "workspace_started")
			h.post(missing, 400)
			if got := h.ledger(t); got != before {
				t.Fatal("missing host assertion settled accounting")
			}
			forged := object()
			proof := forged["startup_failure"].(map[string]any)
			assignment := proof["assignment"].(map[string]any)
			assignment["fence"] = assignment["fence"].(float64) + 1
			h.post(forged, 400)
			if got := h.ledger(t); got != before {
				t.Fatal("forged host assignment settled accounting")
			}
			stale := object()
			stale["fence"] = stale["fence"].(float64) + 1
			want := 409
			if pooled {
				want = 400
			}
			h.post(stale, want)
			if got := h.ledger(t); got != before {
				t.Fatal("stale callback settled accounting")
			}
			valid := object()
			if response := h.post(valid, 200); response["acknowledged"] != true {
				t.Fatal(response)
			}
			after := h.ledger(t)
			if after == before {
				t.Fatal("valid host proof did not settle its original attempt")
			}
			var raw string
			if err := h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE run_id=?`, h.result.RunID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var record usageRecord
			if json.Unmarshal([]byte(raw), &record) != nil || record.Unresolved || record.Attempt.UsageSource != "host_startup_proof" || record.Attempt.ReportedTokens == nil || *record.Attempt.ReportedTokens != 0 || record.Attempt.EstimatedCostMicros != nil {
				t.Fatal("HTTP settlement invented unknown tokens/price", raw)
			}
			if response := h.post(valid, 200); response["acknowledged"] != true {
				t.Fatal(response)
			}
			if got := h.ledger(t); got != after {
				t.Fatal("original result replay settled accounting twice")
			}
		})
	}
}
