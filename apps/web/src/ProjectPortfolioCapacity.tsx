import {ArrowLeft, ArrowUpRight, CalendarDays, ChevronLeft, ChevronRight, CircleCheck, CircleHelp, RefreshCw, TriangleAlert, UsersRound} from 'lucide-react';
import {useCallback, useEffect, useId, useMemo, useState, type MouseEvent} from 'react';
import type {ProjectPortfolioCapacityPage, ProjectPortfolioCapacityPerson, ProjectPortfolioCapacityTask} from '../../../packages/contracts/src';
import {ApiError, type ApiCaller, type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime, tr, UiError, useLocale} from './i18n';
import {isCapacityWeek, mondayOf, planDate} from './planning-model';
import {Empty, ErrorNotice, Loading} from './ui';
import './ProjectPortfolioCapacity.css';

type Props = {
  session: Session;
  week: string;
  refresh: number;
  onBack: () => void;
  onWeek: (week: string) => void;
  onTask: (project: string, task: string) => void;
  onCalendar: (user: string) => void;
};
type Read = {
  api: ApiCaller;
  week: string;
  loading: boolean;
  page?: ProjectPortfolioCapacityPage;
  error?: unknown;
};

const states = ['over_capacity', 'basis_incomplete', 'within_visible_capacity'] as const;
const filters = ['all', ...states] as const;
type Filter = typeof filters[number];
type DayLabel = {date: string; weekday: string; shortDate: string};
const stateIcons = {all: UsersRound, over_capacity: TriangleAlert, basis_incomplete: CircleHelp, within_visible_capacity: CircleCheck};
const t = (key: string, params?: Record<string, unknown>) => tr('project-portfolio-capacity.' + key, params);
const dateOnly = (date: string) => formatDateTime(date, {dateStyle: 'medium', timeZone: 'UTC'});
const dateRange = (start: string, end: string) => start === end ? dateOnly(start) : t('week_range', {start: dateOnly(start), end: dateOnly(end)});

function shiftDate(value: string, offset: number) {
  const ms = planDate(value);
  if (!Number.isFinite(ms)) return '';
  const result = new Date(ms + offset * 86400000).toISOString().split('T')[0];
  return result.length === 10 && !result.startsWith('0000-') ? result : '';
}
const validDate = (value: string) => typeof value === 'string' && value.length === 10 && shiftDate(value, 0) === value;
const validWeekDate = (value: string) => isCapacityWeek(value) && validDate(value) && !!shiftDate(value, 6);
const nonnegative = (value: number) => Number.isFinite(value) && value >= 0;
const count = (value: number) => Number.isSafeInteger(value) && value >= 0;
const hours = (value: number | null) => value === null || nonnegative(value);

function validTask(task: ProjectPortfolioCapacityTask, week: string, end: string) {
  return task && typeof task.project_id === 'string' && !!task.project_id && typeof task.project_title === 'string' &&
    typeof task.task_id === 'string' && !!task.task_id && typeof task.task_title === 'string' &&
    validDate(task.start_date) && validDate(task.due_date) && task.start_date <= task.due_date &&
    task.start_date <= end && task.due_date >= week && hours(task.estimated_hours) && hours(task.allocated_hours);
}

function validPage(page: ProjectPortfolioCapacityPage, week: string) {
  if (!page || page.week_start !== week || typeof page.as_of !== 'string' ||
    !Number.isFinite(Date.parse(page.as_of)) || !Array.isArray(page.items) ||
    new Set(page.items.map(person => person?.user_id)).size !== page.items.length) return false;
  const end = shiftDate(week, 6);
  return page.items.every(person => person && typeof person.user_id === 'string' && !!person.user_id &&
    typeof person.name === 'string' && count(person.calendar_version) && hours(person.available_hours) &&
    nonnegative(person.allocated_hours) && count(person.unestimated_tasks) && count(person.unallocated_tasks) &&
    states.some(state => state === person.state) && Array.isArray(person.days) && person.days.length === 7 &&
    person.days.every((day, index) => day && day.date === shiftDate(week, index) &&
      hours(day.available_hours) && nonnegative(day.allocated_hours)) &&
    Array.isArray(person.tasks) && person.tasks.length > 0 &&
    person.tasks.every(task => validTask(task, week, end)));
}

export function ProjectPortfolioCapacity(props: Props) {
  const {user, organization} = props.session;
  return <CapacityScope key={JSON.stringify([user.id, organization.id])} {...props}/>;
}

function CapacityScope({session, week, refresh, onBack, onWeek, onTask, onCalendar}: Props) {
  const locale = useLocale();
  const api = useApi();
  const id = useId();
  const {user, organization} = session;
  const [read, setRead] = useState<Read>();
  const [revision, setRevision] = useState(0);
  const [filter, setFilter] = useState<Filter>('all');
  const reload = useCallback(() => setRevision(value => value + 1), []);
  const number = useMemo(() => new Intl.NumberFormat(locale, {maximumFractionDigits: 2}), [locale]);
  const validWeek = validWeekDate(week);
  const current = read?.api === api && read.week === week ? read : undefined;
  const page = current?.page;
  const loading = validWeek && (!current || current.loading);
  const permanentError = current?.error instanceof ApiError &&
    (current.error.status === 401 || current.error.status === 403 ||
      (current.error.status === 409 && current.error.code === 'project_capacity_scope_too_large'));
  const thisWeek = mondayOf();
  const previous = shiftDate(week, -7);
  const next = shiftDate(week, 7);
  const lastDay = shiftDate(week, 6);
  const labels: DayLabel[] = validWeek ? Array.from({length: 7}, (_, index) => {
    const date = shiftDate(week, index);
    return {
      date,
      weekday: formatDateTime(date, {weekday: 'short', timeZone: 'UTC'}),
      shortDate: formatDateTime(date, {month: 'short', day: 'numeric', timeZone: 'UTC'}),
    };
  }) : [];

  useEffect(() => {
    if (!validWeek) return;
    const abort = new AbortController();
    setRead(previous => ({
      api, week, loading: true,
      page: previous?.api === api && previous.week === week ? previous.page : undefined,
    }));
    const path = '/project-portfolio/capacity?' + new URLSearchParams({week});
    void api<ProjectPortfolioCapacityPage>(path, undefined, 'GET', {
      'X-Suma-Expected-User': user.id,
      'X-Suma-Expected-Organization': organization.id,
    }, abort.signal).then(result => {
      if (abort.signal.aborted) return;
      if (!validPage(result, week)) throw new UiError('project-portfolio-capacity.invalid_snapshot');
      setRead({api, week, loading: false, page: result});
    }).catch(error => {
      if (!abort.signal.aborted) setRead({api, week, loading: false, error});
    });
    return () => abort.abort();
  }, [api, week, validWeek, refresh, revision, user.id, organization.id]);

  useEffect(() => {
    if (!validWeek || loading || permanentError) return;
    const recheck = () => {
      if (document.visibilityState !== 'hidden' && navigator.onLine !== false) reload();
    };
    window.addEventListener('online', recheck);
    document.addEventListener('visibilitychange', recheck);
    const timer = window.setInterval(recheck, 30000);
    return () => {
      window.removeEventListener('online', recheck);
      document.removeEventListener('visibilitychange', recheck);
      window.clearInterval(timer);
    };
  }, [loading, permanentError, reload, validWeek]);

  const buckets = {
    over_capacity: [] as ProjectPortfolioCapacityPerson[],
    basis_incomplete: [] as ProjectPortfolioCapacityPerson[],
    within_visible_capacity: [] as ProjectPortfolioCapacityPerson[],
  };
  for (const person of page?.items ?? []) {
    const state = states.find(value => value === person.state);
    if (state) buckets[state].push(person);
  }
  const people = filter === 'all' ? states.flatMap(state => buckets[state]) : buckets[filter];
  const scopeLimit = current?.error instanceof ApiError && current.error.status === 409 &&
    current.error.code === 'project_capacity_scope_too_large';
  return <section className="portfolio-capacity" data-project-portfolio-capacity aria-labelledby={id + '-title'}>
    <header className="portfolio-capacity-heading">
      <button type="button" data-capacity-back className="text-button" onClick={onBack}>
        <ArrowLeft size={16} aria-hidden="true"/>{t('back')}
      </button>
      <h2 id={id + '-title'}>{t('title')}</h2>
      <p>{t('description')}</p>
    </header>

    <div className="portfolio-capacity-results panel">
      <div className="portfolio-capacity-toolbar">
        <div className="portfolio-capacity-week">
          <span className="portfolio-capacity-calendar"><CalendarDays size={20} aria-hidden="true"/></span>
          <div>
            <span>{t('selected_week')}</span>
            <strong>{validWeek ? dateRange(week, lastDay) : t('invalid_week_label')}</strong>
          </div>
        </div>
        <div className="portfolio-capacity-controls" role="group" aria-label={t('week_controls')}>
          <button type="button" data-capacity-previous aria-label={t('previous')} title={t('previous')}
            disabled={!validWeek || !validWeekDate(previous)} onClick={() => onWeek(previous)}><ChevronLeft size={18} aria-hidden="true"/></button>
          <button type="button" data-capacity-current disabled={week === thisWeek} onClick={() => onWeek(thisWeek)}>{t('this_week')}</button>
          <button type="button" data-capacity-next aria-label={t('next')} title={t('next')}
            disabled={!validWeek || !validWeekDate(next)} onClick={() => onWeek(next)}><ChevronRight size={18} aria-hidden="true"/></button>
          <button type="button" data-capacity-refresh aria-label={t('refresh')} title={t('refresh')}
            disabled={loading || !validWeek} onClick={reload}><RefreshCw size={16} aria-hidden="true"/></button>
        </div>
      </div>
      <p className="portfolio-capacity-scope">{t('scope_note')}</p>
      {!validWeek && <div className="portfolio-capacity-feedback"><ErrorNotice error={new UiError('project-portfolio-capacity.invalid_week')}/></div>}
      {!page && loading && <Loading/>}
      {Boolean(current?.error) && <div className="portfolio-capacity-feedback">
        <ErrorNotice error={scopeLimit ? new UiError('project-portfolio-capacity.scope_limit') : current?.error}
          onRetry={scopeLimit ? undefined : reload}/>
      </div>}
      {page && <div className="portfolio-capacity-snapshot" role="status">
        <span>{t('as_of', {time: formatDateTime(page.as_of, {dateStyle: 'short', timeStyle: 'medium'})})}</span>
        {loading && <span>{t('checking')}</span>}
      </div>}
      {page?.items.length === 0 && <div data-capacity-empty><Empty title={t('empty')} description={t('empty_hint')}/></div>}
      {!!page?.items.length && <>
        <div className="portfolio-capacity-filters" role="group" aria-label={t('filter_label')}>
          {filters.map(value => {
            const Icon = stateIcons[value];
            const total = value === 'all' ? page.items.length : buckets[value].length;
            return <button key={value} type="button" data-capacity-filter={value} aria-pressed={filter === value}
              onClick={() => setFilter(value)}>
              <Icon size={16} aria-hidden="true"/><span>{t(value)}</span><strong>{number.format(total)}</strong>
            </button>;
          })}
        </div>
        {people.length > 0 && <>
          <div className="portfolio-capacity-legend" role="group" aria-label={t('chart_legend')}>
            <span><i className="portfolio-capacity-load-key"/>{t('allocated')}</span>
            <span><i className="portfolio-capacity-limit-key"/>{t('available')}</span>
            <span><TriangleAlert size={14} aria-hidden="true"/>{t('daily_over_capacity')}</span>
          </div>
          <ul className="portfolio-capacity-people" aria-busy={loading}>
            {people.map(person => <CapacityPerson key={person.user_id} person={person} labels={labels} number={number} onTask={onTask}
              onCalendar={person.user_id === user.id || user.role === 'owner' || user.role === 'admin' ? onCalendar : undefined}/>)}
          </ul>
        </>}
        {people.length === 0 && <Empty title={t('filter_empty')} description={t('filter_empty_hint')}/>}
      </>}
    </div>
  </section>;
}

function hourText(value: number | null, number: Intl.NumberFormat) {
  if (value === null) return t('unknown');
  if (value > 0 && number.format(value) === number.format(0)) {
    return t('small_hours', {value: number.format(0.01)});
  }
  return t('hours', {value: number.format(value)});
}

function CapacityPerson({person, labels, number, onTask, onCalendar}: {
  person: ProjectPortfolioCapacityPerson;
  labels: DayLabel[];
  number: Intl.NumberFormat;
  onTask: Props['onTask'];
  onCalendar?: Props['onCalendar'];
}) {
  const [showTasks, setShowTasks] = useState(false);
  const name = person.name || person.user_id;
  const state = states.find(value => value === person.state) ?? 'basis_incomplete';
  const Icon = stateIcons[state];
  const scale = Math.max(1, ...person.days.map(day => Math.max(day.allocated_hours, day.available_hours ?? 0)));
  return <li className="portfolio-capacity-person" data-capacity-person={person.user_id} data-capacity-state={state}>
    <div className="portfolio-capacity-person-overview">
      <div className="portfolio-capacity-person-heading">
        <h3>{name}</h3>
        <span className="portfolio-capacity-badge"><Icon size={14} aria-hidden="true"/>{t(state)}</span>
      </div>
      <dl className="portfolio-capacity-totals">
        <div><dt>{t('allocated')}</dt><dd data-capacity-allocated>{hourText(person.allocated_hours, number)}</dd></div>
        <div><dt>{t('available')}</dt><dd data-capacity-available>{hourText(person.available_hours, number)}</dd></div>
      </dl>
      {(person.unestimated_tasks > 0 || person.unallocated_tasks > 0 || person.calendar_version === 0) &&
        <ul className="portfolio-capacity-diagnostics">
          {person.calendar_version === 0 && <li>{t('calendar_missing')}</li>}
          {person.unestimated_tasks > 0 && <li>{t('unestimated', {count: number.format(person.unestimated_tasks)})}</li>}
          {person.unallocated_tasks > 0 && <li>{t('unallocated', {count: number.format(person.unallocated_tasks)})}</li>}
        </ul>}
      {person.available_hours === null && (onCalendar
        ? <button type="button" className="text-button portfolio-capacity-calendar-link" data-capacity-calendar={person.user_id}
            onClick={() => onCalendar(person.user_id)}><CalendarDays size={14} aria-hidden="true"/>{t('view_calendar')}</button>
        : <p className="portfolio-capacity-calendar-hint">{t('ask_calendar_admin')}</p>)}
    </div>
    <ol className="portfolio-capacity-days" aria-label={t('daily_breakdown', {person: name})}>
      {person.days.map((day, index) => {
        const label = labels[index];
        const over = day.available_hours !== null && day.allocated_hours > day.available_hours + 1e-9;
        return <li key={day.date} data-capacity-day={day.date} data-capacity-day-over={over}>
          <time dateTime={day.date}><span>{label.weekday}</span><strong>{label.shortDate}</strong></time>
          <div className="portfolio-capacity-bars" aria-hidden="true">
            <span className="portfolio-capacity-load-key" style={{width: day.allocated_hours / scale * 100 + '%'}}/>
            {day.available_hours !== null && <span className="portfolio-capacity-limit-key"
              style={{width: day.available_hours / scale * 100 + '%'}}/>}
          </div>
          <span className="portfolio-capacity-day-values">
            <span className="portfolio-capacity-sr">{t('allocated')}{' '}</span>
            <span data-capacity-day-allocated>
              {hourText(day.allocated_hours, number)}
            </span>
            <span aria-hidden="true">/</span>
            <span className="portfolio-capacity-sr">{t('available')}{' '}</span>
            <span data-capacity-day-available>
              {hourText(day.available_hours, number)}
            </span>
            {over && <TriangleAlert size={12} role="img" aria-label={t('daily_over_capacity')}/>}
          </span>
        </li>;
      })}
    </ol>
    <details className="portfolio-capacity-tasks" data-capacity-tasks onToggle={event => setShowTasks(event.currentTarget.open)}>
      <summary>{t('show_tasks', {count: number.format(person.tasks.length)})}</summary>
      {showTasks && <CapacityTasks tasks={person.tasks} number={number} onTask={onTask}/>}
    </details>
  </li>;
}

function CapacityTasks({tasks, number, onTask}: {
  tasks: ProjectPortfolioCapacityTask[];
  number: Intl.NumberFormat;
  onTask: Props['onTask'];
}) {
  const projects = new Map<string, ProjectPortfolioCapacityTask[]>();
  for (const task of tasks) {
    const items = projects.get(task.project_id);
    if (items) items.push(task);
    else projects.set(task.project_id, [task]);
  }
  return <div className="portfolio-capacity-projects">
    {[...projects].map(([project, items]) => <section key={project} data-capacity-project={project}>
      <h4>{items[0].project_title || project}</h4>
      <ul>{items.map(task => <li key={task.task_id}>
        <div>
          <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'tasks', record: task.task_id})}
            data-capacity-task={task.task_id} onClick={(event: MouseEvent<HTMLAnchorElement>) => {
              if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
              event.preventDefault();
              onTask(project, task.task_id);
            }}>{task.task_title || task.task_id}<ArrowUpRight size={14} aria-hidden="true"/></a>
          <span>{dateRange(task.start_date, task.due_date)}</span>
        </div>
        <div>
          <span>{task.estimated_hours === null ? t('no_estimate') : t('estimate_value', {hours: hourText(task.estimated_hours, number)})}</span>
          <strong>{task.allocated_hours === null ? t('no_allocation') : t('allocated_value', {hours: hourText(task.allocated_hours, number)})}</strong>
        </div>
      </li>)}</ul>
    </section>)}
  </div>;
}
