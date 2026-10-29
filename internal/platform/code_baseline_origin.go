package platform

import (
	"context"
	"database/sql"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// Both origins are current product capabilities. Exactly one accepted origin
// is allowed; a terminal candidate cannot impersonate a multi-branch merge.
func (s *Service) codeBaselineAcceptedOriginTx(ctx context.Context, tx *sql.Tx, p Principal, project string, b ccmp.CodeBaselineBinding) (RepositoryIntegrationBasis, error) {
	if b.TerminalCandidatePlanID != "" && b.TerminalCandidatePlanVersion > 0 && b.CompositionID == "" && b.CompositionVersion == 0 {
		return s.terminalCandidateAcceptedBasisTx(ctx, tx, p, project, b.TerminalCandidatePlanID, b.TerminalCandidatePlanVersion)
	}
	if b.CompositionID != "" && b.CompositionVersion > 0 && b.TerminalCandidatePlanID == "" && b.TerminalCandidatePlanVersion == 0 {
		return s.compositionAcceptedBasisTx(ctx, tx, p, project, b.CompositionID, b.CompositionVersion)
	}
	return RepositoryIntegrationBasis{}, compositionError(409, "baseline_changed", compositionBaseline)
}
