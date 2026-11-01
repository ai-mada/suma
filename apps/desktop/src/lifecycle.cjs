'use strict';
const {setTimeout:delay}=require('node:timers/promises');
const {localError}=require('./localization.cjs');
function startupExitError(exitCode,locale){return localError(locale,exitCode===78?'startup_database_incompatible':'startup_exited')}
async function waitForHealth(origin,hasExited,timeout=15000,locale='zh-CN'){
 const deadline=Date.now()+timeout;
 while(Date.now()<deadline){if(hasExited())throw localError(locale,'startup_exited');try{const response=await fetch(`${origin}/healthz`,{signal:AbortSignal.timeout(750),redirect:'error'});if(response.ok && !hasExited())return;}catch{}await delay(150);}
 throw localError(locale,'startup_unhealthy');
}
function allowedNavigation(candidate,origin){try{return new URL(candidate).origin===new URL(origin).origin}catch{return false}}
async function terminateService(child,grace=5000,locale='zh-CN'){if(!child?.pid || child.exitCode!==null || child.signalCode!==null)return;await new Promise((resolve,reject)=>{let timer;const cleanup=()=>{clearTimeout(timer);child.removeListener('exit',done);child.removeListener('error',failed)};const done=()=>{cleanup();resolve()};const failed=error=>{cleanup();reject(localError(locale,'termination_failed',error))};child.once('exit',done);child.once('error',failed);timer=setTimeout(()=>{try{child.kill('SIGKILL')}catch(error){failed(error);return;}timer=setTimeout(()=>{cleanup();reject(localError(locale,'termination_timeout'))},2000)},grace);try{child.kill('SIGTERM')}catch(error){failed(error)}})}
module.exports={waitForHealth,allowedNavigation,terminateService,startupExitError};
