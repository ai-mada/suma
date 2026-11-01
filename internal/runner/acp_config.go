package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type ACPExecutionSpec struct {
	Runtime          string             `json:"runtime"`
	ReviewedVersion  string             `json:"reviewed_version"`
	Model            string             `json:"model"`
	CredentialEnvRef string             `json:"credential_env_ref,omitempty"`
	CursorPackage    *CursorPackageSpec `json:"cursor_package,omitempty"`
}

func acpLocalText(value string, maximum int) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= maximum && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}
func acpReservedCredentialEnv(name string) bool {
	for _, prefix := range []string{"SUMA_", "CODEX", "OPENAI", "ANTHROPIC", "CLAUDE", "AZURE_", "GEMINI", "GOOGLE_", "QWEN", "DASHSCOPE", "DEEPSEEK", "OPENCODE", "DSH_", "WORKBUDDY", "CODEBUDDY", "AWS_", "HUGGINGFACE", "HF_", "GIT_", "NODE_", "PYTHON"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return map[string]bool{"HOME": true, "USERPROFILE": true, "PATH": true, "APPDATA": true, "LOCALAPPDATA": true, "TEMP": true, "TMP": true, "TMPDIR": true, "COMSPEC": true, "SYSTEMROOT": true, "WINDIR": true, "PATHEXT": true, "HOMEDRIVE": true, "HOMEPATH": true, "SYSTEMDRIVE": true, "PROGRAMDATA": true, "PROGRAMFILES": true, "PUBLIC": true, "LANG": true, "GOFLAGS": true, "GOCACHE": true, "GOTMPDIR": true}[name]
}
func validateACPExecutionSpec(spec CommandSpec) error {
	if spec.ACP == nil && spec.Adapter != "qoder" && spec.Adapter != "cursor" && spec.Protocol != ac.Protocol && spec.Input != "acp-stdio" {
		return nil
	}
	p := spec.ACP
	if p == nil || p.Runtime != "qoder" && p.Runtime != "cursor" || spec.Adapter != p.Runtime || spec.Protocol != ac.Protocol || spec.Input != "acp-stdio" || spec.ReadOnly || spec.NativeWorkspace != nil || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.DispatchRecommendation || spec.TaskConversationReply || spec.TerminalSession || spec.CandidateComposition || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 || !acpLocalText(p.ReviewedVersion, 256) || !acpLocalText(p.Model, 256) {
		return fmt.Errorf("ACP requires its fixed reviewed native foreground configuration")
	}
	if p.Runtime == "qoder" {
		if p.CursorPackage != nil || len(spec.Args) != 1 || spec.Args[0] != "--acp" {
			return fmt.Errorf("Qoder requires its fixed native ACP entry")
		}
	} else if err := validateCursorCommand(spec); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		extension := strings.ToLower(filepath.Ext(spec.Executable))
		if extension != "" && extension != ".exe" {
			return fmt.Errorf("ACP Windows entry must be a native executable")
		}
	}
	if p.CredentialEnvRef != "" {
		name := strings.ToUpper(p.CredentialEnvRef)
		if name != p.CredentialEnvRef || !modelEnvName.MatchString(p.CredentialEnvRef) || acpReservedCredentialEnv(name) {
			return fmt.Errorf("ACP credential reference must be a separate local credential")
		}
	}
	return nil
}
func ACPConfigurationDigest(spec ACPExecutionSpec) string {
	policy := "minimal-os-existing-cli-login-explicit-credential-v1"
	if spec.Runtime == "cursor" {
		policy = "cursor-isolated-home-explicit-api-key-package-closure-" + cu.InputProtocol
	}
	raw, _ := json.Marshal(struct {
		Spec        ACPExecutionSpec `json:"spec"`
		Environment string           `json:"environment_policy"`
	}{spec, policy})
	return digest(raw)
}
func localACPCapability(spec CommandSpec) (*ac.Capability, error) {
	if err := validateACPExecutionSpec(spec); err != nil {
		return nil, err
	}
	if spec.ACP == nil {
		return nil, nil
	}
	auth := "existing_cli_login"
	if spec.ACP.Runtime == "cursor" {
		auth = "explicit_api_key_env"
	}
	return &ac.Capability{Runtime: spec.ACP.Runtime, ProtocolVersion: 1, SDKVersion: ac.SDKVersion, ConfigurationSHA256: ACPConfigurationDigest(*spec.ACP), Model: spec.ACP.Model, ModelSource: "administrator_session_config", BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: auth, PermissionMode: "human_once"}, nil
}
func acpVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string) (string, error) {
	if err := validateACPExecutionSpec(spec); err != nil {
		return "", err
	}
	if spec.ACP.Runtime == "cursor" {
		return cursorVersion(ctx, spec, secrets)
	}
	version, err := probeCommandStreams(ctx, exe, []string{"--version"}, CommandSpec{}, secrets, true)
	version = strings.TrimSpace(version)
	if err != nil || !acpLocalText(version, 256) || version != spec.ACP.ReviewedVersion {
		return "", fmt.Errorf("ACP executable version differs from its exact administrator review")
	}
	return version, nil
}
func acpExecutionEnvironment(spec CommandSpec) ([]string, []string, error) {
	if err := validateACPExecutionSpec(spec); err != nil {
		return nil, nil, err
	}
	if spec.ACP.Runtime == "cursor" {
		return nil, nil, fmt.Errorf("Cursor requires its fresh isolated configuration home")
	}
	environment, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return nil, nil, err
	}
	secrets := []string{}
	if spec.ACP.CredentialEnvRef != "" {
		value, ok := os.LookupEnv(spec.ACP.CredentialEnvRef)
		if !ok || value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return nil, nil, fmt.Errorf("ACP reviewed credential reference is unavailable")
		}
		environment = append(environment, "QODER_PERSONAL_ACCESS_TOKEN="+value)
		secrets = append(secrets, value)
	}
	return environment, secrets, nil
}
func acpExecutionRequest(a Assignment, spec CommandSpec, capability ac.Capability) error {
	fixed, err := tc.DecodeACPNodeConfiguration(a.Config)
	if fixed == nil && err == nil && spec.ACP == nil {
		return nil
	}
	if a.Type != "agent" || spec.ACP == nil || validateACPExecutionSpec(spec) != nil {
		return fmt.Errorf("ACP assignment requires its approved isolated agent profile")
	}
	if err != nil || fixed == nil || *fixed != capability {
		return fmt.Errorf("ACP fixed configuration differs from its current inventory")
	}
	return nil
}
