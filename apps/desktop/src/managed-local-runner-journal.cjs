'use strict';
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const {ensurePrivateDirectory}=require('./workspace-grants.cjs');
const {validConfiguration,validDiscovery,credentialRef,fields}=require('./local-acp-validation.cjs');
const codex=require('./local-codex-validation.cjs');
const opencode=require('./local-opencode-validation.cjs');
const PROTOCOL='suma-desktop-managed-runner-journal-v1',MAX_BYTES=262144;
const ID=/^[a-f0-9]{32}$/,HASH=/^[a-f0-9]{64}$/,ADAPTERS=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','qoder-acp','codex-app-server','opencode-sdk']);
const PHASES=new Set(['unconfigured','needs_workspace','not_installed','unavailable','prepared','issuance_unknown','starting','connected','stopping','stopped','authorization_required','binding_changed','origin_changed','credential_unrecoverable','stop_unconfirmed','failed']);
const plain=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const exact=(value,keys)=>plain(value)&&Object.keys(value).sort().join(',')===[...keys].sort().join(',');
function validScope(value){return exact(value,['project_id','user_id','tenant_id','adapter_id'])&&['project_id','user_id','tenant_id'].every(k=>typeof value[k]==='string'&&ID.test(value[k]))&&ADAPTERS.has(value.adapter_id);}
function scopeKey(scope){if(!validScope(scope))throw Error('Invalid managed runner scope');return crypto.createHash('sha256').update([scope.user_id,scope.tenant_id,scope.project_id,scope.adapter_id].join('\0')).digest('hex');}
function validOrigin(value){try{const url=new URL(value);return url.origin===value&&url.protocol==='http:'&&url.hostname==='127.0.0.1'&&!!url.port&&!url.username&&!url.password;}catch{return false;}}
function safeText(value,max){return typeof value==='string'&&value.length>0&&Buffer.byteLength(value)<=max&&!/\p{Cc}/u.test(value);}
function validateRecord(value,scope){
 const required=['protocol','scope','origin','instance_id','repository_path','repository_identity','binding_sha256','base_commit','token','phase','request'];
 if(!plain(value)||Object.keys(value).some(k=>!required.includes(k)&&!['worker','acp_configuration','acp_selection','codex_configuration','codex_selection','opencode_configuration'].includes(k))||required.some(k=>!Object.hasOwn(value,k))||value.protocol!==PROTOCOL||!validScope(value.scope)||scopeKey(scope)!==scopeKey(value.scope)||!validOrigin(value.origin)||!HASH.test(value.instance_id)||!HASH.test(value.binding_sha256)||!HASH.test(value.token)||!(/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/).test(value.base_commit)||!path.isAbsolute(value.repository_path)||!safeText(value.repository_path,4096)||!safeText(value.repository_identity,1024)||!PHASES.has(value.phase))throw Error('Invalid managed runner journal');
 if(value.scope.adapter_id==='qoder-acp'?(!validConfiguration(value.acp_configuration)||!fields(value.acp_selection,['installation_id','probe_id','model_id'])||!ID.test(value.acp_selection.installation_id)||!ID.test(value.acp_selection.probe_id)||value.acp_selection.model_id!==value.acp_configuration.model):(Object.hasOwn(value,'acp_configuration')||Object.hasOwn(value,'acp_selection')))throw Error('Invalid managed ACP configuration');
 if(value.scope.adapter_id==='codex-app-server'?(!codex.validConfiguration(value.codex_configuration)||!fields(value.codex_selection,['installation_id','probe_id'])||!ID.test(value.codex_selection.installation_id)||!ID.test(value.codex_selection.probe_id)):(Object.hasOwn(value,'codex_configuration')||Object.hasOwn(value,'codex_selection')))throw Error('Invalid managed Codex configuration');
 if(value.scope.adapter_id==='opencode-sdk'?!opencode.validConfiguration(value.opencode_configuration):Object.hasOwn(value,'opencode_configuration'))throw Error('Invalid managed OpenCode configuration');
 const req=value.request,keys=['project_id','user_id','tenant_id','worker_id','adapter_id','instance_id','expected_binding_sha256','base_commit','credential_sha256','idempotency_key'];
 if(!exact(req,keys)||['project_id','user_id','tenant_id','adapter_id'].some(k=>req[k]!==scope[k])||req.instance_id!==value.instance_id||req.expected_binding_sha256!==value.binding_sha256||req.base_commit!==value.base_commit||req.credential_sha256!==crypto.createHash('sha256').update(value.token).digest('hex')||!(/^desktop-[a-f0-9]{32}$/).test(req.worker_id)||!(/^desktop-runner-[a-f0-9]{32}$/).test(req.idempotency_key))throw Error('Managed runner journal does not match its original request');
 if(value.worker!==undefined&&(!plain(value.worker)||value.worker.project_id!==scope.project_id||value.worker.worker_id!==req.worker_id||!ID.test(value.worker.id)||!Number.isInteger(value.worker.version)||value.worker.version<1||!['active','revoked'].includes(value.worker.status)||typeof value.worker.expires_at!=='string'||!Number.isFinite(Date.parse(value.worker.expires_at))))throw Error('Invalid managed runner receipt');
 return value;
}
function checkedRead(file,max){
 const before=fs.lstatSync(file);if(!before.isFile()||before.isSymbolicLink()||before.nlink!==1||before.size>max||before.size===0||process.platform!=='win32'&&(before.mode&0o077)!==0)throw Error('Invalid private journal file');
 const fd=fs.openSync(file,fs.constants.O_RDONLY|(fs.constants.O_NOFOLLOW||0));
 try{const opened=fs.fstatSync(fd);if(opened.dev!==before.dev||opened.ino!==before.ino||!opened.isFile())throw Error('Private journal changed');const raw=fs.readFileSync(fd);const after=fs.lstatSync(file),finished=fs.fstatSync(fd);if(raw.length!==before.size||after.isSymbolicLink()||after.dev!==opened.dev||after.ino!==opened.ino||finished.size!==opened.size||finished.mtimeMs!==opened.mtimeMs||after.mtimeMs!==opened.mtimeMs)throw Error('Private journal changed');return raw;}finally{fs.closeSync(fd);}
}
function syncDirectory(directory){let fd;try{fd=fs.openSync(directory,'r');fs.fsyncSync(fd);}catch(error){if(process.platform!=='win32'||!['EPERM','EACCES','EINVAL','EISDIR','EBADF'].includes(error.code))throw error;}finally{if(fd!==undefined)fs.closeSync(fd);}}
function createManagedRunnerJournal({userData,safeStorage,platform=process.platform}){
 if(!safeStorage||!safeStorage.isEncryptionAvailable()||platform==='linux'&&['basic_text','unknown'].includes(safeStorage.getSelectedStorageBackend?.()||'unknown'))throw Error('OS credential storage is unavailable');
 const directory=ensurePrivateDirectory(path.join(ensurePrivateDirectory(userData),'managed-runners'));
 if(process.platform!=='win32'&&(fs.statSync(directory).mode&0o077)!==0)throw Error('Private managed runner directory required');
 const lock=path.join(directory,'owner.lock'),owner=JSON.stringify({pid:process.pid,nonce:crypto.randomBytes(16).toString('hex')});let closed=false;
 function acquire(){
  let fd;try{fd=fs.openSync(lock,'wx',0o600);}catch(error){
   if(error.code!=='EEXIST')throw error;
   const original=checkedRead(lock,4096),previous=JSON.parse(original.toString('utf8'));if(!exact(previous,['pid','nonce'])||!Number.isInteger(previous.pid)||previous.pid<1||!ID.test(previous.nonce))throw Error('Managed runner ownership is unavailable');
   let stale=false;try{process.kill(previous.pid,0);}catch(status){stale=status.code==='ESRCH';}
   if(!stale||!checkedRead(lock,4096).equals(original))throw Error('Managed runner journal is already owned');
   fs.unlinkSync(lock);fd=fs.openSync(lock,'wx',0o600);
  }
  try{fs.writeFileSync(fd,owner);fs.fsyncSync(fd);}finally{fs.closeSync(fd);}syncDirectory(directory);
 }
 acquire();
 function assertOpen(){if(closed)throw Error('Managed runner journal is closed');ensurePrivateDirectory(directory);if(checkedRead(lock,4096).toString()!==owner)throw Error('Managed runner journal ownership changed');}
 function filename(scope){return path.join(directory,scopeKey(scope)+'.bin');}
 function read(scope){assertOpen();const file=filename(scope);let bytes;try{bytes=checkedRead(file,MAX_BYTES);}catch(error){if(error.code==='ENOENT')return;throw error;}const raw=safeStorage.decryptString(bytes);if(Buffer.byteLength(raw)>MAX_BYTES)throw Error('Managed runner journal is too large');return validateRecord(JSON.parse(raw),scope);}
 function write(value){
  assertOpen();validateRecord(value,value.scope);const raw=JSON.stringify(value);if(Buffer.byteLength(raw)>MAX_BYTES/2)throw Error('Managed runner journal is too large');const encrypted=safeStorage.encryptString(raw);if(!Buffer.isBuffer(encrypted)||encrypted.length===0||encrypted.length>MAX_BYTES)throw Error('OS credential encryption failed');
  const file=filename(value.scope);try{checkedRead(file,MAX_BYTES);}catch(error){if(error.code!=='ENOENT')throw error;}
  const temporary=file+'.'+crypto.randomBytes(16).toString('hex')+'.tmp';let fd;
  try{fd=fs.openSync(temporary,'wx',0o600);fs.writeFileSync(fd,encrypted);fs.fsyncSync(fd);fs.closeSync(fd);fd=undefined;fs.renameSync(temporary,file);syncDirectory(directory);}finally{if(fd!==undefined)fs.closeSync(fd);try{fs.unlinkSync(temporary);}catch(error){if(error.code!=='ENOENT')throw error;}}
 }
 function stateDirectory(scope){assertOpen();return ensurePrivateDirectory(path.join(ensurePrivateDirectory(path.join(directory,scopeKey(scope))),'runtime'));}
 function validateACP(value){
  const required=['operation_id','id','scope','context','installation','source_sha256','created_at','expires_at','phase','request'];
  if(!fields(value,required,['discovery'])||!ID.test(value.operation_id)||!ID.test(value.id)||!validScope(value.scope)||value.scope.adapter_id!=='qoder-acp'||!HASH.test(value.source_sha256)||!['reading','ready','failed','unknown','stop_unconfirmed'].includes(value.phase)||!Number.isFinite(Date.parse(value.created_at))||!Number.isFinite(Date.parse(value.expires_at))||Date.parse(value.expires_at)-Date.parse(value.created_at)!==60000)throw Error('Invalid ACP observation journal');
  const c=value.context,i=value.installation,r=value.request;
  if(!fields(c,['project_id','user_id','tenant_id','instance_id','binding_sha256','repository_path','repository_identity','base_commit'])||['project_id','user_id','tenant_id'].some(k=>c[k]!==value.scope[k])||!HASH.test(c.instance_id)||!HASH.test(c.binding_sha256)||!path.isAbsolute(c.repository_path)||!safeText(c.repository_path,4096)||!safeText(c.repository_identity,1024)||!(/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/).test(c.base_commit)||!fields(i,['installation_id','label','kind','executable','version','executable_sha256'])||!ID.test(i.installation_id)||i.kind!=='native'||!path.isAbsolute(i.executable)||!safeText(i.executable,4096)||!safeText(i.label,256)||!safeText(i.version,256)||!HASH.test(i.executable_sha256)||!fields(r,['installation_id'],['credential_env_ref'])||r.installation_id!==i.installation_id||Object.hasOwn(r,'credential_env_ref')&&!credentialRef(r.credential_env_ref))throw Error('Invalid ACP observation binding');
  if(value.discovery!==undefined&&(!validDiscovery(value.discovery)||value.discovery.executable_sha256!==i.executable_sha256||value.discovery.tool_version!==i.version)||value.phase==='ready'&&value.discovery?.state!=='ready')throw Error('Invalid ACP observation result');
  return value;
 }
 function acpPath(id){if(!ID.test(id))throw Error('Invalid ACP observation identity');return path.join(directory,'acp-'+id+'.bin');}
 function readACP(id){assertOpen();let raw;try{raw=checkedRead(acpPath(id),1048576);}catch(error){if(error.code==='ENOENT')return;throw error;}const clear=safeStorage.decryptString(raw);if(Buffer.byteLength(clear)>524288)throw Error('ACP observation exceeds journal limit');const value=validateACP(JSON.parse(clear));if(value.operation_id!==id)throw Error('ACP observation identity differs');return value;}
 function writeACP(value){
  assertOpen();validateACP(value);const file=acpPath(value.operation_id),prior=readACP(value.operation_id);
  if(!prior&&fs.readdirSync(directory).filter(name=>/^acp-[a-f0-9]{32}\.bin$/.test(name)).length>=128)throw Error('ACP observation journal is full');
  if(prior&&JSON.stringify({...prior,phase:undefined,discovery:undefined})!==JSON.stringify({...value,phase:undefined,discovery:undefined}))throw Error('ACP observation source changed');
  const clear=JSON.stringify(value);if(Buffer.byteLength(clear)>524288)throw Error('ACP observation exceeds journal limit');const encrypted=safeStorage.encryptString(clear);if(!Buffer.isBuffer(encrypted)||!encrypted.length||encrypted.length>1048576)throw Error('ACP observation encryption failed');
  const temporary=file+'.'+crypto.randomBytes(16).toString('hex')+'.tmp';let fd;
  try{fd=fs.openSync(temporary,'wx',0o600);fs.writeFileSync(fd,encrypted);fs.fsyncSync(fd);fs.closeSync(fd);fd=undefined;fs.renameSync(temporary,file);syncDirectory(directory);}finally{if(fd!==undefined)fs.closeSync(fd);try{fs.unlinkSync(temporary);}catch(error){if(error.code!=='ENOENT')throw error;}}
 }
 function findACP(probeID){assertOpen();if(!ID.test(probeID))throw Error('Invalid ACP probe identity');const names=fs.readdirSync(directory).filter(name=>/^acp-[a-f0-9]{32}\.bin$/.test(name));if(names.length>128)throw Error('ACP journal exceeds limit');for(const name of names){const id=name.slice(4,-4),value=readACP(id);if(value?.id===probeID)return id;}}
 function validateCodex(value){
  const required=['operation_id','id','scope','context','installation','source_sha256','created_at','expires_at','phase','request'];
  if(!fields(value,required,['discovery'])||!ID.test(value.operation_id)||!ID.test(value.id)||!validScope(value.scope)||value.scope.adapter_id!=='codex-app-server'||!HASH.test(value.source_sha256)||!['reading','ready','failed','unknown','stop_unconfirmed'].includes(value.phase)||!Number.isFinite(Date.parse(value.created_at))||Date.parse(value.expires_at)-Date.parse(value.created_at)!==60000)throw Error('Invalid Codex observation journal');
  const c=value.context,i=value.installation,r=value.request;
  if(!fields(c,['project_id','user_id','tenant_id','instance_id','binding_sha256','repository_path','repository_identity','base_commit'])||['project_id','user_id','tenant_id'].some(k=>c[k]!==value.scope[k])||!HASH.test(c.instance_id)||!HASH.test(c.binding_sha256)||!path.isAbsolute(c.repository_path)||!safeText(c.repository_path,4096)||!safeText(c.repository_identity,1024)||!(/^[a-f0-9]{40}(?:[a-f0-9]{24})?$/).test(c.base_commit)||!fields(i,['installation_id','label','kind','executable','version','executable_sha256'])||!ID.test(i.installation_id)||i.kind!=='native'||!path.isAbsolute(i.executable)||!safeText(i.executable,4096)||!safeText(i.label,256)||!safeText(i.version,256)||!HASH.test(i.executable_sha256)||!fields(r,['installation_id','configuration'])||r.installation_id!==i.installation_id||!codex.validProviderConfiguration(r.configuration))throw Error('Invalid Codex observation binding');
  if(value.discovery!==undefined&&(!codex.validDiscovery(value.discovery)||value.discovery.executable_sha256!==i.executable_sha256||value.discovery.tool_version!==i.version||value.discovery.configuration_sha256!==codex.configurationDigest(r.configuration))||value.phase==='ready'&&value.discovery?.state!=='ready')throw Error('Invalid Codex observation result');return value;
 }
 function codexPath(id){if(!ID.test(id))throw Error('Invalid Codex observation identity');return path.join(directory,'codex-'+id+'.bin');}
 function readCodex(id){assertOpen();let raw;try{raw=checkedRead(codexPath(id),1048576);}catch(error){if(error.code==='ENOENT')return;throw error;}const clear=safeStorage.decryptString(raw);if(Buffer.byteLength(clear)>524288)throw Error('Codex observation exceeds journal limit');const value=validateCodex(JSON.parse(clear));if(value.operation_id!==id)throw Error('Codex observation identity differs');return value;}
 function writeCodex(value){
  assertOpen();validateCodex(value);const file=codexPath(value.operation_id),prior=readCodex(value.operation_id);
  if(!prior&&fs.readdirSync(directory).filter(name=>/^codex-[a-f0-9]{32}\.bin$/.test(name)).length>=128)throw Error('Codex observation journal is full');
  if(prior&&JSON.stringify({...prior,phase:undefined,discovery:undefined})!==JSON.stringify({...value,phase:undefined,discovery:undefined}))throw Error('Codex observation source changed');
  const clear=JSON.stringify(value);if(Buffer.byteLength(clear)>524288)throw Error('Codex observation exceeds journal limit');const encrypted=safeStorage.encryptString(clear);if(!Buffer.isBuffer(encrypted)||!encrypted.length||encrypted.length>1048576)throw Error('Codex observation encryption failed');const temporary=file+'.'+crypto.randomBytes(16).toString('hex')+'.tmp';let fd;
  try{fd=fs.openSync(temporary,'wx',0o600);fs.writeFileSync(fd,encrypted);fs.fsyncSync(fd);fs.closeSync(fd);fd=undefined;fs.renameSync(temporary,file);syncDirectory(directory);}finally{if(fd!==undefined)fs.closeSync(fd);try{fs.unlinkSync(temporary);}catch(error){if(error.code!=='ENOENT')throw error;}}
 }
 function findCodex(probeID){assertOpen();if(!ID.test(probeID))throw Error('Invalid Codex probe identity');const names=fs.readdirSync(directory).filter(name=>/^codex-[a-f0-9]{32}\.bin$/.test(name));if(names.length>128)throw Error('Codex journal exceeds limit');for(const name of names){const id=name.slice(6,-4),value=readCodex(id);if(value?.id===probeID)return id;}}
 function close(){if(closed)return;assertOpen();fs.unlinkSync(lock);syncDirectory(directory);closed=true;}
 return {directory,read,write,stateDirectory,readACP,writeACP,findACP,readCodex,writeCodex,findCodex,close};
}
module.exports={PROTOCOL,createManagedRunnerJournal,scopeKey,validScope,validOrigin};
