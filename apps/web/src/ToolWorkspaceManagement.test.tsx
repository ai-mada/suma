// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {ToolWorkspace} from './ToolWorkspace';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'o',name:'Org'},projects:[]};
const definition={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const tool={id:'existing',project_id:'p',worker_id:'original-runner',name:'Original team tool',version:4,state:'enabled',profile:definition,enabled_by:'u',created_at:'2026-10-30T08:00:00Z',updated_at:'2026-10-30T08:00:00Z',available:false,reason:'inventory_stale'};
const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
let root:Root|undefined,host:HTMLDivElement|undefined;const reads:URL[]=[],writes:string[]=[];
function View(){const [selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}
async function settle(){await act(async()=>{for(let i=0;i<28;i++)await Promise.resolve();});}
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){await act(async()=>{Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function mount(){
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);reads.length=0;writes.length=0;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1',''),method=init.method||'GET';
  if(method!=='GET'){if(path==='/tool-profiles/matches')return Response.json({project_id:'p',worker_id:'original-runner',profile:definition,items:[tool],total:1});writes.push(method+' '+path);return Response.json({error:{code:'unexpected_write',message:'Unexpected write'}},{status:400});}
  reads.push(url);
  if(path==='/tool-profiles')return Response.json({items:[tool],total:1});
  if(path==='/tool-profiles/existing')return Response.json(tool);
  if(path==='/tool-profiles/existing/diagnostics')return Response.json({tool,checked_at:'2026-10-30T08:00:20Z',expires_at:'2026-10-30T08:01:00Z',checks:['authorization','worker','inventory','profile'].map(code=>({code,status:code==='profile'?'blocked':'pass',reason:code==='profile'?'inventory_profile_mismatch':'',next_action:code==='profile'?'register_profile':'none'})),next_action:'register_profile'});
  if(path==='/release-workers')return Response.json({items:['other-runner','original-runner'].map((worker_id,i)=>({id:'w'+i,worker_id,status:'active',version:1}))});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:'2026-10-30T08:00:20Z'});
  if(path==='/adapter-catalog')return Response.json({project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter]});
  if(path==='/tool-discovery')return Response.json({project_id:'p',worker_id:url.searchParams.get('worker_id'),state:'fresh',as_of:'2026-10-30T08:00:20Z',observed_at:'2026-10-30T08:00:00Z',expires_at:'2026-10-30T08:01:00Z',items:[{profile:definition,available:true,reason:''}]});
  return Response.json({items:[]});
 }));host=document.createElement('div');document.body.append(host);root=createRoot(host);await act(async()=>root!.render(<View/>));await settle();
}
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();document.body.innerHTML='';vi.unstubAllGlobals();vi.restoreAllMocks();window.history.replaceState({},'','/');});

it('T195 keeps maintenance freshness tied to the original profile read and refreshes that exact profile without losing the dialog',async()=>{
 let monotonic=1000;vi.spyOn(performance,'now').mockImplementation(()=>monotonic);
 await mount();const originalFetch=vi.mocked(fetch).getMockImplementation()!;
 let profileReads=0,refreshed=false;
 const maintenance=()=>({state:'supported',protocol:'suma-tool-maintenance-v1',instance_id:'continuous-owner-195',observed_at:refreshed?'2026-11-01T00:01:10Z':'2026-11-01T00:00:00Z',checked_at:refreshed?'2026-11-01T00:01:20Z':'2026-11-01T00:00:55Z',expires_at:refreshed?'2026-11-01T00:02:10Z':'2026-11-01T00:01:00Z'});
 vi.mocked(fetch).mockImplementation(async(input,init={})=>{
  const path=new URL(String(input),'http://local.test').pathname.replace('/api/v1','');
  if(path==='/tool-profiles/existing'){profileReads++;return Response.json({...tool,maintenance:maintenance()});}
  if(path==='/tool-profiles/existing/inspections'&&(!init.method||init.method==='GET'))return Response.json({items:[]});
  return originalFetch(input,init);
 });
 await act(async()=>host!.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();
 expect(profileReads).toBe(1);monotonic=7001;
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-tool-doctor-inspect]')!.click());await settle();
 const dialog=document.querySelector('[role="dialog"]');expect(dialog).not.toBeNull();
 expect(document.querySelector<HTMLButtonElement>('[data-tool-reinspection-create]')?.disabled).toBe(true);
 const refreshCapability=document.querySelector<HTMLButtonElement>('[data-tool-maintenance-refresh]');expect(refreshCapability).not.toBeNull();
 refreshed=true;await act(async()=>refreshCapability!.click());await settle();
 expect(profileReads).toBe(2);expect(document.querySelector('[role="dialog"]')).toBe(dialog);
 expect(document.querySelector<HTMLButtonElement>('[data-tool-reinspection-create]')?.disabled).toBe(false);
 expect(host!.querySelector('.tool-resource.selected')?.textContent).toContain(tool.name);
 expect(reads.some(url=>url.pathname.includes('/tool-profiles/inspection-'))).toBe(false);expect(writes).toEqual([]);
});

it('uses server tool filters without overwriting task status and searches only on an explicit action',async()=>{
 window.history.replaceState({},'','/?project=p&status=running&tools_state=revoked&tools_query=old');await mount();
 expect(reads.find(url=>url.pathname.endsWith('/tool-profiles'))?.searchParams.get('state')).toBe('revoked');
 const input=host!.querySelector<HTMLInputElement>('[data-tools-query]')!;expect(input.value).toBe('old');const before=reads.length;
 await change(input,'Codex');expect(reads.length).toBe(before);
 await act(async()=>host!.querySelector<HTMLFormElement>('[data-tools-search]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();
 expect(reads.filter(url=>url.pathname.endsWith('/tool-profiles')).at(-1)?.searchParams.get('query')).toBe('Codex');
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-tools-state="active"]')!.click());await settle();
 expect(reads.filter(url=>url.pathname.endsWith('/tool-profiles')).at(-1)?.searchParams.get('state')).toBe('active');
 expect(new URLSearchParams(window.location.search).get('status')).toBe('running');expect(writes).toEqual([]);
});

it('takes Doctor to the exact original adapter and worker, then returns from advanced setup with its public draft',async()=>{
 await mount();await act(async()=>host!.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-tool-doctor-action="register_profile"]')!.click());await settle();
 expect(document.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe('codex-jsonl');
 expect(document.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('original-runner');
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-quick-advanced]')!.click());await settle();
 expect(document.querySelector<HTMLSelectElement>('.tool-form select')?.value).toBe('original-runner');
 const name=document.querySelector<HTMLInputElement>('.tool-form input[maxlength="200"]')!;await change(name,'Adjusted team name');
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-tools-return-quick]')!.click());await settle();
 expect(document.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe('codex-jsonl');expect(document.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('original-runner');
 expect(document.querySelector('[data-existing-tool-open="existing"]')).not.toBeNull();expect(writes).toEqual([]);
});

it('T194 requests inspection through a separate receipt and returns to the original profile without selecting the request as a tool',async()=>{
 await mount();const originalFetch=vi.mocked(fetch).getMockImplementation()!,inspectionWrites:Record<string,unknown>[]=[];
 let currentVersion=4;
 const maintenance={state:'supported',protocol:'suma-tool-maintenance-v1',instance_id:'continuous-owner-194',observed_at:'2026-11-01T00:00:00Z',checked_at:'2026-11-01T00:00:20Z',expires_at:'2026-11-01T00:01:00Z'};
 const pending={id:'inspection-main194',project_id:'p',worker_id:tool.worker_id,tool_profile_id:tool.id,tool_profile_version:5,profile_sha256:definition.profile_sha256,command_id:definition.command_id,version:1,fence:0,state:'pending',requested_by:'u',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:05:00Z'};
 const completed={...pending,version:3,fence:1,state:'completed',claim_id:'claim-main-194',claimed_at:'2026-11-01T00:00:10Z',lease_until:'2026-11-01T00:01:10Z',observed_at:'2026-11-01T00:00:12Z',inventory_operation_id:'inventory-main-194',result_code:'reported_unavailable'};
 vi.mocked(fetch).mockImplementation(async(input,init={})=>{const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');if(path==='/tool-profiles/existing')return Response.json({...tool,version:currentVersion,maintenance});if(path==='/tool-profiles/existing/inspections'){if(init.method==='POST'){inspectionWrites.push(JSON.parse(String(init.body)));return Response.json(pending);}return Response.json({items:[]});}if(path==='/tool-inspections/inspection-main194')return Response.json(completed);return originalFetch(input,init);});
 await act(async()=>host!.querySelector<HTMLButtonElement>('.tool-resource')!.click());await settle();
 await act(async()=>host!.querySelector<HTMLButtonElement>('[data-tool-doctor-inspect]')!.click());await settle();
 expect(document.querySelector('[data-tool-reinspection-panel]')).not.toBeNull();expect(inspectionWrites).toEqual([]);
 currentVersion=5;await act(async()=>host!.querySelector<HTMLButtonElement>('.tool-refresh-bar button[aria-label]')!.click());await settle();
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-tool-reinspection-create]')!.click());await settle();
 expect(inspectionWrites).toHaveLength(1);expect(inspectionWrites[0]).toMatchObject({project_id:'p',expected_version:5});expect(typeof inspectionWrites[0].idempotency_key).toBe('string');
 expect(document.querySelector('[data-tool-reinspection-state]')?.getAttribute('data-tool-reinspection-state')).toBe('completed');
 await act(async()=>document.querySelector<HTMLButtonElement>('[data-tool-reinspection-review]')!.click());await settle();
 expect(document.querySelector('[role="dialog"]')).toBeNull();expect(host!.querySelector('.tool-resource.selected')?.textContent).toContain(tool.name);expect(reads.some(url=>url.pathname.endsWith('/tool-profiles/inspection-main194'))).toBe(false);expect(writes).toEqual([]);
});
