package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Diagnosis is a distinct probed purpose. Its text is a proposal, never repair
// execution, workspace access, automatic retry or acceptance authority.
func localFailureDiagnosisCapability(spec CommandSpec) (*tc.FailureDiagnosisCapability, error) {
	if !spec.FailureDiagnosis {
		return nil, nil
	}
	if spec.Planning || spec.KnowledgeExtraction || spec.IntelligentOperations {
		return nil, fmt.Errorf("failure diagnosis requires a dedicated model profile")
	}
	model, origin, err := localReadOnlyModel(spec, "failure diagnosis")
	if err != nil {
		return nil, err
	}
	p := &tc.FailureDiagnosisCapability{Protocol: tc.FailureDiagnosisProtocol, Model: model, EndpointOrigin: origin}
	if err = tc.ValidateFailureDiagnosisCapability(*p); err != nil {
		return nil, err
	}
	return p, nil
}

func probeFailureDiagnosis(ctx context.Context, exe string, spec CommandSpec, secrets []string) error {
	expected, err := localFailureDiagnosisCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	raw, err := probeCommandStreams(ctx, exe, append(append([]string{}, spec.Args...), "--failure-diagnosis-probe"), spec, secrets, true)
	if err != nil {
		return err
	}
	if _, err = decodeToolObject([]byte(raw)); err != nil {
		return fmt.Errorf("failure diagnosis protocol probe is invalid")
	}
	var actual tc.FailureDiagnosisCapability
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&actual) != nil || actual != *expected || tc.ValidateFailureDiagnosisCapability(actual) != nil {
		return fmt.Errorf("failure diagnosis protocol probe does not match fixed local model configuration")
	}
	return nil
}

func failureDiagnosisRequest(a Assignment, spec CommandSpec, profile tc.Profile) (map[string]any, error) {
	expected, err := localFailureDiagnosisCapability(spec)
	if err != nil || expected == nil || a.Type != "agent" || a.Config["purpose"] != "failure_diagnosis" || profile.FailureDiagnosis == nil || *profile.FailureDiagnosis != *expected || profile.Planning != nil || profile.KnowledgeExtraction != nil || profile.IntelligentOperations != nil || !profile.ReadOnly {
		return nil, fmt.Errorf("failure diagnosis requires its dedicated purpose and probed profile")
	}
	return oneCallModelRequest(a, "failure_diagnosis")
}

// Shared bounds do not interpret model business output. The platform preserves
// and strictly parses the complete final document under its own domain schema.
func oneCallModelRequest(a Assignment, purpose string) (map[string]any, error) {
	invalid := func() (map[string]any, error) {
		return nil, fmt.Errorf("%s budget is invalid", strings.ReplaceAll(purpose, "_", " "))
	}
	for _, name := range []string{"max_steps", "max_output_tokens"} {
		if value, present := a.Config[name]; present && value == nil {
			return invalid()
		}
	}
	steps, err := planningLimit(a.Config["max_steps"], 1, 1)
	if err != nil {
		return invalid()
	}
	tokens, err := planningLimit(a.Config["max_output_tokens"], 4096, 8192)
	if err != nil {
		return invalid()
	}
	return map[string]any{"purpose": purpose, "max_steps": steps, "max_tokens": tokens}, nil
}

func oneCallModelProfile(p tc.Profile) (purpose, model string) {
	if p.DispatchRecommendation != nil {
		return "dispatch recommendation", ""
	}
	if p.IntelligentOperations != nil {
		return "intelligent operations", p.IntelligentOperations.Model
	}
	if p.FailureDiagnosis != nil {
		return "failure diagnosis", p.FailureDiagnosis.Model
	}
	if p.KnowledgeExtraction != nil {
		return "knowledge extraction", p.KnowledgeExtraction.Model
	}
	return "", ""
}
