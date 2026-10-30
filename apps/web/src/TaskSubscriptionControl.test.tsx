// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {TaskSubscriptionControl} from './TaskSubscriptionControl';

const session=(id='user-a'):Session=>({user:{id,name:'Viewer',email:'viewer@example.test',role:'viewer'},organization:{id:'org-a',name:'Org'},projects:[]});
let host:HTMLDivElement,root:Root,state:{project_id:string;task_id:string;user_id:string;version:number;following:boolean;updated_at:string},requests:{path:string;body?:Record<string,any>;headers:Headers}[],reply:(path:string,body?:Record<string,any>)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];
 state={project_id:'project-a',task_id:'task-a',user_id:'user-a',version:0,following:false,updated_at:''};
 reply=async(_path,body)=>{if(!body)return Response.json(state);state={...state,version:state.version+1,following:body.action==='follow',updated_at:'2026-10-30T01:00:00Z'};return Response.json({idempotency_key:body.idempotency_key,subscription:state});};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace('/api/v1',''),body=init?.body?JSON.parse(String(init.body)):undefined;requests.push({path,body,headers:new Headers(init?.headers)});return reply(path,body);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render({user='user-a',task='task-a',refresh=0,accessBlocked=false,onAccessDenied=()=>{}}={}){await act(async()=>root.render(<ApiViewProvider session={session(user)}><TaskSubscriptionControl session={session(user)} project="project-a" taskId={task} refresh={refresh} accessBlocked={accessBlocked} onAccessDenied={onAccessDenied}/></ApiViewProvider>));await settle();}
async function settle(){await act(async()=>{for(let n=0;n<16;n++)await Promise.resolve();});}
const toggle=()=>host.querySelector<HTMLButtonElement>('[data-task-subscription-toggle]');
async function click(selector='[data-task-subscription-toggle]'){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
it('only reads on mount and lets a viewer follow then unfollow through explicit scoped commands',async()=>{
 await render();expect(requests.map(item=>item.path)).toEqual(['/tasks/task-a/subscription?project_id=project-a']);expect(requests[0].body).toBeUndefined();expect(toggle()?.getAttribute('aria-pressed')).toBe('false');
 await click();expect(requests[1].path).toBe('/tasks/task-a/subscription/commands?project_id=project-a');expect(requests[1].body).toMatchObject({project_id:'project-a',task_id:'task-a',expected_version:0,action:'follow'});expect(Object.keys(requests[1].body!).sort()).toEqual(['action','expected_version','idempotency_key','project_id','task_id']);expect(requests[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(toggle()?.getAttribute('aria-pressed')).toBe('true');
 await click();expect(requests.filter(item=>item.body).map(item=>item.body!.action)).toEqual(['follow','unfollow']);expect(toggle()?.getAttribute('aria-pressed')).toBe('false');
});
it('keeps an unknown original command locked after fresh reads and restores the same body and key after remount',async()=>{
 const normal=reply;let lost=true,historical={...state};reply=async(path,body)=>{if(body&&lost){lost=false;await normal(path,body);historical={...state};throw Error('response lost');}if(body)return Response.json({idempotency_key:body.idempotency_key,subscription:historical});return normal(path,body);};
 await render();await click();const original=requests.find(item=>item.body)!.body;expect(toggle()?.disabled).toBe(true);await render({refresh:1});expect(toggle()?.disabled).toBe(true);expect(requests.filter(item=>item.body)).toHaveLength(1);
 await act(async()=>root.unmount());root=createRoot(host);state={...state,version:3,following:false};await render();expect(toggle()?.disabled).toBe(true);await click('[data-task-subscription-recover]');const writes=requests.filter(item=>item.body);expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(original);expect(state.version).toBe(3);expect(toggle()?.getAttribute('aria-pressed')).toBe('false');
});
it('does not send an untracked preference when recovery storage is unavailable',async()=>{
 await render();vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('full');});await click();expect(requests.filter(item=>item.body)).toHaveLength(0);expect(toggle()?.getAttribute('aria-pressed')).toBe('false');
});
it('rejects a mismatched success receipt and retains the original operation instead of changing the displayed preference',async()=>{
 const normal=reply;reply=async(path,body)=>body?Response.json({idempotency_key:'different-operation',subscription:{...state,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'}}):normal(path,body);
 await render();await click();expect(toggle()?.disabled).toBe(true);expect(toggle()?.getAttribute('aria-pressed')).toBe('false');expect(host.querySelector('[data-task-subscription-recover]')).not.toBeNull();
});
it('clears protected state after a current refusal and discards late task and account responses',async()=>{
 state={...state,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'};const denied=vi.fn();await render({onAccessDenied:denied});
 reply=async()=>Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403});await render({refresh:1,onAccessDenied:denied});expect(toggle()?.getAttribute('aria-pressed')).toBeNull();expect(toggle()?.disabled).toBe(true);expect(denied).toHaveBeenCalledOnce();
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await render({refresh:2});const stale=finish;
 reply=async()=>Response.json({...state,user_id:'other',task_id:'task-b',following:false});await render({user:'other',task:'task-b'});await act(async()=>stale(Response.json(state)));expect(toggle()?.getAttribute('aria-pressed')).toBe('false');
});
it('keeps known conflicts blocked until the current preference is explicitly checked and changes language without writing',async()=>{
 const normal=reply;reply=async(path,body)=>body?Response.json({error:{code:'version_conflict',message:'Changed'}},{status:409}):normal(path,body);
 await render();await click();expect(toggle()?.disabled).toBe(true);const writes=requests.filter(item=>item.body).length;
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('关注');expect(requests.filter(item=>item.body)).toHaveLength(writes);
 state={...state,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'};await click('.conflict-banner button');expect(toggle()?.disabled).toBe(false);expect(toggle()?.getAttribute('aria-pressed')).toBe('true');expect(requests.filter(item=>item.body)).toHaveLength(writes);
});
it('admits only one command during a pending double click',async()=>{
 await render();let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await act(async()=>{toggle()!.click();toggle()!.click();});await settle();const writes=requests.filter(item=>item.body);expect(writes).toHaveLength(1);expect(toggle()?.disabled).toBe(true);
 const first=writes[0].body!;reply=async()=>Response.json({...state,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'});
 await act(async()=>finish(Response.json({idempotency_key:first.idempotency_key,subscription:{...state,version:1,following:true,updated_at:'2026-10-30T01:00:00Z'}})));await settle();expect(toggle()?.getAttribute('aria-pressed')).toBe('true');
});
