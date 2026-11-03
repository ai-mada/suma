import { expect, it } from 'vitest';
import {
  buildPoolToolHref, currentPoolToolWorker, parsePoolToolLocator,
  poolConfigConnectionCommands, poolRunnerReadinessPath,
  poolRunnerReadinessRemainingMs, readPoolRunnerReadiness,
  validPoolConnectionControl,
} from './pool-config-connection-model';

const target = { poolID: 'pool-one', runnerID: 'runner-one', workerID: 'pool-runner-one', tenantID: 'tenant-one', projectID: 'project-one' };
const at = '2026-11-02T01:00:00Z', end = '2026-11-02T01:01:00Z';
const observation = () => ({
  pool_id: target.poolID, runner_id: target.runnerID, worker_id: target.workerID, pool_status: 'active', checked_at: at,
  runner: { status: 'active', version: 1, credential_version: 2, credential_state: 'valid', credential_expires_at: end, historical_last_seen_at: '' },
  scope: { tenant_id: target.tenantID, project_id: target.projectID, grant_id: 'grant-one', grant_version: 2, grant_status: 'approved', shared_project_name: 'Project', can_open_project: true,
    route_state: 'current', source_current: true, source_reason: 'current', inventory_observed_at: at, inventory_expires_at: end, inventory_item_count: 0 },
});

it('T212 keeps explicit command modes, exact current source leases and bounded project navigation separate from preparation success', () => {
  const input = { prepared: { shell: 'powershell' as const, output: "D:/new/O'Brien $(no).json", scopes: [{ tenant_id: target.tenantID, project_id: target.projectID }] },
    identity: 'D:/private/pool identity.json', state: 'D:/new/config.runner-state', control: 'https://control.example.test', mode: 'maintenance' as const, discovery: false, preparation: false };
  const check = poolConfigConnectionCommands(input)!;
  expect(check.check).toBe("suma-runner --check-pool-connection --control 'https://control.example.test' --pool-config 'D:/new/O''Brien $(no).json' --pool-identity 'D:/private/pool identity.json'");
  expect(check.start).toBeUndefined();
  const inspection = poolConfigConnectionCommands({ ...input, discovery: true })!;
  expect(inspection.start).toContain('--maintenance-only --allow-installed-tool-discovery');
  expect(inspection.start).not.toMatch(/--allow-exec|--describe-tools/);
  const preparation = poolConfigConnectionCommands({...input,discovery:true,preparation:true})!;
  expect(preparation.start).toContain('--allow-installed-tool-discovery --allow-tool-preparation');expect(preparation.start).not.toContain('--allow-exec');expect(preparation.check).not.toContain('--allow-tool-preparation');expect(inspection.start).not.toContain('--allow-tool-preparation');expect(poolConfigConnectionCommands({...input,preparation:true})).toBeUndefined();
  expect(poolConfigConnectionCommands({...input,mode:'tasks',discovery:true,preparation:true})?.start).toContain('--allow-exec --allow-installed-tool-discovery --allow-tool-preparation');
  const tasks = poolConfigConnectionCommands({ ...input, mode: 'tasks', prepared: { ...input.prepared, shell: 'bash' } })!;
  expect(tasks.start).toContain("'D:/new/O'\"'\"'Brien $(no).json'");
  expect(tasks.start).toContain('--allow-exec'); expect(tasks.start).not.toContain('--allow-installed-tool-discovery');
  expect(poolConfigConnectionCommands({ ...input, identity: '' })).toBeUndefined();
  for (const control of ['https://control.example.test/suma', 'https://control.example.test/suma/', 'http://127.0.0.1:4310/suma']) {
    expect(poolConfigConnectionCommands({ ...input, control })?.check).toContain("--control '" + control + "'");
  }
  for (const control of ['http://lan.example.test', 'http://pool.localhost', 'http://192.168.1.2', 'https://user@secure.example.test', 'https://@secure.example.test', 'https://secure.example.test?q=1', 'https://secure.example.test?', 'https://secure.example.test#fragment', 'https://secure.example.test/\nother', 'https://'+'a'.repeat(2048)]) expect(validPoolConnectionControl(control)).toBe(false);
  for (const control of ['http://127.1/suma', 'http://2130706433/suma', 'http://LOCALHOST/suma']) expect(validPoolConnectionControl(control)).toBe(false);
  for (const control of ['https://secure.example.test', 'http://localhost:4310', 'http://127.0.0.1:4310', 'http://[::1]:4310']) expect(validPoolConnectionControl(control)).toBe(true);
  expect(poolConfigConnectionCommands({ ...input, state: 'D:/state\n--allow-exec' })).toBeUndefined();
  expect(poolRunnerReadinessPath(target.poolID, target.runnerID, target.tenantID, target.projectID)).toContain('tenant_id=tenant-one&project_id=project-one');
  const value = readPoolRunnerReadiness(observation(), target)!;
  expect(value.scope.inventory_item_count).toBe(0);
  expect(poolRunnerReadinessRemainingMs(value, 1000)).toBe(59000);
  expect(poolRunnerReadinessRemainingMs(value, 60000)).toBe(0);
  expect(poolRunnerReadinessRemainingMs(value, -1)).toBe(0);
  for (const raw of [
    { ...observation(), worker_id: 'another' },
    { ...observation(), scope: { ...observation().scope, tenant_id: 'another' } },
    { ...observation(), scope: { ...observation().scope, inventory_item_count: null } },
    { ...observation(), scope: { ...observation().scope, source_current: false } },
    { ...observation(), scope: { ...observation().scope, source_reason: 'online' } },
    { ...observation(), scope: { ...observation().scope, inventory_observed_at: '2026-11-02T01:00:01Z' } },
    { ...observation(), scope: { ...observation().scope, inventory_expires_at: '2026-11-02T01:01:01Z' } },
    { ...observation(), ready: true },
  ]) expect(readPoolRunnerReadiness(raw, target)).toBeUndefined();
  const restricted = observation();
  Object.assign(restricted.scope, { can_open_project: false, route_state: 'restricted', source_current: false, source_reason: 'project_access_required', inventory_observed_at: '', inventory_expires_at: '' });
  Reflect.deleteProperty(restricted.scope, 'inventory_item_count');
  expect(readPoolRunnerReadiness(restricted, target)?.scope.inventory_item_count).toBeUndefined();
  const noCount = observation(); Reflect.deleteProperty(noCount.scope, 'inventory_item_count');
  expect(readPoolRunnerReadiness(noCount, target)?.scope.inventory_item_count).toBeUndefined();
  const locator = parsePoolToolLocator('pool-tool:pool-one:runner-one')!;
  expect(locator).toEqual({ poolID: target.poolID, runnerID: target.runnerID });
  for (const invalid of ['pool-tool:pool-one:runner-one:extra', 'pool-tool:pool-one:', 'pool-tool:../pool:runner']) expect(parsePoolToolLocator(invalid)).toBeUndefined();
  const href = buildPoolToolHref(target.projectID, locator)!;
  expect(new URLSearchParams(href.slice(1)).get('panel')).toBe('pool-tool:pool-one:runner-one');
  const row = { id: target.runnerID, pool_id: target.poolID, worker_id: target.workerID, name: 'Machine', status: 'active', credential_expires_at: end };
  expect(currentPoolToolWorker(locator, { items: [row], as_of: at })).toBe(target.workerID);
  expect(currentPoolToolWorker(locator, { items: [{ ...row, pool_id: 'other' }], as_of: at })).toBeUndefined();
  expect(currentPoolToolWorker(locator, { items: [{ ...row, credential_expires_at: at }], as_of: at })).toBeUndefined();
  expect(currentPoolToolWorker(locator, { items: [row, row], as_of: at })).toBeUndefined();
});
