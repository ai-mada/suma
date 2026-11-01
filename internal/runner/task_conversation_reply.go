package runner

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This dedicated purpose has no ambient provider configuration. The Go host
// resolves its frozen deployment and only forwards an attempt-local gateway.
func localTaskConversationReplyCapability(spec CommandSpec) (*tc.TaskConversationReplyCapability, error) {
	if !spec.TaskConversationReply {
		return nil, nil
	}
	if spec.Adapter != "suma" || spec.Protocol != "suma-jsonl-v1" || spec.Input != "agent-jsonl" || !spec.ReadOnly || spec.ModelExecutionProtocol != mc.Protocol || spec.DispatchRecommendation || spec.NativeWorkspace != nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.TestEvidence || spec.TestReport || spec.CandidateComposition || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		return nil, fmt.Errorf("task conversation reply requires a dedicated read-only host-model profile without tools or provider environment")
	}
	return &tc.TaskConversationReplyCapability{Protocol: tc.TaskConversationReplyProtocol}, nil
}

func probeTaskConversationReply(ctx context.Context, executable string, spec CommandSpec) error {
	expected, err := localTaskConversationReplyCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	raw, err := probeCommandStreams(ctx, executable, append(append([]string{}, spec.Args...), "--task-conversation-reply-probe"), CommandSpec{}, nil, true)
	if err != nil {
		return fmt.Errorf("task conversation reply protocol probe failed")
	}
	object, err := decodeToolObject([]byte(raw))
	if err != nil || len(object) != 1 || object["protocol"] != expected.Protocol {
		return fmt.Errorf("task conversation reply probe did not prove its closed no-tools protocol")
	}
	return nil
}

func taskConversationReplyRequest(a Assignment, spec CommandSpec, profile tc.Profile) (map[string]any, error) {
	expected, err := localTaskConversationReplyCapability(spec)
	bad := func() (map[string]any, error) {
		return nil, fmt.Errorf("task conversation reply requires its dedicated purpose and frozen single-call host model binding")
	}
	if err != nil || expected == nil || a.Type != "agent" || a.Config["purpose"] != "task_conversation_reply" || !tc.SupportsTaskConversationReplyProfile(profile) || tc.ValidateProfile(profile, true) != nil || *profile.TaskConversationReply != *expected || a.CandidateConflictRepair != nil || a.CandidateComposition != nil || a.CodeBaseline != nil || a.DevelopmentContinuation != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a) {
		return bad()
	}
	goal, ok := a.Config["goal"].(string)
	if !ok || strings.TrimSpace(goal) == "" || len(goal) > 64000 || !utf8.ValidString(goal) {
		return bad()
	}
	for _, name := range []string{"codex_execution", "opencode_execution", "octop_execution"} {
		if _, present := a.Config[name]; present {
			return bad()
		}
	}
	for _, name := range []string{"max_steps", "max_output_tokens"} {
		if value, present := a.Config[name]; !present || value == nil {
			return bad()
		}
	}
	binding, err := mc.DecodeBinding(a.Config["model_execution"])
	if err != nil || binding.Parameters.MaxSteps != 1 || binding.Parameters.MaxOutputTokens > 8192 {
		return bad()
	}
	request, err := oneCallModelRequest(a, "task_conversation_reply")
	if err != nil || request["max_steps"] != binding.Parameters.MaxSteps || int64(request["max_tokens"].(int)) != binding.Parameters.MaxOutputTokens {
		return bad()
	}
	return request, nil
}
