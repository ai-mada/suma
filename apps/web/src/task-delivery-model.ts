import type {TaskDelivery} from '../../../packages/contracts/src';
export type {TaskDelivery};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown)=>integer(value)&&value>0;
const identifier=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const text=(value:unknown):value is string=>typeof value==='string';
function page(value:unknown,validate:(value:unknown)=>boolean){
 return object(value)&&integer(value.total)&&value.limit===20&&typeof value.complete==='boolean'&&Array.isArray(value.items)&&value.items.length===Math.min(value.total,20)&&value.complete===(value.total<=20)&&value.items.every(validate);
}
function node(value:unknown){return object(value)&&identifier(value.id)&&text(value.name)&&text(value.type)&&text(value.state)&&integer(value.attempt);}
function summary(value:unknown,tests=false){
 if(!page(value,item=>node(item)&&(!tests||object(item)&&item.type==='test'))||!object(value))return false;
 const counts=['waiting','running','succeeded','failed','skipped','reconciling','other'].map(key=>value[key]);
 return counts.every(integer)&&counts.reduce<number>((sum,count)=>sum+Number(count),0)===value.total&&new Set((value.items as Record<string,unknown>[]).map(item=>item.id)).size===(value.items as unknown[]).length;
}
/** This projection is display evidence, never an execution or review grant.
 * Counts describe only the bound run; capped item lists cannot stand in for a
 * complete nested workflow or a formal independent-evidence eligibility check. */
export function validTaskDelivery(value:unknown,project:string,task:string):value is TaskDelivery{
 if(!object(value)||value.project_id!==project||value.task_id!==task||!positive(value.task_version)||!text(value.task_status)||!['not_dispatched','current','changed','unknown'].includes(String(value.run_basis_state))||typeof value.formal_acceptance!=='boolean')return false;
 const run=value.run;
 if(run!==null&&(!object(run)||!identifier(run.run_id)||!positive(run.version)||!text(run.state)||!summary(run.nodes)||!summary(run.tests,true)))return false;
 if(object(run)&&Number((run.tests as Record<string,unknown>).total)>Number((run.nodes as Record<string,unknown>).total))return false;
 if(value.run_basis_state==='current'&&run===null||value.run_basis_state==='not_dispatched'&&run!==null)return false;
 if(value.formal_acceptance&&(value.task_status!=='accepted'||value.run_basis_state!=='current'||!object(run)||run.state!=='succeeded'))return false;
 const plans=value.expert_plans,issues=value.issues;
 if(!object(plans)||plans.scope!=='current_run'||!page(plans,item=>object(item)&&identifier(item.plan_id)&&positive(item.version)&&positive(item.revision)&&identifier(item.team_id)&&text(item.team_name)&&text(item.state)&&text(item.item_state)))return false;
 if(!object(issues)||issues.scope!=='current_run'||!page(issues,item=>object(item)&&identifier(item.issue_id)&&text(item.title)&&positive(item.issue_version)&&text(item.issue_status)&&['execution_failure','review_rejected'].includes(String(item.source))&&text(item.source_review_id)&&(item.source==='execution_failure'||identifier(item.source_review_id))&&text(item.requirement_id)&&(item.requirement_id===''?item.requirement_version===0:identifier(item.requirement_id)&&positive(item.requirement_version))))return false;
 return new Set((plans.items as Record<string,unknown>[]).map(item=>item.plan_id)).size===(plans.items as unknown[]).length&&new Set((issues.items as Record<string,unknown>[]).map(item=>item.issue_id)).size===(issues.items as unknown[]).length;
}
