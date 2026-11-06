package platform

import (
	"context"
	"reflect"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestUsageAccountingLateStopPreservesManualConfirmation(t *testing.T) {
	t.Run("after-unknown-result", func(t *testing.T) { testLateStopAfterManualConfirmation(t, true) })
	t.Run("after-expiry-without-result", func(t *testing.T) { testLateStopAfterManualConfirmation(t, false) })
}

func testLateStopAfterManualConfirmation(t *testing.T, hasOriginalResult bool) {
	h := newUsageFixture(t)
	usageRateForTest(t, h)
	workflow := h.f.call("POST", "/workflows", h.f.body(map[string]any{
		"name": "Usage accounting late stop",
		"nodes": []dag.Node{{
			ID:      "agent",
			Type:    "agent",
			Command: []string{h.profile.Profile.CommandID},
			Config: map[string]any{
				"goal":                "Observe the original attempt",
				"tool_profile_id":     h.profile.ID,
				"tool_profile_sha256": h.profile.Profile.ProfileSHA256,
			},
		}},
	}), 201)
	h.f.call("POST", "/runs", h.f.body(map[string]any{"workflow_id": workflow["id"]}), 201)
	assignment := h.claim(t)
	ctx := context.Background()
	wantFacts := []string{}
	if hasOriginalResult {
		original := h.result(assignment, "partial output")
		original.OutcomeUnknown = true
		original.ToolEvidence.Terminal = "unknown"
		original.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(10)}
		original.ToolEvidence.UsageState = toolcontract.UsagePartial
		h.complete(t, original)
		wantFacts = append(wantFacts, stringify(original))
	} else {
		if _, err := h.f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.f.s.DB.Exec(`UPDATE dag_wakeups SET due_at=1 WHERE run_id=? AND node_id=? AND kind='lease'`, assignment.RunID, assignment.NodeID); err != nil {
			t.Fatal(err)
		}
		if err := h.f.e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	attempt := usageAttemptForTest(t, h, assignment.RunID)
	if attempt.State != "reconciling" || (attempt.SourceResultSHA256 != "") != hasOriginalResult || attempt.HeldTokens == 0 || attempt.HeldCostMicros == nil || *attempt.HeldCostMicros == 0 {
		t.Fatalf("expected an unresolved attempt with the original provenance, got %+v", attempt)
	}
	confirmed, err := h.f.s.ConfirmUsageAttempt(ctx, h.f.actor, h.f.project, attempt.ID, UsageConfirmRequest{
		ProjectID:        h.f.project,
		ExpectedVersion:  attempt.Version,
		Usage:            toolcontract.Usage{InputTokens: usageInt(12), OutputTokens: usageInt(2)},
		SourceRef:        "operator-receipt:fixture",
		Reason:           "Manually checked provider record",
		AcknowledgeUsage: true,
		IdempotencyKey:   newID(),
	})
	if err != nil || confirmed.State != "confirmed" || confirmed.UsageSource != "manual_confirmation" || confirmed.HeldTokens != 0 || confirmed.SourceResultSHA256 != attempt.SourceResultSHA256 {
		t.Fatalf("expected a settled manual confirmation, got %+v, error %v", confirmed, err)
	}
	before, err := h.f.s.GetUsageDashboard(ctx, h.f.actor, h.f.project, "")
	if err != nil {
		t.Fatal(err)
	}

	late := h.result(assignment, "")
	late.Stopped = true
	late.ToolEvidence.Terminal = "unknown"
	late.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(25)}
	late.ToolEvidence.UsageState = toolcontract.UsagePartial
	h.complete(t, late)
	h.complete(t, late)

	got := usageAttemptForTest(t, h, assignment.RunID)
	want := confirmed
	if !hasOriginalResult {
		want.SourceResultSHA256 = digest(stringify(late))
		want.Version++
		want.UpdatedAt = got.UpdatedAt
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("late stop changed confirmed attempt\n got %s\nwant %s", stringify(got), stringify(want))
	}
	after, err := h.f.s.GetUsageDashboard(ctx, h.f.actor, h.f.project, before.PeriodStart[:7])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Balances, before.Balances) {
		t.Fatalf("late stop changed balances\n got %s\nwant %s", stringify(after.Balances), stringify(before.Balances))
	}

	wantFacts = append(wantFacts, stringify(late))
	var resultFacts int
	if err = h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE attempt_id=? AND kind='result'`, attempt.ID).Scan(&resultFacts); err != nil || resultFacts != len(wantFacts) {
		t.Fatalf("expected %d result facts after replay, got %d, error %v", len(wantFacts), resultFacts, err)
	}
	for _, want := range wantFacts {
		var got string
		if err = h.f.s.DB.QueryRow(`SELECT payload FROM suma_usage_facts WHERE attempt_id=? AND kind='result' AND identity_sha256=?`, attempt.ID, digest(want)).Scan(&got); err != nil || got != want {
			t.Fatalf("result fact was changed or lost\n got %s\nwant %s\nerror %v", got, want, err)
		}
	}
}
