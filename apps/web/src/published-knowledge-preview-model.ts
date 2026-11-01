import type {ProjectKnowledgeReference} from '../../../packages/contracts/src';
import type {KnowledgeRevision} from './knowledge-model';
import {currentKnowledgePin,validKnowledgePins,validKnowledgeRevisionDetail,validKnowledgeSummary} from './task-conversation-knowledge-model';
import {UiError} from './i18n';
export type PublishedKnowledgeQualification={eligible:boolean;reason:'current'|'publication_changed'|'retired'|'source_changed'|'unavailable'};
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
