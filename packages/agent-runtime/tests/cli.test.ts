import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {fileURLToPath} from 'node:url';

const cli=fileURLToPath(new URL('../src/cli.js',import.meta.url));
function run(args:string[],env:NodeJS.ProcessEnv,input=''):Promise<{code:number|null;out:string}>{return new Promise((resolve,reject)=>{const p=spawn(process.execPath,[cli,...args],{env:{...process.env,SUMA_MODEL_API_KEY:undefined,...env},stdio:['pipe','pipe','pipe'],windowsHide:true});let out='';p.stdout.on('data',b=>out+=String(b));p.stderr.on('data',b=>out+=String(b));p.on('error',reject);p.on('close',code=>resolve({code,out}));p.stdin.end(input);});}

test('CLI version and help probes never enter the model loop',async()=>{
 const version=await run(['--version'],{SUMA_MODEL:undefined,SUMA_MODEL_BASE_URL:undefined});assert.equal(version.code,0);assert.match(version.out,/suma-agent-runtime 0\.1\.0/);
 const help=await run(['--help'],{});assert.equal(help.code,0);assert.match(help.out,/SUMA_AGENT_TOKEN/);assert.match(help.out,/JSONL/);
});

test('CLI uses a separate agent credential and never the Runner bearer',async()=>{
 let authorization='';const server=createServer((req,res)=>{res.setHeader('Content-Type','application/json');if(req.url?.startsWith('/api/v1/agent/tools')){authorization=req.headers.authorization??'';res.end(JSON.stringify({items:[]}));}else res.end(JSON.stringify({choices:[{message:{content:'actual fixture answer'},finish_reason:'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));});
 await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');const url=`http://127.0.0.1:${address.port}`;
 try{const result=await run([],{SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:url,SUMA_CONTROL_URL:url,SUMA_PROJECT_ID:'p',SUMA_AGENT_TOKEN:'agent-secret',SUMA_RUNNER_TOKEN:'runner-secret'},'{"goal":"inspect","request_id":"fixture"}\n');assert.equal(result.code,0);assert.equal(authorization,'Bearer agent-secret');assert.match(result.out,/completed/);assert.ok(!result.out.includes('agent-secret')&&!result.out.includes('runner-secret'));}finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});
