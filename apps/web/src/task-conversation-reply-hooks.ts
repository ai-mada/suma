import {useCallback, useEffect, useRef, useState} from 'react';
import {ApiError, type ApiCaller} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';

type ReadState<T> = {api: ApiCaller; path: string; value?: T; loading: boolean; error?: unknown};
const denied = (error: unknown) => error instanceof ApiError && [401, 403, 404].includes(error.status);

/** Current-owner reads preserve a confirmed view during transient refresh,
 * but revoke it immediately after access loss. No timer or graph-wide polling. */
export function useReplyRead<T>(path: string, enabled: boolean, validate: (value: unknown) => value is T,
  onDenied: () => void, refresh = 0) {
  const api = useApi(), [snapshot, setSnapshot] = useState<ReadState<T>>();
  const epoch = useRef(0), controller = useRef<AbortController | undefined>(undefined);
  const owner = useRef({api, path}), validator = useRef(validate), refusal = useRef(onDenied);
  owner.current = {api, path}; validator.current = validate; refusal.current = onDenied;
  const read = useCallback(async () => {
    if (!enabled || !path) return;
    const generation = ++epoch.current, abort = new AbortController();
    controller.current?.abort(); controller.current = abort;
    const live = () => !abort.signal.aborted && generation === epoch.current && owner.current.api === api && owner.current.path === path;
    setSnapshot(previous => ({api, path, value: previous?.api === api && previous.path === path ? previous.value : undefined, loading: true}));
    try {
      const value = await api<unknown>(path, undefined, 'GET', undefined, abort.signal);
      if (!live()) return;
      if (!validator.current(value)) throw new UiError('task-reply.invalid_read');
      setSnapshot({api, path, value, loading: false}); return value;
    } catch (error) {
      if (!live()) return;
      setSnapshot(previous => ({api, path, value: denied(error) ? undefined : previous?.value, loading: false, error}));
      if (denied(error)) refusal.current();
    }
  }, [api, path, enabled]);
  useEffect(() => {if (enabled) void read(); else setSnapshot(undefined);
    return () => {epoch.current++; controller.current?.abort();};}, [read, refresh]);
  const visible = enabled && snapshot?.api === api && snapshot.path === path ? snapshot : undefined;
  return {...visible, read};
}

type Page<T> = {items: T[]; total: number; next_cursor?: string};
type PageState<T> = {api: ApiCaller; path: string; value?: Page<T>; loading: boolean; error?: unknown};

/** Two inventories use explicit paging: authorized reply history and profiles.
 * Their cursors never cross a task, project, account or API owner boundary. */
export function useReplyCollection<T extends {id: string}>(base: string, cursorParameter: 'cursor', enabled: boolean,
  validate: (value: unknown) => value is Page<T>, onDenied: () => void, refresh = 0) {
  const api = useApi(), [snapshot, setSnapshot] = useState<PageState<T>>();
  const current = useRef<PageState<T> | undefined>(undefined), epoch = useRef(0);
  const controller = useRef<AbortController | undefined>(undefined), owner = useRef({api, base});
  const validator = useRef(validate), refusal = useRef(onDenied);
  owner.current = {api, base}; validator.current = validate; refusal.current = onDenied;
  const read = useCallback(async (more = false) => {
    if (!enabled) return;
    const previous = current.current?.api === api && current.current.path === base ? current.current.value : undefined;
    const cursor = more ? previous?.next_cursor : '';
    if (more && !cursor) return previous;
    const generation = ++epoch.current, abort = new AbortController();
    controller.current?.abort(); controller.current = abort;
    const live = () => !abort.signal.aborted && generation === epoch.current && owner.current.api === api && owner.current.base === base;
    const pending = {api, path: base, value: more ? previous : undefined, loading: true};
    current.current = pending; setSnapshot(pending);
    try {
      const path = base + (cursor ? '&' + cursorParameter + '=' + encodeURIComponent(cursor) : '');
      const value = await api<unknown>(path, undefined, 'GET', undefined, abort.signal);
      if (!live()) return;
      if (!validator.current(value) || cursor && value.next_cursor === cursor) throw new UiError('task-reply.invalid_read');
      const items = more && previous ? [...new Map([...previous.items, ...value.items].map(item => [item.id, item])).values()] : value.items;
      if (items.length > value.total) throw new UiError('task-reply.invalid_read');
      const next = {api, path: base, value: {...value, items}, loading: false};
      current.current = next; setSnapshot(next); return next.value;
    } catch (error) {
      if (!live()) return;
      const next = {api, path: base, value: denied(error) ? undefined : previous, loading: false, error};
      current.current = next; setSnapshot(next);
      if (denied(error)) refusal.current();
    }
  }, [api, base, cursorParameter, enabled]);
  useEffect(() => {current.current = undefined; setSnapshot(undefined); if (enabled) void read();
    return () => {epoch.current++; controller.current?.abort();};}, [read, refresh]);
  const visible = enabled && snapshot?.api === api && snapshot.path === base ? snapshot : undefined;
  return {...visible, read};
}
