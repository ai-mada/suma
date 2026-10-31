'use strict';
const {spawn:spawnChild}=require('node:child_process');
const path=require('node:path');
const crypto=require('node:crypto'),{performance}=require('node:perf_hooks');
const {strictJSON}=require('./local-acp-validation.cjs');

const CHANNEL='suma:discover-installed-tools';
const PRODUCTS=Object.freeze({'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code','cursor-cli':'Cursor','qoder-acp':'Qoder CLI','codex-app-server':'Codex','opencode-sdk':'OpenCode'});
const ADAPTERS=Object.freeze(Object.keys(PRODUCTS));
const REASONS=new Set(['not_installed','unsupported_launcher','invalid_package','node_unavailable','probe_failed','workspace_required']);
const CURSOR_REASONS=new Set(['not_installed','cursor_editor_only','cursor_agent_unverified','cursor_launcher_unsupported','cursor_scan_incomplete']);
const ID=/^[a-f0-9]{32}$/;
const plain=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const exact=(value,keys)=>plain(value)&&Object.keys(value).sort().join(',')===[...keys].sort().join(',');
const text=(value,max)=>typeof value==='string'&&value.length>0&&Buffer.byteLength(value)<=max&&!/\p{Cc}/u.test(value);
const failure=code=>({status:'failed',code});

// Local command definitions are deliberately stripped at this boundary. The
// renderer receives installation observations, never process-launch authority.
function validInstalledToolReport(value,adapterID){
 if(!exact(value,['protocol','items'])||value.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.items)||value.items.length!==1)return;
 const row=value.items[0];
 if(!plain(row)||Object.keys(row).some(key=>!['adapter_id','product','installed','launch_supported','executable','version','reason_code','command','executable_sha256'].includes(key))||row.adapter_id!==adapterID||row.product!==PRODUCTS[adapterID]||typeof row.installed!=='boolean'||typeof row.launch_supported!=='boolean')return;
 if(row.executable_sha256!==undefined&&(!['qoder-acp','codex-app-server','opencode-sdk'].includes(adapterID)||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(row.executable!==undefined&&(!text(row.executable,4096)||!path.isAbsolute(row.executable)))return;
 // Cursor currently exposes passive file observations only. Even a well-formed
 // executable is not vendor identity, a verified protocol or dispatch authority.
 // Reject command/version fields rather than silently downgrading a contradictory
 // report into an apparently trusted connection fact.
 if(adapterID==='cursor-cli'){
  if(!CURSOR_REASONS.has(row.reason_code)||row.launch_supported!==false||Object.hasOwn(row,'command')||Object.hasOwn(row,'version'))return;
  const candidate=row.reason_code==='cursor_agent_unverified'||row.reason_code==='cursor_launcher_unsupported';
  if(row.installed!==candidate||candidate!==Object.hasOwn(row,'executable')||candidate&&!row.executable)return;
 }else if(adapterID==='qoder-acp'||adapterID==='codex-app-server'||adapterID==='opencode-sdk'){
  const configured=adapterID==='qoder-acp'?'acp_configuration_required':'configuration_required';
  if(row.launch_supported||Object.hasOwn(row,'command')||!['not_installed','unsupported_launcher','probe_failed',configured,...(adapterID==='opencode-sdk'?['version_unsupported']:[])].includes(row.reason_code))return;
  if(row.reason_code===configured&&(!row.installed||!row.executable||!row.version||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
  if(adapterID==='opencode-sdk'&&(row.reason_code===configured&&row.version!=='1.19.4'||row.reason_code==='version_unsupported'&&(!row.installed||!row.executable||!row.version||row.version==='1.19.4'||!/^[a-f0-9]{64}$/.test(row.executable_sha256))))return;
  if(row.reason_code==='not_installed'&&(row.installed||row.executable!==undefined||row.version!==undefined||row.executable_sha256!==undefined))return;
 }else if(!REASONS.has(row.reason_code))return;
 if(row.version!==undefined&&(!text(row.version,256)||/[\u2028\u2029]/u.test(row.version)))return;
 if(row.launch_supported&&(!row.installed||row.reason_code!=='workspace_required'||!row.executable||!row.version)||row.reason_code==='not_installed'&&row.installed||row.reason_code==='workspace_required'&&!row.launch_supported)return;
 const item={adapter_id:row.adapter_id,product:row.product,installed:row.installed,launch_supported:row.launch_supported,reason_code:row.reason_code};
 if(row.executable!==undefined)item.executable=row.executable;
 if(row.version!==undefined)item.version=row.version;
 if(['qoder-acp','codex-app-server','opencode-sdk'].includes(adapterID)&&row.executable_sha256!==undefined)item.executable_sha256=row.executable_sha256;
 return {protocol:value.protocol,items:[item]};
}
function validInstalledToolsBatch(value){
 if(!exact(value,['protocol','items'])||value.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.items)||value.items.length!==ADAPTERS.length)return;
 const rows=new Map();for(const row of value.items){if(!plain(row)||!ADAPTERS.includes(row.adapter_id)||rows.has(row.adapter_id))return;const checked=validInstalledToolReport({protocol:value.protocol,items:[row]},row.adapter_id);if(!checked)return;rows.set(row.adapter_id,checked.items[0]);}
 return {protocol:value.protocol,items:ADAPTERS.map(id=>rows.get(id))};
}
async function boundedJSON(response){
 if(!response?.ok||response.status!==200||response.redirected||!/^application\/json(?:;|$)/i.test(response.headers.get('content-type')||''))throw Error('response');
 const reader=response.body?.getReader();if(!reader)throw Error('response');const chunks=[];let length=0;
 try{for(;;){const part=await reader.read();if(part.done)break;length+=part.value.byteLength;if(length>65536)throw Error('limit');chunks.push(Buffer.from(part.value));}}
 catch(error){await reader.cancel().catch(()=>{});throw error;}finally{reader.releaseLock();}
 return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(Buffer.concat(chunks)));
}
function scanEnvironment(environment,tempDirectory){
 const allowed=new Set(['path','systemroot','windir','comspec','home','userprofile','homedrive','homepath','appdata','localappdata','lang','lc_all']);
 const clean={};for(const [key,value] of Object.entries(environment))if(allowed.has(key.toLowerCase())&&typeof value==='string')clean[key]=value;
 // Overrides are process-local and never touch the user's environment.
 return {...clean,TEMP:tempDirectory,TMP:tempDirectory,TMPDIR:tempDirectory};
}
function createInstalledToolDiscoveryController({parent,origin,binary,tempDirectory,spawn=spawnChild,environment=process.env,deadlineMS=20000,now=Date.now,monotonicNow=()=>performance.now()}){
 let disposed=false,active,cache,lastScope;
 const scopeKey=request=>[request.user_id,request.tenant_id,request.project_id].join(':');
 function useScope(request){const key=scopeKey(request);if(lastScope!==key){lastScope=key;cache=undefined;}return key;}
 function fresh(value){const elapsed=monotonicNow()-value.observedMono;return Number.isFinite(elapsed)&&elapsed>=0&&elapsed<60000;}
 function projectResult(value,request,cached=false){
  const report=structuredClone(value.report);if(request.action!=='scan')return {status:'observed',report:{protocol:report.protocol,items:[report.items.find(row=>row.adapter_id===request.adapter_id)]}};
  return {status:'observed',report,scan:{...value.scan,cached}};
 }
 const platform=new URL(origin);
 if(platform.origin!==origin||platform.protocol!=='http:'||platform.hostname!=='127.0.0.1'||!platform.port||!path.isAbsolute(binary)||!path.isAbsolute(tempDirectory))throw Error('Invalid desktop discovery configuration');
 const caller=event=>{
  try{return !disposed&&!parent.isDestroyed()&&!parent.webContents.isDestroyed()&&event.sender===parent.webContents&&event.senderFrame===parent.webContents.mainFrame&&new URL(event.senderFrame.url).origin===origin&&new URL(parent.webContents.getURL()).origin===origin;}catch{return false;}
 };
 function terminate(record,signal){try{if(process.platform!=='win32'&&record.child.pid)process.kill(-record.child.pid,signal);else record.child.kill(signal);}catch{/* close, not kill's return value, proves termination */}}
 function stop(record){
  record.abort.abort();if(!record.child||record.closed||record.stopping)return;
  record.stopping=true;terminate(record,'SIGTERM');
  // Go observes SIGTERM and cancels its separately contained CLI probes. Only
  // escalate after that cleanup window; admission stays closed until close.
  record.killTimer=setTimeout(()=>{if(!record.closed)terminate(record,'SIGKILL');},1500);record.killTimer.unref?.();
  record.stopTimer=setTimeout(()=>{if(!record.closed)record.fail?.(Error('stop_unconfirmed'));},3500);record.stopTimer.unref?.();
 }
 function dispose(){cache=undefined;lastScope=undefined;if(!disposed){disposed=true;if(active)stop(active);parent.removeListener('closed',dispose);parent.webContents.removeListener('destroyed',dispose);}if(!active)return Promise.resolve();const record=active;return new Promise(resolve=>{const timer=setTimeout(resolve,4000);record.settled.then(()=>{clearTimeout(timer);resolve();});});}
 parent.on('closed',dispose);parent.webContents.on('destroyed',dispose);
 async function authorize(record){
  const headers={Accept:'application/json','X-Suma-Expected-User':record.request.user_id,'X-Suma-Expected-Organization':record.request.tenant_id};
  const options={method:'GET',credentials:'include',redirect:'error',cache:'no-store',headers,signal:record.abort.signal};
  const current=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/auth/session',options));
  if(!caller(record.event)||current?.user?.id!==record.request.user_id||current?.organization?.id!==record.request.tenant_id)throw Error('authority');
  const project=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/workspace?'+new URLSearchParams({project_id:record.request.project_id}),options));
  if(!caller(record.event)||project?.project_id!==record.request.project_id||typeof project.configured!=='boolean')throw Error('authority');
  // Expected-view headers are only hints for ordinary GETs. Read the actual
  // browser session again instead of interpreting those headers as authority.
  const after=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/auth/session',options));
  if(!caller(record.event)||after?.user?.id!==record.request.user_id||after?.organization?.id!==record.request.tenant_id)throw Error('authority');
 }
 function observe(record){
  return new Promise((resolve,reject)=>{
   if(record.abort.signal.aborted)return reject(Error('cancelled'));
   let done=false,length=0,chunks=[];
   const finish=(error,value)=>{if(done)return;done=true;chunks=[];record.abort.signal.removeEventListener('abort',aborted);error?reject(error):resolve(value);};
   record.fail=error=>finish(error);
   const aborted=()=>stop(record);
   record.abort.signal.addEventListener('abort',aborted,{once:true});
   try{
    if(record.abort.signal.aborted)return finish(Error('cancelled'));
    const args=['--language='+record.request.locale,'--discover-installed-tools'];if(!record.batch)args.push('--local-tool',record.request.adapter_id);
    const child=spawn(binary,args,{cwd:tempDirectory,env:scanEnvironment(environment,tempDirectory),stdio:['ignore','pipe','ignore'],windowsHide:true,shell:false,detached:process.platform!=='win32'});record.child=child;
    child.on('error',()=>finish(Error('spawn')));
    child.stdout.on('data',chunk=>{if(done)return;length+=chunk.length;if(length>262144){finish(Error('limit'));stop(record);}else chunks.push(Buffer.from(chunk));});
    child.once('close',code=>{record.closed=true;clearTimeout(record.killTimer);clearTimeout(record.stopTimer);if(record.returned){if(active===record)active=undefined;record.resolveSettled();}if(done)return;if(code!==0||record.abort.signal.aborted)return finish(Error('probe'));try{const raw=Buffer.concat(chunks),parsed=record.batch?strictJSON(raw):JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(raw)),value=record.batch?validInstalledToolsBatch(parsed):validInstalledToolReport(parsed,record.request.adapter_id);if(!value)throw Error('report');finish(null,value);}catch{finish(Error('report'));}});
   }catch{finish(Error('spawn'));}
  });
 }
 async function execute(record){
  const timer=setTimeout(()=>stop(record),deadlineMS);timer.unref?.();
  try{
   await authorize(record);if(!caller(record.event))throw Error('authority');const report=await observe(record),observedMono=monotonicNow(),observedAt=now();await authorize(record);if(!caller(record.event)||record.abort.signal.aborted)throw Error('authority');
   const result={status:'observed',report};
   if(record.batch){result.scan={id:record.scanID,project_id:record.request.project_id,user_id:record.request.user_id,tenant_id:record.request.tenant_id,observed_at:new Date(observedAt).toISOString(),expires_at:new Date(observedAt+60000).toISOString(),cached:false};if(lastScope===record.scope&&fresh({observedMono}))cache={scope:record.scope,observedMono,result:structuredClone(result)};}
   return result;
  }
  catch{return failure('discovery_access_changed');}
  finally{clearTimeout(timer);record.returned=true;if(!record.child||record.closed){if(active===record)active=undefined;record.resolveSettled();}}
 }
 async function joined(record,event,request){
  // The owner's timer ends with its scan. A join's independent HTTP reads may
  // finish later, so bound them without giving that join control of the child.
  const participant={event,request,abort:new AbortController()},ownerAborted=()=>participant.abort.abort(),timer=setTimeout(()=>participant.abort.abort(),deadlineMS);timer.unref?.();
  record.abort.signal.addEventListener('abort',ownerAborted,{once:true});if(record.abort.signal.aborted)ownerAborted();
  try{await authorize(participant);const result=await record.result;if(result.status!=='observed')return result;await authorize(participant);if(!caller(event)||participant.abort.signal.aborted)throw Error('authority');return projectResult(result,request);}catch{return failure('discovery_access_changed');}finally{clearTimeout(timer);record.abort.signal.removeEventListener('abort',ownerAborted);}
 }
 async function cachedResult(value,event,request){
  const participant={event,request,abort:new AbortController()},timer=setTimeout(()=>participant.abort.abort(),deadlineMS);timer.unref?.();
  try{await authorize(participant);if(!caller(event)||participant.abort.signal.aborted)throw Error('authority');if(cache!==value||lastScope!==value.scope||!fresh(value))return discover(event,request);return projectResult(value.result,request,true);}catch{if(cache===value)cache=undefined;return failure('discovery_access_changed');}finally{clearTimeout(timer);}
 }
 async function discover(event,request){
  const batch=request?.action==='scan',base=['project_id','user_id','tenant_id','locale'];
  const shape=batch?(exact(request,[...base,'action'])||exact(request,[...base,'action','force'])&&typeof request.force==='boolean'):exact(request,[...base,'adapter_id'])&&typeof request.adapter_id==='string'&&Object.hasOwn(PRODUCTS,request.adapter_id);
  if(!caller(event)||!shape||!['project_id','user_id','tenant_id'].every(key=>typeof request[key]==='string'&&ID.test(request[key]))||!['zh-CN','en-US'].includes(request.locale))return failure('discovery_request_rejected');
  const scope=useScope(request);
  if(active)return active.batch&&active.scope===scope?joined(active,event,{...request}):failure('discovery_busy');
  if(batch&&!request.force&&cache?.scope===scope&&fresh(cache))return cachedResult(cache,event,{...request});
  const record={event,request:{...request},batch,scope,scanID:batch?crypto.randomBytes(16).toString('hex'):undefined,abort:new AbortController(),child:null,closed:false,returned:false,settled:null,resolveSettled:null};record.settled=new Promise(resolve=>record.resolveSettled=resolve);active=record;record.result=execute(record);return record.result;
 }
 return {discover,dispose};
}
module.exports={CHANNEL,createInstalledToolDiscoveryController,validInstalledToolReport};
