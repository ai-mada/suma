// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale,tr} from './i18n';
import {TaskDispatchDialog} from './TaskDispatch';

const project = 'a'.repeat(32);
const tenant = 'b'.repeat(32);
const taskID = 'c'.repeat(32);
const planID = 'd'.repeat(32);
const profileID = 'e'.repeat(32);
const profileSHA = 'f'.repeat(64);
const session: Session = {
  user: {id: '1'.repeat(32), name: 'Reviewer', email: 'reviewer@example.test'},
  organization: {id: tenant, name: 'Team'},
  projects: [],
};

function readyPlan(task = taskID, title = 'Reviewed task') {
  const node = {join: 'all_success', quorum: 0, timeout_seconds: 60, max_retries: 0, idempotent: false};
  return {
    id: planID, project_id: project, version: 1, state: 'ready',
    task_id: task, task_version: 3, workflow_id: 'reviewed-flow', workflow_version: 2,
    agent_node_id: 'develop', tool_profile_id: profileID, tool_profile_version: 4,
    tool_profile_sha256: profileSHA, run_id: '2'.repeat(32),
    created_at: '2099-11-05T00:00:00Z', expires_at: '2099-11-05T00:30:00Z',
    context: {task: {id: task, version: 3, title, description: 'Fixed task description', acceptance_criteria: ['Independent test passes']}},
    workflow: {
      id: 'reviewed-flow', name: 'Fixed dispatch workflow', version: 2,
      nodes: [
        {...node, id: 'develop', name: 'Develop', type: 'agent', command: ['developer'], depends_on: null,
          config: {goal: 'Fixed development instructions', tool_profile_id: profileID, tool_profile_sha256: profileSHA}},
        {...node, id: 'test', name: 'Independent test', type: 'test', command: ['verify'], depends_on: ['develop'],
          config: {candidate_from: 'develop'}},
      ],
      edges: [{from: 'develop', to: 'test'}],
    },
    execution_route: {tenant_id: tenant, project_id: project, version: 0, pool_id: '', grant_id: '', grant_version: 0},
    valid: true, reason: '',
  };
}

it('withholds task plan facts and Start until an authorized retry returns the opened task', async () => {
  await setLocale('en-US', {persist: false});
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.body.appendChild(document.createElement('div'));
  const root = createRoot(host);
  const writes: string[] = [];
  const reads: string[] = [];
  let respond: (response: Response) => void = () => {};
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const url = new URL(input, location.href);
    const path = url.pathname.replace(/^\/api\/v1/, '');
    if ((init?.method ?? 'GET') !== 'GET') {
      writes.push(path);
      return new Response(null, {status: 400});
    }
    if (path === '/task-dispatch-plans/' + planID) {
      reads.push(url.search);
      return new Promise<Response>(resolve => {respond = resolve;});
    }
    return Response.json({items: [], total: 0, next_cursor: ''});
  }));

  const facts = () => document.querySelector('.tool-plan-facts');
  const confirm = () => document.querySelector<HTMLInputElement>('.tool-start-confirm input');
  const start = () => [...document.querySelectorAll('button')].find(button =>
    button.textContent === tr('development.dispatch.confirm_starting_the_fixed_plan'));
  try {
    await act(async () => root.render(<ApiViewProvider session={session}>
      <TaskDispatchDialog project={project} session={session} planID={planID}
        task={{id: taskID, project_id: project, version: 4, title: 'Current task revision'}}
        onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()} />
    </ApiViewProvider>));
    expect(reads).toEqual(['?project_id=' + project]);
    expect(facts()).toBeNull();
    expect(start()).toBeUndefined();

    await act(async () => respond(Response.json(readyPlan('3'.repeat(32), 'Another task'))));
    expect(facts()).toBeNull();
    expect(start()).toBeUndefined();
    const retry = [...document.querySelectorAll<HTMLButtonElement>('[role="alert"] button')]
      .find(button => button.textContent === tr('common.retry'));
    expect(retry).toBeDefined();

    await act(async () => retry!.click());
    expect(reads).toEqual(['?project_id=' + project, '?project_id=' + project]);
    expect(facts()).toBeNull();
    expect(start()).toBeUndefined();

    await act(async () => respond(Response.json(readyPlan())));
    expect(facts()?.textContent).toContain('Reviewed task');
    expect(confirm()?.checked).toBe(false);
    expect(confirm()?.disabled).toBe(false);
    expect(start()?.disabled).toBe(true);
    expect(writes).toEqual([]);
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});
