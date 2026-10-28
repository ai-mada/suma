import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer,type IncomingMessage} from 'node:http';
import {runAgent,type AgentEvent,type RuntimeConfig} from '../src/index.js';
import {modelHash,type ModelBinding,type LocalModelExecution} from '../src/model-execution.js';

function binding():ModelBinding {
 const deployment={deployment_id:'model-a',revision:1,deployment_sha256:'',protocol:'openai-chat-completions-v1',model:'fixture-a',endpoint_origin:'https://provider.example',endpoint_config_sha256:modelHash('https://provider.example/v1'),context_window:8192};deployment.deployment_sha256=modelHash(deployment);
 const parameters={max_steps:2,max_output_tokens:128};const selected={protocol:'suma-model-execution-v1',deployment,parameters,parameters_sha256:modelHash(parameters),binding_sha256:'',input_token_reservation:16384,output_token_reservation:128};selected.binding_sha256=modelHash(selected);return selected;
}
function local(binding:ModelBinding,endpoint:string,request='bound-request'):LocalModelExecution {
 return {binding,gateway:{protocol:'suma-model-gateway-v1',base_url:endpoint,token:'attempt-token-only-0123456789abcdef',request_id:request,binding_sha256:binding.binding_sha256}};
}
async function fixture(status=200){
 const requests:Record<string,any>[]=[];const auth:string[]=[];const steps:string[]=[];
 const server=createServer(async(req:IncomingMessage,res)=>{const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));requests.push(JSON.parse(Buffer.concat(chunks).toString('utf8')));auth.push(req.headers.authorization??'');steps.push(String(req.headers['x-suma-model-step']??''));res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(status===200?JSON.stringify({id:'gateway-response',model:'fixture-a',choices:[{message:{content:'bound gateway response'},finish_reason:'stop'}],usage:{prompt_tokens:7,completion_tokens:3}}):'{"error":"fixed gateway rejection"}');});
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('fixture address');const endpoint=`http://127.0.0.1:${address.port}/v1`;
 return {requests,auth,steps,endpoint,close:()=>new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()))};
}

test('gateway binding drives the actual request with a step and only a temporary token',async()=>{
 const f=await fixture();try{const selected=binding();const events:AgentEvent[]=[];const config:RuntimeConfig={model:'ambient-override',baseURL:'https://unused.invalid',modelExecution:local(selected,f.endpoint)};await runAgent({goal:'use the selected model',request_id:'bound-request',model_execution:selected},config,event=>events.push(event));assert.equal(events.at(-1)?.type,'completed');assert.equal(f.requests.length,1);assert.equal(f.requests[0].model,'fixture-a');assert.equal(f.requests[0].max_completion_tokens,128);assert.deepEqual(f.auth,['Bearer attempt-token-only-0123456789abcdef']);assert.deepEqual(f.steps,['0']);assert.equal(events.at(-1)?.binding_sha256,selected.binding_sha256);}finally{await f.close();}
});

test('gateway rejects missing capability, old supplier config and binding or request drift before effects',async()=>{
 const f=await fixture();try{const selected=binding();const valid=local(selected,f.endpoint);const changed=structuredClone(valid);changed.gateway.request_id='foreign-request';const wrongBinding=structuredClone(valid);wrongBinding.gateway.binding_sha256='b'.repeat(64);const foreign=structuredClone(valid);foreign.gateway.base_url='https://provider.example/v1';const cases:unknown[]=[undefined,{binding:selected,base_url:f.endpoint,api_key:'old-supplier-secret'},changed,wrongBinding,foreign];for(const config of cases){const events:AgentEvent[]=[];await runAgent({goal:'must reject',request_id:'bound-request',model_execution:selected},{model:'ambient',baseURL:f.endpoint,modelExecution:config as LocalModelExecution},event=>events.push(event));assert.equal(events.at(-1)?.code,'model_binding_invalid');}assert.equal(f.requests.length,0);}finally{await f.close();}
});

test('gateway HTTP rejection has one attempt and no automatic retry permission',async()=>{
 const f=await fixture(429);try{const selected=binding();const events:AgentEvent[]=[];await runAgent({goal:'preserve the original request',request_id:'bound-request',model_execution:selected},{modelExecution:local(selected,f.endpoint)},event=>events.push(event));assert.equal(f.requests.length,1);assert.deepEqual(f.steps,['0']);assert.equal(events.at(-1)?.code,'upstream_rejected');assert.ok(!Object.hasOwn(events.at(-1)!,'failure_class'));}finally{await f.close();}
});

test('workspace coding cannot fall back to an unbound supplier configuration',async()=>{
 const f=await fixture();try{for(const repair of [false,true]){const events:AgentEvent[]=[];await runAgent({goal:'must reject unbound coding',request_id:'bound-request'},{model:'ambient-supplier',baseURL:f.endpoint,apiKey:'ambient-secret',workspaceBridge:{version:1,endpoint:f.endpoint,token:'workspace-token-0123456789',policy_sha256:'a'.repeat(64),request_id:'bound-request',...(repair?{conflict_repair_protocol:'suma.candidate-conflict-repair.v1' as const}:{})}},event=>events.push(event));assert.equal(events.at(-1)?.code,'model_binding_invalid');}assert.equal(f.requests.length,0);}finally{await f.close();}
});

test('bound execution accepts 17 steps and 131072 output tokens in the actual request',async()=>{
 const selected=binding();selected.deployment.context_window=262144;selected.deployment.deployment_sha256=modelHash({...selected.deployment,deployment_sha256:''});selected.parameters={max_steps:17,max_output_tokens:131072};selected.parameters_sha256=modelHash(selected.parameters);selected.input_token_reservation=262144*17;selected.output_token_reservation=131072;selected.binding_sha256=modelHash({...selected,binding_sha256:''});
 const previous=globalThis.fetch;const sent:Record<string,unknown>[]=[];const events:AgentEvent[]=[];
 globalThis.fetch=(async (url,init)=>{assert.equal(String(url),'http://127.0.0.1:4311/v1/chat/completions');assert.equal(new Headers(init?.headers).get('X-Suma-Model-Step'),'0');sent.push(JSON.parse(String(init?.body)));return new Response(JSON.stringify({id:'parameter-response',model:'fixture-a',choices:[{message:{content:'parameter boundary applied'},finish_reason:'stop'}],usage:{prompt_tokens:7,completion_tokens:3}}),{status:200,headers:{'Content-Type':'application/json'}});}) as typeof fetch;
 try{await runAgent({goal:'verify approved parameter boundary',request_id:'bound-request',model_execution:selected},{modelExecution:local(selected,'http://127.0.0.1:4311/v1')},event=>events.push(event));assert.equal(events.at(-1)?.type,'completed');assert.equal(sent.length,1);assert.equal(sent[0].max_completion_tokens,131072);}finally{globalThis.fetch=previous;}
});
