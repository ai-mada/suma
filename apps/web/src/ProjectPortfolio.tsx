import {ArrowRight, CalendarDays, ChevronLeft, ChevronRight, FolderKanban, Plus, RefreshCw, Search, Settings2} from 'lucide-react';
import {useCallback, useEffect, useId, useRef, useState} from 'react';
import type {ProjectPortfolioItem, ProjectPortfolioPage} from '../../../packages/contracts/src';
import {ApiError, type ApiCaller, type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime, formatNumber, tr, UiError, useLocale} from './i18n';
import {Badge, Empty, ErrorNotice, Loading} from './ui';
import './ProjectPortfolio.css';

type Props = {
  session: Session;
  q: string;
  refresh: number;
  onSearch: (q: string) => void;
  onCreate: () => void;
  onOpen: (id: string) => void;
  onOverview: (id: string) => void;
};
type Read = {
  api: ApiCaller;
  path: string;
  loading: boolean;
  page?: ProjectPortfolioPage;
  error?: unknown;
};

const pageSize = 20;
const t = (key: string, params?: Record<string, unknown>) => tr('project-portfolio.' + key, params);
const dateOnly = (date: string) => formatDateTime(date, {dateStyle: 'medium', timeZone: 'UTC'});
const count = (value: number) => Number.isSafeInteger(value) && value >= 0;

function validPage(page: ProjectPortfolioPage, cursor: string): boolean {
  return !!page && typeof page.as_of === 'string' && Number.isFinite(Date.parse(page.as_of)) &&
    Array.isArray(page.items) && page.items.length <= pageSize &&
    (page.next_cursor === undefined || typeof page.next_cursor === 'string' &&
      (!page.next_cursor || page.items.length > 0 && page.next_cursor !== cursor)) &&
    new Set(page.items.map(item => item?.project_id)).size === page.items.length &&
    page.items.every(item => item && typeof item.project_id === 'string' && !!item.project_id &&
      typeof item.title === 'string' && typeof item.description === 'string' && typeof item.status === 'string' &&
      [item.task_total, item.accepted_tasks, item.blocked_tasks, item.active_risks].every(count) &&
      item.accepted_tasks + item.blocked_tasks <= item.task_total &&
      (item.next_milestone === undefined || item.next_milestone && typeof item.next_milestone.title === 'string' &&
        typeof item.next_milestone.id === 'string' && !!item.next_milestone.id &&
        /^\d{4}-\d{2}-\d{2}$/.test(item.next_milestone.target_date) && Number.isFinite(Date.parse(item.next_milestone.target_date))));
}

export function ProjectDirectory(props: Props) {
  const {user, organization} = props.session;
  return <PortfolioScope key={JSON.stringify([user.id, organization.id])} {...props}/>;
}

function PortfolioScope({session, q, refresh, onSearch, onCreate, onOpen, onOverview}: Props) {
  useLocale();
  const api = useApi();
  const {user, organization} = session;
  const id = useId();
  const [search, setSearch] = useState(q);
  const [cursors, setCursors] = useState<{q: string; values: string[]}>({q, values: []});
  const [revision, setRevision] = useState(0);
  const [state, setState] = useState<Read>();
  const [searchError, setSearchError] = useState(false);
  const resultHeading = useRef<HTMLHeadingElement>(null);
  const focusResults = useRef(false);
  const lastCheck = useRef(0);
  const inFlight = useRef<AbortController | undefined>(undefined);

  useEffect(() => {
    setSearch(q);
    setSearchError(false);
    setCursors({q, values: []});
  }, [q]);

  const history = cursors.q === q ? cursors.values : [];
  const after = history.at(-1) || '';
  const params = new URLSearchParams({limit: String(pageSize)});
  if (q) params.set('q', q);
  if (after) params.set('after', after);
  const path = '/project-portfolio?' + params;
  const current = state?.path === path && state.api === api ? state : undefined;
  const page = current?.page;
  const loading = !current || current.loading;
  const canCreate = user.role === 'owner' || user.role === 'admin';

  useEffect(() => {
    const abort = new AbortController();
    inFlight.current = abort;
    lastCheck.current = Date.now();
    setState(previous => ({
      path, api, loading: true,
      page: previous?.path === path && previous.api === api ? previous.page : undefined,
    }));
    void api<ProjectPortfolioPage>(path, undefined, 'GET', {
      'X-Suma-Expected-User': user.id,
      'X-Suma-Expected-Organization': organization.id,
    }, abort.signal).then(result => {
      if (abort.signal.aborted) return;
      if (!validPage(result, after)) throw new UiError('project-portfolio.invalid_snapshot');
      setState({path, api, loading: false, page: result});
    }).catch(error => {
      if (!abort.signal.aborted) setState({path, api, loading: false, error});
    }).finally(() => {
      if (inFlight.current === abort) inFlight.current = undefined;
    });
    return () => abort.abort();
  }, [api, path, after, user.id, organization.id, refresh, revision]);

  const reload = useCallback(() => setRevision(value => value + 1), []);
  useEffect(() => {
    const recheck = () => {
      if (document.visibilityState === 'hidden' || navigator.onLine === false || inFlight.current ||
        Date.now() - lastCheck.current < 3000) return;
      reload();
    };
    window.addEventListener('focus', recheck);
    window.addEventListener('online', recheck);
    document.addEventListener('visibilitychange', recheck);
    const timer = window.setInterval(recheck, 30000);
    return () => {
      window.removeEventListener('focus', recheck);
      window.removeEventListener('online', recheck);
      document.removeEventListener('visibilitychange', recheck);
      window.clearInterval(timer);
    };
  }, [reload]);

  useEffect(() => {
    if (!loading && focusResults.current) {
      focusResults.current = false;
      resultHeading.current?.focus();
    }
  }, [loading, path]);

  function submit() {
    const next = search.trim();
    if (new TextEncoder().encode(next).length > 120) {
      setSearchError(true);
      return;
    }
    setSearch(next);
    setSearchError(false);
    if (next === q) return;
    onSearch(next);
  }

  function changePage(values: string[]) {
    focusResults.current = true;
    setCursors({q, values});
  }

  const cursorError = !!after && current?.error instanceof ApiError && [400, 409].includes(current.error.status);

  return <section className="project-portfolio" data-project-directory data-project-portfolio aria-labelledby={id + '-title'}>
    <header className="project-portfolio-heading">
      <div>
        <div className="project-portfolio-org"><FolderKanban size={16} aria-hidden="true"/>{organization.name}</div>
        <h2 id={id + '-title'}>{t('title')}</h2>
        <p>{t('description')}</p>
      </div>
      {canCreate && <button type="button" className="primary" onClick={onCreate}>
        <Plus size={16} aria-hidden="true"/>{tr('shell.create_project')}
      </button>}
    </header>
    <section className="project-portfolio-results panel" aria-labelledby={id + '-results'}>
      <div className="project-portfolio-toolbar">
        <form className="project-portfolio-search" role="search" onSubmit={event => {event.preventDefault(); submit();}}>
          <label htmlFor={id + '-search'}>{t('search')}</label>
          <div className="project-portfolio-search-controls">
            <div className="project-portfolio-search-input">
              <Search size={16} aria-hidden="true"/>
              <input type="search" id={id + '-search'} data-portfolio-search value={search}
                placeholder={t('search_hint')} aria-invalid={searchError || undefined}
                aria-describedby={searchError ? id + '-search-error' : undefined}
                onChange={event => {setSearch(event.target.value); setSearchError(false);}}/>
            </div>
            <button type="submit" data-portfolio-submit>{t('search_action')}</button>
            {(q || search) && <button type="button" className="text-button" data-portfolio-clear onClick={() => {
              setSearch(''); setSearchError(false); if (q) onSearch('');
            }}>{t('clear')}</button>}
          </div>
          {searchError && <p role="alert" id={id + '-search-error'} className="project-portfolio-search-error">{t('search_limit')}</p>}
        </form>
        <button type="button" data-portfolio-refresh disabled={loading} onClick={reload}>
          <RefreshCw size={15} aria-hidden="true"/>{t('refresh')}
        </button>
      </div>
      <div className="project-portfolio-results-heading">
        <h3 id={id + '-results'} ref={resultHeading} tabIndex={-1}>{q ? t('results_for', {query: q}) : t('results')}</h3>
        <div role="status">
          {page && <span>{t('page_count', {count: formatNumber(page.items.length)})}</span>}
          {page && <span>{t('as_of', {time: formatDateTime(page.as_of, {dateStyle: 'short', timeStyle: 'medium'})})}</span>}
          {page && loading && <span>{t('checking')}</span>}
        </div>
      </div>
      {Boolean(current?.error) && <div className="project-portfolio-feedback">
        <ErrorNotice error={cursorError ? new UiError('project-portfolio.page_changed') : current?.error}
          onRetry={cursorError ? undefined : reload}/>
      </div>}
      {!page && loading && <Loading/>}
      {page?.items.length === 0 && <div data-portfolio-empty><Empty
        title={q ? t('no_matches') : after ? t('empty_page') : t('empty')}
        description={after ? t('empty_page_hint') : q ? t('no_matches_hint') : t('empty_hint')}/>
      </div>}
      {!!page?.items.length && <ul className="project-portfolio-list" aria-busy={loading}>
        {page.items.map(item => <ProjectRow key={item.project_id} item={item} asOf={page.as_of}
          onOverview={onOverview} onOpen={onOpen}/>)}
      </ul>}
      {(after || page?.next_cursor) && <nav aria-label={t('pagination')} className="project-portfolio-pagination">
        <button type="button" data-portfolio-previous disabled={loading || !after}
          onClick={() => changePage(history.slice(0, -1))}><ChevronLeft size={15} aria-hidden="true"/>{t('previous')}</button>
        <span>{t('page_number', {page: formatNumber(history.length + 1)})}</span>
        {cursorError
          ? <button type="button" data-portfolio-first onClick={() => changePage([])}>{t('first_page')}</button>
          : <button type="button" data-portfolio-next disabled={loading || !page?.next_cursor}
            onClick={() => {if (page?.next_cursor) changePage([...history, page.next_cursor]);}}>
            {t('next')}<ChevronRight size={15} aria-hidden="true"/>
          </button>}
      </nav>}
    </section>
  </section>;
}

function ProjectRow({item, asOf, onOpen, onOverview}: {
  item: ProjectPortfolioItem;
  asOf: string;
  onOpen: (id: string) => void;
  onOverview: (id: string) => void;
}) {
  const {project_id: id, title, description, status, task_total: total, accepted_tasks: accepted,
    blocked_tasks: blocked, active_risks: risks, next_milestone: milestone} = item;
  const overdue = milestone && milestone.target_date < new Date(asOf).toISOString().slice(0, 10);
  const name = title || id;
  return <li className="project-portfolio-row" data-portfolio-project={id}>
    <div className="project-portfolio-name">
      <div className="project-portfolio-name-line">
        <h4><button type="button" className="text-button" data-portfolio-overview
          aria-label={t('open_overview', {project: name})}
          onClick={() => onOverview(id)}>{name}<ArrowRight size={15} aria-hidden="true"/></button></h4>
        {status && <Badge state={status}/>}
      </div>
      {description && <p>{description}</p>}
    </div>
    <div className="project-portfolio-progress">
      <span className="project-portfolio-field-label">{t('accepted')}</span>
      {total > 0 ? <>
        <strong>{t('accepted_count', {accepted: formatNumber(accepted), total: formatNumber(total)})}</strong>
        <progress value={accepted} max={total} aria-label={t('accepted_progress', {project: name})}/>
      </> : <span className="project-portfolio-muted">{t('no_tasks')}</span>}
    </div>
    <dl className="project-portfolio-signals">
      <div data-portfolio-blocked={blocked}><dt>{t('blocked')}</dt><dd>{formatNumber(blocked)}</dd></div>
      <div data-portfolio-risks={risks}><dt>{t('active_risks')}</dt><dd>{formatNumber(risks)}</dd></div>
    </dl>
    <div className="project-portfolio-milestone">
      <span className="project-portfolio-field-label"><CalendarDays size={14} aria-hidden="true"/>{t('milestone')}</span>
      {milestone ? <>
        <strong>{milestone.title}</strong>
        <time dateTime={milestone.target_date}>{dateOnly(milestone.target_date)}</time>
        {overdue && <span className="project-portfolio-overdue">{t('overdue')}</span>}
      </> : <span className="project-portfolio-muted">{t('no_milestone')}</span>}
    </div>
    <button type="button" className="project-portfolio-settings" data-portfolio-settings
      aria-label={t('open_settings', {project: name})} onClick={() => onOpen(id)}>
      <Settings2 size={15} aria-hidden="true"/>{t('settings')}
    </button>
  </li>;
}
