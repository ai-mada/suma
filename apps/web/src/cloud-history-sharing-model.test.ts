import {createHash,webcrypto} from 'node:crypto';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudHistoryEntry,CloudHistoryLoad,CloudHistorySharePreview,CloudHistorySharePreviewRequest,CloudHistoryShareRequest,CloudHistorySharedEntry,CloudHistorySharedObservation,TaskConversationMessage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {historySharePreviewPath,historySharePath,eligibleHistoryEntry,validHistoryShareSequences,validHistoryShareRequest,readHistorySharePreview,readHistoryShareReceipt} from './cloud-history-sharing-model';
import {validCloudHistorySharedObservation} from './cloud-history-shared-observation-model';

const sha=(value:string)=>createHash('sha256').update(value,'utf8').digest('hex');
const originalEntries=():CloudHistoryEntry[]=>[{sequence:1,kind:'assistant_text',text:'  first <tag>\n'},{sequence:3,kind:'tool_summary',text:'second & 中文'}];
const load=():CloudHistoryLoad=>({id:'load-a',version:6,operation_id:'private-load-operation',head_version_at_start:2,source:{project_id:'project-a',run_id:'source-run',node_id:'cloud-node',submission_id:'submission-a',submission_version:4,task_id:'vendor-private-task',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)},state:'loaded',coverage:'unverified',started_at:'2026-10-31T05:00:00Z',deadline_at:'2026-10-31T05:00:30Z',overdue:false,finished_at:'2026-10-31T05:00:00.100Z',entry_count:3,unsupported_count:1,text_bytes:new TextEncoder().encode(originalEntries().map(item=>item.text).join('')).length});
const previewRequest=():CloudHistorySharePreviewRequest=>({project_id:'project-a',task_id:'target-task',expected_task_version:4,expected_load_version:6,sequences:[1,3]});
const sharedEntries=(entries=originalEntries()):CloudHistorySharedEntry[]=>entries.map(entry=>({sequence:entry.sequence,kind:entry.kind,text_sha256:sha(entry.text||'')}));
const selectionSHA=(entries=sharedEntries())=>sha(JSON.stringify(entries.map(({sequence,kind,text_sha256})=>({sequence,kind,text_sha256}))));
const preview=(entries=originalEntries(),selectedLoad=load()):CloudHistorySharePreview=>({project_id:'project-a',task_id:'target-task',task_title:'Team handling task',task_version:4,load_id:selectedLoad.id,load_version:selectedLoad.version,load_state:selectedLoad.state,coverage:'unverified',source:selectedLoad.source,entries,body:entries.map(item=>item.text).join('\n\n'),body_sha256:sha(entries.map(item=>item.text).join('\n\n')),selection_sha256:selectionSHA(sharedEntries(entries)),preview_sha256:'c'.repeat(64),sharing_scope:'project_task_readers'});
const observation=(value=preview()):CloudHistorySharedObservation=>({load_id:value.load_id,load_version:value.load_version,load_state:value.load_state,coverage:'unverified',source_run_id:value.source.run_id,source_node_id:value.source.node_id,source_submission_id:value.source.submission_id,entries:sharedEntries(value.entries),body_sha256:value.body_sha256,selection_sha256:value.selection_sha256});
const request=():WriteRecoveryRequest=>{
 const body:CloudHistoryShareRequest={...previewRequest(),preview_sha256:'c'.repeat(64),idempotency_key:'original-project-sharing'};
 return {path:historySharePath('load-a'),method:'POST',idempotency_key:body.idempotency_key,body:{...body}};
};
const message=(value=preview()):TaskConversationMessage=>({id:'message-a',sequence:5,task_version:4,body:value.body,author_id:'user-a',author_name:'Alice',actor_kind:'human',created_at:'2026-10-31T05:01:00Z',requirement_id:'',requirement_version:0,run_id:'target-context-run',reply_id:'',reply_to_sequence:0,execution_run_id:'',cloud_observation:observation(value)});
const context={runID:'source-run',nodeID:'cloud-node'};
beforeEach(()=>vi.stubGlobal('crypto',webcrypto));
afterEach(()=>vi.unstubAllGlobals());

it('uses fixed encoded preview and share paths and refuses alternate locators',()=>{
 expect(historySharePreviewPath('load-a')).toBe('/workbuddy-cloud-history-loads/load-a/share-preview');
 expect(historySharePath('load-a')).toBe('/workbuddy-cloud-history-loads/load-a/shares');
 expect(historySharePath('load a')).toContain('/load%20a/shares');
 for(const id of ['../load','load/other','load\\other','\ud800'])expect(()=>historySharePath(id)).toThrow();
});

it('offers only nonblank supported stored entries from terminal loads with an ordered bounded selection',()=>{
 for(const state of ['loaded','interrupted','limited','rejected']){
  const terminal={...load(),state,...(state==='loaded'?{}:{error_code:'workbuddy_cloud_history_'+state})};
  expect(eligibleHistoryEntry(terminal,originalEntries()[0])).toBe(true);
 }
 const loading={...load(),state:'loading'};delete loading.finished_at;
 expect(eligibleHistoryEntry(loading,originalEntries()[0])).toBe(false);
 for(const entry of [{sequence:2,kind:'unsupported'},{sequence:1,kind:'assistant_text',text:'\n \t'},{sequence:4,kind:'user_text',text:'outside'}])expect(eligibleHistoryEntry(load(),entry)).toBe(false);
 expect(validHistoryShareSequences([1,3])).toBe(true);
 for(const sequences of [[],[3,1],[1,1],[0],[501],Array.from({length:9},(_,index)=>index+1),[1.5]])expect(validHistoryShareSequences(sequences)).toBe(false);
});

it('keeps recovery requests metadata-only and independent of the current private load or task version',()=>{
 const original=request();
 expect(validHistoryShareRequest(original,'project-a')).toBe(true);
 for(const changed of [{...original,path:'/workbuddy-cloud-history-loads/load-a/shares?project_id=project-a'},{...original,method:'PATCH'},{...original,body:{...original.body,project_id:'other'}},{...original,body:{...original.body,body:'private preview'}},{...original,body:{...original.body,author_id:'other'}},{...original,body:{...original.body,expected_load_version:0}},{...original,body:{...original.body,expected_task_version:2147483647}},{...original,body:{...original.body,preview_sha256:'bad'}},{...original,body:{...original.body,idempotency_key:'replacement-operation'}}])expect(validHistoryShareRequest(changed,'project-a')).toBe(false);
 expect(original.body.expected_task_version).toBe(4);
});

it('exposes only closed public attribution and never grants the original private session fields',()=>{
 expect(validCloudHistorySharedObservation(observation())).toBe(true);
 for(const extra of [{task_id:'vendor-private-task'},{operation_id:'private-load-operation'},{source:{...load().source}},{ticket:'private'},{entries:[{...sharedEntries()[0],text:'private extra'}]},{coverage:'complete'},{load_state:'loading'}])expect(validCloudHistorySharedObservation({...observation(),...extra})).toBe(false);
});

it('rejects malformed public sequences, kinds, hashes and source identifiers before rendering attribution',()=>{
 for(const changed of [{entries:[]},{entries:[sharedEntries()[1],sharedEntries()[0]]},{entries:[sharedEntries()[0],sharedEntries()[0]]},{entries:[{sequence:1,kind:'unsupported',text_sha256:'a'.repeat(64)}]},{entries:[{...sharedEntries()[0],text_sha256:'A'.repeat(64)}]},{source_run_id:'bad/run'},{source_node_id:'\ud800'},{load_version:null},{body_sha256:'bad'}])expect(validCloudHistorySharedObservation({...observation(),...changed})).toBe(false);
});

it('accepts the exact selected literal text, two-LF body and canonical ASCII selection hash',async()=>{
 const value=preview();
 expect(await readHistorySharePreview(value,load(),previewRequest(),originalEntries())).toEqual(value);
 expect(value.body).toBe('  first <tag>\n\n\nsecond & 中文');
 expect(value.selection_sha256).toBe(selectionSHA());
});

it('binds preview to the full original private source and exact target/load versions',async()=>{
 const original=preview();
 for(const changed of [{project_id:'other'},{task_id:'other-task'},{task_version:5},{load_id:'other-load'},{load_version:7},{load_state:'limited'},{source:{...original.source,submission_version:5}},{source:{...original.source,task_id:'other-vendor-task'}},{source:{...original.source,request_sha256:'d'.repeat(64)}},{task_title:'x'.repeat(501)},{sharing_scope:'all_users'}])expect(await readHistorySharePreview({...original,...changed},load(),previewRequest(),originalEntries())).toBeUndefined();
});

it('refuses replacement, reordered, unsupported or unprojected selected entry bodies',async()=>{
 const original=preview();
 for(const entries of [[...original.entries].reverse(),[{...original.entries[0],text:'substitute'},original.entries[1]],[{sequence:2,kind:'unsupported',text:'packet'}],[{...original.entries[0],arguments:{secret:'not public'}},original.entries[1]]])expect(await readHistorySharePreview({...original,entries},load(),previewRequest(),originalEntries())).toBeUndefined();
 expect(await readHistorySharePreview(original,load(),{...previewRequest(),sequences:[3,1]},originalEntries())).toBeUndefined();
});

it('checks raw UTF8 body and selection hashes without trimming or Unicode normalization',async()=>{
 const original=preview();
 for(const changed of [{body:original.body.trim()},{body_sha256:'d'.repeat(64)},{selection_sha256:'d'.repeat(64)},{preview_sha256:'not-a-digest'},{body:original.body.replaceAll('\n','\r\n')}])expect(await readHistorySharePreview({...original,...changed},load(),previewRequest(),originalEntries())).toBeUndefined();
 const entries=[{sequence:1,kind:'user_text',text:'e\u0301'}],snapshot={...load(),entry_count:1,unsupported_count:0,text_bytes:3},value=preview(entries,snapshot);
 expect(await readHistorySharePreview(value,snapshot,{...previewRequest(),sequences:[1]},entries)).toBeDefined();
 expect(await readHistorySharePreview({...value,body:'é',body_sha256:sha('é')},snapshot,{...previewRequest(),sequences:[1]},entries)).toBeUndefined();
});

it('applies the discussion codepoint and UTF8 bounds without truncating selected text',async()=>{
 const entries=[{sequence:1,kind:'user_text',text:'😀'.repeat(8000)}],snapshot={...load(),entry_count:1,unsupported_count:0,text_bytes:32000};
 expect(await readHistorySharePreview(preview(entries,snapshot),snapshot,{...previewRequest(),sequences:[1]},entries)).toBeDefined();
 const tooLong=[{sequence:1,kind:'user_text',text:'x'.repeat(8001)}],longSnapshot={...snapshot,text_bytes:8001};
 expect(await readHistorySharePreview(preview(tooLong,longSnapshot),longSnapshot,{...previewRequest(),sequences:[1]},tooLong)).toBeUndefined();
});

it('rejects malformed Unicode and projected control characters rather than escaping them into a quote',async()=>{
 for(const text of ['bad\ud800','bad\0value','bad\u0001value']){
  const entries=[{sequence:1,kind:'user_text',text}],snapshot={...load(),entry_count:1,unsupported_count:0,text_bytes:new TextEncoder().encode(text).length};
  expect(await readHistorySharePreview(preview(entries,snapshot),snapshot,{...previewRequest(),sequences:[1]},entries)).toBeUndefined();
 }
});

it('accepts a human shared receipt while keeping target task run distinct from original cloud run',async()=>{
 const result=await readHistoryShareReceipt(message(),request(),'user-a',context,preview());
 expect(result?.id).toBe('message-a');
 expect(result?.run_id).toBe('target-context-run');
 expect(result?.cloud_observation?.source_run_id).toBe('source-run');
});

it('can recover an original acknowledged share without rereading a now-unavailable private load or old preview',async()=>{
 expect(await readHistoryShareReceipt(message(),request(),'user-a',context)).toBeDefined();
 expect(request().body).not.toHaveProperty('body');
 expect(request().body).not.toHaveProperty('source');
});

it('rejects wrong authors, task basis, original load/selection and corrupt public body or selection digests',async()=>{
 const original=message();
 for(const changed of [{actor_kind:'api'},{author_id:'other-user'},{task_version:5},{body:original.body+' changed'},{cloud_observation:{...observation(),load_id:'other-load'}},{cloud_observation:{...observation(),load_version:7}},{cloud_observation:{...observation(),source_run_id:'other-run'}},{cloud_observation:{...observation(),source_node_id:'other-node'}},{cloud_observation:{...observation(),entries:[sharedEntries()[0]]}},{cloud_observation:{...observation(),selection_sha256:'d'.repeat(64)}}])expect(await readHistoryShareReceipt({...original,...changed},request(),'user-a',context)).toBeUndefined();
});

it('requires strong equality to a present preview but never pretends to reconstruct its opaque server hash',async()=>{
 const original=preview();
 expect(await readHistoryShareReceipt(message(),request(),'user-a',context,{...original,preview_sha256:'d'.repeat(64)})).toBeUndefined();
 expect(await readHistoryShareReceipt(message(),request(),'user-a',context,{...original,source:{...original.source,submission_id:'other-submission'}})).toBeUndefined();
 expect(await readHistoryShareReceipt(message(),request(),'user-a',context,{...original,task_id:'other-task'})).toBeUndefined();
});
