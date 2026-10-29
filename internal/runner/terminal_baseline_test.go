package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
)

func TestTerminalAdoptedBaselinePreservesSelectedCommitAndOriginalBundleAnchor(t *testing.T) {
	ctx := context.Background()
	repo, admin, sources, bundles := compositionSourcesFixture(t, false)
	fixture := &compositionFixture{inputs: bundles}
	server := fixture.server(t)
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	baseline := &ccc.CodeBaselineBinding{Protocol: ccc.CodeBaselineProtocol, ID: "adopted-terminal-baseline", ProjectID: "project", TaskID: "follow-up-task", TaskVersion: 2, TerminalCandidatePlanID: "accepted-source-plan", TerminalCandidatePlanVersion: 2, AcceptedBasisSHA256: strings.Repeat("b", 64), Source: sources[0]}
	baseline.BindingSHA256, _ = ccc.CodeBaselineBindingDigest(*baseline)
	terminalRepo := filepath.Join(t.TempDir(), "terminal-consumer")
	gitTest(t, repo, "clone", "--no-local", repo, terminalRepo)
	a := Assignment{RunID: "terminal-baseline-run", NodeID: "terminal", WorkerID: "terminal-developer", DispatchID: "terminal-baseline-dispatch", Attempt: 1, Fence: 131, Type: "terminal_session", Command: []string{"shell"}, CodeBaseline: baseline, Config: map[string]any{"terminal_session_id": "terminal-baseline-session", "code_baseline": baseline}}
	cfg := ExecConfig{Repository: terminalRepo, Workspace: terminalRepo, BaseCommit: admin, Artifacts: client}
	w, err := prepareWorkspace(ctx, a, cfg, CommandSpec{TerminalSession: true})
	if err != nil {
		t.Fatal("terminal did not import adopted baseline", err)
	}
	if w.BaseCommit != sources[0].HeadCommit || w.AdminBaseCommit != admin || w.candidateImport == nil || w.candidateImport.Binding != sources[0] {
		t.Fatal("selected baseline or immutable import receipt lost", w)
	}
	if got := gitTest(t, w.Source, "show", "HEAD:a.txt"); got != "source-0\n" {
		t.Fatal("terminal silently started from administrator base", got)
	}
	if err = os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("second terminal change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runtimeDir := filepath.Join(w.Directory, "terminal-journal")
	if err = os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		raw, _ := json.Marshal(value)
		if e := os.WriteFile(filepath.Join(runtimeDir, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	snapshot := terminalruntime.Snapshot{ID: "terminal-baseline-session", State: "starting", WorkDir: w.Source, ProcessID: 131, Columns: 100, Rows: 28}
	write("session.json", snapshot)
	snapshot.State = "stopped"
	snapshot.TreeStopped = true
	exit := 1
	snapshot.ExitCode = &exit
	write("terminal.json", snapshot)
	sha, err := captureTerminalCandidateSource(ctx, a, cfg, w, snapshot.ID)
	if err != nil {
		t.Fatal("selected/admin baseline source capture failed", err)
	}
	recovered, base, err := recoverTerminalCandidateSnapshot(a, runtimeDir, w.Source)
	if err != nil || recovered != sha || base != sources[0].HeadCommit {
		t.Fatal("original selected baseline capture could not recover", recovered, base, err)
	}
	source := term.TerminalCandidateSource{SessionID: snapshot.ID, SessionVersion: 4, Original: candidateRef(a), BaseCommit: w.BaseCommit, AdminBaseCommit: admin, CodeBaseline: baseline, SnapshotSHA256: sha, PlanID: "second-terminal-candidate"}
	producer := Assignment{RunID: "second-candidate-run", NodeID: "verify-changes", WorkerID: a.WorkerID, DispatchID: "second-candidate-dispatch", Attempt: 1, Fence: 2, Type: "command", Command: []string{"verify"}, CodeBaseline: baseline, Config: map[string]any{term.CandidateConfigKey: source, "tool_profile_sha256": strings.Repeat("a", 64), "code_baseline": baseline}}
	produced, err := prepareWorkspace(ctx, producer, cfg, CommandSpec{})
	if err != nil {
		t.Fatal("candidate did not freshly import its adopted baseline", err)
	}
	if err = applyTerminalCandidateSource(ctx, producer, cfg, CommandSpec{Adapter: "command", Protocol: "text-v1"}, produced); err != nil {
		t.Fatal(err)
	}
	r := Result{Assignment: producer, ExitCode: 0}
	sealTerminalCandidate(ctx, producer, produced, &r)
	if r.Error != "" || r.ExitCode != 0 {
		t.Fatal("second candidate seal failed", r)
	}
	evidence, err := produced.capture(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if evidence["base_commit"] != sources[0].HeadCommit || evidence["admin_base_commit"] != admin || evidence["code_baseline"] == nil {
		t.Fatal("produced evidence confused selected base and bundle anchor", evidence)
	}
	path, bundleSHA, size, err := createCandidateBundle(ctx, produced)
	if err != nil {
		t.Fatal(err)
	}
	if err = candidateBundleHeader(path, admin, produced.HeadCommit); err != nil {
		t.Fatal("bundle dropped original administrator prerequisite", err)
	}
	binding := sources[0]
	binding.TransferID = "second-terminal-output"
	binding.Source = candidateRef(producer)
	binding.BaseCommit = w.BaseCommit
	binding.BundleBaseCommit = admin
	binding.HeadCommit = produced.HeadCommit
	binding.BundleSHA256 = bundleSHA
	binding.SizeBytes = size
	independent := filepath.Join(t.TempDir(), "independent-test-repository")
	gitTest(t, repo, "clone", "--no-local", repo, independent)
	if err = importCandidateFile(ctx, independent, path, binding); err != nil {
		t.Fatal("independent worker could not import full anchored candidate", err)
	}
	if got := gitTest(t, independent, "show", binding.HeadCommit+":a.txt"); got != "source-0\n" {
		t.Fatal("new bundle lost adopted source", got)
	}
	if got := gitTest(t, independent, "show", binding.HeadCommit+":file.txt"); got != "second terminal change\n" {
		t.Fatal("new bundle lost second terminal edits", got)
	}
	if len(fixture.reads) != 2 || fixture.reads[0].Assignment != candidateRef(a) || fixture.reads[1].Assignment != candidateRef(producer) {
		t.Fatal("baseline reuse skipped current source authorization", fixture.reads)
	}
	// Presence of M in this repository is insufficient when the current
	// source read is denied. There is no fallback to either M or admin B.
	fixture.mu.Lock()
	delete(fixture.inputs, baseline.Source.TransferID)
	fixture.mu.Unlock()
	denied := a
	denied.RunID = "revoked-terminal-run"
	denied.Fence++
	if got, e := prepareWorkspace(ctx, denied, cfg, CommandSpec{TerminalSession: true}); e == nil || got != nil {
		t.Fatal("locally cached source bypassed current authorization", got, e)
	}
	if got := strings.TrimSpace(gitTest(t, terminalRepo, "rev-parse", "HEAD")); got != admin {
		t.Fatal("denied baseline mutated administrator checkout", got)
	}
	wrong := producer
	wrong.CodeBaseline = cloneTerminalCodeBaseline(baseline)
	wrong.CodeBaseline.ID = "another-baseline"
	wrong.CodeBaseline.BindingSHA256, _ = ccc.CodeBaselineBindingDigest(*wrong.CodeBaseline)
	if codeBaselineInputNode(wrong) {
		t.Fatal("candidate source accepted another adopted binding")
	}
}

func TestTerminalBaselineCannotFallBackOrMixSourceBindings(t *testing.T) {
	a := Assignment{Type: "terminal_session", RunID: "terminal-run", NodeID: "terminal", Attempt: 1, Fence: 2, WorkerID: "worker", DispatchID: "dispatch", Command: []string{"shell"}, CodeBaseline: &ccc.CodeBaselineBinding{Protocol: ccc.CodeBaselineProtocol}, Config: map[string]any{"terminal_session_id": "session"}}
	if _, err := candidateBase(context.Background(), "unused", a, ExecConfig{BaseCommit: strings.Repeat("a", 40)}); err == nil {
		t.Fatal("terminal silently fell back from a missing baseline proof")
	}
	if codeBaselineInputNode(a) {
		t.Fatal("incomplete typed baseline acquired terminal import capability")
	}
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	a.CodeBaseline.ID = "caller-mutated-binding"
	if j.entries[key(a)].Assignment.CodeBaseline.ID != "" {
		t.Fatal("caller mutation changed the immutable baseline inbox")
	}
}
