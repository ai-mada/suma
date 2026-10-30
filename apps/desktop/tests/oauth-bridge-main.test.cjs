'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {EventEmitter}=require('node:events');
const channel='suma:open-workbuddy-authorization';

async function mainHarness(){
 const app=new EventEmitter(),handlers=new Map(),calls=[];let window,options,disposed=0,stopped=0,loaded=false;
 Object.assign(app,{isPackaged:false,getLocale:()=> 'en-US',getPath:()=>path.resolve(__dirname,'../../../.cache/workbuddy-147-desktop/bridge-fixture'),requestSingleInstanceLock:()=>true,whenReady:()=>Promise.resolve(),quit:()=>{}});
 class Window extends EventEmitter{
  constructor(){super();window=this;this.webContents=new EventEmitter();this.webContents.session={setPermissionRequestHandler(){},setPermissionCheckHandler(){}};this.webContents.setWindowOpenHandler=fn=>{this.popup=fn};}
  async loadURL(url){this.url=url;loaded=true;}show(){}close(){this.emit('closed');}
 }
 const controller={open:async(event,request)=>{calls.push({event,request});return {status:'opened'};},dispose:()=>{disposed++;}};
 const child=new EventEmitter();
 const dependencies={
  electron:{app,BrowserWindow:Window,dialog:{showErrorBox(){throw Error('unexpected startup dialog');}},ipcMain:{handle:(name,fn)=>handlers.set(name,fn),removeHandler:name=>handlers.delete(name)}},
  'node:child_process':{spawn:()=>child},
  'node:fs':{existsSync:()=>true,lstatSync:()=>{throw Object.assign(Error(),{code:'ENOENT'});}},
  './local-service-port.cjs':{selectLocalServicePort:()=>49147,reserveLocalServicePort:async()=>49147},
  './lifecycle.cjs':{waitForHealth:async()=>{},allowedNavigation:()=>true,terminateService:async()=>{stopped++;},startupExitError:()=>Error('exit')},
  './localization.cjs':{resolveLocale:()=> 'en-US',message:(_locale,key)=>key,localError:(_locale,key)=>Error(key)},
  './workspace-grants.cjs':{CHANNEL:'suma:choose-project-directory',developmentOverrides:()=>null,ensurePrivateDirectory:x=>x,loadWorkspaceKey:()=>Buffer.alloc(32),controlEnvironment:()=>({}),createDirectoryPicker:()=>async()=>({status:'cancelled'})},
  './oauth-window.cjs':{createWorkBuddyOAuthController:value=>{options=value;return controller;}},
  './installed-tools.cjs':{CHANNEL:'suma:discover-installed-tools',createInstalledToolDiscoveryController:()=>({discover:async()=>({status:'failed'}),dispose:()=>Promise.resolve()})},
  './managed-local-runner.cjs':{CHANNEL:'suma:manage-local-runner',createManagedLocalRunnerController:()=>({manage:async()=>({status:'failed'}),dispose:()=>Promise.resolve()})}
 };
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../src/main.cjs'),'utf8'),{__dirname:path.join(__dirname,'../src'),process:{platform:'win32',arch:'x64',env:{}},Buffer,require:name=>dependencies[name]||require(name)});
 for(let n=0;n<20&&!loaded;n++)await new Promise(resolve=>setImmediate(resolve));
 assert.equal(loaded,true);
 return {app,handlers,calls,window,options,get disposed(){return disposed;},get stopped(){return stopped;}};
}

test('desktop OAuth entry forwards original event and fixed request to the owned controller while denying ordinary popups',async()=>{
 const h=await mainHarness();
 assert.equal(typeof h.handlers.get(channel),'function');
 assert.equal(h.options.parent,h.window);assert.equal(h.options.origin,h.window.url);
 assert.equal(Object.keys(h.options).sort().join(','),'BrowserWindow,origin,parent');
 assert.equal(h.window.popup({url:'https://untrusted.example'}).action,'deny');
 const event={sender:h.window.webContents},request={project_id:'project',authorization_id:'a'.repeat(32),user_id:'user',tenant_id:'tenant'};
 assert.equal((await h.handlers.get(channel)(event,request)).status,'opened');
 assert.equal(h.calls[0].event,event);assert.equal(h.calls[0].request,request);
 h.window.close();
 assert.equal(h.handlers.has(channel),false);assert.equal(h.disposed,1);
});

test('desktop shutdown removes OAuth admission and disposes before existing service shutdown, without double disposal',async()=>{
 const h=await mainHarness();let prevented=false;
 h.app.emit('before-quit',{preventDefault(){prevented=true;}});
 assert.equal(prevented,true);assert.equal(h.handlers.has(channel),false);assert.equal(h.disposed,1);
 await new Promise(resolve=>setImmediate(resolve));assert.equal(h.stopped,1);h.window.close();assert.equal(h.disposed,1);
 await new Promise(resolve=>setImmediate(resolve));
});

test('preload OAuth method uses its sole fixed IPC channel and makes transport failures opaque',async()=>{
 let exposed,reject=false;const calls=[];
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../src/preload.cjs'),'utf8'),{process:{platform:'win32'},require:name=>{assert.equal(name,'electron');return {contextBridge:{exposeInMainWorld:(name,value)=>{assert.equal(name,'sumaDesktop');exposed=value;}},ipcRenderer:{invoke:async(...args)=>{calls.push(args);if(reject)throw Error('sensitive diagnostic');return {status:'opened'};}}};}});
 assert.equal(Object.isFrozen(exposed),true);assert.equal(typeof exposed.chooseProjectDirectory,'function');
 const request={project_id:'project',authorization_id:'a'.repeat(32),user_id:'user',tenant_id:'tenant'};
 assert.equal((await exposed.openWorkBuddyAuthorization(request)).status,'opened');
 assert.equal(calls[0][0],channel);assert.equal(calls[0][1],request);
 reject=true;const failure=await exposed.openWorkBuddyAuthorization(request);
 assert.equal(failure.status,'failed');assert.equal(failure.code,'desktop_unavailable');assert.equal(Object.keys(failure).sort().join(','),'code,status');
});
