import type {ProjectPoolRunnerPage} from '../../../../packages/contracts/src';
import type {ToolProfile} from '../tool-model';
import {sameToolProfile} from '../tool-profile-reuse-model';
import {toolDiscoveryTimestampNs} from '../tool-discovery-model';
import {validPoolConfigID} from './pool-config-preparation-model';

/** Public navigation intent only. A URL never grants Pool ownership, project
 * access, credential rotation or execution; both destinations re-read authority. */
export type PoolToolRecoveryPin={
  pool_id:string; runner_id:string; worker_id:string; tenant_id:string;
  project_id:string; tool_profile_id:string; tool_profile_version:number;
  profile_sha256:string; adapter:string; protocol:string;
};
const fields=['pool_id','runner_id','worker_id','tenant_id','project_id','tool_profile_id','tool_profile_version','profile_sha256','adapter','protocol'] as const;
const prefix='pool-tool-recovery:';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const version=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>0&&Number(value)<=2147483647;
export function validPoolToolRecoveryPin(value:unknown):value is PoolToolRecoveryPin{
  return object(value)&&Reflect.ownKeys(value).length===fields.length&&fields.every(key=>Object.hasOwn(value,key))&&
    ['pool_id','runner_id','tenant_id','project_id','tool_profile_id','adapter','protocol'].every(key=>validPoolConfigID(value[key]))&&
    value.worker_id==='pool-'+value.runner_id&&version(value.tool_profile_version)&&typeof value.profile_sha256==='string'&&/^[a-f0-9]{64}$/.test(value.profile_sha256);
}
/** Fixed ten-value tuple avoids free-form JSON/commands/private paths in URLs. */
export function serializePoolToolRecoveryPin(pin:PoolToolRecoveryPin):string{
  if(!validPoolToolRecoveryPin(pin))throw Error('Invalid Pool recovery navigation');
  return prefix+fields.map(key=>String(pin[key])).join(':');
}
export function parsePoolToolRecoveryPin(panel:string):PoolToolRecoveryPin|undefined{
  if(typeof panel!=='string'||!panel.startsWith(prefix)||panel.length>1200)return;
  const values=panel.slice(prefix.length).split(':');
  if(values.length!==fields.length||!/^([1-9][0-9]{0,9})$/.test(values[6]))return;
  const candidate=Object.fromEntries(fields.map((key,index)=>[key,index===6?Number(values[index]):values[index]]));
  return validPoolToolRecoveryPin(candidate)?candidate:undefined;
}
function recoveryProfile(raw:unknown,project:string):raw is ToolProfile{
  return object(raw)&&raw.project_id===project&&validPoolConfigID(raw.id)&&typeof raw.worker_id==='string'&&version(raw.version)&&
    (raw.state==='enabled'||raw.state==='paused')&&sameToolProfile(raw.profile,raw.profile);
}
/** Inventory availability is deliberately not an admission condition here:
 * this explicit recovery must also locate an offline node or stale inventory. */
export function createPoolToolRecoveryPin(profile:ToolProfile,tenantID:string,page:ProjectPoolRunnerPage|undefined):PoolToolRecoveryPin|undefined{
  if(!object(profile)||!validPoolConfigID(tenantID)||!recoveryProfile(profile,profile.project_id)||!validPoolConfigID(profile.project_id)||!page||!Array.isArray(page.items))return;
  try{toolDiscoveryTimestampNs(page.as_of);}catch{return;}
  const rows=page.items.filter(row=>object(row)&&row.worker_id===profile.worker_id&&row.status==='active');
  if(rows.length!==1)return;
  const row=rows[0],pin={pool_id:row.pool_id,runner_id:row.id,worker_id:row.worker_id,tenant_id:tenantID,project_id:profile.project_id,
    tool_profile_id:profile.id,tool_profile_version:profile.version,profile_sha256:profile.profile.profile_sha256,adapter:profile.profile.adapter,protocol:profile.profile.protocol};
  return validPoolToolRecoveryPin(pin)?pin:undefined;
}
/** Re-read the exact old source, independent of list filters/pagination. Drift
 * requires a new explicit choice rather than silently rebinding a replacement. */
export function readPoolRecoveryTool(raw:unknown,pin:PoolToolRecoveryPin):ToolProfile|undefined{
  if(!validPoolToolRecoveryPin(pin)||!recoveryProfile(raw,pin.project_id))return;
  return raw.id===pin.tool_profile_id&&raw.version===pin.tool_profile_version&&raw.worker_id===pin.worker_id&&
    raw.profile.profile_sha256===pin.profile_sha256&&raw.profile.adapter===pin.adapter&&raw.profile.protocol===pin.protocol?raw:undefined;
}
export function buildPoolRecoveryProjectHref(pin:PoolToolRecoveryPin):string|undefined{
  if(!validPoolToolRecoveryPin(pin))return;
  return '?'+new URLSearchParams({project:pin.project_id,view:'settings',sub:'tools',record:pin.tool_profile_id,panel:serializePoolToolRecoveryPin(pin)});
}
export function buildPoolRecoveryOwnerHref(pin:PoolToolRecoveryPin):string|undefined{
  if(!validPoolToolRecoveryPin(pin))return;
  return '?'+new URLSearchParams({view:'settings',sub:'execution-pools',record:pin.pool_id,panel:serializePoolToolRecoveryPin(pin)});
}
