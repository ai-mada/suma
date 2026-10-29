// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect, it, vi} from 'vitest';
import type {WorkspaceTerminalSession} from '../../../packages/contracts/src';
import {useTerminalConnection} from './terminal-hooks';

const harness = vi.hoisted(() => ({transport: vi.fn()}));
vi.mock('./api-context', () => ({useApi: () => harness.transport}));

// A transport/renderer protocol fixture, not a real shell or vendor execution.
it('acknowledges terminal output only after the renderer confirms its original bytes were consumed', async () => {
  vi.useFakeTimers(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host);
  const session: WorkspaceTerminalSession = {id: 'terminal-one', project_id: 'project-one', task_id: 'task-one', task_version: 1,
    version: 1, run_id: 'terminal-run', run_version: 1, state: 'running', tool_profile_id: 'shell-profile', tool_name: 'Owned shell',
    directory: 'attempt-worktree', worker_id: 'worker-one', columns: 80, rows: 24, control_epoch: 1, next_input_sequence: 1,
    output_first_sequence: 1, output_last_sequence: 1, output_ack_sequence: 0, expires_at: '2026-10-29T12:00:00Z',
    created_at: '2026-10-29T11:00:00Z', allowed_actions: ['input', 'resize', 'stop', 'ack'], reason: '', source_snapshot_sha256: '', base_commit: ''};
  let consumed: ((value: boolean) => void) | undefined;
  const writer = vi.fn((_bytes: Uint8Array) => new Promise<boolean>(resolve => {consumed = resolve;}));
  const sendControl = vi.fn(async (_body: Record<string, unknown>) => ({session: {...session, version: 2, output_ack_sequence: 1}, input: null}));
  harness.transport.mockImplementation(async (path: string) => path.includes('/output?') ? {
    session_id: session.id, control_epoch: 1, chunks: [{sequence: 1, data_base64: '4pyT'}], first_sequence: 1, last_sequence: 1, gap: false, state: 'running',
  } : session);
  function Probe() {useTerminalConnection({project: 'project-one', task: 'task-one', session, active: true, writer,
    controlBlocked: false, onSession: () => {}, onDenied: () => {}, sendControl}); return null;}
  const settle = async () => {for (let index = 0; index < 12; index++) await Promise.resolve();};
  try {
    await act(async () => {root.render(<Probe/>); await settle();});
    expect(writer).toHaveBeenCalledTimes(1);
    expect(Array.from(writer.mock.calls[0][0] as unknown as Uint8Array)).toEqual([0xe2, 0x9c, 0x93]);
    expect(sendControl).not.toHaveBeenCalled();
    await act(async () => {consumed?.(true); await settle();});
    expect(sendControl).toHaveBeenCalledTimes(1);
    expect(sendControl.mock.calls[0][0]).toMatchObject({project_id: 'project-one', expected_version: 1, control_epoch: 1, action: 'ack', output_sequence: 1});
  } finally {await act(async () => root.unmount()); host.remove(); vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});
