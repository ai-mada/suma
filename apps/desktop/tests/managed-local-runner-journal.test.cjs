'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..','.cache','desktop-tests','managed153-journal');
const scope={project_id:'1'.repeat(32),user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'codex-jsonl'};
function directory(t,caseRoot=root){fs.mkdirSync(caseRoot,{recursive:true});const dir=fs.mkdtempSync(path.join(caseRoot,'case-'));t.after(()=>{for(const cleanup of t.managedCleanup||[])cleanup();const relative=path.relative(caseRoot,path.resolve(dir));assert.ok(relative.startsWith('case-')&&!relative.includes(path.sep));fs.rmSync(dir,{recursive:true,force:true});});return dir;}
function storage(){const key=crypto.randomBytes(32);return {isEncryptionAvailable:()=>true,getSelectedStorageBackend:()=> 'gnome_libsecret',encryptString:value=>{const iv=crypto.randomBytes(12),cipher=crypto.createCipheriv('aes-256-gcm',key,iv);const bytes=Buffer.concat([cipher.update(value,'utf8'),cipher.final()]);return Buffer.concat([iv,cipher.getAuthTag(),bytes]);},decryptString:raw=>{const decipher=crypto.createDecipheriv('aes-256-gcm',key,raw.subarray(0,12));decipher.setAuthTag(raw.subarray(12,28));return Buffer.concat([decipher.update(raw.subarray(28)),decipher.final()]).toString('utf8');}};}
function record(){const token='9'.repeat(64),instance='4'.repeat(64),binding='5'.repeat(64),base='6'.repeat(40),operation='desktop-runner-'+ '7'.repeat(32);return {protocol:'suma-desktop-managed-runner-journal-v1',scope,origin:'http://127.0.0.1:4319',instance_id:instance,repository_path:root,repository_identity:'opaque-root-identity',binding_sha256:binding,base_commit:base,token,phase:'prepared',request:{...scope,worker_id:'desktop-'+ '8'.repeat(32),instance_id:instance,expected_binding_sha256:binding,base_commit:base,credential_sha256:crypto.createHash('sha256').update(token).digest('hex'),idempotency_key:operation}};}
test('private encrypted journal survives reopen with the original operation and no plaintext token',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),userData=directory(t),safeStorage=storage(),value=record();
 let journal=createManagedRunnerJournal({userData,safeStorage});journal.write(value);const files=fs.readdirSync(journal.directory);assert.ok(files.some(name=>name.endsWith('.bin')));
 for(const name of files)if(fs.statSync(path.join(journal.directory,name)).isFile())assert.equal(fs.readFileSync(path.join(journal.directory,name)).includes(Buffer.from(value.token)),false);
 journal.close();journal=createManagedRunnerJournal({userData,safeStorage});assert.deepEqual(journal.read(scope),value);assert.equal(journal.read({...scope,adapter_id:'codebuddy-cli'}),undefined);journal.close();
});
test('live journal ownership refuses a second owner and releases only its own lock',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),options={userData:directory(t),safeStorage:storage()},first=createManagedRunnerJournal(options);
 assert.throws(()=>createManagedRunnerJournal(options));first.close();const second=createManagedRunnerJournal(options);second.close();
});
test('unavailable OS encryption and Linux plaintext backend fail before persistence',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),userData=directory(t);
 for(const backend of ['basic_text','unknown'])assert.throws(()=>createManagedRunnerJournal({userData,safeStorage:{...storage(),getSelectedStorageBackend:()=>backend},platform:'linux'}));
 assert.throws(()=>createManagedRunnerJournal({userData,safeStorage:{...storage(),isEncryptionAvailable:()=>false}}));assert.deepEqual(fs.readdirSync(userData),[]);
});
test('ciphertext corruption refuses recovery and preserves the original bytes',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),journal=createManagedRunnerJournal({userData:directory(t),safeStorage:storage()});(t.managedCleanup??=[]).push(()=>journal.close());journal.write(record());
 const file=path.join(journal.directory,fs.readdirSync(journal.directory).find(name=>name.endsWith('.bin'))),damaged=Buffer.from('not a valid encrypted journal');fs.writeFileSync(file,damaged);assert.throws(()=>journal.read(scope));assert.deepEqual(fs.readFileSync(file),damaged);
});
test('record scope and credential digest cannot be swapped during recovery',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),journal=createManagedRunnerJournal({userData:directory(t),safeStorage:storage()});(t.managedCleanup??=[]).push(()=>journal.close());
 const value=record();value.request.credential_sha256='0'.repeat(64);assert.throws(()=>journal.write(value));
 const other=record();other.request.user_id='a'.repeat(32);assert.throws(()=>journal.write(other));
});
test('journal never traverses a linked userData directory',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),userData=directory(t),target=directory(t),link=path.join(userData,'linked');
 try{fs.symlinkSync(target,link,process.platform==='win32'?'junction':'dir');}catch(error){if(error.code==='EPERM'){t.skip('symlink permission unavailable');return;}throw error;}
 assert.throws(()=>createManagedRunnerJournal({userData:link,safeStorage:storage()}));assert.deepEqual(fs.readdirSync(target),[]);fs.unlinkSync(link);
});
test('ACP original probe and managed configuration persist encrypted, and changed original sources cannot replace them',t=>{
 const {createManagedRunnerJournal}=require('../src/managed-local-runner-journal.cjs'),userData=directory(t),safeStorage=storage();let journal=createManagedRunnerJournal({userData,safeStorage});
 const value=record();value.scope={...value.scope,adapter_id:'qoder-acp'};value.request={...value.request,adapter_id:'qoder-acp'};value.acp_configuration={executable:process.execPath,executable_sha256:'a'.repeat(64),reviewed_version:'Qoder reviewed',model:'one',credential_env_ref:'LOCAL_QODER_CREDENTIAL',configuration_sha256:'b'.repeat(64)};value.acp_selection={installation_id:'a'.repeat(32),probe_id:'b'.repeat(32),model_id:'one'};
 const probe={operation_id:'c'.repeat(32),id:value.acp_selection.probe_id,scope:value.scope,context:{project_id:value.scope.project_id,user_id:value.scope.user_id,tenant_id:value.scope.tenant_id,instance_id:value.instance_id,binding_sha256:value.binding_sha256,repository_path:value.repository_path,repository_identity:value.repository_identity,base_commit:value.base_commit},installation:{installation_id:value.acp_selection.installation_id,label:'qoder.exe',kind:'native',executable:process.execPath,version:'Qoder reviewed',executable_sha256:'a'.repeat(64)},source_sha256:'d'.repeat(64),created_at:'2026-10-31T01:00:00.000Z',expires_at:'2026-10-31T01:01:00.000Z',phase:'reading',request:{installation_id:value.acp_selection.installation_id,credential_env_ref:'LOCAL_QODER_CREDENTIAL'}};
 journal.write(value);journal.writeACP(probe);journal.close();journal=createManagedRunnerJournal({userData,safeStorage});(t.managedCleanup??=[]).push(()=>journal.close());assert.deepEqual(journal.read(value.scope),value);assert.deepEqual(journal.readACP(probe.operation_id),probe);assert.equal(journal.findACP(probe.id),probe.operation_id);
 const file=path.join(journal.directory,'acp-'+probe.operation_id+'.bin'),before=fs.readFileSync(file);assert.equal(before.includes(Buffer.from('LOCAL_QODER_CREDENTIAL')),false);assert.throws(()=>journal.writeACP({...probe,source_sha256:'e'.repeat(64)}));assert.deepEqual(fs.readFileSync(file),before);
 journal.writeACP({...probe,phase:'unknown'});assert.equal(journal.readACP(probe.operation_id).phase,'unknown');
});

test('T189 Cursor and Qoder journal identities cannot exchange credentials or a discovery runtime',t=>{
 const {createManagedRunnerJournal,scopeKey,validScope}=require('../src/managed-local-runner-journal.cjs'),cursor={...scope,adapter_id:'cursor-cli'},qoder={...scope,adapter_id:'qoder-acp'};
 assert.equal(validScope(cursor),true);assert.notEqual(scopeKey(cursor),scopeKey(qoder));
 const userData=directory(t,path.resolve(__dirname,'../../../.cache/cursor-acp-189/desktop/journal')),safeStorage=storage(),journal=createManagedRunnerJournal({userData,safeStorage});(t.managedCleanup??=[]).push(()=>journal.close());
 const value=record();value.scope=qoder;value.request={...value.request,adapter_id:qoder.adapter_id};value.acp_configuration={executable:process.execPath,executable_sha256:'a'.repeat(64),reviewed_version:'Qoder reviewed',model:'one',credential_env_ref:'LOCAL_QODER_CREDENTIAL',configuration_sha256:'b'.repeat(64)};value.acp_selection={installation_id:'a'.repeat(32),probe_id:'b'.repeat(32),model_id:'one'};
 journal.write(value);assert.equal(journal.read(cursor),undefined);assert.deepEqual(journal.read(qoder),value);
 const originalFile=path.join(journal.directory,scopeKey(qoder)+'.bin'),originalBytes=fs.readFileSync(originalFile);
 assert.throws(()=>journal.write({...value,scope:cursor,request:{...value.request,adapter_id:qoder.adapter_id}}));
 const cursorValue=record();cursorValue.scope=cursor;cursorValue.token='a'.repeat(64);cursorValue.request={...cursorValue.request,adapter_id:'cursor-cli',worker_id:'desktop-'+'c'.repeat(32),idempotency_key:'desktop-runner-'+'d'.repeat(32),credential_sha256:crypto.createHash('sha256').update(cursorValue.token).digest('hex')};cursorValue.acp_configuration={...value.acp_configuration,executable:path.join(userData,process.platform==='win32'?'node.exe':'node'),reviewed_version:'Cursor reviewed',credential_env_ref:'LOCAL_CURSOR_CREDENTIAL',configuration_sha256:'c'.repeat(64)};cursorValue.acp_selection={installation_id:'e'.repeat(32),probe_id:'f'.repeat(32),model_id:'one'};
 journal.write(cursorValue);assert.deepEqual(journal.read(cursor),cursorValue);assert.deepEqual(journal.read(qoder),value);assert.notEqual(cursorValue.request.idempotency_key,value.request.idempotency_key);
 const probe={operation_id:'c'.repeat(32),id:cursorValue.acp_selection.probe_id,scope:cursor,context:{project_id:cursor.project_id,user_id:cursor.user_id,tenant_id:cursor.tenant_id,instance_id:value.instance_id,binding_sha256:value.binding_sha256,repository_path:userData,repository_identity:value.repository_identity,base_commit:value.base_commit},installation:{installation_id:cursorValue.acp_selection.installation_id,label:'Cursor Node package host',kind:'native',executable:cursorValue.acp_configuration.executable,version:'Cursor reviewed',executable_sha256:'a'.repeat(64)},source_sha256:'d'.repeat(64),created_at:'2026-11-01T08:00:00.000Z',expires_at:'2026-11-01T08:01:00.000Z',phase:'reading',request:{installation_id:cursorValue.acp_selection.installation_id,credential_env_ref:'LOCAL_CURSOR_CREDENTIAL'}};
 journal.writeACP(probe);const probeFile=path.join(journal.directory,'acp-'+probe.operation_id+'.bin'),probeBytes=fs.readFileSync(probeFile);
 assert.throws(()=>journal.writeACP({...probe,phase:'ready',discovery:{protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'unknown',executable_sha256:'a'.repeat(64),tool_version:'Cursor reviewed',error_code:'acp_discovery_unknown',evidence:{session_created:false,session_close_requested:false,session_close_acknowledged:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true}}}));
 assert.deepEqual(fs.readFileSync(probeFile),probeBytes);assert.deepEqual(journal.readACP(probe.operation_id),probe);assert.deepEqual(fs.readFileSync(originalFile),originalBytes);assert.deepEqual(journal.read(cursor),cursorValue);assert.deepEqual(journal.read(qoder),value);
 assert.equal(fs.readFileSync(path.join(journal.directory,scopeKey(cursor)+'.bin')).includes(Buffer.from('LOCAL_CURSOR_CREDENTIAL')),false);
});
