package platform

import (
	"context"
	"database/sql"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// All three origins are current product capabilities. Exactly one accepted origin
// is allowed; a terminal candidate cannot impersonate a multi-branch merge.
func (s *Service) codeBaselineAcceptedOriginTx(ctx context.Context, tx *sql.Tx, p Principal, project string, b ccmp.CodeBaselineBinding) (RepositoryIntegrationBasis, error) {
	if b.TerminalCandidatePlanID != "" && b.TerminalCandidatePlanVersion > 0 && b.CompositionID == "" && b.CompositionVersion == 0 && b.SourceTaskID == "" && b.SourceTaskVersion == 0 {
		return s.terminalCandidateAcceptedBasisTx(ctx, tx, p, project, b.TerminalCandidatePlanID, b.TerminalCandidatePlanVersion)
	}
	if b.CompositionID != "" && b.CompositionVersion > 0 && b.TerminalCandidatePlanID == "" && b.TerminalCandidatePlanVersion == 0 && b.SourceTaskID == "" && b.SourceTaskVersion == 0 {
		return s.compositionAcceptedBasisTx(ctx, tx, p, project, b.CompositionID, b.CompositionVersion)
	}
	if b.SourceTaskID != "" && b.SourceTaskVersion > 0 && b.CompositionID == "" && b.CompositionVersion == 0 && b.TerminalCandidatePlanID == "" && b.TerminalCandidatePlanVersion == 0 {
		return s.acceptedTaskCandidateBasisTx(ctx, tx, p, project, b.SourceTaskID, b.SourceTaskVersion, b.Source.Source.NodeID)
	}
	return RepositoryIntegrationBasis{}, compositionError(409, "baseline_changed", compositionBaseline)
}
