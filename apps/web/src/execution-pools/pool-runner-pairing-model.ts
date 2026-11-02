import type {PoolRunnerPairing,PoolRunnerPairingPage} from '../../../../packages/contracts/src';
import type {WriteRecoveryRequest} from '../write-action';
import {wellFormed} from '../i18n/catalog';
import {UiError} from '../i18n';
import {runnerSetupLiteral} from '../cursor-setup-guide-model';

export type PoolPairingScope={poolID:string;organizationID:string;userID:string};
export type PoolPairingQuery={status:string;q:string;limit:number;cursor:string};
export const poolPairingStates=['waiting','proposed','approved','rejected','expired'] as const;
const statuses=['active','all',...poolPairingStates];
const reasons=['pool_changed','pool_inactive','credential_unavailable','control_changed','expired','rejected'];
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{32}$/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const integer=(value:unknown,min:number,max=2147483647):value is number=>Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
const text=(value:unknown,max:number,empty=false):value is string=>typeof value==='string'&&(empty||value.length>0)&&value.trim()===value&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const operation=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_.:-]{8,128}$/.test(value);
function instant(value:unknown):value is string{if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value))return false;const parsed=Date.parse(value.slice(0,19)+'Z');return Number.isFinite(parsed)&&new Date(parsed).toISOString().slice(0,19)===value.slice(0,19);}
function controlURL(value:unknown):value is string{if(!text(value,2048)||!/^https?:\/\//.test(value)||/[\\?#]/.test(value))return false;try{const url=new URL(value),authority=value.slice(value.indexOf('://')+3).split('/')[0],host=/^(\[[^\]]+\]|[^:]+)(?::[0-9]+)?$/.exec(authority)?.[1];return !!host&&!!url.hostname&&!url.username&&!url.password&&!authority.includes('@')&&(url.protocol==='https:'||url.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(host));}catch{return false;}}
export const validPoolPairingScope=(value:unknown):value is PoolPairingScope=>object(value)&&closed(value,['poolID','organizationID','userID'])&&Object.values(value).every(id);
export const poolPairingPath=(poolID:string)=>'/execution-pools/'+encodeURIComponent(poolID)+'/runner-pairings';
const invalid=():never=>{throw new UiError('pool-runner-pairing.invalid_read');};

/** Pool-purpose metadata is never parsed through the project Runner contract.
 * Required false and every approved binding remain explicit public facts. */
export function readPoolRunnerPairing(value:unknown,scope:PoolPairingScope):PoolRunnerPairing{
 if(!validPoolPairingScope(scope)||!object(value)||!closed(value,['id','purpose','owner_tenant_id','pool_id','pool_version','version','state','control_url','name','max_slots','ttl_seconds','created_at','expires_at','can_approve'],['device_name','credential_sha256','runner_id','worker_id','credential_version','credential_expires_at','reason_code'])||!id(value.id)||value.purpose!=='pool_runner'||value.owner_tenant_id!==scope.organizationID||value.pool_id!==scope.poolID||!integer(value.pool_version,1)||!integer(value.version,1)||!poolPairingStates.includes(value.state as typeof poolPairingStates[number])||!controlURL(value.control_url)||!text(value.name,200)||!integer(value.max_slots,1,128)||!integer(value.ttl_seconds,3600,2592000)||!instant(value.created_at)||!instant(value.expires_at)||Date.parse(value.expires_at)<=Date.parse(value.created_at)||typeof value.can_approve!=='boolean'||value.can_approve&&value.state!=='proposed'||Object.hasOwn(value,'reason_code')&&!reasons.includes(String(value.reason_code)))return invalid();
 const machine=Object.hasOwn(value,'device_name')||Object.hasOwn(value,'credential_sha256');
 if(machine&&(!text(value.device_name,100)||!digest(value.credential_sha256))||value.state==='waiting'&&machine||['proposed','approved'].includes(String(value.state))&&!machine)return invalid();
 const approved=['runner_id','worker_id','credential_version','credential_expires_at'];
 if(value.state==='approved'){if(!id(value.runner_id)||value.worker_id!=='pool-'+value.runner_id||!integer(value.credential_version,1)||!instant(value.credential_expires_at)||Date.parse(value.credential_expires_at)<=Date.parse(value.created_at))return invalid();}
 else if(approved.some(key=>Object.hasOwn(value,key)))return invalid();
 return structuredClone(value) as unknown as PoolRunnerPairing;
}
export function poolPairingListPath(scope:PoolPairingScope,query:PoolPairingQuery):string{
 if(!validPoolPairingScope(scope)||!statuses.includes(query.status)||!text(query.q,120,true)||!integer(query.limit,1,100)||!text(query.cursor,4096,true))return invalid();
 const params=new URLSearchParams({status:query.status,q:query.q,limit:String(query.limit)});if(query.cursor)params.set('cursor',query.cursor);return poolPairingPath(scope.poolID)+'?'+params;
}
export function readPoolRunnerPairingPage(value:unknown,scope:PoolPairingScope,query:PoolPairingQuery):PoolRunnerPairingPage{
 poolPairingListPath(scope,query);
 if(!object(value)||!closed(value,['owner_tenant_id','pool_id','as_of','items','total'],['next_cursor'])||value.owner_tenant_id!==scope.organizationID||value.pool_id!==scope.poolID||!instant(value.as_of)||!Array.isArray(value.items)||value.items.length>query.limit||Reflect.ownKeys(value.items).length!==value.items.length+1||!integer(value.total,0,Number.MAX_SAFE_INTEGER)||value.total<value.items.length||Object.hasOwn(value,'next_cursor')&&(!text(value.next_cursor,4096)||value.next_cursor===query.cursor||value.items.length===0))return invalid();
 const items=value.items.map(item=>readPoolRunnerPairing(item,scope));if(new Set(items.map(item=>item.id)).size!==items.length||items.some(item=>query.status==='active'?!['waiting','proposed'].includes(item.state):query.status!=='all'&&item.state!==query.status))return invalid();
 return {...value,items} as unknown as PoolRunnerPairingPage;
}
export function validPoolRunnerPairingRequest(request:WriteRecoveryRequest,scope:PoolPairingScope){
 if(!validPoolPairingScope(scope)||!object(request)||!closed(request,['path','method','idempotency_key','body'])||request.method!=='POST'||!operation(request.idempotency_key)||!object(request.body)||request.body.pool_id!==scope.poolID||request.body.idempotency_key!==request.idempotency_key)return false;
 if(request.path===poolPairingPath(scope.poolID))return closed(request.body,['pool_id','name','max_slots','ttl_seconds','idempotency_key'])&&text(request.body.name,200)&&integer(request.body.max_slots,1,128)&&integer(request.body.ttl_seconds,3600,2592000);
 const prefix=poolPairingPath(scope.poolID)+'/',path=request.path;
 return typeof path==='string'&&path.startsWith(prefix)&&/^[a-f0-9]{32}\/commands$/.test(path.slice(prefix.length))&&closed(request.body,['pool_id','action','expected_version','idempotency_key'])&&['approve','reject'].includes(String(request.body.action))&&integer(request.body.expected_version,1);
}
export function validPoolRunnerPairingReceipt(value:unknown,request:WriteRecoveryRequest,scope:PoolPairingScope){
 if(!validPoolRunnerPairingRequest(request,scope))return false;try{const pairing=readPoolRunnerPairing(value,scope);if(request.path===poolPairingPath(scope.poolID))return pairing.state==='waiting'&&pairing.version===1&&!pairing.can_approve&&pairing.name===request.body.name&&pairing.max_slots===request.body.max_slots&&pairing.ttl_seconds===request.body.ttl_seconds;return request.path===poolPairingPath(scope.poolID)+'/'+pairing.id+'/commands'&&pairing.version===(request.body.expected_version as number)+1&&pairing.state===(request.body.action==='approve'?'approved':'rejected')&&!pairing.can_approve;}catch{return false;}
}
/** A safe public machine locator only. No token is copied into the browser. */
export function poolRunnerPairingCommand(pairing:PoolRunnerPairing,scope:PoolPairingScope,shell:''|'powershell'|'bash',identityPath:string):string|undefined{
 if(shell!=='powershell'&&shell!=='bash'||typeof identityPath!=='string'||identityPath!==''&&(!text(identityPath,4096)||/^[\\/]{2}[?.][\\/]/.test(identityPath)||identityPath.split(/[\\/]/).some(part=>part==='.'||part==='..')||!(/^[A-Za-z]:[\\/].+/.test(identityPath)||/^\/(?!\/).+/.test(identityPath)||/^\\\\[^\\/]+[\\/][^\\/]+[\\/].+/.test(identityPath))))return;
 try{readPoolRunnerPairing(pairing,scope);}catch{return;}
 if(!['waiting','proposed'].includes(pairing.state))return;
 return 'suma-runner --pair-pool-runner '+runnerSetupLiteral(pairing.id,shell)+' --control '+runnerSetupLiteral(pairing.control_url,shell)+' --pool '+runnerSetupLiteral(pairing.pool_id,shell)+(identityPath?' --pool-identity '+runnerSetupLiteral(identityPath,shell):'');
}
