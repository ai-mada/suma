// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import type {AutomationRule} from './automation-model';
import {AutomationWorkspace} from './AutomationWorkspace';
import {setLocale} from './i18n';

const project='project-automation';
const session:Session={user:{id:'reviewer',name:'Reviewer',email:'reviewer@example.test'},organization:{id:'team',name:'Team'},projects:[{id:project,version:1,role:'viewer'}]};
const stamp='2026-11-06T01:00:00Z';
const rule:AutomationRule={id:'rule-fixed',project_id:project,version:1,revision:1,name:'Nightly check',workflow_id:'workflow-fixed',workflow_version:1,trigger:'manual',cron:'',timezone:'UTC',misfire:'skip',catch_up_limit:1,overlap:'forbid',state:'enabled',activated_by:'reviewer',next_at:'',last_error:'',created_at:stamp,updated_at:stamp};
const occurrence={rule_id:rule.id,revision:1,source:'manual',scheduled_at:stamp,state:'started',reason:'',created_at:stamp,updated_at:stamp};

it('loads authoritative attention pages and ignores a late reply from the previous filter',async()=>{
 const host=document.createElement('div');document.body.append(host);
 const requests:{filter:string|null;cursor:string|null;user:string|null;org:string|null}[]=[];
 let denied=false,holdAll=false,releaseAll:(response:Response)=>void=()=>{};
 const lateAll=new Promise<Response>(resolve=>{releaseAll=resolve;});
 const normal={...occurrence,id:'healthy-occurrence',run_id:'run-healthy',run_state:'succeeded'};
 const pending={...occurrence,id:'reconciling-occurrence',run_id:'run-reconcile',run_state:'running',run_needs_reconciliation:true};
 const failed={...occurrence,id:'failed-occurrence',run_id:'run-failure',run_state:'failed'};
 const root=createRoot(host);
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  expect(options?.method).toBe('GET');
  const url=new URL(input,location.origin);
  if(url.pathname.endsWith('/occurrences')){
   expect(url.searchParams.get('project_id')).toBe(project);
   const filter=url.searchParams.get('filter'),cursor=url.searchParams.get('cursor');
   const headers=new Headers(options?.headers);
   requests.push({filter,cursor,user:headers.get('X-Suma-Expected-User'),org:headers.get('X-Suma-Expected-Organization')});
   if(denied)return Response.json({error:{code:'forbidden',message:'Access denied'}},{status:403});
   if(filter==='attention')return cursor?Response.json({items:[failed],total:2}):Response.json({items:[pending],total:2,next_cursor:'attention-next'});
   if(holdAll)return lateAll;
   return Response.json({items:[normal],total:3,next_cursor:'all-next'});
  }
  if(url.pathname.endsWith('/automations/'+rule.id))return Response.json(rule);
  if(url.pathname.endsWith('/automations'))return Response.json({items:[rule],total:1});
  return Response.json({items:[],total:0});
 }));
 const click=async(button:HTMLButtonElement)=>{await act(async()=>{button.click();});};
 const filter=(value:string)=>host.querySelector<HTMLButtonElement>(`[data-automation-occurrence-filter="${value}"]`)!;
 const rows=()=>[...host.querySelectorAll<HTMLAnchorElement>('.automation-occurrence a')].map(link=>new URL(link.href).searchParams.get('run'));
 try{
  await setLocale('en-US',{persist:false});
  await act(async()=>root.render(<ApiViewProvider session={session}><AutomationWorkspace project={project} session={session} selected={rule.id} search="" onSearch={()=>{}} refresh={0} onSelect={()=>{}}/></ApiViewProvider>));
  expect(rows()).toEqual(['run-healthy']);
  expect(requests).toEqual([{filter:'all',cursor:null,user:'reviewer',org:'team'}]);
  holdAll=true;
  await click(host.querySelector<HTMLButtonElement>('[aria-label="Refresh trigger records"]')!);
  await click(filter('attention'));
  expect(filter('attention').getAttribute('aria-pressed')).toBe('true');
  expect(rows()).toEqual(['run-reconcile']);
  expect(host.querySelector('.automation-run-reconciliation')?.textContent).toBe('A node in this run needs reconciliation');
  expect(host.querySelector('.automation-occurrences .automation-occurrence-footer')?.textContent).toContain('Loaded 1 / 2');
  await act(async()=>releaseAll(Response.json({items:[normal],total:3,next_cursor:'all-next'})));
  expect(rows()).toEqual(['run-reconcile']);
  await click(host.querySelector<HTMLButtonElement>('.automation-occurrences .automation-occurrence-footer button')!);
  expect(rows()).toEqual(['run-reconcile','run-failure']);
  expect(requests.slice(2)).toEqual([{filter:'attention',cursor:null,user:'reviewer',org:'team'},{filter:'attention',cursor:'attention-next',user:'reviewer',org:'team'}]);
  expect(host.querySelector('.automation-occurrences .automation-occurrence-footer')?.textContent).toContain('Loaded 2 / 2');
  denied=true;
  await click(host.querySelector<HTMLButtonElement>('[aria-label="Refresh trigger records"]')!);
  expect(rows()).toEqual([]);
  expect(host.querySelector('.automation-occurrences .automation-occurrence-footer')?.textContent).toContain('Loaded — / —');
  expect(host.querySelector('.automation-occurrences .automation-occurrence-footer button')).toBeNull();
  expect(host.querySelector('.automation-occurrences')?.textContent).not.toContain('No triggers need attention');
 }finally{
  await act(async()=>root.unmount());host.remove();
  vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});
 }
});

it('opens an exact trigger independently of history and discards stale or mismatched detail responses',async()=>{
 const host=document.createElement('div');document.body.append(host);
 const root=createRoot(host);
 const detailCalls:{id:string;signal:AbortSignal|null|undefined}[]=[];
 const current={...rule,revision:3,state:'paused'};
 const original={...occurrence,id:'earlier-trigger',revision:1,state:'blocked',reason:'failure_pause_threshold_reached',run_id:''};
 const next={...occurrence,id:'another-trigger',revision:2,run_id:'linked-run',run_state:'failed'};
 let panel='automation-trigger:'+original.id;
 let releaseEarlier:(response:Response)=>void=()=>{};
 const delayed=new Promise<Response>(resolve=>{releaseEarlier=resolve;});
 let mismatch=true;
 const view=()=><ApiViewProvider session={session}><AutomationWorkspace project={project} session={session}
  selected={rule.id} search="" onSearch={()=>{}} refresh={0} onSelect={()=>{}} panel={panel}
  onOccurrenceSelect={id=>{panel=id?'automation-trigger:'+id:'';root.render(view());}}/></ApiViewProvider>;
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(input,location.origin);
  const exactPrefix='/api/v1/automations/'+rule.id+'/occurrences/';
  if(url.pathname.startsWith(exactPrefix)){
   const id=url.pathname.slice(exactPrefix.length);
   expect(options?.method).toBe('GET');
   expect([...url.searchParams]).toEqual([['project_id',project]]);
   const headers=new Headers(options?.headers);
   expect(headers.get('X-Suma-Expected-User')).toBe(session.user.id);
   expect(headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
   detailCalls.push({id,signal:options?.signal});
   if(id===original.id)return delayed;
   if(id===next.id)return Response.json(mismatch?{...next,rule_id:'other-rule'}:next);
   return Response.json({error:{code:'not_found',message:'Not found'}},{status:404});
  }
  if(url.pathname.endsWith('/automations/'+rule.id))return Response.json(current);
  if(url.pathname.endsWith('/automations'))return Response.json({items:[current],total:1});
  if(url.pathname.endsWith('/occurrences'))return Response.json({items:[],total:0});
  return Response.json({items:[],total:0});
 }));
 const render=async()=>{await act(async()=>root.render(view()));};
 const detail=()=>host.querySelector('[data-automation-trigger-detail]')!;
 try{
  await setLocale('en-US',{persist:false});
  await render();
  expect(detail().getAttribute('aria-busy')).toBe('true');
  expect(document.activeElement?.id).toBe('automation-trigger-detail-heading');
  panel='automation-trigger:'+next.id;
  await render();
  expect(detailCalls.map(call=>call.id)).toEqual([original.id,next.id]);
  expect(detailCalls[0].signal?.aborted).toBe(true);
  expect(detail().textContent).toContain('The server did not confirm this trigger');
  expect(detail().querySelector('[data-automation-trigger-record]')).toBeNull();
  await act(async()=>releaseEarlier(Response.json(original)));
  expect(detail().textContent).not.toContain('No run is linked');
  mismatch=false;
  await act(async()=>detail().querySelector<HTMLButtonElement>('[aria-label="Refresh trigger details"]')!.click());
  expect(detail().querySelector('[data-automation-trigger-record]')?.getAttribute('data-automation-trigger-record')).toBe(next.id);
  expect(detail().textContent).toContain('This trigger used revision 2. The rule is now on revision 3.');
  expect(new URL(detail().querySelector('a')!.href).searchParams.get('run')).toBe(next.run_id);
  panel='automation-trigger:missing-trigger';
  await render();
  expect(detail().textContent).toContain('This trigger is unavailable');
  expect(detail().querySelector('[data-automation-trigger-record]')).toBeNull();
  await act(async()=>detail().querySelector<HTMLButtonElement>('[aria-label="Close trigger details"]')!.click());
  expect(detail()).toBeNull();
  expect(document.activeElement?.id).toBe('automation-occurrence-heading');
 }finally{
  await act(async()=>root.unmount());host.remove();
  vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});
 }
});

it('shows only the exact retained pause evidence and preserves the leave guard on linked failed runs',async()=>{
 const host=document.createElement('div');document.body.append(host);
 const root=createRoot(host);
 const stopped={...occurrence,id:'stopped-trigger',state:'blocked',reason:'failure_pause_threshold_reached',run_id:''};
 const retained={
  threshold:2,observed_at:stamp,failed_runs:[{trigger_id:'recent-trigger',run_id:'recent-run'},{trigger_id:'earlier-trigger',run_id:'earlier-run'}],
 };
 let result:unknown={...stopped,failure_pause_evidence:retained},refresh=0;
 const veto=(event:Event)=>event.preventDefault();
 const view=()=><ApiViewProvider session={session}><AutomationWorkspace project={project} session={session}
  selected={rule.id} search="" onSearch={()=>{}} refresh={refresh} onSelect={()=>{}} panel={'automation-trigger:'+stopped.id}/></ApiViewProvider>;
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 const requests=vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(input,location.origin);
  expect(options?.method||'GET').toBe('GET');
  if(url.pathname.endsWith('/occurrences/'+stopped.id))return Response.json(result);
  if(url.pathname.endsWith('/automations/'+rule.id))return Response.json(rule);
  if(url.pathname.endsWith('/automations'))return Response.json({items:[rule],total:1});
  if(url.pathname.endsWith('/occurrences'))return Response.json({items:[stopped],total:1});
  return Response.json({items:[],total:0});
 });
 vi.stubGlobal('fetch',requests);
 const render=async()=>{await act(async()=>root.render(view()));};
 const section=()=>host.querySelector('[data-automation-pause-evidence]');
 const missing=()=>host.querySelector('[data-automation-pause-evidence-unavailable]');
 try{
  await setLocale('en-US',{persist:false});
  await render();
  expect(section()?.closest('[data-automation-trigger-record]')?.getAttribute('data-automation-trigger-record')).toBe(stopped.id);
  expect(section()?.textContent).toContain('after 2 consecutive failed runs');
  expect(section()?.querySelector('time')?.dateTime).toBe(stamp);
  const links=[...section()!.querySelectorAll<HTMLAnchorElement>('[data-automation-pause-evidence-run]')];
  expect(links.map(link=>[...new URL(link.href).searchParams])).toEqual(retained.failed_runs.map(entry=>[
   ['project',project],['view','development'],['sub','runs'],['run',entry.run_id],
  ]));
  window.addEventListener('suma:navigate',veto);
  try{expect(links[0].dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}))).toBe(false);}
  finally{window.removeEventListener('suma:navigate',veto);}
  for(const evidence of [
   undefined,
   {...retained,observed_at:'2026-11-06T00:00:00Z'},
   {...retained,failed_runs:[retained.failed_runs[0],{trigger_id:'another-trigger',run_id:'recent-run'}]},
  ]){
   result={...stopped,failure_pause_evidence:evidence};
   refresh++;
   await render();
   expect(section()).toBeNull();
   expect(missing()?.textContent).toContain('Saved failure evidence is unavailable');
  }
  result={...stopped,reason:'rule_pause',failure_pause_evidence:retained};
  refresh++;
  await render();
  expect(section()).toBeNull();
  expect(missing()).toBeNull();
 }finally{
  await act(async()=>root.unmount());host.remove();
  vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});
 }
});
