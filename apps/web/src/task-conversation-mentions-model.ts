import type {TaskConversationMentionTargetPage} from '../../../packages/contracts/src';
import type {KnowledgeReference} from './knowledge-model';
import {validKnowledgePins} from './task-conversation-knowledge-model';

export const mentionLimit = 20;
export const mentionID = (value: unknown): value is string =>
  typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const object = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value);
const natural = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
export const validMentionQuery = (value: string) => Array.from(value).length <= 200 && !/[\p{Cc}\uD800-\uDFFF]/u.test(value);
export function validMentionResourceIDs(value: unknown): value is string[] {
  return Array.isArray(value) && value.length <= mentionLimit && value.every(mentionID) && new Set(value).size === value.length;
}
export function mentionTargetsPath(project: string, task: string, q = '', cursor = '', selected?: string[]) {
  const params = new URLSearchParams({project_id: project, task_id: task, limit: String(mentionLimit)});
  if (selected) params.set('resource_ids', selected.join(','));
  else { if (q) params.set('q', q); if (cursor) params.set('cursor', cursor); }
  return '/task-conversation-mention-targets?' + params;
}
/** Selected-ID hydration returns the currently authorized subset, in the
 * requested order. Absence is visible unavailability, never a new recipient. */
export function validMentionTargetPage(value: unknown, project: string, task: string, selected?: string[]): value is TaskConversationMentionTargetPage {
  if (!object(value) || value.project_id !== project || value.task_id !== task || value.limit !== mentionLimit ||
    !natural(value.total) || !Array.isArray(value.items) || value.items.length > mentionLimit || value.items.length > value.total ||
    typeof value.next_cursor !== 'string' || value.next_cursor.length > 4096 || /[\p{Cc}]/u.test(value.next_cursor) ||
    !!value.next_cursor && !value.items.length) return false;
  const seen = new Set<string>(), users = new Set<string>(); let previous = -1;
  for (const row of value.items) {
    if (!object(row) || !mentionID(row.resource_id) || !mentionID(row.user_id) || typeof row.name !== 'string' ||
      !natural(row.resource_version) || row.resource_version < 1 || row.resource_version > 2147483647 ||
      seen.has(row.resource_id) || users.has(row.user_id)) return false;
    if (selected) { const position = selected.indexOf(row.resource_id); if (position <= previous) return false; previous = position; }
    seen.add(row.resource_id); users.add(row.user_id);
  }
  return !selected || value.next_cursor === '' && value.total === value.items.length;
}

export type ConversationDraft = {body: string; mention_resource_ids: string[]; knowledge?: KnowledgeReference[]};
type DraftStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;
export const conversationDraftKey = (user: string, organization: string, project: string, task: string) =>
  'suma-task-conversation-draft-v1:' + JSON.stringify([user, organization, project, task]);
const emptyDraft = (): ConversationDraft => ({body: '', mention_resource_ids: []});
export function readConversationDraft(storage: DraftStorage | undefined, key: string): {draft: ConversationDraft; invalid?: boolean} {
  try {
    const raw = storage?.getItem(key); if (!raw) return {draft: emptyDraft()};
    const value: unknown = JSON.parse(raw);
    if (!object(value) || Object.keys(value).sort().join(',') !== ['body','format','mention_resource_ids',...(Object.hasOwn(value,'knowledge')?['knowledge']:[])].sort().join(',') ||
      value.format !== 'suma-task-conversation-draft-v1' || typeof value.body !== 'string' ||
      value.body.length > 16000 || value.body.includes('\0') || /[\uD800-\uDFFF]/u.test(value.body) ||
      !validMentionResourceIDs(value.mention_resource_ids) || Object.hasOwn(value,'knowledge') && !validKnowledgePins(value.knowledge)) return {draft: emptyDraft(), invalid: true};
    return {draft: {body: value.body, mention_resource_ids: value.mention_resource_ids, ...(Object.hasOwn(value,'knowledge') ? {knowledge:value.knowledge as KnowledgeReference[]} : {})}};
  } catch { return {draft: emptyDraft(), invalid: true}; }
}
/** Only owner-authored text and canonical IDs are durable. Directory names,
 * current permissions and message/source snapshots never enter this draft. */
export function saveConversationDraft(storage: DraftStorage | undefined, key: string, draft: ConversationDraft) {
  if (!storage) throw new Error('draft storage unavailable');
  if (!draft.body && !draft.mention_resource_ids.length && !draft.knowledge?.length) storage.removeItem(key);
  else storage.setItem(key, JSON.stringify({format: 'suma-task-conversation-draft-v1', ...draft}));
}
