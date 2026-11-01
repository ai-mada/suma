package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const cursorForegroundFixture = "suma-owned-cursor-foreground-fixture189"

// This is the Go test executable under an owned fixed package name. It is
// deliberately not Node, Cursor, a provider, or an authentication fixture.
func init() {
	if len(os.Args) < 3 || filepath.Base(os.Args[0]) != cursorNodeName() || !filepath.IsAbs(os.Args[1]) || filepath.Base(os.Args[1]) != "index.js" {
		return
	}
	entry, err := os.ReadFile(os.Args[1])
	if err != nil || string(entry) != cursorForegroundFixture {
		return
	}
	if len(os.Args) == 4 && os.Args[2] == "--disable-auto-update" && os.Args[3] == "--version" {
		fmt.Println("Cursor owned foreground fixture189")
		os.Exit(0)
	}
	if os.Getenv("CURSOR_API_KEY") != "owned-foreground-key189" || os.Args[len(os.Args)-1] != "acp" {
		os.Exit(89)
	}
	model := []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "fixed189", "options": []any{map[string]any{"value": "fixed189", "name": "Fixed"}}}}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var p struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &p) != nil {
			os.Exit(90)
		}
		var result any
		switch p.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}}
		case "session/new":
			result = map[string]any{"sessionId": "owned-foreground189", "configOptions": model}
		case "session/set_config_option":
			result = map[string]any{"configOptions": model}
		case "session/prompt":
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": "blocking189", "method": "cursor/task", "params": map[string]any{"sessionId": "owned-foreground189"}})
			continue
		default:
			os.Exit(91)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": p.ID, "result": result})
	}
	// Model an ACP foreground that retains background handles after stdin EOF.
	time.Sleep(time.Minute)
	os.Exit(92)
}

func TestCursorUnsupportedRequestClosesForegroundWithoutFakeSuccess(t *testing.T) {
	t.Setenv("LOCAL_CURSOR_FOREGROUND_KEY", "owned-foreground-key189")
	repo, base := gitFixture(t)
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, cursorNodeName()), bytes, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "index.js"), []byte(cursorForegroundFixture), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := BuildCursorACPCommand(root, "Cursor owned foreground fixture189", "fixed189", "LOCAL_CURSOR_FOREGROUND_KEY")
	if err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"cursor189": spec}}
	// No input RPC is sent for the still-unsupported task extension. The
	// original Worker normally supplies this scope; this direct fixture owns it.
	cfg.ToolInputClient = &Client{ProjectID: "owned-fixture190"}
	cfg.ToolInventory = describeForTest(t, cfg)
	if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
		t.Fatal("owned Cursor inventory failed", cfg.ToolInventory)
	}
	p := cfg.ToolInventory[0].Profile
	a := Assignment{RunID: "cursor-foreground189", NodeID: "develop", Attempt: 1, Fence: 1, DispatchID: "dispatch189", WorkerID: "worker189", Type: "agent", TimeoutSeconds: 30, Command: []string{"cursor189"}, Config: map[string]any{"goal": "Owned unsupported request fixture", "tool_profile_id": "profile189", "tool_profile_sha256": p.ProfileSHA256, "acp_execution": p.ACPExecution}}
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	cfg.Journal = j
	notice := false
	cfg.ToolProgress = func(e tc.Event) error {
		notice = notice || strings.Contains(e.Message, "Unsupported Cursor request: cursor/task")
		return nil
	}
	started := time.Now()
	result := Execute(context.Background(), a, cfg)
	if time.Since(started) > 10*time.Second || result.ExitCode == 0 || !result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ACPExecution == nil {
		t.Fatal("unsupported request lingered or became success", result.Error)
	}
	proof := result.ToolEvidence.ACPExecution
	if !notice || proof.PromptCompleted || !proof.HostProcessStopped || !proof.HostProcessTreeStopped || result.ToolEvidence.Terminal != "unknown" {
		t.Fatal("native request or host closure was fabricated")
	}
}
