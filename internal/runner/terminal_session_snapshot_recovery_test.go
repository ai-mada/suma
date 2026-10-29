package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestTerminalRestartRecoversCapturedSourceBeforeFirstSnapshotExchange(t *testing.T) {
	for _, state := range []string{"stopped", "exited"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			repo, _ := gitFixture(t)
			base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
			a := Assignment{RunID: "terminal-captured-run", NodeID: "terminal", Type: "terminal_session", Attempt: 1, Fence: 130, WorkerID: "original-terminal-worker", DispatchID: "original-terminal-dispatch", Command: []string{"shell"}, Config: map[string]any{"terminal_session_id": "original-terminal-session"}}
			cfg := ExecConfig{Repository: repo, BaseCommit: base}
			w, err := prepareWorkspace(ctx, a, cfg, CommandSpec{})
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(w.Source, "file.txt"), []byte("original stopped terminal changes\n"), 0600); err != nil {
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
			snapshot := terminalruntime.Snapshot{ID: "original-terminal-session", State: "starting", WorkDir: w.Source, ProcessID: 130, Columns: 100, Rows: 28}
			write("session.json", snapshot)
			snapshot.State, snapshot.TreeStopped = state, true
			exit := 1
			snapshot.ExitCode = &exit
			write("terminal.json", snapshot)
			unknown := terminalruntime.InputReceipt{InputID: "original-input-130", Sequence: 1, SHA256: digest([]byte("original ambiguous input\r")), Bytes: len("original ambiguous input\r"), State: "delivery_unknown"}
			write("input-00000000000000000001.intent.json", unknown)
			journalDir := filepath.Join(w.Directory, "runner-journal")
			j, err := OpenJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			profile := tc.Profile{CommandID: "shell", Adapter: "command", Protocol: "text-v1", ToolVersion: "metadata recovery fixture", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), TerminalSessionProtocol: term.Protocol}
			if err = j.checkpointTerminalRuntime(a, runtimeDir, w.Source, profile); err != nil {
				t.Fatal(err)
			}
			snapshotSHA, err := captureTerminalCandidateSource(ctx, a, cfg, w, snapshot.ID)
			if err != nil || snapshotSHA == "" {
				t.Fatal("original source capture failed", snapshotSHA, err)
			}
			manifestPath := filepath.Join(w.Directory, "terminal-candidate-source.json")
			originalManifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			// Crash window: the original manifest is durable, but no source
			// exchange or final result has yet entered the transport outbox.
			if len(j.pendingTerminalExchanges()) != 0 {
				t.Fatal("fixture already exchanged the original snapshot")
			}
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			j, err = OpenJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			pending := j.pendingTerminalExchanges()
			if len(pending) != 1 || pending[0].Request.SourceSnapshotSHA256 != snapshotSHA || pending[0].Request.BaseCommit != base || !pending[0].Request.TreeStopped || pending[0].Request.State != state {
				t.Fatal("restart stranded the original durable source receipt", pending)
			}
			if len(pending[0].Request.InputReceipts) != 1 || pending[0].Request.InputReceipts[0].State != "delivery_unknown" || len(j.Pending()) != 1 || !j.Pending()[0].OutcomeUnknown {
				t.Fatal("source recovery reclassified an unknown input effect", pending, j.Pending())
			}
			operation := pending[0].Request.OperationID
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			j, err = OpenJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			pending = j.pendingTerminalExchanges()
			if len(pending) != 1 || pending[0].Request.OperationID != operation || pending[0].Request.SourceSnapshotSHA256 != snapshotSHA {
				t.Fatal("second restart invented another snapshot exchange", pending)
			}
			afterManifest, err := os.ReadFile(manifestPath)
			if err != nil || !bytes.Equal(originalManifest, afterManifest) {
				t.Fatal("recovery rewrote the original source receipt", err)
			}
			content, err := os.ReadFile(filepath.Join(w.Source, "file.txt"))
			if err != nil || string(content) != "original stopped terminal changes\n" || strings.TrimSpace(gitTest(t, w.Source, "rev-parse", "HEAD")) != base {
				t.Fatal("recovery executed or modified source files", err)
			}
		})
	}
}
