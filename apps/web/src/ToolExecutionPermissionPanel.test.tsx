// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolPermission} from '../../../packages/contracts/src';
import {ApiViewProvider,bindApiView} from './api-context';
import type {ApiCaller,Session} from './api';
import {setLocale} from './i18n';
import {ToolExecutionPermissionPanel} from './ToolExecutionPermissionPanel';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async importOriginal=>{const actual=await importOriginal<typeof import('./api-context')>();return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};});
const session:Session={user:{id:'user-a',name:'Reviewer',email:'reviewer@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const initial={project:'project-a',runID:'run-a',nodeID:'node-a',attempt:2,fence:3,protocol:'acp-stdio-v1' as 'acp-stdio-v1'|'codex-app-server-v1'};
const listPath='/runs/run-a/nodes/node-a/tool-permissions?project_id=project-a';
const decisionPath='/tool-permissions/permission-a/decision';
function permission():ToolPermission{return {id:'permission-a',project_id:initial.project,run_id:initial.runID,node_id:initial.nodeID,attempt:2,fence:3,dispatch_id:'dispatch-a',worker_id:'worker-a',profile_sha256:'a'.repeat(64),version:1,state:'pending',request:{idempotency_key:'runner-original-request',protocol:'acp-stdio-v1',acp:{session_id:'session-a',tool_call_id:'tool-call-a',title:'Review the exact file change',kind:'edit',details:'Change only the approved file <script>data</script>',details_complete:true,options:[{id:'allow-one',name:'Allow this exact patch',kind:'allow_once'},{id:'allow-specific',name:'Allow only the reviewed file',kind:'allow_once'},{id:'reject-one',name:'Reject this exact patch',kind:'reject_once'},{id:'allow-always',name:'Always allow changes',kind:'allow_always'}]}},created_at:'2026-10-31T00:00:00Z',expires_at:'2099-01-01T00:00:00Z',allowed_actions:['allow_once','reject_once']};}
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
type Reply=(request:Request)=>Promise<Response>;
let root:Root,host:HTMLDivElement,items:ToolPermission[],requests:Request[],reply:Reply,onChanged:ReturnType<typeof vi.fn>,saved:Map<string,ToolPermission>;
async function normal(request:Request):Promise<Response>{
 if(request.method==='GET'&&request.path===listPath+'&permission_id=permission-a')return Response.json({items:[...saved.values()].filter(item=>item.id==='permission-a')});
 if(request.method==='GET')return Response.json({items});
 const body=request.body!,key=String(body.idempotency_key);let result=saved.get(key);
 if(!result){const original=items.find(item=>request.path==='/tool-permissions/'+item.id+'/decision')!;const selected=original.request.protocol==='acp-stdio-v1'?{protocol:'acp-stdio-v1' as const,acp:body.acp as NonNullable<ToolPermission['decision']>['acp']}:{protocol:'codex-app-server-v1' as const,codex:body.codex as NonNullable<ToolPermission['decision']>['codex']};result={...original,version:original.version+1,state:'decided',allowed_actions:[],decision:{...selected,actor_id:session.user.id,decided_at:'2026-10-31T00:01:00Z'}};saved.set(key,result);items=items.map(item=>item.id===result!.id?result!:item);}
 return Response.json(result);
}
beforeEach(async()=>{
 callerOverride.current=undefined;await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();items=[permission()];requests=[];saved=new Map();reply=normal;onChanged=vi.fn();host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const body:Record<string,unknown>|undefined=init?.body?JSON.parse(String(init.body)):undefined,request={path:String(input).replace(/^\/api\/v1/,''),method:init?.method||(body?'POST':'GET'),body,headers:new Headers(init?.headers)};requests.push(request);return reply(request);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render({identity=session,scope=initial,revision=0,state='running',readOnly=false}={}){await act(async()=>root.render(<ApiViewProvider session={identity}><p data-public-run>Original run remains available</p><ToolExecutionPermissionPanel {...scope} readOnly={readOnly} state={state} revision={revision} onChanged={onChanged}/></ApiViewProvider>));await settle();}
function toggle(){const button=host.querySelector<HTMLButtonElement>('.tool-permissions-toggle');expect(button).not.toBeNull();return button!;}
async function open(){if(toggle().getAttribute('aria-expanded')!=='true'){await act(async()=>toggle().click());await settle();}}
async function refresh(){const button=host.querySelector<HTMLButtonElement>('.tool-permissions-refresh');expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
function option(id:string){const button=host.querySelector<HTMLButtonElement>('[data-tool-permission="permission-a"] [data-tool-option="'+id+'"]');expect(button,id).not.toBeNull();return button!;}
async function clickOption(id:string){await act(async()=>option(id).click());await settle();}
const writes=()=>requests.filter(request=>request.method==='POST');
function recovery(){const button=host.querySelector<HTMLButtonElement>('[data-tool-permission-recover]');expect(button).not.toBeNull();return button!;}
function journalFor(operation:string){for(let index=0;index<sessionStorage.length;index++){const key=sessionStorage.key(index)!,value=sessionStorage.getItem(key)!;try{if(JSON.parse(value).idempotency_key===operation)return {key,value};}catch{/* Not an operation record. */}}throw new Error('Original decision journal missing');}

it('reads a collapsed summary then sends exactly one original named option with the current view and CAS basis',async()=>{
 await render();expect(toggle().getAttribute('aria-expanded')).toBe('false');expect(requests.map(request=>request.path)).toEqual([listPath]);expect(host.querySelector('.tool-permissions-count')?.textContent).toBe('1');await open();
 expect(host.textContent).toContain(items[0].request.acp!.details);expect(host.querySelector('script')).toBeNull();expect(option('allow-one').textContent).toContain('Allow this exact patch');expect(option('allow-specific').textContent).toContain('Allow only the reviewed file');expect(option('allow-always').disabled).toBe(true);expect(writes()).toHaveLength(0);
 const chosen=option('allow-specific');await act(async()=>{chosen.click();chosen.click();});await settle();expect(writes()).toHaveLength(1);
 expect(writes()[0].path).toBe(decisionPath);expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,protocol:'acp-stdio-v1',acp:{option_id:'allow-specific',kind:'allow_once'},idempotency_key:expect.any(String)});expect(writes()[0].body!.idempotency_key).not.toBe(items[0].request.idempotency_key);
 expect(writes()[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(writes()[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(host.querySelector('[data-tool-permission="permission-a"] [data-state="decided"]')).not.toBeNull();expect(onChanged).toHaveBeenCalledOnce();
});

it('keeps incomplete details unapprovable while allowing the exact one-time rejection',async()=>{
 items=[{...permission(),request:{...permission().request,acp:{...permission().request.acp!,details_complete:false}},allowed_actions:['reject_once']}];await render();await open();expect(option('allow-one').disabled).toBe(true);expect(option('allow-specific').disabled).toBe(true);expect(option('allow-always').disabled).toBe(true);expect(option('reject-one').disabled).toBe(false);
 await clickOption('reject-one');expect(writes()).toHaveLength(1);expect(writes()[0].body).toMatchObject({acp:{option_id:'reject-one',kind:'reject_once'}});expect(saved.values().next().value?.decision?.acp?.kind).toBe('reject_once');
});

it('retains an unknown decision across collapse and remount and never substitutes a GET for the original receipt',async()=>{
 let lost=true;reply=async request=>{if(request.method==='POST'&&lost){lost=false;await normal(request);throw new Error('Decision response lost after commit');}return normal(request);};
 await render();await open();await clickOption('allow-one');const original=writes()[0],operation=String(original.body!.idempotency_key),journal=journalFor(operation);
 const sidecar=Array.from({length:sessionStorage.length},(_,index)=>{const key=sessionStorage.key(index)!;return {key,value:sessionStorage.getItem(key)!};}).find(record=>{try{return JSON.parse(record.value).request?.idempotency_key===operation;}catch{return false;}});
 expect(sidecar).toBeDefined();expect(JSON.parse(sidecar!.value)).toMatchObject({scope:{project:'project-a',runID:'run-a',nodeID:'node-a',attempt:2,fence:3,userID:'user-a',organizationID:'org-a'},permission_id:'permission-a',profile_sha256:'a'.repeat(64),dispatch_id:'dispatch-a',worker_id:'worker-a',request:{path:decisionPath,body:original.body}});
 await act(async()=>toggle().click());await settle();items=[];const next={...initial,attempt:3,fence:4};await render({scope:next,revision:1});
 expect(writes()).toHaveLength(1);expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(sessionStorage.getItem(sidecar!.key)).toBe(sidecar!.value);
 await act(async()=>root.unmount());root=createRoot(host);await render({scope:next,revision:2});await open();await refresh();
 expect(requests.some(request=>request.method==='GET'&&request.path===listPath+'&permission_id=permission-a')).toBe(true);expect(writes()).toHaveLength(1);expect(recovery().disabled).toBe(false);expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(sessionStorage.getItem(sidecar!.key)).toBe(sidecar!.value);
 await act(async()=>recovery().click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].path).toBe(original.path);expect(writes()[1].body).toEqual(original.body);expect(writes()[1].body).toMatchObject({expected_version:1,protocol:'acp-stdio-v1',acp:{option_id:'allow-one',kind:'allow_once'}});expect(saved.size).toBe(1);expect(saved.get(operation)).toMatchObject({attempt:2,fence:3,state:'decided',decision:{protocol:'acp-stdio-v1',acp:{option_id:'allow-one',kind:'allow_once'}}});
 expect(sessionStorage.getItem(journal.key)).toBeNull();expect(sessionStorage.getItem(sidecar!.key)).toBeNull();expect(host.querySelector('[data-tool-permission-recover]')).toBeNull();expect(host.textContent).toContain('Allow this exact patch');expect(host.querySelector('[data-tool-permission="permission-a"] [data-state="decided"]')).not.toBeNull();
});

it('clears current denied content and ignores a retired node and actor response without clearing the public run',async()=>{
 await render();await open();expect(host.textContent).toContain('Review the exact file change');
 reply=request=>request.method==='POST'?Promise.resolve(Response.json({error:{code:'forbidden',message:'Decision authority changed'}},{status:403})):normal(request);await clickOption('allow-one');expect(host.querySelector('[data-tool-permission]')).toBeNull();
 reply=normal;await refresh();expect(option('allow-one').disabled).toBe(true);expect(recovery().disabled).toBe(false);expect(writes()).toHaveLength(1);expect(saved.size).toBe(0);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Authority changed'}},{status:403});await refresh();expect(host.querySelector('[data-tool-permission]')).toBeNull();expect(host.querySelector('[data-public-run]')).not.toBeNull();
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await refresh();const old=items;
 items=[];reply=normal;const identity={...session,user:{...session.user,id:'user-b'},organization:{...session.organization,id:'org-b'}};callerOverride.current=bindApiView(identity);await render({identity,scope:{...initial,nodeID:'node-b',attempt:3,fence:4}});await act(async()=>finish(Response.json({items:old})));await settle();await open();
 expect(host.querySelector('[data-tool-permission]')).toBeNull();expect(host.querySelector('[data-public-run]')?.textContent).toBe('Original run remains available');expect(writes()).toHaveLength(1);
});

it('retains a CAS conflict when its explicit rebase GET fails without issuing another decision',async()=>{
 reply=request=>request.method==='POST'?Promise.resolve(Response.json({error:{code:'version_conflict',message:'Attempt changed'}},{status:409})):normal(request);await render();await open();await clickOption('allow-one');expect(host.querySelector('.conflict-banner')).not.toBeNull();expect(option('allow-one').disabled).toBe(true);
 reply=async()=>Response.json({error:{code:'unavailable',message:'Read unavailable'}},{status:503});const rebase=host.querySelector<HTMLButtonElement>('.conflict-banner button');expect(rebase).not.toBeNull();await act(async()=>rebase!.click());await settle();
 expect(host.querySelector('.conflict-banner')).not.toBeNull();expect(option('allow-one').disabled).toBe(true);expect(writes()).toHaveLength(1);
 expect(host.querySelector('[data-tool-permission-recover]')).toBeNull();const original=writes()[0];items=[{...permission(),id:'permission-b',request:{...permission().request,idempotency_key:'another-machine-request',acp:{...permission().request.acp!,tool_call_id:'tool-call-b'}}}];reply=normal;await act(async()=>host.querySelector<HTMLButtonElement>('.conflict-banner button')!.click());await settle();expect(host.querySelector('.conflict-banner')).toBeNull();const next=host.querySelector<HTMLButtonElement>('[data-tool-permission="permission-b"] [data-tool-option="allow-one"]')!;expect(next.disabled).toBe(false);await act(async()=>next.click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].path).toBe('/tool-permissions/permission-b/decision');expect(writes()[1].body!.idempotency_key).not.toBe(original.body!.idempotency_key);expect(writes().filter(request=>request.path===decisionPath)).toHaveLength(1);
});

it('does not send an unjournalled decision and retains a mismatched success receipt for explicit recovery',async()=>{
 await render();await open();const persist=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('Storage full');});await clickOption('allow-one');expect(writes()).toHaveLength(0);persist.mockRestore();
 reply=async request=>{const response=await normal(request);if(request.method==='POST'){const value=await response.json();return Response.json({...value,decision:{...value.decision,actor_id:'different-user'}});}return response;};
 await clickOption('allow-one');expect(writes()).toHaveLength(1);const journal=journalFor(String(writes()[0].body!.idempotency_key));expect(recovery()).not.toBeNull();await refresh();expect(recovery()).not.toBeNull();expect(sessionStorage.getItem(journal.key)).toBe(journal.value);expect(onChanged).not.toHaveBeenCalled();
});

it('keeps active background reads single-flight and stops them when the original node finishes',async()=>{
 vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');await render();await open();const focused=option('allow-one');focused.focus();let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();const reads=requests.length;expect(reads).toBeGreaterThan(1);
 expect(focused.disabled).toBe(false);expect(document.activeElement).toBe(focused);
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();expect(requests).toHaveLength(reads);
 await act(async()=>finish(Response.json({items})));await settle();await render({state:'succeeded'});const stopped=requests.length;
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,2100));});await settle();expect(requests).toHaveLength(stopped);expect(writes()).toHaveLength(0);
},10000);

function codexPermission():ToolPermission{return {...permission(),request:{idempotency_key:'codex-original-request',protocol:'codex-app-server-v1',codex:{request_id:{kind:'number',value:'9007199254740993'},method:'item/commandExecution/requestApproval',thread_id:'thread-original',turn_id:'turn-original',item_id:'item-original',started_at_ms:1793404800000,binding_sha256:'b'.repeat(64),params_sha256:'c'.repeat(64),item_sha256:'d'.repeat(64),title:'Run the exact verification',details:'Run the reviewed command only',details_complete:true,scope_supported:true,command:'go test ./internal/example',cwd:'.'}},allowed_actions:['accept','decline','cancel']};}
it('sends a native Codex decision once without inventing ACP options and restores focus when its buttons disappear',async()=>{
 items=[codexPermission()];await render({scope:{...initial,protocol:'codex-app-server-v1'}});expect(writes()).toHaveLength(0);await open();const accept=option('accept');accept.focus();await act(async()=>{accept.click();accept.click();});await settle();expect(writes()).toHaveLength(1);expect(writes()[0].body).toEqual({project_id:'project-a',expected_version:1,idempotency_key:expect.any(String),protocol:'codex-app-server-v1',codex:{decision:'accept'}});expect(writes()[0].body).not.toHaveProperty('acp');expect(document.activeElement).toBe(toggle());expect(host.querySelector('[data-tool-observation]')).toBeNull();expect(host.textContent).toContain('Decision saved');
});
it('keeps unsupported or incomplete Codex scope unapprovable while showing exact details and decline/cancel',async()=>{
 const item=codexPermission();item.request.codex={...item.request.codex!,method:'item/fileChange/requestApproval',changes:[{path:'src/old.ts',kind:'update',move_path:'src/new.ts',diff:'@@ exact diff <script>data</script>'}],scope_supported:false,unsupported_reason:'network_policy',details_complete:false};delete item.request.codex.command;delete item.request.codex.cwd;item.allowed_actions=['decline','cancel'];items=[item];await render({scope:{...initial,protocol:'codex-app-server-v1'}});await open();expect(option('accept').disabled).toBe(true);expect(option('decline').disabled).toBe(false);expect(option('cancel').disabled).toBe(false);const details=host.querySelector<HTMLDetailsElement>('[data-tool-source]')!;expect(details.open).toBe(false);expect(details.textContent).toContain('9007199254740993');expect(details.textContent).toContain('network_policy');expect(details.textContent).toContain('@@ exact diff <script>data</script>');expect(host.querySelector('script')).toBeNull();expect(host.querySelector('.tool-permission-impact')?.textContent).toContain('src/new.ts');await clickOption('cancel');expect(writes()[0].body).toMatchObject({codex:{decision:'cancel'}});expect(host.textContent).not.toContain('Remote process stopped');
});
it('recovers a lost native decision with the same body and shows separate observation facts and unknown while collapsed',async()=>{
 items=[codexPermission()];let lost=true;reply=async request=>{const response=await normal(request);if(request.method==='POST'&&lost){lost=false;throw new Error('Lost after commit');}return response;};const scope={...initial,protocol:'codex-app-server-v1' as const};await render({scope});await open();await clickOption('accept');const original=writes()[0];await act(async()=>toggle().click());expect(recovery()).not.toBeNull();await act(async()=>recovery().click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);
 items=[{...items[0],observation:{sequence:1,params_sha256:'c'.repeat(64),decision:'accept',decision_observed:true,reply_started:true,reply_written:false,request_resolved:false}}];await refresh();expect(host.querySelector('[data-tool-observation-unknown]')).not.toBeNull();await open();const facts=host.querySelector('[data-tool-observation]')!;expect(facts.querySelector('[data-fact=decision_observed]')?.getAttribute('data-confirmed')).toBe('true');expect(facts.querySelector('[data-fact=reply_written]')?.getAttribute('data-confirmed')).toBe('false');expect(facts.querySelector('[data-fact=request_resolved]')?.getAttribute('data-confirmed')).toBe('false');
});
it('rejects a different protocol list and keeps historic attempts read-only',async()=>{
 items=[codexPermission()];await render();expect(host.querySelector('[data-tool-permission]')).toBeNull();expect(writes()).toHaveLength(0);await render({scope:{...initial,protocol:'codex-app-server-v1',attempt:3,fence:4}});await open();expect(option('accept').disabled).toBe(true);expect(option('decline').disabled).toBe(true);expect(writes()).toHaveLength(0);
 const scope={...initial,protocol:'codex-app-server-v1' as const};await render({scope,readOnly:true});expect(option('accept').disabled).toBe(true);expect(option('decline').disabled).toBe(true);expect(option('cancel').disabled).toBe(true);await clickOption('accept');expect(writes()).toHaveLength(0);
 await render({scope});let lost=true;reply=async request=>{const value=await normal(request);if(request.method==='POST'&&lost){lost=false;throw new Error('Original result lost');}return value;};await clickOption('decline');const original=writes()[0];await render({scope,readOnly:true});expect(recovery().disabled).toBe(false);await act(async()=>recovery().click());await settle();expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);
});
