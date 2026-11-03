'use strict';
// Deliberately synthetic public HTTP facts. No native tool, credential value,
// local configuration, database or task execution is represented by this data.
const crypto=require('node:crypto');
const sha=value=>crypto.createHash('sha256').update(value).digest('hex');
const project='project-managed224',worker='worker-managed224',preparationID='preparation-ui-224';
const session={user:{id:'owner-managed224',name:'Fixture owner',email:'owner@example.test',role:'owner'},organization:{id:'organization-managed224',name:'Fixture organization'},projects:[{id:project,version:1,title:'Owned synthetic project',role:'admin'}]};
const adapter={id:'qoder-acp',product:'Qoder',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'qoder',protocol:'acp-stdio-v1',capabilities:[],source_refs:[],limitations:[]};
const model='qoder-review-model',credential='TEAM_TOOL_CREDENTIAL',instance='instance-managed224';
const generation=sha('synthetic original generation'),nextGeneration=sha('synthetic accepted generation'),command='managed-'+sha(JSON.stringify(preparationID));
const item={adapter_id:adapter.id,protocol:adapter.protocol,installed:true,launch_supported:false,reason_code:'acp_configuration_required',version:'reviewed-native-224',executable_sha256:sha('synthetic reviewed executable')};
const utc=(value=Date.now())=>new Date(value).toISOString();
function createFacts(){
 const created=utc(),scanSource={ID:'scan-managed224',ProjectID:project,WorkerID:worker,AdapterIDs:[adapter.id],InstanceID:instance,RequestedBy:session.user.id,CreatedAt:created,ExpiresAt:utc(Date.parse(created)+300000)},scanSHA=sha(JSON.stringify(scanSource));
 const scanReference={scan_id:scanSource.ID,claim_id:'scan-claim-managed224',fence:1};
 const scanProof={protocol:'suma-installed-tool-stop-v1',scan:scanReference,source_sha256:scanSHA,receipt_sha256:sha('synthetic scan stop receipt'),probe_returned:true,all_native_trees_stopped:true,native_process_count:0,stopped_process_count:0,observed_at:created};
 const scan={id:scanSource.ID,project_id:project,worker_id:worker,adapter_ids:scanSource.AdapterIDs,instance_id:instance,version:3,state:'completed',requested_by:session.user.id,created_at:created,expires_at:scanSource.ExpiresAt,source_sha256:scanSHA,fence:1,claim_id:scanReference.claim_id,claimed_at:created,lease_until:utc(Date.parse(created)+60000),finished_at:created,result_code:'scan_completed',result:{items:[item],stop_proof:scanProof}};
 const profile={command_id:command,adapter:'qoder',protocol:'acp-stdio-v1',profile_sha256:sha('synthetic accepted complete profile'),executable_sha256:item.executable_sha256,tool_version:item.version,read_only:false,acp_execution:{runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:sha('synthetic fixed configuration'),model,model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'}};
 const base={id:preparationID,tenant_id:session.organization.id,project_id:project,worker_id:worker,version:1,state:'pending',adapter_id:adapter.id,configuration:{model,credential_env_ref:credential},instance_id:instance,installation_scan_id:scan.id,installation_scan_version:scan.version,installation_reference:scanReference,installation_source_sha256:scanSHA,installation_item:item,installation_stop_proof:scanProof,expected_generation_sha256:generation,requested_by:session.user.id,created_at:created,expires_at:scanSource.ExpiresAt,source_sha256:'',fence:0};
 // Current Go SourceSHA256 omits mutable claim/result fields and retains the
 // required zero-valued version/state/hash/fence in declaration order.
 base.source_sha256=sha(JSON.stringify({...base,version:0,state:'',source_sha256:'',fence:0}));
 const reference={preparation_id:preparationID,claim_id:'prepare-claim-managed224',fence:1};
 const proof={protocol:'suma-managed-tool-preparation-stop-v1',preparation:reference,source_sha256:base.source_sha256,receipt_sha256:sha('synthetic configuration stop receipt'),probe_returned:true,all_native_trees_stopped:true,native_process_count:0,stopped_process_count:0,observed_at:created};
 const receipt={parent_generation_sha256:generation,generation_sha256:nextGeneration,command_id:command,artifact_sha256:sha('synthetic new artifact'),local_receipt_sha256:sha('synthetic local acceptance receipt'),profile,stop_proof:proof};
 function operation(state,inventoryAt=created){if(state==='pending')return structuredClone(base);return {...base,version:state==='completed'?4:3,state,fence:1,claim_id:reference.claim_id,claimed_at:created,lease_until:utc(Date.parse(created)+60000),result_code:state==='completed'?'completed':'prepared',receipt,...(state==='completed'?{finished_at:inventoryAt,inventory_operation_id:'inventory-operation-managed224',inventory_observed_at:inventoryAt}:{})};}
 function maintenance(accepted=false){const at=utc();return {state:'supported',checked_at:at,protocol:'suma-tool-maintenance-v1',instance_id:instance,observed_at:at,expires_at:utc(Date.parse(at)+60000),installed_discovery:{protocol:'suma-installed-tool-scan-v1',adapter_ids:[adapter.id]},managed_preparation:{protocol:'suma-managed-tool-preparation-v1',adapter_ids:[adapter.id],generation_sha256:accepted?nextGeneration:generation}};}
 function inventory(at,drift=false){const current=drift?{...profile,profile_sha256:sha('synthetic changed profile same command'),acp_execution:{...profile.acp_execution,model:'different-review-model',configuration_sha256:sha('synthetic changed fixed configuration')}}:profile;return {project_id:project,worker_id:worker,state:'fresh',as_of:utc(Math.max(Date.now(),Date.parse(at))),observed_at:at,expires_at:utc(Date.parse(at)+60000),items:[{profile:current,available:drift,reason:drift?'':'Execution disabled (maintenance-only fixture)'}]};}
 return {scan,profile,operation,maintenance,inventory,created};
}
module.exports={sha,project,worker,preparationID,session,adapter,model,credential,generation,command,utc,createFacts};
