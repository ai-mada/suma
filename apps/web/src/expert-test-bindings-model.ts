import type {ExpertTeamPlanItem,ExpertTeamRole,TaskDispatchTestSelection} from '../../../packages/contracts/src';
import {validTestBindingsPlan} from './task-dispatch-test-bindings-model';

export type ExpertTestTask={taskID:string;title:string;version:number;role:ExpertTeamRole;item?:ExpertTeamPlanItem};
export type ExpertTestChoices={scope:string;ready:boolean;selections:Record<string,TaskDispatchTestSelection[]|undefined>};
export type ExpertTestMode='retain'|'adjust'|'definition';
export const expertTestRoleBasis=(role:ExpertTeamRole)=>JSON.stringify([role.key,role.workflow_id,role.workflow_version,role.agent_node_id,role.tool_profile_id,role.tool_profile_version,role.tool_profile_sha256]);
export const expertTestContext=(project:string,tasks:ExpertTestTask[])=>JSON.stringify([project,tasks.map(task=>[task.taskID,expertTestRoleBasis(task.role),task.item?.role_key,task.item?.plan.workflow_id,task.item?.plan.workflow_version,task.item?.plan.agent_node_id,task.item?.plan.tool_profile_id,task.item?.plan.tool_profile_version,task.item?.plan.tool_profile_sha256,task.item?.plan.test_bindings,task.item?.run_id,task.item?.dispatch_plan_id])]);
export const expertTestScope=(project:string,tasks:ExpertTestTask[])=>JSON.stringify([expertTestContext(project,tasks),tasks.map(task=>[task.taskID,task.version])]);
/** Retention is an intent against the original saved plan, not a live tool
 * qualification. Current authority, exact profiles and route remain server gates. */
export function expertTestCanRetain(task:ExpertTestTask):boolean{
 const item=task.item,bindings=item?.plan.test_bindings;
 if(!item||!bindings?.length)return true;
 const p=item.plan,r=task.role;
 return validTestBindingsPlan(p)&&item.role_key===r.key&&p.workflow_id===r.workflow_id&&p.workflow_version===r.workflow_version&&p.agent_node_id===r.agent_node_id&&p.tool_profile_id===r.tool_profile_id&&p.tool_profile_version===r.tool_profile_version&&p.tool_profile_sha256===r.tool_profile_sha256;
}
/** Preserve omission versus an explicit empty array. Only the current parent
 * scope may call this helper after comparing choices.scope and readiness. */
export function expertTestPayload(choices:ExpertTestChoices|undefined,taskID:string):{test_bindings?:TaskDispatchTestSelection[]}{
 const values=choices?.selections[taskID];
 return values===undefined?{}:{test_bindings:values.map(value=>({...value}))};
}
