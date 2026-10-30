import {useCallback, useEffect, useId, useLayoutEffect, useRef, useState} from 'react';
import {Search, UserPlus, X} from 'lucide-react';
import type {TaskConversationMentionTarget, TaskConversationMentionTargetPage} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi} from './api-context';
import {formatNumber, tr, UiError, useLocale} from './i18n';
import {ErrorNotice, Loading} from './ui';
import {mentionLimit, mentionTargetsPath, validMentionQuery, validMentionTargetPage} from './task-conversation-mentions-model';
import './TaskConversationMentions.css';

export type TaskConversationMentionsProps = {project: string; taskID: string; userID: string; organizationID: string;
  selectedIDs: string[]; onChange: (ids: string[]) => void; disabled: boolean; accessBlocked: boolean; refresh: number;
  onValidityChange: (key: string, valid: boolean) => void; onAccessDenied: () => void};
export const mentionSelectionKey = (ids: string[], refresh: number) => JSON.stringify([refresh, ids]);
const t = (key: string, values?: Record<string, unknown>) => tr('task-conversation-mentions.' + key, values);
const refused = (error: unknown) => error instanceof ApiError && [401, 403, 404].includes(error.status);

/** Names come only from this task's current directory. Neither a plain @name
 * nor a previously cached name can choose a notification recipient. */
export function TaskConversationMentions(props: TaskConversationMentionsProps) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  return <MentionsScope key={JSON.stringify([props.userID, props.organizationID, props.project, props.taskID, caller.current.epoch])} {...props}/>;
}
function MentionsScope({project, taskID, selectedIDs, onChange, disabled, accessBlocked, refresh, onValidityChange, onAccessDenied}: TaskConversationMentionsProps) {
  useLocale(); const api = useApi(), id = useId();
  const [expanded, setExpanded] = useState(false), [search, setSearch] = useState(''), [q, setQ] = useState(''), [queryError, setQueryError] = useState(false);
  const [page, setPage] = useState<TaskConversationMentionTargetPage>(), [loading, setLoading] = useState(false), [error, setError] = useState<unknown>();
  const [hydrated, setHydrated] = useState<{key: string; items: TaskConversationMentionTarget[]}>(), [hydrateError, setHydrateError] = useState<unknown>();
  const [denied, setDenied] = useState(false), [hydrateRetry, setHydrateRetry] = useState(0);
  const alive = useRef(true), catalogEpoch = useRef(0), hydrationEpoch = useRef(0);
  const catalogAbort = useRef<AbortController | undefined>(undefined), hydrationAbort = useRef<AbortController | undefined>(undefined);
  const selectionKey = mentionSelectionKey(selectedIDs, refresh), blocked = accessBlocked || denied;
  const current = useRef({selectionKey, blocked}); current.current = {selectionKey, blocked};
  const loaded = useRef<TaskConversationMentionTargetPage | undefined>(undefined);
  const deny = useCallback((failure: unknown) => {
    if (!alive.current) return;
    catalogAbort.current?.abort(); hydrationAbort.current?.abort(); loaded.current = undefined;
    setPage(undefined); setHydrated(undefined); setError(failure); setDenied(true); onAccessDenied();
  }, [onAccessDenied]);
  useEffect(() => { alive.current = true; return () => { alive.current = false; catalogAbort.current?.abort(); hydrationAbort.current?.abort(); }; }, []);
  const currentTargets = !blocked && hydrated?.key === selectionKey ? hydrated.items : [];
  const valid = !selectedIDs.length || !blocked && hydrated?.key === selectionKey && currentTargets.length === selectedIDs.length;
  useLayoutEffect(() => { onValidityChange(selectionKey, !!valid); }, [selectionKey, valid, onValidityChange]);

  useEffect(() => {
    setHydrated(undefined); setHydrateError(undefined);
    if (blocked || !selectedIDs.length) return;
    const controller = new AbortController(), generation = ++hydrationEpoch.current;
    hydrationAbort.current?.abort(); hydrationAbort.current = controller;
    void api<unknown>(mentionTargetsPath(project, taskID, '', '', selectedIDs), undefined, 'GET', undefined, controller.signal)
      .then(value => {
        if (!alive.current || controller.signal.aborted || generation !== hydrationEpoch.current || current.current.selectionKey !== selectionKey || current.current.blocked) return;
        if (!validMentionTargetPage(value, project, taskID, selectedIDs)) throw new UiError('task-conversation-mentions.invalid_read');
        setHydrated({key: selectionKey, items: value.items});
      }).catch(failure => {
        if (!alive.current) return;
        if (refused(failure)) { deny(failure); return; }
        if (!controller.signal.aborted && generation === hydrationEpoch.current) setHydrateError(failure);
      });
    return () => controller.abort();
  }, [api, project, taskID, selectionKey, blocked, hydrateRetry, deny]);

  const read = useCallback(async (more = false) => {
    if (blocked || !expanded) return;
    const previous = more ? loaded.current : undefined, cursor = previous?.next_cursor || '';
    if (more && !cursor) return;
    const controller = new AbortController(), generation = ++catalogEpoch.current;
    catalogAbort.current?.abort(); catalogAbort.current = controller;
    if (!more) { loaded.current = undefined; setPage(undefined); }
    setLoading(true); setError(undefined);
    try {
      const value = await api<unknown>(mentionTargetsPath(project, taskID, q, cursor), undefined, 'GET', undefined, controller.signal);
      if (!alive.current || controller.signal.aborted || generation !== catalogEpoch.current || current.current.blocked) return;
      if (!validMentionTargetPage(value, project, taskID)) throw new UiError('task-conversation-mentions.invalid_read');
      const items = [...(previous?.items || []), ...value.items];
      if (new Set(items.map(row => row.resource_id)).size !== items.length || new Set(items.map(row => row.user_id)).size !== items.length ||
        items.length > value.total || more && (value.next_cursor === cursor || !value.items.length)) throw new UiError('task-conversation-mentions.changed');
      const next = {...value, items}; loaded.current = next; setPage(next);
    } catch (failure) {
      if (!alive.current) return;
      if (refused(failure)) { deny(failure); return; }
      if (!controller.signal.aborted && generation === catalogEpoch.current) { loaded.current = undefined; setPage(undefined); setError(failure); }
    } finally { if (alive.current && generation === catalogEpoch.current) setLoading(false); }
  }, [api, project, taskID, q, blocked, expanded, deny]);
  useEffect(() => { loaded.current = undefined; setPage(undefined); if (!blocked) void read(); return () => catalogAbort.current?.abort(); }, [read, refresh]);

  if (blocked) return null;
  return <div className="task-conversation-mentions" aria-label={t('title')}>
    <div className="task-conversation-mentions-heading"><button type="button" aria-expanded={expanded} aria-controls={id + '-picker'} disabled={disabled} onClick={() => setExpanded(value => !value)} data-mention-toggle><UserPlus size={14} aria-hidden="true"/>{t('title')}<span>{formatNumber(selectedIDs.length)}/{mentionLimit}</span></button><small>{t('hint')}</small></div>
    {!!selectedIDs.length && <ul className="task-conversation-mentions-selected" aria-label={t('selected')}>
      {selectedIDs.map(resourceID => { const person = currentTargets.find(row => row.resource_id === resourceID); return <li key={resourceID} data-mention-selected={resourceID} className={person ? '' : 'is-unavailable'}><span>{person?.name || (person ? resourceID : hydrated?.key === selectionKey ? t('unavailable', {id: resourceID}) : t('checking', {id: resourceID}))}</span><button type="button" className="icon-button" disabled={disabled} aria-label={t('remove', {name: person?.name || resourceID})} onClick={() => onChange(selectedIDs.filter(value => value !== resourceID))}><X size={13} aria-hidden="true"/></button></li>; })}
    </ul>}
    {!!hydrateError && <ErrorNotice error={hydrateError} onRetry={() => setHydrateRetry(value => value + 1)}/>}
    {selectedIDs.length > 0 && !valid && <small className="task-conversation-mentions-warning" role="status">{t('verify_required')}</small>}
    {expanded && <div id={id + '-picker'} className="task-conversation-mentions-picker">
      <label htmlFor={id + '-search'}>{t('search_label')}</label><div className="task-conversation-mentions-search"><input id={id + '-search'} type="search" value={search} disabled={disabled} maxLength={400} onChange={event => setSearch(event.target.value)} onKeyDown={event => {if (event.key === 'Enter') {event.preventDefault(); if (validMentionQuery(search)) {setQ(search); setQueryError(false); if (search === q) void read();} else setQueryError(true);}}}/><button type="button" disabled={disabled || loading} onClick={() => {if (!validMentionQuery(search)) {setQueryError(true); return;} setQ(search); setQueryError(false); if (search === q) void read();}}><Search size={14} aria-hidden="true"/>{t('search')}</button></div>
      {queryError && <p role="alert" className="task-conversation-mentions-warning">{t('invalid_query')}</p>}
      {loading && <Loading/>}{!!error && <ErrorNotice error={error} onRetry={() => void read()}/>}
      {page && <><p className="task-conversation-hint">{t('showing', {loaded: formatNumber(page.items.length), total: formatNumber(page.total)})}</p><ul className="task-conversation-mentions-options" aria-label={t('people')}>
        {page.items.map(person => <li key={person.resource_id}><button type="button" disabled={disabled || !selectedIDs.includes(person.resource_id) && selectedIDs.length >= mentionLimit} aria-pressed={selectedIDs.includes(person.resource_id)} data-mention-option={person.resource_id} onClick={() => onChange(selectedIDs.includes(person.resource_id) ? selectedIDs.filter(value => value !== person.resource_id) : [...selectedIDs, person.resource_id])}><span>{person.name || person.resource_id}</span><small>{person.resource_id}</small></button></li>)}
      </ul>{!page.items.length && <p className="task-conversation-hint">{t('empty')}</p>}{page.next_cursor && <button type="button" disabled={loading || disabled} onClick={() => void read(true)} data-mention-more>{t('more')}</button>}</>}
    </div>}
  </div>;
}
