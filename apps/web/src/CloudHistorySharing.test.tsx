// @vitest-environment jsdom
import {createHash,webcrypto} from 'node:crypto';
import {act,useLayoutEffect} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudHistorySharePreview,TaskConversationMessage} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {CloudHistorySharing,type CloudHistoryShareSelection} from './CloudHistorySharing';

const session:Session={user:{id:'user-a',name:'Alice',email:'alice@example.test'},organization:{id:'org-a',name:'Team'},projects:[]};
const sha=(value:string)=>createHash('sha256').update(value).digest('hex');
function initialSelection():CloudHistoryShareSelection{
 const entries=[{sequence:1,kind:'assistant_text',text:'  literal <script>ignored()</script>\n'},{sequence:3,kind:'tool_summary',text:'second & 中文'}];
 return {load:{id:'load-a',version:6,operation_id:'private-load-operation',head_version_at_start:2,source:{project_id:'project-a',run_id:'source-run',node_id:'cloud-node',submission_id:'submission-a',submission_version:4,task_id:'vendor-private-task',node_spec_sha256:'a'.repeat(64),request_sha256:'b'.repeat(64)},state:'loaded',coverage:'unverified',started_at:'2026-10-31T05:00:00Z',deadline_at:'2026-10-31T05:00:30Z',overdue:false,finished_at:'2026-10-31T05:00:00.100Z',entry_count:3,unsupported_count:1,text_bytes:entries.reduce((sum,item)=>sum+new TextEncoder().encode(item.text).length,0)},entries,sequences:[1,3]};
}
function preview():CloudHistorySharePreview{
 const selected=initialSelection(),body=selected.entries.map(entry=>entry.text).join('\n\n'),digests=selected.entries.map(entry=>({sequence:entry.sequence,kind:entry.kind,text_sha256:sha(entry.text!)}));
 return {project_id:'project-a',task_id:'target-task',task_title:'Explicit handling task',task_version:4,load_id:selected.load.id,load_version:selected.load.version,load_state:'loaded',coverage:'unverified',source:selected.load.source,entries:selected.entries,body,body_sha256:sha(body),selection_sha256:sha(JSON.stringify(digests)),preview_sha256:'c'.repeat(64),sharing_scope:'project_task_readers'};
}
function message():TaskConversationMessage{
 const value=preview();return {id:'message-a',sequence:5,task_version:4,body:value.body,author_id:'user-a',author_name:'Alice',actor_kind:'human',created_at:'2026-10-31T05:01:00Z',requirement_id:'',requirement_version:0,run_id:'target-run',reply_id:'',reply_to_sequence:0,execution_run_id:'',cloud_observation:{load_id:value.load_id,load_version:value.load_version,load_state:value.load_state,coverage:'unverified',source_run_id:value.source.run_id,source_node_id:value.source.node_id,source_submission_id:value.source.submission_id,entries:value.entries.map(entry=>({sequence:entry.sequence,kind:entry.kind,text_sha256:sha(entry.text!)})),body_sha256:value.body_sha256,selection_sha256:value.selection_sha256}};
}
let root:Root|undefined,host:HTMLDivElement,selection:CloudHistoryShareSelection|undefined,calls:{path:string;body?:Record<string,unknown>;headers:Headers}[],clear:ReturnType<typeof vi.fn>,commitProbe:(()=>void)|undefined;
type Handler=(path:string,body?:Record<string,unknown>)=>Promise<Response|undefined>;
function transport(handler?:Handler){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
 const path=String(input).replace(/^\/api\/v1/,''),body=options?.body?JSON.parse(String(options.body)):undefined;calls.push({path,body,headers:new Headers(options?.headers)});
 const custom=await handler?.(path,body);if(custom)return custom;
 if(path.startsWith('/task-workspace?'))return Response.json({items:[{id:'target-task',project_id:'project-a',version:4,title:'Explicit handling task'}],task_plans:[{task_id:'target-task',iteration_id:'',plan_version:0}],total:1,as_of:'2026-10-31T05:00:00Z'});
 if(path.endsWith('/share-preview'))return Response.json(preview());
 if(path.endsWith('/shares'))return Response.json(message(),{status:201});
 if(path.startsWith('/task-conversation-messages/'))return Response.json({project_id:'project-a',task_id:'target-task',task_version:9,can_post:true,message:message()});
 return Response.json({error:{code:'not_found',message:'Unavailable'}},{status:404});
 }));}
async function settle(){for(let n=0;n<9;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function CommitProbe(){useLayoutEffect(()=>{commitProbe?.();});return null;}
async function render(account=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={account}><CommitProbe/><CloudHistorySharing project="project-a" runID="source-run" nodeID="cloud-node" selection={selection} onClearSelection={clear}/></ApiViewProvider>);});await settle();}
async function click(action:string){const button=document.querySelector<HTMLButtonElement>('[data-history-share-action="'+action+'"]');expect(button,action).not.toBeNull();await act(async()=>button!.click());await settle();}
async function choose(){const select=host.querySelector<HTMLSelectElement>('[data-history-share-task]')!;await act(async()=>{select.value='target-task';select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function consent(){const input=document.querySelector<HTMLInputElement>('[data-history-share-confirm-scope]')!;await act(async()=>input.click());await settle();}
async function open(){await choose();await click('preview');}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);host=document.createElement('div');document.body.append(host);selection=initialSelection();calls=[];clear=vi.fn();commitProbe=undefined;sessionStorage.clear();transport();});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});

it('requires an explicit authorized platform task and previews literal selected text without formal writes',async()=>{
 await render();expect(host.querySelector<HTMLSelectElement>('[data-history-share-task]')!.value).toBe('');expect(host.querySelector<HTMLButtonElement>('[data-history-share-action="preview"]')!.disabled).toBe(true);expect(calls.filter(call=>call.body)).toHaveLength(0);
 await open();const request=calls.find(call=>call.path.endsWith('/share-preview'))!;expect(request.body).toEqual({project_id:'project-a',task_id:'target-task',expected_task_version:4,expected_load_version:6,sequences:[1,3]});expect(request.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(calls.filter(call=>call.path.endsWith('/shares'))).toHaveLength(0);expect(sessionStorage.length).toBe(0);
 expect(document.querySelector('[data-history-share-preview]')!.textContent).toBe(preview().body);expect(document.querySelector('[data-history-share-preview] script')).toBeNull();expect(document.querySelector<HTMLButtonElement>('[data-history-share-action="confirm"]')!.disabled).toBe(true);
});

it('shares only after audience consent and follows the actual target message before confirming',async()=>{
 await render();await open();await consent();await click('confirm');const command=calls.find(call=>call.path.endsWith('/shares'))!;
 expect(Object.keys(command.body!).sort()).toEqual(['expected_load_version','expected_task_version','idempotency_key','preview_sha256','project_id','sequences','task_id']);expect(calls.some(call=>call.path.startsWith('/task-conversation-messages/message-a?project_id=project-a&task_id=target-task'))).toBe(true);expect(clear).toHaveBeenCalledOnce();expect(sessionStorage.length).toBe(0);
 const link=host.querySelector<HTMLAnchorElement>('a')!;expect(link.search).toContain('panel=workbench-task%3Atarget-task');expect(link.search).toContain('conversation_message=message-a');expect(document.querySelector('[data-history-share-preview]')).toBeNull();
});

it('preserves metadata-only original recovery after source loss and does not request a new preview',async()=>{
 let dropped=false;transport(async path=>{if(path.endsWith('/shares')&&!dropped){dropped=true;throw new TypeError('response lost');}return undefined;});await render();await open();await consent();await click('confirm');
 const original=structuredClone(calls.find(call=>call.path.endsWith('/shares'))!.body),stored=sessionStorage.getItem(sessionStorage.key(0)!)!;expect(stored).not.toContain('literal');expect(stored).not.toContain('vendor-private-task');expect(stored).not.toContain('private-load-operation');
 selection=undefined;await render();expect(document.querySelector('[data-history-share-preview]')).toBeNull();expect(host.querySelector('[data-history-share-action="recover"]')).not.toBeNull();await click('recover');
 expect(calls.filter(call=>call.path.endsWith('/shares')).map(call=>call.body)).toEqual([original,original]);expect(calls.filter(call=>call.path.endsWith('/share-preview'))).toHaveLength(1);expect(calls.some(call=>/workbuddy-connections|history-loads\/load-a\?/.test(call.path))).toBe(false);expect(sessionStorage.length).toBe(0);
});

it('remounts recovery with no private load and keeps another actor from replaying the journal',async()=>{
 transport(async path=>{if(path.endsWith('/shares'))throw new TypeError('lost');return undefined;});await render();await open();await consent();await click('confirm');const original=structuredClone(calls.find(call=>call.path.endsWith('/shares'))!.body);await act(async()=>root!.unmount());root=undefined;selection=undefined;transport();
 await render({...session,user:{...session.user,id:'other-user'}});expect(host.querySelector('[data-history-share-action="recover"]')).toBeNull();await render();await click('recover');expect(calls.filter(call=>call.path.endsWith('/shares')).map(call=>call.body)).toEqual([original,original]);expect(sessionStorage.length).toBe(0);
});

it('retains the original operation when its reply belongs to a different target task',async()=>{
 transport(async path=>path.startsWith('/task-conversation-messages/')?Response.json({project_id:'project-a',task_id:'different-task',task_version:9,can_post:true,message:message()}):undefined);await render();await open();await consent();await click('confirm');expect(clear).not.toHaveBeenCalled();expect(sessionStorage.length).toBe(1);expect(document.querySelector('[data-history-share-action="recover"]')).not.toBeNull();expect(host.querySelector('a')).toBeNull();
});

it('clears private preview on target denial while preserving the original operation for reconciliation',async()=>{
 transport(async path=>path.startsWith('/task-conversation-messages/')?Response.json({error:{code:'forbidden',message:'No access'}},{status:403}):undefined);await render();await open();await consent();await click('confirm');expect(document.querySelector('[data-history-share-preview]')).toBeNull();expect(clear).toHaveBeenCalledOnce();expect(sessionStorage.length).toBe(1);expect(document.querySelector('[data-history-share-action="recover"]')).not.toBeNull();
});

it('requires a fresh preview and fresh consent after a task version conflict',async()=>{
 let first=true;transport(async path=>{if(path.endsWith('/shares')&&first){first=false;return Response.json({error:{code:'workbuddy_cloud_history_share_changed',message:'Changed'}},{status:409});}return undefined;});await render();await open();await consent();await click('confirm');expect(document.querySelector('[data-history-share-preview]')).toBeNull();expect(sessionStorage.length).toBe(0);await click('preview');expect(document.querySelector<HTMLInputElement>('[data-history-share-confirm-scope]')!.checked).toBe(false);await consent();expect(document.querySelector<HTMLButtonElement>('[data-history-share-action="confirm"]')!.disabled).toBe(false);await click('confirm');expect(clear).toHaveBeenCalledOnce();
});

it('does not install a late private preview after the selected source is retired',async()=>{
 let release!:(response:Response)=>void;const held=new Promise<Response>(resolve=>{release=resolve;});transport(async path=>path.endsWith('/share-preview')?held:undefined);await render();await choose();await act(async()=>document.querySelector<HTMLButtonElement>('[data-history-share-action="preview"]')!.click());await settle();selection=undefined;await render();await act(async()=>release(Response.json(preview())));await settle();expect(document.querySelector('[data-history-share-preview]')).toBeNull();expect(calls.filter(call=>call.path.endsWith('/shares'))).toHaveLength(0);expect(sessionStorage.length).toBe(0);
});

it('keeps a malformed recovery journal visible and blocks new writes without erasing the original',async()=>{
 selection=undefined;const key='suma-write-recovery-v1:workbuddy-history-share:'+JSON.stringify(['user-a','org-a','project-a','source-run','cloud-node']);sessionStorage.setItem(key,'original invalid record');await render();expect(host.querySelector('[data-history-sharing]')).not.toBeNull();expect(host.querySelector('[role="alert"]')).not.toBeNull();expect(sessionStorage.getItem(key)).toBe('original invalid record');expect(calls).toHaveLength(0);
});

it('hides retired private preview in the commit before passive cleanup while the operation is pending',async()=>{
 transport(async path=>{if(path.endsWith('/shares'))throw new TypeError('lost');return undefined;});await render();await open();await consent();await click('confirm');expect(document.querySelector('[data-history-share-preview]')).not.toBeNull();const snapshots:string[]=[];commitProbe=()=>snapshots.push(document.querySelector('[data-history-share-preview]')?.textContent||'');selection=undefined;await render();expect(snapshots.length).toBeGreaterThan(0);expect(snapshots.every(value=>value==='')).toBe(true);expect(sessionStorage.length).toBe(1);expect(document.querySelector('[data-history-share-action="recover"]')).not.toBeNull();
});

it('allows closing a private preview while keeping the unknown operation available outside the modal',async()=>{
 transport(async path=>{if(path.endsWith('/shares'))throw new TypeError('lost');return undefined;});await render();await open();await consent();await click('confirm');const original=sessionStorage.getItem(sessionStorage.key(0)!)!;await act(async()=>document.querySelector<HTMLButtonElement>('.dialog-heading button')!.click());await settle();expect(document.querySelector('[data-history-share-preview]')).toBeNull();expect(host.querySelector('[data-history-share-action="recover"]')).not.toBeNull();expect(sessionStorage.getItem(sessionStorage.key(0)!)).toBe(original);expect(calls.filter(call=>call.path.endsWith('/shares'))).toHaveLength(1);
});

it('pages and searches the existing task query without choosing a returned row automatically',async()=>{
 let second=false;transport(async path=>{if(!path.startsWith('/task-workspace?'))return undefined;const url=new URL(path,'http://suma.test');second=url.searchParams.get('cursor')==='next';return Response.json({items:[{id:second?'second-task':'target-task',project_id:'project-a',version:4,title:second?'Other task':'Explicit handling task'}],task_plans:[{task_id:second?'second-task':'target-task',iteration_id:'',plan_version:0}],total:2,as_of:'2026-10-31T05:00:00Z',...(second?{}:{next_cursor:'next'})});});await render();await click('tasks-more');expect(second).toBe(true);expect(host.querySelectorAll('option')).toHaveLength(3);expect(host.querySelector<HTMLSelectElement>('[data-history-share-task]')!.value).toBe('');
 const input=host.querySelector<HTMLInputElement>('input:not([type=checkbox])')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,'search term');input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));});await click('search');await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});await settle();expect(calls.some(call=>call.path.includes('q=search+term'))).toBe(true);expect(calls.filter(call=>call.body)).toHaveLength(0);
});
