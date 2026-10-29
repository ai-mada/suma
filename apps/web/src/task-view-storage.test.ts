import {describe, expect, it} from 'vitest';
import {discardTaskViews, readTaskViews, taskViewsKey, writeTaskViews, type SavedTaskView, type TaskViewScope, type TaskViewStorage} from './task-views-storage';

const scope:TaskViewScope=['user-a','org-a','project-a'];
const view=():SavedTaskView=>({id:'view-1',name:'支付任务',preferences:{q:'支付',status:'in_progress',owner_id:'member-a',priority:'high',sort:'updated_desc',layout:'board',group_by:'owner'}});
const encode=(views:unknown[])=>JSON.stringify({version:1,views});
function memoryStorage(initial?:string){
 const values=new Map<string,string>(initial===undefined?[]:[[taskViewsKey(scope),initial]]);
 const storage:TaskViewStorage={getItem:key=>values.get(key)??null,setItem:(key,value)=>{values.set(key,value);},removeItem:key=>{values.delete(key);}};
 return {values,storage};
}

describe('local personal task views',()=>{
 it('reads an absent collection without creating storage and saves only preferences',()=>{
  const {storage,values}=memoryStorage();
  const initial=readTaskViews(storage,scope);
  expect(initial).toEqual({key:'suma-task-views-v1:["user-a","org-a","project-a"]',raw:null,collection:{version:1,views:[]},error:null});
  expect(values.size).toBe(0);
  const written=writeTaskViews(storage,scope,[view()],initial.raw);
  expect(written.error).toBeNull();
  expect(written.collection).toEqual({version:1,views:[view()]});
  expect(readTaskViews(storage,scope)).toEqual(written);
 });

 it('isolates all three scope IDs and does not collapse delimiter-containing IDs',()=>{
  const {storage}=memoryStorage();
  writeTaskViews(storage,scope,[view()],null);
  for(const other of [['user-b','org-a','project-a'],['user-a','org-b','project-a'],['user-a','org-a','project-b']] as TaskViewScope[]){
   expect(readTaskViews(storage,other).collection?.views).toEqual([]);
  }
  expect(taskViewsKey(['u:a','b','p'])).not.toBe(taskViewsKey(['u','a:b','p']));
 });

 it('accepts bounded custom business filters and unassigned owners without translating them',()=>{
  const {storage}=memoryStorage();const custom=view();
  custom.preferences={...custom.preferences,q:'',status:'等待外部确认',priority:'紧急等级',owner_id:'__unassigned__',sort:'priority_desc',layout:'list',group_by:'priority'};
  expect(writeTaskViews(storage,scope,[custom],null).collection?.views[0]).toEqual(custom);
 });

 it('preserves a search of exactly 200 Unicode code points including astral emoji',()=>{
  const {storage}=memoryStorage();const maximum=view();maximum.preferences.q='😀'.repeat(200);
  const result=writeTaskViews(storage,scope,[maximum],null);
  expect(result.error).toBeNull();
  expect(readTaskViews(storage,scope).collection?.views[0].preferences.q).toBe('😀'.repeat(200));
 });

 it('rejects 201 Unicode code points without truncating the query or overwriting saved views',()=>{
  const original=encode([view()]);const {storage,values}=memoryStorage(original);const oversized=view();oversized.preferences.q='😀'.repeat(201);
  expect(writeTaskViews(storage,scope,[oversized],original).error).toBe('invalid');
  expect(values.get(taskViewsKey(scope))).toBe(original);
  values.set(taskViewsKey(scope),encode([oversized]));
  expect(readTaskViews(storage,scope)).toMatchObject({raw:encode([oversized]),collection:null,error:'corrupt'});
 });

 it('allows a 64-character owner ID but rejects 65 without replacing the original',()=>{
  const {storage,values}=memoryStorage();const maximum=view();maximum.preferences.owner_id='a'.repeat(64);
  const written=writeTaskViews(storage,scope,[maximum],null);expect(written.error).toBeNull();
  const oversized=view();oversized.preferences.owner_id='a'.repeat(65);
  expect(writeTaskViews(storage,scope,[oversized],written.raw).error).toBe('invalid');
  expect(values.get(taskViewsKey(scope))).toBe(written.raw);
  values.set(taskViewsKey(scope),encode([oversized]));
  expect(readTaskViews(storage,scope).error).toBe('corrupt');
 });

 it.each([
  ['invalid JSON','{'],['unsupported version','{"version":2,"views":[]}'],
  ['extra business facts',JSON.stringify({version:1,views:[],run_id:'run-1'})],
  ['duplicate property','{"version":1,"version":1,"views":[]}'],
  ['escaped duplicate property','{"version":1,"\\u0076ersion":1,"views":[]}'],
  ['missing collection','{"version":1}'],['null collection','{"version":1,"views":null}'],
  ['extra view field',encode([{...view(),task_status:'accepted'}])],
  ['extra preference field',encode([{...view(),preferences:{...view().preferences,run:'succeeded'}}])],
  ['duplicate view ID',encode([view(),view()])],
  ['empty name',encode([{...view(),name:' '}])],
  ['untrimmed name',encode([{...view(),name:' 名称 '}])],
  ['control in status',encode([{...view(),preferences:{...view().preferences,status:'todo\naccepted'}}])],
  ['control in priority',encode([{...view(),preferences:{...view().preferences,priority:'\u0085'}}])],
  ['status exceeds UTF-8 budget',encode([{...view(),preferences:{...view().preferences,status:'界'.repeat(22)}}])],
  ['priority exceeds UTF-8 budget',encode([{...view(),preferences:{...view().preferences,priority:'界'.repeat(22)}}])],
  ['untrimmed query',encode([{...view(),preferences:{...view().preferences,q:' 支付 '}}])],
  ['invalid owner',encode([{...view(),preferences:{...view().preferences,owner_id:'../member'}}])],
  ['unsupported layout',encode([{...view(),preferences:{...view().preferences,layout:'timeline'}}])],
  ['unsupported grouping',encode([{...view(),preferences:{...view().preferences,group_by:'run'}}])],
  ['unsupported sort',encode([{...view(),preferences:{...view().preferences,sort:'run_state'}}])],
 ])('preserves %s as corrupt instead of presenting an empty saved collection',(_name,raw)=>{
  const {storage,values}=memoryStorage(raw);const result=readTaskViews(storage,scope);
  expect(result).toMatchObject({raw,collection:null,error:'corrupt'});
  expect(writeTaskViews(storage,scope,[view()],raw).error).toBe('corrupt');
  expect(values.get(taskViewsKey(scope))).toBe(raw);
 });

 it('enforces count and field bounds before changing a valid existing collection',()=>{
  const original=encode([view()]);const {storage,values}=memoryStorage(original);
  const invalidLists:unknown[]=[
   Array.from({length:21},(_,index)=>({...view(),id:'view-'+index})),
   [{...view(),name:'a'.repeat(81)}],
   [{...view(),preferences:{...view().preferences,q:'a'.repeat(201)}}],
   [{...view(),preferences:{...view().preferences,owner_id:'a'.repeat(129)}}],
   [{...view(),preferences:{...view().preferences,status:'a'.repeat(65)}}],
   [{...view(),preferences:{...view().preferences,priority:'a'.repeat(65)}}],
   [{...view(),extra:undefined}],
  ];
  for(const views of invalidLists){
   expect(writeTaskViews(storage,scope,views as SavedTaskView[],original).error).toBe('invalid');
   expect(values.get(taskViewsKey(scope))).toBe(original);
  }
  const limit=Array.from({length:20},(_,index)=>({...view(),id:'view-'+index}));
  expect(writeTaskViews(storage,scope,limit,original).collection?.views).toHaveLength(20);
 });

 it('rejects more than 64 KiB including UTF-8 bytes without erasing the original',()=>{
  const raw=' '.repeat(65536)+encode([]);const {storage,values}=memoryStorage(raw);
  expect(readTaskViews(storage,scope).error).toBe('corrupt');
  expect(values.get(taskViewsKey(scope))).toBe(raw);
  // Valid data with legal JSON whitespace fits the character limit but not UTF-8 bytes.
  const large=Array.from({length:20},(_,index)=>({...view(),id:'view-'+index,name:'界'.repeat(80),preferences:{...view().preferences,q:'界'.repeat(200),status:'界'.repeat(21),priority:'界'.repeat(21),owner_id:'a'.repeat(64)}}));
  const encoded=encode(large),padded=' '.repeat(60000-encoded.length)+encoded;
  values.set(taskViewsKey(scope),padded);
  expect(new TextEncoder().encode(padded).length).toBeGreaterThan(65536);
  expect(padded.length).toBeLessThan(65536);
  expect(readTaskViews(storage,scope).error).toBe('corrupt');
 });

 it('does not overwrite or discard another tab’s newer raw value',()=>{
  const original=encode([view()]);const {storage,values}=memoryStorage(original);
  const latest=encode([{...view(),name:'另一标签页'}]);values.set(taskViewsKey(scope),latest);
  expect(writeTaskViews(storage,scope,[],original).error).toBe('conflict');
  expect(discardTaskViews(storage,scope,original).error).toBe('conflict');
  expect(values.get(taskViewsKey(scope))).toBe(latest);
  expect(writeTaskViews(storage,scope,[],null).error).toBe('conflict');
 });

 it('permits only explicit discard of the exact corrupt value and preserves other scopes',()=>{
  const {storage,values}=memoryStorage('{broken');const other:TaskViewScope=['user-b','org-a','project-a'];
  writeTaskViews(storage,other,[view()],null);
  expect(discardTaskViews(storage,scope,'{broken')).toMatchObject({raw:null,collection:{version:1,views:[]},error:null});
  expect(values.has(taskViewsKey(scope))).toBe(false);
  expect(readTaskViews(storage,other).collection?.views).toHaveLength(1);
 });

 it('reports unavailable reads, quota failures, and failed discards without clearing original data',()=>{
  const original=encode([view()]);const {storage,values}=memoryStorage(original);
  const unreadable={...storage,getItem:()=>{throw new Error('denied');}};
  expect(readTaskViews(unreadable,scope)).toMatchObject({collection:null,error:'unavailable'});
  expect(writeTaskViews(unreadable,scope,[],original).error).toBe('unavailable');
  expect(writeTaskViews({...storage,setItem:()=>{throw new Error('quota');}},scope,[],original).error).toBe('unavailable');
  expect(discardTaskViews({...storage,removeItem:()=>{throw new Error('denied');}},scope,original).error).toBe('unavailable');
  expect(readTaskViews(undefined,scope).error).toBe('unavailable');
  expect(values.get(taskViewsKey(scope))).toBe(original);
 });

 it('verifies the write and does not claim success or restore stale data after a concurrent replacement',()=>{
  const original=encode([view()]);const latest=encode([{...view(),name:'另一标签页'}]);const {storage,values}=memoryStorage(original);
  const concurrent={...storage,setItem:(key:string)=>{values.set(key,latest);}};
  expect(writeTaskViews(concurrent,scope,[],original)).toMatchObject({raw:latest,error:'conflict'});
  expect(values.get(taskViewsKey(scope))).toBe(latest);
 });

 it('keeps literal punctuation and markup-like user text as data across round trips',()=>{
  const {storage}=memoryStorage();const literal=view();
  literal.name='<img src=x>';literal.preferences.q='"nested": {"id":"x"}';
  const result=writeTaskViews(storage,scope,[literal],null);
  expect(result.error).toBeNull();
  expect(readTaskViews(storage,scope).collection?.views[0]).toEqual(literal);
 });

 it('rejects missing scope IDs instead of writing into an anonymous shared bucket',()=>{
  const {storage,values}=memoryStorage();const missing:TaskViewScope=['','org-a','project-a'];
  expect(writeTaskViews(storage,missing,[view()],null).error).toBe('unavailable');
  expect(values.size).toBe(0);
 });
});
