import type {SkillCatalogDetail} from '../../../packages/contracts/src';
import {catalogInteger,validCatalogSummary} from './skill-catalog-model';

const eligibility=new Set(['current','source_changed','publication_changed','retired']);
export function validComparisonRevision(value:unknown):value is SkillCatalogDetail{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const item=value as SkillCatalogDetail;
 return validCatalogSummary(item)&&eligibility.has(item.eligibility)&&typeof item.content==='string'&&!!item.content.trim()&&new TextEncoder().encode(item.content).length<=16000;
}
export function sameFixedRevision(left:SkillCatalogDetail,right:SkillCatalogDetail){
 return left.id===right.id&&left.version===right.version&&left.content===right.content&&left.origin.project_id===right.origin.project_id&&left.origin.knowledge_id===right.origin.knowledge_id&&left.origin.knowledge_version===right.origin.knowledge_version&&left.origin.revision===right.origin.revision&&left.origin.sha256===right.origin.sha256;
}
export function validRevisionPage(value:unknown,current:SkillCatalogDetail):boolean{
 if(!value||typeof value!=='object'||Array.isArray(value)||!validComparisonRevision(current))return false;
 const page=value as {items:unknown[];total:number;next_cursor:string};
 if(!Array.isArray(page.items)||page.items.length>20||!catalogInteger(page.total)||page.total<page.items.length||typeof page.next_cursor!=='string'||page.next_cursor.length>4096||/[\u0000-\u001f\u007f]/.test(page.next_cursor))return false;
 let previous=current.version+1;
 return page.items.every(item=>{
  if(!validComparisonRevision(item)||item.id!==current.id||item.version>=previous||item.version>current.version||item.origin.project_id!==current.origin.project_id||item.origin.knowledge_id!==current.origin.knowledge_id||item.can_adopt||item.allowed_actions.length!==0||item.version===current.version&&!sameFixedRevision(item,current))return false;
  previous=item.version;return true;
 });
}
export function comparisonRevisions(current:SkillCatalogDetail,items:SkillCatalogDetail[]):SkillCatalogDetail[]{
 return [current,...items.filter(item=>item.version!==current.version)].sort((a,b)=>b.version-a.version);
}
export function canAdoptComparedCurrent(current:SkillCatalogDetail,left?:SkillCatalogDetail,right?:SkillCatalogDetail):boolean{
 return !!left&&!!right&&left.version!==right.version&&current.state==='published'&&current.eligibility==='current'&&current.source_current&&current.can_adopt&&current.allowed_actions.includes('adopt')&&(sameFixedRevision(left,current)||sameFixedRevision(right,current));
}
