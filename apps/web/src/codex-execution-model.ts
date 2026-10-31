import {tr} from './i18n';
import type {CodexBinding, CodexCatalog, CodexSelection} from '../../../packages/contracts/src';
import {validExecutionSkillPlan} from './codex-native-skills-model';

const hash = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{64}$/.test(v);
const identifier = (v: unknown, optional = false): v is string => typeof v === 'string' && (optional && v === '' || /^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v));
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const closed = (v: Record<string, unknown>, keys: string[]) => Object.keys(v).every(key => keys.includes(key));
const notice = (v: unknown) => v === undefined || typeof v === 'number' && Number.isSafeInteger(v) && v > 0;
/** Validate the public shape before rendering/importing; the server verifies
 * canonical digests and current worker authority before accepting a selection. */
export function validCodexCatalog(value: unknown): value is CodexCatalog {
 if (!object(value) || !closed(value, ['protocol','configuration_sha256','models','catalog_sha256']) || value.protocol !== 'codex-app-server-v1' || !hash(value.configuration_sha256) || !hash(value.catalog_sha256) || !Array.isArray(value.models) || !value.models.length || value.models.length > 256) return false;
 let previous = '';
 return value.models.every(model => {
  if (!object(model) || !closed(model,['model','reasoning_efforts','default_reasoning_effort']) || !identifier(model.model) || model.model <= previous || !Array.isArray(model.reasoning_efforts) || model.reasoning_efforts.length > 32 || !identifier(model.default_reasoning_effort,true)) return false;
  previous = model.model; let before = '';
  return model.reasoning_efforts.every(effort => {if (!identifier(effort) || effort <= before) return false; before = effort; return true;}) && (model.reasoning_efforts.length ? model.reasoning_efforts.includes(model.default_reasoning_effort) : model.default_reasoning_effort === '');
 });
}
export function validCodexSelection(value: unknown): value is CodexSelection {
 return object(value) && closed(value,['catalog_sha256','model','reasoning_effort','token_notice_limit']) && hash(value.catalog_sha256) && identifier(value.model) && identifier(value.reasoning_effort,true) && notice(value.token_notice_limit);
}
export function codexSelection(catalog: CodexCatalog, model: string, effort: string, threshold: string): CodexSelection | undefined {
 const item=catalog.models.find(item=>item.model===model);
 if (!item || (item.reasoning_efforts.length ? !item.reasoning_efforts.includes(effort) : effort!=='')) return;
 let amount: number | undefined;
 if (threshold !== '') {if (!/^[1-9]\d*$/.test(threshold)) return; amount=Number(threshold); if (!notice(amount)) return;}
 return {catalog_sha256:catalog.catalog_sha256,model,reasoning_effort:effort,...(amount===undefined?{}:{token_notice_limit:amount})};
}
export function codexBindingPresent(value: unknown): value is CodexBinding {
 return object(value) && closed(value,['protocol','configuration_sha256','catalog_sha256','model','reasoning_effort','token_notice_limit','budget_mode','identity_evidence','binding_sha256']) && value.protocol==='codex-app-server-v1' && hash(value.binding_sha256) && hash(value.configuration_sha256) && hash(value.catalog_sha256) && identifier(value.model) && identifier(value.reasoning_effort,true) && notice(value.token_notice_limit) && value.budget_mode==='observe_only' && value.identity_evidence==='configuration_ack_only';
}
export function codexBindingMatchesSelection(binding: unknown, selection: unknown): binding is CodexBinding {
 return validCodexSelection(selection) && codexBindingPresent(binding) && binding.catalog_sha256===selection.catalog_sha256 && binding.model===selection.model && binding.reasoning_effort===selection.reasoning_effort && binding.token_notice_limit===selection.token_notice_limit;
}
export function sameCodexBinding(left: unknown, right: CodexBinding): boolean {
 return codexBindingPresent(left) && Object.entries(right).every(([key,value])=>left[key as keyof CodexBinding]===value) && Object.keys(left).length===Object.keys(right).length;
}

/** Recovery acknowledges only the original public model choice and task/tool
 * scope. An unrelated or malformed HTTP 200 keeps the durable operation key. */
export function validCodexDispatchReceipt(value: unknown, body: Record<string,unknown>): boolean {
 if(object(value)&&!validExecutionSkillPlan(value))return false;
 const selection=body.codex_execution_selection;
 if(object(value)&&Object.hasOwn(value,'execution_skills')){
  const knowledge=object(value.context)?value.context.knowledge:undefined;
  if(selection===undefined||!Array.isArray(knowledge)||!Array.isArray(body.knowledge)||body.knowledge.length!==knowledge.length||!body.knowledge.every((ref,index)=>object(ref)&&object(knowledge[index])&&['id','revision','sha256'].every(key=>ref[key]===knowledge[index][key])))return false;
 }
 if (selection === undefined) return true;
 if (!validCodexSelection(selection) || !object(value) || !identifier(value.id) || value.project_id!==body.project_id || value.task_id!==body.task_id || value.task_version!==body.expected_task_version || value.workflow_id!==body.workflow_id || value.workflow_version!==body.workflow_version || value.agent_node_id!==body.agent_node_id || value.tool_profile_id!==body.tool_profile_id || value.tool_profile_version!==body.expected_tool_version || !codexBindingPresent(value.codex_execution)) return false;
 const binding=value.codex_execution;
 if(binding.catalog_sha256!==selection.catalog_sha256 || binding.model!==selection.model || binding.reasoning_effort!==selection.reasoning_effort || binding.token_notice_limit!==selection.token_notice_limit || !object(value.workflow) || !Array.isArray(value.workflow.nodes)) return false;
 const node=value.workflow.nodes.find((node:unknown)=>object(node)&&node.id===body.agent_node_id);
 if (!object(node)||!object(node.config)||!codexBindingPresent(node.config.codex_execution)) return false;
 const nodeBinding=node.config.codex_execution as unknown as Record<string,unknown>;
 return Object.entries(binding).every(([key,value])=>nodeBinding[key]===value);
}

// Known product labels translate; provider-defined future values remain literal
// data rather than acquiring an invented meaning through a fallback map.
export function codexEffortLabel(value:string):string {
 return ['none','minimal','low','medium','high','xhigh','max','ultra'].includes(value)?tr('codex-execution.effort_'+value):value;
}
