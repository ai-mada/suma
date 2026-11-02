import { expect, it } from 'vitest';
import type { Workflow } from './api';
import { associatedDispatchTests, readTestDispatchProfile, testDispatchProfile, validTestBindingsDispatchReceipt, validTestBindingsPlan, validTestSelections } from './task-dispatch-test-bindings-model';

const definition = { command_id: 'verify-command', adapter: 'command', protocol: 'text-v1', test_report_protocol: 'suma-junit-report-v1', read_only: false, profile_sha256: 'a'.repeat(64), executable_sha256: 'b'.repeat(64), tool_version: '1.0.0' };
const tester = { id: 'tester', project_id: 'project', worker_id: 'worker-test', name: 'Tests', version: 3, state: 'enabled' as const, profile: definition, available: true, reason: '', enabled_by: 'owner', created_at: '2026-11-03T00:00:00Z', updated_at: '2026-11-03T00:00:00Z', observation: { profile: { ...definition }, available: true, reason: '' } };
const workflow: Workflow = { id: 'flow', name: 'Development', version: 2, nodes: [{ id: 'code', name: 'Code', type: 'agent' }, { id: 'verify', name: 'Verify', type: 'test', command: ['verify-command'], config: { candidate_from: 'code', tool_profile_id: 'tester', tool_profile_sha256: definition.profile_sha256 } }, { id: 'foreign', name: 'Other', type: 'test', config: { candidate_from: 'other' } }], edges: [{ from: 'code', to: 'verify' }] };
const selection = { node_id: 'verify', tool_profile_id: 'tester', expected_tool_version: 3 };
const binding = { node_id: 'verify', tool_profile_id: 'tester', tool_profile_version: 3, tool_profile_sha256: definition.profile_sha256, worker_id: 'worker-test', command_id: 'verify-command' };
const body = { project_id: 'project', task_id: 'task', expected_task_version: 4, workflow_id: 'flow', workflow_version: 2, agent_node_id: 'code', tool_profile_id: 'developer', expected_tool_version: 2, test_bindings: [selection] };
const plan = () => ({ project_id: 'project', task_id: 'task', task_version: 4, workflow_id: 'flow', workflow_version: 2, agent_node_id: 'code', tool_profile_id: 'developer', tool_profile_version: 2, workflow: structuredClone(workflow), test_bindings: [{ ...binding }] });

it('T216 verifies exact current independent test selections and frozen same-node receipts without refreshing historical metadata', () => {
  expect(associatedDispatchTests(workflow, 'code').map(node => node.id)).toEqual(['verify']);
  expect(testDispatchProfile(tester, 'project')).toBeDefined();
  expect(readTestDispatchProfile(tester, tester, 'project', 'worker-code')?.worker_id).toBe('worker-test');
  for (const raw of [{ ...tester, version: 4 }, { ...tester, worker_id: 'worker-code' }, { ...tester, state: 'paused' }, { ...tester, available: false }, { ...tester, observation: { ...tester.observation, available: false } }, { ...tester, profile: { ...definition, test_report_protocol: undefined } }]) expect(readTestDispatchProfile(raw, tester, 'project', 'worker-code')).toBeUndefined();
  expect(validTestSelections([selection], workflow, 'code')).toBe(true);
  for (const value of [null, [selection, selection], [{ ...selection, node_id: 'foreign' }], [{ ...selection, extra: true }], [{ ...selection, expected_tool_version: 0 }], Array(65).fill(selection)]) expect(validTestSelections(value, workflow, 'code')).toBe(false);
  expect(validTestBindingsPlan({})).toBe(true);
  expect(validTestBindingsPlan(plan())).toBe(true); expect(validTestBindingsDispatchReceipt(plan(), body)).toBe(true);
  for (const mutate of [
    (value: ReturnType<typeof plan>) => { value.test_bindings[0].worker_id = ''; },
    (value: ReturnType<typeof plan>) => { value.test_bindings[0].command_id = 'other'; },
    (value: ReturnType<typeof plan>) => { value.test_bindings[0].tool_profile_sha256 = 'c'.repeat(64); },
    (value: ReturnType<typeof plan>) => { value.workflow.nodes[1].config!.candidate_from = 'other'; },
    (value: ReturnType<typeof plan>) => { value.test_bindings.push({ ...binding }); },
  ]) { const value = plan(); mutate(value); expect(validTestBindingsPlan(value)).toBe(false); }
  expect(validTestBindingsDispatchReceipt({ ...plan(), task_id: 'other' }, body)).toBe(false);
  expect(validTestBindingsDispatchReceipt({ ...plan(), tool_profile_id: 'different-developer' }, body)).toBe(false);
  expect(validTestBindingsDispatchReceipt({ ...plan(), tool_profile_version: 3 }, body)).toBe(false);
  expect(validTestBindingsDispatchReceipt({ ...plan(), tool_profile_version: '2' }, body)).toBe(false);
  expect(validTestBindingsDispatchReceipt(plan(), { ...body, expected_tool_version: '2' })).toBe(false);
  expect(validTestBindingsDispatchReceipt(plan(), { ...body, test_bindings: [{ ...selection, expected_tool_version: 2 }] })).toBe(false);
  expect(validTestBindingsDispatchReceipt(plan(), { ...body, test_bindings: undefined })).toBe(false);
  expect(validTestBindingsPlan({ test_bindings: null })).toBe(false);
  const original = JSON.stringify(workflow); validTestBindingsPlan(plan()); expect(JSON.stringify(workflow)).toBe(original);
});
