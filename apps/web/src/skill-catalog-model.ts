import type {SkillCatalogSummary} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export type SkillCatalogFilters={q:string;domain:string;scenario:string;state:'all'|'published'|'retired'};
export const catalogID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
export const catalogInteger=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
export const catalogText=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value.trim()&&new TextEncoder().encode(value).length<=max;
export const catalogSHA=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
export function catalogTerms(raw:string,max:number,bytes:number,required=false){
 const values=raw.split(/[,，\n]/).map(value=>value.trim()).filter(Boolean);
 return values.length<=max&&(!required||values.length>0)&&new Set(values).size===values.length&&values.every(value=>catalogText(value,bytes))?values:undefined;
}
export function catalogListPath(project:string,filters:SkillCatalogFilters){
 const values=new URLSearchParams({project_id:project,limit:'20'});
 for(const name of ['q','domain','scenario'] as const)if(filters[name])values.set(name,filters[name]);
 if(filters.state!=='all')values.set('state',filters.state);
 return '/skill-catalog?'+values;
}
export function catalogHref(project:string,sourceId=''){
 return '?'+new URLSearchParams({project,view:'knowledge',sub:'skill-catalog',...(sourceId?{catalog_source:sourceId}:{})});
}
export function validCatalogSummary(item:SkillCatalogSummary){
 return catalogID(item.id)&&catalogInteger(item.version)&&item.version>0&&['published','retired'].includes(item.state)&&typeof item.title==='string'&&typeof item.summary==='string'&&typeof item.domain==='string'&&Array.isArray(item.scenarios)&&Array.isArray(item.tags)&&!!item.origin&&catalogID(item.origin.project_id)&&catalogID(item.origin.knowledge_id)&&catalogInteger(item.origin.knowledge_version)&&item.origin.knowledge_version>0&&catalogInteger(item.origin.revision)&&item.origin.revision>0&&catalogSHA(item.origin.sha256)&&typeof item.source_current==='boolean'&&typeof item.can_manage==='boolean'&&typeof item.can_adopt==='boolean'&&Array.isArray(item.allowed_actions);
}
export function catalogRecoveryValid(request:WriteRecoveryRequest,project:string){
 const b=request.body,keys=Object.keys(b),expectedVersion=b.expected_version;
 if(b.project_id!==project||!catalogText(b.reason,2000)||!catalogInteger(expectedVersion))return false;
 if(request.path==='/skill-catalog'&&request.method==='POST'||/^\/skill-catalog\/[A-Za-z0-9_-]+$/.test(request.path)&&request.method==='PATCH'){
  return keys.every(key=>['project_id','knowledge_id','expected_knowledge_version','published_revision','published_sha256','expected_version','domain','scenarios','tags','reason','acknowledge_sharing','idempotency_key'].includes(key))&&catalogID(b.knowledge_id)&&catalogInteger(b.expected_knowledge_version)&&b.expected_knowledge_version>0&&catalogInteger(b.published_revision)&&b.published_revision>0&&catalogSHA(b.published_sha256)&&catalogText(b.domain,80)&&Array.isArray(b.scenarios)&&b.scenarios.length>0&&b.scenarios.length<=8&&b.scenarios.every(value=>catalogText(value,80))&&new Set(b.scenarios).size===b.scenarios.length&&Array.isArray(b.tags)&&b.tags.length<=16&&b.tags.every(value=>catalogText(value,40))&&new Set(b.tags).size===b.tags.length&&b.acknowledge_sharing===true&&(request.path==='/skill-catalog'?expectedVersion===0:expectedVersion>0);
 }
 if(request.method!=='POST'||expectedVersion===0)return false;
 if(/^\/skill-catalog\/[A-Za-z0-9_-]+\/commands$/.test(request.path))return keys.every(key=>['project_id','expected_version','action','reason','idempotency_key'].includes(key))&&b.action==='retire';
 return /^\/skill-catalog\/[A-Za-z0-9_-]+\/adoptions$/.test(request.path)&&keys.every(key=>['project_id','expected_version','origin_sha256','reason','acknowledge_draft','idempotency_key'].includes(key))&&catalogSHA(b.origin_sha256)&&b.acknowledge_draft===true;
}
