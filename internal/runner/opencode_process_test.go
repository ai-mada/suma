package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This child is a labelled transport fixture. It exercises actual Go process,
// duplex, private workspace MCP, effect ledger and host candidate finalization;
// it is not an OpenCode SDK/server/model acceptance result.
const openCodeHostProcessFixture = `import {createInterface} from 'node:readline';
const c=JSON.parse(process.env.SUMA_OPENCODE_CONFIG||'null');
if(!c||process.env.OPENCODE_CONFIG||process.env.NODE_OPTIONS||process.env.SUMA_RUNNER_TOKEN)throw Error('environment');
if(process.argv.includes('--probe')){console.log(JSON.stringify({v:1,type:'probe',protocol:'opencode-sdk-v1',runtime_version:'1.0.0',sdk_version:'1.19.4',server_version:'1.19.4',configuration_sha256:c.configuration_sha256,provider_id:c.provider_id,model:c.model,workspace_tools_version:'attempt-workspace-v1'}));process.exit(0)}
const input=createInterface({input:process.stdin});let request;
const emit=frame=>console.log(JSON.stringify({v:1,request_id:request.request_id,...frame}));
input.on('line',line=>{void(async()=>{const r=JSON.parse(line);if(r.type==='cancel'){if(!request||r.request_id!==request.request_id)throw Error('cancel identity');if(request.goal==='delayed-cancel')await new Promise(resolve=>setTimeout(resolve,2000));emit(request.goal==='no-ack'?{type:'failed',code:'cancel_unconfirmed',server_parent_stopped:true}:{type:'cancelled',session_id:'ses_fixture104',abort_acknowledged:true,server_parent_stopped:true});input.close();return}
 if(request||Object.keys(r).sort().join(',')!=='goal,request_id,type,v,workspace_bridge'||r.v!==1||r.type!=='execute'||r.workspace_bridge.request_id!==r.request_id)throw Error('input');request=r;
 emit({type:'ready',sdk_version:'1.19.4',server_version:'1.19.4',configuration_sha256:c.configuration_sha256,provider_id:c.provider_id,model:c.model});emit({type:'started',session_id:'ses_fixture104',user_message_id:'msg_fixture_user104'});
 if(['cancel','no-ack','delayed-cancel'].includes(r.goal))return;if(r.goal==='no-terminal'){input.close();return}
 if(r.goal==='write'){
  const bridge=r.workspace_bridge;let id=0,protocol='2025-06-18';const rpc=async(method,params)=>{const response=await fetch(bridge.endpoint,{method:'POST',headers:{Authorization:'Bearer '+bridge.token,'Content-Type':'application/json',Accept:'application/json, text/event-stream','MCP-Protocol-Version':protocol},body:JSON.stringify({jsonrpc:'2.0',id:++id,method,params}),redirect:'error'});if(!response.ok)throw Error('MCP status');const message=await response.json();if(message.error||message.result?.isError)throw Error('MCP result');return message.result};
  const initialized=await rpc('initialize',{protocolVersion:protocol,capabilities:{},clientInfo:{name:'suma-opencode-transport-fixture',version:'1'}});protocol=initialized.protocolVersion;
  const catalog=await rpc('tools/list',{});if(catalog.tools.length!==6||catalog.tools.some(tool=>!/^suma_workspace_(list|read|write|edit|remove|check)$/.test(tool.name)))throw Error('catalog');
  await rpc('tools/call',{name:'suma_workspace_read',arguments:{path:'file.txt'}});
  const write={name:'suma_workspace_write',arguments:{path:'output104.txt',content:'host MCP fixture output\n',expected_absent:true},_meta:{'suma/operation_id':'transport-write104'}};
  await rpc('tools/call',write);await rpc('tools/call',write);
 }
 emit({type:'completed',session_id:'ses_fixture104',user_message_id:'msg_fixture_user104',assistant_message_id:'msg_fixture_assistant104',provider_id:c.provider_id,model:c.model,finish:'stop',completed_at:'2026-10-28T00:00:00.000Z',server_parent_stopped:true,usage:{input_tokens:0,output_tokens:2},usage_state:'partial'});input.close();
 })().catch(()=>{if(request)emit({type:'failed',code:'fixture_failed',server_parent_stopped:true});process.exitCode=1;input.close()})});`

func openCodeHostFixture(t *testing.T) (ExecConfig, tc.Profile) {
	t.Helper()
	spec, manifest := openCodeConfigFixture(t)
	if err := os.WriteFile(spec.Args[0], []byte(openCodeHostProcessFixture), 0600); err != nil {
		t.Fatal(err)
	}
	manifest["entry_sha256"] = digest([]byte(openCodeHostProcessFixture))
	for _, asset := range manifest["assets"].([]map[string]any) {
		if asset["path"] == "suma-opencode.bundle.mjs" {
			asset["sha256"] = manifest["entry_sha256"]
			asset["size_bytes"] = len(openCodeHostProcessFixture)
		}
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(spec.NativeWorkspace.RuntimeManifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	repo, base := gitFixture(t)
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"opencode104": spec}}
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || !items[0].Available || !tc.SupportsOpenCodeExecutionProfile(items[0].Profile) || items[0].WorkspaceWriteProtocol != "workspace-write-v1" {
		t.Fatalf("transport fixture probe failed: %+v %v", items, err)
	}
	cfg.ToolInventory = items
	return cfg, items[0].Profile
}

func TestOpenCodeProcessHostMCPSealsCandidateAndIndependentTest(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "must-not-reach-opencode")
	t.Setenv("OPENCODE_CONFIG", "untrusted-global.json")
	cfg, profile := openCodeHostFixture(t)
	a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "opencode-host104", NodeID: "develop", WorkerID: "worker104", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"opencode104"}, Config: map[string]any{"goal": "write", "tool_profile_sha256": profile.ProfileSHA256, "opencode_execution": profile.OpenCodeExecution}}
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.Error != "" || r.Stopped || r.OutcomeUnknown || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" || r.ToolEvidence.OpenCodeExecution == nil || r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence["head_commit"] == cfg.BaseCommit {
		t.Fatalf("host MCP candidate failed: %+v", r)
	}
	evidence, ok := r.WorkspaceEvidence["native_workspace"].(*NativeWorkspaceEvidence)
	if !ok || evidence.Generation != 1 || len(evidence.Files) != 1 || evidence.Seal == nil || evidence.Seal.State != "sealed" {
		t.Fatalf("lost ACK replay bypassed ledger/seal: %+v", evidence)
	}
	if err := tc.ValidateEvidence(*r.ToolEvidence); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Repository, "output104.txt")); !os.IsNotExist(err) {
		t.Fatal("modified administrator checkout")
	}
	if got := strings.TrimSpace(gitTest(t, cfg.Repository, "rev-parse", "HEAD")); got != cfg.BaseCommit {
		t.Fatal("administrator HEAD moved")
	}
	node := cfg.Commands["opencode104"].Executable
	cfg.Commands = map[string]CommandSpec{"verify": {Executable: node, Args: []string{"-e", `const fs=require('node:fs');if(fs.readFileSync('output104.txt','utf8')!=='host MCP fixture output\n')process.exit(2);console.log('independent candidate check passed')`}, TestEvidence: true}}
	cfg.ToolInventory = nil
	test := Assignment{TenantID: "tenant", ProjectID: "project", RunID: a.RunID, NodeID: "test", WorkerID: "independent104", Attempt: 1, Fence: 1, Type: "test", Command: []string{"verify"}, CandidateFrom: a.NodeID, CandidateCommit: r.WorkspaceEvidence["head_commit"].(string), CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateEvidenceSHA256: r.WorkspaceEvidence["evidence_sha256"].(string), Config: map[string]any{"candidate_from": a.NodeID}}
	checked := Execute(context.Background(), test, cfg)
	if checked.ExitCode != 0 || checked.Error != "" || checked.WorkspaceEvidence["head_commit"] != test.CandidateCommit || checked.WorkspaceEvidence["test_command"] != true {
		t.Fatalf("independent exact candidate check failed: %+v", checked)
	}
}

func TestOpenCodeProcessCancellationNeedsOriginalAbortAndServerStop(t *testing.T) {
	for _, mode := range []string{"cancel", "delayed-cancel", "no-ack", "no-terminal"} {
		t.Run(mode, func(t *testing.T) {
			cfg, profile := openCodeHostFixture(t)
			a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "opencode-" + mode, NodeID: "develop", WorkerID: "worker104", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"opencode104"}, Config: map[string]any{"goal": mode, "tool_profile_sha256": profile.ProfileSHA256, "opencode_execution": profile.OpenCodeExecution}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg.ToolProgress = func(event tc.Event) error {
				if mode != "no-terminal" && event.Kind == "session_started" {
					cancel()
				}
				return nil
			}
			r := Execute(ctx, a, cfg)
			if r.ExitCode == 0 || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "unknown" {
				t.Fatalf("cancellation/EOF became success: %+v", r)
			}
			if mode == "cancel" || mode == "delayed-cancel" {
				if !r.Stopped || r.OutcomeUnknown || !r.ToolEvidence.OpenCodeExecution.AbortAcknowledged {
					t.Fatalf("actual protocol stop was not retained: %+v", r)
				}
			} else if !r.OutcomeUnknown || r.Stopped {
				t.Fatalf("unconfirmed stop/EOF became confirmed: %+v", r)
			}
		})
	}
}

func TestOpenCodeFrozenConfigurationRefusesBeforeAttemptCreation(t *testing.T) {
	cfg, profile := openCodeHostFixture(t)
	for _, mode := range []string{"missing", "foreign", "native-limit"} {
		t.Run(mode, func(t *testing.T) {
			config := map[string]any{"goal": "write", "tool_profile_sha256": profile.ProfileSHA256}
			if mode != "missing" {
				c := *profile.OpenCodeExecution
				if mode == "foreign" {
					c.Model = "other104"
				}
				config["opencode_execution"] = &c
			}
			if mode == "native-limit" {
				config["max_steps"] = 2
			}
			a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "opencode-gate-" + mode, NodeID: "develop", WorkerID: "worker104", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"opencode104"}, Config: config}
			r := Execute(context.Background(), a, cfg)
			if r.ExitCode == 0 || r.Error == "" || r.WorkspaceEvidence != nil {
				t.Fatalf("unfrozen configuration reached workspace: %+v", r)
			}
			if _, err := os.Stat(filepath.Join(cfg.Repository, ".cache", "runner", "worktrees", attemptKey(a))); !os.IsNotExist(err) {
				t.Fatal("rejected binding created attempt state")
			}
		})
	}
}
