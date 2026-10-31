// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolPermission} from '../../../packages/contracts/src';
import {ApiViewProvider,bindApiView} from './api-context';
import type {ApiCaller,Session} from './api';
import {setLocale} from './i18n';
import {ACPExecutionPermissionPanel} from './ACPExecutionPermissionPanel';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async importOriginal=>{const actual=await importOriginal<typeof import('./api-context')>();return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};});
const session:Session={user:{id:'user-a',name:'Reviewer',email:'reviewer@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const initial={project:'project-a',runID:'run-a',nodeID:'node-a',attempt:2,fence:3};
const listPath='/runs/run-a/nodes/node-a/tool-permissions?project_id=project-a';
const decisionPath='/tool-permissions/permission-a/decision';
function permission():ToolPermission{return {id:'permission-a',project_id:initial.project,run_id:initial.runID,node_id:initial.nodeID,attempt:2,fence:3,dispatch_id:'dispatch-a',worker_id:'worker-a',profile_sha256:'a'.repeat(64),version:1,state:'pending',request:{idempotency_key:'runner-original-request',session_id:'session-a',tool_call_id:'tool-call-a',title:'Review the exact file change',kind:'edit',details:'Change only the approved file <script>data</script>',details_complete:true,options:[{id:'allow-one',name:'Allow this exact patch',kind:'allow_once'},{id:'allow-specific',name:'Allow only the reviewed file',kind:'allow_once'},{id:'reject-one',name:'Reject this exact patch',kind:'reject_once'},{id:'allow-always',name:'Always allow changes',kind:'allow_always'}]},created_at:'2026-10-31T00:00:00Z',expires_at:'2099-01-01T00:00:00Z',allowed_actions:['allow_once','reject_once']};}
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
type Reply=(request:Request)=>Promise<Response>;
let root:Root,host:HTMLDivElement,items:ToolPermission[],requests:Request[],reply:Reply,onChanged:ReturnType<typeof vi.fn>,saved:Map<string,ToolPermission>;
async function normal(request:Request):Promise<Response>{
 if(request.method==='GET'&&request.path===listPath+'&permission_id=permission-a')return Response.json({items:[...saved.values()].filter(item=>item.id==='permission-a')});
 if(request.method==='GET')return Response.json({items});
 const body=request.body!,key=String(body.idempotency_key);let result=saved.get(key);
 if(!result){const original=items.find(item=>request.path==='/tool-permissions/'+item.id+'/decision')!;const option=original.request.options.find(option=>option.id===body.option_id)!;result={...original,version:original.version+1,state:'decided',allowed_actions:[],decision:{option_id:option.id,kind:option.kind,actor_id:session.user.id,decided_at:'2026-10-31T00:01:00Z'}};saved.set(key,result);items=items.map(item=>item.id===result!.id?result!:item);}
 return Response.json(result);
}
beforeEach(async()=>{
 callerOverride.current=undefined;await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();items=[permission()];requests=[];saved=new Map();reply=normal;onChanged=vi.fn();host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const body:Record<string,unknown>|undefined=init?.body?JSON.parse(String(init.body)):undefined,request={path:String(input).replace(/^\/api\/v1/,''),method:init?.method||(body?'POST':'GET'),body,headers:new Headers(init?.headers)};requests.push(request);return reply(request);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render({identity=session,scope=initial,revision=0,state='running'}={}){await act(async()=>root.render(<ApiViewProvider session={identity}><p data-public-run>Original run remains available</p><ACPExecutionPermissionPanel {...scope} state={state} revision={revision} onChanged={onChanged}/></ApiViewProvider>));await settle();}
function toggle(){const button=host.querySelector<HTMLButtonElement>('.acp-permissions-toggle');expect(button).not.toBeNull();return button!;}
async function open(){if(toggle().getAttribute('aria-expanded')!=='true'){await act(async()=>toggle().click());await settle();}}
async function refresh(){const button=host.querySelector<HTMLButtonElement>('.acp-permissions-refresh');expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
function option(id:string){const button=host.querySelector<HTMLButtonElement>('[data-acp-permission="permission-a"] [data-acp-option="'+id+'"]');expect(button,id).not.toBeNull();return button!;}
async function clickOption(id:string){await act(async()=>option(id).click());await settle();}
const writes=()=>requests.filter(request=>request.method==='POST');
function recovery(){const button=host.querySelector<HTMLButtonElement>('[data-acp-permission-recover]');expect(button).not.toBeNull();return button!;}
function journalFor(operation:string){for(let index=0;index<sessionStorage.length;index++){const key=sessionStorage.key(index)!,value=sessionStorage.getItem(key)!;try{if(JSON.parse(value).idempotency_key===operation)return {key,value};}catch{/* Not an operation record. */}}throw new Error('Original decision journal missing');}

it('reads a collapsed summary then sends exactly one original named option with the current view and CAS basis',async()=>{
 await render();expect(toggle().getAttribute('aria-expanded')).toBe('false');expect(requests.map(request=>request.path)).toEqual([listPath]);expect(host.querySelector('.acp-permissions-count')?.textContent).toBe('1');await open();
 expect(host.textContent).toContain(items[0].request.details);expect(host.querySelector('script')).toBeNull();expect(option('allow-one').textContent).toContain('Allow this exact patch');expect(option('allow-specific').textContent).toContain('Allow only the reviewed file');expect(option('allow-always').disabled).toBe(true);expect(writes()).toHaveLength(0);
 const chosen=option('allow-specific');await act(async()=>{chosen.click();chosen.click();});await settle();expect(writes()).toHaveLength(1);
 expect(writes()[0].path).toBe(decisionPath);expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,option_id:'allow-specific',idempotency_key:expect.any(String)});expect(writes()[0].body!.idempotency_key).not.toBe(items[0].request.idempotency_key);
 expect(writes()[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(writes()[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(host.querySelector('[data-acp-permission="permission-a"] [data-state="decided"]')).not.toBeNull();expect(onChanged).toHaveBeenCalledOnce();
});

it('keeps incomplete details unapprovable while allowing the exact one-time rejection',async()=>{
 items=[{...permission(),request:{...permission().request,details_complete:false},allowed_actions:['reject_once']}];await render();await open();expect(option('allow-one').disabled).toBe(true);expect(option('allow-specific').disabled).toBe(true);expect(option('allow-always').disabled).toBe(true);expect(option('reject-one').disabled).toBe(false);
 await clickOption('reject-one');expect(writes()).toHaveLength(1);expect(writes()[0].body).toMatchObject({option_id:'reject-one'});expect(saved.values().next().value?.decision?.kind).toBe('reject_once');
});

it('retains an unknown decision across collapse and remount and never substitutes a GET for the original receipt',async()=>{
 let lost=true;reply=async request=>{if(request.method==='POST'&&lost){lost=false;await normal(request);throw new Error('Decision response lost after commit');}return normal(request);};
 await render();await open();await clickOption('allow-one');const original=writes()[0],operation=String(original.body!.idempotency_key),journal=journalFor(operation);
 const sidecar=Array.from({length:sessionStorage.length},(_,index)=>{const key=sessionStorage.key(index)!;return {key,value:sessionStorage.getItem(key)!};}).find(record=>{try{return JSON.parse(record.value).request?.idempotency_key===operation;}catch{return false;}});
 expect(sidecar).toBeDefined();expect(JSON.parse(sidecar!.value)).toMatchObject({scope:{project:'project-a',runID:'run-a',nodeID:'node-a',attempt:2,fence:3,userID:'user-a',organizationID:'org-a'},permission_id:'permission-a',profile_sha256:'a'.repeat(64),dispatch_id:'dispatch-a',worker_id:'worker-a',option_kind:'allow_once',request:{path:decisionPath,body:original.body}});
 await act(async()=>toggle().click());await settle();items=[];const next={...initial,attempt:3,fence:4};await render({scope:next,revision:1});
 expect(writes()).toHaveLength(1);expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(sessionStorage.getItem(sidecar!.key)).toBe(sidecar!.value);
 await act(async()=>root.unmount());root=createRoot(host);await render({scope:next,revision:2});await open();await refresh();
 expect(requests.some(request=>request.method==='GET'&&request.path===listPath+'&permission_id=permission-a')).toBe(true);expect(writes()).toHaveLength(1);expect(recovery().disabled).toBe(false);expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(sessionStorage.getItem(sidecar!.key)).toBe(sidecar!.value);
 await act(async()=>recovery().click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].path).toBe(original.path);expect(writes()[1].body).toEqual(original.body);expect(writes()[1].body).toMatchObject({expected_version:1,option_id:'allow-one'});expect(saved.size).toBe(1);expect(saved.get(operation)).toMatchObject({attempt:2,fence:3,state:'decided',decision:{option_id:'allow-one'}});
 expect(sessionStorage.getItem(journal.key)).toBeNull();expect(sessionStorage.getItem(sidecar!.key)).toBeNull();expect(host.querySelector('[data-acp-permission-recover]')).toBeNull();expect(host.textContent).toContain('Allow this exact patch');expect(host.querySelector('[data-acp-permission="permission-a"] [data-state="decided"]')).not.toBeNull();
});

it('clears current denied content and ignores a retired node and actor response without clearing the public run',async()=>{
 await render();await open();expect(host.textContent).toContain('Review the exact file change');
 reply=request=>request.method==='POST'?Promise.resolve(Response.json({error:{code:'forbidden',message:'Decision authority changed'}},{status:403})):normal(request);await clickOption('allow-one');expect(host.querySelector('[data-acp-permission]')).toBeNull();
 reply=normal;await refresh();expect(option('allow-one').disabled).toBe(true);expect(recovery().disabled).toBe(false);expect(writes()).toHaveLength(1);expect(saved.size).toBe(0);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Authority changed'}},{status:403});await refresh();expect(host.querySelector('[data-acp-permission]')).toBeNull();expect(host.querySelector('[data-public-run]')).not.toBeNull();
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await refresh();const old=items;
 items=[];reply=normal;const identity={...session,user:{...session.user,id:'user-b'},organization:{...session.organization,id:'org-b'}};callerOverride.current=bindApiView(identity);await render({identity,scope:{...initial,nodeID:'node-b',attempt:3,fence:4}});await act(async()=>finish(Response.json({items:old})));await settle();await open();
 expect(host.querySelector('[data-acp-permission]')).toBeNull();expect(host.querySelector('[data-public-run]')?.textContent).toBe('Original run remains available');expect(writes()).toHaveLength(1);
});

it('retains a CAS conflict when its explicit rebase GET fails without issuing another decision',async()=>{
 reply=request=>request.method==='POST'?Promise.resolve(Response.json({error:{code:'version_conflict',message:'Attempt changed'}},{status:409})):normal(request);await render();await open();await clickOption('allow-one');expect(host.querySelector('.conflict-banner')).not.toBeNull();expect(option('allow-one').disabled).toBe(true);
 reply=async()=>Response.json({error:{code:'unavailable',message:'Read unavailable'}},{status:503});const rebase=host.querySelector<HTMLButtonElement>('.conflict-banner button');expect(rebase).not.toBeNull();await act(async()=>rebase!.click());await settle();
 expect(host.querySelector('.conflict-banner')).not.toBeNull();expect(option('allow-one').disabled).toBe(true);expect(writes()).toHaveLength(1);
});

it('does not send an unjournalled decision and retains a mismatched success receipt for explicit recovery',async()=>{
 await render();await open();const persist=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('Storage full');});await clickOption('allow-one');expect(writes()).toHaveLength(0);persist.mockRestore();
 reply=async request=>{const response=await normal(request);if(request.method==='POST'){const value=await response.json();return Response.json({...value,decision:{...value.decision,actor_id:'different-user'}});}return response;};
 await clickOption('allow-one');expect(writes()).toHaveLength(1);const journal=journalFor(String(writes()[0].body!.idempotency_key));expect(recovery()).not.toBeNull();await refresh();expect(recovery()).not.toBeNull();expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(onChanged).not.toHaveBeenCalled();
});

it('keeps active background reads single-flight and stops them when the original node finishes',async()=>{
 vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');await render();await open();let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();const reads=requests.length;expect(reads).toBeGreaterThan(1);
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();expect(requests).toHaveLength(reads);
 await act(async()=>finish(Response.json({items})));await settle();await render({state:'succeeded'});const stopped=requests.length;
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();expect(requests).toHaveLength(stopped);expect(writes()).toHaveLength(0);
},10000);
