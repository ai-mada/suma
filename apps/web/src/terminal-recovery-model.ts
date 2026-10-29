import type {WorkspaceTerminalRecoveryInput, WorkspaceTerminalRecoveryView, WorkspaceTerminalRecoveryReceipt} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validTerminalSession} from './terminal-model';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const keys = (value: Record<string, unknown>, expected: string[]) => Object.keys(value).sort().join(',') === [...expected].sort().join(',');
const natural = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
const positive = (value: unknown): value is number => natural(value) && value > 0;
const inputID = (value: unknown): value is string => typeof value === 'string' && /^[A-Za-z0-9_.-]{1,128}$/.test(value);
const states = ['pending', 'ready', 'running', 'waiting', 'reconciling', 'succeeded', 'failed', 'skipped', 'cancelled'];
const dispatchID = (value: unknown): value is string => typeof value === 'string' && value.length >= 8 && value.length <= 128 && !/[\u0000-\u001f\u007f]/.test(value);
export const terminalRecoveryReason = (value: unknown): value is string => typeof value === 'string' && !!value.trim() &&
  new TextEncoder().encode(value).length <= 2000 && !value.includes('\u0000');

export function validTerminalRecoveryInputs(value: unknown): value is WorkspaceTerminalRecoveryInput[] {
  return Array.isArray(value) && value.length <= 65536 && value.every(input => object(input) && keys(input, ['input_id', 'sequence', 'sha256']) &&
    inputID(input.input_id) && positive(input.sequence) && typeof input.sha256 === 'string' && /^[a-f0-9]{64}$/.test(input.sha256)) &&
    new Set(value.map(input => input.input_id)).size === value.length && new Set(value.map(input => input.sequence)).size === value.length;
}

export function validTerminalRecoveryView(value: unknown, session: string, run: string): value is WorkspaceTerminalRecoveryView {
  return object(value) && keys(value, ['session_id', 'run_id', 'run_version', 'run_state', 'attempt', 'fence', 'dispatch_id',
    'tree_stopped', 'unknown_inputs', 'can_recover', 'reason']) && value.session_id === session && value.run_id === run &&
    positive(value.run_version) && states.includes(String(value.run_state)) && natural(value.attempt) && natural(value.fence) &&
    typeof value.dispatch_id === 'string' && (!value.dispatch_id || dispatchID(value.dispatch_id)) && typeof value.tree_stopped === 'boolean' &&
    validTerminalRecoveryInputs(value.unknown_inputs) && typeof value.can_recover === 'boolean' && typeof value.reason === 'string' &&
    (!value.can_recover || value.tree_stopped && value.run_state === 'reconciling' && positive(value.attempt) && positive(value.fence));
}

export function validTerminalRecoverRequest(request: WriteRecoveryRequest, project: string, session: string, run: string): boolean {
  const body = request.body;
  return request.method === 'POST' && request.path === '/workspace/terminal-sessions/' + encodeURIComponent(session) + '/recover' &&
    keys(body, ['project_id', 'expected_version', 'expected_run_version', 'run_id', 'attempt', 'fence', 'dispatch_id', 'unknown_inputs', 'reason', 'idempotency_key']) &&
    body.project_id === project && body.idempotency_key === request.idempotency_key && positive(body.expected_version) &&
    positive(body.expected_run_version) && body.run_id === run && positive(body.attempt) && positive(body.fence) &&
    dispatchID(body.dispatch_id) && validTerminalRecoveryInputs(body.unknown_inputs) && terminalRecoveryReason(body.reason);
}

export function validTerminalRecoverReceipt(value: unknown, request: WriteRecoveryRequest, project: string, task: string, session: string, run: string): value is WorkspaceTerminalRecoveryReceipt {
  if (!object(value) || !keys(value, ['session', 'recovery']) || !validTerminalSession(value.session, project, task) ||
    value.session.id !== session || value.session.run_id !== run || !validTerminalRecoveryView(value.recovery, session, run) || !validTerminalRecoveryInputs(request.body.unknown_inputs)) return false;
  const originalInputs = new Map(request.body.unknown_inputs.map(input => [input.input_id, input]));
  return value.session.version > Number(request.body.expected_version) && value.recovery.run_version > Number(request.body.expected_run_version) &&
    value.recovery.run_state === 'failed' && !value.recovery.can_recover && value.recovery.tree_stopped &&
    value.recovery.attempt === request.body.attempt && value.recovery.fence === request.body.fence && value.recovery.dispatch_id === request.body.dispatch_id &&
    value.recovery.unknown_inputs.length === originalInputs.size && value.recovery.unknown_inputs.every(input => {
      const original = originalInputs.get(input.input_id); return !!original && original.sequence === input.sequence && original.sha256 === input.sha256;
    });
}
