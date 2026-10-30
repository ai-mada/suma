// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {TaskReviewRequestPanel,type TaskReviewRequestPanelProps} from './TaskReviewRequestPanel';
import {ApiViewProvider} from './api-context';
import {setLocale,tr} from './i18n';
import {reviewDraftKey} from './task-review-requests-model';
import type {Session} from './api';

const row=(patch:Record<string,unknown>={})=>({id:'request-a',version:1,project_id:'p',task_id:'t',task_version:3,run_id:'run-a',run_version:4,task_basis_sha256:'a'.repeat(64),requester_user_id:'author',requester_name_at_request:'Recorded author',reviewer_resource_id:'human-reviewer',reviewer_user_id:'reviewer',reviewer_name_at_request:'Recorded reviewer',note:'Original review context',state:'pending',created_at:'2026-10-30T02:00:00.000000000Z',updated_at:'2026-10-30T02:00:00.000000000Z',cancelled_by_user_id:'',cancel_reason:'',closed_review_id:'',binding_current:true,reviewer_available:true,is_current_assignee:false,can_cancel:true,source_available:true,...patch});
const target=(id='human-reviewer')=>({resource_id:id,user_id:'user-'+id,name:'Current eligible '+id,resource_version:2});
let root:Root,host:HTMLDivElement,session:Session,props:TaskReviewRequestPanelProps;
let currentRows:ReturnType<typeof row>[],currentPending:string,runVersion:number,runState:string;
let reads:{path:string;headers:Headers;signal?:AbortSignal|null}[],writes:{path:string;body:Record<string,any>;raw:string;headers:Headers}[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);session={user:{id:'author',name:'Author',email:'author@example.test'},organization:{id:'org',name:'Org'},projects:[]};props={session,project:'p',taskID:'t',taskVersion:3,runID:'run-a',refresh:0,onChanged:vi.fn(),onOpenReview:vi.fn(),onOpenRun:vi.fn(),onForbidden:vi.fn()};currentRows=[];currentPending='';runVersion=4;runState='succeeded';reads=[];writes=[];});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
const page=()=>({project_id:props.project,task_id:props.taskID,task_version:props.taskVersion,can_request:true,limit:20,items:currentRows,total:currentRows.length,next_cursor:'',pending_request_id:currentPending});
const detail=(value=row())=>({project_id:props.project,task_id:props.taskID,task_title:'Current task',task_version:props.taskVersion,can_request:true,request:value});
// GET /runs/:id returns the formal DAG Run shape. Its task binding is held
// by the task and checked again by Create; the Run has no task_id field.
const runWire=(patch:Record<string,unknown>={})=>({id:props.runID,tenant_id:'org',project_id:props.project,workflow_id:'workflow-a',workflow_name:'Independent tests',workflow_version:1,version:runVersion,state:runState,created_at:'2026-10-30T01:00:00Z',updated_at:'2026-10-30T02:00:00Z',nodes:[],...patch});
function transport(reply?:(path:string,init?:RequestInit)=>Response|undefined|Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
 const path=String(input).replace(/^\/api\/v1/,''),headers=new Headers(init?.headers);if(init?.body)writes.push({path,body:JSON.parse(String(init.body)),raw:String(init.body),headers});else reads.push({path,headers,signal:init?.signal});
 const result=await reply?.(path,init);if(result)return result;
 if(path.startsWith('/runs/'))return Response.json(runWire());
 if(path.startsWith('/task-review-targets?')){const params=new URLSearchParams(path.split('?')[1]),id=params.get('resource_ids');return Response.json({project_id:props.project,task_id:props.taskID,limit:20,items:[target(id||undefined)],total:1,next_cursor:''});}
 if(path.startsWith('/task-review-requests?'))return Response.json(page());
 if(path.startsWith('/task-review-requests/')&&!init?.body)return Response.json(detail(currentRows.find(item=>path.includes('/'+item.id+'?'))||row()));
 throw Error('Unexpected fixture command '+path);
}));}
async function settle(){for(let i=0;i<6;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(patch:Partial<TaskReviewRequestPanelProps>={}){props={...props,...patch,session};await act(async()=>root.render(<ApiViewProvider session={session}><TaskReviewRequestPanel {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function type(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLTextAreaElement>(selector);expect(field,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
async function compose(){await click('[data-review-toggle]');await click('[data-review-compose]');await click('[data-review-adopt]');await click('[data-review-target-toggle]');await click('[data-review-target=human-reviewer]');await type('[name=review_request_note]','  Original exact request 🧪  ');}

it('shows the current pending request outside bounded history and opens only existing run/Review actions',async()=>{
 currentPending='request-a';currentRows=[row({id:'old',state:'cancelled',can_cancel:false,cancelled_by_user_id:'author',cancel_reason:'Historical',created_at:'2026-10-29T01:00:00.000000000Z'})];
 transport(path=>path.startsWith('/task-review-requests?')?Response.json({...page(),total:22,next_cursor:'older-page'}):path.startsWith('/task-review-requests/request-a?')?Response.json(detail(row({binding_current:false,reviewer_available:false}))):undefined);
 await render();expect(host.textContent).toContain('Recorded reviewer');expect(host.textContent).toContain('direct task/run binding changed');expect(host.textContent).toContain('no longer a currently eligible reviewer');
 expect(reads.filter(read=>read.path.startsWith('/task-review-requests/request-a?'))).toHaveLength(1);expect(reads.some(read=>read.path.includes('cursor='))).toBe(false);
 const actions=host.querySelector('[data-review-request=request-a] .task-review-actions')!.querySelectorAll<HTMLButtonElement>('button');await act(async()=>{actions[0].click();actions[1].click();});
 expect(props.onOpenRun).toHaveBeenCalledWith('run-a');expect(props.onOpenReview).toHaveBeenCalledTimes(1);expect(writes).toHaveLength(0);
});

it('persists an exact new-request basis and restores original JSON after a lost receipt, later versions and language change',async()=>{
 let lost=true;transport(async(path,init)=>{if(!init?.body)return;const body=JSON.parse(String(init.body));expect(path).toBe('/task-review-requests');if(lost)throw TypeError('original receipt lost');return Response.json(row({note:body.note}));});
 await render();await compose();const stored=JSON.parse(sessionStorage.getItem(reviewDraftKey('author','org','p','t'))!);expect(stored).toMatchObject({reviewer_resource_id:'human-reviewer',basis:{task_version:3,run_id:'run-a',run_version:4},note:'  Original exact request 🧪  '});expect(JSON.stringify(stored)).not.toContain('name');
 await click('[data-review-create]');expect(writes).toHaveLength(1);const leave=new Event('suma:navigate',{cancelable:true});await act(async()=>{window.dispatchEvent(leave);});expect(leave.defaultPrevented).toBe(true);
 runVersion=9;await render({taskVersion:7,refresh:1});await act(async()=>{await setLocale('zh-CN',{persist:false});});lost=false;await click('[data-review-restore]');expect(writes).toHaveLength(2);expect(writes[1].raw).toBe(writes[0].raw);expect(writes[1].body).toMatchObject({expected_task_version:3,expected_run_version:4,note:'  Original exact request 🧪  '});expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('author');expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe('org');expect(sessionStorage.getItem(reviewDraftKey('author','org','p','t'))).toBeNull();
});

it('retains a stale unsent basis until explicitly adopted and isolates it from another actor and task',async()=>{
 transport();await render();await compose();runVersion=5;await render({taskVersion:4,refresh:1});expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(true);expect(host.querySelector<HTMLTextAreaElement>('[name=review_request_note]')?.value).toBe('  Original exact request 🧪  ');expect(JSON.parse(sessionStorage.getItem(reviewDraftKey('author','org','p','t'))!).basis.task_version).toBe(3);
 await click('[data-review-adopt]');expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(false);
 session={...session,user:{...session.user,id:'other'},organization:{...session.organization,id:'other-org'}};await render();expect(host.querySelector('[name=review_request_note]')).toBeNull();expect(host.textContent).not.toContain('Original exact request');
 session={...session,user:{...session.user,id:'author'},organization:{...session.organization,id:'org'}};await render();expect(host.querySelector<HTMLTextAreaElement>('[name=review_request_note]')?.value).toBe('  Original exact request 🧪  ');
 await render({taskID:'other-task'});expect(host.textContent).not.toContain('Original exact request');expect(writes).toHaveLength(0);
});

it('keeps an unavailable reviewer ID visible and blocks a new request instead of changing its recipient',async()=>{
 let unavailable=false;transport(path=>path.includes('resource_ids=')&&unavailable?Response.json({project_id:'p',task_id:'t',limit:20,items:[],total:0,next_cursor:''}):undefined);
 await render();await compose();unavailable=true;await render({refresh:1});expect(host.textContent).toContain('Reviewer unavailable');expect(host.querySelector('[data-review-selected=human-reviewer]')).not.toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(true);await click('[data-review-create]');expect(writes).toHaveLength(0);expect(JSON.parse(sessionStorage.getItem(reviewDraftKey('author','org','p','t'))!).reviewer_resource_id).toBe('human-reviewer');
});

it('uses bounded literal reviewer search and explicit cursor loading',async()=>{
 transport(path=>{if(!path.startsWith('/task-review-targets?'))return;const params=new URLSearchParams(path.split('?')[1]);if(params.has('resource_ids'))return;if(params.has('q'))return Response.json({project_id:'p',task_id:'t',limit:20,items:[],total:0,next_cursor:''});return Response.json({project_id:'p',task_id:'t',limit:20,items:[target(params.has('cursor')?'human-b':'human-reviewer')],total:2,next_cursor:params.has('cursor')?'':'next-target'});});
 await render();await click('[data-review-toggle]');await click('[data-review-compose]');await click('[data-review-target-toggle]');await click('[data-review-target-more]');expect(host.querySelectorAll('[data-review-target]')).toHaveLength(2);expect(reads.at(-1)?.path).toContain('cursor=next-target');
 await type('.task-review-search input','  %_ reviewer  ');await click('.task-review-search button');const params=new URLSearchParams(reads.at(-1)!.path.split('?')[1]);expect(params.get('q')).toBe('  %_ reviewer  ');expect(params.has('cursor')).toBe(false);expect(params.get('limit')).toBe('20');expect(writes).toHaveLength(0);
});

it('recovers only the original cancellation version/reason after the request has already changed',async()=>{
 currentRows=[row()];currentPending='request-a';let lost=true;transport(async(_path,init)=>{if(!init?.body)return;const body=JSON.parse(String(init.body));if(lost){currentRows=[row({state:'cancelled',version:2,can_cancel:false,cancelled_by_user_id:'author',cancel_reason:body.reason})];currentPending='';throw TypeError('cancel receipt lost');}return Response.json(currentRows[0]);});
 await render();await click('[data-review-cancel=request-a]');await type('[name=review_cancel_reason]','Original cancellation');await click('[data-review-confirm-cancel]');await render({refresh:1});lost=false;await click('[data-review-restore]');expect(writes).toHaveLength(2);expect(writes[1].raw).toBe(writes[0].raw);expect(writes[1].body).toMatchObject({expected_version:1,action:'cancel',reason:'Original cancellation'});expect(writes.every(write=>write.path==='/task-review-requests/request-a/commands')).toBe(true);
});

it('can send a fresh request after cancellation using the real Run response without task_id',async()=>{
 props.taskVersion=2;runVersion=3;currentRows=[row({task_version:2,run_version:3})];currentPending='request-a';
 transport((path,init)=>{
  if(!init?.body)return;
  const body=JSON.parse(String(init.body));
  if(path==='/task-review-requests/request-a/commands'){
   currentRows=[row({task_version:2,run_version:3,state:'cancelled',version:2,can_cancel:false,cancelled_by_user_id:'author',cancel_reason:body.reason})];currentPending='';return Response.json(currentRows[0]);
  }
  if(path==='/task-review-requests'){
   const created=row({id:'request-b',task_version:body.expected_task_version,run_version:body.expected_run_version,note:body.note,created_at:'2026-10-30T02:00:01.000000000Z',updated_at:'2026-10-30T02:00:01.000000000Z'});
   currentRows=[created,...currentRows];currentPending=created.id;return Response.json(created,{status:201});
  }
 });
 await render();await click('[data-review-cancel=request-a]');await type('[name=review_cancel_reason]','Original responsibility cancelled');await click('[data-review-confirm-cancel]');
 expect(writes).toHaveLength(1);expect(currentPending).toBe('');expect(host.querySelector('[data-review-request=request-a]')?.textContent).toContain('Original responsibility cancelled');
 await click('[data-review-compose]');expect(runWire()).not.toHaveProperty('task_id');await click('[data-review-adopt]');await click('[data-review-target-toggle]');await click('[data-review-target=human-reviewer]');await type('[name=review_request_note]','A new explicit review responsibility');
 expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(false);await click('[data-review-create]');
 expect(writes).toHaveLength(2);expect(writes[1].path).toBe('/task-review-requests');expect(writes[1].body).toMatchObject({task_id:'t',expected_task_version:2,run_id:'run-a',expected_run_version:3,reviewer_resource_id:'human-reviewer',note:'A new explicit review responsibility'});expect(host.querySelector('[data-review-request=request-b]')).not.toBeNull();
});

it.each<{name:string;patch:Record<string,unknown>}>([
 {name:'another run',patch:{id:'different-run'}},
 {name:'another project',patch:{project_id:'different-project'}},
 {name:'invalid version',patch:{version:0}},
 {name:'invalid state',patch:{state:null}},
])('rejects a Run response with $name instead of enabling a review request',async({patch})=>{
 transport(path=>path.startsWith('/runs/')?Response.json(runWire(patch)):undefined);await render();await click('[data-review-toggle]');await click('[data-review-compose]');
 expect(host.querySelector('[role=alert]')?.textContent).toContain(tr('task-review-requests.invalid_read'));expect(host.querySelector('[data-review-adopt]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(true);await click('[data-review-create]');expect(writes).toHaveLength(0);
});

it('keeps a current Run that has not succeeded ineligible for a review request',async()=>{
 runState='running';transport();await render();await click('[data-review-toggle]');await click('[data-review-compose]');
 expect(host.querySelector('[data-review-adopt]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-review-create]')?.disabled).toBe(true);expect(writes).toHaveLength(0);
});

it('opens and focuses an exact older request once as history, displaying the actual administrator decision',async()=>{
 const old=row({id:'old',state:'closed',version:2,can_cancel:false,binding_current:false,closed_review_id:'review-real',created_at:'2026-10-20T02:00:00.000000000Z',decision:{review_id:'review-real',reviewer_user_id:'actual-admin',decision:'accepted',reason:'Actual immutable review reason',created_at:'2026-10-21T02:00:00Z'}});currentRows=[row()];currentPending='request-a';transport(path=>path.startsWith('/task-review-requests/old?')?Response.json(detail(old)):undefined);
 await render({selectedRequest:'old'});expect(host.querySelectorAll('[data-review-request=old]')).toHaveLength(1);expect(document.activeElement).toBe(host.querySelector('[data-review-request=old]'));expect(host.textContent).toContain('Actual decision by actual-admin');expect(host.querySelector('[data-review-request=old]')?.textContent).not.toContain('direct task/run binding changed');expect(host.querySelector('[data-review-request=old] [data-review-cancel]')).toBeNull();expect(reads.filter(read=>read.path.startsWith('/task-review-requests/old?'))).toHaveLength(1);expect(writes).toHaveLength(0);
});

it('clears all current private facts on target 404 while retaining locally authored recovery data',async()=>{
 let denied=false;transport(path=>path.startsWith('/task-review-targets?')&&denied?Response.json({error:{code:'not_found',message:'Directory access changed'}},{status:404}):undefined);
 await render();await compose();denied=true;await render({refresh:1});expect(props.onForbidden).toHaveBeenCalled();expect(host.textContent).not.toContain('Current eligible human-reviewer');expect(host.querySelector('[data-review-selected]')).toBeNull();expect(sessionStorage.getItem(reviewDraftKey('author','org','p','t'))).toContain('Original exact request');
 await click('[data-review-clear-draft]');const leave=new Event('suma:navigate',{cancelable:true});await act(async()=>{window.dispatchEvent(leave);});expect(leave.defaultPrevented).toBe(false);expect(writes).toHaveLength(0);
});

it('ignores a late old identity denial without exposing that identity’s request or reviewer names',async()=>{
 // GET identity is a scoped component/API lifetime; the API view injects
 // expected-user headers for writes only. Hold the actual first read here.
 let oldRead=true,finish!:(response:Response)=>void;transport(path=>path.startsWith('/task-review-requests?')&&oldRead?new Promise<Response>(resolve=>{finish=resolve;}):undefined);
 await render();const signal=reads[0].signal,original=finish;expect(original).toBeTypeOf('function');oldRead=false;session={...session,user:{...session.user,id:'other'}};await render();expect(signal?.aborted).toBe(true);await act(async()=>{original(Response.json({error:{code:'forbidden',message:'Previous actor refused'}},{status:403}));});await settle();expect(props.onForbidden).not.toHaveBeenCalled();expect(host.textContent).not.toContain('Previous actor refused');expect(host.textContent).toContain('No review responsibility request');expect(writes).toHaveLength(0);
});
