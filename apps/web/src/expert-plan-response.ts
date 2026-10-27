import type {ExpertTeamPlan, ModelSelection} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validOrchestratedPlan} from './expert-team-plan-model';
import {validExpertModelItems} from './expert-model-input';

/** An HTTP success only clears durable intent after this consumer can identify
 * the original plan and fixed task/model choices. Unknown responses keep the key. */
export function expertPlanResponse(value: unknown, request: WriteRecoveryRequest, project: string): boolean {
  const plan=value as ExpertTeamPlan, body=request.body;
  if (!validOrchestratedPlan(plan,project)) return false;
  if (request.path==='/expert-team-plans') {
    return plan.team_id===body.team_id && plan.configuration_version===body.expected_configuration_version &&
      validExpertModelItems(body.items) && plan.items.length===body.items.length && itemsMatch(plan,body.items);
  }
  const route=request.path.match(/^\/expert-team-plans\/([A-Za-z0-9_-]+)\/(start|commands)$/);
  if (!route || plan.id!==route[1] || typeof body.expected_version!=='number' || plan.version<=body.expected_version) return false;
  if (route[2]==='start') return plan.state!=='ready' && plan.activation_generation>0;
  return body.action!=='revise' || validExpertModelItems(body.items) && plan.configuration_version===body.expected_configuration_version && itemsMatch(plan,body.items);
}
function itemsMatch(plan:ExpertTeamPlan, requests: {task_id:string;expected_task_version:number;role_key:string;model_execution_selection?:ModelSelection|null}[]) {
  return requests.every(request=>{
    const item=plan.items.find(item=>item.task_id===request.task_id);
    if (!item || item.task_version!==request.expected_task_version || item.role_key!==request.role_key) return false;
    const selection=request.model_execution_selection,binding=item.plan.model_execution;
    if (!selection) return !binding;
    if (!binding || binding.deployment.deployment_id!==selection.deployment_id || binding.deployment.revision!==selection.deployment_revision || binding.deployment.deployment_sha256!==selection.deployment_sha256 || binding.parameters.max_steps!==selection.parameters.max_steps || binding.parameters.max_output_tokens!==selection.parameters.max_output_tokens) return false;
    const actual=binding.budget, expected=selection.budget;
    return (!actual&&!expected) || !!actual&&!!expected && actual.token_limit===expected.token_limit && actual.cost_limit_micros===expected.cost_limit_micros && actual.currency===expected.currency;
  });
}
