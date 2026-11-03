import {toolDiscoveryTimestampNs,type ToolDiscoveryView} from './tool-discovery-model';
import type {ToolObservation} from './tool-model';

/** An ephemeral selection, never persisted authority or proof of a command
 * having run. Freshness still comes from the current authenticated inventory. */
export type ToolConnectObservationPin=Readonly<{profileSHA256:string;observedAt:string;preparationID?:string}>;
export function validToolConnectObservationPin(value:unknown):value is ToolConnectObservationPin{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const row=value as Record<string,unknown>;
 if(![2,3].includes(Reflect.ownKeys(row).length)||Reflect.ownKeys(row).some(key=>typeof key!=='string'||!['profileSHA256','observedAt','preparationID'].includes(key))||!Object.hasOwn(row,'profileSHA256')||!Object.hasOwn(row,'observedAt')||typeof row.profileSHA256!=='string'||!/^[a-f0-9]{64}$/.test(row.profileSHA256)||Object.hasOwn(row,'preparationID')&&(typeof row.preparationID!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(row.preparationID)))return false;
 try{toolDiscoveryTimestampNs(row.observedAt);return true;}catch{return false;}
}
/** The same configuration may be observed again. A clock rollback or another
 * profile with the same command ID cannot silently replace the chosen one.
 * Callers additionally deduct request/elapsed time from the live inventory TTL. */
export function toolConnectObservationMatches(pin:ToolConnectObservationPin,view:Pick<ToolDiscoveryView,'state'|'observedAt'>,observation:ToolObservation|undefined):boolean{
 if(!validToolConnectObservationPin(pin)||view.state!=='fresh'||!view.observedAt||!observation||!observation.available&&!pin.preparationID||observation.profile.profile_sha256!==pin.profileSHA256)return false;
 try{return toolDiscoveryTimestampNs(view.observedAt)>=toolDiscoveryTimestampNs(pin.observedAt);}catch{return false;}
}
