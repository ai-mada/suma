package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestCodexCandidateSealsOnlyAfterOriginalTreeStop(t *testing.T) {
	repo, _ := gitFixture(t)
	const ignore = ".cache/\nfile.txt\nlocal-runtime.json\n"
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(ignore), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".gitignore")
	gitTest(t, repo, "commit", "-m", "fixture: protect ignored runtime files")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "codex-delivery", NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "worker", Type: "agent", Command: []string{"codex"}}
	w, err := prepareWorkspace(context.Background(), a, ExecConfig{Repository: repo, BaseCommit: base}, CommandSpec{})
	if err != nil {
		t.Fatal(err)
	}
	const edited = "code from the stopped Codex worktree\n"
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	binding := cc.Binding{
		Protocol: cc.Protocol, BudgetMode: cc.BudgetMode, IdentityEvidence: cc.IdentityEvidence,
		Model: "codex-test", ReasoningEffort: "low", ConfigurationSHA256: strings.Repeat("b", 64), CatalogSHA256: strings.Repeat("c", 64),
	}
	binding.BindingSHA256 = cc.BindingDigest(binding)
	evidence := &tc.Evidence{
		Profile: tc.Profile{Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64)}, Terminal: "completed",
		CodexExecution: &cc.Evidence{
			BindingSHA256: binding.BindingSHA256, ThreadID: "original-thread", TurnID: "original-turn",
			RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort,
			AcknowledgedModel: binding.Model, AcknowledgedReasoningEffort: binding.ReasoningEffort,
			AcknowledgedProvider: "owned", ConfigurationAcknowledged: true,
		},
	}
	unknown := Result{ToolEvidence: evidence}
	sealCodexDevelopment(context.Background(), w, binding, false, &unknown)
	if !unknown.OutcomeUnknown || unknown.ExitCode == 0 || gitTest(t, w.Source, "rev-parse", "HEAD") != w.BaseCommit+"\n" {
		t.Fatal("missing tree stop advanced the candidate")
	}
	ignored := filepath.Join(w.Source, "local-runtime.json")
	if err := os.WriteFile(ignored, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assertRefused := func(message string) {
		t.Helper()
		result := Result{ToolEvidence: evidence}
		sealCodexDevelopment(context.Background(), w, binding, true, &result)
		if result.ExitCode == 0 || !strings.Contains(result.Error, message) || gitTest(t, w.Source, "rev-parse", "HEAD") != w.BaseCommit+"\n" {
			t.Fatalf("unsafe candidate advanced: %v", result.Error)
		}
	}
	assertRefused("ignored or staged new files")
	ignoreFile := filepath.Join(w.Source, ".gitignore")
	if err := os.WriteFile(ignoreFile, []byte(".cache/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assertRefused("changed Git ignore rules")
	if err := os.WriteFile(ignoreFile, []byte(ignore), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ignored); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.Source, "new.txt"), []byte("eligible new source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	confirmed := Result{ToolEvidence: evidence}
	sealCodexDevelopment(context.Background(), w, binding, true, &confirmed)
	if confirmed.ExitCode != 0 || confirmed.Error != "" || confirmed.OutcomeUnknown {
		t.Fatalf("stopped original attempt did not seal: %s", confirmed.Error)
	}
	if got := gitTest(t, w.Source, "show", "HEAD:file.txt"); got != edited {
		t.Fatal("the sealed commit does not contain the actual edited bytes")
	}
	if got := gitTest(t, w.Source, "show", "HEAD:new.txt"); got != "eligible new source\n" {
		t.Fatal("the sealed commit lost eligible untracked source")
	}
	if got := gitTest(t, w.Repository, "rev-parse", "HEAD"); got != w.BaseCommit+"\n" {
		t.Fatal("sealing advanced the administrator branch")
	}
}
