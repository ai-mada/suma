// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
import { App } from './App';
import { setLocale, tr } from './i18n';
import type { Session } from './api';
import { ApiViewProvider } from './api-context';
import { TaskDesignProvenance } from './TaskDesignProvenance';
import type { TaskDesignContextView } from '../../../packages/contracts/src';

const project = 'project-provenance', taskId = 'task-original', requirementId = 'requirement-original';
const session: Session = { user: { id: 'user-a', name: 'Owner', email: 'owner@example.test', role: 'owner' }, organization: { id: 'org-a', name: 'Team' }, projects: [{ id: project, title: 'Project', version: 1 }] };
let root: Root | undefined, host: HTMLDivElement, reads: string[], writes: string[];
beforeEach(async () => {
  localStorage.clear(); sessionStorage.clear();
  await setLocale('en-US', { persist: false });
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  host = document.createElement('div'); document.body.append(host); reads = []; writes = [];
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount()); root = undefined; host.remove();
  await setLocale('zh-CN', { persist: false }); vi.unstubAllGlobals(); vi.restoreAllMocks();
});
async function settle() { for (let n = 0; n < 5; n++) await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
const raw = 'Original <script>evidence</script> {{product}} 用户原文';
function adopted(eligibility = 'current'): TaskDesignContextView {
  return { project_id: project, task_id: taskId, task_version: 7, bound: true, eligibility,
    context: { requirement_id: requirementId, requirement_version: 3, revision: 4, sha256: 'a'.repeat(64), task_key: 'stable-task', adoption_id: 'adoption-original', content: { product: raw, experience: 'Original keyboard journey', technical: 'Original transaction boundary' } },
    requirement: { id: requirementId, version: 3, title: 'Original requirement', description: 'Original requirement body', acceptance_criteria: ['Original requirement acceptance'] },
    adopted_task: { key: 'stable-task', title: 'Original approved task', description: 'Original task body', priority: 'high', owner_id: '', depends_on: ['prior-key'], acceptance_criteria: ['Original task acceptance'], covers: [1] },
    adopted_by: 'approver-original', adopted_at: '2026-10-25T01:00:00Z', current_requirement_version: 3, current_design_version: 5, current_published_revision: 4 };
}
async function renderCard(value = session, selected = taskId, version = 7, refresh = 0, selectedProject = project) {
  await act(async () => { root ??= createRoot(host); root.render(<ApiViewProvider session={value}><TaskDesignProvenance session={value} project={selectedProject} taskId={selected} taskVersion={version} refresh={refresh}/></ApiViewProvider>); }); await settle();
}
function transport(reply: (path: string, options?: RequestInit) => Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn(async (input: string, options?: RequestInit) => {
    const path = String(input).replace(/^\/api\/v1/, '');
    if (options?.body) writes.push(path); else reads.push(path);
    return reply(path, options);
  }));
}

it('reads authoritative task provenance and keeps a dirty review-request draft outside the original Review modal', async () => {
  history.replaceState(null, '', '/?' + new URLSearchParams({ project, view: 'development', sub: 'tasks', record: taskId }));
  const task = { id: taskId, project_id: project, version: 7, title: 'Manual task', status: 'todo', owner_id: '', priority: 'medium', requirement_id: '', updated_at: '2026-10-29T00:00:00Z', created_at: '2026-10-29T00:00:00Z', data: { design: { revision: 99 }, adopted_design: 'FORGED DESIGN CLAIM' } };
  vi.stubGlobal('fetch', vi.fn(async (input: string, options?: RequestInit) => {
    const url = new URL(String(input), location.href), path = url.pathname.replace(/^\/api\/v1/, '');
    if (options?.body) writes.push(path); else reads.push(path + url.search);
    if (path === '/auth/session') return Response.json(session);
    if (path === '/overview') return Response.json({ counts: { tasks: 1 } });
    if (path === '/tasks') return Response.json({ items: [task], total: 1 });
    if (path === '/task-collection/summary') return Response.json({ project_id: project, total: 1, group_by: 'status', groups: [{ key: task.status, total: 1 }], groups_total: 1, groups_limit: 20, next_group_cursor: '', as_of: '2026-10-29T00:00:00Z' });
    if (path === '/task-workspace') return Response.json({ project_id: project, limit: 20, items: [task], total: 1, next_cursor: '', as_of: '2026-10-29T00:00:00Z' });
    if (path === '/tasks/' + taskId) return Response.json(task);
    // An authorized task with no review requests has an empty page, not a
    // 404 that correctly revokes the parent task's protected reading scope.
    if (path === '/task-review-requests') return Response.json({ project_id: project, task_id: taskId, task_version: task.version, can_request: true, limit: 20, items: [], total: 0, next_cursor: '', pending_request_id: '' });
    if (path === '/tasks/' + taskId + '/design-context') return Response.json({ project_id: project, task_id: taskId, task_version: 7, bound: false, eligibility: 'none', current_requirement_version: 0, current_design_version: 0, current_published_revision: 0 });
    if (['/requirements', '/members', '/runs', '/organizations', '/workflows', '/tool-profiles'].includes(path)) return Response.json({ items: [], total: 0 });
    return Response.json({}, { status: 404 });
  }));
  await act(async () => { root = createRoot(host); root.render(<App />); }); await settle();
  const card = host.querySelector<HTMLElement>('.task-design-provenance');
  expect(card).not.toBeNull();
  expect(card?.textContent).toContain('No adopted design');
  expect(card?.textContent).not.toContain('dispatch is blocked');
  expect(card?.textContent).not.toContain('FORGED DESIGN CLAIM');
  expect(reads).toContain('/tasks/task-original/design-context?project_id=project-provenance');
  expect(reads.some(path => /^\/(workflows|tool-profiles)/.test(path))).toBe(false);
  expect(writes).toEqual([]);
  // Opening a modal is guarded too: its close action uses the same global
  // leave guard, so opening over this draft would otherwise trap the user.
  await act(async () => host.querySelector<HTMLButtonElement>('[data-review-toggle]')!.click()); await settle();
  await act(async () => host.querySelector<HTMLButtonElement>('[data-review-compose]')!.click()); await settle();
  const note = host.querySelector<HTMLTextAreaElement>('[name=review_request_note]')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(note, 'Keep this unsent responsibility note');
    note.dispatchEvent(new Event('input', {bubbles: true}));
  }); await settle();
  const review = [...host.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === tr('records.review_accept'))!;
  await act(async () => review.click()); await settle();
  expect(document.querySelector('[role=dialog]')).toBeNull();
  expect(note.value).toBe('Keep this unsent responsibility note');
  const clear = [...host.querySelectorAll<HTMLButtonElement>('.task-review-compose button')].find(button => button.textContent === tr('task-review-requests.clear_draft'))!;
  await act(async () => clear.click()); await settle();
  await act(async () => review.click()); await settle();
  expect(document.querySelector('[role=dialog]')).not.toBeNull();
  const close = document.querySelector<HTMLButtonElement>('[role=dialog] button[aria-label="' + tr('common.close') + '"]')!;
  expect(close).not.toBeNull(); await act(async () => close.click()); await settle();
  expect(document.querySelector('[role=dialog]')).toBeNull();
  expect(host.querySelectorAll('.task-design-provenance')).toHaveLength(1);
  expect(writes).toEqual([]);
});

it('keeps the original adopted snapshot, exact text and workspace destination through a locale change', async () => {
  transport(async () => Response.json(adopted())); await renderCard();
  expect(host.textContent).toContain('Current adopted design');
  expect(host.textContent).toContain('Design revision 4'); expect(host.textContent).toContain('Requirement version 3');
  expect(host.querySelector('.tdp-current')?.textContent).toContain('Task read at version 7');
  const link = host.querySelector<HTMLAnchorElement>('.tdp-workspace')!;
  const destination = new URL(link.href);
  expect(Object.fromEntries(destination.searchParams)).toEqual({ project, view: 'requirements', sub: '', record: requirementId, panel: 'requirement-design' });
  const snapshot = host.querySelector<HTMLDetailsElement>('details')!, summary = snapshot.querySelector('summary')!;
  await act(async () => { snapshot.open = true; summary.focus(); await setLocale('zh-CN', { persist: false }); });
  expect(snapshot.isConnected && snapshot.open).toBe(true); expect(document.activeElement).toBe(summary);
  expect(host.querySelector('pre')?.textContent).toBe(raw); expect(host.querySelector('script')).toBeNull();
  expect(snapshot.textContent).toContain('Original task acceptance'); expect(snapshot.textContent).toContain('prior-key');
  expect(snapshot.textContent).toContain('adoption-original'); expect(snapshot.textContent).toContain('a'.repeat(64));
  expect(reads).toHaveLength(1); expect(writes).toEqual([]);
});

it('keeps one actual task design container through asynchronous refresh versions and locale changes', async () => {
  history.replaceState(null, '', '/?' + new URLSearchParams({ project, view: 'development', sub: 'tasks', record: taskId }));
  let task = { id: taskId, project_id: project, version: 3, title: 'Original downstream task', status: 'in_progress', description: 'Original body', owner_id: '', priority: 'medium', requirement_id: '', updated_at: '2026-10-29T00:00:00Z', created_at: '2026-10-29T00:00:00Z', depends_on: [] as string[], data: {} };
  const pending: { resolve: (response: Response) => void; version: number }[] = [];
  const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.stubGlobal('fetch', vi.fn(async (input: string, options?: RequestInit) => {
    const path = new URL(String(input), location.href).pathname.replace(/^\/api\/v1/, '');
    if (options?.body) writes.push(path); else reads.push(path);
    if (path === '/auth/session') return Response.json(session);
    if (path === '/overview') return Response.json({ counts: { tasks: 1 } });
    if (path === '/tasks') return Response.json({ items: [task], total: 1 });
    if (path === '/task-collection/summary') return Response.json({ project_id: project, total: 1, group_by: 'status', groups: [{ key: task.status, total: 1 }], groups_total: 1, groups_limit: 20, next_group_cursor: '', as_of: '2026-10-29T00:00:00Z' });
    if (path === '/task-workspace') return Response.json({ project_id: project, limit: 20, items: [task], total: 1, next_cursor: '', as_of: '2026-10-29T00:00:00Z' });
    if (path === '/tasks/' + taskId) return Response.json(task);
    if (path === '/task-review-requests') return Response.json({ project_id: project, task_id: taskId, task_version: task.version, can_request: true, limit: 20, items: [], total: 0, next_cursor: '', pending_request_id: '' });
    if (path === '/tasks/' + taskId + '/design-context') { const version = task.version; return new Promise<Response>(resolve => pending.push({ resolve, version })); }
    if (['/requirements', '/members', '/runs', '/organizations', '/workflows', '/tool-profiles'].includes(path)) return Response.json({ items: [], total: 0 });
    return Response.json({}, { status: 404 });
  }));
  const one = () => { expect(host.querySelectorAll('.task-design-provenance')).toHaveLength(1); expect(document.body.querySelectorAll('.task-design-provenance')).toHaveLength(1); };
  const finishReads = async () => { const batch = pending.splice(0); await act(async () => batch.forEach(item => item.resolve(Response.json({ project_id: project, task_id: taskId, task_version: item.version, bound: false, eligibility: 'none', current_requirement_version: 0, current_design_version: 0, current_published_revision: 0 })))); await settle(); };
  await act(async () => { root = createRoot(host); root.render(<App/>); }); await settle(); one(); await act(async () => { await setLocale('en-US', { persist: false }); }); await settle();
  for (const version of [4, 5, 6]) {
    task = { ...task, version, status: version === 6 ? 'accepted' : 'in_progress', depends_on: version % 2 ? [] : ['fixed-prerequisite'] };
    const refresh = [...host.querySelectorAll<HTMLButtonElement>('.topbar button')].find(button => button.getAttribute('aria-label') === tr('shell.refresh')); expect(refresh).toBeDefined();
    await act(async () => refresh!.click()); await settle(); one(); await finishReads(); one();
    expect(host.querySelector('.tdp-current')?.textContent).toContain('version ' + version);
  }
  await act(async () => { await setLocale('zh-CN', { persist: false }); }); await settle(); one(); expect(host.querySelector('.task-design-provenance h3')?.textContent).toBe('设计依据');
  await act(async () => { await setLocale('en-US', { persist: false }); }); await settle(); one(); expect(host.querySelector('.task-design-provenance h3')?.textContent).toBe('Design basis'); expect(host.querySelector('.task-design-provenance')?.textContent).not.toContain('设计依据');
  expect(errors.mock.calls.filter(call => call.some(value => typeof value === 'string' && value.includes('same key')))).toHaveLength(0);
  expect(writes).toEqual([]);
});

it.each([
  ['task_changed', 'Task differs from adoption'], ['requirement_changed', 'Requirement changed'],
  ['publication_changed', 'Published design changed'], ['retired', 'Design retired'],
  ['excluded', 'Task excluded from design'], ['source_unavailable', 'Design source unavailable'],
])('explains %s without replacing the original snapshot with current versions', async (eligibility, status) => {
  transport(async () => Response.json({ ...adopted(eligibility), current_requirement_version: 9, current_design_version: 12, current_published_revision: 11 }));
  await renderCard();
  expect(host.textContent).toContain(status); expect(host.textContent).toContain('New dispatch is blocked');
  expect(host.textContent).toContain('Design revision 4'); expect(host.textContent).toContain('Requirement version 3');
  expect(host.querySelector('pre')?.textContent).toBe(raw);
  expect(host.querySelector('.tdp-current')?.textContent).toContain('9');
  expect(host.querySelector('.tdp-current')?.textContent).toContain('11');
});

it('states an unverifiable bound snapshot explicitly and does not fabricate a source link', async () => {
  const value = adopted('source_unavailable'); delete value.context; delete value.requirement; delete value.adopted_task;
  transport(async () => Response.json(value)); await renderCard();
  expect(host.textContent).toContain('The original adopted snapshot could not be verified');
  expect(host.querySelector('details')).toBeNull(); expect(host.querySelector('.tdp-workspace')).toBeNull();
});

it('does not link a foreign project or an invalid requirement locator', async () => {
  transport(async () => Response.json({ ...adopted(), project_id: 'foreign-project' })); await renderCard();
  expect(host.querySelector('.tdp-workspace')).toBeNull();
  transport(async () => Response.json({ ...adopted(), context: { ...adopted().context!, requirement_id: 'https://external.example' }, requirement: { ...adopted().requirement!, id: 'https://external.example' } }));
  await renderCard(session, taskId, 8);
  expect(host.querySelector('.tdp-workspace')).toBeNull();
});

it('hides an already loaded snapshot immediately when the current caller changes on the same task', async () => {
  transport(async () => Response.json(adopted())); await renderCard();
  expect(host.querySelector('pre')?.textContent).toBe(raw);
  let release!: (value: Response) => void;
  transport(async () => new Promise<Response>(resolve => { release = resolve; }));
  const other = { ...session, user: { ...session.user, id: 'user-b' }, organization: { ...session.organization, id: 'org-b' } };
  await renderCard(other);
  expect(host.querySelector('[role=status]')).not.toBeNull(); expect(host.textContent).not.toContain(raw);
  await act(async () => release(Response.json({ error: { code: 'forbidden', message: 'Current access denied' } }, { status: 403 }))); await settle();
  expect(host.textContent).toContain('Current access denied'); expect(host.querySelector('details')).toBeNull();
});

it('clears source facts across callers and ignores an older response after a task switch', async () => {
  let release!: (value: Response) => void;
  transport(async () => new Promise<Response>(resolve => { release = resolve; }));
  await renderCard(); expect(host.querySelector('[role=status]')).not.toBeNull();
  const previous = release, other = { ...session, user: { ...session.user, id: 'user-b' }, organization: { ...session.organization, id: 'org-b' } };
  transport(async () => Response.json({ ...adopted('excluded'), task_id: 'task-b' }));
  await renderCard(other, 'task-b');
  await act(async () => previous(Response.json(adopted()))); await settle();
  expect(host.textContent).toContain('Task excluded from design'); expect(host.textContent).not.toContain('Current adopted design');
  transport(async () => Response.json({ error: { code: 'forbidden', message: 'Current access denied' } }, { status: 403 }));
  await renderCard(other, 'task-b', 8);
  expect(host.textContent).toContain('Current access denied'); expect(host.textContent).not.toContain(raw);
  expect(host.querySelector('details')).toBeNull();
});

it('retries a failed read and reloads when the task version, project or refresh changes', async () => {
  let failed = true;
  transport(async () => failed ? Response.json({ error: { code: 'unavailable', message: 'Read unavailable' } }, { status: 503 }) : Response.json(adopted()));
  await renderCard(); expect(host.textContent).toContain('Read unavailable');
  failed = false;
  await act(async () => [...host.querySelectorAll('button')].find(button => button.textContent === 'Reload')!.click()); await settle();
  expect(host.textContent).toContain('Current adopted design');
  await renderCard(session, taskId, 8); await renderCard(session, taskId, 8, 1); await renderCard(session, taskId, 8, 1, 'project-b');
  expect(reads).toHaveLength(5); expect(reads.at(-1)).toContain('project_id=project-b'); expect(writes).toEqual([]);
});
