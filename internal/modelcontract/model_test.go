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

func TestGatewayCoverageKeepsUnknownCostsAndCertifiesClosedZero(t *testing.T) {
	d := Deployment{ID: "gateway-model", Revision: 1, Protocol: ProviderProtocol, Model: "model", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("a", 64), ContextWindow: 100}
	d.SHA256 = DeploymentDigest(d)
	b, err := NewBinding(d, Parameters{MaxSteps: 2, MaxOutputTokens: 20}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := Evidence{GatewayProtocol: GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: d.SHA256, ParametersSHA256: b.ParametersSHA256, Model: d.Model, Requests: []RequestEvidence{}}
	u, complete := AggregateEvidenceUsage(b, e)
	if !complete || u == nil || u.InputTokens == nil || *u.InputTokens != 0 || u.OutputTokens == nil || *u.OutputTokens != 0 {
		t.Fatal("closed no-request host proof did not certify zero", u, complete)
	}
	input, output := int64(5), int64(4)
	e.Requests = []RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("b", 64), ResponseSHA256: strings.Repeat("c", 64), ProviderModel: d.Model, OutputAllowance: 10, UsageState: "complete", Usage: &RequestUsage{InputTokens: &input, OutputTokens: &output}}, {State: "unknown", Step: 1, RequestSHA256: strings.Repeat("d", 64), OutputAllowance: 10, UsageState: "unreported"}}
	u, complete = AggregateEvidenceUsage(b, e)
	if complete || u == nil || *u.InputTokens != 5 || *u.OutputTokens != 4 || EvidenceResponsesComplete(b, e) {
		t.Fatal("uncertain HTTP request released known allowance or erased actual counters", u, complete)
	}
	raw, _ := json.Marshal(e)
	for _, bad := range []string{strings.Replace(string(raw), `"gateway_closed":true`, `"gateway_closed":null`, 1), strings.TrimSuffix(string(raw), "}") + `,"gateway_closed":true}`, strings.Replace(string(raw), `"input_tokens":5`, `"input_tokens":5.0`, 1), strings.TrimSuffix(string(raw), "}") + `,"sdk_usage":true}`} {
		var decoded Evidence
		if json.Unmarshal([]byte(bad), &decoded) == nil {
			t.Fatal("non-exact gateway evidence accepted", bad)
		}
	}
}
