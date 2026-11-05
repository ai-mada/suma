package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func (c *Client) downloadRepositorySource(ctx context.Context, a Assignment, source ri.RepositorySourceExecution, file string) error {
	path := "repository-source-intakes/" + url.PathEscape(source.IntakeID) + "/bundle?project_id=" + url.QueryEscape(c.ProjectID)
	response, err := c.scopedCandidateRequest(ctx, http.MethodGet, path, a, nil, 0, candidateOperation(a, "repository-source"))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength != source.SizeBytes || response.Uncompressed || response.Header.Get("Content-Encoding") != "" {
		return fmt.Errorf("repository source response differs from the approved bundle")
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(response.Body, source.SizeBytes+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n != source.SizeBytes {
		return fmt.Errorf("repository source download length or persistence mismatch")
	}
	return nil
}

// A repository source candidate imports reviewed objects into a new object
// database. It never executes source code or invokes the selected tool command.
func executeRepositorySourceCandidate(ctx context.Context, a Assignment, cfg ExecConfig) Result {
	r := Result{Assignment: a, ExitCode: -1}
	fail := func(err error) Result {
		r.ExitCode, r.Error = -1, err.Error()
		if r.ToolEvidence != nil {
			r.ToolEvidence.Terminal = "failed"
		}
		return r
	}
	source, err := ri.DecodeRepositorySourceExecution(a.Config[ri.RepositorySourceExecutionKey])
	if err != nil || source == nil || !source.Valid() || !cfg.AllowExec || cfg.Artifacts == nil || cfg.ToolInventory == nil ||
		a.Type != "command" || len(a.Command) != 1 || !candidateRef(a).Valid() || a.Idempotent || hasCandidate(a) || cloudReserved(a) || a.WorkBuddyCloud != nil ||
		a.CandidateComposition != nil || a.CandidateConflictRepair != nil || a.CodeBaseline != nil || a.DevelopmentContinuation != nil ||
		a.RepositoryIntegration != nil || a.ReleaseExecution != nil || a.ExecutionSkills != nil || a.Config["model_execution"] != nil ||
		source.BaseCommit != cfg.BaseCommit || a.ProjectID != "" && a.ProjectID != cfg.Artifacts.ProjectID {
		return fail(fmt.Errorf("repository source candidate requires its fixed isolated execution"))
	}
	spec := normalizedCommand(cfg.Commands[a.Command[0]])
	pinned, _ := a.Config["tool_profile_sha256"].(string)
	if !cc.ValidHash(pinned) || spec.Adapter != "command" || spec.Protocol != "text-v1" || spec.ReadOnly || spec.TerminalSession ||
		spec.NativeWorkspace != nil || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.TestEvidence || spec.TestReport {
		return fail(fmt.Errorf("repository source candidate requires the pinned command profile"))
	}
	profile, _, err := launchProfile(ctx, a, cfg, spec)
	if err != nil {
		return fail(err)
	}
	duration := time.Duration(a.TimeoutSeconds) * time.Second
	if duration <= 0 {
		duration = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, min(duration, 24*time.Hour))
	defer cancel()
	executable, _, err := commandExecutable(CommandSpec{Executable: "git"})
	if err != nil {
		return fail(err)
	}
	ctx = context.WithValue(ctx, nativeGitScope{}, true)
	ctx = context.WithValue(ctx, compositionGitExecutableScope{}, executable)
	version, err := compositionText(ctx, "", "--version")
	if err != nil || !strings.HasPrefix(version, "git version ") {
		return fail(fmt.Errorf("repository source candidate Git probe failed"))
	}
	r.ToolEvidence = &tc.Evidence{Profile: profile, Terminal: "failed", UsageState: tc.UsageUnreported}
	w, bundle, err := prepareRepositorySourceWorkspace(ctx, a, cfg, *source)
	if err != nil {
		return fail(err)
	}
	r.ExitCode = 0
	r.ToolEvidence.Terminal = "completed"
	r.WorkspaceEvidence, err = w.capture(ctx, r)
	if err != nil {
		return fail(err)
	}
	if r.WorkspaceEvidence["clean"] != true || w.HeadCommit != source.HeadCommit {
		return fail(fmt.Errorf("repository source workspace differs from the approved commit"))
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if err = finishCandidateBundle(ctx, a, cfg, w, &r, bundle, source.BundleSHA256, source.SizeBytes); err != nil {
		return fail(fmt.Errorf("candidate transfer: %w", err))
	}
	return r
}

func prepareRepositorySourceWorkspace(ctx context.Context, a Assignment, cfg ExecConfig, source ri.RepositorySourceExecution) (*attemptWorkspace, string, error) {
	repo, root, err := workspaceConfig(ctx, cfg)
	if err != nil {
		return nil, "", err
	}
	key := attemptKey(a)
	dir := filepath.Join(root, key)
	if err = os.Mkdir(dir, 0700); err != nil {
		return nil, "", fmt.Errorf("repository source attempt already exists; reconcile the original attempt")
	}
	w := &attemptWorkspace{AttemptKey: key, Assignment: a, Repository: repo, BaseCommit: source.BaseCommit, AdminBaseCommit: cfg.BaseCommit, Directory: dir, Source: filepath.Join(dir, "src"), State: "preparing", ExitCode: -1}
	if err = w.save(); err != nil {
		return nil, "", err
	}
	bare := filepath.Join(dir, "repository-source.git")
	templates := filepath.Join(dir, "empty-templates")
	if err = safeMkdir(templates); err != nil {
		return nil, "", err
	}
	format := "sha1"
	if len(source.BaseCommit) == 64 {
		format = "sha256"
	}
	if _, err = compositionGit(ctx, dir, 8192, nil, "init", "--bare", "--object-format="+format, "--template="+templates, bare); err != nil {
		return nil, "", fmt.Errorf("repository source private object database could not be initialized")
	}
	bootstrap := filepath.Join(dir, "bootstrap.pack")
	if err = compositionPack(ctx, repo, bootstrap, source.BaseCommit+"\n"); err != nil {
		return nil, "", fmt.Errorf("repository source administrator base closure is unavailable")
	}
	if err = compositionIndexPack(ctx, bare, bootstrap); err != nil {
		return nil, "", fmt.Errorf("repository source administrator base closure could not be imported")
	}
	bundle := filepath.Join(dir, "candidate.bundle")
	if err = cfg.Artifacts.downloadRepositorySource(ctx, a, source, bundle); err != nil {
		return nil, "", err
	}
	manifest := fixedGitBundle{anchor: source.BaseCommit, base: source.BaseCommit, head: source.HeadCommit, sha256: source.BundleSHA256, size: source.SizeBytes}
	if err = importFixedGitBundle(ctx, bare, bundle, manifest); err != nil {
		return nil, "", err
	}
	if err = compositionTreePolicy(ctx, bare, source.HeadCommit); err != nil {
		return nil, "", err
	}
	if _, err = compositionGit(ctx, bare, 8192, nil, "worktree", "add", "--detach", w.Source, source.HeadCommit); err != nil {
		return nil, "", fmt.Errorf("repository source commit could not be checked out in isolation")
	}
	w.State = "prepared"
	return w, bundle, w.save()
}
