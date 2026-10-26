package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Reject a fractional wire number even when float64 would round it to an
// apparently valid integer. A malformed allowance must not acquire a receipt.
func TestUsageHTTPPreservesExactIntegerLexemes(t *testing.T) {
	h := newUsageFixture(t)
	f := h.f
	for _, field := range []string{"token_limit", "cost_limit_micros"} {
		body := map[string]any{"project_id": f.project, "scope": "project", "expected_version": 0, "mode": "observe", "currency": "CNY", "token_limit": nil, "cost_limit_micros": nil, "reason": "Reject fractional wire amount", "acknowledge_enforcement": false, "idempotency_key": newID()}
		body[field] = json.RawMessage(`1.00000000000000001`)
		f.call("POST", "/usage/policies", body, 400)
	}

	request := h.request
	raw, _ := json.Marshal(request)
	var planning map[string]any
	if err := json.Unmarshal(raw, &planning); err != nil {
		t.Fatal(err)
	}
	planning["expected_tool_version"] = json.RawMessage(fmt.Sprintf("%d.00000000000000001", h.profile.Version))
	f.call("POST", "/planning/analyses", planning, 400)
	var count int
	if err := f.s.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM suma_usage_policies`).Scan(&count); err != nil || count != 0 {
		t.Fatal("Invalid monetary request changed policies", count, err)
	}
}
