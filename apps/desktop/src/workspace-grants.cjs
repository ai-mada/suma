'use strict';
const fs=require('node:fs');
const path=require('node:path');
const crypto=require('node:crypto');
const {message}=require('./localization.cjs');
const CHANNEL='suma:choose-project-directory';
const PROTOCOL='suma-desktop-workspace-v1';
const id=/^[a-f0-9]{32}$/;
class DesktopWorkspaceError extends Error{constructor(code){super('Desktop workspace operation is unavailable');this.code=code}}
function samePath(a,b){return process.platform==='win32'?a.toLowerCase()===b.toLowerCase():a===b}
function contained(root,target){const rel=path.relative(root,target);return rel!==''&&rel!=='..'&&!rel.startsWith('..'+path.sep)&&!path.isAbsolute(rel)}
function canonicalLocalPath(value,platform=process.platform){
 if(typeof value!=='string'||!value||/\p{Cc}/u.test(value))throw new DesktopWorkspaceError('picker_failed');
 let normalized;
 if(platform==='win32'){
  if(/^\\\\\?\\[A-Za-z]:\\/.test(value))value=value.slice(4);
  if(value.startsWith('\\\\')||value.startsWith('//')||!path.win32.isAbsolute(value)||!/^([A-Za-z]):[\\/]/.test(value))throw new DesktopWorkspaceError('picker_failed');
  normalized=path.win32.normalize(value);
 }else{
  if(value.startsWith('//')||value.startsWith('\\\\')||!path.posix.isAbsolute(value))throw new DesktopWorkspaceError('picker_failed');
  normalized=path.posix.normalize(value);
 }
 if(Buffer.byteLength(normalized,'utf8')>4096)throw new DesktopWorkspaceError('picker_failed');
 return normalized;
}
function ensurePrivateDirectory(directory){
 const absolute=path.resolve(directory),missing=[];
 for(let current=absolute;;current=path.dirname(current)){
  try{const stat=fs.lstatSync(current);if(!stat.isDirectory()||stat.isSymbolicLink())throw new DesktopWorkspaceError('workspace_key_failed')}
  catch(error){if(error.code==='ENOENT')missing.push(current);else throw new DesktopWorkspaceError('workspace_key_failed')}
  if(path.dirname(current)===current)break;
 }
 for(const item of missing.reverse())try{fs.mkdirSync(item,{mode:0o700})}catch(error){if(error.code!=='EEXIST')throw new DesktopWorkspaceError('workspace_key_failed');const s=fs.lstatSync(item);if(!s.isDirectory()||s.isSymbolicLink())throw new DesktopWorkspaceError('workspace_key_failed')}
 const resolved=canonicalLocalPath(fs.realpathSync(absolute));
 if(!samePath(path.normalize(absolute),resolved))throw new DesktopWorkspaceError('workspace_key_failed');
 return resolved;
}
function loadWorkspaceKey(userData){
 let fd;
 try{
  const parent=ensurePrivateDirectory(userData),file=path.join(parent,'desktop-workspace.key');
  try{
   fd=fs.openSync(file,'wx',0o600);
   const fresh=crypto.randomBytes(32);fs.writeFileSync(fd,fresh);fs.fsyncSync(fd);fs.closeSync(fd);fd=undefined;
  }catch(error){if(error.code!=='EEXIST')throw error}
  const before=fs.lstatSync(file);
  if(!before.isFile()||before.isSymbolicLink()||before.size!==32||process.platform!=='win32'&&(before.mode&0o077)!==0)throw new DesktopWorkspaceError('workspace_key_failed');
  fd=fs.openSync(file,fs.constants.O_RDONLY|(fs.constants.O_NOFOLLOW||0));
  const opened=fs.fstatSync(fd);
  if(!opened.isFile()||opened.dev!==before.dev||opened.ino!==before.ino||opened.size!==32)throw new DesktopWorkspaceError('workspace_key_failed');
  const bytes=Buffer.alloc(33);let count=0;
  while(count<bytes.length){const n=fs.readSync(fd,bytes,count,bytes.length-count,count);if(n===0)break;count+=n}
  const after=fs.lstatSync(file),finished=fs.fstatSync(fd);
  if(count!==32||after.isSymbolicLink()||!after.isFile()||after.dev!==opened.dev||after.ino!==opened.ino||after.size!==32||finished.size!==32||finished.mtimeMs!==opened.mtimeMs)throw new DesktopWorkspaceError('workspace_key_failed');
  return Buffer.from(bytes.subarray(0,32));
 }catch{throw new DesktopWorkspaceError('workspace_key_failed')}
 finally{if(fd!==undefined)try{fs.closeSync(fd)}catch{}}
}
function validRequest(request){
 try{return request!==null&&typeof request==='object'&&!Array.isArray(request)&&Object.keys(request).sort().join(',')==='locale,project_id,tenant_id,user_id'&&['project_id','user_id','tenant_id'].every(name=>typeof request[name]==='string'&&id.test(request[name]))&&['zh-CN','en-US'].includes(request.locale)}catch{return false}
}
function senderAllowed(event,window,origin){
 try{
  if(!window||window.isDestroyed()||window.webContents.isDestroyed()||event.sender!==window.webContents||event.senderFrame!==window.webContents.mainFrame)return false;
  const expected=new URL(origin);
  if(expected.protocol!=='http:'||expected.hostname!=='127.0.0.1'||!expected.port||expected.username||expected.password)return false;
  for(const value of [window.webContents.getURL(),event.senderFrame.url]){const url=new URL(value);if(url.origin!==expected.origin||url.username||url.password)return false}
  return true;
 }catch{return false}
}
function signDirectoryGrant(key,request,selected,{now=Date.now,random=crypto.randomBytes}={}){
 if(!Buffer.isBuffer(key)||key.length!==32||!validRequest(request))throw new DesktopWorkspaceError('invalid_request');
 const grant={protocol:PROTOCOL,operation_id:'desktop-workspace-'+random(16).toString('hex'),user_id:request.user_id,tenant_id:request.tenant_id,project_id:request.project_id,path:selected,expires_at:new Date(now()+120000).toISOString(),nonce:random(16).toString('hex')};
 grant.signature=crypto.createHmac('sha256',key).update([grant.protocol,grant.operation_id,grant.user_id,grant.tenant_id,grant.project_id,grant.path,grant.expires_at,grant.nonce].join('\0'),'utf8').digest('hex');
 return grant;
}
function createDirectoryPicker({key,dialog,getWindow,getOrigin,isShuttingDown,now=Date.now,realpath=fs.promises.realpath,stat=fs.promises.stat}){
 if(!Buffer.isBuffer(key)||key.length!==32)throw new DesktopWorkspaceError('workspace_key_failed');
 const privateKey=Buffer.from(key);let busy=false;
 return async(event,request)=>{
  if(isShuttingDown())return {status:'failed',code:'desktop_unavailable'};
  const window=getWindow(),origin=getOrigin();
  if(!senderAllowed(event,window,origin))return {status:'failed',code:'sender_invalid'};
  if(!validRequest(request))return {status:'failed',code:'invalid_request'};
  if(busy)return {status:'failed',code:'picker_busy'};
  const scope=Object.freeze({project_id:request.project_id,user_id:request.user_id,tenant_id:request.tenant_id,locale:request.locale});
  busy=true;
  try{
   const result=await dialog.showOpenDialog(window,{title:message(scope.locale,'workspace_choose_title'),buttonLabel:message(scope.locale,'workspace_choose_button'),properties:['openDirectory','dontAddToRecent']});
   if(isShuttingDown()||getWindow()!==window||!senderAllowed(event,window,getOrigin()))return {status:'failed',code:'sender_invalid'};
   if(result?.canceled===true)return {status:'cancelled'};
   if(result?.canceled!==false||!Array.isArray(result.filePaths)||result.filePaths.length!==1)throw new DesktopWorkspaceError('picker_failed');
   const requested=canonicalLocalPath(result.filePaths[0]);
   const selected=canonicalLocalPath(await realpath(requested));
   if(!(await stat(selected)).isDirectory())throw new DesktopWorkspaceError('picker_failed');
   if(isShuttingDown()||getWindow()!==window||!senderAllowed(event,window,getOrigin()))return {status:'failed',code:'sender_invalid'};
   return {status:'selected',path:selected,grant:signDirectoryGrant(privateKey,scope,selected,{now})};
  }catch{return {status:'failed',code:'picker_failed'}}
  finally{busy=false}
 };
}
function developmentOverrides(sourceDirectory,environment={},packaged=false){
 if(packaged)return null;
 const names=['SUMA_DESKTOP_USER_DATA','SUMA_DESKTOP_CONTROL_BINARY','SUMA_DESKTOP_WEB_DIR'];
 if(!names.some(name=>environment[name]!==undefined))return null;
 if(!names.every(name=>typeof environment[name]==='string'&&path.isAbsolute(environment[name])&&!/\p{Cc}/u.test(environment[name])))throw new DesktopWorkspaceError('desktop_configuration_invalid');
 const root=path.resolve(sourceDirectory,'../../..','.cache');
 const values=names.map(name=>path.resolve(environment[name]));
 if(values.some(value=>!contained(root,value)))throw new DesktopWorkspaceError('desktop_configuration_invalid');
 for(const [index,value] of values.entries()){
  for(let current=value;;current=path.dirname(current)){
   try{
    const info=fs.lstatSync(current);
    const directory=current!==value||index!==1;
    if(info.isSymbolicLink()||(directory?!info.isDirectory():!info.isFile()))throw new DesktopWorkspaceError('desktop_configuration_invalid');
    if(!samePath(path.normalize(current),canonicalLocalPath(fs.realpathSync(current))))throw new DesktopWorkspaceError('desktop_configuration_invalid');
   }catch(error){if(error.code!=='ENOENT')throw new DesktopWorkspaceError('desktop_configuration_invalid')}
   if(path.dirname(current)===current)break;
  }
 }
 return {userData:values[0],binary:values[1],web:values[2]};
}
function controlEnvironment(environment,{data,web,origin,key}){
 const clean={};for(const [name,value] of Object.entries(environment))if(!/^SUMA_(DB_.*|DESKTOP_.*|WORKSPACE_ROOT(?:_FILE)?|DATA_DIR(?:_FILE)?|WEB_DIR(?:_FILE)?|ADDR(?:_FILE)?|PUBLIC_URL(?:_FILE)?|ALLOWED_ORIGINS(?:_FILE)?)$/i.test(name))clean[name]=value;
 return {...clean,SUMA_DB_DRIVER:'sqlite',SUMA_DB_DSN:path.join(data,'suma.db'),SUMA_DATA_DIR:data,SUMA_WEB_DIR:web,SUMA_PUBLIC_URL:origin,SUMA_ALLOWED_ORIGINS:origin,SUMA_DESKTOP_WORKSPACE_KEY:key.toString('hex')};
}
module.exports={CHANNEL,PROTOCOL,canonicalLocalPath,ensurePrivateDirectory,loadWorkspaceKey,validRequest,senderAllowed,signDirectoryGrant,createDirectoryPicker,developmentOverrides,controlEnvironment};
