package cli

import (
	"encoding/json"
	"strings"

	"github.com/ai-mada/suma/internal/modelcontract"
)

const compositionCLIRepairInvalid = "conflict repair requires an exact conflict identity, typed repair task, workflow/tool versions and a stable operation key"

// This DTO validates transport shape only. The server resolves the actual
// conflict, source acceptance, member ownership and native execution authority.
// JSON versions are checked against their original numeric tokens below.
type compositionCLIRepairTask struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type compositionCLIRepair struct {
	ModelExecutionSelection    *modelcontract.Selection `json:"model_execution_selection,omitempty"`
	ProjectID                  string                   `json:"project_id"`
	ExpectedCompositionVersion json.Number              `json:"expected_composition_version"`
	ExpectedConflictSHA256     string                   `json:"expected_conflict_sha256"`
	Repair                     compositionCLIRepairTask `json:"repair"`
	WorkflowID                 string                   `json:"workflow_id"`
	WorkflowVersion            json.Number              `json:"workflow_version"`
	ProducerNodeID             string                   `json:"producer_node_id"`
	ToolProfileID              string                   `json:"tool_profile_id"`
	ExpectedToolVersion        json.Number              `json:"expected_tool_version"`
	IdempotencyKey             string                   `json:"idempotency_key"`
}

func compositionCLIRepairObject(decoder *json.Decoder, body map[string]any) bool {
	var q compositionCLIRepair
	if decoder.Decode(&q) != nil {
		return false
	}
	repair, ok := body["repair"].(map[string]any)
	if !ok || !diagnosisCLIFields(body, "project_id expected_composition_version expected_conflict_sha256 repair workflow_id workflow_version producer_node_id tool_profile_id expected_tool_version idempotency_key model_execution_selection") || !diagnosisCLIFields(repair, "title description priority owner_id acceptance_criteria") || !compositionCLIRepairModel(q.ModelExecutionSelection) {
		return false
	}
	if !compositionCLINumber(body, "expected_composition_version", 1) || !compositionCLINumber(body, "workflow_version", 1) || !compositionCLINumber(body, "expected_tool_version", 1) || !compositionCLIIdentifier(q.WorkflowID, 100) || !compositionCLIIdentifier(q.ProducerNodeID, 150) || !compositionCLIIdentifier(q.ToolProfileID, 64) || !handoffCLIOperation(q.IdempotencyKey) {
		return false
	}
	if len(q.ExpectedConflictSHA256) != 64 || strings.IndexFunc(q.ExpectedConflictSHA256, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) >= 0 {
		return false
	}
	if !diagnosisCLIText(q.Repair.Title, 200) || !diagnosisCLIText(q.Repair.Description, 16000) || len(q.Repair.AcceptanceCriteria) < 1 || len(q.Repair.AcceptanceCriteria) > 16 || q.Repair.OwnerID != "" && !compositionCLIIdentifier(q.Repair.OwnerID, 64) || len(q.Repair.Priority) > 64 {
		return false
	}
	for _, item := range q.Repair.AcceptanceCriteria {
		if !diagnosisCLIText(item, 1000) {
			return false
		}
	}
	return true
}
