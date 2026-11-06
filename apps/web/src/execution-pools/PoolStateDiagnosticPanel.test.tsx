// @vitest-environment jsdom
import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { setLocale } from '../i18n';
import { PoolStateDiagnosticPanel } from './PoolStateDiagnosticPanel';

it('distinguishes closed local results from pending receipts in both languages', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  const counts = {
    entries: 4, pending_results: 1, acknowledged_results: 1, closed_results: 2, unknown_results: 0,
    pending_progress: 0, unresolved_inbox: 0, pending_artifacts: 0, pending_candidates: 0, truncated_progress: 0,
  };
  function snapshot(journal_counts: typeof counts | Omit<typeof counts, 'closed_results'>) {
    return JSON.stringify({
      schema_version: 1, component: 'local_pool_runner_state', status: 'available', complete: true,
      slots: [{ slot_id: '0', journal_status: 'available', claim_state: 'complete', journal_counts }],
    });
  }
  async function preview(value: string) {
    const input = host.querySelector<HTMLTextAreaElement>('[data-pool-diagnostic-paste]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(input, value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => host.querySelector<HTMLButtonElement>('[data-pool-diagnostic-preview]')!.click());
  }
  try {
    await setLocale('en-US', { persist: false });
    await act(async () => root.render(<PoolStateDiagnosticPanel runnerLabel="Runner A" />));
    host.querySelector<HTMLDetailsElement>('[data-pool-diagnostic-import]')!.open = true;
    await preview(snapshot(counts));
    const slot = host.querySelector('[data-pool-diagnostic-slot="0"]');
    expect(slot?.textContent).toContain('Pending receipts 1');
    expect(slot?.textContent).toContain('Acknowledged results 1');
    expect(slot?.textContent).toContain('Closed-attempt results kept locally 2');
    expect(slot?.textContent).toContain('Unknown results 0');
    expect(document.activeElement).toBe(host.querySelector('[role="region"]'));

    await act(async () => { await setLocale('zh-CN', { persist: false }); });
    expect(slot?.textContent).toContain('待回执 1');
    expect(slot?.textContent).toContain('已回执 1');
    expect(slot?.textContent).toContain('尝试已结束，结果留存本机 2');
    const { closed_results: _omitted, ...oldCounts } = counts;
    await preview(snapshot(oldCounts));
    expect(host.querySelector('[data-pool-diagnostic-slot]')).toBeNull();
    expect(document.activeElement).toBe(host.querySelector('[data-pool-diagnostic-paste]'));
  } finally {
    await act(async () => root.unmount());
    host.remove();
    await setLocale('zh-CN', { persist: false });
    vi.unstubAllGlobals();
  }
});
