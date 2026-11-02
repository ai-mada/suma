import {uniqueJSONMembers} from './unique-json-members';
import type {WorkspaceOperationEvent,WorkspaceOperationCoverage,Event as RunEvent} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
import type {AttemptToolScope} from './attempt-tool-history';
export type WorkspaceOperationFact={operation:WorkspaceOperationEvent;serverSequence:number;serverAt:string};
export type WorkspaceOperationGroup={id:string;facts:WorkspaceOperationFact[];latest:WorkspaceOperationFact;inconsistent:boolean;missingStart:boolean;pendingEnd:boolean};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown,min=0,max=Number.MAX_SAFE_INTEGER):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max;
const text=(value:unknown,max=1024):value is string=>typeof value==='string'&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const identity=(value:unknown):value is string=>text(value,128)&&/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/.test(value);
const time=(value:unknown):value is string=>text(value,35)&&value.endsWith('Z')&&Number.isFinite(Date.parse(value));
const canonical=(value:unknown):string=>JSON.stringify(value,(_key,item:unknown)=>object(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
export const workspaceActions=['list','read','write','edit','remove','check','merge_status','merge_continue','seal'] as const;
export function safeWorkspacePath(value:unknown):value is string{return text(value)&&value!==''&&!value.startsWith('/')&&!/[\\:]/.test(value)&&value.split('/').every(part=>!!part&&part!=='.'&&part!=='..'&&part.toLowerCase()!=='.git');}
/** Closed host facts are distinct from arbitrary SDK tool messages. There is
 * no argument, source text, command, output or credential field in this view. */
export function validWorkspaceOperation(value:unknown):value is WorkspaceOperationEvent{
 if(!object(value))return false;
 const required=['protocol','source','schema_version','event_id','sequence','operation_id','revision','action','phase','generation_before','generation_after','started_at','recorded_at','request_sha256','policy_sha256','profile_sha256'],optional=['effect_operation_id','completed_at','path','check_id','result_sha256','exit_code','stopped','error_code','replayed'];
 if(!required.every(key=>Object.hasOwn(value,key))||Object.keys(value).some(key=>!required.includes(key)&&!optional.includes(key))||value.protocol!=='suma-workspace-operation-v1'||value.source!=='go_workspace_host'||value.schema_version!==1||!hash(value.event_id)||!integer(value.sequence,1,4096)||!identity(value.operation_id)||!workspaceActions.includes(value.action as typeof workspaceActions[number])||!hash(value.request_sha256)||!hash(value.policy_sha256)||!hash(value.profile_sha256)||!integer(value.generation_before,0,2048)||!integer(value.generation_after,value.generation_before,2048)||!time(value.started_at)||!time(value.recorded_at)||Date.parse(value.recorded_at)<Date.parse(value.started_at))return false;
 for(const field of ['effect_operation_id','path','check_id','result_sha256','completed_at','error_code'])if(value[field]!==undefined&&!text(value[field],field==='path'?1024:256))return false;
 if(value.effect_operation_id&&!identity(value.effect_operation_id)||value.path&&!safeWorkspacePath(value.path)||value.check_id&&!identity(value.check_id)||value.result_sha256&&!hash(value.result_sha256)||value.error_code&&(typeof value.error_code!=='string'||!/^[a-z][a-z0-9_]{0,79}$/.test(value.error_code))||value.exit_code!==undefined&&!integer(value.exit_code,-2147483648,2147483647)||value.stopped!==undefined&&typeof value.stopped!=='boolean'||value.replayed!==undefined&&typeof value.replayed!=='boolean'||value.replayed===true&&!['completed','failed'].includes(String(value.phase)))return false;
 if(new TextEncoder().encode(JSON.stringify(value)).length>4096)return false;
 if(value.phase==='started')return value.revision===1&&!value.completed_at&&value.generation_before===value.generation_after&&value.exit_code===undefined&&value.stopped===undefined&&!value.error_code&&!value.replayed&&!value.result_sha256;
 if(value.phase==='unknown')return value.revision===2&&!value.completed_at&&!!value.error_code;
 return ['completed','failed'].includes(String(value.phase))&&value.revision===2&&time(value.completed_at)&&Date.parse(value.completed_at)>=Date.parse(value.started_at)&&Date.parse(value.recorded_at)>=Date.parse(value.completed_at)&&(value.phase!=='failed'||!!value.error_code);
}
export function savedHostOperationMatchesSource(event:Pick<RunEvent,'data'|'raw_data'>):boolean{
 if(typeof event.raw_data!=='string')return false;
 try{const source=JSON.parse(event.raw_data);return object(source)&&!Object.hasOwn(source,'tool_event')&&Object.hasOwn(source,'host_operation')&&['attempt','fence','worker_id'].every(key=>source[key]===event.data[key])&&uniqueJSONMembers(event.raw_data)&&validWorkspaceOperation(source.host_operation)&&validWorkspaceOperation(event.data.host_operation)&&canonical(source.host_operation)===canonical(event.data.host_operation);}catch{return false;}
}
/** Source filtering uses the exact attempt envelope; no SDK item ID or text
 * can be promoted into a host invocation. Raw mismatches stay raw-only. */
export function workspaceOperationsForAttempt(events:readonly unknown[],scope:AttemptToolScope,profileSHA?:string):{scopeKey:string;facts:WorkspaceOperationFact[];invalid:number}{
 const facts:WorkspaceOperationFact[]=[];let invalid=0;
 if(!scope.project_id||!scope.run_id||!scope.node_id||!scope.worker_id||!integer(scope.attempt,1)||!integer(scope.fence,1))return {scopeKey:JSON.stringify(scope),facts,invalid};
 for(const value of events){
  if(!object(value)||value.type!=='tool_progress'||value.run_id!==scope.run_id||value.node_id!==scope.node_id||!object(value.data)||value.data.attempt!==scope.attempt||value.data.fence!==scope.fence||value.data.worker_id!==scope.worker_id||!Object.hasOwn(value.data,'host_operation'))continue;
  const event=value.data.host_operation;
  if(!integer(value.sequence,1)||!time(value.at)||Object.hasOwn(value.data,'tool_event')||!validWorkspaceOperation(event)||profileSHA!==undefined&&event.profile_sha256!==profileSHA||Object.hasOwn(value,'raw_data')&&!savedHostOperationMatchesSource(value as unknown as RunEvent)){invalid++;continue;}
  facts.push({operation:event,serverSequence:value.sequence,serverAt:value.at});
 }
 return {scopeKey:JSON.stringify(scope),facts,invalid};
}
/** Revisions are local invocation transitions, not source generations or
 * server-arrival order. Conflicting same-revision facts fail closed. */
export function groupWorkspaceOperations(facts:readonly WorkspaceOperationFact[]):WorkspaceOperationGroup[]{
 const groups=new Map<string,WorkspaceOperationFact[]>();for(const fact of facts){const key=fact.operation.operation_id;groups.set(key,[...(groups.get(key)||[]),fact]);}
 return [...groups.entries()].map(([id,items])=>{
  const revisions=new Map<number,WorkspaceOperationFact>(),unique:WorkspaceOperationFact[]=[];let inconsistent=false;
  for(const fact of items){const old=revisions.get(fact.operation.revision);if(old){if(canonical(old.operation)!==canonical(fact.operation))inconsistent=true;}else{revisions.set(fact.operation.revision,fact);unique.push(fact);}}
  unique.sort((a,b)=>a.operation.revision-b.operation.revision);const first=unique[0],latest=unique.at(-1)!;
  if(first&&latest){for(const field of ['action','operation_id','request_sha256','policy_sha256','profile_sha256','generation_before','started_at'] as const)if(canonical(first.operation[field])!==canonical(latest.operation[field]))inconsistent=true;for(const field of ['effect_operation_id','path','check_id'] as const)if(first.operation[field]&&latest.operation[field]&&first.operation[field]!==latest.operation[field])inconsistent=true;if(first.operation.revision!==latest.operation.revision&&first.operation.sequence>=latest.operation.sequence)inconsistent=true;}
  return {id,facts:inconsistent?items:unique,latest,inconsistent,missingStart:!revisions.has(1),pendingEnd:!revisions.has(2)};
 }).sort((a,b)=>Math.min(...a.facts.map(item=>item.serverSequence))-Math.min(...b.facts.map(item=>item.serverSequence)));
}
export function workspaceOperationFailed(group:WorkspaceOperationGroup){const op=group.latest.operation;return !group.inconsistent&&(op.phase==='failed'||op.action==='check'&&(op.exit_code!==undefined&&op.exit_code!==0||op.stopped===true));}

export function validWorkspaceOperationCoverage(value:unknown):value is WorkspaceOperationCoverage{
 if(!object(value)||Object.keys(value).some(key=>!['invocation_count','record_count','pending_count','max_invocations','max_records','state','reason'].includes(key))||value.max_invocations!==2048||value.max_records!==4096||!integer(value.invocation_count,0,2048)||!integer(value.record_count,value.invocation_count,4096)||!integer(value.pending_count,0,value.invocation_count)||value.record_count!==2*value.invocation_count-value.pending_count)return false;
 return value.state==='complete'?(value.reason===undefined||value.reason===''):['partial','unknown'].includes(String(value.state))&&typeof value.reason==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(value.reason);
}
