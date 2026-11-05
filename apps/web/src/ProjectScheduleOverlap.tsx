import {ArrowLeft, ArrowUpRight, CalendarDays, ChevronLeft, ChevronRight, Layers, RefreshCw} from 'lucide-react';
import {useCallback, useEffect, useId, useState, type MouseEvent} from 'react';
import type {ProjectScheduleOverlapAssignment, ProjectScheduleOverlapPage, ProjectScheduleOverlapPerson} from '../../../packages/contracts/src';
import {ApiError, type ApiCaller, type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime, formatNumber, tr, UiError, useLocale} from './i18n';
import {isCapacityWeek, mondayOf, planDate} from './planning-model';
import {Empty, ErrorNotice, Loading} from './ui';
import './ProjectScheduleOverlap.css';

type Props = {
  session: Session;
  week: string;
  refresh: number;
  onBack: () => void;
  onWeek: (week: string) => void;
  onTask: (project: string, task: string) => void;
};
type Read = {
  api: ApiCaller;
  week: string;
  loading: boolean;
  page?: ProjectScheduleOverlapPage;
  error?: unknown;
};

const t = (key: string, params?: Record<string, unknown>) => tr('project-schedule-overlap.' + key, params);
const dateOnly = (date: string) => formatDateTime(date, {dateStyle: 'medium', timeZone: 'UTC'});
const shortDate = (date: string) => formatDateTime(date, {month: 'short', day: 'numeric', timeZone: 'UTC'});
const weekday = (date: string) => formatDateTime(date, {weekday: 'short', timeZone: 'UTC'});

function shiftDate(value: string, offset: number) {
  const ms = planDate(value);
  return Number.isFinite(ms) ? new Date(ms + offset * 86400000).toISOString().slice(0, 10) : '';
}

function isDate(value: string) {
  const ms = planDate(value);
  return Number.isFinite(ms) && new Date(ms).toISOString().slice(0, 10) === value;
}

function validPage(page: ProjectScheduleOverlapPage, week: string, days: string[]) {
  return !!page && page.week_start === week && typeof page.as_of === 'string' &&
    Number.isFinite(Date.parse(page.as_of)) && Array.isArray(page.items) &&
    new Set(page.items.map(person => person?.user_id)).size === page.items.length &&
    page.items.every(person => person && typeof person.user_id === 'string' && !!person.user_id &&
      typeof person.name === 'string' && Array.isArray(person.days) && person.days.length > 0 &&
      person.days.every((date, index) => days.includes(date) && (!index || date > person.days[index - 1])) &&
      Array.isArray(person.assignments) && person.assignments.length > 1 &&
      person.assignments.every(assignment => assignment && typeof assignment.project_id === 'string' && !!assignment.project_id &&
        typeof assignment.project_title === 'string' && typeof assignment.task_id === 'string' && !!assignment.task_id &&
        typeof assignment.task_title === 'string' && isDate(assignment.start_date) && isDate(assignment.due_date) &&
        assignment.start_date <= assignment.due_date));
}

export function ProjectScheduleOverlap(props: Props) {
  const {user, organization} = props.session;
  return <ScheduleOverlapScope key={JSON.stringify([user.id, organization.id])} {...props}/>;
}

function ScheduleOverlapScope({session, week, refresh, onBack, onWeek, onTask}: Props) {
  useLocale();
  const api = useApi();
  const id = useId();
  const {user, organization} = session;
  const [read, setRead] = useState<Read>();
  const [revision, setRevision] = useState(0);
  const validWeek = isCapacityWeek(week);
  const days = validWeek ? Array.from({length: 7}, (_, index) => shiftDate(week, index)) : [];
  const current = read?.api === api && read.week === week ? read : undefined;
  const page = current?.page;
  const loading = validWeek && (!current || current.loading);
  const reload = useCallback(() => setRevision(value => value + 1), []);
  const thisWeek = mondayOf();
  const previousWeek = shiftDate(week, -7);
  const nextWeek = shiftDate(week, 7);

  useEffect(() => {
    if (!validWeek) return;
    const abort = new AbortController();
    setRead(previous => ({
      api, week, loading: true,
      page: previous?.api === api && previous.week === week ? previous.page : undefined,
    }));
    const weekDays = Array.from({length: 7}, (_, index) => shiftDate(week, index));
    const path = '/project-portfolio/schedule-overlaps?' + new URLSearchParams({week});
    void api<ProjectScheduleOverlapPage>(path, undefined, 'GET', {
      'X-Suma-Expected-User': user.id,
      'X-Suma-Expected-Organization': organization.id,
    }, abort.signal).then(result => {
      if (abort.signal.aborted) return;
      if (!validPage(result, week, weekDays)) throw new UiError('project-schedule-overlap.invalid_snapshot');
      setRead({api, week, loading: false, page: result});
    }).catch(error => {
      if (!abort.signal.aborted) setRead({api, week, loading: false, error});
    });
    return () => abort.abort();
  }, [api, week, validWeek, refresh, revision, user.id, organization.id]);

  useEffect(() => {
    const recheck = () => {
      if (document.visibilityState !== 'hidden' && navigator.onLine !== false && !loading) reload();
    };
    window.addEventListener('online', recheck);
    document.addEventListener('visibilitychange', recheck);
    const timer = window.setInterval(recheck, 30000);
    return () => {
      window.removeEventListener('online', recheck);
      document.removeEventListener('visibilitychange', recheck);
      window.clearInterval(timer);
    };
  }, [loading, reload]);

  const scopeLimit = current?.error instanceof ApiError && current.error.status === 409 &&
    current.error.code === 'schedule_overlap_scope_too_large';
  return <section className="project-schedule-overlap" data-project-schedule-overlap aria-labelledby={id + '-title'}>
    <header className="overlap-heading">
      <div>
        <button type="button" data-overlap-back className="text-button overlap-back" onClick={onBack}>
          <ArrowLeft size={16} aria-hidden="true"/>{t('back')}
        </button>
        <h2 id={id + '-title'}>{t('title')}</h2>
        <p>{t('description')}</p>
      </div>
    </header>
    <div className="overlap-results panel">
      <div className="overlap-toolbar">
        <div className="overlap-week">
          <CalendarDays size={19} aria-hidden="true"/>
          <div>
            <span>{t('selected_week')}</span>
            {validWeek && <strong>{t('week_range', {start: dateOnly(week), end: dateOnly(days[6])})}</strong>}
          </div>
        </div>
        <div className="overlap-controls" role="group" aria-label={t('week_controls')}>
          <button type="button" data-overlap-previous aria-label={t('previous')} title={t('previous')}
            disabled={!validWeek || !isCapacityWeek(previousWeek)} onClick={() => onWeek(previousWeek)}><ChevronLeft size={18} aria-hidden="true"/></button>
          <button type="button" data-overlap-current disabled={week === thisWeek} onClick={() => onWeek(thisWeek)}>{t('this_week')}</button>
          <button type="button" data-overlap-next aria-label={t('next')} title={t('next')}
            disabled={!validWeek || !isCapacityWeek(nextWeek)} onClick={() => onWeek(nextWeek)}><ChevronRight size={18} aria-hidden="true"/></button>
          <button type="button" data-overlap-refresh aria-label={t('refresh')} title={t('refresh')}
            disabled={loading || !validWeek} onClick={reload}><RefreshCw size={16} aria-hidden="true"/></button>
        </div>
      </div>
      <div className="overlap-status" role="status">
        {page && <strong>{t('member_count', {count: formatNumber(page.items.length)})}</strong>}
        {page && <span>{t('as_of', {time: formatDateTime(page.as_of, {dateStyle: 'short', timeStyle: 'medium'})})}</span>}
        {page && loading && <span>{t('checking')}</span>}
      </div>
      {!validWeek && <div className="overlap-feedback"><ErrorNotice error={new UiError('project-schedule-overlap.invalid_week')}/></div>}
      {!page && loading && <Loading/>}
      {Boolean(current?.error) && <div className="overlap-feedback">
        <ErrorNotice error={scopeLimit ? new UiError('project-schedule-overlap.scope_limit') : current?.error}
          onRetry={scopeLimit ? undefined : reload}/>
      </div>}
      {page?.items.length === 0 && <div data-overlap-empty><Empty title={t('empty')} description={t('empty_hint')}/></div>}
      {!!page?.items.length && <ul className="overlap-people" aria-busy={loading}>
        {page.items.map(person => <OverlapPerson key={person.user_id} person={person} days={days} onTask={onTask}/>)}
      </ul>}
    </div>
    <p className="overlap-scope-note">{t('scope_note')}</p>
  </section>;
}

function OverlapPerson({person, days, onTask}: {
  person: ProjectScheduleOverlapPerson;
  days: string[];
  onTask: Props['onTask'];
}) {
  const projects = new Map<string, ProjectScheduleOverlapAssignment[]>();
  for (const assignment of person.assignments) {
    const items = projects.get(assignment.project_id);
    if (items) items.push(assignment);
    else projects.set(assignment.project_id, [assignment]);
  }
  const marked = new Set(person.days);
  const name = person.name || person.user_id;
  return <li className="overlap-person" data-overlap-person={person.user_id}>
    <div className="overlap-person-summary">
      <div className="overlap-person-name">
        <h3>{name}</h3>
        <span>{t('project_count', {count: formatNumber(projects.size)})}</span>
      </div>
      <ol className="overlap-week-grid" aria-label={t('overlapping_dates', {person: name})}>
        {days.map(day => <li key={day} data-overlap-day={day} data-overlapped={marked.has(day)}>
          <time dateTime={day}>{weekday(day)}<strong>{shortDate(day)}</strong></time>
          {marked.has(day) && <Layers size={15} aria-hidden="true"/>}
          <span className="overlap-day-description">{marked.has(day) ? t('overlapped') : t('not_overlapped')}</span>
        </li>)}
      </ol>
    </div>
    <details className="overlap-assignments" data-overlap-assignments>
      <summary>{t('show_tasks', {count: formatNumber(person.assignments.length)})}</summary>
      <div className="overlap-projects">
        {[...projects].map(([project, assignments]) => <section key={project} data-overlap-project={project}>
          <h4>{assignments[0].project_title || project}</h4>
          <ul>{assignments.map(task => <li key={task.task_id}>
            <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'tasks', record: task.task_id})}
              data-overlap-task={task.task_id} onClick={(event: MouseEvent<HTMLAnchorElement>) => {
                if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
                event.preventDefault();
                onTask(project, task.task_id);
              }}>{task.task_title || task.task_id}<ArrowUpRight size={14} aria-hidden="true"/></a>
            <span>{t('week_range', {start: dateOnly(task.start_date), end: dateOnly(task.due_date)})}</span>
          </li>)}</ul>
        </section>)}
      </div>
    </details>
  </li>;
}
