// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {ToolAction} from './tool-hooks';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';

const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const session={user:{id:'u',name:'Owner',email:'u@example.test'},organization:{id:'o',name:'Org'}};
const item=(id='existing',state='paused')=>({id,project_id:'p',worker_id:'one',name:'Original '+id,version:4,state,profile,available:false,reason:'The current observation does not allow dispatch.',enabled_by:'',created_at:'2026-10-30T08:00:00Z',updated_at:'2026-10-30T08:00:00Z'});
const matches=(items:ReturnType<typeof item>[])=>( {project_id:'p',worker_id:'one',profile,items,total:items.length});
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
let root:Root,host:HTMLDivElement,props:ToolQuickConnectProps,query:ReturnType<typeof vi.fn>,requests:{path:string;body?:Record<string,unknown>}[];
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];query=vi.fn(async()=>Response.json(matches([])));
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const path=String(input).replace('/api/v1',''),body=init.body?JSON.parse(String(init.body)):undefined;requests.push({path,body});if(path==='/tool-profiles/matches')return query(body);return Response.json(path.startsWith('/adapter-catalog')?{project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter]}:{project_id:'p',worker_id:'one',state:'fresh',as_of:'2026-10-30T08:00:20Z',observed_at:'2026-10-30T08:00:00Z',expires_at:'2026-10-30T08:01:00Z',items:[{profile,available:true,reason:''}]});}));
 props={project:'p',adapter:adapter as ToolQuickConnectProps['adapter'],workers:identity(['one']),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{run:vi.fn(async(_path:string,body:Record<string,unknown>)=>({...body,id:'new',version:1,state:'paused',available:false})),clearError:vi.fn(),busy:false,pending:undefined,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onOpen:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function render(){await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLInputElement>(selector);expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}

it('reuses an exact paused profile after reopening without creating a profile or a registration notification',async()=>{
 const existing=item();query.mockImplementation(async()=>Response.json(matches([existing])));await render();
 expect(query).toHaveBeenCalledExactlyOnceWith({project_id:'p',worker_id:'one',profile});expect(host.querySelector('[data-quick-register]')).toBeNull();expect(props.action.run).not.toHaveBeenCalled();expect(props.onRegistered).not.toHaveBeenCalled();expect(props.onEnable).not.toHaveBeenCalled();
 await click('[data-existing-tool-enable]');expect(props.onEnable).toHaveBeenCalledExactlyOnceWith(existing);expect(props.onOpen).not.toHaveBeenCalled();
});
it('requires an explicit choice between existing profiles and opens an enabled but unavailable profile without enabling it',async()=>{
 const enabled=item('enabled','enabled');query.mockImplementation(async()=>Response.json(matches([item('paused'),enabled,item('old','revoked')])));await render();
 expect(host.querySelector<HTMLInputElement>('[data-existing-tool-choice]:checked')).toBeNull();expect(host.querySelector('[data-existing-tool-enable]')).toBeNull();expect(host.querySelector('[data-existing-tool-open]')).not.toBeNull();
 const option=host.querySelector<HTMLInputElement>('[data-existing-tool-choice][value="enabled"]')!;expect(option).not.toBeNull();await act(async()=>option.click());await settle();
 await click('[data-existing-tool-open="enabled"]');expect(props.onOpen).toHaveBeenCalledExactlyOnceWith(enabled);expect(props.onEnable).not.toHaveBeenCalled();expect(props.onRegistered).not.toHaveBeenCalled();expect(props.action.run).not.toHaveBeenCalled();expect(host.textContent).toContain(enabled.reason);
});
it('blocks registration on an unread match set and retains the authored name through an explicit successful retry',async()=>{
 query.mockImplementationOnce(async()=>new Response(JSON.stringify({error:{code:'unavailable',message:'Match lookup unavailable'}}),{status:503}));await render();
 expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);await change('[data-quick-name]','My preserved name');expect(props.action.run).not.toHaveBeenCalled();
 await click('[data-quick-matches-refresh]');await click('[data-quick-register]');expect(props.action.run).toHaveBeenCalledExactlyOnceWith('/tool-profiles',{project_id:'p',worker_id:'one',name:'My preserved name',profile});expect(props.onRegistered).toHaveBeenCalledOnce();
});
it('discards an existing-profile response after the inventory deadline instead of reviving an expired registration',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});let finish!:(value:Response)=>void;query.mockImplementation(()=>new Promise<Response>(resolve=>{finish=resolve;}));await render();
 expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(true);await act(async()=>vi.advanceTimersByTimeAsync(40001));await act(async()=>finish(Response.json(matches([item()]))));await settle();
 expect(host.querySelector('[data-existing-tools]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);expect(props.action.run).not.toHaveBeenCalled();expect(props.onEnable).not.toHaveBeenCalled();
});
it('gives the original uncertain write priority over a new read-only match query',async()=>{
 props.action={...props.action,pending:{path:'/tool-profiles',method:'POST',body:{project_id:'p',worker_id:'one',name:'Original',profile},idempotency_key:'original-operation'}} as ToolAction;await render();expect(query).not.toHaveBeenCalled();expect(props.action.run).not.toHaveBeenCalled();
 props.action={...props.action,pending:undefined};await render();expect(query).toHaveBeenCalledOnce();expect(props.onRegistered).not.toHaveBeenCalled();
});
it('rechecks an authoritative create conflict and retains the name instead of retrying a duplicate registration',async()=>{
 query.mockImplementationOnce(async()=>Response.json(matches([]))).mockImplementationOnce(async()=>Response.json(matches([item()])));await render();await change('[data-quick-name]','Authored name');
 const run=vi.fn(async()=>{props.action={...props.action,conflict:true};await render();return undefined;});props.action={...props.action,run} as unknown as ToolAction;await render();await click('[data-quick-register]');
 expect(run).toHaveBeenCalledOnce();expect(query).toHaveBeenCalledTimes(2);expect(host.querySelector('[data-existing-tools]')).not.toBeNull();expect(host.querySelector('[data-quick-register]')).toBeNull();expect(props.onRegistered).not.toHaveBeenCalled();
 query.mockImplementation(async()=>Response.json(matches([])));const refresh=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent==='Read again')!;expect(refresh).toBeDefined();await act(async()=>refresh.click());await settle();expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('Authored name');expect(run).toHaveBeenCalledOnce();
});
