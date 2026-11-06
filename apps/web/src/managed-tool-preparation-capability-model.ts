/** An explicit maintenance opt-in, not permission, installation or readiness. */
export type ManagedToolPreparationAdapterID='qoder-acp'|'cursor-cli'|'codex-jsonl'|'codebuddy-cli'|'dsh-multica'|'claude-code-cli'|'codex-app-server';
export type ManagedToolPreparationMode='model_and_credential'|'fixed_claude_model'|'existing_local_configuration';
/** Sole adapter-to-form policy. Unknown protocols and SDKs stay on their
 * original manual paths; installation never implies an automatic factory. */
export function managedToolPreparationMode(adapterID:unknown):ManagedToolPreparationMode|undefined{
 switch(adapterID){case 'qoder-acp':case 'cursor-cli':return 'model_and_credential';case 'claude-code-cli':return 'fixed_claude_model';case 'codex-jsonl':case 'codebuddy-cli':case 'dsh-multica':case 'codex-app-server':return 'existing_local_configuration';default:return;}
}
export type ManagedToolPreparationCapability={protocol:'suma-managed-tool-preparation-v1';adapter_ids:ManagedToolPreparationAdapterID[];generation_sha256:string;codex_template_ids?:string[]};
export const validCodexTemplateID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value);
export function managedToolPreparationSupports(capability:ManagedToolPreparationCapability,adapterID:unknown):boolean{
 return capability.adapter_ids.some(id=>id===adapterID)&&(adapterID!=='codex-app-server'||!!capability.codex_template_ids?.length);
}
export function validManagedToolPreparationCapability(value:unknown):value is ManagedToolPreparationCapability{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
 const row=value as Record<string,unknown>,ids=row.adapter_ids,hasTemplates=Object.hasOwn(row,'codex_template_ids');
 if(Reflect.ownKeys(row).length!==(hasTemplates?4:3)||!['protocol','adapter_ids','generation_sha256'].every(key=>Object.hasOwn(row,key))||row.protocol!=='suma-managed-tool-preparation-v1'||typeof row.generation_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.generation_sha256)||!Array.isArray(ids)||ids.length<1||ids.length>7||Reflect.ownKeys(ids).length!==ids.length+1||new Set(ids).size!==ids.length||!Array.from(ids).every(id=>managedToolPreparationMode(id)!==undefined))return false;
 if(!hasTemplates)return true;
 const templates=row.codex_template_ids;
 return ids.includes('codex-app-server')&&Array.isArray(templates)&&templates.length>=1&&templates.length<=8&&
  Reflect.ownKeys(templates).length===templates.length+1&&Array.from(templates).every((id,index)=>validCodexTemplateID(id)&&(index===0||templates[index-1]<id));
}
