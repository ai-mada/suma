// @vitest-environment jsdom
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from '../i18n';
import {Overview} from './Overview';
import {ProjectFocus} from './ProjectFocus';

const date='2026-10-29T04:00:00Z';
const item=(id:string,reason='task_unstarted',change:Record<string,unknown>={})=>({task_id:id,title:id,task_version:2,task_status:'todo',run_id:'',run_state:'',reason,updated_at:date,priority:'high',...change});
function focus(project='project-a'){
 return {project_id:project,observed_at:date,sections:[
  {key:'planning',total:18,items:[item('Plan current work')],truncated:true},
  {key:'execution',total:1,items:[item('Run current work','run_active',{task_status:'in_progress',run_id:'run-a',run_state:'running'})],truncated:false},
  {key:'review',total:1,items:[item('Inspect current result','run_succeeded_review_pending',{task_status:'in_progress',run_id:'run-b',run_state:'succeeded'})],truncated:false},
  {key:'attention',total:1,items:[item('Confirm original execution','run_reconciling',{task_status:'in_progress',run_id:'run-c',run_state:'reconciling'})],truncated:false},
 ]};
}
let host:HTMLDivElement,root:Root,reads:string[],send:(path:string)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];
 send=async path=>Response.json(focus(new URL('http://test'+path).searchParams.get('project_id')||''));
 vi.stubGlobal('fetch',vi.fn(async(input:unknown)=>{const path=String(input).replace(/^\/api\/v1/,'');reads.push(path);return send(path);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function settle(){for(let index=0;index<3;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(node:ReactNode){await act(async()=>root.render(node));await settle();}
async function click(selector:string){const target=host.querySelector<HTMLButtonElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());await settle();}
async function toggleDiagnostics(open:boolean){const target=host.querySelector<HTMLDetailsElement>('[data-focus-diagnostics]');expect(target).not.toBeNull();await act(async()=>{target!.open=open;target!.dispatchEvent(new Event('toggle'));});await settle();}
const navigate=vi.fn();

it('shows server totals, keeps review distinct from acceptance, and drills into the original task without writes',async()=>{
 const open=vi.fn();await render(<ProjectFocus project="project-a" refresh={0} navigate={navigate} openRecord={open}/>);
 expect(host.querySelector('[data-focus-filter=planning]')?.textContent).toContain('18');
 expect(host.textContent).toContain('Showing 1 of 18');
 await click('[data-focus-filter=review]');expect(host.querySelectorAll('[data-focus-task]')).toHaveLength(1);
 expect(host.textContent).toContain('Execution finished; review pending');expect(host.textContent).toContain('independent evidence');
 await click('[data-focus-task="Inspect current result"]');expect(open).toHaveBeenCalledWith('Inspect current result');
 expect(reads.every(path=>path.startsWith('/project-focus?'))).toBe(true);
});

it('keeps the section and keyboard focus on language changes without rereading or interpreting user titles',async()=>{
 const current=focus();current.sections[3].items[0].title='<script>original</script> {{title}}';send=async()=>Response.json(current);
 await render(<ProjectFocus project="project-a" refresh={0} navigate={navigate} openRecord={vi.fn()}/>);await click('[data-focus-filter=attention]');
 const selected=host.querySelector<HTMLButtonElement>('[data-focus-filter=attention]')!;selected.focus();const before=reads.length;
 await act(async()=>setLocale('zh-CN',{persist:false}));
 expect(selected.getAttribute('aria-pressed')).toBe('true');expect(document.activeElement).toBe(selected);expect(reads).toHaveLength(before);
 expect(host.textContent).toContain('需关注');expect(host.textContent).toContain('<script>original</script> {{title}}');expect(host.querySelector('script')).toBeNull();
});

it('clears the prior project immediately, resets its filter and ignores a late response after navigation',async()=>{
 let release:(response:Response)=>void=()=>{};send=async path=>path.includes('project-a')?new Promise<Response>(resolve=>{release=resolve;}):Response.json(focus('project-b'));
 await render(<ProjectFocus project="project-a" refresh={0} navigate={navigate} openRecord={vi.fn()}/>);
 await render(<ProjectFocus project="project-b" refresh={0} navigate={navigate} openRecord={vi.fn()}/>);
 expect(host.querySelector('[data-focus-filter=all]')?.getAttribute('aria-pressed')).toBe('true');
 const old=focus('project-a');old.sections[0].items[0].title='Prior project secret';await act(async()=>release(Response.json(old)));await settle();
 expect(host.textContent).not.toContain('Prior project secret');await click('[data-focus-filter=review]');
 await render(<ProjectFocus project="project-c" refresh={0} navigate={navigate} openRecord={vi.fn()}/>);
 expect(host.querySelector('[data-focus-filter=all]')?.getAttribute('aria-pressed')).toBe('true');
});

it('loads execution-basis diagnostics only on expansion and keeps whole-project delivery, attention and milestones intact on a focus error',async()=>{
 let valid=false;send=async path=>{
  if(path.startsWith('/project-focus?'))return Response.json(valid?focus():{...focus(),project_id:'other-project'});
  if(path.startsWith('/overview?'))return Response.json({project:{id:'project-a',title:'Current upstream project'},counts:{requirements:45,tasks:100,runs:{total:99}},as_of:date,delivery:{scope:'whole_project',total_tasks:100,scope_tasks:90,accepted_tasks:27,cancelled_tasks:10,running_tasks:7,waiting_tasks:3,review_tasks:1,blocked_tasks:0,reconciling_tasks:0},attention:{review:{kind:'review',total:1,items:[{id:'formal-review',title:'Formal review queue',status:'in_review',owner_id:'',owner_name:'',run_id:'run-reviewed',run_state:'succeeded',updated_at:date,due_date:''}],next_cursor:'',as_of:date,date_basis:'UTC'}},milestones:{items:[{id:'milestone-a',title:'Original milestone',target_date:'2026-11-02',status:'planned'}],total:1},activity:[{id:'event-a',title:'Saved original requirement',at:date}]});
  return Response.json({});
 };
 await render(<Overview project="project-a" refresh={0} navigate={navigate} openRecord={vi.fn()} onOpenRisks={vi.fn()} onOpenApplications={vi.fn()}/>);
 expect(reads).toEqual(['/overview?project_id=project-a']);expect(host.querySelector('.project-focus')).toBeNull();
 const unchanged=()=>{expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('27');expect(host.querySelector('[role=progressbar]')?.getAttribute('aria-valuenow')).toBe('30');expect(host.textContent).toContain('Formal review queue');expect(host.textContent).toContain('Original milestone');expect(host.textContent).toContain('Saved original requirement');expect(host.textContent).toContain('45');expect(host.textContent).toContain('99');};unchanged();
 await toggleDiagnostics(true);unchanged();
 expect(host.querySelector('[data-focus-filter=planning]')?.textContent).toContain('—');expect(host.querySelector('[data-focus-task]')).toBeNull();
 const overviewReads=reads.filter(path=>path.startsWith('/overview?')).length;valid=true;await click('.project-focus .error button');
 expect(host.querySelectorAll('[data-focus-task]')).toHaveLength(4);expect(reads.filter(path=>path.startsWith('/overview?'))).toHaveLength(overviewReads);
 await toggleDiagnostics(false);expect(host.querySelector('.project-focus')).toBeNull();unchanged();
 expect(reads.some(path=>path.startsWith('/tasks?'))).toBe(false);
});

it('keeps focus independently usable through an overview failure without refetching overview on focus retry',async()=>{
 send=async path=>path.startsWith('/overview?')?Response.json({error:{code:'unavailable',message:'Overview temporarily unavailable'}},{status:503}):Response.json(focus());
 await render(<Overview project="project-a" refresh={0} navigate={navigate} openRecord={vi.fn()} onOpenRisks={vi.fn()} onOpenApplications={vi.fn()}/>);
 expect(host.querySelector('.project-focus')).toBeNull();expect(reads).toEqual(['/overview?project_id=project-a']);await toggleDiagnostics(true);expect(host.querySelectorAll('[data-focus-task]')).toHaveLength(4);expect(host.textContent).toContain('Overview temporarily unavailable');
 const before=reads.filter(path=>path.startsWith('/overview?')).length;await click('.pf-refresh');expect(host.querySelectorAll('[data-focus-task]')).toHaveLength(4);expect(reads.filter(path=>path.startsWith('/overview?'))).toHaveLength(before);
});

it('clears both private focus and delivery facts when the overview loses project access',async()=>{
 let denied=false;send=async path=>path.startsWith('/overview?')?denied?Response.json({error:{code:'forbidden',message:'Project access revoked'}},{status:403}):Response.json({project:{title:'Private project'},counts:{tasks:27},activity:[]}):Response.json(focus());
 const props={project:'project-a',navigate,openRecord:vi.fn(),onOpenRisks:vi.fn(),onOpenApplications:vi.fn()};
 await render(<Overview {...props} refresh={0}/>);await toggleDiagnostics(true);expect(host.querySelectorAll('[data-focus-task]')).toHaveLength(4);denied=true;await render(<Overview {...props} refresh={1}/>);
 expect(host.querySelector('.project-focus')).toBeNull();expect(host.querySelector('[data-focus-task]')).toBeNull();expect(host.textContent).not.toContain('Private project');expect(host.textContent).toContain('Project access revoked');
});
