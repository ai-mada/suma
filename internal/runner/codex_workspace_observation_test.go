package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func codexObservationLease(scope woc.Scope, policy string) woc.Lease {
	now := time.Now().UTC()
	return woc.Lease{Observation: woc.Observation{ID: "codex-observation", ProjectID: "project", Version: 2, State: "leased", Scope: scope, Operation: "read", Arguments: woc.Arguments{Path: "file.txt"}, WorkspacePolicySHA256: policy, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}, LeaseFence: 1, LeaseUntil: now.Add(10 * time.Second).Format(time.RFC3339Nano)}
}

func TestCodexWorkspaceInventoryDeclaresOnlyProbedObservation(t *testing.T) {
	cfg, _ := codexRPCFixture(t, "completed")
	item := cfg.ToolInventory[0]
	if item.WorkspaceObservationProtocol != woc.Protocol || item.WorkspaceWriteProtocol != "" || item.Profile.WorkspaceToolsVersion != "" || tc.ValidateObservation(item) != nil {
		t.Fatalf("Codex must declare only the compiled read broker after its successful probe: %+v", item)
	}
	cfg.AllowExec = false
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || items[0].Available || items[0].WorkspaceObservationProtocol != "" || items[0].WorkspaceWriteProtocol != "" {
		t.Fatalf("disabled Codex declared a workspace transport: %+v %v", items, err)
	}
	cfg.AllowExec = true
	spec := cfg.Commands["codex041"]
	spec.CodexAppServer = nil
	cfg.Commands["codex041"] = spec
	if _, err = DescribeTools(context.Background(), cfg); err == nil {
		t.Fatal("invalid Codex configuration reached a successful probe")
	}
}

func TestCodexWorkspaceLiveAttemptReadAndTerminalRevocation(t *testing.T) {
	cfg, assignment := codexRPCFixture(t, "completed")
	registry := NewWorkspaceObservationRegistry("project")
	cfg.WorkspaceObservations = registry
	if err := os.WriteFile(filepath.Join(cfg.Repository, "file.txt"), []byte("administrator draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(cfg.Repository, ".cache", "runner", "worktrees", attemptKey(assignment))
	source := filepath.Join(directory, "src")
	var lease woc.Lease
	readDuringTurn, terminalRevoked := false, false
	cfg.ToolProgress = func(event tc.Event) error {
		if event.Kind == "turn_completed" {
			_, _, active := registry.active()
			terminalRevoked = !active && registry.Observe(context.Background(), lease).ErrorCode == "workspace_closed"
		}
		if event.Kind != "turn_started" {
			return nil
		}
		scope, policy, active := registry.active()
		if !active || scope != nativeObservationScope(assignment, cfg.ToolInventory[0].Profile.ProfileSHA256) {
			return fmt.Errorf("current Codex attempt was not registered for observation")
		}
		lease = codexObservationLease(scope, policy)
		result := registry.Observe(context.Background(), lease)
		if result.ErrorCode != "" || result.ObservationResult == nil || result.ObservationResult.Read == nil || result.ObservationResult.Read.Content != "base\n" || result.ObservationResult.Read.SHA256 != digest([]byte("base\n")) {
			return fmt.Errorf("exact Codex attempt read failed: %s", result.ErrorCode)
		}
		listed := lease
		listed.Observation.Operation, listed.Observation.Arguments = "list", woc.Arguments{}
		result = registry.Observe(context.Background(), listed)
		if result.ErrorCode != "" || result.ObservationResult == nil || result.ObservationResult.List == nil || len(result.ObservationResult.List.Entries) != 2 {
			return fmt.Errorf("bounded Codex attempt list failed: %s", result.ErrorCode)
		}
		for _, entry := range result.ObservationResult.List.Entries {
			if entry.Path != "file.txt" && entry.Path != ".gitignore" {
				return fmt.Errorf("Codex list disclosed an unintended root")
			}
		}
		registry.mu.Lock()
		writable := registry.native != nil
		registry.mu.Unlock()
		content := "forbidden remote write\n"
		write := wwc.Lease{Command: wwc.Command{ID: "codex-write", ProjectID: "project", Scope: scope, WorkspacePolicySHA256: policy, Path: "file.txt", ExpectedSHA256: digest([]byte("base\n")), ContentSHA256: wwc.ContentDigest(content)}, Mode: "execute", Content: &content}
		if writable || ExecuteWorkspaceWrite(context.Background(), registry, write).Outcome != "unknown" {
			return fmt.Errorf("Codex received a native writable handle")
		}
		for _, change := range []string{"old-attempt", "old-fence", "other-project", "stale-digest", "sensitive-path"} {
			wrong := lease
			switch change {
			case "old-attempt":
				wrong.Observation.Scope.Attempt++
			case "old-fence":
				wrong.Observation.Scope.Fence++
			case "other-project":
				wrong.Observation.ProjectID = "other"
			case "stale-digest":
				wrong.Observation.ExpectedSHA256 = strings.Repeat("f", 64)
			case "sensitive-path":
				wrong.Observation.Arguments.Path = "../codex-home/config.toml"
			}
			if rejected := registry.Observe(context.Background(), wrong); rejected.ErrorCode == "" || rejected.ObservationResult != nil {
				return fmt.Errorf("%s observation disclosed source", change)
			}
		}
		for _, secret := range []string{filepath.Join(directory, "codex-home"), source, cfg.Repository, "api_key=fixture-private-value"} {
			if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte(secret), 0600); err != nil {
				return err
			}
			result = registry.Observe(context.Background(), lease)
			raw, _ := json.Marshal(result)
			if result.ErrorCode != "sensitive_content" || result.ObservationResult != nil || strings.Contains(string(raw), secret) {
				return fmt.Errorf("Codex observation disclosed sensitive content")
			}
		}
		if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("base\n"), 0600); err != nil {
			return err
		}
		readDuringTurn = true
		return nil
	}
	result := Execute(context.Background(), assignment, cfg)
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || !readDuringTurn || !terminalRevoked {
		t.Fatalf("live Codex read/terminal revocation failed (read=%t terminal=%t): %+v", readDuringTurn, terminalRevoked, result)
	}
	if _, _, active := registry.active(); active || registry.Observe(context.Background(), lease).ObservationResult != nil {
		t.Fatal("terminal Codex attempt reopened source")
	}
	if _, err := os.Stat(filepath.Join(directory, "native-workspace.json")); !os.IsNotExist(err) {
		t.Fatal("Codex observation created a native operation ledger")
	}
	if result.WorkspaceEvidence["native_workspace"] != nil || result.WorkspaceEvidence["head_commit"] != cfg.BaseCommit {
		t.Fatalf("observation invented native effects or a candidate commit: %+v", result.WorkspaceEvidence)
	}
	data, _ := os.ReadFile(filepath.Join(cfg.Repository, "file.txt"))
	if string(data) != "administrator draft\n" {
		t.Fatal("Codex observation changed the administrator checkout")
	}
}
