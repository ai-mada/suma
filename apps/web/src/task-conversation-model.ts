import type {TaskConversationMessage, TaskConversationMessageDetail, TaskConversationPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validMentionResourceIDs} from './task-conversation-mentions-model';

const object = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);
const keys = (value: Record<string, unknown>, expected: string[]) =>
  Object.keys(value).sort().join(',') === [...expected].sort().join(',');
const text = (value: unknown): value is string => typeof value === 'string';
const natural = (value: unknown): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
const positive = (value: unknown): value is number => natural(value) && value > 0;
const identifier = (value: unknown): value is string =>
  text(value) && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const optionalID = (value: unknown) => value === '' || identifier(value);
const optionalRunID = (value: unknown) => value === '' ||
  text(value) && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);

/** Count Unicode scalars rather than UTF-16 code units. A browser must reject
 * lone surrogates instead of sending an implicitly replaced message body. */
export function validConversationBody(value: unknown): value is string {
  return text(value) && !!value.trim() && !value.includes('\0') && !/[\uD800-\uDFFF]/u.test(value) &&
    Array.from(value).length <= 8000 && new TextEncoder().encode(value).length <= 32000;
}

const messageKeys = ['id', 'sequence', 'task_version', 'body', 'author_id',
  'author_name', 'actor_kind', 'created_at', 'requirement_id', 'requirement_version', 'run_id',
  'reply_id', 'reply_to_sequence', 'execution_run_id'];

export function validConversationMessage(value: unknown): value is TaskConversationMessage {
  if (!object(value) || !keys(value, [...messageKeys, ...(Object.hasOwn(value, 'mentions') ? ['mentions'] : [])])) return false;
  if (Object.hasOwn(value, 'mentions')) {
    if (!Array.isArray(value.mentions) || !value.mentions.length || value.mentions.length > 20 || value.actor_kind === 'assistant') return false;
    const resources = new Set(), users = new Set();
    for (const mention of value.mentions) {
      if (!object(mention) || !keys(mention, ['resource_id', 'user_id', 'name_at_post']) ||
        !identifier(mention.resource_id) || !identifier(mention.user_id) || !text(mention.name_at_post) ||
        resources.has(mention.resource_id) || users.has(mention.user_id)) return false;
      resources.add(mention.resource_id); users.add(mention.user_id);
    }
  }
  return identifier(value.id) && positive(value.sequence) && positive(value.task_version) &&
    validConversationBody(value.body) && identifier(value.author_id) && text(value.author_name) &&
    ['human', 'api', 'assistant'].includes(String(value.actor_kind)) && text(value.created_at) &&
    Number.isFinite(Date.parse(value.created_at)) && optionalID(value.requirement_id) &&
    natural(value.requirement_version) && (value.requirement_id === ''
      ? value.requirement_version === 0 : value.requirement_version > 0) && optionalRunID(value.run_id) &&
    (value.actor_kind === 'assistant'
      ? identifier(value.reply_id) && positive(value.reply_to_sequence) && value.reply_to_sequence < value.sequence &&
        text(value.execution_run_id) && value.execution_run_id !== '' && optionalRunID(value.execution_run_id)
      : value.reply_id === '' && value.reply_to_sequence === 0 && value.execution_run_id === '');
}

/** Validate each bounded server page before merging it into the displayed
 * chronological history. Older history cannot substitute a different task. */
export function validConversationPage(value: unknown, project: string, task: string,
  before = 0, limit = 20): value is TaskConversationPage {
  if (!object(value) || !keys(value, ['project_id', 'task_id', 'task_version', 'can_post',
    'items', 'total', 'limit', 'next_before'])) return false;
  if (value.project_id !== project || value.task_id !== task || !positive(value.task_version) ||
    typeof value.can_post !== 'boolean' || !natural(value.total) || value.limit !== limit ||
    !natural(value.next_before) || !Array.isArray(value.items) || value.items.length > limit ||
    value.items.length > value.total || !value.items.every(validConversationMessage)) return false;
  const items = value.items as TaskConversationMessage[];
  if (new Set(items.map(item => item.id)).size !== items.length ||
    items.some((item, index) => item.task_version > (value.task_version as number) ||
      before > 0 && item.sequence >= before || index > 0 && items[index - 1].sequence >= item.sequence)) return false;
  return value.next_before === 0 || items.length > 0 && value.next_before === items[0].sequence;
}

/** Recovery always replays the original body, task basis, operation and actor
 * scope. No client-supplied author or execution status is part of this command. */
export function validConversationRequest(request: WriteRecoveryRequest, project: string, task: string): boolean {
  const body = request.body;
  return request.path === '/task-conversations' && request.method === 'POST' &&
    keys(body, ['project_id', 'task_id', 'expected_task_version', 'body', 'idempotency_key',
      ...(Object.hasOwn(body, 'mention_resource_ids') ? ['mention_resource_ids'] : [])]) &&
    (!Object.hasOwn(body, 'mention_resource_ids') || validMentionResourceIDs(body.mention_resource_ids)) &&
    body.project_id === project && body.task_id === task && positive(body.expected_task_version) &&
    validConversationBody(body.body) && body.idempotency_key === request.idempotency_key;
}

export function validConversationReceipt(value: unknown, request: WriteRecoveryRequest, userID: string): boolean {
  return validConversationMessage(value) && value.body === request.body.body &&
    value.task_version === request.body.expected_task_version &&
    value.actor_kind === 'human' && value.author_id === userID &&
    JSON.stringify(value.mentions?.map(mention => mention.resource_id) || []) === JSON.stringify(request.body.mention_resource_ids || []);
}

export function validConversationMessageDetail(value: unknown, project: string, task: string, message: string): value is TaskConversationMessageDetail {
  return object(value) && keys(value, ['project_id', 'task_id', 'task_version', 'can_post', 'message']) &&
    value.project_id === project && value.task_id === task && positive(value.task_version) && typeof value.can_post === 'boolean' &&
    validConversationMessage(value.message) && value.message.id === message && value.message.task_version <= value.task_version;
}
