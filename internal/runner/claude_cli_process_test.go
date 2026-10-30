package runner

import (
	"context"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This native process fixture speaks the reviewed public CLI frames. It never
// invokes Claude, logs in, installs dependencies or makes a model request.
const claude158NativeFixture = `package main
import("encoding/json";"fmt";"io";"os";"os/exec";"strings";"time")
func frame(v any){if json.NewEncoder(os.Stdout).Encode(v)!=nil{os.Exit(30)}}
func git(args ...string){c:=exec.Command("git",args...);if c.Run()!=nil{os.Exit(31)}}
func main(){a:=os.Args[1:];if len(a)==1&&a[0]=="--version"{fmt.Println("2.1.314 (Claude Code)");return};if len(a)==1&&a[0]=="--help"{fmt.Println("--input-format --output-format --verbose --no-session-persistence --safe-mode --restricted --settings --strict-mcp-config --mcp-config --permission-mode --permission-prompts --tools --allowedTools --model");return};if len(a)<2||a[0]!="-p"||os.Getenv("SUMA_RUNNER_TOKEN")!=""||os.Getenv("CLAUDE_CODE_DISABLE_BACKGROUND_TASKS")!="1"||os.Getenv("CLAUDE_CODE_DISABLE_AGENT_VIEW")!="1"{os.Exit(32)};b,_:=io.ReadAll(os.Stdin);goal:=strings.TrimSpace(string(b));frame(map[string]any{"type":"system","subtype":"init","session_id":"fixture-claude-session","model":"claude-sonnet-5","mcp_servers":[]any{}});if goal=="cancel"{time.Sleep(time.Minute);return};if goal=="unknown"{return};if goal=="develop"{if os.WriteFile("result.txt",[]byte("approved candidate\n"),0600)!=nil{os.Exit(33)};git("add","result.txt");git("-c","user.name=ClaudeFixture","-c","user.email=claude@example.test","commit","-m","fixture candidate")};frame(map[string]any{"type":"result","subtype":"success","is_error":false,"session_id":"fixture-claude-session","permission_denials":[]any{},"result":"fixture finished","usage":map[string]any{"input_tokens":4,"output_tokens":2}})}
`

func TestClaudeCLI158NativeWorktreeAndIndependentCandidateTest(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "owned-fixture-runner-credential-must-not-reach-cli")
	if runtime.GOOS != "windows" {
		t.Skip("This actual process-containment check is scoped to Windows Job Objects")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is needed for the independent local test")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "fixture.go")
	if err = os.WriteFile(source, []byte(claude158NativeFixture), 0600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "claude.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin/go.exe"), "build", "-o", exe, source)
	build.Dir = dir
	build.WaitDelay = time.Second
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build local native protocol fixture: %v %s", err, out)
	}
	discovered, err := discoverInstalledTools(ctx, "claude-code-cli", []string{dir}, localToolDiscoveryVersion)
	if err != nil || len(discovered.Items) != 1 || !discovered.Items[0].LaunchSupported || discovered.Items[0].Command == nil {
		t.Fatalf("native version/help discovery unavailable: %+v %v", discovered, err)
	}
	repo, _ := gitFixture(t)
	testSource := `const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');test('candidate file is from the developer commit',()=>assert.equal(fs.readFileSync('result.txt','utf8'),'approved candidate\n'));`
	if err = os.WriteFile(filepath.Join(repo, "candidate.test.cjs"), []byte(testSource), 0600); err != nil {
		t.Fatal(err)
	}
	base := gitTest(t, repo, "add", "candidate.test.cjs")
	_ = base
	gitTest(t, repo, "commit", "-m", "independent candidate test")
	base = strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	// A user draft in the bound repository never becomes the task worktree.
	if err = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("preserve user draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	spec := *discovered.Items[0].Command
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"claude-develop": spec, "independent-test": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"--test", "candidate.test.cjs"}, TestEvidence: true}}}
	cfg.ToolInventory = describeForTest(t, cfg)
	t.Log("owned native probe complete")
	var profile, testProfile tc.Profile
	for _, o := range cfg.ToolInventory {
		if !o.Available {
			t.Fatalf("fixture unavailable: %s", o.Reason)
		}
		if o.Profile.Adapter == "claude" {
			profile = o.Profile
		} else {
			testProfile = o.Profile
		}
	}
	assignment := func(id, goal string) Assignment {
		return Assignment{RunID: id, NodeID: "develop", WorkerID: "claude-fixture-worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"claude-develop"}, Config: map[string]any{"goal": goal, "tool_profile_id": "fixture-tool", "tool_profile_sha256": profile.ProfileSHA256, "claude_execution": *profile.ClaudeExecution}}
	}
	t.Run("changed_fixed_model_zero_execution", func(t *testing.T) {
		a := assignment("claude-forged", "develop")
		fixed := *profile.ClaudeExecution
		fixed.Model = "claude-other-5"
		a.Config["claude_execution"] = fixed
		r := Execute(context.Background(), a, cfg)
		if r.ExitCode == 0 || r.WorkspaceEvidence != nil {
			t.Fatalf("changed model entered a writable attempt: %+v", r)
		}
	})
	t.Log("starting exact native development")
	a := assignment("claude-native-development", "develop")
	executeCtx, executeStop := context.WithTimeout(context.Background(), 20*time.Second)
	defer executeStop()
	r := Execute(executeCtx, a, cfg)
	if r.ExitCode != 0 || r.Stopped || r.OutcomeUnknown || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" || r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence["head_commit"] == base {
		t.Fatalf("real native process did not yield an isolated committed candidate: %+v", r)
	}
	if err = tc.ValidateEvidence(*r.ToolEvidence); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(repo, "file.txt"))
	if err != nil || string(original) != "preserve user draft\n" {
		t.Fatal("bound repository draft was changed")
	}
	head, _ := r.WorkspaceEvidence["head_commit"].(string)
	evidence, _ := r.WorkspaceEvidence["evidence_sha256"].(string)
	testAssignment := Assignment{RunID: a.RunID, NodeID: "independent-test", WorkerID: a.WorkerID, Attempt: 1, Fence: 2, Type: "test", Command: []string{"independent-test"}, CandidateFrom: a.NodeID, CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateCommit: head, CandidateEvidenceSHA256: evidence, Config: map[string]any{"candidate_from": a.NodeID, "tool_profile_id": "test-tool", "tool_profile_sha256": testProfile.ProfileSHA256}}
	t.Log("starting independent candidate test")
	testCtx, testStop := context.WithTimeout(context.Background(), 20*time.Second)
	defer testStop()
	tested := Execute(testCtx, testAssignment, cfg)
	if tested.ExitCode != 0 || tested.WorkspaceEvidence["base_commit"] != head || tested.WorkspaceEvidence["test_command"] != true {
		t.Fatalf("independent process did not test the exact candidate: %+v", tested)
	}
	for _, scenario := range []string{"unknown", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			value := assignment("claude-"+scenario, scenario)
			local := cfg
			runCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if scenario == "cancel" {
				local.ToolProgress = func(e tc.Event) error {
					if e.Kind == "session_started" {
						stop()
					}
					return nil
				}
			}
			result := Execute(runCtx, value, local)
			if result.ExitCode == 0 || result.ToolEvidence == nil || result.ToolEvidence.Terminal != "unknown" {
				t.Fatalf("unconfirmed CLI execution became successful: %+v", result)
			}
			if scenario == "cancel" && !result.Stopped {
				t.Fatalf("owned Windows process closure unconfirmed: %+v", result)
			}
			if scenario == "unknown" && !result.OutcomeUnknown {
				t.Fatal("missing result was not kept unknown")
			}
		})
	}
	if strings.TrimSpace(profile.ToolVersion) != "2.1.314 (Claude Code)" {
		t.Fatal("native version probe was not used")
	}
}
