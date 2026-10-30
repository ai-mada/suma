// @vitest-environment jsdom
import {act,type ComponentProps} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {TaskHandoffPanel} from './TaskHandoffPanel';

const project='project-handoff',task='task-handoff',raw='Original <script>note</script> {{name}} 原始说明';
const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const recipient={resource_id:'resource-b',resource_version:3,user_id:'user-b',name:'Recipient B',teams:[{team_id:'team-a',name:'Team A',team_version:2,membership_version:4}]};
function handoff(){return {id:'handoff-a',project_id:project,task_id:task,version:1,state:'pending',created_by:'user-a',created_at:'2026-10-26T00:00:00Z',updated_at:'2026-10-26T00:00:00Z',to_resource_id:'resource-b',to_resource_version:3,to_user_id:'user-b',to_name:'Recipient B',to_team_id:'team-a',to_team_name:'Team A',to_team_version:2,to_membership_version:4,note:raw,basis:{task_version:7,task_basis_sha256:'a'.repeat(64),requirement_id:'req-a',requirement_version:2,from_owner_id:'agent-original',source_run_id:'run-original',source_run_version:4,source_run_state:'succeeded',delivery_sha256:'b'.repeat(64)},decision_by:'',decision_at:'',decision_reason:'',result_task_version:0,basis_current:true,blockers:[],allowed_actions:['accept','return']};}
function page(){return {items:[] as ReturnType<typeof handoff>[],total:0,next_cursor:'',recipients:[recipient],can_create:true,create_blockers:[] as string[],task_version:7};}
let root:Root|undefined,host:HTMLDivElement,reads:{path:string;signal?:AbortSignal|null}[],writes:{path:string;body:Record<string,unknown>;headers:Headers}[];
beforeEach(async()=>{await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();reads=[];writes=[];host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<4;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(s=session,p=project,id=task,onChanged:()=>void|Promise<void>=()=>{},extra:Partial<ComponentProps<typeof TaskHandoffPanel>>={}){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={s}><TaskHandoffPanel session={s} project={p} taskId={id} taskVersion={7} refresh={0} onChanged={onChanged} {...extra}/></ApiViewProvider>);});await settle();}
function transport(reply:(path:string,options?:RequestInit)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes.push({path,body:JSON.parse(String(options.body)),headers:new Headers(options.headers)});else reads.push({path,signal:options?.signal});return reply(path,options);}));}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
async function change(selector:string,value:string){const element=host.querySelector<HTMLInputElement|HTMLTextAreaElement|HTMLSelectElement>(selector)!;expect(element,selector).not.toBeNull();const prototype=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:element instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}
async function submit(selector:string){await act(async()=>host.querySelector(selector)!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();}

it('stays folded without reads, loads bounded history on demand and expands basis only when requested',async()=>{
 const first={...page(),items:[handoff()],total:2,next_cursor:'handoff-a',can_create:false};
 transport(async path=>Response.json(path.includes('&before=')?{...first,items:[{...handoff(),id:'handoff-old',state:'withdrawn',allowed_actions:[]}],next_cursor:''}:first));
 await render();expect(reads).toHaveLength(0);expect(host.querySelector('[aria-expanded=false]')).not.toBeNull();
 await click('[data-handoff-toggle]');expect(reads).toHaveLength(1);expect(reads[0].path).toContain('limit=20');expect(host.textContent).toContain(raw);expect(host.querySelector('script')).toBeNull();expect(host.textContent).not.toContain('a'.repeat(64));
 const basis=host.querySelector<HTMLDetailsElement>('[data-handoff-basis]')!;await act(async()=>{basis.open=true;basis.dispatchEvent(new Event('toggle'));});expect(host.textContent).toContain('a'.repeat(64));
 await click('[data-handoff-more]');expect(reads[1].path).toContain('before=handoff-a');expect(host.querySelectorAll('[data-handoff-item]')).toHaveLength(2);
});

it('keeps the exact create operation, target resource and original text after an unknown result and language change',async()=>{
 let unknown=true;const changed=vi.fn(async()=>{});
 transport(async(_path,options)=>{if(options?.body){if(unknown)throw TypeError('lost response');return Response.json(handoff());}return Response.json(page());});
 await render(session,project,task,changed);await click('[data-handoff-toggle]');await change('[name=recipient]','resource-b');await change('[name=team]','team-a');await change('[name=note]',raw);await submit('[data-handoff-create]');
 expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({project_id:project,task_id:task,expected_task_version:7,to_resource_id:'resource-b',to_team_id:'team-a',note:raw});
 expect(host.querySelector<HTMLTextAreaElement>('[name=note]')?.value).toBe(raw);expect(host.querySelector('[name=note]')?.matches(':disabled')).toBe(true);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});unknown=false;await click('[data-handoff-restore]');
 expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(writes[0].body);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(changed).toHaveBeenCalledTimes(1);
});

it('requires explicit invalidation acknowledgement and treats handoff acceptance separately from task approval',async()=>{
 const current={...page(),items:[handoff()],total:1,can_create:false};const changed=vi.fn();
 transport(async(path,options)=>options?.body?Response.json({...handoff(),state:'accepted',version:2,result_task_version:8,allowed_actions:[],decision_reason:JSON.parse(String(options.body)).reason}):Response.json(path.startsWith('/task-handoffs/handoff-a?')?handoff():current));
 await render({...session,user:{...session.user,id:'user-b'}},project,task,changed);await click('[data-handoff-toggle]');await click('[data-handoff-command=accept]');
 const accept=host.querySelector<HTMLButtonElement>('[data-handoff-decision] button.primary')!;expect(accept.disabled).toBe(true);expect(host.textContent).toContain('downstream');
 await click('[name=acknowledge_invalidation]');expect(accept.disabled).toBe(false);await submit('[data-handoff-decision]');
 expect(writes[0].path).toBe('/task-handoffs/handoff-a/commands');expect(writes[0].body).toMatchObject({project_id:project,expected_version:1,action:'accept',acknowledge_invalidation:true});expect(String(writes[0].body.reason)).toBeTruthy();expect(writes[0].body).not.toHaveProperty('status');expect(writes[0].body).not.toHaveProperty('owner_id');expect(changed).toHaveBeenCalledTimes(1);
});

it('aborts obsolete reads and hides old account/project facts and recovery after a scope change',async()=>{
 let release!:(response:Response)=>void;
 transport(async()=>new Promise<Response>(resolve=>{release=resolve;}));await render();await click('[data-handoff-toggle]');const previous=release,signal=reads[0].signal;
 const other={...session,user:{...session.user,id:'user-other'},organization:{id:'org-other',name:'Other'}};
 await render(other,'project-other','task-other');expect(signal?.aborted).toBe(true);expect(host.querySelector('[aria-expanded=false]')).not.toBeNull();expect(reads).toHaveLength(1);
 await act(async()=>previous(Response.json({...page(),items:[handoff()],total:1})));await settle();expect(host.textContent).not.toContain(raw);expect(host.querySelector('[data-handoff-item]')).toBeNull();
 transport(async()=>Response.json({error:{code:'forbidden',message:'Current access denied'}},{status:403}));await click('[data-handoff-toggle]');expect(host.textContent).toContain('Current access denied');expect(host.textContent).not.toContain(raw);expect(writes).toHaveLength(0);
});

it('opens and focuses an exact historical handoff outside the bounded page without writing or changing its count',async()=>{
 const exact={...handoff(),id:'handoff-old',state:'withdrawn',allowed_actions:[],decision_by:'user-a',decision_at:'2026-10-26T01:00:00Z',decision_reason:'Original withdrawal'};
 transport(async path=>Response.json(path.startsWith('/task-handoffs/handoff-old?')?exact:page()));
 await render(session,project,task,()=>{},{selectedHandoff:'handoff-old'});
 expect(reads.some(read=>read.path==='/task-handoffs/handoff-old?project_id='+project+'&task_id='+task)).toBe(true);
 const card=host.querySelector<HTMLElement>('[data-handoff-located="handoff-old"]');expect(card).not.toBeNull();expect(document.activeElement).toBe(card);
 expect(card?.textContent).toContain('Original withdrawal');expect(host.querySelector('.task-handoff-history-head')?.textContent).toContain('0 / 0');
 expect(writes).toHaveLength(0);expect(reads.every(read=>!read.path.includes('before='))).toBe(true);
});

it('preserves an independent proposal draft when a different decision succeeds and across remounts',async()=>{
 const current={...page(),items:[handoff()],total:1};
 transport(async(path,options)=>Response.json(options?.body?{...handoff(),state:'returned',version:2,decision_by:'user-b',decision_at:'2026-10-26T01:00:00Z',decision_reason:'Please clarify',basis_current:false,allowed_actions:[]}:path.startsWith('/task-handoffs/handoff-a?')?handoff():current));
 await render();await click('[data-handoff-toggle]');await change('[name=recipient]','resource-b');await change('[name=note]','Keep my separate proposal');
 await click('[data-handoff-command=return]');await change('[name=reason]','Please clarify');await submit('[data-handoff-decision]');
 expect(host.querySelector<HTMLTextAreaElement>('[name=note]')?.value).toBe('Keep my separate proposal');
 await act(async()=>root?.unmount());root=undefined;await render();await click('[data-handoff-toggle]');
 expect(host.querySelector<HTMLTextAreaElement>('[name=note]')?.value).toBe('Keep my separate proposal');
 expect(host.querySelector<HTMLSelectElement>('[name=recipient]')?.value).toBe('resource-b');
});

it('retains a decision reason on refresh and adopts its new version only explicitly with a fresh acceptance acknowledgement',async()=>{
 let revision=1;
 transport(async(path,options)=>Response.json(options?.body?{...handoff(),version:3,state:'accepted',allowed_actions:[],decision_reason:JSON.parse(String(options.body)).reason}:path.startsWith('/task-handoffs/handoff-a?')?{...handoff(),version:revision}:{...page(),items:[{...handoff(),version:revision}],total:1,can_create:false}));
 await render();await click('[data-handoff-toggle]');await click('[data-handoff-command=accept]');await change('[name=reason]','Keep this acceptance note');await click('[name=acknowledge_invalidation]');
 revision=2;await render(session,project,task,()=>{},{refresh:1});
 expect(host.querySelector<HTMLTextAreaElement>('[name=reason]')?.value).toBe('Keep this acceptance note');expect(host.querySelector<HTMLButtonElement>('[data-handoff-decision] button.primary')?.disabled).toBe(true);
 await click('[data-handoff-adopt-decision]');expect(host.querySelector<HTMLInputElement>('[name=acknowledge_invalidation]')?.checked).toBe(false);
 await click('[name=acknowledge_invalidation]');await submit('[data-handoff-decision]');expect(writes[0].body.expected_version).toBe(2);
});

it('revokes every protected card and input on an exact read refusal without deleting the scoped draft',async()=>{
 let denied=false;const onForbidden=vi.fn();
 transport(async path=>denied&&path.startsWith('/task-handoffs/handoff-a?')?Response.json({error:{code:'forbidden',message:'Current access denied'}},{status:403}):Response.json(path.startsWith('/task-handoffs/handoff-a?')?handoff():page()));
 await render(session,project,task,()=>{},{selectedHandoff:'handoff-a',onForbidden});await change('[name=note]','Preserve my private draft');
 denied=true;await render(session,project,task,()=>{},{selectedHandoff:'handoff-a',onForbidden,refresh:1});
 expect(onForbidden).toHaveBeenCalled();expect(host.querySelector('[data-handoff-item]')).toBeNull();expect(host.querySelector('[name=note]')).toBeNull();expect(host.textContent).not.toContain(raw);expect(writes).toHaveLength(0);
 expect(Object.values(sessionStorage).join(' ')).toContain('Preserve my private draft');
});

it('keeps the selected decision disabled after an exact read failure even when its history page still contains that handoff',async()=>{
 let failed=false;
 transport(async path=>path.startsWith('/task-handoffs/handoff-a?')?(failed?Response.json({error:{code:'unavailable',message:'Exact source unavailable'}},{status:500}):Response.json(handoff())):Response.json({...page(),items:[handoff()],total:1,can_create:false}));
 await render(session,project,task,()=>{},{selectedHandoff:'handoff-a'});await click('[data-handoff-command=accept]');await change('[name=reason]','Preserve the original reason');await click('[name=acknowledge_invalidation]');
 expect(host.querySelector<HTMLButtonElement>('[data-handoff-decision] button.primary')?.disabled).toBe(false);
 failed=true;await render(session,project,task,()=>{},{selectedHandoff:'handoff-a',refresh:1});
 expect(host.querySelector<HTMLButtonElement>('[data-handoff-decision] button.primary')?.disabled).toBe(true);expect(host.querySelector<HTMLTextAreaElement>('[name=reason]')?.value).toBe('Preserve the original reason');expect(writes).toHaveLength(0);
});
