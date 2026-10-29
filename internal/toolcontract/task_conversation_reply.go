package toolcontract

import (
	"fmt"
	"github.com/ai-mada/suma/internal/modelcontract"
)

// This read-only capability has no independent model or provider configuration.
// Every reply uses the host's exact ModelExecution binding and original receipt.
type TaskConversationReplyCapability struct {
	Protocol string `json:"protocol"`
}

const TaskConversationReplyProtocol = "suma-task-conversation-reply-v1"

func ValidateTaskConversationReplyCapability(p TaskConversationReplyCapability) error {
	if p.Protocol != TaskConversationReplyProtocol {
		return fmt.Errorf("invalid task conversation reply protocol")
	}
	return nil
}
func SupportsTaskConversationReplyProfile(p Profile) bool {
	return p.Adapter == "suma" && p.Protocol == "suma-jsonl-v1" && p.ReadOnly && p.ModelExecutionProtocol == modelcontract.Protocol && p.WorkspaceToolsVersion == "" && p.TestReportProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil && p.DispatchRecommendation == nil && p.TaskConversationReply != nil && ValidateTaskConversationReplyCapability(*p.TaskConversationReply) == nil
}
