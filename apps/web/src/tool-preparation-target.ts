import type {AdapterDescriptor} from './AdapterConnections';

/** One current protocol directory shared by navigation and public preparation
 * drafts. A product name or CLI installation never selects another protocol. */
const targets=[
 {localTool:'codex-jsonl',preset:'codex',adapter:'codex',protocol:'codex-jsonl-v1',special:false},
 {localTool:'codebuddy-cli',preset:'codebuddy',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1',special:false},
 {localTool:'dsh-multica',preset:'harness',adapter:'harness',protocol:'dsh-multica-jsonl-v1',special:false},
 {localTool:'claude-code-cli',preset:'claude',adapter:'claude',protocol:'claude-jsonl-v1',special:false},
 {localTool:'qoder-acp',preset:'qoder',adapter:'qoder',protocol:'acp-stdio-v1',special:true},
 {localTool:'opencode-sdk',preset:'opencode',adapter:'opencode',protocol:'opencode-sdk-v1',special:true},
 {localTool:'cursor-cli',preset:'cursor',adapter:'cursor',protocol:'acp-stdio-v1',special:true},
 {localTool:'claude-code-sdk',preset:'claude-sdk',adapter:'claude',protocol:'claude-sdk-v1',special:true},
 {localTool:'codex-app-server',preset:'codex-app-server',adapter:'codex',protocol:'codex-app-server-v1',special:true},
] as const;
export type ToolPreparationTarget=(typeof targets)[number];
export type ToolPreparationPreset=ToolPreparationTarget['preset'];
export type ToolPreparationSelection={localTool:ToolPreparationTarget['localTool'];nonce:number};
export function preparationForLocalTool(value:unknown):ToolPreparationTarget|undefined{return targets.find(target=>target.localTool===value);}
export function preparationForPreset(value:unknown):ToolPreparationTarget|undefined{return targets.find(target=>target.preset===value);}
export function toolPreparationTarget(value:AdapterDescriptor|undefined):ToolPreparationTarget|undefined{
 if(!value||value.direction!=='suma_to_tool'||value.implementation!=='implemented'||value.setup_kind!=='runner_profile')return;
 const target=preparationForLocalTool(value.id);return target&&value.adapter===target.adapter&&value.protocol===target.protocol?target:undefined;
}
export function readToolPreparationSelection(value:unknown):ToolPreparationSelection|undefined{
 if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return;
 const row=value as Record<string,unknown>;
 if(Reflect.ownKeys(row).length!==2||!Object.hasOwn(row,'localTool')||!Object.hasOwn(row,'nonce')||!Number.isSafeInteger(row.nonce)||(row.nonce as number)<=0||!preparationForLocalTool(row.localTool))return;
 return {localTool:row.localTool as ToolPreparationSelection['localTool'],nonce:row.nonce as number};
}
