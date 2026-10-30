// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale,tr} from './i18n';
let host:HTMLDivElement,root:Root,queries:URLSearchParams[],lookupFailure:{path:string;status:number;more?:boolean}|undefined;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();queries=[];lookupFailure=undefined;
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks');
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(path==='/auth/session')return Response.json({user:{id:'user-a',name:'User',role:'owner'},organization:{id:'org-a',name:'Org'},projects:[{id:'project-a',version:1,title:'Project A'},{id:'project-b',version:1,title:'Project B'}]});
  if(path==='/account/preferences')return Response.json({user_id:'user-a',version:0,locale:''});
  if(path==='/organization/preferences')return Response.json({organization_id:'org-a',version:0,locale:''});
  if(path==='/overview')return Response.json({counts:{tasks:205},activity:[]});
  if(lookupFailure&&path===lookupFailure.path&&(!lookupFailure.more||url.searchParams.has('cursor')))return Response.json({error:{code:'forbidden',message:'Current project access revoked'}},{status:lookupFailure.status});
  if(path==='/iterations')return Response.json({items:[],total:0});
  if(path==='/members')return Response.json({items:[{id:'member-a',project_id:'project-a',name:'Assignee A',version:1},{id:'member-last',project_id:'project-a',name:'Last assignee',version:1}],total:3,next_cursor:'members-next'});
  if(path==='/task-collection/summary'){
   const filtered=url.searchParams.get('owner_id')==='member-last',empty=url.searchParams.get('owner_id')==='missing-owner',total=empty?0:filtered?1:205,group_by=url.searchParams.get('group_by')||'status';
   return Response.json({project_id:url.searchParams.get('project_id'),as_of:'2026-10-29T00:00:00Z',total,group_by,groups:total?[{key:group_by==='owner'?(filtered?'member-last':'member-a'):group_by==='priority'?'medium':'todo',total}]:[],groups_total:total?1:0,groups_limit:20,next_group_cursor:''});
  }
  if(path==='/task-workspace'){
   queries.push(new URLSearchParams(url.search));const filtered=url.searchParams.get('owner_id')==='member-last',empty=url.searchParams.get('owner_id')==='missing-owner',cursor=url.searchParams.get('cursor'),limit=Number(url.searchParams.get('limit')||100),start=cursor==='last-page'?200:cursor?100:0;
   const items=Array.from({length:empty?0:filtered?1:cursor==='last-page'?5:limit},(_,index)=>({id:filtered?'task-205':'task-'+(start+index+1),project_id:url.searchParams.get('project_id'),version:1,title:filtered?'Task beyond first page':'Task '+(start+index),status:'todo',owner_id:filtered?'member-last':'member-a',priority:'medium',created_at:'2026-10-29T00:00:00Z',updated_at:'2026-10-29T00:00:00Z'}));
   return Response.json({project_id:url.searchParams.get('project_id'),limit,items,task_plans:items.map(row=>({task_id:row.id,iteration_id:'',plan_version:0})),total:empty?0:filtered?1:205,next_cursor:empty||filtered||cursor==='last-page'?'':cursor?'last-page':'next-page',as_of:'2026-10-29T00:00:00Z'});
  }
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
it('wires full-project owner queries into Records and keeps the same membership through layout changes',async()=>{
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(20);expect(host.textContent).toContain('Loaded 20 of 205');
 const owner=[...host.querySelectorAll<HTMLSelectElement>('.task-views-filters select')].find(select=>select.parentElement?.textContent?.startsWith('Assignee'))!;
 await act(async()=>{owner.value='value:member-last';owner.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 expect(queries.at(-1)?.get('owner_id')).toBe('member-last');expect(host.querySelectorAll('[data-task-id]')).toHaveLength(1);expect(host.textContent).toContain('Task beyond first page');expect(location.search).toContain('task_owner=member-last');
 const before=queries.length,board=[...host.querySelectorAll('button')].find(button=>button.textContent?.trim()==='Board view')!;
 await act(async()=>board.click());await settle();expect(queries.length).toBeGreaterThan(before);expect(host.querySelector('[data-task-id="task-205"]')).toBeTruthy();
 const project=host.querySelector<HTMLOptionElement>('option[value="project-b"]')!.parentElement as HTMLSelectElement;
 await act(async()=>{project.value='project-b';project.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 expect(queries.at(-1)?.get('project_id')).toBe('project-b');expect(queries.at(-1)?.get('owner_id')).toBeNull();expect(new URLSearchParams(location.search).get('task_owner')).toBeNull();
});
it('does not replace an invalid long query with a shorter or unfiltered project query',async()=>{
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&q='+encodeURIComponent('😀'.repeat(201)));
 await act(async()=>root.render(<App/>));await settle();expect(queries).toHaveLength(0);expect(host.textContent).toContain('No broader query was made');
});
it('clears every task-only filter when switching projects through the sidebar',async()=>{
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&task_owner=member-last&task_status=accepted&task_priority=high&task_sort=priority_desc&task_group=owner&task_invalid=1');
 await act(async()=>root.render(<App/>));await settle();expect(queries).toHaveLength(0);
 const target=[...host.querySelectorAll<HTMLButtonElement>('.project-list button')].find(button=>button.textContent?.includes('Project B'))!;
 await act(async()=>target.click());await settle();
 const route=new URLSearchParams(location.search);
 for(const field of ['task_owner','task_status','task_priority','task_sort','task_group','task_invalid'])expect(route.has(field),field).toBe(false);
 expect(queries.at(-1)?.get('project_id')).toBe('project-b');expect(queries.at(-1)?.get('owner_id')).toBeNull();expect(queries.at(-1)?.get('status')).toBeNull();expect(queries.at(-1)?.get('priority')).toBeNull();expect(queries.at(-1)?.get('sort')).toBe('updated_desc');
});
it.each([['/members',401],['/requirements',403]] as const)('clears loaded tasks and an open form when %s refresh returns %i',async(path,status)=>{
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(20);
 await act(async()=>host.querySelector<HTMLButtonElement>('.records-panel .list-toolbar button.primary')!.click());
 const title=document.querySelector<HTMLInputElement>('input[name="title"]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(title,'Private form draft');title.dispatchEvent(new Event('input',{bubbles:true}));});
 expect(document.querySelector('[role=dialog]')).toBeTruthy();lookupFailure={path,status};
 // A data refresh can finish while a modal is open; it must not retain that form after revocation.
 await act(async()=>host.querySelector<HTMLButtonElement>('button[aria-label="'+tr('shell.refresh')+'"]')!.click());await settle();
 expect(host.querySelectorAll('[data-task-id]')).toHaveLength(0);expect(document.querySelector('[role=dialog]')).toBeNull();expect(document.querySelector('input[name="title"]')).toBeNull();expect(host.textContent).toContain('Current project access revoked');
});
it('closes an open form and clears task data when loading more assignees loses authorization',async()=>{
 await act(async()=>root.render(<App/>));await settle();
 await act(async()=>host.querySelector<HTMLButtonElement>('.records-panel .list-toolbar button.primary')!.click());
 lookupFailure={path:'/members',status:403,more:true};
 const modal=document.querySelector('[role=dialog]')!,more=[...modal.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.includes(tr('records.load_more_members')))!;
 expect(more,modal.textContent||'').toBeTruthy();await act(async()=>more.click());await settle();
 expect(document.querySelector('[role=dialog]')).toBeNull();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(0);expect(host.textContent).toContain('Current project access revoked');
});
it.each(['member-last','missing-owner'])('reads prerequisite candidates independently of the %s workspace filter',async owner=>{
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&task_owner='+owner);
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(owner==='missing-owner'?0:1);
 expect(queries).toHaveLength(owner==='missing-owner'?0:1);
 await act(async()=>host.querySelector<HTMLButtonElement>('.records-panel .list-toolbar button.primary')!.click());await settle();
 const modal=document.querySelector('[role=dialog]')!;
 expect(queries.at(-1)?.get('owner_id')).toBeNull();expect(queries.at(-1)?.get('status')).toBeNull();expect(queries.at(-1)?.get('priority')).toBeNull();
 expect(modal.textContent).toContain('independently of workspace filters');expect(modal.textContent).toContain('Loaded 100 of 205 prerequisite candidates');
 const candidate=modal.querySelector<HTMLInputElement>('.dependency-options input')!;expect(candidate.closest('label')?.textContent).toContain('Task 0');
 await act(async()=>candidate.click());expect(modal.querySelector<HTMLInputElement>('input[name="depends_on"]')?.value).toBe('task-1');
 const more=[...modal.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.trim()==='Load more prerequisite tasks')!;
 await act(async()=>more.click());await settle();expect(queries.at(-1)?.get('cursor')).toBe('next-page');expect(queries.at(-1)?.get('owner_id')).toBeNull();
 expect(modal.textContent).toContain('Loaded 200 of 205 prerequisite candidates');expect(modal.querySelector<HTMLInputElement>('input[name="depends_on"]')?.value).toBe('task-1');
});


function iterationRow(id:string,project='project-a',title='Current iteration') {return {id,project_id:project,version:1,title,description:'',status:'active',start_date:'',end_date:'',goal:''};}
async function changeProject(target:'topbar'|'sidebar',project='project-b'){
 if(target==='sidebar'){const button=[...host.querySelectorAll<HTMLButtonElement>('.project-list button')].find(button=>button.textContent?.includes(project==='project-b'?'Project B':'Project A'))!;expect(button).toBeTruthy();await act(async()=>button.click());}
 else{const picker=host.querySelector<HTMLSelectElement>('.breadcrumbs select')!;await act(async()=>{picker.value=project;picker.dispatchEvent(new Event('change',{bubbles:true}));});}
 await settle();
}
it.each([['topbar','exact'],['sidebar','exact'],['topbar','unlinked'],['sidebar','unlinked']] as const)('resets iteration criteria when the %s project switch leaves an %s view',async(target,kind)=>{
 const original=fetch;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(path==='/iterations')return Response.json({items:[iterationRow('iteration-a',url.searchParams.get('project_id')||'')],total:1});
  const response=await original(input,init);if(path!=='/task-workspace'||!url.searchParams.has('iteration_id'))return response;
  const value=await response.json();return Response.json({...value,task_plans:value.items.map((row:{id:string})=>({task_id:row.id,iteration_id:url.searchParams.get('iteration_id'),plan_version:1}))});
 }));
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&'+(kind==='exact'?'task_iteration=iteration-a':'task_iteration_unset=true'));
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(20);
 await changeProject(target);const route=new URLSearchParams(location.search);expect(route.get('project')).toBe('project-b');expect(route.has('task_iteration')).toBe(false);expect(route.has('task_iteration_unset')).toBe(false);expect(queries.at(-1)?.get('project_id')).toBe('project-b');expect(queries.at(-1)?.has('iteration_id')).toBe(false);expect(queries.at(-1)?.has('iteration_unset')).toBe(false);expect(host.querySelectorAll('[data-task-id]')).toHaveLength(20);
});

it.each(['tab','main navigation'] as const)('clears iteration criteria when leaving tasks through %s',async target=>{
 history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&task_iteration_unset=true');await act(async()=>root.render(<App/>));await settle();
 const button=target==='tab'?[...host.querySelectorAll<HTMLButtonElement>('.page-tabs [role=tab]')].find(button=>button.textContent===tr('shell.tabs.runs')):host.querySelector<HTMLButtonElement>('.sidebar nav button[aria-label="'+tr('nav.overview')+'"]');expect(button).toBeTruthy();await act(async()=>button!.click());await settle();expect(new URLSearchParams(location.search).has('task_iteration_unset')).toBe(false);expect(new URLSearchParams(location.search).has('task_iteration')).toBe(false);
});

it('clears private tasks and the editor immediately on iteration denial while another catalog is pending',async()=>{
 const original=fetch;let stalled=false,waiting=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(stalled&&path==='/requirements'){waiting=true;return new Promise<Response>((_resolve,reject)=>{init?.signal?.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError')),{once:true});});}
  return original(input,init);
 }));
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(20);await act(async()=>host.querySelector<HTMLButtonElement>('.records-panel .list-toolbar button.primary')!.click());const title=document.querySelector<HTMLInputElement>('input[name=title]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(title,'Private pending editor title');title.dispatchEvent(new Event('input',{bubbles:true}));});
 stalled=true;lookupFailure={path:'/iterations',status:403};await act(async()=>host.querySelector<HTMLButtonElement>('button[aria-label="'+tr('shell.refresh')+'"]')!.click());await settle();expect(waiting).toBe(true);expect(host.querySelectorAll('[data-task-id]')).toHaveLength(0);expect(document.querySelector('[role=dialog]')).toBeNull();expect(document.querySelector('input[name=title]')).toBeNull();expect(host.textContent).not.toContain('Assignee A');expect(host.textContent).toContain('Current project access revoked');
});

it('ignores a late old-project iteration catalog after a project scope change',async()=>{
 const original=fetch;let oldCatalog!:(response:Response)=>void;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(path==='/iterations'){if(url.searchParams.get('project_id')==='project-a')return new Promise<Response>(resolve=>{oldCatalog=resolve;});return Response.json({items:[iterationRow('iteration-b','project-b','Fresh B iteration')],total:1});}
  return original(input,init);
 }));
 await act(async()=>root.render(<App/>));await settle();expect(typeof oldCatalog).toBe('function');await changeProject('topbar');const picker=host.querySelector<HTMLSelectElement>('[aria-labelledby=task-filter-iteration]')!;expect(picker.textContent).toContain('Fresh B iteration');
 await act(async()=>oldCatalog(Response.json({items:[iterationRow('old-private','project-a','Late private A iteration')],total:1})));await settle();expect(picker.textContent).toContain('Fresh B iteration');expect(host.textContent).not.toContain('Late private A iteration');expect(host.textContent).not.toContain('Current project access revoked');
});
