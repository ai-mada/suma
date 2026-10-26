package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The native executable exercises the real process/pipe/worktree boundary. It
// implements only protocol fixtures and never contacts a model or vendor CLI.
const teamCLIProcessFixture = `package main
import("encoding/json";"fmt";"io";"os";"os/exec";"path/filepath";"strings")
func fail(message string){fmt.Fprintln(os.Stderr,"transport fixture: "+message);os.Exit(31)}
func emit(frame any){if json.NewEncoder(os.Stdout).Encode(frame)!=nil{fail("stdout")}}
func record(event,id,cwd string){f,e:=os.OpenFile(os.Getenv("SUMA_TEAM_RECEIPT"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if e!=nil{fail("receipt")};e=json.NewEncoder(f).Encode(map[string]string{"event":event,"request_id":id,"cwd":cwd});if e==nil{e=f.Sync()};f.Close();if e!=nil{fail("receipt persistence")}}
func workspace(cwd string){actual,e:=os.Getwd();if e!=nil||!strings.EqualFold(filepath.Clean(actual),filepath.Clean(cwd))||!filepath.IsAbs(cwd)||strings.EqualFold(cwd,os.Getenv("SUMA_TEAM_REPOSITORY")){fail("host cwd")};s,e:=os.Lstat(filepath.Join(cwd,".git"));if e!=nil||!s.Mode().IsRegular(){fail("not an isolated worktree")}}
func commit(cwd string){if os.WriteFile(filepath.Join(cwd,"team-cli-output.txt"),[]byte("local protocol fixture output\n"),0600)!=nil{fail("write isolated output")};for _,a:=range [][]string{{"-c","core.hooksPath="+os.DevNull,"add","--","team-cli-output.txt"},{"-c","core.hooksPath="+os.DevNull,"-c","commit.gpgsign=false","-c","user.name=Transport Fixture","-c","user.email=fixture@example.invalid","commit","-m","fixture transport output"}}{c:=exec.Command("git",a...);c.Dir=cwd;if b,e:=c.CombinedOutput();e!=nil{fail("isolated commit: "+string(b))}}}
func main(){args:=os.Args[1:];if len(args)==1&&args[0]=="--version"{fmt.Println("local-team-cli protocol-fixture");return};if len(args)==1&&args[0]=="--help"{fmt.Println("-p --input-format --output-format --verbose");return};if strings.Join(args," ")=="--profile multica --probe"{fmt.Fprintln(os.Stderr,"local transport fixture profile diagnostic");emit(map[string]any{"v":1,"type":"probe","runtime":"dsh","plugin_version":"transport-fixture","protocol_version":1});return}
if strings.Join(args," ")=="--profile multica --stdio"{dsh();return}
if strings.Join(args," ")=="-p --input-format text --output-format stream-json --verbose"{buddy();return};fail("uncontrolled argv")}
func dsh(){emit(map[string]any{"v":1,"type":"ready","runtime":"dsh","plugin_version":"transport-fixture","capabilities":map[string]bool{"cancel":true}});dec:=json.NewDecoder(os.Stdin);var execute map[string]any;if dec.Decode(&execute)!=nil{fail("execute input")};id,_:=execute["request_id"].(string);cwd,_:=execute["cwd"].(string);servers,ok:=execute["mcp_servers"].([]any);if len(execute)!=6||execute["v"]!=float64(1)||execute["type"]!="execute"||id!=os.Getenv("SUMA_TEAM_REQUEST")||execute["prompt"]!="inspect actual transport fixture"||!ok||len(servers)!=0{fail("uncontrolled execute")};workspace(cwd);record("execute",id,cwd);emit(map[string]any{"v":1,"type":"session","request_id":id,"session_id":"dsh-fixture-session","resumed":false});mode:=os.Getenv("SUMA_TEAM_MODE");if mode=="no-result"{record("exit_without_result",id,cwd);return};status:="completed";if mode=="cancel"{var cancel map[string]any;if dec.Decode(&cancel)!=nil||len(cancel)!=3||cancel["v"]!=float64(1)||cancel["type"]!="cancel"||cancel["request_id"]!=id{fail("cancel identity")};record("cancel",id,cwd);status="cancelled"}else{commit(cwd)};emit(map[string]any{"v":1,"type":"result","request_id":id,"session_id":"dsh-fixture-session","status":status,"output":"local protocol fixture done","resume_rejected":false});var extra map[string]any;if dec.Decode(&extra)!=io.EOF{fail("stdin did not close after result or extra execute")};record("eof",id,cwd)}
func buddy(){goal,e:=io.ReadAll(os.Stdin);if e!=nil||strings.TrimSpace(string(goal))!="inspect actual transport fixture"||os.Getenv("CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS")!="1"{fail("CodeBuddy host input")};cwd,e:=os.Getwd();if e!=nil{fail("cwd")};workspace(cwd);record("execute","",cwd);commit(cwd);emit(map[string]any{"type":"system","subtype":"init","session_id":"cb-fixture-session"});emit(map[string]any{"type":"result","subtype":"success","session_id":"cb-fixture-session","is_error":false,"result":"local protocol fixture done","usage":map[string]int{"input_tokens":0,"output_tokens":0}});record("eof","",cwd)}
`

func TestTeamCLIActualProcessPipesAndBoundWorkspace(t *testing.T) {
	fixtureDir := t.TempDir()
	source := filepath.Join(fixtureDir, "main.go")
	if err := os.WriteFile(source, []byte(teamCLIProcessFixture), 0600); err != nil {
		t.Fatal(err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	executable := filepath.Join(fixtureDir, "team-cli-fixture"+suffix)
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, filepath.Join(runtime.GOROOT(), "bin", "go"+suffix), "build", "-o", executable, source)
	build.Dir = fixtureDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile local protocol fixture: %v %s", err, output)
	}
	for _, scenario := range []string{"dsh-valid", "dsh-no-result", "dsh-cancel", "codebuddy-valid"} {
		t.Run(scenario, func(t *testing.T) {
			repo, base := gitFixture(t)
			if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("user draft remains\n"), 0600); err != nil {
				t.Fatal(err)
			}
			a := Assignment{RunID: "team-transport-" + scenario, NodeID: "develop", WorkerID: "fixture-worker", Attempt: 1, Fence: 9, Type: "agent", TimeoutSeconds: 6, Command: []string{"team-cli"}, Config: map[string]any{"goal": "inspect actual transport fixture"}}
			mode := strings.TrimPrefix(scenario, "dsh-")
			receipt := filepath.Join(t.TempDir(), "pipe-receipt.jsonl")
			spec := CommandSpec{Executable: executable, Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Input: "dsh-jsonl", Args: []string{"--profile", "multica", "--stdio"}, Env: map[string]string{"SUMA_TEAM_MODE": mode, "SUMA_TEAM_REQUEST": digest([]byte(key(a))), "SUMA_TEAM_REPOSITORY": repo, "SUMA_TEAM_RECEIPT": receipt}}
			if scenario == "codebuddy-valid" {
				spec.Adapter, spec.Protocol, spec.Input = "codebuddy", "codebuddy-jsonl-v1", "goal"
				spec.Args = []string{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose"}
			}
			cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"team-cli": spec}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
				t.Fatalf("local fixture protocol unavailable: %+v", cfg.ToolInventory)
			}
			profile := cfg.ToolInventory[0].Profile
			a.Config["tool_profile_id"], a.Config["tool_profile_sha256"] = "transport-fixture", profile.ProfileSHA256
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			var events []tc.Event
			cfg.ToolProgress = func(event tc.Event) error {
				events = append(events, event)
				if scenario == "dsh-cancel" && event.Kind == "session_started" {
					cancel()
				}
				return nil
			}
			started := time.Now()
			result := Execute(ctx, a, cfg)
			if result.ToolEvidence == nil || !tc.EqualProfile(result.ToolEvidence.Profile, profile) || result.WorkspaceEvidence == nil || result.WorkspaceEvidence["base_commit"] != base {
				t.Fatalf("missing public profile/workspace binding: %+v", result)
			}
			if err := tc.ValidateEvidence(*result.ToolEvidence); err != nil {
				t.Fatalf("invalid public typed evidence: %v", err)
			}
			if scenario == "dsh-cancel" {
				cancelled := false
				for _, event := range events {
					cancelled = cancelled || event.Kind == "turn_failed" && event.Status == "cancelled"
				}
				if !result.Stopped || result.OutcomeUnknown || result.ExitCode == 0 || !cancelled {
					t.Fatalf("cooperative cancellation did not settle with a known stop: %+v", result)
				}
			} else if scenario == "dsh-no-result" {
				if result.ExitCode == 0 || !result.OutcomeUnknown || result.Stopped || ctx.Err() != nil || result.ToolEvidence.Terminal != "unknown" {
					t.Fatalf("exit without result hung or became success: %+v duration=%v", result, time.Since(started))
				}
			} else if result.ExitCode != 0 || result.Error != "" || result.Stopped || result.OutcomeUnknown || result.ToolEvidence.Terminal != "completed" || result.WorkspaceEvidence["clean"] != true || result.WorkspaceEvidence["head_commit"] == base {
				t.Fatalf("actual fixture execution failed: %+v", result)
			}
			data, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				var fact map[string]string
				if json.Unmarshal([]byte(line), &fact) != nil || fact["cwd"] == repo || fact["cwd"] == "" || scenario != "codebuddy-valid" && fact["request_id"] != digest([]byte(key(a))) {
					t.Fatalf("host input identity/cwd mismatch: %s", line)
				}
				counts[fact["event"]]++
			}
			if counts["execute"] != 1 || scenario == "dsh-cancel" && counts["cancel"] != 1 || scenario != "dsh-no-result" && counts["eof"] != 1 || scenario == "dsh-no-result" && counts["exit_without_result"] != 1 {
				t.Fatalf("process executed twice or stdin did not settle: %+v", counts)
			}
			unchanged, _ := os.ReadFile(filepath.Join(repo, "file.txt"))
			if string(unchanged) != "user draft remains\n" || strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")) != base {
				t.Fatal("team process changed the user's checkout")
			}
		})
	}
}
