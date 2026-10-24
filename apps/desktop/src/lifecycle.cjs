'use strict';
const {setTimeout:delay}=require('node:timers/promises');
async function waitForHealth(origin,hasExited,timeout=15000){
 const deadline=Date.now()+timeout;
 while(Date.now()<deadline){if(hasExited())throw Error('Bundled control service exited before startup');try{const response=await fetch(`${origin}/healthz`,{signal:AbortSignal.timeout(750),redirect:'error'});if(response.ok && !hasExited())return;}catch{}await delay(150);}
 throw Error('Bundled control service did not become healthy');
}
function allowedNavigation(candidate,origin){try{return new URL(candidate).origin===new URL(origin).origin}catch{return false}}
async function terminateService(child,grace=5000){if(!child?.pid || child.exitCode!==null || child.signalCode!==null)return;await new Promise((resolve,reject)=>{let timer;const cleanup=()=>{clearTimeout(timer);child.removeListener('exit',done);child.removeListener('error',failed)};const done=()=>{cleanup();resolve()};const failed=()=>{cleanup();reject(Error('Control service termination failed'))};child.once('exit',done);child.once('error',failed);timer=setTimeout(()=>{try{child.kill('SIGKILL')}catch{failed();return;}timer=setTimeout(()=>{cleanup();reject(Error('Control service did not exit after forced termination'))},2000)},grace);try{child.kill('SIGTERM')}catch{failed()}})}
module.exports={waitForHealth,allowedNavigation,terminateService};
