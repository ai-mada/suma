'use strict';
const {app,BrowserWindow,session}=require('electron'),path=require('node:path'),fs=require('node:fs');
const origin=process.env.SUMA_FEEDBACK204_ORIGIN,dir=process.env.SUMA_FEEDBACK204_DIRECTORY;
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin||'')||!path.isAbsolute(dir||''))throw Error('Expected isolated local origin and absolute evidence directory');
app.commandLine.appendSwitch('force-device-scale-factor','1');app.disableHardwareAcceleration();
for(const [key,name]of Object.entries({userData:'profile',sessionData:'session',temp:'tmp',crashDumps:'crash'})){const target=path.join(dir,name);fs.mkdirSync(target,{recursive:true});app.setPath(key,target);}
app.whenReady().then(async()=>{
 const isolated=session.fromPartition('tool-report-feedback204');
 isolated.setPermissionRequestHandler((_w,_p,done)=>done(false));
 isolated.webRequest.onBeforeRequest((request,done)=>{const url=new URL(request.url);done({cancel:!['about:','data:','blob:'].includes(url.protocol)&&url.origin!==origin});});
 const window=new BrowserWindow({show:false,width:1366,height:1000,webPreferences:{offscreen:true,sandbox:true,contextIsolation:true,nodeIntegration:false,backgroundThrottling:false,session:isolated}});
 window.webContents.setWindowOpenHandler(()=>({action:'deny'}));await window.loadURL('about:blank');
});
app.on('window-all-closed',()=>app.quit());
