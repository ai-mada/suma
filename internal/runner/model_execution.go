package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// ModelDeploymentSpec is administrator-owned local configuration. Only the
// immutable public Deployment is advertised; endpoint paths and credentials
// never come from a node, a browser selection or a tool result.
type ModelDeploymentSpec struct {
	Deployment   mc.Deployment `json:"deployment"`
	BaseURL      string        `json:"base_url"`
	APIKeyEnvRef string        `json:"api_key_env_ref"`
}

var modelEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func DecodeModelDeployments(raw []byte) ([]ModelDeploymentSpec, error) {
	if len(raw) > 2<<20 {
		return nil, fmt.Errorf("model deployment configuration exceeds 2 MiB")
	}
	// Reuse the bounded unique-key JSON decoder, including objects inside arrays.
	if _, err := decodeToolObject(append(append([]byte(`{"items":`), raw...), '}')); err != nil {
		return nil, fmt.Errorf("model deployment configuration requires unique-key JSON")
	}
	var items []ModelDeploymentSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&items) != nil || items == nil {
		return nil, fmt.Errorf("invalid model deployment configuration")
	}
	return items, validateModelDeployments(items)
}

func validateModelDeployments(items []ModelDeploymentSpec) error {
	if len(items) > 64 {
		return fmt.Errorf("at most 64 local model deployments are supported")
	}
	seen := map[string]bool{}
	for _, item := range items {
		endpoint, err := mc.CanonicalEndpoint(item.BaseURL)
		sha, digestErr := mc.EndpointDigest(item.BaseURL)
		parsed, _ := url.Parse(item.BaseURL)
		key := fmt.Sprintf("%s/%d", item.Deployment.ID, item.Deployment.Revision)
		if mc.ValidateDeployment(item.Deployment) != nil || err != nil || digestErr != nil || endpoint != item.BaseURL || parsed.Scheme+"://"+parsed.Host != item.Deployment.EndpointOrigin || sha != item.Deployment.EndpointConfigSHA256 || !modelEnvName.MatchString(item.APIKeyEnvRef) || seen[key] {
			return fmt.Errorf("invalid local model deployment")
		}
		if strings.EqualFold(item.APIKeyEnvRef, "SUMA_RUNNER_TOKEN") || strings.EqualFold(item.APIKeyEnvRef, "SUMA_POOL_RUNNER_TOKEN") || strings.EqualFold(item.APIKeyEnvRef, "SUMA_AGENT_TOKEN") {
			return fmt.Errorf("model credential must use a separate environment reference")
		}
		seen[key] = true
	}
	return nil
}

func validateModelExecutionSpec(spec CommandSpec) error {
	if spec.ModelExecutionProtocol == "" {
		return nil
	}
	if spec.ModelExecutionProtocol != mc.Protocol || spec.Adapter != "suma" || spec.Protocol != "suma-jsonl-v1" || spec.Input != "agent-jsonl" || spec.ReadOnly || spec.NativeWorkspace == nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis {
		return fmt.Errorf("model execution requires the explicitly enabled native workspace protocol")
	}
	return nil
}

func probeModelExecution(ctx context.Context, executable string, spec CommandSpec) error {
	// Reuse the host's bounded read-only probe and process-tree finalization.
	// No provider or platform credential is forwarded to this offline probe.
	output, err := probeCommandStreams(ctx, executable, append(append([]string{}, spec.Args...), "--model-execution-probe"), CommandSpec{}, nil, true)
	if err != nil {
		return fmt.Errorf("model execution capability probe failed")
	}
	var capability struct {
		Protocol         string   `json:"protocol"`
		ProviderProtocol string   `json:"provider_protocol"`
		Parameters       []string `json:"parameters"`
	}
	raw := bytes.TrimSpace([]byte(output))
	if len(raw) > 4096 || json.Unmarshal(raw, &capability) != nil || capability.Protocol != mc.Protocol || capability.ProviderProtocol != mc.ProviderProtocol || len(capability.Parameters) != 2 || capability.Parameters[0] != "max_steps" || capability.Parameters[1] != "max_output_tokens" {
		return fmt.Errorf("model execution capability probe did not prove the supported protocol")
	}
	return nil
}

type localModelExecution struct {
	Binding mc.Binding `json:"binding"`
	BaseURL string     `json:"base_url"`
	APIKey  string     `json:"api_key"`
}

func modelExecutionRequest(a Assignment, cfg ExecConfig, spec CommandSpec, profile tc.Profile) (*localModelExecution, error) {
	value, present := a.Config["model_execution"]
	if !present && spec.ModelExecutionProtocol == "" {
		return nil, nil
	}
	if !present || spec.ModelExecutionProtocol != mc.Protocol || validateModelExecutionSpec(spec) != nil || profile.ModelExecutionProtocol != mc.Protocol || a.Type != "agent" {
		return nil, fmt.Errorf("model execution binding requires its current native capability")
	}
	binding, err := mc.DecodeBinding(value)
	if err != nil {
		return nil, fmt.Errorf("model execution binding is invalid")
	}
	if err = validateModelDeployments(cfg.ModelDeployments); err != nil {
		return nil, err
	}
	for _, name := range []string{"model", "model_id", "base_url", "model_base_url", "endpoint", "endpoint_url", "api_key", "env", "env_from", "temperature", "top_p", "reasoning"} {
		if _, ok := a.Config[name]; ok {
			return nil, fmt.Errorf("model execution does not accept node provider overrides")
		}
	}
	for _, item := range cfg.ModelDeployments {
		if item.Deployment != binding.Deployment {
			continue
		}
		key, ok := os.LookupEnv(item.APIKeyEnvRef)
		if !ok || key == "" || strings.ContainsAny(key, "\x00\r\n") {
			return nil, fmt.Errorf("model execution credential environment reference is unavailable")
		}
		return &localModelExecution{Binding: binding, BaseURL: item.BaseURL, APIKey: key}, nil
	}
	return nil, fmt.Errorf("model execution deployment is absent from the local allowlist")
}

func applyModelExecutionEnvironment(environment []string, local *localModelExecution) []string {
	if local == nil {
		return environment
	}
	filtered := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(strings.ToUpper(name), "SUMA_MODEL") {
			continue
		}
		filtered = append(filtered, value)
	}
	raw, _ := json.Marshal(local)
	return append(filtered, "SUMA_MODEL_EXECUTION="+string(raw))
}

func cloneModelEvidence(value *mc.Evidence) *mc.Evidence {
	if value == nil {
		return nil
	}
	copied := *value
	copied.Requests = append([]mc.RequestEvidence{}, value.Requests...)
	return &copied
}

// Preserve the selected nested object when caller-owned maps are later reused.
// Retain generic JSON values so durable assignment identity stays byte-stable.
func cloneModelAssignment(value Assignment) Assignment {
	for _, field := range []string{"model_execution", "codex_execution"} {
		selected, present := value.Config[field]
		if !present {
			continue
		}
		raw, err := json.Marshal(selected)
		if err != nil {
			continue
		}
		var copied any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&copied) != nil {
			continue
		}
		config := make(map[string]any, len(value.Config))
		for name, item := range value.Config {
			config[name] = item
		}
		config[field] = copied
		value.Config = config
	}
	return value
}
