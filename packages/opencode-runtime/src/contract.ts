import {workspaceBridge,type WorkspaceBridge} from '@suma/agent-runtime/workspace-client';

export const VERSION='1.0.0',SDK_VERSION='1.19.4';
export const TOOL_NAMES=['list','read','write','edit','remove','check'] as const;
export type ToolName=typeof TOOL_NAMES[number];
export const object=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
export const safeID=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&value.length<=256&&!/[\p{Cc}\p{Cs}]/u.test(value);
export class RuntimeError extends Error{constructor(readonly code:string){super(code);}}
export interface ExecuteRequest{v:1;type:'execute';request_id:string;goal:string;workspace_bridge:WorkspaceBridge}
export interface RuntimeConfig{executable:string;executable_sha256:string;home:string;source:string;provider_id:string;model:string;base_url:string;api_key_env_ref?:string;configuration_sha256:string;tool_entry:string;plugin_package_root:string}
export type Frame={v:1;type:string;request_id:string;[key:string]:unknown};
export type Emit=(frame:Frame)=>void;
export function closed(value:unknown,keys:string[]):asserts value is Record<string,unknown>{
 if(!object(value)||Object.keys(value).some(key=>!keys.includes(key)))throw new RuntimeError('request_invalid');
}
export function parseExecute(value:unknown):ExecuteRequest{
 closed(value,['v','type','request_id','goal','workspace_bridge']);
 if(value.v!==1||value.type!=='execute'||!safeID(value.request_id)||typeof value.goal!=='string'||!value.goal.trim()||Buffer.byteLength(value.goal)>262144)throw new RuntimeError('request_invalid');
 const bridge=workspaceBridge(value.workspace_bridge,value.request_id);if(bridge.conflict_repair_protocol)throw new RuntimeError('request_invalid');
 return {v:1,type:'execute',request_id:value.request_id,goal:value.goal,workspace_bridge:bridge};
}
export function stableCallIdentity(request:string,context:unknown,tool:string):string{
 if(!safeID(request)||!object(context)||!safeID(context.sessionID)||!safeID(context.messageID)||!safeID(context.callID)||!TOOL_NAMES.includes(tool as ToolName))throw new RuntimeError('workspace_identity');
 return JSON.stringify([request,context.sessionID,context.messageID,context.callID,tool]);
}
export interface FinalIdentity{sessionID:string;userMessageID:string;providerID:string;model:string}
export function validateFinal(value:unknown,identity:FinalIdentity):{assistant_message_id:string;completed_at:number}{
 if(!object(value)||!object(value.info)||!Array.isArray(value.parts))throw new RuntimeError('terminal_invalid');
 const info=value.info;
 if(info.role!=='assistant'||!safeID(info.id)||info.sessionID!==identity.sessionID||info.parentID!==identity.userMessageID||info.providerID!==identity.providerID||info.modelID!==identity.model||info.finish!=='stop'||info.error!==undefined||!object(info.time)||typeof info.time.completed!=='number'||!Number.isSafeInteger(info.time.completed)||info.time.completed<=0)throw new RuntimeError('terminal_invalid');
 for(const part of value.parts){if(!object(part))throw new RuntimeError('terminal_invalid');if(part.type==='tool'&&(!object(part.state)||!['completed','error'].includes(String(part.state.status))||object(part.state.metadata)&&part.state.metadata.interrupted===true))throw new RuntimeError('terminal_invalid');}
 return {assistant_message_id:info.id,completed_at:info.time.completed};
}
export function errorCode(error:unknown):string{return error instanceof RuntimeError?error.code:'runtime_failed';}
