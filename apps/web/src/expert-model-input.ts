import type {ExpertTeamPlanItemRequest} from '../../../packages/contracts/src';
import {validCodexSelection} from './codex-execution-model';
import {validModelSelection} from './model-execution-input';
import {validTestSelectionFields} from './task-dispatch-test-bindings-model';
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const exact = (v: Record<string, unknown>, keys: string[]) => Object.keys(v).every(key => keys.includes(key));
const integer = (v: unknown, min: number, max = Number.MAX_SAFE_INTEGER): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= min && v <= max;
const id = (v: unknown) => typeof v === 'string' && /^[A-Za-z0-9_-]{1,64}$/.test(v);
export function validExpertModelItems(v: unknown): v is ExpertTeamPlanItemRequest[] {
  return Array.isArray(v) && v.length >= 1 && v.length <= 16 && v.every(item => object(item) &&
    exact(item,['task_id','expected_task_version','role_key','model_execution_selection','codex_execution_selection','test_bindings']) && id(item.task_id) && integer(item.expected_task_version,1) &&
    typeof item.role_key === 'string' && /^[a-z][a-z0-9_-]{0,39}$/.test(item.role_key) &&
    (!Object.hasOwn(item,'model_execution_selection') || validModelSelection(item.model_execution_selection)) &&
    (!Object.hasOwn(item,'codex_execution_selection') || validCodexSelection(item.codex_execution_selection)) &&
    (!Object.hasOwn(item,'test_bindings') || validTestSelectionFields(item.test_bindings)) && !(item.model_execution_selection && item.codex_execution_selection)) &&
    new Set(v.map(item=>item.task_id)).size===v.length;
}
