// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {WorkspaceTaskContext as Context,WorkspaceTaskPage} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import {WorkbenchTaskContext} from './WorkbenchTaskContext';
import {validWorkbenchTaskContext,validWorkbenchTaskPage,workbenchTaskLinks} from './workbench-task-model';
import {setLocale} from './i18n';
import type {Session} from './api';

const session=(user='u'):Session=>({user:{id:user,name:user,email:user+'@example.test'},organization:{id:'org-'+user,name:'Org'},projects:[]});
const counts=()=>({total:0,waiting:0,running:0,succeeded:0,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[]});
function context(taskID='task-a',project='p',title='Current task'):Context{return {project_id:project,observed_at:'2026-10-29T01:00:00Z',task:{id:taskID,version:2,title,description:'Task objective',acceptance_criteria:['Original criterion'],status:'in_review',priority:'medium',owner_id:'',requirement_id:'',run_id:'run-'+taskID},requirement:null,owner:null,design:{state:'none',context:null},delivery:{project_id:project,task_id:taskID,task_version:2,task_status:'in_review',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-'+taskID,version:3,state:'succeeded',nodes:counts(),tests:counts()},expert_plans:{scope:'current_run',total:0,limit:20,complete:true,items:[]},issues:{scope:'current_run',total:0,limit:20,complete:true,items:[]}},quality:{plan_id:'plan-a',revision:1,plan_sha256:'a'.repeat(64),run_id:'run-'+taskID,run_state:'succeeded',state:'passed',basis_current:true,total:2,passed:2,failed:0,errors:0,skipped:0,not_run:0}};}
const page=(search='',items=[{id:'task-b',version:1,title:'Other task',status:'todo',priority:'medium'}],next_cursor='',total=items.length):WorkspaceTaskPage=>({project_id:'p',search,items,total,limit:20,next_cursor});
function designedContext(taskID='task-a'){
 const value=context(taskID);value.task.requirement_id='req-a';value.requirement={id:'req-a',version:4,title:'Current requirement',description:'Original requirement objective',acceptance_criteria:['Requirement criterion']};
 value.design={state:'current',context:{requirement_id:'req-a',requirement_version:4,revision:2,sha256:'b'.repeat(64),task_key:taskID,adoption_id:'adoption-a',content:{product:'Product decisions',experience:'Experience decisions',technical:'Technical decisions'}}};return value;
}
const deferred=<T,>()=>{let resolve!:(value:T)=>void;const promise=new Promise<T>(done=>{resolve=done;});return {promise,resolve};};
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function render(task='task-a',user='u',project='p'){const current=session(user);await act(async()=>root.render(<ApiViewProvider session={current}><WorkbenchTaskContext session={current} project={project} selectedTask={task} refresh={0} onTaskSelect={()=>true}/></ApiViewProvider>));}
async function settle(ready:()=>boolean){for(let step=0;step<40;step++){if(ready())return;await act(async()=>{await new Promise(done=>setTimeout(done,0));});}expect(ready()).toBe(true);}
const button=(name:string)=>[...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.trim()===name||item.getAttribute('aria-label')===name);
async function click(item:HTMLElement|undefined){expect(item).toBeDefined();await act(async()=>item!.click());}
async function input(value:string){const target=host.querySelector<HTMLInputElement>('input[type=search]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(target,value);target.dispatchEvent(new Event('input',{bubbles:true}));});}

it('T126 validates one exact task projection and keeps test counts distinct from formal acceptance and original evidence links',()=>{
 const value=context();expect(validWorkbenchTaskContext(value,'p','task-a')).toBe(true);
 for(const invalid of [{...value,project_id:'other'},{...value,delivery:{...value.delivery,task_version:1}},{...value,delivery:{...value.delivery,formal_acceptance:true}},{...value,task:{...value.task,run_id:'other-run'}},{...value,quality:{...value.quality,total:-1}}])expect(validWorkbenchTaskContext(invalid,'p','task-a')).toBe(false);
 const links=workbenchTaskLinks(value);expect(new URL(links.task,location.href).searchParams.get('record')).toBe('task-a');expect(new URL(links.run!,location.href).searchParams.get('run')).toBe('run-task-a');const quality=new URL(links.quality!,location.href);expect(quality.searchParams.get('record')).toBe('plan-a');expect(quality.searchParams.get('run')).toBe('run-task-a');expect(quality.searchParams.get('panel')).toBe('test-results');
 expect(validWorkbenchTaskPage(page('literal %_'),'p','literal %_')).toBe(true);expect(validWorkbenchTaskPage(page('other'),'p','literal %_')).toBe(false);expect(validWorkbenchTaskPage({...page(),items:[page().items[0],page().items[0]],total:2},'p','')).toBe(false);
 expect(validWorkbenchTaskPage(page('',undefined,'cursor.next'),'p','')).toBe(true);expect(validWorkbenchTaskPage(page('',undefined,'x'.repeat(2100)),'p','')).toBe(true);expect(validWorkbenchTaskPage(page('',undefined,'bad\nnext'),'p','')).toBe(false);expect(validWorkbenchTaskPage(page('',[{...page().items[0],id:'.task-a'}]),'p','')).toBe(true);
});

it('T126 restores a selected task outside the picker with one exact read and switches complete bilingual facts without another request',async()=>{
 const selected=designedContext();
 selected.delivery.repair={issue_id:'issue-source',cycle:2,state:'active'};
 const contextRead=vi.fn(async(url:URL)=>{expect(url.pathname).toBe('/api/v1/workspace/task-context');expect(url.searchParams.get('project_id')).toBe('p');expect(url.searchParams.get('task_id')).toBe('task-a');return Response.json(selected);});
 const subscriptionRead=vi.fn(async(url:URL)=>{expect(url.pathname).toBe('/api/v1/tasks/task-a/subscription');expect(url.searchParams.get('project_id')).toBe('p');return Response.json({project_id:'p',task_id:'task-a',user_id:'u',version:0,following:false,updated_at:''});});
 const fetch=vi.fn(async(input:string,init?:RequestInit)=>{const url=new URL(String(input),location.href);expect(init?.method).toBe('GET');return url.pathname==='/api/v1/tasks/task-a/subscription'?subscriptionRead(url):contextRead(url);});vi.stubGlobal('fetch',fetch);
 await render();await settle(()=>!!host.querySelector('.workbench-task-summary'));expect(contextRead).toHaveBeenCalledTimes(1);expect(subscriptionRead).toHaveBeenCalledTimes(1);expect(fetch).toHaveBeenCalledTimes(2);expect(host.textContent).toContain('Plan cases passed');expect(host.textContent).toContain('Not formally accepted');expect(host.querySelector<HTMLDetailsElement>('.workbench-task-details')?.open).toBe(false);expect(host.textContent).toContain('Independent code sources');expect(host.textContent).toContain('does not bind this editor');
 await click(host.querySelector<HTMLElement>('.workbench-task-details > summary')??undefined);
 const issue=host.querySelector<HTMLAnchorElement>('[data-task-delivery-repair-issue]');
 expect(new URL(issue!.href).searchParams.get('record')).toBe('issue-source');
 expect(contextRead).toHaveBeenCalledTimes(1);
 expect(host.querySelector<HTMLDetailsElement>('.workbench-task-requirement')?.open).toBe(false);expect(host.textContent).toContain('Original requirement objective');expect(host.textContent).toContain('Requirement criterion');expect(host.textContent).toContain('Requirement version: 4');
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('计划用例通过');expect(host.textContent).toContain('尚未正式验收');expect(host.textContent).toContain('不会将编辑器目录绑定');expect(contextRead).toHaveBeenCalledTimes(1);expect(subscriptionRead).toHaveBeenCalledTimes(1);expect(fetch).toHaveBeenCalledTimes(2);
});

it('keeps current business and acceptance facts distinct from a changed original run in the collapsed summary',async()=>{
 const value=context();value.delivery.run_basis_state='changed';
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json(value)));
 await render();await settle(()=>!!host.querySelector('.workbench-task-summary'));
 const summary=host.querySelector('.workbench-task-summary')!;
 expect(summary.textContent).toContain('Current task');expect(summary.textContent).toContain('Original run snapshot');
 expect(summary.textContent).toContain('Task basis changed');expect(summary.textContent).toContain('Not formally accepted');
 expect(host.querySelector<HTMLDetailsElement>('.workbench-task-details')?.open).toBe(false);
 expect(host.querySelector('[data-conversation-toggle]')?.getAttribute('aria-expanded')).toBe('false');
});

it('clears expanded old-project facts and ignores their late refresh when the same task id opens in another project',async()=>{
 const late=deferred<Response>();let oldReads=0;const contextProjects:string[]=[],subscriptionProjects:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const url=new URL(String(input),location.href),project=url.searchParams.get('project_id')!;
  expect(init?.method).toBe('GET');
  if(url.pathname==='/api/v1/tasks/task-a/subscription'){subscriptionProjects.push(project);return Response.json({project_id:project,task_id:'task-a',user_id:'u',version:0,following:false,updated_at:''});}
  expect(url.pathname).toBe('/api/v1/workspace/task-context');expect(url.searchParams.get('task_id')).toBe('task-a');contextProjects.push(project);
  if(project==='p'&&++oldReads>1)return late.promise;
  return Response.json(context('task-a',project,project==='p'?'Private first project task':'Second project task'));
 }));
 await render();await settle(()=>!!host.querySelector('.workbench-task-summary'));
 await click(host.querySelector<HTMLElement>('.workbench-task-details summary')!);
 expect(host.querySelector<HTMLDetailsElement>('.workbench-task-details')?.open).toBe(true);
 await click(button('Refresh context'));await render('task-a','u','second-project');
 await settle(()=>host.textContent!.includes('Second project task'));
 expect(host.textContent).not.toContain('Private first project task');
 expect(host.querySelector<HTMLDetailsElement>('.workbench-task-details')?.open).toBe(false);
 await act(async()=>late.resolve(Response.json(context('task-a','p','Late private first project task'))));
 expect(host.textContent).not.toContain('Late private first project task');
 expect(host.querySelector('.workbench-task-selected')?.textContent).toBe('Second project task');expect(contextProjects).toEqual(['p','p','second-project']);expect(subscriptionProjects).toEqual(['p','second-project']);
});

it('T126 performs explicit literal search and pagination while rejected task changes preserve the original context and picker',async()=>{
 const searches:string[]=[],selected:string[]=[];let blocked=true;
 const guard=(event:Event)=>{if(blocked)event.preventDefault();};window.addEventListener('suma:navigate',guard);
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),location.href);if(url.pathname.endsWith('/tasks')){const search=url.searchParams.get('search')||'';searches.push(search+':'+(url.searchParams.get('cursor')||''));return Response.json(page(search,url.searchParams.get('cursor')?[{id:'task-c',version:1,title:'Third task',status:'todo',priority:'medium'}]:undefined,url.searchParams.get('cursor')?'':'next',2));}return Response.json(designedContext(url.searchParams.get('task_id')!));}));
 function Harness(){const [task,setTask]=useState('task-a'),current=session();return <ApiViewProvider session={current}><WorkbenchTaskContext session={current} project="p" selectedTask={task} refresh={0} onTaskSelect={id=>{selected.push(id);if(!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))return false;setTask(id);return true;}}/></ApiViewProvider>;}
 try{await act(async()=>root.render(<Harness/>));await settle(()=>!!host.querySelector('.workbench-task-summary'));await click(host.querySelector<HTMLElement>('.workbench-task-details summary')!);const originalRequirement=host.querySelector<HTMLAnchorElement>('.knowledge-meta a');expect(originalRequirement).not.toBeNull();const navigation=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>{originalRequirement!.dispatchEvent(navigation);});expect(navigation.defaultPrevented).toBe(true);expect(host.textContent).toContain('Finish or reconcile');await click(button('Change task'));await settle(()=>!!button('Load more tasks'));await input('literal %_');expect(searches).toEqual([':']);await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle(()=>searches.includes('literal %_:'));await click(button('Load more tasks'));await settle(()=>host.textContent!.includes('Third task'));expect(searches).toEqual([':','literal %_:','literal %_:next']);
  await click([...host.querySelectorAll<HTMLButtonElement>('.workbench-task-options button')].find(item=>item.textContent?.includes('Other task')));expect(selected).toEqual(['task-b']);expect(host.querySelector('.workbench-task-selected')?.textContent).toBe('Current task');expect(host.querySelector('form')).not.toBeNull();expect(host.textContent).toContain('Finish or reconcile');await click(button('Clear task'));expect(host.querySelector('.workbench-task-summary')).not.toBeNull();
  blocked=false;await click([...host.querySelectorAll<HTMLButtonElement>('.workbench-task-options button')].find(item=>item.textContent?.includes('Other task')));await settle(()=>!host.querySelector('form'));expect(selected).toEqual(['task-b','','task-b']);
 }finally{window.removeEventListener('suma:navigate',guard);}
});

it('T126 rejects late owner responses and clears context and picker on permission loss before an explicit authorized retry',async()=>{
 const old=deferred<Response>(),retryPicker=deferred<Response>();let calls=0,denied=false,rechecking=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),location.href);if(url.pathname.endsWith('/tasks'))return rechecking?retryPicker.promise:Response.json(page());if(++calls===1)return old.promise;if(denied)return Response.json({error:{code:'forbidden',message:'Access revoked'}},{status:403});return Response.json(context(url.searchParams.get('task_id')!,url.searchParams.get('project_id')!,'New owner task'));}));
 await render('task-a','first');await render('task-b','second');await settle(()=>host.textContent!.includes('New owner task'));await act(async()=>old.resolve(Response.json(context('task-a','p','Private old task'))));expect(host.textContent).not.toContain('Private old task');await click(button('Change task'));await settle(()=>host.textContent!.includes('Other task'));denied=true;await click(button('Refresh context'));await settle(()=>host.textContent!.includes('Access changed'));expect(host.textContent).not.toContain('New owner task');expect(host.textContent).not.toContain('Other task');expect(host.querySelector('.workbench-task-summary')).toBeNull();
 denied=false;rechecking=true;await click(button('Reload'));await settle(()=>!!host.querySelector('.workbench-task-summary'));expect(host.textContent).toContain('New owner task');expect(host.textContent).not.toContain('Private old task');expect(host.textContent).not.toContain('Other task');await act(async()=>retryPicker.resolve(Response.json(page('',[{id:'task-c',version:1,title:'Restored authorized task',status:'todo',priority:'medium'}]))));await settle(()=>host.textContent!.includes('Restored authorized task'));expect(host.textContent).not.toContain('Other task');
});
