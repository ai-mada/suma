package platform

import (
	"context"
	"testing"
)

func TestExecutionPoolCreationAndCurrentOwnerReceipts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	limit := int64(7)
	request := ExecutionPoolCreateRequest{Name: "共享开发资源", Description: "双边授权后使用", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, BudgetLimit: &limit, TenantWeights: map[string]int{}}, IdempotencyKey: newID()}
	pool, err := f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, request)
	if err != nil {
		t.Fatal(err)
	}
	if pool.OwnerTenantID != f.user.TenantID || pool.Policy.Version != 1 || pool.Policy.ConcurrencyLimit != 2 || pool.Policy.BudgetSpent != 0 || pool.Version != 1 {
		t.Fatal("pool creation did not freeze complete admission policy")
	}
	replay, err := f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, request)
	if err != nil || replay.ID != pool.ID {
		t.Fatal("pool operation did not recover original identity", err)
	}
	page, err := f.s.ListExecutionPools(ctx, f.e, f.user, f.cookie.Value, 1, "")
	if err != nil || page.Total != 1 || len(page.Items) != 1 || len(page.AllowedActions) != 1 || page.AllowedActions[0] != "create" {
		t.Fatal("owned list or empty-state permission unavailable", err)
	}
	paused, err := f.s.CommandExecutionPool(ctx, f.e, f.user, f.cookie.Value, pool.ID, ExecutionPoolCommandRequest{Action: "pause", ExpectedVersion: pool.Version, IdempotencyKey: newID()})
	if err != nil || paused.Status != "paused" || paused.Policy.BudgetSpent != 0 || paused.Version != 2 {
		t.Fatal("pause changed ledger or failed version", err)
	}
	if _, err = f.s.CommandExecutionPool(ctx, f.e, f.user, f.cookie.Value, pool.ID, ExecutionPoolCommandRequest{Action: "resume", ExpectedVersion: 1, IdempotencyKey: newID()}); err == nil {
		t.Fatal("stale pool version accepted")
	}
	if err = f.s.Logout(ctx, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, request); err == nil {
		t.Fatal("logged out caller replayed pool creation")
	}
}
