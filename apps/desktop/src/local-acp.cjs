'use strict';
const crypto=require('node:crypto'),path=require('node:path'),fs=require('node:fs'),{spawn:spawnChild}=require('node:child_process');
const {ensurePrivateDirectory}=require('./workspace-grants.cjs');
const {HASH,ID,acpAdapter,fields,text,credentialRef,validExecutable,validConfiguration,validDiscovery,strictJSON}=require('./local-acp-validation.cjs');
const CHANNEL='suma:inspect-local-acp',baseKeys=['project_id','user_id','tenant_id','adapter_id','locale'];
const scopeOf=r=>({project_id:r.project_id,user_id:r.user_id,tenant_id:r.tenant_id,adapter_id:r.adapter_id});
const keyOf=s=>[s.user_id,s.tenant_id,s.project_id,s.adapter_id].join(':');
const canon=value=>JSON.stringify(value,(_k,v)=>v&&typeof v==='object'&&!Array.isArray(v)?Object.fromEntries(Object.keys(v).sort().map(k=>[k,v[k]])):v);
const equal=(a,b)=>canon(a)===canon(b),failure=code=>({status:'failed',code});
function cleanEnvironment(environment,temp,ref){
 const names=new Set(['path','systemroot','windir','comspec','pathext','home','userprofile','homedrive','homepath','appdata','localappdata','lang','lc_all']);
 const out={};for(const [k,v]of Object.entries(environment))if(names.has(k.toLowerCase())&&typeof v==='string')out[k]=v;
 if(ref){if(!credentialRef(ref)||typeof environment[ref]!=='string'||!environment[ref]||/[\x00\r\n]/.test(environment[ref]))throw Error('credential');out[ref]=environment[ref];}
 return {...out,TEMP:temp,TMP:temp,TMPDIR:temp};
}
function createLocalACPController({binary,probeDirectory,credentialKey,environment=process.env,spawn=spawnChild,caller,session,authorize,discover,journal,now=Date.now,deadlineMS=25000,busy=()=>false}){
 const installations=new Map(),latest=new Map(),records=new Map();let disposed=false,active,commandBusy=false;
 function credentialDigest(ref){if(!ref)return '';cleanEnvironment(environment,probeDirectory,ref);return crypto.createHmac('sha256',credentialKey).update(ref+'\0'+environment[ref]).digest('hex');}
 function source(context,installation,ref){return crypto.createHmac('sha256',credentialKey).update(canon({context,installation,credential_ref:ref||'',credential_digest:credentialDigest(ref)})).digest('hex');}
 function save(record){journal().writeACP(record);records.set(record.operation_id,record);}
 function load(id){let record=records.get(id);if(record)return record;record=journal().readACP(id);if(!record)return;if(!fields(record,['operation_id','id','scope','context','installation','source_sha256','created_at','expires_at','phase','request'],['discovery'])||!ID.test(record.operation_id)||!ID.test(record.id)||!HASH.test(record.source_sha256)||!Number.isFinite(Date.parse(record.created_at))||!Number.isFinite(Date.parse(record.expires_at))||Date.parse(record.expires_at)-Date.parse(record.created_at)!==60000)throw Error('record');if(record.phase==='reading')record={...record,phase:'unknown'};records.set(id,record);return record;}
 function safeInstallation(item){return {installation_id:item.installation_id,label:item.label,kind:item.kind,...(item.version?{tool_version:item.version}:{}),...(item.executable_sha256?{executable_sha256:item.executable_sha256}:{})};}
 function response(scope,phase,record,error){
 const rows=record?[record.installation]:(installations.get(keyOf(scope))||[]);const observation={...scope,phase,installations:rows.map(safeInstallation)};
 if(record)observation.probe={id:record.id,operation_id:record.operation_id,installation_id:record.installation.installation_id,source_sha256:record.source_sha256,created_at:record.created_at,expires_at:record.expires_at,...(record.discovery&&phase!=='binding_changed'&&phase!=='authorization_required'?{discovery:structuredClone(record.discovery)}:{})};
 if(error)observation.error_code=error;return {status:'ok',observation};
 }
 function currentPhase(record){if(record.phase==='reading')return active?.record.operation_id===record.operation_id?'reading':'unknown';if(record.phase==='ready'&&now()>=Date.parse(record.expires_at))return 'expired';return record.phase;}
 async function installation(event,request){
 const report=await discover(event,{...scopeOf(request),locale:request.locale});
 const row=report?.status==='observed'&&report.report?.protocol==='suma-installed-tools-v1'&&report.report.items?.length===1?report.report.items[0]:undefined;
 const adapter=acpAdapter(request.adapter_id);if(!adapter||!row||row.adapter_id!==request.adapter_id)throw Error('installation');
 if(!row.installed)return {phase:'not_installed',rows:[]};
 if(row.reason_code==='unsupported_launcher')return {phase:'wrapper_only',rows:[{label:adapter.product,kind:'wrapper_only'}]};
 if(typeof row.executable!=='string'||!path.isAbsolute(row.executable)||!text(row.executable,4096))return {phase:'unavailable',rows:[]};
 let hostReady=validExecutable(row.executable,request.adapter_id);
 if(hostReady&&adapter.runtime==='cursor')try{const node=fs.lstatSync(row.executable),entry=fs.lstatSync(path.join(path.dirname(row.executable),'index.js'));hostReady=node.isFile()&&!node.isSymbolicLink()&&entry.isFile()&&!entry.isSymbolicLink();}catch{hostReady=false;}
 const native=hostReady&&row.reason_code==='acp_configuration_required'&&row.launch_supported===false&&text(row.version,256)&&HASH.test(row.executable_sha256);
 const kind=native?'native':row.reason_code==='unsupported_launcher'?'wrapper_only':'unverified';
 return {phase:native?'detected':kind==='wrapper_only'?'wrapper_only':'unavailable',rows:[{executable:row.executable,label:path.basename(row.executable),version:native?row.version:undefined,executable_sha256:native?row.executable_sha256:undefined,kind}]};
 }
 async function check(event,scope){try{if(disposed||!caller(event))throw Error('authority');await session(scope);const context=await authorize(scope);await session(scope);if(disposed||!caller(event))throw Error('authority');return context;}catch(error){const safe=Error('authority');safe.authority=true;safe.needsWorkspace=error?.status===422&&/workspace|repository|binding/.test(error.code||'');throw safe;}}
 async function runProbe(record,locale){
 const directory=ensurePrivateDirectory(path.join(ensurePrivateDirectory(probeDirectory),record.operation_id));
 const env=cleanEnvironment(environment,directory,record.request.credential_env_ref),secret=record.request.credential_env_ref?env[record.request.credential_env_ref]:undefined,input={adapter_id:record.scope.adapter_id,executable:record.installation.executable,executable_sha256:record.installation.executable_sha256,reviewed_version:record.installation.version,...(record.request.credential_env_ref?{credential_env_ref:record.request.credential_env_ref}:{})};
 return new Promise(resolve=>{
 let chunks=[],length=0,closed=false,failed=false,settled=false,killTimer,stopTimer,child;
 const finish=(phase,discovery)=>{if(settled)return;settled=true;clearTimeout(timer);clearTimeout(killTimer);clearTimeout(stopTimer);chunks=[];resolve({phase,...(discovery?{discovery}:{})});};
 const stop=()=>{if(closed||failed)return;failed=true;killTimer=setTimeout(()=>{if(!closed)try{child?.kill('SIGKILL');}catch{}},1500);stopTimer=setTimeout(()=>{if(!closed)finish('stop_unconfirmed');},3500);try{child?.kill('SIGTERM');}catch{}};
 const timer=setTimeout(stop,deadlineMS);
 try{
 child=spawn(binary,['--language='+locale,'--describe-acp-models-stdin'],{cwd:directory,env,stdio:['pipe','pipe','ignore'],windowsHide:true,shell:false});
 active={record,child,stop,done:new Promise(done=>child.once('close',done))};
 child.on('error',stop);child.stdin.on('error',stop);
 child.stdout.on('data',chunk=>{if(settled||failed)return;length+=chunk.length;if(length>262144){stop();return;}chunks.push(Buffer.from(chunk));});
 child.once('close',(code,signal)=>{closed=true;if(failed||signal){finish('unknown');return;}try{const value=strictJSON(Buffer.concat(chunks));if(!validDiscovery(value,record.scope.adapter_id)||value.executable_sha256!==record.installation.executable_sha256||value.tool_version!==record.installation.version||code!==0&&value.state==='ready')throw Error('result');if(secret&&JSON.stringify(value).includes(secret))throw Error('secret');finish(value.error_code==='acp_discovery_not_stopped'?'stop_unconfirmed':value.state,value);}catch{finish('unknown');}});
 child.stdin.end(JSON.stringify(input)+'\n');
 }catch{finish('unknown');}
 });
 }
 async function inspect(event,request){
 const optional=request?.action==='models'?['credential_env_ref']:request?.action==='status'?['operation_id']:[],required=request?.action==='models'?[...baseKeys,'action','installation_id','operation_id']:[...baseKeys,'action'];
 if(disposed||!caller(event)||!fields(request,required,optional)||!acpAdapter(request.adapter_id)||!['project_id','user_id','tenant_id'].every(k=>ID.test(request[k]))||!['zh-CN','en-US'].includes(request.locale)||!['detect','models','status'].includes(request.action)||Object.hasOwn(request,'operation_id')&&!ID.test(request.operation_id)||request.action==='models'&&(!ID.test(request.installation_id)||Object.hasOwn(request,'credential_env_ref')&&!credentialRef(request.credential_env_ref)))return failure('acp_discovery_invalid');
 if(acpAdapter(request.adapter_id).authStrategy==='explicit_api_key_env'&&request.action==='models'&&!credentialRef(request.credential_env_ref))return failure('acp_discovery_invalid');
 const ownsCommand=request.action!=='status';if(ownsCommand&&commandBusy)return failure('acp_discovery_unknown');if(ownsCommand)commandBusy=true;
 const scope=scopeOf(request);let record;
 try{
 const context=await check(event,scope);
 if(request.operation_id){record=load(request.operation_id);if(record){if(!equal(record.scope,scope))return failure('acp_discovery_invalid');if(request.action==='models'&&!equal(record.request,{installation_id:request.installation_id,...(request.credential_env_ref?{credential_env_ref:request.credential_env_ref}:{})}))return failure('acp_discovery_invalid');latest.set(keyOf(scope),record.operation_id);if(!equal(record.context,context)||record.source_sha256!==source(context,record.installation,record.request.credential_env_ref))return response(scope,'binding_changed',record);return response(scope,currentPhase(record),record);}}
 if(request.action==='status'){const previous=!request.operation_id&&latest.get(keyOf(scope));record=previous?load(previous):undefined;if(record&&(!equal(record.context,context)||record.source_sha256!==source(context,record.installation,record.request.credential_env_ref)))return response(scope,'binding_changed',record);return response(scope,record?currentPhase(record):request.operation_id?'unknown':installations.has(keyOf(scope))?'detected':'unconfigured',record);}
 if(active||busy())return failure('acp_discovery_unknown');
 if(request.action==='detect'){
 const found=await installation(event,request),after=await check(event,scope);if(!equal(context,after))return response(scope,'binding_changed');const before=installations.get(keyOf(scope))||[];
 const rows=found.rows.map(row=>({...row,installation_id:before.find(old=>old.executable===row.executable&&old.version===row.version&&old.executable_sha256===row.executable_sha256)?.installation_id||crypto.randomBytes(16).toString('hex')}));if(installations.size>=128&&!installations.has(keyOf(scope)))return failure('acp_discovery_unavailable');installations.set(keyOf(scope),rows);return response(scope,found.phase);
 }
 const chosen=installations.get(keyOf(scope))?.find(row=>row.installation_id===request.installation_id&&row.kind==='native');if(!chosen)return failure('acp_discovery_invalid');
 const found=await installation(event,request);if(!found.rows.some(row=>row.kind==='native'&&row.executable===chosen.executable&&row.version===chosen.version&&row.executable_sha256===chosen.executable_sha256))return response(scope,'binding_changed');
 const after=await check(event,scope);if(!equal(context,after))return response(scope,'binding_changed');
 const created=now();record={operation_id:request.operation_id,id:crypto.randomBytes(16).toString('hex'),scope,context,installation:chosen,source_sha256:source(context,chosen,request.credential_env_ref),created_at:new Date(created).toISOString(),expires_at:new Date(created+60000).toISOString(),phase:'reading',request:{installation_id:request.installation_id,...(request.credential_env_ref?{credential_env_ref:request.credential_env_ref}:{})}};
 save(record);latest.set(keyOf(scope),record.operation_id);
 const outcome=await runProbe(record,request.locale);record={...record,...outcome};save(record);
 const finalContext=await check(event,scope);if(!equal(finalContext,context)||record.source_sha256!==source(context,record.installation,record.request.credential_env_ref))return response(scope,'binding_changed',record);
 return response(scope,currentPhase(record),record);
 }catch(error){if(record&&record.phase==='reading'){record={...record,phase:'unknown'};try{save(record);}catch{}}if(error?.needsWorkspace)return {status:'ok',observation:{...scope,phase:'needs_workspace',installations:[]}};return failure(error?.authority||disposed||!caller(event)?'acp_discovery_authorization_changed':'acp_discovery_unavailable');}
 finally{if(ownsCommand)commandBusy=false;if(active?.record.operation_id===record?.operation_id&&record?.phase!=='reading'&&record?.phase!=='stop_unconfirmed')active=undefined;}
 }
 async function select(scope,selection,context,event){
 if(disposed||!acpAdapter(scope.adapter_id)||!fields(selection,['installation_id','probe_id','model_id'])||!ID.test(selection.installation_id)||!ID.test(selection.probe_id)||!text(selection.model_id,256))throw Error('selection');
 // Loaded records are bounded; a restart resolves the exact probe by journal
 // index rather than trusting a renderer-provided command or installation.
 let record=[...records.values()].find(row=>row.id===selection.probe_id);if(!record&&journal().findACP){const id=journal().findACP(selection.probe_id);if(id)record=load(id);}
 if(!record||!equal(record.scope,scopeOf(scope))||record.installation.installation_id!==selection.installation_id||currentPhase(record)!=='ready'||!validDiscovery(record.discovery,scope.adapter_id)||!equal(record.context,context)||record.source_sha256!==source(context,record.installation,record.request.credential_env_ref))throw Error('selection');
 const model=record.discovery.catalogue.models.find(row=>row.id===selection.model_id);if(!model)throw Error('selection');
 const found=await installation(event,{...scopeOf(scope),locale:scope.locale||'en-US'});if(!found.rows.some(row=>row.kind==='native'&&row.executable===record.installation.executable&&row.version===record.installation.version&&row.executable_sha256===record.installation.executable_sha256))throw Error('selection');
 if(now()>=Date.parse(record.expires_at))throw Error('selection');
 const configuration={executable:record.installation.executable,executable_sha256:record.installation.executable_sha256,reviewed_version:record.installation.version,model:model.id,...(record.request.credential_env_ref?{credential_env_ref:record.request.credential_env_ref}:{}),configuration_sha256:model.configuration_sha256};
 if(!validConfiguration(configuration,scope.adapter_id))throw Error('selection');return configuration;
 }
 async function dispose(){disposed=true;if(active){active.stop();let timer;try{await Promise.race([active.done,new Promise(resolve=>timer=setTimeout(resolve,4000))]);}finally{clearTimeout(timer);}}installations.clear();}
 return {inspect,select,dispose,isBusy:()=>!!active};
}
module.exports={CHANNEL,createLocalACPController,cleanEnvironment};
