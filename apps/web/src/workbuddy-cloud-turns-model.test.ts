import {describe,it,expect} from 'vitest';
import {createHash} from 'node:crypto';
import {validCloudTurnsScope,cloudTurnsIdentity,cloudTurnViewPath,cloudTurnPagePath,cloudTurnPromptPath,cloudTurnPermissionPath,cloudTurnControlPath,readCloudTurnView,readCloudTurnPage,validCloudTurnRecovery,validCloudTurnReceipt} from './workbuddy-cloud-turns-model';
const sha='a'.repeat(64),at='2026-10-31T12:00:00Z';
const scope={userID:'u1',organizationID:'o1',source:{project_id:'p1',run_id:'r1',node_id:'n1',submission_id:'s1',submission_version:3,task_id:'provider-task1',node_spec_sha256:sha,request_sha256:sha}};
const source=()=>({collaboration:{...scope.source},attempt:1,fence:2,dispatch_id:'',assignment_sha256:sha});
const prompt='Continue the original task';
function turn(){return {id:'turn1',version:1,operation_id:'original-operation',head_version_at_start:0,source:source(),owner_id:'u1',state:'prepared',prompt_sha256:createHash('sha256').update(prompt).digest('hex'),started_at:at,deadline_at:'2026-10-31T12:15:00Z',overdue:false,usage_state:'unreported'};}
const req=()=>({path:'/workbuddy-cloud-submissions/s1/turns',method:'POST',idempotency_key:'original-operation',body:{project_id:'p1',expected_submission_version:3,expected_run_version:7,expected_version:0,prompt,idempotency_key:'original-operation'}});
const view=()=>({source:source(),version:0,allowed_actions:['prompt']});
const page=()=>({turn:turn(),items:[{sequence:1,kind:'assistant_text',text:'Working on the original task'}],permissions:[],commands:[],next_after:1,has_more:false});
describe('cloud turn model',()=>{
 it('keeps source identity stable across observations and isolates users and original tasks',()=>{
  expect(validCloudTurnsScope(scope)).toBe(true);
  expect(cloudTurnsIdentity({...scope,source:{...scope.source,submission_version:4}})).toBe(cloudTurnsIdentity(scope));
  expect(cloudTurnsIdentity({...scope,userID:'u2'})).not.toBe(cloudTurnsIdentity(scope));
  expect(cloudTurnsIdentity({...scope,source:{...scope.source,task_id:'other'}})).not.toBe(cloudTurnsIdentity(scope));
  expect(cloudTurnViewPath(scope)).toBe('/workbuddy-cloud-submissions/s1/turns?project_id=p1');
  expect(cloudTurnPagePath('p1','turn1',50)).toBe('/workbuddy-cloud-turns/turn1?project_id=p1&after=50&limit=50');
  expect(cloudTurnPermissionPath('turn1','permission1')).toBe('/workbuddy-cloud-turns/turn1/permissions/permission1/reply');
 });
 it('reads current heads and original turns without upgrading completed to task success',()=>{
  expect(readCloudTurnView(view(),scope)).toBeTruthy();
  expect(readCloudTurnView({...view(),version:1,latest_turn:turn(),allowed_actions:[]},scope)).toBeTruthy();
  expect(readCloudTurnView({...view(),source:{...source(),collaboration:{...scope.source,task_id:'wrong'}}},scope)).toBeUndefined();
  expect(readCloudTurnView({...view(),version:1,latest_turn:{...turn(),owner_id:'other'}},scope)).toBeUndefined();
  const done={...turn(),state:'completed',finished_at:'2026-10-31T12:01:00Z',stop_reason:'end_turn'};
  expect(readCloudTurnPage({...page(),turn:done},scope,'turn1')?.turn.state).toBe('completed');
  expect(readCloudTurnPage({...page(),turn:{...done,usage_state:'unreported',usage:{input_tokens:0}}},scope,'turn1')).toBeUndefined();
 });
 it('checks dense local paging, bounded safe text and precise turn-bound controls',()=>{
  expect(readCloudTurnPage(page(),scope,'turn1')).toBeTruthy();
  const permission={id:'permission1',turn_id:'turn1',version:1,tool_call_id:'tool-call-1',title:'Write the reviewed file',details:'Only this file',details_sha256:sha,approve_allowed:true,state:'pending',options:[{id:'allow-one',name:'Allow once',kind:'allow_once'},{id:'deny-one',name:'Reject once',kind:'reject_once'},{id:'allow-always',name:'Always',kind:'allow_always'}]};
  expect(readCloudTurnPage({...page(),permissions:[permission]},scope,'turn1')?.permissions[0].options).toHaveLength(3);
  const token='工具:'+ 'é'.repeat(60)+'a'; // 128 UTF-8 bytes, including a provider namespace.
  const providerPermission={...permission,tool_call_id:token,options:[{id:token,name:'Once',kind:'allow_once'}]};
  expect(readCloudTurnPage({...page(),permissions:[providerPermission]},scope,'turn1')).toBeTruthy();
  for(const invalid of [token+'a','工具:\n操作']){
   expect(readCloudTurnPage({...page(),permissions:[{...providerPermission,tool_call_id:invalid}]},scope,'turn1')).toBeUndefined();
   expect(readCloudTurnPage({...page(),permissions:[{...providerPermission,options:[{id:invalid,name:'Once',kind:'allow_once'}]}]},scope,'turn1')).toBeUndefined();
  }
  expect(readCloudTurnPage({...page(),permissions:[{...permission,turn_id:'other'}]},scope,'turn1')).toBeUndefined();
  for(const patch of [{next_after:2},{items:[{sequence:2,kind:'assistant_text',text:'gap'}]},{items:[{sequence:1,kind:'assistant_text',text:'x'.repeat(32769)}]},{items:[{sequence:1,kind:'artifact',text:'file:///secret'}]},{permissions:[{}]}])expect(readCloudTurnPage({...page(),...patch},scope,'turn1')).toBeUndefined();
  expect(readCloudTurnPage(page(),scope,'other')).toBeUndefined();
  expect(readCloudTurnPage({...page(),items:[],next_after:50,has_more:false},scope,'turn1',50)).toBeTruthy();
  expect(readCloudTurnPage({...page(),items:[],next_after:0,has_more:true},scope,'turn1')).toBeUndefined();
 });
 it('admits only closed artifact change metadata and bounded previews',()=>{
  const artifact={sequence:1,kind:'artifact',artifact_type:'plan',artifact_event:'updated',text:'é'.repeat(4096)};
  const read=(entry:unknown)=>readCloudTurnPage({...page(),items:[entry]},scope,'turn1');
  expect(read(artifact)?.items).toEqual([artifact]);
  expect(read({...artifact,artifact_type:'overview',artifact_event:'created',text:'Summary'})).toBeDefined();
  expect(read({...artifact,artifact_type:'media',artifact_event:'deleted',text:''})).toBeDefined();
  expect(read({...artifact,artifact_type:'tasks',text:''})).toBeDefined();
  const {artifact_type,artifact_event,...base}=artifact;
  for(const entry of [
   {...base,artifact_type},
   {...base,artifact_event},
   {...artifact,artifact_type:null},
   {...artifact,artifact_event:null},
   {...artifact,artifact_type:'commit'},
   {...artifact,artifact_event:'downloaded'},
   {...artifact,tool_call_id:'tool-1'},
   {...artifact,tool_state:'completed'},
   {...artifact,artifact_type:'tasks'},
   {...artifact,artifact_type:'media'},
   {...artifact,artifact_event:'deleted'},
   {...artifact,text:artifact.text+'é'},
   {...artifact,text:'bad\u0000text'},
   {...artifact,kind:'assistant_text'},
   {...artifact,url:'https://example.invalid/artifact'},
  ])expect(read(entry)).toBeUndefined();
 });

 it('allows only exact prompt, permission, cancel and reconcile request shapes',()=>{
  expect(cloudTurnPromptPath('s1')).toBe(req().path);expect(validCloudTurnRecovery(req(),scope)).toBe(true);
  for(const kind of ['cancel','reconcile'] as const){const command={path:cloudTurnControlPath('turn1',kind),method:'POST',idempotency_key:'original-operation',body:{project_id:'p1',expected_version:1,idempotency_key:'original-operation'}};expect(validCloudTurnRecovery(command,scope)).toBe(true);}
  const reply={path:cloudTurnPermissionPath('turn1','permission1'),method:'POST',idempotency_key:'original-operation',body:{project_id:'p1',expected_version:1,option_id:'allow-one',idempotency_key:'original-operation'}};expect(validCloudTurnRecovery(reply,scope)).toBe(true);
  for(const modified of [{...req(),path:req().path+'?project_id=p1'},{...req(),path:'/workbuddy-cloud-submissions/other/turns'},{...req(),body:{...req().body,expected_version:2147483647}},{...req(),body:{...req().body,prompt:'\ud800'}},{...req(),body:{...req().body,prompt:'x'.repeat(32001)}},{...req(),body:{...req().body,url:'https://elsewhere'}},{...req(),body:{...req().body,project_id:'other'}}])expect(validCloudTurnRecovery(modified,scope)).toBe(false);
 });
 it('confirms exact original prompt digest and admission without reusing later head versions',async()=>{
  expect(await validCloudTurnReceipt(turn(),req(),scope)).toBe(true);
  expect(await validCloudTurnReceipt({...turn(),version:5,state:'unknown',finished_at:'2026-10-31T12:02:00Z'},req(),{...scope,source:{...scope.source,submission_version:4}})).toBe(true);
  for(const patch of [{operation_id:'other-operation'},{prompt_sha256:sha},{head_version_at_start:1},{source:{...source(),collaboration:{...scope.source,submission_version:4}}}])expect(await validCloudTurnReceipt({...turn(),...patch},req(),scope)).toBe(false);
 });
 it('binds control receipts to original turn, permission, option and operation, never treating prepared as sent',async()=>{
  const request={path:cloudTurnPermissionPath('turn1','permission1'),method:'POST',idempotency_key:'original-operation',body:{project_id:'p1',expected_version:1,option_id:'allow-one',idempotency_key:'original-operation'}};
  const receipt={id:'command1',turn_id:'turn1',operation_id:'original-operation',kind:'permission',state:'prepared',permission_id:'permission1',option_id:'allow-one'};
  expect(await validCloudTurnReceipt(receipt,request,scope)).toBe(true);expect(receipt.state).toBe('prepared');
  for(const patch of [{turn_id:'other'},{permission_id:'other'},{option_id:'allow-always'},{operation_id:'other-operation'},{kind:'cancel'},{token:'secret'}])expect(await validCloudTurnReceipt({...receipt,...patch},request,scope)).toBe(false);
  const cancel={...request,path:cloudTurnControlPath('turn1','cancel'),body:{project_id:'p1',expected_version:1,idempotency_key:'original-operation'}};
  expect(await validCloudTurnReceipt({id:'command2',turn_id:'turn1',operation_id:'original-operation',kind:'cancel',state:'unknown'},cancel,scope)).toBe(true);
 });
});
