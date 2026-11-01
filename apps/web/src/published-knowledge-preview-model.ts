import type {KnowledgeIndexLocator,ProjectKnowledgeReference} from '../../../packages/contracts/src';
import type {KnowledgeRevision} from './knowledge-model';
import {currentKnowledgePin,sameKnowledgePin,validKnowledgePins,validKnowledgeRevisionDetail,validKnowledgeSummary} from './task-conversation-knowledge-model';
import {UiError} from './i18n';
export type PublishedKnowledgeQualification={eligible:boolean;reason:'current'|'publication_changed'|'retired'|'source_changed'|'unavailable'};
/** UI-only location input; the original publication reference stays separate. */
export type PublishedKnowledgePreviewLocation={reference:ProjectKnowledgeReference;locator:KnowledgeIndexLocator};
export type PublishedKnowledgeHighlight={start:number;end:number};
export function readPublishedKnowledgeHighlight(value:unknown,reference:ProjectKnowledgeReference,content:string):PublishedKnowledgeHighlight|undefined{
 if(value===undefined)return;
 const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
 const closed=(v:Record<string,unknown>,keys:string[])=>keys.every(key=>Object.hasOwn(v,key))&&Reflect.ownKeys(v).every(key=>typeof key==='string'&&keys.includes(key));
 if(!object(value)||!closed(value,['reference','locator'])||!validKnowledgePins([reference])||!validKnowledgePins([value.reference])||!sameKnowledgePin(value.reference as ProjectKnowledgeReference,reference)||!object(value.locator)||!closed(value.locator,['unit','start','end']))throw new UiError('knowledge-preview.invalid_location');
 const {unit,start,end}=value.locator;
 if(unit!=='unicode_codepoints'||!Number.isSafeInteger(start)||!Number.isSafeInteger(end)||(start as number)<0||(end as number)<=(start as number)||(end as number)>Array.from(content).length)throw new UiError('knowledge-preview.invalid_location');
 return {start:start as number,end:end as number};
}
export function readPublishedKnowledgeRevision(value:unknown,project:string,reference:ProjectKnowledgeReference):KnowledgeRevision{
 if(!validKnowledgePins([reference])||!validKnowledgeRevisionDetail(value,project,reference)||value.snapshot.state!=='published')throw new UiError('knowledge-preview.invalid_read');
 return value.snapshot;
}
export function readPublishedKnowledgeQualification(value:unknown,project:string,reference:ProjectKnowledgeReference):PublishedKnowledgeQualification{
 if(!validKnowledgePins([reference])||!validKnowledgeSummary(value,project)||value.id!==reference.id)throw new UiError('knowledge-preview.invalid_read');
 if(Boolean(currentKnowledgePin(value,project,reference)))return {eligible:true,reason:'current'};
 if(value.state==='retired'||value.eligibility==='retired')return {eligible:false,reason:'retired'};
 if(value.published_revision!==reference.revision||value.published_sha256!==reference.sha256||value.eligibility==='publication_changed')return {eligible:false,reason:'publication_changed'};
 if(value.eligibility==='source_changed')return {eligible:false,reason:'source_changed'};
 return {eligible:false,reason:'unavailable'};
}
