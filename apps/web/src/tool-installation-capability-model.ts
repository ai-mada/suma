import {preparationForLocalTool} from './tool-preparation-target';

export type InstalledDiscoveryCapability={protocol:'suma-installed-tool-scan-v1';adapter_ids:string[]};
/** A capability is a bounded opt-in declaration, never proof of installation,
 * model authentication, current project permission or dispatch readiness. */
export function validInstalledDiscoveryCapability(value:unknown):value is InstalledDiscoveryCapability{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const record=value as Record<string,unknown>,ids=record.adapter_ids;
 return Reflect.ownKeys(record).length===2&&Object.hasOwn(record,'protocol')&&Object.hasOwn(record,'adapter_ids')&&record.protocol==='suma-installed-tool-scan-v1'&&Array.isArray(ids)&&ids.length>0&&ids.length<=9&&Reflect.ownKeys(ids).length===ids.length+1&&new Set(ids).size===ids.length&&ids.every(id=>!!preparationForLocalTool(id));
}
