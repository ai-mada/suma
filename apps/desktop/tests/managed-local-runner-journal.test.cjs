'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const root=path.resolve(__dirname,'../../..','.cache','desktop-tests','managed153-journal');
const scope={project_id:'1'.repeat(32),user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'codex-jsonl'};
function directory(t){fs.mkdirSync(root,{recursive:true});const dir=fs.mkdtempSync(path.join(root,'case-'));t.after(()=>{for(const cleanup of t.managedCleanup||[])cleanup();const relative=path.relative(root,path.resolve(dir));assert.ok(relative.startsWith('case-')&&!relative.includes(path.sep));fs.rmSync(dir,{recursive:true,force:true});});return dir;}
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
