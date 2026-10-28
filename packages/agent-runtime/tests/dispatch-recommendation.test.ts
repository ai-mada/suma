import {test} from 'node:test';
import assert from 'node:assert/strict';
import {runAgent,type AgentEvent} from '../src/index.js';
import {modelHash,type ModelBinding,type LocalModelExecution} from '../src/model-execution.js';

// No provider or control service is called. This verifies the SDK request seam;
// the Go runtime integration verifies authoritative HTTP and budget receipts.
test('dispatch recommendation sends one frozen gateway request without tools and preserves original closed output',async()=>{
 const deployment={deployment_id:'recommendation-analysis',revision:1,deployment_sha256:'',protocol:'openai-chat-completions-v1',model:'analysis-fixture',endpoint_origin:'https://supplier.invalid',endpoint_config_sha256:modelHash('https://supplier.invalid/v1'),context_window:65536};deployment.deployment_sha256=modelHash(deployment);
 const parameters={max_steps:1,max_output_tokens:8192};const selected:ModelBinding={protocol:'suma-model-execution-v1',deployment,parameters,parameters_sha256:modelHash(parameters),binding_sha256:'',input_token_reservation:65536,output_token_reservation:8192};selected.binding_sha256=modelHash(selected);
 const local:LocalModelExecution={binding:selected,gateway:{protocol:'suma-model-gateway-v1',base_url:'http://127.0.0.1:4311/v1',token:'temporary-gateway-only-0123456789',request_id:'recommendation-request',binding_sha256:selected.binding_sha256}};
 const output=JSON.stringify({schema_version:1,summary:'当前候选建议',rationale:'Only the frozen source is available.',recommendations:[{candidate_id:'a'.repeat(64),max_steps:8,max_output_tokens:4096,reasoning_effort:null,reason:'Fits the current task and approved model capability.'}],questions:[]});
 const originalFetch=globalThis.fetch;const requests:Record<string,unknown>[]=[];
 globalThis.fetch=(async(url,init)=>{assert.equal(String(url),'http://127.0.0.1:4311/v1/chat/completions');const headers=new Headers(init?.headers);assert.equal(headers.get('Authorization'),'Bearer '+local.gateway.token);assert.equal(headers.get('X-Suma-Model-Step'),'0');requests.push(JSON.parse(String(init?.body)));return new Response(JSON.stringify({id:'owned-response',model:deployment.model,choices:[{message:{content:output},finish_reason:'stop'}],usage:{prompt_tokens:7,completion_tokens:3}}),{headers:{'Content-Type':'application/json'}});}) as typeof fetch;
 try{
  const events:AgentEvent[]=[];
  await runAgent({request_id:local.gateway.request_id,purpose:'dispatch_recommendation',goal:'x'.repeat(64000),model_execution:selected},{modelExecution:local,controlURL:'https://must-not-contact.invalid',apiKey:'unused-ambient-provider-key'},event=>events.push(event));
  assert.equal(requests.length,1);assert.equal(requests[0].model,deployment.model);assert.equal(requests[0].max_completion_tokens,8192);assert.equal(requests[0].tool_choice,'none');assert.ok(!Object.hasOwn(requests[0],'tools'));assert.deepEqual(requests[0].response_format,{type:'json_object'});
  assert.deepEqual(events[0].tools,[]);assert.equal(events.at(-1)?.type,'completed');assert.equal(events.at(-1)?.output,output);assert.equal(events.at(-1)?.binding_sha256,selected.binding_sha256);assert.ok(!events.some(event=>event.type.startsWith('tool_')));
  const invalid:AgentEvent[]=[];await runAgent({request_id:local.gateway.request_id,purpose:'dispatch_recommendation',goal:'x'.repeat(64001),model_execution:selected},{modelExecution:local},event=>invalid.push(event));assert.equal(invalid.at(-1)?.code,'invalid_goal');assert.equal(requests.length,1);
  const unbound:AgentEvent[]=[];await runAgent({purpose:'dispatch_recommendation',goal:'must remain host-bound'},{model:'ambient',baseURL:'https://must-not-contact.invalid'},event=>unbound.push(event));assert.equal(unbound.at(-1)?.code,'model_binding_invalid');assert.equal(requests.length,1);
 }finally{globalThis.fetch=originalFetch;}
});
