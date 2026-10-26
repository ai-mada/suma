import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {runAgent,type AgentEvent,type AgentRequest} from '../src/index.js';

test('planning uses bounded JSON model output and filters the project catalog',async()=>{
 let body:Record<string,any>|undefined;
 const server=createServer(async(req,res)=>{res.setHeader('Content-Type','application/json');if(req.method==='GET'){res.end(JSON.stringify({items:[{name:'project.overview',parameters:{type:'object'}},{name:'task.create',parameters:{type:'object'}}]}));return;}const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));body=JSON.parse(Buffer.concat(chunks).toString());res.end(JSON.stringify({choices:[{message:{content:'{"schema_version":1,"summary":"fixture"}'},finish_reason:'stop'}]}));});
 await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const a=server.address();if(!a||typeof a==='string')throw Error('fixture');const url=`http://127.0.0.1:${a.port}`;
 try{const cli=fileURLToPath(new URL('../src/cli.js',import.meta.url));const result=await new Promise<{code:number|null;stdout:string}>((resolve,reject)=>{const p=spawn(process.execPath,[cli],{env:{...process.env,SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:url,SUMA_CONTROL_URL:url,SUMA_PROJECT_ID:'p',SUMA_AGENT_TOKEN:'fixture-agent',SUMA_MODEL_API_KEY:undefined},windowsHide:true});let stdout='';p.stdout.on('data',b=>stdout+=String(b));p.on('error',reject);p.on('close',code=>resolve({code,stdout}));p.stdin.end(JSON.stringify({goal:'Return a schedule JSON proposal',purpose:'planning',max_steps:1,max_tokens:8192,request_id:'planning-cli-fixture'})+'\n');});const events:AgentEvent[]=result.stdout.trim().split('\n').map(line=>JSON.parse(line));assert.equal(result.code,0);assert.equal(events.at(-1)?.type,'completed');assert.equal(events.at(-1)?.output,'{"schema_version":1,"summary":"fixture"}');assert.equal(body?.max_completion_tokens,8192);assert.deepEqual(body?.response_format,{type:'json_object'});assert.match(body?.messages[0].content,/suma-schedule-plan-v1/);assert.equal(body?.tools.length,1);assert.deepEqual(events[0].tools,['project.overview']);}finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});

test('planning rejects workspace bridge before any request and rejects wider step budgets',async()=>{
 for(const extra of [{workspaceBridge:{version:1}},{max_steps:5}]){const events:AgentEvent[]=[];await runAgent({goal:'plan',purpose:'planning',...(extra.max_steps?{max_steps:extra.max_steps}:{})} as AgentRequest,{model:'fixture',baseURL:'http://127.0.0.1:1',...('workspaceBridge' in extra?{workspaceBridge:extra.workspaceBridge as any}:{})},e=>events.push(e));assert.equal(events.length,1);assert.equal(events[0].type,'error');assert.ok(['invalid_limits','invalid_purpose'].includes(String(events[0].code)));}
});

test('CLI planning probe is offline JSON',async()=>{
 const cli=fileURLToPath(new URL('../src/cli.js',import.meta.url));
 const probe=await new Promise<{code:number|null;out:string}>((resolve,reject)=>{const p=spawn(process.execPath,[cli,'--planning-probe'],{env:{...process.env,SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:'http://127.0.0.1:1/v1'},windowsHide:true});let out='';p.stdout.on('data',b=>out+=String(b));p.on('error',reject);p.on('close',code=>resolve({code,out}));p.stdin.end();});
 assert.equal(probe.code,0);assert.deepEqual(JSON.parse(probe.out),{protocol:'suma-schedule-plan-v1',model:'fixture',endpoint_origin:'http://127.0.0.1:1'});
});
