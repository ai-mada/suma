import {validExpertModelItems} from './expert-model-input';
import type {ExpertTeamPlan,ExpertTeamPlanItem,ExpertTeamPlanItemRequest} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {expertBlocker,expertID,expertInteger,expertText,validExpertConfiguration} from './expert-team-model';
import {tr} from './i18n';
import {codexBindingPresent,sameCodexBinding} from './codex-execution-model';
import {bindingPresent} from './model-execution-model';
import {sameModelBinding} from './model-execution-input';

export const expertPlanStates=['ready','active','paused','stopped','completed'];
export const expertItemStates=['waiting_dependencies','ready','queued','executing','awaiting_review','accepted','blocked','stopped'];
export const expertPlanActions=['start','pause','resume','stop','revise'];
export const expertPlanCursor=(value:unknown):value is string=>typeof value==='string'&&value.length<=4096&&!/[\x00-\x1f\x7f]/.test(value);
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const runID=(value:unknown)=>typeof value==='string'&&/^[A-Za-z0-9_.-]{1,150}$/.test(value);
export const expertUnstarted=(item:ExpertTeamPlanItem)=>!item.run_id&&!item.dispatch_plan_id;
export function expertTopology(items:ExpertTeamPlanItem[]){
 const remaining=new Map(items.map(item=>[item.task_id,item])),result:ExpertTeamPlanItem[]=[];
 while(remaining.size){const next=[...remaining.values()].filter(item=>item.depends_on.every(id=>!remaining.has(id)));if(!next.length)return;for(const item of next){remaining.delete(item.task_id);result.push(item);}}
 return result;
}
export function validOrchestratedPlan(value:ExpertTeamPlan,project:string,id?:string){
 if(!value||value.project_id!==project||!expertID(value.id)||id&&value.id!==id||!expertInteger(value.version)||value.version<1||!expertInteger(value.revision)||value.revision<1||!hash(value.graph_sha256)||!expertInteger(value.activation_generation)||!expertPlanStates.includes(value.state)||!validExpertConfiguration(value.configuration,project,value.team_id)||!Array.isArray(value.items)||!value.items.length||value.items.length>16||!Array.isArray(value.blockers)||!value.blockers.every(reason=>typeof reason==='string')||typeof value.can_start!=='boolean'||typeof value.valid!=='boolean'||!Array.isArray(value.allowed_actions)||!value.allowed_actions.every(action=>expertPlanActions.includes(action))||new Set(value.allowed_actions).size!==value.allowed_actions.length)return false;
 const selected=new Set(value.items.map(item=>item.task_id));if(selected.size!==value.items.length)return false;
 if(!value.items.every(item=>expertID(item.task_id)&&expertInteger(item.task_version)&&item.task_version>0&&expertInteger(item.source_revision)&&item.source_revision>0&&hash(item.task_basis_sha256)&&expertItemStates.includes(item.state)&&Array.isArray(item.blockers)&&item.blockers.every(reason=>typeof reason==='string')&&Array.isArray(item.depends_on)&&item.depends_on.every(id=>expertID(id)&&id!==item.task_id)&&new Set(item.depends_on).size===item.depends_on.length&&Array.isArray(item.dependencies)&&item.dependencies.length===item.depends_on.length&&new Set(item.dependencies.map(fact=>fact.task_id)).size===item.depends_on.length&&item.dependencies.every(fact=>item.depends_on.includes(fact.task_id)&&typeof fact.accepted==='boolean'&&fact.selected===selected.has(fact.task_id)&&expertInteger(fact.task_version)&&fact.task_version>0&&(!fact.run_id||runID(fact.run_id)))&&!!item.plan&&item.plan.project_id===project&&item.plan.task_id===item.task_id&&!!item.plan.context?.task&&typeof item.role_title==='string'&&typeof item.resource_name==='string'&&typeof item.run_state==='string'&&(!item.run_id||runID(item.run_id))&&(!item.dispatch_plan_id||expertID(item.dispatch_plan_id))&&(!!item.run_id===!!item.dispatch_plan_id)&&(item.run_id||!item.plan.id&&!item.plan.run_id)))return false;
 if(!value.items.every(item=>{const p=item.plan,native=p.model_execution,codex=p.codex_execution;if(native&&codex)return false;if(!native&&!codex)return true;const node=p.workflow?.nodes.find(node=>node.id===p.agent_node_id);if(!node?.config)return false;return codex?codexBindingPresent(codex)&&!node.config.model_execution&&sameCodexBinding(node.config.codex_execution,codex):bindingPresent(native)&&!node.config.codex_execution&&sameModelBinding(node.config.model_execution,native);}))return false;
 const p=value.progress;if(!p||p.total!==value.items.length)return false;const counts=[p.waiting,p.ready,p.executing,p.awaiting_review,p.accepted,p.blocked,p.stopped];return counts.every(count=>expertInteger(count)&&count<=p.total)&&counts.reduce((sum,count)=>sum+count,0)===p.total&&!!expertTopology(value.items);
}
export const validExpertRevisionItems = validExpertModelItems;
export function expertPlanCommandRecovery(request:WriteRecoveryRequest,project:string,plan:string){const b=request.body;if(request.method!=='POST'||request.path!=='/expert-team-plans/'+plan+'/commands'||b.project_id!==project||!Object.keys(b).every(key=>['project_id','expected_version','action','reason','acknowledge','expected_configuration_version','items','idempotency_key'].includes(key))||!expertInteger(b.expected_version)||b.expected_version<1||!['pause','resume','stop','revise'].includes(String(b.action))||!expertText(b.reason)||b.acknowledge!==true)return false;return b.action==='revise'?expertInteger(b.expected_configuration_version)&&b.expected_configuration_version>0&&validExpertRevisionItems(b.items):!Object.hasOwn(b,'items')&&!Object.hasOwn(b,'expected_configuration_version');}
const reasons:Record<string,string>={
 'Expert team task dependencies are invalid':'graph','Expert team pending task source changed; explicitly revise the plan':'source','Expert team task is waiting for formal predecessor acceptance':'waiting','Expert team predecessor acceptance belongs to a different run':'wrong_run','Expert team plan is stopped; active runs retain their own controls':'stopped','Expert team plan is paused; active runs continue':'paused','Expert team activation credential is no longer authorized; explicitly resume':'authority','Expert team revision may only change unstarted tasks':'revision','Expert team run requires explicit recovery or review; no automatic retry':'run_blocked'
};
export const expertPlanReason=(value:string)=>Object.hasOwn(reasons,value)?tr('expert-team-orchestration.reason.'+reasons[value]):expertBlocker(value);
