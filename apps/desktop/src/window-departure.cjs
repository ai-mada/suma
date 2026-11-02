'use strict';
const {message}=require('./localization.cjs');

/** Native departure belongs to this exact window, not to an IPC request or a
 * business operation. A Leave response never means a task or worker stopped. */
function createWindowDepartureController({parent,dialog,origin,locale='zh-CN',getWindow=()=>parent}){
 const contents=parent.webContents;
 let disposed=false,epoch=0,sequence=0,intent,pending,replay;
 const listeners=[];
 function listen(emitter,name,handler){emitter.on(name,handler);listeners.push([emitter,name,handler]);}
 function live(){try{return !disposed&&getWindow()===parent&&!parent.isDestroyed()&&!contents.isDestroyed();}catch{return false;}}
 function source(){
  if(!live())return;
  try{const url=contents.getURL();if(new URL(url).origin!==origin)return;return {url,frame:contents.mainFrame,epoch};}catch{return;}
 }
 function sameSource(value){const now=source();return !!now&&now.url===value.url&&now.frame===value.frame&&now.epoch===value.epoch;}
 function allowed(url){try{const target=new URL(url);return target.origin===origin&&['http:','https:'].includes(target.protocol)&&!target.username&&!target.password;}catch{return false;}}
 function options(signal){return {type:'warning',title:message(locale,'departure_title'),message:message(locale,'departure_message'),detail:message(locale,'departure_detail'),buttons:[message(locale,'departure_stay'),message(locale,'departure_leave')],defaultId:0,cancelId:0,noLink:true,...(signal?{signal}:{})};}
 function capture(kind,target){
  const basis=source();
  if(!basis){intent=undefined;replay=undefined;return;}
  // JavaScript may request a second departure while the native sheet is open.
  // It invalidates the old answer; it does not borrow that answer or queue a
  // second dialog. The human can retry the new action after the sheet closes.
  if(pending)pending.stale=true;
  replay=undefined;
  intent={id:++sequence,kind,source:basis,...(target?{target}:{})};
 }
 function close(event){
  if(pending){event.preventDefault();return;}
  if(replay&&['close','quit'].includes(replay.intent.kind)&&!replay.entered&&sameSource(replay.intent.source)){
   replay.entered=true;return;
  }
  capture(intent?.kind==='quit'?'quit':'close');
 }
 function navigation(event,url,isSameDocument,isMainFrame,eventName){
  const main=event?.isMainFrame??isMainFrame??(eventName==='will-navigate');
  const same=event?.isSameDocument??isSameDocument??false,target=event?.url??url;
  if(!main||same)return;
  if(!allowed(target)){if(pending)pending.stale=true;intent=undefined;replay=undefined;return;}
  if(replay&&replay.intent.kind==='navigate'&&target===replay.intent.target&&sameSource(replay.intent.source)&&!replay.events.has(eventName)){
   replay.events.add(eventName);replay.entered=true;return;
  }
  // Both documented events can describe the same original navigation. Keep
  // one identity, but do not coalesce a later repeated navigation/ABA.
  if(intent?.kind==='navigate'&&intent.target===target&&sameSource(intent.source)&&!intent.events.has(eventName)){
   intent.events.add(eventName);return;
  }
  capture('navigate',target);if(intent)intent.events=new Set([eventName]);
 }
 function invalidate(){epoch++;intent=undefined;replay=undefined;if(pending){pending.stale=true;pending.abort.abort();}}
 function retry(value){
  if(!sameSource(value.source)||intent!==value)return;
  const permit={intent:value,entered:false,events:new Set()};replay=permit;
  try{
   if(value.kind==='navigate'){
    // loadURL is a public API and replays only the captured owned-origin URL.
    // Its new beforeunload event, not the already-returned old event, consumes
    // the one-shot permission synchronously.
    Promise.resolve(contents.loadURL(value.target)).catch(()=>{}).finally(()=>{if(replay===permit)replay=undefined;if(intent===value)intent=undefined;});
   }else parent.close();
  }catch{if(replay===permit)replay=undefined;}
 }
 async function decide(value){
  const record={intent:value,abort:new AbortController(),stale:false};pending=record;
  try{
   const answer=await dialog.showMessageBox(parent,options(record.abort.signal));
   if(pending!==record||record.stale||answer?.response!==1||!sameSource(value.source)||intent!==value)return;
   pending=undefined;retry(value);
  }catch{/* Native dialog cancellation/failure is Stay; no shutdown or retry. */}
  finally{
   // Stay (including a superseded action) retires this cancelled attempt. A
   // future unknown unload must not replay the previous close or destination.
   if(pending===record){pending=undefined;intent=undefined;replay=undefined;}
  }
 }
 function preventUnload(event){
  if(!source())return;
  if(replay&&replay.entered&&intent===replay.intent&&sameSource(replay.intent.source)){
   replay=undefined;intent=undefined;event.preventDefault();return;
  }
  if(pending)return;
  if(intent&&sameSource(intent.source)){void decide(intent);return;}
  // Electron 35 completes RunBeforeUnloadDialog synchronously. For reloads or
  // other intents whose destination public events have not exposed, use its
  // official native synchronous API. Never await and then mutate an old Event,
  // guess a reload URL, destroy the window, or install a lasting unload bypass.
  const basis=source();
  try{if(dialog.showMessageBoxSync(parent,options())===1&&basis&&sameSource(basis))event.preventDefault();}catch{}
 }
 function requestQuit(){
  if(!live()||pending)return false;
  capture('quit');parent.close();return true;
 }
 function dispose(){
  if(disposed)return;disposed=true;intent=undefined;replay=undefined;
  if(pending){pending.stale=true;pending.abort.abort();pending=undefined;}
  for(const [emitter,name,handler]of listeners)emitter.removeListener(name,handler);
  listeners.length=0;
 }
 listen(parent,'close',close);
 listen(contents,'will-navigate',(event,url)=>navigation(event,url,false,true,'will-navigate'));
 listen(contents,'did-start-navigation',(event,url,same,main)=>navigation(event,url,same,main,'did-start-navigation'));
 listen(contents,'will-prevent-unload',preventUnload);
 listen(contents,'did-navigate',invalidate);
 listen(contents,'did-navigate-in-page',invalidate);
 listen(contents,'render-process-gone',invalidate);
 listen(parent,'closed',dispose);listen(contents,'destroyed',dispose);
 return {requestQuit,dispose};
}
module.exports={createWindowDepartureController};
