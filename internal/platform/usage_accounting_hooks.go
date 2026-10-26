package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

var _ dag.AttemptAccounting = (*Service)(nil)

func usageOutputBudget(spec dag.Node) int64 {
	value, ok := spec.Config["max_output_tokens"]
	if !ok {
		return 4096
	}
	var n int64
	switch v := value.(type) {
	case int:
		n = int64(v)
	case int64:
		n = v
	case float64:
		if v != float64(int64(v)) {
			return usageSafe
		}
		n = int64(v)
	case json.Number:
		var e error
		n, e = v.Int64()
		if e != nil {
			return usageSafe
		}
	default:
		return usageSafe
	}
	if n < 1 || n > usageSafe {
		return usageSafe
	}
	return n
}

func (s *Service) usagePricingTx(ctx context.Context, tx *sql.Tx, ref dag.AccountingAttempt) (profile ToolProfile, rate *UsageRateRevision, org, project UsagePolicy, err error) {
	id, sha, bound, err := dag.ToolBinding(ref.Spec)
	if err != nil || !bound {
		return profile, nil, org, project, err
	}
	profile, err = s.toolProfileTx(ctx, tx, Principal{TenantID: ref.TenantID}, ref.ProjectID, id)
	if err != nil {
		return
	}
	org, err = s.usagePolicyTx(ctx, tx, ref.TenantID, ref.ProjectID, "organization")
	if err != nil {
		return
	}
	project, err = s.usagePolicyTx(ctx, tx, ref.TenantID, ref.ProjectID, "project")
	if err != nil {
		return
	}
	model, origin, priced := usagePricedModel(profile)
	if !priced || profile.Profile.ProfileSHA256 != sha {
		return profile, nil, org, project, nil
	}
	rate, err = s.usageRateTx(ctx, tx, ref.TenantID, ref.ProjectID, id)
	if rate != nil && (rate.ProfileSHA256 != sha || rate.Model != model || rate.EndpointOrigin != origin || rate.OutputTokenReservation < usageOutputBudget(ref.Spec)) {
		rate = nil
	}
	return
}

func (s *Service) CheckAttemptTx(ctx context.Context, tx *sql.Tx, ref dag.AccountingAttempt) (bool, error) {
	_, _, bound, e := dag.ToolBinding(ref.Spec)
	if e != nil || !bound {
		return e == nil, e
	}
	if e = s.usageLockTx(ctx, tx, ref.TenantID); e != nil {
		return false, e
	}
	_, rate, org, project, e := s.usagePricingTx(ctx, tx, ref)
	if e != nil {
		return false, e
	}
	period, e := s.usagePeriodTx(ctx, tx)
	if e != nil {
		return false, e
	}
	for _, policy := range []UsagePolicy{org, project} {
		if policy.Mode != "enforce" {
			continue
		}
		if rate == nil || rate.Currency != policy.Currency {
			return false, nil
		}
		tokens := big.NewInt(rate.InputTokenReservation + rate.OutputTokenReservation)
		cost := usageCost(rate.InputTokenReservation, rate.OutputTokenReservation, *rate)
		for _, resource := range []struct {
			name   string
			amount *big.Int
			limit  *int64
		}{{"tokens", tokens, policy.TokenLimit}, {"cost:" + rate.Currency, cost, policy.CostLimitMicros}} {
			account, e := s.usageAccountTx(ctx, tx, ref.TenantID, policy.Scope, policy.ProjectID, period, resource.name, false)
			if e != nil {
				return false, e
			}
			total := new(big.Int).Add(account.spent, account.reserved)
			total.Add(total, resource.amount)
			if usagePublic(total) == nil || resource.limit != nil && total.Cmp(big.NewInt(*resource.limit)) > 0 {
				return false, nil
			}
		}
	}
	return true, nil
}
func (s *Service) usagePeriodTx(ctx context.Context, tx *sql.Tx) (string, error) {
	clock, e := s.authorityNow(ctx, tx)
	return clock.Format("2006-01"), e
}

func (s *Service) ReserveAttemptTx(ctx context.Context, tx *sql.Tx, ref dag.AccountingAttempt) error {
	_, _, bound, e := dag.ToolBinding(ref.Spec)
	if e != nil || !bound {
		return e
	}
	if e = s.usageLockTx(ctx, tx, ref.TenantID); e != nil {
		return e
	}
	profile, rate, org, project, e := s.usagePricingTx(ctx, tx, ref)
	if e != nil {
		return e
	}
	period, e := s.usagePeriodTx(ctx, tx)
	if e != nil {
		return e
	}
	stamp := now()
	a := UsageAttempt{ID: usageID(ref), ProjectID: ref.ProjectID, Version: 1, RunID: ref.RunID, NodeID: ref.NodeID, Attempt: ref.Attempt, Fence: ref.Fence, WorkerID: ref.WorkerID, ToolProfileID: profile.ID, ProfileSHA256: profile.Profile.ProfileSHA256, Rate: rate, State: "unpriced", UsageState: toolcontract.UsageUnreported, UsageSource: "none", CreatedAt: stamp, UpdatedAt: stamp, AllowedActions: []string{}}
	record := usageRecord{Attempt: a, Period: period, OrganizationPolicyVersion: org.Version, ProjectPolicyVersion: project.Version, ChargedTokens: "0", ChargedCost: "0", HeldTokens: "0", HeldCost: "0", Unresolved: true}
	if model, origin, priced := usagePricedModel(profile); priced {
		record.Attempt.Model = model
		record.Attempt.EndpointOrigin = origin
	}
	if rate != nil {
		record.Attempt.State = "reserved"
		record.Attempt.Currency = rate.Currency
		record.Attempt.ReservedTokens = rate.InputTokenReservation + rate.OutputTokenReservation
		record.Attempt.ReservedCostMicros = usagePublic(usageCost(rate.InputTokenReservation, rate.OutputTokenReservation, *rate))
		record.Attempt.HeldTokens = record.Attempt.ReservedTokens
		record.Attempt.HeldCostMicros = record.Attempt.ReservedCostMicros
		record.HeldTokens = big.NewInt(record.Attempt.ReservedTokens).String()
		record.HeldCost = usageBigIntPointer(record.Attempt.ReservedCostMicros).String()
	}
	a = record.Attempt
	if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_attempts(id,tenant_id,project_id,period,version,state,unresolved,run_id,node_id,attempt,fence,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), a.ID, ref.TenantID, ref.ProjectID, period, a.Version, a.State, 1, ref.RunID, ref.NodeID, ref.Attempt, ref.Fence, stamp, stringify(record)); e != nil {
		return e
	}
	return s.usageBalancesDeltaTx(ctx, tx, ref.TenantID, record, new(big.Int), new(big.Int), usageBig(record.HeldTokens), usageBig(record.HeldCost), 1)
}
func usageBigIntPointer(p *int64) *big.Int {
	if p == nil {
		return new(big.Int)
	}
	return big.NewInt(*p)
}
func (s *Service) usageBalancesDeltaTx(ctx context.Context, tx *sql.Tx, tenant string, r usageRecord, tokens, cost, heldTokens, heldCost *big.Int, unresolved int) error {
	for _, scope := range []struct{ name, project string }{{"organization", ""}, {"project", r.Attempt.ProjectID}} {
		if e := s.usageAccountDeltaTx(ctx, tx, tenant, scope.name, scope.project, r.Period, "tokens", tokens, heldTokens, unresolved); e != nil {
			return e
		}
		if r.Attempt.Currency != "" {
			if e := s.usageAccountDeltaTx(ctx, tx, tenant, scope.name, scope.project, r.Period, "cost:"+r.Attempt.Currency, cost, heldCost, 0); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) usageFactTx(ctx context.Context, tx *sql.Tx, tenant, project, id, kind, sha string, payload any) error {
	_, e := tx.ExecContext(ctx, s.q(`INSERT INTO suma_usage_facts(id,tenant_id,project_id,attempt_id,kind,identity_sha256,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), newID(), tenant, project, id, kind, sha, now(), stringify(payload))
	return e
}
func (s *Service) settleUsageTx(ctx context.Context, tx *sql.Tx, tenant string, record *usageRecord, complete bool, state string) error {
	a := &record.Attempt
	tokens, cost := new(big.Int), new(big.Int)
	var input, output int64
	known := false
	if a.Usage != nil {
		if a.Usage.InputTokens != nil {
			input = *a.Usage.InputTokens
			known = true
		}
		if a.Usage.OutputTokens != nil {
			output = *a.Usage.OutputTokens
			known = true
		}
	}
	tokens.Add(big.NewInt(input), big.NewInt(output))
	a.ReportedTokens = nil
	a.EstimatedCostMicros = nil
	if known {
		a.ReportedTokens = usagePublic(tokens)
		if a.Rate != nil {
			cost = usageCost(input, output, *a.Rate)
			a.EstimatedCostMicros = usagePublic(cost)
		}
	}
	// A monetary overflow is still an actual usage fact. Keep the reservation
	// and unresolved marker; exact internal account values never wrap or truncate.
	overflow := usagePublic(tokens) == nil || a.Rate != nil && usagePublic(cost) == nil
	if overflow {
		complete = false
		state = "reconciling"
	}
	heldTokens, heldCost := new(big.Int), new(big.Int)
	if !complete {
		heldTokens.Sub(big.NewInt(a.ReservedTokens), tokens)
		if heldTokens.Sign() < 0 {
			heldTokens.SetInt64(0)
		}
		heldCost.Sub(usageBigIntPointer(a.ReservedCostMicros), cost)
		if heldCost.Sign() < 0 {
			heldCost.SetInt64(0)
		}
	}
	if overflow {
		heldTokens = usageBig(record.HeldTokens)
		heldCost = usageBig(record.HeldCost)
	}
	a.Overrun = a.Overrun || a.Rate != nil && (tokens.Cmp(big.NewInt(a.ReservedTokens)) > 0 || cost.Cmp(usageBigIntPointer(a.ReservedCostMicros)) > 0)
	unresolved := !complete
	delta := 0
	if record.Unresolved && !unresolved {
		delta = -1
	} else if !record.Unresolved && unresolved {
		delta = 1
	}
	if e := s.usageBalancesDeltaTx(ctx, tx, tenant, *record, new(big.Int).Sub(tokens, usageBig(record.ChargedTokens)), new(big.Int).Sub(cost, usageBig(record.ChargedCost)), new(big.Int).Sub(heldTokens, usageBig(record.HeldTokens)), new(big.Int).Sub(heldCost, usageBig(record.HeldCost)), delta); e != nil {
		return e
	}
	record.ChargedTokens = tokens.String()
	record.ChargedCost = cost.String()
	record.HeldTokens = heldTokens.String()
	record.HeldCost = heldCost.String()
	record.Unresolved = unresolved
	a.HeldTokens = heldTokens.Int64()
	if a.Rate != nil {
		a.HeldCostMicros = usagePublic(heldCost)
	}
	a.State = state
	return nil
}

func (s *Service) RecordResultTx(ctx context.Context, tx *sql.Tx, ref dag.AccountingAttempt, result dag.Result) error {
	_, _, bound, e := dag.ToolBinding(ref.Spec)
	if e != nil || !bound {
		return e
	}
	if e = s.usageLockTx(ctx, tx, ref.TenantID); e != nil {
		return e
	}
	record, e := s.usageRecordTx(ctx, tx, ref.TenantID, ref.ProjectID, usageID(ref))
	if e != nil {
		return e
	}
	if record.Attempt.WorkerID != ref.WorkerID || result.RunID != ref.RunID || result.NodeID != ref.NodeID || result.Attempt != ref.Attempt || result.Fence != ref.Fence || result.WorkerID != ref.WorkerID {
		return forbidden()
	}
	sha := digest(stringify(result))
	var count int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id=? AND kind='result' AND identity_sha256=?`), record.Attempt.ID, sha).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	old := record.Attempt.Version
	a := &record.Attempt
	previousUsage := a.Usage
	if a.SourceResultSHA256 == "" {
		a.SourceResultSHA256 = sha
	}
	if a.UsageSource != "manual_confirmation" {
		a.UsageSource = "none"
		if result.ToolEvidence != nil || previousUsage != nil {
			a.UsageSource = "tool_evidence"
		}
	}
	a.UsageState = toolcontract.UsageUnreported
	a.Usage = nil
	if result.ToolEvidence != nil {
		a.Usage = result.ToolEvidence.Usage
		a.UsageState = result.ToolEvidence.UsageState
	}
	// A later stop/reconciliation cannot erase counters previously reported by
	// the same attempt. Keep the original facts and per-counter lower bounds.
	monotonic := usageCountersAtLeastOrPartial(a.Usage, previousUsage)
	a.Usage = usageCounterMaximum(a.Usage, previousUsage)
	complete := monotonic && !result.Stopped && !result.OutcomeUnknown && a.UsageState == toolcontract.UsageComplete && a.Usage != nil && a.Usage.InputTokens != nil && a.Usage.OutputTokens != nil
	state := "settled"
	if !complete {
		state = "unreported"
		if a.Usage != nil {
			state = "partial"
		}
		if result.OutcomeUnknown {
			state = "reconciling"
		}
	} else if a.Rate == nil {
		state = "unpriced"
	}
	if e = s.settleUsageTx(ctx, tx, ref.TenantID, &record, complete, state); e != nil {
		return e
	}
	a = &record.Attempt
	a.Version++
	a.UpdatedAt = now()
	if e = s.usageFactTx(ctx, tx, ref.TenantID, ref.ProjectID, a.ID, "result", sha, result); e != nil {
		return e
	}
	return s.saveUsageTx(ctx, tx, ref.TenantID, record, old)
}
func usageCountersAtLeastOrPartial(next, old *toolcontract.Usage) bool {
	if old == nil {
		return true
	}
	if next == nil {
		return false
	}
	for _, pair := range [][2]*int64{{next.InputTokens, old.InputTokens}, {next.OutputTokens, old.OutputTokens}, {next.CachedInputTokens, old.CachedInputTokens}, {next.CacheWriteInputTokens, old.CacheWriteInputTokens}, {next.ReasoningOutputTokens, old.ReasoningOutputTokens}} {
		if pair[1] != nil && (pair[0] == nil || *pair[0] < *pair[1]) {
			return false
		}
	}
	return true
}
func usageCounterMaximum(next, old *toolcontract.Usage) *toolcontract.Usage {
	if old == nil {
		return next
	}
	out := *old
	if next == nil {
		return &out
	}
	for _, entry := range []struct {
		target **int64
		value  *int64
	}{{&out.InputTokens, next.InputTokens}, {&out.OutputTokens, next.OutputTokens}, {&out.CachedInputTokens, next.CachedInputTokens}, {&out.CacheWriteInputTokens, next.CacheWriteInputTokens}, {&out.ReasoningOutputTokens, next.ReasoningOutputTokens}} {
		if entry.value != nil && (*entry.target == nil || *entry.value > **entry.target) {
			*entry.target = usageInt(*entry.value)
		}
	}
	return &out
}

func (s *Service) RecordResolutionTx(ctx context.Context, tx *sql.Tx, ref dag.AccountingAttempt, decision string) error {
	_, _, bound, e := dag.ToolBinding(ref.Spec)
	if e != nil || !bound {
		return e
	}
	if e = s.usageLockTx(ctx, tx, ref.TenantID); e != nil {
		return e
	}
	r, e := s.usageRecordTx(ctx, tx, ref.TenantID, ref.ProjectID, usageID(ref))
	if e != nil {
		return e
	}
	sha := digest(decision)
	var count int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id=? AND kind='resolution' AND identity_sha256=?`), r.Attempt.ID, sha).Scan(&count); e != nil || count > 0 {
		return e
	}
	if e = s.usageFactTx(ctx, tx, ref.TenantID, ref.ProjectID, r.Attempt.ID, "resolution", sha, map[string]any{"decision": decision}); e != nil {
		return e
	}
	if !r.Unresolved {
		return nil
	}
	old := r.Attempt.Version
	r.Attempt.State = "reconciling"
	r.Attempt.Version++
	r.Attempt.UpdatedAt = now()
	return s.saveUsageTx(ctx, tx, ref.TenantID, r, old)
}
