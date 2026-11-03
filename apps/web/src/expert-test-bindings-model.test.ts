import {expect,it} from 'vitest';
import type {ExpertTeamPlanItem,ExpertTeamRole} from '../../../packages/contracts/src';
import {validExpertModelItems} from './expert-model-input';
import {expertRecovery} from './expert-team-model';
import {expertPlanCommandRecovery} from './expert-team-plan-model';
import {expertTestCanRetain,expertTestContext,expertTestPayload,expertTestScope,type ExpertTestTask} from './expert-test-bindings-model';

it('T223 distinguishes keep, explicit definition and replacement intents and never retains tests across a different role source',()=>{
 const sha='a'.repeat(64),selection={node_id:'verify',tool_profile_id:'tester',expected_tool_version:3},request={task_id:'task-a',expected_task_version:4,role_key:'developer'};
 for(const value of [request,{...request,test_bindings:[]},{...request,test_bindings:[selection]}])expect(validExpertModelItems([value])).toBe(true);
 for(const test_bindings of [null,undefined,[selection,selection],[{...selection,secret:'value'}],[{...selection,expected_tool_version:0}]])expect(validExpertModelItems([{...request,test_bindings}])).toBe(false);
 const create={method:'POST',path:'/expert-team-plans',idempotency_key:'original-key',body:{project_id:'project',team_id:'team',expected_configuration_version:2,items:[{...request,test_bindings:[]}],idempotency_key:'original-key'}};
 expect(expertRecovery(create,'project')).toBe(true);expect(expertPlanCommandRecovery({...create,path:'/expert-team-plans/batch/commands',body:{project_id:'project',expected_version:2,action:'revise',reason:'Reviewed tests',acknowledge:true,expected_configuration_version:2,items:[{...request,test_bindings:[selection]}],idempotency_key:'original-key'}},'project','batch')).toBe(true);
 const role={key:'developer',workflow_id:'flow',workflow_version:2,agent_node_id:'code',tool_profile_id:'tool',tool_profile_version:3,tool_profile_sha256:sha} as ExpertTeamRole;
 const frozen={node_id:'verify',tool_profile_id:'tester',tool_profile_version:3,tool_profile_sha256:sha,worker_id:'independent-worker',command_id:'test-command'};
 const item={role_key:'developer',plan:{workflow_id:'flow',workflow_version:2,agent_node_id:'code',tool_profile_id:'tool',tool_profile_version:3,tool_profile_sha256:sha,test_bindings:[frozen],workflow:{nodes:[{id:'code',type:'agent'},{id:'verify',type:'test',command:['test-command'],config:{candidate_from:'code',tool_profile_id:'tester',tool_profile_sha256:sha}}]}}} as unknown as ExpertTeamPlanItem;
 const task:ExpertTestTask={taskID:'task-a',title:'Task A',version:4,role,item};
 expect(expertTestCanRetain(task)).toBe(true);expect(expertTestCanRetain({...task,version:5})).toBe(true);
 for(const patch of [{key:'other'},{workflow_version:3},{agent_node_id:'other'},{tool_profile_version:4},{tool_profile_sha256:'b'.repeat(64)}])expect(expertTestCanRetain({...task,role:{...role,...patch}})).toBe(false);
 const scope=expertTestScope('project',[task]);expect(expertTestContext('project',[task])).toBe(expertTestContext('project',[{...task,version:5}]));expect(scope).not.toBe(expertTestScope('project',[{...task,version:5}]));
 expect(Object.hasOwn(expertTestPayload({scope,ready:true,selections:{'task-a':undefined}},'task-a'),'test_bindings')).toBe(false);
 expect(expertTestPayload({scope,ready:true,selections:{'task-a':[]}},'task-a')).toEqual({test_bindings:[]});
 expect(expertTestPayload({scope,ready:true,selections:{'task-a':[selection]}},'task-a')).toEqual({test_bindings:[selection]});
});
