import {tr,UiError,getLocale} from './i18n';
import {api,ApiError,operation,query,type Session,type Workflow} from './api';
import {mutate} from './operations';
import type {AutomationSpec as WireSpec,AutomationRevision as WireRevision,AutomationRule as WireRule,AutomationOccurrence as WireOccurrence,AutomationPreview as WirePreview} from '../../../packages/contracts/src/index';

export type AutomationSpec=Omit<WireSpec,'trigger'|'misfire'|'overlap'>&{trigger:'cron'|'manual';misfire:'skip'|'latest'|'catch_up';overlap:'allow'|'forbid'|'queue'};
export type AutomationRevision=Omit<WireRevision,'spec'>&{spec:AutomationSpec};
export type AutomationRule=Omit<WireRule,keyof WireSpec|'revisions'>&AutomationSpec&{revisions?:AutomationRevision[]};
export type AutomationOccurrence=WireOccurrence;
export type AutomationResult={rule:AutomationRule;occurrence?:AutomationOccurrence};
export type AutomationPreview=WirePreview;
export type AutomationPage<T>={items:T[];total:number;next_cursor?:string};
export const automationStates:Record<string,string>={get draft(){return tr("automation.draft");},get enabled(){return tr("automation.enabled");},get paused(){return tr("automation.paused");},get archived(){return tr("automation.archived");}};
export const occurrenceStates:Record<string,string>={get queued(){return tr("automation.awaiting_dispatch");},get started(){return tr("automation.run_linked");},get skipped(){return tr("automation.skipped");},get blocked(){return tr("automation.dispatch_blocked");}};
export const misfireNames={get skip(){return tr("automation.skip_missed_times");},get latest(){return tr("automation.catch_up_only_the_latest");},get catch_up(){return tr("automation.bounded_catch_up");}};
export const overlapNames={get allow(){return tr("automation.allow_parallel_runs");},get forbid(){return tr("automation.skip_overlapping_triggers");},get queue(){return tr("automation.queue_and_wait");}};
export const newAutomationSpec=():AutomationSpec=>({name:'',workflow_id:'',workflow_version:0,trigger:'cron',cron:'0 9 * * 1-5',timezone:'Asia/Shanghai',misfire:'latest',catch_up_limit:5,overlap:'queue'});
export const automationEnabled=(rule:AutomationRule)=>rule.state==='enabled';
export function automationDetailPath(project:string,id:string){return '/automations/'+encodeURIComponent(id)+query(project);}
export function automationListPath(project:string,cursor='',rule=''){return '/automations'+(rule?'/'+encodeURIComponent(rule)+'/occurrences':'')+query(project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):'');}
export function automationPermissions(session:Session,project:string){const grant=session.projects.find(item=>item.id===project);if(!grant)return {write:false,approve:false};const inherited=['owner','admin'].includes(session.user.role||'');return {write:inherited||['admin','developer','reviewer'].includes(grant.role),approve:inherited||['admin','reviewer'].includes(grant.role)};}
export function automationWorkflowOptions(rows:Workflow[]){return [...new Map(rows.filter(row=>row.version>0).map(row=>[row.id+'@'+row.version,row])).values()].sort((a,b)=>a.id.localeCompare(b.id)||b.version-a.version);}
export function automationSpec(input:AutomationSpec):AutomationSpec{
 if(!input.name.trim())throw new UiError("automation.enter_a_rule_name");
 if(new TextEncoder().encode(input.name.trim()).length>200)throw new UiError("automation.name_is_too_long_shorten_it_to_200");
 if(!input.workflow_id||!Number.isSafeInteger(input.workflow_version)||input.workflow_version<1)throw new UiError("automation.select_an_actual_workflow_and_enter_a_fixed");
 if(!['cron','manual'].includes(input.trigger))throw new UiError("automation.select_a_scheduled_or_manual_trigger");
 if(input.trigger==='cron'&&input.cron.trim().split(/\s+/).length!==5)throw new UiError("automation.cron_requires_five_fields_minute_hour_day_month");
 try{new Intl.DateTimeFormat('en',{timeZone:input.timezone}).format();}catch{throw new UiError("automation.enter_a_valid_iana_time_zone_such_as");}
 if(!Number.isSafeInteger(input.catch_up_limit)||input.catch_up_limit<1||input.catch_up_limit>100)throw new UiError("automation.the_catch_up_limit_must_be_an_integer");
 if(!(input.misfire in misfireNames)||!(input.overlap in overlapNames))throw new UiError("automation.select_valid_missed_run_and_overlap_policies");
 return {name:input.name.trim(),workflow_id:input.workflow_id,workflow_version:input.workflow_version,trigger:input.trigger,cron:input.trigger==='manual'?'':input.cron.trim(),timezone:input.timezone.trim(),misfire:input.misfire,catch_up_limit:input.catch_up_limit,overlap:input.overlap};
}
export function automationError(error:unknown):string|Error{if(error instanceof UiError)return error;if(error instanceof ApiError)return error.status===403?new UiError("automation.your_account_does_not_have_permission_to_access"):error;return error instanceof Error?error.message:new UiError("automation.reading_did_not_complete_try_again");}

export function automationMoment(value:string,timezone='UTC'){
 let local=tr("automation.time_is_temporarily_unreadable"),zone=timezone;try{local=new Intl.DateTimeFormat(getLocale(),{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}).format(new Date(value));}catch{zone='UTC';try{local=new Date(value).toISOString();}catch{/* Keep the raw server time below for audit. */}}
 return {local,timezone:zone,utc:value};
}
const reasonNames:Record<string,string>={
 get activation_authority_or_workflow_unavailable(){return tr("automation.the_activator_s_permissions_or_fixed_workflow_are");},
 get activation_authority_revoked(){return tr("automation.the_activator_s_project_write_or_approval_permission");},
 get workflow_unavailable(){return tr("automation.the_fixed_workflow_version_is_unavailable_dispatch_stopped");},
 get rule_inactive_or_revised(){return tr("automation.the_rule_is_inactive_or_its_specification_changed");},
 get rule_revised(){return tr("automation.the_rule_specification_changed_dispatch_stopped_for_this");},get rule_revise(){return tr("automation.the_rule_policy_changed_waiting_triggers_for_the");},
 get rule_pause(){return tr("automation.the_rule_is_paused_this_waiting_trigger_stopped");},get rule_archive(){return tr("automation.the_rule_is_archived_this_waiting_trigger_stopped");},
 get overlap_forbid(){return tr("automation.the_previous_run_is_unfinished_this_trigger_was");},get overlap_wait(){return tr("automation.the_previous_run_is_unfinished_this_trigger_is");},get queue_wait(){return tr("automation.earlier_triggers_are_still_queued_this_trigger_continues");},
 get misfire_skip(){return tr("automation.the_missed_trigger_window_was_skipped");},get misfire_latest(){return tr("automation.missed_triggers_were_merged_only_the_latest_was");},get misfire_catch_up(){return tr("automation.this_trigger_is_a_catch_up_under_the");},get misfire_catch_up_limit(){return tr("automation.the_recovery_window_reached_its_catch_up_limit");},get already_resolved(){return tr("automation.this_trigger_has_already_been_processed");}
};
export function automationReason(raw:string,scheduledAt='',timezone='UTC'){
 const[code,...parts]=raw.split(';'),fields=new Map(parts.map(part=>{const separator=part.indexOf('=');return [part.slice(0,separator),part.slice(separator+1)];}));
 const start=fields.get('window_start')||(fields.has('window_end')?scheduledAt:''),end=fields.get('window_end')||(fields.has('window_start')?scheduledAt:'');
 const valid=(value:string)=>!!value&&Number.isFinite(Date.parse(value));
 const range=valid(start)&&valid(end)?tr("automation.window_range",{start:automationMoment(start,timezone).local,end:automationMoment(end,timezone).local,timezone}):'';
 return {message:reasonNames[code]||tr("automation.server_reason"),range,raw};
}
type RevisionSnapshot={items:AutomationRevision[]|undefined;total:number;next_cursor:string;loading:boolean;error:string|Error;more_error:boolean};
// Revision numbers are the immutable history identity; these records have no id.
export class AutomationRevisionPager{
 private state:RevisionSnapshot={items:undefined,total:0,next_cursor:'',loading:false,error:'',more_error:false};private generation=0;
 constructor(private project:string,private rule:string){}
 snapshot(){return {...this.state};}
 async first(){return this.read('',false,++this.generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){
  this.state={...this.state,loading:true,error:'',more_error:append};
  try{const page=await api<AutomationPage<AutomationRevision>>('/automations/'+encodeURIComponent(this.rule)+'/revisions'+query(this.project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):''));
   if(generation!==this.generation)return;if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.items.some(item=>!Number.isSafeInteger(item.revision)||item.revision<1||!item.spec))throw new UiError("automation.specification_revision_history_has_not_returned_a_confirmed");
   this.state={items:[...new Map([...(append?this.state.items||[]:[]),...page.items].map(item=>[item.revision,item])).values()],total:page.total,next_cursor:page.next_cursor||'',loading:false,error:'',more_error:false};
  }catch(error){if(generation===this.generation)this.state={...this.state,loading:false,error:automationError(error)};}
 }
}

type RecoveryStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export type AutomationPending={path:string;body:Record<string,unknown>;idempotency_key:string};
const clone=<T,>(value:T):T=>JSON.parse(JSON.stringify(value));
function validateRequest(path:string,body:unknown){
 if(!/^\/automations(?:\/[A-Za-z0-9._%~-]+\/commands)?$/.test(path))throw new UiError("automation.only_rule_writes_and_commands_for_the_current");
 function inspect(value:unknown){if(!value||typeof value!=='object')return;for(const[key,item]of Object.entries(value)){if(/password|token|secret|credential|authorization/i.test(key))throw new UiError("automation.automation_recovery_records_cannot_store_secret_credentials");inspect(item);}}
 inspect(body);if(!body||typeof body!=='object'||typeof (body as Record<string,unknown>).project_id!=='string')throw new UiError("automation.the_automation_request_has_no_project_scope");
}
// A tab-scoped non-secret envelope survives a lost receipt and browser reload.
// operations.mutate replays its immutable body and supplied stable operation ID.
export class AutomationRecovery{
 private value?:AutomationPending;private busy=false;private key:string;private unreadable=false;
 get error(){return this.unreadable?tr("automation.the_automation_recovery_record_is_unreadable_no_new"):'';}
 constructor(scope:string,private storage:RecoveryStorage|undefined,private send:typeof api=api){this.key='suma-automation-recovery-v1:'+scope;try{const raw=storage?.getItem(this.key);if(raw){const saved=JSON.parse(raw) as AutomationPending;validateRequest(saved.path,saved.body);if(typeof saved.idempotency_key!=='string'||!saved.idempotency_key)throw new Error('invalid');this.value=saved;}}catch{this.unreadable=true;}}
 get pending(){return this.value?clone(this.value):undefined;}
 async restore<T=unknown>():Promise<T>{if(!this.value)throw new UiError("automation.there_is_no_automation_operation_to_recover");return this.run<T>(this.value.path,this.value.body);}
 async run<T=unknown>(path:string,body:Record<string,unknown>):Promise<T>{
  if(this.unreadable)throw new UiError("automation.the_automation_recovery_record_is_unreadable_no_new");if(this.busy)throw new UiError("automation.the_original_operation_is_still_awaiting_a_receipt");
  const restoring=!!this.value;
  if(!this.value){validateRequest(path,body);const saved={path,body:clone(body),idempotency_key:operation()};try{if(!this.storage)throw new Error('unavailable');this.storage.setItem(this.key,JSON.stringify(saved));}catch{throw new UiError("automation.the_automation_recovery_record_could_not_be_saved");}this.value=saved;}
  this.busy=true;const saved=this.value;
  try{const result=await mutate<T>(saved.path,{...saved.body,idempotency_key:saved.idempotency_key},'POST','automation:'+this.key+':'+saved.idempotency_key,this.send);this.clear();return result;}
  catch(error){if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();throw error;}finally{this.busy=false;}
 }
 private clear(){this.value=undefined;try{this.storage?.removeItem(this.key);}catch{/* Same saved operation remains safe to replay. */}}
}
