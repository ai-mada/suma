package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
)

type continuation186Fixture struct {
	producer, consumer, base string
	source                   cc.Binding
	bundle                   []byte
	transport                *compositionFixture
	client                   *Client
}

// This fixture exercises owned Git workspaces, capture and the original bundle
// transport. It launches no coding CLI and makes no vendor/model claim.
func newContinuation186Fixture(t *testing.T) continuation186Fixture {
	t.Helper()
	ctx := context.Background()
	producer, base := gitFixture(t)
	consumer := filepath.Join(t.TempDir(), "consumer")
	gitTest(t, producer, "clone", "--no-local", producer, consumer)
	transport := &compositionFixture{inputs: map[string][]byte{}}
	server := transport.server(t)
	t.Cleanup(server.Close)
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	assignment := Assignment{RunID: "prior-task-run", NodeID: "produce", WorkerID: "prior-worker", Attempt: 1, Fence: 4, Type: "command", Command: []string{"owned-git-fixture"}}
	cfg := ExecConfig{AllowExec: true, Repository: producer, Workspace: producer, BaseCommit: base, Artifacts: client}
	workspace, err := prepareWorkspace(ctx, assignment, cfg, CommandSpec{Executable: "git"})
	if err != nil {
		t.Fatal(err)
	}
	commitContinuation186(t, workspace.Source, "previously sealed code\n", "previous candidate M")
	result := Result{Assignment: assignment, ExitCode: 0}
	result.WorkspaceEvidence, err = workspace.capture(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	if err = finishCandidate(ctx, assignment, cfg, workspace, &result); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	source, bundle := transport.binding, append([]byte(nil), transport.bundle...)
	transport.inputs[source.TransferID] = append([]byte(nil), bundle...)
	transport.mu.Unlock()
	if !source.ValidSealed() || source.HeadCommit == base {
		t.Fatal("fixture did not seal a real prior candidate")
	}
	return continuation186Fixture{producer: producer, consumer: consumer, base: base, source: source, bundle: bundle, transport: transport, client: client}
}

func commitContinuation186(t *testing.T, repository, content, message string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repository, "file.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repository, "add", "file.txt")
	gitTest(t, repository, "-c", "user.name=Continuation Fixture", "-c", "user.email=continuation@example.invalid", "commit", "-m", message)
	return strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD"))
}

func continuation186Assignment(t *testing.T, source cc.Binding, attempt int) Assignment {
	t.Helper()
	binding := &dc.DevelopmentContinuationBinding{Protocol: dc.ContinuationProtocol, ID: "continuation-plan", ProjectID: source.ProjectID, TaskID: "same-task", TaskVersion: 4, SourceTaskRunID: source.Source.RunID, SourceTaskVersion: 2, SourceRunVersion: 3, Source: source}
	var err error
	binding.BindingSHA256, err = dc.BindingDigest(*binding)
	if err != nil {
		t.Fatal(err)
	}
	return Assignment{RunID: "next-task-run", NodeID: "develop", WorkerID: "next-worker", Attempt: attempt, Fence: int64(40 + attempt), Type: "agent", Command: []string{"develop"}, Config: map[string]any{"goal": "Continue the explicitly reviewed prior candidate", "development_continuation_id": binding.ID}, DevelopmentContinuation: binding}
}

func continuation186Spec(t *testing.T) CommandSpec {
	t.Helper()
	// Use the original ordinary writable Codex preset only for workspace
	// selection. This owned test executable is never launched as a model tool.
	spec := localToolDirectory()[0].command
	var err error
	spec.Executable, err = os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestDevelopmentContinuationRealGitNewAttemptsPreserveAdministratorAnchor(t *testing.T) {
	ctx := context.Background()
	f := newContinuation186Fixture(t)
	spec := continuation186Spec(t)
	cfg := ExecConfig{AllowExec: true, Repository: f.consumer, Workspace: f.consumer, BaseCommit: f.base, Artifacts: f.client, Commands: map[string]CommandSpec{"develop": spec}}
	if _, err := gitText(ctx, f.consumer, "rev-parse", "--verify", f.source.HeadCommit+"^{commit}"); err == nil {
		t.Fatal("independent consumer already had candidate M")
	}
	const administratorDraft = "administrator draft remains private\n"
	if err := os.WriteFile(filepath.Join(f.consumer, "file.txt"), []byte(administratorDraft), 0600); err != nil {
		t.Fatal(err)
	}
	assignment := continuation186Assignment(t, f.source, 1)
	workspace, err := prepareWorkspace(ctx, assignment, cfg, spec)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.BaseCommit != f.source.HeadCommit || workspace.AdminBaseCommit != f.base {
		t.Fatal("selected M and administrator anchor B were confused")
	}
	if data, err := os.ReadFile(filepath.Join(workspace.Source, "file.txt")); err != nil || string(data) != "previously sealed code\n" {
		t.Fatal("new workspace did not start from reviewed M", err)
	}
	head := commitContinuation186(t, workspace.Source, "candidate\n", "continued candidate N")
	result := Result{Assignment: assignment, ExitCode: 0}
	result.WorkspaceEvidence, err = workspace.capture(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	var receipt cc.ImportReceipt
	raw, _ := json.Marshal(result.WorkspaceEvidence["candidate_import"])
	if json.Unmarshal(raw, &receipt) != nil || !receipt.Imported || receipt.TargetWorkerID != assignment.WorkerID || receipt.Binding != f.source {
		t.Fatalf("original import receipt was lost: %s", raw)
	}
	actual, _ := json.Marshal(result.WorkspaceEvidence["development_continuation"])
	expected, _ := json.Marshal(assignment.DevelopmentContinuation)
	if !bytes.Equal(actual, expected) || result.WorkspaceEvidence["code_baseline"] != nil || result.WorkspaceEvidence["base_commit"] != f.source.HeadCommit || result.WorkspaceEvidence["admin_base_commit"] != f.base {
		t.Fatal("captured continuation acquired accepted-baseline authority or lost its exact source")
	}
	if err = finishCandidate(ctx, assignment, cfg, workspace, &result); err != nil {
		t.Fatal(err)
	}
	f.transport.mu.Lock()
	produced := f.transport.binding
	f.transport.mu.Unlock()
	if !produced.ValidSealed() || produced.HeadCommit != head || head == f.source.HeadCommit || produced.BaseCommit != f.source.HeadCommit || produced.BundleBase() != f.base || produced.EvidenceSHA256 != result.WorkspaceEvidence["evidence_sha256"] {
		t.Fatalf("N did not retain selected M / full B anchor: %+v", produced)
	}
	if err = candidateBundleHeader(filepath.Join(workspace.Directory, "candidate.bundle"), f.base, head); err != nil {
		t.Fatal("new bundle was not anchored to administrator B", err)
	}
	gitTest(t, f.consumer, "merge-base", "--is-ancestor", f.source.HeadCommit, head)
	// Reuse the original independently owned test-process helper against N,
	// through legacy candidate_from, never the prior candidate M's test result.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	testAssignment := Assignment{RunID: assignment.RunID, NodeID: "independent-test", WorkerID: "independent-tester", Attempt: 1, Fence: 99, Type: "test", Command: []string{"verify-N"}, Config: map[string]any{"candidate_from": assignment.NodeID}, CandidateFrom: assignment.NodeID, CandidateCommit: head, CandidateAttempt: assignment.Attempt, CandidateFence: assignment.Fence, CandidateEvidenceSHA256: produced.EvidenceSHA256}
	testConfig := ExecConfig{AllowExec: true, Repository: f.consumer, Workspace: f.consumer, BaseCommit: f.base, Artifacts: f.client, Commands: map[string]CommandSpec{"verify-N": {Executable: executable, Args: []string{"-test.run=TestCandidateTransferCommandHelper", "--"}, Env: map[string]string{"SUMA_CANDIDATE_HELPER": "test"}, TestEvidence: true}}}
	tested := Execute(ctx, testAssignment, testConfig)
	if tested.ExitCode != 0 || tested.Error != "" || tested.OutcomeUnknown || tested.WorkspaceEvidence["head_commit"] != head || tested.WorkspaceEvidence["test_command"] != true || tested.WorkspaceEvidence["candidate_evidence_sha256"] != produced.EvidenceSHA256 {
		t.Fatalf("independent original test process did not consume exact N: %+v", tested)
	}

	// M is now local, but a new attempt still needs a fresh current-consumer
	// HTTP import and a separate worktree, never the first attempt's directory.
	again := continuation186Assignment(t, f.source, 2)
	second, err := prepareWorkspace(ctx, again, cfg, spec)
	if err != nil {
		t.Fatal(err)
	}
	if second.Directory == workspace.Directory || second.BaseCommit != f.source.HeadCommit || second.AdminBaseCommit != f.base || second.candidateImport == nil || !second.candidateImport.Imported {
		t.Fatal("new attempt reused prior workspace or skipped its import receipt")
	}
	if data, err := os.ReadFile(filepath.Join(second.Source, "file.txt")); err != nil || string(data) != "previously sealed code\n" {
		t.Fatal("new attempt inherited N or an unreviewed old working directory", err)
	}
	f.transport.mu.Lock()
	reads := append([]ccc.CodeInputRequest(nil), f.transport.reads...)
	f.transport.mu.Unlock()
	if len(reads) != 2 {
		t.Fatalf("every new attempt requires one fresh authorized HTTP import, got %d", len(reads))
	}
	for i, request := range reads {
		original := []Assignment{assignment, again}[i]
		if request.InputKind != "development_continuation" || request.BindingID != assignment.DevelopmentContinuation.ID || request.TransferID != f.source.TransferID || request.Assignment != candidateRef(original) {
			t.Fatalf("code input escaped its current attempt / original plan: %+v", request)
		}
	}
	for _, repository := range []string{f.producer, f.consumer} {
		if got := strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD")); got != f.base {
			t.Fatal("administrator checkout HEAD changed", repository)
		}
	}
	if data, err := os.ReadFile(filepath.Join(f.consumer, "file.txt")); err != nil || string(data) != administratorDraft || cfg.BaseCommit != f.base {
		t.Fatal("administrator baseline or existing draft was changed", err)
	}
}

func TestDevelopmentContinuationRejectsUntrustedBindingBundleAndAnchor(t *testing.T) {
	ctx := context.Background()
	f := newContinuation186Fixture(t)
	spec := continuation186Spec(t)
	localBundle := filepath.Join(t.TempDir(), "already-local-M.bundle")
	if err := os.WriteFile(localBundle, f.bundle, 0600); err != nil {
		t.Fatal(err)
	}
	if err := importCandidateFile(ctx, f.consumer, localBundle, f.source); err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"binding_digest", "bundle_hash", "binding_anchor", "bundle_header_anchor", "authorization_denied"} {
		t.Run(kind, func(t *testing.T) {
			assignment := continuation186Assignment(t, f.source, i+10)
			binding := assignment.DevelopmentContinuation
			data := append([]byte(nil), f.bundle...)
			wantReads := 1
			switch kind {
			case "binding_digest":
				binding.TaskVersion++
				wantReads = 0
			case "bundle_hash":
				binding.Source.BundleSHA256 = strings.Repeat("f", 64)
			case "binding_anchor":
				binding.Source.BundleBaseCommit = strings.Repeat("d", 40)
				wantReads = 0
			case "bundle_header_anchor":
				needle := []byte("-" + f.base + " ")
				if !bytes.Contains(data, needle) {
					t.Fatal("fixture bundle lacks its real B prerequisite")
				}
				data = bytes.Replace(data, needle, []byte("-"+strings.Repeat("d", 40)+" "), 1)
				binding.Source.BundleSHA256 = digest(data)
			}
			if kind != "binding_digest" {
				var err error
				binding.BindingSHA256, err = dc.BindingDigest(*binding)
				if err != nil {
					t.Fatal(err)
				}
			}
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				var request ccc.CodeInputRequest
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runner/code-inputs/bundle" || r.Header.Get("Authorization") != "Bearer fixture" || json.NewDecoder(r.Body).Decode(&request) != nil || request.InputKind != "development_continuation" || request.BindingID != binding.ID || request.TransferID != binding.Source.TransferID || request.Assignment != candidateRef(assignment) {
					t.Error("invalid or unscoped continuation request")
					w.WriteHeader(400)
					return
				}
				if kind == "authorization_denied" {
					w.WriteHeader(403)
					return
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			cfg := ExecConfig{AllowExec: true, Repository: f.consumer, Workspace: f.consumer, BaseCommit: f.base, Artifacts: &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}, Commands: map[string]CommandSpec{"develop": spec}}
			if workspace, err := prepareWorkspace(ctx, assignment, cfg, spec); err == nil || workspace != nil {
				t.Fatal("tampered or unauthorized continuation produced a workspace", kind)
			}
			if reads != wantReads {
				t.Fatalf("unexpected current-authority reads: got %d want %d", reads, wantReads)
			}
			attempt := filepath.Join(f.consumer, ".cache", "runner", "worktrees", attemptKey(assignment))
			if _, err := os.Lstat(attempt); !os.IsNotExist(err) {
				t.Fatal("rejected input created an execution worktree", err)
			}
			if got := strings.TrimSpace(gitTest(t, f.consumer, "rev-parse", "HEAD")); got != f.base {
				t.Fatal("rejected input moved the administrator checkout")
			}
		})
	}
}
