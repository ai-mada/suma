import type {WriteRecoveryRequest} from './write-action';
import type {TaskDependencyGraph,TaskDependencyNode,TaskDependencyChange,TaskDependencyChangeResult} from '../../../packages/contracts/src';
export type {TaskDependencyGraph,TaskDependencyNode,TaskDependencyChange,TaskDependencyChangeResult};
export type TaskDependencyDraft=Record<string,string[]>;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
const integer=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>=0;
const ids=(value:unknown):value is string[]=>Array.isArray(value)&&value.length<=1000&&value.every(id)&&new Set(value).size===value.length;
const same=(left:string[],right:string[])=>{if(left.length!==right.length)return false;const a=[...left].sort(),b=[...right].sort();return a.every((value,index)=>value===b[index]);};
const closed=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).sort().join(',')===[...keys].sort().join(',');

/** A graph is editable only when every task and edge belongs to one complete
 * authorized snapshot. The cap response is an explicit non-editable outcome. */
export function validTaskDependencyGraph(value:unknown,project:string):value is TaskDependencyGraph{
 if(!object(value)||value.project_id!==project||value.limit!==1000||!integer(value.total)||typeof value.complete!=='boolean'||!Array.isArray(value.tasks))return false;
 if(!value.complete)return value.total>1000&&value.tasks.length===0;
 if(value.total>1000||value.tasks.length!==value.total)return false;
 if(!value.tasks.every(task=>object(task)&&id(task.task_id)&&integer(task.version)&&task.version>0&&typeof task.title==='string'&&typeof task.status==='string'&&ids(task.depends_on)))return false;
 const graph=value as unknown as TaskDependencyGraph;
 return new Set(graph.tasks.map(task=>task.task_id)).size===graph.tasks.length&&!taskDependencyProblem(graph,taskDependencyDraft(graph));
}
export function taskDependencyDraft(graph:TaskDependencyGraph):TaskDependencyDraft{
 return Object.fromEntries(graph.tasks.map(task=>[task.task_id,[...task.depends_on].sort()]));
}
/** Kahn validation is only an immediate editing aid. The control plane still
 * validates the entire current graph under its write transaction and lock. */
export function taskDependencyProblem(graph:TaskDependencyGraph,draft:TaskDependencyDraft):'missing'|'self'|'cycle'|undefined{
 const taskIDs=new Set(graph.tasks.map(task=>task.task_id));
 if(Object.keys(draft).length!==taskIDs.size||Object.keys(draft).some(key=>!taskIDs.has(key)))return 'missing';
 const indegree=new Map<string,number>(),outgoing=new Map([...taskIDs].map(key=>[key,[] as string[]]));
 for(const key of taskIDs){
  const dependencies=draft[key];if(!ids(dependencies)||dependencies.some(key=>!taskIDs.has(key)))return 'missing';
  if(dependencies.includes(key))return 'self';indegree.set(key,dependencies.length);
  for(const source of dependencies)outgoing.get(source)!.push(key);
 }
 const queue=[...taskIDs].filter(key=>indegree.get(key)===0);let visited=0;
 for(let position=0;position<queue.length;position++){
  const key=queue[position];visited++;
  for(const target of outgoing.get(key)!){const remaining=indegree.get(target)!-1;indegree.set(target,remaining);if(!remaining)queue.push(target);}
 }
 return visited===taskIDs.size?undefined:'cycle';
}
export function taskDependencyChanges(graph:TaskDependencyGraph,draft:TaskDependencyDraft):TaskDependencyChange[]{
 return graph.tasks.filter(task=>!same(task.depends_on,draft[task.task_id]||[])).map(task=>({task_id:task.task_id,expected_version:task.version,depends_on:[...(draft[task.task_id]||[])].sort()})).sort((a,b)=>a.task_id.localeCompare(b.task_id,'en'));
}
export function validTaskDependencyRecovery(request:WriteRecoveryRequest,project:string){
 const body=request.body;
 return request.path==='/task-dependencies'&&request.method==='POST'&&closed(body,['project_id','changes','idempotency_key'])&&body.project_id===project&&body.idempotency_key===request.idempotency_key&&Array.isArray(body.changes)&&body.changes.length>0&&body.changes.length<=100&&body.changes.every(change=>object(change)&&closed(change,['task_id','expected_version','depends_on'])&&id(change.task_id)&&integer(change.expected_version)&&change.expected_version>0&&ids(change.depends_on)&&!change.depends_on.includes(change.task_id))&&new Set(body.changes.map(change=>change.task_id)).size===body.changes.length;
}
export function taskDependencyReceipt(value:unknown,request:WriteRecoveryRequest,project:string):value is TaskDependencyChangeResult{
 if(!validTaskDependencyRecovery(request,project)||!object(value)||value.project_id!==project||value.operation_id!==request.idempotency_key||!Array.isArray(value.changes))return false;
 const expected=request.body.changes as TaskDependencyChange[],rows=value.changes;
 return rows.length===expected.length&&new Set(rows.map(row=>object(row)?row.task_id:undefined)).size===rows.length&&rows.every(row=>{
  if(!object(row)||!id(row.task_id)||!ids(row.depends_on))return false;
  const original=expected.find(change=>change.task_id===row.task_id);
  return !!original&&integer(row.previous_version)&&integer(row.version)&&row.previous_version===original.expected_version&&row.version===original.expected_version+1&&same(row.depends_on,original.depends_on);
 });
}
export function taskDependencyReceiptCurrent(result:TaskDependencyChangeResult,graph:TaskDependencyGraph){
 return graph.complete&&result.project_id===graph.project_id&&result.changes.every(change=>{const task=graph.tasks.find(task=>task.task_id===change.task_id);return !!task&&task.version===change.version&&same(task.depends_on,change.depends_on);});
}
/** Rebase only locally edited tasks: copying every old edge would overwrite
 * another collaborator's changes to tasks which this user never edited. */
export function taskDependencyRebase(base:TaskDependencyGraph,draft:TaskDependencyDraft,current:TaskDependencyGraph):TaskDependencyDraft|undefined{
 const next=taskDependencyDraft(current),changes=taskDependencyChanges(base,draft);
 if(changes.some(change=>!Object.hasOwn(next,change.task_id)))return;
 for(const change of changes)next[change.task_id]=[...change.depends_on];
 return next;
}
