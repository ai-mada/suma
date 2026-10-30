'use strict';

const OFFICIAL='https://www.workbuddy.cn';
const CALLBACK='/api/v1/workbuddy-oauth/callback';
const ID=/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/;
const OP_ID=/^[a-f0-9]{32}$/;
const KNOWN_SCOPES=new Set(['user.task.readable','user.task.invokable']);
const MAX_AGE=5*60*1000;
const failed=code=>({status:'failed',code});

function parsed(raw){
 if(typeof raw!=='string'||raw.length>16384||/[\s\\\u0000-\u001f\u007f]/.test(raw))return null;
 try{const url=new URL(raw);return url.username||url.password||url.hash?null:url;}catch{return null;}
}
function exactQuery(url,keys){
 const actual=[...url.searchParams.keys()];
 return actual.length===keys.length&&keys.every(key=>url.searchParams.getAll(key).length===1);
}
function official(raw,url){return !!url&&raw.startsWith(OFFICIAL+'/')&&url.origin===OFFICIAL;}
function stateValid(state,id){
 if(typeof state!=='string'||!state.startsWith(id+'.'))return false;
 const mac=state.slice(id.length+1);
 return /^[A-Za-z0-9_-]{43}$/.test(mac)&&Buffer.from(mac,'base64url').toString('base64url')===mac;
}
function requestValid(value){
 return value&&Object.getPrototypeOf(value)===Object.prototype&&Object.keys(value).length===4&&
 ['project_id','authorization_id','user_id','tenant_id'].every(key=>typeof value[key]==='string'&&ID.test(value[key]))&&OP_ID.test(value.authorization_id);
}
async function readJSON(response,expectedURL){
 if(!response||!response.ok||response.status!==200||response.redirected||response.url&&response.url!==expectedURL||
 !/^application\/json(?:\s*;|$)/i.test(response.headers.get('content-type')||''))throw Error('invalid');
 const reader=response.body?.getReader();if(!reader)throw Error('invalid');
 const chunks=[];let size=0;
 try{for(;;){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>65536)throw Error('limit');chunks.push(value);}}
 catch(error){await reader.cancel().catch(()=>{});throw error;}finally{reader.releaseLock();}
 const bytes=Buffer.concat(chunks.map(chunk=>Buffer.from(chunk)));
 return JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes));
}

// The main-process caller installs its own narrow IPC handler. The renderer
// supplies stable object IDs only; it never supplies an OAuth URL or options.
function createWorkBuddyOAuthController({BrowserWindow,parent,origin,clientID,allowedScopes,now=Date.now}){
 const platform=parsed(origin),scopeList=allowedScopes===undefined?[...KNOWN_SCOPES]:allowedScopes,scopes=new Set(Array.isArray(scopeList)?scopeList:[]);
 const configured=!!platform&&platform.origin===origin&&(platform.protocol==='https:'||platform.protocol==='http:'&&['127.0.0.1','[::1]'].includes(platform.hostname))&&
 (clientID===undefined||typeof clientID==='string'&&clientID.length>0&&clientID.length<=512)&&Array.isArray(scopeList)&&scopes.size===scopeList.length&&scopes.size>0&&[...scopes].every(scope=>KNOWN_SCOPES.has(scope));
 let disposed=false,active=null;
 const live=()=>!disposed&&!parent.isDestroyed()&&!parent.webContents.isDestroyed();
 const caller=(event)=>{
  try{return live()&&event?.sender===parent.webContents&&event.senderFrame===parent.webContents.mainFrame&&
   new URL(event.senderFrame.url).origin===origin&&new URL(parent.webContents.getURL()).origin===origin;}catch{return false;}
 };
 function clear(record,focus=false){
  if(active!==record)return;
  active=null;clearTimeout(record.timer);record.abort.abort();
  for(const [emitter,name,fn] of record.listeners)emitter.removeListener(name,fn);
  record.listeners=[];
  if(record.window&&!record.window.isDestroyed())record.window.destroy();
  if(focus&&live())parent.focus();
 }
 function dispose(){if(disposed)return;disposed=true;if(active)clear(active);parent.removeListener('closed',dispose);parent.webContents.removeListener('destroyed',dispose);}
 parent.on('closed',dispose);parent.webContents.on('destroyed',dispose);
 function listen(record,emitter,name,fn){emitter.on(name,fn);record.listeners.push([emitter,name,fn]);}
 function validAuthorization(data,request){
  const c=data?.connection,expiry=Date.parse(data?.expires_at),url=parsed(data?.authorization_url);
  if(data?.id!==request.authorization_id||!Number.isInteger(data.version)||data.version<1||data.state!=='pending'||!Number.isFinite(expiry)||expiry<=now()||
   !c||typeof c.id!=='string'||!ID.test(c.id)||c.project_id!==request.project_id||c.owner_id!==request.user_id||typeof c.client_id!=='string'||!c.client_id||c.client_id.length>512||clientID!==undefined&&c.client_id!==clientID||c.state!=='authorizing'||
   !official(data.authorization_url,url)||data.authorization_url.split('?')[0]!==OFFICIAL+'/openapi/v2/authorize'||url.pathname!=='/openapi/v2/authorize'||
   !exactQuery(url,['response_type','client_id','redirect_uri','state','scope'])||url.searchParams.get('response_type')!=='code'||
   url.searchParams.get('client_id')!==c.client_id||url.searchParams.get('redirect_uri')!==origin+CALLBACK||
   !stateValid(url.searchParams.get('state'),request.authorization_id))return null;
  const requested=url.searchParams.get('scope').split(' ');
  if(requested.length===0||new Set(requested).size!==requested.length||requested.some(scope=>!scopes.has(scope)))return null;
  return {url:data.authorization_url,state:url.searchParams.get('state'),connectionID:c.id,expiry};
 }
 function installWindow(record,authorization){
  const window=new BrowserWindow({parent,width:860,height:760,webPreferences:{session:parent.webContents.session,contextIsolation:true,sandbox:true,nodeIntegration:false,nodeIntegrationInWorker:false,nodeIntegrationInSubFrames:false,webSecurity:true,allowRunningInsecureContent:false,webviewTag:false}});
  record.window=window;const contents=window.webContents;let callbackSeen=false;
  contents.setWindowOpenHandler(()=>({action:'deny'}));
  function kind(raw,main=true,inPage=false){
   const url=parsed(raw);if(!url)return 'deny';
   if(official(raw,url))return callbackSeen?'deny':'official';
   if(!main||url.origin!==origin)return 'deny';
   if(url.pathname===CALLBACK){
    if(inPage&&callbackSeen&&!url.search)return 'clean';
    const query=url.searchParams;
    if(query.get('state')!==authorization.state)return 'deny';
    if(exactQuery(url,['state','code'])&&query.get('code')&&Buffer.byteLength(query.get('code'))<=8192&&!/[\u0000-\u001f\u007f]/.test(query.get('code')))return 'callback';
    if(query.get('error')==='access_denied'){
     if(exactQuery(url,['state','error']))return 'callback';
     // Provider explanation is only bounded here; never forward it to IPC,
     // diagnostics or the domain command. The bridge submits state/error only.
     const description=query.get('error_description');
     if(exactQuery(url,['state','error','error_description'])&&Buffer.byteLength(description)<=2048&&!/[\u0000-\u001f\u007f-\u009f]/.test(description))return 'callback';
    }
    return 'deny';
   }
   if(callbackSeen&&url.pathname==='/'&&exactQuery(url,['project','view','sub','panel'])&&
    url.searchParams.get('project')===record.request.project_id&&url.searchParams.get('view')==='settings'&&url.searchParams.get('sub')==='tools'&&url.searchParams.get('panel')==='workbuddy:'+authorization.connectionID)return 'return';
   return 'deny';
  }
  function guard(event,legacyURL,_inPlace,legacyMain){
   if(active!==record){event.preventDefault();return;}
   const main=event.isMainFrame??legacyMain??true;
   const result=caller(record.event)?kind(event.url??legacyURL,main):'deny';
   if(result==='deny'||result==='return'){event.preventDefault();clear(record,result==='return');}
  }
  listen(record,contents,'will-navigate',guard);listen(record,contents,'will-redirect',guard);listen(record,contents,'will-frame-navigate',guard);
  listen(record,contents,'did-navigate',(_event,url,status)=>{
   if(active!==record)return;
   const result=kind(url);
   if(result==='callback'&&status>=200&&status<300)callbackSeen=true;
   else if(result!=='official')clear(record);
  });
  listen(record,contents,'did-navigate-in-page',(_event,url,main)=>{
   if(active!==record||!main)return;
   const result=kind(url,true,true);
   if(result==='return')clear(record,true);else if(result!=='clean'&&result!=='official')clear(record);
  });
  listen(record,contents,'will-attach-webview',event=>event.preventDefault());
  listen(record,contents,'did-fail-load',(_event,_code,_description,_url,main)=>{if(main)clear(record);});
  listen(record,contents,'render-process-gone',()=>clear(record));
  listen(record,contents,'destroyed',()=>clear(record));
  listen(record,window,'closed',()=>clear(record));
  listen(record,parent.webContents.session,'will-download',(event,_item,source)=>{if(source===contents)event.preventDefault();});
  return window;
 }
 async function perform(record){
  try{
   const options={method:'GET',credentials:'include',redirect:'error',cache:'no-store',signal:record.abort.signal,headers:{Accept:'application/json','X-Suma-Expected-User':record.request.user_id,'X-Suma-Expected-Organization':record.request.tenant_id}};
   const sessionURL=origin+'/api/v1/auth/session';
   const sessionResponse=await parent.webContents.session.fetch(sessionURL,options);
   const identity=await readJSON(sessionResponse,sessionURL);
   if(active!==record||!caller(record.event)||now()>=record.deadline||identity?.user?.id!==record.request.user_id||identity?.organization?.id!==record.request.tenant_id)throw Error('identity');
   const endpoint=origin+'/api/v1/workbuddy-authorizations/'+record.request.authorization_id+'?'+new URLSearchParams({project_id:record.request.project_id});
   const response=await parent.webContents.session.fetch(endpoint,options);
   const data=await readJSON(response,endpoint);
   if(active!==record||!caller(record.event)||now()>=record.deadline)throw Error('caller');
   const authorization=validAuthorization(data,record.request);if(!authorization)throw Error('authority');
   clearTimeout(record.timer);record.timer=setTimeout(()=>clear(record),Math.min(record.deadline,authorization.expiry)-now());record.timer.unref?.();
   const window=installWindow(record,authorization);
   await window.loadURL(authorization.url);
   if(active!==record||!caller(record.event)){clear(record);return failed('oauth_window_closed');}
   return {status:'opened'};
  }catch{clear(record);return failed('oauth_open_failed');}
 }
 function open(event,request){
  if(!configured||!caller(event)||!requestValid(request))return Promise.resolve(failed('oauth_request_rejected'));
  const identity=JSON.stringify([request.project_id,request.authorization_id,request.user_id,request.tenant_id]);
  if(active){
   if(active.identity!==identity)return Promise.resolve(failed('oauth_window_busy'));
   if(active.window&&!active.window.isDestroyed()){active.window.focus();return Promise.resolve({status:'opened'});}
   return active.promise;
  }
  const record={identity,event,request:{...request},deadline:now()+MAX_AGE,abort:new AbortController(),listeners:[],window:null,timer:null,promise:null};active=record;
  record.timer=setTimeout(()=>clear(record),MAX_AGE);record.timer.unref?.();record.promise=perform(record);return record.promise;
 }
 return {open,dispose};
}
module.exports={createWorkBuddyOAuthController};
