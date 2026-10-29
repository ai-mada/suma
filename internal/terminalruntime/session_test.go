package terminalruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This is an actual PTY/OS-process test, not a mocked stream. Linux/macOS
// descendant containment needs the deployment cgroup/container integration;
// the Windows implementation can prove its Job is empty locally.
func TestTerminalRuntimeActualInputReceiptResizeAndTreeStop(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("actual Windows ConPTY and Job containment test")
	}
	executable, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := Config{ID: "terminal-original-1", Executable: executable, BootstrapExecutable: self, BootstrapArgs: []string{"-test.run=^TestTerminalRuntimeBootstrapChild$", "--"}, Args: []string{"/d", "/q", "/k"}, WorkDir: root, JournalDir: filepath.Join(root, "journal"), Env: []string{"SystemRoot=" + os.Getenv("SystemRoot"), "TEMP=" + root, "TMP=" + root, "PATH=" + filepath.Dir(executable)}, MaxDuration: time.Minute}
	s, err := Start(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		// Preserve bounded diagnostic facts before the fixture's automatic temp
		// cleanup; do not lose a failed interactive run's original byte evidence.
		evidence, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", "terminal-129", "runtime-failure-"+time.Now().UTC().Format("20060102T150405.000000000Z")))
		if err != nil {
			t.Log(err)
			return
		}
		if err = os.MkdirAll(evidence, 0700); err != nil {
			t.Log(err)
			return
		}
		page := s.ReadOutput(0, 128*1024)
		var raw []byte
		for _, chunk := range page.Chunks {
			raw = append(raw, chunk.Data...)
		}
		count, queryErr := s.ActiveProcesses()
		facts, _ := json.MarshalIndent(map[string]any{"snapshot": s.Snapshot(), "job_active_processes": count, "job_query_error": fmt.Sprint(queryErr), "output_first_sequence": page.FirstSequence, "output_last_sequence": page.LastSequence, "output_gap": page.Gap, "output_sha256": digest(raw)}, "", "  ")
		_ = os.WriteFile(filepath.Join(evidence, "facts.public.json"), facts, 0600)
		_ = os.WriteFile(filepath.Join(evidence, "output.bin"), raw, 0600)
		t.Logf("preserved original failed PTY facts at %s", evidence)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	text := []byte("chcp 65001>nul & echo terminal-runtime-original-marker>>once.txt\r")
	first, err := s.SubmitInput(ctx, "original-input", 1, text)
	if err != nil || first.State != "delivered" {
		t.Fatalf("input: %+v %v", first, err)
	}
	repeated, err := s.SubmitInput(ctx, "original-input", 1, text)
	if err != nil || repeated != first {
		t.Fatalf("repeat: %+v %v", repeated, err)
	}
	if _, err = s.SubmitInput(ctx, "original-input", 1, []byte("echo changed\r")); !errors.Is(err, ErrInputConflict) {
		t.Fatalf("changed original input: %v", err)
	}
	if err = s.Resize(120, 35); err != nil {
		t.Fatal(err)
	}
	send := func(id string, seq uint64, data []byte) {
		t.Helper()
		if _, e := s.SubmitInput(ctx, id, seq, data); e != nil {
			t.Fatalf("input %s: %v", id, e)
		}
	}
	send("barrier-input", 2, []byte("type once.txt & echo 终^端验证 & echo terminal-runtime-barr^ier\r"))
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waitOutput := func(marker string) {
		t.Helper()
		var output []byte
		for !bytes.Contains(output, []byte(marker)) {
			select {
			case <-ctx.Done():
				t.Fatalf("actual output did not arrive: %q", output)
			case <-ticker.C:
			}
			page := s.ReadOutput(0, 128*1024)
			output = nil
			for _, chunk := range page.Chunks {
				output = append(output, chunk.Data...)
			}
		}
	}
	waitOutput("terminal-runtime-barrier")
	rawOnce, err := os.ReadFile(filepath.Join(root, "once.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(rawOnce), "terminal-runtime-original-marker") != 1 {
		t.Fatalf("original input physically repeated: %q", rawOnce)
	}
	var output []byte
	for _, chunk := range s.ReadOutput(0, 128*1024).Chunks {
		output = append(output, chunk.Data...)
	}
	if !bytes.Contains(output, []byte("终端验证")) {
		t.Fatalf("UTF-8 output missing: %q", output)
	}
	baselineProcesses, err := s.ActiveProcesses()
	if err != nil || baselineProcesses != 2 {
		t.Fatalf("real shell+contained bootstrap: processes=%d err=%v", baselineProcesses, err)
	}
	// The barrier after Ctrl-C can execute only if the foreground wait was
	// interrupted. The final background child must be included in Job stop.
	waitFor := func(label string, condition func() bool) {
		t.Helper()
		for !condition() {
			select {
			case <-ctx.Done():
				t.Fatalf("actual %s was not observed", label)
			case <-ticker.C:
			}
		}
	}
	childActive := func() bool {
		count, e := s.ActiveProcesses()
		if e != nil {
			t.Fatal(e)
		}
		return count > baselineProcesses
	}
	fileHas := func(name string) bool {
		raw, e := os.ReadFile(filepath.Join(root, name))
		return e == nil && strings.TrimSpace(string(raw)) == "done"
	}
	send("foreground-input", 3, []byte("ping.exe -t 127.0.0.1>nul\r"))
	waitFor("foreground child in original Job", childActive)
	send("interrupt-input", 4, []byte{3})
	// Console Ctrl-C processing can flush buffered keyboard records. Observe
	// the original foreground child's actual termination before typing the next
	// command, rather than racing that command against asynchronous Ctrl-C.
	waitFor("foreground child exited after Ctrl-C", func() bool {
		count, e := s.ActiveProcesses()
		if e != nil {
			t.Fatal(e)
		}
		return count == baselineProcesses
	})
	send("interrupt-barrier", 5, []byte("echo done>interrupt-done.txt\r"))
	waitFor("physical file after foreground Ctrl-C", func() bool { return fileHas("interrupt-done.txt") })
	send("background-input", 6, []byte("start \"\" /b ping.exe -t 127.0.0.1>nul & echo done>child-ready.txt\r"))
	waitFor("physical background launch barrier", func() bool { return fileHas("child-ready.txt") })
	waitFor("background child in original Job", childActive)
	if err = s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	if state.State != "stopped" || !state.TreeStopped || state.InputCount != 6 || state.Columns != 120 || state.Rows != 35 {
		t.Fatalf("terminal facts: %+v", state)
	}
	if _, err = Start(c); !errors.Is(err, ErrJournalExists) {
		t.Fatalf("journal restarted a process: %v", err)
	}
	inspection, err := InspectJournal(c.JournalDir)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Session.State != "stopped" || len(inspection.Inputs) != 6 || inspection.Inputs[0] != first {
		t.Fatalf("durable receipt: %+v", inspection)
	}
	raw, err := os.ReadFile(filepath.Join(c.JournalDir, inputName(1, "intent")))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, text) || bytes.Contains(raw, []byte("terminal-runtime-original-marker")) {
		t.Fatal("journal stored terminal input body")
	}
}

// The subprocess uses the same production bootstrap as suma-runner. The
// surrounding Go-test main only routes the dedicated child-mode arguments.
func TestTerminalRuntimeBootstrapChild(t *testing.T) {
	for i, arg := range os.Args {
		if arg == BootstrapFlag {
			handled, code := HandleChildProcess(os.Args[i:])
			if !handled {
				t.Fatal("bootstrap mode not handled")
			}
			os.Exit(code)
		}
	}
	t.Skip("dedicated terminal subprocess entrypoint")
}

func TestTerminalRuntimeOutputFileBoundAndExplicitReplayGap(t *testing.T) {
	root := t.TempDir()
	journal, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	s := &Session{journal: journal, config: Config{MaxOutputBytes: MaxChunkBytes, MaxOutputChunks: 2}}
	for _, data := range []string{"first", "second", "third", "fourth"} {
		if err = s.retainOutput([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	page := s.ReadOutput(0, 128*1024)
	if !page.Gap || page.FirstSequence != 3 || page.LastSequence != 4 || page.NextSequence != 4 || len(page.Chunks) != 2 || string(page.Chunks[0].Data) != "third" || page.Chunks[1].ByteOffset != uint64(len("firstsecondthird")) {
		t.Fatalf("bounded output: %+v", page)
	}
	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("small chunks retained %d files", len(files))
	}
	if replay := s.ReadOutput(3, 128*1024); replay.Gap || len(replay.Chunks) != 1 || string(replay.Chunks[0].Data) != "fourth" {
		t.Fatalf("current replay: %+v", replay)
	}
}

func TestTerminalRuntimeCrashIntentDoesNotInventDelivery(t *testing.T) {
	root := t.TempDir()
	store := func(name string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	store("session.json", Snapshot{ID: "original", State: "running", ProcessID: 12345, WorkDir: root})
	store(inputName(1, "intent"), InputReceipt{InputID: "original-input", Sequence: 1, SHA256: digest([]byte("sensitive-input")), Bytes: len("sensitive-input"), State: "delivery_unknown"})
	result, err := InspectJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.State != "reconciling" || result.Session.TreeStopped || len(result.Inputs) != 1 || result.Inputs[0].State != "delivery_unknown" {
		t.Fatalf("crash facts: %+v", result)
	}
	store(inputName(1, "result"), InputReceipt{InputID: "another-input", Sequence: 1, SHA256: strings.Repeat("a", 64), Bytes: 4, State: "delivered", BytesWritten: 4})
	if _, err = InspectJournal(root); err == nil {
		t.Fatal("forged final input receipt accepted")
	}
}
