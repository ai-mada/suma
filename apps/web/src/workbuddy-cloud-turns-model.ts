import type {CloudCollaborationSource,CloudTurnView,CloudTurnPage,CloudTurn,CloudTurnSource,CloudTurnCommand} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {collaborationIdentity} from './workbuddy-cloud-collaboration-model';
import {wellFormed} from './i18n/catalog';
export type CloudTurnsScope={source:CloudCollaborationSource;userID:string;organizationID:string};
const max=2147483647,encoder=new TextEncoder(),bytes=(v:string)=>encoder.encode(v).length;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const fields=(v:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const integer=(v:unknown,min=0,limit=max):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min&&v<=limit;
const id=(v:unknown):v is string=>typeof v==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(v);
const text=(v:unknown,limit=32768):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=limit&&!/[\p{Cc}]/u.test(v.replace(/[\r\n\t]/g,''));
const labelID=(v:unknown):v is string=>text(v,128)&&!!v&&v.trim()===v&&!/[\p{Cc}]/u.test(v);
const operation=(v:unknown):v is string=>text(v,128)&&bytes(v)>=8&&v.trim()===v&&!/[\p{Cc}]/u.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const errors=['workbuddy_cloud_turn_unknown','workbuddy_cloud_turn_interrupted','workbuddy_cloud_turn_limited','workbuddy_cloud_turn_authorization_changed','workbuddy_cloud_turn_rejected','workbuddy_cloud_turn_unavailable'];
const commandStates=['prepared','sent','acknowledged','unknown','rejected'];
function instant(v:unknown):bigint|undefined{
 if(typeof v!=='string'||v.length>40)return;
 const match=/^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?Z$/.exec(v);if(!match)return;
 const value=Date.parse(match[1]+'Z');if(!Number.isFinite(value)||new Date(value).toISOString().slice(0,19)!==match[1]||match[1]==='0001-01-01T00:00:00')return;
 return BigInt(value)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
}
function sourceIdentity(v:unknown):string|undefined{try{return collaborationIdentity(v as CloudCollaborationSource);}catch{return;}}
export function validCloudTurnsScope(scope:CloudTurnsScope):boolean{return object(scope)&&fields(scope,['source','userID','organizationID'])&&id(scope.userID)&&id(scope.organizationID)&&sourceIdentity(scope.source)!==undefined;}
export function cloudTurnsIdentity(scope:CloudTurnsScope):string{if(!validCloudTurnsScope(scope))throw Error('Invalid cloud turn scope');return JSON.stringify([scope.userID,scope.organizationID,collaborationIdentity(scope.source)]);}
const pathID=(v:string)=>{if(!id(v))throw Error('Invalid cloud turn locator');return encodeURIComponent(v);};
export const cloudTurnPromptPath=(submissionID:string)=>'/workbuddy-cloud-submissions/'+pathID(submissionID)+'/turns';
export function cloudTurnViewPath(scope:CloudTurnsScope):string{if(!validCloudTurnsScope(scope))throw Error('Invalid cloud turn scope');return cloudTurnPromptPath(scope.source.submission_id)+'?'+new URLSearchParams({project_id:scope.source.project_id});}
export function cloudTurnPagePath(project:string,turnID:string,after=0):string{if(!id(project)||!integer(after,0,1000))throw Error('Invalid cloud turn page');return '/workbuddy-cloud-turns/'+pathID(turnID)+'?'+new URLSearchParams({project_id:project,after:String(after),limit:'50'});}
export const cloudTurnPermissionPath=(turnID:string,permissionID:string)=>'/workbuddy-cloud-turns/'+pathID(turnID)+'/permissions/'+pathID(permissionID)+'/reply';
export function cloudTurnControlPath(turnID:string,kind:'cancel'|'reconcile'):string{if(kind!=='cancel'&&kind!=='reconcile')throw Error('Invalid cloud turn command');return '/workbuddy-cloud-turns/'+pathID(turnID)+'/'+kind;}
function validSource(v:unknown,scope:CloudTurnsScope):v is CloudTurnSource{
 return object(v)&&fields(v,['collaboration','attempt','fence','dispatch_id','assignment_sha256'])&&sourceIdentity(v.collaboration)===sourceIdentity(scope.source)&&sourceIdentity(v.collaboration)!==undefined&&integer(v.attempt,1)&&integer(v.fence,1,Number.MAX_SAFE_INTEGER)&&(v.dispatch_id===''||id(v.dispatch_id))&&hash(v.assignment_sha256);
}
function usageValid(v:Record<string,unknown>):boolean{
 if(v.usage_state==='unreported')return !Object.hasOwn(v,'usage');
 if(v.usage_state!=='partial'||!object(v.usage)||!fields(v.usage,[],['input_tokens','cached_input_tokens','cache_write_input_tokens','output_tokens','reasoning_output_tokens']))return false;
 return Object.keys(v.usage).length>0&&Object.values(v.usage).every(n=>integer(n,0,Number.MAX_SAFE_INTEGER));
}
function validTurn(v:unknown,scope:CloudTurnsScope):v is CloudTurn{
 if(!object(v)||!fields(v,['id','version','operation_id','head_version_at_start','source','owner_id','state','prompt_sha256','started_at','deadline_at','overdue','usage_state'],['finished_at','stop_reason','error_code','usage'])||!id(v.id)||!integer(v.version,1)||!operation(v.operation_id)||!integer(v.head_version_at_start,0,max-1)||!validSource(v.source,scope)||v.source.collaboration.submission_version>scope.source.submission_version||v.owner_id!==scope.userID||!hash(v.prompt_sha256)||typeof v.state!=='string'||!['prepared','running','waiting_permission','completed','unknown','rejected'].includes(v.state)||typeof v.overdue!=='boolean'||!usageValid(v))return false;
 const started=instant(v.started_at),deadline=instant(v.deadline_at),finished=instant(v.finished_at);
 if(started===undefined||deadline===undefined||deadline<=started||Object.hasOwn(v,'finished_at')&&(finished===undefined||finished<started))return false;
 if(Object.hasOwn(v,'stop_reason')&&(typeof v.stop_reason!=='string'||!['end_turn','max_tokens','max_turn_requests','refusal','cancelled'].includes(v.stop_reason)))return false;
 if(Object.hasOwn(v,'error_code')&&(typeof v.error_code!=='string'||!errors.includes(v.error_code)))return false;
 if(v.state==='completed'&&(finished===undefined||!Object.hasOwn(v,'stop_reason')||Object.hasOwn(v,'error_code')||v.overdue))return false;
 return true;
}
export function readCloudTurnView(raw:unknown,scope:CloudTurnsScope):CloudTurnView|undefined{
 if(!validCloudTurnsScope(scope)||!object(raw)||!fields(raw,['source','version','allowed_actions'],['latest_turn'])||!validSource(raw.source,scope)||raw.source.collaboration.submission_version<scope.source.submission_version||!integer(raw.version)||!Array.isArray(raw.allowed_actions))return;
 const actions=['prompt','cancel','reconcile'];let previous=-1;
 for(const action of raw.allowed_actions){const index=actions.indexOf(action);if(index<=previous)return;previous=index;}
 if(Object.hasOwn(raw,'latest_turn')){
  const currentScope={...scope,source:raw.source.collaboration};
  if(!validTurn(raw.latest_turn,currentScope)||raw.version<=raw.latest_turn.head_version_at_start||raw.latest_turn.source.attempt!==raw.source.attempt||raw.latest_turn.source.fence!==raw.source.fence||raw.latest_turn.source.dispatch_id!==raw.source.dispatch_id||raw.latest_turn.source.assignment_sha256!==raw.source.assignment_sha256)return;
  if(raw.allowed_actions.includes('prompt')&&['prepared','running','waiting_permission','unknown'].includes(raw.latest_turn.state))return;
 }else if(raw.version!==0)return;
 return raw as unknown as CloudTurnView;
}
function permission(v:unknown,turnID:string):boolean{
 if(!object(v)||!fields(v,['id','turn_id','version','tool_call_id','title','details','details_sha256','approve_allowed','state','options'])||!id(v.id)||v.turn_id!==turnID||!integer(v.version,1)||!labelID(v.tool_call_id)||!text(v.title)||!text(v.details)||!hash(v.details_sha256)||typeof v.approve_allowed!=='boolean'||typeof v.state!=='string'||!['pending',...commandStates].includes(v.state)||!Array.isArray(v.options)||v.options.length===0)return false;
 return v.options.every(option=>object(option)&&fields(option,['id','name','kind'])&&labelID(option.id)&&text(option.name)&&typeof option.kind==='string'&&['allow_once','allow_always','reject_once','reject_always'].includes(option.kind))&&new Set(v.options.map(option=>option.id)).size===v.options.length;
}
function command(v:unknown,turnID:string):v is CloudTurnCommand{
 if(!object(v)||!fields(v,['id','turn_id','operation_id','kind','state'],['permission_id','option_id','error_code'])||!id(v.id)||v.turn_id!==turnID||!operation(v.operation_id)||typeof v.kind!=='string'||!['permission','cancel','reconcile'].includes(v.kind)||typeof v.state!=='string'||!commandStates.includes(v.state)||Object.hasOwn(v,'error_code')&&(typeof v.error_code!=='string'||!errors.includes(v.error_code)))return false;
 return v.kind==='permission'?id(v.permission_id)&&labelID(v.option_id):!Object.hasOwn(v,'permission_id')&&!Object.hasOwn(v,'option_id');
}
export function readCloudTurnPage(raw:unknown,scope:CloudTurnsScope,turnID:string,after=0):CloudTurnPage|undefined{
 if(!validCloudTurnsScope(scope)||!id(turnID)||!integer(after,0,1000)||!object(raw)||!fields(raw,['turn','items','permissions','commands','next_after','has_more'])||!validTurn(raw.turn,scope)||raw.turn.id!==turnID||!Array.isArray(raw.items)||raw.items.length>50||!Array.isArray(raw.permissions)||raw.permissions.length>32||!Array.isArray(raw.commands)||!integer(raw.next_after,after,1000)||raw.next_after!==after+raw.items.length||typeof raw.has_more!=='boolean'||raw.has_more&&(raw.items.length!==50||raw.next_after>=1000))return;
 let totalBytes=0;
 for(let i=0;i<raw.items.length;i++){
  const entry=raw.items[i];
  if(!object(entry)||entry.sequence!==after+i+1)return;
  if(entry.kind==='artifact'){
   if(!fields(entry,['sequence','kind','text','artifact_type','artifact_event'])||!text(entry.text,8192))return;
   if(typeof entry.artifact_type!=='string'||!['plan','tasks','media','overview'].includes(entry.artifact_type)||
      typeof entry.artifact_event!=='string'||!['created','updated','deleted'].includes(entry.artifact_event))return;
   const canPreview=(entry.artifact_type==='plan'||entry.artifact_type==='overview')&&entry.artifact_event!=='deleted';
   if(!canPreview&&entry.text!=='')return;
  }else{
   if(!fields(entry,['sequence','kind','text'],['tool_call_id','tool_state'])||!text(entry.text))return;
   if(typeof entry.kind!=='string'||!['assistant_text','tool_state','plan_summary','notice','unsupported'].includes(entry.kind))return;
   if(Object.hasOwn(entry,'tool_call_id')&&!labelID(entry.tool_call_id)||Object.hasOwn(entry,'tool_state')&&!text(entry.tool_state,128))return;
  }
  totalBytes+=bytes(entry.text);
 }
 if(totalBytes>1048576||!raw.permissions.every(p=>permission(p,turnID))||new Set(raw.permissions.map(p=>p.id)).size!==raw.permissions.length||!raw.commands.every(c=>command(c,turnID))||new Set(raw.commands.map(c=>c.id)).size!==raw.commands.length)return;
 return raw as unknown as CloudTurnPage;
}
type Target={kind:'prompt'|'permission'|'cancel'|'reconcile';turnID?:string;permissionID?:string};
function requestTarget(raw:unknown,scope:CloudTurnsScope):Target|undefined{
 if(!validCloudTurnsScope(scope)||!object(raw)||!fields(raw,['path','method','body','idempotency_key'])||typeof raw.path!=='string'||raw.method!=='POST'||!operation(raw.idempotency_key)||!object(raw.body)||raw.body.project_id!==scope.source.project_id||raw.body.idempotency_key!==raw.idempotency_key)return;
 const body=raw.body;
 if(raw.path===cloudTurnPromptPath(scope.source.submission_id)){
  if(!fields(body,['project_id','expected_submission_version','expected_run_version','expected_version','prompt','idempotency_key'])||!integer(body.expected_submission_version,1,max-1)||body.expected_submission_version>scope.source.submission_version||!integer(body.expected_run_version,1,max-1)||!integer(body.expected_version,0,max-1)||!text(body.prompt,32000)||!body.prompt.trim())return;
  return {kind:'prompt'};
 }
 const match=/^\/workbuddy-cloud-turns\/([A-Za-z0-9_.-]{1,64})\/(cancel|reconcile|permissions\/([A-Za-z0-9_.-]{1,64})\/reply)$/.exec(raw.path);
 if(!match||!id(match[1])||!integer(body.expected_version,1,max-1))return;
 if(match[3])return id(match[3])&&fields(body,['project_id','expected_version','option_id','idempotency_key'])&&labelID(body.option_id)?{kind:'permission',turnID:match[1],permissionID:match[3]}:undefined;
 return fields(body,['project_id','expected_version','idempotency_key'])?{kind:match[2] as 'cancel'|'reconcile',turnID:match[1]}:undefined;
}
export function validCloudTurnRecovery(req:unknown,scope:CloudTurnsScope):req is WriteRecoveryRequest{return !!requestTarget(req,scope);}
/** A prepared command receipt confirms persistence only. The caller reads the
 * current page for actual send/terminal facts; no state is advanced locally. */
export async function validCloudTurnReceipt(raw:unknown,req:WriteRecoveryRequest,scope:CloudTurnsScope):Promise<boolean>{
 const target=requestTarget(req,scope);if(!target)return false;
 if(target.kind==='prompt'){
  if(!validTurn(raw,scope)||raw.operation_id!==req.idempotency_key||raw.head_version_at_start!==req.body.expected_version||raw.source.collaboration.submission_version!==req.body.expected_submission_version)return false;
  const digest=await crypto.subtle.digest('SHA-256',encoder.encode(req.body.prompt as string));
  return raw.prompt_sha256===Array.from(new Uint8Array(digest),v=>v.toString(16).padStart(2,'0')).join('');
 }
 return command(raw,target.turnID!)&&raw.operation_id===req.idempotency_key&&raw.kind===target.kind&&(target.kind!=='permission'||raw.permission_id===target.permissionID&&raw.option_id===req.body.option_id);
}
