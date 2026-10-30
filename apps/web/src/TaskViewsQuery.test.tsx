// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import {ApiError,type ApiCaller,type RecordItem} from './api';
import {defaultTaskView,useTaskWorkspacePage,type TaskWorkspacePage} from './task-views-model';
import type {TaskViewPreferences,TaskViewScope} from './task-views-storage';

type PendingRead={path:string;body:unknown;method?:string;signal?:AbortSignal;resolve:(value:TaskWorkspacePage)=>void;reject:(error:unknown)=>void};
type HarnessProps={scope:TaskViewScope;view:TaskViewPreferences;refresh:number};
const originalScope:TaskViewScope=['user-a','org-a','project-a'];
const row=(id:string,project='project-a'):RecordItem=>({id,project_id:project,version:1,title:'Task '+id,status:'todo',owner_id:'member-a',priority:'high'});
const response=(ids:string[],total=ids.length,cursor='',project='project-a'):TaskWorkspacePage=>({items:ids.map(id=>row(id,project)),task_plans:ids.map(task_id=>({task_id,iteration_id:'',plan_version:0})),total,next_cursor:cursor,as_of:'2026-10-29T11:00:00Z'});
let host:HTMLDivElement,root:Root,pending:PendingRead[],api:ApiCaller,latest:ReturnType<typeof useTaskWorkspacePage>;

function Harness({scope,view,refresh}:HarnessProps){
 latest=useTaskWorkspacePage(api,scope,view,refresh);
 return <output>{latest.items?.map(item=>item.id).join(',')||''}</output>;
}
async function render(view:TaskViewPreferences={...defaultTaskView},scope:TaskViewScope=originalScope,refresh=0){
 await act(async()=>root.render(<Harness scope={scope} view={view} refresh={refresh}/>));
}
async function advance(){await act(async()=>{await vi.advanceTimersByTimeAsync(200);});}
async function resolve(index:number,value:TaskWorkspacePage){await act(async()=>{pending[index].resolve(value);});}
async function reject(index:number,error:unknown){await act(async()=>{pending[index].reject(error);});}
async function more(){await act(async()=>{void latest.more();});}
function parameters(index:number){
 const url=new URL(pending[index].path,'https://suma.invalid');
 expect(url.pathname).toBe('/task-workspace');
 return Object.fromEntries(url.searchParams);
}

beforeEach(()=>{
 vi.useFakeTimers();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);pending=[];
 // Deliberately do not honor abort: stale responses must also fail the hook's scope guard.
 api=<T,>(path:string,body?:unknown,method?:string,_headers?:Record<string,string>,signal?:AbortSignal)=>new Promise<T>((accept,decline)=>{
  pending.push({path,body,method,signal,resolve:value=>accept(value as T),reject:decline});
 });
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.clearAllTimers();vi.useRealTimers();vi.unstubAllGlobals();});

describe('authorized task workspace query window',()=>{
 it('sends every filter and opaque cursor unchanged, keeping the server filtered total',async()=>{
  const view:TaskViewPreferences={...defaultTaskView,q:'  支付 #1 & API  ',status:'all',owner_id:'member-a',priority:'交付优先',sort:'priority_desc'};
  await render(view);await advance();
  expect(parameters(0)).toEqual({project_id:'project-a',sort:'priority_desc',limit:'100',q:'  支付 #1 & API  ',status:'all',owner_id:'member-a',priority:'交付优先'});
  expect(pending[0]).toMatchObject({method:'GET',body:undefined});
  await resolve(0,response(['one','two'],3,'cursor/+&next'));
  expect(latest.total).toBe(3);expect(latest.items).toHaveLength(2);
  await more();
  expect(parameters(1)).toEqual({project_id:'project-a',sort:'priority_desc',limit:'100',q:'  支付 #1 & API  ',status:'all',owner_id:'member-a',priority:'交付优先',cursor:'cursor/+&next'});
  await resolve(1,response(['three'],3));
  expect(latest.items?.map(item=>item.id)).toEqual(['one','two','three']);
  expect(latest.total).toBe(3);expect(latest.next_cursor).toBe('');
 });

 it('hides the old window immediately and ignores an old page arriving after a query change',async()=>{
  await render();await advance();await resolve(0,response(['old-first'],5,'old-cursor'));
  await more();const oldRead=pending[1];
  await render({...defaultTaskView,q:'new filter',unassigned:true});
  expect(latest.items).toBeUndefined();expect(latest.total).toBe(0);expect(host.textContent).toBe('');
  expect(oldRead.signal?.aborted).toBe(true);
  await advance();expect(parameters(2)).toEqual({project_id:'project-a',sort:'updated_desc',limit:'100',q:'new filter',unassigned:'true'});
  await resolve(2,response(['new-first'],1));await resolve(1,response(['old-late'],5));
  expect(latest.items?.map(item=>item.id)).toEqual(['new-first']);expect(latest.total).toBe(1);expect(latest.error).toBeNull();
 });

 it.each([
  ['account',['user-b','org-a','project-a']],
  ['organization',['user-a','org-b','project-a']],
  ['project',['user-a','org-a','project-b']],
 ] as const)('isolates a new %s scope even when the old transport ignores cancellation',async(_label,nextScope)=>{
  await render();await advance();const oldRead=pending[0];
  await render({...defaultTaskView},nextScope);expect(latest.items).toBeUndefined();
  expect(oldRead.signal?.aborted).toBe(true);
  await advance();await resolve(1,response(['new-scope'],1,'',nextScope[2]));
  await resolve(0,response(['foreign-late'],99));
  expect(latest.items?.map(item=>item.id)).toEqual(['new-scope']);expect(latest.total).toBe(1);
  expect(parameters(1).project_id).toBe(nextScope[2]);
 });

 it.each([401,403])('clears previously loaded tasks and cursors after HTTP %i',async status=>{
  await render();await advance();await resolve(0,response(['private-task'],12,'next-private'));
  await more();const error=new ApiError(status,'forbidden','Current access is gone');
  await reject(1,error);
  expect(latest.items).toBeUndefined();expect(latest.total).toBe(0);expect(latest.next_cursor).toBe('');
  expect(latest.as_of).toBe('');expect(latest.loading).toBe(false);expect(latest.error).toBe(error);expect(host.textContent).toBe('');
  await more();expect(pending).toHaveLength(2);
 });

 it.each([401,403])('treats a late HTTP %i from an older query as a denial for the same authority',async status=>{
  await render({...defaultTaskView,q:'old query'});await advance();
  await render({...defaultTaskView,q:'current query'});await advance();await resolve(1,response(['protected-current'],3,'next-current'));
  await more();const currentRequest=pending[2],error=new ApiError(status,'forbidden','Access revoked for this project');
  await reject(0,error);
  expect(currentRequest.signal?.aborted).toBe(true);expect(latest.items).toBeUndefined();expect(latest.error).toBe(error);
  expect(latest.total).toBe(0);expect(latest.next_cursor).toBe('');expect(latest.action).toBe('first');expect(latest.loading).toBe(false);
  await resolve(2,response(['late-current'],3));await advance();
  expect(latest.items).toBeUndefined();expect(latest.error).toBe(error);expect(pending).toHaveLength(3);
 });

 it.each([
  ['account',['user-b','org-a','project-a']],
  ['organization',['user-a','org-b','project-a']],
  ['project',['user-a','org-a','project-b']],
 ] as const)('ignores an old %s scope denial and its retained invalidation callback',async(_label,nextScope)=>{
  await render();await advance();const oldInvalidate=latest.invalidate;
  await render({...defaultTaskView},nextScope);await advance();await resolve(1,response(['authorized-current'],1,'',nextScope[2]));
  const oldError=new ApiError(403,'forbidden','Old scope denied');
  await reject(0,oldError);await act(async()=>oldInvalidate(oldError));
  expect(latest.items?.map(item=>item.id)).toEqual(['authorized-current']);expect(latest.error).toBeNull();
  expect(latest.loading).toBe(false);expect(pending).toHaveLength(2);
 });

 it.each(['','debounced search'])('does not start a planned query after invalidation (%s)',async q=>{
  await render({...defaultTaskView,q});expect(pending).toHaveLength(0);
  const error=new ApiError(403,'forbidden','Side read denied');
  await act(async()=>latest.invalidate(error));await advance();
  expect(pending).toHaveLength(0);expect(latest.items).toBeUndefined();expect(latest.error).toBe(error);expect(latest.loading).toBe(false);
  await act(async()=>{void latest.reload();});expect(pending).toHaveLength(1);
  await resolve(0,response(['explicitly-reloaded'],1));expect(latest.items?.map(item=>item.id)).toEqual(['explicitly-reloaded']);
 });

 it('uses the current query key when the same authority retains an older invalidation callback',async()=>{
  await render();await advance();await resolve(0,response(['original'],1));const oldInvalidate=latest.invalidate;
  await render({...defaultTaskView,q:'new query'});await advance();await resolve(1,response(['new-result'],1));
  const error=new ApiError(403,'forbidden','Current project access changed');
  await act(async()=>oldInvalidate(error));
  expect(latest.items).toBeUndefined();expect(latest.error).toBe(error);expect(latest.loading).toBe(false);
  await advance();expect(pending).toHaveLength(2);
 });

 it('changes layout and grouping without re-fetching or reordering the current membership',async()=>{
  await render();await advance();await resolve(0,response(['second','first'],19,'next'));
  await render({...defaultTaskView,layout:'board',group_by:'owner'});await advance();
  await render({...defaultTaskView,layout:'list',group_by:'priority'});await advance();
  expect(pending).toHaveLength(1);expect(latest.items?.map(item=>item.id)).toEqual(['second','first']);expect(latest.total).toBe(19);
 });

 it('keeps the current window on a failed more request and retries the original cursor',async()=>{
  const view:TaskViewPreferences={...defaultTaskView,status:'blocked',sort:'created_desc'};
  await render(view);await advance();await resolve(0,response(['existing'],2,'retry/+cursor'));
  await more();const failedPath=pending[1].path,error=new ApiError(503,'unavailable','Temporary read failure');
  await reject(1,error);
  expect(latest.items?.map(item=>item.id)).toEqual(['existing']);expect(latest.total).toBe(2);
  expect(latest.next_cursor).toBe('retry/+cursor');expect(latest.action).toBe('more');expect(latest.error).toBe(error);
  await more();expect(pending[2].path).toBe(failedPath);
  await resolve(2,response(['last'],2));
  expect(latest.items?.map(item=>item.id)).toEqual(['existing','last']);expect(latest.error).toBeNull();expect(latest.next_cursor).toBe('');
 });

 it('clears an inconsistent continuation after the matching total shrinks and waits for an explicit first-page reload',async()=>{
  await render();await advance();await resolve(0,response(Array.from({length:100},(_,index)=>'old-'+index),125,'next-100'));
  await more();await resolve(1,response(['only-current'],1));
  expect(latest.items).toBeUndefined();expect(latest.total).toBe(0);expect(latest.next_cursor).toBe('');
  expect(latest.action).toBe('first');expect(latest.error).toMatchObject({ui_key:'task-views.window_changed'});expect(latest.loading).toBe(false);
  expect(host.textContent).toBe('');await advance();await more();expect(pending).toHaveLength(2);
  await act(async()=>{void latest.reload();});
  expect(parameters(2)).toEqual({project_id:'project-a',sort:'updated_desc',limit:'100'});
  await resolve(2,response(['only-current'],1));
  expect(latest.items?.map(item=>item.id)).toEqual(['only-current']);expect(latest.total).toBe(1);expect(latest.error).toBeNull();
 });

 it.each([
  ['unchanged cursor',['second'],'next'],
  ['no new task IDs',['first'],'different-next'],
  ['premature empty end',[],''],
 ] as const)('rejects a continuation with %s instead of leaving a non-progressing window',async(_label,ids,cursor)=>{
  await render();await advance();await resolve(0,response(['first'],5,'next'));
  await more();await resolve(1,response([...ids],5,cursor));
  expect(latest.items).toBeUndefined();expect(latest.next_cursor).toBe('');expect(latest.action).toBe('first');
  expect(latest.error).toMatchObject({ui_key:'task-views.window_changed'});
  await advance();await more();expect(pending).toHaveLength(2);
 });

 it('restarts from the first page on refresh and rejects an older refresh response',async()=>{
  await render();await advance();await resolve(0,response(['initial'],4,'initial-next'));
  await render({...defaultTaskView},originalScope,1);await advance();const older=pending[1];
  await render({...defaultTaskView},originalScope,2);await advance();
  expect(older.signal?.aborted).toBe(true);expect(parameters(2)).not.toHaveProperty('cursor');
  await resolve(2,response(['fresh'],1));await resolve(1,response(['stale-refresh'],8));
  expect(latest.items?.map(item=>item.id)).toEqual(['fresh']);expect(latest.total).toBe(1);
 });
});


it('invalidates iteration membership immediately and ignores a late response from the prior exact scope',async()=>{
 await render({...defaultTaskView,iteration_id:'iteration-a',group_by:'iteration'});await advance();expect(parameters(0).iteration_id).toBe('iteration-a');
 await render({...defaultTaskView,iteration_unset:true,group_by:'iteration'});expect(latest.items).toBeUndefined();expect(latest.task_plans).toEqual([]);expect(pending[0].signal?.aborted).toBe(true);await advance();expect(parameters(1).iteration_unset).toBe('true');expect(parameters(1)).not.toHaveProperty('iteration_id');
 await resolve(1,response(['current-unlinked']));await resolve(0,{...response(['old-iteration']),task_plans:[{task_id:'old-iteration',iteration_id:'iteration-a',plan_version:3}]});expect(latest.items?.map(row=>row.id)).toEqual(['current-unlinked']);
});

it('rejects missing or contradictory formal planning proof and prevents a conflicting continuation',async()=>{
 await render({...defaultTaskView,iteration_id:'iteration-a'});await advance();await resolve(0,{...response(['one']),task_plans:[]});expect(latest.items).toBeUndefined();expect(latest.error).toMatchObject({ui_key:'workspace.pagination.invalid_page'});
 await act(async()=>{void latest.reload();});await resolve(1,{...response(['one'],2,'next'),task_plans:[{task_id:'one',iteration_id:'iteration-a',plan_version:3}]});await more();await resolve(2,{...response(['one','two'],2),task_plans:[{task_id:'one',iteration_id:'iteration-a',plan_version:2},{task_id:'two',iteration_id:'iteration-a',plan_version:1}]});expect(latest.items).toBeUndefined();expect(latest.task_plans).toEqual([]);expect(latest.error).toMatchObject({ui_key:'task-views.window_changed'});
});
