import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {createServer,type IncomingMessage} from 'node:http';
import {runAgent,type AgentEvent,type AgentRequest,type RuntimeConfig} from '../src/index.js';

const hash=(value:unknown)=>createHash('sha256').update(typeof value==='string'?value:JSON.stringify(value)).digest('hex');
function binding(endpoint:string,model='fixture-a'){
 const deployment={deployment_id:'model-a',revision:1,deployment_sha256:'',protocol:'openai-chat-completions-v1',model,endpoint_origin:new URL(endpoint).origin,endpoint_config_sha256:hash(endpoint),context_window:8192};deployment.deployment_sha256=hash(deployment);
 const parameters={max_steps:2,max_output_tokens:128};const binding={protocol:'suma-model-execution-v1',deployment,parameters,parameters_sha256:hash(parameters),binding_sha256:'',input_token_reservation:16384,output_token_reservation:128};binding.binding_sha256=hash(binding);return binding;
}
async function fixture(reply?:(request:Record<string,any>)=>Record<string,unknown>){
 const requests:Record<string,any>[]=[];const auth:string[]=[];
 const server=createServer(async(req:IncomingMessage,res)=>{const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));const value=JSON.parse(Buffer.concat(chunks).toString('utf8'));requests.push(value);auth.push(req.headers.authorization??'');res.setHeader('Content-Type','application/json');res.end(JSON.stringify(reply?reply(value):{id:'fixture-response',model:'fixture-a',choices:[{message:{content:'bound protocol response'},finish_reason:'stop'}],usage:{prompt_tokens:7,completion_tokens:3}}));});
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('fixture address');const endpoint=`http://127.0.0.1:${address.port}/v1`;
 return {requests,auth,endpoint,close:()=>new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()))};
}
test('a bound model cannot use ambient model configuration without a local deployment',async()=>{
 const f=await fixture();try{const events:AgentEvent[]=[];await runAgent({goal:'must reject',model_execution:binding(f.endpoint)} as AgentRequest,{model:'ambient-override',baseURL:f.endpoint},event=>events.push(event));assert.equal(f.requests.length,0);assert.equal(events.at(-1)?.code,'model_binding_invalid');}finally{await f.close();}
});

test('the exact local binding drives the actual provider model endpoint parameters and receipt',async()=>{
 const f=await fixture();try{const selected=binding(f.endpoint);const events:AgentEvent[]=[];const config={model:'ambient-override',baseURL:'https://unused.invalid',modelExecution:{binding:selected,base_url:f.endpoint,api_key:'fixture-only-key'}} as RuntimeConfig;await runAgent({goal:'use the selected model',request_id:'bound-request',max_steps:2,max_tokens:128,model_execution:selected} as AgentRequest,config,event=>events.push(event));assert.equal(events.at(-1)?.type,'completed');assert.equal(f.requests.length,1);assert.equal(f.requests[0].model,'fixture-a');assert.equal(f.requests[0].max_completion_tokens,128);assert.deepEqual(f.auth,['Bearer fixture-only-key']);for(const event of events)assert.equal(event.binding_sha256,selected.binding_sha256);const request=events.find(event=>event.type==='request_started');assert.equal(request?.parameters_sha256,selected.parameters_sha256);assert.match(String(request?.request_sha256),/^[a-f0-9]{64}$/);const usage=events.find(event=>event.type==='usage');assert.equal(usage?.provider_model,'fixture-a');assert.equal(usage?.provider_request_id,'fixture-response');assert.match(String(usage?.response_sha256),/^[a-f0-9]{64}$/);assert.equal(usage?.input_tokens,7);assert.equal(usage?.output_tokens,3);}finally{await f.close();}
});

test('foreign or absent provider model preserves exact usage and never completes or retries',async()=>{
 for(const providerModel of ['foreign-alias',undefined]){const f=await fixture(()=>({id:'fixture-response',...(providerModel!==undefined?{model:providerModel}:{}),choices:[{message:{content:'cannot certify this model'},finish_reason:'stop'}],usage:{prompt_tokens:7,completion_tokens:3}}));try{const selected=binding(f.endpoint);const events:AgentEvent[]=[];await runAgent({goal:'reconcile provider identity',model_execution:selected} as AgentRequest,{modelExecution:{binding:selected,base_url:f.endpoint,api_key:'fixture-only-key'}} as RuntimeConfig,event=>events.push(event));assert.equal(f.requests.length,1);assert.equal(events.at(-1)?.code,'provider_model_mismatch');assert.ok(!events.some(event=>event.type==='completed'));const usage=events.find(event=>event.type==='usage');assert.equal(usage?.input_tokens,7);assert.equal(usage?.output_tokens,3);assert.equal(usage?.usage_state,'complete');assert.equal(usage?.provider_model,providerModel??'');}finally{await f.close();}}
});

test('binding endpoint revision parameters and cost cap drift fail before model effects',async()=>{
 const f=await fixture();try{const selected=binding(f.endpoint);const original={binding:selected,base_url:f.endpoint,api_key:'fixture-only-key'};for(const change of ['endpoint','deployment','parameters','cost']){const local=structuredClone(original);let request=structuredClone(selected);if(change==='endpoint')local.base_url+='/other-api';if(change==='deployment')request.deployment.revision++;if(change==='parameters')(request.parameters as unknown as Record<string,unknown>).temperature=0.1;if(change==='cost'){(request as unknown as Record<string,unknown>).budget={cost_limit_micros:0,currency:'CNY'};}const events:AgentEvent[]=[];await runAgent({goal:'tampered fixed binding',model_execution:request} as AgentRequest,{modelExecution:local} as RuntimeConfig,event=>events.push(event));assert.equal(events.at(-1)?.code,'model_binding_invalid',change);assert.equal(f.requests.length,0,change);}}finally{await f.close();}
});

test('a bound HTTP429 retains unknown usage without retry permission',async()=>{
 let calls=0;const server=createServer((_req,res)=>{calls++;res.statusCode=429;res.end('{"error":"owned protocol rejection"}');});await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('fixture address');const endpoint=`http://127.0.0.1:${address.port}/v1`;
 try{const selected=binding(endpoint);const events:AgentEvent[]=[];await runAgent({goal:'preserve the original request',model_execution:selected},{modelExecution:{binding:selected,base_url:endpoint,api_key:'fixture-only-key'}},event=>events.push(event));assert.equal(calls,1);assert.equal(events.at(-1)?.code,'upstream_rejected');assert.ok(!Object.hasOwn(events.at(-1)!,'failure_class'));assert.equal(events.find(event=>event.type==='usage')?.usage_state,'unreported');}finally{await new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()));}
});
