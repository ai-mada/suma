'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events');
const {createInstalledToolDiscoveryController,validInstalledToolReport}=require('../src/installed-tools.cjs');
const origin='http://127.0.0.1:4317',request={project_id:'1'.repeat(32),user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'codex-jsonl',locale:'en-US'};
const report=()=>({protocol:'suma-installed-tools-v1',items:[{adapter_id:'codex-jsonl',product:'Codex',installed:true,launch_supported:true,executable:process.execPath,version:'1.0.0',reason_code:'workspace_required',command:{executable:'/private/local',args:[]}}]});
function harness(options={}){
 const frame={url:origin+'/'},contents=new EventEmitter(),parent=new EventEmitter(),calls=[],children=[];let identity=request.user_id;
 Object.assign(contents,{mainFrame:frame,isDestroyed:()=>false,getURL:()=>origin+'/',session:{fetch:async(url,options)=>{calls.push({url,options});return Response.json(url.includes('/auth/session')?{user:{id:identity},organization:{id:request.tenant_id}}:{project_id:request.project_id,configured:false});}}});
 Object.assign(parent,{webContents:contents,isDestroyed:()=>false});
 const spawn=(binary,args,options)=>{const child=new EventEmitter();Object.assign(child,{stdout:new EventEmitter(),stderr:new EventEmitter(),pid:123,kill:()=>{child.emit('close',null,'SIGTERM');return true;}});children.push({binary,args,options,child});return child;};
 const controller=createInstalledToolDiscoveryController({parent,origin,binary:process.execPath,tempDirectory:process.cwd(),spawn,environment:{PATH:process.env.PATH,SystemRoot:process.env.SystemRoot,SUMA_RUNNER_TOKEN:'never-forward',OPENAI_API_KEY:'never-forward',NODE_OPTIONS:'never-forward'},...options});
 const event={sender:contents,senderFrame:frame};
 const complete=(value=report(),code=0)=>{const child=children.at(-1).child;child.stdout.emit('data',Buffer.from(JSON.stringify(value)));child.emit('close',code,null);};
 return {parent,contents,event,controller,calls,children,complete,setIdentity:value=>identity=value};
}
const tick=()=>new Promise(setImmediate);
test('rejects wrong frame, arbitrary launcher and added IPC options before any I/O',async()=>{
 const h=harness();for(const [event,body] of [[{...h.event,senderFrame:{url:origin}},request],[h.event,{...request,adapter_id:'shell'}],[h.event,{...request,executable:'powershell'}]])assert.equal((await h.controller.discover(event,body)).status,'failed');assert.equal(h.calls.length,0);assert.equal(h.children.length,0);h.controller.dispose();
});
test('scan is current-project authorized, fixed-argument and strips command/credentials',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();assert.equal(h.children.length,1);const child=h.children[0];assert.equal(child.binary,process.execPath);assert.deepEqual(child.args,['--language=en-US','--discover-installed-tools','--local-tool','codex-jsonl']);assert.equal(child.options.shell,false);assert.equal(child.options.env.SUMA_RUNNER_TOKEN,undefined);assert.equal(child.options.env.OPENAI_API_KEY,undefined);assert.equal(child.options.env.NODE_OPTIONS,undefined);h.complete();const result=await pending;assert.equal(result.status,'observed');assert.equal(result.report.items[0].command,undefined);assert.equal(h.calls.length,6);assert.ok(h.calls.every(c=>c.options.headers['X-Suma-Expected-User']===request.user_id));h.controller.dispose();
});
test('only one scan runs and a later caller cannot replace the pending scope',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();assert.equal((await h.controller.discover(h.event,{...request,adapter_id:'dsh-multica'})).code,'discovery_busy');h.complete();assert.equal((await pending).status,'observed');h.controller.dispose();
});
test('account change after local observation suppresses all facts',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();h.setIdentity('4'.repeat(32));h.complete();assert.deepEqual(await pending,{status:'failed',code:'discovery_access_changed'});h.controller.dispose();
});
test('disposed or navigated original frame terminates observation without exposing late facts',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();h.contents.mainFrame={url:origin};h.complete();assert.equal((await pending).status,'failed');h.controller.dispose();
 const next=harness(),waiting=next.controller.discover(next.event,request);await tick();next.controller.dispose();assert.equal((await waiting).status,'failed');
});
test('bounded malformed or cross-tool child output never becomes an installation fact',async()=>{
 for(const mutate of [r=>r.items.push(r.items[0]),r=>r.items[0].adapter_id='codebuddy-cli',r=>r.items[0].version='secret\nnext',r=>r.items[0].installed=false,r=>r.protocol='wrong']){const h=harness(),pending=h.controller.discover(h.event,request);await tick();const r=report();mutate(r);h.complete(r);assert.equal((await pending).status,'failed');h.controller.dispose();}
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();h.children[0].child.stdout.emit('data',Buffer.alloc(262145));assert.equal((await pending).status,'failed');h.controller.dispose();
});
test('report validator keeps install, launch support and dispatch readiness separate',()=>{
 const unavailable={protocol:'suma-installed-tools-v1',items:[{adapter_id:'codex-jsonl',product:'Codex',installed:false,launch_supported:false,reason_code:'not_installed'}]};assert.ok(validInstalledToolReport(unavailable,'codex-jsonl'));assert.equal(validInstalledToolReport({...unavailable,available:true},'codex-jsonl'),undefined);
});
test('ACP actual discovery IPC preserves native identity but never grants launch authority',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,{...request,adapter_id:'qoder-acp'});await tick();
 h.complete({protocol:'suma-installed-tools-v1',items:[{adapter_id:'qoder-acp',product:'Qoder CLI',installed:true,launch_supported:false,reason_code:'acp_configuration_required',executable:process.execPath,executable_sha256:'a'.repeat(64),version:'Qoder reviewed'}]});
 const out=await pending;assert.equal(out.status,'observed');assert.equal(out.report.items[0].launch_supported,false);assert.equal(out.report.items[0].executable_sha256,'a'.repeat(64));assert.equal(Object.hasOwn(out.report.items[0],'command'),false);await h.controller.dispose();
});

const cursorReport=(reason,extra={})=>({protocol:'suma-installed-tools-v1',items:[{adapter_id:'cursor-cli',product:'Cursor',installed:['cursor_agent_unverified','cursor_launcher_unsupported'].includes(reason),launch_supported:false,reason_code:reason,...extra}]});
test('Cursor passive observations remain closed and cannot acquire executable authority',()=>{
 for(const reason of ['not_installed','cursor_editor_only','cursor_scan_incomplete','cursor_agent_unverified','cursor_launcher_unsupported']){
  const candidate=reason.endsWith('_unverified')||reason==='cursor_launcher_unsupported',value=cursorReport(reason,candidate?{executable:process.execPath}:{});
  assert.deepEqual(validInstalledToolReport(value,'cursor-cli'),value);
  for(const extra of [{launch_supported:true},{version:'2026.10.22-27dcf5f'},{command:{}},{installed:!candidate},{available:true},{reason_code:'workspace_required'},{executable:candidate?'relative/agent':process.execPath}]){
   assert.equal(validInstalledToolReport({protocol:value.protocol,items:[{...value.items[0],...extra}]},'cursor-cli'),undefined,reason+' '+JSON.stringify(extra));
  }
 }
 assert.equal(validInstalledToolReport(cursorReport('cursor_agent_unverified'),'cursor-cli'),undefined);
 assert.equal(validInstalledToolReport(cursorReport('not_installed'),'codex-jsonl'),undefined);
 const other=report();other.items[0].reason_code='cursor_agent_unverified';assert.equal(validInstalledToolReport(other,'codex-jsonl'),undefined);
});
test('Cursor checks reuse current-project authorization without forwarding command or credentials',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,{...request,adapter_id:'cursor-cli'});await tick();
 assert.equal(h.children.length,1);assert.equal(h.children[0].args.at(-1),'cursor-cli');
 h.complete(cursorReport('cursor_editor_only'));const result=await pending;
 assert.equal(result.status,'observed');assert.equal(result.report.items[0].installed,false);assert.equal(result.report.items[0].launch_supported,false);assert.equal(result.report.items[0].command,undefined);assert.equal(h.calls.length,6);await h.controller.dispose();
});
test('shutdown waits for child close rather than treating a sent signal as proof',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();const child=h.children[0].child;child.kill=()=>true;let settled=false;const disposal=h.controller.dispose().then(()=>settled=true);await tick();assert.equal(settled,false);h.complete();assert.equal((await pending).status,'failed');await disposal;assert.equal(settled,true);
});
test('unconfirmed stopping child keeps admission busy until its real close',async()=>{
 const h=harness(),pending=h.controller.discover(h.event,request);await tick();const child=h.children[0].child;child.kill=()=>true;child.stdout.emit('data',Buffer.alloc(262145));assert.equal((await pending).status,'failed');assert.equal((await h.controller.discover(h.event,request)).code,'discovery_busy');child.emit('close',null,'SIGTERM');const next=h.controller.discover(h.event,request);await tick();h.complete();assert.equal((await next).status,'observed');await h.controller.dispose();
});
test('deadline reached during authorization never starts a local process',async()=>{
 const h=harness({deadlineMS:1});let release;const original=h.contents.session.fetch;h.contents.session.fetch=async(url,options)=>{if(!release)return new Promise(resolve=>release=()=>resolve(Response.json({user:{id:request.user_id},organization:{id:request.tenant_id}})));return original(url,options);};const pending=h.controller.discover(h.event,request);await new Promise(resolve=>setTimeout(resolve,10));release();assert.equal((await pending).status,'failed');assert.equal(h.children.length,0);await h.controller.dispose();
});
test('preload installation checks use only the fixed channel and hide transport diagnostics',async()=>{
 const fs=require('node:fs'),vm=require('node:vm');let bridge,failed=false;const calls=[];
 vm.runInNewContext(fs.readFileSync(require.resolve('../src/preload.cjs'),'utf8'),{process:{platform:'win32'},require:name=>{assert.equal(name,'electron');return {contextBridge:{exposeInMainWorld:(_key,value)=>bridge=value},ipcRenderer:{invoke:async(...args)=>{calls.push(args);if(failed)throw Error('private');return {status:'observed'};}}};}});
 assert.equal((await bridge.discoverInstalledTools(request)).status,'observed');assert.equal(calls[0][0],'suma:discover-installed-tools');assert.equal(calls[0][1],request);failed=true;assert.equal((await bridge.discoverInstalledTools(request)).code,'desktop_unavailable');
});
