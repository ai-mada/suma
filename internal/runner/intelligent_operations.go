package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Intelligent operations only proposes local drafts. The declaration cannot
// enable workspace, candidate, artifact or another model execution purpose.
func localIntelligentOperationsCapability(spec CommandSpec) (*tc.IntelligentOperationsCapability, error) {
	if !spec.IntelligentOperations {
		return nil, nil
	}
	if spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.ModelExecutionProtocol != "" || spec.OpenCode != nil || spec.CodexAppServer != nil || spec.CandidateComposition {
		return nil, fmt.Errorf("intelligent operations requires a dedicated model profile")
	}
	model, origin, err := localReadOnlyModel(spec, "intelligent operations")
	if err != nil {
		return nil, err
	}
	p := &tc.IntelligentOperationsCapability{Protocol: tc.IntelligentOperationsProtocol, Model: model, EndpointOrigin: origin}
	if err = tc.ValidateIntelligentOperationsCapability(*p); err != nil {
		return nil, err
	}
	return p, nil
}

func probeIntelligentOperations(ctx context.Context, exe string, spec CommandSpec, secrets []string) error {
	expected, err := localIntelligentOperationsCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	// An offline capability probe needs only the fixed public identity, never
	// provider keys, forwarded environments or control credentials.
	probeSpec := CommandSpec{Env: map[string]string{"SUMA_MODEL": spec.Env["SUMA_MODEL"], "SUMA_MODEL_BASE_URL": spec.Env["SUMA_MODEL_BASE_URL"]}}
	raw, err := probeCommandStreams(ctx, exe, append(append([]string{}, spec.Args...), "--intelligent-operations-probe"), probeSpec, secrets, true)
	if err != nil {
		return err
	}
	if _, err = decodeToolObject([]byte(raw)); err != nil {
		return fmt.Errorf("intelligent operations protocol probe is invalid")
	}
	var actual tc.IntelligentOperationsCapability
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&actual) != nil || actual != *expected || tc.ValidateIntelligentOperationsCapability(actual) != nil {
		return fmt.Errorf("intelligent operations protocol probe does not match fixed local model configuration")
	}
	return nil
}

func intelligentOperationsRequest(a Assignment, spec CommandSpec, profile tc.Profile) (map[string]any, error) {
	expected, err := localIntelligentOperationsCapability(spec)
	_, modelExecution := a.Config["model_execution"]
	_, codexExecution := a.Config["codex_execution"]
	_, openCodeExecution := a.Config["opencode_execution"]
	if err != nil || expected == nil || a.Type != "agent" || a.Config["purpose"] != "intelligent_operations" || profile.IntelligentOperations == nil || *profile.IntelligentOperations != *expected || tc.ValidateProfile(profile, true) != nil || a.CandidateConflictRepair != nil || a.CandidateComposition != nil || a.CodeBaseline != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a) || modelExecution || codexExecution || openCodeExecution {
		return nil, fmt.Errorf("intelligent operations requires its dedicated purpose and probed profile")
	}
	return oneCallModelRequest(a, "intelligent_operations")
}
