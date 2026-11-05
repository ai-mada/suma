// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {TaskPlanningStatusChangeRequest,TaskPlanningStatusPreviewRequest} from '../../../packages/contracts/src';
import {ApiError,stateLabel,type RecordItem,type Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {TaskPlanningStatusChanges} from './TaskPlanningStatusChanges';

const project='project-a';
const session:Session={user:{id:'user-a',name:'Ana',email:'ana@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const tasks=[
  {id:'task-a',project_id:project,version:3,title:'Update requirements',status:'draft'},
  {id:'task-b',project_id:project,version:5,title:'Review evidence',status:'todo'},
];
const base='/api/v1/task-planning-status-changes';
const launch='[data-task-planning-status-launch]',apply='[data-task-planning-status-apply]';
const restore='[data-task-planning-status-restore]',receipt='[data-task-planning-status-receipt]';
let host:HTMLElement,root:Root|undefined;
let onChanged:ReturnType<typeof vi.fn>,onRefresh:ReturnType<typeof vi.fn>,onForbidden:ReturnType<typeof vi.fn>;
let previews:TaskPlanningStatusPreviewRequest[],writes:TaskPlanningStatusChangeRequest[];
let previewReply:(request:TaskPlanningStatusPreviewRequest)=>Promise<Response>;
let writeReply:(request:TaskPlanningStatusChangeRequest)=>Promise<Response>;

const expectedSelection=tasks.map(task=>({task_id:task.id,expected_version:task.version}));
const itemStatus=(id:string)=>tasks.find(task=>task.id===id)!.status;
const previewResult=(request:TaskPlanningStatusPreviewRequest)=>({
  project_id:request.project_id,
  items:request.tasks.map(({task_id,expected_version})=>({task_id,version:expected_version,status:itemStatus(task_id),eligibility:'editable'})),
});
const writeResult=(request:TaskPlanningStatusChangeRequest)=>({
  project_id:request.project_id,operation_id:request.idempotency_key,
  changes:request.tasks.map(({task_id,expected_version})=>{
    const previous_status=itemStatus(task_id),changed=previous_status!==request.status;
    return {task_id,previous_version:expected_version,version:expected_version+Number(changed),previous_status,status:request.status,changed};
  }),
});

beforeEach(async()=>{
  await setLocale('en-US',{persist:false});
  sessionStorage.clear();
  host=document.createElement('div');
  document.body.append(host);
  previews=[];
  writes=[];
  onChanged=vi.fn();
  onRefresh=vi.fn();
  onForbidden=vi.fn();
  previewReply=async request=>Response.json(previewResult(request));
  writeReply=async request=>Response.json(writeResult(request));
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
  vi.stubGlobal('fetch',vi.fn(async(path:string,init?:RequestInit)=>{
    expect(init?.method).toBe('POST');
    const headers=new Headers(init?.headers);
    expect(headers.get('X-Suma-Expected-User')).toBe(session.user.id);
    expect(headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
    const body=JSON.parse(String(init?.body));
    if(path===base+'/preview'){
      previews.push(body);
      return previewReply(body);
    }
    expect(path).toBe(base);
    writes.push(body);
    return writeReply(body);
  }));
});
afterEach(async()=>{
  if(root)await act(async()=>root!.unmount());
  root=undefined;
  host.remove();
  await setLocale('zh-CN',{persist:false});
  vi.unstubAllGlobals();
});

async function render(selection:RecordItem[]=tasks,authorityError?:unknown){
  await act(async()=>{
    root??=createRoot(host);
    root.render(<ApiViewProvider session={session}>
      <TaskPlanningStatusChanges project={project} userID={session.user.id} organizationID={session.organization.id}
        tasks={selection} authorityError={authorityError} onChanged={onChanged} onRefresh={onRefresh} onForbidden={onForbidden}/>
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
  const select=document.querySelector<HTMLSelectElement>('[data-task-planning-status-target]')!;
  await act(async()=>{
    select.value=value;
    select.dispatchEvent(new Event('change',{bubbles:true}));
  });
}

it('shows a per-task preview and writes the exact authorized selection once',async()=>{
  await render();
  await click(launch);
  expect(previews).toEqual([{project_id:project,tasks:expectedSelection}]);
  expect(button(apply).disabled).toBe(true);
  await pick('todo');
  expect(button(apply).textContent).toContain('1 to change');
  expect(document.body.textContent).toContain('1 to change · 1 unchanged');
  expect(document.querySelector('[data-task-planning-status-row=task-a]')?.textContent).toContain(stateLabel('draft'));
  expect(document.querySelector('[data-task-planning-status-row=task-b]')?.textContent).toContain('Unchanged');
  expect(writes).toHaveLength(0);
  await click(apply);
  expect(writes).toEqual([{project_id:project,status:'todo',tasks:expectedSelection,idempotency_key:expect.any(String)}]);
  expect(document.querySelector(receipt)?.textContent).toContain('Plan status changes saved');
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('explains the blocked task and prevents partial submission in a mixed selection',async()=>{
  await setLocale('zh-CN',{persist:false});
  previewReply=async request=>{
    const response=previewResult(request);
    response.items[1].eligibility='active_run';
    return Response.json(response);
  };
  await render();
  await click(launch);
  expect(document.body.textContent).toContain('本批次有 1 项无法调整');
  expect(document.querySelector('[data-task-planning-status-row=task-b]')?.textContent).toContain('请先核对原运行');
  expect(document.querySelector('[data-task-planning-status-target]')).toBeNull();
  await click(apply);
  expect(button(apply).disabled).toBe(true);
  expect(writes).toHaveLength(0);
});

it('blocks no-op and stale selections without sending a write',async()=>{
  await render([tasks[0]]);
  await click(launch);
  await pick('draft');
  expect(button(apply).disabled).toBe(true);
  await pick('blocked');
  expect(button(apply).disabled).toBe(false);
  await render([{...tasks[0],version:4}]);
  expect(button(apply).disabled).toBe(true);
  await click('[data-task-planning-status-refresh]');
  expect(onRefresh).toHaveBeenCalledTimes(1);
  expect(writes).toHaveLength(0);
});

it('validates the receipt and restores the original operation after remount without selection',async()=>{
  writeReply=async request=>{
    const result=writeResult(request);
    result.changes[0].previous_status='accepted';
    return Response.json(result);
  };
  await render();
  await click(launch);
  await pick('blocked');
  await click(apply);
  expect(document.querySelector(receipt)).toBeNull();
  expect(button(restore).disabled).toBe(false);
  const original=writes[0];
  await act(async()=>root!.unmount());
  root=undefined;
  await render([]);
  await click(launch);
  writeReply=async request=>Response.json(writeResult(request));
  await click(restore);
  expect(writes).toEqual([original,original]);
  expect(previews).toHaveLength(1);
  expect(document.querySelector(receipt)).not.toBeNull();
  expect(onChanged).toHaveBeenCalledTimes(1);
});

it('rejects an unbound preview and hides task facts if permission is lost before the next read completes',async()=>{
  previewReply=async request=>{
    const result=previewResult(request);
    result.items[0].version++;
    return Response.json(result);
  };
  await render();
  await click(launch);
  expect(button(apply).disabled).toBe(true);
  expect(document.querySelector('[data-task-planning-status-row]')).toBeNull();
  let finish!:(response:Response)=>void;
  previewReply=()=>new Promise(resolve=>{finish=resolve;});
  await click('[data-task-planning-status-preview-retry]');
  await render(tasks,new ApiError(403,'forbidden','Access changed'));
  await act(async()=>finish(Response.json(previewResult(previews[1]))));
  expect(document.querySelector('[data-task-planning-status-row]')).toBeNull();
  expect(document.body.textContent).not.toContain('Update requirements');
  expect(writes).toHaveLength(0);
});
