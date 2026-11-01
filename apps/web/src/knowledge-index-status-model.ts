import type {KnowledgeIndexReceipt,KnowledgeIndexSource,ProjectKnowledgeIndexStatus,ProjectKnowledgeProjectionEvent} from '../../../packages/contracts/src';
import {UiError} from './i18n';

export type KnowledgeIndexStatusScope={project:string;organizationID:string};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const closed=(v:Record<string,unknown>,keys:string[])=>keys.every(key=>Object.hasOwn(v,key))&&Reflect.ownKeys(v).every(key=>typeof key==='string'&&keys.includes(key));
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&new TextEncoder().encode(v).length>0&&new TextEncoder().encode(v).length<=max&&!/^\p{White_Space}|\p{White_Space}$/u.test(v)&&!/[\p{Cc}]|[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(v);
const integer=(v:unknown,min=0,max=Number.MAX_SAFE_INTEGER):v is number=>Number.isSafeInteger(v)&&(v as number)>=min&&(v as number)<=max;
const digest=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
function timestamp(v:unknown):v is string{
 if(typeof v!=='string')return false;const match=v.match(/^(\d{4})-(\d\d)-(\d\d)T(\d\d):(\d\d):(\d\d)(?:\.\d{1,9})?(?:Z|[+-](\d\d):(\d\d))$/);if(!match)return false;
 const [,year,month,day,hour,minute,second,zoneHour='0',zoneMinute='0']=match,y=Number(year),m=Number(month),days=[31,y%4===0&&(y%100!==0||y%400===0)?29:28,31,30,31,30,31,31,30,31,30,31];
 return m>=1&&m<=12&&Number(day)>=1&&Number(day)<=days[m-1]&&Number(hour)<=23&&Number(minute)<=59&&Number(second)<=59&&Number(zoneHour)<=23&&Number(zoneMinute)<=59;
}
function source(v:unknown,action:'upsert'|'delete'):v is KnowledgeIndexSource{
 return object(v)&&closed(v,['kind','id','sequence','revision','sha256'])&&v.kind==='project_knowledge'&&text(v.id,64)&&integer(v.sequence,1,2147483647)&&(action==='delete'&&v.revision===0&&v.sha256===''||integer(v.revision,1,2147483647)&&digest(v.sha256));
}
function event(v:unknown):v is ProjectKnowledgeProjectionEvent{
 return object(v)&&closed(v,['event_id','source','action','state','created_at'])&&text(v.event_id,64)&&(v.action==='upsert'||v.action==='delete')&&source(v.source,v.action)&&['pending','indexed','failed'].includes(String(v.state))&&timestamp(v.created_at);
}
function receipt(v:unknown,scope:KnowledgeIndexStatusScope):v is KnowledgeIndexReceipt{
 return object(v)&&closed(v,['protocol','event_id','tenant_id','project_id','source','action','disposition','generation','index_watermark','request_sha256'])&&v.protocol==='suma-knowledge-index-v1'&&text(v.event_id,64)&&v.tenant_id===scope.organizationID&&v.project_id===scope.project&&(v.action==='upsert'||v.action==='delete')&&source(v.source,v.action)&&['applied','duplicate','obsolete'].includes(String(v.disposition))&&text(v.generation,128)&&integer(v.index_watermark)&&digest(v.request_sha256);
}
export function knowledgeIndexStatusPath(project:string){if(!text(project,64))throw new UiError('knowledge-index-status.invalid_response');return '/project-knowledge/index-status?'+new URLSearchParams({project_id:project});}
/** Source scope and receipt bytes are facts, never provider coverage or a new
 * authorization decision. Latest event and last acknowledgement are independent. */
export function readKnowledgeIndexStatus(value:unknown,scope:KnowledgeIndexStatusScope):ProjectKnowledgeIndexStatus{
 if(text(scope.project,64)&&text(scope.organizationID,64)&&object(value)&&closed(value,['domain','as_of','configured','coverage','pending','indexed','failed','total','current_publications','untracked_publications','latest_event','last_ack'])&&value.domain==='project_knowledge'&&timestamp(value.as_of)&&typeof value.configured==='boolean'&&value.coverage==='outbox_delivery_only'&&integer(value.pending)&&integer(value.indexed)&&integer(value.failed)&&integer(value.total)&&value.pending+value.indexed+value.failed===value.total&&integer(value.current_publications)&&integer(value.untracked_publications)&&value.untracked_publications<=value.current_publications&&(value.latest_event===null||event(value.latest_event))&&(value.last_ack===null||receipt(value.last_ack,scope)))return value as unknown as ProjectKnowledgeIndexStatus;
 throw new UiError('knowledge-index-status.invalid_response');
}
