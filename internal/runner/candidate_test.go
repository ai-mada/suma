package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func candidateFixture(t *testing.T) (ExecConfig, Assignment, string, string) {
	t.Helper()
	repo, base := gitFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "file.txt")
	gitTest(t, repo, "commit", "-m", "fixture candidate revision")
	candidate := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("user dirty draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"verify": {Executable: exe, Args: []string{"-test.run=TestCandidateHelper", "--"}, TestEvidence: true, Env: map[string]string{"SUMA_CANDIDATE_HELPER": "1"}}}}
	a := Assignment{RunID: "candidate-run", NodeID: "test", WorkerID: "candidate-worker", Attempt: 1, Fence: 1, Type: "test", Command: []string{"verify"}, Config: map[string]any{"candidate_from": "code"}, CandidateCommit: candidate, CandidateFrom: "code", CandidateAttempt: 2, CandidateFence: 5, CandidateEvidenceSHA256: strings.Repeat("a", 64)}
	return cfg, a, base, candidate
}

func TestCandidateEnvelopeSelectsExactLocalCommittedRevision(t *testing.T) {
	cfg, a, base, candidate := candidateFixture(t)
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.Error != "" || !strings.Contains(r.Output, "tested candidate") {
		t.Fatalf("candidate execution: %+v", r)
	}
	e := r.WorkspaceEvidence
	if e["base_commit"] != candidate || e["head_commit"] != candidate || e["admin_base_commit"] != base || e["candidate_from"] != "code" || e["candidate_attempt"] != 2 || e["candidate_fence"] != int64(5) || e["candidate_evidence_sha256"] != a.CandidateEvidenceSHA256 || e["clean"] != true || e["test_command"] != true {
		t.Fatalf("candidate proof: %+v", e)
	}
	draft, err := os.ReadFile(filepath.Join(cfg.Repository, "file.txt"))
	if err != nil || string(draft) != "user dirty draft\n" {
		t.Fatal("candidate test changed user checkout")
	}
	if head := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD")); head != candidate {
		t.Fatal("candidate test moved user HEAD")
	}
	// The tested candidate already contains the source code commit. A read-only
	// preflight may approve it even though this test attempt made no new commit.
	if err = os.WriteFile(filepath.Join(cfg.Repository, "file.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	request := MergePreflightRequest{AttemptKey: e["attempt_key"].(string), ExpectedTargetHEAD: candidate, ExpectedCandidateHEAD: candidate, ExpectedEvidenceSHA256: e["evidence_sha256"].(string), ReviewedBy: "fixture reviewer"}
	preflight, err := MergePreflight(context.Background(), cfg, request)
	if err != nil || !preflight.Ready || preflight.Merged {
		t.Fatalf("candidate preflight: %+v %v", preflight, err)
	}
}

func TestCandidateEnvelopeRejectsIncompleteWrongTypeAndUntrustedSource(t *testing.T) {
	cfg, a, _, _ := candidateFixture(t)
	cases := map[string]func(*Assignment, *ExecConfig){
		"missing-commit":         func(a *Assignment, _ *ExecConfig) { a.CandidateCommit = "" },
		"missing-source":         func(a *Assignment, _ *ExecConfig) { a.CandidateFrom = "" },
		"missing-source-attempt": func(a *Assignment, _ *ExecConfig) { a.CandidateAttempt = 0 },
		"missing-source-fence":   func(a *Assignment, _ *ExecConfig) { a.CandidateFence = 0 },
		"missing-source-hash":    func(a *Assignment, _ *ExecConfig) { a.CandidateEvidenceSHA256 = "" },
		"invalid-source-hash":    func(a *Assignment, _ *ExecConfig) { a.CandidateEvidenceSHA256 = strings.Repeat("x", 64) },
		"not-test":               func(a *Assignment, _ *ExecConfig) { a.Type = "command" },
		"external-workflow":      func(a *Assignment, _ *ExecConfig) { a.Type = "external_workflow" },
		"config-source-mismatch": func(a *Assignment, _ *ExecConfig) { a.Config = map[string]any{"candidate_from": "different"} },
		"config-source-absent":   func(a *Assignment, _ *ExecConfig) { a.Config = nil },
		"shared-read-only": func(_ *Assignment, c *ExecConfig) {
			spec := c.Commands["verify"]
			spec.ReadOnly = true
			c.Commands = map[string]CommandSpec{"verify": spec}
		},
		"not-admin-test": func(_ *Assignment, c *ExecConfig) {
			spec := c.Commands["verify"]
			spec.TestEvidence = false
			c.Commands = map[string]CommandSpec{"verify": spec}
		},
		"object-not-local": func(a *Assignment, _ *ExecConfig) { a.CandidateCommit = strings.Repeat("f", 40) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			copyA := a
			copyCfg := cfg
			mutate(&copyA, &copyCfg)
			r := Execute(context.Background(), copyA, copyCfg)
			if r.ExitCode == 0 || r.Error == "" || r.WorkspaceEvidence != nil {
				t.Fatalf("unsafe candidate allowed: %+v", r)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(cfg.Repository, ".cache", "runner", "worktrees", attemptKey(a))); !os.IsNotExist(err) {
		t.Fatal("invalid envelope created an attempt source")
	}
}

func TestCandidateCommitMustDescendFromAdministratorBase(t *testing.T) {
	cfg, a, _, _ := candidateFixture(t)
	tree := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD^{tree}"))
	orphan := strings.TrimSpace(gitTest(t, cfg.Repository, "commit-tree", tree, "-m", "fixture unrelated candidate"))
	a.CandidateCommit = orphan
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode == 0 || !strings.Contains(r.Error, "lineage") {
		t.Fatalf("unrelated candidate: %+v", r)
	}
}

func TestCandidateHelper(t *testing.T) {
	if os.Getenv("SUMA_CANDIDATE_HELPER") == "1" {
		b, err := os.ReadFile("file.txt")
		if err != nil || string(b) != "candidate\n" {
			os.Exit(6)
		}
		_, _ = os.Stdout.WriteString("tested candidate\n")
		os.Exit(0)
	}
}
