import type {ModelBinding, ModelSelection} from '../../../packages/contracts/src';
import {bindingPresent} from './model-execution-model';

const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const exact = (v: Record<string, unknown>, keys: string[]) => Object.keys(v).every(key => keys.includes(key));
const integer = (v: unknown, min: number, max = Number.MAX_SAFE_INTEGER): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max;
/** Only public numerical allowances are admitted into durable request recovery. */
export function validModelSelection(v: unknown): v is ModelSelection {
  if (!object(v) || !exact(v, ['deployment_id','deployment_revision','deployment_sha256','parameters','budget']) ||
      typeof v.deployment_id !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(v.deployment_id) ||
      !integer(v.deployment_revision,1,2147483647) || typeof v.deployment_sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(v.deployment_sha256) ||
      !object(v.parameters) || !exact(v.parameters,['max_steps','max_output_tokens']) || !integer(v.parameters.max_steps,1,128) || !integer(v.parameters.max_output_tokens,1,131072)) return false;
  if (v.budget === undefined || v.budget === null) return true;
  if (!object(v.budget) || !exact(v.budget,['token_limit','cost_limit_micros','currency'])) return false;
  const b=v.budget, token=Object.hasOwn(b,'token_limit'), cost=Object.hasOwn(b,'cost_limit_micros');
  return (token || cost) && (!token || integer(b.token_limit,0)) && (!cost || integer(b.cost_limit_micros,0)) &&
    (cost ? ['CNY','USD'].includes(String(b.currency)) : !Object.hasOwn(b,'currency'));
}
export function modelBindingMatchesSelection(binding: ModelBinding | null | undefined, selection: unknown): boolean {
  if (selection == null) return binding == null;
  if (!validModelSelection(selection) || !bindingPresent(binding) || binding.deployment.deployment_id!==selection.deployment_id || binding.deployment.revision!==selection.deployment_revision || binding.deployment.deployment_sha256!==selection.deployment_sha256 || binding.parameters?.max_steps!==selection.parameters.max_steps || binding.parameters?.max_output_tokens!==selection.parameters.max_output_tokens) return false;
  const actual=binding.budget, expected=selection.budget;
  return (!actual&&!expected) || !!actual&&!!expected && actual.token_limit===expected.token_limit && actual.cost_limit_micros===expected.cost_limit_micros && actual.currency===expected.currency;
}
export function sameModelBinding(left: unknown, right: ModelBinding): boolean {
  if (!object(left)) return false;
  // Binding shape is nested; compare values independent of JSON key order.
  const same=(a:unknown,b:unknown):boolean=>a===b || object(a)&&object(b)&&Object.keys(a).length===Object.keys(b).length&&Object.keys(a).every(key=>same(a[key],b[key]));
  return same(left,right);
}
