// @vitest-environment jsdom
import {act, useState} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ProjectScheduleOverlap} from './ProjectScheduleOverlap';

const week = '2026-11-02';
const nextWeek = '2026-11-09';
const session: Session = {
  user: {id: 'user', name: 'User', email: 'user@example.test', role: 'member'},
  organization: {id: 'org', name: 'Engineering'},
  projects: [],
};
const person = {
  user_id: 'member',
  name: 'Casey',
  days: ['2026-11-03'],
  assignments: [
    {project_id: 'alpha', project_title: 'Alpha', task_id: 'a1', task_title: 'Prepare build', start_date: '2026-11-02', due_date: '2026-11-03'},
    {project_id: 'beta', project_title: 'Beta', task_id: 'b1', task_title: 'Review integration', start_date: '2026-11-03', due_date: '2026-11-05'},
  ],
};
const page = (selected: string, items = [person]) => Response.json({
  week_start: selected, as_of: '2026-11-04T10:30:00Z', items,
});
const read = vi.fn<(url: string, init: RequestInit) => Response | Promise<Response>>();
const onBack = vi.fn(), onTask = vi.fn();
let host: HTMLDivElement, root: Root;

function View({current, selected, refresh}: {current: Session; selected: string; refresh: number}) {
  const [requested, setRequested] = useState(selected);
  return <ApiViewProvider session={current}>
    <ProjectScheduleOverlap session={current} week={requested} refresh={refresh}
      onWeek={setRequested} onBack={onBack} onTask={onTask}/>
  </ApiViewProvider>;
}

async function draw(current = session, refresh = 0, selected = week) {
  await act(async () => root.render(<View current={current} selected={selected} refresh={refresh}/>));
}
async function click(selector: string) {
  const control = host.querySelector<HTMLElement>(selector);
  expect(control).toBeTruthy();
  await act(async () => control!.click());
}

beforeEach(async () => {
  await setLocale('en-US', {persist: false});
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  read.mockReset();
  onBack.mockReset();
  onTask.mockReset();
  vi.stubGlobal('fetch', read);
  host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
});

afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

it('shows the actual dates and links each task through its project', async () => {
  read.mockReturnValue(page(week));
  await draw();
  expect(host.querySelectorAll('[data-overlap-day]')).toHaveLength(7);
  expect(host.querySelector('[data-overlapped="true"]')?.getAttribute('data-overlap-day')).toBe('2026-11-03');
  expect(host.querySelector('[data-overlap-day="2026-11-04"]')?.getAttribute('data-overlapped')).toBe('false');
  expect(host.querySelector('[data-overlap-person="member"] h3')?.textContent).toBe('Casey');
  expect(host.querySelectorAll('[data-overlap-project]')).toHaveLength(2);
  expect(host.querySelector('[data-overlap-task="b1"]')?.getAttribute('href'))
    .toBe('?project=beta&view=development&sub=tasks&record=b1');
  await click('[data-overlap-assignments] > summary');
  await click('[data-overlap-task="b1"]');
  expect(onTask).toHaveBeenCalledExactlyOnceWith('beta', 'b1');
  await click('[data-overlap-back]');
  expect(onBack).toHaveBeenCalledTimes(1);
  const [url, init] = read.mock.calls[0];
  expect(url).toBe('/api/v1/project-portfolio/schedule-overlaps?week=2026-11-02');
  expect(init.method).toBe('GET');
  expect(new Headers(init.headers).get('X-Suma-Expected-Organization')).toBe('org');
  expect(new Headers(init.headers).get('X-Suma-Expected-User')).toBe('user');
});

it('replaces the week and ignores the late response from its previous request', async () => {
  let resolveOld!: (value: Response) => void;
  read.mockImplementation(url => url.endsWith(week)
    ? new Promise(resolve => {resolveOld = resolve;})
    : page(nextWeek, []));
  await draw();
  await click('[data-overlap-next]');
  expect(read.mock.calls[1][0]).toContain(nextWeek);
  expect(read.mock.calls[0][1].signal?.aborted).toBe(true);
  expect(host.querySelector('[data-overlap-empty]')).toBeTruthy();
  await act(async () => resolveOld(page(week)));
  expect(host.querySelector('[data-overlap-person]')).toBeNull();
  expect(host.querySelector('[data-overlap-empty]')).toBeTruthy();
});

it('removes schedule sources after a current authority rejection', async () => {
  read.mockReturnValueOnce(page(week)).mockReturnValueOnce(Response.json({
    error: {code: 'forbidden', message: 'Access revoked'},
  }, {status: 403}));
  await draw();
  expect(host.querySelector('[data-overlap-person]')).toBeTruthy();
  await click('[data-overlap-refresh]');
  expect(host.querySelector('[data-overlap-person]')).toBeNull();
  expect(host.querySelector('[data-overlap-empty]')).toBeNull();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('Access revoked');
});

it('cannot repopulate task details from an organization that was left', async () => {
  let resolveOld!: (value: Response) => void;
  const moved = {...session, organization: {id: 'other', name: 'Other'}};
  read.mockImplementation((_url, init) => new Headers(init.headers).get('X-Suma-Expected-Organization') === 'org'
    ? new Promise(resolve => {resolveOld = resolve;})
    : page(week, []));
  await draw();
  await draw(moved);
  expect(read.mock.calls[0][1].signal?.aborted).toBe(true);
  await act(async () => resolveOld(page(week)));
  expect(host.querySelector('[data-overlap-person]')).toBeNull();
  expect(host.querySelector('[data-overlap-empty]')).toBeTruthy();
});

it('distinguishes a bounded service read from a week with no overlap', async () => {
  read.mockReturnValue(Response.json({
    error: {code: 'schedule_overlap_scope_too_large', message: 'Scope too large'},
  }, {status: 409}));
  await draw();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('too many scheduled tasks');
  expect(host.querySelector('[data-overlap-empty]')).toBeNull();
  expect(host.querySelector('[data-overlap-next]')).toMatchObject({disabled: false});
});
