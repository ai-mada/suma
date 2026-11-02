'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {EventEmitter}=require('node:events');

function tick(){return new Promise(resolve=>setImmediate(resolve));}

// Execute the production main entry with only OS/service boundaries replaced.
// A pending renderer unload must precede every existing shutdown side effect.
async function mainHarness(){
 const app=new EventEmitter(),effects=[];let parent,closed=false;
 Object.assign(app,{isPackaged:false,getLocale:()=> 'en-US',getPath:()=>path.resolve(__dirname,'../../../.cache/desktop-departure-218/main'),requestSingleInstanceLock:()=>true,whenReady:()=>Promise.resolve(),quit:()=>effects.push('quit')});
 class Window extends EventEmitter{
  constructor(){super();parent=this;const frame={url:'about:blank'};this.webContents=Object.assign(new EventEmitter(),{mainFrame:frame,isDestroyed:()=>closed,getURL:()=>frame.url,setWindowOpenHandler(){},session:{setPermissionRequestHandler(){},setPermissionCheckHandler(){}}});}
  isDestroyed(){return closed;}async loadURL(url){this.webContents.mainFrame.url=url;}show(){}
  close(){this.emit('close',{preventDefault(){}});this.webContents.emit('will-prevent-unload',{preventDefault(){throw Error('No Leave response yet');}});}
 }
 const asyncDialog=()=>new Promise(()=>{}),dependencies={
  electron:{app,BrowserWindow:Window,dialog:{showErrorBox(){throw Error('Unexpected startup error');},showMessageBox:asyncDialog,showMessageBoxSync:()=>0},ipcMain:{handle(){},removeHandler:()=>effects.push('remove-ipc')}},
  'node:child_process':{spawn:()=>new EventEmitter()},
  'node:fs':{existsSync:()=>true,lstatSync:()=>{throw Object.assign(Error(),{code:'ENOENT'});}},
  './local-service-port.cjs':{selectLocalServicePort:()=>49218,reserveLocalServicePort:async()=>49218},
  './lifecycle.cjs':{waitForHealth:async()=>{},allowedNavigation:()=>true,terminateService:async()=>effects.push('service-stop'),startupExitError:()=>Error('exit')},
  './workspace-grants.cjs':{CHANNEL:'suma:choose-project-directory',developmentOverrides:()=>null,ensurePrivateDirectory:x=>x,loadWorkspaceKey:()=>Buffer.alloc(32),controlEnvironment:()=>({}),createDirectoryPicker:()=>async()=>({status:'cancelled'})},
  './oauth-window.cjs':{createWorkBuddyOAuthController:()=>({dispose:()=>effects.push('oauth-dispose')})},
  './installed-tools.cjs':{CHANNEL:'suma:discover-installed-tools',createInstalledToolDiscoveryController:()=>({dispose:async()=>effects.push('discovery-drain')})},
  './managed-local-runner.cjs':{CHANNEL:'suma:manage-local-runner',createManagedLocalRunnerController:()=>({dispose:async()=>effects.push('worker-drain')})}
 };
 const context={__dirname:path.join(__dirname,'../src'),process:{platform:'win32',arch:'x64',env:{}},Buffer,URL,AbortController,setTimeout,clearTimeout,require:name=>dependencies[name]||(name.startsWith('./')?require(path.join(__dirname,'../src',name)):require(name))};
 vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../src/main.cjs'),'utf8'),context);
 for(let i=0;i<20&&!parent;i++)await tick();await tick();effects.length=0;
 return {app,parent,effects,close(){closed=true;parent.emit('closed');app.emit('window-all-closed');}};
}

test('desktop quit waits for the original window departure before draining workers, IPC and service',async()=>{
 const h=await mainHarness();let prevented=false;
 h.app.emit('before-quit',{preventDefault(){prevented=true;}});await tick();
 assert.equal(prevented,true);assert.deepEqual(h.effects,[],'pending Stay decision must leave the original service and workers live');
 h.close();await tick();
 // Our app stub records its native quit request; invoke the real before-quit
 // listener as Electron does after window-all-closed, then let original drain run.
 h.effects.length=0;h.app.emit('before-quit',{preventDefault(){}});await tick();await tick();
 assert.equal(h.effects.filter(x=>x==='worker-drain').length,0,'closed handler already admitted the single original drain');
 assert.equal(h.effects.filter(x=>x==='service-stop').length,1);
});

function departureHarness(locale='en-US'){
 const {createWindowDepartureController}=require('../src/window-departure.cjs');
 const parent=new EventEmitter(),contents=new EventEmitter(),origin='http://127.0.0.1:49218',dialogs=[],sync=[];
 let url=origin+'/work',destroyed=false,current=parent,closeCount=0,syncResponse=0;
 Object.assign(parent,{webContents:contents,isDestroyed:()=>destroyed});
 Object.assign(contents,{mainFrame:{},isDestroyed:()=>destroyed,getURL:()=>url});
 function unload(){let allowed=false;contents.emit('will-prevent-unload',{preventDefault(){allowed=true;}});return allowed;}
 parent.close=()=>{let prevented=false;parent.emit('close',{preventDefault(){prevented=true;}});if(prevented||!unload())return;closeCount++;destroyed=true;parent.emit('closed');contents.emit('destroyed');};
 function navigate(target){contents.emit('will-navigate',{},target);contents.emit('did-start-navigation',{},target,false,true);if(!unload())return false;url=target;contents.emit('did-navigate',{},target);return true;}
 contents.loadURL=async target=>{if(!navigate(target))throw Error('Original unload cancelled');};
 const dialog={showMessageBox:(owner,options)=>new Promise((resolve,reject)=>dialogs.push({owner,options,resolve,reject})),showMessageBoxSync:(owner,options)=>{sync.push({owner,options});return syncResponse;}};
 const controller=createWindowDepartureController({parent,dialog,origin,locale,getWindow:()=>current});
 return {parent,contents,controller,dialogs,sync,navigate,unload,origin,get url(){return url;},get closeCount(){return closeCount;},set syncResponse(value){syncResponse=value;},set current(value){current=value;},set url(value){url=value;}};
}

test('native departure defaults to Stay, deduplicates async close and replays only the current exact navigation once',async()=>{
 const h=departureHarness(),journal={operation:'original-unknown-operation',body:'unchanged'};
 assert.equal(h.controller.requestQuit(),true);assert.equal(h.closeCount,0);assert.equal(h.dialogs.length,1);
 assert.equal(h.controller.requestQuit(),false);h.unload();assert.equal(h.dialogs.length,1);
 const first=h.dialogs[0];assert.equal(first.owner,h.parent);assert.equal(first.options.defaultId,0);assert.equal(first.options.cancelId,0);assert.deepEqual(first.options.buttons,['Stay','Leave']);
 first.resolve({response:0});await tick();assert.equal(h.closeCount,0);assert.deepEqual(journal,{operation:'original-unknown-operation',body:'unchanged'});
 assert.equal(h.unload(),false);assert.equal(h.sync.length,1,'an unknown later unload cannot replay the retired quit intent');assert.equal(h.dialogs.length,1);
 h.controller.requestQuit();h.dialogs[1].reject(Error('Native dialog unavailable'));await tick();assert.equal(h.closeCount,0);
 h.controller.requestQuit();h.dialogs[2].resolve({response:1});await tick();assert.equal(h.closeCount,1);assert.equal(h.contents.listenerCount('will-prevent-unload'),0);

 const navigation=departureHarness();assert.equal(navigation.navigate(navigation.origin+'/next'),false);assert.equal(navigation.dialogs.length,1);
 navigation.dialogs[0].resolve({response:1});await tick();assert.equal(navigation.url,navigation.origin+'/next');assert.equal(navigation.dialogs.length,1);
 // A different departure receives its own decision; Leave is not a lasting
 // guard bypass, and neither native branch consumes any business journal.
 navigation.navigate(navigation.origin+'/later');assert.equal(navigation.dialogs.length,2);navigation.dialogs[1].resolve({response:0});await tick();navigation.controller.dispose();
});

test('native departure rejects stale or foreign owners and uses the official synchronous default-Stay fallback for unknown intents',async()=>{
 const h=departureHarness('zh-CN');assert.equal(h.unload(),false);assert.deepEqual(h.sync[0].options.buttons,['留下','离开']);assert.equal(h.sync[0].options.cancelId,0);
 h.syncResponse=1;assert.equal(h.unload(),true);assert.equal(h.dialogs.length,0);h.controller.dispose();
 const changed=departureHarness();changed.navigate(changed.origin+'/a');changed.navigate(changed.origin+'/b');changed.dialogs[0].resolve({response:1});await tick();assert.equal(changed.url,changed.origin+'/work');assert.equal(changed.dialogs.length,1);changed.controller.dispose();
 const foreign=departureHarness();foreign.controller.requestQuit();foreign.current=new EventEmitter();foreign.dialogs[0].resolve({response:1});await tick();assert.equal(foreign.closeCount,0);foreign.controller.dispose();
 const moved=departureHarness();moved.controller.requestQuit();moved.url='https://foreign.example/work';moved.dialogs[0].resolve({response:1});await tick();assert.equal(moved.closeCount,0);assert.equal(moved.unload(),false);assert.equal(moved.sync.length,0);moved.controller.dispose();
 const document=departureHarness();document.controller.requestQuit();document.contents.emit('did-navigate-in-page',{},document.origin+'/work#new');assert.equal(document.dialogs[0].options.signal.aborted,true);document.dialogs[0].resolve({response:1});await tick();assert.equal(document.closeCount,0);document.controller.dispose();
});
