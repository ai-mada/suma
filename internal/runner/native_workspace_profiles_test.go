package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This deterministic CLI tests host finalization, not a model or SDK. The
// actual bundled SDK/client/workspace journey has separate delivery evidence.
func nativeHostFixture(t *testing.T) (ExecConfig, string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node22 required for native host integration", err)
	}
	repo, base := gitFixture(t)
	folder := t.TempDir()
	entry := filepath.Join(folder, "agent.bundle.mjs")
	script := `import {createInterface} from 'node:readline';
if(process.argv.includes('--version')){console.log('suma-agent-runtime 0.1.0');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}
const input=createInterface({input:process.stdin});input.once('line',line=>{const r=JSON.parse(line);const emit=value=>console.log(JSON.stringify({...value,request_id:r.request_id}));emit({type:'started',model:'host-fixture',tools:[]});emit({type:'request_started',step:0,output_allowance:100});emit({type:'usage',step:0,input_tokens:3,output_tokens:2,usage_state:'complete',usage_reported:true});emit({type:'completed',output:'No source changes',usage_state:'complete',output_tokens:2});if(r.goal==='cancel')process.stderr.write('cancel-finalization\n');input.close()});`
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := nativeRuntimeManifest{SchemaVersion: 1, Entry: filepath.Base(entry), EntrySHA256: digest([]byte(script)), ExternalImports: []string{"node:readline"}, Dependencies: map[string]string{"@modelcontextprotocol/client": "2.4.0"}}
	raw, _ := json.Marshal(manifest)
	name := filepath.Join(folder, "agent.manifest.json")
	if err = os.WriteFile(name, raw, 0600); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}, RuntimeManifest: name}}
	return ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"native": spec}}, entry
}

func TestNativeWorkspaceProfilePinsBundleAndCheckClosure(t *testing.T) {
	cfg, entry := nativeHostFixture(t)
	exe, _ := os.Executable()
	parent := cfg.Commands["native"]
	parent.NativeWorkspace.Checks = []string{"checks"}
	cfg.Commands["native"] = parent
	cfg.Commands["checks"] = CommandSpec{Executable: exe, Args: []string{"original"}}
	_, sha, err := commandExecutable(parent)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := effectiveCommandProfileDigest(cfg, parent, sha)
	if err != nil {
		t.Fatal(err)
	}
	check := cfg.Commands["checks"]
	check.Args = []string{"changed"}
	cfg.Commands["checks"] = check
	next, err := effectiveCommandProfileDigest(cfg, parent, sha)
	if err != nil || next == pin {
		t.Fatal("check closure was not pinned", err)
	}
	if _, err = runPinnedNativeCheck(context.Background(), cfg, &attemptWorkspace{Source: cfg.Repository}, parent, pin, "checks"); err == nil {
		t.Fatal("stale check closure executed")
	}
	if err = os.WriteFile(entry, []byte("// changed module"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = effectiveCommandProfileDigest(cfg, parent, sha); err == nil {
		t.Fatal("changed bundle accepted")
	}
}

func TestNativeWorkspaceHostProbesMJSAndSealsNoChange(t *testing.T) {
	cfg, _ := nativeHostFixture(t)
	inventory, err := DescribeTools(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || !inventory[0].Available || inventory[0].Profile.WorkspaceToolsVersion != NativeWorkspaceVersion {
		t.Fatalf("MJS native inventory unavailable: %+v", inventory)
	}
	cfg.ToolInventory = inventory
	a := Assignment{RunID: "native-host-run", NodeID: "code", Attempt: 1, Fence: 1, WorkerID: "worker", Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "inspect", "tool_profile_sha256": inventory[0].Profile.ProfileSHA256}}
	result := Execute(context.Background(), a, cfg)
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown {
		t.Fatalf("host failed: %+v", result)
	}
	evidence, ok := result.WorkspaceEvidence["native_workspace"].(*NativeWorkspaceEvidence)
	if !ok || evidence.Seal == nil || evidence.Seal.State != "no_change" {
		t.Fatal("known native success lacks seal", evidence)
	}
	path := filepath.Join(cfg.Repository, ".cache", "suma-worktrees", attemptKey(a), "evidence.json")
	// Locate the authoritative captured receipt without assuming the configured
	// cache subdirectory's name is part of this wire contract.
	var captured []byte
	_ = filepath.WalkDir(filepath.Join(cfg.Repository, ".cache"), func(name string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && entry.Name() == "evidence.json" {
			captured, _ = os.ReadFile(name)
		}
		return nil
	})
	_ = path
	if len(captured) == 0 || digest(captured) != result.WorkspaceEvidence["evidence_sha256"] || !strings.Contains(string(captured), `"native_workspace"`) {
		t.Fatal("native evidence was not bound before capture hashing")
	}
	if got := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD")); got != cfg.BaseCommit {
		t.Fatal("host changed the administrator checkout")
	}
}

func TestNativeWorkspaceCancellationCannotReportUnsealedSuccess(t *testing.T) {
	cfg, _ := nativeHostFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg.Progress = func(message string) error {
		if strings.Contains(message, "cancel-finalization") {
			cancel()
		}
		return nil
	}
	a := Assignment{RunID: "native-cancel", NodeID: "code", Attempt: 1, Fence: 1, WorkerID: "worker", Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "cancel"}}
	result := Execute(ctx, a, cfg)
	if result.ExitCode == 0 && result.Error == "" {
		t.Fatal("cancelled native work reported success")
	}
}
