// @vitest-environment jsdom
import {act, useState} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {ProjectPortfolioCapacityPerson} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ProjectPortfolioCapacity} from './ProjectPortfolioCapacity';

const week = '2026-11-02';
const nextWeek = '2026-11-09';
const session: Session = {
  user: {id: 'user', name: 'User', email: 'user@example.test', role: 'member'},
  organization: {id: 'org', name: 'Engineering'},
  projects: [],
};
const over: ProjectPortfolioCapacityPerson = {
  user_id: 'over', name: 'Casey', state: 'over_capacity', calendar_version: 1,
  available_hours: 8, allocated_hours: 9, unestimated_tasks: 0, unallocated_tasks: 0,
  days: Array.from({length: 7}, (_, i) => ({
    date: '2026-11-0' + (i + 2),
    available_hours: i === 0 ? 8 : 0,
    allocated_hours: i === 0 ? 9 : 0,
  })),
  tasks: [
    {project_id: 'alpha', project_title: 'Alpha', task_id: 'a1', task_title: 'Prepare build', start_date: week, due_date: week,
      estimated_hours: 6, allocated_hours: 6},
    {project_id: 'beta', project_title: 'Beta', task_id: 'b1', task_title: 'Review integration', start_date: week, due_date: week,
      estimated_hours: 3, allocated_hours: 3},
  ],
};
const incomplete: ProjectPortfolioCapacityPerson = {
  user_id: 'user', name: 'My workload', state: 'basis_incomplete', calendar_version: 0,
  available_hours: null, allocated_hours: 0, unestimated_tasks: 0, unallocated_tasks: 1,
  days: over.days.map(day => ({...day, available_hours: null, allocated_hours: 0})),
  tasks: [{...over.tasks[0], task_id: 'a2', estimated_hours: 3, allocated_hours: null}],
};
const peer: ProjectPortfolioCapacityPerson = {
  ...incomplete, user_id: 'peer', name: 'Team member',
  tasks: [{...incomplete.tasks[0], task_id: 'a3'}],
};
const response = (selected: string, items = [incomplete, over, peer]) => Response.json({
  week_start: selected, as_of: '2026-11-04T10:30:00Z', items,
});
const read = vi.fn<(url: string, init: RequestInit) => Response | Promise<Response>>();
const onBack = vi.fn(), onTask = vi.fn(), onCalendar = vi.fn();
let host: HTMLDivElement, root: Root;

function View({current, selected}: {current: Session; selected: string}) {
  const [requested, setRequested] = useState(selected);
  return <ApiViewProvider session={current}>
    <ProjectPortfolioCapacity session={current} week={requested} refresh={0}
      onWeek={setRequested} onBack={onBack} onTask={onTask} onCalendar={onCalendar}/>
  </ApiViewProvider>;
}

async function draw(current = session, selected = week) {
  await act(async () => root.render(<View current={current} selected={selected}/>));
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
  onCalendar.mockReset();
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

it('prioritizes overload and keeps the daily, weekly and source values distinct', async () => {
  read.mockReturnValue(response(week));
  await draw();
  const [url, init] = read.mock.calls[0];
  expect(url).toBe('/api/v1/project-portfolio/capacity?week=2026-11-02');
  expect(new Headers(init.headers).get('X-Suma-Expected-Organization')).toBe('org');
  expect(new Headers(init.headers).get('X-Suma-Expected-User')).toBe('user');
  expect(host.querySelector('[data-capacity-person]')?.getAttribute('data-capacity-person')).toBe('over');
  const card = host.querySelector('[data-capacity-person="over"]')!;
  expect(card.querySelector('[data-capacity-allocated]')?.textContent).toBe('9 h');
  expect(card.querySelector('[data-capacity-available]')?.textContent).toBe('8 h');
  expect(card.querySelectorAll('[data-capacity-day]')).toHaveLength(7);
  expect(card.querySelector('[data-capacity-day-over="true"]')?.getAttribute('data-capacity-day')).toBe(week);
  expect(card.querySelector('[data-capacity-day="2026-11-03"] [data-capacity-day-available]')?.textContent).toBe('0 h');
  const details = card.querySelector<HTMLDetailsElement>('details[data-capacity-tasks]')!;
  await act(async () => {
    details.open = true;
    details.dispatchEvent(new Event('toggle'));
  });
  expect(card.querySelector('[data-capacity-task="b1"]')?.getAttribute('href'))
    .toBe('?project=beta&view=development&sub=tasks&record=b1');
  await click('[data-capacity-task="b1"]');
  expect(onTask).toHaveBeenCalledExactlyOnceWith('beta', 'b1');

  await click('[data-capacity-filter="basis_incomplete"]');
  expect(host.querySelector('[data-capacity-person="over"]')).toBeNull();
  expect(host.querySelector('[data-capacity-person="user"] [data-capacity-available]')?.textContent).toBe('Unknown');
  expect(host.querySelector('[data-capacity-calendar="peer"]')).toBeNull();
  await click('[data-capacity-calendar="user"]');
  expect(onCalendar).toHaveBeenCalledExactlyOnceWith('user');
  await draw({...session, user: {...session.user, role: 'admin'}});
  await click('[data-capacity-calendar="peer"]');
  expect(onCalendar).toHaveBeenLastCalledWith('peer');
});

it('clears task evidence when a refresh is denied', async () => {
  read.mockReturnValueOnce(response(week)).mockReturnValueOnce(Response.json({
    error: {code: 'forbidden', message: 'Access revoked'},
  }, {status: 403}));
  await draw();
  expect(host.querySelector('[data-capacity-person]')).toBeTruthy();
  await click('[data-capacity-refresh]');
  expect(host.querySelector('[data-capacity-person]')).toBeNull();
  expect(host.querySelector('[data-capacity-empty]')).toBeNull();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('Access revoked');
});

it('ignores late results from an earlier week or organization', async () => {
  let resolveWeek!: (value: Response) => void;
  let resolveOrganization!: (value: Response) => void;
  const moved = {...session, organization: {id: 'other', name: 'Other'}};
  read.mockImplementationOnce(() => new Promise(resolve => {resolveWeek = resolve;}))
    .mockImplementationOnce(() => new Promise(resolve => {resolveOrganization = resolve;}))
    .mockReturnValueOnce(response(nextWeek, []));
  await draw();
  await click('[data-capacity-next]');
  await draw(moved);
  expect(read.mock.calls.map(call => call[0])).toEqual([
    '/api/v1/project-portfolio/capacity?week=' + week,
    '/api/v1/project-portfolio/capacity?week=' + nextWeek,
    '/api/v1/project-portfolio/capacity?week=' + nextWeek,
  ]);
  expect(read.mock.calls[0][1].signal?.aborted).toBe(true);
  expect(read.mock.calls[1][1].signal?.aborted).toBe(true);
  await act(async () => {
    resolveWeek(response(week));
    resolveOrganization(response(nextWeek));
  });
  expect(host.querySelector('[data-capacity-person]')).toBeNull();
  expect(host.querySelector('[data-capacity-empty]')).toBeTruthy();
});

it('keeps bounded results distinct from an empty week', async () => {
  read.mockReturnValue(Response.json({
    error: {code: 'project_capacity_scope_too_large', message: 'Scope too large'},
  }, {status: 409}));
  await draw();
  expect(host.querySelector('[role="alert"]')?.textContent).toContain('too many qualifying tasks');
  expect(host.querySelector('[data-capacity-empty]')).toBeNull();
  expect(host.querySelector('[data-capacity-next]')).toMatchObject({disabled: false});
});
