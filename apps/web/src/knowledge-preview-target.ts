import type {ProjectKnowledgeDiscoveryHit,ProjectKnowledgeIndexHit,ProjectKnowledgeReference} from '../../../packages/contracts/src';
import type {PublishedKnowledgePreviewLocation} from './published-knowledge-preview-model';
import {validKnowledgePins} from './task-conversation-knowledge-model';

export type KnowledgePreviewTarget={reference:ProjectKnowledgeReference;location?:PublishedKnowledgePreviewLocation};

/** Index location is presentation context, never a task knowledge reference. */
export function knowledgePreviewTarget(hit:ProjectKnowledgeDiscoveryHit|ProjectKnowledgeIndexHit,project:string):KnowledgePreviewTarget|undefined{
 if(hit.project_id!==project||!validKnowledgePins([hit.reference]))return;
 const reference={...hit.reference};
 return 'locator' in hit?{reference,location:{reference:{...reference},locator:{...hit.locator}}}:{reference};
}
