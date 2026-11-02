// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, expect, it, vi } from 'vitest';
import { ApiViewProvider } from '../api-context';
import { setLocale } from '../i18n';
import { PoolConfigConnectionPanel, type PoolConfigConnectionPanelProps } from './PoolConfigConnectionPanel';

let root: Root | undefined, host: HTMLDivElement | undefined;
afterEach(async () => { if (root) await act(async () => root!.unmount()); host?.remove(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
const session = { user: { id: 'owner', name: 'Owner', email: 'owner@example.test' }, organization: { id: 'tenant-one', name: 'Team' }, projects: [] };
const stamp = '2026-11-02T01:00:00Z';
const props: PoolConfigConnectionPanelProps = { ownerTenantID: 'tenant-one', poolID: 'pool-one', runnerID: 'runner-one', workerID: 'pool-runner-one',
  prepared: { shell: 'powershell', output: 'D:/new/config.json', scopes: [{ tenant_id: 'tenant-one', project_id: 'project-one' }, { tenant_id: 'tenant-two', project_id: 'project-two' }] } };
const response = () => ({ pool_id: props.poolID, runner_id: props.runnerID, worker_id: props.workerID, pool_status: 'active', checked_at: stamp,
  runner: { status: 'active', version: 1, credential_version: 1, credential_state: 'valid', credential_expires_at: '2026-11-03T01:00:00Z', historical_last_seen_at: '2026-11-01T01:00:00Z' },
  scope: { tenant_id: 'tenant-one', project_id: 'project-one', grant_id: 'grant', grant_version: 1, grant_status: 'approved', shared_project_name: 'Project', can_open_project: true, route_state: 'current', source_current: true, source_reason: 'current', inventory_observed_at: stamp, inventory_expires_at: '2026-11-02T01:01:00Z', inventory_item_count: 0 } });
async function settle() { await act(async () => { for (let i = 0; i < 24; i++) await Promise.resolve(); }); }
async function change(field: string, value: string) {
  const element = host!.querySelector<HTMLInputElement | HTMLSelectElement>('[data-pool-connection-field="' + field + '"]')!;
  await act(async () => { Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype, 'value')!.set!.call(element, value); element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', { bubbles: true })); });
}
async function click(selector: string) { await act(async () => host!.querySelector<HTMLElement>(selector)!.click()); await settle(); }

it('T212 opens a progressive connection guide with explicit consent and clears stale exact-scope reads on edits, revocation and caller changes', async () => {
  await setLocale('en-US', { persist: false }); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const copy = vi.fn().mockResolvedValue(undefined); Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } });
  const paths: string[] = []; let pending: ((value: Response) => void) | undefined, deny = false, defer = false, snapshot = response();
  vi.stubGlobal('fetch', vi.fn(async (path: string, options: RequestInit = {}) => {
    paths.push(String(path)); expect(options.method || 'GET').toBe('GET');
    if (defer) return new Promise<Response>(resolve => { pending = resolve; });
    return deny ? Response.json({ error: { code: 'forbidden', message: 'Revoked' } }, { status: 403 }) : Response.json(snapshot);
  }));
  host = document.createElement('div'); document.body.append(host); root = createRoot(host);
  const draw = async (identity = session, disabled = false) => { await act(async () => root!.render(<ApiViewProvider session={identity}><PoolConfigConnectionPanel {...props} disabled={disabled} /></ApiViewProvider>)); await settle(); };
  await draw();
  expect(host.querySelector<HTMLDetailsElement>('[data-pool-config-connection]')?.open).toBe(false); expect(paths).toEqual([]);
  await change('identity', "D:/identity/O'Brien.json");
  expect(host.querySelector<HTMLInputElement>('[data-pool-connection-field=state]')?.value).toBe('D:/new/config.json.runner-state');
  expect(host.querySelector<HTMLButtonElement>('[data-pool-connection-copy=start]')?.disabled).toBe(true);
  await click('[data-pool-connection-field=discovery]'); await click('[data-pool-connection-copy=start]');
  expect(copy.mock.calls[0][0]).toContain('--maintenance-only --allow-installed-tool-discovery'); expect(copy.mock.calls[0][0]).not.toContain('--allow-exec');
  await change('mode', 'tasks'); await click('[data-pool-connection-copy=start]');
  expect(copy.mock.calls[1][0]).toContain('--allow-exec'); expect(copy.mock.calls[1][0]).not.toContain('--allow-installed-tool-discovery');
  await click('[data-pool-connection-refresh]');
  expect(paths[0]).toContain('/execution-pools/pool-one/runners/runner-one/readiness?tenant_id=tenant-one&project_id=project-one');
  expect(host.querySelector('[data-source-current=true]')).not.toBeNull(); expect(host.textContent).toContain('0 observed tool entries'); expect(host.textContent).toContain('Last contact (historical)');
  expect(host.querySelector('.pool-connection-facts>p')?.textContent).toMatch(/\d{1,2}:\d{2}:\d{2}/);
  const historical = [...host.querySelectorAll('.pool-connection-facts dl>div')].find(row => row.querySelector('dt')?.textContent === 'Last contact (historical)');
  expect(historical?.querySelector('dd')?.textContent).toMatch(/\d{1,2}:\d{2}:\d{2}/);
  expect(host.querySelector<HTMLAnchorElement>('[data-pool-connection-tools]')?.getAttribute('href')).toContain('panel=pool-tool%3Apool-one%3Arunner-one');
  Object.assign(snapshot.scope, { source_current: false, source_reason: 'inventory_unreported', inventory_observed_at: '', inventory_expires_at: '' });
  snapshot.runner.credential_state = 'version_mismatch';
  Reflect.deleteProperty(snapshot.scope, 'inventory_item_count');
  await click('[data-pool-connection-refresh]');
  expect(host.querySelector('[data-pool-connection-readiness]')?.textContent).toContain('Inventory has not been reported.');
  expect(host.querySelector('[data-pool-connection-readiness]')?.textContent).not.toContain('Read-time validity expired');
  expect(host.querySelector('[data-pool-connection-tools]')).not.toBeNull();
  Object.assign(snapshot.scope, { can_open_project: false, route_state: 'restricted', source_reason: 'project_access_required' });
  await click('[data-pool-connection-refresh]');
  expect(host.querySelector('[data-pool-connection-readiness]')?.textContent).toContain('Project access is required; private inventory was not read.');
  expect(host.querySelector('[data-pool-connection-tools]')).toBeNull();
  snapshot = response();
  defer = true; await click('[data-pool-connection-refresh]');
  await change('scope', '1'); await change('scope', '0');
  await act(async () => pending!(Response.json(response()))); await settle();
  expect(host.querySelector('[data-pool-connection-readiness]')).toBeNull();
  defer = false; deny = true; await click('[data-pool-connection-refresh]');
  expect(host.querySelector('[data-pool-connection-status=forbidden]')).not.toBeNull(); expect(host.querySelector('[data-pool-connection-tools]')).toBeNull();
  await draw({ ...session, user: { ...session.user, id: 'different-owner' } });
  expect(host.querySelector<HTMLInputElement>('[data-pool-connection-field=identity]')?.value).toBe('');
  await draw(session, true); expect(host.querySelector<HTMLButtonElement>('[data-pool-connection-refresh]')?.disabled).toBe(true);
  await setLocale('zh-CN', { persist: false }); await draw(); expect(host.textContent).toContain('下一步：连接这台机器');
});
