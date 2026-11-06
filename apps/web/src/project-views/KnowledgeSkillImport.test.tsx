// @vitest-environment jsdom
import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { Session } from '../api';
import { ApiViewProvider } from '../api-context';
import { setLocale, tr } from '../i18n';
import type { KnowledgeDetail } from '../knowledge-model';
import { KnowledgeWorkspace } from './KnowledgeWorkspace';

vi.mock('../KnowledgeIntelligence', () => ({ KnowledgeIntelligence: () => null }));

const project = 'project-a';
const draftID = 'review-draft';
const stamp = '2026-11-06T00:00:00Z';
const session: Session = {
    user: { id: 'a'.repeat(32), name: 'Owner', email: 'owner@example.test' },
    organization: { id: 'b'.repeat(32), name: 'Team' },
    projects: [{ id: project, title: 'Project A', version: 1 }],
};
const description = 'Review candidate changes against the agreed scope.';
const markdown = '# Candidate review\n\nRecord any missing evidence.';
const skill = `---\nname: candidate-review\ndescription: ${description}\nallowed-tools: Bash\n---\n${markdown}`;
let root: Root;
let host: HTMLDivElement;
let writes: { path: string; method: string; body: Record<string, unknown>; headers: Headers }[];

function savedDraft(body: Record<string, unknown>): KnowledgeDetail {
    const kind = body.kind === 'spec' ? 'spec' : 'skill';
    const title = String(body.title);
    const summary = String(body.summary);
    const content = String(body.content);
    return {
        id: draftID, project_id: project, version: 1, kind, state: 'draft',
        title, summary, latest_revision: 1, published_revision: 0,
        published_sha256: '', published_title: '', published_summary: '',
        has_draft: true, published_eligible: false, eligibility: 'none', created_at: stamp, updated_at: stamp,
        latest: {
            revision: 1, based_on_revision: 0, state: 'draft', kind, title, summary, content,
            sha256: 'c'.repeat(64), sources: [], method: 'manual', created_by: session.user.id, created_at: stamp,
        },
    };
}

function WorkspaceHarness() {
    const [selected, setSelected] = useState('');
    return <ApiViewProvider session={session}>
        <KnowledgeWorkspace
            project={project} session={session} selected={selected}
            filters={{ kind: 'all', state: 'all', q: '' }}
            onFilter={() => {}} onSelect={setSelected} refresh={0} onRefresh={() => {}}
        />
    </ApiViewProvider>;
}

beforeEach(async () => {
    await setLocale('en-US', { persist: false });
    await import('../skill-markdown-import');
    await import('../helix-spec-import');
    sessionStorage.clear();
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
    host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    writes = [];
    let current: KnowledgeDetail | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
        const path = new URL(input, location.href).pathname.replace(/^\/api\/v1/, '');
        if (init?.body) {
            const body = JSON.parse(String(init.body)) as Record<string, unknown>;
            writes.push({ path, method: init.method ?? 'GET', body, headers: new Headers(init.headers) });
            current = savedDraft(body);
            return Response.json(current);
        }
        if (path === '/project-knowledge/' + draftID && current) return Response.json(current);
        const items = path === '/project-knowledge' && current ? [current] : [];
        return Response.json({ items, total: items.length, next_cursor: '' });
    }));
    await act(async () => root.render(<WorkspaceHarness/>));
});

afterEach(async () => {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
    await setLocale('zh-CN', { persist: false });
});

function button(text: string, container: ParentNode) {
    const element = Array.from(container.querySelectorAll('button')).find(item => item.textContent?.trim() === text);
    if (!element) throw new Error('Missing button ' + text);
    return element;
}

async function openImport(format: 'skill' | 'helix' = 'skill') {
    const action = tr(format === 'helix' ? 'knowledge.import_helix_spec' : 'knowledge.import_skill');
    await act(async () => button(action, host).click());
    const dialog = document.querySelector('[role="dialog"]');
    if (!dialog) throw new Error('Missing import dialog');
    expect(dialog.querySelector('h2')?.textContent).toBe(format === 'helix' ? action : tr('knowledge.import_title'));
    return dialog;
}

function utf8(text: string): ArrayBuffer {
    return new TextEncoder().encode(text).buffer as ArrayBuffer;
}

function markdownFile(text: string, name = 'SKILL.md') {
    const file = new File([text], name, { type: 'text/markdown' });
    if (typeof file.arrayBuffer !== 'function') {
        // Older JSDOM Blob implementations omit the native binary read method.
        const bytes = utf8(text);
        Object.defineProperty(file, 'arrayBuffer', { value: async () => bytes });
    }
    return file;
}

async function chooseFile(dialog: Element, file: File, started?: Promise<void>) {
    const input = dialog.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input?.disabled).toBe(false);
    Object.defineProperty(input!, 'files', { value: [file], configurable: true });
    await act(async () => {
        input!.dispatchEvent(new Event('change', { bubbles: true }));
        await started;
    });
}

function titleField(dialog: Element) {
    const field = dialog.querySelector<HTMLInputElement>('input[name="title"]');
    if (!field) throw new Error('Missing editable title');
    return field;
}

it('prefills an editable skill and creates one project draft only after Save draft', async () => {
    const dialog = await openImport();
    expect(button(tr('knowledge.save'), dialog).disabled).toBe(true);
    expect(writes).toHaveLength(0);

    await chooseFile(dialog, markdownFile(skill));

    const title = titleField(dialog);
    expect(title.value).toBe('candidate-review');
    expect(dialog.querySelector<HTMLTextAreaElement>('textarea[name="summary"]')?.value).toBe(description);
    expect(dialog.querySelector<HTMLTextAreaElement>('textarea[name="content"]')?.value).toBe(markdown);
    expect(writes).toHaveLength(0);

    const editedTitle = 'Candidate review for Project A';
    const setTitle = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
    await act(async () => {
        setTitle.call(title, editedTitle);
        title.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => button(tr('knowledge.save'), dialog).click());

    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({
        path: '/project-knowledge', method: 'POST',
        body: {
            project_id: project, kind: 'skill', title: editedTitle, summary: description,
            content: markdown, expected_version: 0, sources: [], idempotency_key: expect.any(String),
        },
    });
    expect(writes[0].headers.get('X-Suma-Expected-User')).toBe(session.user.id);
    expect(writes[0].headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
    expect(host.querySelector('.knowledge-detail h2')?.textContent).toBe(editedTitle);
    expect(host.querySelector('.knowledge-detail')?.textContent).toContain(tr('knowledge.state.draft'));
});

it('retains the import dialog after an invalid file and accepts a local retry without writes', async () => {
    const dialog = await openImport();

    await chooseFile(dialog, markdownFile(skill, 'candidate-review.md'));

    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain(tr('knowledge.import_invalid'));
    expect(button(tr('knowledge.save'), dialog).disabled).toBe(true);
    expect(writes).toHaveLength(0);

    await chooseFile(dialog, markdownFile(skill));

    expect(dialog.querySelector('[role="alert"]')).toBeNull();
    expect(titleField(dialog).value).toBe('candidate-review');
    expect(button(tr('knowledge.save'), dialog).disabled).toBe(false);
    expect(writes).toHaveLength(0);
});

it('ignores a file read that finishes after the import dialog is cancelled and reopened', async () => {
    const dialog = await openImport();
    let resolveRead!: (bytes: ArrayBuffer) => void;
    let startRead!: () => void;
    const pending = new Promise<ArrayBuffer>(resolve => { resolveRead = resolve; });
    const started = new Promise<void>(resolve => { startRead = resolve; });
    const slowFile = new File([skill], 'SKILL.md');
    Object.defineProperty(slowFile, 'arrayBuffer', {
        value: () => { startRead(); return pending; },
    });
    await chooseFile(dialog, slowFile, started);
    expect(button(tr('knowledge.save'), dialog).disabled).toBe(true);
    await act(async () => button(tr('knowledge.cancel'), dialog).click());
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    const fresh = await openImport();
    await act(async () => resolveRead(utf8(skill)));
    expect(button(tr('knowledge.save'), fresh).disabled).toBe(true);

    await chooseFile(fresh, markdownFile(skill.replace('name: candidate-review', 'name: release-review')));
    expect(titleField(fresh).value).toBe('release-review');
    expect(writes).toHaveLength(0);
});

it('retries a rejected HelixHarness file and saves the reviewed text as a specification draft', async () => {
    const dialog = await openImport('helix');
    await chooseFile(dialog, markdownFile('# Work plan', 'tasks.md'));
    expect(dialog.querySelector('[role="alert"]')?.textContent).toContain(tr('knowledge.helix_spec_invalid'));
    expect(button(tr('knowledge.save'), dialog).disabled).toBe(true);

    const content = '\n# 集成回执\n\n```text\nEvidence  \n```\n';
    await chooseFile(dialog, markdownFile('\uFEFF' + content.replace(/\n/g, '\r\n'), 'SPEC.md'));
    expect(titleField(dialog).value).toBe('集成回执');
    expect(dialog.querySelector<HTMLTextAreaElement>('textarea[name="content"]')?.value).toBe(content);
    expect(writes).toHaveLength(0);

    await act(async () => button(tr('knowledge.save'), dialog).click());
    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({
        path: '/project-knowledge', method: 'POST',
        body: {
            project_id: project, kind: 'spec', title: '集成回执', summary: '', content,
            expected_version: 0, sources: [], idempotency_key: expect.any(String),
        },
    });
    expect(host.querySelector('.knowledge-detail')?.textContent).toContain(tr('knowledge.state.draft'));
});
