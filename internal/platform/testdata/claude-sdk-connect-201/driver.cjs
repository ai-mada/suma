'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{EventEmitter}=require('node:events'),{spawn}=require('node:child_process');
const root=path.resolve(__dirname,'../..'),input=JSON.parse(fs.readFileSync(0,'utf8'));
const {createManagedLocalRunnerController}=require(path.join(root,'apps/desktop/src/managed-local-runner.cjs'));
const {createInstalledToolDiscoveryController}=require(path.join(root,'apps/desktop/src/installed-tools.cjs'));
const hash=p=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex'),digest=s=>crypto.createHash('sha256').update(s).digest('hex');
assert.equal(path.dirname(input.directory),__dirname);assert.ok(['actual','actual-final'].includes(path.basename(input.directory)));
const origin=input.origin,key=Buffer.from(input.key_base64,'base64'),scope={project_id:input.project_id,user_id:input.user_id,tenant_id:input.tenant_id,adapter_id:'claude-code-sdk',locale:'en-US'};
assert.equal(key.length,32);assert.ok(/^http:\/\/127\.0\.0\.1:\d+$/.test(origin));
const children=[],requests=[],registrationBodies=[],facts={passed:false,scope:'Real owned desktop controller/HTTP/SQLite/Go Worker and reviewed SDK resources. Version-only owned native fixture, no SDK query, vendor/model, actual Electron window or production acceptance.',model_calls:0,vendor_calls:0,safe_storage:'owned AES-GCM fixture, not OS DPAPI'};
let dropped=false,controller,discovery;
const record=()=>fs.writeFileSync(path.join(input.directory,'partial.json'),JSON.stringify({...facts,requests,children:children.map(({pid,kind,exit})=>({pid,kind,exit}))},null,2));
const nativeSHA=hash(input.native_cli),environment={};
for(const [name,value]of Object.entries(process.env))if(['path','systemroot','windir','comspec','pathext','programfiles','programfiles(x86)','programw6432'].includes(name.toLowerCase()))environment[name]=value;
const isolatedHome=path.join(input.directory,'isolated-home');fs.mkdirSync(isolatedHome,{recursive:true});
for(const field of ['HOME','USERPROFILE','APPDATA','LOCALAPPDATA'])environment[field]=isolatedHome;
const oldPath=Object.keys(environment).find(k=>k.toLowerCase()==='path');const originalPath=oldPath?environment[oldPath]:'';if(oldPath)delete environment[oldPath];environment.PATH=path.dirname(input.native_cli)+path.delimiter+originalPath;
environment.OWNED_CLAUDE_CONNECT_KEY='owned-fixture-not-a-provider-key';
const encryptedKey=crypto.createHash('sha256').update('suma201-owned-safe-storage').update(key).digest();
const safeStorage={isEncryptionAvailable:()=>true,getSelectedStorageBackend:()=> 'owned-fixture',encryptString(value){const iv=crypto.randomBytes(12),cipher=crypto.createCipheriv('aes-256-gcm',encryptedKey,iv);const data=Buffer.concat([cipher.update(value,'utf8'),cipher.final()]);return Buffer.concat([iv,cipher.getAuthTag(),data]);},decryptString(value){const decipher=crypto.createDecipheriv('aes-256-gcm',encryptedKey,value.subarray(0,12));decipher.setAuthTag(value.subarray(12,28));return Buffer.concat([decipher.update(value.subarray(28)),decipher.final()]).toString('utf8');}};
const parent=new EventEmitter(),contents=new EventEmitter();parent.isDestroyed=contents.isDestroyed=()=>false;contents.getURL=()=>origin+'/';contents.mainFrame={url:origin+'/'};
contents.session={async fetch(url,options={}){assert.equal(new URL(url).origin,origin);const headers=new Headers(options.headers);headers.set('Cookie',input.cookie);const response=await fetch(url,{...options,headers});requests.push({method:options.method||'GET',path:new URL(url).pathname,status:response.status});if(new URL(url).pathname.endsWith('/desktop-runners/register')&&options.method==='POST'){assert.equal(typeof options.body,'string');registrationBodies.push(digest(options.body));if(response.status===201&&!dropped){dropped=true;await response.arrayBuffer();record();return new Response('{"worker":',{status:201,headers:{'Content-Type':'application/json'}});}}record();return response;}};
parent.webContents=contents;const event={sender:contents,senderFrame:contents.mainFrame};
function ownedSpawn(file,args,options){const child=spawn(file,args,options),item={pid:child.pid,kind:args.includes('--desktop-managed-runner')?'managed-worker':'read-only-helper'};children.push(item);item.closed=new Promise(resolve=>child.once('close',(code,signal)=>{item.exit={code,signal};record();resolve();}));return child;}
function createController(){return createManagedLocalRunnerController({parent,origin,binary:input.binary,key,userData:path.join(input.directory,'user-data'),safeStorage,discover:discovery.discover,spawn:ownedSpawn,environment,checkIntervalMS:15000});}
const api=async(method,suffix,body)=>{const response=await contents.session.fetch(origin+'/api/v1'+suffix,{method,redirect:'error',headers:{'Content-Type':'application/json','X-Suma-Expected-User':scope.user_id,'X-Suma-Expected-Organization':scope.tenant_id},...(body?{body:JSON.stringify(body)}:{})});assert.ok(response.status===200||response.status===201,method+' '+suffix+' status '+response.status);return response.json();};
const manage=async request=>{const value=await controller.manage(event,{...scope,...request});assert.equal(value.status,'ok');record();return value.connection;};
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms));
async function main(){
 try{
  fs.mkdirSync(path.join(input.directory,'scan-tmp'),{recursive:true});
  discovery=createInstalledToolDiscoveryController({parent,origin,binary:input.binary,tempDirectory:path.join(input.directory,'scan-tmp'),spawn:ownedSpawn,environment});controller=createController();
  const setup={executable_sha256:nativeSHA,model:'claude-sonnet-5',credential_env_ref:'OWNED_CLAUDE_CONNECT_KEY'};
  let value=await manage({action:'connect',claude_sdk_setup:setup});assert.equal(value.phase,'issuance_unknown');assert.equal(dropped,true);assert.equal(registrationBodies.length,1);assert.equal(children.filter(c=>c.kind==='managed-worker').length,0);
  // Recovery intentionally sends no new setup. The original encrypted journal
  // and same registration body/key recover the already committed issuance.
  value=await manage({action:'connect'});assert.ok(['starting','connected'].includes(value.phase));assert.equal(registrationBodies.length,2);assert.equal(registrationBodies[0],registrationBodies[1]);
  const deadline=Date.now()+25000;while(value.phase!=='connected'){assert.ok(Date.now()<deadline,'fresh Worker inventory did not arrive');await sleep(150);value=await manage({action:'status'});assert.ok(['starting','connected'].includes(value.phase));}
  assert.equal(value.model,setup.model);assert.equal(value.credential_env_ref,setup.credential_env_ref);assert.equal(value.tool_version,'2.1.314');assert.equal(value.executable_sha256,nativeSHA);assert.ok(/^[a-f0-9]{64}$/.test(value.runtime_executable_sha256));assert.notEqual(value.runtime_executable_sha256,nativeSHA);assert.equal(value.claude_sdk_execution.model,setup.model);assert.equal(value.claude_sdk_execution.configuration_sha256,value.configuration_sha256);
  const inventory=await api('GET','/tool-discovery?'+new URLSearchParams({project_id:scope.project_id,worker_id:value.worker_id}));const rows=inventory.items.filter(row=>row.available===true&&row.profile?.adapter==='claude'&&row.profile?.protocol==='claude-sdk-v1');assert.equal(rows.length,1);const profile=rows[0].profile;
  assert.equal(profile.tool_version,'2.1.314');assert.equal(profile.executable_sha256,value.runtime_executable_sha256);assert.deepEqual(profile.claude_sdk_execution,value.claude_sdk_execution);assert.equal(JSON.stringify(profile).includes(setup.credential_env_ref),false);
  const body={project_id:scope.project_id,worker_id:value.worker_id,name:'Owned Claude SDK connect201',profile,idempotency_key:'claude-sdk-connect-201-profile'};
  const registered=await api('POST','/tool-profiles',body);assert.equal(registered.state,'paused');assert.equal(registered.version,1);const replay=await api('POST','/tool-profiles',body);assert.equal(replay.id,registered.id);
  Object.assign(facts,{worker_id:value.worker_id,profile_id:registered.id,profile_state:registered.state,profile_version:registered.version,configuration_sha256:value.configuration_sha256,native_cli_sha256:nativeSHA,node_sha256:value.runtime_executable_sha256,original_register_receipt_recovered:true,registration_request_body_sha256:registrationBodies[0],registration_sends:registrationBodies.length,profile_replay_same_id:true});record();
  assert.equal(children.filter(c=>c.kind==='managed-worker').length,1);
  value=await manage({action:'stop'});assert.equal(value.phase,'stopped');await controller.dispose();controller=undefined;
  await Promise.all(children.map(c=>c.closed));for(const child of children){assert.equal(child.exit?.code,0);assert.equal(child.exit?.signal,null);assert.throws(()=>process.kill(child.pid,0),error=>error.code==='ESRCH');}
  controller=createController();const restored=await manage({action:'status'});assert.equal(restored.phase,'stopped');assert.equal(restored.worker_id,facts.worker_id);assert.equal(restored.credential_env_ref,setup.credential_env_ref);assert.equal(restored.configuration_sha256,facts.configuration_sha256);await controller.dispose();controller=undefined;
  const probes=fs.readFileSync(path.join(path.dirname(input.native_cli),'version-probes.jsonl'),'utf8').trim().split(/\r?\n/).map(row=>JSON.parse(row));assert.ok(probes.length>=1);assert.ok(probes.every(row=>row.version_only===true));for(const probe of probes)assert.throws(()=>process.kill(probe.pid,0),error=>error.code==='ESRCH');
  Object.assign(facts,{passed:true,stopped:true,persistent_stopped_scope_restored:true,native_calls_version_only:true,native_version_calls:probes.length,worker_processes:1,child_exits:children.map(({pid,kind,exit})=>({pid,kind,exit})),requests});fs.writeFileSync(path.join(input.directory,'receipt.json'),JSON.stringify(facts,null,2));console.log(JSON.stringify({passed:true,worker_processes:1,original_receipt_recovered:true,model_calls:0}));
 }catch(error){facts.error=String(error.stack||error);record();throw error;}
 finally{if(controller)await controller.dispose();if(discovery)await discovery.dispose();await Promise.all(children.map(c=>c.closed));}
}
main().catch(error=>{console.error(String(error.stack||error));process.exitCode=1;});
