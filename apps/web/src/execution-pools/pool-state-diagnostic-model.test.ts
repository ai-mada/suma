import { describe, expect, it } from 'vitest';
import {
  POOL_STATE_DIAGNOSTIC_MAX_CHARS, poolStateDiagnosticCommand, readPoolStateDiagnostic,
} from './pool-state-diagnostic-model';

const counts = {
  entries: 3, pending_results: 1, acknowledged_results: 1, closed_results: 1, unknown_results: 1,
  pending_progress: 3, unresolved_inbox: 0, pending_artifacts: 0,
  pending_candidates: 0, truncated_progress: 1,
};
const slot = {
  slot_id: '2', journal_status: 'available', journal_counts: counts, claim_state: 'response_recorded',
};
const snapshot = {
  schema_version: 1, component: 'local_pool_runner_state', status: 'available', complete: true,
  slots: [slot, { ...slot, slot_id: '10', claim_state: 'missing' }],
};

describe('pool state diagnostic import', () => {
  it('preserves complete, partial and unavailable local snapshots', () => {
    expect(readPoolStateDiagnostic(JSON.stringify(snapshot) + '\r\n')).toEqual(snapshot);
    const partial = {
      ...snapshot, complete: false, slots: [slot, {
        slot_id: '10', journal_status: 'invalid', claim_state: 'unreadable',
      }],
    };
    expect(readPoolStateDiagnostic(JSON.stringify(partial))).toEqual(partial);
    const missing = { ...snapshot, status: 'missing', complete: false, slots: [] };
    expect(readPoolStateDiagnostic(JSON.stringify(missing))).toEqual(missing);
  });

  it('rejects unknown members, duplicate fields and oversized input', () => {
    expect(readPoolStateDiagnostic(JSON.stringify({ ...snapshot, worker_id: 'private' }))).toBeUndefined();
    expect(readPoolStateDiagnostic(JSON.stringify({ ...snapshot, slots: [{ ...slot, run_id: 'private' }] }))).toBeUndefined();
    expect(readPoolStateDiagnostic(JSON.stringify({ ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, raw: 'private' } }] }))).toBeUndefined();
    expect(readPoolStateDiagnostic(JSON.stringify(snapshot).replace('"slot_id":"2"', '"slot_id":"9","slot_id":"2"'))).toBeUndefined();
    expect(readPoolStateDiagnostic(' '.repeat(POOL_STATE_DIAGNOSTIC_MAX_CHARS) + JSON.stringify(snapshot))).toBeUndefined();
  });

  it('rejects invalid ordering, status contradictions and implausible counts', () => {
    for (const badSlot of ['02', '128', '-1', '2/../3', 2]) {
      expect(readPoolStateDiagnostic(JSON.stringify({ ...snapshot, slots: [{ ...slot, slot_id: badSlot }] }))).toBeUndefined();
    }
    for (const invalid of [
      { ...snapshot, slots: [...snapshot.slots].reverse() },
      { ...snapshot, slots: [slot, slot] },
      { ...snapshot, status: 'missing', complete: false },
      { ...snapshot, slots: [{ ...slot, claim_state: 'invalid' }] },
      { ...snapshot, complete: false, slots: [{ ...slot, journal_status: 'invalid' }] },
      { ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, entries: -1 } }] },
      { ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, unknown_results: 2 } }] },
      { ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, closed_results: 2 } }] },
      { ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, closed_results: -1 } }] },
      { ...snapshot, slots: [{ ...slot, journal_counts: { ...counts, closed_results: undefined } }] },
    ]) {
      expect(readPoolStateDiagnostic(JSON.stringify(invalid))).toBeUndefined();
    }
  });
});

describe('pool state diagnostic commands', () => {
  it('quotes exact state paths in each supported shell', () => {
    expect(poolStateDiagnosticCommand({ state: '.cache/runner', shell: 'bash', locale: 'zh-CN' }))
      .toBe("suma-runner --inspect-pool-state --state '.cache/runner' --language zh-CN");
    expect(poolStateDiagnosticCommand({ state: "C:\\Team's state", shell: 'powershell', locale: 'en-US', slotID: '127' }))
      .toBe("suma-runner --inspect-state --state 'C:\\Team''s state/pool/slots/127' --language en-US");
    expect(poolStateDiagnosticCommand({ state: "/tmp/team's state/", shell: 'bash', locale: 'en-US', slotID: '0' }))
      .toBe(`suma-runner --inspect-state --state '/tmp/team'"'"'s state/pool/slots/0' --language en-US`);
  });

  it('guards shell selection, slot IDs, locale and unsafe PowerShell quotes', () => {
    const args = { state: '.cache/runner', shell: 'powershell', locale: 'en-US' } as const;
    for (const state of ['', '  .cache/runner', 'state/../other', 'a\nb', 'team’state', 'team"state']) {
      expect(poolStateDiagnosticCommand({ ...args, state })).toBeUndefined();
    }
    for (const slotID of ['00', '128', '2; echo']) {
      expect(poolStateDiagnosticCommand({ ...args, slotID })).toBeUndefined();
    }
    expect(poolStateDiagnosticCommand({ ...args, locale: 'en-US; echo' as never })).toBeUndefined();
    expect(poolStateDiagnosticCommand({ ...args, shell: 'cmd' as never })).toBeUndefined();
  });
});
