package modelcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBindingRejectsTamperingAndUnpricedCostBudget(t *testing.T) {
	d := Deployment{ID: "local-one", Revision: 1, Protocol: ProviderProtocol, Model: "model-one", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("a", 64), ContextWindow: 32000}
	d.SHA256 = DeploymentDigest(d)
	b, err := NewBinding(d, Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil || b.InputTokenReservation != 64000 || b.OutputTokenReservation != 100 {
		t.Fatal(b, err)
	}
	raw, _ := json.Marshal(b)
	if _, err = DecodeBinding(json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	b.Deployment.Model = "foreign"
	if ValidateBinding(b) == nil {
		t.Fatal("changed real model accepted")
	}
	cap := int64(10)
	if _, err = NewBinding(d, Parameters{MaxSteps: 2, MaxOutputTokens: 100}, &Budget{Currency: "CNY", CostLimitMicros: &cap}, nil); err == nil {
		t.Fatal("unknown cost accepted a cost ceiling")
	}
	rate := &RateSnapshot{ID: "operator-rate", Version: 1, Currency: "CNY", InputRateMicrosPerMillion: 1000000, OutputRateMicrosPerMillion: 2000000}
	cap = 64199
	if _, err = NewBinding(d, Parameters{MaxSteps: 2, MaxOutputTokens: 100}, &Budget{Currency: "CNY", CostLimitMicros: &cap}, rate); err == nil {
		t.Fatal("insufficient exact cost reservation accepted")
	}
	cap = 64200
	if _, err = NewBinding(d, Parameters{MaxSteps: 2, MaxOutputTokens: 100}, &Budget{Currency: "CNY", CostLimitMicros: &cap}, rate); err != nil {
		t.Fatal("exact sufficient cost reservation rejected", err)
	}
	rate.InputRateMicrosPerMillion = MaxSafeInteger
	if _, err = NewBinding(d, Parameters{MaxSteps: 128, MaxOutputTokens: 100}, nil, rate); err == nil {
		t.Fatal("overflowing conservative quote accepted")
	}
}

func TestBindingDecodeRejectsUnknownAndNullParameters(t *testing.T) {
	for _, raw := range []string{`null`, `{"parameters":null}`, `{"parameters":{"max_steps":1,"max_output_tokens":2,"temperature":1}}`, `{"parameters":{"max_steps":1.5,"max_output_tokens":2}}`} {
		if _, err := DecodeBinding(json.RawMessage(raw)); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}
