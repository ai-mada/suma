package dispatchrecommendationcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// Exercise the boundary shared by model output and explicit human selection;
// current candidate membership is deliberately tested by the platform instead.
func TestDispatchRecommendationClosedProposal(t *testing.T) {
	valid := `{"schema_version":1,"summary":"候选建议","rationale":"Frozen current facts only","recommendations":[{"candidate_id":"` + strings.Repeat("a", 64) + `","max_steps":128,"max_output_tokens":131072,"reasoning_effort":null,"reason":"Uses the current approved model"}],"questions":[]}`
	proposal, err := ParseProposal(valid)
	if err != nil || len(proposal.Recommendations) != 1 || *proposal.Recommendations[0].MaxSteps != 128 {
		t.Fatalf("valid proposal: %v", err)
	}
	for name, raw := range map[string]string{
		"alias":              strings.Replace(valid, `"summary"`, `"Summary"`, 1),
		"duplicate":          strings.Replace(valid, `"summary":"候选建议"`, `"summary":"first","summary":"second"`, 1),
		"missing nullable":   strings.Replace(valid, `"reasoning_effort":null,`, "", 1),
		"unexpected command": strings.Replace(valid, `"reason":"Uses`, `"command":"run","reason":"Uses`, 1),
		"fraction integer":   strings.Replace(valid, `"max_steps":128`, `"max_steps":1.0`, 1),
		"range":              strings.Replace(valid, `"max_output_tokens":131072`, `"max_output_tokens":131073`, 1),
		"null questions":     strings.Replace(valid, `"questions":[]`, `"questions":null`, 1),
		"invalid Unicode":    strings.Replace(valid, `"summary":"候选建议"`, `"summary":"\ud800"`, 1),
		"extra document":     valid + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProposal(raw); err == nil {
				t.Fatal("invalid closed proposal was accepted")
			}
		})
	}
	fixed := strings.Replace(strings.Replace(valid, `"max_steps":128`, `"max_steps":null`, 1), `"max_output_tokens":131072`, `"max_output_tokens":null`, 1)
	if _, err := ParseProposal(fixed); err != nil {
		t.Fatal("explicit null fixed-tool choice rejected:", err)
	}
	var selected DispatchRecommendationChoice
	if err := json.Unmarshal([]byte(`{"candidate_id":"`+strings.Repeat("b", 64)+`","max_steps":129,"max_output_tokens":null,"reasoning_effort":null,"reason":"bad bound"}`), &selected); err == nil {
		t.Fatal("independent choice bypassed bounds")
	}
}
