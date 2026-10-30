import {expect,it} from 'vitest';
import type {CloudCollaborationSource,CloudCollaborationView,CloudHistoryLoad,CloudHistoryPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {collaborationIdentity,collaborationPath,historyLoadPath,historyPagePath,readCloudCollaboration,readCloudHistoryPage,validCloudHistoryRequest,readCloudHistoryReceipt} from './workbuddy-cloud-collaboration-model';

const source=(submission_version=3):CloudCollaborationSource=>({project_id:'project-a',run_id:'run-a',node_id:'cloud-a',submission_id:'submission-a',submission_version,task_id:'provider-original',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)});
const load=(changes:Partial<CloudHistoryLoad>={}):CloudHistoryLoad=>({id:'load-a',version:7,operation_id:'original-history-operation',head_version_at_start:0,source:source(),state:'loaded',coverage:'unverified',started_at:'2026-10-31T02:00:00Z',deadline_at:'2026-10-31T02:00:30Z',overdue:false,finished_at:'2026-10-31T02:00:00.050Z',entry_count:1,unsupported_count:0,text_bytes:2,...changes});
const view=(changes:Partial<CloudCollaborationView>={}):CloudCollaborationView=>({source:source(),version:2,latest_load:load(),last_loaded:load(),allowed_actions:['load_history'],...changes});
const page=(changes:Partial<CloudHistoryPage>={}):CloudHistoryPage=>({load:load(),items:[{sequence:1,kind:'assistant_text',text:'Hi'}],total:1,next_after:1,has_more:false,...changes});
const request=(head=0):WriteRecoveryRequest=>({path:'/workbuddy-cloud-submissions/submission-a/history-loads',method:'POST',idempotency_key:'original-history-operation',body:{project_id:'project-a',expected_submission_version:3,expected_version:head,idempotency_key:'original-history-operation'}});

it('keeps all fixed source identifiers in identity while observation-only versions may advance',()=>{
 expect(collaborationIdentity(source(3))).toBe(collaborationIdentity(source(4)));
 for(const changed of [{...source(),task_id:'another-task'},{...source(),node_spec_sha256:'c'.repeat(64)},{...source(),request_sha256:'c'.repeat(64)}])expect(collaborationIdentity(changed)).not.toBe(collaborationIdentity(source()));
 expect(collaborationPath(source())).toBe('/workbuddy-cloud-submissions/submission-a/collaboration?project_id=project-a');
 expect(historyLoadPath(source())).toBe('/workbuddy-cloud-submissions/submission-a/history-loads');
 expect(historyPagePath('load-a','project-a')).toBe('/workbuddy-cloud-history-loads/load-a?project_id=project-a&after=0&limit=50');
 expect(()=>historyPagePath('../load','project-a')).toThrow();
});

it('rejects foreign or open source objects before exposing a stored conversation',()=>{
 expect(readCloudCollaboration(view(),source())).toBeDefined();
 for(const changed of [{...source(),run_id:'other-run'},{...source(),submission_id:'other-submission'},{...source(),task_id:'other-task'},{...source(),request_sha256:'c'.repeat(64)},{...source(),node_id:'bad/route'},{...source(),node_id:'\ud800'},{...source(),ticket:'private'}])expect(readCloudCollaboration(view({source:changed}),source())).toBeUndefined();
});

it('distinguishes an untouched head from attempted or malformed empty history',()=>{
 expect(readCloudCollaboration({source:source(),version:0,allowed_actions:[]},source())).toEqual({source:source(),version:0,allowed_actions:[]});
 for(const changed of [{source:source(),version:1,allowed_actions:[]},{source:source(),version:0,latest_load:load(),allowed_actions:[]},{...view(),last_loaded:null},{...view(),allowed_actions:['load_history','load_history']},{...view(),allowed_actions:['prompt']},{...view(),allowed_actions:null}])expect(readCloudCollaboration(changed,source())).toBeUndefined();
});

it('retains an older stored load as source observations advance without accepting a future or changed source',()=>{
 expect(readCloudCollaboration(view({source:source(5)}),source(4))).toBeDefined();
 expect(readCloudCollaboration(view({source:source(2)}),source(3))).toBeUndefined();
 expect(readCloudCollaboration(view({source:source(4),latest_load:load({source:source(5)})}),source())).toBeUndefined();
 expect(readCloudHistoryPage(page(),source(5),'load-a')).toBeDefined();
});

it('enforces bounded and internally possible snapshot counters',()=>{
 for(const changed of [{entry_count:501},{entry_count:-1},{unsupported_count:2},{text_bytes:1048577},{entry_count:0,text_bytes:2},{entry_count:1.5},{version:0}])expect(readCloudCollaboration(view({latest_load:load(changed)}),source())).toBeUndefined();
 expect(readCloudCollaboration(view({latest_load:load({entry_count:500,unsupported_count:500,text_bytes:0})}),source())).toBeDefined();
});

it('reads server overdue facts and rejects invalid UTC deadlines without consulting the browser clock',()=>{
 const pending=load({state:'loading',finished_at:undefined,overdue:false,started_at:'2000-01-01T00:00:00Z',deadline_at:'2000-01-01T00:00:30Z'});
 delete pending.finished_at;
 expect(readCloudCollaboration(view({latest_load:pending}),source())).toBeDefined();
 expect(readCloudCollaboration(view({latest_load:{...pending,overdue:true}}),source())).toBeDefined();
 const originalView=view({latest_load:pending});
 for(const changed of [{deadline_at:pending.started_at},{deadline_at:'2026-02-30T00:00:00Z'},{deadline_at:'2026-10-31T02:00:30+00:00'},{overdue:null}]){
  const malformedWire:unknown={...originalView,latest_load:{...pending,...changed}};
  expect(readCloudCollaboration(malformedWire,source())).toBeUndefined();
 }
});

it('keeps loaded, loading and interrupted receipts distinct with only closed safe error codes',()=>{
 const interrupted=load({state:'interrupted',error_code:'workbuddy_cloud_history_interrupted'});
 expect(readCloudCollaboration(view({latest_load:interrupted}),source())).toBeDefined();
 for(const changed of [{state:'loaded',error_code:'workbuddy_cloud_history_interrupted'},{state:'loaded',overdue:true},{state:'loading'},{state:'interrupted',error_code:'raw_vendor_secret'},{state:'accepted'},{coverage:'complete'},{finished_at:'2026-10-31T01:59:59Z'}])expect(readCloudCollaboration(view({latest_load:load(changed)}),source())).toBeUndefined();
 expect(readCloudCollaboration(view({last_loaded:interrupted}),source())).toBeUndefined();
});

it('preserves literal multiline text and opaque unsupported positions without interpreting links or code',()=>{
 const text='<script>literal</script>\nhttps://example.invalid/private\t你好\r';
 const result=page({load:load({entry_count:2,unsupported_count:1,text_bytes:new TextEncoder().encode(text).length}),items:[{sequence:1,kind:'user_text',text},{sequence:2,kind:'unsupported'}],total:2,next_after:2});
 expect(readCloudHistoryPage(result,source(),'load-a')?.items).toEqual(result.items);
 expect(readCloudHistoryPage({...result,items:[result.items[0],{sequence:2,kind:'unsupported',text:''}]},source(),'load-a')).toBeUndefined();
});

it('rejects unprojected fields, unsafe Unicode and text beyond the per-entry byte bound',()=>{
 for(const entry of [{sequence:1,kind:'assistant_text',text:'\ud800'},{sequence:1,kind:'tool_summary',text:'bad\u0000value'},{sequence:1,kind:'assistant_text',text:'x'.repeat(65537)},{sequence:1,kind:'thinking',text:'hidden'},{sequence:1,kind:'tool_summary',text:'title',arguments:{command:'hidden'}}])expect(readCloudHistoryPage(page({items:[entry]}),source(),'load-a')).toBeUndefined();
 const text='界'.repeat(21845)+'x';
 expect(new TextEncoder().encode(text)).toHaveLength(65536);
 expect(readCloudHistoryPage(page({load:load({text_bytes:65536}),items:[{sequence:1,kind:'assistant_text',text}]}),source(),'load-a')).toBeDefined();
});

it('paginates the platform dense local sequence with a bounded default and an explicit end',()=>{
 const items=Array.from({length:50},(_,index)=>({sequence:index+1,kind:'assistant_text',text:'x'}));
 const first=page({load:load({entry_count:51,text_bytes:51}),items,total:51,next_after:50,has_more:true});
 expect(readCloudHistoryPage(first,source(),'load-a')).toBeDefined();
 expect(readCloudHistoryPage({...first,items:[{sequence:51,kind:'assistant_text',text:'x'}],next_after:51,has_more:false},source(),'load-a',50)).toBeDefined();
 expect(readCloudHistoryPage({...first,items:[],next_after:60,has_more:false},source(),'load-a',60)).toBeDefined();
 expect(historyPagePath('load-a','project-a',50,1)).toContain('&after=50&limit=1');
});

it('refuses gaps, false continuation, wrong totals and oversized pages rather than silently skipping history',()=>{
 const original=page({load:load({entry_count:3,text_bytes:3}),items:[{sequence:1,kind:'assistant_text',text:'x'},{sequence:2,kind:'assistant_text',text:'x'}],total:3,next_after:2,has_more:true});
 for(const changed of [{items:[original.items[0],{sequence:3,kind:'assistant_text',text:'x'}]},{next_after:1},{has_more:false},{total:2},{items:[]}])expect(readCloudHistoryPage({...original,...changed},source(),'load-a',0,2)).toBeUndefined();
 expect(readCloudHistoryPage(original,source(),'load-a',1,2)).toBeUndefined();
 expect(readCloudHistoryPage(original,source(),'load-a',0,1)).toBeUndefined();
 expect(()=>historyPagePath('load-a','project-a',0,101)).toThrow();
});

it('binds archive pages to the requested load and stable source rather than trusting a load locator alone',()=>{
 expect(readCloudHistoryPage(page(),source(),'other-load')).toBeUndefined();
 expect(readCloudHistoryPage(page({load:load({source:{...source(),task_id:'foreign-task'}})}),source(),'load-a')).toBeUndefined();
 expect(readCloudHistoryPage(page({load:load({source:source(4)})}),source(),'load-a')).toBeUndefined();
 expect(readCloudHistoryPage({...page(),provider_cursor:'invented'},source(),'load-a')).toBeUndefined();
});

it('accepts exact first-head and original older-observation commands without rewriting their version or key',()=>{
 const original=request();
 expect(validCloudHistoryRequest(original,source())).toBe(true);
 expect(validCloudHistoryRequest(original,source(5))).toBe(true);
 expect(validCloudHistoryRequest(original,source(2))).toBe(false);
 expect(original.body.expected_version).toBe(0);
 expect(original.body.expected_submission_version).toBe(3);
});

it('rejects alternate actions, scope, secret or prompt fields and mismatched original operation keys before sending',()=>{
 const original=request();
 for(const changed of [{...original,path:'/workbuddy-cloud-submissions/other/history-loads'},{...original,method:'PATCH'},{...original,body:{...original.body,project_id:'other'}},{...original,body:{...original.body,prompt:'second turn'}},{...original,body:{...original.body,ticket:'secret'}},{...original,body:{...original.body,idempotency_key:'different-operation'}},{...original,body:{...original.body,expected_version:-1}},{...original,body:{...original.body,expected_version:2147483647}},{...original,body:{...original.body,expected_submission_version:null}}])expect(validCloudHistoryRequest(changed,source())).toBe(false);
});

it('confirms only the original operation and original head precondition while keeping load.version independent',()=>{
 expect(readCloudHistoryReceipt(load(),request(),source(5))).toBeDefined();
 expect(readCloudHistoryReceipt(load({version:1,head_version_at_start:4}),request(4),source())).toBeDefined();
 for(const changed of [{operation_id:'other-history-operation'},{head_version_at_start:1},{source:source(4)},{source:{...source(),request_sha256:'c'.repeat(64)}}])expect(readCloudHistoryReceipt(load(changed),request(),source(5))).toBeUndefined();
});

it('accepts acknowledged loading and rejected results without inventing history completion or overwriting an old loaded snapshot',()=>{
 const pending=load({state:'loading'});delete pending.finished_at;
 expect(readCloudHistoryReceipt(pending,request(),source())).toBeDefined();
 const rejected=load({state:'rejected',error_code:'workbuddy_cloud_history_rejected',entry_count:0,text_bytes:0});
 expect(readCloudHistoryReceipt(rejected,request(),source())).toBeDefined();
 const original=view({latest_load:rejected});
 expect(readCloudCollaboration(original,source())?.last_loaded).toEqual(load());
 expect(original.latest_load?.state).toBe('rejected');
});
