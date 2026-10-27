import {expect,it} from 'vitest';
import {expertRecovery} from './expert-team-model';
import {expertPlanCommandRecovery,validExpertRevisionItems} from './expert-team-plan-model';
const selection={deployment_id:'model-a',deployment_revision:1,deployment_sha256:'a'.repeat(64),parameters:{max_steps:2,max_output_tokens:100},budget:{token_limit:20000,cost_limit_micros:20000,currency:'CNY'}};
const item={task_id:'task-a',expected_task_version:2,role_key:'developer',model_execution_selection:selection};
it('recovers exact create and revision model allowances without broad secret exceptions',()=>{
 expect(expertRecovery({method:'POST',path:'/expert-team-plans',idempotency_key:'original-key',body:{project_id:'project-a',team_id:'team-a',expected_configuration_version:1,items:[item],idempotency_key:'original-key'}},'project-a')).toBe(true);
 expect(expertPlanCommandRecovery({method:'POST',path:'/expert-team-plans/plan-a/commands',idempotency_key:'original-key',body:{project_id:'project-a',expected_version:1,action:'revise',reason:'Reviewed model change',acknowledge:true,expected_configuration_version:1,items:[item],idempotency_key:'original-key'}},'project-a','plan-a')).toBe(true);
 for(const bad of [{...item,token:'private'},{...item,model_execution_selection:{...selection,api_key:'private'}},{...item,model_execution_selection:{...selection,budget:{token_limit:'private'}}},{...item,model_execution_selection:{...selection,parameters:{...selection.parameters,max_output_tokens:0}}}]) expect(validExpertRevisionItems([bad])).toBe(false);
});
