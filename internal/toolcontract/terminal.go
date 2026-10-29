package toolcontract

import "fmt"

const TerminalSessionProtocol = "suma-pty-v1"

// A terminal shell is an explicit administrator capability. It does not carry
// model credentials, grant workspace tools to an agent, or produce test proof.
func SupportsTerminalSessionProfile(p Profile) bool {
	return p.TerminalSessionProtocol == TerminalSessionProtocol && p.Adapter == "command" && p.Protocol == "text-v1" && !p.ReadOnly && p.ModelExecutionProtocol == "" && p.WorkspaceToolsVersion == "" && p.TestReportProtocol == "" && p.ConflictRepairProtocol == "" && p.Planning == nil && p.KnowledgeExtraction == nil && p.FailureDiagnosis == nil && p.TaskConversationReply == nil && p.DispatchRecommendation == nil && p.IntelligentOperations == nil && p.OpenCodeExecution == nil && p.OctopExecution == nil
}
func validateTerminalSessionProfile(p Profile) error {
	if p.TerminalSessionProtocol != "" && !SupportsTerminalSessionProfile(p) {
		return fmt.Errorf("Terminal capability requires an isolated administrator shell profile")
	}
	return nil
}
