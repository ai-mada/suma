'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events'),{PassThrough}=require('node:stream'),path=require('node:path'),fs=require('node:fs');
const base={project_id:'1'.repeat(32),user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'qoder-acp',locale:'en-US'};
const fixtureRoot=path.resolve(__dirname,'../../../.cache/acp-quick-connect-171/desktop/tmp');
function result(){return {protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'ready',executable_sha256:'a'.repeat(64),tool_version:'Qoder reviewed',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model',current_value:'one',models:[{id:'one',name:'One',configuration_sha256:'b'.repeat(64)}]},evidence:{session_created:true,session_close_requested:false,session_close_acknowledged:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true}};}
function harness(t,options={}){
 fs.mkdirSync(fixtureRoot,{recursive:true});const directory=fs.mkdtempSync(path.join(fixtureRoot,'case-')),operations=options.operations||new Map(),children=[],context={binding_sha256:'4'.repeat(64),base_commit:'5'.repeat(40),repository_path:directory},environment={PATH:process.env.PATH,LOCAL_QODER_CREDENTIAL:'private-credential',SUMA_AGENT_TOKEN:'must-not-inherit'};let allowed=true,clock=Date.now(),version='Qoder reviewed';
 const journal={readACP:id=>operations.has(id)?structuredClone(operations.get(id)):undefined,writeACP:value=>operations.set(value.operation_id,structuredClone(value))};
 const spawn=(binary,args,opts)=>{const child=new EventEmitter();Object.assign(child,{stdin:new PassThrough(),stdout:new PassThrough(),stderr:new PassThrough(),kill:()=>{child.emit('close',null,'SIGKILL');return true;}});let input='';child.stdin.on('data',b=>input+=b);children.push({child,binary,args,opts,input:()=>JSON.parse(input)});return child;};
 const {createLocalACPController}=require('../src/local-acp.cjs');
 const make=()=>createLocalACPController({binary:process.execPath,probeDirectory:directory,credentialKey:Buffer.alloc(32,7),environment,spawn,caller:()=>allowed,session:async()=>{if(!allowed)throw Error('authority');},authorize:async()=>({...context}),discover:options.discover||(async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'qoder-acp',product:'Qoder CLI',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:path.join(directory,'qoder.exe'),executable_sha256:'a'.repeat(64),version}]}})),journal:()=>journal,now:()=>clock,deadlineMS:1000});
 const controller=make();t.after(async()=>{await controller.dispose();});
 return {controller,make,children,operations,environment,context,event:{},setAllowed:v=>allowed=v,advance:n=>clock+=n,setVersion:v=>version=v,async detect(){return (await controller.inspect({}, {...base,action:'detect'})).observation.installations[0];},async started(){for(let i=0;i<100&&!children.length;i++)await new Promise(setImmediate);assert.equal(children.length,1);},complete(value=result()){children.at(-1).child.stdout.emit('data',Buffer.from(JSON.stringify(value)));children.at(-1).child.emit('close',0);}};
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
