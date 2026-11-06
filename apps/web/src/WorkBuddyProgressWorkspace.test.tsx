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
function page(items:unknown[],next?:string,filter?:'attention'){
 return Response.json({project_id:project,items,...(next?{next_cursor:next}:{}),...(filter?{filter}:{})});
}

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
 reply=async url=>url.searchParams.has('before')?page([known('run-10')]):page(first,'cursor-1');
 await render();
 expect(ids()).toHaveLength(10);
 await click('[data-workbuddy-progress-more]');
 expect(requests.at(-1)?.url.searchParams.get('before')).toBe('cursor-1');
 expect(ids()).toHaveLength(11);
 expect(host.querySelector('[data-workbuddy-progress-more]')).toBeNull();

 reply=async url=>url.searchParams.has('before')?page([first[0]]):page(first,'cursor-1');
 await click('[data-workbuddy-progress-refresh]');
 await click('[data-workbuddy-progress-more]');
 expect(ids()).toEqual([]);
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('does not match this project or page');
});

it('clears protected data on denial and ignores a late response after an identity change',async()=>{
 reply=async()=>page([known('original-run')]);
 await render();
 reply=async()=>Response.json({error:'forbidden'},{status:403});
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual([]);
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('Protected records were cleared');
 reply=async()=>page([known('original-run')]);
 await click('[data-workbuddy-progress-refresh]');
 expect(ids()).toEqual(['original-run']);
 let resolveOld!:(response:Response)=>void;
 reply=async()=>new Promise(resolve=>{resolveOld=resolve;});
 await act(async()=>host.querySelector<HTMLButtonElement>('[data-workbuddy-progress-refresh]')!.click());
 const old=requests.at(-1);
 expect(host.querySelector('[data-workbuddy-progress-open]')).toBeNull();
 reply=async()=>Response.json({error:'forbidden'},{status:403});
 const next={...session,user:{...session.user,id:'viewer-b'},organization:{...session.organization,id:'org-b'}};
 await render(next);
 expect(old?.init?.signal?.aborted).toBe(true);
 await act(async()=>resolveOld(page([known('late-run')])));
 await settle();
 expect(ids()).toEqual([]);
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
