// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ApiError,type Session} from './api';
import {setLocale} from './i18n';
import {TaskIterationAssignments} from './TaskIterationAssignments';

const project='project-a';
const session:Session={user:{id:'user-a',name:'Ana',email:'ana@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const tasks=[
  {id:'task-a',project_id:project,version:3,title:'Review candidate'},
  {id:'task-b',project_id:project,version:5,title:'Publish evidence'},
];
const plans=[
  {task_id:'task-a',iteration_id:'source',plan_version:6},
  {task_id:'task-b',iteration_id:'target',plan_version:8},
];
const iterations=[
  {id:'source',version:2,title:'Previous iteration',status:'active',start_date:'',end_date:'',description:'',goal:''},
  {id:'target',version:4,title:'Next iteration',status:'planned',start_date:'',end_date:'',description:'',goal:''},
  {id:'done',version:5,title:'Completed iteration',status:'completed',start_date:'',end_date:'',description:'',goal:''},
];
const operationPath='/api/v1/planning/iteration-assignments';
let host:HTMLElement,root:Root|undefined;
let onChanged:ReturnType<typeof vi.fn>,onRefresh:ReturnType<typeof vi.fn>,onForbidden:ReturnType<typeof vi.fn>;
let writes:Array<Record<string,unknown>>;
let onAuthorityRestored:ReturnType<typeof vi.fn>;
let reply:(body:Record<string,unknown>)=>Promise<Response>;
const response=(body:Record<string,unknown>)=>({
  project_id:body.project_id,operation_id:body.idempotency_key,iteration_id:body.iteration_id,
  changes:(body.tasks as Array<{task_id:string;expected_plan_version:number}>).map(task=>{
    const changed=plans.find(plan=>plan.task_id===task.task_id)!.iteration_id!==body.iteration_id;
    return {task_id:task.task_id,changed,plan_version:task.expected_plan_version+Number(changed)};
  }),
});

beforeEach(async()=>{
  await setLocale('en-US',{persist:false});
  sessionStorage.clear();
  host=document.createElement('div');
  document.body.append(host);
  writes=[];
  onChanged=vi.fn();
  onRefresh=vi.fn();
  onForbidden=vi.fn();
  onAuthorityRestored=vi.fn();
  reply=async body=>Response.json(response(body));
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
  vi.stubGlobal('fetch',vi.fn(async(path:string,init?:RequestInit)=>{
    if(path==='/api/v1/task-owner-targets?project_id=project-a&limit=1'){
      const headers=new Headers(init?.headers);
      expect(headers.get('X-Suma-Expected-User')).toBe(session.user.id);
      expect(headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
      return Response.json({project_id:project,items:[],total:0,limit:1,next_cursor:''});
    }
    expect(path).toBe(operationPath);
    const body=JSON.parse(String(init?.body));
    writes.push(body);
    return reply(body);
  }));
});
afterEach(async()=>{
  if(root)await act(async()=>root!.unmount());
  root=undefined;
  host.remove();
  await setLocale('zh-CN',{persist:false});
  vi.unstubAllGlobals();
});

async function render(selection=tasks,refs=plans,periods=iterations,authorityError?:unknown){
  await act(async()=>{
    root??=createRoot(host);
    root.render(<ApiViewProvider session={session}>
      <TaskIterationAssignments project={project} userID={session.user.id} organizationID={session.organization.id}
        tasks={selection} plans={refs} iterations={periods} authorityError={authorityError}
        onChanged={onChanged} onRefresh={onRefresh} onForbidden={onForbidden} onAuthorityRestored={onAuthorityRestored}/>
    </ApiViewProvider>);
  });
}
function button(selector:string){
  const element=document.querySelector<HTMLButtonElement>(selector);
  expect(element,selector).not.toBeNull();
  return element!;
}
async function click(selector:string){await act(async()=>button(selector).click());}
async function pick(value:string){
  const select=document.querySelector<HTMLSelectElement>('[name=task_iteration_target]')!;
  await act(async()=>{
    select.value=value;
    select.dispatchEvent(new Event('change',{bubbles:true}));
  });
}

it('shows changed and unchanged tasks before one atomic, versioned write',async()=>{
  await render();
  await click('[data-iteration-assignment-launch]');
  const target=document.querySelector<HTMLSelectElement>('[name=task_iteration_target]')!;
  expect([...target.options].map(option=>option.text)).not.toContain('Completed iteration');
  await pick('iteration:target');
  expect(writes).toHaveLength(0);
  expect(document.querySelector('[data-iteration-assignment-row=task-a]')?.textContent).toContain('Previous iteration');
  expect(document.querySelector('[data-iteration-assignment-row=task-b]')?.textContent).toContain('Unchanged');
  await click('[data-iteration-assignment-apply]');
  expect(writes).toHaveLength(1);
  expect(writes[0]).toEqual({
    project_id:project,iteration_id:'target',expected_iteration_version:4,idempotency_key:expect.any(String),
    tasks:[{task_id:'task-a',expected_task_version:3,expected_plan_version:6},{task_id:'task-b',expected_task_version:5,expected_plan_version:8}],
  });
  expect(document.querySelector('[data-iteration-assignment-receipt]')?.textContent).toContain('Iteration changes saved');
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('blocks a changed task in a completed source and requires a new review after stale projections',async()=>{
  await render(tasks,[{...plans[0],iteration_id:'done'},plans[1]]);
  await click('[data-iteration-assignment-launch]');
  await pick('iteration:target');
  expect(button('[data-iteration-assignment-apply]').disabled).toBe(true);
  expect(document.body.textContent).toContain('Reopen the completed iteration');
  await render(tasks,[{...plans[0],plan_version:7},plans[1]]);
  expect(button('[data-iteration-assignment-apply]').disabled).toBe(true);
  await click('[data-iteration-assignment-refresh]');
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(writes).toHaveLength(0);
});

it('does not retry a rejected schedule and requires an explicit refresh',async()=>{
  reply=async()=>Response.json({error:{code:'conflict',message:'The task schedule changed'}},{status:409});
  await render();
  await click('[data-iteration-assignment-launch]');
  await pick('remove');
  await click('[data-iteration-assignment-apply]');
  expect(writes[0]).toMatchObject({iteration_id:'',expected_iteration_version:0});
  expect(button('[data-iteration-assignment-apply]').disabled).toBe(true);
  await click('[data-iteration-assignment-apply]');
  expect(writes).toHaveLength(1);
  expect(onChanged).not.toHaveBeenCalled();
  expect(onForbidden).not.toHaveBeenCalled();
  await click('[data-iteration-assignment-refresh]');
  expect(onRefresh).toHaveBeenCalledTimes(1);
});

it('retains an unverified receipt and replays the exact request after remount with no selection',async()=>{
  reply=async body=>Response.json({...response(body),iteration_id:'wrong-iteration'});
  await render();
  await click('[data-iteration-assignment-launch]');
  await pick('iteration:target');
  await click('[data-iteration-assignment-apply]');
  expect(document.querySelector('[data-iteration-assignment-receipt]')).toBeNull();
  const original=writes[0];
  await act(async()=>root!.unmount());
  root=undefined;
  await render([]);
  await click('[data-iteration-assignment-launch]');
  reply=async body=>Response.json(response(body));
  await click('[data-iteration-assignment-restore]');
  expect(writes).toEqual([original,original]);
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('keeps a late receipt private when the same project loses authority',async()=>{
  let finish!:(response:Response)=>void;
  reply=async()=>new Promise(resolve=>{finish=resolve;});
  await render();
  await click('[data-iteration-assignment-launch]');
  await pick('iteration:target');
  await click('[data-iteration-assignment-apply]');
  await render([],[],[],new ApiError(403,'forbidden','Access changed'));
  await act(async()=>finish(Response.json(response(writes[0]))));
  expect(document.body.textContent).not.toContain('Review candidate');
  expect(document.querySelector('[data-iteration-assignment-restore]')).toBeNull();
  expect(document.querySelector('[data-iteration-assignment-receipt]')).toBeNull();
  const key='suma-write-recovery-v1:'+JSON.stringify(['task-iteration-assignments',session.user.id,session.organization.id,project]);
  expect(JSON.parse(sessionStorage.getItem(key)!).body).toEqual(writes[0]);
  expect(onChanged).not.toHaveBeenCalled();
});

it('requires explicit write-authority recheck and then exact replay after a denied recovery',async()=>{
  reply=async()=>{throw new TypeError('lost response');};
  await render();
  await click('[data-iteration-assignment-launch]');
  await pick('iteration:target');
  await click('[data-iteration-assignment-apply]');
  reply=async()=>Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403});
  await click('[data-iteration-assignment-restore]');
  expect(onForbidden).toHaveBeenCalledTimes(1);
  const denied=new ApiError(403,'forbidden','Access changed');
  await render([],[],[],denied);
  await click('[data-iteration-assignment-check-access]');
  expect(onAuthorityRestored).toHaveBeenCalledTimes(1);
  expect(writes).toHaveLength(2);
  expect(onChanged).not.toHaveBeenCalled();
  await render([],[],[]);
  reply=async body=>Response.json(response(body));
  await click('[data-iteration-assignment-restore]');
  expect(writes).toEqual([writes[0],writes[0],writes[0]]);
  expect(onChanged).toHaveBeenCalledTimes(1);
});