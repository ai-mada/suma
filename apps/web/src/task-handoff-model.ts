import type {TaskHandoff,TaskHandoffPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const handoffID=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const optionalID=(value:unknown)=>value===''||handoffID(value);
const natural=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0&&value<=2147483647;
export const handoffVersion=(value:unknown):value is number=>natural(value)&&value>0;
const instant=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
const strings=(value:unknown):value is string[]=>Array.isArray(value)&&value.every(item=>typeof item==='string');
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const draftText=(value:unknown):value is string=>typeof value==='string'&&value.length<=4000&&!/[\0\uD800-\uDFFF]/u.test(value);
export const validHandoffText=(value:unknown):value is string=>draftText(value)&&!!value.trim()&&new TextEncoder().encode(value).length<=4000;
const states=['pending','accepted','returned','withdrawn'];
const actions=['accept','return','withdraw'];
export type HandoffDecision={id:string;version:number;action:'accept'|'return'|'withdraw';reason:string};
export type HandoffDraft={recipient:string;team:string;note:string;task_version?:number;decision?:HandoffDecision};
export function emptyHandoffDraft():HandoffDraft{return {recipient:'',team:'',note:''};}
export function handoffPath(project:string,task:string,id:string){return '/task-handoffs/'+encodeURIComponent(id)+'?'+new URLSearchParams({project_id:project,task_id:task});}
export function handoffsPath(project:string,task:string){return '/task-handoffs?'+new URLSearchParams({project_id:project,task_id:task,limit:'20'});}

/** Validate the current public DTO before any source facts become actionable. */
export function validHandoff(value:unknown,project:string,task:string,id?:string):value is TaskHandoff{
 if(!object(value)||!handoffID(value.id)||id!==undefined&&value.id!==id||value.project_id!==project||value.task_id!==task||!handoffVersion(value.version)||!states.includes(String(value.state))||!handoffID(value.created_by)||!instant(value.created_at)||!instant(value.updated_at)||!handoffID(value.to_resource_id)||!handoffVersion(value.to_resource_version)||!handoffID(value.to_user_id)||typeof value.to_name!=='string'||!optionalID(value.to_team_id)||typeof value.to_team_name!=='string'||!natural(value.to_team_version)||!natural(value.to_membership_version)||!validHandoffText(value.note)||!optionalID(value.decision_by)||!(value.decision_at===''||instant(value.decision_at))||!draftText(value.decision_reason)||!natural(value.result_task_version)||typeof value.basis_current!=='boolean'||!strings(value.blockers)||!strings(value.allowed_actions)||new Set(value.allowed_actions).size!==value.allowed_actions.length||value.allowed_actions.some(action=>!actions.includes(action))||value.state!=='pending'&&value.allowed_actions.length>0)return false;
 const basis=value.basis;
 return object(basis)&&exact(basis,['task_version','task_basis_sha256','requirement_id','requirement_version','from_owner_id','source_run_id','source_run_version','source_run_state','delivery_sha256'])&&handoffVersion(basis.task_version)&&typeof basis.task_basis_sha256==='string'&&/^[a-f0-9]{64}$/.test(basis.task_basis_sha256)&&optionalID(basis.requirement_id)&&natural(basis.requirement_version)&&optionalID(basis.from_owner_id)&&typeof basis.source_run_id==='string'&&(basis.source_run_id===''||/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(basis.source_run_id))&&natural(basis.source_run_version)&&typeof basis.source_run_state==='string'&&typeof basis.delivery_sha256==='string'&&(basis.delivery_sha256===''||/^[a-f0-9]{64}$/.test(basis.delivery_sha256));
}
export function validHandoffPage(value:unknown,project:string,task:string):value is TaskHandoffPage{
 if(!object(value)||!Array.isArray(value.items)||value.items.length>20||!natural(value.total)||value.items.length>value.total||!(value.next_cursor===''||handoffID(value.next_cursor))||!Array.isArray(value.recipients)||typeof value.can_create!=='boolean'||!strings(value.create_blockers)||!handoffVersion(value.task_version))return false;
 const seen=new Set<string>();
 for(const row of value.items){if(!validHandoff(row,project,task)||seen.has(row.id))return false;seen.add(row.id);}
 if(value.next_cursor!==''&&!seen.has(value.next_cursor as string))return false;
 return value.recipients.every(row=>object(row)&&handoffID(row.resource_id)&&handoffVersion(row.resource_version)&&handoffID(row.user_id)&&typeof row.name==='string'&&Array.isArray(row.teams)&&row.teams.every(team=>object(team)&&handoffID(team.team_id)&&typeof team.name==='string'&&handoffVersion(team.team_version)&&handoffVersion(team.membership_version)));
}
export function appendHandoffs(previous:TaskHandoffPage,next:TaskHandoffPage):TaskHandoffPage|undefined{
 const items=[...previous.items,...next.items];
 if(new Set(items.map(item=>item.id)).size!==items.length||items.length>next.total||next.next_cursor&&next.next_cursor===previous.next_cursor)return;
 return {...next,items};
}
export function handoffCommandID(path:string){const match=path.match(/^\/task-handoffs\/([^/]+)\/commands$/);try{const id=match&&decodeURIComponent(match[1]);return handoffID(id)?id:undefined;}catch{return;}}
export function validHandoffRecovery(request:WriteRecoveryRequest,project:string,task:string){
 const body=request.body;if(request.method!=='POST'||body.project_id!==project)return false;
 if(request.path==='/task-handoffs')return Object.keys(body).every(key=>['project_id','task_id','expected_task_version','to_resource_id','to_team_id','note','idempotency_key'].includes(key))&&body.task_id===task&&handoffVersion(body.expected_task_version)&&body.expected_task_version<2147483647&&handoffID(body.to_resource_id)&&(body.to_team_id===undefined||optionalID(body.to_team_id))&&validHandoffText(body.note);
 return !!handoffCommandID(request.path)&&Object.keys(body).every(key=>['project_id','expected_version','action','reason','acknowledge_invalidation','idempotency_key'].includes(key))&&handoffVersion(body.expected_version)&&body.expected_version<2147483647&&actions.includes(String(body.action))&&validHandoffText(body.reason)&&typeof body.acknowledge_invalidation==='boolean'&&(body.action==='accept'?body.acknowledge_invalidation:!body.acknowledge_invalidation);
}
export function validHandoffReceipt(value:unknown,request:WriteRecoveryRequest,project:string,task:string){
 if(!validHandoff(value,project,task))return false;const body=request.body;
 if(request.path==='/task-handoffs')return value.version===1&&value.state==='pending'&&value.note===body.note&&value.to_resource_id===body.to_resource_id&&value.to_team_id===(body.to_team_id||'')&&value.basis.task_version===body.expected_task_version;
 return value.id===handoffCommandID(request.path)&&value.version===Number(body.expected_version)+1&&value.state===({accept:'accepted',return:'returned',withdraw:'withdrawn'} as Record<string,string>)[String(body.action)]&&value.decision_reason===body.reason;
}
type DraftStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export function handoffDraftKey(user:string,organization:string,project:string,task:string){return 'suma-task-handoff-draft:'+JSON.stringify([user,organization,project,task]);}
export function readHandoffDraft(storage:DraftStorage|undefined,key:string):{draft:HandoffDraft;invalid?:boolean}{
 try{
  const raw=storage?.getItem(key);if(!raw)return {draft:emptyHandoffDraft()};const value:unknown=JSON.parse(raw);
  if(!object(value)||value.format!=='suma-task-handoff-draft-v1'||!optionalID(value.recipient)||!optionalID(value.team)||!draftText(value.note)||Object.keys(value).some(key=>!['format','recipient','team','note','task_version','decision'].includes(key))||value.task_version!==undefined&&!handoffVersion(value.task_version))return {draft:emptyHandoffDraft(),invalid:true};
  const decision=value.decision;
  if(decision!==undefined&&(!object(decision)||!exact(decision,['id','version','action','reason'])||!handoffID(decision.id)||!handoffVersion(decision.version)||!actions.includes(String(decision.action))||!draftText(decision.reason)))return {draft:emptyHandoffDraft(),invalid:true};
  return {draft:{recipient:value.recipient as string,team:value.team as string,note:value.note,...(value.task_version!==undefined?{task_version:value.task_version as number}:{}),...(decision?{decision:decision as HandoffDecision}:{})}};
 }catch{return {draft:emptyHandoffDraft(),invalid:true};}
}
export function saveHandoffDraft(storage:DraftStorage|undefined,key:string,draft:HandoffDraft){
 if(!storage)throw Error('draft storage unavailable');
 if(!draft.recipient&&!draft.team&&!draft.note&&!draft.decision)storage.removeItem(key);else storage.setItem(key,JSON.stringify({format:'suma-task-handoff-draft-v1',...draft}));
}
/** A receipt clears only the original text and version it actually confirmed. */
export function draftAfterHandoffReceipt(draft:HandoffDraft,request:WriteRecoveryRequest):HandoffDraft{
 const body=request.body;
 if(request.path==='/task-handoffs')return draft.recipient===body.to_resource_id&&draft.team===(body.to_team_id||'')&&draft.note===body.note&&draft.task_version===body.expected_task_version?{...emptyHandoffDraft(),...(draft.decision?{decision:draft.decision}:{})}:draft;
 const decision=draft.decision;
 return decision&&decision.id===handoffCommandID(request.path)&&decision.version===body.expected_version&&decision.action===body.action&&(!decision.reason||decision.reason===body.reason)?{...draft,decision:undefined}:draft;
}
