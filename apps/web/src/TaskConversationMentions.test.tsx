// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {TaskConversation} from './TaskConversation';
import {setLocale} from './i18n';
vi.mock('./TaskSubscriptionControl',()=>({TaskSubscriptionControl:()=>null}));
import {conversationDraftKey} from './task-conversation-mentions-model';

const person = (id='a', name='Ada') => ({resource_id:'human-'+id,user_id:'person-'+id,name,resource_version:3});
const note = (id='old', sequence=1) => ({id:'message-'+id,sequence,task_version:7,body:'Original '+id,author_id:'user-a',author_name:'Recorded author',actor_kind:'human',created_at:'2026-10-29T00:00:00Z',requirement_id:'',requirement_version:0,run_id:'',reply_id:'',reply_to_sequence:0,execution_run_id:''});
const page = (items: ReturnType<typeof note>[] = []) => ({project_id:'project-a',task_id:'task-a',task_version:7,can_post:true,items,total:items.length,limit:20,next_before:0});
const people = (items=[person()], patch:Record<string,unknown>={}) => ({project_id:'project-a',task_id:'task-a',limit:20,items,total:items.length,next_cursor:'',...patch});
let host:HTMLDivElement, root:Root, current:Session;
let reads:{path:string;signal?:AbortSignal|null;headers:Headers}[], writes:{path:string;raw:string;body:Record<string,any>;headers:Headers}[];
let props:{project:string;taskId:string;refresh:number;selectedMessage?:string;accessBlocked:boolean;onAccessDenied:()=>void};
beforeEach(async()=>{
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true); await setLocale('en-US',{persist:false}); sessionStorage.clear();
  host=document.createElement('div'); document.body.append(host); root=createRoot(host); reads=[]; writes=[];
  current={user:{id:'user-a',name:'User A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
  props={project:'project-a',taskId:'task-a',refresh:0,accessBlocked:false,onAccessDenied:vi.fn()};
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(patch:Partial<typeof props>={}){props={...props,...patch};await act(async()=>root.render(<ApiViewProvider session={current}><TaskConversation session={current} {...props}/></ApiViewProvider>));await settle();}
function transport(reply?:(path:string,init?:RequestInit)=>Response|undefined|Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=String(input).replace(/^\/api\/v1/,''),headers=new Headers(init?.headers);
  if(init?.body)writes.push({path,raw:String(init.body),body:JSON.parse(String(init.body)),headers});else reads.push({path,signal:init?.signal,headers});
  const custom=await reply?.(path,init);if(custom)return custom;
  if(path.startsWith('/task-conversation-mention-targets?')){
    const ids=new URLSearchParams(path.split('?')[1]).get('resource_ids');return Response.json(people(ids?ids.split(',').map(id=>person(id.replace('human-',''))):[person(),person('b','Bo')]));
  }
  if(path.startsWith('/task-conversation-messages/'))return Response.json({project_id:props.project,task_id:props.taskId,task_version:7,can_post:true,message:note()});
  return Response.json({...page([note('recent',30)]),project_id:props.project,task_id:props.taskId});
}));}
async function click(selector:string){const target=host.querySelector<HTMLElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());await settle();}
async function type(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLTextAreaElement>(selector);expect(field,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field!.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
async function submit(){await act(async()=>{host.querySelector('[data-conversation-form]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}
async function openPicker(){await click('[data-conversation-toggle]');await click('[data-mention-toggle]');}

it('chooses canonical recipients explicitly, persists body plus IDs and replays the identical unknown request across refresh and language',async()=>{
  let lost=true;
  transport(async(_path,init)=>{
    if(!init?.body)return;
    const body=JSON.parse(String(init.body));if(lost)throw TypeError('receipt lost');
    return Response.json({...note('saved',31),body:body.body,mentions:body.mention_resource_ids.map((id:string)=>({resource_id:id,user_id:'person-'+id.slice(6),name_at_post:'Recorded '+id}))},{status:201});
  });
  await render();await openPicker();await type('[name=conversation_body]','  Plain @Nobody 🧪  ');
  expect(writes).toHaveLength(0);await click('[data-mention-option=human-a]');await click('[data-mention-option=human-b]');
  const draft=sessionStorage.getItem(conversationDraftKey('user-a','org-a','project-a','task-a'))!;
  expect(JSON.parse(draft)).toMatchObject({body:'  Plain @Nobody 🧪  ',mention_resource_ids:['human-a','human-b']});expect(draft).not.toContain('Ada');
  await submit();expect(writes).toHaveLength(1);expect(writes[0].body.mention_resource_ids).toEqual(['human-a','human-b']);
  expect(host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')?.disabled).toBe(true);
  await render({refresh:1});await act(async()=>{await setLocale('zh-CN',{persist:false});});
  lost=false;await click('[data-conversation-restore]');expect(writes).toHaveLength(2);expect(writes[1].raw).toBe(writes[0].raw);
  expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
  expect(host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')?.value).toBe('');expect(host.querySelectorAll('[data-mention-selected]')).toHaveLength(0);
  expect(writes.every(write=>write.path==='/task-conversations')).toBe(true);
});

it('never parses @text and omits mention fields from an ordinary immutable request',async()=>{
  transport(async(_path,init)=>init?.body?Response.json({...note('saved'),body:JSON.parse(String(init.body)).body},{status:201}):undefined);
  await render();await click('[data-conversation-toggle]');await type('[name=conversation_body]','@Ada is plain text');await submit();
  expect(writes).toHaveLength(1);expect(Object.hasOwn(writes[0].body,'mention_resource_ids')).toBe(false);
  expect(reads.some(read=>read.path.startsWith('/task-conversation-mention-targets'))).toBe(false);
});

it('keeps an unavailable selected ID visible, blocks sending, and treats recipient-only changes as dirty',async()=>{
  let visible=true;transport((path)=>path.includes('resource_ids=')?Response.json(people(visible?[person()]:[])):undefined);
  await render();await openPicker();await click('[data-mention-option=human-a]');
  const event=new Event('suma:navigate',{cancelable:true});await act(async()=>{window.dispatchEvent(event);});expect(event.defaultPrevented).toBe(true);
  visible=false;await render({refresh:1});expect(host.textContent).toContain('Unavailable · human-a');expect(host.textContent).not.toContain('Checking · human-a');
  await type('[name=conversation_body]','Keep this original selection');await submit();expect(writes).toHaveLength(0);
  expect(host.querySelector<HTMLButtonElement>('[data-conversation-form] button[type=submit]')?.disabled).toBe(true);
  expect(JSON.parse(sessionStorage.getItem(conversationDraftKey('user-a','org-a','project-a','task-a'))!).mention_resource_ids).toEqual(['human-a']);
  await click('[data-mention-selected=human-a] button');expect(host.querySelectorAll('[data-mention-selected]')).toHaveLength(0);
});

it('bounds directory pages and searches literal text without losing selected people outside search results',async()=>{
  transport(path=>{
    if(!path.startsWith('/task-conversation-mention-targets?'))return;
    const query=new URLSearchParams(path.split('?')[1]);if(query.has('resource_ids'))return Response.json(people([person()]));
    if(query.has('q'))return Response.json(people([]));
    return Response.json(query.has('cursor')?people([person('b','Bo')],{total:2}):people([person()],{total:2,next_cursor:'opaque-page'}));
  });
  await render();await openPicker();await click('[data-mention-option=human-a]');await click('[data-mention-more]');
  expect(reads.find(read=>read.path.includes('cursor='))?.path).toContain('cursor=opaque-page');
  expect(host.querySelectorAll('[data-mention-option]')).toHaveLength(2);
  await type('.task-conversation-mentions-search input','  literal %_  ');await click('.task-conversation-mentions-search button');
  const query=new URLSearchParams(reads.at(-1)!.path.split('?')[1]);expect(query.get('q')).toBe('  literal %_  ');expect(query.get('limit')).toBe('20');
  expect(host.querySelectorAll('[data-mention-option]')).toHaveLength(0);expect(host.querySelector('[data-mention-selected=human-a]')?.textContent).toContain('Ada');
});

it('restores only owner/task-authored draft data and ignores a late old directory refusal after identity switch',async()=>{
  let release!:(response:Response)=>void;
  transport((path)=>path.startsWith('/task-conversation-mention-targets?')?new Promise<Response>(resolve=>{release=resolve;}):undefined);
  await render();await openPicker();await type('[name=conversation_body]','Only owner A draft');
  const old=release,signal=reads.find(read=>read.path.startsWith('/task-conversation-mention-targets'))!.signal;
  current={...current,user:{...current.user,id:'user-b'},organization:{...current.organization,id:'org-b'}};
  await render();expect(signal?.aborted).toBe(true);await act(async()=>{old(Response.json({error:{code:'forbidden',message:'Old person denied'}},{status:403}));});await settle();
  expect(host.textContent).not.toContain('Only owner A draft');expect(host.textContent).not.toContain('Old person denied');expect(props.onAccessDenied).not.toHaveBeenCalled();
  current={...current,user:{...current.user,id:'user-a'},organization:{...current.organization,id:'org-a'}};await render();
  expect(host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')?.value).toBe('Only owner A draft');
});

it('keeps the newer same-task directory visible when an aborted older read later returns 403',async()=>{
  let release!:(response:Response)=>void, directoryReads=0;
  transport(path=>{
    if(!path.startsWith('/task-conversation-mention-targets?'))return;
    if(++directoryReads===1)return new Promise<Response>(resolve=>{release=resolve;});
    return Response.json(people([person('b','Current Bo')]));
  });
  await render();await openPicker();
  const old=reads.find(read=>read.path.startsWith('/task-conversation-mention-targets?'))!;
  await type('.task-conversation-mentions-search input','Bo');
  await act(async()=>{host.querySelector<HTMLInputElement>('.task-conversation-mentions-search input')!.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));});await settle();
  expect(old.signal?.aborted).toBe(true);
  expect(host.querySelector('[data-mention-option=human-b]')?.textContent).toContain('Current Bo');
  await act(async()=>{release(Response.json({error:{code:'forbidden',message:'Stale directory refused'}},{status:403}));});await settle();
  expect(host.querySelector('[data-mention-option=human-b]')?.textContent).toContain('Current Bo');
  expect(host.querySelector('[data-conversation-message]')).not.toBeNull();
  expect(host.textContent).not.toContain('Stale directory refused');
  expect(props.onAccessDenied).not.toHaveBeenCalled();expect(writes).toHaveLength(0);
});

it('reads and focuses an exact older message once without walking history, marking read or starting a reply',async()=>{
  transport();await render({selectedMessage:'message-old'});
  expect(host.querySelectorAll('[data-conversation-message=message-old]')).toHaveLength(1);
  expect(document.activeElement).toBe(host.querySelector('[data-conversation-message=message-old]'));
  expect(host.querySelector('[data-conversation-message=message-old]')?.classList.contains('is-located')).toBe(true);
  expect(reads.filter(read=>read.path.startsWith('/task-conversation-messages/'))).toHaveLength(1);
  expect(reads.some(read=>read.path.includes('before='))).toBe(false);expect(writes).toHaveLength(0);
  expect(reads.every(read=>!read.path.includes('/notification-inbox'))).toBe(true);
});

it('deduplicates a located message already in the current window and preserves recent history on transient locator failure',async()=>{
  let fail=false;transport(path=>{
    if(path.startsWith('/task-conversation-messages/'))return fail?Response.json({error:{code:'unavailable',message:'Original locator temporarily unavailable'}},{status:503}):Response.json({project_id:'project-a',task_id:'task-a',task_version:7,can_post:true,message:note()});
    if(path.startsWith('/task-conversations?'))return Response.json(page([note()]));
  });
  await render({selectedMessage:'message-old'});expect(host.querySelectorAll('[data-conversation-message=message-old]')).toHaveLength(1);
  fail=true;await render({refresh:1});expect(host.textContent).toContain('Original old');expect(host.textContent).toContain('Original locator temporarily unavailable');expect(writes).toHaveLength(0);
});

it('keeps the exact message focused when its direct source is later replaced by the recent-window instance',async()=>{
  let release!:(value:Response)=>void;transport(path=>path.startsWith('/task-conversations?')?new Promise<Response>(resolve=>{release=resolve;}):undefined);
  await render({selectedMessage:'message-old'});const direct=host.querySelector('[data-conversation-message=message-old]');expect(document.activeElement).toBe(direct);
  await act(async()=>{release(Response.json(page([note()])));});await settle();
  expect(host.querySelectorAll('[data-conversation-message=message-old]')).toHaveLength(1);
  expect(document.activeElement).toBe(host.querySelector('[data-conversation-message=message-old]'));
  expect(reads.filter(read=>read.path.startsWith('/task-conversation-messages/'))).toHaveLength(1);expect(writes).toHaveLength(0);
});

it('preserves unreadable draft bytes until explicitly cleared even while current task access is blocked',async()=>{
  const key=conversationDraftKey('user-a','org-a','project-a','task-a'),original='{"format":"unknown","body":"unique original"}';sessionStorage.setItem(key,original);
  transport();await render({accessBlocked:true});expect(sessionStorage.getItem(key)).toBe(original);expect(reads).toHaveLength(0);
  const leave=new Event('suma:navigate',{cancelable:true});await act(async()=>{window.dispatchEvent(leave);});expect(leave.defaultPrevented).toBe(true);
  const clear=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent==='Clear unreadable saved draft');expect(clear).toBeDefined();
  await act(async()=>{clear!.click();});await settle();expect(sessionStorage.getItem(key)).toBeNull();
  const after=new Event('suma:navigate',{cancelable:true});await act(async()=>{window.dispatchEvent(after);});expect(after.defaultPrevented).toBe(false);expect(writes).toHaveLength(0);
});

it('clears current private history and selected names on directory 404 while retaining only the local body and IDs',async()=>{
  let refuse=false;transport(path=>path.startsWith('/task-conversation-mention-targets?')&&refuse?Response.json({error:{code:'not_found',message:'Current directory unavailable'}},{status:404}):undefined);
  await render();await openPicker();await click('[data-mention-option=human-a]');await type('[name=conversation_body]','Local draft retained');
  refuse=true;await render({refresh:1});expect(props.onAccessDenied).toHaveBeenCalled();expect(host.querySelector('[data-conversation-message]')).toBeNull();expect(host.querySelector('[data-mention-selected]')).toBeNull();
  expect(host.textContent).not.toContain('Ada');expect(host.textContent).not.toContain('Original recent');expect(writes).toHaveLength(0);
  expect(JSON.parse(sessionStorage.getItem(conversationDraftKey('user-a','org-a','project-a','task-a'))!)).toMatchObject({body:'Local draft retained',mention_resource_ids:['human-a']});
});
