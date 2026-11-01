import type {ToolProfile,ToolProfileDiagnosticCheck,ToolProfileDiagnostics,ToolProfileMaintenanceResolution} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {sameToolProfile} from './tool-profile-reuse-model';
import {parseToolManifest} from './tool-model';
import {readToolProfileImpact} from './tool-profile-impact-model';
import {validToolMaintenanceStatus} from './tool-maintenance-model';
export type ToolDoctorAction='none'|'review_profile'|'open_worker'|'register_profile'|'refresh';
export function isToolDoctorAction(value:unknown):value is ToolDoctorAction{
 return typeof value==='string' && ['none','review_profile','open_worker','register_profile','refresh'].includes(value);
}
const record=(value:unknown):value is Record<string,unknown>=>!!value && typeof value==='object' && !Array.isArray(value) && [Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key)) && Reflect.ownKeys(value).every(key=>typeof key==='string' && [...required,...optional].includes(key));
function diagnostic(value:unknown):boolean{
 if(value===null)return true;
 return record(value) && closed(value,['schema_version','message_key'],['message_params']) && value.schema_version===1 && typeof value.message_key==='string' && (!Object.hasOwn(value,'message_params') || record(value.message_params) && Object.values(value.message_params).every(param=>typeof param==='string' || typeof param==='number' && Number.isFinite(param)));
}
function observation(value:unknown):boolean{
 if(value===null)return true;
 if(!record(value) || !closed(value,['profile','available','reason'],['model_deployments','codex_catalog','workspace_observation_protocol','workspace_write_protocol']))return false;
 try{parseToolManifest(JSON.stringify([value]));return true;}catch{return false;}
}
function validTool(value:unknown,expected:{project:string;toolID:string}):value is ToolProfile{
 if(!record(value) || !closed(value,['id','project_id','worker_id','name','version','state','profile','enabled_by','created_at','updated_at','available','reason'],['observation','last_seen_at','reason_diagnostic','maintenance']))return false;
 return value.id===expected.toolID && value.project_id===expected.project && typeof value.id==='string' && /^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(value.id) && typeof value.worker_id==='string' && !!value.worker_id && Number.isSafeInteger(value.version) && (value.version as number)>0 && typeof value.state==='string' && ['paused','enabled','revoked'].includes(value.state) && sameToolProfile(value.profile,value.profile) && typeof value.available==='boolean' && ['name','enabled_by','created_at','updated_at','reason'].every(key=>typeof value[key]==='string') && (!Object.hasOwn(value,'last_seen_at') || typeof value.last_seen_at==='string') && (!Object.hasOwn(value,'reason_diagnostic') || diagnostic(value.reason_diagnostic)) && (!Object.hasOwn(value,'observation') || observation(value.observation)) && (!Object.hasOwn(value,'maintenance') || validToolMaintenanceStatus(value.maintenance));
}
function check(value:unknown,code:string):value is ToolProfileDiagnosticCheck{
 return record(value) && closed(value,['code','status','reason','next_action'],['reason_diagnostic']) && value.code===code && typeof value.status==='string' && ['pass','blocked','not_checked'].includes(value.status) && typeof value.reason==='string' && isToolDoctorAction(value.next_action) && (!Object.hasOwn(value,'reason_diagnostic') || diagnostic(value.reason_diagnostic));
}
const maintenanceReasons={
 inspect:['maintenance_supported'],
 view_inspection:['inspection_pending','inspection_claimed','inspection_unknown'],
 recover_inspection:['inspection_unknown','recovery_expired'],
 view_recovery:['recovery_requested','recovery_proof_stored'],
 open_worker:['maintenance_expired','maintenance_unsupported','maintenance_unknown','worker_unavailable'],
 none:['profile_revoked'],
} satisfies Record<ToolProfileMaintenanceResolution['action'],readonly ToolProfileMaintenanceResolution['reason'][]>;
const inspectionID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$/.test(value);
/** A recommendation locates an original operation. It cannot carry commands,
 * authorize a repair, or replace the source stored in a local write journal. */
function maintenanceResolution(value:unknown,tool:ToolProfile):value is ToolProfileMaintenanceResolution{
 if(!record(value)||!closed(value,['profile_id','profile_version','worker_id','action','reason'],['inspection_id','recovery_id'])||value.profile_id!==tool.id||value.profile_version!==tool.version||value.worker_id!==tool.worker_id||typeof value.action!=='string'||!Object.hasOwn(maintenanceReasons,value.action))return false;
 const action=value.action as ToolProfileMaintenanceResolution['action'];
 if(!(maintenanceReasons[action] as readonly unknown[]).includes(value.reason))return false;
 const hasInspection=Object.hasOwn(value,'inspection_id'),hasRecovery=Object.hasOwn(value,'recovery_id');
 if(action==='view_recovery')return hasInspection&&inspectionID(value.inspection_id)&&hasRecovery&&inspectionID(value.recovery_id);
 if(action==='view_inspection'||action==='recover_inspection')return hasInspection&&inspectionID(value.inspection_id)&&!hasRecovery;
 return !hasInspection&&!hasRecovery;
}
/** UTC instants must describe an actual calendar date, not Date.parse rollover. */
function instant(value:unknown):number|undefined{
 if(typeof value!=='string' || !/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value))return;
 const base=value.slice(0,19)+'Z',whole=Date.parse(base);
 if(!Number.isFinite(whole) || new Date(whole).toISOString().slice(0,19)!==value.slice(0,19))return;
 return whole+(value[19]==='.'?Number('0'+value.slice(19,-1))*1000:0);
}
function validReport(value:unknown,expected:{project:string;toolID:string}):value is ToolProfileDiagnostics{
 return record(value) && closed(value,['tool','checked_at','expires_at','checks','next_action','maintenance_resolution'],['impact']) && (!Object.hasOwn(value,'impact') || !!readToolProfileImpact(value.impact)) && validTool(value.tool,expected) && maintenanceResolution(value.maintenance_resolution,value.tool) && isToolDoctorAction(value.next_action) && Array.isArray(value.checks) && value.checks.length===4 && ['authorization','worker','inventory','profile'].every((code,index)=>check(value.checks instanceof Array?value.checks[index]:undefined,code));
}
/** The caller deducts elapsed request time using its monotonic clock. No action
 * or dispatch eligibility is inferred from a reason or presentation identity. */
export function readToolProfileDiagnostics(value:unknown,expected:{project:string;toolID:string}):{report:ToolProfileDiagnostics;remainingMs:number}{
 if(validReport(value,expected)){
  const checked=instant(value.checked_at),expires=instant(value.expires_at);
  if(checked!==undefined && expires!==undefined && expires>=checked && expires-checked<=60000)return {report:value,remainingMs:expires-checked};
 }
 throw new UiError('tool-doctor.invalid_response');
}
