// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {NotificationInbox,type NotificationInboxProps} from './NotificationInbox';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {notificationInboxScope,notificationOrganizationInboxScope} from './notification-inbox-model';
import type {Session} from './api';

const item=(patch:Record<string,unknown>={})=>({id:'notice-a',version:1,sequence:'9007199254740993',kind:'task_assigned',task_id:'task-a',source_task_version:2,source_resource_id:'human-a',created_at:'2026-10-29T02:00:00Z',read:false,archived:false,read_at:'',archived_at:'',source_available:true,assignment_current:true,current_task:{id:'task-a',version:4,title:'Private assigned task',status:'draft'},...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',user_id:'user-a',limit:20,items:[item()],total:1,counts:{inbox:1,unread:1,archived:0},next_cursor:'',as_of:'2026-10-29T03:00:00Z',...patch});
const orgRow=(project_id:string,project_title:string,notification=item())=>({project_id,project_title,notification});
const orgPage=(items:ReturnType<typeof orgRow>[],patch:Record<string,unknown>={})=>({
 organization_id:'org-a',user_id:'user-a',limit:20,items,total:items.length,
 counts:{inbox:items.length,unread:items.filter(row=>!row.notification.read).length,archived:0},
 next_cursor:'',as_of:'2026-10-29T03:00:00Z',scope_revision:'a'.repeat(64),...patch,
});
let host:HTMLDivElement,root:Root,props:NotificationInboxProps,records:ReturnType<typeof item>[],reads:{path:string;signal?:AbortSignal|null;headers:Headers}[],writes:{path:string;body:Record<string,any>;headers:Headers}[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);reads=[];writes=[];records=[item()];props={project:'project-a',userID:'user-a',organizationID:'org-a',filter:'inbox',refresh:0,onFilter:vi.fn(),onTask:vi.fn()};});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
function receipt(body:Record<string,any>,patch:Record<string,unknown>={}){return {idempotency_key:body.idempotency_key,notification_id:'notice-a',version:body.expected_version+1,action:body.action,read:body.action==='read',archived:body.action==='archive',read_at:body.action==='read'?'2026-10-29T04:00:00Z':'',archived_at:body.action==='archive'?'2026-10-29T04:00:00Z':'',updated_at:'2026-10-29T04:00:00Z',...patch};}
function transport(reply?:(path:string,init?:RequestInit)=>Response|undefined|Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
 const path=String(input).replace(/^\/api\/v1/,''),body=init?.body?JSON.parse(String(init.body)):undefined;
 if(path.startsWith('/notification-preferences'))return Response.json({project_id:props.project,user_id:props.userID,version:0,followed_discussions_muted:false,updated_at:''});
 if(body)writes.push({path,body,headers:new Headers(init?.headers)});else reads.push({path,signal:init?.signal,headers:new Headers(init?.headers)});
 const custom=await reply?.(path,init);if(custom)return custom;
 if(body){const id=path.split('/')[2],index=records.findIndex(record=>record.id===id);if(index<0)return Response.json({error:{code:'not_found',message:'Notification unavailable'}},{status:404});const current=records[index];if(current.version!==body.expected_version)return Response.json({error:{code:'version_conflict',message:'Notification changed'}},{status:409});const read=body.action==='read'?true:body.action==='unread'?false:current.read,archived=body.action==='archive'?true:body.action==='restore'?false:current.archived,version=current.version+(read!==current.read||archived!==current.archived?1:0),read_at=read?current.read_at||'2026-10-29T04:00:00Z':'',archived_at=archived?current.archived_at||'2026-10-29T04:00:00Z':'';records[index]={...current,version,read,archived,read_at,archived_at};return Response.json(receipt(body,{notification_id:id,version,read,archived,read_at,archived_at}));}
 const filter=new URLSearchParams(path.split('?')[1]).get('filter'),counts={inbox:records.filter(row=>!row.archived).length,unread:records.filter(row=>!row.archived&&!row.read).length,archived:records.filter(row=>row.archived).length},items=records.filter(row=>filter==='archived'?row.archived:!row.archived&&(filter!=='unread'||!row.read));return Response.json(page({project_id:props.project,user_id:props.userID,items,total:items.length,counts}));
}));}
async function settle(){for(let i=0;i<6;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(patch:Partial<NotificationInboxProps>={}){props={...props,...patch};const session:Session={user:{id:props.userID,name:'User',email:'user@example.test'},organization:{id:props.organizationID,name:'Organization'},projects:[]};await act(async()=>root.render(<ApiViewProvider session={session}><NotificationInbox {...props}/></ApiViewProvider>));await settle();}

it('opens a followed task notification at its original message and keeps opening separate from marking read',async()=>{
 records=[item({kind:'task_followed',assignment_current:false,source_resource_id:'subscription-a',message_id:'message-original'})];transport();await render();
 expect(host.querySelector<HTMLButtonElement>('.notification-inbox-source')?.disabled).toBe(true);
 const onMessage=vi.fn();await render({onMessage});expect(host.textContent).toContain('New discussion from a followed task');expect(host.textContent).not.toContain('Assignment has changed');expect(host.querySelector('.notification-inbox-source')?.getAttribute('aria-label')).toBe('Open discussion: Private assigned task');
 await click('.notification-inbox-source');expect(onMessage).toHaveBeenCalledExactlyOnceWith('task-a','message-original');expect(props.onTask).not.toHaveBeenCalled();expect(writes).toHaveLength(0);expect(records[0].read).toBe(false);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('已关注任务有新讨论');expect(host.querySelector('.notification-inbox-source')?.getAttribute('aria-label')).toBe('打开讨论：Private assigned task');
 records=[item({kind:'task_followed',assignment_current:false,source_available:false,current_task:undefined,source_resource_id:'subscription-a'})];await render({refresh:1});expect(host.querySelector('.notification-inbox-source')).toBeNull();expect(host.textContent).not.toContain('Private assigned task');expect(writes).toHaveLength(0);
});

it('opens an exact handoff only through its callback without marking read or accepting responsibility',async()=>{
 const onHandoff=vi.fn();records=[item({kind:'task_handoff_requested',assignment_current:false,handoff_id:'handoff-original'})];transport();await render();
 const unavailable=host.querySelector<HTMLButtonElement>('.notification-inbox-source');expect(unavailable).not.toBeNull();expect(unavailable?.disabled).toBe(true);
 await render({onHandoff});const source=host.querySelector<HTMLButtonElement>('.notification-inbox-source')!;
 expect(source.disabled).toBe(false);expect(source.type).toBe('button');expect(source.getAttribute('aria-label')).toBe('Open handoff request: Private assigned task');source.focus();expect(document.activeElement).toBe(source);
 expect(host.textContent).toContain('You were asked to take responsibility for a task');expect(host.textContent).not.toContain('Assignment has changed');
 await click('.notification-inbox-source');expect(onHandoff).toHaveBeenCalledWith('task-a','handoff-original');expect(props.onTask).not.toHaveBeenCalled();expect(writes).toHaveLength(0);expect(reads).toHaveLength(1);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('有人请你接收任务责任');expect(host.querySelector('.notification-inbox-source')?.getAttribute('aria-label')).toBe('打开交接请求：Private assigned task');expect(reads).toHaveLength(1);
 records=[item({kind:'task_handoff_requested',assignment_current:false,source_available:false,current_task:undefined})];await render({refresh:1});expect(host.querySelector('.notification-inbox-source')).toBeNull();expect(writes).toHaveLength(0);
});

it('opens the exact original review request without accepting or marking the notification read',async()=>{
 const onReviewRequest=vi.fn();records=[item({kind:'task_review_requested',assignment_current:false,review_request_id:'request-old'})];transport();await render({onReviewRequest});expect(host.textContent).toContain('You were asked to review a task delivery');expect(host.textContent).not.toContain('Assignment has changed');
 await click('.notification-inbox-source');expect(onReviewRequest).toHaveBeenCalledWith('task-a','request-old');expect(props.onTask).not.toHaveBeenCalled();expect(writes).toHaveLength(0);expect(reads).toHaveLength(1);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('有人请你验收任务交付');
 records=[item({kind:'task_review_requested',assignment_current:false,source_available:false,current_task:undefined})];await render({refresh:1});expect(host.querySelector('.notification-inbox-source')).toBeNull();
});

it('opens a mention at its exact original discussion without marking read or changing assignment semantics',async()=>{
 const onMessage=vi.fn();records=[item({kind:'task_mentioned',assignment_current:false,message_id:'message-old'})];transport();await render({onMessage});
 expect(host.textContent).toContain('You were mentioned in a task discussion');expect(host.textContent).not.toContain('Assignment has changed');
 expect(host.querySelector('.notification-inbox-source')?.getAttribute('aria-label')).toContain('Open discussion');
 await click('.notification-inbox-source');expect(onMessage).toHaveBeenCalledWith('task-a','message-old');expect(props.onTask).not.toHaveBeenCalled();expect(writes).toHaveLength(0);expect(reads).toHaveLength(1);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('有人在任务讨论中提及了你');
 records=[item({kind:'task_mentioned',assignment_current:false,source_available:false,current_task:undefined})];await render({refresh:1});expect(host.querySelector('.notification-inbox-source')).toBeNull();expect(writes).toHaveLength(0);
});
async function click(selector:string){const target=host.querySelector<HTMLButtonElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());await settle();}

it('shows real complete counts and current source facts, and opening the task never marks it read',async()=>{
 records=[item(),item({id:'notice-b',sequence:'9007199254740992',read:true,read_at:'2026-10-29T02:30:00Z'}),item({id:'notice-c',sequence:'9007199254740991',archived:true,archived_at:'2026-10-29T02:40:00Z'})];transport();await render();expect(host.querySelector('[data-notification-filter=inbox] .notification-inbox-count')?.textContent).toBe('2');expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('1');expect(host.querySelector('[data-notification-filter=archived] .notification-inbox-count')?.textContent).toBe('1');expect(host.querySelectorAll('[data-notification-id]')).toHaveLength(2);expect(host.textContent).toContain('Current task status');expect(reads[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(reads[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(reads[0].path).not.toContain('user_id');await click('.notification-inbox-source');expect(props.onTask).toHaveBeenCalledWith('task-a');expect(writes).toHaveLength(0);expect(reads).toHaveLength(1);
});

it('marks read and unread explicitly with current versions and fresh complete counts',async()=>{
 transport();await render();await click('[data-notification-action=read]');expect(writes[0].body).toMatchObject({project_id:'project-a',expected_version:1,action:'read'});expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('0');await click('[data-notification-action=unread]');expect(writes[1].body).toMatchObject({expected_version:2,action:'unread'});expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('1');
});

it('archives and restores a notification without marking it read',async()=>{
 transport();await render();await click('[data-notification-action=archive]');expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.querySelector('[data-notification-filter=archived] .notification-inbox-count')?.textContent).toBe('1');await click('[data-notification-filter=archived]');expect(props.onFilter).toHaveBeenCalledWith('archived');await render({filter:'archived'});expect(host.querySelector('[data-notification-action=read]')).not.toBeNull();await click('[data-notification-action=restore]');expect(writes[1].body).toMatchObject({action:'restore',expected_version:2});expect(records[0].read).toBe(false);expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('1');
});

it('does not invent a task title or source link when the task is unavailable and distinguishes changed assignments',async()=>{
 records=[item({source_available:false,assignment_current:false,current_task:undefined})];transport();await render();expect(host.textContent).toContain('Task source unavailable');expect(host.querySelector('.notification-inbox-source')).toBeNull();expect(host.textContent).not.toContain('Private assigned task');records=[item({assignment_current:false})];await render({refresh:1});expect(host.textContent).toContain('Assignment has changed');expect(host.querySelector('.notification-inbox-source')).not.toBeNull();
});

it('loads a lower sequence page explicitly and clears the cursor when the filter changes',async()=>{
 transport(path=>{const params=new URLSearchParams(path.split('?')[1]);if(params.get('filter')==='unread')return Response.json(page());return Response.json(page({items:[item(params.has('cursor')?{id:'notice-b',sequence:'9007199254740992'}:{})],total:3,counts:{inbox:3,unread:3,archived:0},next_cursor:params.has('cursor')?'':'opaque+/='}));});await render();expect(reads).toHaveLength(1);await click('[data-notification-more]');expect(host.querySelectorAll('[data-notification-id]')).toHaveLength(2);expect(new URLSearchParams(reads[1].path.split('?')[1]).get('cursor')).toBe('opaque+/=');await render({filter:'unread'});expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.querySelectorAll('[data-notification-id]')).toHaveLength(1);
});

it.each([401,403,404])('clears every private row, count and task target after HTTP%i and retries from page one',async status=>{
 let refused=false;transport(()=>refused?Response.json({error:{code:'denied',message:'Current access refused'}},{status}):Response.json(page({total:2,counts:{inbox:2,unread:2,archived:0},next_cursor:'private-cursor'})));await render();refused=true;await click('[data-notification-more]');expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.querySelector('.notification-inbox-source')).toBeNull();expect(host.textContent).not.toContain('Private assigned task');expect(host.querySelector('[data-notification-filter=inbox] .notification-inbox-count')?.textContent).toBe('—');expect(host.textContent).toContain('Private notification and task details have been cleared');refused=false;await click('.error button');expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.textContent).toContain('Private assigned task');
});

it('drops private facts immediately after a lifecycle command loses permission',async()=>{
 transport((_path,init)=>init?.body?Response.json({error:{code:'forbidden',message:'Lifecycle access revoked'}},{status:403}):undefined);await render();await click('[data-notification-action=read]');expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.textContent).not.toContain('Private assigned task');expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('—');expect(host.textContent).toContain('Private notification and task details have been cleared');
});

it('ignores a late old identity read refusal after a new user and organization receive their own page',async()=>{
 let finish!:(value:Response)=>void;transport((_path,init)=>new Headers(init?.headers).get('X-Suma-Expected-User')==='user-a'?new Promise<Response>(resolve=>{finish=resolve;}):Response.json(page({user_id:'user-b',items:[item({id:'notice-b',task_id:'task-b',current_task:{id:'task-b',version:1,title:'Only user B source',status:'todo'}})]})));await render();const original=reads[0];await render({userID:'user-b',organizationID:'org-b'});expect(original.signal?.aborted).toBe(true);await act(async()=>finish(Response.json({error:{code:'forbidden',message:'Old identity refused'}},{status:403})));await settle();expect(host.textContent).toContain('Only user B source');expect(host.textContent).not.toContain('Old identity refused');expect(host.textContent).not.toContain('Private assigned task');
});

it('ignores a late old identity command refusal and retains only that identity’s original recovery request',async()=>{
 let finish!:(value:Response)=>void;transport((_path,init)=>{
  if(init?.body)return new Promise<Response>(resolve=>{finish=resolve;});
  if(new Headers(init?.headers).get('X-Suma-Expected-User')==='user-b')return Response.json(page({user_id:'user-b',items:[item({id:'notice-b',task_id:'task-b',current_task:{id:'task-b',version:1,title:'User B current task',status:'todo'}})]}));
 });await render();await click('[data-notification-action=read]');const original=writes[0];await render({userID:'user-b',organizationID:'org-b'});await act(async()=>finish(Response.json({error:{code:'forbidden',message:'Original identity denied'}},{status:403})));await settle();expect(host.textContent).toContain('User B current task');expect(host.textContent).not.toContain('Original identity denied');expect(host.querySelector('[data-notification-recover]')).toBeNull();expect(original.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(sessionStorage.getItem('suma-write-recovery-v1:'+notificationInboxScope('user-a','org-a','project-a'))).toContain(original.body.idempotency_key);expect(sessionStorage.getItem('suma-write-recovery-v1:'+notificationInboxScope('user-b','org-b','project-a'))).toBeNull();
});

it('clears a failed transient page and retries from the beginning without stale counts',async()=>{
 let unavailable=false;transport(()=>unavailable?Response.json({error:{code:'unavailable',message:'Temporary read failure'}},{status:503}):Response.json(page({total:2,counts:{inbox:2,unread:2,archived:0},next_cursor:'next-page'})));await render();unavailable=true;await click('[data-notification-more]');expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.querySelector('[data-notification-filter=inbox] .notification-inbox-count')?.textContent).toBe('—');unavailable=false;await click('.error button');expect(reads.at(-1)?.path).not.toContain('cursor=');expect(host.textContent).toContain('Private assigned task');
});

it('keeps an unknown command through refresh and restores the exact original ID/version without source snapshots',async()=>{
 let lost=true,originalBody:Record<string,any>|undefined;transport((_path,init)=>{if(!init?.body)return;const body=JSON.parse(String(init.body));if(lost){originalBody=body;records=[item({version:2,archived:true,archived_at:'2026-10-29T04:00:00Z'})];throw new TypeError('Receipt lost');}return Response.json(receipt(body,{archived:true,read:false,read_at:'',archived_at:'2026-10-29T04:00:00Z'}));});await render();await click('[data-notification-action=archive]');expect(host.querySelector('[data-notification-recover]')).not.toBeNull();let navigationAllowed=true;await act(async()=>{navigationAllowed=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});expect(navigationAllowed).toBe(false);expect(host.querySelector<HTMLButtonElement>('[data-notification-filter=unread]')?.disabled).toBe(true);const key='suma-write-recovery-v1:'+notificationInboxScope('user-a','org-a','project-a'),stored=sessionStorage.getItem(key)!;expect(stored).not.toContain('Private assigned task');expect(stored).not.toContain('current_task');await render({refresh:1});expect(host.querySelector('[data-notification-recover]')).not.toBeNull();lost=false;await click('[data-notification-recover]');expect(writes[1].body).toEqual(originalBody);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(sessionStorage.getItem(key)).toBeNull();expect(host.querySelector('[data-notification-id]')).toBeNull();await act(async()=>{navigationAllowed=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});expect(navigationAllowed).toBe(true);
});

it('does not acknowledge a malformed receipt or accept a source snapshot as a personal-state receipt',async()=>{
 transport((_path,init)=>init?.body?Response.json({...receipt(JSON.parse(String(init.body))),current_task:{id:'task-a',title:'Untrusted receipt snapshot'}}):undefined);await render();await click('[data-notification-action=read]');expect(host.querySelector('[data-notification-recover]')).not.toBeNull();expect(host.textContent).not.toContain('Untrusted receipt snapshot');expect(host.textContent).not.toContain('Your notification update was confirmed.');expect(sessionStorage.length).toBe(1);
});

it('requires an explicit conflict refresh before issuing a new command at the newer version',async()=>{
 let conflict=true;transport((_path,init)=>{if(init?.body&&conflict){conflict=false;records=[item({version:3})];return Response.json({error:{code:'version_conflict',message:'State changed elsewhere'}},{status:409});}});await render();await click('[data-notification-action=read]');expect(writes).toHaveLength(1);expect(host.querySelector('.conflict-banner')).not.toBeNull();await click('.conflict-banner button');expect(writes).toHaveLength(1);await click('[data-notification-action=read]');expect(writes[1].body.expected_version).toBe(3);expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
});

it('acknowledges an older original receipt without overwriting a newer authorized row or counts',async()=>{
 let finish!:(value:Response)=>void;transport((_path,init)=>init?.body?new Promise<Response>(resolve=>{finish=resolve;}):undefined);await render();await click('[data-notification-action=read]');records=[item({version:5,current_task:{id:'task-a',version:8,title:'Newer current source',status:'in_progress'}})];await render({refresh:1});expect(host.textContent).toContain('Newer current source');const before=reads.length;await act(async()=>finish(Response.json(receipt(writes[0].body))));await settle();expect(host.textContent).toContain('Newer current source');expect(host.querySelector('[data-notification-action=read]')).not.toBeNull();expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('1');expect(reads).toHaveLength(before);expect(host.textContent).toContain('newer notification state');
});

it('never widens an invalid route and updates Chinese/English copy without re-reading private data',async()=>{
 transport();await render({invalid:true});expect(reads).toHaveLength(0);expect(host.textContent).toContain('No broader query was made');await click('[data-notification-filter=unread]');expect(props.onFilter).toHaveBeenCalledWith('unread');await render({invalid:false,filter:'unread'});const before=reads.length;await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('任务已分配给本人');expect(reads).toHaveLength(before);await act(async()=>setLocale('en-US',{persist:false}));expect(host.textContent).toContain('Task assigned to you');expect(reads).toHaveLength(before);
});

it('clears inbox private rows and retires an in-flight page when current preferences access is refused',async()=>{
 let denyPreferences=false,finishPage!:(value:Response)=>void,pageSignal:AbortSignal|null|undefined;
 const calls:{path:string;method:string}[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=String(input).replace(/^\/api\/v1/,'');calls.push({path,method:init?.method||'GET'});
  if(path.startsWith('/notification-preferences'))return denyPreferences
   ?Response.json({error:{code:'forbidden',message:'Project access revoked'}},{status:403})
   :Response.json({project_id:props.project,user_id:props.userID,version:0,followed_discussions_muted:false,updated_at:''});
  if(new URLSearchParams(path.split('?')[1]).has('cursor')){pageSignal=init?.signal;return new Promise<Response>(resolve=>{finishPage=resolve;});}
  return Response.json(page({total:2,counts:{inbox:2,unread:2,archived:0},next_cursor:'next-page'}));
 }));
 await render();expect(host.textContent).toContain('Private assigned task');
 await click('[data-notification-preferences-toggle]');
 await click('[data-notification-more]');expect(pageSignal?.aborted).toBe(false);
 denyPreferences=true;await click('[data-notification-preferences-refresh]');
 expect(pageSignal?.aborted).toBe(true);expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.textContent).not.toContain('Private assigned task');
 expect(host.querySelector('[data-notification-filter=unread] .notification-inbox-count')?.textContent).toBe('—');
 await act(async()=>finishPage(Response.json(page({items:[item({id:'notice-b',sequence:'9007199254740992',current_task:{id:'task-b',version:1,title:'Late private task',status:'draft'}})],total:2,counts:{inbox:2,unread:2,archived:0},next_cursor:''}))));await settle();
 expect(host.querySelector('[data-notification-id]')).toBeNull();expect(host.textContent).not.toContain('Late private task');
 expect(calls.every(call=>call.method==='GET')).toBe(true);
});

it('opens the exact organization source in its project and sends explicit lifecycle updates only to that project',async()=>{
 const onHandoff=vi.fn(),onScopeMode=vi.fn();
 records=[item({kind:'task_handoff_requested',assignment_current:false,handoff_id:'handoff-b'})];
 transport((path,init)=>!init?.body&&path.startsWith('/notification-inbox/organization')
  ?Response.json(orgPage([orgRow('project-b','Second project',records[0])])):undefined);
 await render({scopeMode:'organization',onScopeMode,onHandoff});
 expect(reads[0].path).toBe('/notification-inbox/organization?filter=inbox&limit=20');
 expect(reads[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(host.querySelector('.notification-inbox-project')?.textContent).toBe('Project: Second project');
 expect(host.querySelector('[data-notification-preferences-toggle]')).toBeNull();
 const scope=host.querySelector<HTMLSelectElement>('[data-notification-scope-mode]')!;
 expect(scope.value).toBe('organization');
 await act(async()=>{scope.value='project';scope.dispatchEvent(new Event('change',{bubbles:true}));});
 expect(onScopeMode).toHaveBeenCalledWith('project');
 await click('.notification-inbox-source');
 expect(onHandoff).toHaveBeenCalledExactlyOnceWith('task-a','handoff-b','project-b');
 expect(writes).toHaveLength(0);
 await click('[data-notification-action=read]');
 expect(writes[0].path).toBe('/notification-inbox/notice-a/commands');
 expect(writes[0].body).toMatchObject({project_id:'project-b',expected_version:1,action:'read'});
 expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('user-a');
 expect(writes[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(host.querySelector('[data-notification-action=unread]')).not.toBeNull();
});

it('clears organization rows and totals after a visible-project cursor conflict and refreshes without the old cursor',async()=>{
 const revoked=orgRow('project-b','Private project label');
 const current=orgRow('project-a','Current project',item({id:'notice-current',current_task:{id:'task-a',version:1,title:'Current visible source',status:'todo'}}));
 let accessChanged=false;
 transport(path=>{
  if(new URLSearchParams(path.split('?')[1]).has('cursor')){
   accessChanged=true;return Response.json({error:{code:'version_conflict',message:'Current project scope changed'}},{status:409});
  }
  return Response.json(accessChanged?orgPage([current],{scope_revision:'b'.repeat(64)}):orgPage([revoked],{total:2,counts:{inbox:2,unread:2,archived:0},next_cursor:'first-access'}));
 });
 await render({scopeMode:'organization'});
 expect(host.textContent).toContain('Private project label');
 await click('[data-notification-more]');
 expect(host.querySelector('[data-notification-id]')).toBeNull();
 expect(host.textContent).not.toContain('Private project label');
 expect(host.querySelector('[data-notification-filter=inbox] .notification-inbox-count')?.textContent).toBe('—');
 expect(host.textContent).toContain('Your project access changed');
 await click('[data-notification-refresh]');
 expect(reads.at(-1)?.path).not.toContain('cursor=');
 expect(host.textContent).toContain('Current visible source');
 expect(host.textContent).not.toContain('Private assigned task');
});

it('retains an unknown organization command after the source is revoked while reopening current rows and allowing navigation',async()=>{
 const revoked=orgRow('project-b','Private project label');
 const current=orgRow('project-a','Current project',item({id:'notice-current',current_task:{id:'task-a',version:1,title:'Current visible source',status:'todo'}}));
 let revokedAccess=false;
 transport((_path,init)=>{
  if(init?.body){
   if(!revokedAccess)throw new TypeError('Receipt lost');
   return Response.json({error:{code:'forbidden',message:'Source project revoked'}},{status:403});
  }
  return Response.json(revokedAccess?orgPage([current],{scope_revision:'b'.repeat(64)}):orgPage([revoked]));
 });
 await render({scopeMode:'organization',onScopeMode:vi.fn()});
 await click('[data-notification-action=archive]');
 const key='suma-write-recovery-v1:'+notificationOrganizationInboxScope('user-a','org-a');
 expect(sessionStorage.getItem(key)).toContain(writes[0].body.idempotency_key);
 expect(sessionStorage.getItem('suma-write-recovery-v1:'+notificationInboxScope('user-a','org-a','project-b'))).toBeNull();
 revokedAccess=true;
 await click('[data-notification-recover]');
 expect(writes[1].body).toEqual(writes[0].body);
 expect(host.textContent).toContain('Current visible source');
 expect(host.textContent).not.toContain('Private project label');
 expect(host.textContent).toContain('The source project refused this update');
 expect(host.querySelector<HTMLSelectElement>('[data-notification-scope-mode]')?.disabled).toBe(false);
 expect(host.querySelector<HTMLButtonElement>('[data-notification-action=archive]')?.disabled).toBe(true);
 expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(true);
 expect(sessionStorage.getItem(key)).toContain(writes[0].body.idempotency_key);
});

it('reads an empty organization inbox without a selected project and coalesces foreground rechecks',async()=>{
 transport(()=>Response.json(orgPage([])));
 await render({scopeMode:'organization',project:''});
 expect(host.querySelector('[data-notification-filter=inbox] .notification-inbox-count')?.textContent).toBe('0');
 expect(reads).toHaveLength(1);
 await act(async()=>{window.dispatchEvent(new Event('focus'));window.dispatchEvent(new Event('online'));});
 await settle();
 expect(reads).toHaveLength(2);
 expect(reads.every(read=>read.path==='/notification-inbox/organization?filter=inbox&limit=20')).toBe(true);
});
