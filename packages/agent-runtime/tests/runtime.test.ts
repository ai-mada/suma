import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer, type Server, type RequestListener, type IncomingMessage} from 'node:http';
import {runAgent, type AgentEvent} from '../src/index.js';

test('missing model is explicit and never simulates success',async()=>{
 const events:AgentEvent[]=[];await runAgent({goal:'write a feature'}, {}, e=>events.push(e));
 assert.equal(events.at(-1)?.type,'error');assert.equal(events.at(-1)?.code,'missing_model');assert.ok(!events.some(e=>e.type==='completed'));
});

async function fixture(handler:RequestListener):Promise<{url:string;close:()=>Promise<void>}>{
 const server:Server=createServer(handler);await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const a=server.address();if(!a || typeof a==='string')throw Error('no address');return {url:`http://127.0.0.1:${a.port}`,close:()=>new Promise((r,j)=>server.close(e=>e?j(e):r()))};
}
test('unknown model tool calls are rejected before authenticated dispatch',async()=>{
 let invoked=false;let authorization='';
 const f=await fixture((req,res)=>{if(req.url?.startsWith('/api/v1/agent/tools'))authorization=req.headers.authorization??'';res.setHeader('Content-Type','application/json');if(req.url?.startsWith('/api/v1/agent/tools')){if(req.method==='POST')invoked=true;res.end(JSON.stringify({items:[{name:'project.overview',description:'Overview',parameters:{type:'object',properties:{}}}]}));return;}res.end(JSON.stringify({choices:[{message:{role:'assistant',content:null,tool_calls:[{id:'call1',type:'function',function:{name:'shell.exec',arguments:'{}'}}]}}],usage:{prompt_tokens:4,completion_tokens:2}}));});
 try{const events:AgentEvent[]=[];await runAgent({goal:'inspect'}, {model:'configured',baseURL:f.url,controlURL:f.url,controlToken:'control-secret',projectID:'p'},e=>events.push(e));assert.equal(events.at(-1)?.code,'unknown_tool');assert.equal(invoked,false);assert.equal(authorization,'Bearer control-secret');}finally{await f.close();}
});
test('bounded loop records usage and never invents a completion',async()=>{
 const f=await fixture(async(req,res)=>{res.setHeader('Content-Type','application/json');if(req.method==='GET'&&req.url?.startsWith('/api/v1/agent/tools')){res.end(JSON.stringify({items:[{name:'project.overview',parameters:{type:'object',properties:{}}}]}));return;}if(req.url==='/api/v1/agent/tools/invoke'){res.end(JSON.stringify({items:[]}));return;}const body=await requestBody(req);res.end(JSON.stringify({choices:[{message:{role:'assistant',tool_calls:[{id:'call1',type:'function',function:{name:body.tools[0].function.name,arguments:'{}'}}]}}],usage:{prompt_tokens:7,completion_tokens:3}}));});
 try{const events:AgentEvent[]=[];await runAgent({goal:'inspect',max_steps:2},{model:'configured',baseURL:f.url,controlURL:f.url,controlToken:'control-secret',projectID:'p'},e=>events.push(e));assert.equal(events.at(-1)?.code,'step_limit');assert.equal(events.filter(e=>e.type==='usage').length,2);assert.ok(!events.some(e=>e.type==='completed'));}finally{await f.close();}
});

async function requestBody(req:IncomingMessage):Promise<Record<string,any>>{const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.isBuffer(chunk)?chunk:Buffer.from(chunk));return JSON.parse(Buffer.concat(chunks).toString('utf8'));}

test('provider-safe aliases reverse to canonical tools without normalization collisions',async()=>{
 const names=['project.a','project_a'];const invocations:string[]=[];const providerBodies:Record<string,any>[]=[];
 const f=await fixture(async(req,res)=>{
  res.setHeader('Content-Type','application/json');
  if(req.method==='GET'&&req.url?.startsWith('/api/v1/agent/tools')){res.end(JSON.stringify({items:names.map(name=>({name,parameters:{type:'object',properties:{},additionalProperties:false}}))}));return;}
  const body=await requestBody(req);
  if(req.url==='/api/v1/agent/tools/invoke'){invocations.push(body.tool);res.end(JSON.stringify({queried:body.tool}));return;}
  providerBodies.push(body);
  if(!Array.isArray(body.tools)||body.tools.some((t:any)=>typeof t.function?.name!=='string'||! /^[A-Za-z0-9_-]{1,64}$/.test(t.function.name))){res.statusCode=400;res.end('{"error":"invalid provider tool name"}');return;}
  if(providerBodies.length===1){res.end(JSON.stringify({choices:[{message:{role:'assistant',tool_calls:body.tools.map((t:any,i:number)=>({id:'call-'+i,type:'function',function:{name:t.function.name,arguments:'{}'}}))}}],usage:{prompt_tokens:2,completion_tokens:1}}));return;}
  res.end(JSON.stringify({choices:[{message:{content:'fixture completed'},finish_reason:'stop'}],usage:{prompt_tokens:3,completion_tokens:1}}));
 });
 try{
  const events:AgentEvent[]=[];await runAgent({goal:'inspect alias fixture',request_id:'alias-fixture'},{model:'fixture',baseURL:f.url,controlURL:f.url,controlToken:'fixture-control-token',projectID:'p'},e=>events.push(e));
  assert.equal(events.at(-1)?.type,'completed');assert.deepEqual(invocations,names);assert.equal(providerBodies.length,2);
  const aliases=providerBodies[0].tools.map((t:any)=>t.function.name);assert.equal(new Set(aliases).size,2);aliases.forEach((name:string)=>assert.match(name,/^[A-Za-z0-9_-]{1,64}$/));assert.deepEqual(providerBodies[1].tools.map((t:any)=>t.function.name),aliases);
  const assistant=providerBodies[1].messages.find((m:any)=>Array.isArray(m.tool_calls));assert.deepEqual(assistant.tool_calls.map((c:any)=>c.function.name),aliases);
  assert.deepEqual(events.filter(e=>e.type==='tool_started').map(e=>e.name),names);assert.deepEqual(events.filter(e=>e.type==='tool_completed').map(e=>e.name),names);
  for(const tool of providerBodies[0].tools)assert.deepEqual(Object.keys(tool.function).sort(),['name','parameters']);
 }finally{await f.close();}
});

test('model cannot bypass the provider map by returning a canonical catalog name',async()=>{
 let invoked=false;
 const f=await fixture((req,res)=>{res.setHeader('Content-Type','application/json');if(req.method==='GET'&&req.url?.startsWith('/api/v1/agent/tools')){res.end(JSON.stringify({items:[{name:'project.overview',parameters:{type:'object',properties:{}}}]}));return;}if(req.url==='/api/v1/agent/tools/invoke'){invoked=true;res.end('{}');return;}res.end(JSON.stringify({choices:[{message:{tool_calls:[{id:'canonical-call',type:'function',function:{name:'project.overview',arguments:'{}'}}]}}],usage:{prompt_tokens:1,completion_tokens:1}}));});
 try{const events:AgentEvent[]=[];await runAgent({goal:'inspect',max_steps:1},{model:'fixture',baseURL:f.url,controlURL:f.url,controlToken:'fixture-control-token',projectID:'p'},e=>events.push(e));assert.equal(events.at(-1)?.code,'unknown_tool');assert.equal(invoked,false);}finally{await f.close();}
});

test('duplicate canonical catalog entries fail before a provider request',async()=>{
 let modelCalls=0;
 const f=await fixture((req,res)=>{res.setHeader('Content-Type','application/json');if(req.method==='GET'&&req.url?.startsWith('/api/v1/agent/tools')){res.end(JSON.stringify({items:[{name:'project.overview',parameters:{type:'object',properties:{}}},{name:'project.overview',parameters:{type:'object',properties:{}}}]}));return;}modelCalls++;res.end(JSON.stringify({choices:[{message:{content:'must not be called'},finish_reason:'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));});
 try{const events:AgentEvent[]=[];await runAgent({goal:'inspect'},{model:'fixture',baseURL:f.url,controlURL:f.url,controlToken:'fixture-control-token',projectID:'p'},e=>events.push(e));assert.equal(events.at(-1)?.code,'invalid_tools');assert.equal(modelCalls,0);}finally{await f.close();}
});
