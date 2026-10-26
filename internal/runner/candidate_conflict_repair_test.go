package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func conflictRepairFixture(t *testing.T, count int, firstConflict ...int) (*nativeWorkspace, *attemptWorkspace, ExecConfig, *compositionFixture, CommandSpec) {
	t.Helper()
	repo, base := gitFixture(t)
	sources := []cc.Binding{}
	bundles := map[string][]byte{}
	first := 1
	if len(firstConflict) > 0 {
		first = firstConflict[0]
	}
	for i := 0; i < count; i++ {
		gitTest(t, repo, "checkout", "--detach", base)
		name := fmt.Sprintf("later-%d.txt", i)
		if i == 0 || i == first || first == 1 && i == 3 {
			name = "file.txt"
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte(fmt.Sprintf("source-%d\n", i)), 0600); err != nil {
			t.Fatal(err)
		}
		gitTest(t, repo, "add", name)
		gitTest(t, repo, "commit", "-m", fmt.Sprintf("source-%d", i))
		head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
		p := filepath.Join(t.TempDir(), "source.bundle")
		gitTest(t, repo, "bundle", "create", p, "HEAD", "^"+base)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("source-%d", i)
		sources = append(sources, cc.Binding{SchemaVersion: 1, TransferID: id, ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: id, NodeID: "code", WorkerID: "source", Attempt: 1, Fence: 1}, BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("a", 64), BundleSHA256: digest(data), SizeBytes: int64(len(data))})
		bundles[id] = data
	}
	gitTest(t, repo, "checkout", "--detach", base)
	f := &compositionFixture{inputs: bundles}
	server := f.server(t)
	t.Cleanup(server.Close)
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	original := compositionAssignment(t, base, sources)
	cfg := compositionConfig(t, repo, base, client)
	r := Execute(context.Background(), original, cfg)
	if r.ExitCode != 1 || r.OutcomeUnknown || f.starts != 0 {
		t.Fatalf("original conflict must remain an unsealed known failure: %+v", r)
	}
	var facts ccc.CandidateCompositionEvidence
	raw, _ := json.Marshal(r.WorkspaceEvidence["candidate_composition"])
	if json.Unmarshal(raw, &facts) != nil || facts.ConflictTree == "" || len(facts.ConflictStages) != 3 {
		t.Fatalf("missing genuine original stages: %s", raw)
	}
	assignment := Assignment{RunID: "repair-run", NodeID: "repair", WorkerID: "repairer", Type: "agent", Command: []string{"native"}, Attempt: 1, Fence: 2, Config: map[string]any{"goal": "Resolve the fixed sources."}}
	assignment.CandidateConflictRepair = &ccc.CandidateConflictRepairExecution{Protocol: ccc.CandidateConflictRepairProtocol, PlanID: "repair-plan", PlanVersion: 1, ProjectID: "project", CompositionID: original.CandidateComposition.PlanID, CompositionVersion: 2, OriginalAssignment: candidateRef(original), OriginalEvidenceSHA256: digest(raw), Composition: *original.CandidateComposition, Conflict: facts, Instructions: "Resolve each fixed conflict and preserve later sources.", FixedTimestamp: "2026-10-27T01:00:00Z", OperationID: "repair-operation"}
	if err := ccc.ValidateCandidateConflictRepairExecution(*assignment.CandidateConflictRepair); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{NativeWorkspace: &NativeWorkspacePolicy{ConflictRepair: true, Checks: []string{"unit"}}}
	ctx := context.WithValue(context.Background(), nativeGitScope{}, true)
	w, err := prepareWorkspace(ctx, assignment, cfg, spec)
	if err != nil {
		t.Fatal(err)
	}
	n, err := openNativeWorkspace(ctx, assignment, cfg, w, spec, strings.Repeat("a", 64), func(context.Context, string) (NativeWorkspaceCheckResult, error) {
		return NativeWorkspaceCheckResult{ProfileSHA256: strings.Repeat("b", 64), ExitCode: 0}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close() })
	return n, w, cfg, f, spec
}

func repairContinueArgs(t *testing.T, n *nativeWorkspace) map[string]any {
	t.Helper()
	status, err := nativeCall(t, n, "merge_status", "", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	s := status.(map[string]any)
	resolutions := []any{}
	for _, name := range n.workspace.ConflictRepair.Conflicts {
		data, err := os.ReadFile(filepath.Join(n.workspace.Source, name))
		if err != nil {
			t.Fatal(err)
		}
		resolutions = append(resolutions, map[string]any{"path": name, "expected_sha256": digest(data)})
	}
	return map[string]any{"expected_step": s["step"], "expected_generation": s["generation"], "expected_state_sha256": s["state_sha256"], "resolutions": resolutions}
}

func repairWriteConflict(t *testing.T, n *nativeWorkspace, operation, content string) {
	t.Helper()
	old, err := os.ReadFile(filepath.Join(n.workspace.Source, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = nativeCall(t, n, "write", operation, map[string]any{"path": "file.txt", "expected_sha256": digest(old), "content": content}); err != nil {
		t.Fatal(err)
	}
}

func TestCandidateConflictRepairRealGitSixteenSourcesAndSecondConflict(t *testing.T) {
	n, w, cfg, f, _ := conflictRepairFixture(t, 16)
	markers, err := os.ReadFile(filepath.Join(w.Source, "file.txt"))
	if err != nil || !conflictMarker(markers) {
		t.Fatal("actual Git conflict markers not materialized")
	}
	if got := strings.TrimSpace(gitTest(t, w.Source, "ls-files", "--unmerged")); got == "" {
		t.Fatal("true unmerged index stages missing")
	}
	if err = n.ReadyToSeal(); err == nil {
		t.Fatal("conflicted seed was sealable")
	}
	repairWriteConflict(t, n, "resolve-first", "resolved-first\n")
	if _, err = nativeCall(t, n, "check", "old-check", map[string]any{"check_id": "unit"}); err != nil {
		t.Fatal(err)
	}
	if _, err = nativeCall(t, n, "merge_continue", "continue-first", repairContinueArgs(t, n)); err != nil {
		t.Fatal(err)
	}
	if w.ConflictRepair.SourceIndex != 3 || w.ConflictRepair.State != "conflicted" {
		t.Fatalf("later conflict skipped: %+v", w.ConflictRepair)
	}
	if got, err := os.ReadFile(filepath.Join(w.Source, "later-2.txt")); err != nil || string(got) != "source-2\n" {
		t.Fatal("intervening clean source was dropped")
	}
	if len(n.Evidence().Checks) != 0 {
		t.Fatal("host seed transition retained stale checks")
	}
	repairWriteConflict(t, n, "resolve-second", "resolved-final\n")
	if _, err = nativeCall(t, n, "merge_continue", "continue-second", repairContinueArgs(t, n)); err != nil {
		t.Fatal(err)
	}
	if w.ConflictRepair.SourceIndex != 16 || len(w.ConflictRepair.Evidence.MergeCommits) != 15 || w.ConflictRepair.State != "resolved" {
		t.Fatalf("remaining frozen sources were not processed: %+v", w.ConflictRepair)
	}
	if err = n.ReadyToSeal(); err == nil {
		t.Fatal("final generation reused an earlier check")
	}
	if _, err = nativeCall(t, n, "check", "final-check", map[string]any{"check_id": "unit"}); err != nil {
		t.Fatal(err)
	}
	if err = n.ReadyToSeal(); err != nil {
		t.Fatal(err)
	}
	if err = n.Close(); err != nil {
		t.Fatal(err)
	}
	seal, err := sealNativeWorkspace(context.Background(), w, n.Evidence())
	if err != nil || seal.State != "no_change" || seal.ChangedPaths != 0 {
		t.Fatalf("real repair seal: %+v %v", seal, err)
	}
	if err = ccc.ValidateCandidateConflictRepairEvidence(w.ConflictRepair.Evidence, *w.Assignment.CandidateConflictRepair); err != nil {
		t.Fatal(err)
	}
	for _, source := range w.Assignment.CandidateConflictRepair.Composition.Sources {
		gitTest(t, w.Source, "merge-base", "--is-ancestor", source.HeadCommit, seal.Commit)
	}
	if head := strings.TrimSpace(gitTest(t, w.Repository, "rev-parse", "HEAD")); head != cfg.BaseCommit {
		t.Fatal("administrator checkout changed")
	}
	r := Result{Assignment: w.Assignment, ExitCode: 0}
	r.WorkspaceEvidence, err = w.capture(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if err = finishCandidate(context.Background(), w.Assignment, cfg, w, &r); err != nil {
		t.Fatal(err)
	}
	if f.starts != 1 || !f.binding.ValidSealed() {
		t.Fatalf("repair must upload exactly one candidate: %+v", f)
	}
	consumer := filepath.Join(t.TempDir(), "independent")
	gitTest(t, w.Repository, "clone", "--no-local", w.Repository, consumer)
	testAssignment := Assignment{RunID: w.Assignment.RunID, NodeID: "formal-test", WorkerID: "independent", Type: "test", Command: []string{"verify"}, Attempt: 1, Fence: 3, Config: map[string]any{"candidate_from": w.Assignment.NodeID}, CandidateFrom: w.Assignment.NodeID, CandidateCommit: seal.Commit, CandidateAttempt: 1, CandidateFence: 2, CandidateEvidenceSHA256: f.binding.EvidenceSHA256}
	exe, _ := os.Executable()
	testCfg := ExecConfig{AllowExec: true, Repository: consumer, Workspace: consumer, BaseCommit: cfg.BaseCommit, Artifacts: cfg.Artifacts, Commands: map[string]CommandSpec{"verify": {Executable: exe, Args: []string{"-test.run=TestCandidateConflictRepairProcessHelper", "--"}, Env: map[string]string{"SUMA_REPAIR_HELPER": "yes"}, TestEvidence: true}}}
	tested := Execute(context.Background(), testAssignment, testCfg)
	if tested.ExitCode != 0 || tested.Error != "" || tested.WorkspaceEvidence["head_commit"] != seal.Commit {
		t.Fatalf("independent exact repaired candidate test failed: %+v", tested)
	}
}

func TestCandidateConflictRepairCASMarkersAndOriginalOperationRecovery(t *testing.T) {
	n, w, cfg, _, spec := conflictRepairFixture(t, 2)
	if _, err := nativeCall(t, n, "merge_continue", "markers", repairContinueArgs(t, n)); err == nil {
		t.Fatal("unresolved original markers accepted")
	}
	repairWriteConflict(t, n, "resolve", "resolved-final\n")
	args := repairContinueArgs(t, n)
	object := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD:file.txt"))
	gitTest(t, w.Source, "update-index", "--add", "--cacheinfo", "100644", object, "unaccepted-index.txt")
	if _, err := nativeCall(t, n, "merge_continue", "changed-index", args); err == nil {
		t.Fatal("index change outside accepted file effects was accepted")
	}
	gitTest(t, w.Source, "update-index", "--force-remove", "--", "unaccepted-index.txt")
	stale := map[string]any{}
	for k, v := range args {
		stale[k] = v
	}
	stale["expected_state_sha256"] = strings.Repeat("f", 64)
	if _, err := nativeCall(t, n, "merge_continue", "stale", stale); err == nil {
		t.Fatal("stale cursor accepted")
	}
	if _, err := nativeCall(t, n, "merge_continue", "continue", args); err != nil {
		t.Fatal(err)
	}
	commit := w.ConflictRepair.CurrentCommit
	// Simulate loss of the durable acknowledgement after the exact host effect.
	n.mu.Lock()
	op := n.ledger.Operations["continue"]
	op.State = "prepared"
	op.Result = nil
	if err := n.saveLedger(); err != nil {
		t.Fatal(err)
	}
	n.mu.Unlock()
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openNativeWorkspace(context.Background(), w.Assignment, cfg, w, spec, strings.Repeat("a", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = nativeCall(t, reopened, "merge_continue", "continue", args); err != nil {
		t.Fatal("original continuation could not reconcile:", err)
	}
	if w.ConflictRepair.CurrentCommit != commit || len(w.ConflictRepair.Evidence.MergeCommits) != 1 || reopened.Evidence().Generation != 2 {
		t.Fatal("reconciliation created another merge or generation")
	}
	args["expected_step"] = 2
	if _, err = nativeCall(t, reopened, "merge_continue", "continue", args); err == nil {
		t.Fatal("same operation accepted another request")
	}
	if _, err = prepareWorkspace(context.Background(), w.Assignment, cfg, spec); err == nil {
		t.Fatal("existing attempt restarted")
	}
}

func TestCandidateConflictRepairProcessHelper(t *testing.T) {
	if os.Getenv("SUMA_REPAIR_HELPER") != "yes" {
		return
	}
	data, err := os.ReadFile("file.txt")
	if err != nil || string(data) != "resolved-final\n" {
		t.Fatal("exact resolved content missing")
	}
	for i := 2; i < 16; i++ {
		if i == 3 {
			continue
		}
		data, err = os.ReadFile(fmt.Sprintf("later-%d.txt", i))
		if err != nil || string(data) != fmt.Sprintf("source-%d\n", i) {
			t.Fatalf("source %d dropped", i)
		}
	}
}

func TestCandidateConflictRepairReplaysOriginalCleanPrefix(t *testing.T) {
	n, w, _, _, _ := conflictRepairFixture(t, 3, 2)
	e := w.Assignment.CandidateConflictRepair
	if e.Conflict.ResultTree != "" || e.Conflict.ResultCommit != "" || len(e.Conflict.MergeCommits) != 1 || w.ConflictRepair.CurrentCommit != e.Conflict.MergeCommits[0] {
		t.Fatal("conflicted report leaked a clean result or lost the actual prefix")
	}
	if data, err := os.ReadFile(filepath.Join(w.Source, "later-1.txt")); err != nil || string(data) != "source-1\n" {
		t.Fatal("original clean prefix was not materialized")
	}
	repairWriteConflict(t, n, "resolve-prefix", "resolved-final\n")
	if _, err := nativeCall(t, n, "merge_continue", "continue-prefix", repairContinueArgs(t, n)); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeCall(t, n, "check", "check-prefix", map[string]any{"check_id": "unit"}); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	seal, err := sealNativeWorkspace(context.Background(), w, n.Evidence())
	if err != nil || seal.State != "no_change" || seal.ChangedPaths != 0 {
		t.Fatalf("prefix repair seal %+v %v", seal, err)
	}
	if err = ccc.ValidateCandidateConflictRepairEvidence(w.ConflictRepair.Evidence, *e); err != nil {
		t.Fatal(err)
	}
	if len(w.ConflictRepair.Evidence.MergeCommits) != 2 || w.ConflictRepair.Evidence.MergeCommits[0] != e.Conflict.MergeCommits[0] {
		t.Fatal("original prefix identity was rewritten")
	}
}

func TestCandidateConflictRepairSealStateRetainsGenuinePrefix(t *testing.T) {
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged_prefix", true: "edited_prefix_child"}[edited], func(t *testing.T) {
			n, w, _, _, _ := conflictRepairFixture(t, 2)
			repairWriteConflict(t, n, "resolve-for-seal", "resolved-final\n")
			if _, err := nativeCall(t, n, "merge_continue", "continue-for-seal", repairContinueArgs(t, n)); err != nil {
				t.Fatal(err)
			}
			prefix := w.ConflictRepair.CurrentCommit
			if edited {
				repairWriteConflict(t, n, "post-continue-edit", "resolved-final-with-additional-edit\n")
			}
			if _, err := nativeCall(t, n, "check", "final-seal-check", map[string]any{"check_id": "unit"}); err != nil {
				t.Fatal(err)
			}
			if err := n.ReadyToSeal(); err != nil {
				t.Fatal(err)
			}
			if err := n.Close(); err != nil {
				t.Fatal(err)
			}
			seal, err := sealNativeWorkspace(context.Background(), w, n.Evidence())
			if err != nil {
				t.Fatal(err)
			}
			if seal.PreviousCommit != prefix {
				t.Fatal("seal rewrote the genuine prefix parent to administrator B")
			}
			if edited {
				if seal.State != "sealed" || seal.Commit == prefix || seal.ChangedPaths != 1 {
					t.Fatalf("edited prefix did not produce a genuine child: %+v", seal)
				}
				if parent := strings.TrimSpace(gitTest(t, w.Source, "rev-parse", seal.Commit+"^")); parent != prefix {
					t.Fatal("final edit child lost full resolved prefix lineage")
				}
			} else if seal.State != "no_change" || seal.Commit != prefix || seal.ChangedPaths != 0 {
				t.Fatalf("unchanged prefix had a fabricated new seal commit: %+v", seal)
			}
			if err = ccc.ValidateCandidateConflictRepairEvidence(w.ConflictRepair.Evidence, *w.Assignment.CandidateConflictRepair); err != nil {
				t.Fatal(err)
			}
			result := Result{Assignment: w.Assignment, ExitCode: 0}
			proof, err := w.capture(context.Background(), result)
			if err != nil {
				t.Fatal(err)
			}
			base := w.Assignment.CandidateConflictRepair.Composition.BaseCommit
			if proof["base_commit"] != base || proof["admin_base_commit"] != base || proof["head_commit"] != seal.Commit || proof["clean"] != true {
				t.Fatal("selected bundle anchor or final clean candidate changed")
			}
			for _, source := range w.Assignment.CandidateConflictRepair.Composition.Sources {
				gitTest(t, w.Source, "merge-base", "--is-ancestor", source.HeadCommit, seal.Commit)
			}
		})
	}
}
