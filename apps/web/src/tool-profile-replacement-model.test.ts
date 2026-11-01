import {expect,it} from 'vitest';
import type {ToolProfile,ToolProfileReplacement} from '../../../packages/contracts/src';
import {buildToolProfileReplacementBody,readToolProfileReplacement,readToolProfileReplacementList,toolProfileReplacementPath,validToolProfileReplacementBody,validToolProfileReplacementReceipt,validToolProfileReplacementRequest} from './tool-profile-replacement-model';

const original:ToolProfile={id:'source-a',project_id:'project-one',worker_id:'worker-a',name:'Original Codex',version:4,state:'enabled',profile:{command_id:'codex-original',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0.0',read_only:false},enabled_by:'owner',available:false,reason:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z'};
const profile={...original.profile,command_id:'codex-next',profile_sha256:'c'.repeat(64)};
const target:ToolProfile={...original,id:'target-b',worker_id:'worker-b',name:'New Codex',version:1,state:'paused',profile,enabled_by:''};
const relation=(pause=true):ToolProfileReplacement=>({id:'handoff-1',project_id:original.project_id,original:pause?{...original,state:'paused',version:5,enabled_by:''}:original,replacement:target,disposition:'created',pause_original:pause,created_at:'2026-11-02T00:00:00.123456789Z',created_by:'owner'});
const request=(pause=true)=>({path:toolProfileReplacementPath(original.id),method:'POST' as const,idempotency_key:'original-operation-200',body:{project_id:original.project_id,expected_version:4,worker_id:target.worker_id,name:target.name,profile,pause_original:pause}});

it('T200 binds explicit configuration replacement to its original immutable receipt without granting activation',()=>{
 const pending=request(),draft={original,workerID:target.worker_id,profile,name:target.name,pauseOriginal:true};
 expect(buildToolProfileReplacementBody(draft)).toEqual(pending.body);
 expect(validToolProfileReplacementRequest(pending)).toBe(true);
 expect(validToolProfileReplacementReceipt(relation(),pending,'owner')).toBe(true);
 expect(readToolProfileReplacement(relation(),{project:original.project_id,originalID:original.id,replacementID:'handoff-1'})).toEqual(relation());
 expect(validToolProfileReplacementReceipt(relation(false),request(false),'owner')).toBe(true);
 const alreadyPaused={...original,state:'paused' as const,enabled_by:''};
 expect(validToolProfileReplacementReceipt({...relation(),original:alreadyPaused},pending,'owner')).toBe(true);
 const reused={...target,version:9,state:'enabled' as const,enabled_by:'earlier-approver'};
 const reuseDraft={original,workerID:target.worker_id,profile,reuse:reused,pauseOriginal:false};
 const reuseBody=buildToolProfileReplacementBody(reuseDraft);
 expect(reuseBody).toEqual({project_id:original.project_id,expected_version:4,worker_id:target.worker_id,profile,pause_original:false,reuse:{profile_id:reused.id,expected_version:9}});
 expect(reuseBody).not.toHaveProperty('name');
 const reusedRelation={...relation(false),disposition:'reused' as const,replacement:reused};
 const reusePending={...request(false),body:reuseBody};
 expect(validToolProfileReplacementReceipt(reusedRelation,reusePending,'owner')).toBe(true);
 expect(validToolProfileReplacementReceipt({...reusedRelation,replacement:{...reused,version:10}},reusePending,'owner')).toBe(false);
 expect(validToolProfileReplacementReceipt({...relation(false),original:{...original,version:5}},request(false),'owner')).toBe(false);
 expect(validToolProfileReplacementRequest({...pending,body:{...pending.body,expected_version:2147483648}})).toBe(false);
 expect(validToolProfileReplacementRequest({...reusePending,body:{...reuseBody,reuse:{profile_id:reused.id,expected_version:2147483648}}})).toBe(false);
 expect(()=>readToolProfileReplacement({...relation(false),original:{...original,version:2147483648}},{project:original.project_id})).toThrow();
 for(const bad of [{...pending,method:'PATCH'},{...pending,path:'/tool-profiles/other/commands'},{...pending,idempotency_key:'short'},{...pending,body:{...pending.body,pause_original:undefined}},{...pending,body:{...pending.body,pause_original:'false'}},{...pending,body:{...pending.body,name:'x'.repeat(201)}},{...pending,body:{...pending.body,name:'私'.repeat(67)}},{...pending,body:{...pending.body,profile:{...profile,api_key:'private'}}},{...reusePending,body:{...reuseBody,name:''}},{...reusePending,body:{...reuseBody,reuse:null}},{...reusePending,body:{...reuseBody,reuse:{profile_id:original.id,expected_version:4}}}])expect(validToolProfileReplacementRequest(bad)).toBe(false);
 for(const bad of [{...relation(),project_id:'other'},{...relation(),original:{...relation().original,id:'other'}},{...relation(),original:{...relation().original,version:6}},{...relation(),replacement:{...target,worker_id:'other'}},{...relation(),replacement:{...target,profile:{...profile,read_only:true}}},{...relation(),replacement:{...target,state:'enabled'}},{...relation(),pause_original:false},{...relation(),created_by:'other'},{...relation(),deployment:'healthy'},{...reusedRelation,replacement:{...reused,version:10}}])expect(validToolProfileReplacementReceipt(bad,pending,'owner')).toBe(false);
 expect(()=>buildToolProfileReplacementBody({...reuseDraft,reuse:original,workerID:original.worker_id,profile:original.profile})).toThrow();
 expect(validToolProfileReplacementBody(pending.body,original.id)).toBe(true);
});

it('T200 reads only exact scoped replacement history and preserves cursor coverage and handoff snapshots',()=>{
 const item=relation(false),scope={project:original.project_id,profileID:original.id,direction:'outgoing' as const};
 const page={project_id:original.project_id,profile_id:original.id,direction:'outgoing',items:[item],next_cursor:'opaque-next-page'};
 expect(readToolProfileReplacementList(page,scope)).toEqual(page);
 expect(readToolProfileReplacementList({...page,items:[],next_cursor:'still-bounded'},scope).items).toEqual([]);
 const incoming={...page,profile_id:target.id,direction:'incoming'};
 expect(readToolProfileReplacementList(incoming,{...scope,profileID:target.id,direction:'incoming'})).toEqual(incoming);
 for(const bad of [{...page,project_id:'other'},{...page,profile_id:'other'},{...page,direction:'incoming'},{...page,total:1},{...page,items:null},{...page,items:[item,item]},{...page,next_cursor:''},{...page,next_cursor:'x'.repeat(2049)},{...page,items:[{...item,replacement:{...target,state:'revoked'}}]},{...page,items:[{...item,created_at:'2026-02-30T00:00:00Z'}]},{...page,items:[{...item,created_by:''}]},{...page,items:[{...item,original:{...original,profile:{...original.profile,args:['execute']}}}]}])expect(()=>readToolProfileReplacementList(bad,scope)).toThrow();
});
