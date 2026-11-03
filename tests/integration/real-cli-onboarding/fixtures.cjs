'use strict';
// Public synthetic observations; no vendor executable, credential or OS IPC.
const crypto=require('node:crypto');
const sha=value=>crypto.createHash('sha256').update(value).digest('hex'),utc=value=>new Date(value??Date.now()).toISOString();
const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),worker='worker-probe226';
const session={user:{id:user,name:'Synthetic owner',email:'owner@example.test'},organization:{id:tenant,name:'Synthetic team'}};
const codebuddy={id:'codebuddy-cli',product:'CodeBuddy Code',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',entrypoint_kind:'cli',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const decoy={...codebuddy,id:'claude-code-sdk',product:'Claude Code',entrypoint_kind:'sdk',adapter:'claude',protocol:'claude-sdk-v1'};
function scan(){
 const at=utc(),source={ID:'scan-probe226',ProjectID:project,WorkerID:worker,AdapterIDs:[codebuddy.id],InstanceID:'instance-probe226',RequestedBy:user,CreatedAt:at,ExpiresAt:utc(Date.parse(at)+300000)},source_sha256=sha(JSON.stringify(source));
 return {id:source.ID,project_id:project,worker_id:worker,adapter_ids:source.AdapterIDs,instance_id:source.InstanceID,version:3,state:'completed',requested_by:user,created_at:at,expires_at:source.ExpiresAt,source_sha256,fence:1,claim_id:'claim-probe226',claimed_at:at,lease_until:utc(Date.parse(at)+60000),finished_at:at,result_code:'scan_completed',result:{items:[{adapter_id:codebuddy.id,protocol:codebuddy.protocol,installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic:{stage:'help',code:'help_contract_mismatch',next_action:'guide'}}],stop_proof:{protocol:'suma-installed-tool-stop-v1',scan:{scan_id:source.ID,claim_id:'claim-probe226',fence:1},source_sha256,receipt_sha256:sha('synthetic stopped scan 226'),probe_returned:true,all_native_trees_stopped:true,native_process_count:0,stopped_process_count:0,observed_at:at}}};
}
function maintenance(){const at=utc();return {state:'supported',checked_at:at,protocol:'suma-tool-maintenance-v1',instance_id:'instance-probe226',observed_at:at,expires_at:utc(Date.parse(at)+60000),installed_discovery:{protocol:'suma-installed-tool-scan-v1',adapter_ids:[codebuddy.id,decoy.id]}};}
function localResult(stopped=false){
 const ids=['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','qoder-acp','codex-app-server','opencode-sdk','claude-code-sdk'],products=['Codex','CodeBuddy Code','DeepSeek Harness / dsh','Claude Code','Cursor','Qoder CLI','Codex','OpenCode','Claude Code'],at=utc();
 const items=ids.map((id,index)=>({adapter_id:id,product:products[index],installed:false,launch_supported:false,reason_code:'not_installed'}));
 items[1]={adapter_id:'codebuddy-cli',product:'CodeBuddy Code',installed:true,launch_supported:false,reason_code:'probe_failed',diagnostic:{stage:'version',code:stopped?'stop_unverified':'probe_timeout',next_action:stopped?'none':'scan'}};
 return {status:'observed',report:{protocol:'suma-installed-tools-v1',items},scan:{id:(stopped?'5':'4').repeat(32),project_id:project,user_id:user,tenant_id:tenant,observed_at:at,expires_at:utc(Date.parse(at)+60000),cached:false}};
}
module.exports={sha,utc,project,user,tenant,worker,session,codebuddy,decoy,scan,maintenance,localResult};
