/** An explicit maintenance opt-in, not permission, installation or readiness. */
export type ManagedToolPreparationCapability={protocol:'suma-managed-tool-preparation-v1';adapter_ids:('qoder-acp'|'cursor-cli')[];generation_sha256:string};
export function validManagedToolPreparationCapability(value:unknown):value is ManagedToolPreparationCapability{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const row=value as Record<string,unknown>,ids=row.adapter_ids;
 return Reflect.ownKeys(row).length===3&&['protocol','adapter_ids','generation_sha256'].every(key=>Object.hasOwn(row,key))&&row.protocol==='suma-managed-tool-preparation-v1'&&typeof row.generation_sha256==='string'&&/^[a-f0-9]{64}$/.test(row.generation_sha256)&&Array.isArray(ids)&&ids.length>0&&ids.length<=2&&Reflect.ownKeys(ids).length===ids.length+1&&new Set(ids).size===ids.length&&ids.every(id=>id==='qoder-acp'||id==='cursor-cli');
}
