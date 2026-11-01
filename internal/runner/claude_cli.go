package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const claudeDefaultModel = "claude-sonnet-5"
const claudeSettings = `{"disableAllHooks":true,"permissions":{"blockReadsOutsideWorkingDirectories":true,"blockWritesOutsideWorkingDirectories":true}}`
const claudeTools = "Read,Edit,Write,Glob,Grep,Bash"

// Git recording is an explicit administrator policy, not unrestricted Bash.
const claudeAllowedTools = "Read,Edit,Write,Glob,Grep,Bash(git status *),Bash(git diff *),Bash(git add *),Bash(git commit *)"

func claudeArguments(model string) []string {
	return []string{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--safe-mode", "--restricted", "--settings", claudeSettings, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--permission-mode", "dontAsk", "--permission-prompts", "none", "--tools", claudeTools, "--allowedTools", claudeAllowedTools, "--model", model}
}

func claudePreset() CommandSpec {
	return CommandSpec{Adapter: "claude", Protocol: tc.ClaudeProtocol, Executable: "claude", Input: "goal", Args: claudeArguments(claudeDefaultModel)}
}

func claudePolicyEnvironment() map[string]string {
	return map[string]string{"CLAUDE_CODE_DISABLE_BACKGROUND_TASKS": "1", "CLAUDE_CODE_DISABLE_AGENT_VIEW": "1", "CLAUDE_CODE_DISABLE_CRON": "1", "CLAUDE_CODE_DISABLE_AUTO_MEMORY": "1", "CLAUDE_CODE_DISABLE_CLAUDE_MDS": "1", "CLAUDE_CODE_DISABLE_ARTIFACT": "1", "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS": "1", "DISABLE_UPDATES": "1"}
}

func normalizeClaudeCommand(spec CommandSpec) CommandSpec {
	env := make(map[string]string, len(spec.Env)+7)
	forced := claudePolicyEnvironment()
	for name, value := range spec.Env {
		if _, protected := forced[strings.ToUpper(name)]; !protected {
			env[name] = value
		}
	}
	for name, value := range forced {
		env[name] = value
	}
	spec.Env = env
	return spec
}

func validateClaudeCommand(spec CommandSpec) error {
	if spec.ClaudeSDK != nil || spec.Protocol == tc.ClaudeSDKProtocol {
		return nil // The independent SDK entry has its own fixed policy.
	}
	if spec.Adapter != "claude" && spec.Protocol != tc.ClaudeProtocol {
		return nil
	}
	if spec.Adapter != "claude" || spec.Protocol != tc.ClaudeProtocol || spec.Input != "goal" || spec.ReadOnly || spec.TestEvidence || spec.TestReport || spec.NativeWorkspace != nil || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || spec.TerminalSession || spec.CandidateComposition || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.DispatchRecommendation || spec.TaskConversationReply {
		return fmt.Errorf("Claude CLI requires its fixed foreground development profile")
	}
	name := strings.ToLower(filepath.Base(spec.Executable))
	ext := strings.ToLower(filepath.Ext(spec.Executable))
	if spec.Executable == "" || name == "node" || name == "node.exe" || ext == ".cmd" || ext == ".bat" || ext == ".ps1" || ext == ".js" || ext == ".mjs" || ext == ".cjs" {
		return fmt.Errorf("Claude CLI requires a native executable")
	}
	if len(spec.Args) < 2 {
		return fmt.Errorf("Claude CLI requires fixed safe-mode arguments")
	}
	model := spec.Args[len(spec.Args)-1]
	// A named model is a fixed configuration request, never an alias or gateway.
	if !strings.HasPrefix(model, "claude-") || len(model) > 200 || strings.ContainsAny(model, " /\\\x00\r\n\t") {
		return fmt.Errorf("Claude CLI requires an explicit full model ID")
	}
	for _, r := range model {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_') {
			return fmt.Errorf("Claude CLI model ID is invalid")
		}
	}
	expected := claudeArguments(model)
	if len(spec.Args) != len(expected) {
		return fmt.Errorf("Claude CLI arguments are outside the fixed foreground contract")
	}
	for i := range expected {
		if spec.Args[i] != expected[i] {
			return fmt.Errorf("Claude CLI arguments are outside the fixed foreground contract")
		}
	}
	if len(spec.EnvFrom) != 0 {
		return fmt.Errorf("Claude CLI uses existing local login without inherited credential or policy overrides")
	}
	for name, value := range spec.Env {
		fixed, ok := claudePolicyEnvironment()[name]
		if !ok || value != fixed {
			return fmt.Errorf("Claude CLI environment is outside the fixed foreground policy")
		}
	}
	return nil
}

func localClaudeCapability(spec CommandSpec) (*tc.ClaudeExecutionCapability, error) {
	if spec.ClaudeSDK != nil || spec.Protocol == tc.ClaudeSDKProtocol {
		return nil, nil
	}
	if spec.Adapter != "claude" && spec.Protocol != tc.ClaudeProtocol {
		return nil, nil
	}
	spec = normalizeClaudeCommand(spec)
	if err := validateClaudeCommand(spec); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return &tc.ClaudeExecutionCapability{Model: spec.Args[len(spec.Args)-1], ModelSource: "administrator_cli_argument", ConfigurationSHA256: digest(raw), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login"}, nil
}

func claudeVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string) (string, error) {
	if err := validateClaudeCommand(spec); err != nil {
		return "", err
	}
	version, err := probeCommand(ctx, exe, []string{"--version"}, spec, secrets)
	version = strings.TrimSpace(version)
	if err != nil || tc.ValidateClaudeCLIVersion(version) != nil {
		return "", fmt.Errorf("Claude CLI requires the reviewed 2.1.314 protocol baseline")
	}
	help, err := probeCommand(ctx, exe, []string{"--help"}, spec, secrets)
	if err != nil {
		return "", fmt.Errorf("Claude CLI fixed safety flags could not be probed")
	}
	for _, flag := range []string{"--input-format", "--output-format", "--verbose", "--no-session-persistence", "--safe-mode", "--restricted", "--settings", "--strict-mcp-config", "--mcp-config", "--permission-mode", "--permission-prompts", "--tools", "--allowedTools", "--model"} {
		if !strings.Contains(help, flag) {
			return "", fmt.Errorf("Claude CLI help does not expose the fixed safety contract")
		}
	}
	return version, nil
}
