package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This dedicated purpose has no ambient provider configuration. The Go host
// resolves its frozen deployment and only forwards an attempt-local gateway.
func localDispatchRecommendationCapability(spec CommandSpec) (*tc.DispatchRecommendationCapability, error) {
	if !spec.DispatchRecommendation {
		return nil, nil
	}
	if spec.Adapter != "suma" || spec.Protocol != "suma-jsonl-v1" || spec.Input != "agent-jsonl" || !spec.ReadOnly || spec.ModelExecutionProtocol != mc.Protocol || spec.NativeWorkspace != nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.TestEvidence || spec.TestReport || spec.CandidateComposition || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		return nil, fmt.Errorf("dispatch recommendation requires a dedicated read-only host-model profile without tools or provider environment")
	}
	return &tc.DispatchRecommendationCapability{Protocol: tc.DispatchRecommendationProtocol}, nil
}

func probeDispatchRecommendation(ctx context.Context, executable string, spec CommandSpec) error {
	expected, err := localDispatchRecommendationCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	raw, err := probeCommandStreams(ctx, executable, append(append([]string{}, spec.Args...), "--dispatch-recommendation-probe"), CommandSpec{}, nil, true)
	if err != nil {
		return fmt.Errorf("dispatch recommendation protocol probe failed")
	}
	object, err := decodeToolObject([]byte(raw))
	if err != nil || len(object) != 1 || object["protocol"] != expected.Protocol {
		return fmt.Errorf("dispatch recommendation probe did not prove its closed no-tools protocol")
	}
	return nil
}

func dispatchRecommendationRequest(a Assignment, spec CommandSpec, profile tc.Profile) (map[string]any, error) {
	expected, err := localDispatchRecommendationCapability(spec)
	bad := func() (map[string]any, error) {
		return nil, fmt.Errorf("dispatch recommendation requires its dedicated purpose and frozen single-call host model binding")
	}
	if err != nil || expected == nil || a.Type != "agent" || a.Config["purpose"] != "dispatch_recommendation" || !tc.SupportsDispatchRecommendationProfile(profile) || tc.ValidateProfile(profile, true) != nil || *profile.DispatchRecommendation != *expected || a.CandidateConflictRepair != nil || a.CandidateComposition != nil || a.CodeBaseline != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a) {
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
	request, err := oneCallModelRequest(a, "dispatch_recommendation")
	if err != nil || request["max_steps"] != binding.Parameters.MaxSteps || int64(request["max_tokens"].(int)) != binding.Parameters.MaxOutputTokens {
		return bad()
	}
	return request, nil
}

// A read-only model purpose needs private durable gateway metadata, not a Git
// worktree. The SDK's cwd is empty and it receives no project/MCP capability.
// Reusing the same attempt directory is refused instead of silently starting a
// second model execution; the ordinary Journal owns restart reconciliation.
func prepareReadOnlyModelAttempt(a Assignment, cfg ExecConfig) (*attemptWorkspace, error) {
	root := cfg.WorktreeRoot
	if root == "" {
		root = filepath.Join(cfg.Workspace, ".cache", "runner", "model-attempts")
	}
	root, err := filepath.Abs(root)
	if err != nil || safeMkdir(root) != nil {
		return nil, fmt.Errorf("read-only model metadata directory is unavailable")
	}
	directory := filepath.Join(root, attemptKey(a))
	if err = os.Mkdir(directory, 0700); err != nil {
		return nil, fmt.Errorf("read-only model attempt already exists or is unavailable; reconcile the original operation")
	}
	source := filepath.Join(directory, "cwd")
	if err = os.Mkdir(source, 0700); err != nil {
		return nil, fmt.Errorf("read-only model private working directory is unavailable")
	}
	w := &attemptWorkspace{AttemptKey: attemptKey(a), Assignment: a, Directory: directory, Source: source, State: "prepared"}
	if err = w.save(); err != nil {
		return nil, fmt.Errorf("read-only model original attempt could not be persisted")
	}
	return w, nil
}
