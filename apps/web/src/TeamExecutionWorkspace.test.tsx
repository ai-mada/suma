// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ApiError,type Session} from './api';
import {setLocale,tr} from './i18n';
import {ExecutionWorkspace} from './ExecutionWorkspace';
import {TeamExecutionWorkspace,selectedTeamExecutionPlan} from './TeamExecutionWorkspace';
import {parseWorkbenchRoute} from './navigation';

// Existing dialog's original-operation and write controls have independent
// coverage. Here verify only the exact plan locator and empty task selection
// handed over by this new read-only entry, with no duplicate dispatch logic.
vi.mock('./TeamTaskDispatchDialog',()=>({TeamTaskDispatchDialog:(props:{planID?:string;tasks:unknown[];onClose:()=>void;onForbidden?:(error:unknown)=>void})=><div data-opened-team-plan={props.planID}><span>Task selection {props.tasks.length}</span><button onClick={props.onClose}>Close existing plan</button><button onClick={()=>props.onForbidden?.(new ApiError(403,'forbidden','Plan read revoked'))}>Refuse detail read</button></div>}));

const project='project-a',sha='a'.repeat(64);
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[{id:project,version:1,title:'Project'}]};
function plan(id='plan-a',name='Owned team',itemState='awaiting_review',team='team-a'){
 const configuration={project_id:project,team_id:team,team_name:name,team_version:1,version:2,state:'published',roles:[{key:'developer',title:'Developer',resource_id:'resource-a',resource_version:1,resource_kind:'agent',resource_name:'Actual agent',membership_id:'member-a',membership_version:1,workflow_id:'wf-a',workflow_version:1,agent_node_id:'code',tool_profile_id:'tool-a',tool_profile_version:2,tool_profile_sha256:sha,tool_name:'Actual CLI',knowledge:[]}],created_at:'',updated_at:'',valid:true,blockers:[],can_edit:false,can_publish:false,can_retire:false};
 const task={id:'task-a',project_id:project,version:3,title:'Actual task',status:'review',depends_on:[]};
 const dispatched=!['waiting_dependencies','ready','stopped'].includes(itemState);
 const item={task_id:task.id,task_version:3,task_basis_sha256:sha,source_revision:1,depends_on:[],dependencies:[],state:itemState,blockers:[],dispatch_plan_id:dispatched?'dispatch-a':'',run_id:dispatched?'run-a':'',role_key:'developer',role_title:'Developer',resource_id:'resource-a',resource_name:'Actual agent',run_state:dispatched?'succeeded':'',plan:{id:dispatched?'dispatch-a':'',project_id:project,version:1,state:dispatched?'started':'ready',task_id:task.id,task_version:3,workflow_id:'wf-a',workflow_version:1,agent_node_id:'code',tool_profile_id:'tool-a',tool_profile_version:2,tool_profile_sha256:sha,run_id:dispatched?'run-a':'',created_at:'',expires_at:'',context:{task:{...task,description:'Fixed task',acceptance_criteria:[]}},workflow:{id:'wf-a',name:'Fixed workflow',version:1,nodes:[],edges:[]},valid:true,reason:''}};
 const progress={total:1,waiting:0,ready:0,executing:0,awaiting_review:0,accepted:0,blocked:0,stopped:0};progress[itemState==='waiting_dependencies'?'waiting':itemState as keyof Omit<typeof progress,'total'>]=1;
 return {id,project_id:project,team_id:team,team_name:name,version:2,revision:1,graph_sha256:sha,activation_generation:1,state:itemState==='accepted'?'completed':'active',configuration_version:2,configuration,items:[item],created_at:'2026-10-28T00:00:00Z',expires_at:'',valid:true,blockers:[],can_start:false,allowed_actions:['pause','stop'],progress};
}
let root:Root|undefined,host:HTMLDivElement,requests:{url:URL;init?:RequestInit}[],reply:(url:URL,init?:RequestInit)=>Promise<Response>;
const onPlan=vi.fn(),onTasks=vi.fn(),onTeam=vi.fn();
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);onPlan.mockClear();onTasks.mockClear();onTeam.mockClear();requests=[];
 host=document.createElement('div');document.body.append(host);
 reply=async()=>Response.json({items:[plan()],total:1,next_cursor:''});
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const url=new URL(String(input),location.href);requests.push({url,init});return reply(url,init);}));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(identity=session,panel='',team?:string,invalidTeam=false){
 await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={identity}><TeamExecutionWorkspace session={identity} project={project} refresh={0} panel={panel} team={team} invalidTeam={invalidTeam} onTeam={onTeam} onPlan={onPlan} onTasks={onTasks}/></ApiViewProvider>);});await settle();
}
const button=(text:string)=>[...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===text);
async function click(text:string){expect(button(text),text).toBeDefined();await act(async()=>button(text)!.click());await settle();}
async function clickControl(selector:string){const control=host.querySelector<HTMLButtonElement>(selector);expect(control,selector).not.toBeNull();await act(async()=>control!.click());await settle();}

it('filters the authorized loaded page locally and enters a guarded exact team scope',async()=>{
 const waiting={...plan(),valid:false};
 const blocked=plan('plan-b','Second team','blocked','team-b');
 reply=async()=>Response.json({items:[waiting,blocked,plan('plan-c','Third team','accepted','team-c')],total:8,next_cursor:'next-page'});
 await render();
 const list=()=>[...host.querySelectorAll('[data-team-execution-plan]')].map(row=>row.getAttribute('data-team-execution-plan'));
 await clickControl('[data-team-execution-filter="attention"]');expect(list()).toEqual(['plan-a','plan-b']);
 await clickControl('[data-team-execution-filter="review"]');expect(list()).toEqual(['plan-a']);expect(requests).toHaveLength(1);
 expect(host.textContent).toContain('Loaded 3 of 8');expect(button('Load more team plans')).toBeDefined();
 window.addEventListener('suma:navigate',event=>event.preventDefault(),{once:true});
 await clickControl('[data-team-execution-filter-team="team-a"]');expect(onTeam).not.toHaveBeenCalled();
 await clickControl('[data-team-execution-filter-team="team-a"]');expect(onTeam).toHaveBeenCalledWith('team-a');
 reply=async()=>Response.json({items:[waiting],total:1,next_cursor:''});await render(session,'','team-a');
 expect(requests.at(-1)?.url.searchParams.get('team_id')).toBe('team-a');expect(list()).toEqual(['plan-a']);
 expect(host.querySelector('[data-team-execution-scope="team-a"]')?.textContent).toContain('Owned team');
 expect(host.querySelector('[data-team-execution-filter="all"]')?.getAttribute('aria-pressed')).toBe('true');
 expect(document.activeElement).toBe(host.querySelector('[data-team-execution-heading]'));
 await clickControl('[data-team-execution-clear-team]');expect(onTeam).toHaveBeenLastCalledWith('');
});

it('does not show other teams, previously loaded results after 404 or late responses from the old scope',async()=>{
 await render();let resolveOld!:(value:Response)=>void;reply=async()=>new Promise<Response>(resolve=>{resolveOld=resolve;});
 await act(async()=>button('Refresh team plans')!.click());const old=requests.at(-1)!;
 reply=async()=>Response.json({items:[plan('plan-b','Wrong scoped team','blocked','team-b')],total:1,next_cursor:''});
 await render(session,'','team-a');expect(old.init?.signal?.aborted).toBe(true);
 expect(host.querySelector('[data-team-execution-plan]')).toBeNull();expect(host.querySelector('[role="alert"]')).not.toBeNull();
 await act(async()=>resolveOld(Response.json({items:[plan('late-plan','Old project list')],total:1})));await settle();
 expect(host.querySelector('[data-team-execution-plan]')).toBeNull();
 reply=async()=>Response.json({items:[plan()],total:1,next_cursor:''});await click('Refresh team plans');
 expect(host.textContent).toContain('Owned team');
 reply=async()=>Response.json({error:{code:'not_found',message:'Scope unavailable'}},{status:404});await click('Refresh team plans');
 expect(host.textContent).not.toContain('Owned team');expect(host.querySelector('[data-team-execution-plan]')).toBeNull();
 expect(host.querySelector('[data-team-execution-clear-team]')).not.toBeNull();
});

it('parses only one canonical team locator in the team-execution route and requires clearing malformed filters',async()=>{
 const scope='?project=project-a&view=development&sub=runs&layout=team';
 expect(parseWorkbenchRoute(scope+'&execution_team=team-a').execution_team).toBe('team-a');
 expect(parseWorkbenchRoute(scope.replace('layout=team','layout=queue')+'&execution_team=team-a').execution_team).toBeUndefined();
 expect(parseWorkbenchRoute(scope+'&execution_team=team-a&execution_team=team-b')).toMatchObject({execution_team_invalid:'1'});
 await render(session,'',undefined,true);expect(requests).toHaveLength(0);
 expect(host.querySelector('[role="alert"]')).not.toBeNull();await clickControl('[data-team-execution-clear-team]');
 expect(onTeam).toHaveBeenCalledWith('');
});

it('reads real paged plans and acceptance counts without starting work, then opens the existing plan and switches languages',async()=>{
 reply=async url=>Response.json(url.searchParams.has('before')?{items:[plan(),plan('plan-b','Second team','blocked')],total:2,next_cursor:''}:{items:[plan()],total:2,next_cursor:'original-cursor'});
 await render();expect(requests).toHaveLength(1);expect(requests[0].url.pathname).toBe('/api/v1/expert-team-plans');expect(requests[0].url.searchParams.get('limit')).toBe('10');
 expect(host.textContent).toContain('Loaded 1 of 2');expect(host.textContent).toContain('Formally accepted 0 / 1 tasks');expect(host.querySelector('progress')?.value).toBe(0);expect(host.textContent).toContain('Awaiting human review');
 await click('Load more team plans');expect(requests[1].url.searchParams.get('before')).toBe('original-cursor');expect(host.textContent).toContain('Loaded 2 of 2');expect(host.querySelectorAll('[data-team-execution-plan]')).toHaveLength(2);
 await click('Owned team');expect(onPlan).toHaveBeenCalledWith('plan-a');await render(session,'expert-team:plan-a');expect(host.querySelector('[data-opened-team-plan]')?.getAttribute('data-opened-team-plan')).toBe('plan-a');expect(host.textContent).toContain('Task selection 0');
 await click('Close existing plan');expect(onPlan).toHaveBeenLastCalledWith('');expect(host.querySelectorAll('[data-team-execution-plan]')).toHaveLength(2);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('已正式验收 0 / 1 项任务');expect(host.textContent).toContain('已加载你可见的 2 个项目计划中的 2 个');
 expect(requests.every(item=>!item.init?.body)).toBe(true);
});

it('preserves stale loaded results after a transient read failure but hides them after current authorization fails',async()=>{
 await render();reply=async()=>Response.json({error:{code:'unavailable',message:'Retry current read'}},{status:503});await click('Refresh team plans');
 expect(host.textContent).toContain('Owned team');expect(host.textContent).toContain('Previously loaded results');expect(host.querySelector('[data-team-execution-plan]')).not.toBeNull();
 reply=async()=>Response.json({error:{code:'forbidden',message:'Current access revoked'}},{status:403});await click('Refresh team plans');expect(host.textContent).not.toContain('Owned team');expect(host.querySelector('[data-team-execution-plan]')).toBeNull();expect(host.textContent).toContain('Current access revoked');
});

it('isolates accounts and organizations before paint and ignores a late previous-owner response',async()=>{
 await render();let resolveOld!:(value:Response)=>void;reply=async()=>new Promise<Response>(resolve=>{resolveOld=resolve;});
 await act(async()=>button('Refresh team plans')!.click());const old=requests.at(-1)!;
 reply=async()=>Response.json({items:[plan('plan-b','Current owner team')],total:1,next_cursor:''});
 await render({...session,user:{...session.user,id:'user-b'},organization:{...session.organization,id:'org-b'}});
 expect(old.init?.signal?.aborted).toBe(true);expect(host.textContent).not.toContain('Owned team');expect(host.textContent).toContain('Current owner team');
 await act(async()=>resolveOld(Response.json({items:[plan('late-plan','Old private team')],total:1,next_cursor:''})));await settle();expect(host.textContent).not.toContain('Old private team');expect(host.textContent).toContain('Current owner team');
});

it('opens a shared existing plan without a selected task and provides a task entry for a truly empty project list',async()=>{
 reply=async()=>Response.json({items:[],total:0,next_cursor:''});await render(session,'expert-team:shared-plan');
 expect(host.querySelector('[data-opened-team-plan]')?.getAttribute('data-opened-team-plan')).toBe('shared-plan');expect(host.textContent).toContain('No team plans yet');await click('Choose tasks for a team');expect(onTasks).toHaveBeenCalledOnce();
 expect(selectedTeamExecutionPlan('expert-team:shared-plan')).toBe('shared-plan');expect(selectedTeamExecutionPlan('expert-team:../foreign')).toBe('');expect(selectedTeamExecutionPlan('dispatch:other')).toBe('');
});

it('adds a third execution view without reading team plans while the ordinary run list is selected',async()=>{
 const onLayout=vi.fn();await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ExecutionWorkspace session={session} project={project} refresh={0} panel='' layout='list' onLayout={onLayout} onRun={()=>{}} onPlan={onPlan} onTasks={onTasks}><p>Original run list</p></ExecutionWorkspace></ApiViewProvider>);});await settle();
 expect(requests).toHaveLength(0);expect(host.textContent).toContain('Original run list');await click('Team execution');expect(onLayout).toHaveBeenCalledWith('team');expect(requests).toHaveLength(0);
});

it('rejects malformed plan links visibly without a read or a replacement preview',async()=>{
 await render(session,'expert-team:../foreign');expect(host.textContent).toContain('This team plan link is invalid');expect(requests).toHaveLength(0);expect(host.querySelector('[data-opened-team-plan]')).toBeNull();await click('Return to team plans');expect(onPlan).toHaveBeenCalledWith('');expect(requests).toHaveLength(0);
});

it('clears the underlying loaded list when an opened plan read reports current authorization loss',async()=>{
 await render(session,'expert-team:plan-a');expect(host.textContent).toContain('Owned team');await click('Refuse detail read');expect(host.textContent).toContain('Plan read revoked');expect(host.textContent).not.toContain('Owned team');expect(host.querySelector('[data-team-execution-plan]')).toBeNull();expect(host.querySelector('[data-opened-team-plan]')).toBeNull();
});

it('keeps revoked results hidden during a pending retry until a fresh scoped read succeeds',async()=>{
 await render(session,'expert-team:plan-a');expect(host.textContent).toContain('Owned team');await click('Refuse detail read');
 let resolveCurrent!:(value:Response)=>void;reply=async()=>new Promise<Response>(resolve=>{resolveCurrent=resolve;});await click(tr('common.retry'));
 expect(requests).toHaveLength(2);expect(host.textContent).not.toContain('Owned team');expect(host.querySelector('[data-team-execution-plan]')).toBeNull();
 await act(async()=>resolveCurrent(Response.json({items:[plan('current-plan','Fresh current team')],total:1,next_cursor:''})));await settle();
 expect(host.textContent).toContain('Fresh current team');expect(host.textContent).not.toContain('Owned team');expect(host.querySelectorAll('[data-team-execution-plan]')).toHaveLength(1);
});
