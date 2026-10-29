package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// Public templates describe fixed checks. Cross-run stopped source authority
// is added only by the server's protected preview, never by template JSON.
func validatePublicTerminalCandidateWorkflow(workflow dag.Workflow) error {
	for _, n := range workflow.Nodes {
		if _, declared := n.Config[term.CandidateConfigKey]; declared {
			return invalid("Terminal candidate sources require an explicitly reviewed stopped-session plan")
		}
	}
	return nil
}
