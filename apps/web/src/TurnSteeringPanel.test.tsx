// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {SteeringContext,SteeringInstruction} from '../../../packages/contracts/src';
import type {ApiCaller,Session} from './api';
import {ApiViewProvider,bindApiView} from './api-context';
import {setLocale} from './i18n';
import {TurnSteeringPanel,type TurnSteeringPanelProps} from './TurnSteeringPanel';
import {turnSteeringContext,turnSteeringInstruction,turnSteeringMessage,turnSteeringSession} from './turn-steering.test-support';

const callerOverride=vi.hoisted(()=>({current:undefined as ApiCaller|undefined}));
vi.mock('./api-context',async load=>{const actual=await load<typeof import('./api-context')>();return {...actual,useApi:()=>{const current=actual.useApi();return callerOverride.current??current;}};});
const identity:Session={user:{id:'user-a',name:'Person',email:'person@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const initial:TurnSteeringPanelProps={session:identity,project:'project-a',taskID:'task-a',selectedMessage:'message-a',refresh:0,accessBlocked:false};
type Request={path:string;method:string;body?:Record<string,unknown>;headers:Headers};
let host:HTMLDivElement,root:Root,contexts:Map<string,SteeringContext>,requests:Request[],reply:(request:Request)=>Promise<Response>,saved:Map<string,SteeringInstruction>;
const locked=vi.fn(),denied=vi.fn();
const messageB={...turnSteeringMessage,id:'message-b',sequence:13,body:'A separate saved human idea.',body_sha256:'8'.repeat(64)};
const sessionB={...turnSteeringSession,id:'session-b',run_id:'run-b',attempt:3,fence:10,source:{...turnSteeringSession.source,thread_id:'thread-b',turn_id:'turn-b'}};
async function normal(request:Request):Promise<Response>{
 if(request.method==='GET'){const id=new URL(request.path,'https://fixture.test').searchParams.get('message_id')!;return Response.json(contexts.get(id));}
 const body=request.body!,key=String(body.idempotency_key);let result=saved.get(key);
 if(!result){
  const context=contexts.get(String(body.message_id))!,target=context.targets.find(item=>item.id===body.session_id)!;
  result={...turnSteeringInstruction,session_id:target.id,run_id:target.run_id,node_id:target.node_id,attempt:target.attempt,fence:target.fence,message:context.message};saved.set(key,result);
  contexts.set(context.message.id,{...context,can_send:false,reason:'no_live_turn',targets:[{...target,version:target.version+1,state:'closed',allowed_actions:[]}],instructions:[{...result,version:2,observation:{sequence:1,request_id:{kind:'number',value:'81'},params_sha256:'7'.repeat(64),observed:true,write_started:true,write_written:true,native_accepted:true,native_rejected:false,turn_closed:true}}]});
 }
 return Response.json(result);
}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();callerOverride.current=undefined;locked.mockReset();denied.mockReset();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 contexts=new Map([['message-a',structuredClone(turnSteeringContext)],['message-b',{...structuredClone(turnSteeringContext),message:messageB,targets:[sessionB]}]]);requests=[];saved=new Map();reply=normal;
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const body=init?.body?JSON.parse(String(init.body)) as Record<string,unknown>:undefined,request={path:String(input).replace(/^\/api\/v1/,''),method:init?.method||(body?'POST':'GET'),body,headers:new Headers(init?.headers)};requests.push(request);return reply(request);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();callerOverride.current=undefined;vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(props:Partial<TurnSteeringPanelProps>={}){const actual={...initial,...props};await act(async()=>root.render(<ApiViewProvider session={actual.session}><p data-public-task>Original task remains available</p><TurnSteeringPanel {...actual} onLockChange={locked} onAccessDenied={denied}/></ApiViewProvider>));await settle();}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
const writes=()=>requests.filter(item=>item.method==='POST');
function journals(key:string){return Array.from({length:sessionStorage.length},(_,index)=>{const name=sessionStorage.key(index)!,raw=sessionStorage.getItem(name)!;let value:Record<string,any>={};try{value=JSON.parse(raw);}catch{/* Other preferences are not request journals. */}return {name,raw,value};}).filter(item=>item.value.idempotency_key===key||item.value.request?.idempotency_key===key);}

it('T188 sends only an explicitly selected original human message and restores its exact request before returning to another selected message',async()=>{
 await render({selectedMessage:''});expect(requests).toHaveLength(0);await render();
 expect(requests[0].path).toBe('/tasks/task-a/execution-steering?project_id=project-a&message_id=message-a');expect(requests[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(writes()).toHaveLength(0);
 expect(host.querySelector<HTMLInputElement>('[data-turn-steering-target="session-a"]')?.checked).toBe(false);await click('[data-turn-steering-target="session-a"]');
 let lost=true;reply=async request=>{const response=await normal(request);if(request.method==='POST'&&lost){lost=false;throw TypeError('Original create ACK lost after save');}return response;};
 const send=host.querySelector<HTMLButtonElement>('[data-turn-steering-send]')!;await act(async()=>{send.click();send.click();});await settle();
 expect(writes()).toHaveLength(1);const original=writes()[0],key=String(original.body!.idempotency_key);
 expect(original.path).toBe('/task-execution-steering');expect(original.body).toEqual({project_id:'project-a',task_id:'task-a',message_id:'message-a',expected_task_version:7,session_id:'session-a',expected_session_version:3,idempotency_key:key});
 expect(original.headers.get('X-Suma-Expected-Organization')).toBe('org-a');const records=journals(key);expect(records).toHaveLength(2);
 const metadata=records.find(item=>item.value.request)!;expect(metadata.value).toMatchObject({message:{id:'message-a',sequence:12,author_id:'user-a',body_sha256:turnSteeringMessage.body_sha256},session:{attempt:2,fence:9,source:{turn_id:'turn-a'}},request:{body:original.body}});expect(metadata.raw).not.toContain(turnSteeringMessage.body);
 await click('[data-turn-steering-refresh]');expect(journals(key)).toHaveLength(2);expect(writes()).toHaveLength(1);
 await render({selectedMessage:'message-b'});expect(host.textContent).not.toContain(messageB.body);expect(journals(key)).toHaveLength(2);
 await act(async()=>root.unmount());root=createRoot(host);await render({selectedMessage:'message-b',disabled:true});expect(requests.at(-1)?.path).toContain('message_id=message-a');
 const remove=Storage.prototype.removeItem,cleanup=vi.spyOn(Storage.prototype,'removeItem').mockImplementation(function(this:Storage,name:string){if(name.startsWith('suma-write-recovery-v1:'))throw Error('Fixture cleanup failed');return remove.call(this,name);});
 await click('[data-turn-steering-restore]');expect(writes()).toHaveLength(2);expect(writes()[1].body).toEqual(original.body);expect(journals(key)).toHaveLength(2);
 cleanup.mockRestore();await click('[data-turn-steering-restore]');expect(writes()).toHaveLength(2);expect(journals(key)).toHaveLength(0);
 expect(requests.at(-1)?.path).toContain('message_id=message-b');expect(host.textContent).toContain(messageB.body);expect(host.querySelector<HTMLInputElement>('[data-turn-steering-target="session-b"]')?.checked).toBe(false);expect(locked).toHaveBeenLastCalledWith(false);
});

it('T188 ignores a late selected-message or caller response and clears private facts immediately after current denial',async()=>{
 let finish!:(response:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});await render();reply=normal;await render({selectedMessage:'message-b'});
 await act(async()=>finish(Response.json(turnSteeringContext)));await settle();expect(host.textContent).toContain(messageB.body);expect(host.textContent).not.toContain(turnSteeringMessage.body);
 reply=()=>new Promise(resolve=>{finish=resolve;});await click('[data-turn-steering-refresh]');callerOverride.current=bindApiView(identity);reply=normal;await render();
 await act(async()=>finish(Response.json(contexts.get('message-b'))));await settle();expect(host.textContent).toContain(turnSteeringMessage.body);expect(host.textContent).not.toContain(messageB.body);
 await render({disabled:true});expect(host.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);expect(writes()).toHaveLength(0);
 await render();reply=async()=>Response.json({error:{code:'forbidden',message:'Current steering access denied'}},{status:403});await click('[data-turn-steering-refresh]');
 expect(host.textContent).not.toContain(turnSteeringMessage.body);expect(host.querySelector('[data-turn-steering-facts]')).toBeNull();expect(denied).toHaveBeenCalledOnce();expect(host.querySelector('[data-public-task]')).not.toBeNull();expect(writes()).toHaveLength(0);
});

it('T188 does not send unpersisted instructions or settle a mismatched ACK, and preserves the original operation across a parent access boundary',async()=>{
 await render();await click('[data-turn-steering-target="session-a"]');const store=vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw Error('Fixture storage full');});
 await click('[data-turn-steering-send]');expect(writes()).toHaveLength(0);store.mockRestore();await click('[data-turn-steering-refresh]');await click('[data-turn-steering-target="session-a"]');
 reply=async request=>{const response=await normal(request);if(request.method!=='POST')return response;const result=await response.json();return Response.json({...result,actor_id:'another-person'});};
 await click('[data-turn-steering-send]');expect(writes()).toHaveLength(1);const key=String(writes()[0].body!.idempotency_key);expect(journals(key)).toHaveLength(2);expect(locked).toHaveBeenLastCalledWith(true);
 await render({accessBlocked:true,selectedMessage:'message-b'});expect(host.textContent).not.toContain(turnSteeringMessage.body);expect(host.textContent).not.toContain(messageB.body);expect(host.querySelector('[data-turn-steering-restore]')).toBeNull();expect(journals(key)).toHaveLength(2);expect(writes()).toHaveLength(1);
});
