/** An explicit maintenance opt-in, not permission, installation or readiness. */
export type ManagedToolPreparationAdapterID='qoder-acp'|'cursor-cli'|'codex-jsonl'|'codebuddy-cli'|'dsh-multica'|'claude-code-cli'|'codex-app-server';
export type ManagedToolPreparationMode='model_and_credential'|'fixed_claude_model'|'existing_local_configuration';
/** Sole adapter-to-form policy. Unknown protocols and SDKs stay on their
 * original manual paths; installation never implies an automatic factory. */
export function managedToolPreparationMode(adapterID:unknown):ManagedToolPreparationMode|undefined{
 switch(adapterID){case 'qoder-acp':case 'cursor-cli':return 'model_and_credential';case 'claude-code-cli':return 'fixed_claude_model';case 'codex-jsonl':case 'codebuddy-cli':case 'dsh-multica':case 'codex-app-server':return 'existing_local_configuration';default:return;}
}
export type ManagedToolPreparationCapability={protocol:'suma-managed-tool-preparation-v1';adapter_ids:ManagedToolPreparationAdapterID[];generation_sha256:string};
export function validManagedToolPreparationCapability(value:unknown):value is ManagedToolPreparationCapability{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const row=value as Record<string,unknown>,ids=row.adapter_ids;
 return Reflect.ownKeys(row).length===3&&['protocol','adapter_ids','generation_sha256'].every(key=>Object.hasOwn(row,key))&&row.protocol==='suma-managed-tool-preparation-v1'&&typeof row.generation_sha256==='string'&&/^[a-f0-9]{64}$/.test(row.generation_sha256)&&Array.isArray(ids)&&ids.length>0&&ids.length<=7&&Reflect.ownKeys(ids).length===ids.length+1&&new Set(ids).size===ids.length&&ids.every(id=>managedToolPreparationMode(id)!==undefined);
}
