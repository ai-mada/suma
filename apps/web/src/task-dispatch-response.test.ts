// @vitest-environment jsdom
import {expect,it} from 'vitest';
import type {ApiCaller} from './api';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';

const project='project159';
const request:ToolPendingRequest={path:'/task-dispatch-plans/plan159/start',method:'POST',body:{project_id:project,expected_version:1},idempotency_key:'original-start-operation159'};
const fixed=()=>({model:'claude-sonnet-5',model_source:'administrator_cli_argument',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'});
const plan=()=>({id:'plan159',project_id:project,state:'started',version:2,run_id:'run159',workflow_id:'published159',workflow_version:7,agent_node_id:'develop',claude_execution:fixed(),workflow:{id:'published159',version:7,nodes:[{id:'develop',type:'agent',max_retries:0,config:{claude_execution:fixed()}}]}});
const run=()=>({id:'run159',project_id:project,workflow_id:'managed-run159',workflow_version:1});

it('acknowledges the reserved managed run instead of the original published workflow',async()=>{
 const calls:string[]=[];const read:ApiCaller=async path=>{calls.push(path);return plan() as never;};
 expect(await validateTaskDispatchResponse(run(),request,read,project)).toBe(true);
 expect(calls).toEqual(['/task-dispatch-plans/plan159?project_id='+project]);
});

it('rejects original or foreign workflow identities, versions and fixed-plan bindings',async()=>{
 const read:ApiCaller=async()=>plan() as never;
 for(const value of [{...run(),workflow_id:'published159',workflow_version:7},{...run(),workflow_id:'managed-foreign159'},{...run(),workflow_version:7},{...run(),id:'foreign159'},{...run(),project_id:'foreign159'}])expect(await validateTaskDispatchResponse(value,request,read,project)).toBe(false);
 for(const original of [{...plan(),id:'foreign159'},{...plan(),run_id:'foreign159'},{...plan(),project_id:'foreign159'},{...plan(),state:'ready'},{...plan(),claude_execution:{...fixed(),model:'claude-other-5'}}]){
  const wrong:ApiCaller=async()=>original as never;
  expect(await validateTaskDispatchResponse(run(),request,wrong,project)).toBe(false);
 }
});

it('keeps the same Start body and key when the authorized plan read fails, then recovers that run',async()=>{
 const values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
 let readable=false;const writes:unknown[]=[];
 const send:ApiCaller=async(path,body,method)=>{if(method==='POST'){writes.push(structuredClone(body));return run() as never;}expect(path).toBe('/task-dispatch-plans/plan159?project_id='+project);if(!readable)throw Error('original current-authorized read unavailable');return plan() as never;};
 const action=new ToolWriteRecovery('managed159',storage,send,(value,pending)=>validateTaskDispatchResponse(value,pending,send,project));
 await expect(action.run(request.path,request.body)).rejects.toThrow();
 const pending=action.pending;expect(pending?.path).toBe(request.path);expect(pending?.body).toEqual(request.body);expect(pending?.idempotency_key).toBeTruthy();expect(values.size).toBe(1);
 readable=true;await expect(action.restore()).resolves.toEqual(run());
 expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(action.pending).toBeUndefined();expect(values.size).toBe(0);
});
