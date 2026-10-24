import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {mkdir,writeFile,access} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {join} from 'node:path';
import fsPromises from 'node:fs/promises';
import childProcess from 'node:child_process';
import {syncBuiltinESMExports} from 'node:module';
import {runAgent,type AgentEvent,type RuntimeConfig} from '../src/index.js';
import {normalizeLocale,nativeDisplay,nativeCatalog,type NativeMessageKey} from '../src/display.js';
import {inspectAdapters} from '../src/catalog.js';

test('invalid explicit SDK locale rejects before events or model and control HTTP without mutating callers',async()=>{
 let requests=0;const server=createServer((_req,res)=>{requests++;res.end('{"items":[],"choices":[{"message":{"content":"must not run"}}]}');});await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');const url=`http://127.0.0.1:${address.port}`;
 try{for(const locale of ['','fr-FR','zh-Hant','zh-TW','en-US\n','en-US\u0085','e'.repeat(129),null,42]){const events:AgentEvent[]=[];const config=Object.freeze({locale:locale as string,model:'fixture',baseURL:url,controlURL:url,controlToken:'fixture',projectID:'fixture'});const request=Object.freeze({goal:'must not run',request_id:'stable'});await assert.rejects(runAgent(request,config,e=>events.push(e)),(error:unknown)=>{const e=error as Error&{code:string;display:{message_key:string}};assert.equal(e.code,'invalid_locale');assert.equal(e.message,'Unsupported explicit native display language');assert.equal(e.display.message_key,'native.config.invalid_locale');return true;});assert.deepEqual(events,[]);assert.equal(config.locale,locale);assert.deepEqual(request,{goal:'must not run',request_id:'stable'});assert.equal(requests,0);}}
 finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});

test('invalid catalog SDK locale rejects before filesystem access or any process',async t=>{
 let accesses=0,processes=0;
 t.mock.method(fsPromises,'access',async()=>{accesses++;});t.mock.method(childProcess,'spawn',()=>{processes++;throw Error('unexpected probe');});syncBuiltinESMExports();
 try{for(const locale of ['','fr-FR','zh-Hant','en-US\n',null,42]){await assert.rejects(inspectAdapters(locale as string),(error:unknown)=>{const e=error as Error&{code:string;display:{message_key:string}};assert.equal(e.code,'invalid_locale');assert.equal(e.message,'Unsupported explicit native display language');assert.equal(e.display.message_key,'native.config.invalid_locale');return true;});assert.equal(accesses,0);assert.equal(processes,0);}}
 finally{t.mock.restoreAll();syncBuiltinESMExports();}
});

test('SDK default and supported aliases preserve immutable caller configuration',async()=>{
 for(const [locale,expected] of [[undefined,'zh-CN'],['zh_Hans_SG','zh-CN'],['en-GB','en-US']] as const){const config=Object.freeze({locale});const events:AgentEvent[]=[];await runAgent(Object.freeze({goal:'inspect'}),config,e=>events.push(e));assert.equal(events.at(-1)?.code,'missing_model');assert.equal(events.at(-1)?.display?.locale,expected);assert.equal(config.locale,locale);}
 const previousPath=process.env.PATH;process.env.PATH='';
 try{for(const [locale,expected] of [[undefined,'zh-CN'],['zh_Hans_SG','zh-CN'],['en-GB','en-US']] as const){const items=await inspectAdapters(locale);assert.equal(items.length,4);assert.ok(items.every(item=>item.display?.locale===expected&&item.reason==='CLI not found on PATH'));}}
 finally{if(previousPath===undefined)delete process.env.PATH;else process.env.PATH=previousPath;}
});

test('concurrent run display language is independent while original evidence stays English',async()=>{
 const events:AgentEvent[][]=[[],[]];
 await Promise.all(['zh-CN','en-US'].map((locale,i)=>runAgent({goal:'inspect',request_id:'local-'+i},{locale} as RuntimeConfig,e=>events[i].push(e))));
 const zh=events[0].at(-1)!;const en=events[1].at(-1)!;
 assert.equal(zh.message,'Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent');
 assert.equal(en.message,zh.message);assert.equal(zh.request_id,'local-0');assert.equal(en.request_id,'local-1');
 assert.deepEqual(zh.display,{message_key:'native.config.missing_model',locale:'zh-CN',localized_message:'运行智能体前请配置 SUMA_MODEL 和 SUMA_MODEL_BASE_URL'});
 assert.deepEqual(en.display,{message_key:'native.config.missing_model',locale:'en-US',localized_message:en.message});
});

test('BCP47 aliases select only supported scripts and bounded display parameters fail closed',()=>{
 for(const [value,expected] of [['zh','zh-CN'],['zh_Hans_SG','zh-CN'],['zh-MY','zh-CN'],['en-GB','en-US'],['en_Latn_AU','en-US'],['zh-Hant',undefined],['zh-TW',undefined],['zh-HK',undefined],['en-Arab',undefined],['',undefined],['en-US\u0085',undefined],['en-US\n',undefined],['e'.repeat(129),undefined],['en--US',undefined]] as const)assert.equal(normalizeLocale(value),expected,value);
 assert.deepEqual(nativeDisplay('native.endpoint.rejected','en-US',{status:429}),{message_key:'native.endpoint.rejected',message_params:{status:429},locale:'en-US',localized_message:'Endpoint rejected request (HTTP 429)'});
 for(const params of [{},{status:'429'},{status:-1},{status:1.5},{status:429,secret:'must not echo'}])assert.equal(nativeDisplay('native.endpoint.rejected','zh-CN',params),undefined);
 assert.equal(nativeDisplay('native.catalog.probe_exited','en-US',{status:'\nsecret'}),undefined);
 const params={status:429};const display=nativeDisplay('native.endpoint.rejected','zh-CN',params)!;params.status=500;assert.deepEqual(display.message_params,{status:429});
 for(const key of Object.keys(nativeCatalog) as NativeMessageKey[]){const params=key==='native.endpoint.rejected'?{status:429}:key==='native.catalog.probe_exited'?{status:'2'}:{};const zh=nativeDisplay(key,'zh-CN',params);const en=nativeDisplay(key,'en-US',params);assert.ok(zh&&en,key);assert.ok(!zh.localized_message.includes('{{')&&!en.localized_message.includes('{{'),key);assert.notEqual(zh.localized_message,en.localized_message,key);}
});

test('untrusted keys and inherited or malformed parameters cannot enter display metadata',()=>{
 assert.equal(nativeDisplay('toString' as NativeMessageKey,'en-US'),undefined);
 assert.equal(nativeDisplay('native.endpoint.rejected','en-US',Object.assign(Object.create({status:429}),{other:'secret'})),undefined);
 assert.equal(nativeDisplay('native.endpoint.rejected','en-US',null as unknown as Record<string,unknown>),undefined);
 assert.equal(nativeDisplay('native.cli.busy','en-US',[] as unknown as Record<string,unknown>),undefined);
});

test('Windows catalog wrappers and foreign OS probe failures keep raw facts and never run a shell',{skip:process.platform!=='win32'},async()=>{
 const directory=fileURLToPath(new URL('../../../../.cache/e2e/localization-stage-010/native/catalog-fixture/',import.meta.url));await mkdir(directory,{recursive:true});const marker=join(directory,'must-not-execute.txt');
 await writeFile(join(directory,'codex.cmd'),'@echo fixture>"'+marker+'"\r\n','utf8');await mkdir(join(directory,'opencode.exe'),{recursive:true});
 const originalPath=process.env.PATH;process.env.PATH=directory;
 try{const items=await inspectAdapters('zh-CN');const wrapper=items.find(item=>item.id==='codex')!;assert.equal(wrapper.installed,true);assert.equal(wrapper.reason,'Shell wrapper is not probed automatically');assert.equal(wrapper.display?.message_key,'native.catalog.shell_wrapper');assert.deepEqual(wrapper.operations,[]);assert.deepEqual(wrapper.verified,[]);await assert.rejects(access(marker));const foreign=items.find(item=>item.id==='opencode')!;assert.equal(foreign.installed,true);assert.ok(foreign.reason?.includes('spawn'));assert.equal(foreign.display?.message_key,'native.catalog.probe_failed');assert.equal(foreign.display?.localized_message,'CLI 探测失败，请检查原始原因');assert.deepEqual(foreign.operations,[]);assert.deepEqual(foreign.verified,[]);}
 finally{if(originalPath===undefined)delete process.env.PATH;else process.env.PATH=originalPath;}
});

test('actual concurrent model requests preserve body and raw output and capture locale at run entry',async()=>{
 const bodies:string[]=[];let count=0;let release!:()=>void;const arrived=new Promise<void>(r=>release=r);const configs:RuntimeConfig[]=[];
 const server=createServer(async(req,res)=>{let raw='';for await(const chunk of req)raw+=String(chunk);bodies.push(raw);if(++count===2)release();await arrived;for(const config of configs)config.locale='zh-CN';res.end(JSON.stringify({choices:[{message:{content:'用户原始 output <tag> English\n证据'},finish_reason:'stop'}]}));});
 await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');
 try{const events:AgentEvent[][]=[[],[]];configs.push(...['zh-CN','en-US'].map(locale=>({locale,model:'fixture',baseURL:`http://127.0.0.1:${address.port}`})));await Promise.all(configs.map((config,i)=>runAgent({goal:'用户 Goal stays English',request_id:'same'},config,e=>events[i].push(e))));assert.equal(bodies[0],bodies[1]);const request=JSON.parse(bodies[0]);assert.deepEqual(Object.keys(request).sort(),['max_completion_tokens','messages','model','stream']);assert.equal(request.messages[1].content,'用户 Goal stays English');for(const stream of events){assert.equal(stream.at(-1)?.output,'用户原始 output <tag> English\n证据');assert.ok(!('display' in stream.at(-1)!));assert.equal(stream.find(e=>e.type==='usage')?.usage_state,'unreported');assert.ok(!('output_tokens' in stream.at(-1)!));}}
 finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
 // The same mutable caller object must not change the display of an in-flight run.
 const config:RuntimeConfig={locale:'en-US',model:'fixture',baseURL:'http://127.0.0.1:1'};const abort=new AbortController();const events:AgentEvent[]=[];
 await runAgent({goal:'stop'},config,e=>{events.push(e);if(e.type==='started'){config.locale='zh-CN';abort.abort();}},abort.signal);
 assert.equal((events.at(-1)?.display as {locale:string}).locale,'en-US');assert.equal(events.at(-1)?.code,'aborted');assert.equal(events.at(-1)?.message,'Agent aborted or timed out');
});

test('localized model HTTP429 keeps exact status, retry facts and unknown usage',async()=>{
 const server=createServer((_req,res)=>{res.statusCode=429;res.end('{"error":"raw fixture must not echo"}');});await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');
 try{for(const locale of ['zh-CN','en-US']){const events:AgentEvent[]=[];await runAgent({goal:'inspect'},{model:'fixture',baseURL:`http://127.0.0.1:${address.port}`,locale},e=>events.push(e));const event=events.at(-1)!;assert.equal(event.message,'Endpoint rejected request (HTTP 429)');assert.equal(event.code,'upstream_rejected');assert.equal(event.http_status,429);assert.equal(event.source,'model');assert.equal(event.failure_class,'transient');assert.deepEqual(event.display,{message_key:'native.endpoint.rejected',message_params:{status:429},locale,localized_message:locale==='zh-CN'?'端点拒绝请求（HTTP 429）':'Endpoint rejected request (HTTP 429)'});assert.equal(events.find(e=>e.type==='usage')?.usage_state,'unreported');assert.ok(!('input_tokens' in events.find(e=>e.type==='usage')!));}}
 finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
});

test('same machine code carries distinct explicit diagnostic keys',async()=>{
 const keys:string[]=[];
 for(const reply of ['[]','{bad','{}']){
  const server=createServer((_req,res)=>res.end(reply));await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');
  try{const events:AgentEvent[]=[];await runAgent({goal:'inspect'},{model:'fixture',baseURL:`http://127.0.0.1:${address.port}`,locale:'zh-CN'} as RuntimeConfig,e=>events.push(e));const event=events.at(-1)!;assert.equal(event.code,'invalid_response');keys.push((event.display as {message_key:string})?.message_key);}
  finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}
 }
 assert.deepEqual(keys,['native.response.object_required','native.response.invalid_json','native.response.no_choices']);
});

test('runtime validation and response failures retain precise raw diagnostic alongside display',async()=>{
 for(const [request,config,code,key] of [
  [{goal:''},{},'invalid_goal','native.request.invalid_goal'],
  [{goal:'inspect',max_steps:0},{},'invalid_limits','native.request.invalid_limits'],
  [{goal:'inspect'},{model:'fixture'},'missing_provider','native.config.missing_provider'],
  [{goal:'inspect'},{model:'fixture',baseURL:'http://untrusted.invalid'},'unsafe_endpoint','native.endpoint.unsafe'],
  [{goal:'inspect'},{model:'fixture',baseURL:'http://127.0.0.1:1',controlURL:'http://127.0.0.1:1'},'missing_control_auth','native.config.missing_control_auth'],
  [{goal:'inspect'},{model:'fixture',baseURL:'not a URL'},'runtime_error','native.run.failed'],
 ] as const){const events:AgentEvent[]=[];await runAgent(request,{...config,locale:'en-US'},e=>events.push(e));const event=events.at(-1)!;assert.equal(event.code,code);assert.equal(event.display?.message_key,key);assert.equal(event.display?.localized_message,event.message);assert.ok(!events.some(e=>e.type==='started'));}
 for(const [response,code,key] of [
  [{choices:[{message:{content:'partial'},finish_reason:'length'}]},'token_limit','native.limit.output_truncated'],
  [{choices:[{message:{content:'  '}}]},'empty_completion','native.response.no_text'],
  [{choices:[{message:{tool_calls:{}}}]},'tool_limit','native.limit.tool_calls'],
  [{choices:[{message:{tool_calls:[{function:{arguments:'{}'}}]}}]},'invalid_tool_call','native.tools.malformed_call'],
  [{choices:[{message:{content:'must not complete'}}],usage:{prompt_tokens:1.5}},'invalid_response','native.usage.invalid_counter'],
  [{choices:[{message:{content:'must not complete'}}],usage:{prompt_tokens:1,cached_input_tokens:1,prompt_tokens_details:{cached_tokens:2}}},'invalid_response','native.usage.conflicting_counters'],
 ] as const){const server=createServer((_req,res)=>res.end(JSON.stringify(response)));await new Promise<void>(r=>server.listen(0,'127.0.0.1',r));const address=server.address();if(!address||typeof address==='string')throw Error('no fixture');try{const events:AgentEvent[]=[];await runAgent({goal:'inspect'},{model:'fixture',baseURL:`http://127.0.0.1:${address.port}`,locale:'en-US'},e=>events.push(e));const event=events.at(-1)!;assert.equal(event.code,code);assert.equal(event.display?.message_key,key);assert.equal(event.display?.localized_message,event.message);assert.ok(!events.some(e=>e.type==='completed'));}finally{await new Promise<void>((r,j)=>server.close(e=>e?j(e):r()));}}
});
