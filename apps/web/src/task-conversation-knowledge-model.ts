import type {ProjectKnowledgeContextItem, ProjectKnowledgeRevisionDetail, TaskConversationKnowledgeReference} from '../../../packages/contracts/src';
import type {KnowledgeReference, KnowledgeRevision, KnowledgeSummary} from './knowledge-model';
import {publishedKnowledgeReference} from './knowledge-model';

export const conversationKnowledgeLimit = 16;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const closed = (value: Record<string, unknown>, names: string[]) => Object.keys(value).sort().join(',') === [...names].sort().join(',');
const id = (value: unknown): value is string => typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const integer = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 && value <= 2147483647;
const positive = (value: unknown): value is number => integer(value) && value > 0;
const text = (value: unknown): value is string => typeof value === 'string';
const hash = (value: unknown): value is string => text(value) && /^[a-f0-9]{64}$/.test(value);
const kind = (value: unknown) => typeof value === 'string' && ['memory', 'spec', 'skill'].includes(value);
export const sameKnowledgePin = (left: KnowledgeReference, right: KnowledgeReference) => left.id === right.id && left.revision === right.revision && left.sha256 === right.sha256;
export const sameKnowledgePins = (left: readonly KnowledgeReference[], right: readonly KnowledgeReference[]) => left.length === right.length && left.every((pin, index) => sameKnowledgePin(pin, right[index]));
export const knowledgeSelectionKey = (pins: readonly KnowledgeReference[], refresh: number) => JSON.stringify([refresh, pins]);
export const knowledgeDetailPath = (project: string, knowledge: string) => '/project-knowledge/' + encodeURIComponent(knowledge) + '?' + new URLSearchParams({project_id: project});
export const knowledgeRevisionPath = (project: string, pin: KnowledgeReference) => '/project-knowledge/' + encodeURIComponent(pin.id) + '/revisions/' + pin.revision + '?' + new URLSearchParams({project_id: project});

export function validKnowledgePins(value: unknown): value is KnowledgeReference[] {
  return Array.isArray(value) && value.length <= conversationKnowledgeLimit && value.every(pin => object(pin) &&
    closed(pin, ['id', 'revision', 'sha256']) && id(pin.id) && positive(pin.revision) && hash(pin.sha256)) && new Set(value.map(pin => pin.id)).size === value.length;
}
export function validConversationKnowledge(value: unknown): value is TaskConversationKnowledgeReference[] {
  return Array.isArray(value) && value.length > 0 && value.length <= conversationKnowledgeLimit && value.every(pin => object(pin) &&
    closed(pin, ['id', 'revision', 'sha256', 'title_at_post', 'kind_at_post']) && id(pin.id) && positive(pin.revision) &&
    hash(pin.sha256) && text(pin.title_at_post) && kind(pin.kind_at_post)) && new Set(value.map(pin => pin.id)).size === value.length;
}
export function validKnowledgeSummary(value: unknown, project: string): value is KnowledgeSummary {
  return object(value) && id(value.id) && value.project_id === project && positive(value.version) && kind(value.kind) &&
    ['draft', 'published', 'retired'].includes(String(value.state)) && text(value.title) && text(value.summary) && positive(value.latest_revision) &&
    integer(value.published_revision) && text(value.published_sha256) && (value.published_revision === 0 ? value.published_sha256 === '' : hash(value.published_sha256)) &&
    text(value.published_title) && text(value.published_summary) && typeof value.has_draft === 'boolean' && typeof value.published_eligible === 'boolean' &&
    ['none', 'current', 'source_changed', 'retired', 'publication_changed'].includes(String(value.eligibility));
}
/** A current entry can qualify a pin, but can never upgrade that pin. */
export function currentKnowledgePin(value: unknown, project: string, pin: KnowledgeReference): value is KnowledgeSummary {
  if (!validKnowledgeSummary(value, project) || value.id !== pin.id) return false;
  try { return sameKnowledgePin(publishedKnowledgeReference(value, project), pin); } catch { return false; }
}
const source = (value: unknown, quote: boolean) => object(value) && ['documents', 'requirements', 'tasks', 'issues'].includes(String(value.kind)) &&
  id(value.id) && positive(value.version) && text(value.title) && hash(value.sha256) && (!quote || text(value.quote));
const revision = (value: unknown): value is KnowledgeRevision => object(value) && closed(value, ['revision', 'based_on_revision', 'state', 'kind', 'title', 'summary', 'content', 'sha256', 'sources', 'method', 'created_by', 'created_at']) &&
  positive(value.revision) && integer(value.based_on_revision) && value.based_on_revision < value.revision &&
  ['draft', 'published', 'retired'].includes(String(value.state)) && kind(value.kind) && text(value.title) && text(value.summary) && text(value.content) && hash(value.sha256) &&
  Array.isArray(value.sources) && value.sources.every(item => source(item, true)) && ['manual', 'extractive', 'catalog_adoption', 'model_assisted'].includes(String(value.method)) && id(value.created_by) && text(value.created_at) && Number.isFinite(Date.parse(value.created_at));
export function validKnowledgeRevisionDetail(value: unknown, project: string, pin: KnowledgeReference): value is ProjectKnowledgeRevisionDetail & {snapshot: KnowledgeRevision} {
  return object(value) && closed(value, ['project_id', 'knowledge_id', 'snapshot']) && value.project_id === project && value.knowledge_id === pin.id &&
    revision(value.snapshot) && value.snapshot.revision === pin.revision && value.snapshot.sha256 === pin.sha256;
}
// Validate the fields consumed by the shared immutable-facts renderer, including
// optional lineage. No source body is resolved again from the current catalog.
const lineage = (value: Record<string, unknown>) => {
  const model = value.model_adoption, catalog = value.catalog_adoption;
  return (model == null || object(model) && id(model.project_id) && id(model.analysis_id) && positive(model.analysis_version) && text(model.run_id) &&
    Array.isArray(model.source_indices) && model.source_indices.every(integer) && hash(model.source_sha256) && hash(model.original_output_sha256) && hash(model.tool_profile_sha256) &&
    positive(model.revision) && id(model.knowledge_id) && hash(model.sha256) && text(model.adopted_by) && text(model.adopted_at) && text(model.reason)) &&
    (catalog == null || object(catalog) && id(catalog.catalog_id) && positive(catalog.catalog_version) && object(catalog.origin) &&
      id(catalog.origin.project_id) && id(catalog.origin.knowledge_id) && positive(catalog.origin.revision) && hash(catalog.origin.sha256) && text(catalog.adopted_by) && text(catalog.adopted_at) && text(catalog.reason));
};
export function validKnowledgeContext(value: unknown): value is ProjectKnowledgeContextItem[] {
  return Array.isArray(value) && value.length <= conversationKnowledgeLimit && value.every(item => object(item) &&
    id(item.id) && positive(item.revision) && hash(item.sha256) && kind(item.kind) && text(item.title) && text(item.summary) && text(item.content) &&
    Array.isArray(item.sources) && item.sources.every(value => source(value, false)) && lineage(item)) && new Set(value.map(item => item.id)).size === value.length;
}
/** Preserve message order, deduplicate identical pins, and expose conflicting
 * revisions as an error rather than choosing a publication on the caller's behalf. */
export function conversationKnowledgeUnion(messages: readonly {knowledge?: readonly KnowledgeReference[]}[]): KnowledgeReference[] | undefined {
  const pins: KnowledgeReference[] = [];
  for (const message of messages) for (const pin of message.knowledge || []) {
    const prior = pins.find(item => item.id === pin.id);
    if (prior && !sameKnowledgePin(prior, pin)) return;
    if (!prior) pins.push({id: pin.id, revision: pin.revision, sha256: pin.sha256});
    if (pins.length > conversationKnowledgeLimit) return;
  }
  return pins;
}
