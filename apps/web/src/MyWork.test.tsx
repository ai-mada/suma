// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {MyWork,type MyWorkProps} from './MyWork';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';

const session:Session={user:{id:'user-a',name:'Account A',email:'a@example.test'},organization:{id:'org-a',name:'Org A'},projects:[]};
const task=(patch:Record<string,unknown>={})=>({id:'task-a',project_id:'project-a',version:1,title:'Protect the delivery',description:'Scoped task',status:'todo',priority:'high',owner_id:'human-a',requirement_id:'requirement-a',updated_at:'2026-10-29T01:00:00Z',run_id:'not-execution-proof',...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',user_id:'user-a',resource_id:'human-a',resource_name:'Human resource A',resource_status:'active',items:[task()],total:1,limit:20,next_cursor:'',status_counts:[{status:'todo',total:1}],as_of:'2026-10-29T02:00:00Z',...patch});
let host:HTMLDivElement,root:Root,props:MyWorkProps,reads:{path:string;signal?:AbortSignal|null;headers:Headers;method?:string}[];
beforeEach(async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];
 props={project:'project-a',userID:'user-a',organizationID:'org-a',q:'',status:'',refresh:0,onFilter:vi.fn(),onTask:vi.fn(),onWorkspace:vi.fn(),onResources:vi.fn()};
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
function transport(reply:(path:string,init?:RequestInit)=>Promise<Response>|Response=()=>Response.json(page())){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');reads.push({path,signal:init?.signal,headers:new Headers(init?.headers),method:init?.method});return reply(path,init);}));}
async function settle(){await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(patch:Partial<MyWorkProps>={}){props={...props,...patch};const view={...session,user:{...session.user,id:props.userID},organization:{...session.organization,id:props.organizationID}};await act(async()=>root.render(<ApiViewProvider session={view}><MyWork {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(value:string){const input=host.querySelector<HTMLInputElement>('[name=my-work-q]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});}
async function submit(){await act(async()=>{host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('uses canonical human ownership, whole-filter counts and existing explicit navigation only',async()=>{
 transport(()=>Response.json(page({total:7,status_counts:[{status:'todo',total:5},{status:'in_review',total:2}]})));await render();
 expect(reads).toHaveLength(1);expect(reads[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(reads[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(reads[0].path).not.toContain('owner_id');
 expect(host.textContent).toContain('Assigned to Human resource A');expect(host.textContent).toContain('Showing 1 of 7');expect(host.querySelector('[data-my-work-status=in_review]')?.textContent).toContain('2');expect(host.textContent).toContain('Linked requirement: requirement-a');expect(host.querySelectorAll('[data-my-work-task]')).toHaveLength(1);expect(host.textContent).not.toContain('Execution is active');
 await click('.my-work-task-title');await click('.my-work-workspace');expect(props.onTask).toHaveBeenCalledWith('task-a');expect(props.onWorkspace).toHaveBeenCalledWith('task-a');expect(reads).toHaveLength(1);expect(reads.every(read=>read.method==='GET')).toBe(true);
});

it('shows assigned drafts and custom current states on All without dropping their counts',async()=>{
 transport(()=>Response.json(page({items:[task({status:'draft',title:'Draft assigned task'}),task({id:'task-b',status:'waiting_for_customer',title:'Current custom task'})],total:3,status_counts:[{status:'draft',total:1},{status:'waiting_for_customer',total:2}]})));await render();expect(host.querySelectorAll('[data-my-work-task]')).toHaveLength(2);expect(host.textContent).toContain('Draft assigned task');expect(host.textContent).toContain('Current custom task');expect(host.textContent).toContain('waiting_for_customer');expect(host.textContent).toContain('Showing 2 of 3');expect(host.querySelector('[data-my-work-status=all] .my-work-count')?.textContent).toBe('3');expect(host.querySelector('[data-my-work-status=draft] .my-work-count')?.textContent).toBe('1');await click('[data-my-work-status=draft]');expect(props.onFilter).toHaveBeenCalledWith({q:'',status:'draft'});
});

it('submits literal search explicitly and preserves committed search when changing status',async()=>{
 transport();await render({q:'old query',status:'todo'});await change(' 50%_中文 + ');expect(reads).toHaveLength(1);expect(props.onFilter).not.toHaveBeenCalled();await submit();expect(props.onFilter).toHaveBeenLastCalledWith({q:' 50%_中文 + ',status:'todo'});
 await click('[data-my-work-status=blocked]');expect(props.onFilter).toHaveBeenLastCalledWith({q:'old query',status:'blocked'});
 await change('😀'.repeat(201));await submit();expect(host.querySelector('[role=alert]')?.textContent).toContain('200');expect(props.onFilter).toHaveBeenCalledTimes(2);
});

it('ignores out-of-order search results and aborts the old request before showing new facts',async()=>{
 let old!:(value:Response)=>void;
 transport(path=>new URLSearchParams(path.split('?')[1]).get('q')==='old'?new Promise<Response>(resolve=>{old=resolve;}):Response.json(page({items:[task({title:'Fresh filtered task'})]})));
 await render({q:'old'});const original=reads[0];await render({q:'new'});expect(original.signal?.aborted).toBe(true);expect(host.textContent).toContain('Fresh filtered task');await act(async()=>old(Response.json(page({items:[task({title:'Old search secret'})]}))));await settle();expect(host.textContent).not.toContain('Old search secret');
});

it('drops previous project and user facts immediately and ignores a late cross-scope response',async()=>{
 let old!:(value:Response)=>void;
 transport(path=>path.includes('project_id=project-a')?new Promise<Response>(resolve=>{old=resolve;}):Response.json(page({project_id:'project-b',user_id:'user-b',resource_id:'human-b',resource_name:'Human B',items:[task({id:'task-b',project_id:'project-b',owner_id:'human-b',title:'Only B work'})]})));
 await render();const original=reads[0];await render({project:'project-b',userID:'user-b',organizationID:'org-b'});expect(original.signal?.aborted).toBe(true);await act(async()=>old(Response.json(page())));await settle();expect(host.textContent).toContain('Only B work');expect(host.textContent).not.toContain('Human resource A');expect(host.textContent).not.toContain('Protect the delivery');
});

it('hides a loaded queue during an organization-only switch before its new read finishes',async()=>{
 let resolve!:(value:Response)=>void;let switched=false;
 transport(()=>switched?new Promise<Response>(done=>{resolve=done;}):Response.json(page()));await render();expect(host.textContent).toContain('Protect the delivery');switched=true;await render({organizationID:'org-b'});expect(host.querySelector('[data-my-work-task]')).toBeNull();expect(host.textContent).not.toContain('Human resource A');expect(host.querySelector('[data-my-work-status=todo] .my-work-count')?.textContent).toBe('—');expect(reads.at(-1)?.headers.get('X-Suma-Expected-Organization')).toBe('org-b');await act(async()=>resolve(Response.json(page({items:[task({title:'Current organization task'})]}))));await settle();expect(host.textContent).toContain('Current organization task');
});

it.each([401,403,404])('clears rows, resource, counts and cursor after HTTP %i and retries from page one',async status=>{
 let denied=false;transport(path=>denied?Response.json({error:{code:'unavailable',message:'Access unavailable'}},{status}):Response.json(page({total:6,status_counts:[{status:'todo',total:6}],next_cursor:'private-cursor'})));await render();denied=true;await click('[data-my-work-more]');expect(host.textContent).not.toContain('Protect the delivery');expect(host.textContent).not.toContain('Human resource A');expect(host.textContent).not.toContain('Showing');expect(host.textContent).toContain('Private task and resource details have been cleared');expect(host.querySelector('[data-my-work-more]')).toBeNull();expect(host.querySelector('[data-my-work-status=todo] .my-work-count')?.textContent).toBe('—');
 denied=false;await click('.error button');expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.textContent).toContain('Protect the delivery');
});

it('drops stale counts on transient pagination failure and never retries its cursor',async()=>{
 let mode='success',resolve!:(value:Response)=>void;transport(()=>mode==='fail'?Response.json({error:{code:'read_failed',message:'Temporary failure'}},{status:503}):mode==='pending'?new Promise<Response>(done=>{resolve=done;}):Response.json(page({total:4,status_counts:[{status:'todo',total:4}],next_cursor:'next-a'})));await render();mode='fail';await click('[data-my-work-more]');expect(host.querySelector('[data-my-work-task]')).toBeNull();expect(host.querySelector('[data-my-work-status=all] .my-work-count')?.textContent).toBe('—');mode='pending';await click('.error button');expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.textContent).not.toContain('Human resource A');expect(host.querySelector('[data-my-work-status=todo] .my-work-count')?.textContent).toBe('—');await act(async()=>resolve(Response.json(page())));await settle();expect(host.textContent).toContain('Protect the delivery');
});

it('loads another page explicitly with the same filters and then resets cursor for changed filters',async()=>{
 transport(path=>Response.json(page({items:[task({id:path.includes('cursor=')?'task-b':'task-a',status:'blocked'})],total:3,status_counts:[{status:'todo',total:8},{status:'blocked',total:3}],next_cursor:path.includes('cursor=')?'':'opaque+/='})));await render({q:'literal %',status:'blocked'});expect(reads).toHaveLength(1);await click('[data-my-work-more]');expect(host.querySelectorAll('[data-my-work-task]')).toHaveLength(2);const params=new URLSearchParams(reads[1].path.split('?')[1]);expect(params.get('cursor')).toBe('opaque+/=');expect(params.get('q')).toBe('literal %');expect(params.get('status')).toBe('blocked');expect(host.textContent).toContain('Showing 2 of 3');await render({q:'new query'});expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.querySelectorAll('[data-my-work-task]')).toHaveLength(1);
});

it.each(['unbound','inactive'])('explains %s resources with an explicit resource navigation',async resource_status=>{
 transport(()=>Response.json(page({resource_status,resource_id:'',resource_name:'',items:[],total:0,status_counts:[]})));await render();expect(host.querySelector('[data-my-work-task]')).toBeNull();expect(host.querySelector('.empty h3')?.textContent).toContain(resource_status==='unbound'?'Connect':'inactive');await click('.empty button');expect(props.onResources).toHaveBeenCalledTimes(1);
});

it('renders Chinese empty/filter states and switches current queue copy to English without re-reading',async()=>{
 transport(()=>Response.json(page({items:[],total:0,status_counts:[]})));await act(async()=>setLocale('zh-CN',{persist:false}));await render({q:'中文'});expect(host.textContent).toContain('没有符合筛选条件的任务');expect(host.querySelector('[aria-pressed=true]')?.textContent).toContain('全部');await click('.empty button');expect(props.onFilter).toHaveBeenCalledWith({q:'',status:''});const initial=reads.length;await act(async()=>setLocale('en-US',{persist:false}));expect(host.textContent).toContain('No tasks match these filters');expect(reads).toHaveLength(initial);await render({q:''});expect(host.textContent).toContain('No tasks assigned to you');expect(host.querySelector('.empty button')).toBeNull();
});

it('rejects malformed canonical ownership instead of rendering protected records',async()=>{
 transport(()=>Response.json(page({items:[task({owner_id:'user-a'})]})));await render();expect(host.querySelector('[data-my-work-task]')).toBeNull();expect(host.textContent).not.toContain('Human resource A');expect(host.textContent).toContain('could not be confirmed');
});
