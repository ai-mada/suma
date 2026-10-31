'use strict';
const {spawn:spawnChild}=require('node:child_process'),path=require('node:path'),crypto=require('node:crypto');
const {PROTOCOL,createManagedRunnerJournal,scopeKey,validScope,validOrigin}=require('./managed-local-runner-journal.cjs');
const {createLocalACPController,cleanEnvironment}=require('./local-acp.cjs');
const {fields,validConfiguration}=require('./local-acp-validation.cjs');
const {createLocalCodexController,cleanCodexEnvironment}=require('./local-codex.cjs');
const codexValidation=require('./local-codex-validation.cjs');
const opencodeValidation=require('./local-opencode-validation.cjs');
const {resolveOpenCodeAssets}=require('./local-opencode-assets.cjs');
const CHANNEL='suma:manage-local-runner';
const TOOL_PROTOCOLS=Object.freeze({'codex-jsonl':['codex','codex-jsonl-v1'],'codebuddy-cli':['codebuddy','codebuddy-jsonl-v1'],'dsh-multica':['harness','dsh-multica-jsonl-v1'],'claude-code-cli':['claude','claude-jsonl-v1'],'qoder-acp':['qoder','acp-stdio-v1'],'codex-app-server':['codex','codex-app-server-v1'],'opencode-sdk':['opencode','opencode-sdk-v1']});
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
function createManagedLocalRunnerController({parent,origin,binary,key,userData,safeStorage,discover,spawn=spawnChild,environment=process.env,stopGraceMS=12000,stopDeadlineMS=16000,checkIntervalMS=15000,now=Date.now,opencodeAssets=resolveOpenCodeAssets}){
 if(!validOrigin(origin)||!path.isAbsolute(binary)||!path.isAbsolute(userData)||!Buffer.isBuffer(key)||key.length!==32||typeof discover!=='function')throw Error('Invalid managed local runner configuration');
 const privateKey=Buffer.from(key),instanceID=crypto.createHmac('sha256',privateKey).update('suma-desktop-instance-v1').digest('hex'),records=new Map();
 let disposed=false,journal,busy=false,disposing,activeManage,journalFinalized=false;
 function alive(){try{return !disposed&&!parent.isDestroyed()&&!parent.webContents.isDestroyed()&&new URL(parent.webContents.getURL()).origin===origin;}catch{return false;}}
 function caller(event){try{return alive()&&event.sender===parent.webContents&&event.senderFrame===parent.webContents.mainFrame&&new URL(event.senderFrame.url).origin===origin;}catch{return false;}}
 function openJournal(){if(journalFinalized)throw Error('Managed runner controller is closed');return journal??=createManagedRunnerJournal({userData,safeStorage});}
 function save(record){openJournal().write(record.value);}
 function connection(scope,phase,record){const value={project_id:scope.project_id,adapter_id:scope.adapter_id,phase};if(record?.value){value.worker_id=record.value.request.worker_id;value.base_commit=record.value.base_commit;value.repository_name=path.basename(record.value.repository_path);if(record.expiresAt||record.value.worker?.expires_at)value.expires_at=record.expiresAt||record.value.worker.expires_at;const config=record.value.acp_configuration;if(config)Object.assign(value,{model:config.model,tool_version:config.reviewed_version,executable_sha256:config.executable_sha256,configuration_sha256:config.configuration_sha256});const codex=record.value.codex_configuration;if(codex)Object.assign(value,{tool_version:codex.reviewed_version,executable_sha256:codex.executable_sha256,configuration_sha256:codex.configuration_sha256,...(phase==='connected'&&record.catalogSHA?{catalog_sha256:record.catalogSHA}:{})});const open=record.value.opencode_configuration;if(open)Object.assign(value,{model:open.model,provider_id:open.provider_id,endpoint_origin:opencodeValidation.endpointOrigin(open),tool_version:open.reviewed_version,executable_sha256:open.executable_sha256,configuration_sha256:open.configuration_sha256});}return {status:'ok',connection:value};}
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
  if(matching.length!==1)return false;
  const fixed=record.value.acp_configuration;
  if(fixed){const profile=matching[0].profile,cap=profile.acp_execution;if(profile.executable_sha256!==fixed.executable_sha256||profile.tool_version!==fixed.reviewed_version||!exact(cap,['runtime','protocol_version','sdk_version','configuration_sha256','model','model_source','budget_mode','identity_evidence','auth_strategy','permission_mode'])||cap.runtime!=='qoder'||cap.protocol_version!==1||cap.sdk_version!=='0.13.5'||cap.configuration_sha256!==fixed.configuration_sha256||cap.model!==fixed.model||cap.model_source!=='administrator_session_config'||cap.budget_mode!=='observe_only'||cap.identity_evidence!=='configuration_ack_only'||cap.auth_strategy!=='existing_cli_login'||cap.permission_mode!=='human_once')return false;}
  const codex=record.value.codex_configuration;
  if(codex){const profile=matching[0].profile,catalog=matching[0].codex_catalog;if(profile.executable_sha256!==codex.executable_sha256||profile.tool_version!==codex.reviewed_version||!codexValidation.validCatalog(catalog)||catalog.configuration_sha256!==codex.configuration_sha256)return false;record.catalogSHA=catalog.catalog_sha256;}
  const open=record.value.opencode_configuration;
  if(open){const p=matching[0].profile;if(p.executable_sha256!==open.node_sha256||p.tool_version!=='suma-opencode-runtime 1.0.0'||p.workspace_tools_version!=='attempt-workspace-v1'||!opencodeValidation.matchesCapability(open,p.opencode_execution))return false;}
  record.expiresAt=out.expires_at;return true;
 }
 async function observe(record){if(!record.child||record.closed||record.stopping||record.value.phase==='stop_unconfirmed')return;const fresh=await readInventory(record);await session(record.value.scope);if(record.closed||record.stopping)return;phase(record,fresh?'connected':'starting');if(!fresh)record.expiresAt=undefined;}
 async function guard(record){
  if(disposed||record.closed||record.guardBusy||busy)return;record.guardBusy=true;
  try{const context=await authorize(record.value.scope);if(!sameContext(record,context))await softStop(record,'binding_changed');else if(!await checkWorker(record))await softStop(record,'authorization_required');else await observe(record);}catch(error){await softStop(record,gitUnavailable(error)?'unavailable':'authorization_required');}finally{record.guardBusy=false;}
 }
 function launch(record,locale){
  const value=record.value,stateDirectory=openJournal().stateDirectory(value.scope);phase(record,'starting');
  const env=value.opencode_configuration?cleanCodexEnvironment(environment,stateDirectory,value.opencode_configuration.api_key_env_ref):value.codex_configuration?cleanCodexEnvironment(environment,stateDirectory,value.codex_configuration.api_key_env_ref):value.acp_configuration?cleanEnvironment(environment,stateDirectory,value.acp_configuration.credential_env_ref):runnerEnvironment(environment,stateDirectory);
  const child=spawn(binary,['--language='+locale,'--desktop-managed-runner'],{cwd:value.repository_path,env,stdio:['pipe','ignore','ignore'],windowsHide:true,shell:false});
  record.child=child;record.closed=false;record.hardKilled=false;record.spawnedAt=now();record.expiresAt=undefined;
  child.on('error',()=>{void softStop(record,'failed').catch(()=>{});});
  child.stdin.on('error',()=>{void softStop(record,'failed').catch(()=>{});});
  child.once('close',(code)=>{
   record.closed=true;clearInterval(record.guard);if(record.finishStop){const done=record.finishStop;record.finishStop=null;done();}else if(record.value.phase!=='stop_unconfirmed'){try{phase(record,code===0?'stopped':'failed');}catch{record.value.phase='failed';}}
  });
  const config={protocol:'suma-desktop-managed-runner-v1',control_url:origin,project_id:value.scope.project_id,worker_id:value.request.worker_id,adapter_id:value.scope.adapter_id,repository:value.repository_path,base_commit:value.base_commit,state_directory:stateDirectory,token:value.token};
  if(value.acp_configuration)config.acp_configuration={...value.acp_configuration};
  if(value.codex_configuration)config.codex_configuration={...value.codex_configuration};
  if(value.opencode_configuration)config.opencode_configuration={...value.opencode_configuration};
  child.stdin.write(JSON.stringify(config)+'\n');record.guard=setInterval(()=>{record.guardTask=guard(record).catch(()=>{});},checkIntervalMS);record.guard.unref?.();
 }
 async function register(record){
  phase(record,'issuance_unknown');const value=record.value,out=await signed('register',value.request,value.scope);
  if(!exact(out,['operation_id','project_id','user_id','tenant_id','instance_id','adapter_id','binding_sha256','base_commit','worker'])||out.operation_id!==value.request.idempotency_key||['project_id','user_id','tenant_id','adapter_id'].some(k=>out[k]!==value.scope[k])||out.instance_id!==value.instance_id||out.binding_sha256!==value.binding_sha256||out.base_commit!==value.base_commit||!validWorkerReceipt(out.worker,record)||value.worker&&value.worker.id!==out.worker.id)throw Error('registration');
  value.worker=out.worker;phase(record,'prepared');
 }
 function newRecord(scope,context,configuration,selection){
  const token=crypto.randomBytes(32).toString('hex'),value={protocol:PROTOCOL,scope,origin,instance_id:context.instance_id,repository_path:context.repository_path,repository_identity:context.repository_identity,binding_sha256:context.binding_sha256,base_commit:context.base_commit,token,phase:'prepared',request:{project_id:scope.project_id,user_id:scope.user_id,tenant_id:scope.tenant_id,worker_id:'desktop-'+crypto.randomBytes(16).toString('hex'),adapter_id:scope.adapter_id,instance_id:context.instance_id,expected_binding_sha256:context.binding_sha256,base_commit:context.base_commit,credential_sha256:crypto.createHash('sha256').update(token).digest('hex'),idempotency_key:'desktop-runner-'+crypto.randomBytes(16).toString('hex')}};
  if(configuration){if(scope.adapter_id==='opencode-sdk')value.opencode_configuration={...configuration};else if(scope.adapter_id==='codex-app-server'){value.codex_configuration={...configuration};value.codex_selection={...selection};}else{value.acp_configuration={...configuration};value.acp_selection={...selection};}}
  const record={value,child:null,closed:true,stopping:null,guardBusy:false};save(record);records.set(scopeKey(scope),record);return record;
 }
 async function manage(event,request){
  if(request?.adapter_id==='codex-app-server'&&['detect','models','model-status'].includes(request?.action))return codex.inspect(event,request);
  const qoder=request?.adapter_id==='qoder-acp',configurable=qoder&&request?.action==='connect';
  const nativeCodex=request?.adapter_id==='codex-app-server',codexConfigurable=nativeCodex&&request?.action==='connect';
  const openCode=request?.adapter_id==='opencode-sdk';
  if(!caller(event)||!fields(request,['action','project_id','user_id','tenant_id','adapter_id','locale'],configurable?['acp_selection','expected_configuration_sha256']:codexConfigurable?['codex_selection','expected_configuration_sha256']:openCode&&request?.action==='connect'?['opencode_setup']:[])||!['connect','status','stop'].includes(request.action)||!['zh-CN','en-US'].includes(request.locale)||!validScope(requestScope(request))||Object.hasOwn(request,'expected_configuration_sha256')&&!HASH.test(request.expected_configuration_sha256)||Object.hasOwn(request,'acp_selection')&&(!fields(request.acp_selection,['installation_id','probe_id','model_id'])||!ID.test(request.acp_selection.installation_id)||!ID.test(request.acp_selection.probe_id)||typeof request.acp_selection.model_id!=='string'||!HASH.test(request.expected_configuration_sha256))||Object.hasOwn(request,'codex_selection')&&(!fields(request.codex_selection,['installation_id','probe_id'])||!ID.test(request.codex_selection.installation_id)||!ID.test(request.codex_selection.probe_id)||!HASH.test(request.expected_configuration_sha256))||Object.hasOwn(request,'opencode_setup')&&!opencodeValidation.validSetup(request.opencode_setup))return failure('managed_runner_request_rejected');
  if(busy||(acp.isBusy()||codex.isBusy())&&request.action==='connect')return failure('managed_runner_busy');busy=true;let resolveManage;activeManage=new Promise(resolve=>resolveManage=resolve);const scope=requestScope(request);let record,configuration;
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
   if(qoder){
    if(request.acp_selection){
     const same=record?.value.acp_selection&&['installation_id','probe_id','model_id'].every(k=>record.value.acp_selection[k]===request.acp_selection[k]);
     if(same){configuration=record.value.acp_configuration;}
     else{
      if(record&&(!record.closed||record.value.phase!=='stopped'))return connection(scope,'binding_changed',record);
      try{configuration=await acp.select({...scope,locale:request.locale},request.acp_selection,context,event);}catch{return failure('managed_runner_request_rejected');}
     }
     if(!validConfiguration(configuration)||configuration.configuration_sha256!==request.expected_configuration_sha256)return connection(scope,'binding_changed',record);
    }else{
     configuration=record?.value.acp_configuration;if(!configuration)return failure('managed_runner_request_rejected');
     if(request.expected_configuration_sha256&&request.expected_configuration_sha256!==configuration.configuration_sha256)return connection(scope,'binding_changed',record);
    }
   }
   if(nativeCodex){
    const changed=record&&request.codex_selection&&!['installation_id','probe_id'].every(k=>record.value.codex_selection[k]===request.codex_selection[k]);
    if(changed&&(!record.closed||record.value.phase!=='stopped'))return connection(scope,'binding_changed',record);
    if(record&&!changed)configuration=record.value.codex_configuration;
    else{
     if(!request.codex_selection)return failure('managed_runner_request_rejected');
     try{configuration=await codex.select({...scope,locale:request.locale},request.codex_selection,context,event);}catch{return failure('managed_runner_request_rejected');}
    }
    if(!codexValidation.validConfiguration(configuration)||request.expected_configuration_sha256&&request.expected_configuration_sha256!==configuration.configuration_sha256)return connection(scope,'binding_changed',record);
   }
   let openChanged=false;
   if(openCode){
    const original=record?.value.opencode_configuration,setup=request.opencode_setup||(original&&opencodeValidation.setupFromConfiguration(original));if(!setup)return failure('managed_runner_request_rejected');
    openChanged=!!original&&Object.keys(setup).some(k=>setup[k]!==opencodeValidation.setupFromConfiguration(original)[k])||!!original&&Object.keys(opencodeValidation.setupFromConfiguration(original)).some(k=>setup[k]!==original[k]);
    if(openChanged&&(!record.closed||record.value.phase!=='stopped'))return connection(scope,'binding_changed',record);
    configuration=original;
   }
   if(record?.value.phase==='stop_unconfirmed')return connection(scope,'stop_unconfirmed',record);
   if(record?.child&&!record.closed){await observe(record);return connection(scope,record.value.phase,record);}
   const observed=qoder||nativeCodex?undefined:await discover(event,{...scope,locale:request.locale});
   await session(scope);if(!caller(event))throw Error('authority');
   const item=observed?.status==='observed'&&observed.report?.protocol==='suma-installed-tools-v1'&&observed.report.items?.length===1?observed.report.items[0]:undefined;
   if(openCode){
    const setup=request.opencode_setup||(configuration&&opencodeValidation.setupFromConfiguration(configuration));
    if(!item||item.adapter_id!==scope.adapter_id)return connection(scope,'unavailable',record);if(!item.installed)return connection(scope,'not_installed',record);
    if(item.launch_supported!==false||item.reason_code!=='configuration_required'||item.version!=='1.19.4'||!HASH.test(item.executable_sha256)||!path.isAbsolute(item.executable||''))return connection(scope,'unavailable',record);
    if(item.executable_sha256!==setup.executable_sha256)return connection(scope,'binding_changed',record);
    let assets;try{assets=await opencodeAssets({binary,environment});const ref=setup.api_key_env_ref;if(ref&&(typeof environment[ref]!=='string'||!environment[ref]||Buffer.byteLength(environment[ref])>16384||/[\x00\r\n]/.test(environment[ref])))throw Error('credential');}catch{return connection(scope,'unavailable',record);}
    const next={executable:item.executable,executable_sha256:item.executable_sha256,reviewed_version:item.version,...assets,provider_id:setup.provider_id,model:setup.model,base_url:setup.base_url,...(setup.api_key_env_ref?{api_key_env_ref:setup.api_key_env_ref}:{})};next.configuration_sha256=opencodeValidation.configurationDigest(next);
    if(!opencodeValidation.validConfiguration(next))return connection(scope,'unavailable',record);
    if(configuration&&!openChanged&&Object.keys(next).some(k=>next[k]!==configuration[k]))return connection(scope,'binding_changed',record);configuration=next;
   }else if(!qoder&&!nativeCodex){if(!item||item.adapter_id!==scope.adapter_id)return connection(scope,'unavailable',record);if(!item.installed)return connection(scope,'not_installed',record);if(!item.launch_supported)return connection(scope,'unavailable',record);}
   const current=await authorize(scope);if(!caller(event))throw Error('authority');if(record&&!sameContext(record,current)||JSON.stringify(current)!==JSON.stringify(context))return connection(scope,'binding_changed',record);
   if(record&&qoder&&request.acp_selection&&!['installation_id','probe_id','model_id'].every(k=>record.value.acp_selection[k]===request.acp_selection[k])){record.value.acp_configuration={...configuration};record.value.acp_selection={...request.acp_selection};save(record);}
   if(record&&nativeCodex&&request.codex_selection&&!['installation_id','probe_id'].every(k=>record.value.codex_selection[k]===request.codex_selection[k])){record.value.codex_configuration={...configuration};record.value.codex_selection={...request.codex_selection};save(record);}
   if(record&&openChanged){record.value.opencode_configuration={...configuration};save(record);}
   record??=newRecord(scope,current,configuration,nativeCodex?request.codex_selection:request.acp_selection);
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
   await acp.dispose();
   await codex.dispose();
   for(const record of records.values())clearInterval(record.guard);
   await Promise.all([...records.values()].map(record=>record.guardTask).filter(Boolean));
   await Promise.all([...records.values()].map(record=>softStop(record).catch(()=>{})));
   journalFinalized=true;if(journal){journal.close();journal=undefined;}
  })();return disposing;
 }
 parent.on('closed',dispose);parent.webContents.on('destroyed',dispose);
 const acp=createLocalACPController({binary,probeDirectory:path.join(userData,'.cache','acp-probes'),credentialKey:privateKey,environment,spawn,caller,session,authorize,discover,journal:openJournal,now,busy:()=>busy||codex.isBusy()});
 const codex=createLocalCodexController({binary,probeDirectory:path.join(userData,'.cache','codex-probes'),credentialKey:privateKey,environment,spawn,caller,session,authorize,discover,journal:openJournal,now,busy:()=>busy||acp.isBusy()});
 return {manage,inspectACP:acp.inspect,dispose};
}
module.exports={CHANNEL,createManagedLocalRunnerController};
