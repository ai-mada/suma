'use strict';
const {spawn:spawnChild}=require('node:child_process'),path=require('node:path'),crypto=require('node:crypto');
const {PROTOCOL,createManagedRunnerJournal,scopeKey,validScope,validOrigin}=require('./managed-local-runner-journal.cjs');
const CHANNEL='suma:manage-local-runner';
const TOOL_PROTOCOLS=Object.freeze({'codex-jsonl':['codex','codex-jsonl-v1'],'codebuddy-cli':['codebuddy','codebuddy-jsonl-v1'],'dsh-multica':['harness','dsh-multica-jsonl-v1'],'claude-code-cli':['claude','claude-jsonl-v1']});
const HASH=/^[a-f0-9]{64}$/,ID=/^[a-f0-9]{32}$/;
const gitUnavailable=error=>error?.status===422&&['desktop_runner_head_unavailable','desktop_runner_repository_unsafe','desktop_runner_repository_unavailable'].includes(error.code);
const plain=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const exact=(value,keys)=>plain(value)&&Object.keys(value).sort().join(',')===[...keys].sort().join(',');
const failure=code=>({status:'failed',code});
function requestScope(request){return {project_id:request.project_id,user_id:request.user_id,tenant_id:request.tenant_id,adapter_id:request.adapter_id};}
function runnerEnvironment(environment,temp){
 const names=new Set(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','programdata','programfiles','programfiles(x86)','programw6432','lang','lc_all']);
 const clean={};for(const [name,value]of Object.entries(environment))if(names.has(name.toLowerCase())&&typeof value==='string')clean[name]=value;
 return {...clean,TEMP:temp,TMP:temp,TMPDIR:temp};
}
async function boundedJSON(response){
 if(!response||response.redirected||!/^application\/json(?:;|$)/i.test(response.headers?.get('content-type')||''))throw Error('response');
 const reader=response.body?.getReader();if(!reader)throw Error('response');const chunks=[];let size=0;
 try{for(;;){const item=await reader.read();if(item.done)break;size+=item.value.byteLength;if(size>262144)throw Error('response_limit');chunks.push(Buffer.from(item.value));}}catch(error){await reader.cancel().catch(()=>{});throw error;}finally{reader.releaseLock();}
 const value=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(Buffer.concat(chunks)));
 if(!response.ok||![200,201].includes(response.status)){const error=Error('request_failed');error.status=response.status;error.code=typeof value?.error?.code==='string'?value.error.code:'';throw error;}
 return value;
}
function createManagedLocalRunnerController({parent,origin,binary,key,userData,safeStorage,discover,spawn=spawnChild,environment=process.env,stopGraceMS=12000,stopDeadlineMS=16000,checkIntervalMS=15000,now=Date.now}){
 if(!validOrigin(origin)||!path.isAbsolute(binary)||!path.isAbsolute(userData)||!Buffer.isBuffer(key)||key.length!==32||typeof discover!=='function')throw Error('Invalid managed local runner configuration');
 const privateKey=Buffer.from(key),instanceID=crypto.createHmac('sha256',privateKey).update('suma-desktop-instance-v1').digest('hex'),records=new Map();
 let disposed=false,journal,busy=false,disposing,activeManage,journalFinalized=false;
 function alive(){try{return !disposed&&!parent.isDestroyed()&&!parent.webContents.isDestroyed()&&new URL(parent.webContents.getURL()).origin===origin;}catch{return false;}}
 function caller(event){try{return alive()&&event.sender===parent.webContents&&event.senderFrame===parent.webContents.mainFrame&&new URL(event.senderFrame.url).origin===origin;}catch{return false;}}
 function openJournal(){if(journalFinalized)throw Error('Managed runner controller is closed');return journal??=createManagedRunnerJournal({userData,safeStorage});}
 function save(record){openJournal().write(record.value);}
 function connection(scope,phase,record){const value={project_id:scope.project_id,adapter_id:scope.adapter_id,phase};if(record?.value){value.worker_id=record.value.request.worker_id;value.base_commit=record.value.base_commit;value.repository_name=path.basename(record.value.repository_path);if(record.expiresAt||record.value.worker?.expires_at)value.expires_at=record.expiresAt||record.value.worker.expires_at;}return {status:'ok',connection:value};}
 function phase(record,value){record.value.phase=value;save(record);}
 function getRecord(scope){
  const id=scopeKey(scope);if(records.has(id))return records.get(id);const value=openJournal().read(scope);if(!value)return;
  // A saved observation never proves that an old child is running now. An
  // explicit reconnect reuses its credential and the Runner's exclusive log.
  if(['starting','connected'].includes(value.phase))value.phase='prepared';else if(value.phase==='stopping')value.phase='stop_unconfirmed';
  const record={value,child:null,closed:true,stopping:null,guardBusy:false};records.set(id,record);return record;
 }
 function baseOptions(scope){return {credentials:'include',redirect:'error',cache:'no-store',signal:AbortSignal.timeout(5000),headers:{Accept:'application/json','X-Suma-Expected-User':scope.user_id,'X-Suma-Expected-Organization':scope.tenant_id}};}
 async function session(scope){if(!alive())throw Error('authority');const value=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/auth/session',{...baseOptions(scope),method:'GET'}));if(!alive()||value?.user?.id!==scope.user_id||value?.organization?.id!==scope.tenant_id)throw Error('authority');}
 async function signed(action,body,scope){
  const nonce=crypto.randomBytes(16).toString('hex'),expires=new Date(now()+60000).toISOString();
  const fields=action==='context'?['project_id','user_id','tenant_id']:['project_id','user_id','tenant_id','worker_id','adapter_id','instance_id','expected_binding_sha256','base_commit','credential_sha256','idempotency_key'];
  const options=baseOptions(scope);options.headers={...options.headers,'Content-Type':'application/json','X-Suma-Desktop-Nonce':nonce,'X-Suma-Desktop-Expires':expires,'X-Suma-Desktop-Signature':crypto.createHmac('sha256',privateKey).update(['suma-desktop-runner-v1',action,...fields.map(name=>body[name]),nonce,expires].join('\0')).digest('hex')};
  return boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/desktop-runners/'+action,{...options,method:'POST',body:JSON.stringify(body)}));
 }
 async function authorize(scope){
  await session(scope);const context=await signed('context',{project_id:scope.project_id,user_id:scope.user_id,tenant_id:scope.tenant_id},scope);await session(scope);
  if(!exact(context,['project_id','user_id','tenant_id','instance_id','binding_sha256','repository_path','repository_identity','base_commit'])||['project_id','user_id','tenant_id'].some(k=>context[k]!==scope[k])||context.instance_id!==instanceID||!HASH.test(context.binding_sha256)||typeof context.repository_path!=='string'||!path.isAbsolute(context.repository_path)||Buffer.byteLength(context.repository_path)>4096||/\p{Cc}/u.test(context.repository_path)||typeof context.repository_identity!=='string'||!context.repository_identity||Buffer.byteLength(context.repository_identity)>1024||!(/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/).test(context.base_commit))throw Error('authority');
  return context;
 }
 function sameContext(record,context){const value=record.value;return value.instance_id===context.instance_id&&value.repository_path===context.repository_path&&value.repository_identity===context.repository_identity&&value.binding_sha256===context.binding_sha256&&value.base_commit===context.base_commit;}
 function softStop(record,target='stopped'){
  if(record.value.phase==='stop_unconfirmed')return Promise.resolve();
  if(record.stopping)return record.stopping;
  if(!record.child||record.closed){if(target!=='stopped'||!['prepared','issuance_unknown','binding_changed','origin_changed','credential_unrecoverable','authorization_required'].includes(record.value.phase))phase(record,target);return Promise.resolve();}
  phase(record,'stopping');clearInterval(record.guard);
  record.stopping=new Promise(resolve=>{
   let settled=false;const finish=()=>{if(settled)return;settled=true;clearTimeout(record.killTimer);clearTimeout(record.stopTimer);record.stopping=null;resolve();};
   record.finishStop=()=>{try{phase(record,record.hardKilled?'stop_unconfirmed':target);}catch{record.value.phase='stop_unconfirmed';}finish();};
   record.killTimer=setTimeout(()=>{if(record.closed)return;record.hardKilled=true;try{phase(record,'stop_unconfirmed');}catch{record.value.phase='stop_unconfirmed';}try{record.child.kill('SIGKILL');}catch{}},stopGraceMS);
   record.stopTimer=setTimeout(()=>{record.hardKilled=true;try{phase(record,'stop_unconfirmed');}catch{record.value.phase='stop_unconfirmed';}finish();},stopDeadlineMS);
   try{record.child.stdin.end();}catch{record.hardKilled=true;}
  });
  return record.stopping;
 }
 // Original receipts establish historical identity; only the current registry
 // and future expiry establish permission to launch that same worker now.
 function validWorkerReceipt(worker,record){return exact(worker,['id','project_id','worker_id','version','status','profiles','deployment','created_at','expires_at'])&&ID.test(worker.id)&&worker.project_id===record.value.scope.project_id&&worker.worker_id===record.value.request.worker_id&&Number.isInteger(worker.version)&&worker.version>0&&worker.status==='active'&&Array.isArray(worker.profiles)&&worker.profiles.length===0&&worker.deployment===false&&typeof worker.created_at==='string'&&Number.isFinite(Date.parse(worker.created_at))&&typeof worker.expires_at==='string'&&Number.isFinite(Date.parse(worker.expires_at))&&Date.parse(worker.expires_at)>Date.parse(worker.created_at);}
 function validWorker(worker,record){return validWorkerReceipt(worker,record)&&Date.parse(worker.expires_at)>now();}
 async function checkWorker(record){
  const scope=record.value.scope,out=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/release-workers?'+new URLSearchParams({project_id:scope.project_id}),{...baseOptions(scope),method:'GET'}));
  if(!plain(out)||!Array.isArray(out.items)||out.items.length>512)throw Error('worker');const matches=out.items.filter(w=>w.worker_id===record.value.request.worker_id);
  return matches.length===1&&validWorker(matches[0],record)&&matches[0].id===record.value.worker?.id;
 }
 async function readInventory(record,captureWatermark=false){
  const scope=record.value.scope,started=now(),out=await boundedJSON(await parent.webContents.session.fetch(origin+'/api/v1/tool-discovery?'+new URLSearchParams({project_id:scope.project_id,worker_id:record.value.request.worker_id}),{...baseOptions(scope),method:'GET'}));
  if(out?.project_id!==scope.project_id||out?.worker_id!==record.value.request.worker_id||!['fresh','stale','unreported'].includes(out.state)||!Array.isArray(out.items)||out.items.length>128)throw Error('inventory');
  if(captureWatermark){const observed=Date.parse(out.observed_at);record.inventoryWatermark=Number.isFinite(observed)?observed:-Infinity;return false;}
  if(out.state!=='fresh')return false;
  const observed=Date.parse(out.observed_at),expires=Date.parse(out.expires_at),asOf=Date.parse(out.as_of),elapsed=now()-started;
  if(!Number.isFinite(observed)||!Number.isFinite(expires)||!Number.isFinite(asOf)||expires-observed!==60000||observed>asOf||asOf>=expires||elapsed<0||expires-asOf<=elapsed||observed<record.spawnedAt||observed<=record.inventoryWatermark)return false;
  const [adapter,protocol]=TOOL_PROTOCOLS[scope.adapter_id],matching=out.items.filter(item=>item?.available===true&&item.profile?.adapter===adapter&&item.profile.protocol===protocol&&HASH.test(item.profile.profile_sha256)&&HASH.test(item.profile.executable_sha256)&&typeof item.profile.command_id==='string'&&typeof item.profile.tool_version==='string'&&item.profile.tool_version.length>0&&item.profile.read_only===false);
  if(matching.length!==1)return false;record.expiresAt=out.expires_at;return true;
 }
 async function observe(record){if(!record.child||record.closed||record.stopping||record.value.phase==='stop_unconfirmed')return;const fresh=await readInventory(record);await session(record.value.scope);if(record.closed||record.stopping)return;phase(record,fresh?'connected':'starting');if(!fresh)record.expiresAt=undefined;}
 async function guard(record){
  if(disposed||record.closed||record.guardBusy||busy)return;record.guardBusy=true;
  try{const context=await authorize(record.value.scope);if(!sameContext(record,context))await softStop(record,'binding_changed');else if(!await checkWorker(record))await softStop(record,'authorization_required');else await observe(record);}catch(error){await softStop(record,gitUnavailable(error)?'unavailable':'authorization_required');}finally{record.guardBusy=false;}
 }
 function launch(record,locale){
  const value=record.value,stateDirectory=openJournal().stateDirectory(value.scope);phase(record,'starting');
  const child=spawn(binary,['--language='+locale,'--desktop-managed-runner'],{cwd:value.repository_path,env:runnerEnvironment(environment,stateDirectory),stdio:['pipe','ignore','ignore'],windowsHide:true,shell:false});
  record.child=child;record.closed=false;record.hardKilled=false;record.spawnedAt=now();record.expiresAt=undefined;
  child.on('error',()=>{void softStop(record,'failed').catch(()=>{});});
  child.stdin.on('error',()=>{void softStop(record,'failed').catch(()=>{});});
  child.once('close',(code)=>{
   record.closed=true;clearInterval(record.guard);if(record.finishStop){const done=record.finishStop;record.finishStop=null;done();}else if(record.value.phase!=='stop_unconfirmed'){try{phase(record,code===0?'stopped':'failed');}catch{record.value.phase='failed';}}
  });
  const config={protocol:'suma-desktop-managed-runner-v1',control_url:origin,project_id:value.scope.project_id,worker_id:value.request.worker_id,adapter_id:value.scope.adapter_id,repository:value.repository_path,base_commit:value.base_commit,state_directory:stateDirectory,token:value.token};
  child.stdin.write(JSON.stringify(config)+'\n');record.guard=setInterval(()=>{record.guardTask=guard(record).catch(()=>{});},checkIntervalMS);record.guard.unref?.();
 }
 async function register(record){
  phase(record,'issuance_unknown');const value=record.value,out=await signed('register',value.request,value.scope);
  if(!exact(out,['operation_id','project_id','user_id','tenant_id','instance_id','adapter_id','binding_sha256','base_commit','worker'])||out.operation_id!==value.request.idempotency_key||['project_id','user_id','tenant_id','adapter_id'].some(k=>out[k]!==value.scope[k])||out.instance_id!==value.instance_id||out.binding_sha256!==value.binding_sha256||out.base_commit!==value.base_commit||!validWorkerReceipt(out.worker,record)||value.worker&&value.worker.id!==out.worker.id)throw Error('registration');
  value.worker=out.worker;phase(record,'prepared');
 }
 function newRecord(scope,context){
  const token=crypto.randomBytes(32).toString('hex'),value={protocol:PROTOCOL,scope,origin,instance_id:context.instance_id,repository_path:context.repository_path,repository_identity:context.repository_identity,binding_sha256:context.binding_sha256,base_commit:context.base_commit,token,phase:'prepared',request:{project_id:scope.project_id,user_id:scope.user_id,tenant_id:scope.tenant_id,worker_id:'desktop-'+crypto.randomBytes(16).toString('hex'),adapter_id:scope.adapter_id,instance_id:context.instance_id,expected_binding_sha256:context.binding_sha256,base_commit:context.base_commit,credential_sha256:crypto.createHash('sha256').update(token).digest('hex'),idempotency_key:'desktop-runner-'+crypto.randomBytes(16).toString('hex')}};
  const record={value,child:null,closed:true,stopping:null,guardBusy:false};save(record);records.set(scopeKey(scope),record);return record;
 }
 async function manage(event,request){
  if(!caller(event)||!exact(request,['action','project_id','user_id','tenant_id','adapter_id','locale'])||!['connect','status','stop'].includes(request.action)||!['zh-CN','en-US'].includes(request.locale)||!validScope(requestScope(request)))return failure('managed_runner_request_rejected');
  if(busy)return failure('managed_runner_busy');busy=true;let resolveManage;activeManage=new Promise(resolve=>resolveManage=resolve);const scope=requestScope(request);let record;
  try{
   await session(scope);if(!caller(event))throw Error('authority');
   try{record=getRecord(scope);}catch{return connection(scope,'credential_unrecoverable');}
   if(record?.value.origin!==undefined&&record.value.origin!==origin)return connection(scope,'origin_changed',record);
   if(request.action==='stop'){if(record)await softStop(record);await session(scope);return connection(scope,record?.value.phase||'unconfigured',record);}
   let context;try{context=await authorize(scope);}catch(error){
    if(gitUnavailable(error)){if(record)await softStop(record,'unavailable');return connection(scope,'unavailable',record);}
    if(record)await softStop(record,'authorization_required');
    if(error.status===422&&/workspace|repository|binding/.test(error.code||''))return connection(scope,record?'binding_changed':'needs_workspace',record);
    return failure('managed_runner_access_changed');
   }
   if(!caller(event))throw Error('authority');
   if(record&&!sameContext(record,context)){await softStop(record,'binding_changed');return connection(scope,'binding_changed',record);}
   if(request.action==='status'){
    if(record&&record.child&&!record.closed){if(!await checkWorker(record))await softStop(record,'authorization_required');else await observe(record);}
    else if(record?.closed&&record.value.phase==='authorization_required'&&record.value.worker&&await checkWorker(record)){await session(scope);phase(record,'prepared');}
    await session(scope);return connection(scope,record?.value.phase||'unconfigured',record);
   }
   if(record?.value.phase==='stop_unconfirmed')return connection(scope,'stop_unconfirmed',record);
   if(record?.child&&!record.closed){await observe(record);return connection(scope,record.value.phase,record);}
   const observed=await discover(event,{...scope,locale:request.locale});
   await session(scope);if(!caller(event))throw Error('authority');
   const item=observed?.status==='observed'&&observed.report?.protocol==='suma-installed-tools-v1'&&observed.report.items?.length===1?observed.report.items[0]:undefined;
   if(!item||item.adapter_id!==scope.adapter_id)return connection(scope,'unavailable',record);
   if(!item.installed)return connection(scope,'not_installed',record);
   if(!item.launch_supported)return connection(scope,'unavailable',record);
   const current=await authorize(scope);if(!caller(event))throw Error('authority');if(record&&!sameContext(record,current)||JSON.stringify(current)!==JSON.stringify(context))return connection(scope,'binding_changed',record);
   record??=newRecord(scope,current);
   if(record.value.worker&&!await checkWorker(record)){phase(record,'authorization_required');return connection(scope,'authorization_required',record);}
   try{await register(record);}catch(error){if([401,403].includes(error.status)){phase(record,'authorization_required');return connection(scope,'authorization_required',record);}return connection(scope,'issuance_unknown',record);}
   const beforeLaunch=await authorize(scope);if(!caller(event)||!sameContext(record,beforeLaunch))return connection(scope,'binding_changed',record);
   if(!await checkWorker(record)){phase(record,'authorization_required');return connection(scope,'authorization_required',record);}
   await readInventory(record,true);await session(scope);if(!caller(event))throw Error('authority');launch(record,request.locale);await observe(record);return connection(scope,record.value.phase,record);
  }catch(error){if(gitUnavailable(error)){if(record)await softStop(record,'unavailable').catch(()=>{});return connection(scope,'unavailable',record);}if(record?.child&&!record.closed)await softStop(record,'authorization_required').catch(()=>{});return failure('managed_runner_access_changed');}
  finally{busy=false;resolveManage();activeManage=undefined;}
 }
 async function dispose(){
  if(disposing)return disposing;disposed=true;parent.removeListener('closed',dispose);parent.webContents.removeListener('destroyed',dispose);
  disposing=(async()=>{
   if(activeManage)await activeManage;
   for(const record of records.values())clearInterval(record.guard);
   await Promise.all([...records.values()].map(record=>record.guardTask).filter(Boolean));
   await Promise.all([...records.values()].map(record=>softStop(record).catch(()=>{})));
   journalFinalized=true;if(journal){journal.close();journal=undefined;}
  })();return disposing;
 }
 parent.on('closed',dispose);parent.webContents.on('destroyed',dispose);
 return {manage,dispose};
}
module.exports={CHANNEL,createManagedLocalRunnerController};
