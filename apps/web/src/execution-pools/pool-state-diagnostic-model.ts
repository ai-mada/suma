import { runnerSetupLiteral } from '../cursor-setup-guide-model';
import type { Locale } from '../i18n';
import { wellFormed } from '../i18n/catalog';
import { uniqueJSONMembers } from '../unique-json-members';
import { validPoolConfigPath } from './pool-config-preparation-model';

export const POOL_STATE_DIAGNOSTIC_MAX_CHARS = 65536;
const slotLimit = 128;
const maxJournalCount = 16 << 20;
const readStatuses = ['available', 'missing', 'invalid', 'unreadable', 'over_limit'] as const;
const observedClaims = ['missing', 'request_recorded', 'response_recorded', 'complete'] as const;
const claimStates = [...observedClaims, 'invalid', 'unreadable', 'over_limit'] as const;
const countFields = [
  'entries', 'pending_results', 'acknowledged_results', 'closed_results', 'unknown_results',
  'pending_progress', 'unresolved_inbox', 'pending_artifacts', 'pending_candidates', 'truncated_progress',
] as const;
const snapshotFields = ['schema_version', 'component', 'status', 'complete', 'slots'];
const slotFields = ['slot_id', 'journal_status', 'claim_state'];

export type PoolStateDiagnosticStatus = typeof readStatuses[number];
export type PoolStateDiagnosticClaim = typeof claimStates[number];
export type PoolStateDiagnosticCounts = Record<typeof countFields[number], number>;
export type PoolStateDiagnosticSlot = {
  slot_id: string;
  journal_status: PoolStateDiagnosticStatus;
  claim_state: PoolStateDiagnosticClaim;
  journal_counts?: PoolStateDiagnosticCounts;
};
export type PoolStateDiagnostic = {
  schema_version: 1;
  component: 'local_pool_runner_state';
  status: PoolStateDiagnosticStatus;
  complete: boolean;
  slots: PoolStateDiagnosticSlot[];
};
export type PoolStateDiagnosticShell = 'powershell' | 'bash';

const object = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);
const exact = (value: Record<string, unknown>, fields: readonly string[]) =>
  Object.keys(value).length === fields.length && fields.every(field => Object.hasOwn(value, field));
const isOneOf = <T extends string>(value: unknown, options: readonly T[]): value is T =>
  typeof value === 'string' && (options as readonly string[]).includes(value);
const validSlot = (value: unknown): value is string =>
  typeof value === 'string' && /^(?:0|[1-9][0-9]{0,2})$/.test(value) && Number(value) < slotLimit;
const validCount = (value: unknown): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 && value <= maxJournalCount;

function validCounts(value: unknown): value is PoolStateDiagnosticCounts {
  if (!object(value) || !exact(value, countFields) || !countFields.every(key => validCount(value[key]))) return false;
  const counts = value as PoolStateDiagnosticCounts;
  return counts.unknown_results <= counts.pending_results &&
    counts.pending_results + counts.acknowledged_results + counts.closed_results + counts.unresolved_inbox <= counts.entries &&
    counts.pending_artifacts <= counts.entries && counts.pending_candidates <= counts.entries &&
    counts.truncated_progress <= counts.entries;
}

function validSlotReport(value: unknown): value is PoolStateDiagnosticSlot {
  if (!object(value) || !validSlot(value.slot_id) || !isOneOf(value.journal_status, readStatuses) ||
      !isOneOf(value.claim_state, claimStates)) return false;
  return value.journal_status === 'available'
    ? exact(value, [...slotFields, 'journal_counts']) && validCounts(value.journal_counts)
    : exact(value, slotFields);
}

/** This is a local display projection of untrusted pasted JSON. No imported
 * report establishes machine identity, freshness or server acknowledgement. */
export function readPoolStateDiagnostic(text: unknown): PoolStateDiagnostic | undefined {
  if (typeof text !== 'string' || text.length > POOL_STATE_DIAGNOSTIC_MAX_CHARS || !wellFormed(text)) return;
  try {
    const value: unknown = JSON.parse(text);
    if (!object(value) || !exact(value, snapshotFields) || value.schema_version !== 1 ||
        value.component !== 'local_pool_runner_state' || !isOneOf(value.status, readStatuses) ||
        typeof value.complete !== 'boolean' || !Array.isArray(value.slots) || value.slots.length > slotLimit ||
        !uniqueJSONMembers(text)) return;
    if (value.status !== 'available' && (value.complete || value.slots.length)) return;
    let previous = -1;
    for (const slot of value.slots) {
      if (!validSlotReport(slot) || Number(slot.slot_id) <= previous ||
          value.complete && (slot.journal_status !== 'available' || !isOneOf(slot.claim_state, observedClaims))) return;
      previous = Number(slot.slot_id);
    }
    return value as PoolStateDiagnostic;
  } catch {
    return;
  }
}

function validDiagnosticPath(state: string, shell: PoolStateDiagnosticShell): boolean {
  // PowerShell treats smart quotes as delimiters. Legacy native argument
  // passing can also reinterpret double quotes inside an otherwise literal arg.
  if (shell === 'powershell' && /["\u2018-\u201b]/u.test(state)) return false;
  if (!state || state.trim() !== state || !wellFormed(state) ||
      new TextEncoder().encode(state).length > 4096 || /[\p{Cc}\u2028\u2029]/u.test(state)) return false;
  if (/^(?:[A-Za-z]:|[\\/])/.test(state)) return validPoolConfigPath(state);
  const parts = state.split(shell === 'powershell' ? /[\\/]/ : '/');
  return parts.every((part, index) => part !== '..' && (part !== '.' || index === 0));
}

export function poolStateDiagnosticCommand({
  state, shell, locale, slotID,
}: {
  state: string;
  shell: PoolStateDiagnosticShell;
  locale: Locale;
  slotID?: string;
}): string | undefined {
  if (typeof state !== 'string' || (shell !== 'powershell' && shell !== 'bash') ||
      (locale !== 'zh-CN' && locale !== 'en-US') || !validDiagnosticPath(state, shell) ||
      (slotID !== undefined && !validSlot(slotID))) return;
  const path = slotID === undefined ? state : state + (state.endsWith('/') ? '' : '/') + 'pool/slots/' + slotID;
  if (!validDiagnosticPath(path, shell)) return;
  const mode = slotID === undefined ? '--inspect-pool-state' : '--inspect-state';
  return 'suma-runner ' + mode + ' --state ' + runnerSetupLiteral(path, shell) + ' --language ' + locale;
}
