// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {webcrypto} from 'node:crypto';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiError,type ApiCaller} from './api';
import {useToolAction,type ToolAction} from './tool-hooks';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';

// Keep the real recovery class, hook and synchronous journal. Only transport
// and the receipt validator's existing asynchronous boundary are controlled.
function memory(){const values=new Map<string,string>();return {values,getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};}
function heldValidator(){let entered!:()=>void,release!:(value:boolean)=>void;const started=new Promise<void>(resolve=>{entered=resolve;}),result=new Promise<boolean>(resolve=>{release=resolve;});return {started,release,validate:async(_value:unknown,_request:ToolPendingRequest)=>{entered();return result;}};}
const scopeA='org-a:user-a:project-a:dispatch',scopeB='org-a:user-a:project-b:dispatch',key=(scope:string)=>'suma-tool-recovery-v1:'+scope;
const body={project_id:'project-a',task_id:'task-original',workflow_id:'workflow-a'};
let root:Root|undefined,host:HTMLDivElement,latest:ToolAction;
function Harness({recovery}:{recovery:ToolWriteRecovery}){latest=useToolAction(recovery);return <output>{latest.busy?'busy':latest.pending?'pending':'idle'}</output>;}
beforeEach(()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('crypto',webcrypto);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(recovery:ToolWriteRecovery,owner='stable-pane'){await act(async()=>root!.render(<Harness key={owner} recovery={recovery}/>));}
async function start(validation:ReturnType<typeof heldValidator>,request:Record<string,unknown>=body){let result!:Promise<unknown>;await act(async()=>{result=latest.run('/task-dispatch-plans',request);await validation.started;});return {result};}

it.each(['unmount','replace-scope'] as const)('retains the original journal when the consumer loses ownership during validation: %s',async(change)=>{
 const store=memory(),validation=heldValidator();const send:ApiCaller=async()=>({id:'plan-original'} as never);const original=new ToolWriteRecovery(scopeA,store,send,validation.validate);await render(original);const execution=await start(validation),raw=store.getItem(key(scopeA));expect(raw).not.toBeNull();
 if(change==='unmount')await act(async()=>{root!.unmount();root=undefined;});else await render(new ToolWriteRecovery(scopeB,store,send));
 let ignored:unknown;await act(async()=>{validation.release(true);ignored=await execution.result;});expect(ignored).toBeUndefined();expect(store.getItem(key(scopeA))).toBe(raw);
});

it('cannot let an old validated result remove a newer unknown operation after the same scope remounts',async()=>{
 const store=memory(),validation=heldValidator(),calls:Parameters<ApiCaller>[]=[];
 const oldSend:ApiCaller=async(...args)=>{calls.push(args);return {id:'plan-original'} as never;};const original=new ToolWriteRecovery(scopeA,store,oldSend,validation.validate);await render(original,'old-owner');const oldExecution=await start(validation),originalRaw=store.getItem(key(scopeA));expect(originalRaw).not.toBeNull();
 let unknown=false;const currentSend:ApiCaller=async(...args)=>{calls.push(args);if(unknown)throw new ApiError(0,'network','Original response unconfirmed');return {id:'plan-original'} as never;};const current=new ToolWriteRecovery(scopeA,store,currentSend);await render(current,'current-owner');await act(async()=>{await latest.restore();});expect(calls[1]).toEqual(calls[0]);expect(store.getItem(key(scopeA))).toBeNull();
 unknown=true;await act(async()=>{await latest.run('/task-dispatch-plans',{...body,task_id:'task-new'});});const newerRaw=store.getItem(key(scopeA)),newer=current.pending;expect(newerRaw).not.toBeNull();expect(newerRaw).not.toBe(originalRaw);expect(newer?.body.task_id).toBe('task-new');expect(calls).toHaveLength(3);
 let ignored:unknown;await act(async()=>{validation.release(true);ignored=await oldExecution.result;});expect(ignored).toBeUndefined();expect(current.pending).toEqual(newer);expect(store.getItem(key(scopeA))).toBe(newerRaw);
});


it('does not stage an inactive call or overwrite an occupied slot, including an empty stored string',async()=>{
 let sent=0;const send:ApiCaller=async()=>{sent++;return {id:'owned'} as never;};const raw=JSON.stringify({path:'/task-dispatch-plans',body,method:'POST',idempotency_key:'original-key'});
 const inactive=memory(),recovery=new ToolWriteRecovery(scopeA,inactive,send);await expect(recovery.run('/task-dispatch-plans',body,'POST',()=>false)).rejects.toThrow();expect(inactive.values.size).toBe(0);
 inactive.setItem(key(scopeA),raw);const restored=new ToolWriteRecovery(scopeA,inactive,send);await expect(restored.restore(()=>false)).rejects.toThrow();expect(inactive.getItem(key(scopeA))).toBe(raw);
 for(const occupied of [raw,'']){const store=memory(),fresh=new ToolWriteRecovery(scopeA,store,send);store.setItem(key(scopeA),occupied);await expect(fresh.run('/task-dispatch-plans',body)).rejects.toThrow();expect(store.getItem(key(scopeA))).toBe(occupied);}
 const empty=memory();empty.setItem(key(scopeA),'');await expect(new ToolWriteRecovery(scopeA,empty,send).run('/task-dispatch-plans',body)).rejects.toThrow();expect(empty.getItem(key(scopeA))).toBe('');expect(sent).toBe(0);
});

it('matches hydrated journal bytes exactly and rejects missing or replaced slots before resend and after validation',async()=>{
 const raw=JSON.stringify({idempotency_key:'byte-exact-key',method:'POST',body,path:'/task-dispatch-plans'},null,2),store=memory();store.setItem(key(scopeA),raw);let sent=0;const calls:Parameters<ApiCaller>[]=[];
 const send:ApiCaller=async(...args)=>{sent++;calls.push(args);throw new ApiError(0,'network','unknown');};const recovery=new ToolWriteRecovery(scopeA,store,send);await expect(recovery.restore()).rejects.toThrow('unknown');expect(sent).toBe(1);expect(store.getItem(key(scopeA))).toBe(raw);expect((calls[0][1] as Record<string,unknown>).idempotency_key).toBe('byte-exact-key');
 for(const replacement of [null,'another original operation']){if(replacement===null)store.removeItem(key(scopeA));else store.setItem(key(scopeA),replacement);await expect(recovery.restore()).rejects.toThrow();expect(sent).toBe(1);expect(recovery.pending?.idempotency_key).toBe('byte-exact-key');expect(store.getItem(key(scopeA))).toBe(replacement);}
 store.setItem(key(scopeA),raw);const validation=heldValidator(),ack:ApiCaller=async()=>({id:'owned'} as never),pending=new ToolWriteRecovery(scopeA,store,ack,validation.validate);const result=pending.restore();await validation.started;store.setItem(key(scopeA),'newer untouched bytes');validation.release(true);await expect(result).rejects.toThrow();expect(store.getItem(key(scopeA))).toBe('newer untouched bytes');expect(pending.pending?.idempotency_key).toBe('byte-exact-key');
});

it('clears only a fresh actual send4xx and retains restored refusals or other transport statuses',async()=>{
 for(const status of [400,409,302,503])for(const restoring of [false,true]){
  const store=memory();if(restoring)store.setItem(key(scopeA),JSON.stringify({path:'/task-dispatch-plans',body,method:'POST',idempotency_key:'restore-key'}));const send:ApiCaller=async()=>{throw new ApiError(status,'refusal','known response');};const recovery=new ToolWriteRecovery(scopeA,store,send);
  await expect(restoring?recovery.restore():recovery.run('/task-dispatch-plans',body)).rejects.toThrow();const clears=!restoring&&status>=400&&status<500;expect(recovery.pending===undefined).toBe(clears);expect(store.getItem(key(scopeA))===null).toBe(clears);
 }
});

it.each(['read-after-send','validator','remove'] as const)('retains pending when %s fails, without treating that stage as a send refusal',async(stage)=>{
 const base=memory();let failRead=false,sent=0;const store={getItem:(key:string)=>{if(failRead)throw new ApiError(409,'storage','read failure');return base.getItem(key);},setItem:base.setItem,removeItem:(key:string)=>{if(stage==='remove')throw new ApiError(409,'storage','delete failure');base.removeItem(key);}};
 const send:ApiCaller=async()=>{sent++;if(stage==='read-after-send')failRead=true;return {id:'owned'} as never;};const recovery=new ToolWriteRecovery(scopeA,store,send,()=>{if(stage==='validator')throw new ApiError(409,'validation','validator failure');return true;});
 await expect(recovery.run('/task-dispatch-plans',body)).rejects.toThrow();failRead=false;expect(recovery.pending?.body).toEqual(body);expect(base.getItem(key(scopeA))).not.toBeNull();expect(sent).toBe(1);
});

it('keeps one busy ticket for same-tick calls and consumes its receipt through ordinary rerenders',async()=>{
 const store=memory(),validation=heldValidator();let sent=0;const send:ApiCaller=async()=>{sent++;return {id:'owned'} as never;};const recovery=new ToolWriteRecovery(scopeA,store,send,validation.validate);await render(recovery);const action=latest;let first!:Promise<unknown>,second!:Promise<unknown>;
 await act(async()=>{first=action.run('/task-dispatch-plans',body);second=action.run('/task-dispatch-plans',body);await validation.started;await second;});expect(sent).toBe(1);expect(latest.busy).toBe(true);expect(latest.error).toBe('');await render(recovery);expect(latest.busy).toBe(true);let value:unknown;await act(async()=>{validation.release(true);value=await first;});expect(value).toEqual({id:'owned'});expect(latest.busy).toBe(false);expect(store.getItem(key(scopeA))).toBeNull();
});

it('lets a replacement recovery start independently and never lets the old finally reset its busy ticket',async()=>{
 const store=memory(),oldValidation=heldValidator(),newValidation=heldValidator(),sendA:ApiCaller=async()=>({id:'old'} as never),sendB:ApiCaller=async()=>({id:'current'} as never);const original=new ToolWriteRecovery(scopeA,store,sendA,oldValidation.validate);await render(original);const old=await start(oldValidation),oldRaw=store.getItem(key(scopeA));const current=new ToolWriteRecovery(scopeB,store,sendB,newValidation.validate);await render(current);expect(latest.busy).toBe(false);const active=await start(newValidation,{...body,project_id:'project-b'}),newRaw=store.getItem(key(scopeB));expect(latest.busy).toBe(true);
 await act(async()=>{oldValidation.release(true);await old.result;});expect(latest.busy).toBe(true);expect(store.getItem(key(scopeA))).toBe(oldRaw);expect(store.getItem(key(scopeB))).toBe(newRaw);let value:unknown;await act(async()=>{newValidation.release(true);value=await active.result;});expect(value).toEqual({id:'current'});expect(latest.busy).toBe(false);
});
