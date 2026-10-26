package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func newUsageFixture(t *testing.T) *intelligentFixture {
	t.Helper()
	h := newIntelligentFixture(t)
	if e := h.f.s.MigrateUsageAccounting(context.Background()); e != nil {
		t.Fatal(e)
	}
	h.f.e.AttemptAccounting = h.f.s
	return h
}
func usageRateForTest(t *testing.T, h *intelligentFixture) UsageRateRevision {
	t.Helper()
	r, e := h.f.s.WriteUsageRate(context.Background(), h.f.actor, h.f.project, UsageRateWriteRequest{ProjectID: h.f.project, ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version, Currency: "CNY", InputRateMicrosPerMillion: 1000000, OutputRateMicrosPerMillion: 2000000, InputTokenReservation: 100, OutputTokenReservation: 4096, Reason: "Operator configured estimate; no vendor billing", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func usagePolicyForTest(t *testing.T, h *intelligentFixture, scope string, tokens, cost *int64) UsagePolicy {
	t.Helper()
	p, e := h.f.s.WriteUsagePolicy(context.Background(), h.f.actor, h.f.project, UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: scope, Mode: "enforce", Currency: "CNY", TokenLimit: tokens, CostLimitMicros: cost, Reason: "Explicit fixture allowance", AcknowledgeEnforcement: true, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func usageAttemptForTest(t *testing.T, h *intelligentFixture, id string) UsageAttempt {
	t.Helper()
	page, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "", 100, "")
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range page.Items {
		if a.RunID == id {
			return a
		}
	}
	t.Fatal("no accounting attempt", id)
	return UsageAttempt{}
}
func usageMeasuredResult(h *intelligentFixture, a dag.Assignment, input, output int64) dag.Result {
	r := h.result(a, h.output())
	r.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(input), OutputTokens: usageInt(output)}
	r.ToolEvidence.UsageState = toolcontract.UsageComplete
	return r
}
func usageBalanceForTest(t *testing.T, h *intelligentFixture) UsageBalance {
	t.Helper()
	d, e := h.f.s.GetUsageDashboard(context.Background(), h.f.actor, h.f.project, "")
	if e != nil {
		t.Fatal(e)
	}
	return d.Balances[0]
}

func TestUsageAccountingExactMoneyAndSafeOverflow(t *testing.T) {
	rate := UsageRateRevision{InputRateMicrosPerMillion: 1, OutputRateMicrosPerMillion: 3}
	for _, tc := range []struct {
		input, output int64
		want          string
	}{{0, 0, "0"}, {1, 0, "1"}, {999999, 0, "1"}, {1000000, 1, "2"}, {1000000, 1000000, "4"}} {
		if got := usageCost(tc.input, tc.output, rate).String(); got != tc.want {
			t.Fatalf("%+v cost=%s", tc, got)
		}
	}
	rate.InputRateMicrosPerMillion = usageSafe
	rate.OutputRateMicrosPerMillion = usageSafe
	actual := usageCost(usageSafe, usageSafe, rate)
	if actual.Cmp(big.NewInt(usageSafe)) <= 0 || usagePublic(actual) != nil {
		t.Fatal("large exact cost truncated", actual)
	}
	if usagePublic(big.NewInt(0)) == nil || *usagePublic(big.NewInt(0)) != 0 {
		t.Fatal("measured zero became unknown")
	}
}

func TestUsageAccountingPolicyRateCurrentAuthorityCASAndReplay(t *testing.T) {
	h := newUsageFixture(t)
	ctx := context.Background()
	rate := usageRateForTest(t, h)
	req := UsagePolicyWriteRequest{ProjectID: h.f.project, Scope: "project", Mode: "enforce", Currency: "CNY", TokenLimit: usageInt(0), CostLimitMicros: nil, Reason: "Zero is a strict allowance", AcknowledgeEnforcement: true, IdempotencyKey: newID()}
	policy, e := h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, req)
	if e != nil || policy.TokenLimit == nil || *policy.TokenLimit != 0 {
		t.Fatal(policy, e)
	}
	replay, e := h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, req)
	if e != nil || replay.Version != policy.Version {
		t.Fatal(replay, e)
	}
	req.IdempotencyKey = newID()
	if _, e = h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, req); e == nil {
		t.Fatal("stale policy CAS")
	}
	h.create(t)
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if reply["assignment"] != nil {
		t.Fatal("zero allowance admitted")
	}
	var attempts int
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_attempts`).Scan(&attempts); e != nil || attempts != 0 {
		t.Fatal("denied claim reserved money", attempts, e)
	}
	actor := toolActorForTest(t, h.f, h.token)
	if _, e = h.f.s.GetUsageDashboard(ctx, actor, h.f.project, ""); e == nil {
		t.Fatal("runner read management balances")
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE tenant_id=? AND project_id=? AND worker_id=?`, h.f.user.TenantID, h.f.project, h.profile.WorkerID); e != nil {
		t.Fatal(e)
	}
	originalRate := rate
	rateRequest := UsageRateWriteRequest{ProjectID: h.f.project, ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version + 1, ExpectedVersion: rate.Version, Currency: "CNY", InputRateMicrosPerMillion: 3000000, OutputRateMicrosPerMillion: 4000000, InputTokenReservation: 100, OutputTokenReservation: 4096, Reason: "Offline pricing configuration without dispatch", IdempotencyKey: newID()}
	_, e = h.f.s.WriteUsageRate(ctx, h.f.actor, h.f.project, rateRequest)
	releaseWantStatus(t, e, 409)
	rateRequest.ExpectedToolVersion = h.profile.Version
	rateRequest.IdempotencyKey = newID()
	rate, e = h.f.s.WriteUsageRate(ctx, h.f.actor, h.f.project, rateRequest)
	if e != nil || rate.Version != 2 || rate.ProfileSHA256 != h.profile.Profile.ProfileSHA256 || rate.Model != originalRate.Model || rate.EndpointOrigin != originalRate.EndpointOrigin {
		t.Fatal("offline rate update incorrectly required live inventory", rate, e)
	}
	var originalRaw string
	if e = h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_rates WHERE id=?`, originalRate.ID).Scan(&originalRaw); e != nil || originalRaw != stringify(originalRate) {
		t.Fatal("offline revision changed frozen original pricing", e)
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, h.f.user.TenantID, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = h.f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'admin')`, h.f.project, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	_, e = h.f.s.WriteUsageRate(ctx, h.f.actor, h.f.project, UsageRateWriteRequest{ProjectID: h.f.project, ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version, ExpectedVersion: rate.Version, Currency: "CNY", InputRateMicrosPerMillion: 0, OutputRateMicrosPerMillion: 0, InputTokenReservation: 100, OutputTokenReservation: 4096, Reason: "Project administrator cannot alter organization pricing", IdempotencyKey: newID()})
	releaseWantStatus(t, e, 403)
	projectView, e := h.f.s.GetUsageDashboard(ctx, h.f.actor, h.f.project, "")
	if e != nil || !projectView.CanManageProject || projectView.CanManageRates || projectView.CanManageOrganization || projectView.OrganizationPolicy != nil {
		t.Fatal("rate authority was inferred from project quota authority", projectView, e)
	}
	req.ExpectedVersion = policy.Version
	req.IdempotencyKey = newID()
	policy, e = h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, req)
	if e != nil {
		t.Fatal("project administrator cannot manage own allowance", e)
	}
	if _, e = h.f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, h.f.project, h.f.user.ID); e != nil {
		t.Fatal(e)
	}
	req.ExpectedVersion = policy.Version
	req.IdempotencyKey = newID()
	if _, e = h.f.s.WriteUsagePolicy(ctx, h.f.actor, h.f.project, req); e == nil {
		t.Fatal("revoked administrator replay/write")
	}
	d, e := h.f.s.GetUsageDashboard(ctx, h.f.actor, h.f.project, "")
	if e != nil || d.OrganizationPolicy != nil || len(d.Balances) != 1 || d.CanManageProject || d.CanManageOrganization || d.CanManageRates || len(d.Rates) != 1 || d.Rates[0].ID != rate.ID {
		t.Fatal("ordinary reader leaked organization totals", d, e)
	}
}

func TestUsageAccountingAtomicClaimCompetitionAndFinalReplay(t *testing.T) {
	h := newUsageFixture(t)
	usageRateForTest(t, h)
	usagePolicyForTest(t, h, "project", usageInt(4196), nil)
	first := h.create(t)
	h.request.IdempotencyKey = newID()
	second := h.create(t)
	actor := toolActorForTest(t, h.f, h.token)
	var wg sync.WaitGroup
	assigned := make(chan *dag.Assignment, 2)
	fail := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			authority := h.f.s.engineAuthority(context.Background(), actor, commandTarget{tenantID: h.f.user.TenantID, projectID: h.f.project}, purposeRunnerWrite)
			defer authority.close()
			a, e := h.f.e.ClaimWithOperation(authority.ctx, h.f.user.TenantID, h.f.project, h.profile.WorkerID, newID(), []string{"command:" + h.profile.Profile.CommandID}, func(tx *sql.Tx) error { return authority.beforeRunner(tx, h.profile.WorkerID) }, nil)
			if e != nil {
				fail <- e
				return
			}
			assigned <- a
		}()
	}
	wg.Wait()
	close(assigned)
	close(fail)
	for e := range fail {
		t.Fatal(e)
	}
	var picked *dag.Assignment
	count := 0
	for a := range assigned {
		if a != nil {
			count++
			picked = a
		}
	}
	if count != 1 {
		t.Fatal("atomic allowance admitted", count)
	}
	if picked.RunID != first.RunID && picked.RunID != second.RunID {
		t.Fatal(picked)
	}
	balance := usageBalanceForTest(t, h)
	if balance.ReservedTokens == nil || *balance.ReservedTokens != 4196 || balance.ReportedTokens == nil || *balance.ReportedTokens != 0 || balance.UnresolvedAttempts != 1 {
		t.Fatal(balance)
	}
	result := usageMeasuredResult(h, *picked, 10, 5)
	h.complete(t, result)
	h.complete(t, result)
	a := usageAttemptForTest(t, h, picked.RunID)
	if a.State != "settled" || a.ReportedTokens == nil || *a.ReportedTokens != 15 || a.EstimatedCostMicros == nil || *a.EstimatedCostMicros != 20 || a.HeldTokens != 0 || a.HeldCostMicros == nil || *a.HeldCostMicros != 0 {
		t.Fatal(a)
	}
	balance = usageBalanceForTest(t, h)
	if *balance.ReportedTokens != 15 || *balance.ReservedTokens != 0 || balance.UnresolvedAttempts != 0 {
		t.Fatal(balance)
	}
	var facts int
	if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE kind='result'`).Scan(&facts); e != nil || facts != 1 {
		t.Fatal("double charged result replay", facts, e)
	}
}

func TestUsageAccountingUnknownResolutionAndManualConfirmation(t *testing.T) {
	h := newUsageFixture(t)
	usageRateForTest(t, h)
	a := h.create(t)
	assignment := h.claim(t)
	var originalLease int64
	if e := h.f.s.DB.QueryRow(`SELECT lease_until FROM dag_nodes WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID).Scan(&originalLease); e != nil {
		t.Fatal(e)
	}
	if _, e := h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID); e != nil {
		t.Fatal(e)
	}
	expired, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "reconciling", 1, "")
	if e != nil || expired.Total != 1 || len(expired.Items) != 1 || expired.Items[0].State != "reconciling" {
		t.Fatal("expired financial hold missing from filtered total/page", expired, e)
	}
	storedID := expired.Items[0].ID
	reserved, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "reserved", 1, "")
	if e != nil || reserved.Total != 0 || len(reserved.Items) != 0 {
		t.Fatal("effective unknown still counted as reserved", reserved, e)
	}
	if _, e = h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "reserved", 1, storedID); e == nil {
		t.Fatal("cursor from another effective state accepted")
	}
	continued, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "reconciling", 1, storedID)
	if e != nil || continued.Total != 1 || len(continued.Items) != 0 {
		t.Fatal("same-state keyset cursor changed total", continued, e)
	}
	if _, e = h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=? WHERE run_id=? AND id=?`, originalLease, assignment.RunID, assignment.NodeID); e != nil {
		t.Fatal(e)
	}
	result := h.result(assignment, h.output())
	result.OutcomeUnknown = true
	result.ToolEvidence.Terminal = "unknown"
	result.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(10)}
	result.ToolEvidence.UsageState = toolcontract.UsagePartial
	h.complete(t, result)
	attempt := usageAttemptForTest(t, h, a.RunID)
	if attempt.State != "reconciling" || attempt.HeldTokens != 4186 || attempt.ReportedTokens == nil || *attempt.ReportedTokens != 10 {
		t.Fatal(attempt)
	}
	originalSHA := attempt.SourceResultSHA256
	ctx, auth, close, e := h.f.s.planningAuthority(context.Background(), h.f.actor, h.f.project, "", purposeBrowserProjectApprove)
	if e != nil {
		t.Fatal(e)
	}
	ref := dag.AccountingAttempt{TenantID: h.f.user.TenantID, ProjectID: h.f.project, RunID: assignment.RunID, NodeID: assignment.NodeID, WorkerID: assignment.WorkerID, Attempt: assignment.Attempt, Fence: assignment.Fence, Spec: dag.Node{Type: "agent", Command: assignment.Command, Config: assignment.Config}}
	if e = h.f.s.RecordResolutionTx(ctx, auth.tx, ref, "not_started"); e != nil {
		close()
		t.Fatal(e)
	}
	if e = auth.tx.Commit(); e != nil {
		close()
		t.Fatal(e)
	}
	close()
	attempt = usageAttemptForTest(t, h, a.RunID)
	if attempt.HeldTokens != 4186 {
		t.Fatal("manual business outcome refunded unknown usage", attempt)
	}
	req := UsageConfirmRequest{ProjectID: h.f.project, ExpectedVersion: attempt.Version, Usage: toolcontract.Usage{InputTokens: usageInt(9), OutputTokens: usageInt(2)}, SourceRef: "operator-receipt:fixture", Reason: "Manually checked provider record", AcknowledgeUsage: true, IdempotencyKey: newID()}
	if _, e = h.f.s.ConfirmUsageAttempt(context.Background(), h.f.actor, h.f.project, attempt.ID, req); e == nil {
		t.Fatal("manual usage lowered measured input")
	}
	req.Usage.InputTokens = usageInt(12)
	req.IdempotencyKey = newID()
	confirmed, e := h.f.s.ConfirmUsageAttempt(context.Background(), h.f.actor, h.f.project, attempt.ID, req)
	if e != nil || confirmed.State != "confirmed" || confirmed.UsageSource != "manual_confirmation" || confirmed.HeldTokens != 0 || confirmed.SourceResultSHA256 != originalSHA {
		t.Fatal(confirmed, e)
	}
	replay, e := h.f.s.ConfirmUsageAttempt(context.Background(), h.f.actor, h.f.project, attempt.ID, req)
	if e != nil || replay.Version != confirmed.Version {
		t.Fatal(replay, e)
	}
	var facts int
	if e = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id=?`, attempt.ID).Scan(&facts); e != nil || facts != 3 {
		t.Fatal("original/result/resolution/manual audit lost", facts, e)
	}
	closed, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "reconciling", 1, "")
	if e != nil || closed.Total != 0 || len(closed.Items) != 0 {
		t.Fatal("settled financial attempt inherited DAG unknown state", closed, e)
	}
}

func TestUsageAccountingPartialUnpricedAndMeasuredZero(t *testing.T) {
	t.Run("stopped-retains-holds", func(t *testing.T) {
		h := newUsageFixture(t)
		usageRateForTest(t, h)
		a := h.create(t)
		assignment := h.claim(t)
		result := usageMeasuredResult(h, assignment, 0, 0)
		result.Stopped = true
		result.ToolEvidence.Terminal = "unknown"
		h.complete(t, result)
		attempt := usageAttemptForTest(t, h, a.RunID)
		if attempt.State != "partial" || attempt.HeldTokens != 4196 || attempt.HeldCostMicros == nil || *attempt.HeldCostMicros != 8292 || attempt.ReportedTokens == nil || *attempt.ReportedTokens != 0 {
			t.Fatal("stopped process automatically refunded the financial hold", attempt)
		}
	})
	for _, priced := range []bool{false, true} {
		t.Run(map[bool]string{false: "unpriced", true: "priced-zero"}[priced], func(t *testing.T) {
			h := newUsageFixture(t)
			if priced {
				usageRateForTest(t, h)
			}
			a := h.create(t)
			assignment := h.claim(t)
			result := usageMeasuredResult(h, assignment, 0, 0)
			h.complete(t, result)
			attempt := usageAttemptForTest(t, h, a.RunID)
			if attempt.ReportedTokens == nil || *attempt.ReportedTokens != 0 {
				t.Fatal("measured zero lost", attempt)
			}
			if !priced && (attempt.State != "unpriced" || attempt.EstimatedCostMicros != nil || attempt.Rate != nil || attempt.ReservedCostMicros != nil) {
				t.Fatal("invented price", attempt)
			}
			if priced && (attempt.State != "settled" || attempt.EstimatedCostMicros == nil || *attempt.EstimatedCostMicros != 0) {
				t.Fatal(attempt)
			}
		})
	}
}

func TestUsageAccountingOverrunOverflowAndImmutableRate(t *testing.T) {
	h := newUsageFixture(t)
	rate := usageRateForTest(t, h)
	usagePolicyForTest(t, h, "organization", usageInt(4196), nil)
	a := h.create(t)
	assignment := h.claim(t)
	_, e := h.f.s.WriteUsageRate(context.Background(), h.f.actor, h.f.project, UsageRateWriteRequest{ProjectID: h.f.project, ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version, ExpectedVersion: rate.Version, Currency: "USD", InputRateMicrosPerMillion: 7, OutputRateMicrosPerMillion: 9, InputTokenReservation: 100, OutputTokenReservation: 4096, Reason: "New immutable revision", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	result := usageMeasuredResult(h, assignment, usageSafe, usageSafe)
	h.complete(t, result)
	attempt := usageAttemptForTest(t, h, a.RunID)
	if !attempt.Overrun || attempt.State != "reconciling" || attempt.ReportedTokens != nil || attempt.HeldTokens != 4196 || attempt.Rate == nil || attempt.Rate.ID != rate.ID || attempt.Currency != "CNY" {
		t.Fatal("overflow lost actual facts/frozen rate/holds", attempt)
	}
	var raw string
	if e = h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_facts WHERE attempt_id=? AND kind='result'`, attempt.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var stored dag.Result
	if json.Unmarshal([]byte(raw), &stored) != nil || *stored.ToolEvidence.Usage.InputTokens != usageSafe {
		t.Fatal("overflow discarded original result")
	}
	h.request.IdempotencyKey = newID()
	h.create(t)
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if reply["assignment"] != nil {
		t.Fatal("overrun or currency mismatch bypassed organization policy")
	}
}

func TestUsageAccountingFailedClaimAndSettlementRollback(t *testing.T) {
	h := newUsageFixture(t)
	usageRateForTest(t, h)
	a := h.create(t)
	// A later resource admission failure must leave the financial extension with
	// no attempts, balances or facts even though its read-only Check ran.
	if e := h.f.e.SetResourceLimits(context.Background(), h.f.user.TenantID, h.f.project, map[string]int64{"execution_slots": 0}); e != nil {
		t.Fatal(e)
	}
	reply := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	if reply["assignment"] != nil {
		t.Fatal("zero scheduler slots admitted")
	}
	var count int
	if e := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_attempts WHERE run_id=?`, a.RunID).Scan(&count); e != nil || count != 0 {
		t.Fatal("resource deny leaked money", count, e)
	}
	if e := h.f.e.SetResourceLimits(context.Background(), h.f.user.TenantID, h.f.project, map[string]int64{"execution_slots": 1}); e != nil {
		t.Fatal(e)
	}
	assignment := h.claim(t)
	attempt := usageAttemptForTest(t, h, a.RunID)
	req := UsageConfirmRequest{ProjectID: h.f.project, ExpectedVersion: attempt.Version, Usage: toolcontract.Usage{InputTokens: usageInt(0), OutputTokens: usageInt(0)}, SourceRef: "unsupported-active", Reason: "Must not settle a current live attempt", AcknowledgeUsage: true, IdempotencyKey: newID()}
	if _, e := h.f.s.ConfirmUsageAttempt(context.Background(), h.f.actor, h.f.project, attempt.ID, req); e == nil {
		t.Fatal("settled an active process")
	}
	result := usageMeasuredResult(h, assignment, 20, 2)
	h.complete(t, result)
	page, e := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "", 1, "")
	if e != nil || len(page.Items) != 1 || page.Items[0].HeldTokens != 0 {
		t.Fatal(page, e)
	}
	if _, e = h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "2001-01", "", 1, attempt.ID); e == nil {
		t.Fatal("cross-month cursor accepted")
	}
}
