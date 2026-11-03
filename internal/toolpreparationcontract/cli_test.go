package toolpreparationcontract_test

import (
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
	"strings"
	"testing"
)

func TestManagedCLIOptionsCannotBecomeIgnoredModelOrCredentialOverrides(t *testing.T) {
	for _, id := range []string{"codex-jsonl", "codebuddy-cli", "dsh-multica"} {
		t.Run(id, func(t *testing.T) {
			if tp.ValidateConfiguration(id, tp.Configuration{}) != nil || tp.ConfigurationMode(id) != "existing_local_configuration" {
				t.Fatal("exact native configuration rejected")
			}
			for _, v := range []tp.Configuration{{Model: "claude-sonnet-5"}, {CredentialEnvRef: "TEAM_KEY"}} {
				if tp.ValidateConfiguration(id, v) == nil {
					t.Fatal("unsupported option silently ignored")
				}
			}
			item := ii.Item{AdapterID: id, Protocol: map[string]string{"codex-jsonl": "codex-jsonl-v1", "codebuddy-cli": "codebuddy-jsonl-v1", "dsh-multica": "dsh-multica-jsonl-v1"}[id], Installed: true, LaunchSupported: true, ReasonCode: "workspace_required", Version: "reviewed225", ExecutableSHA256: strings.Repeat("a", 64)}
			if !tp.CanPrepareItem(item) {
				t.Fatal("reviewed original command lost its preparation route")
			}
			item.LaunchSupported = false
			if tp.CanPrepareItem(item) {
				t.Fatal("unverified original command admitted")
			}
		})
	}
	good := tp.Configuration{Model: "claude-sonnet-5"}
	if tp.ValidateConfiguration("claude-code-cli", good) != nil || !tc.ValidClaudeCLIModel(good.Model) {
		t.Fatal("original Claude fixed-model policy rejected")
	}
	for _, v := range []tp.Configuration{{}, {Model: "claude-sonnet-5", CredentialEnvRef: "TEAM_KEY"}, {Model: " claude-sonnet-5"}, {Model: "other-model"}, {Model: strings.Repeat("x", 257)}} {
		if tp.ValidateConfiguration("claude-code-cli", v) == nil {
			t.Fatal("Claude CLI option bypasses original reviewed policy")
		}
	}
	if tp.ValidateConfiguration("claude-code-sdk", good) == nil || tp.ConfigurationMode("codex-app-server") != "" {
		t.Fatal("catalogue entry became an unimplemented factory")
	}
}

func TestManagedCLIProfilesRetainTheirExactProtocolAndCapability(t *testing.T) {
	for _, id := range []string{"codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli"} {
		t.Run(id, func(t *testing.T) {
			p := tc.Profile{CommandID: "managed225", Adapter: tp.AdapterRuntime(id), Protocol: map[string]string{"codex-jsonl": "codex-jsonl-v1", "codebuddy-cli": "codebuddy-jsonl-v1", "dsh-multica": "dsh-multica-jsonl-v1", "claude-code-cli": tc.ClaudeProtocol}[id], ExecutableSHA256: strings.Repeat("a", 64), ProfileSHA256: strings.Repeat("b", 64), ToolVersion: "reviewed225"}
			configuration := tp.Configuration{}
			if id == "claude-code-cli" {
				configuration.Model = "claude-sonnet-5"
				p.ToolVersion = tc.ClaudeCLIVersion
				p.ClaudeExecution = &tc.ClaudeExecutionCapability{Model: configuration.Model, ModelSource: "administrator_cli_argument", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login"}
			}
			if !tp.ProfileMatchesConfiguration(id, configuration, p) {
				t.Fatal("exact original complete profile rejected")
			}
			changed := tc.CloneProfile(p)
			changed.ReadOnly = true
			if tp.ProfileMatchesConfiguration(id, configuration, changed) {
				t.Fatal("write factory borrowed readonly authority")
			}
			changed = tc.CloneProfile(p)
			changed.Adapter = "other"
			if tp.ProfileMatchesConfiguration(id, configuration, changed) {
				t.Fatal("protocol borrowed another adapter")
			}
			changed = tc.CloneProfile(p)
			changed.WorkspaceToolsVersion = "attempt-workspace-v1"
			if tp.ProfileMatchesConfiguration(id, configuration, changed) {
				t.Fatal("preparation added unreviewed capabilities")
			}
			if id == "claude-code-cli" {
				configuration.Model = "claude-opus-5"
				if tp.ProfileMatchesConfiguration(id, configuration, p) {
					t.Fatal("changed fixed model retained original capability")
				}
			}
		})
	}
}
