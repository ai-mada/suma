// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {RunnerPairing} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {RunnerPairingPanel} from './RunnerPairingPanel';
import {setLocale} from './i18n';

const project='project-a',firstID='a'.repeat(32),secondID='b'.repeat(32);
const owner:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const other:Session={...owner,user:{...owner.user,id:'user-b'}};
const locator=(session=owner)=>'suma-runner-pairing:'+JSON.stringify([session.user.id,session.organization.id,project]);
const waiting=(id=firstID):RunnerPairing=>({id,project_id:project,version:1,state:'waiting',control_url:'https://suma.example.test/team',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:10:00Z',can_approve:false});
const proposed=(id=firstID):RunnerPairing=>({...waiting(id),version:2,state:'proposed',device_name:id===firstID?'Original remote device':'Other remote device',credential_sha256:'c'.repeat(64),can_approve:true});
const approved=():RunnerPairing=>({...proposed(),version:3,state:'approved',can_approve:false,worker_id:'remote-'+'d'.repeat(32),credential_expires_at:'2026-11-02T00:00:00Z'});
const pairingPage=(items:RunnerPairing[]=[])=>({project_id:project,status:'active',q:'',items,total:items.length,limit:20,as_of:'2026-11-01T00:00:00Z'});
let host:HTMLDivElement,root:Root,onPaired:ReturnType<typeof vi.fn>,lock:ReturnType<typeof vi.fn>;
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function render(session=owner){await act(async()=>root.render(<ApiViewProvider session={session}><RunnerPairingPanel project={project} onLockChange={lock} onPaired={onPaired}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function shell(value:string){const field=host.querySelector<HTMLSelectElement>('[data-runner-pairing-shell]');expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
function deferred(){let resolve!:(value:Response)=>void;const promise=new Promise<Response>(done=>{resolve=done;});return {promise,resolve};}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);onPaired=vi.fn();lock=vi.fn();});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('creates a public command, reviews the exact proposal and continues only after explicit approval',async()=>{
 const writes:{path:string;body:Record<string,unknown>;headers:Headers}[]=[],copy=vi.fn(async(_value:string)=>{});let current=waiting();Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:copy}});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const path=String(input).replace('/api/v1','');if(init.method==='POST'){const body=JSON.parse(String(init.body));writes.push({path,body,headers:new Headers(init.headers)});if(path==='/runner-pairings')return Response.json(waiting(),{status:201});current=approved();return Response.json(current);}
  return Response.json(path.startsWith('/runner-pairings?')?pairingPage():current);
 }));
 await render();await click('[data-runner-pairing-create]');expect(sessionStorage.getItem(locator())).toBe(firstID);expect(writes).toHaveLength(1);expect(writes[0].headers.get('X-Suma-Expected-User')).toBe(owner.user.id);
 await shell('bash');await click('[data-runner-pairing-copy]');expect(copy.mock.calls[0][0]).toContain('--pair-runner');expect(copy.mock.calls[0][0]).toContain(firstID);expect(copy.mock.calls[0][0]).not.toContain('TOKEN');expect(writes).toHaveLength(1);
 current=proposed();await click('[data-runner-pairing-refresh]');expect(host.textContent).toContain(current.device_name);expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-approve]')!.disabled).toBe(true);expect(onPaired).not.toHaveBeenCalled();
 await click('[data-runner-pairing-confirm]');await click('[data-runner-pairing-approve]');expect(writes).toHaveLength(2);expect(writes[1].body).toMatchObject({action:'approve',expected_version:2,project_id:project});expect(writes[1].body).not.toHaveProperty('credential_sha256');expect(onPaired).not.toHaveBeenCalled();
 await click('[data-runner-pairing-continue]');expect(onPaired).toHaveBeenCalledExactlyOnceWith(approved());expect(lock.mock.calls.some(([value])=>value===true)).toBe(true);expect(lock.mock.lastCall?.[0]).toBe(false);expect(sessionStorage.getItem(locator())).toBe(firstID);
});
it('preserves the exact unknown creation across remount and locator-storage failure before permitting another action',async()=>{
 const writes:Record<string,unknown>[]=[],originalSet=Storage.prototype.setItem,originalRemove=Storage.prototype.removeItem;let failLocator=false,failCleanup=false,knownFailure=false,current=waiting();
 vi.spyOn(Storage.prototype,'setItem').mockImplementation(function(this:Storage,key:string,value:string){if(failLocator&&key===locator())throw new Error('Controlled locator storage failure');originalSet.call(this,key,value);});
 vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,key:string){if(failCleanup&&key.startsWith('suma-write-recovery-v1:runner-pairing:'))throw new Error('Controlled pending cleanup failure');originalRemove.call(this,key);});
 vi.stubGlobal('fetch',vi.fn(async(_input:string,init:RequestInit={})=>{
  if(init.method!=='POST')return Response.json(String(_input).includes('/runner-pairings?')?pairingPage():current);writes.push(JSON.parse(String(init.body)));
  if(knownFailure){knownFailure=false;return Response.json({error:{code:'pairing_expired',message:'The original pairing expired'}},{status:400});}
  return Response.json(writes.length===1?proposed():waiting(),{status:201});
 }));
 await render();await click('[data-runner-pairing-create]');expect(host.querySelector('[data-runner-pairing-restore]')).not.toBeNull();expect(sessionStorage.getItem(locator())).toBeNull();expect(writes).toHaveLength(1);
 await act(async()=>root.render(null));await render();expect(host.querySelector('[data-runner-pairing-create]')).toBeNull();failLocator=true;await click('[data-runner-pairing-restore]');expect(writes[1]).toEqual(writes[0]);expect(host.querySelector('[data-runner-pairing-restore]')).not.toBeNull();expect(sessionStorage.getItem(locator())).toBeNull();
 failLocator=false;failCleanup=true;await click('[data-runner-pairing-restore]');expect(writes[2]).toEqual(writes[0]);expect(sessionStorage.getItem(locator())).toBe(firstID);expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-restore]')?.disabled).toBe(false);
 failCleanup=false;await click('[data-runner-pairing-restore]');expect(writes).toHaveLength(3);expect(host.querySelector('[data-runner-pairing-restore]')).toBeNull();expect(onPaired).not.toHaveBeenCalled();
 knownFailure=true;await click('[data-runner-pairing-reject]');expect(writes).toHaveLength(4);current={...waiting(),version:2,state:'expired'};
 await click('[data-runner-pairing-refresh]');expect(writes).toHaveLength(4);expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-create]')?.disabled).toBe(false);
 await act(async()=>root.render(null));sessionStorage.removeItem(locator());await render();knownFailure=true;await click('[data-runner-pairing-create]');expect(writes).toHaveLength(5);
 await click('[data-runner-pairing-refresh]');expect(writes).toHaveLength(5);expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-create]')?.disabled).toBe(false);
});
it('ignores a late old-account refusal and hides protected proposal details on current read or write refusals',async()=>{
 sessionStorage.setItem(locator(),firstID);sessionStorage.setItem(locator(other),secondID);const old=deferred();let read=0;
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{if(String(input).includes('/runner-pairings?'))return Response.json(pairingPage());read++;return read===1?old.promise:read===2?Response.json(proposed(secondID)):Response.json({error:{code:'forbidden',message:'Current access changed'}},{status:403});}));
 await render();await render(other);expect(host.textContent).toContain('Other remote device');await act(async()=>old.resolve(Response.json({error:{code:'forbidden',message:'Old account refusal'}},{status:403})));await settle();expect(host.textContent).toContain('Other remote device');expect(host.textContent).not.toContain('Old account refusal');
 await click('[data-runner-pairing-refresh]');expect(host.textContent).not.toContain('Other remote device');expect(host.querySelector('[data-runner-pairing-approve]')).toBeNull();expect(onPaired).not.toHaveBeenCalled();expect(sessionStorage.getItem(locator(other))).toBe(secondID);
 await act(async()=>root.render(null));vi.stubGlobal('fetch',vi.fn(async(_input:string,init:RequestInit={})=>init.method==='POST'?Response.json({error:{code:'forbidden',message:'Current write access changed'}},{status:403}):Response.json(String(_input).includes('/runner-pairings?')?pairingPage():proposed(secondID))));
 await render(other);expect(host.textContent).toContain('Other remote device');await click('[data-runner-pairing-confirm]');await click('[data-runner-pairing-approve]');expect(host.textContent).not.toContain('Other remote device');expect(host.querySelector('[data-runner-pairing-continue]')).toBeNull();expect(onPaired).not.toHaveBeenCalled();
});

it('selects only a public inbox locator, reads current details and keeps the list locked through original pending recovery and current denial',async()=>{
 const thirdID='e'.repeat(32);let current=proposed(),wrongAck=true,deny=false,createdNew=false;const details:string[]=[],writes:Record<string,unknown>[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');
  if(init.method==='POST'){writes.push(JSON.parse(String(init.body)));if(path==='/runner-pairings'){createdNew=true;return Response.json(waiting(thirdID),{status:201});}current=approved();if(wrongAck){wrongAck=false;return Response.json({...approved(),id:secondID});}return Response.json(current);}
  if(path==='/runner-pairings')return deny?Response.json({error:{code:'forbidden',message:'Current list access changed'}},{status:403}):Response.json(pairingPage([...(current.state==='approved'?[]:[current]),waiting(secondID),...(createdNew?[waiting(thirdID)]:[])]));
  details.push(path);return Response.json(path==='/runner-pairings/'+thirdID?waiting(thirdID):current);
 }));
 await render();expect(details).toEqual([]);expect(writes).toEqual([]);await click('[data-runner-pairing-inbox-select="'+firstID+'"]');expect(details).toEqual(['/runner-pairings/'+firstID]);expect(sessionStorage.getItem(locator())).toBe(firstID);expect(onPaired).not.toHaveBeenCalled();
 await click('[data-runner-pairing-confirm]');await click('[data-runner-pairing-approve]');expect(writes).toHaveLength(1);expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-inbox-select="'+secondID+'"]')?.disabled).toBe(true);await click('[data-runner-pairing-inbox-select="'+secondID+'"]');expect(details).toHaveLength(1);expect(sessionStorage.getItem(locator())).toBe(firstID);
 await click('[data-runner-pairing-restore]');expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(host.querySelector('[data-runner-pairing-continue]')).not.toBeNull();expect(onPaired).not.toHaveBeenCalled();
 deny=true;await click('[data-runner-pairing-inbox-refresh]');expect(host.querySelector('[data-runner-pairing-worker]')).toBeNull();expect(host.querySelector('[data-runner-pairing-continue]')).toBeNull();expect(host.textContent).not.toContain('Original remote device');expect(sessionStorage.getItem(locator())).toBe(firstID);expect(onPaired).not.toHaveBeenCalled();
 await act(async()=>root.render(null));deny=false;await render();expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-new]')?.disabled).toBe(false);await click('[data-runner-pairing-new]');expect(writes).toHaveLength(3);expect(writes[2]).toMatchObject({project_id:project});expect(writes[2]).not.toHaveProperty('action');expect(sessionStorage.getItem(locator())).toBe(thirdID);expect(host.querySelector('[data-runner-pairing-continue]')).toBeNull();
});
