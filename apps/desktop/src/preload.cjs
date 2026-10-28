'use strict';
const {contextBridge,ipcRenderer}=require('electron');
contextBridge.exposeInMainWorld('sumaDesktop',Object.freeze({platform:process.platform,version:'1.0.0',chooseProjectDirectory:async request=>{
 try{return await ipcRenderer.invoke('suma:choose-project-directory',request)}catch{return {status:'failed',code:'desktop_unavailable'}}
}}));
