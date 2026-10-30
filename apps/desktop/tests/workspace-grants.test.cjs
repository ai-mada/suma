'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const vm=require('node:vm');
const grants=require('../src/workspace-grants.cjs');
const stage=path.resolve(__dirname,'../../../.cache/e2e/desktop-107');
fs.mkdirSync(stage,{recursive:true});
const scope=()=>({project_id:'a'.repeat(32),user_id:'b'.repeat(32),tenant_id:'c'.repeat(32),locale:'en-US'});
function folder(t){const dir=fs.mkdtempSync(path.join(stage,'key-test-'));t.after(()=>{const resolved=fs.realpathSync(dir);assert.equal(path.dirname(resolved),fs.realpathSync(stage));fs.rmSync(resolved,{recursive:true,force:true})});return dir}
function sender(){const frame={url:'http://127.0.0.1:49107/development?view=ide'};const contents={mainFrame:frame,getURL:()=>frame.url,isDestroyed:()=>false};const window={webContents:contents,isDestroyed:()=>false};return {window,event:{sender:contents,senderFrame:frame},origin:'http://127.0.0.1:49107'}}
function verify(key,grant){const raw=[grant.protocol,grant.operation_id,grant.user_id,grant.tenant_id,grant.project_id,grant.path,grant.expires_at,grant.nonce].join('\0');return crypto.createHmac('sha256',key).update(raw,'utf8').digest('hex')===grant.signature}

test('real key stays 32 bytes across restart and refuses malformed keys without resetting them',t=>{
 const dir=folder(t),key=grants.loadWorkspaceKey(dir),again=grants.loadWorkspaceKey(dir);
 assert.equal(key.length,32);assert.equal(crypto.timingSafeEqual(key,again),true);
 const file=path.join(dir,'desktop-workspace.key');assert.equal(fs.statSync(file).size,32);
 fs.writeFileSync(file,'broken');assert.throws(()=>grants.loadWorkspaceKey(dir),error=>error.code==='workspace_key_failed');assert.equal(fs.readFileSync(file,'utf8'),'broken');
});
test('linked key is refused without changing its target',t=>{
 const dir=folder(t),file=path.join(dir,'desktop-workspace.key'),target=path.join(dir,'unique-target');fs.writeFileSync(target,Buffer.alloc(32,7));
 try{fs.symlinkSync(target,file,'file')}catch(error){if(['EPERM','EACCES'].includes(error.code)){t.skip('Windows file symlink permission is unavailable');return}throw error}
 assert.throws(()=>grants.loadWorkspaceKey(dir),error=>error.code==='workspace_key_failed');assert.equal(fs.statSync(target).size,32);
});
test('grant binds every scope field and expires 120 seconds after selection completes',async t=>{
 const dir=folder(t),key=grants.loadWorkspaceKey(dir),selected=fs.realpathSync(dir),host=sender();let clock=1000,dialogOptions;
 const picker=grants.createDirectoryPicker({key,dialog:{showOpenDialog:async(_window,options)=>{dialogOptions=options;clock=50000;return {canceled:false,filePaths:[dir]}}},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false,now:()=>clock});
 const request=scope(),result=await picker(host.event,request);
 assert.equal(result.status,'selected');assert.equal(result.path,selected);assert.equal(verify(key,result.grant),true);assert.equal(result.grant.expires_at,new Date(170000).toISOString());
 assert.equal(Object.keys(result.grant).sort().join(','),'expires_at,nonce,operation_id,path,project_id,protocol,signature,tenant_id,user_id');
 assert.equal(/desktop-workspace-[a-f0-9]{32}$/.test(result.grant.operation_id),true);assert.equal(/^[a-f0-9]{32}$/.test(result.grant.nonce),true);assert.equal(Object.hasOwn(dialogOptions,'defaultPath'),false);assert.deepEqual(dialogOptions.properties,['openDirectory','dontAddToRecent']);
 assert.equal(/^[a-f0-9]{64}$/.test(result.grant.signature),true);
 for(const [name,value] of Object.entries({protocol:'other-protocol',project_id:'d'.repeat(32),user_id:'e'.repeat(32),tenant_id:'f'.repeat(32),path:selected+'-other',expires_at:new Date(171000).toISOString(),nonce:'0'.repeat(32),operation_id:'desktop-workspace-'+'0'.repeat(32)}))assert.equal(verify(key,{...result.grant,[name]:value}),false);
 const later=grants.signDirectoryGrant(key,request,selected,{now:()=>clock});assert.notEqual(later.operation_id,result.grant.operation_id);assert.notEqual(later.nonce,result.grant.nonce);assert.equal(verify(grants.loadWorkspaceKey(dir),result.grant),true);
});
test('fixed request validation rejects additional fields and non-string identities',()=>{
 for(const request of [null,[],{...scope(),extra:true},{...scope(),locale:'en-GB'},{...scope(),project_id:'A'.repeat(32)},{...scope(),user_id:new String('b'.repeat(32))}])assert.equal(grants.validRequest(request),false);
});
test('picker cancellation has no side effects, blocks concurrency and rechecks the sender after dialog',async()=>{
 const host=sender();let finish,calls=0;
 const picker=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{showOpenDialog:()=>{calls++;return new Promise(resolve=>finish=resolve)}},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false});
 const first=picker(host.event,scope());assert.equal((await picker(host.event,scope())).code,'picker_busy');finish({canceled:true,filePaths:[]});assert.deepEqual(await first,{status:'cancelled'});assert.equal(calls,1);
 const second=picker(host.event,scope());host.window.webContents.mainFrame.url='https://untrusted.example/';finish({canceled:false,filePaths:['private-do-not-print']});assert.deepEqual(await second,{status:'failed',code:'sender_invalid'});
});
test('sender checks reject other contents, subframes, wrong origin, closed window and shutdown',async()=>{
 const host=sender(),picker=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{showOpenDialog:()=>{throw Error('must not open')}},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false});
 for(const event of [{sender:{},senderFrame:host.event.senderFrame},{sender:host.event.sender,senderFrame:{url:host.origin}},{sender:host.event.sender,senderFrame:null}])assert.equal((await picker(event,scope())).code,'sender_invalid');
 assert.equal((await picker(host.event,{...scope(),extra:true})).code,'invalid_request');
 host.window.webContents.mainFrame.url='http://127.0.0.1:49108';assert.equal((await picker(host.event,scope())).code,'sender_invalid');
 host.window.webContents.mainFrame.url=host.origin;host.window.isDestroyed=()=>true;assert.equal((await picker(host.event,scope())).code,'sender_invalid');
 const shutdown=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>true});assert.equal((await shutdown(host.event,scope())).code,'desktop_unavailable');
});
test('selection rejects files, UNC and control characters and normalizes extended drive paths',async t=>{
 const dir=folder(t),file=path.join(dir,'source.txt');fs.writeFileSync(file,'fixture');const host=sender();
 const picker=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{showOpenDialog:async()=>({canceled:false,filePaths:[file]})},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false});assert.equal((await picker(host.event,scope())).code,'picker_failed');
 for(const value of ['\\\\server\\share','\\\\?\\UNC\\server\\share','C:\\bad\nname'])assert.throws(()=>grants.canonicalLocalPath(value,'win32'));
 assert.equal(grants.canonicalLocalPath('\\\\?\\C:\\work\\project','win32'),'C:\\work\\project');
});

test('canonical selection paths are limited to 4096 UTF-8 bytes before a grant is minted',async()=>{
 const prefix='C:\\',exact=prefix+'界'.repeat(1364)+'x',tooLong=exact+'x';
 assert.equal(Buffer.byteLength(exact,'utf8'),4096);assert.equal(grants.canonicalLocalPath(exact,'win32'),exact);
 assert.throws(()=>grants.canonicalLocalPath(tooLong,'win32'),error=>error.code==='picker_failed');
 const host=sender(),local=process.platform==='win32'?tooLong:'/'+ '界'.repeat(1366);let inspected=false;
 const picker=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{showOpenDialog:async()=>({canceled:false,filePaths:[local]})},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false,realpath:async()=>{inspected=true;throw Error('must not resolve overlong input')}});
 assert.deepEqual(await picker(host.event,scope()),{status:'failed',code:'picker_failed'});assert.equal(inspected,false);
});
test('sender is checked again after asynchronous canonical path validation',async t=>{
 const dir=folder(t),host=sender();
 const picker=grants.createDirectoryPicker({key:Buffer.alloc(32,1),dialog:{showOpenDialog:async()=>({canceled:false,filePaths:[dir]})},getWindow:()=>host.window,getOrigin:()=>host.origin,isShuttingDown:()=>false,realpath:async()=>{host.window.webContents.mainFrame.url='https://untrusted.example';return fs.realpathSync(dir)}});
 const result=await picker(host.event,scope());assert.equal(result.code,'sender_invalid');assert.equal(Object.hasOwn(result,'grant'),false);
});
test('development overrides stay inside the repository cache and are ignored in packaged mode',t=>{
 const source=path.resolve(__dirname,'../src'),env={SUMA_DESKTOP_USER_DATA:path.join(stage,'userdata'),SUMA_DESKTOP_CONTROL_BINARY:path.join(stage,'control/suma-control.exe'),SUMA_DESKTOP_WEB_DIR:path.join(stage,'web')};
 assert.equal(grants.developmentOverrides(source,env,false).userData,env.SUMA_DESKTOP_USER_DATA);assert.equal(grants.developmentOverrides(source,env,true),null);
 const otherCache=path.resolve(stage,'../../desktop-development');assert.equal(grants.developmentOverrides(source,{...env,SUMA_DESKTOP_USER_DATA:path.join(otherCache,'userdata')},false).userData,path.join(otherCache,'userdata'));
 assert.throws(()=>grants.developmentOverrides(source,{...env,SUMA_DESKTOP_USER_DATA:path.resolve(stage,'../../../user-data')},false));assert.throws(()=>grants.developmentOverrides(source,{SUMA_DESKTOP_USER_DATA:env.SUMA_DESKTOP_USER_DATA},false));
 const dir=folder(t),nonDirectory=path.join(dir,'ordinary-file');fs.writeFileSync(nonDirectory,'owned fixture');assert.throws(()=>grants.developmentOverrides(source,{...env,SUMA_DESKTOP_USER_DATA:path.join(nonDirectory,'child')},false),error=>error.code==='desktop_configuration_invalid');
 const target=path.join(dir,'target'),linked=path.join(dir,'linked');fs.mkdirSync(target);fs.symlinkSync(target,linked,process.platform==='win32'?'junction':'dir');
 try{assert.throws(()=>grants.developmentOverrides(source,{...env,SUMA_DESKTOP_WEB_DIR:path.join(linked,'web')},false),error=>error.code==='desktop_configuration_invalid')}finally{fs.unlinkSync(linked)}
});
test('preload exposes only the fixed bridge and keeps IPC failures opaque',async()=>{
 let exposed,reject=false;const calls=[];
 const electron={contextBridge:{exposeInMainWorld:(name,value)=>{assert.equal(name,'sumaDesktop');exposed=value}},ipcRenderer:{invoke:async(...args)=>{calls.push(args);if(reject)throw Error('private diagnostic');return {status:'cancelled'}}}};
 const source=fs.readFileSync(path.join(__dirname,'../src/preload.cjs'),'utf8');
 vm.runInNewContext(source,{process:{platform:'win32'},require:id=>{assert.equal(id,'electron');return electron}});
 assert.equal(Object.keys(exposed).sort().join(','),'chooseProjectDirectory,openWorkBuddyAuthorization,platform,version');assert.equal((await exposed.chooseProjectDirectory(scope())).status,'cancelled');assert.equal(calls[0][0],grants.CHANNEL);
 reject=true;const failed=await exposed.chooseProjectDirectory(scope());assert.equal(failed.code,'desktop_unavailable');assert.equal(Object.keys(failed).sort().join(','),'code,status');
});
