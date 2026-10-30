// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {App} from './App';
import {setLocale} from './i18n';
import type {Session} from './api';

// Exercise the real App session-change callback while keeping account switching
// itself outside this task collection regression's scope.
vi.mock('./Governance',async importOriginal=>{
 const original=await importOriginal<typeof import('./Governance')>();
 return {...original,OrganizationSwitch:({session,onSwitched}:{session:Session;onSwitched:(session:Session)=>void})=><button type="button" data-test-switch-view onClick={()=>onSwitched({...session,user:{...session.user,id:'user-b',name:'User B'},organization:{id:'org-b',name:'Org B'}})}>Switch account view</button>};
});

const record=(id:string,title:string,status='draft',version=1)=>({id,project_id:'project-a',version,title,description:'Current project record',status,owner_id:'human-a',priority:'high',requirement_id:'',depends_on:[],acceptance_criteria:[],data:{},created_at:'2026-10-29T01:00:00Z',updated_at:'2026-10-29T02:00:00Z'});
let host:HTMLDivElement,root:Root,reads:{path:string;headers:Headers}[],version:number,denied:boolean;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);localStorage.clear();sessionStorage.clear();await setLocale('en-US',{persist:false});history.replaceState(null,'','/?project=project-a&view=development&sub=tasks&record=outside-task&q=literal&task_status=draft');host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];version=1;denied=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');reads.push({path,headers:new Headers(init?.headers)});
  if(path==='/auth/session')return Response.json({user:{id:'user-a',name:'User A',email:'a@example.test',role:'owner'},organization:{id:'org-a',name:'Org A'},projects:[record('project-a','Project A')]});
  if(path==='/account/preferences')return Response.json({user_id:'user-a',locale:'en-US',version:1,updated_at:'2026-10-29T00:00:00Z'});
  if(path==='/overview')return Response.json({counts:{tasks:2,requirements:0},activity:[],capabilities:[]});
  if(path.startsWith('/task-collection/')&&denied)return Response.json({error:{code:'forbidden',message:'Current project access refused'}},{status:403});
  if(path==='/task-collection/summary')return Response.json({project_id:'project-a',group_by:url.searchParams.get('group_by')||'status',groups_total:1,groups_limit:20,next_group_cursor:'',as_of:'2026-10-29T03:00:00Z',total:1,groups:[{key:url.searchParams.get('group_by')==='owner'?'human-a':url.searchParams.get('group_by')==='priority'?'high':'draft',total:1}]});
  if(path==='/task-workspace'){const limit=Number(url.searchParams.get('limit')||100);return Response.json({project_id:'project-a',limit,as_of:'2026-10-29T03:01:00Z',items:limit===100?[record('loaded-task','Loaded filtered task'),record('prerequisite-task','Independent prerequisite')]:[record('loaded-task','Loaded filtered task')],task_plans:(limit===100?['loaded-task','prerequisite-task']:['loaded-task']).map(task_id=>({task_id,iteration_id:'',plan_version:0})),total:limit===100?2:1,next_cursor:''});}
  if(path==='/tasks/outside-task')return Response.json(record('outside-task','Outside filter details '+version,'todo',version));
  if(path==='/iterations')return Response.json({items:[],total:0});
  if(path==='/members')return Response.json({items:[{...record('human-a','Private member name','active'),kind:'human',data:{user_id:'user-a'}}],total:1});
  return Response.json({project_id:'project-a',items:[],total:0,next_cursor:''});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(until:()=>boolean){for(let i=0;i<40&&!until();i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});expect(until()).toBe(true);}
async function render(){await act(async()=>root.render(<App/>));await settle(()=>!!host.querySelector('.detail-pane h2')&&!!host.querySelector('[data-task-collection-row]'));}

it('preserves a selected task outside the server filter, reloads its exact detail, and never fetches generic task rows',async()=>{
 await render();expect(host.querySelector('.detail-pane h2')?.textContent).toBe('Outside filter details 1');expect(host.querySelectorAll('[data-task-collection-row]')).toHaveLength(1);expect(host.querySelector('[data-task-collection-row]')?.textContent).toContain('Loaded filtered task');expect(reads.some(read=>read.path==='/tasks')).toBe(false);expect(new URLSearchParams(location.search).get('task_status')).toBe('draft');
 const direct=reads.find(read=>read.path==='/tasks/outside-task')!;expect(direct.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(direct.headers.get('X-Suma-Expected-Organization')).toBe('org-a');version=2;await act(async()=>host.querySelector<HTMLButtonElement>('.topbar-right button[aria-label="Refresh data"]')!.click());await settle(()=>host.querySelector('.detail-pane h2')?.textContent==='Outside filter details 2');expect(reads.filter(read=>read.path==='/tasks/outside-task').length).toBeGreaterThanOrEqual(2);
});

it('clears selected parent details and loaded picker names when a collection read loses authorization',async()=>{
 await render();expect(host.textContent).toContain('Private member name');denied=true;await act(async()=>host.querySelector<HTMLButtonElement>('[data-task-collection-refresh]')!.click());await settle(()=>host.textContent?.includes('Private tasks, counts and details have been cleared')??false);expect(host.querySelector('.detail-pane')).toBeNull();expect(host.querySelector('[data-task-collection-row]')).toBeNull();expect(host.textContent).not.toContain('Outside filter details');expect(host.textContent).not.toContain('Private member name');expect(new URLSearchParams(location.search).get('record')).toBeNull();denied=false;await act(async()=>host.querySelector<HTMLButtonElement>('[data-task-collection-refresh]')!.click());await settle(()=>!!host.querySelector('[data-task-collection-row]'));expect(host.textContent).not.toContain('Private tasks, counts and details have been cleared');
});

it('composes one saved-view control set with canonical live rows, sort and grouping',async()=>{
 await render();expect(host.querySelector('.task-perspective')).toBeNull();expect(host.querySelectorAll('.task-views-filters')).toHaveLength(1);expect(host.querySelector('.task-collection-filters')).toBeNull();
 const sort=host.querySelector<HTMLSelectElement>('[aria-labelledby=task-filter-sort]')!;await act(async()=>{sort.value='priority_desc';sort.dispatchEvent(new Event('change',{bubbles:true}));});await settle(()=>new URLSearchParams(location.search).get('task_sort')==='priority_desc'&&!!host.querySelector('[data-task-collection-row]'));
 const group=host.querySelector<HTMLSelectElement>('[aria-labelledby=task-filter-group_by]')!;await act(async()=>{group.value='owner';group.dispatchEvent(new Event('change',{bubbles:true}));});await settle(()=>new URLSearchParams(location.search).get('task_group')==='owner'&&!!host.querySelector('[data-task-collection-row]'));expect(host.querySelector('.task-collection-list-group summary')?.textContent).toContain('Private member name');
 const save=Array.from(host.querySelectorAll<HTMLButtonElement>('button')).find(button=>button.textContent==='Save view')!;await act(async()=>save.click());const input=document.querySelector<HTMLInputElement>('[role=dialog] input')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'Canonical local view');input.dispatchEvent(new Event('input',{bubbles:true}));});const submit=Array.from(document.querySelectorAll<HTMLButtonElement>('[role=dialog] button')).find(button=>button.textContent==='Save')!;await act(async()=>submit.click());expect(localStorage.getItem('suma-task-views-v1:["user-a","org-a","project-a"]')).toContain('Canonical local view');expect(host.querySelector('.detail-pane h2')?.textContent).toBe('Outside filter details 1');
});

it('keeps status queue edit prerequisites independent of filtered collection rows',async()=>{
 await render();await act(async()=>host.querySelector<HTMLButtonElement>('.detail-bottom button')!.click());
 await settle(()=>document.querySelector('[role=dialog]')?.textContent?.includes('Independent prerequisite')??false);
 expect(Array.from(document.querySelectorAll('[role=dialog] label')).some(label=>label.textContent?.includes('Independent prerequisite')&&!!label.querySelector('input[type=checkbox]'))).toBe(true);
 expect(reads.filter(read=>read.path==='/task-workspace')).toHaveLength(2);
});

it('honors navigation refusal before changing layout or current filters',async()=>{
 await render();const before=location.search,prevent=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',prevent);
 try{const button=Array.from(host.querySelectorAll<HTMLButtonElement>('button')).find(button=>button.textContent?.trim()==='Board view')!;await act(async()=>button.click());expect(location.search).toBe(before);expect(host.querySelector('[data-task-collection-row]')).toBeTruthy();}finally{window.removeEventListener('suma:navigate',prevent);}
});

it('hides editor and picker facts on a same-project user and organization switch before fresh reads, and ignores a late old lookup',async()=>{
 const original=fetch;let oldLookup!:(response:Response)=>void;
 const pending:{path:string;resolve:(response:Response)=>void}[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,''),headers=new Headers(init?.headers),organization=headers.get('X-Suma-Expected-Organization');
  if(organization==='org-a'&&path==='/members'){
   if(url.searchParams.has('cursor'))return new Promise<Response>(resolve=>{oldLookup=resolve;});
   return Response.json({items:[{...record('human-a','Original private picker name','active'),kind:'human',data:{user_id:'user-a'}}],total:2,next_cursor:'old-more'});
  }
  if(organization==='org-b'&&(['/members','/requirements','/tasks/outside-task','/task-workspace'].includes(path)||path.startsWith('/task-collection/')))return new Promise<Response>(resolve=>pending.push({path,resolve}));
  return original(input,init);
 }));
 await render();await act(async()=>host.querySelector<HTMLButtonElement>('.detail-bottom button')!.click());await settle(()=>!!document.querySelector('[role=dialog] input[name=title]'));
 expect(document.querySelector<HTMLInputElement>('[role=dialog] input[name=title]')?.value).toContain('Outside filter details');
 const more=Array.from(document.querySelectorAll<HTMLButtonElement>('[role=dialog] button')).find(button=>button.textContent==='Load more members');
 expect(more).not.toBeUndefined();await act(async()=>more!.click());await act(async()=>host.querySelector<HTMLButtonElement>('[data-test-switch-view]')!.click());
 expect(document.querySelector('[role=dialog] input[name=title]')).toBeNull();expect(host.querySelector('.detail-pane')).toBeNull();expect(host.textContent).not.toContain('Original private picker name');expect(host.querySelector('[data-task-collection-row]')).toBeNull();
 await act(async()=>oldLookup(Response.json({items:[{...record('late-old-human','Late original account private name','active'),kind:'human'}],total:2,next_cursor:''})));
 expect(host.textContent).not.toContain('Late original account private name');
 for(let round=0;round<3;round++){
  const current=pending.splice(0);await act(async()=>{for(const read of current){
   const data=read.path==='/members'?{items:[{...record('human-b','Fresh account picker name','active'),kind:'human',data:{user_id:'user-b'}}],total:1}:read.path==='/requirements'?{items:[],total:0}:read.path==='/tasks/outside-task'?record('outside-task','Fresh account selected details','todo'):read.path.endsWith('/summary')?{project_id:'project-a',group_by:'status',groups_total:1,groups_limit:20,next_group_cursor:'',as_of:'2026-10-29T03:00:00Z',total:1,groups:[{key:'draft',total:1}]}:{project_id:'project-a',as_of:'2026-10-29T03:01:00Z',items:[record('new-task','Fresh account loaded task')],task_plans:[{task_id:'new-task',iteration_id:'',plan_version:0}],total:1,limit:20,next_cursor:''};read.resolve(Response.json(data));
  }});
 }
 await settle(()=>host.textContent?.includes('Fresh account picker name')??false);expect(host.textContent).not.toContain('Late original account private name');expect(host.textContent).not.toContain('Original private picker name');
});

it.each(['current identity','old identity'])('treats prerequisite HTTP404 under the %s with the correct parent scope',async identity=>{
 const original=fetch;let finish!:(response:Response)=>void;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,''),headers=new Headers(init?.headers),organization=headers.get('X-Suma-Expected-Organization');
  if(path==='/task-workspace'&&url.searchParams.get('limit')==='100')return new Promise<Response>(resolve=>{finish=resolve;});
  if(organization==='org-b'&&path==='/task-workspace')return Response.json({project_id:'project-a',as_of:'2026-10-29T04:00:00Z',limit:20,items:[record('fresh-task','Fresh identity task')],task_plans:[{task_id:'fresh-task',iteration_id:'',plan_version:0}],total:1,next_cursor:''});
  if(organization==='org-b'&&path==='/tasks/outside-task')return Response.json(record('outside-task','Fresh identity details','todo'));
  return original(input,init);
 }));
 await render();await act(async()=>host.querySelector<HTMLButtonElement>('.detail-bottom button')!.click());await settle(()=>typeof finish==='function');
 if(identity==='old identity'){await act(async()=>host.querySelector<HTMLButtonElement>('[data-test-switch-view]')!.click());await settle(()=>host.textContent?.includes('Fresh identity task')??false);}
 await act(async()=>finish(Response.json({error:{code:'not_found',message:'Prerequisite access unavailable'}},{status:404})));
 if(identity==='current identity'){await settle(()=>host.textContent?.includes('Private tasks, counts and details have been cleared')??false);expect(document.querySelector('[role=dialog]')).toBeNull();expect(host.querySelector('.detail-pane')).toBeNull();expect(host.querySelector('[data-task-collection-row]')).toBeNull();}
 else{expect(host.textContent).toContain('Fresh identity task');expect(host.textContent).not.toContain('Private tasks, counts and details have been cleared');expect(document.querySelector('[role=dialog]')).toBeNull();}
});

it('clears the editor and selected facts when a CAS refresh reads HTTP403 after a rejected save',async()=>{
 const original=fetch;let conflict=false,followup=false;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,'');
  if(path==='/tasks/outside-task'&&init?.method==='PATCH'){conflict=true;return Response.json({error:{code:'version_conflict',message:'Task changed'}},{status:409});}
  if(path==='/tasks/outside-task'&&conflict){followup=true;return Response.json({error:{code:'forbidden',message:'Current task access revoked during refresh'}},{status:403});}
  return original(input,init);
 }));await render();await act(async()=>host.querySelector<HTMLButtonElement>('.detail-bottom button')!.click());await settle(()=>!!document.querySelector('[role=dialog] form'));await act(async()=>document.querySelector<HTMLFormElement>('[role=dialog] form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle(()=>host.textContent?.includes('Private tasks, counts and details have been cleared')??false);expect(conflict).toBe(true);expect(followup).toBe(true);expect(document.querySelector('[role=dialog]')).toBeNull();expect(host.querySelector('.detail-pane')).toBeNull();expect(host.querySelector('[data-task-collection-row]')).toBeNull();
});
