import {describe,it,expect} from 'vitest';
import {validACPCapability,validACPEvidence,validACPPlan,validACPDispatchReceipt} from './acp-execution-model';
import {permissionListPath,validPermission,validPermissionRecovery,validPermissionReceipt,canChoosePermission,type ToolPermissionScope} from './tool-permission-model';
import {validManagedTaskDispatchRun} from './task-dispatch-response';
import {validModelOptions} from './model-execution-model';
import type {ModelExecutionOptions} from '../../../packages/contracts/src';
const hash='a'.repeat(64),created='2026-10-31T00:00:00Z',expiry='2099-10-31T00:00:00Z';
const scope:ToolPermissionScope={project:'p1',runID:'run1',nodeID:'node1',attempt:2,fence:3,userID:'u1',organizationID:'o1',protocol:'acp-stdio-v1'};
const cap=()=>({runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:hash,model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'});
const proof=()=>({configuration_sha256:hash,requested_model:'fixed-model',selected_model:'fixed-model',model_selection_acknowledged:true,session_id:'session1',protocol_version:1,prompt_started:true,prompt_completed:true,stop_reason:'end_turn',tools_complete:true,permissions_requested:1,permissions_denied:0,cancel_requested:false,host_process_stopped:true,host_process_tree_stopped:true});
const allow={id:'permission:允许',name:'Allow once',kind:'allow_once'},reject={id:'reject:once',name:'Reject once',kind:'reject_once'};
const permission=()=>({id:'permission1',project_id:'p1',run_id:'run1',node_id:'node1',attempt:2,fence:3,dispatch_id:'dispatch1',worker_id:'worker1',profile_sha256:hash,version:1,state:'pending',request:{idempotency_key:'runner-request-key',protocol:'acp-stdio-v1',acp:{session_id:'session1',tool_call_id:'tool:一',title:'Change file',kind:'edit',details:'One reviewed file',details_complete:true,options:[allow,reject,{id:'always',name:'Always',kind:'allow_always'}]}},created_at:created,expires_at:expiry,allowed_actions:['allow_once','reject_once']});
const once=(option:typeof allow)=>({option_id:option.id,kind:option.kind});
const request=()=>({path:'/tool-permissions/permission1/decision',method:'POST',idempotency_key:'human-operation-key',body:{project_id:'p1',expected_version:1,idempotency_key:'human-operation-key',protocol:'acp-stdio-v1',acp:once(allow)}});
describe('ACP execution and permission contracts',()=>{
 it('accepts only fixed administrator Qoder capability without claiming hard budget or vendor identity',()=>{
  expect(validACPCapability(cap())).toBe(true);
  for(const patch of [{runtime:'other'},{sdk_version:'latest'},{budget_mode:'enforce'},{identity_evidence:'verified'},{permission_mode:'always'},{executable:'qoder'}])expect(validACPCapability({...cap(),...patch})).toBe(false);
 });
 it('separates model selection acknowledgement, prompt completion and host stop evidence',()=>{
  expect(validACPEvidence(proof())).toBe(true);
  expect(validACPEvidence({...proof(),prompt_completed:false,stop_reason:undefined})).toBe(false);
  const {stop_reason:_,selected_model:__,session_id:___,protocol_version:____,...notStarted}=proof();
  expect(validACPEvidence({...notStarted,model_selection_acknowledged:false,prompt_started:false,prompt_completed:false,tools_complete:false,host_process_stopped:false,host_process_tree_stopped:false,permissions_requested:0})).toBe(true);
  expect(validACPEvidence({...proof(),selected_model:'different'})).toBe(false);expect(validACPEvidence({...proof(),permissions_denied:2})).toBe(false);
 });
 it('requires exact frozen ACP capability in the original nonretrying producer and rejects mixed bindings',()=>{
  const value={agent_node_id:'agent1',acp_execution:cap(),workflow:{nodes:[{id:'agent1',type:'agent',max_retries:0,config:{acp_execution:cap()}}]}};
  expect(validACPPlan(value)).toBe(true);expect(validACPPlan({workflow:{nodes:[]}})).toBe(true);
  expect(validACPPlan({...value,acp_execution:{...cap(),model:'other'}})).toBe(false);
  expect(validACPPlan({...value,model_execution:{}})).toBe(false);
  expect(validACPPlan({...value,workflow:{nodes:[{...value.workflow.nodes[0],max_retries:1}]}})).toBe(false);
  const body={project_id:'p1',task_id:'task1',expected_task_version:2,workflow_id:'workflow1',workflow_version:3,agent_node_id:'agent1',tool_profile_id:'profile1',expected_tool_version:4};
  const receipt={...value,id:'plan1',project_id:'p1',task_id:'task1',task_version:2,workflow_id:'workflow1',workflow_version:3,tool_profile_id:'profile1',tool_profile_version:4};
  expect(validACPDispatchReceipt(receipt,body)).toBe(true);
  for(const patch of [{task_id:'other'},{task_version:3},{workflow_id:'other'},{workflow_version:4},{tool_profile_id:'other'},{tool_profile_version:5}])expect(validACPDispatchReceipt({...receipt,...patch},body)).toBe(false);
  expect(validACPDispatchReceipt(receipt,{...body,model_execution_selection:{}})).toBe(false);
  const original={...receipt,state:'started',run_id:'run1'},run={id:'run1',project_id:'p1',workflow_id:'managed-run1',workflow_version:1};
  expect(validManagedTaskDispatchRun(run,original,'p1')).toBe(true);
  expect(validManagedTaskDispatchRun(run,{...original,acp_execution:{...cap(),model:'changed'}},'p1')).toBe(false);
 });
 it('reads exact-node history with bounded details and provider option tokens',()=>{
  expect(permissionListPath(scope)).toBe('/runs/run1/nodes/node1/tool-permissions?project_id=p1');
  expect(validPermission(permission(),scope)).toBe(true);expect(validPermission({...permission(),attempt:1,fence:1,allowed_actions:[]},scope)).toBe(true);
  const longScope={...scope,nodeID:'n'.repeat(100)},longPermission={...permission(),node_id:longScope.nodeID,worker_id:'worker:pool/@+'.padEnd(150,'w'),dispatch_id:'dispatch:pool/@+',version:Number.MAX_SAFE_INTEGER};
  expect(permissionListPath(longScope)).toContain('/nodes/'+longScope.nodeID+'/tool-permissions');
  expect(validPermission(longPermission,longScope)).toBe(true);
  expect(validPermission({...longPermission,worker_id:longPermission.worker_id+'x'},longScope)).toBe(false);
  expect(validPermission({...longPermission,version:Number.MAX_SAFE_INTEGER+1},longScope)).toBe(false);
  expect(validPermission({...longPermission,dispatch_id:''},longScope)).toBe(false);
  expect(()=>permissionListPath({...longScope,nodeID:longScope.nodeID+'x'})).toThrow();
  for(const patch of [{project_id:'other'},{run_id:'other'},{profile_sha256:'bad'},{token:'secret'}])expect(validPermission({...permission(),...patch},scope)).toBe(false);
  expect(validPermission({...permission(),request:{...permission().request,acp:{...permission().request.acp,details:'x'.repeat(32769)}}},scope)).toBe(false);
  expect(validPermission({...permission(),request:{...permission().request,acp:{...permission().request.acp,options:[{...allow,id:'bad\nvalue'}]}}},scope)).toBe(false);
 });
 it('offers only exact once decisions with current scope, complete details and unexpired permission',()=>{
  expect(canChoosePermission(permission(),undefined,scope)).toBe(false);
  expect(canChoosePermission(permission(),once(allow),scope)).toBe(true);expect(canChoosePermission(permission(),once(reject),scope)).toBe(true);
  expect(canChoosePermission(permission(),once(permission().request.acp.options[2]),scope)).toBe(false);
  const incomplete={...permission(),request:{...permission().request,acp:{...permission().request.acp,details_complete:false}},allowed_actions:['reject_once']};
  expect(canChoosePermission(incomplete,once(allow),scope)).toBe(false);
  expect(canChoosePermission(incomplete,once(reject),scope)).toBe(true);
  for(const patch of [{attempt:1},{fence:2},{allowed_actions:[]},{state:'expired'},{expires_at:created}])expect(canChoosePermission({...permission(),...patch},once(allow),scope)).toBe(false);
  expect(canChoosePermission(permission(),{...once(allow),option_id:'different'},scope)).toBe(false);
 });
 it('keeps original decision path and body closed and binds the saved decision to the actor and attempt',()=>{
  expect(validPermissionRecovery(request(),scope)).toBe(true);
  for(const bad of [{...request(),path:request().path+'?project_id=p1'},{...request(),body:{...request().body,expected_version:Number.MAX_SAFE_INTEGER}},{...request(),body:{...request().body,decision:'approve'}}])expect(validPermissionRecovery(bad,scope)).toBe(false);
  const receipt={...permission(),state:'decided',version:2,allowed_actions:[],decision:{protocol:'acp-stdio-v1',acp:once(allow),actor_id:'u1',decided_at:'2026-10-31T00:01:00Z'}};
  expect(validPermissionReceipt(receipt,request(),scope)).toBe(true);
  const maxRequest={...request(),path:'/tool-permissions/'+encodeURIComponent('permission:operator@a+')+'/decision',body:{...request().body,expected_version:Number.MAX_SAFE_INTEGER-1}};
  const maxReceipt={...receipt,id:'permission:operator@a+',version:Number.MAX_SAFE_INTEGER};
  expect(validPermissionRecovery(maxRequest,scope)).toBe(true);
  expect(validPermissionReceipt(maxReceipt,maxRequest,scope)).toBe(true);
  expect(canChoosePermission({...permission(),version:Number.MAX_SAFE_INTEGER-1},once(allow),scope)).toBe(true);
  expect(canChoosePermission({...permission(),version:Number.MAX_SAFE_INTEGER},once(allow),scope)).toBe(false);
  for(const name of ['permission/other','x'.repeat(65)])expect(validPermissionRecovery({...maxRequest,path:'/tool-permissions/'+encodeURIComponent(name)+'/decision'},scope)).toBe(false);
  for(const bad of [{...receipt,id:'other'},{...receipt,attempt:1},{...receipt,version:3},{...receipt,decision:{...receipt.decision,actor_id:'u2'}},{...receipt,decision:{...receipt.decision,acp:once(reject)}}])expect(validPermissionReceipt(bad,request(),scope)).toBe(false);
 });
 it('reads fixed ACP model options without opening a gateway budget or parameter selector',()=>{
  const modelScope={project:'p1',profile:'profile1',version:2,sha256:hash};
  const value={project_id:'p1',tool_profile_id:'profile1',tool_profile_version:2,tool_profile_sha256:hash,observed_at:created,supported:false,reason:'fixed_by_tool',codex_supported:false,codex_reason:'',items:[],fixed_acp:cap()} as ModelExecutionOptions;
  expect(validModelOptions(value,modelScope)).toBe(true);
  expect(validModelOptions({...value,supported:true},modelScope)).toBe(false);
  expect(validModelOptions({...value,parameter_limits:{max_steps:2,max_output_tokens:100}},modelScope)).toBe(false);
  expect(validModelOptions({...value,tool_profile_sha256:'b'.repeat(64)},modelScope)).toBe(false);
 });
});
