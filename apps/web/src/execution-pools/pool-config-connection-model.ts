import type { PoolRunnerReadiness, ProjectPoolRunnerPage } from '../../../../packages/contracts/src';
import { runnerSetupLiteral } from '../cursor-setup-guide-model';
import { wellFormed } from '../i18n/catalog';
import { toolDiscoveryTimestampNs } from '../tool-discovery-model';
import { validPoolConfigID, validPoolConfigPath } from './pool-config-preparation-model';

export type PreparedPoolConnection = {
  shell: 'powershell' | 'bash';
  output: string;
  scopes: Array<{ tenant_id: string; project_id: string }>;
};
export type PoolConfigConnectionInput = {
  prepared: PreparedPoolConnection;
  identity: string;
  state: string;
  control: string;
  mode: 'maintenance' | 'tasks';
  discovery: boolean;
};
export type PoolToolLocator = { poolID: string; runnerID: string };
/** Match the native cloudControlOrigin boundary. The browser never rewrites a
 * host to make an insecure or differently paired address appear acceptable. */
export function validPoolConnectionControl(value: string): boolean {
  if (!wellFormed(value) || new TextEncoder().encode(value).length > 2048 || /[\p{Cc}\u2028\u2029]/u.test(value) || value.includes('?') || value.includes('#')) return false;
  try {
    const authority = value.match(/^https?:\/\/([^/?#]*)/);
    if (!authority?.[1] || /[@\\]/.test(authority[1])) return false;
    const url = new URL(value);
    // Preserve the explicit path prefix and trailing slash in copied argv.
    // Native identity validation owns any trailing-slash canonical comparison.
    // WHATWG canonicalizes HTTP aliases such as 127.1 and LOCALHOST. Native
    // cloudControlOrigin checks the literal host, so use the raw authority.
    return !url.username && !url.password &&
      (url.protocol === 'https:' || url.protocol === 'http:' && /^(?:localhost|127\.0\.0\.1|\[::1\])(?::[0-9]{1,5})?$/.test(authority[1]));
  } catch { return false; }
}
const object = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value);
const exact = (value: Record<string, unknown>, required: string[], optional: string[] = []) =>
  required.every(key => Object.hasOwn(value, key)) &&
  Object.keys(value).every(key => required.includes(key) || optional.includes(key));
const text = (value: unknown): value is string => typeof value === 'string' && value.length > 0;
const integer = (value: unknown) => Number.isSafeInteger(value) && (value as number) >= 1;
const timestamp = (value: unknown): value is string => {
  try { toolDiscoveryTimestampNs(value); return true; } catch { return false; }
};

/** Reuse the original target-shell quoting and local path contract. Copying
 * either command is never evidence that a file exists or that a node started. */
export function poolConfigConnectionCommands(input: PoolConfigConnectionInput): { check: string; start?: string } | undefined {
  const { prepared, identity, state, control, mode, discovery } = input;
  if (!prepared || !['powershell', 'bash'].includes(prepared.shell) ||
      !validPoolConfigPath(prepared.output) || !validPoolConfigPath(identity) ||
      !validPoolConfigPath(state) || !['maintenance', 'tasks'].includes(mode) || typeof discovery !== 'boolean') return;
  if (!validPoolConnectionControl(control)) return;
  const quote = (value: string) => runnerSetupLiteral(value, prepared.shell);
  const base = 'suma-runner --control ' + quote(control) +
    ' --pool-config ' + quote(prepared.output) + ' --pool-identity ' + quote(identity);
  const check = 'suma-runner --check-pool-connection --control ' + quote(control) +
    ' --pool-config ' + quote(prepared.output) + ' --pool-identity ' + quote(identity);
  const start = mode === 'maintenance'
    ? discovery ? base + ' --state ' + quote(state) + ' --maintenance-only --allow-installed-tool-discovery' : undefined
    : base + ' --state ' + quote(state) + ' --allow-exec' + (discovery ? ' --allow-installed-tool-discovery' : '');
  return { check, ...(start ? { start } : {}) };
}

/** A bounded locator is navigation intent, never a project or node grant. */
export function parsePoolToolLocator(value: string): PoolToolLocator | undefined {
  const parts = value.split(':');
  return parts.length === 3 && parts[0] === 'pool-tool' && parts.slice(1).every(validPoolConfigID)
    ? { poolID: parts[1], runnerID: parts[2] } : undefined;
}
export function buildPoolToolHref(projectID: string, target: PoolToolLocator): string | undefined {
  if (!validPoolConfigID(projectID) || !validPoolConfigID(target.poolID) || !validPoolConfigID(target.runnerID)) return;
  return '?' + new URLSearchParams({ project: projectID, view: 'settings', sub: 'tools', panel: 'pool-tool:' + target.poolID + ':' + target.runnerID });
}
/** A public locator never supplies worker authority. The current project's
 * original shared-worker response must identify this exact active Pool row. */
export function currentPoolToolWorker(target: PoolToolLocator, page: ProjectPoolRunnerPage | undefined): string | undefined {
  if (!page || !timestamp(page.as_of) || !Array.isArray(page.items)) return;
  const rows = page.items.filter(row => row.id === target.runnerID && row.pool_id === target.poolID &&
    row.worker_id === 'pool-' + target.runnerID && row.status === 'active' &&
    timestamp(row.credential_expires_at) && toolDiscoveryTimestampNs(row.credential_expires_at) > toolDiscoveryTimestampNs(page.as_of));
  return rows.length === 1 ? rows[0].worker_id : undefined;
}
export function poolRunnerReadinessPath(poolID: string, runnerID: string, tenantID: string, projectID: string): string | undefined {
  if (![poolID, runnerID, tenantID, projectID].every(validPoolConfigID)) return;
  return '/execution-pools/' + encodeURIComponent(poolID) + '/runners/' + encodeURIComponent(runnerID) +
    '/readiness?' + new URLSearchParams({ tenant_id: tenantID, project_id: projectID });
}

/** Keep current authorization, historical last contact, and inventory facts
 * separate. Unknown counts stay absent; a preparation SHA is not a runtime SHA. */
export function readPoolRunnerReadiness(value: unknown, target: {
  poolID: string; runnerID: string; workerID: string; tenantID: string; projectID: string;
}): PoolRunnerReadiness | undefined {
  if (!object(value) || !exact(value, ['pool_id', 'runner_id', 'worker_id', 'pool_status', 'checked_at', 'runner', 'scope']) ||
      value.pool_id !== target.poolID || value.runner_id !== target.runnerID || value.worker_id !== target.workerID ||
      !text(value.pool_status) || !timestamp(value.checked_at) || !object(value.runner) || !object(value.scope)) return;
  const runner = value.runner, scope = value.scope;
  if (!exact(runner, ['status', 'version', 'credential_version', 'credential_state', 'credential_expires_at', 'historical_last_seen_at']) ||
      !text(runner.status) || !integer(runner.version) || !integer(runner.credential_version) ||
      !['valid', 'expired', 'revoked', 'version_mismatch'].includes(String(runner.credential_state)) ||
      !timestamp(runner.credential_expires_at) ||
      runner.historical_last_seen_at !== '' && !timestamp(runner.historical_last_seen_at)) return;
  if (!exact(scope, ['tenant_id', 'project_id', 'grant_id', 'grant_version', 'grant_status', 'shared_project_name', 'can_open_project', 'route_state', 'source_current', 'source_reason', 'inventory_observed_at', 'inventory_expires_at'],
      ['inventory_item_count']) ||
      scope.tenant_id !== target.tenantID || scope.project_id !== target.projectID ||
      typeof scope.grant_id !== 'string' || !Number.isSafeInteger(scope.grant_version) || (scope.grant_version as number) < 0 ||
      typeof scope.grant_status !== 'string' || typeof scope.shared_project_name !== 'string' ||
      typeof scope.can_open_project !== 'boolean' || typeof scope.source_current !== 'boolean' ||
      !['restricted', 'unconfigured', 'different', 'current'].includes(String(scope.route_state)) ||
      !['project_access_required', 'route_not_current', 'inventory_unreported', 'inventory_stale', 'maintenance_unknown', 'maintenance_stale', 'observation_mismatch', 'current'].includes(String(scope.source_reason))) return;
  if (scope.inventory_observed_at !== '' && !timestamp(scope.inventory_observed_at) ||
      scope.inventory_expires_at !== '' && !timestamp(scope.inventory_expires_at)) return;
  if (!scope.can_open_project && (scope.route_state !== 'restricted' || scope.source_reason !== 'project_access_required' ||
      scope.inventory_observed_at !== '' || scope.inventory_expires_at !== '')) return;
  if (scope.source_current) {
    if (scope.source_reason !== 'current' || scope.route_state !== 'current' || !scope.can_open_project ||
        !timestamp(scope.inventory_observed_at) || !timestamp(scope.inventory_expires_at) ||
        toolDiscoveryTimestampNs(scope.inventory_observed_at) > toolDiscoveryTimestampNs(value.checked_at) ||
        toolDiscoveryTimestampNs(scope.inventory_expires_at) <= toolDiscoveryTimestampNs(value.checked_at) ||
        toolDiscoveryTimestampNs(scope.inventory_expires_at) - toolDiscoveryTimestampNs(scope.inventory_observed_at) !== 60_000_000_000n ||
        Object.hasOwn(scope, 'inventory_item_count') && (!Number.isSafeInteger(scope.inventory_item_count) || (scope.inventory_item_count as number) < 0)) return;
  } else if (Object.hasOwn(scope, 'inventory_item_count') || scope.source_reason === 'current') return;
  return structuredClone(value) as unknown as PoolRunnerReadiness;
}

/** Monotonic elapsed time can only consume the server's read-time lease. */
export function poolRunnerReadinessRemainingMs(value: PoolRunnerReadiness, elapsedMs: number): number {
  if (!Number.isFinite(elapsedMs) || elapsedMs < 0 || value.runner.credential_state !== 'valid') return 0;
  try {
    const checked = toolDiscoveryTimestampNs(value.checked_at);
    const ends = [toolDiscoveryTimestampNs(value.runner.credential_expires_at)];
    if (value.scope.inventory_expires_at) ends.push(toolDiscoveryTimestampNs(value.scope.inventory_expires_at));
    const remaining = Number((ends.reduce((a, b) => a < b ? a : b) - checked) / 1_000_000n);
    return Math.max(0, remaining - elapsedMs);
  } catch { return 0; }
}
