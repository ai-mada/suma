// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ApiError,type Session} from './api';
import {setLocale} from './i18n';
import {TaskPriorityChanges} from './TaskPriorityChanges';

const project='project-a';
const session:Session={user:{id:'user-a',name:'Ana',email:'ana@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const tasks=[
  {id:'task-a',project_id:project,version:3,title:'Review candidate',status:'accepted',priority:'medium'},
  {id:'task-b',project_id:project,version:5,title:'Publish evidence',status:'in_progress',priority:'high'},
];
const launch='[data-task-priority-launch]',apply='[data-task-priority-apply]',restore='[data-task-priority-restore]',receipt='[data-task-priority-receipt]';
let host:HTMLElement,root:Root|undefined;
let onChanged:ReturnType<typeof vi.fn>,onRefresh:ReturnType<typeof vi.fn>,onForbidden:ReturnType<typeof vi.fn>,onAuthorityRestored:ReturnType<typeof vi.fn>;
let writes:Array<Record<string,unknown>>;
let reply:(body:Record<string,unknown>)=>Promise<Response>;
const result=(body:Record<string,unknown>)=>({
  project_id:body.project_id,operation_id:body.idempotency_key,
  changes:(body.tasks as {task_id:string;expected_version:number}[]).map(({task_id,expected_version})=>{
    const previous_priority=tasks.find(task=>task.id===task_id)!.priority;
    const changed=previous_priority!==body.priority;
    return {task_id,previous_version:expected_version,version:expected_version+Number(changed),previous_priority,priority:body.priority,changed};
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
  reply=async body=>Response.json(result(body));
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
  vi.stubGlobal('fetch',vi.fn(async(path:string,init?:RequestInit)=>{
    if(path==='/api/v1/task-owner-targets?project_id=project-a&limit=1'){
      const headers=new Headers(init?.headers);
      expect(headers.get('X-Suma-Expected-User')).toBe(session.user.id);
      expect(headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
      return Response.json({project_id:project,items:[],total:0,limit:1,next_cursor:''});
    }
    expect(path).toBe('/api/v1/task-priority-changes');
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

async function render(selection=tasks,authorityError?:unknown){
  await act(async()=>{
    root??=createRoot(host);
    root.render(<ApiViewProvider session={session}>
      <TaskPriorityChanges project={project} userID={session.user.id} organizationID={session.organization.id}
        tasks={selection} authorityError={authorityError}
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
  const select=document.querySelector<HTMLSelectElement>('[data-task-priority-target]')!;
  await act(async()=>{
    select.value=value;
    select.dispatchEvent(new Event('change',{bubbles:true}));
  });
}

it('previews current and target priorities and submits all exact versions in one operation',async()=>{
  await render();
  await click(launch);
  await pick('high');
  expect(writes).toHaveLength(0);
  expect(button(apply).textContent).toContain('1 to change');
  expect(document.querySelector('.task-priority-review')?.textContent).toContain('1 changed · 1 unchanged');
  const first=document.querySelector('[data-task-priority-row=task-a]')!;
  expect(first.textContent).toContain('Medium');
  expect(first.textContent).toContain('High');
  expect(document.querySelector('[data-task-priority-row=task-b]')?.textContent).toContain('Unchanged');
  await click(apply);
  expect(writes).toEqual([{
    project_id:project,priority:'high',idempotency_key:expect.any(String),
    tasks:[{task_id:'task-a',expected_version:3},{task_id:'task-b',expected_version:5}],
  }]);
  expect(document.querySelector(receipt)?.textContent).toContain('Priority changes saved');
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('blocks no-op, unavailable and stale selections before sending a write',async()=>{
  await render([tasks[0]]);
  await click(launch);
  await pick('medium');
  expect(button(apply).disabled).toBe(true);
  await pick('urgent');
  expect(button(apply).disabled).toBe(false);
  await render([{...tasks[0],version:4}]);
  expect(button(apply).disabled).toBe(true);
  await click('[data-task-priority-refresh]');
  expect(onRefresh).toHaveBeenCalledTimes(1);
  await render([{...tasks[0],status:'archived'}]);
  await click(launch);
  expect(button(apply).disabled).toBe(true);
  expect(writes).toHaveLength(0);
});

it('retains an invalid receipt and replays the exact operation after remount without selection',async()=>{
  reply=async body=>{
    const invalid=result(body);
    invalid.changes[0].version++;
    return Response.json(invalid);
  };
  await render();
  await click(launch);
  await pick('urgent');
  await click(apply);
  expect(document.querySelector(receipt)).toBeNull();
  expect(button(restore).disabled).toBe(false);
  const original=writes[0];
  await act(async()=>root!.unmount());
  root=undefined;
  await render([]);
  await click(launch);
  reply=async body=>Response.json(result(body));
  await click(restore);
  expect(writes).toEqual([original,original]);
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('retains an unknown operation across authorization refusal and clears it only after a proven conflict',async()=>{
  reply=async()=>new Response(null,{status:503});
  await render();
  await click(launch);
  await pick('urgent');
  await click(apply);
  const original=writes[0];
  reply=async()=>Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403});
  await click(restore);
  expect(writes).toEqual([original,original]);
  expect(onForbidden).toHaveBeenCalledTimes(1);
  expect(document.querySelector(restore)).toBeNull();
  expect(document.body.textContent).not.toContain('Review candidate');
  await click('[data-task-priority-check-access]');
  expect(onAuthorityRestored).toHaveBeenCalledTimes(1);
  reply=async()=>Response.json({error:{code:'version_conflict',message:'Selected tasks changed'}},{status:409});
  await click(restore);
  expect(writes).toEqual([original,original,original]);
  expect(document.querySelector(restore)).toBeNull();
  expect(button('[data-task-priority-refresh]')).not.toBeNull();
  expect(onChanged).not.toHaveBeenCalled();
});

it('retires an in-flight response when authority is lost in the same project',async()=>{
  let finish!:(response:Response)=>void;
  reply=()=>new Promise(resolve=>{finish=resolve;});
  await render();
  await click(launch);
  await pick('urgent');
  await click(apply);
  await render(tasks,new ApiError(403,'forbidden','Access changed'));
  await act(async()=>finish(Response.json(result(writes[0]))));
  expect(document.querySelector(receipt)).toBeNull();
  expect(document.body.textContent).not.toContain('Review candidate');
  expect(onChanged).not.toHaveBeenCalled();
  expect(Object.keys(sessionStorage).some(key=>key.includes('task-priority-changes'))).toBe(true);
});
