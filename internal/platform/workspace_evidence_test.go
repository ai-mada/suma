package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	"strings"
	"testing"
)

func TestCodeAcceptanceRequiresExactCleanCandidateAndAdminTest(t *testing.T) {
	code := reviewWorkspaceEvidence("c", false)
	tested := reviewWorkspaceEvidence("c", true)
	tested["base_commit"] = strings.Repeat("c", 40)
	valid := dag.Run{Nodes: []dag.NodeRun{{ID: "code", Type: "command", State: "succeeded", WorkspaceEvidence: code}, {ID: "test", Type: "test", State: "succeeded", WorkspaceEvidence: tested}}}
	if err := validateCodeTestEvidence(valid); err != nil {
		t.Fatal("same committed revision should be certifiable", err)
	}
	for _, scenario := range []string{"missing-code", "missing-test", "dirty-code", "unknown", "uncommitted-test", "wrong-candidate", "not-admin-test", "invalid-hash"} {
		t.Run(scenario, func(t *testing.T) {
			a := reviewWorkspaceEvidence("c", false)
			b := reviewWorkspaceEvidence("c", true)
			b["base_commit"] = strings.Repeat("c", 40)
			switch scenario {
			case "missing-code":
				a = nil
			case "missing-test":
				b = nil
			case "dirty-code":
				a["clean"] = false
			case "unknown":
				a["outcome_unknown"] = true
			case "uncommitted-test":
				b["head_commit"] = strings.Repeat("d", 40)
			case "wrong-candidate":
				b["base_commit"] = strings.Repeat("b", 40)
				b["head_commit"] = strings.Repeat("b", 40)
			case "not-admin-test":
				b["test_command"] = false
			case "invalid-hash":
				a["evidence_sha256"] = "bad"
			}
			run := dag.Run{Nodes: []dag.NodeRun{{Type: "agent", State: "succeeded", WorkspaceEvidence: a}, {Type: "test", State: "succeeded", WorkspaceEvidence: b}}}
			if err := validateCodeTestEvidence(run); err == nil {
				t.Fatal("unproved code accepted")
			}
		})
	}
}

func TestTestOnlyNonCodeFactsCannotCertifyCode(t *testing.T) {
	if err := validateCodeTestEvidence(dag.Run{Nodes: []dag.NodeRun{{Type: "test", State: "succeeded"}}}); err != nil {
		t.Fatal("non-code test facts changed", err)
	}
	if err := validateCodeTestEvidence(dag.Run{Nodes: []dag.NodeRun{{Type: "command", State: "succeeded"}, {Type: "test", State: "succeeded"}}}); err == nil {
		t.Fatal("non-code facts certified code")
	}
}
