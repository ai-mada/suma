package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Purpose declarations share fixed local environment validation, while their
// public typed capabilities and offline probes remain distinct.
func localReadOnlyModel(spec CommandSpec, purpose string) (model, origin string, err error) {
	if spec.Adapter != "suma" || spec.Protocol != "suma-jsonl-v1" || !spec.ReadOnly || spec.NativeWorkspace != nil || spec.Input != "agent-jsonl" || spec.TestEvidence || spec.ArtifactInput || spec.ArtifactOutput != "" {
		return "", "", fmt.Errorf("%s requires a read-only Suma profile without workspace or artifact capabilities", purpose)
	}
	for _, name := range spec.EnvFrom {
		if strings.EqualFold(name, "SUMA_MODEL") || strings.EqualFold(name, "SUMA_MODEL_BASE_URL") {
			return "", "", fmt.Errorf("%s model and endpoint must be fixed local environment values", purpose)
		}
	}
	for name := range spec.Env {
		if strings.EqualFold(name, "SUMA_MODEL") && name != "SUMA_MODEL" || strings.EqualFold(name, "SUMA_MODEL_BASE_URL") && name != "SUMA_MODEL_BASE_URL" {
			return "", "", fmt.Errorf("%s model environment names must be canonical", purpose)
		}
	}
	u, err := url.Parse(spec.Env["SUMA_MODEL_BASE_URL"])
	if err != nil || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", "", fmt.Errorf("%s endpoint is invalid", purpose)
	}
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port != "" && !(u.Scheme == "https" && port == "443" || u.Scheme == "http" && port == "80") {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return spec.Env["SUMA_MODEL"], u.Scheme + "://" + host, nil
}

func localKnowledgeExtractionCapability(spec CommandSpec) (*tc.KnowledgeExtractionCapability, error) {
	if !spec.KnowledgeExtraction {
		return nil, nil
	}
	if spec.Planning || spec.FailureDiagnosis {
		return nil, fmt.Errorf("planning and knowledge extraction profiles are mutually exclusive")
	}
	model, origin, err := localReadOnlyModel(spec, "knowledge extraction")
	if err != nil {
		return nil, err
	}
	p := &tc.KnowledgeExtractionCapability{Protocol: tc.KnowledgeExtractionProtocol, Model: model, EndpointOrigin: origin}
	if err = tc.ValidateKnowledgeExtractionCapability(*p); err != nil {
		return nil, err
	}
	return p, nil
}

func probeKnowledgeExtraction(ctx context.Context, exe string, spec CommandSpec, secrets []string) error {
	expected, err := localKnowledgeExtractionCapability(spec)
	if err != nil || expected == nil {
		return err
	}
	raw, err := probeCommandStreams(ctx, exe, append(append([]string{}, spec.Args...), "--knowledge-extraction-probe"), spec, secrets, true)
	if err != nil {
		return err
	}
	if _, err = decodeToolObject([]byte(raw)); err != nil {
		return fmt.Errorf("knowledge extraction protocol probe is invalid")
	}
	var actual tc.KnowledgeExtractionCapability
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&actual) != nil || actual != *expected || tc.ValidateKnowledgeExtractionCapability(actual) != nil {
		return fmt.Errorf("knowledge extraction protocol probe does not match fixed local model configuration")
	}
	return nil
}

func modelPurposeRequest(a Assignment, spec CommandSpec, profile tc.Profile) (map[string]any, error) {
	if spec.FailureDiagnosis || a.Config["purpose"] == "failure_diagnosis" {
		return failureDiagnosisRequest(a, spec, profile)
	}
	if !spec.KnowledgeExtraction && a.Config["purpose"] != "knowledge_extraction" {
		return planningRequest(a, spec, profile)
	}
	if a.Config["purpose"] != "knowledge_extraction" || a.Type != "agent" || !spec.KnowledgeExtraction || profile.KnowledgeExtraction == nil || profile.Planning != nil || profile.FailureDiagnosis != nil || spec.NativeWorkspace != nil || spec.Input != "agent-jsonl" {
		return nil, fmt.Errorf("knowledge extraction requires its dedicated purpose and probed profile")
	}
	return oneCallModelRequest(a, "knowledge_extraction")
}
