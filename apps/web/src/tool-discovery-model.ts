import {UiError} from './i18n';
import {parseToolManifest} from './tool-model';

const identity=/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const fields=new Set(['project_id','worker_id','state','as_of','observed_at','expires_at','items']);
function invalid():never{throw new UiError('tool-discovery.invalid_read');}
function timestamp(value:unknown):number{
 if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(value))return invalid();
 const parsed=Date.parse(value);
 if(!Number.isFinite(parsed)||new Date(parsed).toISOString()!==value.replace(/Z$/,'.000Z'))return invalid();
 return parsed;
}

/** A local view derived from a checked response, not another public wire DTO. */
export function parseToolDiscovery(value:unknown,project:string,worker:string){
 if(!identity.test(project)||!identity.test(worker)||!value||typeof value!=='object'||Array.isArray(value))return invalid();
 const raw=value as Record<string,unknown>;
 if(Object.keys(raw).some(key=>!fields.has(key))||raw.project_id!==project||raw.worker_id!==worker||!Array.isArray(raw.items))return invalid();
 const state=raw.state;
 if(state!=='fresh'&&state!=='stale'&&state!=='unreported')return invalid();
 const asOfMs=timestamp(raw.as_of);
 if(state!=='fresh'&&raw.items.length!==0)return invalid();
 let observedAt:string|undefined,expiresAt:string|undefined,remainingMs=0;
 if(raw.observed_at!==undefined||raw.expires_at!==undefined){
  const observedMs=timestamp(raw.observed_at),expiresMs=timestamp(raw.expires_at);
  if(expiresMs-observedMs!==60000||state==='unreported')return invalid();
  if(state==='fresh'&&(observedMs>asOfMs||asOfMs>=expiresMs))return invalid();
  if(state==='stale'&&observedMs<=asOfMs&&asOfMs<expiresMs)return invalid();
  observedAt=raw.observed_at as string;expiresAt=raw.expires_at as string;
  if(state==='fresh')remainingMs=expiresMs-asOfMs;
 }else if(state==='fresh')return invalid();
 // Reuse the existing bounded, nonsecret Observation validator, including all
 // adapter capabilities; do not maintain a second list of permitted fields.
 let items:ReturnType<typeof parseToolManifest>;
 try{items=parseToolManifest(JSON.stringify({items:raw.items}));}catch{return invalid();}
 return {state,asOf:raw.as_of as string,observedAt,expiresAt,remainingMs,items};
}

export type ToolDiscoveryView=ReturnType<typeof parseToolDiscovery>;
