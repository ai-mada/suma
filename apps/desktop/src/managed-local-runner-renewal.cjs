'use strict';
const crypto=require('node:crypto'),{spawn:spawnChild}=require('node:child_process');
const {HASH,ID,fields,strictJSON}=require('./local-acp-validation.cjs');
const RENEW_FIELDS=['project_id','user_id','tenant_id','worker_id','adapter_id','instance_id','expected_binding_sha256','base_commit','registration_operation_id','expected_worker_version','previous_credential_sha256','credential_sha256','idempotency_key'];
const digest=value=>crypto.createHash('sha256').update(value).digest('hex');
function validRenewRequest(request,value){
 return fields(request,RENEW_FIELDS)&&['project_id','user_id','tenant_id','worker_id','adapter_id','instance_id','expected_binding_sha256','base_commit'].every(k=>request[k]===value.request[k])&&request.registration_operation_id===value.request.idempotency_key&&Number.isSafeInteger(request.expected_worker_version)&&request.expected_worker_version>0&&request.expected_worker_version<2147483647&&HASH.test(request.previous_credential_sha256)&&HASH.test(request.credential_sha256)&&request.previous_credential_sha256!==request.credential_sha256&&typeof request.idempotency_key==='string'&&/^desktop-runner-renew-[a-f0-9]{32}$/.test(request.idempotency_key);
}
function validRenewReceipt(receipt,value,request){
 if(!fields(receipt,['operation_id','project_id','user_id','tenant_id','instance_id','adapter_id','binding_sha256','base_commit','worker'])||receipt.operation_id!==request.idempotency_key||['project_id','user_id','tenant_id','instance_id','adapter_id','base_commit'].some(k=>receipt[k]!==request[k])||receipt.binding_sha256!==request.expected_binding_sha256)return false;
 const w=receipt.worker,old=value.worker;return fields(w,['id','project_id','worker_id','version','status','profiles','deployment','created_at','expires_at'])&&ID.test(w.id)&&w.id===old?.id&&w.project_id===request.project_id&&w.worker_id===request.worker_id&&w.version===request.expected_worker_version+1&&w.status==='active'&&Array.isArray(w.profiles)&&w.profiles.length===0&&w.deployment===false&&w.created_at===old.created_at&&typeof w.expires_at==='string'&&Number.isFinite(Date.parse(w.expires_at))&&Date.parse(w.expires_at)>Date.parse(w.created_at);
}
function validateRenewalState(value){
 const current=digest(value.token),last=value.last_renewal,pending=value.pending_renew;
 if(last!==undefined){if(!fields(last,['request','receipt','registration_sha256'])||last.registration_sha256!==digest(JSON.stringify(value.request))||!validRenewRequest(last.request,value)||!validRenewReceipt(last.receipt,value,last.request)||last.request.credential_sha256!==current||JSON.stringify(last.receipt.worker)!==JSON.stringify(value.worker))throw Error('Invalid last managed credential renewal');}
 else if(value.request.credential_sha256!==current)throw Error('Managed credential differs from its original registration');
 if(pending!==undefined&&(!fields(pending,['token','request'])||!HASH.test(pending.token)||!validRenewRequest(pending.request,value)||pending.request.previous_credential_sha256!==current||pending.request.credential_sha256!==digest(pending.token)||pending.request.expected_worker_version!==value.worker?.version||pending.request.idempotency_key===last?.request.idempotency_key))throw Error('Invalid pending managed credential renewal');
}
const reasons=new Set(['active_worker','pending_execution','pending_receipt','unknown_outcome','invalid_state','unreadable_state']);
function createRenewalReadinessChecker({binary,spawn=spawnChild,environment=process.env,deadlineMS=10000}){
 let active,disposed=false;
 function check(directory,locale){
  if(disposed||active)return Promise.resolve(false);
  return new Promise(resolve=>{
   let child,closed=false,settled=false,failed=false,size=0,chunks=[],killTimer,stopTimer;const record={child:null,stop:null,done:null};active=record;
   const finish=safe=>{if(settled)return;settled=true;clearTimeout(timer);clearTimeout(killTimer);clearTimeout(stopTimer);chunks=[];if(closed||!child)active=undefined;resolve(safe);};
   const stop=()=>{if(closed||failed)return;failed=true;killTimer=setTimeout(()=>{if(!closed)try{child?.kill('SIGKILL');}catch{}},1500);stopTimer=setTimeout(()=>finish(false),3500);try{child?.kill('SIGTERM');}catch{}};record.stop=stop;const timer=setTimeout(stop,deadlineMS);
   try{const env={};for(const [k,v]of Object.entries(environment))if(['path','systemroot','windir','comspec','pathext','lang','lc_all'].includes(k.toLowerCase()))env[k]=v;Object.assign(env,{TEMP:directory,TMP:directory,TMPDIR:directory});child=spawn(binary,['--language='+locale,'--desktop-runner-renewal-check-stdin'],{cwd:directory,env,stdio:['pipe','pipe','ignore'],windowsHide:true,shell:false});record.child=child;record.done=new Promise(done=>child.once('close',done));child.on('error',stop);child.stdin.on('error',stop);child.stdout.on('data',b=>{if(failed||settled)return;size+=b.length;if(size>4096){stop();return;}chunks.push(Buffer.from(b));});child.once('close',code=>{closed=true;if(active===record)active=undefined;if(failed||disposed||code!==0){finish(false);return;}try{const out=strictJSON(Buffer.concat(chunks));if(!fields(out,['protocol','safe','reasons'])||out.protocol!=='suma-desktop-runner-renewal-check-v1'||typeof out.safe!=='boolean'||!Array.isArray(out.reasons)||out.reasons.length>6||new Set(out.reasons).size!==out.reasons.length||out.reasons.some(r=>!reasons.has(r))||out.safe!==!out.reasons.length)throw Error('Invalid readiness');finish(out.safe);}catch{finish(false);}});child.stdin.end(JSON.stringify({state_directory:directory})+'\n');}catch{finish(false);}
  });
 }
 async function dispose(){disposed=true;if(active){const record=active;record.stop();let timer;try{await Promise.race([record.done||Promise.resolve(),new Promise(resolve=>timer=setTimeout(resolve,4000))]);}finally{clearTimeout(timer);}}}
 return {check,dispose};
}
module.exports={RENEW_FIELDS,digest,validRenewRequest,validRenewReceipt,validateRenewalState,createRenewalReadinessChecker};
