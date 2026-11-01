import {expect,it} from 'vitest';
import type {ToolPermission,ToolPermissionDecisionRequest} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {buildPermissionDecisionBody} from './tool-permission-model';
import {claudeSDKPermissionRecord as sdkRow,claudeSDKPermissionScope as sdkScope,claudeSDKPermissionChoice as sdkChoice,claudeSDKDeliveryObservation as sdkObservation} from './claude-sdk.test-support';
import {permissionListPath,validPermission,validPermissionRecovery,validPermissionReceipt,canChoosePermission,permissionRecoveryScope,validPermissionIntent,permissionMatchesIntent,buildPermissionIntent,type ToolPermissionScope} from './tool-permission-model';

const hash='a'.repeat(64),created='2026-10-31T00:00:00Z',expires='2099-10-31T00:00:00Z';
const scope:ToolPermissionScope={project:'project-a',runID:'run-a',nodeID:'node-a',attempt:2,fence:3,userID:'user-a',organizationID:'org-a',protocol:'acp-stdio-v1'};
const choice={option_id:'edit:一次',kind:'allow_once'};
function permission(){return {id:'permission-a',project_id:scope.project,run_id:scope.runID,node_id:scope.nodeID,attempt:2,fence:3,dispatch_id:'dispatch-a',worker_id:'worker-a',profile_sha256:hash,version:1,state:'pending',request:{idempotency_key:'machine-original-key',protocol:'acp-stdio-v1',acp:{session_id:'session-a',tool_call_id:'call:一',title:'Reviewed edit',kind:'edit',details:'Original reviewed file',details_complete:true,options:[{id:choice.option_id,name:'Allow once',kind:'allow_once'},{id:'reject',name:'Reject once',kind:'reject_once'}]}},created_at:created,expires_at:expires,allowed_actions:['allow_once','reject_once']};}
function request():WriteRecoveryRequest{return {path:'/tool-permissions/permission-a/decision',method:'POST',idempotency_key:'human-original-key',body:{project_id:scope.project,expected_version:1,idempotency_key:'human-original-key',protocol:'acp-stdio-v1',acp:choice} satisfies ToolPermissionDecisionRequest};}

it('reads the exact current protocol branch and rejects flattened or mixed permission sources',()=>{
 const value=permission();expect(validPermission(value,scope)).toBe(true);
 const displayNames={...value,request:{...value.request,acp:{...value.request.acp,options:value.request.acp.options.map(option=>({...option,name:` ${option.name} `}))}}};
 expect(validPermission(displayNames,scope)).toBe(true);expect(displayNames.request.acp.options[0].name).toBe(' Allow once ');
 expect(validPermission({...value,allowed_actions:['reject_once']},scope)).toBe(true);
 expect(validPermission({...value,allowed_actions:['reject_once','allow_once']},scope)).toBe(false);
 const rejectOnly={...value,request:{...value.request,acp:{...value.request.acp,options:[value.request.acp.options[1]]}},allowed_actions:['reject_once']};
 expect(validPermission(rejectOnly,scope)).toBe(true);
 expect(validPermission({...rejectOnly,allowed_actions:['allow_once','reject_once']},scope)).toBe(false);
 expect(permissionListPath(scope)).toBe('/runs/run-a/nodes/node-a/tool-permissions?project_id=project-a');
 expect(validPermission(value,{...scope,protocol:'codex-app-server-v1'})).toBe(false);
 for(const bad of [{...value,request:{idempotency_key:value.request.idempotency_key,...value.request.acp}},{...value,request:{...value.request,codex:{}}},{...value,request:{...value.request,protocol:'unknown'}},{...value,request:{...value.request,acp:{...value.request.acp,argv:['shell']}}},{...value,request:{...value.request,acp:null}}])expect(validPermission(bad,scope)).toBe(false);
});

it('keeps the native choice and original caller operation closed through decision recovery',()=>{
 const original=request(),value=permission();expect(validPermissionRecovery(original,scope)).toBe(true);
 const receipt={...value,state:'decided',version:2,allowed_actions:[],decision:{protocol:'acp-stdio-v1',acp:choice,actor_id:scope.userID,decided_at:'2026-10-31T00:01:00Z'}};
 expect(validPermissionReceipt(receipt,original,scope)).toBe(true);
 for(const body of [{...original.body,option_id:choice.option_id},{...original.body,codex:{decision:'accept'}},{...original.body,protocol:'codex-app-server-v1'},{...original.body,idempotency_key:'another-operation'}])expect(validPermissionRecovery({...original,body},scope)).toBe(false);
 for(const patch of [{attempt:1},{fence:2},{version:3},{decision:{...receipt.decision,actor_id:'someone-else'}},{decision:{...receipt.decision,acp:{option_id:'reject',kind:'reject_once'}}}])expect(validPermissionReceipt({...receipt,...patch},original,scope)).toBe(false);
});

it('retains exact old attempt source identity while separating another user or protocol',()=>{
 const value=permission() as ToolPermission,original=request(),intent=buildPermissionIntent(value,scope,original);
 expect(intent).toBeDefined();if(!intent)throw Error('Missing source-bound intent');
 expect(validPermissionIntent(intent,{...scope,attempt:3,fence:4})).toBe(true);expect(permissionMatchesIntent(value,intent)).toBe(true);
 expect(permissionRecoveryScope(scope)).toBe(permissionRecoveryScope({...scope,attempt:3,fence:4}));
 expect(permissionRecoveryScope(scope)).not.toBe(permissionRecoveryScope({...scope,protocol:'codex-app-server-v1'}));
 expect(validPermissionIntent(intent,{...scope,userID:'someone-else'})).toBe(false);
 expect(validPermissionIntent(intent,{...scope,protocol:'codex-app-server-v1'})).toBe(false);
 expect(JSON.stringify(intent)).not.toContain('Original reviewed file');
 for(const patch of [{session_id:'other-session'},{tool_call_id:'other-call'}])expect(permissionMatchesIntent({...value,request:{...permission().request,acp:{...permission().request.acp,...patch}}},intent)).toBe(false);
 expect(permissionMatchesIntent({...value,request:{...permission().request,idempotency_key:'another-machine-key'}},intent)).toBe(false);
 expect(permissionMatchesIntent({...value,profile_sha256:'b'.repeat(64)},intent)).toBe(false);
});

it('offers only native once choices from a complete current unexpired request',()=>{
 const value=permission() as ToolPermission;
 expect(canChoosePermission(value,choice,scope)).toBe(true);expect(canChoosePermission(value,{option_id:'reject',kind:'reject_once'},scope)).toBe(true);
 expect(canChoosePermission(value,{decision:'accept'},scope)).toBe(false);
 expect(canChoosePermission(value,undefined,scope)).toBe(false);
 expect(canChoosePermission({...value,attempt:1},choice,scope)).toBe(false);expect(canChoosePermission({...value,expires_at:created},choice,scope)).toBe(false);
 expect(canChoosePermission({...value,allowed_actions:[]},choice,scope)).toBe(false);
 const incomplete={...permission(),request:{...permission().request,acp:{...permission().request.acp,details_complete:false}},allowed_actions:['reject_once']} as ToolPermission;
 expect(canChoosePermission(incomplete,choice,scope)).toBe(false);expect(canChoosePermission(incomplete,{option_id:'reject',kind:'reject_once'},scope)).toBe(true);
});

const codexScope:ToolPermissionScope={...scope,protocol:'codex-app-server-v1'};
function codexPermission(){return {...permission(),request:{idempotency_key:'machine-codex-key',protocol:'codex-app-server-v1',codex:{request_id:{kind:'number',value:'9007199254740993'},method:'item/commandExecution/requestApproval',thread_id:'thread-a',turn_id:'turn-a',item_id:'item-a',started_at_ms:1793404800000,approval_id:'approval-a',binding_sha256:hash,params_sha256:'b'.repeat(64),item_sha256:'c'.repeat(64),title:'Run reviewed command',details:'Reviewed command details',details_complete:true,scope_supported:true,command:'npm test',cwd:'.'}},allowed_actions:['accept','decline','cancel']};}
function codexRequest():WriteRecoveryRequest{return {...request(),body:{project_id:scope.project,expected_version:1,idempotency_key:'human-original-key',protocol:codexScope.protocol,codex:{decision:'accept'}}};}
it('keeps Codex typed RPC identity and reviewed command or file scope distinct',()=>{
 const value=codexPermission();expect(validPermission(value,codexScope)).toBe(true);
 for(const request_id of [{kind:'number',value:'9223372036854775808'},{kind:'number',value:'01'},{kind:'number',value:'1.5'}])expect(validPermission({...value,request:{...value.request,codex:{...value.request.codex,request_id}}},codexScope)).toBe(false);
 expect(validPermission({...value,request:{...value.request,codex:{...value.request.codex,request_id:{kind:'string',value:'9007199254740993'}}}},codexScope)).toBe(true);
 const {command:_,cwd:__,...base}=value.request.codex;
 const files={...value,request:{...value.request,codex:{...base,method:'item/fileChange/requestApproval',changes:[{path:'src/file.ts',kind:'update',diff:'Reviewed patch',move_path:'src/renamed.ts'}]}}};
 expect(validPermission(files,codexScope)).toBe(true);
 for(const path of ['../file.ts','C:/secret','/secret','src//file.ts'])expect(validPermission({...files,request:{...files.request,codex:{...files.request.codex,changes:[{path,kind:'update',diff:''}]}}},codexScope)).toBe(false);
 const unsupported={...value,request:{...value.request,codex:{...value.request.codex,scope_supported:false,unsupported_reason:'network_extension'}},allowed_actions:['decline','cancel']};
 expect(validPermission(unsupported,codexScope)).toBe(true);expect(canChoosePermission(unsupported as ToolPermission,{decision:'accept'},codexScope)).toBe(false);expect(canChoosePermission(unsupported as ToolPermission,{decision:'decline'},codexScope)).toBe(true);
});
it('recovers the original Codex choice without substituting another native request or machine key',()=>{
 const value=codexPermission() as ToolPermission,original=codexRequest(),intent=buildPermissionIntent(value,codexScope,original);
 expect(intent).toBeDefined();if(!intent)throw Error('Missing native intent');
 expect(validPermissionIntent(intent,{...codexScope,attempt:9,fence:10})).toBe(true);
 expect(permissionMatchesIntent(value,intent)).toBe(true);expect(JSON.stringify(intent)).not.toContain('npm test');
 const source=codexPermission().request.codex;
 for(const patch of [{request_id:{kind:'string',value:source.request_id.value}},{turn_id:'another-turn'},{approval_id:'another-approval'},{started_at_ms:source.started_at_ms+1},{params_sha256:hash}])expect(permissionMatchesIntent({...value,request:{...codexPermission().request,codex:{...source,...patch}}},intent)).toBe(false);
 const receipt={...value,state:'decided',version:2,allowed_actions:[],decision:{protocol:codexScope.protocol,codex:{decision:'accept'},actor_id:scope.userID,decided_at:'2026-10-31T00:01:00Z'}};
 expect(validPermissionReceipt(receipt,original,codexScope)).toBe(true);
 expect(validPermissionReceipt({...receipt,decision:{...receipt.decision,codex:{decision:'cancel'}}},original,codexScope)).toBe(false);
});
it('reads Codex observations as separate machine facts without inventing reply or item success',()=>{
 const value={...codexPermission(),allowed_actions:[]},observation={sequence:1,params_sha256:value.request.codex.params_sha256,decision_observed:false,reply_started:false,reply_written:false,request_resolved:true};
 expect(validPermission({...value,observation},codexScope)).toBe(true);
 expect(validPermission({...value,allowed_actions:['decline','cancel'],observation},codexScope)).toBe(false);
 expect(validPermission({...value,allowed_actions:['decline','cancel'],observation:{...observation,request_resolved:false,item_status:'failed'}},codexScope)).toBe(false);
 expect(validPermission({...value,allowed_actions:['accept','decline','cancel'],observation:{...observation,request_resolved:false,item_status:'inProgress'}},codexScope)).toBe(true);
 expect(validPermission({...value,observation:{...observation,reply_written:true}},codexScope)).toBe(false);
 expect(validPermission({...value,observation:{...observation,params_sha256:hash}},codexScope)).toBe(false);
 expect(validPermission({...permission(),observation},scope)).toBe(false);
 const decided={...value,state:'decided',version:2,allowed_actions:[],decision:{protocol:codexScope.protocol,codex:{decision:'accept'},actor_id:scope.userID,decided_at:'2026-10-31T00:01:00Z'},observation:{...observation,decision:'accept',decision_observed:true,reply_started:true,reply_written:true,item_status:'failed'}};
 expect(validPermission(decided,codexScope)).toBe(true);
 expect(validPermission({...decided,observation:{...decided.observation,decision:'decline'}},codexScope)).toBe(false);
});
it('T192 preserves exact SDK one-call choices and original source recovery without Codex acknowledgements or input rewriting',()=>{
 expect(validPermission(sdkRow,sdkScope)).toBe(true);expect(canChoosePermission(sdkRow,sdkChoice,sdkScope)).toBe(true);expect(canChoosePermission(sdkRow,{outcome:'reject_once',reason:'Review required.'},sdkScope)).toBe(true);
 const key='sdk-permission-original',body=buildPermissionDecisionBody(sdkRow,sdkScope,sdkChoice,key);expect(body).toEqual({project_id:sdkScope.project,expected_version:1,idempotency_key:key,protocol:'claude-sdk-v1',claude:sdkChoice});
 const req:WriteRecoveryRequest={path:'/tool-permissions/'+sdkRow.id+'/decision',method:'POST',idempotency_key:key,body:body!},intent=buildPermissionIntent(sdkRow,sdkScope,req)!;expect(validPermissionIntent(intent,{...sdkScope,attempt:3,fence:10})).toBe(true);expect(permissionMatchesIntent(sdkRow,intent)).toBe(true);expect(JSON.stringify(intent)).not.toContain('input_json');expect(JSON.stringify(intent)).not.toContain('git diff --');
 const receipt={...sdkRow,version:2,state:'decided',allowed_actions:[],decision:{protocol:'claude-sdk-v1',claude:{outcome:'allow_once',reason:''},actor_id:sdkScope.userID,decided_at:'2026-11-01T04:01:00Z'},claude_observation:{...sdkObservation,reply_written:true,execution_closed:true}};expect(validPermissionReceipt(receipt,req,sdkScope)).toBe(true);expect(permissionMatchesIntent(receipt,intent)).toBe(true);expect(canChoosePermission(receipt,sdkChoice,sdkScope)).toBe(false);
 for(const patch of [{request_id:'another-request'},{callback_id:'another-callback'},{params_sha256:'f'.repeat(64)},{tool_name:'Write'}])expect(permissionMatchesIntent({...sdkRow,request:{...sdkRow.request,claude:{...sdkRow.request.claude,...patch}}},intent)).toBe(false);
 expect(validPermission({...receipt,claude_observation:{...receipt.claude_observation,request_resolved:true}},sdkScope)).toBe(false);expect(validPermission({...receipt,observation:{sequence:1}},sdkScope)).toBe(false);expect(validPermission({...sdkRow,request:{...sdkRow.request,codex:codexPermission().request.codex}},sdkScope)).toBe(false);expect(validPermissionRecovery({...req,body:{...req.body,acp:choice}},sdkScope)).toBe(false);expect(validPermissionIntent(intent,{...sdkScope,userID:'other-user'})).toBe(false);
 expect(buildPermissionDecisionBody(sdkRow,sdkScope,{outcome:'allow_once',updated_input:{}} as never,key)).toBeUndefined();
});
