package toolinstallationcontract_test

import (
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	"testing"
)

func TestProbeDiagnosticCannotInventSuccessOrAuthorizeRepair(t *testing.T) {
	valid := ii.ProbeDiagnostic{Stage: "help", Code: "help_contract_mismatch", NextAction: "guide"}
	item := ii.Item{AdapterID: "codex-jsonl", Protocol: "codex-jsonl-v1", Installed: true, ReasonCode: "probe_failed", Diagnostic: &valid}
	if ii.ValidateItem(item) != nil {
		t.Fatal("classified failed metadata is not accepted")
	}
	for _, bad := range []ii.ProbeDiagnostic{
		{Stage: "version", Code: "help_contract_mismatch", NextAction: "guide"},
		{Stage: "help", Code: "help_contract_mismatch", NextAction: "enable"},
		{Stage: "help", Code: "stop_unverified", NextAction: "scan"},
		{Stage: "help", Code: "C:/private/secret-value", NextAction: "guide"},
		{Stage: "exec", Code: "probe_failed", NextAction: "guide"},
	} {
		item.Diagnostic = &bad
		if ii.ValidateItem(item) == nil {
			t.Fatal("unobserved phase or unauthorized instruction accepted", bad)
		}
	}
	item.Diagnostic = &valid
	item.ReasonCode = "not_installed"
	item.Installed = false
	if ii.ValidateItem(item) == nil {
		t.Fatal("version/help failure became proof of missing installation")
	}
	item.ReasonCode = "workspace_required"
	item.Installed, item.LaunchSupported, item.Version = true, true, "observed"
	if ii.ValidateItem(item) == nil {
		t.Fatal("failed metadata became a launch-ready source")
	}
	incomplete := ii.Item{AdapterID: "codex-jsonl", Protocol: "codex-jsonl-v1", ReasonCode: "scan_incomplete", Diagnostic: &ii.ProbeDiagnostic{Stage: "resolve", Code: "source_unavailable", NextAction: "guide"}}
	if ii.ValidateItem(incomplete) != nil {
		t.Fatal("unknown installation resolution lost its explicit state")
	}
	without := ii.Item{AdapterID: "codex-jsonl", Protocol: "codex-jsonl-v1", Installed: true, ReasonCode: "probe_failed"}
	if ii.ValidateItem(without) != nil {
		t.Fatal("an unclassified observation was forced to invent a cause")
	}
}
