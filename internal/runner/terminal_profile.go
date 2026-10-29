package runner

import (
	"fmt"
	"github.com/ai-mada/suma/internal/terminalruntime"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func terminalSessionProtocol(spec CommandSpec) string {
	if spec.TerminalSession {
		return tc.TerminalSessionProtocol
	}
	return ""
}

// Environment inheritance and workspace/model adapters are independent grants;
// a terminal never acquires them by being a normal administrator command.
func validateTerminalSessionSpec(spec CommandSpec) error {
	if !spec.TerminalSession {
		return nil
	}
	profile := tc.Profile{Adapter: spec.Adapter, Protocol: spec.Protocol, ReadOnly: spec.ReadOnly, TerminalSessionProtocol: tc.TerminalSessionProtocol, ModelExecutionProtocol: spec.ModelExecutionProtocol, TaskConversationReply: nil}
	if !tc.SupportsTerminalSessionProfile(profile) || len(spec.EnvFrom) > 0 || spec.NativeWorkspace != nil || spec.TestEvidence || spec.TestReport || spec.Planning || spec.DispatchRecommendation || spec.TaskConversationReply || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.CandidateComposition || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.Input != "" || len(spec.Env) > 0 {
		return fmt.Errorf("Interactive terminal requires a dedicated shell without inherited secrets or automation capabilities")
	}
	return nil
}

// Syntax validation is separate from observed OS availability: a valid
// configured shell must not make an unsupported host look available, nor stop
// the Runner from discovering unrelated tools on that host.
func probeTerminalSessionSpec(spec CommandSpec) error {
	if err := validateTerminalSessionSpec(spec); err != nil {
		return err
	}
	if !spec.TerminalSession {
		return nil
	}
	return terminalruntime.Probe()
}
