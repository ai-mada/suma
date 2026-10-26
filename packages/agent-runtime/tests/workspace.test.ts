import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createServer,type IncomingMessage,type ServerResponse} from 'node:http';
import {createHash} from 'node:crypto';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {runAgent,type AgentEvent,type RuntimeConfig} from '../src/index.js';

const names=['list','read','write','remove','check','edit'];
const fields:Record<string,string[]>={list:['directory','limit','cursor'],read:['path','start_line','max_lines'],write:['path','content','expected_sha256','expected_absent'],remove:['path','expected_sha256'],check:['check_id'],edit:['path','expected_sha256','edits']};
const alias=(name:string)=>'tool_'+createHash('sha256').update(name).digest('hex').slice(0,32);
async function body(req:IncomingMessage){const chunks:Buffer[]=[];for await(const chunk of req)chunks.push(Buffer.from(chunk));return Buffer.concat(chunks).toString('utf8');}
async function fixture(options:{fatal?:boolean;omitFatal?:boolean;oversize?:boolean;badCatalog?:boolean;redirect?:boolean;holdCall?:boolean;repair?:boolean;repairCatalogOnly?:boolean;omitRepairTool?:boolean;toolCalls?:number}={}){
 const wire:string[]=[],models:Record<string,any>[]=[];let calls=0;
 const server=createServer(async(req:IncomingMessage,res:ServerResponse)=>{
  const raw=await body(req);res.setHeader('Content-Type','application/json');
  if(req.url==='/model/chat/completions'){
   const input=JSON.parse(raw);models.push(input);const call=models.length<=(options.toolCalls??2);
   const toolName=options.repair?(models.length===1?'merge_status':'merge_continue'):'read';
   const args=options.repair?(models.length===1?'{}':JSON.stringify({expected_step:1,expected_generation:2,expected_state_sha256:'a'.repeat(64),resolutions:[{path:'a.ts',expected_sha256:'b'.repeat(64)}]})):models.length===1?'{"path":"a.ts","start_line":1e0,"start_line":9007199254740993}':'{"path":"a.ts","start_line":1}';
   res.end(JSON.stringify({choices:[{finish_reason:'stop',message:call?{tool_calls:[{id:'call-'+models.length,type:'function',function:{name:alias('workspace.'+toolName),arguments:args}}]}:{content:'actual fixture completion'}}],usage:{prompt_tokens:2,completion_tokens:1}}));return;
  }
  if(req.url!=='/mcp'){res.statusCode=404;res.end('{}');return;}
  assert.equal(req.headers.authorization,'Bearer bridge-secret-fixture');assert.equal(req.headers.cookie,undefined);assert.equal(req.headers.origin,undefined);
  if(req.method==='GET'){res.statusCode=405;res.end('{}');return;}
  const message=JSON.parse(raw);wire.push(raw);
  if(options.redirect){res.statusCode=307;res.setHeader('Location','http://127.0.0.1:1/stolen');res.end('{}');return;}
  if(message.method==='notifications/initialized'){res.statusCode=202;res.end();return;}
  let result:unknown={};
  if(message.method==='initialize')result={protocolVersion:'2025-11-25',serverInfo:{name:'suma-native-workspace',version:'attempt-workspace-v1'},capabilities:{tools:{}}};
  if(message.method==='tools/list'){
   const repairFields={merge_status:[],merge_continue:['expected_step','expected_generation','expected_state_sha256','resolutions']};const expected:Record<string,string[]>={...fields,...repairFields};
   const catalog=options.repair||options.repairCatalogOnly?[...names,...(options.omitRepairTool?['merge_status']:Object.keys(repairFields))]:options.badCatalog?names.slice(1):names;
   result={tools:catalog.map(name=>({name:'suma_workspace_'+name,description:'fixture '+name,inputSchema:{type:'object',properties:Object.fromEntries(expected[name].map(field=>[field,{}])),additionalProperties:false}}))};
  }
  if(message.method==='tools/call'){
   calls++;if(options.holdCall)return;
   result=options.oversize?{content:[{type:'text',text:'x'.repeat(65537)}]}:calls===1?{isError:true,content:[{type:'text',text:'safe business error'}],structuredContent:{error:{code:options.fatal?'native_workspace_unknown':'native_workspace_args',message:'safe business error',...(options.omitFatal?{}:{fatal:!!options.fatal})}}}:{content:[{type:'text',text:'{"path":"a.ts","content":"verified fixture"}'}]};
  }
  res.end(JSON.stringify({jsonrpc:'2.0',id:message.id,result}));
 });
 await new Promise<void>(resolve=>server.listen(0,'127.0.0.1',resolve));const address=server.address();if(!address||typeof address==='string')throw Error('address');const origin='http://127.0.0.1:'+address.port;
 const config:RuntimeConfig={model:'fixture',baseURL:origin+'/model',timeoutMS:3000,...{workspaceBridge:{version:1 as const,endpoint:origin+'/mcp',token:'bridge-secret-fixture',policy_sha256:'a'.repeat(64),request_id:'workspace-request',...(options.repair?{conflict_repair_protocol:'suma.candidate-conflict-repair.v1' as const}:{})}}};
 return {config,endpoint:origin+'/mcp',wire,models,get calls(){return calls;},close:async()=>{server.closeAllConnections();await new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()));}};
}

test('official MCP client discovers six workspace tools, preserves raw arguments and recovers ordinary business errors',async()=>{
 const f=await fixture();try{const events:AgentEvent[]=[];await runAgent({goal:'fix fixture',request_id:'workspace-request'},f.config,event=>events.push(event));assert.equal(events.at(-1)?.type,'completed');assert.equal(f.calls,2);assert.deepEqual(events.find(event=>event.type==='started')?.tools,names.map(name=>'workspace.'+name));
 const requests=f.wire.filter(raw=>JSON.parse(raw).method==='tools/call');assert.ok(requests[0].includes('"start_line":1e0,"start_line":9007199254740993'));const call=JSON.parse(requests[0]);assert.equal(call.params._meta['suma/operation_id'],createHash('sha256').update(JSON.stringify(['workspace-request',0,'call-1'])).digest('hex'));
 const visible=JSON.stringify({models:f.models,events});for(const secret of ['bridge-secret','policy_sha256','workspace_bridge',f.endpoint])if(secret)assert.ok(!visible.includes(secret),String(secret));assert.ok(f.models[1].messages.some((message:any)=>message.role==='tool'&&message.content.includes('native_workspace_args')));
 }finally{await f.close();}
});
test('unknown workspace outcomes stop instead of allowing the model to continue',async()=>{
 for(const options of [{fatal:true},{omitFatal:true},{oversize:true}]){const f=await fixture(options);try{const events:AgentEvent[]=[];await runAgent({goal:'fix fixture',request_id:'workspace-request'},f.config,event=>events.push(event));assert.equal(events.at(-1)?.code,'workspace_fatal');assert.equal(f.models.length,1);assert.ok(!events.some(event=>event.type==='completed'));}finally{await f.close();}}
});
test('repair capability binds exactly eight official SDK tools and host continuation arguments',async()=>{
 const f=await fixture({repair:true});try{const events:AgentEvent[]=[];await runAgent({goal:'resolve all fixed sources',request_id:'workspace-request'},f.config,e=>events.push(e));assert.equal(events.at(-1)?.type,'completed');assert.deepEqual(events.find(e=>e.type==='started')?.tools,[...names,'merge_status','merge_continue'].map(name=>'workspace.'+name));const calls=f.wire.map(raw=>JSON.parse(raw)).filter(message=>message.method==='tools/call');assert.deepEqual(calls.map(call=>call.params.name),['suma_workspace_merge_status','suma_workspace_merge_continue']);assert.equal(calls[1].params.arguments.expected_step,1);assert.equal(calls[1].params.arguments.expected_generation,2);assert.equal(calls[1].params.arguments.resolutions[0].path,'a.ts');assert.equal(calls[1].params._meta['suma/operation_id'],createHash('sha256').update(JSON.stringify(['workspace-request',1,'call-2'])).digest('hex'));assert.ok(String(f.models[0].messages[0].content).includes('workspace.merge_continue'));assert.equal(f.models[0].max_completion_tokens,1024);}finally{await f.close();}
});
test('repair tools without capability or an incomplete repair catalogue stop before model access',async()=>{
 for(const options of [{repairCatalogOnly:true},{repair:true,omitRepairTool:true}]){const f=await fixture(options);try{const events:AgentEvent[]=[];await runAgent({goal:'repair',request_id:'workspace-request'},f.config,e=>events.push(e));assert.equal(events.at(-1)?.code,'workspace_catalog');assert.equal(f.models.length,0);assert.equal(f.calls,0);}finally{await f.close();}}
});
test('repair budgets support fifteen conflict stages and explicit exhaustion never starts another session',async()=>{
 const full=await fixture({repair:true,toolCalls:45});try{const events:AgentEvent[]=[];await runAgent({goal:'resolve fifteen frozen conflict stages',request_id:'workspace-request'},full.config,e=>events.push(e));assert.equal(events.at(-1)?.type,'completed');assert.equal(full.models.length,46);assert.equal(full.calls,45);assert.equal(events.filter(e=>e.type==='started').length,1);}finally{await full.close();}
 const limited=await fixture({repair:true,toolCalls:45});try{const events:AgentEvent[]=[];await runAgent({goal:'bounded repair',request_id:'workspace-request',max_steps:2},limited.config,e=>events.push(e));assert.equal(events.at(-1)?.type,'error');assert.equal(limited.models.length,2);assert.equal(events.filter(e=>e.type==='started').length,1);assert.ok(!events.some(e=>e.type==='completed'));}finally{await limited.close();}
});
test('an incomplete workspace catalogue and redirects fail before any model request',async()=>{
 for(const options of [{badCatalog:true},{redirect:true}]){const f=await fixture(options);try{const events:AgentEvent[]=[];await runAgent({goal:'fix fixture',request_id:'workspace-request'},f.config,event=>events.push(event));assert.equal(events.at(-1)?.type,'error');assert.equal(f.models.length,0);}finally{await f.close();}}
});
test('cancelling an in-flight workspace call stops the runtime without a completed result',async()=>{
 const f=await fixture({holdCall:true}),controller=new AbortController();try{const events:AgentEvent[]=[];const running=runAgent({goal:'fix fixture',request_id:'workspace-request'},f.config,event=>events.push(event),controller.signal);const deadline=Date.now()+1000;while(f.calls===0&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,5));controller.abort();await running;assert.equal(events.at(-1)?.code,'aborted');assert.equal(events.at(-1)?.message,'Agent aborted or timed out');assert.equal(events.at(-1)?.display?.message_key,'native.run.aborted');assert.equal(f.models.length,1);assert.ok(!events.some(event=>event.type==='completed'));}finally{await f.close();}
});
test('CLI receives the hidden bridge only from stdin and omits it from JSONL events and model messages',async()=>{
 const f=await fixture();try{
  const result=await new Promise<{code:number|null;out:string;err:string}>((resolve,reject)=>{const child=spawn(process.execPath,[fileURLToPath(new URL('../src/cli.js',import.meta.url))],{windowsHide:true,stdio:['pipe','pipe','pipe'],env:{...process.env,SUMA_MODEL:'fixture',SUMA_MODEL_BASE_URL:f.config.baseURL,SUMA_MODEL_API_KEY:undefined,SUMA_CONTROL_URL:undefined,SUMA_AGENT_TOKEN:undefined,SUMA_PROJECT_ID:undefined,SUMA_WORKSPACE_BRIDGE:'must-never-be-used'}});let out='',err='';child.stdout.on('data',chunk=>out+=String(chunk));child.stderr.on('data',chunk=>err+=String(chunk));child.on('error',reject);child.on('close',code=>resolve({code,out,err}));child.stdin.end(JSON.stringify({goal:'fix fixture',request_id:'workspace-request',workspace_bridge:f.config.workspaceBridge})+'\n');});
  assert.equal(result.code,0,result.out+result.err);assert.equal(result.err,'');assert.equal(JSON.parse(result.out.trim().split('\n').at(-1)!).type,'completed');assert.ok(!result.out.includes('bridge-secret'));assert.ok(!JSON.stringify(f.models).includes(f.endpoint));assert.equal(f.calls,2);
 }finally{await f.close();}
});
test('remote endpoints and mismatched bridge scope are refused before model HTTP',async()=>{
 const f=await fixture();try{for(const bridge of [{...f.config.workspaceBridge,endpoint:'https://example.test/mcp'},{...f.config.workspaceBridge,request_id:'foreign-attempt'},{...f.config.workspaceBridge,version:2},{...f.config.workspaceBridge,token:'bad\nheader'}]){const events:AgentEvent[]=[];await runAgent({goal:'fix fixture',request_id:'workspace-request'},{...f.config,workspaceBridge:bridge as any},event=>events.push(event));assert.equal(events.at(-1)?.code,'workspace_config');}assert.equal(f.models.length,0);assert.equal(f.wire.length,0);}finally{await f.close();}
});
test('workspace catch diagnostics use their dedicated canonical sources in both display languages',async()=>{
 const cases=[
  {code:'workspace_config',key:'native.workspace.config',en:'Workspace bridge configuration is invalid',zh:'工作区连接配置无效',options:{},invalid:true},
  {code:'workspace_catalog',key:'native.workspace.catalog',en:'Workspace tool identity or catalogue is invalid',zh:'工作区工具身份或目录无效',options:{badCatalog:true}},
  {code:'workspace_transport',key:'native.workspace.transport',en:'Workspace tool connection failed; inspect the current execution',zh:'工作区工具连接失败，请核对当前执行',options:{redirect:true}},
  {code:'workspace_fatal',key:'native.workspace.effects',en:'Workspace effects require reconciliation; do not restart the model',zh:'工作区操作效果需要核对，请勿重新运行模型',options:{fatal:true}},
 ];
 for(const locale of ['en-US','zh-CN'])for(const scenario of cases){const f=await fixture(scenario.options);try{const events:AgentEvent[]=[];const config={...f.config,locale,...(scenario.invalid?{workspaceBridge:{...f.config.workspaceBridge,version:2} as any}:{})};await runAgent({goal:'fix fixture',request_id:'workspace-request'},config,event=>events.push(event));const error=events.at(-1)!;assert.equal(error.code,scenario.code);assert.equal(error.message,scenario.en);assert.equal(error.display?.message_key,scenario.key);assert.equal(error.display?.localized_message,locale==='en-US'?scenario.en:scenario.zh);assert.equal(error.display?.locale,locale);assert.ok(!JSON.stringify(events).includes('bridge-secret'));}finally{await f.close();}}
});
