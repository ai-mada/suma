import {useCallback, useEffect, useRef, useState} from 'react';
import type {TaskConversationMessageDetail, TaskConversationPage} from '../../../packages/contracts/src';
import {ApiError, query, type ApiCaller} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {validConversationMessageDetail, validConversationPage} from './task-conversation-model';

type Snapshot = {api: ApiCaller; scope: string; page?: TaskConversationPage;
  error?: unknown; loading: boolean; denied?: boolean};

/** Fetch on demand only. A closed conversation has no polling/stream cost.
 * A late response cannot expose an old account, project or task's messages. */
export function useTaskConversationRead(project: string, task: string, enabled: boolean, refresh: number) {
  const api = useApi(), scope = JSON.stringify([project, task]);
  const [snapshot, setSnapshot] = useState<Snapshot>();
  const current = useRef<Snapshot | undefined>(undefined);
  const owner = useRef({api, scope, enabled});
  owner.current = {api, scope, enabled};
  const epoch = useRef(0), abort = useRef<AbortController | undefined>(undefined);

  const read = useCallback(async (older = false): Promise<TaskConversationPage | undefined> => {
    if (!enabled) return;
    const previous = current.current?.api === api && current.current.scope === scope
      ? current.current.page : undefined;
    const before = older ? previous?.next_before ?? 0 : 0;
    if (older && !before) return previous;
    const generation = ++epoch.current, controller = new AbortController();
    abort.current?.abort(); abort.current = controller;
    const live = () => !controller.signal.aborted && generation === epoch.current &&
      owner.current.api === api && owner.current.scope === scope && owner.current.enabled;
    const pending: Snapshot = {api, scope, page: older ? previous : undefined, loading: true};
    current.current = pending; setSnapshot(pending);
    try {
      const path = '/task-conversations' + query(project) + '&task_id=' + encodeURIComponent(task) +
        '&limit=20' + (before ? '&before=' + before : '');
      const response = await api<unknown>(path, undefined, 'GET', undefined, controller.signal);
      if (!live()) return;
      if (!validConversationPage(response, project, task, before))
        throw new UiError('task-conversation.invalid_read');
      // More history is prepended, never appended in reverse chronological order.
      // Immutable message IDs and sequence identities may not disagree.
      let page = response;
      if (older && previous) {
        const all = [...response.items, ...previous.items];
        if (new Set(all.map(item => item.id)).size !== all.length ||
          new Set(all.map(item => item.sequence)).size !== all.length || all.length > response.total)
          throw new UiError('task-conversation.invalid_read');
        page = {...response, items: all};
      }
      const next = {api, scope, page, loading: false};
      current.current = next; setSnapshot(next); return page;
    } catch (error) {
      if (!live()) return;
      const denied = error instanceof ApiError && [401, 403, 404].includes(error.status);
      const next = {api, scope, error, denied, loading: false};
      current.current = next; setSnapshot(next);
    }
  }, [api, scope, project, task, enabled]);

  useEffect(() => {
    current.current = undefined; setSnapshot(undefined);
    if (enabled) void read();
    return () => { epoch.current++; abort.current?.abort(); };
  }, [read, refresh]);

  const visible = snapshot?.api === api && snapshot.scope === scope && enabled ? snapshot : undefined;
  return {...visible, read};
}

/** One exact source read, independent of the bounded recent window. Never
 * walk older pages to locate a notification, and never mutate read state. */
export function useTaskConversationMessage(project: string, task: string, message: string, enabled: boolean, refresh: number) {
  const api = useApi(), key = JSON.stringify([project, task, message, refresh]);
  const [state, setState] = useState<{api: ApiCaller; key: string; value?: TaskConversationMessageDetail; error?: unknown; denied?: boolean; loading: boolean}>();
  const owner = useRef({api, key, enabled}); owner.current = {api, key, enabled};
  const epoch = useRef(0), abort = useRef<AbortController | undefined>(undefined);
  const read = useCallback(async () => {
    if (!enabled || !message) return;
    const controller = new AbortController(), generation = ++epoch.current;
    abort.current?.abort(); abort.current = controller;
    const live = () => !controller.signal.aborted && generation === epoch.current && owner.current.api === api && owner.current.key === key && owner.current.enabled;
    setState({api, key, loading: true});
    try {
      const value = await api<unknown>('/task-conversation-messages/' + encodeURIComponent(message) + query(project) + '&task_id=' + encodeURIComponent(task), undefined, 'GET', undefined, controller.signal);
      if (!live()) return;
      if (!validConversationMessageDetail(value, project, task, message)) throw new UiError('task-conversation-mentions.invalid_message');
      setState({api, key, value, loading: false});
    } catch (error) {
      if (!live()) return;
      setState({api, key, error, denied: error instanceof ApiError && [401, 403, 404].includes(error.status), loading: false});
    }
  }, [api, key, enabled, message, project, task]);
  useEffect(() => { setState(undefined); if (enabled && message) void read(); return () => { epoch.current++; abort.current?.abort(); }; }, [read]);
  return {...(enabled && state?.api === api && state.key === key ? state : undefined), read};
}
