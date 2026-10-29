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
  if(path==='/members')return Response.json({items:[{id:'member-a',project_id:'project-a',name:'Assignee A',version:1},{id:'member-last',project_id:'project-a',name:'Last assignee',version:1}],total:3,next_cursor:'members-next'});
  if(path==='/task-workspace'){
   queries.push(new URLSearchParams(url.search));const filtered=url.searchParams.get('owner_id')==='member-last',empty=url.searchParams.get('owner_id')==='missing-owner',cursor=url.searchParams.get('cursor'),start=cursor==='last-page'?200:cursor?100:0;
   const items=Array.from({length:empty?0:filtered?1:cursor==='last-page'?5:100},(_,index)=>({id:filtered?'task-205':'task-'+(start+index+1),project_id:url.searchParams.get('project_id'),version:1,title:filtered?'Task beyond first page':'Task '+(start+index),status:'todo',owner_id:filtered?'member-last':'member-a',priority:'medium',created_at:'2026-10-29T00:00:00Z'}));
   return Response.json({items,total:empty?0:filtered?1:205,next_cursor:empty||filtered||cursor==='last-page'?undefined:cursor?'last-page':'next-page',as_of:'2026-10-29T00:00:00Z'});
  }
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
it('wires full-project owner queries into Records and keeps the same membership through layout changes',async()=>{
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(100);expect(host.textContent).toContain('Loaded 100 of 205');
 const owner=[...host.querySelectorAll<HTMLSelectElement>('.task-views-filters select')].find(select=>select.parentElement?.textContent?.startsWith('Assignee'))!;
 await act(async()=>{owner.value='member-last';owner.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 expect(queries.at(-1)?.get('owner_id')).toBe('member-last');expect(host.querySelectorAll('[data-task-id]')).toHaveLength(1);expect(host.textContent).toContain('Task beyond first page');expect(location.search).toContain('task_owner=member-last');
 const before=queries.length,board=[...host.querySelectorAll('button')].find(button=>button.textContent?.trim()==='Board view')!;
 await act(async()=>board.click());await settle();expect(queries).toHaveLength(before);expect(host.querySelector('[data-task-id="task-205"]')).toBeTruthy();
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
 await act(async()=>root.render(<App/>));await settle();expect(host.querySelectorAll('[data-task-id]')).toHaveLength(100);
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
 expect(queries).toHaveLength(1);
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
