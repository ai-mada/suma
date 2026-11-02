// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ExecutionPool,ExecutionPoolRunner} from '../../../../packages/contracts/src';
import {ApiViewProvider} from '../api-context';
import {setLocale} from '../i18n';
import {PoolWorkspace} from './PoolWorkspace';
import {PoolToolRecoveryPanel,type PoolToolRecoverySnapshot,type PoolToolTargetPin} from './PoolToolRecoveryPanel';
import {serializePoolToolRecoveryPin,type PoolToolRecoveryPin} from './pool-tool-recovery-model';
import {parsePoolToolSetupPin,serializePoolToolSetupPin,type PoolToolSetupPin} from './pool-tool-setup-model';

const tenant='a'.repeat(32),poolID='b'.repeat(32),runnerID='c'.repeat(32),project='d'.repeat(32),grantID='e'.repeat(32),stamp='2026-11-03T00:00:00Z';
const session={user:{id:'owner',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:tenant,name:'Team'},projects:[]};
const pool={id:poolID,owner_tenant_id:tenant,name:'Owned Pool',description:'',version:2,status:'active',allowed_actions:['pause','register_runner'],policy:{},created_at:stamp,updated_at:stamp} as ExecutionPool;
const runner:ExecutionPoolRunner={id:runnerID,pool_id:poolID,worker_id:'pool-'+runnerID,name:'Exact machine beyond first page',version:3,status:'active',max_slots:2,credential_version:2,credential_expires_at:'2026-12-03T00:00:00Z',allowed_actions:['rotate_credential','revoke'],created_at:stamp};
const pin:PoolToolRecoveryPin={pool_id:poolID,runner_id:runnerID,worker_id:runner.worker_id,tenant_id:tenant,project_id:project,tool_profile_id:'original-tool',tool_profile_version:4,profile_sha256:'f'.repeat(64),adapter:'codex',protocol:'codex-jsonl-v1'};
const setupPin:PoolToolSetupPin={pool_id:poolID,runner_id:runnerID,worker_id:runner.worker_id,tenant_id:tenant,project_id:project,adapter_id:'codex-jsonl',adapter:'codex',protocol:'codex-jsonl-v1'};
const reply=(projectID=project)=>({pool_id:poolID,runner_id:runnerID,worker_id:runner.worker_id,pool_status:'active',checked_at:stamp,runner:{status:runner.status,version:runner.version,credential_version:runner.credential_version,credential_state:'valid',credential_expires_at:runner.credential_expires_at,historical_last_seen_at:''},scope:{tenant_id:tenant,project_id:projectID,grant_id:grantID,grant_version:2,grant_status:'approved',shared_project_name:'Original project',can_open_project:true,route_state:'current',source_current:false,source_reason:'inventory_unreported',inventory_observed_at:'',inventory_expires_at:''}});
let root:Root,host:HTMLDivElement,reads:string[],writes:string[],snapshot:ReturnType<typeof reply>,node:ExecutionPoolRunner,deny:boolean,denyStatus:number,defer:boolean,releases:Array<(value:Response)=>void>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:vi.fn().mockResolvedValue(undefined)}});
 host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];writes=[];snapshot=reply();node=structuredClone(runner);deny=false;denyStatus=403;defer=false;releases=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://test.local'),path=url.pathname.replace('/api/v1','');
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'No write authorized'}},{status:409});}
  reads.push(path);
  if(path==='/execution-pools')return Response.json({items:[],total:0,allowed_actions:[]});
  if(path==='/execution-pools/'+poolID)return Response.json(pool);
  if(path==='/execution-pools/'+poolID+'/runners/'+runnerID)return deny?Response.json({error:{code:'forbidden',message:'Owner access denied'}},{status:denyStatus}):Response.json(node);
  if(path.endsWith('/readiness')){if(defer)return new Promise<Response>(resolve=>releases.push(resolve));return Response.json({...snapshot,scope:{...snapshot.scope,project_id:url.searchParams.get('project_id')!}});}
  if(path.endsWith('/grants')||path.endsWith('/runners'))return Response.json({items:[],total:0,allowed_actions:['register']});
  return Response.json({error:{code:'missing',message:'No fixture source'}},{status:404});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();document.body.innerHTML='';sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector)!;expect(button,selector).not.toBeNull();expect(button.disabled).not.toBe(true);await act(async()=>button.click());await settle();}
async function change(selector:string,value:string){const field=document.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;expect(field,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function workspace(panel=serializePoolToolRecoveryPin(pin)){await act(async()=>root.render(<ApiViewProvider session={session}><PoolWorkspace session={session} selected={poolID} panel={panel} refresh={0} onSelect={()=>{}} onPanel={()=>{}}/></ApiViewProvider>));await settle();}

it('T218 uses the exact owner-authorized runner and scope beyond collection pages, prepares only public IDs and returns the original tool pin',async()=>{
 await workspace();expect(reads).toContain('/execution-pools/'+poolID+'/runners/'+runnerID);
 await click('[data-pool-recovery-prepare]');expect(reads.filter(path=>path==='/execution-pools/'+poolID+'/runners/'+runnerID)).toHaveLength(2);
 expect(document.querySelector<HTMLSelectElement>('[data-pool-config-project=scope-1]')?.value).toBe(grantID);
 for(const key of ['tenant_id','project_id'])expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field='+key+']')?.value).toBe(key==='tenant_id'?tenant:project);
 for(const key of ['workspace','commands_file','repository','base_commit','worktrees','model_deployments_file','external_config_file'])expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field='+key+']')?.value).toBe('');
 await change('[data-pool-config-field=shell]','powershell');await change('[data-pool-config-field=output]','D:/owned/new-pool.json');await change('[data-pool-config-scope-field=workspace]','D:/owned/project');await click('[data-pool-config-copy]');await click('[data-pool-connection-refresh]');
 const href=document.querySelector<HTMLAnchorElement>('[data-pool-connection-tools]')!.getAttribute('href')!,query=new URLSearchParams(href.slice(1));expect(query.get('project')).toBe(project);expect(query.get('record')).toBe('original-tool');expect(query.get('panel')).toBe(serializePoolToolRecoveryPin(pin));expect(writes).toEqual([]);
 // Explicitly choosing another project keeps its original generic navigation.
 await click('[data-pool-config-add]');await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=tenant_id]',tenant);await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=project_id]','other-project');await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=workspace]','D:/owned/other');await click('[data-pool-config-copy]');await change('[data-pool-connection-field=scope]','1');await click('[data-pool-connection-refresh]');
 const other=new URLSearchParams(document.querySelector<HTMLAnchorElement>('[data-pool-connection-tools]')!.getAttribute('href')!.slice(1));expect(other.get('project')).toBe('other-project');expect(other.get('record')).toBeNull();expect(other.get('panel')).toBe('pool-tool:'+poolID+':'+runnerID);expect(writes).toEqual([]);
});

it('T218 keeps original unknown writes ahead of preparation and never treats project access as Pool ownership',async()=>{
 const key='suma-pool-recovery-v1:'+JSON.stringify([tenant,session.user.id,'organization',poolID]),original=JSON.stringify({path:'/execution-pools/'+poolID+'/commands',method:'POST',body:{action:'pause',expected_version:2},idempotency_key:'original-unconfirmed-operation'});sessionStorage.setItem(key,original);
 await workspace();expect(document.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);expect(reads.some(path=>path.endsWith('/readiness')||path.endsWith('/runners/'+runnerID))).toBe(false);expect(sessionStorage.getItem(key)).toBe(original);expect(writes).toEqual([]);
 await act(async()=>root.unmount());root=createRoot(host);sessionStorage.clear();deny=true;await workspace();expect(document.querySelector('[data-pool-recovery-state]')?.getAttribute('data-pool-recovery-state')).toBe('restricted');expect(document.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);expect(document.querySelector('[data-pool-config-preparation]')).toBeNull();expect(writes).toEqual([]);
});

it('T218 leaves revoked grants blocked and sends expired credentials only to the original explicit identity action',async()=>{
 const prepare=vi.fn(),manage=vi.fn(),observe=vi.fn();
 const draw=async(refresh:number)=>{await act(async()=>root.render(<ApiViewProvider session={session}><PoolToolRecoveryPanel pin={pin} pool={pool} refresh={refresh} onSnapshot={observe} onPrepare={prepare} onManage={manage}/></ApiViewProvider>));await settle();};
 snapshot.scope.grant_status='revoked';await draw(0);expect(document.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);expect(prepare).not.toHaveBeenCalled();
 snapshot=reply();node={...runner,credential_expires_at:'2026-11-02T00:00:00Z'};snapshot.runner={...snapshot.runner,credential_state:'expired',credential_expires_at:node.credential_expires_at};await draw(1);expect(document.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);await click('[data-pool-recovery-manage]');expect(manage).toHaveBeenCalledOnce();expect(manage.mock.calls[0][0].id).toBe(runnerID);expect(prepare).not.toHaveBeenCalled();expect(writes).toEqual([]);
});

it('T218 rejects late pin and actor ABA reads and consumes request latency before allowing preparation',async()=>{
 defer=true;const observed:Array<PoolToolRecoverySnapshot|undefined>=[],prepare=vi.fn();
 const draw=async(value=pin,active=session)=>{await act(async()=>root.render(<ApiViewProvider session={active}><PoolToolRecoveryPanel pin={value} pool={pool} refresh={0} onSnapshot={value=>observed.push(value)} onPrepare={prepare}/></ApiViewProvider>));await settle();};
 await draw();const old=releases[0];await draw({...pin,tool_profile_version:5});await draw();await act(async()=>old(Response.json(reply())));await settle();expect(observed.filter(Boolean)).toEqual([]);
 const beforeActor=releases.at(-1)!;await draw(pin,{...session,user:{...session.user,id:'other-owner'}});await draw();await act(async()=>beforeActor(Response.json(reply())));await settle();expect(observed.filter(Boolean)).toEqual([]);
 const clock=vi.spyOn(performance,'now');clock.mockReturnValue(1000);await draw({...pin,tool_profile_version:6});const late=releases.at(-1)!;clock.mockReturnValue(32000);await act(async()=>late(Response.json(reply())));await settle();expect(document.querySelector('[data-pool-recovery-state]')?.getAttribute('data-pool-recovery-state')).toBe('expired');expect(document.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);expect(observed.filter(Boolean)).toEqual([]);expect(prepare).not.toHaveBeenCalled();expect(writes).toEqual([]);clock.mockRestore();
});

it('T218 preserves same-scope private form inputs while a check expires and reenables preparation only after an explicit current reread',async()=>{
 const clock=vi.spyOn(performance,'now');clock.mockReturnValue(1000);
 await workspace();await click('[data-pool-recovery-prepare]');
 await change('[data-pool-config-field=shell]','powershell');await change('[data-pool-config-field=output]','D:/owned/preserved.json');
 const privateValues={workspace:'D:/owned/project',commands_file:'D:/owned/commands.json',repository:'D:/owned/repo',base_commit:'1'.repeat(40),worktrees:'D:/owned/trees',model_deployments_file:'D:/owned/models.json',external_config_file:'D:/owned/external.json'};
 for(const [field,value] of Object.entries(privateValues))await change('[data-pool-config-scope-field='+field+']',value);
 await click('[data-pool-config-copy]');expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();
 clock.mockReturnValue(32000);await act(async()=>{await new Promise(resolve=>setTimeout(resolve,1050));});await settle();
 const copy=document.querySelector<HTMLButtonElement>('[data-pool-config-copy]')!;expect(copy).not.toBeNull();expect(copy.disabled).toBe(true);expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
 expect(document.querySelector<HTMLSelectElement>('[data-pool-config-field=shell]')?.value).toBe('powershell');expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('D:/owned/preserved.json');
 for(const [field,value] of Object.entries(privateValues))expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field='+field+']')?.value).toBe(value);
 const before=reads.length;await click('[data-pool-recovery-form-refresh]');expect(reads.length-before).toBe(2);expect(document.querySelector<HTMLButtonElement>('[data-pool-config-copy]')?.disabled).toBe(false);
 for(const [field,value] of Object.entries(privateValues))expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field='+field+']')?.value).toBe(value);
 expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();await click('[data-pool-config-copy]');expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();expect(writes).toEqual([]);clock.mockRestore();
});

it('T219 prepares a new shared tool from the exact owner scope and returns its adapter pin without inventing an original profile',async()=>{
 await workspace(serializePoolToolSetupPin(setupPin));
 expect(host.querySelector('[data-pool-tool-intent=setup]')?.textContent).toContain('Connect a shared tool');expect(host.querySelector('[data-pool-tool-intent=setup]')?.textContent).toContain('codex-jsonl');
 await click('[data-pool-recovery-prepare]');
 expect(document.querySelector<HTMLSelectElement>('[data-pool-config-project=scope-1]')?.value).toBe(grantID);
 expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=project_id]')?.value).toBe(project);
 for(const key of ['workspace','commands_file','repository','base_commit','worktrees','model_deployments_file','external_config_file'])expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field='+key+']')?.value).toBe('');
 await change('[data-pool-config-field=shell]','powershell');await change('[data-pool-config-field=output]','D:/owned/new-shared-tool.json');await change('[data-pool-config-scope-field=workspace]','D:/owned/project');await click('[data-pool-config-copy]');await click('[data-pool-connection-refresh]');
 const back=new URLSearchParams(document.querySelector<HTMLAnchorElement>('[data-pool-connection-tools]')!.getAttribute('href')!.slice(1));expect(back.get('project')).toBe(project);expect(back.get('record')).toBe('');expect(parsePoolToolSetupPin(back.get('panel')!)).toEqual(setupPin);expect(back.get('panel')).not.toContain('original-tool');
 await click('[data-pool-config-add]');await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=tenant_id]',tenant);await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=project_id]','other-project');await change('[data-pool-config-row=scope-2] [data-pool-config-scope-field=workspace]','D:/owned/other');await click('[data-pool-config-copy]');await change('[data-pool-connection-field=scope]','1');await click('[data-pool-connection-refresh]');
 const other=new URLSearchParams(document.querySelector<HTMLAnchorElement>('[data-pool-connection-tools]')!.getAttribute('href')!.slice(1));expect(other.get('project')).toBe('other-project');expect(other.get('panel')).toBe('pool-tool:'+poolID+':'+runnerID);expect(reads.some(path=>path.startsWith('/tool-profiles')||path.startsWith('/project-execution'))).toBe(false);expect(writes).toEqual([]);
});

it('T219 keeps setup and existing-profile recovery purposes separate across ABA and clears setup eligibility on owner denial',async()=>{
 defer=true;const observations:Array<PoolToolRecoverySnapshot|undefined>=[],prepared=vi.fn();
 const draw=async(target:PoolToolTargetPin)=>{await act(async()=>root.render(<ApiViewProvider session={session}><PoolToolRecoveryPanel pin={target} pool={pool} refresh={0} onSnapshot={value=>observations.push(value)} onPrepare={prepared}/></ApiViewProvider>));await settle();};
 await draw(setupPin);const oldSetup=releases[0];await draw(pin);await draw(setupPin);await act(async()=>oldSetup(Response.json(reply())));await settle();expect(observations.filter(Boolean)).toEqual([]);
 await act(async()=>releases.at(-1)!(Response.json(reply())));await settle();const current=observations.filter(Boolean).at(-1)!;expect(current.recovery.pin).toEqual(setupPin);expect(Object.hasOwn(current.recovery.pin,'tool_profile_id')).toBe(false);expect(prepared).not.toHaveBeenCalled();
 defer=false;deny=true;await click('[data-pool-recovery-refresh]');expect(host.querySelector('[data-pool-recovery-state]')?.getAttribute('data-pool-recovery-state')).toBe('restricted');expect(host.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);expect(observations.at(-1)).toBeUndefined();expect(prepared).not.toHaveBeenCalled();expect(reads.some(path=>path.startsWith('/tool-profiles')||path.startsWith('/project-execution'))).toBe(false);expect(writes).toEqual([]);
});

it('T219 removes private preparation after an explicit refresh denies the current owner while preserving same-scope expiry drafts',async()=>{
 const clock=vi.spyOn(performance,'now');
 vi.useFakeTimers({toFake:['setInterval','clearInterval']});
 try{
 for(const status of ['403','404','revoked-grant','revoked-runner','expired-credential']){
  clock.mockReturnValue(1000);deny=false;denyStatus=403;node=structuredClone(runner);snapshot=reply();
  await workspace(serializePoolToolSetupPin(setupPin));await click('[data-pool-recovery-prepare]');
  await change('[data-pool-config-field=output]','D:/private-'+status+'/new.json');await change('[data-pool-config-scope-field=workspace]','D:/private-'+status+'/workspace');
  clock.mockReturnValue(32000);await act(async()=>{vi.advanceTimersByTime(1000);});await settle();
  expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('D:/private-'+status+'/new.json');expect(document.querySelector<HTMLButtonElement>('[data-pool-config-copy]')?.disabled).toBe(true);
  const httpDenied=status==='403'||status==='404';
  if(httpDenied){deny=true;denyStatus=Number(status);}
  else if(status==='revoked-grant')snapshot.scope.grant_status='revoked';
  else if(status==='revoked-runner'){node={...node,status:'revoked',allowed_actions:[]};snapshot.runner={...snapshot.runner,status:'revoked',credential_state:'revoked'};}
  else{node={...node,credential_expires_at:'2026-11-02T00:00:00Z'};snapshot.runner={...snapshot.runner,credential_state:'expired',credential_expires_at:node.credential_expires_at};}
  await click('[data-pool-recovery-form-refresh]');
  expect(document.querySelector('[data-pool-config-preparation]'),status).toBeNull();expect(document.querySelector('[role=dialog]'),status).toBeNull();expect(host.querySelector('[data-pool-recovery-state]')?.getAttribute('data-pool-recovery-state')).toBe(httpDenied?'restricted':'read');expect(host.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(true);
  deny=false;node=structuredClone(runner);snapshot=reply();await click('[data-pool-recovery-refresh]');await click('[data-pool-recovery-prepare]');expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('');expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=workspace]')?.value).toBe('');expect(writes).toEqual([]);
  await act(async()=>root.unmount());root=createRoot(host);
 }
 }finally{clock.mockRestore();vi.useRealTimers();}
});

it('T219 allows an owner to prepare an approved foreign-tenant scope without project access or a project return link',async()=>{
 const foreign={...setupPin,tenant_id:'foreign-consumer'};
 snapshot.scope={...snapshot.scope,tenant_id:foreign.tenant_id,can_open_project:false,route_state:'restricted',source_reason:'project_access_required'};
 await workspace(serializePoolToolSetupPin(foreign));expect(host.querySelector<HTMLButtonElement>('[data-pool-recovery-prepare]')?.disabled).toBe(false);expect(host.querySelector('[data-pool-tool-intent=setup]')?.textContent).toContain('Your account cannot read the project');await click('[data-pool-recovery-prepare]');
 expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=tenant_id]')?.value).toBe(foreign.tenant_id);expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=project_id]')?.value).toBe(foreign.project_id);
 await change('[data-pool-config-field=shell]','powershell');await change('[data-pool-config-field=output]','D:/owned/foreign-scope.json');await change('[data-pool-config-scope-field=workspace]','D:/owned/foreign');await click('[data-pool-config-copy]');expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();await click('[data-pool-connection-refresh]');
 expect(document.querySelector('[data-pool-connection-readiness]')).not.toBeNull();expect(document.querySelector('[data-pool-connection-tools]')).toBeNull();expect(reads.some(path=>path.startsWith('/tool-profiles')||path.startsWith('/project-execution'))).toBe(false);expect(writes).toEqual([]);
});

it('T219 retains the latest valid runner basis and its new private draft when the next read expires',async()=>{
 const clock=vi.spyOn(performance,'now');vi.useFakeTimers({toFake:['setInterval','clearInterval']});
 try{
  node={...runner,version:1,credential_version:1};snapshot.runner={...snapshot.runner,version:1,credential_version:1};clock.mockReturnValue(1000);
  await workspace(serializePoolToolSetupPin(setupPin));await click('[data-pool-recovery-prepare]');await change('[data-pool-config-field=output]','D:/owned/v1.json');await change('[data-pool-config-scope-field=workspace]','D:/owned/v1');
  clock.mockReturnValue(32000);await act(async()=>{vi.advanceTimersByTime(1000);});await settle();expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('D:/owned/v1.json');
  node={...runner,version:2,credential_version:2};snapshot.runner={...snapshot.runner,version:2,credential_version:2};await click('[data-pool-recovery-form-refresh]');
  expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('');expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=workspace]')?.value).toBe('');
  await change('[data-pool-config-field=output]','D:/owned/v2.json');await change('[data-pool-config-scope-field=workspace]','D:/owned/v2');
  clock.mockReturnValue(63000);await act(async()=>{vi.advanceTimersByTime(1000);});await settle();
  expect(document.querySelector<HTMLInputElement>('[data-pool-config-field=output]')?.value).toBe('D:/owned/v2.json');expect(document.querySelector<HTMLInputElement>('[data-pool-config-scope-field=workspace]')?.value).toBe('D:/owned/v2');expect(document.querySelector<HTMLButtonElement>('[data-pool-config-copy]')?.disabled).toBe(true);expect(writes).toEqual([]);
 }finally{clock.mockRestore();vi.useRealTimers();}
});
