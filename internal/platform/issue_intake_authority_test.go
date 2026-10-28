package platform

import (
	"context"
	"testing"
)

func TestIssueIntakePublicRecordKeysCannotReserveInternalSources(t *testing.T) {
	f := newFixture(t)
	ordinary := f.call("POST", "/requirements", f.body(map[string]any{"title": "An ordinary editable requirement"}), 201)
	for _, prefix := range []string{"issue-intake-", "failure-", "review-rejected-"} {
		f.call("POST", "/requirements", f.body(map[string]any{"title": "Not an internal producer", "idempotency_key": prefix + newID(), "data": map[string]any{"source_issue_id": newID()}}), 400)
		f.call("PATCH", "/requirements/"+ordinary["id"].(string), f.body(map[string]any{"title": "Must not occupy an internal slot", "expected_version": 1, "idempotency_key": prefix + newID()}), 400)
	}
	if expertCount(t, f, "suma_requirements") != 1 {
		t.Fatal("reserved-key command created a generic requirement")
	}
	current := f.call("GET", "/requirements/"+ordinary["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["title"] != ordinary["title"] || current["version"] != ordinary["version"] {
		t.Fatal("a rejected reserved-key patch modified the requirement")
	}
}

func TestIssueIntakeForgedGenericReceiptFailsClosedInDeliveryAndIntake(t *testing.T) {
	h, plan := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	expertCompleteProtocol(t, h, plan.Items[0].RunID)
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", plan.Items[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "rejected", "reason": "Quality source for intake provenance verification"}), f.e); err != nil {
		t.Fatal(err)
	}
	view := f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	issueID := view["issues"].(map[string]any)["items"].([]any)[0].(map[string]any)["issue_id"].(string)
	publicKey := newID()
	f.call("POST", "/requirements", f.body(map[string]any{"title": "Editable metadata cannot establish intake", "idempotency_key": publicKey, "data": map[string]any{"source_issue_id": issueID}}), 201)
	// Fault-inject a generic public receipt into the internal slot, retaining
	// its real generic request hash. This is not a migration or a supported alias.
	if _, err = f.s.DB.ExecContext(ctx, f.s.q(`UPDATE suma_operations SET operation_id=? WHERE tenant_id=? AND project_id=? AND operation_id=?`), "issue-intake-"+issueID, f.user.TenantID, f.project, publicKey); err != nil {
		t.Fatal(err)
	}
	before := expertCount(t, f, "suma_requirements")
	f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 409)
	f.call("POST", "/issues/"+issueID+"/intake", f.body(map[string]any{}), 409)
	if expertCount(t, f, "suma_requirements") != before {
		t.Fatal("an untrusted slot was silently replaced or adopted")
	}
}
