import { useEffect, useRef, useState } from 'react';
import { ChevronRight, RefreshCw } from 'lucide-react';
import type { Workflow } from './api';
import { ApiError, query } from './api';
import { useApi, useApiView } from './api-context';
import { formatNumber, tr, useLocale } from './i18n';
import type { ToolProfile } from './tool-model';
import { toolReason } from './ToolFacts';
import { associatedDispatchTests, readTestDispatchProfile, testDispatchBlocker, testDispatchProfile, type TaskDispatchTestSelection } from './task-dispatch-test-bindings-model';
import './TaskDispatchTestBindings.css';

export type TestBindingsChoice = { ready: boolean; hasIntent: boolean; selections: TaskDispatchTestSelection[] };
type Inventory = { items?: ToolProfile[]; total: number; loading: boolean; error: string; next_cursor: string; read: (more?: boolean) => Promise<unknown> };
type Props = { project: string; workflow: Workflow; agentID: string; developer: ToolProfile; scopeKey: string; inventory: Inventory; disabled?: boolean; onChange: (value: TestBindingsChoice) => void };
type Choice = { expected: ToolProfile; phase: 'reading' | 'ready' | 'changed' | 'unknown' | 'forbidden'; profile?: ToolProfile };
const t = (key: string, values?: Record<string, unknown>) => tr('dispatch-test-bindings.' + key, values);

/** One shared directory supplies display candidates. Each explicit choice uses
 * the current scoped API to verify that exact version before adoption. */
export function TaskDispatchTestBindings(props: Props) {
  const api = useApi(), view = useApiView();
  const key = JSON.stringify([view?.userID, view?.organizationID, props.project, props.scopeKey]);
  const owner = useRef({ api, key, epoch: 0 });
  if (owner.current.api !== api || owner.current.key !== key) owner.current = { api, key, epoch: owner.current.epoch + 1 };
  const current = owner.current;
  return <BindingsScope key={current.epoch} {...props} disabled={props.disabled || !view?.userID || !view.organizationID}
    owns={() => owner.current === current} />;
}
function BindingsScope({ project, workflow, agentID, developer, inventory, disabled = false, onChange, owns }: Props & { owns: () => boolean }) {
  const api = useApi(); useLocale();
  const [choices, setChoices] = useState<Record<string, Choice>>({}), [denied, setDenied] = useState(false), [refreshing, setRefreshing] = useState(false);
  const [shown, setShown] = useState(20), [open, setOpen] = useState(false);
  const alive = useRef(true), blocked = useRef(disabled), callbacks = useRef(onChange), state = useRef(choices);
  const requests = useRef(new Map<string, { generation: number; controller: AbortController }>()), generation = useRef(0);
  blocked.current = disabled; callbacks.current = onChange; state.current = choices;
  const tests = associatedDispatchTests(workflow, agentID);
  const catalog = (inventory.items || []).map(value => testDispatchProfile(value, project)).filter((value): value is ToolProfile => !!value);
  useEffect(() => { alive.current = true; return () => { alive.current = false; generation.current++; for (const value of requests.current.values()) value.controller.abort(); }; }, []);
  useEffect(() => {
    if (!disabled) return;
    for (const value of requests.current.values()) value.controller.abort();
    requests.current.clear(); generation.current++; setRefreshing(false);
    if (Object.values(state.current).some(choice => choice.phase === 'reading'))
      publish(Object.fromEntries(Object.entries(state.current).map(([node, choice]) => [node, choice.phase === 'reading' ? { expected: choice.expected, phase: 'changed' as const } : choice])));
  }, [disabled]);
  function publish(next: Record<string, Choice>, accessDenied = denied, loading = false) {
    if (!alive.current || !owns()) return;
    state.current = next; setChoices(next);
    const entries = Object.entries(next);
    callbacks.current({ ready: !accessDenied && !loading && entries.every(([, choice]) => choice.phase === 'ready'), hasIntent: accessDenied || loading || entries.length > 0,
      selections: entries.filter(([, choice]) => choice.phase === 'ready' && choice.profile).map(([node_id, choice]) => ({ node_id, tool_profile_id: choice.profile!.id, expected_tool_version: choice.profile!.version })) });
  }
  async function choose(nodeID: string, id: string) {
    if (blocked.current || !owns() || denied || refreshing || inventory.loading || inventory.error) return;
    requests.current.get(nodeID)?.controller.abort(); requests.current.delete(nodeID);
    if (!id) { const next = { ...state.current }; delete next[nodeID]; publish(next); return; }
    if (Object.keys(state.current).length >= 64 && !state.current[nodeID]) return;
    const expected = catalog.find(value => value.id === id) || state.current[nodeID]?.expected;
    if (!expected || expected.id !== id || !tests.some(node => node.id === nodeID)) return;
    const copy = structuredClone(expected);
    if (testDispatchBlocker(copy, developer.worker_id)) { publish({ ...state.current, [nodeID]: { expected: copy, phase: 'changed' } }); return; }
    const controller = new AbortController(), ticket = ++generation.current;
    requests.current.set(nodeID, { generation: ticket, controller });
    publish({ ...state.current, [nodeID]: { expected: copy, phase: 'reading' } });
    const current = () => alive.current && owns() && !blocked.current && !controller.signal.aborted && requests.current.get(nodeID)?.generation === ticket;
    try {
      const raw = await api<unknown>('/tool-profiles/' + encodeURIComponent(id) + query(project), undefined, 'GET', undefined, controller.signal);
      if (!current()) return;
      const profile = readTestDispatchProfile(raw, copy, project, developer.worker_id);
      publish({ ...state.current, [nodeID]: { expected: copy, phase: profile ? 'ready' : 'changed', ...(profile ? { profile } : {}) } });
    } catch (error) {
      if (!current()) return;
      const forbidden = error instanceof ApiError && [401, 403, 404].includes(error.status);
      if (forbidden) {
        setDenied(true);
        const cleared = Object.fromEntries(Object.entries(state.current).map(([node, choice]) => [node, { expected: choice.expected, phase: 'forbidden' as const }]));
        publish(cleared, true);
      } else publish({ ...state.current, [nodeID]: { expected: copy, phase: 'unknown' } });
    } finally { if (requests.current.get(nodeID)?.generation === ticket) requests.current.delete(nodeID); }
  }
  async function refresh() {
    if (blocked.current || !owns() || refreshing) return;
    for (const value of requests.current.values()) value.controller.abort(); requests.current.clear();
    const interrupted = Object.fromEntries(Object.entries(state.current).map(([node, choice]) => [node, choice.phase === 'reading' ? { expected: choice.expected, phase: 'unknown' as const } : choice]));
    const ticket = ++generation.current; setRefreshing(true); publish(interrupted, denied, true);
    try {
      const accepted = await inventory.read();
      if (!alive.current || !owns() || ticket !== generation.current || blocked.current) return;
      if (accepted === true) { setDenied(false); publish({}, false); }
      else publish(interrupted, denied);
    } catch { if (alive.current && owns() && ticket === generation.current) publish(interrupted, denied); }
    finally { if (alive.current && owns() && ticket === generation.current) setRefreshing(false); }
  }
  return <details className="dispatch-test-bindings" data-dispatch-test-bindings onToggle={event => setOpen(event.currentTarget.open)}>
    <summary><ChevronRight size={16} aria-hidden="true" />{t('title')}<small>{formatNumber(tests.length)}</small></summary>
    {open && <div className="dispatch-test-bindings-body"><p>{t('boundary')}</p>
      <p className="dispatch-test-muted">{t('catalog_scope', { loaded: formatNumber(catalog.length), total: formatNumber(inventory.total) })}</p>
      {denied ? <p role="alert">{t('denied')}</p> : <>
        {tests.slice(0, shown).map(node => {
          const choice = choices[node.id], options = choice && !catalog.some(value => value.id === choice.expected.id) ? [...catalog, choice.expected] : catalog;
          return <section key={node.id} data-dispatch-test-node={node.id}><h4>{node.name || node.id}</h4>
            <p className="dispatch-test-muted">{t('published', { command: node.command?.join(' ') || t('unconfirmed'), profile: typeof node.config?.tool_profile_id === 'string' ? node.config.tool_profile_id : t('unconfirmed') })}</p>
            <label>{t('tool')}<select data-dispatch-test-select={node.id} value={choice?.expected.id || ''} disabled={disabled || inventory.loading || !!inventory.error || refreshing || !choice && Object.keys(choices).length >= 64}
              onChange={event => void choose(node.id, event.target.value)}><option value="">{t('keep_published')}</option>
              {options.map(value => { const reason = testDispatchBlocker(value, developer.worker_id); return <option key={value.id} value={value.id} disabled={!!reason}>{value.name} · {value.worker_id || t('unconfirmed')} · {reason ? t('blocked.' + reason) : t('observed')}</option>; })}
            </select></label>
            {choice && <p role="status" data-dispatch-test-status={choice.phase}>{t('phase.' + choice.phase)}</p>}
            {choice && <button type="button" data-dispatch-test-recheck={node.id} disabled={disabled || inventory.loading || !!inventory.error || refreshing || choice.phase === 'reading'} onClick={() => void choose(node.id, choice.expected.id)}>{t('recheck')}</button>}
            {choice?.profile && <p>{t('frozen_choice', { version: formatNumber(choice.profile.version), worker: choice.profile.worker_id })}</p>}
            {choice?.expected.reason && <small>{toolReason(choice.expected.reason)}</small>}
          </section>;
        })}
        {!tests.length && <p>{t('no_tests')}</p>}
        {Object.keys(choices).length >= 64 && <p>{t('limit')}</p>}
        {shown < tests.length && <button type="button" disabled={disabled} onClick={() => setShown(value => value + 20)}>{t('show_more')}</button>}
      </>}
      {!!inventory.error && <p role="alert">{t('catalog_error')}</p>}
      <div className="dispatch-test-actions"><button type="button" data-dispatch-test-refresh disabled={disabled || inventory.loading || refreshing} onClick={() => void refresh()}><RefreshCw size={14} aria-hidden="true" />{t('refresh')}</button>
        {!!inventory.next_cursor && !denied && <button type="button" disabled={disabled || inventory.loading || refreshing} onClick={() => void inventory.read(true)}>{t('more_tools')}</button>}</div>
    </div>}
  </details>;
}
