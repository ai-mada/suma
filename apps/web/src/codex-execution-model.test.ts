import {it,expect} from 'vitest';
import {ApiError,type ApiCaller} from './api';
import {ToolWriteRecovery} from './tool-write';
import {validCodexCatalog,validCodexSelection,validCodexDispatchReceipt} from './codex-execution-model';
import {parseToolManifest} from './tool-model';
const catalog={protocol:'codex-app-server-v1',configuration_sha256:'a'.repeat(64),catalog_sha256:'b'.repeat(64),models:[{model:'model-a',reasoning_efforts:['high','low'],default_reasoning_effort:'low'}]};
const selection={catalog_sha256:catalog.catalog_sha256,model:'model-a',reasoning_effort:'high',token_notice_limit:40};
const binding={protocol:'codex-app-server-v1',configuration_sha256:catalog.configuration_sha256,catalog_sha256:catalog.catalog_sha256,model:'model-a',reasoning_effort:'high',token_notice_limit:40,budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:'c'.repeat(64)};
function storage(){const map=new Map<string,string>();return {getItem:(key:string)=>map.get(key)??null,setItem:(key:string,value:string)=>{map.set(key,value);},removeItem:(key:string)=>{map.delete(key);}};}
it('imports a closed public catalog and rejects hidden configuration and hard-limit aliases',()=>{
 expect(validCodexCatalog(catalog)).toBe(true);expect(validCodexCatalog({...catalog,api_key:'private'})).toBe(false);expect(validCodexSelection({...selection,token_notice_limit:'40'})).toBe(false);expect(validCodexSelection({...selection,cost_limit_micros:1})).toBe(false);
 const observation={profile:{command_id:'codex',adapter:'codex',protocol:'codex-app-server-v1',profile_sha256:'d'.repeat(64),executable_sha256:'e'.repeat(64),tool_version:'codex-cli unit fixture',read_only:false},available:true,reason:'',codex_catalog:catalog};
 expect(parseToolManifest(JSON.stringify([observation]))[0].codex_catalog).toEqual(catalog);expect(()=>parseToolManifest(JSON.stringify([{...observation,codex_catalog:{...catalog,env:{API_KEY:'private'}}}]))).toThrow();expect(()=>parseToolManifest(JSON.stringify([{...observation,codex_catalog:undefined}]))).toThrow();
});
it('keeps the exact notice and original key after a bad 200 until the matching plan is recovered',async()=>{
 const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3,codex_execution_selection:selection};
 const good={id:'plan-a',project_id:body.project_id,task_id:body.task_id,task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,codex_execution:binding,workflow:{nodes:[{id:'develop',config:{codex_execution:binding}}]}};
 let wrong=true;const calls:unknown[][]=[];const send:ApiCaller=async(...args)=>{calls.push(args);return (wrong?{...good,codex_execution:{...binding,token_notice_limit:1}}:good) as never;};const store=storage(),validate=(value:unknown,request:{body:Record<string,unknown>})=>validCodexDispatchReceipt(value,request.body);
 const first=new ToolWriteRecovery('codex-recovery',store,send,validate);await expect(first.run('/task-dispatch-plans',body)).rejects.toThrow();expect(first.pending?.body).toEqual(body);wrong=false;const second=new ToolWriteRecovery('codex-recovery',store,send,validate);await second.restore();expect(calls[1]).toEqual(calls[0]);expect(second.pending).toBeUndefined();
 for(const bad of [{...selection,api_token:'private'},{...selection,token_notice_limit:{value:40}}])await expect(new ToolWriteRecovery('bad',storage(),send).run('/task-dispatch-plans',{...body,codex_execution_selection:bad})).rejects.toThrow();expect(calls).toHaveLength(2);
});

it('keeps Start intent for invalid or unrelated runs until the reserved run is confirmed',async()=>{
 const {validateTaskDispatchResponse}=await import('./task-dispatch-response');
 const original={id:'plan-a',project_id:'project-a',run_id:'reserved-run',workflow:{id:'derived-flow',version:1}};
 let response:unknown=null,readFails=false;const calls:unknown[][]=[];
 const send:ApiCaller=async(...args)=>{if(args[0].startsWith('/task-dispatch-plans/plan-a?')){if(readFails)throw new ApiError(403,'forbidden','not currently readable');return original as never;}calls.push(args);return response as never;};
 const validate=(value:unknown,request:import('./tool-write').ToolPendingRequest)=>validateTaskDispatchResponse(value,request,send,'project-a');
 const store=storage(),body={project_id:'project-a',expected_version:1};const first=new ToolWriteRecovery('start',store,send,validate);
 await expect(first.run('/task-dispatch-plans/plan-a/start',body)).rejects.toThrow();expect(first.pending).toBeDefined();
 response={id:'unrelated-run',project_id:'project-a',workflow_id:'derived-flow',workflow_version:1};await expect(first.restore()).rejects.toThrow();expect(first.pending).toBeDefined();
 response={id:'reserved-run',project_id:'project-a',workflow_id:'derived-flow',workflow_version:1};readFails=true;await expect(first.restore()).rejects.toThrow();expect(first.pending).toBeDefined();readFails=false;
 const recovered=new ToolWriteRecovery('start',store,send,validate);await recovered.restore();expect(recovered.pending).toBeUndefined();expect(calls.every(call=>JSON.stringify(call)===JSON.stringify(calls[0]))).toBe(true);
});
