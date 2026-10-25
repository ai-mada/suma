// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { beforeEach, afterEach, it, expect, vi } from 'vitest';
import { ApiViewProvider } from './api-context';
import { RequirementDesignWorkspace } from './project-views/RequirementDesignWorkspace';
import { setLocale, tr } from './i18n';
import type { Session } from './api';
import type { RequirementDesign, DesignPreview, DesignRevision, DesignTask } from './requirement-design-model';
const project = 'project-design', id = 'req-design', base = '/requirements/' + id + '/design';
const session: Session = { user: { id: 'owner-a', name: 'Owner', email: 'owner@example.test' }, organization: { id: 'org-a', name: 'Team' }, projects: [{ id: project, version: 1, title: 'Project' }] };
const raw = 'Original <script>raw</script> {{content}} 用户原文';
const task: DesignTask = { key: 'stable-key', title: 'Implement access', description: 'Only the approved scope', priority: 'high', owner_id: '', depends_on: [], acceptance_criteria: ['Access is verified'], covers: [1] };
let current: RequirementDesign, root: Root | undefined, host: HTMLDivElement, writes: {
    path: string;
    body: any;
    headers: Headers;
}[], reads: string[], send: (path: string, body: any) => Promise<Response>;
function revision(number = 1): DesignRevision { return { revision: number, state: 'draft', requirement: { id, version: 4, title: 'Requirement', description: 'Requirement source', acceptance_criteria: ['Access is restricted'] }, content: { product: raw, experience: 'Keyboard and empty states', technical: 'Enforce tenant permissions' }, tasks: [structuredClone(task)], sha256: 'a'.repeat(64), created_by: 'owner-a', created_at: '2026-10-25T00:00:00Z' }; }
function plan(): DesignPreview { return { project_id: project, requirement_id: id, version: current.version, revision: current.published_revision, sha256: current.published!.sha256, requirement_version: 4, plan_sha256: 'b'.repeat(64), create_count: 1, update_count: 1, keep_count: 1, exclude_count: 1, has_changes: true, changes: [{ key: 'create', action: 'create', task_id: '', expected_task_version: 0, proposed: { ...task, key: 'create', title: 'New scope' } }, { key: 'update', action: 'update', task_id: 'task-old', expected_task_version: 8, current: { id: 'task-old', version: 8, title: 'Independent task edit', description: 'Current text', acceptance_criteria: 'Old acceptance', depends_on: [] }, proposed: { ...task, key: 'update', title: 'Reviewed replacement' } }, { key: 'keep', action: 'keep', task_id: 'task-keep', expected_task_version: 2, current: { id: 'task-keep', version: 2, title: 'Keep status', status: 'accepted' }, proposed: { ...task, key: 'keep' } }, { key: 'exclude', action: 'exclude', task_id: 'task-exclude', expected_task_version: 3, current: { id: 'task-exclude', version: 3, title: 'Archived scope', status: 'in_review' } }] }; }
beforeEach(async () => {
    await setLocale('en-US', { persist: false });
    localStorage.clear();
    sessionStorage.clear();
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    host = document.createElement('div');
    document.body.append(host);
    writes = [];
    reads = [];
    current = { project_id: project, requirement_id: id, version: 1, requirement: revision().requirement, latest_revision: 1, published_revision: 0, has_draft: true, eligibility: 'none', latest: revision(), validation: [] };
    send = async (path, body) => {
        if (path === base + '/preview')
            return Response.json(plan());
        if (path === base) {
            const latest = { ...revision(current.latest_revision + 1), content: body.content, tasks: body.tasks, requirement: { ...current.requirement, version: body.expected_requirement_version } };
            current = { ...current, version: current.version + 1, latest_revision: latest.revision, has_draft: true, latest };
            return Response.json({ ...current, version: 0 });
        }
        if (path === base + '/commands') {
            const published = { ...current.latest!, revision: current.latest_revision + 1, state: body.action === 'publish' ? 'published' : 'retired' };
            current = { ...current, version: current.version + 1, latest_revision: published.revision, latest: published, published_revision: body.action === 'publish' ? published.revision : 0, published: body.action === 'publish' ? published : undefined, has_draft: false, eligibility: body.action === 'publish' ? 'current' : 'retired' };
            return Response.json(current);
        }
        if (path === base + '/adopt') {
            const adoption = { id: 'adoption-a', project_id: project, requirement_id: id, version: current.version + 1, revision: body.revision, sha256: body.sha256, requirement_version: 4, plan_sha256: body.plan_sha256, tasks: [{ key: 'create', task_id: 'task-created', version: 1, action: 'create' }], created_by: 'owner-a', created_at: '2026-10-25T01:00:00Z' };
            current = { ...current, version: current.version + 1, last_adoption: adoption };
            return Response.json(adoption);
        }
        throw Error('Unexpected write ' + path);
    };
    vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => { const url = new URL(String(input), location.href), path = url.pathname.replace(/^\/api\/v1/, ''); if (init?.body) {
        const body = JSON.parse(String(init.body));
        writes.push({ path, body, headers: new Headers(init.headers) });
        return send(path, body);
    } reads.push(path + url.search); if (path === base)
        return Response.json(current); if (path === base + '/revisions')
        return Response.json({ items: [revision(url.searchParams.has('cursor') ? 1 : 2)], total: 2, next_cursor: url.searchParams.has('cursor') ? '' : 'older-page' }); if (path === base + '/adoptions' || path === '/members')
        return Response.json({ items: [], total: 0, next_cursor: '' }); throw Error('Unexpected read ' + path); }));
});
afterEach(async () => { if (root)
    await act(async () => root?.unmount()); root = undefined; host.remove(); await setLocale('zh-CN', { persist: false }); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
async function settle() { for (let n = 0; n < 5; n++)
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
async function render() { await act(async () => { root ??= createRoot(host); root.render(<ApiViewProvider session={session}><RequirementDesignWorkspace project={project} requirementId={id} session={session} onClose={() => { }} onRefresh={() => { }}/></ApiViewProvider>); }); await settle(); }
async function click(name: string, scope: ParentNode = document) { const button = [...scope.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent?.trim() === name); expect(button, 'button ' + name).toBeDefined(); await act(async () => button!.click()); await settle(); }
async function fill(field: HTMLTextAreaElement, value: string) { await act(async () => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(field, value); field.dispatchEvent(new Event('input', { bubbles: true })); }); }
async function confirm() { await act(async () => document.querySelector<HTMLInputElement>('.rd-review input[type=checkbox]')!.click()); }
const t = (key: string) => tr('requirement-design.' + key);
it('treats version zero as an editable empty design and saves an incomplete draft without creating tasks', async () => {
    current = { ...current, version: 0, latest_revision: 0, has_draft: false, latest: undefined };
    await render();
    expect(document.body.textContent).toContain(t('empty'));
    await fill(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!, raw);
    await click(t('save'));
    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({ path: base, body: { project_id: project, expected_version: 0, expected_requirement_version: 4, content: { product: raw, experience: '', technical: '' }, tasks: [] } });
    expect(reads.filter(value => value.startsWith(base + '?'))).toHaveLength(2);
    expect(document.querySelector('.rd-heading')?.textContent).toContain('Design version 1');
    expect([...document.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === t('publish'))?.disabled).toBe(true);
    expect(document.querySelector('script')).toBeNull();
});
it('keeps the unknown original save through locale switch and reload, then reads current CAS instead of the historical receipt', async () => {
    let first = true;
    send = async () => { if (first) {
        first = false;
        throw new TypeError('response lost');
    } current = { ...current, version: 9, latest: { ...current.latest!, content: { ...current.latest!.content, product: raw + ' submitted' } } }; return Response.json({ ...current, version: 2 }); };
    await render();
    await fill(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!, raw + ' submitted');
    await click(t('save'));
    const original = structuredClone(writes[0].body);
    await act(async () => { await setLocale('zh-CN', { persist: false }); });
    expect(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!.value).toBe(raw + ' submitted');
    expect(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!.matches(':disabled')).toBe(true);
    await act(async () => root?.unmount());
    root = undefined;
    const concurrentSource={...current.requirement,version:5,title:'Concurrent source title',description:'CONCURRENT NEW SOURCE'};
    current={...current,requirement:concurrentSource,latest:{...current.latest!,requirement:concurrentSource}};
    await render();
    expect(document.querySelector('.rd-basis')?.textContent).toContain(t('pending_basis_unknown'));
    expect(document.querySelector('.rd-basis')?.textContent).not.toContain('CONCURRENT NEW SOURCE');
    expect(document.querySelector('.rd-heading')?.textContent).toContain('Concurrent source title');
    await click(t('restore'));
    expect(writes).toHaveLength(2);
    expect(writes[1].body).toEqual(original);
    expect(writes[1].headers.get('X-Suma-Expected-User')).toBe(session.user.id);
    expect(document.querySelector('.rd-heading')?.textContent).toContain('设计版本 9');
    expect(sessionStorage.length).toBe(0);
});
it('separates publication from explicit full-scope task adoption and loads immutable history only on demand', async () => {
    await render();
    expect(reads.some(value => value.includes('/revisions'))).toBe(false);
    await click(t('publish'));
    expect(writes).toHaveLength(0);
    await confirm();
    await click(t('confirm_publish'));
    expect(writes[0].path).toBe(base + '/commands');
    expect(writes.some(value => value.path === base + '/adopt')).toBe(false);
    const revisionSelect = document.querySelector<HTMLSelectElement>('.requirement-design select')!;
    await act(async () => { Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(revisionSelect, 'published'); revisionSelect.dispatchEvent(new Event('change', {bubbles:true})); });
    await click(t('tasks'));
    const inspect = document.querySelector<HTMLButtonElement>('.rd-task-table td button')!;
    expect(inspect.matches(':disabled')).toBe(false);
    await act(async () => inspect.click());
    expect(document.querySelector('.rd-task-details .rd-task-text')?.textContent).toContain(task.description);
    await click(t('preview'));
    const preview = writes.at(-1)!;
    expect(preview.path).toBe(base + '/preview');
    expect(preview.body).not.toHaveProperty('idempotency_key');
    expect(document.querySelector('.rd-change-counts')?.textContent).toContain('Create');
    expect(document.querySelector('.rd-review')?.textContent).toContain('Independent task edit');
    expect(document.querySelector('.rd-review')?.textContent).toContain('Reviewed replacement');
    expect(document.querySelector('.rd-review')?.textContent).toContain('Archive the task');
    expect([...document.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === t('adopt'))?.disabled).toBe(true);
    await confirm();
    await click(t('adopt'));
    expect(writes.at(-1)).toMatchObject({ path: base + '/adopt', body: { expected_version: 2, revision: 2, sha256: 'a'.repeat(64), plan_sha256: 'b'.repeat(64) } });
    expect(document.querySelector('.rd-adoption a')?.getAttribute('href')).toContain('record=task-created');
    expect(writes.some(value => value.path.includes('/runs'))).toBe(false);
    const history = [...document.querySelectorAll<HTMLDetailsElement>('.rd-histories>details')][0];
    await act(async () => { history.open = true; history.dispatchEvent(new Event('toggle')); });
    await settle();
    expect(reads.filter(value => value.includes('/revisions'))).toHaveLength(1);
    await click(t('more'), history);
    expect(reads.at(-1)).toContain('cursor=older-page');
    expect(history.textContent).toContain('Loaded 2 of 2');
    await click(t('retire')); await confirm(); await click(t('confirm_retire'));
    await click(t('product'));
    const retiredProduct=document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!;
    expect(retiredProduct.readOnly).toBe(false);
    await fill(retiredProduct,raw+' renewed draft'); await click(t('save'));
    expect(writes.at(-1)).toMatchObject({path:base,body:{content:{product:raw+' renewed draft'}}});

});
it('requires explicit old/new requirement review before changing the draft basis while retaining design and task keys', async () => {
    const oldPublication={...revision(2),state:'published'};
    current = { ...current, version:3, latest_revision:2,published_revision:2,published:oldPublication,latest:structuredClone(oldPublication),has_draft:false,eligibility: 'requirement_changed', requirement: { ...current.requirement, version: 5, description: 'Changed source', acceptance_criteria: ['Changed first', 'Added second'] } };
    current.latest!.tasks[0].depends_on=['stable-key'];
    await render();
    await fill(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!, raw + ' preserved');
    const save = [...document.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === t('save'))!;
    expect(save.disabled).toBe(true);
    expect(document.querySelector('.rd-review')?.textContent).toContain('Requirement source');
    expect(document.querySelector('.rd-review')?.textContent).toContain('Changed source');
    await click(t('tasks'));
    await act(async()=>document.querySelector<HTMLButtonElement>('.rd-task-table td button')!.click());
    const selfDependency=[...document.querySelectorAll<HTMLLabelElement>('.rd-task-details label')].find(label=>label.textContent?.includes(t('diagnostic.dependency_cycle')))!;
    expect(selfDependency).toBeDefined();await act(async()=>selfDependency.querySelector<HTMLInputElement>('input')!.click());
    await click(t('product'));
    await click(t('use_current_basis'));
    expect(document.querySelector<HTMLTextAreaElement>('textarea[name=product]')!.value).toBe(raw + ' preserved');
    await click(t('tasks'));
    await act(async()=>document.querySelector<HTMLButtonElement>('.rd-task-table td button')!.click());
    const addedCriterion=[...document.querySelectorAll<HTMLLabelElement>('.rd-task-details label')].find(label=>label.textContent?.includes('2. Added second'))!;
    expect(addedCriterion).toBeDefined();await act(async()=>addedCriterion.querySelector<HTMLInputElement>('input')!.click());
    await click(t('save'));
    expect(writes[0].body).toMatchObject({ expected_requirement_version: 5, content: { product: raw + ' preserved' }, tasks: [{ key: 'stable-key', covers: [1,2], depends_on: [] }] });
    expect([...document.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent===t('publish'))?.disabled).toBe(false);
    expect([...document.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent===t('preview'))?.disabled).toBe(true);
});
