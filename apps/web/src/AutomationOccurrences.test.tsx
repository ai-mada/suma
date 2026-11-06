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
