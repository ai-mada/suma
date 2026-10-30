import {UiError} from './i18n';
import {parseToolManifest} from './tool-model';

const identity=/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const fields=new Set(['project_id','worker_id','state','as_of','observed_at','expires_at','items']);
function invalid():never{throw new UiError('tool-discovery.invalid_read');}
function timestamp(value:unknown):bigint{
 if(typeof value!=='string')return invalid();
 const match=/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
 if(!match)return invalid();
 const fraction=(match[2]??'').padEnd(9,'0'),canonical=match[1]+'.'+fraction.slice(0,3)+'Z',parsed=Date.parse(canonical);
 // Date checks the Gregorian calendar at millisecond precision. Retain the
 // remaining digits separately so future observations and expiry boundaries
 // within the same millisecond cannot become a fresh report through rounding.
 if(!Number.isFinite(parsed)||new Date(parsed).toISOString()!==canonical)return invalid();
 return BigInt(parsed)*1000000n+BigInt(fraction.slice(3));
}

/** A local view derived from a checked response, not another public wire DTO. */
export function parseToolDiscovery(value:unknown,project:string,worker:string){
 if(!identity.test(project)||!identity.test(worker)||!value||typeof value!=='object'||Array.isArray(value))return invalid();
 const raw=value as Record<string,unknown>;
 if(Object.keys(raw).some(key=>!fields.has(key))||raw.project_id!==project||raw.worker_id!==worker||!Array.isArray(raw.items))return invalid();
 const state=raw.state;
 if(state!=='fresh'&&state!=='stale'&&state!=='unreported')return invalid();
 const asOfNs=timestamp(raw.as_of);
 if(state!=='fresh'&&raw.items.length!==0)return invalid();
 let observedAt:string|undefined,expiresAt:string|undefined,remainingMs=0;
 if(raw.observed_at!==undefined||raw.expires_at!==undefined){
  const observedNs=timestamp(raw.observed_at),expiresNs=timestamp(raw.expires_at);
  if(expiresNs-observedNs!==60000000000n||state==='unreported')return invalid();
  if(state==='fresh'&&(observedNs>asOfNs||asOfNs>=expiresNs))return invalid();
  if(state==='stale'&&observedNs<=asOfNs&&asOfNs<expiresNs)return invalid();
  observedAt=raw.observed_at as string;expiresAt=raw.expires_at as string;
  if(state==='fresh')remainingMs=Number((expiresNs-asOfNs)/1000000n);
 }else if(state==='fresh')return invalid();
 // Reuse the existing bounded, nonsecret Observation validator, including all
 // adapter capabilities; do not maintain a second list of permitted fields.
 let items:ReturnType<typeof parseToolManifest>;
 try{items=parseToolManifest(JSON.stringify({items:raw.items}));}catch{return invalid();}
 return {state,asOf:raw.as_of as string,observedAt,expiresAt,remainingMs,items};
}

export type ToolDiscoveryView=ReturnType<typeof parseToolDiscovery>;
