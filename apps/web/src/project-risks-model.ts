import type {ProjectRisk,ProjectRiskFields,ProjectRiskPage,ProjectRiskReview,ProjectRiskReviewInput,ProjectRiskSummary,ProjectRiskView} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';

export const riskStates=['open','mitigating','in_review','closed'] as const;
export const riskStrategies=['','mitigate','avoid','transfer','accept'] as const;
export const riskDispositions=['mitigated','accepted','avoided','transferred','not_applicable'] as const;
export const riskCommands=['start','submit_review','return','close','reopen'] as const;
export type RiskCommand=typeof riskCommands[number];
export const riskPath=(project:string,id='')=>'/risks'+(id?'/'+encodeURIComponent(id):'')+'?project_id='+encodeURIComponent(project);
export const riskHref=(project:string,id='')=>'?'+new URLSearchParams({project,view:'development',sub:'planning',layout:'risks',...(id?{panel:'project-risk:'+id}:{})});
export const riskRecordHref=(project:string,kind:string,id:string)=>'?'+new URLSearchParams({project,view:kind==='requirement'?'requirements':kind==='issue'?'quality':'development',...(kind==='requirement'?{}:{sub:kind==='issue'?'issues':'tasks'}),record:id,status:'all',layout:'list'});
export const riskRunHref=(project:string,id:string)=>'?'+new URLSearchParams({project,view:'development',sub:'runs',run:id});
export const riskObject=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const riskID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,100}$/.test(value);
export const riskText=(value:unknown,max=2000,empty=false):value is string=>typeof value==='string'&&(empty||!!value.trim())&&wellFormed(value)&&new TextEncoder().encode(value).length<=max;
const integer=(value:unknown,min=0):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min;
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const date=(value:unknown)=>value===''||typeof value==='string'&&/^\d{4}-\d{2}-\d{2}$/.test(value)&&!Number.isNaN(Date.parse(value))&&new Date(value+'T00:00:00Z').toISOString().slice(0,10)===value;
const ordinal=(value:unknown):value is number|null=>value===null||integer(value,1)&&value<=5;
const enumValue=(value:unknown,allowed:readonly string[])=>typeof value==='string'&&allowed.includes(value);
const same=(left:unknown,right:unknown)=>JSON.stringify(left)===JSON.stringify(right);
const linkKey=(value:ProjectRiskFields['links'][number])=>value.kind+':'+value.id;
const sameLinks=(left:ProjectRiskFields['links'],right:ProjectRiskFields['links'])=>same(left.map(linkKey).sort(),right.map(linkKey).sort());
const closed=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const fieldNames=['title','detail','owner_id','review_date','strategy','plan','probability','impact','links'];

export function validRiskFields(value:unknown):value is ProjectRiskFields{
 if(!riskObject(value)||!riskText(value.title,500)||!riskText(value.detail,8000)||!(value.owner_id===''||riskID(value.owner_id))||!date(value.review_date)||!enumValue(value.strategy,riskStrategies)||!riskText(value.plan,8000,true)||!ordinal(value.probability)||!ordinal(value.impact)||!Array.isArray(value.links)||value.links.length>32)return false;
 return value.links.every(item=>riskObject(item)&&closed(item,['kind','id'])&&enumValue(item.kind,['task','requirement','issue'])&&riskID(item.id))&&new Set(value.links.map(item=>linkKey(item as ProjectRiskFields['links'][number]))).size===value.links.length;
}
function validSource(value:unknown):boolean{return riskObject(value)&&riskID(value.alert_id)&&integer(value.alert_version,1)&&riskID(value.analysis_id)&&riskID(value.run_id)&&riskText(value.risk_key,100)&&hash(value.source_sha256)&&hash(value.original_output_sha256)&&riskID(value.adopted_by)&&riskText(value.adopted_at,100);}
export function validProjectRisk(value:unknown,project:string,id?:string):value is ProjectRisk{
 if(!riskObject(value)||!validRiskFields(value)||!riskID(value.id)||(id!==undefined&&value.id!==id)||value.project_id!==project||!riskID(value.tenant_id)||!integer(value.version,1)||!integer(value.cycle,1)||!integer(value.assessment_revision,1)||!enumValue(value.state,riskStates)||!riskID(value.created_by)||!riskText(value.created_at,100)||!riskText(value.updated_at,100)||!(value.current_review_id===undefined||riskID(value.current_review_id))||value.source!=null&&!validSource(value.source))return false;
 return value.score===(value.probability!==null&&value.impact!==null?value.probability*value.impact:null);
}
export function validRiskReviewInput(value:unknown):value is ProjectRiskReviewInput{return riskObject(value)&&closed(value,['disposition','residual_probability','residual_impact','rationale','evidence'])&&enumValue(value.disposition,riskDispositions)&&ordinal(value.residual_probability)&&ordinal(value.residual_impact)&&riskText(value.rationale,4000)&&riskText(value.evidence,8000);}
function validReview(value:unknown):value is ProjectRiskReview{
 if(!riskObject(value)||!validRiskReviewInput(Object.fromEntries(['disposition','residual_probability','residual_impact','rationale','evidence'].map(key=>[key,value[key]])))||!riskID(value.id)||!integer(value.cycle,1)||!integer(value.assessment_revision,1)||!hash(value.assessment_sha256)||!riskID(value.owner_id)||!integer(value.owner_version,1)||!riskID(value.owner_user_id)||!Array.isArray(value.links)||value.links.length>32||!riskID(value.submitted_by)||!riskText(value.submitted_at,100)||!enumValue(value.decision,['pending','returned','closed']))return false;
 if(!value.links.every(item=>riskObject(item)&&enumValue(item.kind,['task','requirement','issue'])&&riskID(item.id)&&integer(item.version,1)&&(item.verified_run_id===undefined||riskID(item.verified_run_id))&&(item.review_id===undefined||riskID(item.review_id))))return false;
 if(!['decided_by','decided_at','decision_reason'].every(key=>value[key]===undefined||typeof value[key]==='string'))return false;
 return value.residual_score===(value.residual_probability!==null&&value.residual_impact!==null?Number(value.residual_probability)*Number(value.residual_impact):null);
}
/** Runtime checks bind each response to its exact project and selected risk.
 * Server readiness controls lifecycle actions; list status is never evidence. */
export function validRiskView(value:unknown,project:string,id?:string):value is ProjectRiskView{
 if(!riskObject(value))return false;const readiness=value.readiness;
 if(!validProjectRisk(value.risk,project,id)||!Array.isArray(value.history)||value.history.length>10||!integer(value.history_total)||value.history_total<value.history.length||typeof value.history_complete!=='boolean'||value.history_complete&&value.history_total!==value.history.length||!Array.isArray(value.allowed_actions)||!value.allowed_actions.every(action=>enumValue(action,['save',...riskCommands]))||!riskObject(readiness)||!['assessment_ready','mitigation_ready','close_ready'].every(key=>typeof readiness[key]==='boolean')||!Array.isArray(readiness.blockers)||readiness.blockers.length>100||!readiness.blockers.every(item=>riskObject(item)&&typeof item.code==='string'&&/^[a-z][a-z0-9_]{0,99}$/.test(item.code)&&(item.ref_id===undefined||riskID(item.ref_id)))||value.created!==undefined&&typeof value.created!=='boolean')return false;
 if(value.current_review!=null&&(!validReview(value.current_review)||value.current_review.id!==value.risk.current_review_id))return false;
 if(value.risk.current_review_id&&!value.current_review)return false;
 return value.history.every(validReview)&&new Set(value.history.map(item=>item.id)).size===value.history.length;
}
export function validRiskPage(value:unknown,project:string):value is ProjectRiskPage{return riskObject(value)&&Array.isArray(value.items)&&value.items.length<=100&&value.items.every(item=>validProjectRisk(item,project))&&integer(value.total)&&value.total>=value.items.length&&typeof value.next_cursor==='string'&&value.next_cursor.length<=4096&&typeof value.can_create==='boolean'&&new Set(value.items.map(item=>item.id)).size===value.items.length;}
export function validRiskSummary(value:unknown,project:string):value is ProjectRiskSummary{
 return riskObject(value)&&value.project_id===project&&riskText(value.as_of,100)&&['total','active_count','open_count','mitigating_count','in_review_count','closed_count','unassessed_count','overdue_count'].every(key=>integer(value[key]))&&Array.isArray(value.matrix)&&value.matrix.length<=25&&value.matrix.every(item=>riskObject(item)&&integer(item.probability,1)&&item.probability<=5&&integer(item.impact,1)&&item.impact<=5&&integer(item.count))&&value.matrix_scope==='active';
}
export function blankRiskFields():ProjectRiskFields{return {title:'',detail:'',owner_id:'',review_date:'',strategy:'',plan:'',probability:null,impact:null,links:[]};}
export function riskFields(risk:ProjectRisk):ProjectRiskFields{return Object.fromEntries(fieldNames.map(key=>[key,risk[key as keyof ProjectRisk]])) as unknown as ProjectRiskFields;}

/** Only these existing domain commands can be recovered from session storage.
 * The original immutable request is retained when a write outcome is unknown. */
export function validRiskRequest(request:WriteRecoveryRequest,project:string,id?:string,alertID?:string):boolean{
 const body=request.body;if(body.project_id!==project||!riskText(body.reason))return false;
 const common=['project_id','reason','idempotency_key'];
 if(!id){return request.method==='POST'&&request.path==='/risks'&&closed(body,[...common,...fieldNames,'source_alert_id','expected_alert_version'])&&validRiskFields(body)&&(alertID?body.source_alert_id===alertID&&integer(body.expected_alert_version,1):body.source_alert_id===undefined&&body.expected_alert_version===undefined);}
 if(!integer(body.expected_version,1))return false;
 if(request.path==='/risks/'+encodeURIComponent(id)&&request.method==='PATCH')return closed(body,[...common,...fieldNames,'expected_version'])&&validRiskFields(body);
 if(request.path!=='/risks/'+encodeURIComponent(id)+'/commands'||request.method!=='POST'||!closed(body,[...common,'expected_version','action','review','expected_review_id'])||!enumValue(body.action,riskCommands))return false;
 if(body.action==='submit_review')return validRiskReviewInput(body.review)&&body.expected_review_id===undefined;
 if(body.action==='close')return riskID(body.expected_review_id)&&body.review===undefined;
 return body.review===undefined&&body.expected_review_id===undefined;
}
function appliedFields(risk:ProjectRisk,body:Record<string,unknown>){return fieldNames.filter(key=>key!=='links').every(key=>same(risk[key as keyof ProjectRisk],body[key]))&&sameLinks(risk.links,body.links as ProjectRiskFields['links']);}
export function validRiskResponse(value:unknown,request:WriteRecoveryRequest,project:string,id?:string,alertID?:string):boolean{
 if(!validRiskRequest(request,project,id,alertID)||!validRiskView(value,project,id))return false;
 const body=request.body,risk=value.risk,currentReview=value.current_review;
 if(!id){if(typeof value.created!=='boolean')return false;if(alertID){if(risk.source?.alert_id!==alertID)return false;if(value.created&&risk.source.alert_version!==body.expected_alert_version)return false;}else if(risk.source||!value.created)return false;return !value.created||risk.version===1&&appliedFields(risk,body);}
 if(risk.version!==Number(body.expected_version)+1)return false;
 if(request.method==='PATCH')return appliedFields(risk,body);
 if(body.action==='submit_review')return risk.state==='in_review'&&!!currentReview&&Object.entries(body.review as Record<string,unknown>).every(([key,item])=>same(currentReview[key as keyof ProjectRiskReview],item));
 if(body.action==='close')return risk.state==='closed'&&!!currentReview&&currentReview.id===body.expected_review_id&&currentReview.decision==='closed'&&currentReview.decision_reason===body.reason;
 if(body.action==='return')return risk.state==='mitigating'&&!value.current_review&&value.history.some(item=>item.decision==='returned'&&item.decision_reason===body.reason);
 return body.action==='start'?risk.state==='mitigating':risk.state==='open'&&!value.current_review;
}
