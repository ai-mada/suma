package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func TestTerminalCandidateInventoryRejectsLaterDriftAndProtectedEdits(t *testing.T) {
	w, _ := nativeSealFixture(t)
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("terminal change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, files, err := terminalCandidateInventory(context.Background(), w)
	if err != nil || len(first.Files) != 1 || string(files["file.txt"].data) != "terminal change\n" {
		t.Fatalf("actual terminal inventory: %+v %v", first, err)
	}
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("later change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, _, err := terminalCandidateInventory(context.Background(), w)
	if err != nil || first.SHA256 == second.SHA256 {
		t.Fatalf("later mutation did not change fixed digest: %+v %v", second, err)
	}
	if err := os.WriteFile(filepath.Join(w.Source, ".env.secret"), []byte("secret=value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, safe, err := terminalCandidateInventory(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if _, exposed := safe[".env.secret"]; exposed {
		t.Fatal("protected source was copied")
	}
	gitTest(t, w.Source, "add", "file.txt")
	gitTest(t, w.Source, "commit", "-m", "unsupported shell commit")
	if _, _, err = terminalCandidateInventory(context.Background(), w); err == nil {
		t.Fatal("shell-created commit was accepted as the pinned base")
	}
}

func TestTerminalCandidateSealOnlyAcceptsCapturedFiles(t *testing.T) {
	w, _ := nativeSealFixture(t)
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, _, err := terminalCandidateInventory(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	evidence := terminalCandidateNativeEvidence(inventory, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	seal, err := sealNativeWorkspace(context.Background(), w, evidence)
	if err != nil || seal.State != "sealed" || seal.Commit == w.BaseCommit {
		t.Fatalf("real parent seal: %+v %v", seal, err)
	}
}

func TestTerminalCandidateChangedImportManifestCannotAuthorizeCheckEdits(t *testing.T) {
	w, _ := nativeSealFixture(t)
	if err := os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("captured terminal code\n"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, _, err := terminalCandidateInventory(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	source := term.TerminalCandidateSource{SessionID: "original-terminal", SessionVersion: 3, Original: cc.AssignmentRef{RunID: "original-run", NodeID: "terminal", Attempt: 1, Fence: 4, WorkerID: w.Assignment.WorkerID}, BaseCommit: w.BaseCommit, AdminBaseCommit: w.AdminBaseCommit, SnapshotSHA256: inventory.SHA256, PlanID: "protected-plan"}
	w.Assignment.Config[term.CandidateConfigKey] = source
	// A check process must not rewrite the import ledger and bless its own edits.
	inventory.Files[0].SHA256 = digest([]byte("unreviewed check mutation\n"))
	raw, _ := json.Marshal(inventory)
	if err = writeEvidence(filepath.Join(w.Directory, "terminal-candidate-import.json"), raw); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("unreviewed check mutation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := Result{ExitCode: 0}
	sealTerminalCandidate(context.Background(), w.Assignment, w, &result)
	if result.ExitCode == 0 || result.Error == "" {
		t.Fatal("changed captured manifest authorized a different candidate")
	}
	if got := gitTest(t, w.Source, "rev-parse", "HEAD"); strings.TrimSpace(got) != w.BaseCommit {
		t.Fatal("ref was changed before fixed manifest validation")
	}
}
