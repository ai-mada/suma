// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ProjectNotificationPreferences} from '../../../packages/contracts/src';
import {ProjectNotificationPreferencesControl} from './ProjectNotificationPreferences';
import {ApiViewProvider,bindApiView} from './api-context';
import type {ApiCaller,Session} from './api';
import {setLocale} from './i18n';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async importOriginal=>{
 const actual=await importOriginal<typeof import('./api-context')>();
 return {...actual,useApi:()=>{const caller=actual.useApi();return callerOverride.current??caller;}};
});

type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
type Reply=(path:string,body?:Record<string,unknown>)=>Promise<Response>;
const session=(id='user-a',organization='org-a'):Session=>({user:{id,name:'Viewer',email:'viewer@example.test',role:'viewer'},organization:{id:organization,name:'Org'},projects:[]});
const readPath=(project='project-a')=>'/notification-preferences?project_id='+project;
const commandPath='/notification-preferences/commands?project_id=project-a';
const timestamp='2026-10-31T07:00:00Z';
let host:HTMLDivElement,root:Root,state:ProjectNotificationPreferences,requests:Request[],reply:Reply;

beforeEach(async()=>{
 callerOverride.current=undefined;await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();
 host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];
 state={project_id:'project-a',user_id:'user-a',version:0,followed_discussions_muted:false,updated_at:''};
 reply=async(_path,body)=>{
  if(!body)return Response.json(state);
  if(typeof body.followed_discussions_muted!=='boolean')throw new Error('Invalid test command');
  state={...state,version:state.version+1,followed_discussions_muted:body.followed_discussions_muted,updated_at:timestamp};
  return Response.json({idempotency_key:body.idempotency_key,preferences:state});
 };
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=String(input).replace('/api/v1',''),body:Record<string,unknown>|undefined=init?.body?JSON.parse(String(init.body)):undefined;
  requests.push({path,method:init?.method||'GET',body,headers:new Headers(init?.headers)});return reply(path,body);
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.unstubAllGlobals();vi.restoreAllMocks();});

async function settle(){await act(async()=>{for(let index=0;index<18;index++)await Promise.resolve();});}
async function render({project='project-a',user='user-a',organization='org-a',disabled=false,accessBlocked=false,onAccessDenied=(_error:unknown)=>{}}={}){
 await act(async()=>root.render(<ApiViewProvider session={session(user,organization)}><ProjectNotificationPreferencesControl project={project} userID={user} organizationID={organization} disabled={disabled} accessBlocked={accessBlocked} onAccessDenied={onAccessDenied}/></ApiViewProvider>));await settle();
}
const selector=(name:string)=>'[data-notification-preferences-'+name+']';
function element<T extends HTMLElement=HTMLButtonElement>(name:string){const found=host.querySelector<T>(selector(name));expect(found).not.toBeNull();return found!;}
const checkbox=()=>element<HTMLInputElement>('muted');
const shownState=()=>element('state').getAttribute('data-state');
const writes=()=>requests.filter(request=>request.method==='POST');
async function click(name:string){await act(async()=>element(name).click());await settle();}
async function open(){if(element('toggle').getAttribute('aria-expanded')==='false')await click('toggle');}

it('reads once while collapsed, preserves a local draft across parent renders, and saves an explicit scoped command before reading its current result',async()=>{
 await render();expect(requests.map(request=>[request.method,request.path])).toEqual([['GET',readPath()]]);
 expect(element('toggle').getAttribute('aria-expanded')).toBe('false');expect(shownState()).toBe('enabled');
 const panel=host.querySelector<HTMLElement>(selector('panel'));expect(!panel||panel.hidden).toBe(true);
 await render();expect(requests).toHaveLength(1);await open();expect(element('toggle').getAttribute('aria-expanded')).toBe('true');
 expect(element('panel').hidden).toBe(false);await click('muted');expect(checkbox().checked).toBe(true);expect(writes()).toHaveLength(0);
 await render();expect(checkbox().checked).toBe(true);expect(requests).toHaveLength(1);
 await click('save');const command=writes()[0];expect(command.path).toBe(commandPath);
 expect(command.body).toEqual({project_id:'project-a',expected_version:0,followed_discussions_muted:true,idempotency_key:expect.any(String)});
 expect(String(command.body!.idempotency_key).length).toBeGreaterThanOrEqual(8);
 expect(command.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(command.headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(requests.map(request=>[request.method,request.path])).toEqual([['GET',readPath()],['POST',commandPath],['GET',readPath()]]);
 expect(shownState()).toBe('muted');expect(checkbox().checked).toBe(true);
});

it('keeps a lost original command locked through close, reopen and remount, then recovers its exact key before reading a newer preference',async()=>{
 const normal=reply;let lost=true,historical={...state};
 reply=async(path,body)=>{
  if(body&&lost){lost=false;await normal(path,body);historical={...state};throw new Error('Response lost after commit');}
  if(body)return Response.json({idempotency_key:body.idempotency_key,preferences:historical});
  return normal(path,body);
 };
 await render();await open();await click('muted');await click('save');
 const original=writes()[0];expect(element<HTMLButtonElement>('save').disabled).toBe(true);expect(checkbox().disabled).toBe(true);
 await click('toggle');expect(element('toggle').getAttribute('aria-expanded')).toBe('false');expect(host.querySelector(selector('recover'))).not.toBeNull();
 await open();expect(element<HTMLButtonElement>('save').disabled).toBe(true);expect(writes()).toHaveLength(1);
 await act(async()=>root.unmount());root=createRoot(host);
 state={...state,version:2,followed_discussions_muted:false,updated_at:'2026-10-31T07:01:00Z'};
 await render();await open();expect(element<HTMLButtonElement>('save').disabled).toBe(true);expect(checkbox().disabled).toBe(true);
 await click('recover');expect(writes()).toHaveLength(2);expect(writes()[1].path).toBe(original.path);expect(writes()[1].body).toEqual(original.body);
 expect(writes()[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(requests.at(-1)?.method).toBe('GET');expect(requests.at(-1)?.path).toBe(readPath());
 expect(state.version).toBe(2);expect(shownState()).toBe('enabled');expect(checkbox().checked).toBe(false);expect(checkbox().disabled).toBe(false);
 expect(host.querySelector(selector('recover'))).toBeNull();
});

it('sends no POST when recovery storage cannot persist the command and retains the local choice',async()=>{
 await render();await open();await click('muted');vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('Storage full');});
 await click('save');expect(writes()).toHaveLength(0);expect(checkbox().checked).toBe(true);expect(shownState()).toBe('enabled');
 expect(host.querySelector('[role="alert"]')).not.toBeNull();
});

it('preserves a conflicting draft until an explicit GET rebases it and never retries with an invented version',async()=>{
 const normal=reply;let conflict=true;
 reply=async(path,body)=>body&&conflict?(conflict=false,Response.json({error:{code:'version_conflict',message:'Changed'}},{status:409})):normal(path,body);
 await render();await open();await click('muted');await click('save');expect(writes()).toHaveLength(1);
 expect(checkbox().checked).toBe(true);expect(element<HTMLButtonElement>('save').disabled).toBe(true);
 state={...state,version:1,updated_at:timestamp};await click('refresh');
 expect(writes()).toHaveLength(1);expect(requests.at(-1)?.method).toBe('GET');expect(checkbox().checked).toBe(true);
 expect(element<HTMLButtonElement>('save').disabled).toBe(false);await click('save');
 expect(writes()).toHaveLength(2);expect(writes()[1].body).toMatchObject({expected_version:1,followed_discussions_muted:true});
 expect(writes()[1].body!.idempotency_key).not.toBe(writes()[0].body!.idempotency_key);expect(shownState()).toBe('muted');
});

it('clears protected preferences on a current refusal and reports the denial to the owning inbox',async()=>{
 state={...state,version:1,followed_discussions_muted:true,updated_at:timestamp};const onAccessDenied=vi.fn();
 await render({onAccessDenied});await open();expect(shownState()).toBe('muted');
 reply=async()=>Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403});await click('refresh');
 expect(shownState()).toBe('unknown');expect(checkbox().checked).toBe(false);expect(checkbox().disabled).toBe(true);
 expect(onAccessDenied).toHaveBeenCalledOnce();expect(onAccessDenied.mock.calls[0][0]).toMatchObject({status:403});expect(writes()).toHaveLength(0);
});

it.each(['project','account'] as const)('ignores a late preference read after the %s changes',async(changed)=>{
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await render();
 const stale=finish,old={...state,version:1,followed_discussions_muted:true,updated_at:timestamp};
 const project=changed==='project'?'project-b':'project-a',user=changed==='account'?'user-b':'user-a';
 state={project_id:project,user_id:user,version:0,followed_discussions_muted:false,updated_at:''};reply=async()=>Response.json(state);
 await render({project,user});await open();expect(shownState()).toBe('enabled');
 await act(async()=>stale(Response.json(old)));await settle();expect(shownState()).toBe('enabled');expect(checkbox().checked).toBe(false);
 expect(writes()).toHaveLength(0);expect(requests.map(request=>request.path)).toEqual([readPath(),readPath(project)]);
});

it('admits only one explicit save during a same-tick double click',async()=>{
 await render();await open();await click('muted');const normal=reply;let finish!:(value:Response)=>void;
 reply=(path,body)=>body?new Promise(resolve=>{finish=resolve;}):normal(path,body);
 const save=element<HTMLButtonElement>('save');await act(async()=>{save.click();save.click();});await settle();
 expect(writes()).toHaveLength(1);expect(element<HTMLButtonElement>('save').disabled).toBe(true);
 const original=writes()[0].body!;state={...state,version:1,followed_discussions_muted:true,updated_at:timestamp};reply=normal;
 await act(async()=>finish(Response.json({idempotency_key:original.idempotency_key,preferences:state})));await settle();
 expect(writes()).toHaveLength(1);expect(shownState()).toBe('muted');expect(requests.at(-1)?.method).toBe('GET');
});

it('retains the original pending command when a success response has a mismatched receipt',async()=>{
 const normal=reply;reply=(path,body)=>body?Promise.resolve(Response.json({idempotency_key:'other-operation',preferences:{...state,version:1,followed_discussions_muted:true,updated_at:timestamp}})):normal(path,body);
 await render();await open();await click('muted');await click('save');
 expect(writes()).toHaveLength(1);expect(element<HTMLButtonElement>('save').disabled).toBe(true);expect(checkbox().disabled).toBe(true);
 expect(host.querySelector(selector('recover'))).not.toBeNull();expect(shownState()).toBe('enabled');
 expect(requests.map(request=>request.method)).toEqual(['GET','POST']);
});

it.each(['organization','API caller'] as const)('retires a pending receipt across %s changes without applying it to the current view',async(changed)=>{
 state={...state,version:1,followed_discussions_muted:true,updated_at:timestamp};
 const normal=reply;let finish!:(value:Response)=>void;
 reply=(path,body)=>body?new Promise(resolve=>{finish=resolve;}):normal(path,body);
 await render();await open();await click('muted');await click('save');
 const original=writes()[0].body!;expect(original).toMatchObject({expected_version:1,followed_discussions_muted:false});
 const journalKey='suma-write-recovery-v1:project-notification-preferences:'+JSON.stringify(['user-a','org-a','project-a']);
 const journal=sessionStorage.getItem(journalKey);expect(journal).not.toBeNull();
 state={...state,version:5,followed_discussions_muted:true,updated_at:'2026-10-31T07:05:00Z'};
 if(changed==='API caller')callerOverride.current=bindApiView(session());
 await render({organization:changed==='organization'?'org-b':'org-a'});await open();
 expect(shownState()).toBe('muted');expect(checkbox().checked).toBe(true);
 expect(requests.map(request=>request.method)).toEqual(['GET','POST','GET']);
 expect(requests.at(-1)?.headers.get('X-Suma-Expected-Organization')).toBe(changed==='organization'?'org-b':'org-a');
 expect(!!host.querySelector(selector('recover'))).toBe(changed==='API caller');
 const notices=[...host.querySelectorAll('[role="status"]')].map(node=>node.textContent);
 const readCount=requests.filter(request=>request.method==='GET').length;
 await act(async()=>finish(Response.json({idempotency_key:original.idempotency_key,preferences:{...state,version:2,followed_discussions_muted:false,updated_at:timestamp}})));await settle();
 expect(shownState()).toBe('muted');expect(checkbox().checked).toBe(true);expect(state.version).toBe(5);
 expect(writes()).toHaveLength(1);expect(requests.filter(request=>request.method==='GET')).toHaveLength(readCount);
 expect([...host.querySelectorAll('[role="status"]')].map(node=>node.textContent)).toEqual(notices);
 expect(sessionStorage.getItem(journalKey)).toBe(journal);
});
