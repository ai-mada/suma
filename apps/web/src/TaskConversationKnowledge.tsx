import {useCallback, useEffect, useId, useLayoutEffect, useRef, useState} from 'react';
import {BookOpen, Search, X} from 'lucide-react';
import type {TaskConversationKnowledgeReference} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi} from './api-context';
import {formatNumber, tr, UiError, useLocale} from './i18n';
import {ErrorNotice, Loading, Modal} from './ui';
import {KnowledgeRevisionFacts} from './KnowledgeFacts';
import {useKnowledgeEntries} from './knowledge-hooks';
import {publishedKnowledgeReference, type KnowledgeReference, type KnowledgeRevision, type KnowledgeSummary} from './knowledge-model';
import {conversationKnowledgeLimit, currentKnowledgePin, knowledgeDetailPath, knowledgeRevisionPath, knowledgeSelectionKey,
  sameKnowledgePin, validKnowledgeRevisionDetail, validKnowledgeSummary} from './task-conversation-knowledge-model';
import './TaskConversationKnowledge.css';

const t = (key: string, values?: Record<string, unknown>) => tr('task-conversation-knowledge.' + key, values);
const refused = (error: unknown) => error instanceof ApiError && [401, 403].includes(error.status);
const queryValid = (value: string) => Array.from(value).length <= 200 && !/[\p{Cc}\uD800-\uDFFF]/u.test(value);
type Props = {project: string; taskID: string; userID: string; organizationID: string; selected: KnowledgeReference[];
  onChange: (pins: KnowledgeReference[]) => void; disabled: boolean; accessBlocked: boolean; refresh: number;
  onValidityChange: (key: string, valid: boolean) => void; onAccessDenied: () => void};

/** Owner-authored pins survive separately from current catalog labels. Every
 * new post needs current publication eligibility; history needs only read access. */
export function TaskConversationKnowledge(props: Props) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  return <KnowledgePickerScope key={JSON.stringify([props.userID, props.organizationID, props.project, props.taskID, caller.current.epoch])} {...props}/>;
}
function KnowledgePickerScope({project, selected, onChange, disabled, accessBlocked, refresh, onValidityChange, onAccessDenied}: Props) {
  useLocale(); const api = useApi(), id = useId();
  const [expanded, setExpanded] = useState(false), [search, setSearch] = useState(''), [q, setQ] = useState('');
  const [queryError, setQueryError] = useState(false), [conflict, setConflict] = useState(false), [denied, setDenied] = useState(false);
  const [hydrated, setHydrated] = useState<{key: string; items: KnowledgeSummary[]}>(), [error, setError] = useState<unknown>(), [retry, setRetry] = useState(0);
  const alive = useRef(true), epoch = useRef(0), controller = useRef<AbortController | undefined>(undefined);
  const selectionKey = knowledgeSelectionKey(selected, refresh), blocked = accessBlocked || denied;
  const list = useKnowledgeEntries(project, {kind: 'all', state: 'published', q}, refresh, expanded && !blocked);
  const listDenied = refused(list.error), hidden = blocked || listDenied;
  const current = useRef({selectionKey, hidden}); current.current = {selectionKey, hidden};
  const deny = useCallback(() => {if (!alive.current) return; controller.current?.abort(); setHydrated(undefined); setDenied(true); onAccessDenied();}, [onAccessDenied]);
  useEffect(() => {alive.current = true; return () => {alive.current = false; controller.current?.abort();};}, []);
  useLayoutEffect(() => {if (listDenied) deny();}, [listDenied, deny]);
  const items = !hidden && hydrated?.key === selectionKey ? hydrated.items : [];
  const ready = !selected.length || !hidden && hydrated?.key === selectionKey && items.length === selected.length;
  useLayoutEffect(() => {onValidityChange(selectionKey, !!ready);}, [selectionKey, ready, onValidityChange]);
  useEffect(() => {
    setHydrated(undefined); setError(undefined); setConflict(false);
    if (hidden || !selected.length) return;
    const abort = new AbortController(), generation = ++epoch.current;
    controller.current?.abort(); controller.current = abort;
    // At most three exact-ID reads in flight; do not walk the catalog to hydrate
    // pins absent from the current search page. A missing pin remains removable.
    const rows: KnowledgeSummary[] = []; let next = 0, failure: unknown;
    const active = () => alive.current && !abort.signal.aborted && generation === epoch.current && current.current.selectionKey === selectionKey && !current.current.hidden;
    async function worker() {
      while (active() && next < selected.length) {
        const pin = selected[next++];
        try {
          const value = await api<unknown>(knowledgeDetailPath(project, pin.id), undefined, 'GET', undefined, abort.signal);
          if (!active()) return;
          if (!validKnowledgeSummary(value, project) || value.id !== pin.id) throw new UiError('task-conversation-knowledge.invalid_read');
          if (currentKnowledgePin(value, project, pin)) rows.push(value);
        } catch (reason) {
          if (!active()) return;
          if (refused(reason)) {deny(); return;}
          if (!(reason instanceof ApiError && reason.status === 404)) failure = reason;
        }
      }
    }
    void Promise.all(Array.from({length: Math.min(3, selected.length)}, worker)).then(() => {
      if (!active()) return;
      if (failure) setError(failure);
      else setHydrated({key: selectionKey, items: rows});
    });
    return () => abort.abort();
  }, [api, project, selectionKey, hidden, retry, deny]);

  const listValid = !list.items || list.items.every(item => validKnowledgeSummary(item, project)) && list.items.length <= list.total &&
    new Set(list.items.map(item => item.id)).size === list.items.length && typeof list.cursor === 'string' && list.cursor.length <= 4096 && !/[\p{Cc}]/u.test(list.cursor);
  function choose(entry: KnowledgeSummary) {
    const pin = publishedKnowledgeReference(entry, project), existing = selected.find(item => item.id === pin.id);
    if (existing && !sameKnowledgePin(existing, pin)) {setConflict(true); return;}
    setConflict(false);
    onChange(existing ? selected.filter(item => item.id !== pin.id) : [...selected, pin]);
  }
  function searchNow() {if (!queryValid(search)) {setQueryError(true); return;} setQueryError(false); setQ(search); if (q === search) void list.read();}
  if (hidden) return null;
  return <div className="conversation-knowledge" aria-label={t('title')}>
    <div className="conversation-knowledge-heading"><button type="button" data-knowledge-toggle aria-expanded={expanded} aria-controls={id} disabled={disabled} onClick={() => setExpanded(value => !value)}><BookOpen size={14} aria-hidden="true"/>{t('title')}<span>{formatNumber(selected.length)}/{conversationKnowledgeLimit}</span></button><small>{t('hint')}</small></div>
    {!!selected.length && <ul className="conversation-knowledge-selected" aria-label={t('selected')}>
      {selected.map(pin => {const entry = items.find(item => item.id === pin.id); return <li key={pin.id} data-knowledge-selected={pin.id} className={entry ? '' : 'is-unavailable'}>
        <span><strong>{entry?.published_title || t(hydrated?.key === selectionKey ? 'unavailable' : 'checking', {id: pin.id})}</strong><small>{t('revision', {revision: formatNumber(pin.revision)})} · <code>{pin.id}</code></small></span>
        <button type="button" data-knowledge-remove className="icon-button" disabled={disabled} aria-label={t('remove', {name: entry?.published_title || pin.id})} onClick={() => onChange(selected.filter(item => item.id !== pin.id))}><X size={13} aria-hidden="true"/></button>
      </li>;})}
    </ul>}
    {!!error && <ErrorNotice error={error} onRetry={() => setRetry(value => value + 1)}/>}
    {!!selected.length && !ready && <p className="conversation-knowledge-warning" role="status">{t('verify_required')}</p>}
    {conflict && <p className="conversation-knowledge-warning" role="alert">{t('conflict')}</p>}
    {expanded && <div id={id} className="conversation-knowledge-picker">
      <label htmlFor={id + '-search'}>{t('search_label')}</label><div className="conversation-knowledge-search"><input id={id + '-search'} type="search" value={search} maxLength={400} disabled={disabled} onChange={event => setSearch(event.target.value)} onKeyDown={event => {if (event.key === 'Enter') {event.preventDefault();searchNow();}}}/><button type="button" disabled={disabled || list.loading} onClick={searchNow}><Search size={14} aria-hidden="true"/>{t('search')}</button></div>
      {queryError && <p className="conversation-knowledge-warning" role="alert">{t('invalid_query')}</p>}
      {list.loading && <Loading/>}{!!list.error && <ErrorNotice error={list.error} onRetry={() => void list.read()}/>}
      {!listValid && <ErrorNotice error={new UiError('task-conversation-knowledge.invalid_read')} onRetry={() => void list.read()}/>}
      {listValid && list.items && <><p className="task-conversation-hint">{t('showing', {loaded: formatNumber(list.items.length), total: formatNumber(list.total)})}</p><ul className="conversation-knowledge-options" aria-label={t('options')}>
        {list.items.map(entry => {let eligible = true;try {publishedKnowledgeReference(entry, project);} catch {eligible = false;} const chosen = selected.find(pin => pin.id === entry.id); return <li key={entry.id}><button type="button" data-knowledge-option={entry.id} disabled={disabled || !eligible || !chosen && selected.length >= conversationKnowledgeLimit} aria-pressed={!!chosen} onClick={() => choose(entry)}><span><strong>{entry.published_title || entry.id}</strong><small>{tr('knowledge.kind.' + entry.kind)} · {t('revision', {revision: formatNumber(entry.published_revision)})}</small></span>{entry.published_summary && <small>{entry.published_summary}</small>}</button></li>;})}
      </ul>{!list.items.length && <p className="task-conversation-hint">{t('empty')}</p>}{list.cursor && <button type="button" disabled={disabled || list.loading} data-knowledge-more onClick={() => void list.read(true)}>{t('more')}</button>}</>}
    </div>}
  </div>;
}

/** The recorded title is immutable metadata. Opening it never adopts a newer
 * publication or grants permission to consume the historical revision again. */
type RecordedProps = {project: string; items?: TaskConversationKnowledgeReference[]; refresh: number; onAccessDenied: () => void};
export function RecordedConversationKnowledge(props: RecordedProps) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  return <RecordedKnowledgeScope key={JSON.stringify([props.project, caller.current.epoch])} {...props}/>;
}
function RecordedKnowledgeScope({project, items, refresh, onAccessDenied}: RecordedProps) {
  useLocale(); const api = useApi();
  const [selected, setSelected] = useState<TaskConversationKnowledgeReference>();
  const [state, setState] = useState<{key: string; revision?: KnowledgeRevision; error?: unknown}>();
  const [retry, setRetry] = useState(0), [denied, setDenied] = useState(false), alive = useRef(true);
  const key = JSON.stringify([project, selected, refresh]), current = useRef(key); current.current = key;
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  useEffect(() => {
    if (!selected) return;
    const abort = new AbortController();setState(undefined);
    void api<unknown>(knowledgeRevisionPath(project, selected), undefined, 'GET', undefined, abort.signal).then(value => {
      if (!alive.current || abort.signal.aborted || current.current !== key) return;
      if (!validKnowledgeRevisionDetail(value, project, selected)) throw new UiError('task-conversation-knowledge.invalid_read');
      setState({key, revision: value.snapshot});
    }).catch(error => {
      if (!alive.current || abort.signal.aborted || current.current !== key) return;
      setState({key, error});if (refused(error)) {setSelected(undefined);setDenied(true);onAccessDenied();}
    });
    return () => abort.abort();
  }, [api, project, key, retry, onAccessDenied]);
  if (denied || !items?.length) return null;
  const value = state?.key === key ? state : undefined;
  return <div className="conversation-knowledge-recorded"><small>{t('recorded')}</small><ul aria-label={t('recorded')}>
    {items.map(pin => <li key={pin.id}><button type="button" data-knowledge-history={pin.id} onClick={() => setSelected(pin)}><BookOpen size={13} aria-hidden="true"/><span>{pin.title_at_post || pin.id} · {tr('knowledge.kind.' + pin.kind_at_post)} · {t('revision', {revision: formatNumber(pin.revision)})}</span></button></li>)}
  </ul><Modal open={!!selected} onClose={() => {setSelected(undefined);setState(undefined);}} title={selected?.title_at_post || t('history')} description={t('history_hint')}>
    {selected && <><p>{!value?.revision && <>{t('revision', {revision: formatNumber(selected.revision)})} · </>}<code>{selected.id}</code></p>{!value?.revision && <code className="knowledge-hash">{selected.sha256}</code>}
      {!value && <Loading/>}{!!value?.error && <ErrorNotice error={value.error} onRetry={() => setRetry(number => number + 1)}/>}
      {value?.revision && <div onClickCapture={event => {if (event.target instanceof Element && event.target.closest('a[href]') && !window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}}><KnowledgeRevisionFacts project={project} revision={value.revision}/></div>}</>}
  </Modal></div>;
}
