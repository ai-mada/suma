package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The Octop runtime declares only its execution protocol. Model deployments,
// parameters, credentials and budget belong to the common ModelBinding path.
func validateOctopSpec(spec CommandSpec) error {
	if spec.Adapter != "octop" && spec.Protocol != tc.OctopProtocol {
		return nil
	}
	if spec.Adapter != "octop" || spec.Protocol != tc.OctopProtocol || spec.Input != "octop-acp-jsonl" || spec.ReadOnly || spec.NativeWorkspace == nil || spec.NativeWorkspace.ConflictRepair || spec.ModelExecutionProtocol != mc.Protocol || spec.CodexAppServer != nil || spec.OpenCode != nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.CandidateComposition || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" || len(spec.Args) != 1 || !filepath.IsAbs(spec.Args[0]) || strings.ToLower(filepath.Ext(spec.Args[0])) != ".pyz" || !filepath.IsAbs(spec.Executable) || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		return fmt.Errorf("Octop Harness requires its fixed Python bundle, model gateway and isolated host workspace")
	}
	return nil
}

func localOctopCapability(spec CommandSpec) (*tc.OctopExecutionCapability, error) {
	if spec.Adapter != "octop" && spec.Protocol != tc.OctopProtocol {
		return nil, nil
	}
	if err := validateOctopSpec(spec); err != nil {
		return nil, err
	}
	c := &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion}
	return c, tc.ValidateOctopExecutionCapability(*c)
}

func octopExecutionRequest(a Assignment, spec CommandSpec, profile tc.Profile) error {
	if spec.Adapter != "octop" {
		if _, present := a.Config["octop_execution"]; present {
			return fmt.Errorf("unsupported Octop model override")
		}
		return nil
	}
	if a.Type != "agent" || !tc.SupportsOctopExecutionProfile(profile) || tc.ValidateOctopNodeConfiguration(a.Config) != nil {
		return fmt.Errorf("Octop execution requires its current host model gateway capability")
	}
	if _, err := mc.DecodeBinding(a.Config["model_execution"]); err != nil {
		return fmt.Errorf("Octop execution requires its reviewed model binding")
	}
	return nil
}

func octopEnvironment(spec CommandSpec, home string) ([]string, []string, error) {
	if err := validateOctopSpec(spec); err != nil {
		return nil, nil, err
	}
	if !filepath.IsAbs(home) || safeMkdir(home) != nil {
		return nil, nil, fmt.Errorf("Octop private state directory is unavailable")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, nil, fmt.Errorf("Octop requires a fresh empty private state directory")
	}
	base, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return nil, nil, err
	}
	environment := []string{}
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "PROGRAMDATA": true, "LANG": true}[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "OCTOP_HOME="+home, "TEMP="+home, "TMP="+home, "TMPDIR="+home, "PYTHONNOUSERSITE=1", "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1", "HF_HUB_OFFLINE=1", "LANGCHAIN_TRACING_V2=false", "LANGSMITH_TRACING=false")
	raw, _ := json.Marshal(map[string]string{"home": home})
	return append(environment, "SUMA_OCTOP_CONFIG="+string(raw)), nil, nil
}

func probeOctop(ctx context.Context, executable string, spec CommandSpec, secrets []string, workspace string) (string, error) {
	root, err := realDirectory(workspace)
	if err != nil {
		return "", fmt.Errorf("Octop probe workspace is unavailable")
	}
	parent := filepath.Join(root, ".cache", "runner", "octop-probes")
	if err = safeMkdir(parent); err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(parent, "probe-")
	if err != nil {
		return "", err
	}
	defer func() {
		if resolved, e := realDirectory(directory); e == nil && resolved == directory && within(parent, directory) {
			_ = os.RemoveAll(directory)
		}
	}()
	environment, _, err := octopEnvironment(spec, filepath.Join(directory, "home"))
	if err != nil {
		return "", err
	}
	probeSpec := CommandSpec{Env: map[string]string{}}
	for _, item := range environment {
		name, value, _ := strings.Cut(item, "=")
		probeSpec.Env[name] = value
	}
	result, err := probeCommandStreams(ctx, executable, append(append([]string{}, spec.Args...), "--probe"), probeSpec, secrets, true)
	if err != nil {
		return "", fmt.Errorf("Octop Harness read-only SDK probe failed")
	}
	values, err := decodeToolObject(bytes.TrimSpace([]byte(result)))
	v, valid := nativeInteger(values, "v")
	if err != nil || len(values) != 9 || !closedOpenCodeFrame(values, "protocol", "runtime_version", "harness_version", "acp_sdk_version", "workspace_tools_version", "model_execution_protocol", "gateway_protocol") || !valid || v != 1 || textAt(values, "type") != "probe" || textAt(values, "protocol") != tc.OctopProtocol || textAt(values, "runtime_version") != "1.0.0" || textAt(values, "harness_version") != tc.OctopHarnessVersion || textAt(values, "acp_sdk_version") != tc.OctopACPSDKVersion || textAt(values, "workspace_tools_version") != NativeWorkspaceVersion || textAt(values, "model_execution_protocol") != mc.Protocol || textAt(values, "gateway_protocol") != mc.GatewayProtocol {
		return "", fmt.Errorf("Octop SDK probe differs from its supported runtime")
	}
	return "suma-octop-runtime 1.0.0", nil
}
