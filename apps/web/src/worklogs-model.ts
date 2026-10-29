import type {Worklog,WorklogPage,WorklogRevisionPage} from '../../../packages/contracts/src';
import type {RecordItem} from './api';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';

export type WorklogMode='create'|'correct'|'void';
export type WorklogDraft={member_id:string;task_id:string;expected_task_version:number;work_date:string;minutes:string;note:string;reason:string;on_behalf_ack:boolean};
export type WorklogRecordPage={items:RecordItem[];total:number;next_cursor?:string};
export const worklogScope=(user:string,organization:string,project:string,week:string)=>JSON.stringify([user,organization,project,week]);
export const worklogRecoveryKey=(scope:string)=>'suma-write-recovery-v1:worklogs:'+scope;
export const worklogObject=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown,min=0):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min;
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_.-]{1,128}$/.test(value);
const text=(value:unknown,max=200):value is string=>typeof value==='string'&&wellFormed(value)&&!value.includes('\0')&&Array.from(value).length<=max;
export const worklogDate=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d{2}-\d{2}$/.test(value)&&value>='0001-01-01'&&Number.isFinite(Date.parse(value+'T00:00:00Z'))&&new Date(value+'T00:00:00Z').toISOString().slice(0,10)===value;
export const worklogWeekEnd=(week:string)=>new Date(Date.parse(week+'T00:00:00Z')+6*86400000).toISOString().slice(0,10);
export const worklogPath=(log='',command='')=>'/worklogs'+(log?'/'+encodeURIComponent(log):'')+(command?'/'+command:'');
export const worklogPagePath=(project:string,week:string,cursor='')=>worklogPath()+'?'+new URLSearchParams({project_id:project,from_date:week,to_date:worklogWeekEnd(week),status:'all',limit:'20',...(cursor?{cursor}:{})});
export const newWorklogDraft=(week:string,self=''):WorklogDraft=>({member_id:self,task_id:'',expected_task_version:0,work_date:week,minutes:'',note:'',reason:'',on_behalf_ack:false});
export const editWorklogDraft=(row:Worklog):WorklogDraft=>({member_id:row.member_id,task_id:row.task_id,expected_task_version:row.task_version,work_date:row.work_date,minutes:String(row.minutes),note:row.note,reason:'',on_behalf_ack:false});
export function worklogDraftError(draft:WorklogDraft,mode:WorklogMode,onBehalf:boolean,canManage:boolean){
 if(mode!=='void'){
  if(!id(draft.member_id))return 'member_required';
  if(!id(draft.task_id)||!integer(draft.expected_task_version,1))return 'task_required';
  if(!worklogDate(draft.work_date))return 'date_invalid';
  if(!/^\d+$/.test(draft.minutes)||!integer(Number(draft.minutes),1)||Number(draft.minutes)>1440)return 'minutes_invalid';
  if(!text(draft.note))return 'note_invalid';
 }
 if(mode!=='create'&&(!text(draft.reason)||!draft.reason.trim()))return 'reason_invalid';
 if(onBehalf&&!canManage)return 'on_behalf_denied';
 if(onBehalf&&!draft.on_behalf_ack)return 'ack_required';
 return '';
}
export function validWorklog(value:unknown,project:string,log=''):value is Worklog{
 if(!worklogObject(value)||!id(value.id)||log&&value.id!==log||value.project_id!==project||!integer(value.version,1)||!['active','voided'].includes(String(value.status))||value.source!=='human_manual')return false;
 return id(value.task_id)&&integer(value.task_version,1)&&text(value.task_title,1000)&&id(value.member_id)&&integer(value.member_version,1)&&text(value.member_name,1000)&&id(value.user_id)&&text(value.user_name,1000)&&worklogDate(value.work_date)&&integer(value.minutes,1)&&value.minutes<=1440&&text(value.note)&&text(value.reason)&&id(value.actor_id)&&text(value.actor_name,1000)&&typeof value.on_behalf_ack==='boolean'&&typeof value.created_at==='string'&&Number.isFinite(Date.parse(value.created_at))&&typeof value.updated_at==='string'&&Number.isFinite(Date.parse(value.updated_at))&&typeof value.can_correct==='boolean'&&typeof value.can_void==='boolean';
}
export function validWorklogPage(value:unknown,project:string):value is WorklogPage{
 if(!worklogObject(value)||value.project_id!==project||!Array.isArray(value.items)||value.items.length>50||!value.items.every(row=>validWorklog(row,project))||new Set(value.items.map(row=>row.id)).size!==value.items.length||!integer(value.total)||value.total<value.items.length||!integer(value.limit,1)||value.limit>50||value.items.length>value.limit||typeof value.next_cursor!=='string'||!integer(value.scope_version)||!worklogObject(value.summary))return false;
 const summary=value.summary;return integer(summary.active_count)&&integer(summary.voided_count)&&summary.active_count+summary.voided_count===value.total&&integer(summary.total_minutes)&&typeof value.can_create==='boolean'&&typeof value.can_manage_others==='boolean'&&(value.self_member_id===''||id(value.self_member_id));
}
export function validWorklogRevisions(value:unknown,project:string,log:string):value is WorklogRevisionPage{return worklogObject(value)&&value.project_id===project&&value.worklog_id===log&&Array.isArray(value.items)&&value.items.length<=50&&value.items.every(row=>validWorklog(row,project,log))&&new Set(value.items.map(row=>row.version)).size===value.items.length&&integer(value.total,1)&&value.total>=value.items.length&&integer(value.limit,1)&&value.limit<=50&&value.items.length<=value.limit&&typeof value.next_cursor==='string'&&integer(value.scope_version);}
export function validWorklogRecord(value:unknown,project:string):value is RecordItem{return worklogObject(value)&&value.project_id===project&&id(value.id)&&integer(value.version,1);}
export function validWorklogRecords(value:unknown,project:string):value is WorklogRecordPage{return worklogObject(value)&&Array.isArray(value.items)&&value.items.length<=100&&value.items.every(row=>validWorklogRecord(row,project))&&integer(value.total)&&value.total>=value.items.length&&(value.next_cursor===undefined||typeof value.next_cursor==='string');}
export function validWorklogRequest(request:WriteRecoveryRequest,project:string,mode:WorklogMode,log=''){
 const body=request.body,expected=mode==='create'?'expected_task_version,idempotency_key,member_id,minutes,note,on_behalf_ack,project_id,task_id,work_date':mode==='correct'?'expected_task_version,expected_version,idempotency_key,minutes,note,on_behalf_ack,project_id,reason,task_id,work_date':'expected_version,idempotency_key,on_behalf_ack,project_id,reason';
 if(request.path!==worklogPath(log,mode==='correct'?'corrections':mode==='void'?'void':'')||request.method!=='POST'||body.project_id!==project||body.idempotency_key!==request.idempotency_key||Object.keys(body).sort().join(',')!==expected||typeof body.on_behalf_ack!=='boolean')return false;
 if(mode!=='create'&&(!integer(body.expected_version,1)||!text(body.reason)||!body.reason.trim()||body.reason!==body.reason.trim()))return false;
 return mode==='void'||id(body.task_id)&&integer(body.expected_task_version,1)&&(mode!=='create'||id(body.member_id))&&worklogDate(body.work_date)&&integer(body.minutes,1)&&body.minutes<=1440&&text(body.note);
}
export function validWorklogReceipt(value:unknown,request:WriteRecoveryRequest,project:string,mode:WorklogMode,log='',member=''){
 if(!validWorklogRequest(request,project,mode,log)||!validWorklog(value,project,log))return false;
 const body=request.body;if(value.version!==(mode==='create'?1:Number(body.expected_version)+1)||value.status!==(mode==='void'?'voided':'active')||value.on_behalf_ack!==body.on_behalf_ack)return false;
 if(mode!=='create'&&(value.member_id!==member||value.reason!==body.reason))return false;
 return mode==='void'||value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.work_date===body.work_date&&value.minutes===body.minutes&&value.note===body.note&&(mode!=='create'||value.member_id===body.member_id);
}
/** One durable slot per actor/project/week keeps an original operation
 * reachable even when its correction moves the head outside the visible week. */
export function worklogRecovery(storage:Pick<Storage,'getItem'>|undefined,scope:string,project:string):{mode:WorklogMode;log:string;request:WriteRecoveryRequest}|{invalid:true}|undefined{
 try{const raw=storage?.getItem(worklogRecoveryKey(scope));if(!raw)return;
  const value:unknown=JSON.parse(raw);if(!worklogObject(value)||Object.keys(value).sort().join(',')!=='body,idempotency_key,method,path'||typeof value.path!=='string'||typeof value.idempotency_key!=='string'||value.idempotency_key.length<8||value.idempotency_key.length>128||/[\x00-\x1f\x7f]/.test(value.idempotency_key)||!worklogObject(value.body))return {invalid:true};
  const route=value.path.match(/^\/worklogs(?:\/([A-Za-z0-9_.-]+)\/(corrections|void))?$/);if(!route)return {invalid:true};
  const mode:WorklogMode=route[2]==='corrections'?'correct':route[2]==='void'?'void':'create',log=route[1]||'',request=value as WriteRecoveryRequest;
  return validWorklogRequest(request,project,mode,log)?{mode,log,request}:{invalid:true};
 }catch{return {invalid:true};}
}
