import type { TaskDispatchTestBinding, TaskDispatchTestOption, TaskDispatchTestSelection } from '../../../packages/contracts/src';
import type { Workflow, WNode } from './api';
import { parseToolManifest, type ToolProfile } from './tool-model';
import { sameToolProfile } from './tool-profile-reuse-model';
import { validTerminalCandidateTools } from './terminal-candidate-model';
export type { TaskDispatchTestBinding, TaskDispatchTestSelection };

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const id = (value: unknown): value is string => typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,200}$/.test(value);
const positive = (value: unknown): value is number => Number.isSafeInteger(value) && Number(value) > 0 && Number(value) <= 2147483647;
const hash = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const exact = (value: Record<string, unknown>, fields: string[]) => Reflect.ownKeys(value).length === fields.length && fields.every(field => Object.hasOwn(value, field));

export function associatedDispatchTests(workflow: Workflow, agentID: string): WNode[] {
  return workflow.nodes.filter(node => node.type === 'test' && node.config?.candidate_from === agentID && !Object.hasOwn(node.config, 'artifact_from'));
}
/** Full original public profile parsing remains shared. This reader does not
 * turn a directory row or same-machine slot into independent-test evidence. */
export function testDispatchProfile(value: unknown, project: string): ToolProfile | undefined {
  if (!validTerminalCandidateTools({ items: [value], total: 1 }, project) || !object(value)) return;
  const profile = value as unknown as ToolProfile;
  try {
    const [declared] = parseToolManifest(JSON.stringify([{ profile: profile.profile, available: profile.available, reason: profile.reason }]));
    if (!declared || profile.profile.adapter !== 'command' || profile.profile.protocol !== 'text-v1' ||
        profile.profile.read_only !== false || profile.profile.test_report_protocol !== 'suma-junit-report-v1') return;
    return profile;
  } catch { return; }
}
export function testDispatchBlocker(profile: ToolProfile, developerWorker: string): 'state' | 'unavailable' | 'worker' | undefined {
  if (profile.state !== 'enabled') return 'state';
  if (!profile.available) return 'unavailable';
  if (!profile.worker_id || profile.worker_id === developerWorker) return 'worker';
}
export function readTestDispatchProfile(value: unknown, expected: ToolProfile, project: string, developerWorker: string): ToolProfile | undefined {
  const profile = testDispatchProfile(value, project);
  if (!profile || profile.id !== expected.id || profile.version !== expected.version || profile.worker_id !== expected.worker_id ||
      !sameToolProfile(profile.profile, expected.profile) || testDispatchBlocker(profile, developerWorker)) return;
  try {
    const [observation] = parseToolManifest(JSON.stringify([profile.observation]));
    return observation?.available && sameToolProfile(observation.profile, profile.profile) ? structuredClone(profile) : undefined;
  } catch { return; }
}
type TestDispatchOptionBasis = {
  project: string;
  task: { id: string; version: number };
  workflow: Pick<Workflow, 'id' | 'version'>;
  agentID: string;
  developer: ToolProfile;
  nodeID: string;
  tester: ToolProfile;
};

export function testDispatchOptionPath({ project, task, workflow, agentID, developer, nodeID, tester }: TestDispatchOptionBasis) {
  const params = new URLSearchParams({
    project_id: project,
    expected_task_version: String(task.version),
    workflow_id: workflow.id,
    workflow_version: String(workflow.version),
    agent_node_id: agentID,
    development_tool_profile_id: developer.id,
    development_tool_profile_version: String(developer.version),
    test_node_id: nodeID,
    test_tool_profile_id: tester.id,
    test_tool_profile_version: String(tester.version),
  });
  return '/tasks/' + encodeURIComponent(task.id) + '/dispatch-test-option?' + params;
}

/** Only the original task, workflow, two profile pins, and organization can
 * authorize adoption of this read. A successful response is a snapshot. */
export function validTestDispatchOption(value: unknown, basis: TestDispatchOptionBasis, organizationID: string): value is TaskDispatchTestOption {
  const { project, task, workflow, agentID, developer, nodeID, tester } = basis;
  if (!object(value) || value.project_id !== project || value.task_id !== task.id || value.task_version !== task.version ||
      value.workflow_id !== workflow.id || value.workflow_version !== workflow.version || value.agent_node_id !== agentID ||
      value.development_tool_profile_id !== developer.id || value.development_tool_profile_version !== developer.version ||
      !hash(value.development_tool_profile_sha256) || value.development_tool_profile_sha256 !== developer.profile.profile_sha256 ||
      typeof value.checked_at !== 'string' || !Number.isFinite(Date.parse(value.checked_at))) return false;
  const route = value.execution_route;
  if (!object(route) || route.project_id !== project || !organizationID || route.tenant_id !== organizationID ||
      !Number.isSafeInteger(route.version) || Number(route.version) < 0) return false;
  if (route.pool_id === '') {
    if (route.grant_id !== '' || route.grant_version !== 0) return false;
  } else if (!id(route.pool_id) || !id(route.grant_id) || !positive(route.grant_version) || !positive(route.version)) return false;
  const confirmed = [value.binding];
  if (!validTestBindingFields(confirmed)) return false;
  const [binding] = confirmed;
  return binding.node_id === nodeID && binding.tool_profile_id === tester.id && binding.tool_profile_version === tester.version &&
    binding.tool_profile_sha256 === tester.profile.profile_sha256 && binding.worker_id === tester.worker_id &&
    binding.worker_id !== developer.worker_id && binding.command_id === tester.profile.command_id;
}

/** Durable request validation cannot depend on a currently loaded directory or
 * graph. Association and current eligibility are checked at their own seam. */
export function validTestSelectionFields(value: unknown): value is TaskDispatchTestSelection[] {
  if (!Array.isArray(value) || value.length > 64) return false;
  const seen = new Set<string>();
  return value.every(item => {
    if (!object(item) || !exact(item, ['node_id', 'tool_profile_id', 'expected_tool_version']) || !id(item.node_id) ||
        !id(item.tool_profile_id) || !positive(item.expected_tool_version) || seen.has(item.node_id)) return false;
    seen.add(item.node_id); return true;
  });
}
export function validTestSelections(value: unknown, workflow: Workflow, agentID: string): value is TaskDispatchTestSelection[] {
  return validTestSelectionFields(value) && value.every(item => associatedDispatchTests(workflow, agentID).some(node => node.id === item.node_id));
}

/** A historical six-field snapshot is shape-checked without replacing any pin
 * from today's profile directory or the unmodified published graph. */
export function validTestBindingFields(value: unknown): value is TaskDispatchTestBinding[] {
  if (!Array.isArray(value) || value.length > 64) return false;
  const seen = new Set<string>();
  return value.every(item => {
    if (!object(item) || !exact(item, ['node_id', 'tool_profile_id', 'tool_profile_version', 'tool_profile_sha256', 'worker_id', 'command_id']) ||
        !id(item.node_id) || !id(item.tool_profile_id) || !positive(item.tool_profile_version) || !hash(item.tool_profile_sha256) ||
        !id(item.worker_id) || !id(item.command_id) || seen.has(item.node_id)) return false;
    seen.add(item.node_id); return true;
  });
}

/** Frozen history is validated against its own derived graph, not refreshed
 * tool metadata. Omission means no explicit overrides in the current contract. */
export function validTestBindingsPlan(value: unknown): boolean {
  if (!object(value)) return false;
  if (!Object.hasOwn(value, 'test_bindings')) return true;
  if (!validTestBindingFields(value.test_bindings) || !id(value.agent_node_id) ||
      !object(value.workflow) || !Array.isArray(value.workflow.nodes)) return false;
  const nodes = value.workflow.nodes;
  if (nodes.filter(node => object(node) && node.id === value.agent_node_id && node.type === 'agent').length !== 1) return false;
  return value.test_bindings.every(item => {
    const matching = nodes.filter(node => object(node) && node.id === item.node_id), node = matching[0];
    if (matching.length !== 1 || !object(node) || node.type !== 'test' || !object(node.config) ||
        node.config.candidate_from !== value.agent_node_id || Object.hasOwn(node.config, 'artifact_from') ||
        node.config.tool_profile_id !== item.tool_profile_id || node.config.tool_profile_sha256 !== item.tool_profile_sha256 ||
        !Array.isArray(node.command) || node.command.length !== 1 || node.command[0] !== item.command_id) return false;
    return true;
  });
}
export function validTestBindingsDispatchReceipt(value: unknown, body: Record<string, unknown>): boolean {
  if (!object(value) || !validTestBindingsPlan(value)) return false;
  const requested = body.test_bindings;
  if (requested === undefined) return !Object.hasOwn(value, 'test_bindings') || Array.isArray(value.test_bindings) && value.test_bindings.length === 0;
  if (!object(value.workflow) || !Array.isArray(value.workflow.nodes) || !Array.isArray(value.workflow.edges) ||
      typeof body.agent_node_id !== 'string' || !validTestSelections(requested, value.workflow as unknown as Workflow, body.agent_node_id)) return false;
  if (requested.length && (!id(body.tool_profile_id) || !positive(body.expected_tool_version) ||
      !id(value.tool_profile_id) || !positive(value.tool_profile_version) ||
      value.tool_profile_id !== body.tool_profile_id || value.tool_profile_version !== body.expected_tool_version)) return false;
  if (requested.length && (value.project_id !== body.project_id || value.task_id !== body.task_id ||
      value.task_version !== body.expected_task_version || value.workflow_id !== body.workflow_id ||
      value.workflow_version !== body.workflow_version || value.agent_node_id !== body.agent_node_id)) return false;
  const frozen = Array.isArray(value.test_bindings) ? value.test_bindings : [];
  if (frozen.length !== requested.length) return false;
  return requested.every(item => frozen.some(binding => object(binding) && binding.node_id === item.node_id &&
    binding.tool_profile_id === item.tool_profile_id && binding.tool_profile_version === item.expected_tool_version));
}
