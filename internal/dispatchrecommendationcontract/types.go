// Package dispatchrecommendationcontract contains only the closed proposal
// language. Current candidates, permissions, budgets and execution admission
// remain platform authority; this document cannot dispatch a tool.
package dispatchrecommendationcontract

import tc "github.com/ai-mada/suma/internal/toolcontract"

const Protocol = tc.DispatchRecommendationProtocol
const MaxOutputBytes = 32 << 10

type DispatchRecommendationChoice struct {
	CandidateID     string  `json:"candidate_id"`
	MaxSteps        *int    `json:"max_steps"`
	MaxOutputTokens *int64  `json:"max_output_tokens"`
	ReasoningEffort *string `json:"reasoning_effort"`
	Reason          string  `json:"reason"`
}

type DispatchRecommendationProposal struct {
	SchemaVersion   int                            `json:"schema_version"`
	Summary         string                         `json:"summary"`
	Rationale       string                         `json:"rationale"`
	Recommendations []DispatchRecommendationChoice `json:"recommendations"`
	Questions       []string                       `json:"questions"`
}
