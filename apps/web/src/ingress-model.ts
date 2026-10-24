import {tr,UiError} from './i18n';
import {api,ApiError,operation,query} from './api';
import {automationError} from './automation-model';
import type {AutomationIngress,AutomationIngressSpec} from '../../../packages/contracts/src/index';
export type {AutomationIngress,AutomationIngressSpec,AutomationIngressRevision,AutomationIngressEvent,AutomationIngressDelivery,AutomationIngressCreateRequest,AutomationIngressCommandRequest} from '../../../packages/contracts/src/index';
export const ingressEvents:Record<string,string[]>={github:['push','workflow_run.completed','ping'],suma:['git.push','ci.completed','monitor.firing','monitor.resolved']};
const eventNames:Record<string,string>={push:'automation.event_repository_push','workflow_run.completed':'automation.event_workflow_completed',ping:'automation.event_connection_check','git.push':'automation.event_git_push','ci.completed':'automation.event_ci_completed','monitor.firing':'automation.event_monitor_firing','monitor.resolved':'automation.event_monitor_resolved'};
/** Labels change with locale; the signed protocol identifier stays visible and unchanged. */
export function ingressEventLabel(event:string){return Object.prototype.hasOwnProperty.call(eventNames,event)?tr('automation.event_identity',{label:tr(eventNames[event]),event}):event;}
export const ingressStates:Record<string,string>={get paused(){return tr("automation.paused");},get enabled(){return tr("automation.enabled");},get revoked(){return tr("automation.revoked");}};
export const ingressDeliveryStates:Record<string,string>={get accepted(){return tr("automation.trigger_created");},get ignored(){return tr("automation.ignored");},get blocked(){return tr("automation.blocked_by_gate");}};
const ingressReasons:Record<string,string>={
 get connection_check(){return tr("automation.this_is_a_connectivity_check_no_trigger_is");},
 get branch_deleted(){return tr("automation.the_source_branch_or_tag_was_deleted_no");},
 get action_not_completed(){return tr("automation.the_source_workflow_action_has_not_reached_completed");},
 get ingress_inactive(){return tr("automation.the_ingress_is_disabled_or_has_no_key");},
 get rule_inactive_or_revised(){return tr("automation.the_rule_is_disabled_or_the_bound_revision");},
 get activation_authority_revoked(){return tr("automation.the_ingress_or_rule_activator_lacks_current_project");},
 get workflow_unavailable(){return tr("automation.the_fixed_workflow_is_currently_unavailable_restore_and");},
 get filter_mismatch(){return tr("automation.the_event_did_not_match_the_ingress_restrictions");},
 get ingress_delivery_unavailable(){return tr("automation.the_delivery_record_associated_with_this_trigger_is");},
 get ingress_revised_or_rotated(){return tr("automation.the_source_revision_or_key_changed_original_queued");},
 get ingress_pause(){return tr("automation.the_ingress_is_paused_original_queued_triggers_are");},
 get ingress_revise(){return tr("automation.the_ingress_source_changed_original_queued_triggers_are");},
 get ingress_revoke(){return tr("automation.the_ingress_is_revoked_original_queued_triggers_are");},
 get ingress_rotate_key(){return tr("automation.the_ingress_key_changed_original_queued_triggers_are");}
};
export function ingressReason(raw:string){const code=raw.split(';')[0],known=Object.prototype.hasOwnProperty.call(ingressReasons,code);return {known,message:known?ingressReasons[code]:tr("automation.the_server_retained_the_decision_reason_expand_the"),raw};}
export const newIngressSpec=():AutomationIngressSpec=>({name:'',provider:'github',events:['push'],repository:'',repository_id:'',ref:'',conclusions:[]});
const bytes=(text:string)=>new TextEncoder().encode(text).length;
const controls=/[\u0000-\u001f\u007f]/;
export function ingressSpec(input:AutomationIngressSpec):AutomationIngressSpec{
 const allowed=['name','provider','events','repository','repository_id','ref','conclusions'];
 if(Object.keys(input).some(key=>!allowed.includes(key)))throw new UiError("automation.event_ingresses_accept_only_restricted_filters_without_execution");
 if(!input.name.trim()||bytes(input.name.trim())>200||controls.test(input.name))throw new UiError("automation.the_ingress_name_must_contain_1_200_utf");
 const events=ingressEvents[input.provider];if(!events)throw new UiError("automation.select_the_github_or_suma_signing_protocol");
 if(!Array.isArray(input.events)||input.events.length<1||input.events.length>8||new Set(input.events).size!==input.events.length||input.events.some(event=>!events.includes(event)))throw new UiError("automation.select_events_supported_by_this_protocol_without_duplicates");
 for(const field of ['repository','repository_id','ref'] as const)if(typeof input[field]!=='string'||controls.test(input[field]))throw new UiError("automation.source_filter_fields_cannot_contain_control_characters");
 if(bytes(input.repository)>201||input.repository_id.length>30||input.repository_id&&!/^[1-9][0-9]*$/.test(input.repository_id))throw new UiError("automation.repository_names_allow_at_most_201_bytes_repository");
 if(input.provider==='github'&&(!/^[^\s/]+\/[^\s/]+$/.test(input.repository)||!input.repository_id))throw new UiError("automation.github_ingresses_require_both_owner_repo_and_a");
 if(bytes(input.ref)>250||input.ref&&!/^refs\/(?:heads|tags)\/.+$/.test(input.ref))throw new UiError("automation.branch_and_tag_filters_must_use_refs_heads");
 if(!Array.isArray(input.conclusions)||input.conclusions.length>16||new Set(input.conclusions).size!==input.conclusions.length||input.conclusions.some(value=>!/^[-a-z0-9_.]{1,40}$/.test(value)))throw new UiError("automation.source_conclusions_allow_at_most_16_unique_lowercase");
 return {name:input.name.trim(),provider:input.provider,events:[...input.events],repository:input.repository,repository_id:input.repository_id,ref:input.ref,conclusions:[...input.conclusions]};
}
export function ingressCanEnable(entry:AutomationIngress,rule:{version:number;revision:number;state:string}){return entry.state!=='revoked'&&entry.key_configured&&entry.rule_revision===rule.revision&&rule.state==='enabled';}
export const ingressPanel=(id:string)=>id?'ingress:'+id:'';
export const selectedIngress=(panel:string)=>panel.startsWith('ingress:')?panel.slice(8):'';
export const ingressPath=(id:string)=>'/automation-ingresses/'+encodeURIComponent(id);
export function ingressKey(){return Array.from(crypto.getRandomValues(new Uint8Array(32)),byte=>byte.toString(16).padStart(2,'0')).join('');}
export async function ingressFingerprint(secret:string){if(!/^[0-9a-f]{64}$/.test(secret))throw new UiError("automation.signing_keys_must_contain_exactly_64_lowercase_hexadecimal");return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(secret))),byte=>byte.toString(16).padStart(2,'0')).join('');}
export function ingressSpecOf(entry:AutomationIngress):AutomationIngressSpec{return {name:entry.name,provider:entry.provider,events:[...entry.events],repository:entry.repository,repository_id:entry.repository_id,ref:entry.ref,conclusions:[...entry.conclusions]};}

type StorageLike=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export type IngressPending={path:string;body:Record<string,unknown>;idempotency_key:string;fingerprint?:string;secret_lost?:boolean};
const clone=<T,>(value:T):T=>JSON.parse(JSON.stringify(value));
function rejectSecrets(value:unknown){if(!value||typeof value!=='object')return;for(const[key,item]of Object.entries(value)){if(/secret|token|password|credential|authorization/i.test(key))throw new UiError("automation.recovery_metadata_cannot_contain_secrets");rejectSecrets(item);}}
// Only metadata reaches tab storage. A rotation's original secret lives here
// until its receipt arrives or the user explicitly closes the secret dialog.
export class IngressRecovery{
 private value?:IngressPending;private secret?:string;private busy=false;private key:string;private unreadable=false;
 get error(){return this.unreadable?tr("automation.the_event_ingress_recovery_record_is_unreadable_no"):'';}
 constructor(scope:string,private project:string,private rule:string,private storage:StorageLike|undefined){
  this.key='suma-ingress-recovery-v1:'+scope;
  try{const raw=storage?.getItem(this.key);if(raw){const saved=JSON.parse(raw) as IngressPending;this.validate(saved.path,saved.body,true);if(!saved.idempotency_key||typeof saved.idempotency_key!=='string'||saved.body.action==='rotate_key'&&!/^[a-f0-9]{64}$/.test(saved.fingerprint||''))throw new Error('invalid');this.value=saved;}}
  catch{this.unreadable=true;}
 }
 get pending():IngressPending|undefined{return this.value?{...clone(this.value),secret_lost:this.value.body.action==='rotate_key'&&!this.secret}:undefined;}
 private validate(path:string,body:Record<string,unknown>,saved=false){
  if(path!=='/automations/'+encodeURIComponent(this.rule)+'/ingresses'&&!/^\/automation-ingresses\/[A-Za-z0-9._%~-]+\/commands$/.test(path))throw new UiError("automation.the_recovery_path_is_outside_event_ingresses_for");
  if(!body||body.project_id!==this.project)throw new UiError("automation.the_event_ingress_request_has_no_current_project");
  const{secret,...metadata}=body;rejectSecrets(metadata);if(saved&&secret!==undefined)throw new UiError("automation.persistent_recovery_records_cannot_contain_signing_keys");
  if(body.action==='rotate_key'){if(!saved&&typeof secret!=='string')throw new UiError("automation.generate_a_signing_key_first");if(secret!==undefined&&!/^[a-f0-9]{64}$/.test(String(secret)))throw new UiError("automation.signing_keys_must_contain_exactly_64_lowercase_hexadecimal");}
  else if(secret!==undefined)throw new UiError("automation.metadata_operations_cannot_contain_signing_keys");
 }
 async run<T=AutomationIngress>(path:string,body:Record<string,unknown>):Promise<T>{
  if(this.unreadable)throw new UiError("automation.the_event_ingress_recovery_record_is_unreadable_no");if(this.busy||this.value)throw new UiError("automation.the_original_operation_still_needs_a_receipt_recover");this.validate(path,body);this.busy=true;
  try{const{secret,...metadata}=body;const saved:IngressPending={path,body:clone(metadata),idempotency_key:operation(),...(secret?{fingerprint:await ingressFingerprint(String(secret))}:{})};
   try{if(!this.storage)throw new Error('unavailable');this.storage.setItem(this.key,JSON.stringify(saved));}catch{throw new UiError("automation.event_ingress_recovery_metadata_could_not_be_saved");}
   this.value=saved;this.secret=secret as string|undefined;return await this.send<T>(false);
  }finally{this.busy=false;}
 }
 async restore<T=AutomationIngress>():Promise<T>{if(this.busy)throw new UiError("automation.the_original_operation_is_still_awaiting_a_receipt_433");if(!this.value)throw new UiError("automation.there_is_no_event_ingress_operation_to_recover");if(this.pending?.secret_lost)throw new UiError("automation.the_current_key_has_left_memory_load_the");this.busy=true;try{return await this.send<T>(true);}finally{this.busy=false;}}
 private async send<T>(restoring:boolean):Promise<T>{const saved=this.value!;try{const result=await api<T>(saved.path,{...saved.body,...(this.secret?{secret:this.secret}:{}),idempotency_key:saved.idempotency_key});this.clear();return result;}catch(error){if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();throw error;}}
 forgetSecret(){this.secret=undefined;}
 reconcile(entry:AutomationIngress){if(!this.pending?.secret_lost||this.value?.path!==ingressPath(entry.id)+'/commands')return false;if(entry.key_fingerprint===this.value.fingerprint&&entry.version>Number(this.value.body.expected_version)){this.clear();return true;}return false;}
 acknowledgeMetadata(entry:AutomationIngress){if(!this.pending?.secret_lost||this.value?.path!==ingressPath(entry.id)+'/commands'||entry.version<Number(this.value.body.expected_version))throw new UiError("automation.load_the_latest_metadata_for_the_original_ingress");this.clear();}
 private clear(){this.value=undefined;this.secret=undefined;try{this.storage?.removeItem(this.key);}catch{/* Replaying the same stable operation remains safe. */}}
}

type Page<T>={items:T[];total:number;next_cursor?:string};
type Snapshot<T>={items:T[]|undefined;total:number;next_cursor:string;loading:boolean;error:string|Error;more_error:boolean};
export class IngressPager<T>{
 private state:Snapshot<T>={items:undefined,total:0,next_cursor:'',loading:false,error:'',more_error:false};private generation=0;
 constructor(private path:string,private project:string,private identity:'id'|'revision'='id'){}
 snapshot(){return {...this.state};}
 invalidate(){this.generation++;}
 first(){return this.read('',false,++this.generation);}
 async more(){if(this.state.loading||!this.state.next_cursor)return;return this.read(this.state.next_cursor,true,this.generation);}
 private async read(cursor:string,append:boolean,generation:number){this.state={...this.state,loading:true,error:'',more_error:append};try{
  const page=await api<Page<T>>(this.path+query(this.project)+'&limit=40'+(cursor?'&cursor='+encodeURIComponent(cursor):''));if(generation!==this.generation)return;
  if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.total<0||page.items.some(item=>{const id=(item as Record<string,unknown>)[this.identity];return this.identity==='id'?typeof id!=='string'||!id:!Number.isSafeInteger(id)||Number(id)<1;}))throw new UiError("automation.no_confirmed_event_ingress_record_range_has_been");
  this.state={items:[...new Map([...(append?this.state.items||[]:[]),...page.items].map(item=>[(item as Record<string,unknown>)[this.identity],item])).values()],total:page.total,next_cursor:page.next_cursor||'',loading:false,error:'',more_error:false};
 }catch(error){if(generation===this.generation)this.state={...this.state,loading:false,error:automationError(error)};}}
}
