package dag

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestCodexBindingRequiresExactAgentNoRetriesOrNativeAllowances(t *testing.T) {
	catalog, err := cc.NewCatalog(strings.Repeat("a", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	node := func() Node {
		return Node{ID: "develop", Type: "agent", Command: []string{"codex-code"}, Config: map[string]any{"tool_profile_id": "approved-codex", "tool_profile_sha256": strings.Repeat("b", 64), "codex_execution": binding}}
	}
	if err = Compile(Workflow{Nodes: []Node{node()}}); err != nil {
		t.Fatal("valid fixed Codex node was rejected", err)
	}
	for _, kind := range []string{"retry", "native-output", "native-steps", "unbound", "malformed", "model", "model_id", "effort", "reasoning_effort", "model_provider", "provider", "base_url", "endpoint", "api_key", "env", "env_from", "cwd", "argv", "model_execution", "service_tier", "config", "configuration", "sandbox", "approval_policy"} {
		t.Run(kind, func(t *testing.T) {
			n := node()
			switch kind {
			case "retry":
				n.MaxRetries, n.Idempotent = 1, true
			case "native-output":
				n.Config["max_output_tokens"] = 100
			case "native-steps":
				n.Config["max_steps"] = 8
			case "unbound":
				delete(n.Config, "tool_profile_id")
				delete(n.Config, "tool_profile_sha256")
			case "malformed":
				n.Config["codex_execution"] = nil
			default:
				n.Config[kind] = "explicit override"
			}
			if err := Compile(Workflow{Nodes: []Node{n}}); err == nil {
				t.Fatal("invalid Codex execution binding compiled")
			}
		})
	}
	e := &tc.Evidence{Profile: tc.Profile{CommandID: "codex-code", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("b", 64), ExecutableSHA256: strings.Repeat("c", 64), ToolVersion: "Codex protocol fixture"}, SessionID: "thread-original", Terminal: "completed", EventCount: 4, UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: binding.BindingSHA256, ThreadID: "thread-original", TurnID: "turn-original", RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort, AcknowledgedModel: binding.Model, AcknowledgedReasoningEffort: binding.ReasoningEffort, AcknowledgedProvider: "configured-provider", ConfigurationAcknowledged: true}}
	result := Result{ToolEvidence: e}
	if err := validateToolResult(node(), result); err != nil {
		t.Fatal("exact acknowledged completed turn was rejected", err)
	}
	e.CodexExecution.TurnID = ""
	if err := validateToolResult(node(), result); err == nil {
		t.Fatal("missing turn ACK proved successful execution")
	}
	result.OutcomeUnknown, e.Terminal = true, "unknown"
	if err := validateToolResult(node(), result); err != nil {
		t.Fatal("unknown ACK discarded original configuration facts", err)
	}
}
