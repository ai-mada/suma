import {describe,expect,it} from 'vitest';
import type {WriteRecoveryRequest} from './write-action';
import {appendOwnerTargets,ownerTargetsPath,validOwnerPreview,validOwnerRecovery,validOwnerResult,validOwnerSelection,validOwnerTargets} from './task-owner-changes-model';

const target={resource_id:'human-b',resource_version:4,user_id:'user-b',name:'Ben'};
const tasks=[{task_id:'task-a',expected_version:3},{task_id:'task-b',expected_version:5}];
const request={project_id:'project-a',owner_id:target.resource_id,expected_owner_version:4,tasks};
const preview={project_id:'project-a',owner:target,tasks:[{task_id:'task-a',version:3,title:'A',status:'accepted',owner_id:'human-a',owner_name:'Ann',run_id:'run-a',will_change:true},{task_id:'task-b',version:5,title:'B',status:'accepted',owner_id:'human-b',owner_name:'Ben',run_id:'run-b',will_change:false}],changed_count:1,unchanged_count:1,preview_sha256:'a'.repeat(64),can_apply:true};
const recovery:WriteRecoveryRequest={path:'/task-owner-changes',method:'POST',idempotency_key:'operation-a',body:{...request,preview_sha256:preview.preview_sha256,acknowledge_invalidation:true,idempotency_key:'operation-a'}};
const result={project_id:'project-a',operation_id:'operation-a',changes:[{task_id:'task-a',previous_version:3,version:4,previous_owner_id:'human-a',owner_id:'human-b',status:'in_progress',owner_changed:true},{task_id:'task-b',previous_version:5,version:6,previous_owner_id:'human-b',owner_id:'human-b',status:'in_progress',owner_changed:false}]};

describe('bounded owner change contracts',()=>{
 it('accepts only explicit unique ordered selections with exact integer versions',()=>{
  expect(validOwnerSelection(tasks)).toBe(true);
  for(const invalid of [[],[tasks[0],tasks[0]],[{...tasks[0],expected_version:1.5}],[{...tasks[0],expected_version:2147483648}],[{...tasks[0],extra:true}],Array.from({length:17},(_,i)=>({...tasks[0],task_id:'task-'+i}))])expect(validOwnerSelection(invalid)).toBe(false);
 });
 it('binds preview order versions canonical owner and truthful unchanged counts',()=>{
  expect(validOwnerPreview(preview,request)).toBe(true);
  for(const invalid of [{...preview,tasks:[...preview.tasks].reverse()},{...preview,owner:{...target,resource_version:5}},{...preview,changed_count:2},{...preview,tasks:preview.tasks.map(row=>({...row,will_change:true}))},{...preview,can_apply:false},{...preview,preview_sha256:'invalid'}])expect(validOwnerPreview(invalid,request)).toBe(false);
  const noop={...preview,tasks:preview.tasks.map(row=>({...row,owner_id:'human-b',will_change:false})),changed_count:0,unchanged_count:2,can_apply:false};
  expect(validOwnerPreview(noop,request)).toBe(true);
 });
 it('rejects altered recovery envelopes and missing invalidation acknowledgment',()=>{
  expect(validOwnerRecovery(recovery,'project-a')).toBe(true);
  for(const invalid of [{...recovery,path:'/tasks'},{...recovery,method:'PATCH'},{...recovery,extra:true},{...recovery,body:null},{...recovery,body:{...recovery.body,project_id:'project-b'}},{...recovery,body:{...recovery.body,acknowledge_invalidation:false}},{...recovery,body:{...recovery.body,extra:true}},{...recovery,body:{...recovery.body,idempotency_key:'other-key'}}])expect(validOwnerRecovery(invalid as WriteRecoveryRequest,'project-a')).toBe(false);
 });
 it('binds receipt identity and ordered tasks while allowing real downstream version changes on unchanged owners',()=>{
  expect(validOwnerResult(result,recovery,'project-a')).toBe(true);
  for(const invalid of [{...result,operation_id:'other-operation'},{...result,changes:[...result.changes].reverse()},{...result,changes:result.changes.map(row=>({...row,previous_version:1}))},{...result,changes:result.changes.map(row=>({...row,owner_id:'human-c'}))},{...result,changes:result.changes.map(row=>({...row,version:row.previous_version-1}))}])expect(validOwnerResult(invalid,recovery,'project-a')).toBe(false);
 });
 it('keeps literal directory search and rejects scope duplicate-user and cursor corruption',()=>{
  const page={project_id:'project-a',items:[target],total:2,limit:20,next_cursor:'opaque+cursor'};
  expect(new URLSearchParams(ownerTargetsPath('project-a',' 50%_中文 + ','opaque+cursor').split('?')[1]).get('q')).toBe(' 50%_中文 + ');
  expect(validOwnerTargets(page,'project-a')).toBe(true);
  for(const invalid of [{...page,project_id:'project-b'},{...page,total:0},{...page,items:[target,{...target,resource_id:'duplicate-human'}]},{...page,next_cursor:'bad\n'}])expect(validOwnerTargets(invalid,'project-a')).toBe(false);
  const next={...page,items:[{...target,resource_id:'human-c',user_id:'user-c'}],next_cursor:''};
  expect(appendOwnerTargets(page,next)?.items).toHaveLength(2);
  expect(appendOwnerTargets(page,{...next,items:[target]})).toBeUndefined();
 });
});
