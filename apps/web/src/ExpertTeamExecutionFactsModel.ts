import type {ExpertTeamExecutionFact,ExpertTeamExecutionNodeFact} from '../../../packages/contracts/src';
/** Read-only projection of the original run; never an execution permission. */
export type TeamExecutionNode=ExpertTeamExecutionNodeFact;
export type TeamExecutionFact=ExpertTeamExecutionFact;
const locator=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_.-]{1,150}$/.test(value);
const date=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d{2}-\d{2}T/.test(value)&&Number.isFinite(Date.parse(value));
const integer=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>=0;
export function readTeamExecution(value:unknown,runID:string):TeamExecutionFact|undefined{
 if(!value||typeof value!=='object'||Array.isArray(value))return;
 const fact=value as TeamExecutionFact;
 if(!locator(runID)||fact.run_id!==runID||!date(fact.observed_at)||!integer(fact.nodes_total)||!Array.isArray(fact.nodes)||fact.nodes.length!==Math.min(24,fact.nodes_total)||typeof fact.truncated!=='boolean'||fact.truncated!==(fact.nodes_total>fact.nodes.length))return;
 const ids=new Set<string>();
 for(const node of fact.nodes){
  if(!node||!locator(node.node_id)||ids.has(node.node_id)||!integer(node.attempt)||![node.name,node.type,node.state,node.reason,node.worker_id,node.ready_at].every(value=>typeof value==='string')||node.ready_at!==''&&!date(node.ready_at))return;
  ids.add(node.node_id);
 }
 return fact;
}
export const executionStates=['pending','ready','running','waiting','reconciling','cancelling','paused','failed','blocked','succeeded','skipped','cancelled'] as const;
export function executionFocus(nodes:TeamExecutionNode[]){
 // The server selects and orders the bounded projection from the complete run.
 return nodes.slice(0,3);
}
export function executionNodeHref(project:string,run:string,node?:TeamExecutionNode){
 return '?'+new URLSearchParams({project,view:'development',sub:'runs',run,...(node?{node:node.node_id,...(node.attempt>0?{attempt:String(node.attempt)}:{})}:{})});
}
export function executionNext(state:string){
 if(['failed','blocked'].includes(state))return 'failure';
 if(state==='reconciling')return 'reconcile';
 if(state==='running')return 'output';
 if(['pending','ready','waiting','cancelling','paused'].includes(state))return 'waiting';
 return 'record';
}
