// @vitest-environment jsdom
import {act, useState} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ProjectDirectory} from './ManagementNavigation';

const session: Session = {
  user: {id: 'user', name: 'User', email: 'user@example.test', role: 'member'},
  organization: {id: 'org', name: 'Engineering'},
  projects: [{id: 'not-in-current-snapshot', title: 'Old session project', version: 1}],
};
const item = (id: string, details = {}) => ({
  project_id: id, title: id, description: '', status: 'active',
  task_total: 0, accepted_tasks: 0, blocked_tasks: 0, active_risks: 0, ...details,
});
const page = (items: ReturnType<typeof item>[], next_cursor?: string) =>
  Response.json({items, next_cursor, as_of: '2026-11-05T08:00:00Z'});
let root: Root;
let host: HTMLDivElement;
const read = vi.fn<(url: string, init: RequestInit) => Response | Promise<Response>>();
const open = vi.fn(), overview = vi.fn(), create = vi.fn();

function View({session: current, refresh = 0}: {session: Session; refresh?: number}) {
  const [q, setQuery] = useState('');
  return <ApiViewProvider session={current}>
    <ProjectDirectory session={current} refresh={refresh} q={q} onSearch={setQuery}
      onOverview={overview} onOpen={open} onCreate={create} onScheduleOverlap={()=>{}}/>
  </ApiViewProvider>;
}

async function draw(current = session, refresh = 0) {
  await act(async () => root.render(<View session={current} refresh={refresh}/>));
}
async function click(selector: string) {
  const button = host.querySelector<HTMLButtonElement>(selector);
  expect(button).toBeTruthy();
  await act(async () => button!.click());
}
async function search(text: string) {
  const input = host.querySelector<HTMLInputElement>('[data-portfolio-search]')!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, text);
    input.dispatchEvent(new Event('input', {bubbles: true}));
  });
  await act(async () => input.form!.dispatchEvent(new Event('submit', {bubbles: true, cancelable: true})));
}
const visible = () => Array.from(host.querySelectorAll<HTMLElement>('[data-portfolio-project]'), row => row.dataset.portfolioProject);

beforeEach(async () => {
  await setLocale('en-US', {persist: false});
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  read.mockReset();
  open.mockReset();
  overview.mockReset();
  create.mockReset();
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

it('uses formal counts, milestone dates, and the exact selected project from the current snapshot', async () => {
  read.mockReturnValue(page([
    item('Ready project', {task_total: 8, accepted_tasks: 3, blocked_tasks: 2, active_risks: 1,
      next_milestone: {id: 'm', title: 'Delivery review', target_date: '2026-11-04'}}),
    item('Empty project'),
  ]));
  await draw();
  const [filled, empty] = host.querySelectorAll('[data-portfolio-project]');
  expect(filled.querySelector('progress')).toMatchObject({value: 3, max: 8});
  expect(filled.querySelector('time')?.dateTime).toBe('2026-11-04');
  expect(filled.textContent).toContain('Overdue');
  expect(filled.querySelector('[data-portfolio-blocked="2"] dd')?.textContent).toBe('2');
  expect(filled.querySelector('[data-portfolio-risks="1"] dd')?.textContent).toBe('1');
  expect(empty.querySelector('progress')).toBeNull();
  expect(empty.textContent).toContain('No dated milestone');
  expect(visible()).toEqual(['Ready project', 'Empty project']);
  expect(host.textContent).not.toContain('New project');
  const [url, init] = read.mock.calls[0];
  expect(url).toBe('/api/v1/project-portfolio?limit=20');
  expect(init.method).toBe('GET');
  expect(new Headers(init.headers).get('X-Suma-Expected-Organization')).toBe('org');
  expect(new Headers(init.headers).get('X-Suma-Expected-User')).toBe('user');
  await click('[data-portfolio-overview]');
  await click('[data-portfolio-settings]');
  expect(overview).toHaveBeenCalledExactlyOnceWith('Ready project');
  expect(open).toHaveBeenCalledExactlyOnceWith('Ready project');
});

it('searches on the server, replaces pages, and returns to the same query', async () => {
  read.mockImplementation(url => {
    const p = new URL(url, 'http://test').searchParams;
    if (p.get('q') === 'needle') return page([item('Matched')]);
    return p.get('after') ? page([item('Second')]) : page([item('First')], 'cursor-one');
  });
  await draw();
  await click('[data-portfolio-next]');
  expect(visible()).toEqual(['Second']);
  expect(document.activeElement?.textContent).toBe('Accessible projects');
  await click('[data-portfolio-previous]');
  expect(visible()).toEqual(['First']);
  await search('  needle  ');
  expect(visible()).toEqual(['Matched']);
  expect(read.mock.lastCall?.[0]).toBe('/api/v1/project-portfolio?limit=20&q=needle');
  await click('[data-portfolio-clear]');
  expect(visible()).toEqual(['First']);
  expect(read.mock.lastCall?.[0]).toBe('/api/v1/project-portfolio?limit=20');
});

it('drops previous rows on a refresh refusal and does not fall back to session projects', async () => {
  read.mockReturnValueOnce(page([item('Protected')])).mockReturnValueOnce(
    Response.json({error: {code: 'forbidden', message: 'Access changed'}}, {status: 403}),
  );
  await draw();
  expect(visible()).toEqual(['Protected']);
  await click('[data-portfolio-refresh]');
  expect(visible()).toEqual([]);
  expect(host.querySelector('[role=alert]')?.textContent).toContain('Access changed');
  expect(host.textContent).not.toContain('Old session project');
});

it('ignores late data from a previously visible organization and replaces rows on scope change', async () => {
  let finish!: (response: Response) => void;
  read.mockImplementationOnce(() => new Promise(resolve => {finish = resolve;}))
    .mockImplementationOnce(() => page([item('Only new organization')]));
  await draw();
  await draw({...session, organization: {id: 'new-org', name: 'Second org'}});
  expect(visible()).toEqual(['Only new organization']);
  expect(read.mock.calls[0][1].signal?.aborted).toBe(true);
  await act(async () => finish(page([item('Leaked old organization')])));
  expect(visible()).toEqual(['Only new organization']);
});

it('offers a way back to the first page after a revoked or invalid cursor', async () => {
  read.mockImplementation(url => url.includes('after=') ? Response.json(
    {error: {code: 'invalid', message: 'Invalid pagination position'}}, {status: 400},
  ) : page([item('First')], 'no-longer-authorized'));
  await draw();
  await click('[data-portfolio-next]');
  expect(visible()).toEqual([]);
  expect(host.querySelector('[role=alert]')?.textContent).toContain('Go back to the first page');
  await click('[data-portfolio-first]');
  expect(visible()).toEqual(['First']);
});
