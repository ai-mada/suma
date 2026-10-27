import type {ExpertTeamPlan, ExpertTeamPlanItemRequest} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validOrchestratedPlan} from './expert-team-plan-model';
import {validExpertModelItems} from './expert-model-input';
import {modelBindingMatchesSelection, sameModelBinding} from './model-execution-input';
import {codexBindingMatchesSelection, sameCodexBinding} from './codex-execution-model';

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
function itemsMatch(plan:ExpertTeamPlan, requests: ExpertTeamPlanItemRequest[]) {
  return requests.every(request=>{
    const item=plan.items.find(item=>item.task_id===request.task_id);
    if (!item || item.task_version!==request.expected_task_version || item.role_key!==request.role_key) return false;
    const selection=request.model_execution_selection,binding=item.plan.model_execution,codexSelection=request.codex_execution_selection,codexBinding=item.plan.codex_execution;
    if (!modelBindingMatchesSelection(binding,selection) || !!codexSelection!==!!codexBinding) return false;
    if (!selection && !codexSelection) return true;
    const node=item.plan.workflow?.nodes.find(node=>node.id===item.plan.agent_node_id);
    if (!node?.config) return false;
    if (codexSelection) return !binding && codexBindingMatchesSelection(codexBinding,codexSelection) && !node.config.model_execution && sameCodexBinding(node.config.codex_execution,codexBinding);
    return !codexBinding && !!binding && !node.config.codex_execution && sameModelBinding(node.config.model_execution,binding);
  });
}
