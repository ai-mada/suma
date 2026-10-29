import {expect, it} from 'vitest';
import type {WorkspaceTerminalSession} from '../../../packages/contracts/src';
import {terminalBytes, validTerminalCommand, validTerminalOutput, validTerminalRecovery, validTerminalSession} from './terminal-model';

const session: WorkspaceTerminalSession = {id: 'terminal-one', project_id: 'project-one', task_id: 'task-one', task_version: 2,
  version: 3, run_id: 'terminal-run-one', run_version: 1, state: 'running', tool_profile_id: 'profile-one', tool_name: 'Owned shell',
  directory: 'attempt-worktree', worker_id: 'worker-one', columns: 80, rows: 24, control_epoch: 1, next_input_sequence: 1,
  output_first_sequence: 1, output_last_sequence: 2, output_ack_sequence: 0, expires_at: '2026-10-29T12:00:00Z',
  created_at: '2026-10-29T11:00:00Z', allowed_actions: ['input', 'resize', 'ack', 'stop'], reason: '', source_snapshot_sha256: '', base_commit: '', admin_base_commit: ''};

it('binds terminal output to its exact session and epoch, exposes gaps and excludes shell input from durable recovery', () => {
  const page = {session_id: session.id, control_epoch: 1, chunks: [{sequence: 1, data_base64: 'YQ=='}], first_sequence: 1, last_sequence: 2, gap: false, state: 'running'};
  expect(validTerminalSession(session, 'project-one', 'task-one')).toBe(true);
  expect(validTerminalSession({...session, project_id: 'project-two'}, 'project-one', 'task-one')).toBe(false);
  expect(validTerminalOutput(page, session, 0)).toBe(true);
  expect(validTerminalOutput({...page, session_id: 'another-terminal'}, session, 0)).toBe(false);
  expect(validTerminalOutput({...page, control_epoch: 2}, session, 0)).toBe(false);
  const retained = {...page, chunks: [{sequence: 8, data_base64: 'YQ=='}], first_sequence: 8, last_sequence: 9};
  expect(validTerminalOutput(retained, session, 0)).toBe(false);
  expect(validTerminalOutput({...retained, gap: true}, session, 0)).toBe(true);
  expect(terminalBytes('YR==')).toBeUndefined(); // Noncanonical trailing bits.
  expect(validTerminalOutput({...page, chunks: [{...page.chunks[0], command: 'untrusted'}]}, session, 0)).toBe(false);
  const common = {project_id: 'project-one', expected_version: 3, control_epoch: 1, idempotency_key: 'original-control-key'};
  const request = {path: '/workspace/terminal-sessions/terminal-one/commands', method: 'POST', idempotency_key: common.idempotency_key};
  expect(validTerminalRecovery({...request, body: {...common, action: 'stop'}}, 'project-one', 'task-one')).toBe(true);
  expect(validTerminalRecovery({...request, body: {...common, action: 'input', input_id: 'input-one', input_sequence: 1, data_base64: 'c2VjcmV0'}}, 'project-one', 'task-one')).toBe(false);
  expect(validTerminalRecovery({...request, body: {...common, action: 'stop', data_base64: 'c2VjcmV0'}}, 'project-one', 'task-one')).toBe(false);
  const receipt = {session: {...session, version: 4, next_input_sequence: 2},
    input: {input_id: 'original-input', sequence: 1, sha256: 'a'.repeat(64), state: 'accepted', bytes_written: 0}};
  expect(validTerminalCommand(receipt, 'project-one', 'task-one', session.id)).toBe(true);
  expect(validTerminalCommand({...receipt, input: {...receipt.input, state: 'queued'}}, 'project-one', 'task-one', session.id)).toBe(false);
});
