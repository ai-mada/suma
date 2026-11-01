// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {TaskConversationMessage,TaskConversationPage} from '../../../packages/contracts/src';
import {TaskConversation} from './TaskConversation';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';

type SteeringProps={selectedMessage:string;disabled?:boolean;onLockChange?:(locked:boolean)=>void};
type ReplyProps={disabled:boolean;onLockChange?:(locked:boolean)=>void};
const children=vi.hoisted(()=>({steering:undefined as SteeringProps|undefined,reply:undefined as ReplyProps|undefined}));
vi.mock('./TurnSteeringPanel',()=>({TurnSteeringPanel:(props:SteeringProps)=>{children.steering=props;return <section data-parent-steering-consumer/>;}}));
vi.mock('./TaskConversationReply',()=>({TaskConversationReply:(props:ReplyProps)=>{children.reply=props;return null;}}));
vi.mock('./TaskSubscriptionControl',()=>({TaskSubscriptionControl:()=>null}));
vi.mock('./TaskConversationMentions',()=>({TaskConversationMentions:()=>null,mentionSelectionKey:()=>''}));
vi.mock('./TaskConversationKnowledge',()=>({TaskConversationKnowledge:()=>null,RecordedConversationKnowledge:()=>null}));

const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Team'},projects:[]};
const message=(sequence:number,extra:Partial<TaskConversationMessage>={}):TaskConversationMessage=>({id:'message-'+sequence,sequence,task_version:7,body:'Original saved instruction '+sequence,author_id:'user-a',author_name:'Owner',actor_kind:'human',created_at:'2026-11-01T07:00:00Z',requirement_id:'',requirement_version:0,run_id:'old-run',reply_id:'',reply_to_sequence:0,execution_run_id:'',...extra});
const messages=[message(1),message(2),message(3,{author_id:'other-user'}),message(4,{actor_kind:'api'}),message(5,{actor_kind:'assistant',reply_id:'reply-a',reply_to_sequence:1,execution_run_id:'reply-run'}),message(6,{body:'汉'.repeat(8000)})];
const page:TaskConversationPage={project_id:'project-a',task_id:'task-a',task_version:7,can_post:true,items:messages,total:messages.length,limit:20,next_before:0};
let host:HTMLDivElement,root:Root,writes:number;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();children.steering=undefined;children.reply=undefined;writes=0;host=document.createElement('div');document.body.append(host);root=createRoot(host);vi.stubGlobal('fetch',vi.fn(async(_input:unknown,init?:RequestInit)=>{if(init?.method&&init.method!=='GET')writes++;return Response.json(page);}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function click(selector:string){const element=host.querySelector<HTMLButtonElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
const draft=()=>host.querySelector<HTMLTextAreaElement>('[name=conversation_body]')!;

it('selects only an original own human note and keeps independent steering and reply locks without losing the unsent discussion draft',async()=>{
 await act(async()=>root.render(<ApiViewProvider session={session}><TaskConversation session={session} project="project-a" taskId="task-a" refresh={0} accessBlocked={false}/></ApiViewProvider>));await settle();
 expect(fetch).not.toHaveBeenCalled();await click('[data-conversation-toggle]');
 expect(host.querySelectorAll('[data-conversation-steer]')).toHaveLength(3);expect(host.querySelector('[data-conversation-steer="message-3"]')).toBeNull();expect(host.querySelector('[data-conversation-steer="message-4"]')).toBeNull();expect(host.querySelector('[data-conversation-steer="message-5"]')).toBeNull();
 const fullLength=host.querySelector<HTMLButtonElement>('[data-conversation-steer="message-6"]')!;expect(fullLength.disabled).toBe(false);expect(host.querySelector('[data-conversation-message="message-6"] .task-conversation-message-text')?.textContent).toBe('汉'.repeat(8000));expect(children.steering?.selectedMessage).toBe('');await click('[data-conversation-steer="message-6"]');expect(children.steering?.selectedMessage).toBe('message-6');
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value')!.set!.call(draft(),'A different unsaved thought');draft().dispatchEvent(new Event('input',{bubbles:true}));});
 await click('[data-conversation-steer="message-1"]');expect(children.steering?.selectedMessage).toBe('message-1');expect(draft().value).toBe('A different unsaved thought');expect(writes).toBe(0);
 await act(async()=>children.steering!.onLockChange?.(true));await settle();expect(draft().disabled).toBe(true);expect(children.reply?.disabled).toBe(true);expect(children.steering?.disabled).toBe(false);
 await act(async()=>children.reply!.onLockChange?.(false));await settle();expect(draft().disabled).toBe(true);await click('[data-conversation-steer="message-2"]');expect(children.steering?.selectedMessage).toBe('message-1');
 await act(async()=>host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();expect(writes).toBe(0);expect(draft().value).toBe('A different unsaved thought');
 await act(async()=>children.reply!.onLockChange?.(true));await act(async()=>children.steering!.onLockChange?.(false));await settle();expect(draft().disabled).toBe(true);expect(children.steering?.disabled).toBe(true);
 await act(async()=>children.reply!.onLockChange?.(false));await settle();expect(draft().disabled).toBe(false);await click('[data-conversation-steer="message-2"]');expect(children.steering?.selectedMessage).toBe('message-2');expect(writes).toBe(0);
});
