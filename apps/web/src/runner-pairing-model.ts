import type {RunnerPairing} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{32}$/.test(value);
const projectID=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&wellFormed(value)&&new TextEncoder().encode(value).length<=64&&!/[\u0000-\u0020/\\\u007f]/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const operation=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_.:-]{8,128}$/.test(value);
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
function instant(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value))return false;
 const whole=Date.parse(value.slice(0,19)+'Z');
 return Number.isFinite(whole)&&new Date(whole).toISOString().slice(0,19)===value.slice(0,19);
}
function controlURL(value:unknown):value is string{
 if(!text(value,2048)||!/^https?:\/\//.test(value)||/[\\?#]/.test(value))return false;
 try{
  const parsed=new URL(value),authority=value.slice(value.indexOf('://')+3).split('/')[0];
  const host=/^(\[[^\]]+\]|[^:]+)(?::[0-9]+)?$/.exec(authority)?.[1];
  return !!parsed.hostname&&!parsed.username&&!parsed.password&&!authority.includes('@')&&!!host&&
   (parsed.protocol==='https:'||parsed.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(host));
 }catch{return false;}
}
function publicPairing(value:unknown,project:string):value is RunnerPairing{
 if(!projectID(project)||!object(value)||!closed(value,['id','project_id','version','state','control_url','created_at','expires_at','can_approve'],['device_name','credential_sha256','worker_id','credential_expires_at'])||
  !id(value.id)||value.project_id!==project||!positive(value.version)||!['waiting','proposed','approved','rejected','expired'].includes(String(value.state))||!controlURL(value.control_url)||
  !instant(value.created_at)||!instant(value.expires_at)||typeof value.can_approve!=='boolean'||value.can_approve&&value.state!=='proposed')return false;
 const proposal=Object.hasOwn(value,'device_name')||Object.hasOwn(value,'credential_sha256');
 if(proposal&&(!text(value.device_name,100)||!digest(value.credential_sha256)))return false;
 if(value.state==='waiting'&&proposal||(value.state==='proposed'||value.state==='approved')&&!proposal)return false;
 if(value.state==='approved')return !value.can_approve&&typeof value.worker_id==='string'&&/^remote-[a-f0-9]{32}$/.test(value.worker_id)&&instant(value.credential_expires_at);
 return !Object.hasOwn(value,'worker_id')&&!Object.hasOwn(value,'credential_expires_at');
}

/** Qualification is a server fact; neither the local clock nor a self-reported
 * name can turn an observed machine into an authorized execution node. */
export function readRunnerPairing(value:unknown,project:string):RunnerPairing{
 if(publicPairing(value,project))return value;
 throw new UiError('runner-pairing.invalid_read');
}
export function validRunnerPairingRequest(request:WriteRecoveryRequest,project:string):boolean{
 if(!projectID(project)||!object(request)||!closed(request,['path','method','idempotency_key','body'])||request.method!=='POST'||!operation(request.idempotency_key)||!object(request.body)||request.body.project_id!==project||request.body.idempotency_key!==request.idempotency_key)return false;
 if(request.path==='/runner-pairings')return closed(request.body,['project_id','idempotency_key']);
 return typeof request.path==='string'&&/^\/runner-pairings\/[a-f0-9]{32}\/commands$/.test(request.path)&&closed(request.body,['project_id','expected_version','idempotency_key','action'])&&positive(request.body.expected_version)&&['approve','reject'].includes(String(request.body.action));
}
export function validRunnerPairingReceipt(value:unknown,request:WriteRecoveryRequest,project:string):boolean{
 if(!validRunnerPairingRequest(request,project)||!publicPairing(value,project))return false;
 if(request.path==='/runner-pairings')return value.version===1&&value.state==='waiting';
 const resource=/^\/runner-pairings\/([a-f0-9]{32})\/commands$/.exec(request.path)![1];
 return value.id===resource&&value.version===(request.body.expected_version as number)+1&&value.state===(request.body.action==='approve'?'approved':'rejected')&&!value.can_approve;
}
function identityPath(value:string):boolean{
 if(!text(value,4096)||/^\\\\[.?]\\/.test(value))return false;
 return /^[A-Za-z]:[\\/].+/.test(value)||/^\/(?!\/).+/.test(value)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/].+/.test(value);
}
/** A copyable public locator only. The Runner generates and keeps its own
 * secret; this command neither passes a token nor authorizes execution. */
export function runnerPairingCommand(pairing:RunnerPairing,shell:''|'powershell'|'bash',path:string):string|undefined{
 if(!['powershell','bash'].includes(shell)||typeof path!=='string'||path!==''&&!identityPath(path))return;
 try{readRunnerPairing(pairing,pairing.project_id);}catch{return;}
 const quote=(value:string)=>"'"+(shell==='powershell'?value.replaceAll("'","''"):value.replaceAll("'","'\"'\"'"))+"'";
 return 'suma-runner --pair-runner '+quote(pairing.id)+' --control '+quote(pairing.control_url)+' --project '+quote(pairing.project_id)+(path?' --identity '+quote(path):'');
}
