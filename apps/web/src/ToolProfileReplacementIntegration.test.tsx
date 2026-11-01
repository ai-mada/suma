// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolProfile} from '../../../packages/contracts/src';
import {api,type Session} from './api';
import * as ApiContext from './api-context';
import {ApiViewProvider,bindApiView} from './api-context';
import {ToolWorkspace} from './ToolWorkspace';
import {setLocale} from './i18n';

const stamp='2026-11-01T00:00:00Z',session:Session={user:{id:'user-a',name:'Manager',email:'manager@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const source:ToolProfile={id:'old-profile',project_id:'p',worker_id:'old-worker',name:'Original tool',version:4,state:'enabled',profile:{command_id:'original-code',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'user-a',created_at:stamp,updated_at:stamp,available:true,reason:''};
const targetDefinition={...source.profile,command_id:'new-code',profile_sha256:'c'.repeat(64)},existing:ToolProfile={...source,id:'existing-target',worker_id:'new-worker',name:'Existing team tool',version:6,profile:targetDefinition};
type Request={path:string;method:string;body?:Record<string,unknown>};
let root:Root|undefined,host:HTMLDivElement,currentSource:ToolProfile,currentTarget:ToolProfile|undefined,requests:Request[],reuse:boolean,conflict:boolean,caller:ReturnType<typeof bindApiView>,reply:(request:Request)=>Promise<Response>,lastReceipt:Record<string,unknown>|undefined;
function diagnostic(tool:ToolProfile){const revoked=tool.state==='revoked';return {tool,checked_at:'2026-11-01T00:00:20Z',expires_at:'2026-11-01T00:01:00Z',checks:['authorization','worker','inventory','profile'].map(code=>({code,status:revoked&&code==='profile'?'blocked':'pass',reason:revoked&&code==='profile'?'Profile permanently revoked':'',next_action:revoked&&code==='profile'?'register_profile':'none'})),next_action:revoked?'register_profile':'none',maintenance_resolution:{profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:revoked?'none':'open_worker',reason:revoked?'profile_revoked':'maintenance_unknown'}};}
function View(){const[selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project="p" session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}
async function normal(request:Request):Promise<Response>{
 const url=new URL(request.path,'http://local.test'),path=url.pathname;
 if(request.method==='GET'){
  if(path==='/tool-profiles')return Response.json({items:[currentSource],total:1});
  if(path==='/tool-profiles/old-profile')return Response.json(currentSource);
  if(path==='/tool-profiles/existing-target')return Response.json(existing);
  if(path==='/tool-profiles/new-target')return Response.json(currentTarget??lastReceipt?.replacement);
  if(path.endsWith('/diagnostics')){const tool=path.includes('old-profile')?currentSource:path.includes('new-target')?currentTarget??(lastReceipt!.replacement as ToolProfile):existing;return Response.json(diagnostic(tool));}
  if(path==='/release-workers')return Response.json({items:['old-worker','new-worker'].map((worker_id,i)=>({id:'worker-'+i,worker_id,status:'active',version:1}))});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:'2026-11-01T00:00:20Z'});
  if(path==='/adapter-catalog')return Response.json({project_id:'p',as_of:'2026-11-01T00:00:20Z',items:[{id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]}]});
  if(path==='/tool-discovery')return Response.json({project_id:'p',worker_id:url.searchParams.get('worker_id'),state:'fresh',as_of:'2026-11-01T00:00:20Z',observed_at:stamp,expires_at:'2026-11-01T00:01:00Z',items:[{profile:url.searchParams.get('worker_id')==='old-worker'?source.profile:targetDefinition,available:true,reason:''}]});
  if(path.endsWith('/replacements'))return Response.json({project_id:'p',profile_id:path.split('/')[2],direction:url.searchParams.get('direction')||'outgoing',items:[]});
  return Response.json({items:[]});
 }
 if(path==='/tool-profiles/matches'){
  const items=request.body?.worker_id==='old-worker'?[currentSource]:reuse?[existing]:[];
  return Response.json({project_id:'p',worker_id:request.body?.worker_id,profile:request.body?.profile,items,total:items.length});
 }
 if(path==='/tool-profiles/old-profile/replacements'){
  if(conflict)return Response.json({error:{code:'tool_profile_match_exists',message:'Choose the current matching profile explicitly'}},{status:409});
  const body=request.body!,original=body.pause_original?{...currentSource,state:'paused',version:currentSource.state==='enabled'?currentSource.version+1:currentSource.version,available:false,reason:'Profile paused'}:currentSource;
  const replacement=body.reuse?existing:{...existing,id:'new-target',name:body.name,version:1,state:'paused',enabled_by:'',available:false,reason:'Profile paused'};
  lastReceipt={id:'replacement-relation',project_id:'p',original,replacement,disposition:body.reuse?'reused':'created',pause_original:body.pause_original,created_at:'2026-11-01T00:00:30Z',created_by:'user-a'};currentSource=original as ToolProfile;return Response.json(lastReceipt,{status:201});
 }
 return Response.json({error:{code:'unexpected_write',message:'No implicit lifecycle command'}},{status:400});
}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);requests=[];reuse=false;conflict=false;currentSource=structuredClone(source);currentTarget=undefined;lastReceipt=undefined;caller=bindApiView(session,api);vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);reply=normal;vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const request={path:String(input).replace(/^\/api\/v1/,''),method:init.method||'GET',body:init.body?JSON.parse(String(init.body)):undefined};requests.push(request);return reply(request);}));host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();sessionStorage.clear();vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(done=>setTimeout(done,0));});}
async function render(){await act(async()=>root!.render(<View/>));await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(selector:string,value:string){const element=document.querySelector<HTMLInputElement|HTMLSelectElement>(selector);expect(element,selector).not.toBeNull();const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element!.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
const mutations=()=>requests.filter(request=>request.method!=='GET'&&!request.path.startsWith('/tool-profiles/matches'));
const rawJournal=()=>sessionStorage.getItem('suma-tool-recovery-v1:org-a:user-a:p:profiles');
async function prepare(){await click('[data-tools-adjust]');await change('[data-quick-worker]','new-worker');await click('[data-quick-replacement-prepare]');}

it('T200 confirms one cross-worker replacement and explicitly reuses an enabled target without activating or mutating history',async()=>{
 await render();await click('.tool-resource');await click('[data-tools-adjust]');await click('[data-quick-advanced]');expect(document.querySelector('.tool-form')).not.toBeNull();await click('[data-tools-return-quick]');await change('[data-quick-worker]','new-worker');await click('[data-quick-replacement-prepare]');expect(mutations()).toEqual([]);
 expect(document.querySelector<HTMLInputElement>('[data-tool-replacement-pause]')?.checked).toBe(false);await click('[data-tool-replacement-submit]');
 expect(mutations()).toHaveLength(1);expect(mutations()[0]).toMatchObject({path:'/tool-profiles/old-profile/replacements',body:{project_id:'p',expected_version:4,worker_id:'new-worker',profile:targetDefinition,pause_original:false,idempotency_key:expect.any(String)}});
 expect(lastReceipt?.original).toMatchObject({id:source.id,worker_id:'old-worker',version:4,state:'enabled'});expect(lastReceipt?.replacement).toMatchObject({id:'new-target',worker_id:'new-worker',version:1,state:'paused'});expect(document.querySelector('[data-tool-replacement-receipt]')).not.toBeNull();expect(document.querySelector('.tool-detail')?.textContent).toContain('new-code');
 const targetReads=requests.filter(request=>request.path.startsWith('/tool-profiles/new-target/diagnostics')).length;currentTarget={...(lastReceipt!.replacement as ToolProfile),version:2};expect(mutations()).toHaveLength(1);
 await click('[data-tool-replacement-enable]');expect(requests.filter(request=>request.path.startsWith('/tool-profiles/new-target/diagnostics')).length).toBeGreaterThan(targetReads);expect(document.querySelector('[role="dialog"] .check')?.textContent).toContain('configuration v2');expect(mutations()).toHaveLength(1);expect(lastReceipt?.replacement).toMatchObject({version:1,state:'paused'});await click('[role="dialog"] .dialog-actions button[type="button"]');
 await click('[data-tool-replacement-original]');reuse=true;await click('[data-tools-adjust]');await change('[data-quick-worker]','new-worker');expect(document.querySelector('[data-tool-replacement-confirm]')).toBeNull();await change('[data-quick-replacement-existing]',existing.id);await click('[data-quick-replacement-prepare]');
 await click('[data-tool-replacement-pause]');await click('[data-tool-replacement-submit]');expect(mutations()).toHaveLength(2);expect(mutations()[1].body).toMatchObject({expected_version:4,worker_id:'new-worker',pause_original:true,reuse:{profile_id:existing.id,expected_version:6}});expect(mutations()[1].body).not.toHaveProperty('name');
 expect(lastReceipt?.original).toMatchObject({id:source.id,worker_id:'old-worker',version:5,state:'paused'});expect(lastReceipt?.replacement).toMatchObject({id:existing.id,version:6,state:'enabled'});expect(document.querySelector('[data-tool-replacement-enable]')).toBeNull();expect(mutations().every(request=>request.path==='/tool-profiles/old-profile/replacements')).toBe(true);
 // A revoked historical source still offers a separate fresh registration,
 // never a replacement transaction or restoration of the revoked record.
 currentSource={...currentSource,state:'revoked',version:6,available:false,reason:'Profile permanently revoked'};await click('[data-tool-replacement-original]');await click('[data-tool-doctor-action="register_profile"]');
 expect(document.querySelector('[data-quick-replacement-prepare]')).toBeNull();expect(document.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(false);expect(currentSource).toMatchObject({id:source.id,state:'revoked',version:6});expect(mutations()).toHaveLength(2);
});

it('T200 keeps an uncertain original replacement body and key through caller changes, invalid receipts and a later known CAS refusal',async()=>{
 await render();await click('.tool-resource');await prepare();let finish!:(response:Response)=>void;reply=request=>request.path==='/tool-profiles/old-profile/replacements'?new Promise(done=>{finish=done;}):normal(request);await click('[data-tool-replacement-submit]');
 const original=mutations()[0],raw=rawJournal();expect(raw).not.toBeNull();caller=bindApiView(session,api);await render();
 const committed=await normal(original);await act(async()=>finish(committed));await settle();expect(rawJournal()).toBe(raw);expect(document.querySelector('[data-tool-replacement-receipt]')).toBeNull();
 reply=async request=>request.path==='/tool-profiles/old-profile/replacements'?Response.json({...lastReceipt,replacement:{...(lastReceipt!.replacement as ToolProfile),worker_id:'wrong-worker'}},{status:201}):normal(request);
 await click('.tool-recovery .primary');expect(rawJournal()).toBe(raw);expect(mutations()).toHaveLength(2);expect(mutations()[1].body).toEqual(original.body);
 currentSource={...currentSource,version:9};reply=request=>request.path==='/tool-profiles/old-profile/replacements'?Promise.resolve(Response.json(lastReceipt,{status:201})):normal(request);await click('.tool-recovery .primary');
 expect(mutations()).toHaveLength(3);expect(mutations()[2].body).toEqual(original.body);expect(rawJournal()).toBeNull();expect(document.querySelector('[data-tool-replacement-receipt]')).not.toBeNull();expect(document.querySelector('.tool-detail')?.textContent).toContain('new-code');
 await click('[data-tool-replacement-original]');await prepare();conflict=true;reply=normal;await click('[data-tool-replacement-submit]');expect(mutations()).toHaveLength(4);expect(mutations()[3].body?.expected_version).toBe(9);expect(rawJournal()).toBeNull();
 // A known refusal retains the reviewed source and candidate, with its
 // confirmation blocked until an explicit reread; it is no longer an unknown.
 expect(document.querySelector('[data-tool-replacement-confirm]')).not.toBeNull();expect(document.querySelector<HTMLButtonElement>('[data-tool-replacement-submit]')?.disabled).toBe(true);expect(mutations().every(request=>request.path==='/tool-profiles/old-profile/replacements')).toBe(true);
});
