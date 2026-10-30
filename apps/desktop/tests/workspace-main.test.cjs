'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {EventEmitter}=require('node:events');
const repository=path.resolve(__dirname,'../../..');
const stage=path.join(repository,'.cache/e2e/desktop-107');
fs.mkdirSync(stage,{recursive:true});

async function mainHarness(t,environment={}){
 const userData=fs.mkdtempSync(path.join(stage,'main-test-'));
 t.after(()=>{const checked=fs.realpathSync(userData);assert.equal(path.dirname(checked),fs.realpathSync(stage));fs.rmSync(checked,{recursive:true,force:true})});
 const handlers=new Map(),events=new Map(),dialogs=[],spawns=[];let ownedWindow;
 const app={isPackaged:false,getLocale:()=> 'en-US',getPath:()=>userData,setPath:()=>{},requestSingleInstanceLock:()=>true,on:(name,fn)=>events.set(name,fn),quit:()=>{},whenReady:()=>Promise.resolve()};
 class Window extends EventEmitter{
  constructor(options){super();this.options=options;ownedWindow=this;const mainFrame={url:'about:blank'};this.webContents=Object.assign(new EventEmitter(),{mainFrame,isDestroyed:()=>false,getURL:()=>mainFrame.url,setWindowOpenHandler:()=>{},session:{setPermissionRequestHandler:()=>{},setPermissionCheckHandler:()=>{}}})}
  isDestroyed(){return false} async loadURL(url){this.webContents.mainFrame.url=url} show(){} close(){this.emit('closed')}
 }
 const child=new EventEmitter();Object.assign(child,{pid:123,exitCode:null,signalCode:null,kill(){this.signalCode='SIGTERM';queueMicrotask(()=>this.emit('exit',null,'SIGTERM'));return true}});
 const source=fs.readFileSync(path.join(__dirname,'../src/main.cjs'),'utf8');
 const context={__dirname:path.join(__dirname,'../src'),process:{platform:process.platform,arch:process.arch,env:environment},Buffer,URL,setTimeout,clearTimeout,require:id=>{
  if(id==='electron')return {app,BrowserWindow:Window,ipcMain:{handle:(name,fn)=>handlers.set(name,fn),removeHandler:name=>handlers.delete(name)},dialog:{showErrorBox:(...args)=>dialogs.push(args),showOpenDialog:async()=>({canceled:true,filePaths:[]})}};
  if(id==='node:fs')return {...fs,existsSync:()=>true};
  if(id==='node:child_process')return {spawn:(...args)=>{spawns.push(args);return child}};
  if(id==='./lifecycle.cjs')return {...require('../src/lifecycle.cjs'),waitForHealth:async()=>{}};
  if(id.startsWith('./'))return require(path.join(__dirname,'../src',id));
  return require(id);
 }};
 vm.runInNewContext(source,context);
 const until=Date.now()+1000;while(!ownedWindow&&!dialogs.length&&Date.now()<until)await new Promise(resolve=>setTimeout(resolve,5));
 assert.equal(dialogs.length,0,'owned desktop must start without a generic error dialog');
 return {handlers,events,spawns,window:ownedWindow};
}

test('main exposes fixed picker and OAuth channels while guarding the owned picker frame and local database',async t=>{
 const h=await mainHarness(t,{SUMA_DB_DRIVER:'postgres',SUMA_DB_DSN:'postgres://untrusted.example',SUMA_WORKSPACE_ROOT:'untrusted-root'});
 assert.deepEqual([...h.handlers.keys()].sort(),['suma:choose-project-directory','suma:open-workbuddy-authorization']);
 const handler=h.handlers.get('suma:choose-project-directory');assert.equal(typeof handler,'function');
 const env=h.spawns[0][2].env;
 const target=path.join(repository,'.cache/desktop',`${process.platform}-${process.arch}`);
 assert.equal(h.spawns[0][0],path.join(target,'control',process.platform==='win32'?'suma-control.exe':'suma-control'));
 assert.equal(env.SUMA_WEB_DIR,path.join(target,'web'),'development must consume the same prepared target');
 assert.equal(env.SUMA_DB_DRIVER,'sqlite');assert.equal(env.SUMA_DB_DSN.endsWith(path.join('data','suma.db')),true);
 assert.equal(Object.hasOwn(env,'SUMA_WORKSPACE_ROOT'),false);
 assert.equal(/^[a-f0-9]{64}$/.test(env.SUMA_DESKTOP_WORKSPACE_KEY),true,'child needs the private local key');
 const request={project_id:'a'.repeat(32),user_id:'b'.repeat(32),tenant_id:'c'.repeat(32),locale:'en-US'};
 const cancelled=await handler({sender:h.window.webContents,senderFrame:h.window.webContents.mainFrame},request);
 assert.equal(cancelled.status,'cancelled');
 const refused=await handler({sender:{},senderFrame:h.window.webContents.mainFrame},request);
 assert.equal(refused.status,'failed');assert.equal(refused.code,'sender_invalid');
 assert.equal(h.window.options.webPreferences.contextIsolation,true);assert.equal(h.window.options.webPreferences.sandbox,true);assert.equal(h.window.options.webPreferences.nodeIntegration,false);
});
