package runner

import (
	"encoding/json"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
)

func acpRunnerSpec() CommandSpec {
	return CommandSpec{Adapter: "qoder", Protocol: ac.Protocol, Executable: "qoder", Args: []string{"--acp"}, Input: "acp-stdio", ACP: &ACPExecutionSpec{Runtime: "qoder", ReviewedVersion: "Qoder CLI reviewed-170", Model: "fixed-model170"}}
}

func TestACPConfigurationRejectsMixedCapabilitiesAndOverrides(t *testing.T) {
	for _, mutate := range []func(*CommandSpec){func(s *CommandSpec) { s.Adapter = "suma" }, func(s *CommandSpec) { s.Input = "goal" }, func(s *CommandSpec) { s.Args = []string{"--acp", "--yolo"} }, func(s *CommandSpec) { s.ReadOnly = true }, func(s *CommandSpec) { s.NativeWorkspace = &NativeWorkspacePolicy{} }, func(s *CommandSpec) { s.ModelExecutionProtocol = "suma-model-execution-v1" }, func(s *CommandSpec) { s.TestEvidence = true }, func(s *CommandSpec) { s.Env = map[string]string{"QODER_YOLO": "1"} }, func(s *CommandSpec) { s.EnvFrom = []string{"SUMA_RUNNER_TOKEN"} }, func(s *CommandSpec) { s.ACP.CredentialEnvRef = "SUMA_POOL_TOKEN" }, func(s *CommandSpec) { s.ACP.ReviewedVersion = "version\nunsafe" }, func(s *CommandSpec) { s.ACP.Model = "" }} {
		spec := acpRunnerSpec()
		mutate(&spec)
		if validateACPExecutionSpec(spec) == nil {
			t.Fatal("invalid or mixed ACP execution configuration accepted")
		}
	}
	if err := validateACPExecutionSpec(acpRunnerSpec()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"CODEX_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "CLAUDE_API_KEY", "SUMA_MODEL_API_KEY", "AZURE_OPENAI_API_KEY", "GEMINI_API_KEY", "QWEN_API_KEY", "HOME", "PATH", "GIT_CONFIG_GLOBAL"} {
		spec := acpRunnerSpec()
		spec.ACP.CredentialEnvRef = name
		if validateACPExecutionSpec(spec) == nil {
			t.Fatal("protected tool/provider/system credential reference accepted")
		}
	}
}

func TestACPConfigurationDigestBindsModelVersionAndCredentialReference(t *testing.T) {
	first := acpRunnerSpec()
	capability, err := localACPCapability(first)
	if err != nil || capability == nil || capability.Model != first.ACP.Model || capability.BudgetMode != "observe_only" || capability.IdentityEvidence != "configuration_ack_only" {
		t.Fatal("fixed ACP capability missing")
	}
	for _, change := range []func(*ACPExecutionSpec){func(s *ACPExecutionSpec) { s.Model = "other" }, func(s *ACPExecutionSpec) { s.ReviewedVersion = "other-version" }, func(s *ACPExecutionSpec) { s.CredentialEnvRef = "LOCAL_QODER_CREDENTIAL" }} {
		next := *first.ACP
		change(&next)
		if ACPConfigurationDigest(next) == capability.ConfigurationSHA256 {
			t.Fatal("ACP fixed configuration changed without changing its digest")
		}
	}
	encoded, _ := json.Marshal(capability)
	if strings.Contains(string(encoded), "executable") || strings.Contains(string(encoded), "credential_env_ref") {
		t.Fatal("private executable/credential configuration crossed public capability")
	}
}

func TestACPEnvironmentMapsOnlyReviewedCredentialAndRejectsNodeOverrides(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "platform-secret-must-not-pass")
	t.Setenv("LOCAL_QODER_CREDENTIAL", "local-credential170")
	spec := acpRunnerSpec()
	spec.ACP.CredentialEnvRef = "LOCAL_QODER_CREDENTIAL"
	env, secrets, err := acpExecutionEnvironment(spec)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(env, "\n")
	if !strings.Contains(all, "QODER_PERSONAL_ACCESS_TOKEN=local-credential170") || strings.Contains(all, "LOCAL_QODER_CREDENTIAL=") || strings.Contains(all, "platform-secret-must-not-pass") || len(secrets) != 1 {
		t.Fatal("ACP credential scope was widened or its value was not protected")
	}
	capability, _ := localACPCapability(spec)
	assignment := Assignment{Type: "agent", Config: map[string]any{"acp_execution": capability}}
	if err = acpExecutionRequest(assignment, spec, *capability); err != nil {
		t.Fatal(err)
	}
	assignment.Config["model"] = "unauthorized-override"
	if acpExecutionRequest(assignment, spec, *capability) == nil {
		t.Fatal("node overrode the administrator ACP model")
	}
}
