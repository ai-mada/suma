package runner

import (
	"context"
	"fmt"
	"path/filepath"

	rc "github.com/ai-mada/suma/internal/releasecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// Reject ambiguous execution before special-purpose handlers can launch.
func applicationBuildAssignment(a Assignment) (*rc.ApplicationBuildBasis, error) {
	basis, err := rc.DecodeApplicationBuild(a.Config)
	if err != nil || basis == nil {
		return basis, err
	}
	_, repositorySource := a.Config[ri.RepositorySourceExecutionKey]
	if a.Type != "command" || repositorySource || cloudReserved(a) || a.WorkBuddyCloud != nil ||
		a.RepositoryIntegration != nil || a.ReleaseExecution != nil || a.CodeBaseline != nil ||
		a.DevelopmentContinuation != nil || a.CandidateComposition != nil || a.CandidateConflictRepair != nil || a.ExecutionSkills != nil {
		return nil, fmt.Errorf("application build requires a dedicated project command")
	}
	return basis, nil
}

func validApplicationBuildCommand(spec CommandSpec) bool {
	return dedicatedArtifactCommand(spec) && spec.ArtifactOutput != "" && !spec.ArtifactInput && !spec.TestEvidence
}

func validPoolPackageTestCommand(spec CommandSpec) bool {
	return dedicatedArtifactCommand(spec) && spec.ArtifactOutput == "" && spec.ArtifactInput && spec.TestEvidence
}

func dedicatedArtifactCommand(spec CommandSpec) bool {
	spec = normalizedCommand(spec)
	return spec.Adapter == "command" && spec.Protocol == "text-v1" && !spec.ReadOnly && !spec.TestReport &&
		spec.Input == "" && spec.NativeWorkspace == nil && spec.ModelExecutionProtocol == "" &&
		spec.CodexAppServer == nil && spec.OpenCode == nil && spec.ACP == nil && spec.ClaudeSDK == nil &&
		!spec.Planning && !spec.KnowledgeExtraction && !spec.FailureDiagnosis && !spec.IntelligentOperations &&
		!spec.DispatchRecommendation && !spec.TaskConversationReply && !spec.TerminalSession && !spec.CandidateComposition
}

type applicationBuildWorkspace struct {
	basis   *rc.ApplicationBuildBasis
	root    string
	parents []artifactParent
}

func prepareApplicationBuildWorkspace(ctx context.Context, work *attemptWorkspace, basis *rc.ApplicationBuildBasis) (*applicationBuildWorkspace, error) {
	if basis == nil {
		return nil, nil
	}
	if work == nil || work.BaseCommit != basis.SourceCommit {
		return nil, fmt.Errorf("application build requires the registered source commit")
	}
	root := filepath.Join(work.Source, filepath.FromSlash(basis.ScopeRoot))
	parents, err := artifactParents(work.Source, filepath.Join(root, ".suma-scope"))
	if err != nil {
		return nil, fmt.Errorf("application build scope is unavailable or linked")
	}
	scope := &applicationBuildWorkspace{basis: basis, root: root, parents: parents}
	if err = scope.verify(ctx, work); err != nil {
		return nil, err
	}
	return scope, nil
}

// The scope pins the actual command directory and Git tree. Administrator
// command trust is still required for processes that read beyond that directory.
func (s *applicationBuildWorkspace) verify(ctx context.Context, work *attemptWorkspace) error {
	if !artifactParentsStable(s.parents) {
		return fmt.Errorf("application build scope directory changed")
	}
	head, err := gitText(ctx, work.Source, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || head != s.basis.SourceCommit {
		return fmt.Errorf("application build source differs from its registered commit")
	}
	treeRef := "HEAD^{tree}"
	if s.basis.ScopeRoot != "" {
		treeRef = "HEAD:" + s.basis.ScopeRoot
	}
	tree, err := gitText(ctx, work.Source, "rev-parse", "--verify", treeRef)
	if err != nil || tree != s.basis.ScopeTreeOID {
		return fmt.Errorf("application build scope differs from its registered Git tree")
	}
	kind, err := gitText(ctx, work.Source, "cat-file", "-t", tree)
	if err != nil || kind != "tree" {
		return fmt.Errorf("application build scope is not a Git tree")
	}
	return nil
}
