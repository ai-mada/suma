import type {WorkspaceTerminalSession, WorkspaceTerminalPage, WorkspaceTerminalOutputPage, WorkspaceTerminalCommandReceipt} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const keys = (value: Record<string, unknown>, expected: string[]) => Object.keys(value).sort().join(',') === [...expected].sort().join(',');
const text = (value: unknown): value is string => typeof value === 'string';
const natural = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
const positive = (value: unknown): value is number => natural(value) && value > 0;
export const terminalID = (value: unknown): value is string => text(value) && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
export const terminalStates = ['queued', 'running', 'stop_requested', 'exited', 'stopped', 'reconciling'] as const;
const actions = ['input', 'resize', 'stop', 'ack'];
export const terminalEnded = (session: WorkspaceTerminalSession) => ['exited', 'stopped'].includes(session.state);

export function validTerminalSession(value: unknown, project: string, task: string): value is WorkspaceTerminalSession {
  if (!object(value) || !keys(value, ['id', 'project_id', 'task_id', 'task_version', 'version', 'run_id', 'run_version', 'state',
    'tool_profile_id', 'tool_name', 'directory', 'worker_id', 'columns', 'rows', 'control_epoch', 'next_input_sequence',
    'output_first_sequence', 'output_last_sequence', 'output_ack_sequence', 'expires_at', 'created_at', 'allowed_actions', 'reason'])) return false;
  return terminalID(value.id) && value.project_id === project && value.task_id === task && positive(value.task_version) &&
    positive(value.version) && terminalID(value.run_id) && positive(value.run_version) && terminalStates.includes(value.state as typeof terminalStates[number]) &&
    terminalID(value.tool_profile_id) && text(value.tool_name) && text(value.directory) && text(value.worker_id) &&
    positive(value.columns) && Number(value.columns) <= 1000 && positive(value.rows) && Number(value.rows) <= 500 &&
    positive(value.control_epoch) && positive(value.next_input_sequence) && natural(value.output_first_sequence) &&
    natural(value.output_last_sequence) && natural(value.output_ack_sequence) && value.output_ack_sequence <= value.output_last_sequence &&
    value.output_first_sequence <= value.output_last_sequence + 1 && text(value.expires_at) && Number.isFinite(Date.parse(value.expires_at)) &&
    text(value.created_at) && Number.isFinite(Date.parse(value.created_at)) && text(value.reason) && Array.isArray(value.allowed_actions) &&
    value.allowed_actions.every(action => actions.includes(String(action))) && new Set(value.allowed_actions).size === value.allowed_actions.length;
}

export function validTerminalPage(value: unknown, project: string, task: string): value is WorkspaceTerminalPage {
  return object(value) && keys(value, ['items', 'profiles', 'task_version', 'can_create', 'profiles_complete']) && Array.isArray(value.items) &&
    typeof value.profiles_complete === 'boolean' && value.items.length <= 20 && value.items.every(item => validTerminalSession(item, project, task)) &&
    new Set(value.items.map(item => item.id)).size === value.items.length && positive(value.task_version) && typeof value.can_create === 'boolean' &&
    Array.isArray(value.profiles) && value.profiles.length <= 200 && value.profiles.every(profile => object(profile) && terminalID(profile.id) &&
      profile.project_id === project && positive(profile.version) && text(profile.name) && object(profile.profile) &&
      profile.profile.terminal_session_protocol === 'suma-pty-v1' && text(profile.profile.profile_sha256) && /^[a-f0-9]{64}$/.test(profile.profile.profile_sha256));
}

/** Binary output is passed to xterm's incremental decoder; it is never HTML. */
export function terminalBytes(value: unknown, maximum = 32768): Uint8Array | undefined {
  if (!text(value) || !value || value.length > Math.ceil(maximum / 3) * 4 || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return;
  try {const binary = atob(value); if (!binary.length || binary.length > maximum || btoa(binary) !== value) return;
    return Uint8Array.from(binary, character => character.charCodeAt(0));} catch {return;}
}
export function terminalBase64(bytes: Uint8Array): string {return btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(''));}
export async function terminalSHA(bytes: Uint8Array): Promise<string> {
  const result = await crypto.subtle.digest('SHA-256', bytes.slice().buffer);
  return Array.from(new Uint8Array(result), byte => byte.toString(16).padStart(2, '0')).join('');
}
export function validTerminalOutput(value: unknown, session: WorkspaceTerminalSession, after: number): value is WorkspaceTerminalOutputPage {
  if (!object(value) || !keys(value, ['session_id', 'control_epoch', 'chunks', 'first_sequence', 'last_sequence', 'gap', 'state']) ||
    value.session_id !== session.id || value.control_epoch !== session.control_epoch || !natural(value.first_sequence) ||
    !natural(value.last_sequence) || value.first_sequence > value.last_sequence + 1 || typeof value.gap !== 'boolean' ||
    !terminalStates.includes(value.state as typeof terminalStates[number]) || !Array.isArray(value.chunks) || value.chunks.length > 64) return false;
  let previous = after, total = 0;
  for (const chunk of value.chunks) {
    if (!object(chunk) || !keys(chunk, ['sequence', 'data_base64']) || !positive(chunk.sequence) || chunk.sequence <= previous ||
      chunk.sequence < value.first_sequence || chunk.sequence > value.last_sequence || !value.gap && chunk.sequence !== previous + 1) return false;
    const bytes = terminalBytes(chunk.data_base64); if (!bytes) return false; total += bytes.length; if (total > 2 * 1024 * 1024) return false;
    previous = chunk.sequence;
  }
  return true;
}
export function validTerminalCommand(value: unknown, project: string, task: string, session: string): value is WorkspaceTerminalCommandReceipt {
  if (!object(value) || !keys(value, ['session', 'input']) || !validTerminalSession(value.session, project, task) || value.session.id !== session) return false;
  if (value.input === null) return true;
  const input = value.input;
  return object(input) && keys(input, ['input_id', 'sequence', 'sha256', 'state', 'bytes_written']) && text(input.input_id) && /^[A-Za-z0-9_.-]{1,128}$/.test(input.input_id) &&
    positive(input.sequence) && text(input.sha256) && /^[a-f0-9]{64}$/.test(input.sha256) &&
    ['accepted', 'delivered', 'delivery_unknown'].includes(String(input.state)) && natural(input.bytes_written) && input.bytes_written <= 8192;
}
/** Only non-input controls use durable browser recovery. Typed shell bytes
 * stay in short-lived memory and are never replayed after an unknown result. */
export function validTerminalRecovery(request: WriteRecoveryRequest, project: string, task: string): boolean {
  const body = request.body;
  if (request.method !== 'POST' || body.project_id !== project || body.idempotency_key !== request.idempotency_key) return false;
  if (request.path === '/workspace/terminal-sessions') return keys(body, ['project_id', 'task_id', 'expected_task_version', 'tool_profile_id',
    'expected_tool_version', 'columns', 'rows', 'timeout_seconds', 'acknowledge_shell', 'idempotency_key']) && body.task_id === task &&
    positive(body.expected_task_version) && terminalID(body.tool_profile_id) && positive(body.expected_tool_version) && positive(body.columns) &&
    positive(body.rows) && natural(body.timeout_seconds) && body.timeout_seconds >= 60 && body.timeout_seconds <= 3600 && body.acknowledge_shell === true;
  if (!/^\/workspace\/terminal-sessions\/[A-Za-z0-9_.-]{1,100}\/commands$/.test(request.path) || !positive(body.expected_version) || !positive(body.control_epoch)) return false;
  const common = ['project_id', 'expected_version', 'control_epoch', 'action', 'idempotency_key'];
  return body.action === 'stop' && keys(body, common) || body.action === 'ack' && keys(body, [...common, 'output_sequence']) && natural(body.output_sequence) ||
    body.action === 'resize' && keys(body, [...common, 'columns', 'rows']) && positive(body.columns) && positive(body.rows);
}
