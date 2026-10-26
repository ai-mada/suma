package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// Parse Git's NUL-delimited index records, never free-form conflict messages.
func parseCompositionConflictStages(records [][]byte, oidLength int) ([]ccc.CandidateConflictStage, []string, error) {
	stages := []ccc.CandidateConflictStage{}
	paths := []string{}
	seen := map[string]bool{}
	total := 0
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		header, p, ok := bytes.Cut(record, []byte{'\t'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || !utf8.Valid(p) || len(p) > 512 {
			return nil, nil, fmt.Errorf("composition conflict stage evidence is invalid")
		}
		stage, e := strconv.Atoi(fields[2])
		name := string(p)
		if e != nil || stage < 1 || stage > 3 || fields[0] != "100644" && fields[0] != "100755" || !commitID.MatchString(fields[1]) || len(fields[1]) != oidLength || strings.ContainsAny(name, "\\:\r\n\x00") || strings.HasPrefix(name, "/") {
			return nil, nil, fmt.Errorf("composition conflict stage evidence is unsupported")
		}
		stages = append(stages, ccc.CandidateConflictStage{Path: name, Mode: fields[0], Object: fields[1], Stage: stage})
		if !seen[name] {
			seen[name] = true
			paths = append(paths, name)
			total += len(name)
		}
		if total > 64<<10 || len(stages) > 6000 {
			return nil, nil, fmt.Errorf("composition conflict paths exceed the supported report bound")
		}
	}
	if len(stages) == 0 {
		return nil, nil, fmt.Errorf("composition conflict paths are unavailable")
	}
	return stages, paths, nil
}

func repairMergeTree(ctx context.Context, repo, left, right string) (string, []ccc.CandidateConflictStage, []string, error) {
	raw, err := compositionGit(ctx, repo, 1<<20, nil, "merge-tree", "--write-tree", "--no-messages", "-z", left, right)
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", nil, nil, fmt.Errorf("repair native three-way merge failed")
		}
	}
	parts := bytes.Split(bytes.TrimRight(raw, "\x00\n"), []byte{0})
	if len(parts) == 0 || !commitID.MatchString(string(parts[0])) {
		return "", nil, nil, fmt.Errorf("repair merge tree identity is invalid")
	}
	tree := string(parts[0])
	if policyErr := compositionTreePolicy(ctx, repo, tree); policyErr != nil {
		return "", nil, nil, policyErr
	}
	if err == nil {
		return tree, nil, nil, nil
	}
	stages, paths, parseErr := parseCompositionConflictStages(parts[1:], len(left))
	return tree, stages, paths, parseErr
}

func repairCommitTree(ctx context.Context, repo, tree, left, right, operation, timestamp string, step int) (string, error) {
	t, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return "", err
	}
	date := strconv.FormatInt(t.Unix(), 10) + " +0000"
	args := []string{"commit-tree", tree, "-p", left}
	if right != "" && right != left {
		args = append(args, "-p", right)
	}
	cmd := nativeGitCommand(ctx, repo, args...)
	cmd.Stdin = strings.NewReader("suma conflict repair v1\noperation: " + operation + "\nstep: " + strconv.Itoa(step) + "\n")
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=Suma Conflict Repair", "GIT_AUTHOR_EMAIL=repair@suma.invalid", "GIT_COMMITTER_NAME=Suma Conflict Repair", "GIT_COMMITTER_EMAIL=repair@suma.invalid", "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out := newOutput(256, nil)
	cmd.Stdout = out
	cmd.Stderr = newOutput(8192, nil)
	if err = cmd.Run(); err != nil || out.truncated {
		return "", fmt.Errorf("repair prefix commit could not be fixed")
	}
	commit := strings.TrimSpace(out.b.String())
	if !commitID.MatchString(commit) {
		return "", fmt.Errorf("repair prefix commit is invalid")
	}
	return commit, nil
}

type candidateRepairState struct {
	Version           int                                 `json:"version"`
	ExecutionSHA256   string                              `json:"execution_sha256"`
	Repository        string                              `json:"repository"`
	CurrentCommit     string                              `json:"current_commit"`
	SourceIndex       int                                 `json:"source_index"`
	SeedTree          string                              `json:"seed_tree"`
	Stages            []ccc.CandidateConflictStage        `json:"stages"`
	Conflicts         []string                            `json:"conflicts"`
	State             string                              `json:"state"`
	Evidence          ccc.CandidateConflictRepairEvidence `json:"evidence"`
	LastOperationID   string                              `json:"last_operation_id,omitempty"`
	LastRequestSHA256 string                              `json:"last_request_sha256,omitempty"`
	Pending           *candidateRepairPending             `json:"pending,omitempty"`
}

type candidateRepairPending struct {
	OperationID   string                `json:"operation_id"`
	RequestSHA256 string                `json:"request_sha256"`
	Next          *candidateRepairState `json:"next"`
}

func (w *attemptWorkspace) saveRepair() error {
	raw, err := json.MarshalIndent(w.ConflictRepair, "", "  ")
	if err != nil {
		return err
	}
	return writeEvidence(filepath.Join(w.Directory, "conflict-repair.json"), raw)
}

func prepareConflictRepairWorkspace(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec) (*attemptWorkspace, error) {
	e := a.CandidateConflictRepair
	if e == nil || !candidateRef(a).Valid() || ccc.ValidateCandidateConflictRepairExecution(*e) != nil || spec.NativeWorkspace == nil || !spec.NativeWorkspace.ConflictRepair || cfg.Artifacts == nil || e.ProjectID != cfg.Artifacts.ProjectID || e.Composition.BaseCommit != cfg.BaseCommit {
		return nil, fmt.Errorf("conflict repair requires its fixed native administrator capability")
	}
	repo, root, err := workspaceConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, attemptKey(a))
	if err = os.Mkdir(dir, 0700); err != nil {
		return nil, fmt.Errorf("repair attempt exists; reconcile the original attempt")
	}
	w := &attemptWorkspace{AttemptKey: attemptKey(a), Assignment: a, Repository: repo, BaseCommit: cfg.BaseCommit, AdminBaseCommit: cfg.BaseCommit, Directory: dir, Source: filepath.Join(dir, "src"), State: "preparing", ExitCode: -1}
	raw, _ := json.Marshal(e)
	s := &candidateRepairState{Version: 1, ExecutionSHA256: digest(raw), Repository: filepath.Join(dir, "repair.git"), SourceIndex: 1, State: "preparing", Evidence: ccc.CandidateConflictRepairEvidence{Protocol: e.Protocol, PlanID: e.PlanID, PlanVersion: e.PlanVersion, CompositionID: e.CompositionID, OriginalAssignment: e.OriginalAssignment, OriginalEvidenceSHA256: e.OriginalEvidenceSHA256, SourcesSHA256: e.Composition.SourcesSHA256, OperationID: e.OperationID, State: "preparing", Steps: []ccc.CandidateConflictRepairStep{}, Parents: []string{}, SourceHeads: []string{}, MergeCommits: []string{}}}
	w.ConflictRepair = s
	if err = w.save(); err != nil {
		return nil, err
	}
	if err = w.saveRepair(); err != nil {
		return nil, err
	}
	templates := filepath.Join(dir, "empty-templates")
	if err = safeMkdir(templates); err != nil {
		return nil, err
	}
	format := "sha1"
	if len(cfg.BaseCommit) == 64 {
		format = "sha256"
	}
	if _, err = compositionGit(ctx, dir, 8192, nil, "init", "--bare", "--object-format="+format, "--template="+templates, s.Repository); err != nil {
		return nil, err
	}
	bootstrap := filepath.Join(dir, "bootstrap.pack")
	if err = compositionPack(ctx, repo, bootstrap, cfg.BaseCommit+"\n"); err != nil {
		return nil, err
	}
	if err = compositionIndexPack(ctx, s.Repository, bootstrap); err != nil {
		return nil, err
	}
	for i, b := range e.Composition.Sources {
		p := filepath.Join(dir, "source-"+strconv.Itoa(i)+".bundle")
		if err = cfg.Artifacts.codeInputBundle(ctx, a, "conflict_repair", e.PlanID, b, p); err != nil {
			return nil, err
		}
		if err = importCandidateFile(ctx, s.Repository, p, b); err != nil {
			return nil, err
		}
		if err = compositionTreePolicy(ctx, s.Repository, b.HeadCommit); err != nil {
			return nil, err
		}
		s.Evidence.SourceHeads = append(s.Evidence.SourceHeads, b.HeadCommit)
	}
	// Replay only the original deterministic clean prefix with its original
	// timestamp/message. No conflicted tree is ever manufactured as a commit.
	s.CurrentCommit = e.Composition.Sources[0].HeadCommit
	for i := 1; i <= e.Conflict.ConflictSourceIndex; i++ {
		tree, stages, paths, mergeErr := repairMergeTree(ctx, s.Repository, s.CurrentCommit, e.Composition.Sources[i].HeadCommit)
		if mergeErr != nil {
			return nil, mergeErr
		}
		if i < e.Conflict.ConflictSourceIndex {
			if len(stages) != 0 {
				return nil, fmt.Errorf("original clean prefix changed")
			}
			// Reproduce the exact 031 commit identity, not the repair author.
			date, _ := time.Parse(time.RFC3339Nano, e.Composition.FixedTimestamp)
			gitDate := strconv.FormatInt(date.Unix(), 10) + " +0000"
			cmd := nativeGitCommand(ctx, s.Repository, "commit-tree", tree, "-p", s.CurrentCommit, "-p", e.Composition.Sources[i].HeadCommit)
			cmd.Stdin = strings.NewReader("suma candidate composition v1\noperation: " + e.Composition.OperationID + "\nplan: " + e.Composition.PlanID + "\nstep: " + strconv.Itoa(i) + "\nsources: " + e.Composition.SourcesSHA256 + "\n")
			cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=Suma Composition", "GIT_AUTHOR_EMAIL=composition@suma.invalid", "GIT_COMMITTER_NAME=Suma Composition", "GIT_COMMITTER_EMAIL=composition@suma.invalid", "GIT_AUTHOR_DATE="+gitDate, "GIT_COMMITTER_DATE="+gitDate)
			out := newOutput(256, nil)
			cmd.Stdout = out
			cmd.Stderr = newOutput(8192, nil)
			if err = cmd.Run(); err != nil || strings.TrimSpace(out.b.String()) != e.Conflict.MergeCommits[i-1] {
				return nil, fmt.Errorf("original composition prefix identity changed")
			}
			s.CurrentCommit = e.Conflict.MergeCommits[i-1]
			s.Evidence.MergeCommits = append(s.Evidence.MergeCommits, s.CurrentCommit)
		} else {
			actual, _ := json.Marshal(stages)
			expected, _ := json.Marshal(e.Conflict.ConflictStages)
			actualPaths, _ := json.Marshal(paths)
			expectedPaths, _ := json.Marshal(e.Conflict.Conflicts)
			if len(stages) == 0 || tree != e.Conflict.ConflictTree || !bytes.Equal(actual, expected) || !bytes.Equal(actualPaths, expectedPaths) {
				return nil, fmt.Errorf("original conflict tree or stages changed")
			}
			s.SourceIndex = i
			s.SeedTree = tree
			s.Stages = stages
			s.Conflicts = paths
			s.State = "conflicted"
			if err = repairSeedSupported(s); err != nil {
				return nil, err
			}
		}
	}
	if _, err = compositionGit(ctx, s.Repository, 8192, nil, "worktree", "add", "--detach", w.Source, s.CurrentCommit); err != nil {
		return nil, err
	}
	if err = installRepairSeed(ctx, w); err != nil {
		return nil, err
	}
	s.Evidence.State = s.State
	w.State = "prepared"
	if err = w.saveRepair(); err != nil {
		return nil, err
	}
	if err = w.save(); err != nil {
		return nil, err
	}
	return w, nil
}

func installRepairSeed(ctx context.Context, w *attemptWorkspace) error {
	s := w.ConflictRepair
	if err := repairSeedSupported(s); err != nil {
		return err
	}
	if err := compositionTreePolicy(ctx, s.Repository, s.SeedTree); err != nil {
		return err
	}
	if _, err := compositionGit(ctx, w.Source, 8192, nil, "read-tree", "--reset", "-u", s.SeedTree); err != nil {
		return err
	}
	if len(s.Stages) > 0 {
		var input strings.Builder
		for _, p := range s.Conflicts {
			input.WriteString("0 " + strings.Repeat("0", len(w.BaseCommit)) + "\t" + p + "\x00")
		}
		for _, stage := range s.Stages {
			input.WriteString(stage.Mode + " " + stage.Object + " " + strconv.Itoa(stage.Stage) + "\t" + stage.Path + "\x00")
		}
		if _, err := compositionGit(ctx, w.Source, 8192, strings.NewReader(input.String()), "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	gitdir, err := compositionText(ctx, w.Source, "rev-parse", "--absolute-git-dir")
	if err != nil || !within(filepath.Join(s.Repository, "worktrees"), gitdir) {
		return fmt.Errorf("repair private Git directory changed")
	}
	if len(s.Stages) > 0 {
		for name, value := range map[string]string{"MERGE_HEAD": w.Assignment.CandidateConflictRepair.Composition.Sources[s.SourceIndex].HeadCommit + "\n", "ORIG_HEAD": s.CurrentCommit + "\n", "MERGE_MSG": "suma governed conflict repair\n"} {
			if err = writeEvidence(filepath.Join(gitdir, name), []byte(value)); err != nil {
				return err
			}
		}
	} else {
		for _, name := range []string{"MERGE_HEAD", "MERGE_MSG"} {
			if err = os.Remove(filepath.Join(gitdir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func repairSeedSupported(s *candidateRepairState) error {
	for _, name := range s.Conflicts {
		if _, err := nativeWorkspacePath(name); err != nil {
			return fmt.Errorf("repair conflict path is unsupported by native workspace policy")
		}
	}
	raw, _ := json.Marshal([]any{s.Stages, s.Conflicts})
	if len(raw) > 48<<10 {
		return fmt.Errorf("repair conflict stage report exceeds the bounded native tool result")
	}
	return nil
}
