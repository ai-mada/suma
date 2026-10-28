import {validOpenCodeCapability} from './opencode-execution-model';
import type {ModelBinding, ModelExecutionOptions, ModelSelection} from '../../../packages/contracts/src';

const digest = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
export const modelInteger = (value: string, max = Number.MAX_SAFE_INTEGER) => /^(0|[1-9]\d*)$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) <= max ? Number(value) : undefined;
/** Decimal money is converted without binary floating point rounding. */
export function modelMoneyMicros(value: string): number | undefined {
  if (!/^(0|[1-9]\d*)(\.\d{1,6})?$/.test(value)) return;
  const [whole, fraction = ''] = value.split('.');
  const result = BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6, '0'));
  return result <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(result) : undefined;
}
/** Exact form input stays independent of localized display/group separators. */
export function modelMoneyInput(value:number):string{
 const micros=BigInt(value),whole=String(micros/1000000n),fraction=String(micros%1000000n).padStart(6,'0').replace(/0+$/,'');return whole+(fraction?'.'+fraction:'');
}
export type ModelScope = {project: string; profile: string; version: number; sha256: string};
export function modelOptionsPath(scope: ModelScope) {
  return '/model-execution/options?' + new URLSearchParams({project_id: scope.project, tool_profile_id: scope.profile, expected_tool_version: String(scope.version)});
}
/** Match the whole caller/tool scope before displaying or adopting a response. */
export function validModelOptions(value: ModelExecutionOptions, scope: ModelScope): boolean {
  if(!value||!closed(value as unknown as Record<string,unknown>,['parameter_limits','fixed_opencode','codex_supported','codex_catalog','codex_reason','project_id','tool_profile_id','tool_profile_version','tool_profile_sha256','observed_at','supported','reason','items'])||value.supported&&(!validParameterLimits(value.parameter_limits)||value.codex_supported)||value.parameter_limits!==undefined&&!validParameterLimits(value.parameter_limits))return false;
  return !!value && (value.fixed_opencode===undefined||!value.supported&&!value.codex_supported&&value.reason==='fixed_by_tool'&&Array.isArray(value.items)&&value.items.length===0&&validOpenCodeCapability(value.fixed_opencode)) && value.project_id === scope.project && value.tool_profile_id === scope.profile && value.tool_profile_version === scope.version && value.tool_profile_sha256 === scope.sha256 && typeof value.supported === 'boolean' && typeof value.observed_at === 'string' && Number.isFinite(Date.parse(value.observed_at)) && Array.isArray(value.items) && value.items.length <= 1000 && value.items.every(item => {
    const d = item.deployment;
    return !!d && typeof d.deployment_id === 'string' && d.deployment_id.length > 0 && Number.isSafeInteger(d.revision) && d.revision > 0 && digest(d.deployment_sha256) && digest(d.endpoint_config_sha256) && d.protocol === 'openai-chat-completions-v1' && typeof d.model === 'string' && d.model.length > 0 && Number.isSafeInteger(d.context_window) && d.context_window > 0 && validParameterLimits(item.default_parameters) && (!value.parameter_limits||item.default_parameters.max_steps<=value.parameter_limits.max_steps&&item.default_parameters.max_output_tokens<=value.parameter_limits.max_output_tokens) && ['priced', 'unpriced'].includes(item.cost_state) && (item.cost_state !== 'priced' || !!item.rate);
  });
}
export type ModelDraft = {deployment: string; steps: string; output: string; tokens: string; cost: string};
export function modelSelection(page: ModelExecutionOptions, draft: ModelDraft): ModelSelection | undefined {
  const item = page.items.find(item => item.deployment.deployment_sha256 === draft.deployment);
  if(!validParameterLimits(page.parameter_limits))return;
  const steps = modelInteger(draft.steps, page.parameter_limits.max_steps), output = modelInteger(draft.output, page.parameter_limits.max_output_tokens);
  if (!page.supported || !item || !steps || !output) return;
  const tokens = draft.tokens === '' ? undefined : modelInteger(draft.tokens);
  const cost = draft.cost === '' ? undefined : modelMoneyMicros(draft.cost);
  if (draft.tokens !== '' && tokens === undefined || draft.cost !== '' && (cost === undefined || !item.rate || item.cost_state !== 'priced')) return;
  const inputReserve=BigInt(item.deployment.context_window)*BigInt(steps), outputReserve=BigInt(output);
  if(tokens!==undefined&&BigInt(tokens)<inputReserve+outputReserve)return;
  if(cost!==undefined&&item.rate){
    const quoted=(inputReserve*BigInt(item.rate.input_rate_micros_per_million)+outputReserve*BigInt(item.rate.output_rate_micros_per_million)+999999n)/1000000n;
    if(BigInt(cost)<quoted)return;
  }
  const selection: ModelSelection = {deployment_id: item.deployment.deployment_id, deployment_revision: item.deployment.revision, deployment_sha256: item.deployment.deployment_sha256, parameters: {max_steps: steps, max_output_tokens: output}};
  if (tokens !== undefined || cost !== undefined) selection.budget = {...(tokens === undefined ? {} : {token_limit: tokens}), ...(cost === undefined ? {} : {cost_limit_micros: cost, currency: item.rate!.currency})};
  return selection;
}
export function bindingPresent(value: ModelBinding | null | undefined): value is ModelBinding {
  return !!value && value.protocol === 'suma-model-execution-v1' && digest(value.binding_sha256) && digest(value.deployment?.deployment_sha256) && digest(value.parameters_sha256);
}
export function validParameterLimits(value:unknown):value is {max_steps:number;max_output_tokens:number}{
 if(!object(value)||Object.keys(value).some(key=>!['max_steps','max_output_tokens'].includes(key)))return false;
 return positive(value.max_steps,128)&&positive(value.max_output_tokens,131072);
}
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const positive=(value:unknown,max=Number.MAX_SAFE_INTEGER):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=max;
const number=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const closed=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item:unknown)=>object(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
/** Immutable binding equality includes budget, rate and reservations, not only
 * a matching label or a caller-supplied digest. Go remains the hash authority. */
export function sameModelBinding(left:unknown,right:unknown):boolean{
 return validModelBinding(left)&&validModelBinding(right)&&canonical(left)===canonical(right);
}
/** A selected platform deployment must survive as the same complete producer
 * binding. A successful HTTP response alone cannot settle original intent. */
export function validModelPlan(value:unknown):boolean{
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],matches=nodes.filter(item=>object(item)&&item.id===value.agent_node_id),node=matches[0],config=object(node)&&object(node.config)?node.config:undefined;
 if(Object.hasOwn(value,'octop_execution')||nodes.some(item=>object(item)&&object(item.config)&&Object.hasOwn(item.config,'octop_execution')))return false;
 if(value.model_execution===undefined&&!matches.some(item=>object(item)&&object(item.config)&&item.config.model_execution!==undefined))return true;
 return matches.length===1&&object(node)&&node.type==='agent'&&(node.max_retries===undefined||node.max_retries===0)&&!!config&&digest(value.tool_profile_sha256)&&typeof value.tool_profile_id==='string'&&value.tool_profile_id.length>0&&config.tool_profile_id===value.tool_profile_id&&config.tool_profile_sha256===value.tool_profile_sha256&&!value.codex_execution&&!value.opencode_execution&&!config.codex_execution&&!config.opencode_execution&&sameModelBinding(value.model_execution,config.model_execution);
}
export function validModelDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!validModelPlan(value)||!object(value))return false;
 const selection=body.model_execution_selection;
 if(selection===undefined)return value.model_execution===undefined;
 if(typeof value.id!=='string'||value.id.length===0||!object(selection)||!validModelBinding(value.model_execution)||!object(selection.parameters))return false;
 const binding=value.model_execution;
 return value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&binding.deployment.deployment_id===selection.deployment_id&&binding.deployment.revision===selection.deployment_revision&&binding.deployment.deployment_sha256===selection.deployment_sha256&&canonical(binding.parameters)===canonical(selection.parameters)&&canonical(binding.budget)===canonical(selection.budget);
}
export function validModelBinding(value:unknown):value is ModelBinding{
 if(!object(value)||!closed(value,['protocol','deployment','parameters','parameters_sha256','binding_sha256','input_token_reservation','output_token_reservation','budget','rate'])||value.protocol!=='suma-model-execution-v1'||!digest(value.parameters_sha256)||!digest(value.binding_sha256)||!object(value.deployment)||!object(value.parameters))return false;
 const d=value.deployment,p=value.parameters;
 if(!closed(d,['deployment_id','revision','deployment_sha256','protocol','model','endpoint_origin','endpoint_config_sha256','context_window'])||typeof d.deployment_id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(d.deployment_id)||!positive(d.revision,2147483647)||!digest(d.deployment_sha256)||!digest(d.endpoint_config_sha256)||d.protocol!=='openai-chat-completions-v1'||typeof d.model!=='string'||!d.model||!positive(d.context_window,10000000)||!validParameterLimits(p)||value.input_token_reservation!==d.context_window*p.max_steps||value.output_token_reservation!==p.max_output_tokens)return false;
 if(value.budget!==undefined){const b=value.budget;if(!object(b)||!closed(b,['token_limit','cost_limit_micros','currency'])||b.token_limit===undefined&&b.cost_limit_micros===undefined||b.token_limit!==undefined&&!number(b.token_limit)||b.cost_limit_micros!==undefined&&(!number(b.cost_limit_micros)||!['CNY','USD'].includes(String(b.currency))))return false;}
 if(value.rate!==undefined){const rate=value.rate;if(!object(rate)||!closed(rate,['id','version','currency','input_rate_micros_per_million','output_rate_micros_per_million'])||typeof rate.id!=='string'||!rate.id||!positive(rate.version,2147483647)||!['CNY','USD'].includes(String(rate.currency))||!number(rate.input_rate_micros_per_million)||!number(rate.output_rate_micros_per_million))return false;}
 return true;
}
/** Select a binding only from the exact published definition and current
 * attempt identity. Current inventory cannot rewrite a historical model. */
export function frozenModelForNode(definition:unknown,run:unknown,node:unknown):ModelBinding|undefined{
 if(!object(definition)||!object(run)||!object(node)||definition.id!==run.workflow_id||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes))return;
 const actual=run.nodes.find(item=>object(item)&&item.id===node.id),declared=definition.nodes.find(item=>object(item)&&item.id===node.id);
 if(!object(actual)||!object(declared)||!object(declared.config)||node.type!=='agent'||actual.type!=='agent'||declared.type!=='agent'||!validModelBinding(declared.config.model_execution))return;
 for(const current of [actual,node])if(current.attempt!==node.attempt||current.fence!==node.fence||current.tool_profile_id!==declared.config.tool_profile_id||current.tool_profile_sha256!==declared.config.tool_profile_sha256||canonical(current.command)!==canonical(declared.command))return;
 return declared.config.model_execution;
}
