'use strict';
const {app,BrowserWindow,dialog,ipcMain,safeStorage}=require('electron');
const {spawn}=require('node:child_process');
const {existsSync,lstatSync}=require('node:fs');
const {join,resolve}=require('node:path');
const {selectLocalServicePort,reserveLocalServicePort}=require('./local-service-port.cjs');
const {waitForHealth,allowedNavigation,terminateService}=require('./lifecycle.cjs');
const {resolveLocale,message,localError}=require('./localization.cjs');
const {CHANNEL,loadWorkspaceKey,createDirectoryPicker,developmentOverrides,ensurePrivateDirectory,controlEnvironment}=require('./workspace-grants.cjs');
const {createWorkBuddyOAuthController}=require('./oauth-window.cjs');
const {CHANNEL:TOOL_DISCOVERY_CHANNEL,createInstalledToolDiscoveryController}=require('./installed-tools.cjs');
const {CHANNEL:MANAGED_RUNNER_CHANNEL,createManagedLocalRunnerController}=require('./managed-local-runner.cjs');
const OAUTH_CHANNEL='suma:open-workbuddy-authorization';
let oauthController;
function disposeOAuth(){ipcMain.removeHandler(OAUTH_CHANNEL);oauthController?.dispose();oauthController=undefined;}
let installedToolController;let installedToolStop=Promise.resolve();
function disposeInstalledTools(){ipcMain.removeHandler(TOOL_DISCOVERY_CHANNEL);if(installedToolController)installedToolStop=installedToolController.dispose();installedToolController=undefined;return installedToolStop;}
let managedRunnerController;let managedRunnerStop=Promise.resolve();
function disposeManagedRunner(){ipcMain.removeHandler(MANAGED_RUNNER_CHANNEL);if(managedRunnerController)managedRunnerStop=managedRunnerController.dispose();managedRunnerController=undefined;return managedRunnerStop;}
let service;let window;let shuttingDown=false;let quitRequested=false;let locale='zh-CN';let origin='';let overrides;let configurationError;
try{overrides=developmentOverrides(__dirname,process.env,app.isPackaged);if(overrides)app.setPath('userData',ensurePrivateDirectory(overrides.userData))}catch{configurationError='desktop_configuration_invalid'}
async function stopService(){await terminateService(service,5000,locale)}
async function start(){
 // app.getLocale 在 ready 之后读取；SUMA_LANGUAGE 只覆盖本进程显示偏好。
 locale=resolveLocale(app.getLocale(),process.env);
 if(configurationError)throw localError(locale,configurationError);
 const resources=app.isPackaged?process.resourcesPath:resolve(__dirname,'../../..');
 // Development preparation and distribution use the same target identity.
 // Keep each architecture's executable and Web assets together; never consume
 // a different target's last build from a shared staging directory.
 const prepared=resolve(resources,'.cache/desktop',`${process.platform}-${process.arch}`);
 const binary=overrides?.binary||(app.isPackaged?join(resources,'control',process.platform==='win32'?'suma-control.exe':'suma-control'):join(prepared,'control',process.platform==='win32'?'suma-control.exe':'suma-control'));
 const web=overrides?.web||(app.isPackaged?join(resources,'web'):join(prepared,'web'));
 const sourceInspector=join(app.isPackaged?resources:prepared,'source-inspector');
 const sourceInspectorNode=join(sourceInspector,process.platform==='win32'?'node.exe':'node');
 const sourceInspectorEntry=join(sourceInspector,'source-inspector.bundle.mjs');
 if(!existsSync(binary))throw localError(locale,'missing_control');if(!existsSync(join(web,'index.html')))throw localError(locale,'missing_web');
 if(!existsSync(sourceInspectorNode)||!existsSync(sourceInspectorEntry))throw localError(locale,'missing_source_inspector');
 let key;try{key=loadWorkspaceKey(app.getPath('userData'))}catch{throw localError(locale,'workspace_key_failed')}
 let data;try{
  data=ensurePrivateDirectory(join(app.getPath('userData'),'data'));
  try{const database=lstatSync(join(data,'suma.db'));if(!database.isFile()||database.isSymbolicLink())throw Error('invalid local database')}catch(error){if(error.code!=='ENOENT')throw error}
 }catch{throw localError(locale,'desktop_data_failed')}
 let port;try{port=selectLocalServicePort(process.env)}catch{throw localError(locale,'desktop_port_invalid')}
 try{await reserveLocalServicePort(port)}catch{throw localError(locale,'desktop_port_unavailable')}
 origin=`http://127.0.0.1:${port}`;
 const environment=controlEnvironment(process.env,{data,web,origin,key});
 // Fixed packaged resources, never renderer input or inherited executable
 // settings. The Go service pins their real-file hashes before first use.
 environment.SUMA_SOURCE_INSPECTOR_NODE=sourceInspectorNode;
 environment.SUMA_SOURCE_INSPECTOR_ENTRY=sourceInspectorEntry;
 service=spawn(binary,['--addr',`127.0.0.1:${port}`],{cwd:data,env:environment,stdio:'ignore',windowsHide:true});let exited=false;let spawnError;service.once('error',()=>{spawnError=true;exited=true});service.once('exit',()=>{exited=true;if(window && !shuttingDown){dialog.showErrorBox(message(locale,'service_stopped_title'),message(locale,'service_stopped'));window.close()}});
 try{await waitForHealth(origin,()=>exited,15000,locale)}catch(error){if(spawnError)throw localError(locale,'startup_failed');throw error}
 window=new BrowserWindow({width:1440,height:960,minWidth:960,minHeight:640,show:false,title:message(locale,'window_title'),backgroundColor:'#101014',webPreferences:{preload:join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true,allowRunningInsecureContent:false}});
 window.webContents.setWindowOpenHandler(()=>({action:'deny'}));window.webContents.on('will-navigate',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.on('will-redirect',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.session.setPermissionRequestHandler((_contents,_permission,callback)=>callback(false));window.webContents.session.setPermissionCheckHandler(()=>false);
 ipcMain.handle(CHANNEL,createDirectoryPicker({key,dialog,getWindow:()=>window,getOrigin:()=>origin,isShuttingDown:()=>shuttingDown}));
 oauthController=createWorkBuddyOAuthController({BrowserWindow,parent:window,origin});
 ipcMain.handle(OAUTH_CHANNEL,(event,request)=>oauthController.open(event,request));
 // Installation checks reuse this distribution's exact control executable in
 // its credential-free CLI mode; no second runtime is downloaded or installed.
 installedToolController=createInstalledToolDiscoveryController({parent:window,origin,binary,tempDirectory:ensurePrivateDirectory(join(data,'.cache','tool-discovery'))});
 ipcMain.handle(TOOL_DISCOVERY_CHANNEL,(event,request)=>installedToolController.discover(event,request));
 managedRunnerController=createManagedLocalRunnerController({parent:window,origin,binary,key,userData:app.getPath('userData'),safeStorage,discover:(event,request)=>installedToolController.discover(event,request)});
 ipcMain.handle(MANAGED_RUNNER_CHANNEL,(event,request)=>managedRunnerController.manage(event,request));
 window.once('ready-to-show',()=>window.show());window.once('closed',()=>{disposeOAuth();disposeInstalledTools();disposeManagedRunner();window=undefined});await window.loadURL(origin);
}
if(!configurationError&&!app.requestSingleInstanceLock()){app.quit()}else{
 app.on('second-instance',()=>{if(window){if(window.isMinimized())window.restore();window.focus()}});
 app.whenReady().then(start).catch(async error=>{shuttingDown=true;dialog.showErrorBox(message(locale,'startup_failed'),error.message);try{await stopService()}catch(stopError){dialog.showErrorBox(message(locale,'shutdown_failed'),stopError.message)}finally{app.quit()}});
 app.on('window-all-closed',()=>app.quit());
 app.on('before-quit',event=>{if(quitRequested)return;event.preventDefault();shuttingDown=true;disposeOAuth();const localChildrenStopped=Promise.all([disposeManagedRunner(),disposeInstalledTools()]);ipcMain?.removeHandler(CHANNEL);void localChildrenStopped.then(stopService).catch(error=>dialog.showErrorBox(message(locale,'shutdown_failed'),error.message)).finally(()=>{quitRequested=true;app.quit()})});
}
