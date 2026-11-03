package toolcontract

import (
	"strings"
	"testing"
)

func claudeContractProfile() Profile {
	return Profile{CommandID: "claude-code-cli", Adapter: "claude", Protocol: ClaudeProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: ClaudeCLIVersion, ClaudeExecution: &ClaudeExecutionCapability{Model: "claude-sonnet-5", ModelSource: "administrator_cli_argument", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login"}}
}

func TestClaudeProfileClosedConfigurationAndIndependentValues(t *testing.T) {
	p := claudeContractProfile()
	if ValidateProfile(p, true) != nil || !SupportsClaudeExecutionProfile(p) {
		t.Fatal("fixed Claude configuration rejected")
	}
	copy := CloneProfile(p)
	if !EqualProfile(copy, p) {
		t.Fatal("independent equal capabilities differ")
	}
	copy.ClaudeExecution.Model = "claude-opus-5-5"
	if EqualProfile(copy, p) || p.ClaudeExecution.Model != "claude-sonnet-5" {
		t.Fatal("profile capability aliases mutable input")
	}
	// CLI and SDK are both real identities, but their full capabilities are
	// not interchangeable. Reject crossed tuples and then the borrowed profile.
	for _, pair := range [][2]string{{"claude", "codebuddy-jsonl-v1"}, {"codebuddy", ClaudeProtocol}} {
		if ValidCommandIdentity("fixed", pair[0], pair[1]) {
			t.Fatal("invented interchangeable Claude adapter", pair)
		}
	}
	sdkIdentity := CloneProfile(p)
	sdkIdentity.Protocol = ClaudeSDKProtocol
	if !ValidCommandIdentity(sdkIdentity.CommandID, sdkIdentity.Adapter, sdkIdentity.Protocol) || ValidateProfile(sdkIdentity, true) == nil {
		t.Fatal("CLI capability borrowed the real SDK protocol")
	}
	for _, mutate := range []func(*Profile){func(p *Profile) { p.ReadOnly = true }, func(p *Profile) { p.WorkspaceToolsVersion = "attempt-workspace-v1" }, func(p *Profile) { p.ModelExecutionProtocol = "suma-model-execution-v1" }, func(p *Profile) { p.Adapter = "codebuddy"; p.Protocol = "codebuddy-jsonl-v1" }, func(p *Profile) { p.ToolVersion = "2.1.315" }, func(p *Profile) { p.ClaudeExecution.Model = "sonnet" }, func(p *Profile) { p.ClaudeExecution.BudgetMode = "enforce" }, func(p *Profile) { p.ClaudeExecution.AuthStrategy = "api_key_environment" }} {
		q := CloneProfile(p)
		mutate(&q)
		if ValidateProfile(q, true) == nil {
			t.Fatal("unsafe/fake capability accepted", q)
		}
	}
	p.ClaudeExecution = nil
	if ValidateProfile(p, false) != nil || ValidateProfile(p, true) == nil {
		t.Fatal("unavailable probe became complete capability")
	}
}

func TestClaudeEvidenceRequiresExactForegroundCompletion(t *testing.T) {
	p := claudeContractProfile()
	input, output := int64(2), int64(1)
	base := Evidence{Profile: p, SessionID: "session-a", Terminal: "completed", EventCount: 4, Usage: &Usage{InputTokens: &input, OutputTokens: &output}, UsageState: UsagePartial, ClaudeExecution: &ClaudeExecutionEvidence{ConfigurationSHA256: p.ClaudeExecution.ConfigurationSHA256, SessionID: "session-a", RequestedModel: p.ClaudeExecution.Model, AcknowledgedModel: p.ClaudeExecution.Model, ResultSubtype: "success"}}
	if err := ValidateEvidence(base); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Evidence){func(e *Evidence) { e.ClaudeExecution.SessionID = "session-b" }, func(e *Evidence) { e.ClaudeExecution.AcknowledgedModel = "claude-opus-5-5" }, func(e *Evidence) { e.ClaudeExecution.ResultIsError = true }, func(e *Evidence) { e.ClaudeExecution.ResultTerminalReason = "tool_deferred" }, func(e *Evidence) { e.ClaudeExecution.DeferredToolUse = true }, func(e *Evidence) { e.ClaudeExecution.QueuedTurnCount = 1 }, func(e *Evidence) { e.ClaudeExecution.PermissionDenialCount = 1 }, func(e *Evidence) { e.UsageState = UsageComplete }, func(e *Evidence) { e.ClaudeExecution = nil }} {
		q := base
		proof := *base.ClaudeExecution
		q.ClaudeExecution = &proof
		mutate(&q)
		if ValidateEvidence(q) == nil {
			t.Fatal("incomplete Claude turn accepted", q)
		}
	}
	unknown := base
	proof := *base.ClaudeExecution
	unknown.ClaudeExecution = &proof
	unknown.Terminal = "unknown"
	proof.ResultTerminalReason = "background_requested"
	if err := ValidateEvidence(unknown); err != nil {
		t.Fatal("unknown evidence lost original terminal reason", err)
	}
}

func TestClaudeNodeConfigurationRejectsOverridesAndUnknownFields(t *testing.T) {
	p := claudeContractProfile()
	config := map[string]any{"claude_execution": p.ClaudeExecution}
	fixed, err := DecodeClaudeNodeConfiguration(config)
	if err != nil || fixed == nil || *fixed != *p.ClaudeExecution {
		t.Fatal("fixed node binding rejected", err)
	}
	for _, key := range []string{"env", "permission_mode", "model", "max_cost", "resume", "tools", "mcp_servers", "codex_execution", "opencode_execution", "octop_execution"} {
		q := map[string]any{"claude_execution": p.ClaudeExecution, key: nil}
		if _, err := DecodeClaudeNodeConfiguration(q); err == nil {
			t.Fatal("node override accepted", key)
		}
	}
	if _, err := DecodeClaudeNodeConfiguration(map[string]any{"claude_execution": map[string]any{"model": "claude-sonnet-5", "arbitrary": "payload"}}); err == nil {
		t.Fatal("unknown nested capability accepted")
	}
}

func TestClaudeCatalogDoesNotMergeCLIWithSDKOrModelGateway(t *testing.T) {
	cli, sdk, client := catalogEntry(t, "claude-code-cli"), catalogEntry(t, "claude-code-sdk"), catalogEntry(t, "claude-code-client")
	if cli.Adapter != "claude" || cli.Protocol != ClaudeProtocol || cli.Implementation != "implemented" || cli.SetupKind != "runner_profile" || sdk.Implementation != "planned" || client.Direction != "tool_to_suma" {
		t.Fatal("Claude identities were merged", cli, sdk, client)
	}
	for _, key := range []string{"task_start", "terminal_observation", "usage_observation"} {
		if catalogCapability(t, cli, key) != "supported" {
			t.Fatal("actual foreground capability missing", key)
		}
	}
	for _, key := range []string{"model_selection", "gateway_budget", "history_read", "session_load", "new_turn", "interrupted_resume", "human_input", "provider_billing", "independent_test", "workspace_read", "workspace_write"} {
		if catalogCapability(t, cli, key) != "unsupported" {
			t.Fatal("CLI invented independent capability", key)
		}
	}
}
