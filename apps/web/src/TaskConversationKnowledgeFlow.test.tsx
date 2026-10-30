// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {TaskConversation} from './TaskConversation';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
vi.mock('./TaskSubscriptionControl',()=>({TaskSubscriptionControl:()=>null}));
import {conversationDraftKey} from './task-conversation-mentions-model';

const pin={id:'knowledge-a',revision:2,sha256:'a'.repeat(64)};
const entry={id:pin.id,project_id:'p',version:4,kind:'spec',state:'published',title:'Private newer draft',summary:'',latest_revision:3,published_revision:2,published_sha256:pin.sha256,published_title:'Approved source',published_summary:'',has_draft:true,published_eligible:true,eligibility:'current',created_at:'2026-10-30T00:00:00Z',updated_at:'2026-10-30T00:00:00Z'};
const note={id:'m',sequence:1,task_version:7,body:'Original note',author_id:'u',author_name:'User',actor_kind:'human',created_at:'2026-10-30T00:00:00Z',requirement_id:'',requirement_version:0,run_id:'',reply_id:'',reply_to_sequence:0,execution_run_id:''};
const page={project_id:'p',task_id:'t',task_version:7,can_post:true,items:[],total:0,limit:20,next_before:0};
let host:HTMLDivElement,root:Root,identity:Session,writes:{raw:string;body:Record<string,any>;headers:Headers}[],reads:string[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=[];reads=[];identity={user:{id:'u',name:'User',email:'u@example.test'},organization:{id:'o',name:'Org'},projects:[]};});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<7;i++)await act(async()=>{await Promise.resolve();await Promise.resolve();});}
async function render(refresh=0){await act(async()=>root.render(<ApiViewProvider session={identity}><TaskConversation session={identity} project="p" taskId="t" refresh={refresh} accessBlocked={false}/></ApiViewProvider>));await settle();}
async function click(selector:string){const node=host.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
async function type(value:string){const field=host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}
async function submit(){await act(async()=>{host.querySelector('[data-conversation-form]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('keeps explicit exact pins dirty and replays identical bytes after a lost receipt despite changed publication',async()=>{
  let lost=true,eligible=true;
  vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
    const path=String(input).replace(/^\/api\/v1/,'');reads.push(path);
    if(init?.body){const raw=String(init.body),body=JSON.parse(raw);writes.push({raw,body,headers:new Headers(init.headers)});if(lost)throw TypeError('receipt lost');return Response.json({...note,body:body.body,knowledge:body.knowledge.map((value:typeof pin)=>({...value,title_at_post:'Original publication',kind_at_post:'spec'}))},{status:201});}
    if(path.startsWith('/project-knowledge?'))return Response.json({items:[entry],total:1,next_cursor:''});
    if(path.startsWith('/project-knowledge/'))return Response.json({...entry,published_eligible:eligible,eligibility:eligible?'current':'retired'});
    return Response.json(page);
  }));
  await render();await click('[data-conversation-toggle]');expect(reads.some(path=>path.startsWith('/project-knowledge'))).toBe(false);
  await click('[data-knowledge-toggle]');await click('[data-knowledge-option=knowledge-a]');
  expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);
  const stored=sessionStorage.getItem(conversationDraftKey('u','o','p','t'))!;expect(JSON.parse(stored).knowledge).toEqual([pin]);expect(stored).not.toContain('Approved source');
  await type('  original body 🧪  ');await submit();expect(writes).toHaveLength(1);expect(writes[0].body.knowledge).toEqual([pin]);expect(Object.keys(writes[0].body.knowledge[0]).sort()).toEqual(['id','revision','sha256']);
  eligible=false;await render(1);await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('添加项目资料');lost=false;
  await click('[data-conversation-restore]');expect(writes).toHaveLength(2);expect(writes[1].raw).toBe(writes[0].raw);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('u');
  expect(host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')?.value).toBe('');expect(host.querySelector('[data-knowledge-selected]')).toBeNull();
});
it('preserves plain requests without an empty knowledge field',async()=>{
  vi.stubGlobal('fetch',vi.fn(async(_input:string,init?:RequestInit)=>{if(init?.body){const raw=String(init.body),body=JSON.parse(raw);writes.push({raw,body,headers:new Headers(init.headers)});return Response.json({...note,body:body.body},{status:201});}return Response.json(page);}));
  await render();await click('[data-conversation-toggle]');await type('Plain note');await submit();expect(writes).toHaveLength(1);expect(Object.hasOwn(writes[0].body,'knowledge')).toBe(false);
});
