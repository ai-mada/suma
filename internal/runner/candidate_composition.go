package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// Composition is a host implementation selected by an explicit local allowlist,
// never an executable, arguments or merge policy supplied by a workflow/model.
func validateCompositionCommand(id string, spec CommandSpec) error {
	if !spec.CandidateComposition && id != ccc.CompositionCommand {
		return nil
	}
	spec = normalizedCommand(spec)
	if id != ccc.CompositionCommand || !spec.CandidateComposition || spec.Adapter != "command" || spec.Protocol != "text-v1" || spec.Executable == "" || len(spec.Args) != 0 || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 || spec.Input != "" || spec.ReadOnly || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.TestEvidence || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.NativeWorkspace != nil {
		return fmt.Errorf("candidate composition requires the dedicated fixed local Git command")
	}
	return nil
}

func probeCompositionGit(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, compositionGitExecutableScope{}, executable)
	version, err := compositionText(ctx, "", "--version")
	if err != nil || !strings.HasPrefix(version, "git version ") || len(version) > 200 {
		return "", fmt.Errorf("candidate composition Git version probe failed")
	}
	// Git's short usage exits 129. Its actual installed help must advertise the
	// required object-writing mode; no repository, network or model is involved.
	cmd := nativeGitCommand(ctx, "", "merge-tree", "-h")
	out := newOutput(32<<10, nil)
	cmd.Stdout = out
	cmd.Stderr = out
	if toolProbeCollectorFrom(ctx) != nil {
		_, stopErr := runProbeProcess(ctx, cmd)
		if stopErr != nil {
			return "", fmt.Errorf("candidate composition probe stop unverified")
		}
	} else {
		_ = cmd.Run()
	}
	help := out.b.Bytes()
	if ctx.Err() != nil || out.truncated || !strings.Contains(string(help), "--write-tree") || !strings.Contains(string(help), "--name-only") {
		return "", fmt.Errorf("candidate composition requires Git merge-tree write-tree support")
	}
	return version, nil
}

func executeCandidateComposition(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec) Result {
	r := Result{Assignment: a, ExitCode: -1}
	fail := func(err error) Result { r.Error = err.Error(); return r }
	if validateCompositionCommand(ccc.CompositionCommand, spec) != nil || len(a.Command) != 1 || a.Command[0] != ccc.CompositionCommand || a.Type != "command" || a.CandidateComposition == nil || a.CodeBaseline != nil || hasCandidate(a) || a.RepositoryIntegration != nil || a.ReleaseExecution != nil || cfg.Artifacts == nil {
		return fail(fmt.Errorf("candidate composition requires its authenticated dedicated execution"))
	}
	e := *a.CandidateComposition
	if ccc.ValidateCandidateCompositionExecution(e) != nil || e.ProjectID != cfg.Artifacts.ProjectID || e.BaseCommit != cfg.BaseCommit || a.Config["purpose"] != "candidate_composition" {
		return fail(fmt.Errorf("candidate composition execution differs from the fixed administrator scope"))
	}
	profile, executable, err := launchProfile(ctx, a, cfg, normalizedCommand(spec))
	if err != nil {
		return fail(err)
	}
	gitSHA, version := profile.ExecutableSHA256, profile.ToolVersion
	duration := time.Duration(a.TimeoutSeconds) * time.Second
	if duration <= 0 {
		duration = 10 * time.Minute
	}
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	ctx = context.WithValue(ctx, nativeGitScope{}, true)
	ctx = context.WithValue(ctx, compositionGitExecutableScope{}, executable)
	w, err := prepareWorkspace(ctx, a, cfg, spec)
	if err != nil {
		return fail(err)
	}
	w.compositionClient = cfg.Artifacts
	w.compositionGitVersion = version
	w.compositionGitSHA256 = gitSHA
	facts, composeErr := composeCandidateTrees(ctx, w, e, gitSHA)
	w.CompositionEvidence = &facts
	if composeErr == nil {
		r.ExitCode = 0
	} else if errors.Is(composeErr, errCompositionConflict) {
		r.ExitCode = 1
		r.Error = errCompositionConflict.Error()
	} else {
		r.Error = composeErr.Error()
		r.OutcomeUnknown = true
	}
	if ctx.Err() != nil {
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "candidate composition interrupted; reconcile the original attempt"
	}
	// Capture even on conflict/uncertainty. A known conflict never calls seal;
	// uncertainty remains on the original attempt and is not a new composition.
	captureCtx := context.WithValue(context.Background(), nativeGitScope{}, true)
	captureCtx = context.WithValue(captureCtx, compositionGitExecutableScope{}, executable)
	r.WorkspaceEvidence, err = w.capture(captureCtx, r)
	if err != nil {
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "composition workspace evidence requires reconciliation"
		return r
	}
	raw, _ := json.Marshal(facts)
	r.Output = string(raw)
	if r.ExitCode == 0 && (r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence["head_commit"] != facts.ResultCommit) {
		r.ExitCode = -1
		r.OutcomeUnknown = true
		r.Error = "composition workspace differs from the fixed result; reconcile the original attempt"
	}
	if r.ExitCode == 0 && !r.OutcomeUnknown && r.Error == "" && r.WorkspaceEvidence["clean"] == true {
		if err = finishCandidate(ctx, a, cfg, w, &r); err != nil {
			r.ExitCode = -1
			r.Error = "candidate transfer: " + err.Error()
		}
	}
	return r
}
