// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudCollaborationSource,CloudCollaborationView,CloudHistoryLoad,CloudHistoryEntry,CloudHistoryPage,CloudHistorySharedObservation} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {WorkBuddyCloudCollaboration} from './WorkBuddyCloudCollaboration';
import {CloudHistorySharedSource} from './CloudHistorySharedSource';

type Selection={load:CloudHistoryLoad;entries:CloudHistoryEntry[];sequences:number[]};
const account:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const initialSource=():CloudCollaborationSource=>({project_id:'project-a',run_id:'run-a',node_id:'node-a',submission_id:'submission-a',submission_version:1,task_id:'task-a',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)});
let root:Root|undefined,host:HTMLDivElement,source:CloudCollaborationSource,view:CloudCollaborationView,load:CloudHistoryLoad,items:CloudHistoryEntry[],calls:{path:string;method:string}[],onSelection:ReturnType<typeof vi.fn<(selection?:Selection)=>void>>;
const entry=(sequence:number,kind:CloudHistoryEntry['kind']='assistant_text',text='Saved text '+sequence):CloudHistoryEntry=>kind==='unsupported'?{sequence,kind}:{sequence,kind,text};
function observation(state:CloudHistoryLoad['state']='loaded'){
 load={id:'load-original',version:state==='loading'?1:2,operation_id:'operation-original',head_version_at_start:0,source:{...source},state,coverage:'unverified',started_at:'2026-10-31T00:00:00Z',deadline_at:'2026-10-31T00:00:30Z',overdue:false,...(state==='loading'?{}:{finished_at:'2026-10-31T00:00:05Z'}),entry_count:items.length,unsupported_count:items.filter(e=>e.kind==='unsupported').length,text_bytes:items.reduce((sum,e)=>sum+new TextEncoder().encode(e.text||'').length,0)};
 if(state!=='loading'&&state!=='loaded')load.error_code='workbuddy_cloud_history_'+state;
 view={source:{...source},version:2,latest_load:load,...(state==='loaded'?{last_loaded:load}:{}),allowed_actions:[]};
}
function transport(custom?:(path:string)=>Promise<Response|undefined>){
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const path=String(input).replace(/^\/api\/v1/,'');calls.push({path,method:options?.method||'GET'});
  const override=await custom?.(path);if(override)return override;
  if(path.includes('/collaboration?'))return Response.json(view);
  if(path.startsWith('/workbuddy-cloud-history-loads/')){const page:CloudHistoryPage={load,items,total:items.length,next_after:items.length,has_more:false};return Response.json(page);}
  return Response.json({error:{code:'missing',message:'unavailable'}},{status:404});
 }));
}
async function settle(){for(let n=0;n<5;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(shareReset=0){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={account}><WorkBuddyCloudCollaboration source={source} onShareSelection={onSelection} shareReset={shareReset}/></ApiViewProvider>);});await settle();}
const checkbox=(sequence:number)=>host.querySelector<HTMLInputElement>('[data-history-share-select="'+sequence+'"]');
async function toggle(sequence:number){const target=checkbox(sequence);expect(target,'selectable stored sequence '+sequence).not.toBeNull();expect(target!.disabled).toBe(false);await act(async()=>target!.click());await settle();}
const current=()=>onSelection.mock.calls.at(-1)?.[0];
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);sessionStorage.clear();host=document.createElement('div');document.body.append(host);source=initialSource();items=[entry(1),entry(2)];observation();calls=[];onSelection=vi.fn<(selection?:Selection)=>void>();});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});

it('selects at most eight stored entries in sequence order without OAuth load permission or writes',async()=>{
 items=Array.from({length:9},(_,n)=>entry(n+1));observation();transport();await render();
 expect(host.querySelector('[data-collaboration-action="load"]')).toBeNull();
 for(const sequence of [8,2,7,1,6,3,5,4])await toggle(sequence);
 expect(current()?.sequences).toEqual([1,2,3,4,5,6,7,8]);expect(current()?.load).toEqual(load);
 expect(current()?.entries).toEqual(items.slice(0,8));
 for(const sequence of current()!.sequences)expect(current()!.entries.find(e=>e.sequence===sequence)).toEqual(items[sequence-1]);
 expect(checkbox(9)?.disabled).toBe(true);await act(async()=>checkbox(9)!.click());await settle();expect(current()?.sequences).toHaveLength(8);
 await toggle(2);expect(checkbox(9)?.disabled).toBe(false);await toggle(9);expect(current()?.sequences).toEqual([1,3,4,5,6,7,8,9]);
 expect(calls.every(call=>call.method==='GET')).toBe(true);expect(calls.some(call=>call.path.includes('workbuddy-connections'))).toBe(false);
});

it.each(['loaded','interrupted','limited','rejected'] as const)('allows supported nonblank text from terminal %s observations only',async state=>{
 items=[entry(1,'user_text'),entry(2,'assistant_text'),entry(3,'tool_summary'),entry(4,'plan_summary'),entry(5,'unsupported'),entry(6,'assistant_text',' \n\t')];observation(state);transport();await render();
 for(const sequence of [1,2,3,4])await toggle(sequence);
 expect(current()?.sequences).toEqual([1,2,3,4]);
 for(const sequence of [5,6]){const input=checkbox(sequence);expect(input===null||input.disabled).toBe(true);}
 expect(calls.every(call=>call.method==='GET')).toBe(true);
});

it('never selects an in-progress load even when it has saved displayable text',async()=>{
 observation('loading');transport();await render();expect(host.textContent).toContain('Saved text 1');
 for(const input of host.querySelectorAll<HTMLInputElement>('[data-history-share-select]'))expect(input.disabled).toBe(true);
 expect(current()).toBeUndefined();expect(calls.every(call=>call.method==='GET')).toBe(true);
});

it('clears the selection when its last entry is unchecked',async()=>{
 transport();await render();await toggle(1);expect(current()?.sequences).toEqual([1]);await toggle(1);expect(current()).toBeUndefined();
});

it('clears the selected entries after confirmed sharing without reloading the private archive',async()=>{
 transport();await render();await toggle(1);const reads=calls.length;await render(1);
 expect(current()).toBeUndefined();expect(checkbox(1)?.checked).toBe(false);expect(calls).toHaveLength(reads);
});

it.each([401,403,404])('clears the callback and private selection after current %i refusal',async status=>{
 let deny=false;transport(async()=>deny?Response.json({error:{code:'forbidden',message:'no access'}},{status}):undefined);await render();await toggle(1);expect(current()).toBeDefined();
 deny=true;await act(async()=>host.querySelector<HTMLButtonElement>('[data-collaboration-action="read"]')!.click());await settle();
 expect(current()).toBeUndefined();expect(host.querySelector('[data-history-share-select]')).toBeNull();expect(host.textContent).not.toContain('Saved text 1');
});

it('clears selections across exact source changes and again on unmount',async()=>{
 transport();await render();await toggle(1);expect(current()?.load.source.task_id).toBe('task-a');
 source={...initialSource(),task_id:'task-b',request_sha256:'c'.repeat(64)};items=[entry(1,'assistant_text','Different source text')];observation();await render();
 expect(current()).toBeUndefined();expect(checkbox(1)?.checked).toBe(false);await toggle(1);expect(current()?.load.source.task_id).toBe('task-b');
 await act(async()=>root!.unmount());root=undefined;expect(current()).toBeUndefined();
});

it('renders only public attribution metadata without querying the private load',async()=>{
 const publicSource:CloudHistorySharedObservation={load_id:'load-public',load_version:2,load_state:'limited',coverage:'unverified',source_run_id:'run-public',source_node_id:'node-public',source_submission_id:'submission-public',entries:[{sequence:2,kind:'assistant_text',text_sha256:'a'.repeat(64)}],body_sha256:'b'.repeat(64),selection_sha256:'c'.repeat(64)};
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);await act(async()=>{root=createRoot(host);root.render(<CloudHistorySharedSource source={publicSource}/>);});await settle();
 expect(host.querySelector('[data-cloud-history-shared-source]')).not.toBeNull();expect(host.textContent).toContain('load-public');expect(host.textContent).toContain('run-public');expect(host.textContent).toContain(publicSource.body_sha256);expect(host.querySelector('a')).toBeNull();expect(fetch).not.toHaveBeenCalled();
});

it('retains selected page-two text across first-page refresh and clears retained entries on share reset',async()=>{
 items=Array.from({length:51},(_,n)=>entry(n+1));observation();
 transport(async path=>{
  if(!path.startsWith('/workbuddy-cloud-history-loads/'))return;
  const after=Number(new URL(path,'http://fixture.test').searchParams.get('after')||0),pageItems=items.slice(after,after+50);
  const page:CloudHistoryPage={load,items:pageItems,total:items.length,next_after:after+pageItems.length,has_more:after+pageItems.length<items.length};
  return Response.json(page);
 });
 await render();expect(checkbox(51)).toBeNull();
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-collaboration-action="more"]')!.click());await settle();
 await toggle(51);expect(current()?.entries).toEqual([items[50]]);
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-collaboration-action="read"]')!.click());await settle();
 expect(checkbox(51)).toBeNull();expect(current()?.sequences).toEqual([51]);
 await toggle(2);
 expect(current()?.sequences).toEqual([2,51]);expect(current()?.entries).toEqual([items[1],items[50]]);
 expect(current()?.load.id).toBe(load.id);expect(current()?.load.version).toBe(load.version);
 await render(1);expect(current()).toBeUndefined();expect(checkbox(2)?.checked).toBe(false);
 await toggle(2);expect(current()?.sequences).toEqual([2]);expect(current()?.entries).toEqual([items[1]]);
 expect(calls.every(call=>call.method==='GET')).toBe(true);
});
