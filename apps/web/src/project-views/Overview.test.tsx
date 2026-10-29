// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {Overview} from './Overview';
import {setLocale} from '../i18n';

const at='2026-10-29T08:00:00Z';
const task=(id:string,title:string,status='in_review')=>({id,title,status,owner_id:'owner-1',owner_name:'Lin',run_id:'run-1',run_state:'succeeded',updated_at:at,due_date:'2026-10-28'});
const page=(kind:string,items:ReturnType<typeof task>[],total=items.length,next_cursor='')=>({kind,items,total,next_cursor,as_of:at,date_basis:'UTC'});
function fixture(project='project-a'){return {project:{id:project,title:'Project <raw>',description:'Original description'},counts:{requirements:12,tasks:1000,issues:9,runs:20},as_of:at,delivery:{scope:'whole_project',total_tasks:1000,scope_tasks:980,accepted_tasks:340,cancelled_tasks:20,running_tasks:7,waiting_tasks:8,review_tasks:15,blocked_tasks:3,reconciling_tasks:2},attention:{review:page('review',[task('review-1','Review original')],15,'review-next'),blocked:page('blocked',[task('blocked-1','Blocked original','blocked')],3),overdue:page('overdue',[task('overdue-1','Overdue original','in_progress')],4),reconciling:page('reconciling',[],2)},milestones:{items:[{id:'milestone-1',title:'Release candidate',target_date:'2026-11-02',status:'planned'}],total:1},activity:[{id:'created',kind:'task.created',title:'Task created',at},{id:'reviewed',kind:'task.reviewed',title:'Task reviewed',at},{id:'conversation',kind:'task_conversation.posted',title:'Raw conversation',at},{id:'unknown',kind:'unrecognized.event',title:'Unknown event',at}]};}
let host:HTMLDivElement,root:Root|undefined,data:any,paths:string[],nextPage:()=>Promise<unknown>;
const navigate=vi.fn(),openRecord=vi.fn(),onOpenRisks=vi.fn(),onOpenApplications=vi.fn(),onIntelligent=vi.fn();
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);data=fixture();paths=[];nextPage=async()=>page('review',[task('review-2','Next review')],15);
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');paths.push(path);let body:unknown;
  if(path==='/overview')body=data;
  else if(path==='/project-focus')body={project_id:url.searchParams.get('project_id'),observed_at:at,sections:['planning','execution','review','attention'].map(key=>({key,total:0,items:[],truncated:false}))};
  else if(path==='/overview/attention')body=await nextPage();
  else if(path==='/tasks')body={items:[task('paged-task','Unrelated paged task')],total:1000};
  else return new Response(JSON.stringify({error:{code:'not_found',message:'No summary'}}),{status:404,headers:{'Content-Type':'application/json'}});
  return new Response(JSON.stringify(body),{headers:{'Content-Type':'application/json'}});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.clearAllMocks();});
async function settle(){for(let n=0;n<5;n++)await act(async()=>{await Promise.resolve();});}
async function render(project='project-a',refresh=0){await act(async()=>{root??=createRoot(host);root.render(<Overview project={project} refresh={refresh} navigate={navigate} openRecord={openRecord} onOpenRisks={onOpenRisks} onOpenApplications={onOpenApplications} onIntelligent={onIntelligent}/>);});await settle();}
const button=(text:string)=>[...host.querySelectorAll<HTMLButtonElement>('button')].find(item=>item.textContent?.includes(text))!;
async function click(text:string){const target=button(text);expect(target).toBeDefined();await act(async()=>target.click());await settle();}

it('uses full-project delivery and queue totals from one overview read without requesting paged tasks',async()=>{
 await render();expect(paths).toEqual(['/overview']);expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('340');expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('980');expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('1,000');expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('15');expect(host.textContent).toContain('Review original');expect(host.textContent).not.toContain('Unrelated paged task');expect(host.querySelector('.quick-path')).toBeNull();expect(paths).not.toContain('/tasks');
});
it('combines the project heading and actions with one primary planning action',async()=>{
 await render();expect(host.querySelector('.po-header h1')?.textContent).toBe('Project <raw>');expect(host.querySelectorAll('.po-header button.primary')).toHaveLength(1);await click('Plan requirements');expect(navigate).toHaveBeenCalledWith('requirements');const intelligent=host.querySelector<HTMLButtonElement>('.po-header button.intent-entry');expect(intelligent).not.toBeNull();await act(async()=>intelligent!.click());expect(onIntelligent).toHaveBeenCalledOnce();expect(paths).toEqual(['/overview']);
});
it('supports arrow, Home and End keys on queue tabs and opens the selected task',async()=>{
 await render();const review=host.querySelector<HTMLButtonElement>('[role=tab][aria-selected=true]')!;expect(review).not.toBeNull();review.focus();await act(async()=>review.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('Blocked');expect(document.activeElement?.getAttribute('aria-selected')).toBe('true');await click('Blocked original');expect(openRecord).toHaveBeenCalledWith('blocked-1');
 await act(async()=>document.activeElement!.dispatchEvent(new KeyboardEvent('keydown',{key:'End',bubbles:true})));expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('Overdue');await act(async()=>document.activeElement!.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true})));expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('Pending review');
});
it('appends matching queue pages and ignores a late page after the selected tab changes',async()=>{
 await render();await click('Load more');expect(host.textContent).toContain('Next review');expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('15');
 data=fixture();await render('project-a',1);let resolve!:(value:unknown)=>void;nextPage=()=>new Promise(done=>{resolve=done;});await click('Load more');await click('Blocked');await act(async()=>resolve(page('review',[task('late','Private late review')],15)));await settle();expect(host.textContent).not.toContain('Private late review');expect(host.textContent).toContain('Blocked original');
});
it('replaces the queue with one fresh first page when its total changes during continuation',async()=>{
 data.attention.review=page('review',Array.from({length:6},(_,index)=>task('old-'+index,'Old queued task '+index)),15,'review-next');let reads=0;nextPage=async()=>++reads===1?page('review',[task('continuation','Outdated continuation')],1):page('review',[task('current','Current remaining task')],1);await render();await click('Load more');expect(reads).toBe(2);expect(host.textContent).not.toContain('Old queued task');expect(host.textContent).not.toContain('Outdated continuation');expect(host.textContent).toContain('Current remaining task');expect(host.textContent).toContain('Showing 1 of 1 tasks');expect(host.textContent).toContain('The queue changed');const requests=vi.mocked(fetch).mock.calls.map(([input])=>new URL(String(input),location.href)).filter(url=>url.pathname.endsWith('/overview/attention'));expect(requests[0].searchParams.get('cursor')).toBe('review-next');expect(requests[1].searchParams.has('cursor')).toBe(false);
});
it('keeps whole-project delivery marked stale across paging until an authorized overview refresh',async()=>{
 const first=Array.from({length:6},(_,index)=>task('current-'+index,'Current queued task '+index));data.delivery.review_tasks=7;data.attention.review=page('review',first,7,'review-next');let reads=0;nextPage=async()=>++reads===1?page('review',[task('new','New pending task')],8):reads===2?page('review',first,8,'fresh-next'):page('review',[task('last-1','Last task one'),task('last-2','Last task two')],8);
 await render();await click('Load more');expect(host.querySelector('.po-delivery-stale')?.textContent).toContain('Delivery snapshot is out of date');expect(host.querySelector('.po-metric-review strong')?.textContent).toBe('7');expect(host.querySelector('.po-delivery-foot time')?.getAttribute('dateTime')).toBe(at);expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('8');
 await click('Load more');await click('Blocked');await click('Pending review');expect(host.querySelector('.po-delivery-stale')).not.toBeNull();expect(host.textContent).toContain('Last task two');expect(host.querySelector('.po-metric-review strong')?.textContent).toBe('7');
 data={...data,as_of:'2026-10-29T09:00:00Z',delivery:{...data.delivery,review_tasks:8},attention:{...data.attention,review:page('review',first,8,'new-snapshot-next')}};await click('Refresh project');expect(paths.filter(path=>path==='/overview')).toHaveLength(2);expect(paths.filter(path=>path==='/overview/attention')).toHaveLength(3);expect(host.querySelector('.po-delivery-stale')).toBeNull();expect(host.querySelector('.po-metric-review strong')?.textContent).toBe('8');expect(host.querySelector('.po-delivery-foot time')?.getAttribute('dateTime')).toBe('2026-10-29T09:00:00Z');
});
it('clears a stale delivery snapshot if the full-project refresh loses access',async()=>{
 nextPage=async()=>page('review',[task('current','Current remaining task')],1);await render();await click('Load more');const base=vi.mocked(fetch).getMockImplementation()!;vi.mocked(fetch).mockImplementation(async(input,init)=>new URL(String(input),location.href).pathname==='/api/v1/overview'?new Response(JSON.stringify({error:{code:'forbidden',message:'Project refresh denied'}}),{status:403,headers:{'Content-Type':'application/json'}}):base(input,init));await click('Refresh project');expect(host.querySelector('[data-overview-delivery]')).toBeNull();expect(host.textContent).not.toContain('Project <raw>');expect(host.textContent).not.toContain('Current remaining task');expect(host.querySelector('.project-focus')).toBeNull();expect(host.textContent).toContain('Project refresh denied');
});
it('discards old project data and refuses a page for another queue kind',async()=>{
 await render();nextPage=async()=>page('blocked',[task('wrong','Wrong queue')],15);await click('Load more');expect(host.textContent).not.toContain('Wrong queue');expect(host.textContent).toContain('This queue could not be updated');data=fixture('project-b');data.attention.review=page('review',[task('new','Current project review')],1);await render('project-b');expect(host.textContent).toContain('Current project review');expect(host.textContent).not.toContain('Review original');
});
it('shows unavailable delivery and queue fields explicitly instead of zero or a made-up percentage',async()=>{
 data={project:data.project,counts:{tasks:1000},activity:[]};await render();expect(host.querySelector('[data-overview-delivery]')?.textContent).toContain('Not reported');expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('—');expect(host.querySelector('[role=progressbar]')).toBeNull();expect(host.textContent).not.toContain('0%');expect(host.querySelector('.quick-path')).toBeNull();
});
it('keeps create, conversation and ambiguous review activity distinct from approval',async()=>{
 await render();const kinds=[...host.querySelectorAll('[data-overview-event]')].map(item=>item.getAttribute('data-overview-event'));expect(kinds).toEqual(['created','review','conversation','neutral']);expect(host.querySelector('[data-overview-event=approved]')).toBeNull();expect(host.textContent).toContain('Task discussion message recorded');
});
it('retains source text and selected queue during language changes without another read',async()=>{
 await render();await click('Overdue');const reads=paths.length;await act(async()=>setLocale('zh-CN',{persist:false}));expect(paths).toHaveLength(reads);expect(host.querySelector('[role=tab][aria-selected=true]')?.textContent).toContain('已逾期');expect(host.textContent).toContain('Overdue original');expect(host.textContent).toContain('Project <raw>');
});
it('offers first steps only when the server reports a truly empty project and keeps supporting links actionable',async()=>{
 data.counts={requirements:0,tasks:0,runs:0,issues:0};data.delivery={...data.delivery,total_tasks:0,scope_tasks:0,accepted_tasks:0,cancelled_tasks:0};data.attention={review:page('review',[]),blocked:page('blocked',[]),overdue:page('overdue',[])};await render();expect(host.querySelector('.quick-path')).not.toBeNull();await click('Risk register');expect(onOpenRisks).toHaveBeenCalledOnce();await click('Application catalog');expect(onOpenApplications).toHaveBeenCalledOnce();
});
it('clears the whole private overview when a continuation read loses project access',async()=>{
 await render();vi.mocked(fetch).mockImplementationOnce(async()=>new Response(JSON.stringify({error:{code:'forbidden',message:'Project access denied'}}),{status:403,headers:{'Content-Type':'application/json'}}));await click('Load more');expect(host.textContent).not.toContain('Project <raw>');expect(host.textContent).not.toContain('Review original');expect(host.querySelector('[data-overview-delivery]')).toBeNull();expect(host.textContent).toContain('Project access denied');
});
it('does not install a pending continuation after the project changes',async()=>{
 await render();let resolve!:(value:unknown)=>void;nextPage=()=>new Promise(done=>{resolve=done;});await click('Load more');data=fixture('project-b');data.attention.review=page('review',[task('new','Current project review')],1);await render('project-b');await act(async()=>resolve(page('review',[task('late','Old project continuation')],15)));await settle();expect(host.textContent).toContain('Current project review');expect(host.textContent).not.toContain('Old project continuation');
});
it('clears a same-project snapshot on a late permission denial even after changing queue tabs',async()=>{
 await render();let resolve!:(response:Response)=>void;vi.mocked(fetch).mockImplementationOnce(()=>new Promise(done=>{resolve=done;}));await click('Load more');await click('Blocked');await act(async()=>resolve(new Response(JSON.stringify({error:{code:'forbidden',message:'Access revoked during read'}}),{status:403,headers:{'Content-Type':'application/json'}})));await settle();expect(host.textContent).not.toContain('Project <raw>');expect(host.textContent).not.toContain('Blocked original');expect(host.textContent).toContain('Access revoked during read');
});
it('does not apply a previous-project permission denial to the current project',async()=>{
 await render();let resolve!:(response:Response)=>void;vi.mocked(fetch).mockImplementationOnce(()=>new Promise(done=>{resolve=done;}));await click('Load more');data=fixture('project-b');data.attention.review=page('review',[task('new','Current project review')],1);await render('project-b');await act(async()=>resolve(new Response(JSON.stringify({error:{code:'forbidden',message:'Old project denied'}}),{status:403,headers:{'Content-Type':'application/json'}})));await settle();expect(host.textContent).toContain('Current project review');expect(host.textContent).not.toContain('Old project denied');
});
