// @vitest-environment jsdom
import {act} from 'react';
import {createHash,webcrypto} from 'node:crypto';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolProfile} from '../../../packages/contracts/src';
import type {ApiCaller,Session} from './api';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {ToolReinspectionPanel,type ToolReinspectionPanelProps} from './ToolReinspectionPanel';

const override=vi.hoisted(()=>({caller:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async load=>{const actual=await load<typeof import('./api-context')>();return {...actual,useApi:()=>{const current=actual.useApi();return override.caller??current;}};});
const session:Session={user:{id:'user-a',name:'Person',email:'person@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const tool:ToolProfile={id:'tool-a',project_id:'p',worker_id:'worker-a',name:'Team Codex',version:3,state:'paused',profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Profile paused',maintenance:{state:'supported',protocol:'suma-tool-maintenance-v1',instance_id:'durable-instance',checked_at:'2026-11-01T00:00:20Z',observed_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:01:00Z'}};
const pending={id:'inspection-a',project_id:'p',worker_id:'worker-a',tool_profile_id:'tool-a',tool_profile_version:3,profile_sha256:'a'.repeat(64),command_id:'codex-team',version:1,fence:0,state:'pending',requested_by:'user-a',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:05:00Z'};
const completed={...pending,version:3,fence:1,state:'completed',claim_id:'claim-original',claimed_at:'2026-11-01T00:00:10Z',lease_until:'2026-11-01T00:01:10Z',observed_at:'2026-11-01T00:00:12Z',inventory_operation_id:'inventory-original',result_code:'reported_unavailable'};
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
let root:Root,host:HTMLDivElement,requests:Request[],items:unknown[],reply:(request:Request)=>Promise<Response>,readStartedAt:number;
const locked=vi.fn(),observed=vi.fn(),denied=vi.fn(),deferred=vi.fn();
async function normal(request:Request):Promise<Response>{if(request.path.includes('/recoveries'))return Response.json({items:[]});if(request.method==='POST'){items=[pending];return Response.json(pending);}return Response.json(request.path.startsWith('/tool-inspections/')?items[0]:{items});}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();override.caller=undefined;locked.mockReset();observed.mockReset();denied.mockReset();deferred.mockReset();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);readStartedAt=performance.now();items=[];requests=[];reply=normal;host=document.createElement('div');document.body.append(host);root=createRoot(host);vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const body=init?.body?JSON.parse(String(init.body)) as Record<string,unknown>:undefined,request={path:String(input).replace(/^\/api\/v1/,''),method:init?.method||(body?'POST':'GET'),body,headers:new Headers(init?.headers)};requests.push(request);return reply(request);}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();override.caller=undefined;vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(done=>setTimeout(done,0));});}
async function render(props:Partial<ToolReinspectionPanelProps>={}){await act(async()=>root.render(<ApiViewProvider session={session}><ToolReinspectionPanel project="p" tool={tool} maintenanceReadStartedAt={readStartedAt} {...props} onLockChange={locked} onObserved={observed} onAccessDenied={denied} onDefer={deferred}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
const writes=()=>requests.filter(request=>request.method==='POST');
const state=()=>host.querySelector('[data-tool-reinspection-state]')?.getAttribute('data-tool-reinspection-state');
function journals(){return Array.from({length:sessionStorage.length},(_,i)=>{const key=sessionStorage.key(i)!;return {key,value:sessionStorage.getItem(key)!};}).filter(item=>item.key.includes('reinspection'));}

it('T194 requests an exact target explicitly and separates saved request, unknown machine result and inventory acceptance',async()=>{
 await render();expect(requests[0].path).toBe('/tool-profiles/tool-a/inspections?project_id=p');expect(writes()).toHaveLength(0);
 await click('[data-tool-reinspection-create]');expect(writes()).toHaveLength(1);expect(writes()[0].body).toEqual({project_id:'p',expected_version:3,idempotency_key:expect.any(String)});expect(writes()[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(state()).toBe('pending');expect(observed).not.toHaveBeenCalled();expect(locked).toHaveBeenLastCalledWith(false);
 items=[{...completed,state:'unknown',observed_at:undefined,inventory_operation_id:undefined,result_code:'probe_timeout'}];await click('[data-tool-reinspection-refresh]');expect(state()).toBe('unknown');expect(host.querySelector<HTMLButtonElement>('[data-tool-reinspection-create]')?.disabled).toBe(true);expect(writes()).toHaveLength(1);
 expect(host.querySelector('[data-tool-reinspection-create]')?.classList.contains('primary')).toBe(false);
 let finish!:(r:Response)=>void;reply=request=>request.method==='POST'&&request.path.endsWith('/recoveries')?new Promise(done=>{finish=done;}):normal(request);await click('[data-tool-recovery-authorize]');await click('[data-tool-recovery-confirm]');expect(writes()).toHaveLength(2);expect(host.querySelector<HTMLButtonElement>('[data-tool-reinspection-defer]')?.disabled).toBe(true);await click('[data-tool-reinspection-defer]');expect(deferred).not.toHaveBeenCalled();
 const original=completed,source=createHash('sha256').update(JSON.stringify({ID:original.id,ProjectID:'p',WorkerID:'worker-a',ToolProfileID:'tool-a',ToolProfileVersion:3,ProfileSHA256:'a'.repeat(64),CommandID:'codex-team',RequestedBy:'user-a',CreatedAt:original.created_at,ExpiresAt:original.expires_at,ClaimID:original.claim_id,ClaimedAt:original.claimed_at,LeaseUntil:original.lease_until,Fence:1})).digest('hex');
 await act(async()=>finish(Response.json({id:'recovery-a',project_id:'p',worker_id:'worker-a',inspection:{inspection_id:original.id,claim_id:original.claim_id,fence:1},inspection_version:3,source_sha256:source,version:1,state:'requested',requested_by:'user-a',created_at:'2026-11-01T00:02:00Z',expires_at:'2026-11-01T00:07:00Z'})));await settle();
 items=[completed];await click('[data-tool-reinspection-refresh]');expect(state()).toBe('completed');expect(observed).not.toHaveBeenCalled();await click('[data-tool-reinspection-review]');expect(observed).toHaveBeenCalledOnce();expect(writes()).toHaveLength(2);expect(host.querySelector<HTMLDetailsElement>('[data-tool-reinspection-source]')?.open).toBe(false);
});

it('T194 restores the original create body and key before using a changed profile and does not replay an accepted receipt on cleanup failure',async()=>{
 await render();let lost=true;reply=async request=>{const response=await normal(request);if(request.method==='POST'&&lost){lost=false;throw TypeError('Original create ACK lost');}return response;};await click('[data-tool-reinspection-create]');const original=writes()[0];expect(journals()).toHaveLength(2);expect(locked).toHaveBeenLastCalledWith(true);expect(host.querySelector<HTMLButtonElement>('[data-tool-reinspection-defer]')?.disabled).toBe(false);
 const changed={...tool,version:4,worker_id:'worker-b',profile:{...tool.profile,profile_sha256:'c'.repeat(64)}};await act(async()=>root.unmount());root=createRoot(host);await render({tool:changed,disabled:true});expect(host.textContent).toContain('worker-a');expect(writes()).toHaveLength(1);expect(host.querySelector<HTMLButtonElement>('[data-tool-reinspection-restore]')?.disabled).toBe(false);
 const metadata=journals().find(item=>item.key.startsWith('suma-tool-reinspection-intent:'))!;sessionStorage.setItem(metadata.key,'{"corrupt":true}');await click('[data-tool-reinspection-restore]');expect(writes()).toHaveLength(1);sessionStorage.setItem(metadata.key,metadata.value);await click('[data-tool-reinspection-refresh]');
 const remove=Storage.prototype.removeItem,cleanup=vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,key:string){if(key.startsWith('suma-write-recovery-v1:'))throw Error('Fixture cleanup blocked');return remove.call(this,key);});
 await click('[data-tool-reinspection-restore]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(journals()).toHaveLength(2);
 cleanup.mockRestore();await click('[data-tool-reinspection-restore]');expect(writes()).toHaveLength(2);expect(journals()).toHaveLength(0);expect(observed).not.toHaveBeenCalled();
 // The shared write journal can be safely consumed while removal of the
 // independent source metadata fails. A later view must retain that source.
 await act(async()=>root.unmount());root=createRoot(host);items=[];reply=normal;await render();
 const metadataCleanup=vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,key:string){if(key.startsWith('suma-tool-reinspection-intent:'))throw Error('Fixture metadata cleanup blocked');return remove.call(this,key);});
 await click('[data-tool-reinspection-create]');const originalMetadataOnly=writes().at(-1)!,metadataOnly=journals();expect(metadataOnly).toHaveLength(1);expect(metadataOnly[0].key).toContain('suma-tool-reinspection-intent:');metadataCleanup.mockRestore();
 await act(async()=>root.unmount());root=createRoot(host);reply=async request=>request.method==='POST'?Response.json({error:{code:'forbidden',message:'Current restore access denied'}},{status:403}):normal(request);await render({disabled:true});const writesBeforeRestore=writes().length;
 await click('[data-tool-reinspection-restore]');expect(writes()).toHaveLength(writesBeforeRestore+1);expect(writes().at(-1)!.body).toEqual(originalMetadataOnly.body);expect(journals()).toEqual(metadataOnly);expect(host.querySelector('[data-tool-reinspection-restore]')).toBeNull();
 await click('[data-tool-reinspection-defer]');expect(deferred).toHaveBeenCalledOnce();expect(journals()).toEqual(metadataOnly);expect(writes()).toHaveLength(writesBeforeRestore+1);
 // A controlled close is unavailable while a new scope's actual POST is in flight.
 const otherTool={...tool,id:'tool-b'};items=[];let finish!:(value:Response)=>void;reply=request=>request.method==='POST'?new Promise(done=>{finish=done;}):normal(request);await render({tool:otherTool});await click('[data-tool-reinspection-create]');
 expect(host.querySelector<HTMLButtonElement>('[data-tool-reinspection-defer]')?.disabled).toBe(true);await click('[data-tool-reinspection-defer]');expect(deferred).toHaveBeenCalledOnce();
 items=[{...pending,id:'inspection-b',tool_profile_id:'tool-b'}];await act(async()=>finish(Response.json(items[0])));await settle();expect(journals()).toEqual(metadataOnly);
});

it('T194 isolates late caller reads, latches current access denial and refuses writes when durable staging fails',async()=>{
 let finish!:(response:Response)=>void;reply=()=>new Promise(done=>{finish=done;});await render();override.caller=bindApiView(session);reply=normal;await render();await act(async()=>finish(Response.json({items:[completed]})));await settle();expect(state()).not.toBe('completed');
 const store=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('Fixture storage unavailable');});await click('[data-tool-reinspection-create]');expect(writes()).toHaveLength(0);store.mockRestore();await click('[data-tool-reinspection-refresh]');
 reply=async()=>Response.json({error:{code:'forbidden',message:'Current inspection access denied'}},{status:403});await click('[data-tool-reinspection-refresh]');expect(host.querySelector('[data-tool-reinspection-source]')).toBeNull();expect(denied).toHaveBeenCalledOnce();
 let finishRetry!:(response:Response)=>void;reply=()=>new Promise(done=>{finishRetry=done;});await click('[data-tool-reinspection-refresh]');expect(host.textContent).not.toContain('worker-a');await act(async()=>finishRetry(Response.json({error:{code:'unavailable',message:'Temporary error'}},{status:503})));await settle();expect(host.textContent).not.toContain('worker-a');
 reply=normal;await click('[data-tool-reinspection-refresh]');expect(host.textContent).toContain('worker-a');await render({accessBlocked:true});expect(host.textContent).not.toContain('worker-a');expect(writes()).toHaveLength(0);
 const corruptKey='suma-tool-reinspection-intent:'+JSON.stringify(['org-a','user-a','p','tool-a']);sessionStorage.setItem(corruptKey,'not valid JSON');await act(async()=>root.unmount());root=createRoot(host);await render({accessBlocked:true});expect(locked).toHaveBeenLastCalledWith(true);
 const raw=journals();await click('[data-tool-reinspection-defer]');expect(deferred).toHaveBeenCalledOnce();expect(journals()).toEqual(raw);expect(writes()).toHaveLength(0);
});

it('T199 reads the explicitly located inspection and recovery while preserving an original local receipt before another locator',async()=>{
 const {observed_at:_observed,inventory_operation_id:_inventory,...source}=completed,unknown={...source,state:'unknown',result_code:'probe_timeout'};
 const sourceSHA=createHash('sha256').update(JSON.stringify({ID:unknown.id,ProjectID:'p',WorkerID:'worker-a',ToolProfileID:'tool-a',ToolProfileVersion:3,ProfileSHA256:'a'.repeat(64),CommandID:'codex-team',RequestedBy:'user-a',CreatedAt:unknown.created_at,ExpiresAt:unknown.expires_at,ClaimID:unknown.claim_id,ClaimedAt:unknown.claimed_at,LeaseUntil:unknown.lease_until,Fence:1})).digest('hex');
 reply=async request=>request.path.startsWith('/tool-inspection-recoveries/recovery-original?')?Response.json({id:'recovery-original',project_id:'p',worker_id:'worker-a',inspection:{inspection_id:unknown.id,claim_id:unknown.claim_id,fence:1},inspection_version:unknown.version,source_sha256:sourceSHA,version:1,state:'requested',requested_by:'user-a',created_at:'2026-11-01T00:02:00Z',expires_at:'2026-11-01T00:07:00Z'}):request.path.startsWith('/tool-inspections/inspection-a?')?Response.json(unknown):Response.json({items:[pending]});
 await render({locator:{inspectionID:unknown.id,recoveryID:'recovery-original'}});
 expect(requests[0].path).toBe('/tool-inspections/inspection-a?project_id=p');
 expect(requests.some(request=>request.path==='/tool-inspection-recoveries/recovery-original?project_id=p')).toBe(true);expect(state()).toBe('unknown');expect(host.querySelector('[data-tool-recovery-state]')?.getAttribute('data-tool-recovery-state')).toBe('requested');expect(writes()).toEqual([]);
 await act(async()=>root.unmount());root=createRoot(host);requests=[];
 const scope={project:'p',toolID:'tool-a',userID:'user-a',organizationID:'org-a'},request={path:'/tool-profiles/tool-a/inspections',method:'POST',idempotency_key:'original-199-operation',body:{project_id:'p',expected_version:3,idempotency_key:'original-199-operation'}},base=JSON.stringify(['org-a','user-a','p','tool-a']),metadata=JSON.stringify({scope,tool,request});
 sessionStorage.setItem('suma-tool-reinspection-intent:'+base,metadata);sessionStorage.setItem('suma-write-recovery-v1:tool-reinspection:'+base,JSON.stringify(request));
 reply=normal;items=[pending];await render({locator:{inspectionID:'another-server-inspection'},disabled:true});
 expect(host.querySelector('[data-tool-reinspection-restore]')).not.toBeNull();expect(requests.some(item=>item.path.includes('another-server-inspection'))).toBe(false);expect(writes()).toEqual([]);
 expect(sessionStorage.getItem('suma-tool-reinspection-intent:'+base)).toBe(metadata);
});
