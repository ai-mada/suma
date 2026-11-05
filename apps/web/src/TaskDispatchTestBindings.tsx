import { useEffect, useRef, useState } from 'react';
import { CheckCircle2, ChevronRight, CircleAlert, CircleHelp, Clock3, RefreshCw } from 'lucide-react';
import type { TaskDispatchTestOption } from '../../../packages/contracts/src';
import type { Workflow } from './api';
import { ApiError } from './api';
import { useApi, useApiView } from './api-context';
import { formatDateTime, formatNumber, tr, useLocale } from './i18n';
import type { ToolProfile } from './tool-model';
import { associatedDispatchTests, testDispatchBlocker, testDispatchOptionPath, testDispatchProfile, validTestDispatchOption, type TaskDispatchTestSelection } from './task-dispatch-test-bindings-model';
import { toolError } from './tool-hooks';
import { ErrorNotice } from './ui';
import './TaskDispatchTestBindings.css';

export type TestBindingsChoice = { ready: boolean; hasIntent: boolean; selections: TaskDispatchTestSelection[] };
type Inventory = { items?: ToolProfile[]; total: number; loading: boolean; error: string; next_cursor: string; read: (more?: boolean) => Promise<unknown> };
type Props = { project: string; task: { id: string; version: number }; workflow: Workflow; agentID: string; developer: ToolProfile; scopeKey: string; inventory: Inventory; disabled?: boolean; onChange: (value: TestBindingsChoice) => void };
type Phase = 'reading' | 'ready' | 'changed' | 'blocked' | 'unknown' | 'forbidden';
type Choice = { expected: ToolProfile; phase: Phase; confirmation?: TaskDispatchTestOption; error?: ApiError };
const phaseIcon = { reading: Clock3, ready: CheckCircle2, changed: CircleAlert, blocked: CircleAlert, unknown: CircleHelp, forbidden: CircleAlert };
const t = (key: string, values?: Record<string, unknown>) => tr('dispatch-test-bindings.' + key, values);

/** The shared directory supplies display candidates. An explicit selection
 * checks the exact testing binding in the current task and execution route. */
export function TaskDispatchTestBindings(props: Props) {
  const api = useApi(), view = useApiView();
  const { project, task, workflow, agentID, developer, scopeKey } = props;
  const key = JSON.stringify([view?.userID, view?.organizationID, project, task.id, task.version, workflow.id, workflow.version,
    agentID, developer.id, developer.version, developer.profile.profile_sha256, developer.worker_id, scopeKey]);
  const owner = useRef({ api, key, epoch: 0 });
  if (owner.current.api !== api || owner.current.key !== key) owner.current = { api, key, epoch: owner.current.epoch + 1 };
  const current = owner.current;
  return <BindingsScope key={current.epoch} {...props} disabled={props.disabled || !view?.userID || !view.organizationID}
    organizationID={view?.organizationID || ''} owns={() => owner.current === current} />;
}
function BindingsScope({ project, task, workflow, agentID, developer, inventory, disabled = false, onChange, organizationID, owns }: Props & { organizationID: string; owns: () => boolean }) {
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
      selections: entries.filter(([, choice]) => choice.phase === 'ready' && choice.confirmation).map(([node_id, choice]) => ({ node_id, tool_profile_id: choice.expected.id, expected_tool_version: choice.expected.version })) });
  }
  async function choose(nodeID: string, id: string, recheck = false) {
    if (blocked.current || !owns() || denied || refreshing || inventory.loading || inventory.error) return;
    requests.current.get(nodeID)?.controller.abort(); requests.current.delete(nodeID);
    if (!id) { const next = { ...state.current }; delete next[nodeID]; publish(next); return; }
    if (Object.keys(state.current).length >= 64 && !state.current[nodeID]) return;
    const previous = state.current[nodeID]?.expected;
    const expected = recheck ? previous : catalog.find(value => value.id === id);
    if (!expected || expected.id !== id || !tests.some(node => node.id === nodeID)) return;
    const copy = structuredClone(expected);
    if (testDispatchBlocker(copy, developer.worker_id)) { publish({ ...state.current, [nodeID]: { expected: copy, phase: 'changed' } }); return; }
    const controller = new AbortController(), ticket = ++generation.current;
    requests.current.set(nodeID, { generation: ticket, controller });
    publish({ ...state.current, [nodeID]: { expected: copy, phase: 'reading' } });
    const current = () => alive.current && owns() && !blocked.current && !controller.signal.aborted && requests.current.get(nodeID)?.generation === ticket;
    try {
      const basis = { project, task, workflow, agentID, developer, nodeID, tester: copy };
      const raw = await api<unknown>(testDispatchOptionPath(basis), undefined, 'GET', undefined, controller.signal);
      if (!current()) return;
      const next: Choice = validTestDispatchOption(raw, basis, organizationID)
        ? { expected: copy, phase: 'ready', confirmation: raw }
        : { expected: copy, phase: 'changed' };
      publish({ ...state.current, [nodeID]: next });
    } catch (error) {
      if (!current()) return;
      const forbidden = error instanceof ApiError && [401, 403, 404].includes(error.status);
      if (forbidden) {
        setDenied(true);
        for (const request of requests.current.values()) request.controller.abort();
        requests.current.clear();
        const cleared = Object.fromEntries(Object.entries(state.current).map(([node, choice]) => [node, { expected: choice.expected, phase: 'forbidden' as const }]));
        publish(cleared, true);
      } else {
        const next: Choice = error instanceof ApiError && [400, 409].includes(error.status)
          ? { expected: copy, phase: 'blocked', error }
          : { expected: copy, phase: 'unknown' };
        publish({ ...state.current, [nodeID]: next });
      }
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
          const Icon = choice && phaseIcon[choice.phase];
          const confirmation = choice?.confirmation;
          return <section key={node.id} data-dispatch-test-node={node.id}><h4>{node.name || node.id}</h4>
            <p className="dispatch-test-muted">{t('published', { command: node.command?.join(' ') || t('unconfirmed'), profile: typeof node.config?.tool_profile_id === 'string' ? node.config.tool_profile_id : t('unconfirmed') })}</p>
            <label>{t('tool')}<select data-dispatch-test-select={node.id} value={choice?.expected.id || ''} disabled={disabled || inventory.loading || !!inventory.error || refreshing || !choice && Object.keys(choices).length >= 64}
              onChange={event => void choose(node.id, event.target.value)}><option value="">{t('keep_published')}</option>
              {options.map(value => { const reason = testDispatchBlocker(value, developer.worker_id); return <option key={value.id} value={value.id} disabled={!!reason}>{value.name} · {value.worker_id || t('unconfirmed')} · {reason ? t('blocked.' + reason) : t('observed')}</option>; })}
            </select></label>
            {choice && Icon && <div className="dispatch-test-preflight" data-phase={choice.phase}>
              <p role="status" data-dispatch-test-status={choice.phase}><Icon size={16} aria-hidden="true" />{t('phase.' + choice.phase)}</p>
              {confirmation && <div className="dispatch-test-preflight-basis">
                <small>{t('frozen_choice', { version: formatNumber(choice.expected.version), worker: choice.expected.worker_id })}</small>
                <small data-dispatch-test-route>{confirmation.execution_route.pool_id
                  ? t('route_pool', { pool: confirmation.execution_route.pool_id })
                  : t('route_local')}</small>
                <small>{t('checked_at', { time: formatDateTime(confirmation.checked_at, { dateStyle: 'short', timeStyle: 'medium' }) })}</small>
              </div>}
              {choice.error && <ErrorNotice error={toolError(choice.error)} />}
              <button type="button" data-dispatch-test-recheck={node.id} disabled={disabled || inventory.loading || !!inventory.error || refreshing || choice.phase === 'reading'}
                onClick={() => void choose(node.id, choice.expected.id, true)}>{t('recheck')}</button>
            </div>}
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
