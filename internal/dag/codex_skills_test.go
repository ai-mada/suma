package dag

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func codexSkillNode(t *testing.T) (Node, *cc.ExecutionSkillBundle, cc.Binding) {
	t.Helper()
	catalog, err := cc.NewCatalog(strings.Repeat("a", 64), []cc.ModelOption{{Model: "codex-a", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "codex-a", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := cc.BuildExecutionSkillBundle([]cc.ExecutionSkillDocument{{ID: "approved-skill", Revision: 3, SHA256: strings.Repeat("d", 64), Title: "Independent review", Content: "Review the exact candidate, keep human acceptance separate."}})
	if err != nil {
		t.Fatal(err)
	}
	node := Node{ID: "develop", Type: "agent", Command: []string{"codex-code"}, Config: map[string]any{"tool_profile_id": "approved-codex", "tool_profile_sha256": strings.Repeat("b", 64), "codex_execution": binding, "execution_skills": bundle}}
	return node, bundle, binding
}

func TestCodexNativeSkillCompilationRequiresBoundAgent(t *testing.T) {
	node, _, _ := codexSkillNode(t)
	if err := Compile(Workflow{Nodes: []Node{node}}); err != nil {
		t.Fatal("valid reviewed skill node rejected", err)
	}
	for _, mode := range []string{"noncodex", "command", "null", "digest"} {
		t.Run(mode, func(t *testing.T) {
			node, bundle, _ := codexSkillNode(t)
			switch mode {
			case "noncodex":
				delete(node.Config, "codex_execution")
			case "command":
				node.Type = "command"
			case "null":
				node.Config["execution_skills"] = nil
			case "digest":
				bundle.Documents[0].Content = "unreviewed content"
			}
			if err := Compile(Workflow{Nodes: []Node{node}}); err == nil {
				t.Fatal("unbound or changed native skill node compiled")
			}
		})
	}
}

func TestCodexNativeSkillResultNeedsExactCompleteEvidence(t *testing.T) {
	node, bundle, binding := codexSkillNode(t)
	skills := &cc.ExecutionSkillEvidence{BundleSHA256: bundle.BundleSHA256, Documents: cc.ExecutionSkillReceipts(*bundle), Materialized: true, CatalogAcknowledged: true, InvocationRequested: true, TurnAcknowledged: true, FilesVerified: true}
	evidence := &cc.Evidence{BindingSHA256: binding.BindingSHA256, ThreadID: "thread", TurnID: "turn", RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort, AcknowledgedModel: binding.Model, AcknowledgedReasoningEffort: binding.ReasoningEffort, AcknowledgedProvider: "configured-provider", ConfigurationAcknowledged: true, Skills: skills}
	result := Result{ToolEvidence: &tc.Evidence{CodexExecution: evidence}}
	if err := validateCodexResult(node, result); err != nil {
		t.Fatal("complete exact skill receipt rejected", err)
	}
	for _, mode := range []string{"missing", "materialized", "catalog", "invocation", "turn", "files", "wrong-reference"} {
		t.Run(mode, func(t *testing.T) {
			copy := cc.CloneEvidence(evidence)
			switch mode {
			case "missing":
				copy.Skills = nil
			case "materialized":
				copy.Skills.Materialized = false
			case "catalog":
				copy.Skills.CatalogAcknowledged = false
			case "invocation":
				copy.Skills.InvocationRequested = false
			case "turn":
				copy.Skills.TurnAcknowledged = false
			case "files":
				copy.Skills.FilesVerified = false
			case "wrong-reference":
				copy.Skills.Documents[0].Revision++
			}
			if err := validateCodexResult(node, Result{ToolEvidence: &tc.Evidence{CodexExecution: copy}}); err == nil {
				t.Fatal("incomplete or wrong skill receipt became successful result")
			}
		})
	}
	delete(node.Config, "execution_skills")
	if validateCodexResult(node, result) == nil {
		t.Fatal("unbound result smuggled native skill evidence")
	}
}
