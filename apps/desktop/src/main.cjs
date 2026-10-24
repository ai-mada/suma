'use strict';
const {app,BrowserWindow,dialog}=require('electron');
const {spawn}=require('node:child_process');
const {existsSync,mkdirSync}=require('node:fs');
const {join,resolve}=require('node:path');
const net=require('node:net');
const {waitForHealth,allowedNavigation,terminateService}=require('./lifecycle.cjs');
let service;let window;let shuttingDown=false;let quitRequested=false;
function availablePort(){return new Promise((resolvePort,reject)=>{const server=net.createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(error=>error?reject(error):resolvePort(port))})})}
async function stopService(){await terminateService(service)}
async function start(){
 const resources=app.isPackaged?process.resourcesPath:resolve(__dirname,'../../..');
 const binary=app.isPackaged?join(resources,'control',process.platform==='win32'?'suma-control.exe':'suma-control'):resolve(resources,'.cache/desktop/control',process.platform==='win32'?'suma-control.exe':'suma-control');
 const web=app.isPackaged?join(resources,'web'):resolve(resources,'apps/web/dist');
 if(!existsSync(binary))throw Error('The bundled control service is missing. Build the desktop bundle first.');if(!existsSync(join(web,'index.html')))throw Error('The web bundle is missing. Build the workbench first.');
 const data=join(app.getPath('userData'),'data');mkdirSync(data,{recursive:true});const port=await availablePort();const origin=`http://127.0.0.1:${port}`;
 service=spawn(binary,['--addr',`127.0.0.1:${port}`],{cwd:data,env:{...process.env,SUMA_DATA_DIR:data,SUMA_WEB_DIR:web},stdio:'ignore',windowsHide:true});let exited=false;let spawnError;service.once('error',error=>{spawnError=error;exited=true});service.once('exit',()=>{exited=true;if(window && !shuttingDown){dialog.showErrorBox('Suma service stopped','The local service stopped. Restart Suma to reconnect.');window.close()}});
 await waitForHealth(origin,()=>exited);if(spawnError)throw spawnError;
 window=new BrowserWindow({width:1440,height:960,minWidth:960,minHeight:640,show:false,title:'Suma',backgroundColor:'#101014',webPreferences:{preload:join(__dirname,'preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true,webSecurity:true,allowRunningInsecureContent:false}});
 window.webContents.setWindowOpenHandler(()=>({action:'deny'}));window.webContents.on('will-navigate',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.on('will-redirect',(event,url)=>{if(!allowedNavigation(url,origin))event.preventDefault()});window.webContents.session.setPermissionRequestHandler((_contents,_permission,callback)=>callback(false));window.webContents.session.setPermissionCheckHandler(()=>false);
 window.once('ready-to-show',()=>window.show());window.once('closed',()=>{window=undefined});await window.loadURL(origin);
}
if(!app.requestSingleInstanceLock()){app.quit()}else{
 app.on('second-instance',()=>{if(window){if(window.isMinimized())window.restore();window.focus()}});
 app.whenReady().then(start).catch(async error=>{shuttingDown=true;dialog.showErrorBox('Suma could not start',error.message);try{await stopService()}finally{app.quit()}});
 app.on('window-all-closed',()=>app.quit());
 app.on('before-quit',event=>{if(quitRequested)return;event.preventDefault();shuttingDown=true;void stopService().catch(error=>dialog.showErrorBox('Suma service shutdown failed',error.message)).finally(()=>{quitRequested=true;app.quit()})});
}
