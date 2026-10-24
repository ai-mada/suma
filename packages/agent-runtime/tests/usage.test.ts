import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer,type IncomingMessage,type ServerResponse} from 'node:http';
import {runAgent,type AgentEvent,type AgentRequest} from '../src/index.js';

async function body(req:IncomingMessage):Promise<Record<string,any>>{const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));return JSON.parse(Buffer.concat(chunks).toString('utf8'));}
async function modelFixture(reply:(request:Record<string,any>,index:number)=>Record<string,unknown>|Promise<Record<string,unknown>>,control=false){
 const requests:Record<string,any>[]=[];const invocations:Record<string,any>[]=[];let catalogCalls=0;
 const server=createServer(async(req:IncomingMessage,res:ServerResponse)=>{res.setHeader('Content-Type','application/json');if(req.method==='GET'){catalogCalls++;res.end(JSON.stringify({items:[{name:'project.tasks',parameters:{type:'object',properties:{}}}]}));return;}const value=await body(req);if(req.url==='/api/v1/agent/tools/invoke'){invocations.push(value);res.end(JSON.stringify({items:[]}));return;}requests.push(value);res.end(JSON.stringify(await reply(value,requests.length-1)));});
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const addr=server.address();if(!addr||typeof addr==='string')throw Error('fixture address');const url=`http://127.0.0.1:${addr.port}`;
 return {requests,invocations,get catalogCalls(){return catalogCalls;},config:{model:'usage-fixture-no-vendor',baseURL:url,...(control?{controlURL:url,controlToken:'fixture-agent-token',projectID:'p'}:{})},close:()=>new Promise<void>((resolve,reject)=>server.close(e=>e?reject(e):resolve()))};
}
const completed=(usage?:unknown)=>({choices:[{message:{content:'local HTTP fixture completed'},finish_reason:'stop'}],...(usage!==undefined?{usage}:{})});
const call=(request:Record<string,any>,usage?:unknown)=>({choices:[{message:{tool_calls:[{id:'fixture-call',type:'function',function:{name:request.tools[0].function.name,arguments:'{}'}}]},finish_reason:'tool_calls'}],...(usage!==undefined?{usage}:{})});

test('explicit invalid process limits fail before any model or catalog HTTP request',async()=>{
 for(const [field,values] of Object.entries({max_steps:[0,-1,1.5,17,null,'2',NaN,Infinity],max_tokens:[0,-1,1.5,32769,null,'2',NaN,Infinity]})){
  const f=await modelFixture(()=>completed(),true);
  try{for(const value of values){const events:AgentEvent[]=[];await runAgent({goal:'validate local fixture',[field]:value} as AgentRequest,f.config,e=>events.push(e));assert.equal(events.at(-1)?.type,'error');assert.equal(events.at(-1)?.code,'invalid_limits');assert.equal(f.catalogCalls,0);assert.equal(f.requests.length,0);}}
  finally{await f.close();}
 }
});

test('missing and empty provider usage remain unknown and never become reported zeros',async()=>{
 for(const usage of [undefined,{},null]){const f=await modelFixture(()=>completed(usage));try{const events:AgentEvent[]=[];await runAgent({goal:'unknown usage'},f.config,e=>events.push(e));const event=events.find(e=>e.type==='usage');assert.ok(event);assert.equal(event.usage_reported,false);assert.equal(event.usage_state,'unreported');assert.ok(!('input_tokens' in event));assert.ok(!('output_tokens' in event));assert.equal(events.at(-1)?.usage_state,'unreported');assert.ok(!('output_tokens' in events.at(-1)!));}finally{await f.close();}}
});

test('known zero counts and optional split zero survive exactly as reported',async()=>{
 const f=await modelFixture(()=>completed({prompt_tokens:0,completion_tokens:0,prompt_tokens_details:{cached_tokens:0},completion_tokens_details:{reasoning_tokens:0}}));
 try{const events:AgentEvent[]=[];await runAgent({goal:'zero usage'},f.config,e=>events.push(e));const event=events.find(e=>e.type==='usage');assert.equal(event?.usage_reported,true);assert.equal(event?.usage_state,'complete');assert.equal(event?.input_tokens,0);assert.equal(event?.output_tokens,0);assert.equal(event?.cached_input_tokens,0);assert.equal(event?.reasoning_output_tokens,0);assert.equal(events.at(-1)?.output_tokens,0);assert.equal(events.at(-1)?.usage_state,'complete');}
 finally{await f.close();}
});

test('every present provider counter is a nonnegative safe integer before tool actions',async()=>{
 for(const value of [-1,1.5,'1',null,true,9007199254740992]){
  for(const field of ['prompt_tokens','completion_tokens','cached_tokens','reasoning_tokens','cache_write_input_tokens']){
   const usage:Record<string,unknown>={prompt_tokens:2,completion_tokens:1};if(field==='cached_tokens')usage.prompt_tokens_details={cached_tokens:value};else if(field==='reasoning_tokens')usage.completion_tokens_details={reasoning_tokens:value};else usage[field]=value;
   const f=await modelFixture(request=>call(request,usage),true);
   try{const events:AgentEvent[]=[];await runAgent({goal:'invalid usage',max_steps:1},f.config,e=>events.push(e));assert.equal(events.at(-1)?.code,'invalid_response',field+':'+String(value));assert.equal(f.invocations.length,0);assert.ok(!events.some(e=>e.type==='tool_started'));}
   finally{await f.close();}
  }
 }
});

test('unknown usage consumes reserved allowance across actual model requests',async()=>{
 const f=await modelFixture(request=>call(request),true);
 try{const events:AgentEvent[]=[];await runAgent({goal:'unknown multi-step usage',max_steps:8,max_tokens:5000},f.config,e=>events.push(e));assert.equal(events.at(-1)?.code,'token_limit');assert.deepEqual(f.requests.map(r=>r.max_completion_tokens),[4096,904]);assert.equal(f.invocations.length,2);const starts=events.filter(e=>e.type==='request_started');assert.deepEqual(starts.map(e=>e.output_allowance),[4096,904]);assert.equal(events.filter(e=>e.type==='usage').length,2);}
 finally{await f.close();}
});

test('partial provider usage preserves known input but reserves full unknown output allowance',async()=>{
 const f=await modelFixture(request=>call(request,{prompt_tokens:3}),true);
 try{const events:AgentEvent[]=[];await runAgent({goal:'partial request usage',max_tokens:5,max_steps:8},f.config,e=>events.push(e));assert.equal(f.requests.length,1);assert.equal(events.at(-1)?.code,'token_limit');const event=events.find(e=>e.type==='usage');assert.equal(event?.input_tokens,3);assert.ok(!('output_tokens' in event!));assert.equal(event?.usage_state,'partial');}
 finally{await f.close();}
});

test('a provider exceeding this request allowance records actual usage then blocks tool dispatch',async()=>{
 const f=await modelFixture(request=>call(request,{prompt_tokens:7,completion_tokens:11}),true);
 try{const events:AgentEvent[]=[];await runAgent({goal:'excess provider result',max_tokens:10},f.config,e=>events.push(e));assert.equal(f.requests[0]?.max_completion_tokens,10);assert.equal(events.find(e=>e.type==='usage')?.output_tokens,11);assert.equal(events.at(-1)?.code,'provider_allowance_exceeded');assert.equal(f.invocations.length,0);}
 finally{await f.close();}
});

test('known actual output releases unused reservation while missing later usage keeps its allowance',async()=>{
 const f=await modelFixture((request,index)=>index===0?call(request,{prompt_tokens:3,completion_tokens:2}):call(request),true);
 try{const events:AgentEvent[]=[];await runAgent({goal:'mixed coverage',max_tokens:10,max_steps:8},f.config,e=>events.push(e));assert.deepEqual(f.requests.map(r=>r.max_completion_tokens),[10,8]);assert.equal(events.at(-1)?.code,'token_limit');assert.equal(events.filter(e=>e.type==='usage')[1]?.usage_state,'unreported');}
 finally{await f.close();}
});

test('abort during the actual second HTTP request preserves an explicit unreported step',async()=>{
 const abort=new AbortController();const f=await modelFixture((request,index)=>{if(index===0)return call(request,{prompt_tokens:2,completion_tokens:0});abort.abort();return new Promise<Record<string,unknown>>(()=>{});},true);
 try{const events:AgentEvent[]=[];await runAgent({goal:'second actual HTTP request abort',max_tokens:10},f.config,e=>events.push(e),abort.signal);assert.equal(f.requests.length,2);assert.equal(events.at(-1)?.code,'aborted');assert.deepEqual(events.filter(e=>e.type==='usage').map(e=>e.usage_state),['complete','unreported']);assert.equal(events.filter(e=>e.type==='request_started').length,2);assert.ok(!events.some(e=>e.type==='completed'));}
 finally{await f.close();}
});

test('only an actual rejected model HTTP429 carries an explicit transient failure classification',async()=>{
 const server=createServer((_req,res)=>{res.statusCode=429;res.end('{"error":"local model fixture rate limit"}');});await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const addr=server.address();if(!addr||typeof addr==='string')throw Error('fixture address');
 try{const events:AgentEvent[]=[];await runAgent({goal:'known rejection'},{model:'rejection-fixture',baseURL:`http://127.0.0.1:${addr.port}`},e=>events.push(e));assert.equal(events.at(-1)?.code,'upstream_rejected');assert.equal(events.at(-1)?.http_status,429);assert.equal(events.at(-1)?.failure_class,'transient');assert.equal(events.at(-1)?.source,'model');assert.equal(events.find(e=>e.type==='usage')?.usage_state,'unreported');}
 finally{await new Promise<void>((resolve,reject)=>server.close(e=>e?reject(e):resolve()));}
});

test('raw provider numeric lexemes cannot round fractional usage into a safe integer',async()=>{
 for(const [literal,valid] of [['0',true],['9007199254740991',true],['9007199254740991.1',false],['1.5',false],['"1"',false],['null',false],['9007199254740992',false]] as const){
  const server=createServer((_req,res)=>{res.setHeader('Content-Type','application/json');res.end('{"choices":[{"message":{"content":"raw numeric HTTP fixture"},"finish_reason":"stop"}],"usage":{"prompt_tokens":'+literal+',"completion_tokens":0}}');});
  await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const addr=server.address();if(!addr||typeof addr==='string')throw Error('fixture address');
  try{const events:AgentEvent[]=[];await runAgent({goal:'raw number fixture'},{model:'raw-usage-no-vendor',baseURL:`http://127.0.0.1:${addr.port}`},e=>events.push(e));if(valid){assert.equal(events.at(-1)?.type,'completed',literal);assert.equal(events.find(e=>e.type==='usage')?.input_tokens,Number(literal));}else{assert.equal(events.at(-1)?.code,'invalid_response',literal);assert.ok(!events.some(e=>e.type==='completed'),literal);}}
  finally{await new Promise<void>((resolve,reject)=>server.close(e=>e?reject(e):resolve()));}
 }
});

test('missing JSON primitive source support fails closed only for reported counters',async()=>{
 const original=JSON.parse;
 JSON.parse=((text:string,reviver?: (this:any,key:string,value:any)=>any)=>original(text,reviver?function(this:any,key:string,value:any){return reviver.call(this,key,value);}:undefined)) as typeof JSON.parse;
 try{for(const usage of [undefined,{prompt_tokens:0,completion_tokens:0}]){const f=await modelFixture(()=>completed(usage));try{const events:AgentEvent[]=[];await runAgent({goal:'runtime source capability fixture'},f.config,e=>events.push(e));if(usage===undefined){assert.equal(events.at(-1)?.type,'completed');assert.equal(events.at(-1)?.usage_state,'unreported');}else{assert.equal(events.at(-1)?.code,'lossless_usage_unavailable');assert.equal(events.find(e=>e.type==='usage')?.usage_state,'unreported');assert.ok(!events.some(e=>e.type==='completed'));}}finally{await f.close();}}}
 finally{JSON.parse=original;}
});
