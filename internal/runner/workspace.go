package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

var commitID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
var attemptID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var evidenceHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

type attemptWorkspace struct {
	AttemptKey            string     `json:"attempt_key"`
	Assignment            Assignment `json:"assignment"`
	Repository            string     `json:"repository"`
	BaseCommit            string     `json:"base_commit"`
	AdminBaseCommit       string     `json:"admin_base_commit"`
	Directory             string     `json:"directory"`
	Source                string     `json:"source"`
	State                 string     `json:"state"`
	TestCommand           bool       `json:"test_command"`
	HeadCommit            string     `json:"head_commit,omitempty"`
	ExitCode              int        `json:"exit_code"`
	OutcomeUnknown        bool       `json:"outcome_unknown"`
	candidateImport       *cc.ImportReceipt
	nativeEvidence        *NativeWorkspaceEvidence
	CompositionEvidence   *ccc.CandidateCompositionEvidence `json:"candidate_composition,omitempty"`
	ConflictRepair        *candidateRepairState             `json:"candidate_conflict_repair,omitempty"`
	compositionClient     *Client
	compositionGitVersion string
	compositionGitSHA256  string
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func attemptKey(a Assignment) string {
	if a.PoolID != "" {
		return digest([]byte(key(a)))[:32]
	}
	b, _ := json.Marshal([]any{a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID})
	return digest(b)[:32]
}
func hasCandidate(a Assignment) bool {
	_, declared := a.Config["candidate_from"]
	return declared || a.CandidateCommit != "" || a.CandidateFrom != "" || a.CandidateAttempt != 0 || a.CandidateFence != 0 || a.CandidateEvidenceSHA256 != ""
}

// Only the authenticated scheduler's complete envelope may select a candidate.
// A workflow config commit or an unresolved source never changes the admin pin.
func candidateBase(ctx context.Context, repo string, a Assignment, cfg ExecConfig) (string, error) {
	if a.CodeBaseline != nil {
		base, receipt, err := importCodeBaseline(ctx, repo, a, cfg)
		if err == nil && cfg.candidateImport != nil {
			*cfg.candidateImport = *receipt
		}
		return base, err
	}
	if !hasCandidate(a) {
		return cfg.BaseCommit, nil
	}
	declared, ok := a.Config["candidate_from"].(string)
	build := false
	if (a.Type == "command" || a.Type == "agent") && len(a.Command) == 1 {
		if spec, exists := cfg.Commands[a.Command[0]]; exists && spec.ArtifactOutput != "" {
			_, _, e := artifactProfile(a.Command[0], spec)
			build = e == nil
		}
	}
	if (a.Type != "test" && !build) || !ok || strings.TrimSpace(declared) == "" || a.CandidateFrom != declared || a.CandidateFrom == a.NodeID || !commitID.MatchString(a.CandidateCommit) || a.CandidateAttempt < 1 || a.CandidateFence < 1 || !evidenceHash.MatchString(a.CandidateEvidenceSHA256) {
		return "", fmt.Errorf("candidate test requires a complete scheduler envelope matching config.candidate_from")
	}
	resolved, err := gitText(ctx, repo, "rev-parse", "--verify", a.CandidateCommit+"^{commit}")
	if err != nil || resolved != a.CandidateCommit {
		receipt, transferErr := importCandidate(ctx, repo, a, cfg)
		if transferErr != nil {
			return "", transferErr
		}
		if cfg.candidateImport != nil {
			*cfg.candidateImport = *receipt
		}
	}
	if _, err = gitRun(ctx, repo, "merge-base", "--is-ancestor", cfg.BaseCommit, a.CandidateCommit); err != nil {
		return "", fmt.Errorf("candidate commit is outside the administrator base lineage")
	}
	return a.CandidateCommit, nil
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
func realDirectory(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	s, err := os.Stat(p)
	if err != nil || !s.IsDir() {
		return "", fmt.Errorf("directory unavailable")
	}
	return filepath.Clean(p), nil
}

// safeMkdir refuses existing links along the administrator-configured cache
// path. Worktree source files are never used as metadata destinations.
func safeMkdir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var missing []string
	for p := abs; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if os.IsNotExist(e) {
			missing = append(missing, p)
		} else if e != nil {
			return e
		} else {
			if s.Mode()&os.ModeSymlink != 0 || !s.IsDir() {
				return fmt.Errorf("cache path contains a link or non-directory")
			}
			resolved, e := filepath.EvalSymlinks(p)
			if e != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(p)) {
				return fmt.Errorf("cache path traverses a link")
			}
			break
		}
		if filepath.Dir(p) == p {
			return fmt.Errorf("cache root unavailable")
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err = os.Mkdir(missing[i], 0700); err != nil {
			return err
		}
	}
	return nil
}
func writeEvidence(path string, b []byte) error {
	if s, err := os.Lstat(path); err == nil && (!s.Mode().IsRegular() || s.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("evidence destination is not a regular file")
	}
	f, err := os.CreateTemp(filepath.Dir(path), "evidence-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return replaceFile(name, path)
}
func (a *attemptWorkspace) save() error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return writeEvidence(filepath.Join(a.Directory, "attempt.json"), b)
}
func gitRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotePath=false", "-c", "core.hooksPath=" + filepath.Join(dir, ".suma-disabled-hooks")}, args...)...)
	cmd.Dir = dir
	// Git never prompts or invokes a credential helper for these local operations.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	if native, _ := ctx.Value(nativeGitScope{}).(bool); native {
		cmd = nativeGitCommand(ctx, dir, args...)
	}
	out := newOutput(4<<20, nil)
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("local git operation failed: %w", err)
	}
	if out.truncated {
		return nil, fmt.Errorf("Git evidence exceeds 4 MiB")
	}
	return []byte(out.b.String()), nil
}
func gitText(ctx context.Context, dir string, args ...string) (string, error) {
	b, err := gitRun(ctx, dir, args...)
	return strings.TrimSpace(string(b)), err
}
func workspaceConfig(ctx context.Context, cfg ExecConfig) (string, string, error) {
	if cfg.Repository == "" || !commitID.MatchString(cfg.BaseCommit) {
		return "", "", fmt.Errorf("write command requires administrator repository and full pinned base commit")
	}
	repo, err := realDirectory(cfg.Repository)
	if err != nil {
		return "", "", fmt.Errorf("repository unavailable")
	}
	top, err := gitText(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	top, err = realDirectory(top)
	if err != nil || !strings.EqualFold(top, repo) {
		return "", "", fmt.Errorf("administrator repository must be the Git checkout root")
	}
	resolved, err := gitText(ctx, repo, "rev-parse", "--verify", cfg.BaseCommit+"^{commit}")
	if err != nil || resolved != cfg.BaseCommit {
		return "", "", fmt.Errorf("pinned base commit unavailable")
	}
	root := cfg.WorktreeRoot
	if root == "" {
		root = filepath.Join(repo, ".cache", "runner", "worktrees")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	// The documented task cache is deliberately the only automatic write root.
	cache := filepath.Join(repo, ".cache")
	if !within(cache, root) || strings.EqualFold(cache, root) {
		return "", "", fmt.Errorf("worktrees must use a dedicated child of repository .cache")
	}
	if err = safeMkdir(root); err != nil {
		return "", "", err
	}
	if _, err = gitRun(ctx, repo, "check-ignore", "--quiet", root); err != nil {
		return "", "", fmt.Errorf("administrator worktree cache must be ignored by Git")
	}
	return repo, root, nil
}
func prepareWorkspace(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec) (*attemptWorkspace, error) {
	a = cloneModelAssignment(a)
	if a.CandidateConflictRepair != nil {
		return prepareConflictRepairWorkspace(ctx, a, cfg, spec)
	}
	if a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 {
		return nil, fmt.Errorf("write command requires complete assignment identity")
	}
	repo, root, err := workspaceConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var imported cc.ImportReceipt
	cfg.candidateImport = &imported
	base, err := candidateBase(ctx, repo, a, cfg)
	if err != nil {
		return nil, err
	}
	if spec.NativeWorkspace != nil || spec.CandidateComposition || a.CodeBaseline != nil {
		if err = nativeBaseAttributes(ctx, repo, base); err != nil {
			return nil, err
		}
	}
	if binding := cfg.resolvedArtifact; binding != nil {
		if !commitID.MatchString(binding.SourceCommit) {
			return nil, fmt.Errorf("artifact source commit is invalid")
		}
		resolved, e := gitText(ctx, repo, "rev-parse", "--verify", binding.SourceCommit+"^{commit}")
		if e != nil || resolved != binding.SourceCommit {
			return nil, fmt.Errorf("artifact source commit is not available locally; no fetch was attempted")
		}
		if _, e = gitRun(ctx, repo, "merge-base", "--is-ancestor", cfg.BaseCommit, binding.SourceCommit); e != nil {
			return nil, fmt.Errorf("artifact source is outside administrator base lineage")
		}
		if hasCandidate(a) && base != binding.SourceCommit {
			return nil, fmt.Errorf("artifact and candidate source commits disagree")
		}
		base = binding.SourceCommit
	}
	id := attemptKey(a)
	dir := filepath.Join(root, id)
	if err = os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("attempt workspace already exists or is unavailable; reconcile instead of rerunning")
	}
	state := &attemptWorkspace{AttemptKey: id, Assignment: a, Repository: repo, BaseCommit: base, AdminBaseCommit: cfg.BaseCommit, Directory: dir, Source: filepath.Join(dir, "src"), State: "preparing", TestCommand: spec.TestEvidence, ExitCode: -1}
	if imported.Imported {
		state.candidateImport = &imported
	}
	if err = state.save(); err != nil {
		return nil, err
	}
	if _, err = gitRun(ctx, repo, "worktree", "add", "--detach", state.Source, base); err != nil {
		return nil, err
	}
	head, err := gitText(ctx, state.Source, "rev-parse", "HEAD")
	if err != nil || head != base {
		return nil, fmt.Errorf("isolated base does not match pin")
	}
	status, err := gitText(ctx, state.Source, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || status != "" {
		return nil, fmt.Errorf("new attempt workspace is dirty or unavailable")
	}
	if err = filepath.WalkDir(state.Source, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("isolated source contains a link; administrator review required")
		}
		if d.IsDir() {
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(path)) {
				return fmt.Errorf("isolated source traverses a directory link")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	state.State = "prepared"
	if err = state.save(); err != nil {
		return nil, err
	}
	return state, nil
}
func (a *attemptWorkspace) capture(ctx context.Context, result Result) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dir, err := realDirectory(a.Directory)
	if err != nil || dir != a.Directory {
		return nil, fmt.Errorf("attempt evidence directory changed")
	}
	source, err := realDirectory(a.Source)
	if err != nil || source != a.Source {
		return nil, fmt.Errorf("attempt source directory changed")
	}
	top, err := gitText(ctx, source, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	top, err = realDirectory(top)
	if err != nil || top != source {
		return nil, fmt.Errorf("attempt Git identity changed")
	}
	expectedRepository := a.Repository
	if a.ConflictRepair != nil {
		if a.Assignment.CandidateConflictRepair == nil || a.ConflictRepair.Repository != filepath.Join(a.Directory, "repair.git") || !within(filepath.Join(a.Repository, ".cache"), a.Directory) {
			return nil, fmt.Errorf("repair evidence repository identity changed")
		}
		expectedRepository = a.ConflictRepair.Repository
		if resolved, e := realDirectory(expectedRepository); e != nil || resolved != expectedRepository {
			return nil, fmt.Errorf("repair evidence repository traverses a link")
		}
	}
	expectedCommon, err := gitText(ctx, expectedRepository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	common, err := gitText(ctx, source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || common != expectedCommon {
		return nil, fmt.Errorf("attempt Git repository changed")
	}
	head, err := gitText(ctx, source, "rev-parse", "HEAD")
	if err != nil || !commitID.MatchString(head) {
		return nil, fmt.Errorf("attempt HEAD unavailable")
	}
	status, err := gitRun(ctx, source, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	diff, err := gitRun(ctx, source, "diff", "--binary", a.BaseCommit, "--")
	if err != nil {
		return nil, err
	}
	changedNames, err := gitRun(ctx, source, "diff", "--name-only", "-z", a.BaseCommit, "--")
	if err != nil {
		return nil, err
	}
	inventory := map[string]string{}
	for _, line := range strings.Split(string(status), "\x00") {
		if !strings.HasPrefix(line, "?? ") {
			continue
		}
		name := strings.TrimPrefix(line, "?? ")
		path := filepath.Join(source, filepath.FromSlash(name))
		if !within(source, path) {
			return nil, fmt.Errorf("untracked path escaped attempt")
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if resolveErr != nil || !within(source, resolved) {
			return nil, fmt.Errorf("untracked evidence traverses a link outside the attempt")
		}
		s, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !s.Mode().IsRegular() {
			return nil, fmt.Errorf("untracked evidence contains a non-regular file")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, f)
		_ = f.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		inventory[name] = hex.EncodeToString(h.Sum(nil))
	}
	changed := map[string]bool{}
	for _, path := range strings.Split(string(changedNames), "\x00") {
		if path != "" {
			changed[path] = true
		}
	}
	for path := range inventory {
		changed[path] = true
	}
	allPaths := make([]string, 0, len(changed))
	for path := range changed {
		allPaths = append(allPaths, path)
	}
	sort.Strings(allPaths)
	pathsTruncated := len(allPaths) > 200
	changedPaths := []string{}
	for _, path := range allPaths[:min(len(allPaths), 200)] {
		path = progressText(path, nil)
		if len(path) > 512 {
			path = path[:512]
			for !utf8.ValidString(path) {
				path = path[:len(path)-1]
			}
			pathsTruncated = true
		}
		changedPaths = append(changedPaths, path)
	}
	// Raw patches and status stay local. The control service receives hashes and
	// metadata only, preventing patch contents from becoming event messages.
	if err = writeEvidence(filepath.Join(dir, "changes.patch"), diff); err != nil {
		return nil, err
	}
	if err = writeEvidence(filepath.Join(dir, "status.txt"), status); err != nil {
		return nil, err
	}
	inv, _ := json.MarshalIndent(inventory, "", "  ")
	if err = writeEvidence(filepath.Join(dir, "untracked.json"), inv); err != nil {
		return nil, err
	}
	a.State = "completed"
	a.HeadCommit = head
	a.ExitCode = result.ExitCode
	a.OutcomeUnknown = result.OutcomeUnknown
	if result.OutcomeUnknown {
		a.State = "unknown"
	}
	if err = a.save(); err != nil {
		return nil, err
	}
	evidence := map[string]any{"attempt_key": a.AttemptKey, "base_commit": a.BaseCommit, "head_commit": head, "state": a.State, "exit_code": result.ExitCode, "outcome_unknown": result.OutcomeUnknown, "test_command": a.TestCommand, "clean": len(status) == 0, "diff_sha256": digest(diff), "status_sha256": digest(status), "untracked_sha256": digest(inv), "captured_at": time.Now().UTC().Format(time.RFC3339Nano)}
	evidence["admin_base_commit"] = a.AdminBaseCommit
	if a.ConflictRepair != nil {
		evidence["candidate_conflict_repair"] = a.ConflictRepair.Evidence
	}
	if a.Assignment.CodeBaseline != nil {
		evidence["code_baseline"] = a.Assignment.CodeBaseline
	}
	if a.CompositionEvidence != nil {
		evidence["candidate_composition"] = a.CompositionEvidence
		evidence["composition_git_version"] = a.compositionGitVersion
		evidence["composition_git_sha256"] = a.compositionGitSHA256
		evidence["composition_git_policy"] = "isolated-bare-no-drivers-v1"
	}
	if hasCandidate(a.Assignment) {
		evidence["candidate_from"] = a.Assignment.CandidateFrom
		evidence["candidate_attempt"] = a.Assignment.CandidateAttempt
		evidence["candidate_fence"] = a.Assignment.CandidateFence
		evidence["candidate_evidence_sha256"] = a.Assignment.CandidateEvidenceSHA256
	}
	evidence["changed_paths"] = changedPaths
	evidence["changed_paths_total"] = len(allPaths)
	evidence["changed_paths_truncated"] = pathsTruncated
	if a.nativeEvidence != nil {
		evidence["native_workspace"] = a.nativeEvidence
	}
	if err := appendTerminalCandidateEvidence(a, evidence); err != nil {
		return nil, err
	}
	b, _ := json.MarshalIndent(evidence, "", "  ")
	if err = writeEvidence(filepath.Join(dir, "evidence.json"), b); err != nil {
		return nil, err
	}
	evidence["evidence_sha256"] = digest(b)
	if a.candidateImport != nil {
		evidence["candidate_import"] = *a.candidateImport
	}
	return evidence, nil
}

type MergePreflightRequest struct {
	AttemptKey             string `json:"attempt_key"`
	ExpectedTargetHEAD     string `json:"expected_target_head"`
	ExpectedCandidateHEAD  string `json:"expected_candidate_head"`
	ExpectedEvidenceSHA256 string `json:"expected_evidence_sha256"`
	ReviewedBy             string `json:"reviewed_by"`
}
type MergePreflightResult struct {
	Ready          bool   `json:"ready"`
	Merged         bool   `json:"merged"`
	AttemptKey     string `json:"attempt_key"`
	TargetHEAD     string `json:"target_head"`
	CandidateHEAD  string `json:"candidate_head"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	ReviewedBy     string `json:"reviewed_by"`
}

// MergePreflight validates an operator-reviewed candidate. It performs no merge,
// checkout, reset, branch update or commit in the target repository.
func MergePreflight(ctx context.Context, cfg ExecConfig, request MergePreflightRequest) (MergePreflightResult, error) {
	var out MergePreflightResult
	if !attemptID.MatchString(request.AttemptKey) || !commitID.MatchString(request.ExpectedTargetHEAD) || !commitID.MatchString(request.ExpectedCandidateHEAD) || len(request.ExpectedEvidenceSHA256) != 64 || strings.TrimSpace(request.ReviewedBy) == "" || len(request.ReviewedBy) > 200 {
		return out, fmt.Errorf("review identity, expected heads and evidence hash required")
	}
	repo, root, err := workspaceConfig(ctx, cfg)
	if err != nil {
		return out, err
	}
	dir := filepath.Join(root, request.AttemptKey)
	actualDir, err := realDirectory(dir)
	if err != nil || !strings.EqualFold(actualDir, dir) {
		return out, fmt.Errorf("attempt evidence directory unavailable or linked")
	}
	for _, name := range []string{"attempt.json", "evidence.json"} {
		s, e := os.Lstat(filepath.Join(dir, name))
		if e != nil || !s.Mode().IsRegular() {
			return out, fmt.Errorf("attempt evidence is not a regular file")
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "attempt.json"))
	if err != nil {
		return out, err
	}
	var a attemptWorkspace
	if json.Unmarshal(b, &a) != nil || a.AttemptKey != request.AttemptKey || a.Directory != dir || a.Repository != repo || a.Source != filepath.Join(dir, "src") || a.AdminBaseCommit != cfg.BaseCommit || attemptKey(a.Assignment) != a.AttemptKey {
		return out, fmt.Errorf("attempt metadata identity mismatch")
	}
	selectedBase, err := candidateBase(ctx, repo, a.Assignment, cfg)
	if err != nil || selectedBase != a.BaseCommit {
		return out, fmt.Errorf("attempt candidate source changed")
	}
	if a.State != "completed" || a.ExitCode != 0 || a.OutcomeUnknown || !a.TestCommand {
		return out, fmt.Errorf("successful administrator-designated test command evidence required")
	}
	b, err = os.ReadFile(filepath.Join(dir, "evidence.json"))
	if err != nil || digest(b) != request.ExpectedEvidenceSHA256 {
		return out, fmt.Errorf("execution evidence changed")
	}
	var evidence map[string]any
	if json.Unmarshal(b, &evidence) != nil {
		return out, fmt.Errorf("invalid execution evidence")
	}
	if evidence["attempt_key"] != a.AttemptKey || evidence["base_commit"] != a.BaseCommit || evidence["head_commit"] != a.HeadCommit || evidence["state"] != "completed" || evidence["exit_code"] != float64(0) || evidence["outcome_unknown"] != false || evidence["test_command"] != true {
		return out, fmt.Errorf("successful test evidence does not match the attempt metadata")
	}
	if evidence["admin_base_commit"] != a.AdminBaseCommit {
		return out, fmt.Errorf("administrator base evidence changed")
	}
	if hasCandidate(a.Assignment) && (evidence["candidate_from"] != a.Assignment.CandidateFrom || evidence["candidate_attempt"] != float64(a.Assignment.CandidateAttempt) || evidence["candidate_fence"] != float64(a.Assignment.CandidateFence) || evidence["candidate_evidence_sha256"] != a.Assignment.CandidateEvidenceSHA256) {
		return out, fmt.Errorf("candidate handoff evidence changed")
	}
	for file, field := range map[string]string{"changes.patch": "diff_sha256", "status.txt": "status_sha256", "untracked.json": "untracked_sha256"} {
		s, e := os.Lstat(filepath.Join(dir, file))
		if e != nil || !s.Mode().IsRegular() {
			return out, fmt.Errorf("evidence file unavailable")
		}
		data, e := os.ReadFile(filepath.Join(dir, file))
		if e != nil || evidence[field] != digest(data) {
			return out, fmt.Errorf("execution evidence file changed")
		}
	}
	target, err := gitText(ctx, repo, "rev-parse", "HEAD")
	if err != nil || target != request.ExpectedTargetHEAD {
		return out, fmt.Errorf("target HEAD changed; review again")
	}
	actualSource, err := realDirectory(a.Source)
	if err != nil || !strings.EqualFold(actualSource, a.Source) {
		return out, fmt.Errorf("candidate source changed or is linked")
	}
	top, err := gitText(ctx, a.Source, "rev-parse", "--show-toplevel")
	if err != nil {
		return out, err
	}
	actualTop, err := realDirectory(top)
	if err != nil || !strings.EqualFold(actualTop, a.Source) {
		return out, fmt.Errorf("candidate Git identity changed")
	}
	common, err := gitText(ctx, a.Source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return out, err
	}
	expectedCommon, err := gitText(ctx, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || common != expectedCommon {
		return out, fmt.Errorf("candidate repository changed")
	}
	candidate, err := gitText(ctx, a.Source, "rev-parse", "HEAD")
	if err != nil || candidate != request.ExpectedCandidateHEAD || candidate != a.HeadCommit || evidence["head_commit"] != candidate {
		return out, fmt.Errorf("candidate HEAD changed after tests")
	}
	for _, path := range []string{repo, a.Source} {
		status, e := gitText(ctx, path, "status", "--porcelain=v1", "--untracked-files=all")
		if e != nil || status != "" {
			return out, fmt.Errorf("merge preflight requires clean target and candidate")
		}
	}
	if evidence["clean"] != true || candidate == a.AdminBaseCommit {
		return out, fmt.Errorf("committed candidate and clean test evidence required")
	}
	if _, err = gitRun(ctx, repo, "merge-base", "--is-ancestor", target, candidate); err != nil {
		return out, fmt.Errorf("candidate does not include expected target HEAD")
	}
	out = MergePreflightResult{Ready: true, Merged: false, AttemptKey: a.AttemptKey, TargetHEAD: target, CandidateHEAD: candidate, EvidenceSHA256: request.ExpectedEvidenceSHA256, ReviewedBy: request.ReviewedBy}
	receipt, _ := json.MarshalIndent(out, "", "  ")
	if err = writeEvidence(filepath.Join(dir, "merge-preflight-"+digest(receipt)[:16]+".json"), receipt); err != nil {
		return MergePreflightResult{}, err
	}
	return out, nil
}
