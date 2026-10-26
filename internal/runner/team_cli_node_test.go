package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// One stdlib-only fixture runs unchanged as .js, .mjs and .cjs. It exercises
// real Node stdio, Git and Runner process ownership; it is not a vendor CLI.
const teamCLINodeFixture = `(async()=>{
const fs=await import('node:fs'),path=await import('node:path'),cp=await import('node:child_process'),rl=await import('node:readline');
const args=process.argv.slice(2),emit=value=>process.stdout.write(JSON.stringify(value)+'\n');
if(args.length===1&&args[0]==='--version'){console.log('node-team-cli transport-fixture');return}
if(args.length===1&&args[0]==='--help'){console.log('-p --input-format --output-format --verbose');return}
if(args.join(' ')==='--profile multica --probe'){console.error('node fixture profile diagnostic');emit({v:1,type:'probe',runtime:'dsh',plugin_version:'node-fixture-v1',protocol_version:1});return}
const record=(event,id,cwd)=>{const fd=fs.openSync(process.env.SUMA_TEAM_RECEIPT,'a',0o600);try{fs.writeSync(fd,JSON.stringify({event,request_id:id,cwd})+'\n');fs.fsyncSync(fd)}finally{fs.closeSync(fd)}};
const workspace=cwd=>{if(!path.isAbsolute(cwd)||path.resolve(cwd).toLowerCase()!==path.resolve(process.cwd()).toLowerCase()||path.resolve(cwd).toLowerCase()===path.resolve(process.env.SUMA_TEAM_REPOSITORY).toLowerCase()||!fs.lstatSync(path.join(cwd,'.git')).isFile())throw Error('not an attempt-local worktree')};
const commit=cwd=>{fs.writeFileSync(path.join(cwd,'team-node-output.txt'),'stdlib Node fixture candidate\n');for(const argv of [['-c','core.hooksPath='+process.env.SUMA_TEAM_DEVNULL,'add','--','team-node-output.txt'],['-c','core.hooksPath='+process.env.SUMA_TEAM_DEVNULL,'-c','commit.gpgsign=false','-c','user.name=Node Transport Fixture','-c','user.email=fixture@example.invalid','commit','-m','node transport fixture']]){const p=cp.spawnSync('git',argv,{cwd,encoding:'utf8',windowsHide:true});if(p.error||p.status!==0)throw Error('isolated fixture commit failed')}};
if(args.join(' ')==='--profile multica --stdio'){
 const input=rl.createInterface({input:process.stdin,crlfDelay:Infinity}),lines=input[Symbol.asyncIterator]();
 emit({v:1,type:'ready',runtime:'dsh',plugin_version:'node-fixture-v1',capabilities:{cancel:true}});
 const first=await lines.next();if(first.done)throw Error('missing execute');const frame=JSON.parse(first.value),id=process.env.SUMA_TEAM_REQUEST;
 if(Object.keys(frame).length!==6||frame.v!==1||frame.type!=='execute'||frame.request_id!==id||frame.prompt!=='inspect actual Node transport fixture'||!Array.isArray(frame.mcp_servers)||frame.mcp_servers.length!==0)throw Error('uncontrolled execute input');
 workspace(frame.cwd);record('execute',id,frame.cwd);emit({v:1,type:'session',request_id:id,session_id:'node-dsh-session',resumed:false});commit(frame.cwd);
 emit({v:1,type:'result',request_id:id,session_id:'node-dsh-session',status:'completed',output:'local Node fixture completed',resume_rejected:false});
 if(!(await lines.next()).done)throw Error('extra execute or stdin not settled');record('eof',id,frame.cwd);input.close();return
}
if(args.join(' ')==='-p --input-format text --output-format stream-json --verbose'){
 let goal='';for await(const chunk of process.stdin)goal+=chunk;
 if(goal.trim()!=='inspect actual Node transport fixture'||process.env.CODEBUDDY_CODE_DISABLE_BACKGROUND_TASKS!=='1')throw Error('uncontrolled CodeBuddy input');
 const cwd=process.cwd();workspace(cwd);record('execute','',cwd);commit(cwd);
 emit({type:'system',subtype:'init',session_id:'node-codebuddy-session'});
 emit({type:'result',subtype:'success',is_error:false,session_id:'node-codebuddy-session',result:'local Node fixture completed',usage:{input_tokens:0,output_tokens:0}});record('eof','',cwd);return
}
throw Error('uncontrolled Node CLI argv');
})().catch(error=>{console.error('Node protocol fixture: '+error.message);process.exitCode=31});
`

func TestTeamCLINodeEntryWorktreeAndDigestPin(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("real Node executable is unavailable; Node-entry execution is not verified")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		adapter, extension string
		drift              bool
	}{
		{"harness", ".js", false},
		{"harness", ".mjs", false},
		{"codebuddy", ".cjs", false},
		{"harness", ".mjs", true},
		{"codebuddy", ".cjs", true},
	} {
		name := scenario.adapter + scenario.extension
		if scenario.drift {
			name += "-entry-drift"
		}
		t.Run(name, func(t *testing.T) {
			fixtureDir := t.TempDir()
			entry := filepath.Join(fixtureDir, "local-cli"+scenario.extension)
			if err := os.WriteFile(entry, []byte(teamCLINodeFixture), 0600); err != nil {
				t.Fatal(err)
			}
			repository, base := gitFixture(t)
			if err := os.WriteFile(filepath.Join(repository, "file.txt"), []byte("user draft remains\n"), 0600); err != nil {
				t.Fatal(err)
			}
			a := Assignment{RunID: "node-" + strings.ReplaceAll(name, ".", "-"), NodeID: "develop", WorkerID: "node-fixture-worker", Attempt: 1, Fence: 19, Type: "agent", TimeoutSeconds: 10, Command: []string{"team-node"}, Config: map[string]any{"goal": "inspect actual Node transport fixture"}}
			receipt, marker := filepath.Join(fixtureDir, "execution.jsonl"), filepath.Join(fixtureDir, "tampered-entry-invoked.txt")
			spec := CommandSpec{Executable: node, Adapter: scenario.adapter, Protocol: "dsh-multica-jsonl-v1", Input: "dsh-jsonl", Args: []string{entry, "--profile", "multica", "--stdio"}, Env: map[string]string{
				"SUMA_TEAM_RECEIPT": receipt, "SUMA_TEAM_REQUEST": digest([]byte(key(a))), "SUMA_TEAM_REPOSITORY": repository, "SUMA_TEAM_TAMPER_MARKER": marker, "SUMA_TEAM_DEVNULL": os.DevNull,
			}}
			if scenario.adapter == "codebuddy" {
				spec.Protocol, spec.Input = "codebuddy-jsonl-v1", "goal"
				spec.Args = []string{entry, "-p", "--input-format", "text", "--output-format", "stream-json", "--verbose"}
			}
			cfg := ExecConfig{AllowExec: true, Workspace: repository, Repository: repository, BaseCommit: base, Commands: map[string]CommandSpec{"team-node": spec}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
				t.Fatalf("actual Node protocol probes failed: %+v", cfg.ToolInventory)
			}
			profile := cfg.ToolInventory[0].Profile
			a.Config["tool_profile_id"], a.Config["tool_profile_sha256"] = "node-fixture-profile", profile.ProfileSHA256
			if scenario.drift {
				changed := teamCLINodeFixture + "\nimport('node:fs').then(fs=>fs.writeFileSync(process.env.SUMA_TEAM_TAMPER_MARKER,'unexpected entry execution'));\n"
				if err := os.WriteFile(entry, []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result := Execute(ctx, a, cfg)
			if scenario.drift {
				if result.ExitCode == 0 || !strings.Contains(result.Error, "profile") || result.ToolEvidence != nil || result.WorkspaceEvidence != nil || result.OutcomeUnknown {
					t.Fatalf("changed entry was not refused before execution: %+v", result)
				}
				for _, path := range []string{receipt, marker} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("changed entry was invoked: %s (%v)", filepath.Base(path), err)
					}
				}
			} else {
				if result.ExitCode != 0 || result.Error != "" || result.Stopped || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.Profile != profile || result.ToolEvidence.Terminal != "completed" {
					t.Fatalf("actual Node fixture did not settle successfully: %+v", result)
				}
				if err := tc.ValidateEvidence(*result.ToolEvidence); err != nil {
					t.Fatal(err)
				}
				evidence := result.WorkspaceEvidence
				head, _ := evidence["head_commit"].(string)
				if evidence["base_commit"] != base || evidence["clean"] != true || evidence["state"] != "completed" || head == "" || head == base || !tc.ValidDigest(evidence["diff_sha256"].(string)) {
					t.Fatalf("missing actual candidate/workspace facts: %+v", evidence)
				}
				if strings.TrimSpace(gitTest(t, repository, "rev-parse", head+"^")) != base || gitTest(t, repository, "show", head+":team-node-output.txt") != "stdlib Node fixture candidate\n" {
					t.Fatal("candidate is not the fixture's actual committed output on the administrator base")
				}
				data, err := os.ReadFile(receipt)
				if err != nil {
					t.Fatal(err)
				}
				counts := map[string]int{}
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					var fact map[string]string
					if json.Unmarshal([]byte(line), &fact) != nil || fact["cwd"] == "" || strings.EqualFold(filepath.Clean(fact["cwd"]), filepath.Clean(repository)) || scenario.adapter == "harness" && fact["request_id"] != digest([]byte(key(a))) {
						t.Fatalf("Node input or worktree identity mismatch: %s", line)
					}
					counts[fact["event"]]++
				}
				if counts["execute"] != 1 || counts["eof"] != 1 || len(counts) != 2 {
					t.Fatalf("Node execution was duplicated or stdio did not settle: %+v", counts)
				}
			}
			unchanged, err := os.ReadFile(filepath.Join(repository, "file.txt"))
			if err != nil || string(unchanged) != "user draft remains\n" || strings.TrimSpace(gitTest(t, repository, "rev-parse", "HEAD")) != base {
				t.Fatal("Node entry changed the user's checkout")
			}
			if _, err := os.Stat(filepath.Join(repository, "team-node-output.txt")); !os.IsNotExist(err) {
				t.Fatal("candidate output escaped into the user's checkout")
			}
		})
	}
}
