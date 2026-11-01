package runner

import (
	"context"
	"fmt"
	"path/filepath"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// A reviewed continuation is a direct code input, never accepted delivery or a
// generic workflow configuration. Keep it separate from CodeBaseline authority.
func developmentContinuationInput(a Assignment) error {
	_, raw := a.Config["development_continuation"]
	marker, marked := a.Config["development_continuation_id"]
	b := a.DevelopmentContinuation
	if b == nil && !raw && !marked {
		return nil
	}
	_, purposePresent := a.Config["purpose"]
	if raw || b == nil || dc.ValidateBinding(*b) != nil || !marked || marker != b.ID || a.Type != "agent" || purposePresent || cloudReserved(a) || a.WorkBuddyCloud != nil || a.CodeBaseline != nil || a.CandidateConflictRepair != nil || a.CandidateComposition != nil || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || hasCandidate(a) {
		return fmt.Errorf("development continuation requires its exact reviewed binding and a dedicated development agent")
	}
	return nil
}

func developmentContinuationSpec(a Assignment, spec CommandSpec) error {
	if err := developmentContinuationInput(a); err != nil {
		return err
	}
	if a.DevelopmentContinuation == nil {
		return nil
	}
	spec = normalizedCommand(spec)
	// These are the existing structured development adapters. The subsequent
	// ordinary launch checks still prove their exact local profile and protocol.
	structured := spec.Adapter == "suma" || spec.Adapter == "codex" || spec.Adapter == "harness" || spec.Adapter == "codebuddy" || spec.Adapter == "qoder" || spec.Adapter == "claude" || spec.Adapter == "opencode" || spec.Adapter == "octop"
	if !structured || !tc.ValidCommandIdentity("development", spec.Adapter, spec.Protocol) || spec.ReadOnly || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.DispatchRecommendation || spec.TaskConversationReply || spec.TerminalSession || spec.CandidateComposition || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" {
		return fmt.Errorf("development continuation requires an ordinary isolated writable development profile")
	}
	return nil
}

func cloneDevelopmentContinuation(b *dc.DevelopmentContinuationBinding) *dc.DevelopmentContinuationBinding {
	if b == nil {
		return nil
	}
	copy := *b
	return &copy
}

func importDevelopmentContinuation(ctx context.Context, repo string, a Assignment, cfg ExecConfig) (string, *cc.ImportReceipt, error) {
	b := a.DevelopmentContinuation
	if developmentContinuationInput(a) != nil || b == nil || cfg.Artifacts == nil || b.ProjectID != cfg.Artifacts.ProjectID || b.Source.BundleBase() != cfg.BaseCommit {
		return "", nil, fmt.Errorf("development continuation requires its authorized source and administrator bundle anchor")
	}
	dir := filepath.Join(repo, ".cache", "runner", "code-inputs", attemptKey(a))
	if err := safeMkdir(dir); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "continuation.bundle")
	// Local object presence grants no authority. Every new attempt rechecks the
	// original source and current consumer lease through the existing broker.
	if err := cfg.Artifacts.codeInputBundle(ctx, a, "development_continuation", b.ID, b.Source, path); err != nil {
		return "", nil, err
	}
	if err := importCandidateFile(ctx, repo, path, b.Source); err != nil {
		return "", nil, err
	}
	return b.Source.HeadCommit, &cc.ImportReceipt{Binding: b.Source, Imported: true, TargetWorkerID: a.WorkerID}, nil
}
