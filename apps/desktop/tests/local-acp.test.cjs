'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events'),{PassThrough}=require('node:stream'),path=require('node:path'),fs=require('node:fs');
const base={project_id:'1'.repeat(32),user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'qoder-acp',locale:'en-US'};
const fixtureRoot=path.resolve(__dirname,'../../../.cache/acp-quick-connect-171/desktop/tmp');
function result(){return {protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'ready',executable_sha256:'a'.repeat(64),tool_version:'Qoder reviewed',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model',current_value:'one',models:[{id:'one',name:'One',configuration_sha256:'b'.repeat(64)}]},evidence:{session_created:true,session_cancel_requested:false,session_cancel_written:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true,host_process_exit_code:0,host_stopped_after_rpc:false}};}
function harness(t,options={}){
 const caseRoot=options.fixtureRoot||fixtureRoot;fs.mkdirSync(caseRoot,{recursive:true});const directory=fs.mkdtempSync(path.join(caseRoot,'case-')),operations=options.operations||new Map(),children=[],context={binding_sha256:'4'.repeat(64),base_commit:'5'.repeat(40),repository_path:directory},environment={PATH:process.env.PATH,LOCAL_QODER_CREDENTIAL:'private-credential',SUMA_AGENT_TOKEN:'must-not-inherit'};let allowed=true,clock=Date.now(),version='Qoder reviewed';
 const journal={readACP:id=>operations.has(id)?structuredClone(operations.get(id)):undefined,writeACP:value=>operations.set(value.operation_id,structuredClone(value))};
 const spawn=(binary,args,opts)=>{const child=new EventEmitter();Object.assign(child,{stdin:new PassThrough(),stdout:new PassThrough(),stderr:new PassThrough(),kill:()=>{child.emit('close',null,'SIGKILL');return true;}});let input='';child.stdin.on('data',b=>input+=b);children.push({child,binary,args,opts,input:()=>JSON.parse(input)});return child;};
 const {createLocalACPController}=require('../src/local-acp.cjs');
 const make=()=>createLocalACPController({binary:process.execPath,probeDirectory:directory,credentialKey:Buffer.alloc(32,7),environment,spawn,caller:()=>allowed,session:async()=>{if(!allowed)throw Error('authority');},authorize:async()=>({...context}),discover:options.discover||(async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'qoder-acp',product:'Qoder CLI',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:path.join(directory,'qoder.exe'),executable_sha256:'a'.repeat(64),version}]}})),journal:()=>journal,now:()=>clock,deadlineMS:1000});
 const controller=make();t.after(async()=>{await controller.dispose();});
 return {controller,make,children,operations,environment,context,directory,event:{},setAllowed:v=>allowed=v,advance:n=>clock+=n,setVersion:v=>version=v,async detect(){return (await controller.inspect({}, {...base,action:'detect'})).observation.installations[0];},async started(){for(let i=0;i<100&&!children.length;i++)await new Promise(setImmediate);assert.equal(children.length,1);},complete(value=result()){children.at(-1).child.stdout.emit('data',Buffer.from(JSON.stringify(value)));children.at(-1).child.emit('close',0);}};
}
test('ACP explicit discovery sends one bounded private probe and selects only its exact fresh model',async t=>{
 const h=harness(t),installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'6'.repeat(32),credential_env_ref:'LOCAL_QODER_CREDENTIAL'};
 const pending=h.controller.inspect(h.event,request);await h.started();assert.deepEqual(h.children[0].args,['--language=en-US','--describe-acp-models-stdin']);assert.equal(h.children[0].input().credential_env_ref,'LOCAL_QODER_CREDENTIAL');assert.equal(h.children[0].opts.env.LOCAL_QODER_CREDENTIAL,'private-credential');assert.equal(h.children[0].opts.env.SUMA_AGENT_TOKEN,undefined);
 h.complete();const out=await pending;assert.equal(out.observation.phase,'ready');assert.equal(JSON.stringify(out).includes('private-credential'),false);assert.equal(JSON.stringify(out).includes('executable"'),false);
 const selected=await h.controller.select(base,{installation_id:installation.installation_id,probe_id:out.observation.probe.id,model_id:'one'},h.context);assert.equal(selected.configuration_sha256,'b'.repeat(64));assert.equal(selected.model,'one');
 assert.equal((await h.controller.inspect({},request)).observation.probe.id,out.observation.probe.id);assert.equal(h.children.length,1);
 h.advance(60001);assert.equal((await h.controller.inspect({}, {...base,action:'status',operation_id:request.operation_id})).observation.phase,'expired');await assert.rejects(h.controller.select(base,{installation_id:installation.installation_id,probe_id:out.observation.probe.id,model_id:'one'},h.context));
});
test('ACP pending operation survives controller restart as unknown and never repeats a session',async t=>{
 const h=harness(t),installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'7'.repeat(32)};
 const pending=h.controller.inspect({},request);await h.started();const next=h.make();t.after(()=>next.dispose());assert.equal((await next.inspect({},request)).observation.phase,'unknown');assert.equal(h.children.length,1);h.complete();await pending;
});
test('ACP unexpected activity, mismatched source and late authorization cannot expose an adoptable catalogue',async t=>{
 for(const kind of ['activity','source','authority']){const h=harness(t),installation=await h.detect(),pending=h.controller.inspect({}, {...base,action:'models',installation_id:installation.installation_id,operation_id:'8'.repeat(32)});await h.started();const value=result();if(kind==='activity')value.evidence.prompt_requests=1;if(kind==='source')value.executable_sha256='f'.repeat(64);if(kind==='authority')h.setAllowed(false);h.complete(value);const out=await pending;assert.notEqual(out.observation?.phase,'ready');assert.equal(JSON.stringify(out).includes('catalogue'),false);}
});
test('ACP changed credentials or context invalidate adoption without restarting or changing the original probe',async t=>{
 const h=harness(t),installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'9'.repeat(32),credential_env_ref:'LOCAL_QODER_CREDENTIAL'},pending=h.controller.inspect({},request);await h.started();h.complete();const out=await pending,selection={installation_id:installation.installation_id,probe_id:out.observation.probe.id,model_id:'one'};
 h.environment.LOCAL_QODER_CREDENTIAL='rotated';await assert.rejects(h.controller.select(base,selection,h.context));h.environment.LOCAL_QODER_CREDENTIAL='private-credential';h.context.base_commit='f'.repeat(40);await assert.rejects(h.controller.select(base,selection,h.context));assert.equal(h.children.length,1);
});
test('ACP rejects renderer launcher fields and concurrent model requests before a second child can start',async t=>{
 const h=harness(t);assert.equal((await h.controller.inspect({}, {...base,action:'detect',executable:process.execPath})).status,'failed');assert.equal(h.children.length,0);
 const installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'a'.repeat(32)},pending=h.controller.inspect({},request);
 const duplicate=await h.controller.inspect({},request);assert.equal(duplicate.status,'failed');await h.started();const status=await h.controller.inspect({}, {...base,action:'status',operation_id:request.operation_id});assert.equal(status.observation.phase,'reading');assert.equal(h.children.length,1);h.complete();await pending;
});
test('ACP rejects duplicate JSON, secrets and output overflow without exposing a reusable catalogue',async t=>{
 for(const variant of ['duplicate','secret','overflow']){const h=harness(t),installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'b'.repeat(32),credential_env_ref:'LOCAL_QODER_CREDENTIAL'},pending=h.controller.inspect({},request);await h.started();
  if(variant==='overflow')h.children[0].child.stdout.emit('data',Buffer.alloc(262145));
  else {let value=result();if(variant==='secret')value.catalogue.models[0].name='private-credential';let raw=JSON.stringify(value);if(variant==='duplicate')raw=raw.replace('"state":"ready"','"state":"failed","state":"ready"');h.children[0].child.stdout.emit('data',Buffer.from(raw));h.children[0].child.emit('close',0);}
  const out=await pending;assert.equal(out.observation.phase,'unknown');assert.equal(JSON.stringify(out).includes('private-credential'),false);assert.equal(JSON.stringify(out).includes('catalogue'),false);await h.controller.inspect({},request);assert.equal(h.children.length,1);
 }
});
test('ACP wrapper-only detection remains distinct without an executable path or a model session',async t=>{
 const h=harness(t,{discover:async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'qoder-acp',product:'Qoder CLI',installed:true,launch_supported:false,reason_code:'unsupported_launcher'}]}})});
 const out=await h.controller.inspect({}, {...base,action:'detect'});assert.equal(out.observation.phase,'wrapper_only');assert.equal(out.observation.installations[0].kind,'wrapper_only');assert.equal(Object.hasOwn(out.observation.installations[0],'executable'),false);assert.equal(h.children.length,0);
 const attempt=await h.controller.inspect({}, {...base,action:'models',installation_id:out.observation.installations[0].installation_id,operation_id:'c'.repeat(32)});assert.equal(attempt.status,'failed');assert.equal(h.children.length,0);
});

test('T189 ACP model discovery and recovery keep Cursor and Qoder identities and explicit credentials separate',async t=>{
 const validation=require('../src/local-acp-validation.cjs'),cursor={...base,adapter_id:'cursor-cli'},cursorResult={...result(),runtime:'cursor'};
 assert.equal(validation.validDiscovery(result(),'qoder-acp'),true);assert.equal(validation.validDiscovery(result(),'cursor-cli'),false);
 assert.equal(validation.validDiscovery(cursorResult,'cursor-cli'),true);assert.equal(validation.validDiscovery(cursorResult,'qoder-acp'),false);assert.equal(validation.validDiscovery(cursorResult,'other-acp'),false);
 const stageRoot=path.resolve(__dirname,'../../../.cache/cursor-acp-189/desktop/local');let h,editorOnly=false;
 h=harness(t,{fixtureRoot:stageRoot,discover:async(_event,scope)=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:scope.adapter_id,product:scope.adapter_id==='cursor-cli'?'Cursor CLI':'Qoder CLI',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:scope.adapter_id==='cursor-cli'?path.join(h.directory,editorOnly?'cursor.cmd':process.platform==='win32'?'node.exe':'node'):path.join(h.directory,'qoder.exe'),executable_sha256:'a'.repeat(64),version:scope.adapter_id==='cursor-cli'?'Cursor reviewed':'Qoder reviewed'}]}})});
 fs.writeFileSync(path.join(h.directory,process.platform==='win32'?'node.exe':'node'),'synthetic-node-bytes');fs.writeFileSync(path.join(h.directory,'index.js'),'// Synthetic package entry; never executed.');
 t.after(()=>{const relative=path.relative(stageRoot,h.directory);assert.ok(relative.startsWith('case-')&&!relative.includes(path.sep));fs.rmSync(h.directory,{recursive:true,force:true});});
 const installation=await h.detect(),request={...base,action:'models',installation_id:installation.installation_id,operation_id:'d'.repeat(32),credential_env_ref:'LOCAL_QODER_CREDENTIAL'},pending=h.controller.inspect({},request);await h.started();h.complete();const original=await pending;
 const saved=structuredClone(h.operations.get(request.operation_id));
 assert.equal((await h.controller.inspect({}, {...cursor,action:'status'})).observation.phase,'unconfigured');
 assert.equal((await h.controller.inspect({}, {...cursor,action:'status',operation_id:request.operation_id})).status,'failed');
 const next=await h.controller.inspect({}, {...cursor,action:'models',installation_id:installation.installation_id,operation_id:'e'.repeat(32),credential_env_ref:'LOCAL_QODER_CREDENTIAL'});
 assert.equal(next.status,'failed');assert.equal(h.children.length,1);assert.deepEqual(h.operations.get(request.operation_id),saved);
 const detected=await h.controller.inspect({}, {...cursor,action:'detect'});assert.equal(detected.observation.phase,'detected');const cursorInstallation=detected.observation.installations[0];
 const cursorRequest={...cursor,action:'models',installation_id:cursorInstallation.installation_id,operation_id:'f'.repeat(32),credential_env_ref:'LOCAL_CURSOR_CREDENTIAL'};
 assert.equal((await h.controller.inspect({}, {...cursorRequest,credential_env_ref:undefined})).status,'failed');assert.equal(h.children.length,1);
 h.environment.LOCAL_CURSOR_CREDENTIAL='explicit-cursor-value';h.environment.CURSOR_API_KEY='ambient-not-reviewed';
 const cursorPending=h.controller.inspect({},cursorRequest);for(let i=0;i<100&&h.children.length<2;i++)await new Promise(setImmediate);assert.equal(h.children.length,2);
 assert.equal(h.children[1].input().adapter_id,'cursor-cli');assert.equal(h.children[1].input().credential_env_ref,'LOCAL_CURSOR_CREDENTIAL');assert.equal(h.children[1].opts.env.LOCAL_CURSOR_CREDENTIAL,'explicit-cursor-value');assert.equal(h.children[1].opts.env.CURSOR_API_KEY,undefined);assert.equal(h.children[1].opts.env.LOCAL_QODER_CREDENTIAL,undefined);
 const restarted=h.make();t.after(()=>restarted.dispose());assert.equal((await restarted.inspect({},cursorRequest)).observation.phase,'unknown');assert.equal(h.children.length,2);
 h.complete({...cursorResult,tool_version:'Cursor reviewed'});const cursorReady=await cursorPending;assert.equal(cursorReady.observation.phase,'ready');assert.equal(cursorReady.observation.adapter_id,'cursor-cli');
 const cursorSelection={installation_id:cursorInstallation.installation_id,probe_id:cursorReady.observation.probe.id,model_id:'one'};
 assert.equal((await h.controller.select(cursor,cursorSelection,h.context)).credential_env_ref,'LOCAL_CURSOR_CREDENTIAL');
 await assert.rejects(h.controller.select(base,cursorSelection,h.context));await assert.rejects(h.controller.select(cursor,{installation_id:installation.installation_id,probe_id:original.observation.probe.id,model_id:'one'},h.context));
 const originalCursor=structuredClone(h.operations.get(cursorRequest.operation_id));assert.equal((await h.controller.inspect({}, {...cursorRequest,credential_env_ref:'LOCAL_QODER_CREDENTIAL'})).status,'failed');assert.deepEqual(h.operations.get(cursorRequest.operation_id),originalCursor);assert.equal(h.children.length,2);
 editorOnly=true;assert.notEqual((await h.controller.inspect({}, {...cursor,action:'detect'})).observation.phase,'detected');await assert.rejects(h.controller.select(cursor,cursorSelection,h.context));
 assert.equal((await h.controller.inspect({}, {...base,action:'status',operation_id:request.operation_id})).observation.probe.id,original.observation.probe.id);
});
test('T191 Cursor discovery adopts only complete post-RPC stop evidence and preserves the actual host exit',async t=>{
 const {validDiscovery}=require('../src/local-acp-validation.cjs'),cursor={...base,adapter_id:'cursor-cli'},value={...result(),runtime:'cursor',tool_version:'Cursor reviewed'};
 Object.assign(value.evidence,{session_cancel_requested:true,session_cancel_written:true,host_process_exit_code:1,host_stopped_after_rpc:true});
 assert.equal(validDiscovery(value,'cursor-cli'),true);assert.equal(validDiscovery({...value,runtime:'qoder'},'qoder-acp'),false);
 for(const host_process_exit_code of [-1,0,4294967295])assert.equal(validDiscovery({...value,evidence:{...value.evidence,host_process_exit_code}},'cursor-cli'),true);
 for(const patch of [{host_stopped_after_rpc:false},{host_process_exit_code:undefined},{host_process_exit_code:null},{host_process_exit_code:-2},{host_process_exit_code:1.5},{host_process_exit_code:4294967296},{session_cancel_requested:false},{session_cancel_written:false},{session_created:false},{host_process_stopped:false},{host_process_tree_stopped:false},{prompt_requests:1},{configuration_writes:1},{permission_requests:1},{unexpected_activity:1},{session_close_acknowledged:true}])assert.equal(validDiscovery({...value,evidence:{...value.evidence,...patch}},'cursor-cli'),false,JSON.stringify(patch));
 const missing=structuredClone(value);delete missing.evidence.host_stopped_after_rpc;assert.equal(validDiscovery(missing,'cursor-cli'),false);
 const {catalogue:_,...partial}=value,history={...partial,state:'unknown',error_code:'acp_discovery_unknown',evidence:{...partial.evidence,host_stopped_after_rpc:false,permission_requests:4096}};assert.equal(validDiscovery(history,'cursor-cli'),true);assert.equal(validDiscovery({...history,state:'ready'},'cursor-cli'),false);assert.equal(validDiscovery({...history,evidence:{...history.evidence,host_process_stopped:false,host_process_tree_stopped:false}},'cursor-cli'),false);
 const stageRoot=path.resolve(__dirname,'../../../.cache/cursor-setup-191/desktop/discovery');let h;
 h=harness(t,{fixtureRoot:stageRoot,discover:async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'cursor-cli',product:'Cursor CLI',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:path.join(h.directory,process.platform==='win32'?'node.exe':'node'),executable_sha256:'a'.repeat(64),version:'Cursor reviewed'}]}})});
 fs.writeFileSync(path.join(h.directory,process.platform==='win32'?'node.exe':'node'),'synthetic-node-bytes');fs.writeFileSync(path.join(h.directory,'index.js'),'// Synthetic package entry; never executed.');h.environment.LOCAL_CURSOR_CREDENTIAL='synthetic-cursor-value';
 const detected=await h.controller.inspect({}, {...cursor,action:'detect'}),installation=detected.observation.installations[0],request={...cursor,action:'models',installation_id:installation.installation_id,operation_id:'e'.repeat(32),credential_env_ref:'LOCAL_CURSOR_CREDENTIAL'},pending=h.controller.inspect({},request);await h.started();h.complete(value);
 const observed=await pending;assert.equal(observed.observation.phase,'ready');assert.equal(observed.observation.probe.discovery.evidence.host_process_exit_code,1);assert.equal(observed.observation.probe.discovery.evidence.host_stopped_after_rpc,true);
 assert.equal((await h.controller.inspect({},request)).observation.probe.id,observed.observation.probe.id);assert.equal(h.children.length,1);assert.equal((await h.controller.select(cursor,{installation_id:installation.installation_id,probe_id:observed.observation.probe.id,model_id:'one'},h.context)).model,'one');
 assert.equal(JSON.stringify(observed).includes('synthetic-cursor-value'),false);
});
