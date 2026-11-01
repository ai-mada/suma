import type {ToolMaintenanceStatus} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,keys:readonly string[])=>keys.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&keys.includes(key));
function instant(value:unknown):bigint|undefined{
 if(typeof value!=='string')return;
 const match=/^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d{1,9}))?Z$/.exec(value);if(!match)return;
 const milliseconds=Date.parse(match[1]+'Z');if(!Number.isFinite(milliseconds)||new Date(milliseconds).toISOString().slice(0,19)!==match[1])return;
 return BigInt(milliseconds)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
}
function instance(value:unknown):value is string{
 if(typeof value!=='string'||!wellFormed(value)||value.trim()!==value||/\p{Cc}/u.test(value))return false;
 const size=new TextEncoder().encode(value).length;return size>=8&&size<=128;
}

/** Server state and observation lifetime only. Capability is neither current
 * authorization, native-stop evidence nor tool/model availability. The caller
 * deducts request latency and uses a monotonic deadline after this read. */
export function readToolMaintenanceStatus(value:unknown):{status:ToolMaintenanceStatus;remainingMs:number}|undefined{
 if(!object(value))return;const checked=instant(value.checked_at);if(checked===undefined)return;
 if(value.state==='unknown')return closed(value,['state','checked_at'])?{status:value as unknown as ToolMaintenanceStatus,remainingMs:0}:undefined;
 if(!closed(value,['state','checked_at','protocol','instance_id','observed_at','expires_at'])||typeof value.protocol!=='string'||!/^suma-tool-maintenance-v[1-9][0-9]{0,3}$/.test(value.protocol)||!instance(value.instance_id))return;
 const observed=instant(value.observed_at),expires=instant(value.expires_at);
 if(observed===undefined||expires===undefined||observed>checked||expires-observed!==60000000000n)return;
 const currentProtocol=value.protocol==='suma-tool-maintenance-v1';
 if(value.state==='supported'&&currentProtocol&&expires>checked)return {status:value as unknown as ToolMaintenanceStatus,remainingMs:Number(expires-checked)/1000000};
 if(value.state==='expired'&&currentProtocol&&expires<=checked||value.state==='unsupported'&&!currentProtocol)return {status:value as unknown as ToolMaintenanceStatus,remainingMs:0};
 return;
}
export function validToolMaintenanceStatus(value:unknown):value is ToolMaintenanceStatus{return readToolMaintenanceStatus(value)!==undefined;}
