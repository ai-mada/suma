'use strict';
const {app,BrowserWindow,dialog}=require('electron');
const {spawn}=require('node:child_process');
const {existsSync,mkdirSync}=require('node:fs');
const {join,resolve}=require('node:path');
const net=require('node:net');
const {waitForHealth,allowedNavigation,terminateService}=require('./lifecycle.cjs');
const {resolveLocale,message,localError}=require('./localization.cjs');
let service;let window;let shuttingDown=false;let quitRequested=false;let locale='zh-CN';
function availablePort(){return new Promise((resolvePort,reject)=>{const server=net.createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(error=>error?reject(error):resolvePort(port))})})}
async function stopService(){await terminateService(service,5000,locale)}
async function start(){
 // app.getLocale 在 ready 之后读取；SUMA_LANGUAGE 只覆盖本进程显示偏好。
 locale=resolveLocale(app.getLocale(),process.env);
 const resources=app.isPackaged?process.resourcesPath:resolve(__dirname,'../../..');
 const binary=app.isPackaged?join(resources,'control',process.platform==='win32'?'suma-control.exe':'suma-control'):resolve(resources,'.cache/desktop/control',process.platform==='win32'?'suma-control.exe':'suma-control');
 const web=app.isPackaged?join(resources,'web'):resolve(resources,'apps/web/dist');
 if(!existsSync(binary))throw localError(locale,'missing_control');if(!existsSync(join(web,'index.html')))throw localError(locale,'missing_web');
 const data=join(app.getPath('userData'),'data');mkdirSync(data,{recursive:true});const port=await availablePort();const origin=`http://127.0.0.1:${port}`;
 service=spawn(binary,['--addr',`127.0.0.1:${port}`],{cwd:data,env:{...process.env,SUMA_DATA_DIR:data,SUMA_WEB_DIR:web},stdio:'ignore',windowsHide:true});let exited=false;let spawnError;service.once('error',error=>{spawnError=error;exited=true});service.once('exit',()=>{exited=true;if(window && !shuttingDown){dialog.showErrorBox(message(locale,'service_stopped_title'),message(locale,'service_stopped'));window.close()}});
 try{await waitForHealth(origin,()=>exited,15000,locale)}catch(error){if(spawnError)throw localError(locale,'startup_failed',spawnError);throw error}
 window=new BrowserWindow({width:1440,height:960,minWidth:960,minHeight:640,show:false,title:message(locale,'window_title'),backgroundColor:'#101014',webPreferences:{preload:join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true,allowRunningInsecureContent:false}});
 window.webContents.setWindowOpenHandler(()=>({action:'deny'}));window.webContents.on('will-navigate',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.on('will-redirect',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.session.setPermissionRequestHandler((_contents,_permission,callback)=>callback(false));window.webContents.session.setPermissionCheckHandler(()=>false);
 window.once('ready-to-show',()=>window.show());window.once('closed',()=>{window=undefined});await window.loadURL(origin);
}
if(!app.requestSingleInstanceLock()){app.quit()}else{
 app.on('second-instance',()=>{if(window){if(window.isMinimized())window.restore();window.focus()}});
 app.whenReady().then(start).catch(async error=>{shuttingDown=true;dialog.showErrorBox(message(locale,'startup_failed'),error.message);try{await stopService()}catch(stopError){dialog.showErrorBox(message(locale,'shutdown_failed'),stopError.message)}finally{app.quit()}});
 app.on('window-all-closed',()=>app.quit());
 app.on('before-quit',event=>{if(quitRequested)return;event.preventDefault();shuttingDown=true;void stopService().catch(error=>dialog.showErrorBox(message(locale,'shutdown_failed'),error.message)).finally(()=>{quitRequested=true;app.quit()})});
}
