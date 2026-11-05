// @vitest-environment jsdom
import {act, useState} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {
  ProjectExecutionRoute, ProjectKnowledgeDetail, ProjectKnowledgeDiscovery, ProjectKnowledgeReference,
  ProjectKnowledgeRevision, ProjectKnowledgeRevisionDetail, TaskDispatchOptions, TaskDispatchPlan,
  TaskDispatchPreviewRequest, Workflow,
} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import {TaskDispatchDialog} from './TaskDispatch';
import {setLocale, tr} from './i18n';

const project = 'project-a';
const stamp = '2026-10-25T00:00:00Z';
const session = {
  user: {id: 'a'.repeat(32), name: 'Owner', email: 'a@example.test'},
  organization: {id: 'b'.repeat(32), name: 'Team'},
  projects: [],
};
const task = {id: 'task-a', project_id: project, version: 2, title: 'Task', description: 'Task text'};
const nodeDefaults = {depends_on: [], join: 'all_success', quorum: 0, timeout_seconds: 60, max_retries: 0, idempotent: false};
const workflow: Workflow = {
  id: 'flow-a', name: 'Flow', version: 1,
  nodes: [
    {...nodeDefaults, id: 'agent', name: 'Develop', type: 'agent', command: ['template-command'], config: {goal: 'Implement the task'}},
    {...nodeDefaults, id: 'test', name: 'Verify', type: 'test', command: ['test-command'], config: {candidate_from: 'agent'}},
  ],
  edges: [{from: 'agent', to: 'test'}],
};
const profile = {
  id: 'tool-a', project_id: project, worker_id: 'worker-a', name: 'Tool', version: 1, state: 'enabled', available: true, reason: '',
  profile: {command_id: 'agent-command', adapter: 'suma', protocol: 'suma-jsonl-v1', profile_sha256: 'd'.repeat(64), executable_sha256: 'e'.repeat(64), tool_version: '1', read_only: false},
};
const route: ProjectExecutionRoute = {tenant_id: session.organization.id, project_id: project, version: 0, pool_id: '', grant_id: '', grant_version: 0};
const contextTask = {id: task.id, version: task.version, title: task.title, description: task.description, acceptance_criteria: []};
const basis = {project_id: project, task_id: task.id, task_version: task.version, workflow_id: workflow.id, workflow_version: workflow.version, agent_node_id: 'agent'};

const knowledgeID = 'knowledge-a';
const original: ProjectKnowledgeRevision = {
  revision: 3, based_on_revision: 2, state: 'published', kind: 'spec', title: 'APPROVED title', summary: 'Approved summary',
  content: 'Frozen <raw> {{text}}', sha256: 'c'.repeat(64),
  sources: [{kind: 'documents', id: 'doc-a', version: 2, title: 'Source', quote: 'Reviewed evidence', sha256: 'f'.repeat(64)}],
  method: 'manual', created_by: session.user.id, created_at: stamp,
};
const draft: ProjectKnowledgeRevision = {
  ...original, revision: 4, based_on_revision: 3, state: 'draft', title: 'UNREVIEWED draft title', summary: 'Unreviewed summary',
  content: 'PRIVATE draft body', sha256: '8'.repeat(64),
};
const updated: ProjectKnowledgeRevision = {
  ...original, revision: 5, based_on_revision: 4, title: 'NEW PUBLICATION', content: 'Second approved body', sha256: '9'.repeat(64),
};
const reference = ({revision, sha256}: ProjectKnowledgeRevision): ProjectKnowledgeReference => ({id: knowledgeID, revision, sha256});
const discoveryButton = `[data-knowledge-discovery-preview="${knowledgeID}"]`;
const selectedRow = `[data-knowledge-selected="${knowledgeID}"]`;
const prepareButton = '[data-dispatch-prepare]';

let root: Root;
let host: HTMLDivElement;
let writes: TaskDispatchPreviewRequest[];
let published: ProjectKnowledgeRevision;
let plan: TaskDispatchPlan | undefined;

function Harness() {
  const [id, setID] = useState('new');
  return <ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={task} planID={id}
    initialWorkflow={workflow} onPlan={next => setID(next || 'new')} onClose={() => {}} onRun={() => {}}/></ApiViewProvider>;
}

function detail(): ProjectKnowledgeDetail {
  const latest = published === original ? draft : published;
  return {
    id: knowledgeID, project_id: project, version: latest.revision, kind: latest.kind, state: 'published', title: latest.title,
    summary: latest.summary, latest_revision: latest.revision, published_revision: published.revision, published_sha256: published.sha256,
    published_title: published.title, published_summary: published.summary, has_draft: latest === draft, published_eligible: true,
    eligibility: 'current', created_at: stamp, updated_at: stamp, latest, published,
  };
}

function discovery(): ProjectKnowledgeDiscovery {
  return {
    items: [{reference: reference(published), project_id: project, kind: 'spec', title: published.title, summary: published.summary,
      snippet: published.content, matched_field: 'none', source_count: published.sources.length, published_at: stamp}],
    next_cursor: '', mode: 'keyword', scope: 'current_project_publications', scanned: 1, scan_limit: 200, exhausted: true,
  };
}

function freezePlan(body: TaskDispatchPreviewRequest) {
  const knowledge = body.knowledge ?? [];
  const current = reference(published);
  if (knowledge.some(pin => pin.id !== current.id || pin.revision !== current.revision || pin.sha256 !== current.sha256)) {
    return Response.json({error: {code: 'version_conflict', message: 'The knowledge publication has changed'}}, {status: 409});
  }
  const context = {
    task: contextTask,
    knowledge: knowledge.map(pin => ({...pin, kind: published.kind, title: published.title, summary: published.summary,
      content: published.content, sources: published.sources.map(({quote: _quote, ...source}) => source)})),
  };
  const createdAt = Date.now();
  plan = {
    ...basis, id: 'plan-a', version: 1, state: 'ready', tool_profile_id: profile.id, tool_profile_version: profile.version,
    tool_profile_sha256: profile.profile.profile_sha256, run_id: 'reserved-run',
    created_at: new Date(createdAt).toISOString(), expires_at: new Date(createdAt + 30 * 60 * 1000).toISOString(),
    context, workflow: {...workflow, name: 'Task dispatch', nodes: [
      {...workflow.nodes[0], command: [profile.profile.command_id], config: {goal: JSON.stringify(context), tool_profile_id: profile.id, tool_profile_sha256: profile.profile.profile_sha256}},
      workflow.nodes[1],
    ]}, execution_route: route, valid: true, reason: '',
  };
  return Response.json(plan, {status: 201});
}

beforeEach(async () => {
  await setLocale('en-US', {persist: false});
  localStorage.clear();
  sessionStorage.clear();
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  writes = [];
  published = original;
  plan = undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const path = new URL(String(input), location.href).pathname.replace(/^\/api\/v1/, '');
    if (path === '/task-dispatch-plans' && init?.method === 'POST') {
      const body: TaskDispatchPreviewRequest = JSON.parse(String(init.body));
      writes.push(body);
      return freezePlan(body);
    }
    if (path === '/task-dispatch-plans/plan-a' && plan) return Response.json(plan);
    if (path === `/tasks/${task.id}`) return Response.json(task);
    if (path === `/tasks/${task.id}/dispatch-options`) return Response.json({
      ...basis, observed_at: stamp, context: {task: contextTask}, execution_route: route,
      items: [], total: 0, next_cursor: '', recommended_tool_profile_id: '',
    } satisfies TaskDispatchOptions);
    if (path === '/workflows') return Response.json({items: [workflow]});
    if (path === '/tool-profiles') return Response.json({items: [profile], total: 1});
    if (path === '/project-knowledge/discovery') return Response.json(discovery());
    if (path === `/project-knowledge/${knowledgeID}`) return Response.json(detail());
    const revision = [original, updated].find(value => path === `/project-knowledge/${knowledgeID}/revisions/${value.revision}`);
    if (revision) return Response.json({project_id: project, knowledge_id: knowledgeID, snapshot: revision} satisfies ProjectKnowledgeRevisionDetail);
    return Response.json({items: [], total: 0});
  }));
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  await setLocale('zh-CN', {persist: false});
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function settle() {
  for (let i = 0; i < 5; i++) await act(async () => {await new Promise(resolve => setTimeout(resolve, 0));});
}

async function render() {
  await act(async () => root.render(<Harness/>));
  await settle();
}

function dialog() {
  const element = document.querySelector<HTMLElement>('[role="dialog"]');
  expect(element).not.toBeNull();
  return element!;
}

function button(selector: string) {
  const element = dialog().querySelector<HTMLButtonElement>(selector);
  expect(element, selector).not.toBeNull();
  return element!;
}

async function click(element: HTMLElement) {
  await act(async () => element.click());
  await settle();
}

async function chooseTool() {
  const select = dialog().querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')!;
  await act(async () => {
    select.value = profile.id;
    select.dispatchEvent(new Event('change', {bubbles: true}));
  });
  await settle();
}

it('starts empty and freezes only the explicitly selected publication and its reviewed content', async () => {
  await render();
  await chooseTool();
  await click(button('[data-knowledge-toggle]'));
  expect(dialog().querySelector(selectedRow)).toBeNull();
  expect(dialog().textContent).toContain(original.title);
  expect(dialog().textContent).not.toContain(draft.title);

  await click(button(discoveryButton));
  expect(dialog().querySelector('[data-knowledge-preview-state="current"]')).not.toBeNull();
  expect(dialog().textContent).toContain(original.content);
  expect(dialog().textContent).not.toContain(draft.content);
  expect(dialog().querySelector(selectedRow)).toBeNull();
  expect(writes).toHaveLength(0);

  await click(button('[data-knowledge-preview-choose]'));
  expect(dialog().querySelector(selectedRow)?.textContent).toContain(original.title);
  expect(writes).toHaveLength(0);
  expect(button(prepareButton).disabled).toBe(false);
  await click(button(prepareButton));

  expect(writes.map(body => body.knowledge)).toEqual([[reference(original)]]);
  expect(fetch).toHaveBeenCalledWith(`/api/v1/task-dispatch-plans/plan-a?project_id=${project}`, expect.objectContaining({method: 'GET'}));
  const summary = dialog().querySelector<HTMLElement>('.knowledge-fixed summary');
  expect(summary?.textContent).toContain(original.title);
  expect(summary?.textContent).toContain('Revision 3');
  await click(summary!);
  expect(dialog().querySelector<HTMLDetailsElement>('.knowledge-fixed details')?.open).toBe(true);
  expect(dialog().querySelector('.knowledge-fixed .knowledge-content')?.textContent).toBe(original.content);
  expect(dialog().querySelector('.knowledge-fixed raw')).toBeNull();
  const source = dialog().querySelector('.knowledge-fixed .knowledge-source');
  expect(source?.textContent).toContain(original.sources[0].title);
  expect(source?.textContent).toContain(original.sources[0].sha256);
  expect(source?.querySelector('a')?.getAttribute('href')).toBe(`?project=${project}&view=knowledge&sub=documents&record=doc-a`);
});

it('requires removing a stale selection before explicitly pinning a newer publication', async () => {
  await render();
  await chooseTool();
  await click(button('[data-knowledge-toggle]'));
  await click(button(discoveryButton));
  await click(button('[data-knowledge-preview-choose]'));
  expect(button(prepareButton).disabled).toBe(false);

  published = updated;
  await click(button(prepareButton));
  expect(writes.map(body => body.knowledge)).toEqual([[reference(original)]]);
  expect(plan).toBeUndefined();
  const refresh = button('.conflict-banner button');
  expect(refresh.textContent).toBe(tr('development.dispatch.read_current_context_keep_selections'));
  await click(refresh);

  expect(button(prepareButton).disabled).toBe(true);
  expect(dialog().querySelector(selectedRow)?.classList.contains('is-unavailable')).toBe(true);
  expect(dialog().querySelector(selectedRow)?.textContent).toContain('Revision 3');
  await click(button(discoveryButton));
  expect(dialog().querySelector('.conversation-knowledge-preview h4')?.textContent).toBe(updated.title);
  await click(button('[data-knowledge-preview-choose]'));
  expect(dialog().textContent).toContain(tr('task-conversation-knowledge.conflict'));
  expect(dialog().querySelector(selectedRow)?.textContent).toContain('Revision 3');
  expect(button(prepareButton).disabled).toBe(true);
  expect(writes).toHaveLength(1);

  await click(button('[data-knowledge-remove]'));
  await click(button('[data-knowledge-preview-choose]'));
  expect(dialog().querySelector(selectedRow)?.textContent).toContain('Revision 5');
  expect(button(prepareButton).disabled).toBe(false);
  await click(button(prepareButton));
  expect(writes.map(body => body.knowledge)).toEqual([[reference(original)], [reference(updated)]]);
  expect(dialog().querySelector('.knowledge-fixed .knowledge-content')?.textContent).toBe(updated.content);
});
