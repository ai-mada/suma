// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {RunnerPairingInbox,type RunnerPairingInboxProps} from './RunnerPairingInbox';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';

const stamp='2026-11-01T04:00:00.125Z',identity:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
function pairing(project:string,index=1,name='Needle machine'){
 return {id:index.toString(16).padStart(32,'0'),project_id:project,version:2,state:'proposed',control_url:'https://control.example.test/suma',created_at:stamp,expires_at:'2026-11-01T04:10:00Z',can_approve:true,device_name:name,credential_sha256:'a'.repeat(64),reason_code:'approval_required'};
}
function page(url:URL,name='Needle machine'){
 const project=url.searchParams.get('project_id')!,limit=Number(url.searchParams.get('limit')),next=url.searchParams.get('cursor'),items=next?[pairing(project,21,name)]:Array.from({length:Math.min(limit,21)},(_,index)=>pairing(project,index+1,name+' '+(index+1)));
 return {project_id:project,status:url.searchParams.get('status')!,q:url.searchParams.get('q')!,items,total:21,limit,...(!next&&limit<21?{next_cursor:'opaque/next:+ 中文'}:{}),as_of:stamp};
}
function deferred(){let resolve!:(value:Response)=>void;const promise=new Promise<Response>(done=>{resolve=done;});return {promise,resolve};}
let root:Root,host:HTMLDivElement;
async function settle(){await act(async()=>{for(let i=0;i<22;i++)await Promise.resolve();});}
async function render(props:RunnerPairingInboxProps,session:Session=identity){await act(async()=>root.render(<ApiViewProvider session={session}><RunnerPairingInbox {...props}/></ApiViewProvider>));await settle();}
async function click(suffix:string){const button=host.querySelector<HTMLElement>('[data-runner-pairing-inbox-'+suffix+']');expect(button,suffix).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(suffix:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>('[data-runner-pairing-inbox-'+suffix+']');expect(field,suffix).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('submits one filter source, pages with opaque cursors and selects only an explicit exact ID without writing',async()=>{
 const reads:{url:URL;init:RequestInit}[]=[],selected=vi.fn(),denied=vi.fn();
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const url=new URL(String(input),'https://local.test');reads.push({url,init});if(reads.length===2)return Response.json({error:{code:'temporary_unavailable',message:'Temporary read failure'}},{status:503});return Response.json(page(url));}));
 await render({project:'project-a',onSelect:selected,onDenied:denied,selectedID:pairing('project-a').id});expect(reads).toHaveLength(1);expect(selected).not.toHaveBeenCalled();
 expect(reads[0].url.searchParams.get('status')).toBe('active');expect(reads[0].init.headers).toMatchObject({'X-Suma-Expected-User':identity.user.id,'X-Suma-Expected-Organization':identity.organization.id});
 const row=host.querySelector<HTMLButtonElement>('[data-runner-pairing-inbox-select="'+pairing('project-a').id+'"]');expect(row?.getAttribute('aria-pressed')).toBe('true');await act(async()=>row!.click());expect(selected).toHaveBeenCalledExactlyOnceWith(pairing('project-a').id);
 await click('next');expect(reads[1].url.searchParams.get('cursor')).toBe('opaque/next:+ 中文');expect(host.textContent).toContain('Temporary read failure');await click('previous');expect(reads[2].url.searchParams.has('cursor')).toBe(false);
 await change('query',' needle ');expect(reads).toHaveLength(3);await click('search');expect(reads[3].url.searchParams.get('q')).toBe('needle');await click('next');expect(reads[4].url.searchParams.has('cursor')).toBe(true);
 await change('status','proposed');expect(reads[5].url.searchParams.get('status')).toBe('proposed');expect(reads[5].url.searchParams.get('q')).toBe('needle');expect(reads[5].url.searchParams.has('cursor')).toBe(false);
 await change('limit','50');expect(reads[6].url.searchParams.get('limit')).toBe('50');expect(host.querySelector<HTMLButtonElement>('[data-runner-pairing-inbox-next]')?.disabled).toBe(true);expect(host.querySelector('time')?.dateTime).toBe(stamp);
 expect(reads.every(({init})=>init.method==='GET'&&init.body===undefined)).toBe(true);expect(denied).not.toHaveBeenCalled();
});

it('blocks pending interactions and discards late or cross-scope pages while clearing current denied facts',async()=>{
 const old=deferred(),fresh=deferred(),held=deferred(),requests:{url:URL;init:RequestInit}[]=[],selected=vi.fn(),denied=vi.fn();
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const url=new URL(String(input),'https://local.test');requests.push({url,init});switch(requests.length){case 1:return old.promise;case 2:return fresh.promise;case 3:return held.promise;case 4:return Response.json({...page(url),project_id:'wrong-project'});default:return Response.json({error:{code:'forbidden',message:'Current access changed'}},{status:403});}}));
 const props={project:'project-a',onSelect:selected,onDenied:denied},other={...identity,user:{...identity.user,id:'user-b'},organization:{id:'org-b',name:'Other organization'}};
 await render(props);await render({...props,project:'project-b'},other);expect(requests[0].init.signal?.aborted).toBe(true);
 await act(async()=>fresh.resolve(Response.json(page(requests[1].url,'Current scope machine'))));await settle();await act(async()=>old.resolve(Response.json({error:{code:'forbidden',message:'Old actor denied'}},{status:403})));await settle();
 expect(denied).not.toHaveBeenCalled();expect(host.textContent).toContain('Current scope machine');expect(requests[1].init.headers).toMatchObject({'X-Suma-Expected-User':'user-b','X-Suma-Expected-Organization':'org-b'});
 await render({...props,project:'project-b',disabled:true},other);await click('next');await change('status','all');await click('search');const button=host.querySelector<HTMLButtonElement>('[data-runner-pairing-inbox-select]');expect(button?.disabled).toBe(true);await act(async()=>button!.click());expect(selected).not.toHaveBeenCalled();expect(requests).toHaveLength(2);
 await render({...props,project:'project-b',refreshToken:1},other);expect(host.querySelector('[data-runner-pairing-inbox-stale]')).not.toBeNull();await render({...props,project:'project-b',refreshToken:1,accessBlocked:true},other);expect(requests[2].init.signal?.aborted).toBe(true);expect(host.textContent).not.toContain('Current scope machine');
 await act(async()=>held.resolve(Response.json(page(requests[2].url,'Late protected machine'))));await settle();expect(host.textContent).not.toContain('Late protected machine');
 await render({...props,project:'project-b',refreshToken:1},other);expect(host.querySelector('[data-runner-pairing-inbox-select]')).toBeNull();expect(host.textContent).not.toContain('Current scope machine');expect(denied).not.toHaveBeenCalled();
 await click('refresh');expect(denied).toHaveBeenCalledTimes(1);expect(host.querySelector('[data-runner-pairing-inbox-select]')).toBeNull();expect(requests.every(({init})=>init.method==='GET'&&init.body===undefined)).toBe(true);
});
