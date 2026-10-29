import {useCallback, useEffect, useId, useRef, useState} from 'react';
import {ChevronDown, RefreshCw} from 'lucide-react';
import type {WorkspaceTerminalSession, WorkspaceTerminalRecoveryView, WorkspaceTerminalRecoveryReceipt} from '../../../packages/contracts/src';
import {query, stateLabel} from './api';
import {useApi} from './api-context';
import {formatNumber, renderDiagnostic, tr, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {useWriteAction, WriteNotice} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {useReplyRead} from './task-conversation-reply-hooks';
import {terminalRecoveryReason, validTerminalRecoverReceipt, validTerminalRecoverRequest, validTerminalRecoveryView} from './terminal-recovery-model';

const t = (key: string, values?: Record<string, unknown>) => tr('terminal.' + key, values);
type Props = {project: string; task: string; session: WorkspaceTerminalSession; scope: string; active: boolean; disabled: boolean;
  onSession: (session: WorkspaceTerminalSession) => void; onDenied: () => void; onLockChange: (locked: boolean) => void};

/** Reconciliation records a failed original execution. It cannot restart the
 * shell, attest tree stop, or rewrite unknown input as delivered. */
export function TerminalRecoveryPanel({project, task, session, scope, active, disabled, onSession, onDenied, onLockChange}: Props) {
  const api = useApi(); useLocale(); const id = useId();
  const [open, setOpen] = useState(session.state === 'reconciling'), [reason, setReason] = useState(''), [consent, setConsent] = useState(false);
  const [reviewed, setReviewed] = useState<string[]>([]), [completed, setCompleted] = useState(false);
  const callbacks = useRef({onSession, onDenied, onLockChange}); callbacks.current = {onSession, onDenied, onLockChange};
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  const action = useWriteAction({scope: 'terminal-execution-recovery:' + scope + ':' + session.id, storage: toolSessionStorage(),
    validateRequest: request => validTerminalRecoverRequest(request, project, session.id, session.run_id),
    validateResponse: (value, request) => validTerminalRecoverReceipt(value, request, project, task, session.id, session.run_id)});
  const locked = action.busy || !!action.pending;
  const blocked = action.errorStatus === 401 || action.errorStatus === 403;
  const refuse = useCallback(() => callbacks.current.onDenied(), []);
  const read = useReplyRead('/workspace/terminal-sessions/' + encodeURIComponent(session.id) + '/recovery' + query(project), open && active && !blocked,
    (value): value is WorkspaceTerminalRecoveryView => validTerminalRecoveryView(value, session.id, session.run_id), refuse);
  const view = read.value;
  const reviewedInputs = new Set(reviewed);
  const basis = JSON.stringify([session.version, view]);
  useEffect(() => {alive.current = true; return () => {alive.current = false; callbacks.current.onLockChange(false);};}, []);
  useEffect(() => {callbacks.current.onLockChange(locked);}, [locked]);
  useEffect(() => {if (blocked) callbacks.current.onDenied();}, [blocked]);
  useEffect(() => {if (action.pending) setOpen(true);}, [!!action.pending]);
  useEffect(() => {setConsent(false); setReviewed([]);}, [basis]);
  const ready = !!view?.can_recover && view.tree_stopped && view.unknown_inputs.every(input => reviewedInputs.has(input.input_id)) &&
    consent && terminalRecoveryReason(reason) && !disabled && !locked && !blocked && !read.loading && !read.error && !action.conflict && !action.persistenceBlocked;
  async function recover(original = false) {
    const pending = action.pending, caller = api;
    if (disabled || blocked || (original ? !pending || action.busy : !ready || !view)) return;
    const value = original && pending ? await action.run<WorkspaceTerminalRecoveryReceipt>(pending.path, pending.body, pending.method) :
      await action.run<WorkspaceTerminalRecoveryReceipt>('/workspace/terminal-sessions/' + encodeURIComponent(session.id) + '/recover', {
        project_id: project, expected_version: session.version, expected_run_version: view!.run_version, run_id: view!.run_id,
        attempt: view!.attempt, fence: view!.fence, dispatch_id: view!.dispatch_id, unknown_inputs: view!.unknown_inputs, reason: reason.trim()});
    if (!value || !alive.current || owner.current !== caller) return;
    callbacks.current.onSession(value.session); setCompleted(true); setConsent(false); setReviewed([]); await read.read();
  }
  return <section className="terminal-recovery" aria-label={t('recovery.title')}>
    <button type="button" className="terminal-subsection-toggle" data-terminal-recovery-toggle aria-expanded={open} aria-controls={id}
      disabled={locked} onClick={() => setOpen(value => !value)}><span>{t('recovery.title')}</span><ChevronDown size={14} aria-hidden="true"/></button>
    {open && <div id={id} className="terminal-subsection-body">
      <p className="terminal-note">{t('recovery.intro')}</p>
      <WriteNotice action={action} onRebase={async () => {await read.read(); action.clearConflict(); setConsent(false); setReviewed([]);}}/>
      {action.pending && <div role="status"><p>{t('recovery.pending')}</p><button type="button" disabled={disabled || action.busy || blocked} onClick={() => void recover(true)}>{t('original')}</button></div>}
      {completed && <p role="status">{t('recovery.completed')}</p>}
      <button type="button" disabled={locked || read.loading} onClick={() => void read.read()}><RefreshCw size={13} aria-hidden="true"/>{t('recovery.check')}</button>
      {read.loading && <Loading/>}{!!read.error && <ErrorNotice error={read.error} onRetry={() => void read.read()}/>}
      {view && <>
        <p className={view.tree_stopped ? 'terminal-note' : 'terminal-warning'} role="status">{t(view.tree_stopped ? 'recovery.tree_stopped' : 'recovery.tree_unknown')}</p>
        {!!view.reason && <p className="terminal-warning">{renderDiagnostic(view.reason)}</p>}
        {!!view.unknown_inputs.length && <div className="terminal-unknown-inputs"><p className="terminal-warning">{t('recovery.inputs_unknown')}</p>
          <ul>{view.unknown_inputs.map(input => <li key={input.input_id}><label className="terminal-recovery-consent"><input type="checkbox" checked={reviewedInputs.has(input.input_id)}
            disabled={disabled || locked || !view.can_recover} onChange={event => {setReviewed(items => event.target.checked ? [...items, input.input_id] : items.filter(item => item !== input.input_id)); setConsent(false);}}/>
            <span>{t('recovery.input_ack', {sequence: formatNumber(input.sequence)})}<small>{input.input_id}</small><small>{input.sha256}</small></span></label></li>)}</ul></div>}
        {view.can_recover && <>
          <Field label={t('recovery.reason')} hint={t('recovery.reason_hint')}><textarea data-terminal-recovery-reason value={reason} maxLength={2000} disabled={disabled || locked}
            onChange={event => {setReason(event.target.value); setConsent(false);}}/></Field>
          <label className="terminal-recovery-consent"><input type="checkbox" checked={consent} disabled={disabled || locked} onChange={event => setConsent(event.target.checked)}/><span>{t('recovery.consent')}</span></label>
          <button type="button" data-terminal-recover disabled={!ready} onClick={() => void recover()}>{t('recovery.reconcile')}</button>
        </>}
        <details><summary>{t('recovery.basis')}</summary><dl className="terminal-facts"><dt>{t('recovery.run')}</dt><dd>{view.run_id}</dd><dt>{t('recovery.run_state')}</dt><dd>{stateLabel(view.run_state)}</dd>
          <dt>{t('recovery.attempt')}</dt><dd>{formatNumber(view.attempt)} / {formatNumber(view.fence)}</dd><dt>{t('recovery.dispatch')}</dt><dd>{view.dispatch_id || '—'}</dd></dl></details>
      </>}
    </div>}
  </section>;
}
