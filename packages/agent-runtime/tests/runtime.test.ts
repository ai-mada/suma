import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer, type Server, type RequestListener} from 'node:http';
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
 const f=await fixture((req,res)=>{res.setHeader('Content-Type','application/json');if(req.url?.startsWith('/api/v1/agent/tools')){res.end(JSON.stringify({items:[{name:'project.overview',parameters:{type:'object',properties:{}}}]}));return;}res.end(JSON.stringify({choices:[{message:{role:'assistant',tool_calls:[{id:'call1',type:'function',function:{name:'project.overview',arguments:'{}'}}]}}],usage:{prompt_tokens:7,completion_tokens:3}}));});
 try{const events:AgentEvent[]=[];await runAgent({goal:'inspect',max_steps:2},{model:'configured',baseURL:f.url,controlURL:f.url,controlToken:'control-secret',projectID:'p'},e=>events.push(e));assert.equal(events.at(-1)?.code,'step_limit');assert.equal(events.filter(e=>e.type==='usage').length,2);assert.ok(!events.some(e=>e.type==='completed'));}finally{await f.close();}
});
