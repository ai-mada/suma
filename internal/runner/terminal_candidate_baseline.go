package runner

import (
	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func sameTerminalCodeBaseline(a, b *ccc.CodeBaselineBinding) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func cloneTerminalCodeBaseline(b *ccc.CodeBaselineBinding) *ccc.CodeBaselineBinding {
	if b == nil {
		return nil
	}
	copy := *b
	return &copy
}

// The selected accepted commit and administrator bundle prerequisite remain
// separate facts. An ordinary terminal cannot choose another base by SHA.
func terminalWorkspaceBasis(a Assignment, selected, admin string) bool {
	if !commitID.MatchString(selected) || !commitID.MatchString(admin) {
		return false
	}
	if a.CodeBaseline == nil {
		return selected == admin
	}
	b := a.CodeBaseline
	return ccc.ValidateCodeBaselineBinding(*b) == nil && b.Source.HeadCommit == selected && b.Source.BundleBase() == admin
}

func terminalCandidateBaseline(a Assignment) bool {
	if a.Type != "command" || a.Attempt != 1 || a.Idempotent || len(a.Command) != 1 || a.CodeBaseline == nil || a.CandidateComposition != nil || a.CandidateConflictRepair != nil || a.ReleaseExecution != nil || a.RepositoryIntegration != nil || hasCandidate(a) {
		return false
	}
	source, err := term.DecodeCandidateSource(a.Config[term.CandidateConfigKey])
	return err == nil && source != nil && source.CodeBaseline != nil && sameTerminalCodeBaseline(source.CodeBaseline, a.CodeBaseline) && terminalWorkspaceBasis(a, source.BaseCommit, source.AdminBaseCommit)
}

// The service independently proves protected terminal node kinds against
// frozen domain records. Config alone never supplies the typed baseline input.
func codeBaselineInputNode(a Assignment) bool {
	if a.CodeBaseline == nil || ccc.ValidateCodeBaselineBinding(*a.CodeBaseline) != nil {
		return false
	}
	if a.Type == "agent" {
		return true
	}
	if a.Type == "terminal_session" {
		id, _ := a.Config["terminal_session_id"].(string)
		_, candidate := a.Config[term.CandidateConfigKey]
		return id != "" && a.NodeID == "terminal" && a.Attempt == 1 && a.Fence > 0 && a.DispatchID != "" && !a.Idempotent && len(a.Command) == 1 && !candidate && a.CandidateComposition == nil && a.CandidateConflictRepair == nil && a.ReleaseExecution == nil && a.RepositoryIntegration == nil && !hasCandidate(a)
	}
	return terminalCandidateBaseline(a)
}
