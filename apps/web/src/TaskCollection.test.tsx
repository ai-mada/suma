// @vitest-environment jsdom
import {act,type ComponentProps} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {TaskCollection} from './TaskCollection';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';

type Props=ComponentProps<typeof TaskCollection>;
const task=(patch:Record<string,unknown>={})=>({id:'task-a',project_id:'project-a',version:1,title:'Current scoped task',description:'Delivery context',status:'todo',priority:'high',owner_id:'human-a',requirement_id:'req-a',updated_at:'2026-10-29T02:00:00Z',...patch});
const summary=(patch:Record<string,unknown>={})=>{const value={project_id:'project-a',group_by:'status',as_of:'2026-10-29T03:00:00Z',total:3,groups:[{key:'todo',total:2},{key:'draft',total:1}],...patch};return {...value,groups_total:patch.groups_total??value.groups.length,groups_limit:20,next_group_cursor:patch.next_group_cursor??''};};
const page=(patch:Record<string,unknown>={})=>{const value={project_id:'project-a',as_of:'2026-10-29T03:01:00Z',items:[task()],total:3,limit:20,next_cursor:'',...patch};return {...value,task_plans:Object.hasOwn(patch,'task_plans')?patch.task_plans:value.items.map(row=>({task_id:row.id,iteration_id:'',plan_version:0}))};};
let host:HTMLDivElement,root:Root,props:Props,reads:{path:string;params:URLSearchParams;signal?:AbortSignal|null;headers:Headers}[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];props={project:'project-a',userID:'user-a',organizationID:'org-a',refresh:0,filters:{q:'',status:'all',layout:'list'},members:[{id:'human-a',version:1,title:'Human A'}],requirements:[{id:'req-a',version:1,title:'Requirement A'}],iterations:[],moreMembers:false,moreRequirements:false,onMoreMembers:vi.fn(),onMoreRequirements:vi.fn(),lookupBusy:false,selected:'',selection:[],onFilter:vi.fn(),onToggleSelection:vi.fn(),onRows:vi.fn(),onDenied:vi.fn(),onSelect:vi.fn()};});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
function transport(reply:(path:string,params:URLSearchParams)=>Response|Promise<Response>=path=>Response.json(path.startsWith('/task-collection/summary')?summary():page())){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,''),params=new URLSearchParams(path.split('?')[1]);reads.push({path,params,signal:init?.signal,headers:new Headers(init?.headers)});return reply(path,params);}));}
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(patch:Partial<Props>={}){props={...props,...patch};const session:Session={user:{id:props.userID,name:'User',email:'user@example.test'},organization:{id:props.organizationID,name:'Organization'},projects:[]};await act(async()=>root.render(<ApiViewProvider session={session}><TaskCollection {...props}/></ApiViewProvider>));await settle();}
async function click(selector:string){const target=host.querySelector<HTMLButtonElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());await settle();}
async function change(name:string,value:string){const target=host.querySelector<HTMLInputElement|HTMLSelectElement>('[name="'+name+'"]')!;await act(async()=>{Object.getOwnPropertyDescriptor(target instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(target,value);target.dispatchEvent(new Event('input',{bubbles:true}));target.dispatchEvent(new Event('change',{bubbles:true}));});}
async function submit(){await act(async()=>{host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('reads full server counts and bounded pages, retains all filters on load more and uses explicit task selection',async()=>{
 transport((path,params)=>Response.json(path.includes('/summary')?summary({total:101,groups:[{key:'todo',total:101}]}):page({items:[task({id:params.has('cursor')?'task-b':'task-a'})],total:101,next_cursor:params.has('cursor')?'':'opaque+/='})));await render({filters:{...props.filters,q:'literal %_',task_owner:'human-a',task_priority:'high',task_requirement:'req-a'}});
 expect(host.textContent).toContain('101 tasks');expect(host.textContent).toContain('Loaded 1 of 101');expect(reads.some(read=>/^\/tasks\?/.test(read.path))).toBe(false);expect(reads[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(reads[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 await click('[data-task-collection-more]');expect(host.querySelectorAll('[data-task-collection-row]')).toHaveLength(2);expect(reads.at(-1)?.params.get('cursor')).toBe('opaque+/=');for(const key of ['q','owner_id','priority','requirement_id'])expect(reads.at(-1)?.params.get(key)).toBe(reads[0].params.get(key));await click('.row-title');expect(props.onSelect).toHaveBeenCalledWith('task-a');await click('input[type=checkbox]');expect(props.onToggleSelection).toHaveBeenCalledWith('task-a',true);
});

it('applies literal filters explicitly, keeps outside-window IDs visible and requests more picker records on demand',async()=>{
 transport();await render({filters:{...props.filters,task_owner:'outside-owner',task_requirement:'outside-requirement'},moreMembers:true,moreRequirements:true});expect(host.querySelector('[name=task-collection-owner]')?.textContent).toContain('outside-owner');expect(host.querySelector('[name=task-collection-requirement]')?.textContent).toContain('outside-requirement');const before=reads.length;await change('task-collection-q',' 50%_中文 + ');await change('task-collection-owner','unset');await change('task-collection-priority','value:low');expect(reads).toHaveLength(before);await submit();expect(props.onFilter).toHaveBeenLastCalledWith(expect.objectContaining({q:' 50%_中文 + ',task_owner:'',task_unassigned:true,task_priority:'low',task_requirement:'outside-requirement'}));await click('label:has([name=task-collection-owner]) button');await click('label:has([name=task-collection-requirement]) button');expect(props.onMoreMembers).toHaveBeenCalledTimes(1);expect(props.onMoreRequirements).toHaveBeenCalledTimes(1);
});

it('renders independent draft/custom/literal-all board columns and skips zero-count columns',async()=>{
 transport((path,params)=>Response.json(path.includes('/summary')?summary({total:3,groups:[{key:'draft',total:1},{key:'waiting_for_customer',total:1},{key:'all',total:1},{key:'todo',total:0}]}):page({items:[task({id:'task-'+params.get('status'),status:params.get('status')})],total:1})));await render({filters:{...props.filters,layout:'board'}});expect(host.querySelectorAll('[data-task-column]')).toHaveLength(3);expect(host.querySelectorAll('.task-collection-card')).toHaveLength(3);expect(reads.filter(read=>read.path.startsWith('/task-workspace')).map(read=>read.params.get('status')).sort()).toEqual(['all','draft','waiting_for_customer']);expect(host.textContent).toContain('waiting_for_customer');expect(host.textContent).toContain('separate live reads');
});

it('distinguishes the literal all filter from all project task states',async()=>{
 transport((path,params)=>Response.json(path.includes('/summary')?summary({total:4,groups:[{key:'all',total:1},{key:'todo',total:3}]}):page({items:[task({status:params.get('status')||'todo'})],total:params.has('status')?1:4})));await render({filters:{...props.filters,task_status:'all'}});expect(reads.at(-1)?.params.get('status')).toBe('all');expect(host.textContent).toContain('Loaded 1 of 1');await change('task-collection-status','');await submit();expect(props.onFilter).toHaveBeenLastCalledWith(expect.objectContaining({task_status:''}));
});

it.each([401,403,404])('clears every private column after a current HTTP %i and ignores other late successes',async status=>{
 let finish!:(value:Response)=>void,deny!:(value:Response)=>void;transport((path,params)=>path.includes('/summary')?Response.json(summary({groups:[{key:'todo',total:1},{key:'draft',total:1},{key:'blocked',total:1}]})):params.get('status')==='draft'?new Promise<Response>(resolve=>{deny=resolve;}):params.get('status')==='blocked'?new Promise<Response>(resolve=>{finish=resolve;}):Response.json(page({items:[task({title:'Private loaded task'})],total:1})));await render({filters:{...props.filters,layout:'board'}});expect(host.textContent).toContain('Private loaded task');await act(async()=>deny(Response.json({error:{code:'denied',message:'Access unavailable'}},{status})));await settle();expect(host.querySelector('[data-task-column]')).toBeNull();expect(host.textContent).not.toContain('Private loaded task');expect(host.textContent).not.toContain('Human A');expect(host.textContent).toContain('Private tasks, counts and details have been cleared');expect(props.onDenied).toHaveBeenCalledTimes(1);await act(async()=>finish(Response.json(page({items:[task({status:'blocked',title:'Late private task'})],total:1}))));await settle();expect(host.textContent).not.toContain('Late private task');expect(reads.find(read=>read.params.get('status')==='blocked')?.signal?.aborted).toBe(true);
});

it('isolates a transient column failure and retries only that column from its first page',async()=>{
 let failed=true;transport((path,params)=>path.includes('/summary')?Response.json(summary({total:2,groups:[{key:'todo',total:1},{key:'draft',total:1}]})):params.get('status')==='draft'&&failed?Response.json({error:{code:'unavailable',message:'Temporary column failure'}},{status:503}):Response.json(page({items:[task({id:'task-'+params.get('status'),status:params.get('status'),title:params.get('status')==='todo'?'Other column remains':'Retried draft'})],total:1})));await render({filters:{...props.filters,layout:'board'}});expect(host.textContent).toContain('Other column remains');expect(host.querySelector('[data-task-column=draft] .error')).not.toBeNull();const before=reads.length;failed=false;await click('[data-task-column=draft] .error button');expect(reads).toHaveLength(before+1);expect(reads.at(-1)?.params.get('status')).toBe('draft');expect(reads.at(-1)?.params.has('cursor')).toBe(false);expect(host.textContent).toContain('Retried draft');expect(host.textContent).toContain('Other column remains');expect(props.onDenied).not.toHaveBeenCalled();
});

it('keeps a moved task once at the higher version while pending columns arrive',async()=>{
 let finish!:(value:Response)=>void;transport((path,params)=>path.includes('/summary')?Response.json(summary({total:2,groups:[{key:'todo',total:1},{key:'in_progress',total:1}]})):params.get('status')==='in_progress'?new Promise<Response>(resolve=>{finish=resolve;}):Response.json(page({items:[task({version:2,title:'Before move'})],total:1})));await render({filters:{...props.filters,layout:'board'}});expect(host.querySelectorAll('.task-collection-card')).toHaveLength(1);await act(async()=>finish(Response.json(page({items:[task({version:3,status:'in_progress',title:'After move'})],total:1}))));await settle();expect(host.querySelectorAll('.task-collection-card')).toHaveLength(1);expect(host.querySelector('[data-task-column=todo] .task-collection-card')).toBeNull();expect(host.textContent).toContain('After move');expect(host.textContent).not.toContain('Before move');expect(host.querySelectorAll('.task-collection-live-change')).toHaveLength(2);const rows=(props.onRows as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0];expect(rows).toHaveLength(1);expect(rows[0].version).toBe(3);
});

it('bounds automatic board requests to three and cancels queued old-scope reads',async()=>{
 const pending:((value:Response)=>void)[]=[];transport(path=>path.includes('/summary')?Response.json(summary({total:6,groups:Array.from({length:6},(_,i)=>({key:'custom-'+i,total:1}))})):new Promise<Response>(resolve=>pending.push(resolve)));await render({filters:{...props.filters,layout:'board'}});expect(reads.filter(read=>read.path.startsWith('/task-workspace'))).toHaveLength(3);transport(()=>Response.json(summary({project_id:'project-b',total:0,groups:[]})));await render({project:'project-b',organizationID:'org-b'});expect(host.querySelector('[data-task-column]')).toBeNull();await act(async()=>pending.forEach(resolve=>resolve(Response.json(page()))));await settle();expect(reads.filter(read=>read.params.get('project_id')==='project-a'&&read.path.startsWith('/task-workspace'))).toHaveLength(3);expect(host.textContent).not.toContain('Current scoped task');
});

it('clears an old filter scope before a replacement read and ignores its late denial',async()=>{
 let old!:(value:Response)=>void;transport((path,params)=>params.get('q')==='old'?new Promise<Response>(resolve=>{old=resolve;}):Response.json(path.includes('/summary')?summary():page({items:[task({title:'New scope task'})]})));await render({filters:{...props.filters,q:'old'}});const original=reads[0];await render({filters:{...props.filters,q:'new'}});expect(original.signal?.aborted).toBe(true);await act(async()=>old(Response.json({error:{code:'denied',message:'Old refusal'}},{status:403})));await settle();expect(host.textContent).toContain('New scope task');expect(props.onDenied).not.toHaveBeenCalled();
});

it('shows bilingual zero-scope guidance without fetching page rows and retries summary failures explicitly',async()=>{
 let failed=true;transport(()=>failed?Response.json({error:{code:'temporarily_unavailable',message:'Summary unavailable'}},{status:503}):Response.json(summary({total:0,groups:[]})));await render();expect(host.querySelector('table')).toBeNull();failed=false;await click('.error button');expect(reads.some(read=>read.path.startsWith('/task-workspace'))).toBe(false);expect(host.textContent).toContain('No tasks match these filters');await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('没有符合筛选条件的任务');expect(host.textContent).toContain('归档任务');
});

it('drops parent-blocked private pages and starts fresh after explicit authority retry',async()=>{
 transport();await render();expect(host.textContent).toContain('Current scoped task');await render({blocked:new Error('Parent detail access refused'),onRetryAuthority:vi.fn()});expect(host.querySelector('table')).toBeNull();expect(host.textContent).not.toContain('Human A');await click('.error button');expect(props.onRetryAuthority).toHaveBeenCalledTimes(1);await render({blocked:undefined,refresh:1});expect(reads.at(-1)?.params.has('cursor')).toBe(false);expect(host.textContent).toContain('Current scoped task');
});

it('loads more group headers explicitly and requests only the newly revealed bounded column',async()=>{
 transport((path,params)=>Response.json(path.includes('/summary')?summary({total:3,groups_total:2,groups:params.has('group_cursor')?[{key:'draft',total:1}]:[{key:'todo',total:2}],next_group_cursor:params.has('group_cursor')?'':'next-groups',as_of:params.has('group_cursor')?'2026-10-29T04:00:00Z':'2026-10-29T03:00:00Z'}):page({items:[task({id:'task-'+params.get('status'),status:params.get('status')})],total:params.get('status')==='todo'?2:1})));await render({filters:{...props.filters,layout:'board'}});expect(host.querySelectorAll('[data-task-column]')).toHaveLength(1);expect(reads.filter(read=>read.path.startsWith('/task-workspace'))).toHaveLength(1);await click('[data-task-groups-more]');expect(host.querySelectorAll('[data-task-column]')).toHaveLength(2);expect(reads.filter(read=>read.path.startsWith('/task-workspace'))).toHaveLength(2);expect(reads.find(read=>read.params.has('group_cursor'))?.params.get('group_cursor')).toBe('next-groups');expect(reads[0].params.get('group_limit')).toBe('20');expect(host.querySelector('[data-task-column=todo] .task-collection-group-asof')?.textContent).not.toBe(host.querySelector('[data-task-column=draft] .task-collection-group-asof')?.textContent);
});

it.each(['status','owner','priority'])('uses exact unset filters for an empty %s group instead of broad reads',async group=>{
 transport((path,params)=>Response.json(path.includes('/summary')?summary({group_by:group,total:1,groups:[{key:'',total:1}]}):page({items:[task(group==='status'?{status:''}:group==='owner'?{owner_id:''}:{priority:''})],total:1})));await render({filters:{...props.filters,layout:'board',task_group:group}});expect(host.querySelectorAll('.task-collection-card')).toHaveLength(1);expect(reads.at(-1)?.params.get(group==='status'?'status_unset':group==='owner'?'unassigned':'priority_unset')).toBe('true');expect(reads.at(-1)?.params.has(group==='owner'?'owner_id':group)).toBe(false);
});

it('applies only filter fields after a detail selection changes without remounting the form',async()=>{
 transport();const original={...props.filters,record:'old-task',panel:'old-panel'};await render({filters:original,selected:'old-task'});const updated={...original,record:'new-task',panel:'new-panel'};await render({filters:updated,selected:'new-task'});await change('task-collection-q','new query');await submit();const patch=(props.onFilter as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[0];expect(patch.q).toBe('new query');expect(patch).not.toHaveProperty('record');expect(patch).not.toHaveProperty('panel');expect(patch).not.toHaveProperty('run');
});

it('requires an explicit first-page refresh when live groups collapse below the merged header count',async()=>{
 let collapsed=false;transport((path,params)=>{
  if(path.includes('/summary')){if(params.has('group_cursor')){collapsed=true;return Response.json(summary({total:2,groups_total:2,groups:[{key:'new',total:1}]}));}return Response.json(summary(collapsed?{total:2,groups_total:2,groups:[{key:'new',total:1},{key:'second',total:1}]}:{total:6,groups_total:6,groups:['a','b','c','d','e'].map(key=>({key,total:1})),next_group_cursor:'next-groups'}));}
  return Response.json(page({items:[task({id:'task-'+params.get('status'),status:params.get('status')})],total:1}));
 });await render({filters:{...props.filters,layout:'board'}});expect(host.querySelectorAll('[data-task-column]')).toHaveLength(5);await click('[data-task-groups-more]');expect(host.querySelectorAll('[data-task-column]')).toHaveLength(5);expect(host.querySelector<HTMLButtonElement>('[data-task-groups-more]')?.disabled).toBe(true);expect(host.querySelector('.task-collection-groups-footer .error')?.textContent).toContain('Refresh all');await click('.task-collection-groups-footer .error button');expect(reads.filter(read=>read.path.includes('/summary')).at(-1)?.params.has('group_cursor')).toBe(false);expect(host.querySelectorAll('[data-task-column]')).toHaveLength(2);
});


it.each(['list','board'])('uses authoritative iteration lanes with exact counts in %s and preserves completed/missing names',async layout=>{
 const iterations=[{id:'done',version:2,title:'Accepted scope',status:'completed',description:'',start_date:'',end_date:'',goal:''}];
 const rows=[task({id:'done-task',title:'Previously completed scope task',data:{iteration_id:'forged'}}),task({id:'unknown-task',title:'Restricted catalog name task'}),task({id:'unset-task',title:'Unlinked planning task'})];
 const plans=[{task_id:'done-task',iteration_id:'done',plan_version:2},{task_id:'unknown-task',iteration_id:'missing',plan_version:7},{task_id:'unset-task',iteration_id:'',plan_version:0}];
 transport((path,params)=>{
  if(path.includes('/summary'))return Response.json(summary({group_by:'iteration',total:3,groups:[{key:'done',total:1},{key:'missing',total:1},{key:'',total:1}]}));
  const selected=layout==='list'?plans:plans.filter(plan=>params.get('iteration_unset')==='true'?!plan.iteration_id:plan.iteration_id===params.get('iteration_id'));
  return Response.json(page({items:rows.filter(row=>selected.some(plan=>plan.task_id===row.id)),task_plans:selected,total:layout==='list'?3:1}));
 });
 await render({filters:{...props.filters,layout,task_group:'iteration'},iterations});
 expect(host.querySelectorAll('[data-task-id]')).toHaveLength(3);expect(host.textContent).toContain('Accepted scope · Completed');expect(host.textContent).toContain('missing · Needs review');expect(host.textContent).toContain('No linked iteration');expect(host.textContent).not.toContain('forged');
 expect(reads.filter(read=>read.path.startsWith('/task-workspace'))).toHaveLength(layout==='board'?3:1);expect(reads.some(read=>read.path.startsWith('/iterations'))).toBe(false);
 if(layout==='board'){const unlinked=reads.find(read=>read.params.get('iteration_unset')==='true');expect(unlinked?.params.has('iteration_id')).toBe(false);expect(host.querySelector('[data-task-column=missing] .task-collection-card')?.textContent).toContain('Restricted catalog name task');}
 const metadata=(props.onRows as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[2];expect(metadata.task_plans).toHaveLength(3);
});

it('updates only the dominating iteration lane after a plan-only move and retains the formal plan version',async()=>{
 let finish!:(value:Response)=>void;
 transport((path,params)=>path.includes('/summary')?Response.json(summary({group_by:'iteration',total:2,groups:[{key:'old',total:1},{key:'new',total:1}]})):params.get('iteration_id')==='new'?new Promise<Response>(resolve=>{finish=resolve;}):Response.json(page({items:[task({title:'Before plan move'})],task_plans:[{task_id:'task-a',iteration_id:'old',plan_version:3}],total:1})));
 await render({filters:{...props.filters,layout:'board',task_group:'iteration'}});expect(host.querySelectorAll('.task-collection-card')).toHaveLength(1);
 await act(async()=>finish(Response.json(page({items:[task({title:'After plan move'})],task_plans:[{task_id:'task-a',iteration_id:'new',plan_version:4}],total:1}))));await settle();
 expect(host.querySelectorAll('.task-collection-card')).toHaveLength(1);expect(host.querySelector('[data-task-column=old] .task-collection-card')).toBeNull();expect(host.querySelector('[data-task-column=new] .task-collection-card')?.textContent).toContain('After plan move');
 expect(host.querySelectorAll('.task-collection-live-change')).toHaveLength(2);const metadata=(props.onRows as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[2];expect(metadata.task_plans).toEqual([{task_id:'task-a',iteration_id:'new',plan_version:4}]);
});

it('clears a page without planning proof instead of treating record data as an unlinked iteration',async()=>{
 transport((path)=>Response.json(path.includes('/summary')?summary({group_by:'iteration',total:1,groups:[{key:'',total:1}]}):page({items:[task({data:{iteration_id:''}})],task_plans:undefined,total:1})));
 await render({filters:{...props.filters,layout:'board',task_group:'iteration'}});expect(host.querySelector('.task-collection-card')).toBeNull();expect(host.querySelector('.task-collection-column .error')?.textContent).toContain('could not be confirmed');
 expect((props.onRows as ReturnType<typeof vi.fn>).mock.calls.at(-1)?.[2].task_plans).toEqual([]);
});

it('retains an unknown exact iteration filter and distinguishes the literal unset ID from no linked iteration',async()=>{
 transport(path=>Response.json(path.includes('/summary')?summary({total:0,groups:[]}):page()));
 await render({filters:{...props.filters,task_iteration:'missing'},iterations:[{id:'unset',version:1,title:'Literal unset iteration',description:'',status:'active',start_date:'',end_date:'',goal:''}]});const picker=host.querySelector<HTMLSelectElement>('[name=task-collection-iteration]')!;expect(picker.value).toBe('value:missing');expect(picker.selectedOptions[0].textContent).toBe('missing · Needs review');expect(reads[0].params.get('iteration_id')).toBe('missing');
 await submit();expect(props.onFilter).toHaveBeenLastCalledWith(expect.objectContaining({task_iteration:'missing'}));
 await change('task-collection-iteration','value:unset');await submit();expect(props.onFilter).toHaveBeenLastCalledWith(expect.objectContaining({task_iteration:'unset',task_iteration_unset:false}));
 await change('task-collection-iteration','unset');await submit();expect(props.onFilter).toHaveBeenLastCalledWith(expect.objectContaining({task_iteration:'',task_iteration_unset:true}));
});

it('retires a late iteration page after changing to another exact filter',async()=>{
 let old!:(value:Response)=>void;
 transport((path,params)=>path.includes('/summary')?Response.json(summary({group_by:'iteration',total:1,groups:[{key:params.get('iteration_id')||'',total:1}]})):params.get('iteration_id')==='old'?new Promise<Response>(resolve=>{old=resolve;}):Response.json(page({items:[task({title:'Current iteration task'})],task_plans:[{task_id:'task-a',iteration_id:'new',plan_version:2}],total:1})));
 await render({filters:{...props.filters,task_iteration:'old',task_group:'iteration'}});const oldRead=reads.at(-1)!;
 await render({filters:{...props.filters,task_iteration:'new'}});expect(oldRead.signal?.aborted).toBe(true);await act(async()=>old(Response.json(page({items:[task({title:'Late old private task'})],task_plans:[{task_id:'task-a',iteration_id:'old',plan_version:1}],total:1}))));await settle();
 expect(host.textContent).toContain('Current iteration task');expect(host.textContent).not.toContain('Late old private task');expect(reads.at(-1)?.params.get('iteration_id')).toBe('new');
});

it('renders the configured data order and fixed rails without rereading, dropping selection or losing the current cursor',async()=>{
 transport(path=>Response.json(path.includes('/summary')?summary():page({next_cursor:'original-cursor'})));await render({selection:['task-a']});const count=reads.length;await render({columns:['title','updated','status']});expect([...host.querySelectorAll('thead [data-task-column]')].map(cell=>cell.getAttribute('data-task-column'))).toEqual(['title','updated','status']);expect(host.querySelector<HTMLInputElement>('[data-task-collection-row] input[type=checkbox]')?.checked).toBe(true);expect(reads).toHaveLength(count);await render({columns:['title']});expect(host.querySelectorAll('thead [data-task-column]')).toHaveLength(1);expect(host.querySelector('[data-task-collection-row] input[type=checkbox]')).not.toBeNull();expect(host.querySelector('[data-task-collection-row] .icon-button')).not.toBeNull();expect(reads).toHaveLength(count);await click('[data-task-collection-more]');expect(reads.at(-1)?.params.get('cursor')).toBe('original-cursor');
});
