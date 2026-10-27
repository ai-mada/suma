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
export type ModelScope = {project: string; profile: string; version: number; sha256: string};
export function modelOptionsPath(scope: ModelScope) {
  return '/model-execution/options?' + new URLSearchParams({project_id: scope.project, tool_profile_id: scope.profile, expected_tool_version: String(scope.version)});
}
/** Match the whole caller/tool scope before displaying or adopting a response. */
export function validModelOptions(value: ModelExecutionOptions, scope: ModelScope): boolean {
  return !!value && value.project_id === scope.project && value.tool_profile_id === scope.profile && value.tool_profile_version === scope.version && value.tool_profile_sha256 === scope.sha256 && typeof value.supported === 'boolean' && typeof value.observed_at === 'string' && Number.isFinite(Date.parse(value.observed_at)) && Array.isArray(value.items) && value.items.length <= 1000 && value.items.every(item => {
    const d = item.deployment;
    return !!d && typeof d.deployment_id === 'string' && d.deployment_id.length > 0 && Number.isSafeInteger(d.revision) && d.revision > 0 && digest(d.deployment_sha256) && digest(d.endpoint_config_sha256) && d.protocol === 'openai-chat-completions-v1' && typeof d.model === 'string' && d.model.length > 0 && Number.isSafeInteger(d.context_window) && d.context_window > 0 && ['priced', 'unpriced'].includes(item.cost_state) && (item.cost_state !== 'priced' || !!item.rate);
  });
}
export type ModelDraft = {deployment: string; steps: string; output: string; tokens: string; cost: string};
export function modelSelection(page: ModelExecutionOptions, draft: ModelDraft): ModelSelection | undefined {
  const item = page.items.find(item => item.deployment.deployment_sha256 === draft.deployment);
  const steps = modelInteger(draft.steps, 16), output = modelInteger(draft.output, 32768);
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
