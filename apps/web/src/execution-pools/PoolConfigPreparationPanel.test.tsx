// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ApiViewProvider } from '../api-context';
import { setLocale } from '../i18n';
import { PoolConfigPreparationPanel, type PoolConfigPreparationPanelProps } from './PoolConfigPreparationPanel';
import { PoolWorkspace } from './PoolWorkspace';
const owner='a'.repeat(32), pool='b'.repeat(32), runnerID='c'.repeat(32), at='2026-11-02T00:00:00Z';
const session={ user: { id: 'd'.repeat(32), name: 'Owner', email: 'owner@example.test' }, organization: { id: owner, name: 'Team' }, projects: [] };
const runner={ id: runnerID, pool_id: pool, worker_id: 'pool-'+runnerID, max_slots: 2, status: 'active' };
const grant={ id: 'e'.repeat(32), pool_id: pool, owner_tenant_id: owner, tenant_id: 'consumer-one', project_id: 'project-one', status: 'approved', shared_project_name: 'Reviewed project' };
let root: Root, host: HTMLDivElement, props: PoolConfigPreparationPanelProps, copy: ReturnType<typeof vi.fn>, clipboard: PropertyDescriptor|undefined;
beforeEach(async () => {
  await setLocale('en-US', { persist: false });
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  copy=vi.fn().mockResolvedValue(undefined);
  clipboard=Object.getOwnPropertyDescriptor(navigator, 'clipboard');
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: copy } });
  vi.stubGlobal('fetch', vi.fn());
  host=document.createElement('div');
  document.body.append(host);
  root=createRoot(host);
  props={ ownerTenantID: owner, poolID: pool, runner, grants: [grant, { ...grant, id: 'f'.repeat(32), status: 'revoked' }, { ...grant, id: '0'.repeat(32), pool_id: 'wrong-pool' }] };
});
afterEach(async () => {
  await act(async () => root.unmount()); host.remove(); document.body.innerHTML=''; if(clipboard)
    Object.defineProperty(navigator, 'clipboard', clipboard);
  else
    Reflect.deleteProperty(navigator, 'clipboard'); vi.unstubAllGlobals(); vi.restoreAllMocks();
});
async function settle() {
  await act(async () => {
    for(let i=0; i<28; i++)
      await Promise.resolve();
  });
}
async function draw(identity=session) {
  await act(async () => root.render(<ApiViewProvider session={identity}>
    <PoolConfigPreparationPanel {...props} />
  </ApiViewProvider>)); await settle();
}
async function input(selector: string, value: string) { const field=document.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!; expect(field, selector).not.toBeNull(); await act(async () => { Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement? HTMLSelectElement.prototype:HTMLInputElement.prototype, 'value')!.set!.call(field, value); field.dispatchEvent(new Event(field instanceof HTMLSelectElement? 'change':'input', { bubbles: true })); }); }
async function click(selector: string) { const element=document.querySelector<HTMLButtonElement>(selector)!; expect(element, selector).not.toBeNull(); await act(async () => { element.focus(); element.click(); }); await settle(); }
const scope=(row: string, field: string) => '[data-pool-config-row="'+row+'"] [data-pool-config-scope-field="'+field+'"]';
async function fill() { await input('[data-pool-config-field=shell]', 'powershell'); await input('[data-pool-config-field=output]', "D:/configs/O'Brien $(Write-Output unexpected).json"); await input('[data-pool-config-project=scope-1]', grant.id); await input(scope('scope-1', 'workspace'), 'D:/work/project-one'); }
it('T210 clears all private references only when the actual project tuple changes, preserving same-scope modes and sibling rows', async () => {
  const secondGrant={ ...grant, id: '2'.repeat(32), tenant_id: 'consumer-b', project_id: 'project-b' };
  const sameScopeGrant={ ...grant, id: '3'.repeat(32) };
  props={ ...props, grants: [grant, secondGrant, sameScopeGrant] };
  await draw();
  await fill();
  const references={
    workspace: '/srv/a/workspace',
    commands_file: '/srv/a/commands.json',
    repository: '/srv/a/repository',
    base_commit: 'a'.repeat(40),
    worktrees: '/srv/a/worktrees',
    model_deployments_file: '/srv/a/models.json',
    external_config_file: '/srv/a/external.json',
  };
  const readReferences=(row: string) => Object.fromEntries(Object.keys(references).map(field => [field, document.querySelector<HTMLInputElement>(scope(row, field))!.value]));
  const writeReferences=async (row: string) => {
    for(const [field, value] of Object.entries(references)) await input(scope(row, field), value);
  };
  await writeReferences('scope-1');
  await click('[data-pool-config-add]');
  await input(scope('scope-2', 'tenant_id'), 'sibling-tenant');
  await input(scope('scope-2', 'project_id'), 'sibling-project');
  await writeReferences('scope-2');
  await click('[data-pool-config-copy]');
  expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();

  await input('[data-pool-config-project=scope-1]', grant.id);
  expect(readReferences('scope-1')).toEqual(references);
  await input('[data-pool-config-project=scope-1]', '');
  expect(readReferences('scope-1')).toEqual(references);
  await input('[data-pool-config-project=scope-1]', sameScopeGrant.id);
  expect(readReferences('scope-1')).toEqual(references);
  await click('[data-pool-config-copy]');
  expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();

  await input('[data-pool-config-project=scope-1]', secondGrant.id);
  expect(document.querySelector<HTMLInputElement>(scope('scope-1', 'tenant_id'))!.value).toBe(secondGrant.tenant_id);
  expect(document.querySelector<HTMLInputElement>(scope('scope-1', 'project_id'))!.value).toBe(secondGrant.project_id);
  expect(Object.values(readReferences('scope-1'))).toEqual(Array(7).fill(''));
  expect(readReferences('scope-2')).toEqual(references);
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  expect(document.querySelector('[data-pool-config-message]')?.textContent).toContain('Project scope changed');

  await writeReferences('scope-1');
  await input('[data-pool-config-project=scope-1]', '');
  expect(readReferences('scope-1')).toEqual(references);
  await input(scope('scope-1', 'project_id'), secondGrant.project_id);
  expect(readReferences('scope-1')).toEqual(references);
  await click('[data-pool-config-copy]');
  await input(scope('scope-1', 'project_id'), 'different-project');
  expect(Object.values(readReferences('scope-1'))).toEqual(Array(7).fill(''));
  expect(readReferences('scope-2')).toEqual(references);
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  await writeReferences('scope-1');
  await input(scope('scope-1', 'tenant_id'), 'different-tenant');
  expect(Object.values(readReferences('scope-1'))).toEqual(Array(7).fill(''));
  expect(readReferences('scope-2')).toEqual(references);
  expect(fetch).not.toHaveBeenCalled();
});
it('T209 uses only current loaded approvals or explicit manual IDs, copies one private command and preserves keyboard row focus without storage or project reads', async () => {
  const stored=vi.spyOn(Storage.prototype, 'setItem');
  await draw();
  const options=[...document.querySelectorAll<HTMLOptionElement>('[data-pool-config-project=scope-1] option')];
  expect(options.map(option => option.value)).toEqual(['', grant.id]);
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  await fill();
  expect(document.querySelector<HTMLInputElement>(scope('scope-1', 'project_id'))?.readOnly).toBe(true);
  await click('[data-pool-config-add]');
  expect(document.activeElement).toBe(document.querySelector(scope('scope-2', 'tenant_id')));
  await input(scope('scope-2', 'tenant_id'), 'consumer-two');
  await input(scope('scope-2', 'project_id'), 'project-two');
  await input(scope('scope-2', 'workspace'), '/srv/project-two');
  await input(scope('scope-2', 'commands_file'), '/srv/tools.json');
  await click('[data-pool-config-add]');
  await click('[data-pool-config-remove=scope-3]');
  expect(document.activeElement).toBe(document.querySelector(scope('scope-2', 'tenant_id')));
  await click('[data-pool-config-copy]');
  const command=document.querySelector('[data-pool-config-prepare]')!.textContent!, request=JSON.parse(document.querySelector('[data-pool-config-input]')!.textContent!);
  expect(copy).toHaveBeenCalledExactlyOnceWith(command);
  expect(request).toMatchObject({ pool_id: pool, runner_id: runnerID, slots: 1, scopes: [{ tenant_id: 'consumer-one', project_id: 'project-one', commands_file: '' }, { tenant_id: 'consumer-two', project_id: 'project-two', commands_file: '/srv/tools.json' }] });
  expect(Object.keys(request)).toHaveLength(4);
  expect(request.scopes.every((row: object) => Object.keys(row).length===9)).toBe(true);
  expect(fetch).not.toHaveBeenCalled();
  expect(stored).not.toHaveBeenCalled();
  await act(async () => setLocale('zh-CN', { persist: false }));
  expect(document.body.textContent).toContain('命令已复制');
  expect(document.body.textContent).not.toContain('pool-config-preparation.');
  expect(document.body.textContent).toContain(pool);
});
it('T209 invalidates scripts immediately on edits, scope or current approval changes including ABA and ignores a late clipboard acknowledgement', async () => {
  await draw();
  await fill();
  let finish!: () => void;
  copy.mockImplementationOnce(() => new Promise<void>(resolve => { finish=resolve; }));
  await click('[data-pool-config-copy]');
  props={ ...props, grants: [{ ...grant, status: 'revoked' }] };
  await draw();
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  props={ ...props, grants: [grant] };
  await draw();
  await act(async () => finish());
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  expect(document.querySelector('[data-pool-config-copy]')?.textContent).not.toContain('copied');
  await click('[data-pool-config-copy]');
  expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();
  props={ ...props, disabled: true };
  await draw();
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  props={ ...props, disabled: false };
  await draw();
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  await input(scope('scope-1', 'workspace'), 'relative');
  await click('[data-pool-config-copy]');
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  expect(document.querySelector('[data-pool-config-message]')?.textContent).toContain('absolute paths');
  await input(scope('scope-1', 'workspace'), '/srv/project-one');
  const advanced=document.querySelector<HTMLDetailsElement>('[data-pool-config-row=scope-1] details')!;
  await act(async () => { advanced.open=true; });
  await input(scope('scope-1', 'repository'), '/srv/repository');
  await input(scope('scope-1', 'base_commit'), 'bad');
  await act(async () => { advanced.open=false; });
  await click('[data-pool-config-copy]');
  expect(advanced.open).toBe(true);
  expect(document.activeElement).toBe(document.querySelector(scope('scope-1', 'base_commit')));
  await draw({ ...session, user: { ...session.user, id: '1'.repeat(32) } });
  expect(document.querySelector<HTMLInputElement>(scope('scope-1', 'workspace'))?.value).toBe('');
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});
it('T209 opens configuration preparation only on the actual current runner row and removes stale preparation after a node refresh revokes it', async () => {
  let node={ ...runner, name: 'Registered machine', version: 1, credential_version: 1, credential_expires_at: '2026-11-03T00:00:00Z', created_at: at, allowed_actions: [] };
  const writes: string[]=[];
  vi.stubGlobal('fetch', vi.fn(async (url: string, init: RequestInit={}) => {
    const path=new URL(String(url), 'https://suma.test').pathname.replace('/api/v1', ''); if(init.method&&init.method!=='GET')
      writes.push(path); if(path==='/execution-pools/'+pool)
      return Response.json({ id: pool, owner_tenant_id: owner, name: 'Owned Pool', status: 'active', version: 1, allowed_actions: [] }); if(path.endsWith('/grants'))
      return Response.json({ items: [grant], total: 1, allowed_actions: [] }); if(path.endsWith('/runners'))
      return Response.json({ items: [node], total: 1, allowed_actions: [] }); return Response.json({ items: [], total: 0, allowed_actions: [] });
  }));
  await act(async () => root.render(<ApiViewProvider session={session}>
    <PoolWorkspace session={session} selected={pool} panel="runners" refresh={0} onSelect={vi.fn()} onPanel={vi.fn()} />
  </ApiViewProvider>));
  await settle();
  const open=document.querySelector<HTMLButtonElement>('[data-pool-config-open="'+runnerID+'"]')!;
  expect(open).not.toBeNull();
  expect(open.closest('li')?.textContent).toContain('Registered machine');
  expect(document.querySelector('[data-pool-config-open="'+grant.id+'"]')).toBeNull();
  await click('[data-pool-config-open="'+runnerID+'"]');
  await fill();
  await click('[data-pool-config-copy]');
  expect(document.querySelector('[data-pool-config-prepare]')).not.toBeNull();
  node={ ...node, status: 'revoked' };
  const refresh=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent==='Reload nodes')!;
  expect(refresh).toBeDefined();
  await act(async () => refresh.click());
  await settle();
  expect(document.querySelector('[data-pool-config-prepare]')).toBeNull();
  expect(document.body.textContent).toContain('active registered node');
  expect(writes).toHaveLength(0);
  expect((fetch as ReturnType<typeof vi.fn>).mock.calls.some(([url]) => String(url).includes('project_id='))).toBe(false);
});
