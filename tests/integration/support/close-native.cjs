'use strict';
const cp=require('node:child_process'),path=require('node:path');
async function bounded(promise,milliseconds){let timer;try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Owned native close deadline')),milliseconds);})]);}finally{clearTimeout(timer);}}

/** Only a child process handle returned by this fixture's launch is accepted.
 * A teardown fallback is never a product cancellation or stop acknowledgement. */
async function closeOwnedNative(app,exit,timeout=15000){
 const child=app.process();
 if(!Number.isInteger(child?.pid)||child.pid<=0)throw Error('Expected owned native child handle');
 try{return{forced:false,native_exit:await bounded((async()=>{await app.close();return await exit;})(),timeout)};}
 catch(error){
  const result={forced:false,close_error:String(error)};
  // The live original handle excludes PID reuse. Do not kill a process merely
  // because a saved PID/lock exists, or after this child is already terminal.
  if(child.exitCode===null&&child.signalCode===null){
   result.forced=true;
   if(process.platform==='win32'){
    const executable=path.join(process.env.SystemRoot||'C:\\Windows','System32','taskkill.exe');
    result.termination=await new Promise(resolve=>cp.execFile(executable,['/PID',String(child.pid),'/T','/F'],{windowsHide:true,timeout:8000},failure=>resolve({exit_code:failure?.code??0})));
   }else child.kill('SIGKILL');
  }
  result.native_exit=await bounded(exit,8000);
  return result;
 }
}
module.exports={closeOwnedNative};
