'use strict';
const {contextBridge}=require('electron');
contextBridge.exposeInMainWorld('sumaDesktop',Object.freeze({platform:process.platform,version:'1.0.0'}));
