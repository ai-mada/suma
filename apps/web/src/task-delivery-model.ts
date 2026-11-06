import type {TaskDelivery,TaskDeliveryRun} from '../../../packages/contracts/src';
export type {TaskDelivery};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown):value is number=>integer(value)&&value>0;
const identifier=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const text=(value:unknown):value is string=>typeof value==='string';
const shortID=(value:unknown):value is string=>identifier(value)&&value.length<=64;
function recordedTime(value:unknown):boolean{
 if(!text(value)||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$/.test(value))return false;
 const stamp=Date.parse(value);
 return Number.isFinite(stamp)&&new Date(stamp).toISOString().replace('.000Z','Z')===value;
}
function repairAutomation(value:unknown){
 if(!object(value)||!identifier(value.run_id)||!object(value.origin))return undefined;
 const origin=value.origin;
 if(!shortID(origin.rule_id)||!shortID(origin.occurrence_id)||!text(origin.rule_name)||!origin.rule_name.trim()||origin.rule_name.length>200||
    !positive(origin.revision)||!recordedTime(origin.scheduled_at)||(origin.source!=='cron'&&origin.source!=='manual'&&origin.source!=='webhook'))return undefined;
 const pause=origin.failure_pause;
 const pauseID=object(pause)&&shortID(pause.occurrence_id)&&pause.occurrence_id!==origin.occurrence_id&&
  positive(pause.threshold)&&pause.threshold>=2&&pause.threshold<=10&&recordedTime(pause.observed_at)?pause.occurrence_id:undefined;
 return {runID:value.run_id,ruleID:origin.rule_id,triggerID:origin.occurrence_id,ruleName:origin.rule_name,revision:origin.revision,source:origin.source,pauseID};
}
function repairRegression(value:unknown){
 if(!object(value)||!shortID(value.plan_id)||!positive(value.plan_revision)||!text(value.plan_sha256)||!/^[a-f0-9]{64}$/.test(value.plan_sha256)||
    !Array.isArray(value.case_keys)||!value.case_keys.length||value.case_keys.length>100||!value.case_keys.every(shortID)||
    new Set(value.case_keys).size!==value.case_keys.length)return undefined;
 return {planID:value.plan_id,revision:value.plan_revision,count:value.case_keys.length};
}
// Supplementary links are checked independently of the delivery and its gates.
export function taskDeliveryRepair(value:unknown){
 if(!object(value)||!shortID(value.issue_id)||!positive(value.cycle)||(value.state!=='active'&&value.state!=='closed'&&value.state!=='reopened'))return undefined;
 return {issueID:value.issue_id,cycle:value.cycle,state:value.state,automation:repairAutomation(value.automation_source),regression:repairRegression(value.regression)};
}
function page(value:unknown,validate:(value:unknown)=>boolean){
 return object(value)&&integer(value.total)&&value.limit===20&&typeof value.complete==='boolean'&&Array.isArray(value.items)&&value.items.length===Math.min(value.total,20)&&value.complete===(value.total<=20)&&value.items.every(validate);
}
function node(value:unknown){return object(value)&&identifier(value.id)&&text(value.name)&&text(value.type)&&text(value.state)&&integer(value.attempt);}
function summary(value:unknown,tests=false):value is TaskDeliveryRun['nodes']{
 if(!page(value,item=>node(item)&&(!tests||object(item)&&item.type==='test'))||!object(value))return false;
 const counts=['waiting','running','succeeded','failed','skipped','reconciling','other'].map(key=>value[key]);
 return counts.every(integer)&&counts.reduce<number>((sum,count)=>sum+Number(count),0)===value.total&&new Set((value.items as Record<string,unknown>[]).map(item=>item.id)).size===(value.items as unknown[]).length;
}
export function validTaskDeliveryRun(value:unknown):value is TaskDeliveryRun{
 if(!object(value)||!identifier(value.run_id)||!positive(value.version)||!text(value.state)||!summary(value.nodes)||!summary(value.tests,true)||value.tests.total>value.nodes.total)return false;
 const origin=value.workbuddy_origin;
 if(origin!==undefined&&(!object(origin)||Object.keys(origin).length!==4||
  !text(origin.run_id)||!/^[A-Za-z0-9_-]{1,100}$/.test(origin.run_id)||origin.run_id===value.run_id||
  origin.node_id!=='workbuddy-cloud'||!positive(origin.attempt)||
  (origin.kind!=='repository_source'&&origin.kind!=='local_candidate')))return false;
 const trace=value.workbuddy_delivery;
 if(trace===undefined)return true;
 if(!object(trace)||Object.keys(trace).length!==3||trace.run_id!==value.run_id||!identifier(trace.node_id)||!positive(trace.attempt)||value.nodes.waiting===0)return false;
 const node=value.nodes.items.find(item=>item.id===trace.node_id)??value.tests.items.find(item=>item.id===trace.node_id);
 // The bounded summary can omit the original WorkBuddy node.
 return node?node.attempt===trace.attempt&&node.type==='external_workflow'&&node.state==='waiting':!value.nodes.complete;
}
/** This projection is display evidence, never an execution or review grant.
 * Counts describe only the bound run; capped item lists cannot stand in for a
 * complete nested workflow or a formal independent-evidence eligibility check. */
export function validTaskDelivery(value:unknown,project:string,task:string):value is TaskDelivery{
 if(!object(value)||value.project_id!==project||value.task_id!==task||!positive(value.task_version)||!text(value.task_status)||!['not_dispatched','current','changed','unknown'].includes(String(value.run_basis_state))||typeof value.formal_acceptance!=='boolean')return false;
 if(value.test_plan_id!==undefined&&!identifier(value.test_plan_id))return false;
 const run=value.run;
 if(run!==null&&!validTaskDeliveryRun(run))return false;
 if(value.run_basis_state==='current'&&run===null||value.run_basis_state==='not_dispatched'&&run!==null)return false;
 if(value.formal_acceptance&&(value.task_status!=='accepted'||value.run_basis_state!=='current'||!object(run)||run.state!=='succeeded'))return false;
 const plans=value.expert_plans,issues=value.issues;
 if(!object(plans)||plans.scope!=='current_run'||!page(plans,item=>object(item)&&identifier(item.plan_id)&&positive(item.version)&&positive(item.revision)&&identifier(item.team_id)&&text(item.team_name)&&text(item.state)&&text(item.item_state)))return false;
 if(!object(issues)||issues.scope!=='current_run'||!page(issues,item=>object(item)&&identifier(item.issue_id)&&text(item.title)&&positive(item.issue_version)&&text(item.issue_status)&&['execution_failure','review_rejected','test_case','package_test'].includes(String(item.source))&&text(item.source_review_id)&&(item.source!=='review_rejected'||identifier(item.source_review_id))&&text(item.requirement_id)&&(item.requirement_id===''?item.requirement_version===0:identifier(item.requirement_id)&&positive(item.requirement_version))))return false;
 return new Set((plans.items as Record<string,unknown>[]).map(item=>item.plan_id)).size===(plans.items as unknown[]).length&&new Set((issues.items as Record<string,unknown>[]).map(item=>item.issue_id)).size===(issues.items as unknown[]).length;
}
