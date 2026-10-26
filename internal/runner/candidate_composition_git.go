package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

type compositionGitExecutableScope struct{}

var errCompositionConflict = errors.New("candidate composition has conflicts")

type compositionIntent struct {
	SchemaVersion    int                              `json:"schema_version"`
	AssignmentSHA256 string                           `json:"assignment_sha256"`
	ExecutionSHA256  string                           `json:"execution_sha256"`
	GitSHA256        string                           `json:"git_sha256"`
	Evidence         ccc.CandidateCompositionEvidence `json:"evidence"`
}

func saveCompositionIntent(path string, intent compositionIntent) error {
	raw, err := json.MarshalIndent(intent, "", "  ")
	if err != nil {
		return err
	}
	return writeEvidence(path, raw)
}
func compositionGit(ctx context.Context, dir string, limit int, input io.Reader, args ...string) ([]byte, error) {
	cmd := nativeGitCommand(ctx, dir, args...)
	cmd.Stdin = input
	out := newOutput(limit, nil)
	cmd.Stdout = out
	cmd.Stderr = newOutput(8192, nil)
	err := cmd.Run()
	if out.truncated {
		return nil, fmt.Errorf("composition Git output exceeds its fixed bound")
	}
	return append([]byte(nil), out.b.Bytes()...), err
}
func compositionText(ctx context.Context, dir string, args ...string) (string, error) {
	raw, err := compositionGit(ctx, dir, 8192, nil, args...)
	return strings.TrimSpace(string(raw)), err
}

// Complete B bootstrap is bounded too. We do not clone an unbounded repository,
// share alternates, or copy its local merge/filter/remote configuration.
func compositionPack(ctx context.Context, repo, path, revisions string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	packCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := nativeGitCommand(packCtx, repo, "pack-objects", "--stdout", "--revs")
	cmd.Stdin = strings.NewReader(revisions)
	cmd.Stdout = &candidateLimitWriter{file: f, cancel: cancel}
	cmd.Stderr = newOutput(8192, nil)
	err = cmd.Run()
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err == nil {
		err = closed
	}
	return err
}
func compositionIndexPack(ctx context.Context, repo, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = compositionGit(ctx, repo, 8192, f, "index-pack", "--stdin", "--fix-thin", "--strict", "--max-input-size=67108864")
	return err
}

func compositionTreePolicy(ctx context.Context, repo, tree string) error {
	w := &attemptWorkspace{Source: repo, BaseCommit: tree}
	entries, _, err := nativeSealTree(ctx, w)
	if err != nil {
		return fmt.Errorf("composition tree contains unsupported entries or aliases")
	}
	if err = nativeBaseAttributes(ctx, repo, tree); err != nil {
		return err
	}
	var paths bytes.Buffer
	aliases := map[string]string{}
	for name := range entries {
		if len(name) > 512 || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\r\n\x00") || filepath.IsAbs(name) {
			return fmt.Errorf("composition source path is unsupported")
		}
		parts := strings.Split(name, "/")
		for i, part := range parts {
			lower := strings.ToLower(part)
			device := strings.SplitN(lower, ".", 2)[0]
			if part == "" || part == "." || part == ".." || lower == ".git" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.IndexFunc(part, unicode.IsControl) >= 0 || device == "con" || device == "prn" || device == "aux" || device == "nul" || device == "conin$" || device == "conout$" {
				return fmt.Errorf("composition source path is unsupported")
			}
			if strings.HasPrefix(device, "com") || strings.HasPrefix(device, "lpt") {
				tail := device[3:]
				if len(tail) == 1 && tail[0] >= '1' && tail[0] <= '9' || tail == "¹" || tail == "²" || tail == "³" {
					return fmt.Errorf("composition source path is unsupported")
				}
			}
			prefix := strings.Join(parts[:i+1], "/")
			key := strings.ToLower(prefix)
			if prior, ok := aliases[key]; ok && prior != prefix {
				return fmt.Errorf("composition tree contains unsupported entries or aliases")
			}
			aliases[key] = prefix
		}
		paths.WriteString(name)
		paths.WriteByte(0)
	}
	raw, err := compositionGit(ctx, repo, 4<<20, &paths, "check-attr", "--source="+tree, "-z", "merge", "--stdin")
	if err != nil {
		return fmt.Errorf("composition merge attributes cannot be inspected")
	}
	fields := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(fields) == 1 && len(fields[0]) == 0 {
		return nil
	}
	if len(fields)%3 != 0 {
		return fmt.Errorf("composition merge attributes are invalid")
	}
	for i := 2; i < len(fields); i += 3 {
		switch string(fields[i]) {
		case "unspecified", "unset", "set", "text", "binary":
		default:
			return fmt.Errorf("composition custom merge drivers are forbidden")
		}
	}
	return nil
}

func composeCandidateTrees(ctx context.Context, w *attemptWorkspace, e ccc.CandidateCompositionExecution, gitSHA string) (ccc.CandidateCompositionEvidence, error) {
	facts := ccc.CandidateCompositionEvidence{Protocol: e.Protocol, PlanID: e.PlanID, PlanVersion: e.PlanVersion, SourcesSHA256: e.SourcesSHA256, OperationID: e.OperationID, BaseCommit: e.BaseCommit, State: "unknown", Parents: []string{}, SourceHeads: []string{}, MergeCommits: []string{}, Conflicts: []string{}}
	for _, source := range e.Sources {
		facts.SourceHeads = append(facts.SourceHeads, source.HeadCommit)
	}
	intentPath := filepath.Join(w.Directory, "composition.json")
	if _, err := os.Lstat(intentPath); !os.IsNotExist(err) {
		return facts, fmt.Errorf("composition intent already exists; reconcile the original attempt")
	}
	a, _ := json.Marshal(w.Assignment)
	execution, _ := json.Marshal(e)
	intent := compositionIntent{SchemaVersion: 1, AssignmentSHA256: digest(a), ExecutionSHA256: digest(execution), GitSHA256: gitSHA, Evidence: facts}
	if err := saveCompositionIntent(intentPath, intent); err != nil {
		return facts, err
	}
	bare := filepath.Join(w.Directory, "composition.git")
	templates := filepath.Join(w.Directory, "empty-templates")
	if err := safeMkdir(bare); err != nil {
		return facts, err
	}
	if err := safeMkdir(templates); err != nil {
		return facts, err
	}
	format := "sha1"
	if len(e.BaseCommit) == 64 {
		format = "sha256"
	}
	if _, err := compositionGit(ctx, w.Directory, 8192, nil, "init", "--bare", "--object-format="+format, "--template="+templates, bare); err != nil {
		return facts, fmt.Errorf("composition private object repository could not be initialized")
	}
	bootstrap := filepath.Join(w.Directory, "bootstrap.pack")
	if err := compositionPack(ctx, w.Repository, bootstrap, e.BaseCommit+"\n"); err != nil {
		return facts, fmt.Errorf("administrator base closure unavailable or exceeds 64 MiB")
	}
	if err := compositionIndexPack(ctx, bare, bootstrap); err != nil {
		return facts, fmt.Errorf("administrator base closure could not be imported")
	}
	for i, binding := range e.Sources {
		path := filepath.Join(w.Directory, "source-"+strconv.Itoa(i)+".bundle")
		if err := w.compositionClient.codeInputBundle(ctx, w.Assignment, "composition", e.PlanID, binding, path); err != nil {
			return facts, fmt.Errorf("composition fixed source download failed: %w", err)
		}
		if err := importCandidateFile(ctx, bare, path, binding); err != nil {
			return facts, err
		}
		if err := compositionTreePolicy(ctx, bare, binding.HeadCommit); err != nil {
			return facts, err
		}
	}
	current := e.Sources[0].HeadCommit
	for i := 1; i < len(e.Sources); i++ {
		next := e.Sources[i].HeadCommit
		raw, err := compositionGit(ctx, bare, 1<<20, nil, "merge-tree", "--write-tree", "--name-only", "--no-messages", "-z", current, next)
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				return facts, fmt.Errorf("composition native three-way merge failed")
			}
			parts := bytes.Split(bytes.TrimRight(raw, "\x00\n"), []byte{0})
			if len(parts) < 2 || !commitID.MatchString(string(parts[0])) {
				return facts, fmt.Errorf("composition conflict evidence is invalid")
			}
			total := 0
			for _, p := range parts[1:] {
				if len(p) == 0 {
					continue
				}
				total += len(p)
				if !utf8.Valid(p) || total > 64<<10 {
					return facts, fmt.Errorf("composition conflict paths exceed the supported report bound")
				}
				facts.Conflicts = append(facts.Conflicts, string(p))
			}
			if len(facts.Conflicts) == 0 {
				return facts, fmt.Errorf("composition conflict paths are unavailable")
			}
			facts.State = "conflicted"
			facts.ConflictSourceIndex = i
			intent.Evidence = facts
			if err = saveCompositionIntent(intentPath, intent); err != nil {
				return facts, err
			}
			return facts, errCompositionConflict
		}
		tree := strings.TrimSpace(strings.TrimRight(string(raw), "\x00"))
		if !commitID.MatchString(tree) {
			return facts, fmt.Errorf("composition result tree is invalid")
		}
		if err = compositionTreePolicy(ctx, bare, tree); err != nil {
			return facts, err
		}
		date, _ := time.Parse(time.RFC3339Nano, e.FixedTimestamp)
		gitDate := strconv.FormatInt(date.Unix(), 10) + " +0000"
		message := "suma candidate composition v1\noperation: " + e.OperationID + "\nplan: " + e.PlanID + "\nstep: " + strconv.Itoa(i) + "\nsources: " + e.SourcesSHA256 + "\n"
		cmd := nativeGitCommand(ctx, bare, "commit-tree", tree, "-p", current, "-p", next)
		cmd.Stdin = strings.NewReader(message)
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME=Suma Composition", "GIT_AUTHOR_EMAIL=composition@suma.invalid", "GIT_COMMITTER_NAME=Suma Composition", "GIT_COMMITTER_EMAIL=composition@suma.invalid", "GIT_AUTHOR_DATE="+gitDate, "GIT_COMMITTER_DATE="+gitDate)
		out := newOutput(8192, nil)
		cmd.Stdout = out
		cmd.Stderr = newOutput(8192, nil)
		if err = cmd.Run(); err != nil {
			return facts, fmt.Errorf("composition commit-tree failed")
		}
		commit := strings.TrimSpace(out.b.String())
		if !commitID.MatchString(commit) {
			return facts, fmt.Errorf("composition commit identity is invalid")
		}
		facts.Parents = []string{current, next}
		if current == next {
			facts.Parents = []string{current}
		}
		facts.MergeCommits = append(facts.MergeCommits, commit)
		facts.ResultTree = tree
		current = commit
	}
	facts.ResultCommit = current
	facts.State = "completed"
	intent.Evidence = facts
	// The exact prospective commit is durable before the detached worktree ref
	// changes. Recovery inspects this intent; it never recomposes automatically.
	if err := saveCompositionIntent(intentPath, intent); err != nil {
		return facts, err
	}
	pack := filepath.Join(w.Directory, "composed.pack")
	if err := compositionPack(ctx, bare, pack, current+"\n^"+e.BaseCommit+"\n"); err != nil {
		return facts, fmt.Errorf("composition closure unavailable or exceeds 64 MiB")
	}
	if err := compositionIndexPack(ctx, w.Repository, pack); err != nil {
		return facts, fmt.Errorf("composition closure import failed")
	}
	if err := nativeBaseAttributes(ctx, w.Repository, current); err != nil {
		return facts, err
	}
	if _, err := compositionGit(ctx, w.Source, 8192, nil, "reset", "--hard", current); err != nil {
		return facts, fmt.Errorf("composition worktree publication requires reconciliation")
	}
	return facts, nil
}
