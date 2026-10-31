import type {OpenCodeExecutionCapability} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';

/** Go stores Scheme + "://" + Host verbatim, including host case and port. */
export function openCodeEndpointOrigin(baseURL:string):string|undefined {
 if(typeof baseURL!=='string'||!wellFormed(baseURL)||!baseURL||baseURL.trim()!==baseURL||new TextEncoder().encode(baseURL).length>2048||/[\p{Cc}\u2028\u2029?#\\]/u.test(baseURL))return;
 const match=/^(https|http):\/\/([^/]+)(?:\/.*)?$/u.exec(baseURL);if(!match||match[2].includes('@'))return;
 const authority=match[2],host=/^(\[[^\]]+\]|[^:]+)(?::([0-9]*))?$/.exec(authority);if(!host)return;
 try{const parsed=new URL(match[1]+'://'+host[1]);if(!parsed.hostname||parsed.username||parsed.password)return;}catch{return;}
 if(match[1]==='http'&&!['localhost','127.0.0.1','[::1]'].includes(host[1]))return;
 return match[1]+'://'+authority;
}

const keys=['sdk_version','server_version','provider_id','model','endpoint_origin','configuration_sha256','budget_mode','identity_evidence'];
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&value.trim()===value&&value.length>0&&new TextEncoder().encode(value).length<=max&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
/** Public fixed configuration only. Credentials, executable paths and SDK
 * options never belong in browser inventory or a restored task preview. */
export function validOpenCodeCapability(value:unknown):value is OpenCodeExecutionCapability {
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const p=value as Record<string,unknown>;
 if(Object.keys(p).length!==keys.length||Object.keys(p).some(key=>!keys.includes(key))||p.sdk_version!=='1.19.4'||p.server_version!=='1.19.4'||!text(p.provider_id,100)||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(p.provider_id)||!text(p.model,256)||!/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$/.test(p.model)||!text(p.endpoint_origin,2048)||typeof p.configuration_sha256!=='string'||!/^[a-f0-9]{64}$/.test(p.configuration_sha256)||p.budget_mode!=='observe_only'||p.identity_evidence!=='configuration_ack_only')return false;
 return openCodeEndpointOrigin(p.endpoint_origin)===p.endpoint_origin;
}

export function sameOpenCodeCapability(a:unknown,b:unknown):boolean {
 return validOpenCodeCapability(a)&&validOpenCodeCapability(b)&&keys.every(key=>a[key as keyof OpenCodeExecutionCapability]===b[key as keyof OpenCodeExecutionCapability]);
}

/** Fixed facts in a preview must agree with its actual producer definition;
 * displaying these facts never grants execution or confirms vendor identity. */
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export function validOpenCodePlan(value:unknown):boolean {
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[];
 const node=nodes.find((item:unknown)=>object(item)&&item.id===value.agent_node_id);
 const config=object(node)&&object(node.config)?node.config:undefined;
 if(value.opencode_execution===undefined&&config?.opencode_execution===undefined)return true;
 return !value.model_execution&&!value.codex_execution&&!config?.model_execution&&!config?.codex_execution&&sameOpenCodeCapability(value.opencode_execution,config?.opencode_execution);
}

/** A malformed success must not erase the original operation key. Match the
 * original task/tool scope as well as the fixed producer configuration. */
export function validOpenCodeDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean {
 if(!validOpenCodePlan(value)||!object(value))return false;
 if(value.opencode_execution===undefined)return true;
 return typeof value.id==='string'&&value.id.length>0&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!body.model_execution_selection&&!body.codex_execution_selection;
}
