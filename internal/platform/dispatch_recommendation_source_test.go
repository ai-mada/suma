package platform

import (
	"context"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
)

func TestDispatchRecommendationBudgetFeasibilityAndStableCandidateIdentity(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	ctx := context.Background()
	original, err := h.f.s.GetDispatchRecommendationOptions(ctx, h.f.e, h.f.actor, h.f.project, h.request.DispatchRecommendationOptionsRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(original.Candidates) != 2 || !original.Coverage.Complete || original.PolicySHA256 == "" {
		t.Fatal("incomplete bounded source", original)
	}
	candidate := original.Candidates[0]
	identity := dispatchRecommendationCandidateID(h.request.DispatchRecommendationOptionsRequest, candidate)
	candidate.InventoryObservedAt = "a later renewed identical heartbeat"
	if dispatchRecommendationCandidateID(h.request.DispatchRecommendationOptionsRequest, candidate) != identity {
		t.Fatal("heartbeat alone changed semantic choice identity")
	}
	candidate.ModelSelection.Parameters.MaxSteps--
	if dispatchRecommendationCandidateID(h.request.DispatchRecommendationOptionsRequest, candidate) == identity {
		t.Fatal("changed execution parameters reused a candidate identity")
	}
	// The normal eight-step defaults do not fit this hard allowance. There is
	// still a valid one-step configuration, so reject neither all candidates nor
	// the user's budget. Go's normal binding is the feasibility authority.
	deployment := h.deployments[0]
	limit := deployment.ContextWindow + 32
	constraints := DispatchRecommendationConstraints{RequireHardBudget: true, Budget: &mc.Budget{TokenLimit: &limit}, MaxSteps: 8, MaxOutputTokens: 4096}
	parameters, ok := dispatchRecommendationFeasibleParameters(deployment, constraints, nil)
	if !ok || parameters.MaxSteps != 1 || parameters.MaxOutputTokens != 32 {
		t.Fatal("feasible smaller setting was discarded", parameters, ok)
	}
	if _, err = mc.NewBinding(deployment, parameters, constraints.Budget, nil); err != nil {
		t.Fatal(err)
	}
	constraints.Budget.TokenLimit = usageInt(deployment.ContextWindow)
	if _, ok = dispatchRecommendationFeasibleParameters(deployment, constraints, nil); ok {
		t.Fatal("an allowance with no output capacity was accepted")
	}
	cost := int64(1)
	constraints.Budget = &mc.Budget{CostLimitMicros: &cost, Currency: "USD"}
	if _, ok = dispatchRecommendationFeasibleParameters(deployment, constraints, nil); ok {
		t.Fatal("unknown pricing was presented as an enforceable cost budget")
	}
}
