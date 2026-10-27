import type {ExpertTeamPlanItemRequest, ModelSelection} from '../../../packages/contracts/src';
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const exact = (v: Record<string, unknown>, keys: string[]) => Object.keys(v).every(key => keys.includes(key));
const integer = (v: unknown, min: number, max = Number.MAX_SAFE_INTEGER): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max;
const id = (v: unknown) => typeof v === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(v);
/** Public numerical allowances are accepted only inside this closed typed path.
 * Local recovery never admits arbitrary tokens, URLs, environment or credentials. */
export function validExpertModelSelection(v: unknown): v is ModelSelection {
  if (!object(v) || !exact(v, ['deployment_id','deployment_revision','deployment_sha256','parameters','budget']) ||
      typeof v.deployment_id !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(v.deployment_id) ||
      !integer(v.deployment_revision,1,2147483647) || typeof v.deployment_sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(v.deployment_sha256) ||
      !object(v.parameters) || !exact(v.parameters,['max_steps','max_output_tokens']) || !integer(v.parameters.max_steps,1,16) || !integer(v.parameters.max_output_tokens,1,32768)) return false;
  if (v.budget === undefined || v.budget === null) return true;
  if (!object(v.budget) || !exact(v.budget,['token_limit','cost_limit_micros','currency'])) return false;
  const b=v.budget, token=Object.hasOwn(b,'token_limit'), cost=Object.hasOwn(b,'cost_limit_micros');
  return (token || cost) && (!token || integer(b.token_limit,0)) && (!cost || integer(b.cost_limit_micros,0)) &&
    (cost ? ['CNY','USD'].includes(String(b.currency)) : !Object.hasOwn(b,'currency'));
}
export function validExpertModelItems(v: unknown): v is ExpertTeamPlanItemRequest[] {
  return Array.isArray(v) && v.length >= 1 && v.length <= 16 && v.every(item => object(item) &&
    exact(item,['task_id','expected_task_version','role_key','model_execution_selection']) && id(item.task_id) && integer(item.expected_task_version,1) &&
    typeof item.role_key === 'string' && /^[a-z][a-z0-9_-]{0,39}$/.test(item.role_key) &&
    (!Object.hasOwn(item,'model_execution_selection') || item.model_execution_selection === null || validExpertModelSelection(item.model_execution_selection))) &&
    new Set(v.map(item=>item.task_id)).size===v.length;
}
