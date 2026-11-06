// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {ExecutionWorkspace} from './ExecutionWorkspace';
import {WorkBuddyProgressWorkspace} from './WorkBuddyProgressWorkspace';
import {setLocale} from './i18n';
import {clearWorkBuddyProgressScope,parseWorkbenchRoute} from './navigation';
import {taskDeliveryNavigateEvent} from './task-delivery-navigation-model';

const project='project-a';
const session:Session={
 user:{id:'user-a',name:'Viewer',email:'viewer@example.test'},
 organization:{id:'org-a',name:'Team'},
 projects:[{id:project,title:'Project',version:1}],
};
function known(id:string,reason?:string){
 return {project_id:project,run_id:id,node_id:'workbuddy-cloud',run_version:2,run_state:'running',
  node_state:'waiting',attempt:2,submission_state:'task_known',provider_status:'completed',
  observed_at:'2026-11-06T06:00:00Z',observation_failed:false,...(reason?{node_reason:reason}:{})};
}
function page(items:unknown[],next?:string,filter?:'attention',taskLinks?:unknown){
 return Response.json({project_id:project,items,...(next?{next_cursor:next}:{}),...(filter?{filter}:{}),
  ...(taskLinks!==undefined?{task_links:taskLinks}:{})});
}
const taskLink=(run:string,id:string,relation:string,title?:string)=>({run_id:run,node_id:'workbuddy-cloud',task_id:id,
 task_version:4,run_relation:relation,...(title!==undefined?{task_title:title}:{})});

let root:Root,host:HTMLDivElement;
let requests:{url:URL;init?:RequestInit}[];
let reply:(url:URL,init?:RequestInit)=>Promise<Response>;
const onRun=vi.fn();
const layoutChanged=vi.fn();
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 onRun.mockClear();
 layoutChanged.mockClear();
 requests=[];
 host=document.createElement('div');
 document.body.append(host);
 root=createRoot(host);
 reply=async()=>page([]);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href);
  requests.push({url,init});
  return reply(url,init);
 }));
});
afterEach(async()=>{
 await act(async()=>root.unmount());
 host.remove();
 vi.unstubAllGlobals();
 await setLocale('zh-CN',{persist:false});
});
async function settle(){
 for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
}
async function render(identity=session,filter?:'attention',invalidFilter=false){
 function show(next?:'attention',invalid=false){
  root.render(<ApiViewProvider session={identity}><WorkBuddyProgressWorkspace project={project} session={identity} refresh={0} onRun={onRun}
   filter={next} invalidFilter={invalid} onFilter={value=>show(value)}/></ApiViewProvider>);
 }
 await act(async()=>show(filter,invalidFilter));
 await settle();
}
async function click(selector:string){
 const target=host.querySelector<HTMLElement>(selector);
 expect(target,selector).not.toBeNull();
 await act(async()=>target!.click());
 await settle();
}
const ids=()=>[...host.querySelectorAll('[data-workbuddy-progress-row]')].map(row=>row.getAttribute('data-workbuddy-progress-row'));

it('reads only on entry, keeps platform and provider facts separate, and opens the exact run attempt',async()=>{
 await act(async()=>root.render(<ApiViewProvider session={session}>
  <ExecutionWorkspace project={project} session={session} panel="" refresh={0} layout="list" onLayout={layoutChanged} onRun={onRun} onPlan={()=>{}} onTasks={()=>{}}>Runs</ExecutionWorkspace>
 </ApiViewProvider>));
 await click('[data-workbuddy-progress-tab]');
 expect(layoutChanged).toHaveBeenCalledWith('workbuddy');
 expect(requests).toHaveLength(0);
 reply=async url=>url.searchParams.get('filter')==='attention'
  ?page([known('run-a','cloud.delivery_required')],undefined,'attention')
  :page([known('run-a','cloud.delivery_required'),known('run-b')]);
 await render();
 expect(requests).toHaveLength(1);
 const request=requests[0];
 expect(request.url.pathname).toBe('/api/v1/workbuddy-cloud-progress');
 expect(request.url.searchParams.get('project_id')).toBe(project);
 expect(request.url.searchParams.get('limit')).toBe('10');
 expect(request.init?.method).toBe('GET');
 expect(new Headers(request.init?.headers).get('X-Suma-Expected-User')).toBe('user-a');
 expect(new Headers(request.init?.headers).get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(host.textContent).toContain('2 submissions loaded');
 expect(host.textContent).toContain('Platform delivery evidence required');
 expect(host.textContent).toContain('Waiting');
 expect(host.textContent).not.toContain('Awaiting approval');
 expect(host.textContent).toContain('Completed');
 await click('[data-workbuddy-progress-filter="attention"]');
 expect(ids()).toEqual(['run-a']);
 expect(requests).toHaveLength(2);
 expect(requests[1].url.searchParams.get('filter')).toBe('attention');
 expect(requests[1].url.searchParams.has('before')).toBe(false);
 const open=host.querySelector<HTMLAnchorElement>('[data-workbuddy-progress-open]')!;
 const params=new URL(open.href).searchParams;
 expect(Object.fromEntries(params)).toEqual({project,view:'development',sub:'runs',run:'run-a',node:'workbuddy-cloud',attempt:'2'});
 await click('[data-workbuddy-progress-open]');
 expect(onRun).toHaveBeenCalledWith('run-a','workbuddy-cloud',2);
});

it('uses the server continuation and clears rows if a later page repeats a scoped item',async()=>{
 const first=Array.from({length:10},(_,i)=>known('run-'+i));
 reply=async url=>url.searchParams.has('before')
  ?page([known('run-10')],undefined,undefined,[taskLink('run-10','task-10','unbound')])
  :page(first,'cursor-1',undefined,[taskLink('run-0','task-0','current','Task from the first page')]);
 await render();
 expect(ids()).toHaveLength(10);
 await click('[data-workbuddy-progress-more]');
 expect(requests.at(-1)?.url.searchParams.get('before')).toBe('cursor-1');
 expect(ids()).toHaveLength(11);
 expect(host.querySelector('[data-workbuddy-progress-task="task-0"]')?.textContent).toBe('Task from the first page');
 expect(host.querySelector('[data-workbuddy-progress-task="task-10"]')?.textContent).toContain('task-10');
 expect(host.querySelector('[data-workbuddy-progress-more]')).toBeNull();

 reply=async url=>url.searchParams.has('before')?page([first[0]]):page(first,'cursor-1');
 await click('[data-workbuddy-progress-refresh]');
 await click('[data-workbuddy-progress-more]');
 expect(ids()).toEqual([]);
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('does not match this project or page');
});

it('opens the exact task delivery or current delivery using the page sidecar, without extra reads',async()=>{
 reply=async()=>page([known('run-a'),known('run-b'),known('run-c'),known('run-d')],undefined,undefined,[
  taskLink('run-c','task-c','unbound'),
  taskLink('run-b','task-b','changed','Moved task'),
  taskLink('run-a','task-a','current','Set up CI'),
 ]);
 await render();
 expect(ids()).toEqual(['run-a','run-b','run-c','run-d']);
 const expected=[
  ['run-a','task-a','Set up CI','current','task-delivery:run-a','Open task delivery'],
  ['run-b','task-b','Moved task','changed','task-delivery','Open current task delivery'],
  ['run-c','task-c','task-c','unbound','task-delivery','Open current task delivery'],
 ];
 for(const [run,task,title,relation,panel,label] of expected){
  const row=host.querySelector(`[data-workbuddy-progress-row="${run}"]`)!;
  expect(row.querySelector(`[data-workbuddy-progress-task="${task}"]`)?.textContent).toContain(title);
  expect(row.querySelector(`[data-workbuddy-progress-task-relation="${relation}"]`)).not.toBeNull();
  const anchor=row.querySelector<HTMLAnchorElement>('[data-workbuddy-progress-task-link]')!;
  expect(anchor.textContent).toBe(label);
  expect(Object.fromEntries(new URL(anchor.href).searchParams)).toEqual({project,view:'development',sub:'tasks',layout:'list',record:task,panel});
  expect(row.querySelector('[data-workbuddy-progress-open]')).not.toBeNull();
 }
 expect(host.querySelector('[data-workbuddy-progress-row="run-d"] [data-workbuddy-progress-task-link]')).toBeNull();
 const navigate=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener(taskDeliveryNavigateEvent,navigate,{once:true});
 await click('[data-workbuddy-progress-row="run-a"] [data-workbuddy-progress-task-link]');
 expect(navigate.mock.calls[0]?.[0]).toMatchObject({detail:expect.stringContaining('panel=task-delivery%3Arun-a')});
 expect(onRun).not.toHaveBeenCalled();
 expect(requests).toHaveLength(1);
});

it.each([
 ['foreign row',[taskLink('run-other','task-a','current')]],
 ['different node',[{...taskLink('run-a','task-a','current'),node_id:'other-node'}]],
 ['duplicate row',[taskLink('run-a','task-a','current'),taskLink('run-a','task-b','changed')]],
 ['unknown field',[{...taskLink('run-a','task-a','current'),provider_id:'private'}]],
 ['unknown relation',[taskLink('run-a','task-a','ready')]],
 ['invalid title',[taskLink('run-a','task-a','current','长'.repeat(167))]],
 ['control in title',[taskLink('run-a','task-a','current','Invalid\ntitle')]],
 ['invalid target',[taskLink('run-a','../task-a','current')]],
 ['invalid version',[{...taskLink('run-a','task-a','current'),task_version:0}]],
 ['null sidecar',null],
] as const)('clears rows and links on an invalid task sidecar: %s',async(_reason,links)=>{
 reply=async()=>page([known('run-a'),known('run-b')],undefined,undefined,links);
 await render();
 expect(ids()).toEqual([]);
 expect(host.querySelector('[data-workbuddy-progress-task-link]')).toBeNull();
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('does not match this project or page');
});

it('clears protected data on denial and ignores a late response after an identity change',async()=>{
 const original=()=>page([known('original-run')],undefined,undefined,[taskLink('original-run','task-a','current','Private task title')]);
 reply=async()=>original();
 await render();
 expect(host.textContent).toContain('Private task title');
 reply=async()=>Response.json({error:'forbidden'},{status:403});
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual([]);
 expect(host.textContent).not.toContain('Private task title');
 expect(host.querySelector('[data-workbuddy-progress-task-link]')).toBeNull();
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('Protected records were cleared');
 reply=async()=>original();
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual(['original-run']);
 let resolveOld!:(response:Response)=>void;
 reply=async()=>new Promise(resolve=>{resolveOld=resolve;});
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-workbuddy-progress-refresh]')!.click());
 const old=requests.at(-1);
 expect(host.querySelector('[data-workbuddy-progress-open]')).toBeNull();
 expect(host.querySelector('[data-workbuddy-progress-task-link]')).toBeNull();
 reply=async()=>Response.json({error:'forbidden'},{status:403});
 const next={...session,user:{...session.user,id:'viewer-b'},organization:{...session.organization,id:'org-b'}};
 await render(next);
 expect(old?.init?.signal?.aborted).toBe(true);
 await act(async()=>resolveOld(original()));
 await settle();
 expect(ids()).toEqual([]);
 expect(host.textContent).not.toContain('Private task title');
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('Protected records were cleared');
 expect(new Headers(requests.at(-1)?.init?.headers).get('X-Suma-Expected-User')).toBe('viewer-b');
});

it('keeps searching by explicit continuation after an empty filtered batch and confirms when the search ends',async()=>{
 reply=async url=>url.searchParams.has('before')?page([],undefined,'attention'):page([],'cursor-1','attention');
 await render(session,'attention');
 expect(requests).toHaveLength(1);
 expect(host.querySelector('[data-workbuddy-progress-empty-batch]')).not.toBeNull();
 expect(host.querySelector('[data-workbuddy-progress-empty-attention]')).toBeNull();
 await click('[data-workbuddy-progress-more]');
 expect(requests.at(-1)?.url.searchParams.get('filter')).toBe('attention');
 expect(requests.at(-1)?.url.searchParams.get('before')).toBe('cursor-1');
 expect(host.querySelector('[data-workbuddy-progress-more]')).toBeNull();
 expect(host.querySelector('[data-workbuddy-progress-empty-attention]')).not.toBeNull();

 reply=async url=>url.searchParams.has('before')
  ?page([known('later-run','cloud.delivery_required')],undefined,'attention')
  :page([known('short-batch','cloud.delivery_required')],'cursor-2','attention');
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual(['short-batch']);
 await click('[data-workbuddy-progress-more]');
 expect(ids()).toEqual(['short-batch','later-run']);

 reply=async()=>page([known('wrong-scope','cloud.delivery_required')]);
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual([]);
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('does not match this project or page');
});

it('keeps the filter shareable only in an exact WorkBuddy route and blocks malformed reads until cleared',async()=>{
 const url='?project=project-a&view=development&sub=runs&layout=workbuddy';
 expect(parseWorkbenchRoute(url+'&workbuddy_filter=attention').workbuddy_filter).toBe('attention');
 expect(parseWorkbenchRoute(url.replace('layout=workbuddy','layout=list')+'&workbuddy_filter=attention').workbuddy_filter).toBeUndefined();
 for(const suffix of ['workbuddy_filter=all','workbuddy_filter=attention&workbuddy_filter=attention','workbuddy_filter=attention&run=run-a','workbuddy_filter=attention&project=project-b']){
  const parsed=parseWorkbenchRoute(url+'&'+suffix);
  expect(parsed.workbuddy_filter).toBeUndefined();
  expect(parsed.workbuddy_filter_invalid).toBe('1');
  expect(clearWorkBuddyProgressScope(parsed).workbuddy_filter_invalid).toBeUndefined();
 }
 await render(session,undefined,true);
 expect(requests).toHaveLength(0);
 expect(host.querySelector('[data-workbuddy-progress-invalid-filter]')).not.toBeNull();
 await click('[data-workbuddy-progress-filter="all"]');
 expect(requests).toHaveLength(1);
 expect(requests[0].url.searchParams.has('filter')).toBe(false);
});
