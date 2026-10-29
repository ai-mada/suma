package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	"testing"
)

func TestTerminalCandidatePublicTemplateCannotGrantSourceAuthority(t *testing.T) {
	f := newFixture(t)
	w := dag.Workflow{ID: "forged-stopped-source", Name: "Untrusted source", Nodes: []dag.Node{{ID: "capture", Type: "command", Command: []string{"fixed-check"}, Config: map[string]any{"terminal_candidate_source": map[string]any{"session_id": "someone-else"}}}}}
	f.call("POST", "/workflows", f.body(map[string]any{"workflow": w}), 400)
	f.call("POST", "/workflows/validate", f.body(map[string]any{"workflow": w}), 400)
	if domainACount(t, f.s, "dag_workflows") != 0 || domainACount(t, f.s, "suma_workspace_terminal_candidate_plans") != 0 {
		t.Fatal("public template granted protected source state")
	}
}
