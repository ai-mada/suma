'use strict';
const {spawn:spawnChild}=require('node:child_process');
const path=require('node:path');

const CHANNEL='suma:discover-installed-tools';
const PRODUCTS=Object.freeze({'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code'});
const REASONS=new Set(['not_installed','unsupported_launcher','invalid_package','node_unavailable','probe_failed','workspace_required']);
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
 if(!plain(row)||Object.keys(row).some(key=>!['adapter_id','product','installed','launch_supported','executable','version','reason_code','command'].includes(key))||row.adapter_id!==adapterID||row.product!==PRODUCTS[adapterID]||typeof row.installed!=='boolean'||typeof row.launch_supported!=='boolean'||!REASONS.has(row.reason_code))return;
 if(row.executable!==undefined&&(!text(row.executable,4096)||!path.isAbsolute(row.executable)))return;
 if(row.version!==undefined&&(!text(row.version,256)||/[\u2028\u2029]/u.test(row.version)))return;
 if(row.launch_supported&&(!row.installed||row.reason_code!=='workspace_required'||!row.executable||!row.version)||row.reason_code==='not_installed'&&row.installed||row.reason_code==='workspace_required'&&!row.launch_supported)return;
 const item={adapter_id:row.adapter_id,product:row.product,installed:row.installed,launch_supported:row.launch_supported,reason_code:row.reason_code};
 if(row.executable!==undefined)item.executable=row.executable;
 if(row.version!==undefined)item.version=row.version;
 return {protocol:value.protocol,items:[item]};
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
function createInstalledToolDiscoveryController({parent,origin,binary,tempDirectory,spawn=spawnChild,environment=process.env,deadlineMS=20000}){
 let disposed=false,active;
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
 function dispose(){if(!disposed){disposed=true;if(active)stop(active);parent.removeListener('closed',dispose);parent.webContents.removeListener('destroyed',dispose);}if(!active)return Promise.resolve();const record=active;return new Promise(resolve=>{const timer=setTimeout(resolve,4000);record.settled.then(()=>{clearTimeout(timer);resolve();});});}
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
    const child=spawn(binary,['--language='+record.request.locale,'--discover-installed-tools','--local-tool',record.request.adapter_id],{cwd:tempDirectory,env:scanEnvironment(environment,tempDirectory),stdio:['ignore','pipe','ignore'],windowsHide:true,shell:false,detached:process.platform!=='win32'});record.child=child;
    child.on('error',()=>finish(Error('spawn')));
    child.stdout.on('data',chunk=>{if(done)return;length+=chunk.length;if(length>262144){finish(Error('limit'));stop(record);}else chunks.push(Buffer.from(chunk));});
    child.once('close',code=>{record.closed=true;clearTimeout(record.killTimer);clearTimeout(record.stopTimer);if(record.returned){if(active===record)active=undefined;record.resolveSettled();}if(done)return;if(code!==0||record.abort.signal.aborted)return finish(Error('probe'));try{const value=validInstalledToolReport(JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(Buffer.concat(chunks))),record.request.adapter_id);if(!value)throw Error('report');finish(null,value);}catch{finish(Error('report'));}});
   }catch{finish(Error('spawn'));}
  });
 }
 async function discover(event,request){
  if(!caller(event)||!exact(request,['project_id','user_id','tenant_id','adapter_id','locale'])||!['project_id','user_id','tenant_id'].every(key=>typeof request[key]==='string'&&ID.test(request[key]))||typeof request.adapter_id!=='string'||!Object.hasOwn(PRODUCTS,request.adapter_id)||!['zh-CN','en-US'].includes(request.locale))return failure('discovery_request_rejected');
  if(active)return failure('discovery_busy');
  const record={event,request:{...request},abort:new AbortController(),child:null,closed:false,returned:false,settled:null,resolveSettled:null};record.settled=new Promise(resolve=>record.resolveSettled=resolve);active=record;
  const timer=setTimeout(()=>stop(record),deadlineMS);timer.unref?.();
  try{await authorize(record);if(!caller(event))throw Error('authority');const report=await observe(record);await authorize(record);if(!caller(event))throw Error('authority');return {status:'observed',report};}
  catch{return failure('discovery_access_changed');}
  finally{clearTimeout(timer);record.returned=true;if(!record.child||record.closed){if(active===record)active=undefined;record.resolveSettled();}}
 }
 return {discover,dispose};
}
module.exports={CHANNEL,createInstalledToolDiscoveryController,validInstalledToolReport};
