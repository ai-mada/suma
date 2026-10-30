import {useCallback, useEffect, useId, useRef, useState, type MouseEvent} from 'react';
import {ArrowUpRight, ChevronDown, RefreshCw} from 'lucide-react';
import type {CandidateCompositionBaseline, TaskDelivery, WorkspaceTerminalCandidatePlan} from '../../../packages/contracts/src';
import {query, type RecordItem, type Run} from './api';
import {useApi} from './api-context';
import {formatNumber, renderDiagnostic, tr, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {useWriteAction, WriteNotice} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {useReplyCollection, useReplyRead} from './task-conversation-reply-hooks';
import {validTaskDelivery} from './task-delivery-model';
import {compositionBinding, compositionRecovery, compositionText, validCodeBaseline, validCompositionTask} from './candidate-composition-model';
import {TerminalBaselineFacts} from './TerminalBaselineFacts';
import {validManagedRunIdentity} from './task-dispatch-response';

const t = (key: string, values?: Record<string, unknown>) => tr('terminal.' + key, values);
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
type Props = {project: string; plan: WorkspaceTerminalCandidatePlan; scope: string; active: boolean; disabled: boolean;
  onDenied: () => void; onLockChange: (locked: boolean) => void; onRefresh: () => void};

/** Bind an actually accepted terminal candidate through the existing governed
 * baseline command. This does not create or start another execution. */
export function TerminalCandidateHandoffPanel({project, plan, scope, active, disabled, onDenied, onLockChange, onRefresh}: Props) {
  const api = useApi(); useLocale(); const id = useId();
  const [open, setOpen] = useState(false), [targetID, setTargetID] = useState(''), [reason, setReason] = useState(''), [consent, setConsent] = useState(false), [refresh, setRefresh] = useState(0), [bound, setBound] = useState(false);
  const callbacks = useRef({onDenied, onLockChange, onRefresh}); callbacks.current = {onDenied, onLockChange, onRefresh};
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  const action = useWriteAction({scope: 'terminal-candidate-handoff:' + scope + ':' + plan.id, storage: toolSessionStorage(),
    validateRequest: request => compositionRecovery(request, project, '') && /^\/tasks\/[A-Za-z0-9_-]+\/code-baseline$/.test(request.path) &&
      request.body.terminal_candidate_plan_id === plan.id && !Object.hasOwn(request.body, 'composition_id'),
    validateResponse: async (value, request) => {
      const target = /^\/tasks\/([A-Za-z0-9_-]+)\/code-baseline$/.exec(request.path)?.[1];
      if (!target || !validCompositionTask(value, target, project) || value.version <= Number(request.body.expected_task_version)) return false;
      const original = await api<unknown>(request.path + query(project));
      if (!validCodeBaseline(original, project, target) || original.version <= Number(request.body.expected_binding_version)) return false;
      const binding = original.binding;
      return !!binding && binding.terminal_candidate_plan_id === request.body.terminal_candidate_plan_id && binding.terminal_candidate_plan_version === request.body.expected_terminal_candidate_plan_version &&
        binding.source.source.run_id === plan.run_id && binding.source.source.node_id === plan.producer_node_id;
    }});
  const locked = action.busy || !!action.pending, blocked = action.errorStatus === 401 || action.errorStatus === 403;
  const refuse = useCallback(() => callbacks.current.onDenied(), []);
  const source = useReplyRead('/tasks/' + encodeURIComponent(plan.task_id) + '/delivery' + query(project), open && active && !blocked,
    (value): value is TaskDelivery => validTaskDelivery(value, project, plan.task_id), refuse, refresh);
  const run = useReplyRead('/runs/' + encodeURIComponent(plan.run_id) + query(project), open && active && !blocked,
    (value): value is Run => object(value) && value.id === plan.run_id && value.project_id === project && typeof value.version === 'number' &&
      typeof value.state === 'string' && validManagedRunIdentity(value, project, plan.run_id) &&
      Array.isArray(value.nodes) && value.nodes.every(node => object(node) && typeof node.id === 'string' && typeof node.state === 'string'), refuse, refresh);
  const tasks = useReplyCollection('/tasks' + query(project) + '&limit=100', 'cursor', open && active && !blocked,
    (value): value is {items: RecordItem[]; total: number; next_cursor?: string} => object(value) && Array.isArray(value.items) && value.items.length <= 100 &&
      typeof value.total === 'number' && Number.isSafeInteger(value.total) && value.total >= value.items.length &&
      (value.next_cursor === undefined || typeof value.next_cursor === 'string') && value.items.every(item => validCompositionTask(item, undefined, project)), refuse, refresh);
  const target = useReplyRead('/tasks/' + encodeURIComponent(targetID) + query(project), open && active && !!targetID && !blocked,
    (value): value is RecordItem => validCompositionTask(value, targetID, project), refuse, refresh);
  const baseline = useReplyRead('/tasks/' + encodeURIComponent(targetID) + '/code-baseline' + query(project), open && active && !!targetID && !blocked,
    (value): value is CandidateCompositionBaseline => validCodeBaseline(value, project, targetID), refuse, refresh);
  const rawCandidate = run.value?.nodes.find(node => node.id === plan.producer_node_id)?.workspace_evidence?.candidate_bundle;
  const candidate = compositionBinding(rawCandidate, project) && rawCandidate.source.run_id === plan.run_id && rawCandidate.source.node_id === plan.producer_node_id && rawCandidate.source.worker_id === plan.worker_id ? rawCandidate : undefined;
  const accepted = source.value?.formal_acceptance && source.value.run_basis_state === 'current' && source.value.run?.run_id === plan.run_id && source.value.run.state === 'succeeded' &&
    source.value.run.tests.total > 0 && source.value.run.tests.succeeded === source.value.run.tests.total && run.value?.state === 'succeeded' && !!candidate && !source.loading && !source.error && !run.loading && !run.error;
  const ready = !!accepted && !!target.value && target.value.id !== plan.task_id && !target.value.run_id && baseline.value?.can_bind && compositionText(reason) && consent &&
    !disabled && !locked && !blocked && !target.loading && !target.error && !baseline.loading && !baseline.error && !action.conflict && !action.persistenceBlocked;
  const actual = baseline.value?.binding, applied = actual?.terminal_candidate_plan_id === plan.id && actual.terminal_candidate_plan_version === plan.version && actual.source.head_commit === candidate?.head_commit;
  useEffect(() => {alive.current = true; return () => {alive.current = false; callbacks.current.onLockChange(false);};}, []);
  useEffect(() => {callbacks.current.onLockChange(locked);}, [locked]);
  useEffect(() => {if (blocked) callbacks.current.onDenied();}, [blocked]);
  useEffect(() => {setConsent(false);}, [targetID, target.value?.version, baseline.value?.version, source.value?.task_version, source.value?.formal_acceptance, plan.version, refresh]);
  useEffect(() => {if (action.pending) {setOpen(true); const target = /^\/tasks\/([A-Za-z0-9_-]+)\/code-baseline$/.exec(action.pending.path)?.[1]; if (target) setTargetID(target);}}, [!!action.pending]);
  async function bind(original = false) {
    const pending = action.pending, caller = api;
    if (disabled || blocked || (original ? !pending || action.busy : !ready || !target.value || !baseline.value)) return;
    const value = original && pending ? await action.run<RecordItem>(pending.path, pending.body, pending.method) : await action.run<RecordItem>('/tasks/' + encodeURIComponent(targetID) + '/code-baseline', {
      project_id: project, expected_task_version: target.value!.version, expected_binding_version: baseline.value!.version,
      terminal_candidate_plan_id: plan.id, expected_terminal_candidate_plan_version: plan.version, acknowledge: true, reason});
    if (!value || !alive.current || owner.current !== caller) return;
    setBound(true); setConsent(false); await Promise.allSettled([target.read(), baseline.read()]); callbacks.current.onRefresh();
  }
  function follow(event: MouseEvent) {if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}
  return <section className="terminal-candidate-handoff" aria-label={t('handoff.title')}>
    <button type="button" className="terminal-subsection-toggle" data-terminal-handoff-toggle aria-expanded={open} aria-controls={id} disabled={locked} onClick={() => setOpen(value => !value)}><span>{t('handoff.title')}</span><ChevronDown size={14} aria-hidden="true"/></button>
    {open && <div id={id} className="terminal-subsection-body">
      <p className="terminal-note">{t('handoff.intro')}</p><WriteNotice action={action} onRebase={async () => {setConsent(false); setRefresh(value => value + 1); action.clearConflict();}}/>
      {action.pending && <div role="status"><p>{t('handoff.pending')}</p><button type="button" disabled={disabled || action.busy || blocked} onClick={() => void bind(true)}>{t('original')}</button></div>}
      <button type="button" disabled={locked || source.loading || run.loading} onClick={() => {setRefresh(value => value + 1); setConsent(false);}}><RefreshCw size={13} aria-hidden="true"/>{t('handoff.refresh')}</button>
      {(source.loading || run.loading || target.loading || baseline.loading || tasks.loading) && <Loading/>}
      {[source.error, run.error, tasks.error, target.error, baseline.error].filter(Boolean).map((error, index) => <ErrorNotice key={index} error={error}/>)}
      {!accepted && <p className="terminal-warning" role="status">{t('handoff.acceptance_required')}</p>}
      {candidate && <TerminalBaselineFacts project={project} binding={plan.source.code_baseline} adminCommit={plan.source.admin_base_commit} selectedCommit={plan.source.base_commit} candidateCommit={candidate.head_commit} title={t('baseline.candidate_lineage')}/>}
      {!!accepted && <>
        <Field label={t('handoff.target')}><select data-terminal-handoff-target value={targetID} disabled={disabled || locked || tasks.loading} onChange={event => {setTargetID(event.target.value); setConsent(false); setBound(false);}}>
          <option value="">{t('handoff.choose_target')}</option>{tasks.value?.items.filter(item => item.id !== plan.task_id && !item.run_id).map(item => <option key={item.id} value={item.id}>{item.title || item.id} · v{formatNumber(item.version)}</option>)}
          {targetID && !tasks.value?.items.some(item => item.id === targetID) && <option value={targetID} disabled>{targetID}</option>}</select></Field>
        {tasks.value && <p className="terminal-note">{t('handoff.task_page', {loaded: formatNumber(tasks.value.items.length), total: formatNumber(tasks.value.total)})}</p>}
        {tasks.value?.next_cursor && <button type="button" disabled={locked || tasks.loading} onClick={() => void tasks.read(true)}>{t('handoff.more_tasks')}</button>}
        {baseline.value && <>
          {!!baseline.value.blockers.length && <ul className="terminal-warning">{baseline.value.blockers.map((reason, index) => <li key={index}>{renderDiagnostic(reason)}</li>)}</ul>}
          {actual && <p className="terminal-note">{t('handoff.previous_commit')} <code>{actual.source.head_commit}</code></p>}
          {!baseline.value.can_bind && <p className="terminal-warning">{t('handoff.unavailable')}</p>}
          <Field label={t('handoff.reason')} hint={t('handoff.reason_hint')}><textarea data-terminal-handoff-reason value={reason} maxLength={2000} disabled={disabled || locked} onChange={event => {setReason(event.target.value); setConsent(false); setBound(false);}}/></Field>
          <label className="terminal-recovery-consent"><input type="checkbox" checked={consent} disabled={disabled || locked || !baseline.value.can_bind} onChange={event => setConsent(event.target.checked)}/><span>{t('handoff.consent')}</span></label>
          <button type="button" className="primary" data-terminal-handoff-bind disabled={!ready} onClick={() => void bind()}>{t('handoff.bind')}</button>
        </>}
        {bound && applied && <p role="status">{t('handoff.bound')} <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'tasks', record: targetID})} onClick={follow}>{t('handoff.open_target')} <ArrowUpRight size={12} aria-hidden="true"/></a></p>}
      </>}
    </div>}
  </section>;
}
