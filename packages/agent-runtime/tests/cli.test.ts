import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {fileURLToPath} from 'node:url';

const cli=fileURLToPath(new URL('../src/cli.js',import.meta.url));
const catalogCLI=fileURLToPath(new URL('../src/catalog-cli.js',import.meta.url));
function detailed(args:string[],env:NodeJS.ProcessEnv,input='',file=cli):Promise<{code:number|null;stdout:string;stderr:string}>{return new Promise((resolve,reject)=>{const p=spawn(process.execPath,[file,...args],{env:{...process.env,SUMA_LANGUAGE:undefined,LC_ALL:undefined,LC_MESSAGES:undefined,LANG:undefined,SUMA_MODEL_API_KEY:undefined,...env},stdio:['pipe','pipe','pipe'],windowsHide:true});let stdout='',stderr='';p.stdout.on('data',b=>stdout+=String(b));p.stderr.on('data',b=>stderr+=String(b));p.on('error',reject);p.on('close',code=>resolve({code,stdout,stderr}));p.stdin.end(input);});}
function run(args:string[],env:NodeJS.ProcessEnv,input=''):Promise<{code:number|null;out:string}>{return new Promise((resolve,reject)=>{const p=spawn(process.execPath,[cli,...args],{env:{...process.env,SUMA_MODEL_API_KEY:undefined,...env},stdio:['pipe','pipe','pipe'],windowsHide:true});let out='';p.stdout.on('data',b=>out+=String(b));p.stderr.on('data',b=>out+=String(b));p.on('error',reject);p.on('close',code=>resolve({code,out}));p.stdin.end(input);});}

test('CLI version and help probes never enter the model loop',async()=>{
 const version=await run(['--version'],{SUMA_MODEL:undefined,SUMA_MODEL_BASE_URL:undefined});assert.equal(version.code,0);assert.match(version.out,/suma-agent-runtime 1\.0\.0/);
 const help=await run(['--help'],{});assert.equal(help.code,0);assert.match(help.out,/SUMA_AGENT_TOKEN/);assert.match(help.out,/JSONL/);
});

test('CLI language selects read-only help and rejects invalid explicit input before HTTP',async()=>{
 let requests=0;const server=createServer((_req,res)=>{requests++;res.end('{}');});await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');
 const env={SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:`http://127.0.0.1:${address.port}`,SUMA_CONTROL_URL:`http://127.0.0.1:${address.port}`,SUMA_AGENT_TOKEN:'fixture',SUMA_PROJECT_ID:'p'};
 try{
  const zh=await run(['--language','zh-Hans','--help'],env);assert.equal(zh.code,0);assert.match(zh.out,/智能体/);assert.match(zh.out,/JSONL/);
  const en=await run(['--language=en-GB','--help'],env);assert.equal(en.code,0);assert.match(en.out,/Suma agent runtime/);
  for(const args of [['--language='],['--language'],['--language','zh-Hant'],['--language','garbage'],['--language','en-US\n']]){const rejected=await run(args,env,'{"goal":"must never run"}\n');assert.equal(rejected.code,2,JSON.stringify(args));}
  assert.equal(requests,0);
 }finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});

test('machine stdout retains its JSONL shape and absent raw messages for CLI diagnostics',async()=>{
 const bad=await detailed(['--language','en-US'],{},'bad\nnull\n'+ 'x'.repeat(65537)+'\n');assert.equal(bad.code,1);assert.equal(bad.stderr,'');
 const events=bad.stdout.trim().split('\n').map(line=>JSON.parse(line));assert.deepEqual(events.map(e=>e.code),['invalid_json','invalid_json','request_limit']);
 assert.ok(!Object.hasOwn(events[0],'message'));assert.equal(events[2].message,'Request exceeded 64 KiB');
 assert.deepEqual(events.map(e=>e.display.message_key),['native.cli.invalid_json','native.cli.invalid_json','native.cli.request_limit']);
 const invalid=await detailed(['--language='],{SUMA_LANGUAGE:'en-US'},'{"goal":"never"}\n');assert.equal(invalid.code,2);assert.equal(invalid.stdout,'');assert.match(invalid.stderr,/--language/);
 const busy=await detailed(['--language','zh-CN'],{SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:'http://127.0.0.1:1'},'{"goal":"inspect","request_id":"first"}\n{"goal":"second"}\n{"cancel":true}\n');assert.equal(busy.code,1);assert.equal(busy.stderr,'');
 const emitted=busy.stdout.trim().split('\n').map(line=>JSON.parse(line));const event=emitted.find(e=>e.code==='busy');assert.ok(event);assert.ok(!Object.hasOwn(event,'message'));assert.equal(event.display.localized_message,'已有智能体请求正在运行');assert.equal(emitted.at(-1).code,'aborted');assert.equal(emitted.at(-1).message,'Agent aborted or timed out');
});

test('CLI environment precedence is deterministic and does not claim Traditional Chinese support',async()=>{
 for(const [args,env,locale] of [
  [[],{},'zh-CN'],
  [[],{SUMA_LANGUAGE:'en_GB',LC_ALL:'zh_CN.UTF-8'},'en-US'],
  [['--language','zh-Hans'],{SUMA_LANGUAGE:'en-US'},'zh-CN'],
  [[],{SUMA_LANGUAGE:'fr',LC_ALL:'en_AU.UTF-8'},'en-US'],
  [[],{LC_ALL:'fr_FR.UTF-8',LC_MESSAGES:'en_US.UTF-8'},'zh-CN'],
  [[],{LC_ALL:'C',LANG:'en_US.UTF-8'},'zh-CN'],
  [[],{LC_MESSAGES:'en_US.UTF-8',LANG:'zh_CN.UTF-8'},'en-US'],
  [[],{SUMA_LANGUAGE:'zh-Hant',LANG:'en_US.UTF-8'},'en-US'],
 ] as const){const result=await detailed([...args],{SUMA_MODEL:undefined,SUMA_MODEL_BASE_URL:undefined,...env},'{"goal":"inspect"}\n');assert.equal(result.code,1);assert.equal(result.stderr,'');const event=JSON.parse(result.stdout);assert.equal(event.code,'missing_model');assert.equal(event.display.locale,locale);assert.equal(event.message,'Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent');}
});

test('catalog language localizes only display reasons and read-only help does not probe',async()=>{
 const env={PATH:'',SUMA_LANGUAGE:'zh-CN'};
 const help=await detailed(['--help','--language','en-US'],env,'',catalogCLI);assert.equal(help.code,0);assert.match(help.stdout,/read-only/);assert.equal(help.stderr,'');
 const invalid=await detailed(['--language','zh-TW'],env,'',catalogCLI);assert.equal(invalid.code,2);assert.equal(invalid.stdout,'');
 for(const locale of ['zh-CN','en-US']){const result=await detailed(['--language',locale],env,'',catalogCLI);assert.equal(result.code,0);assert.equal(result.stderr,'');const catalog=JSON.parse(result.stdout);assert.equal(catalog.execution_enabled,false);assert.equal(catalog.items.length,5);for(const item of catalog.items){assert.equal(item.reason,'CLI not found on PATH');assert.equal(item.installed,false);assert.deepEqual(item.operations,[]);assert.deepEqual(item.verified,[]);assert.equal(item.display.message_key,'native.catalog.not_found');assert.equal(item.display.locale,locale);assert.equal(item.display.localized_message,locale==='zh-CN'?'在 PATH 中未找到 CLI':'CLI not found on PATH');}}
});

test('CLI uses a separate agent credential and never the Runner bearer',async()=>{
 let authorization='';const server=createServer((req,res)=>{res.setHeader('Content-Type','application/json');if(req.url?.startsWith('/api/v1/agent/tools')){authorization=req.headers.authorization??'';res.end(JSON.stringify({items:[]}));}else res.end(JSON.stringify({choices:[{message:{content:'actual fixture answer'},finish_reason:'stop'}],usage:{prompt_tokens:1,completion_tokens:1}}));});
 await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');const url=`http://127.0.0.1:${address.port}`;
 try{const result=await run([],{SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:url,SUMA_CONTROL_URL:url,SUMA_PROJECT_ID:'p',SUMA_AGENT_TOKEN:'agent-secret',SUMA_RUNNER_TOKEN:'runner-secret'},'{"goal":"inspect","request_id":"fixture"}\n');assert.equal(result.code,0);assert.equal(authorization,'Bearer agent-secret');assert.match(result.out,/completed/);assert.ok(!result.out.includes('agent-secret')&&!result.out.includes('runner-secret'));}finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});
