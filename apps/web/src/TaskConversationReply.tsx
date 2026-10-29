import {useCallback, useEffect, useId, useRef, useState} from 'react';
import {ArrowUpRight, ChevronDown, RefreshCw, Sparkles} from 'lucide-react';
import type {TaskConversationPage, TaskConversationReply as Reply} from '../../../packages/contracts/src';
import type {Session} from './api';
import {query} from './api';
import {useApi} from './api-context';
import {formatDateTime, formatNumber, renderDiagnostic, tr, UiError, useLocale} from './i18n';
import {DiscussionDevelopmentPanel} from './DiscussionDevelopmentPanel';
import {ErrorNotice, Field, Loading} from './ui';
import {ModelExecutionFacts, ModelExecutionPanel, type ModelChoice} from './ModelExecutionPanel';
import {modelInteger} from './model-execution-model';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction, WriteNotice} from './write-action';
import {connectRunEvents, runConnectionMessage, SnapshotRefreshQueue, type RunConnectionState} from './run-events';
import {useReplyCollection, useReplyRead} from './task-conversation-reply-hooks';
import {activeReply, replyID, replyProfile, validReply, validReplyPage, validReplyProfiles,
  validReplyReceipt, validReplyRequest} from './task-conversation-reply-model';
import './TaskConversationReply.css';

const t = (key: string, values?: Record<string, unknown>) => tr('task-reply.' + key, values);
type Props = {session: Session; project: string; taskId: string; page?: TaskConversationPage; accessBlocked: boolean;
  disabled: boolean; refresh: number; onMessagesRefresh: () => Promise<unknown>; onLockChange: (locked: boolean) => void;
  onDevelopmentStarted?: () => Promise<unknown> | void};
const runHref = (project: string, run: string) => '?' + new URLSearchParams({project, view: 'development', sub: 'runs', run});
const navigate = () => window.dispatchEvent(new Event('suma:navigate', {cancelable: true}));

/** A real reply is a separate managed execution. It never replaces the task's
 * delivery run or turns generated text into terminal/code/review authority. */
export function TaskConversationReply(props: Props) {
  const api = useApi(), owner = useRef({api, epoch: 0});
  if (owner.current.api !== api) owner.current = {api, epoch: owner.current.epoch + 1};
  const scope = JSON.stringify([props.session.user.id, props.session.organization.id, props.project, props.taskId]);
  return <ReplyScope key={scope + ':' + owner.current.epoch} {...props} scope={scope}/>;
}

function ReplyScope({session, project, taskId, page, accessBlocked, disabled, refresh, onMessagesRefresh, onLockChange, onDevelopmentStarted,
  scope}: Props & {scope: string}) {
  const api = useApi(), locale = useLocale(), id = useId(), storage = toolSessionStorage();
  const locator = 'suma-task-reply-locator:' + scope;
  const [open, setOpen] = useState(false), [profileID, setProfileID] = useState(''), [ack, setAck] = useState(false);
  const [windowSize, setWindowSize] = useState('20'), [model, setModel] = useState<ModelChoice>({ready: false});
  const [selected, setSelected] = useState(() => {try {const value = storage?.getItem(locator); return replyID(value) ? value : '';} catch {return '';}});
  const [revoked, setRevoked] = useState(false), [retry, setRetry] = useState(0), [notice, setNotice] = useState('');
  const [connection, setConnection] = useState<RunConnectionState>();
  const callbacks = useRef({onMessagesRefresh, onLockChange, onDevelopmentStarted}); callbacks.current = {onMessagesRefresh, onLockChange, onDevelopmentStarted};
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  useEffect(() => {alive.current = true; return () => {alive.current = false; callbacks.current.onLockChange(false);};}, []);
  const action = useWriteAction({scope: 'task-reply:' + scope, storage,
    validateRequest: request => validReplyRequest(request, project, taskId),
    validateResponse: (value, request) => validReplyReceipt(value, request, project, taskId, session.user.id, session.organization.id)});
  const writeDenied = action.errorStatus === 401 || action.errorStatus === 403;
  const blocked = accessBlocked || revoked || writeDenied;
  const refuse = useCallback(() => setRevoked(true), []);
  const history = useReplyCollection('/task-conversation-replies' + query(project) + '&task_id=' + encodeURIComponent(taskId) + '&limit=20',
    'cursor', open && !blocked, value => validReplyPage(value, project, taskId, session.organization.id), refuse, refresh + retry);
  const inventory = useReplyCollection('/tool-profiles' + query(project) + '&limit=50', 'cursor', open && !blocked,
    value => validReplyProfiles(value, project), refuse, retry);
  const detail = useReplyRead('/task-conversation-replies/' + encodeURIComponent(selected) + query(project),
    !!selected && !blocked, value => validReply(value, project, taskId, session.organization.id), refuse, refresh + retry);
  const [developmentLocked,setDevelopmentLocked]=useState(false);
  const current = detail.value, locked = action.busy || !!action.pending;
  const candidates = inventory.value?.items.filter(replyProfile) || [], profile = candidates.find(item => item.id === profileID);
  const head = page?.items[page.items.length - 1];
  const turn = page?.items.slice().reverse().find(message => message.actor_kind === 'human' || message.actor_kind === 'api');
  const limit = modelInteger(windowSize, 20), selection = model.selection;
  const includesTurn = !!turn && !!limit && !!page?.items.slice(-limit).some(message => message.sequence === turn.sequence);
  const historyItems = history.value?.items || [];
  const active = historyItems.some(activeReply) || !!current && activeReply(current);
  const completed = !!turn && (historyItems.some(item => item.source.reply_to_sequence === turn.sequence && item.state === 'completed') ||
    !!current && current.source.reply_to_sequence === turn.sequence && current.state === 'completed');
  const allowed = !blocked && !disabled && !!page?.can_post && !!turn && !!head && !!limit && limit >= 1 && includesTurn &&
    !!profile && !!selection && model.ready && selection.parameters.max_steps === 1 && selection.parameters.max_output_tokens <= 8192 &&
    ack && !active && !completed && !locked && !developmentLocked && !action.conflict && !action.persistenceBlocked && !history.loading && !history.error &&
    !inventory.loading && !inventory.error;
  useEffect(() => {callbacks.current.onLockChange(locked||developmentLocked);}, [locked,developmentLocked]);
  useEffect(() => {if (writeDenied) setRevoked(true);}, [writeDenied]);
  useEffect(() => {if (action.pending) setOpen(true);}, [!!action.pending]);
  useEffect(() => {setAck(false);}, [head?.sequence, page?.task_version, windowSize, profileID]);
  useEffect(() => {
    if (!selected && !action.pending && !blocked) {
      const original = history.value?.items.find(activeReply);
      if (original) select(original.id);
    }
  }, [selected, history.value, !!action.pending, blocked]);
  const updateModel = useCallback((value: ModelChoice) => {setModel(value); setAck(false);}, []);
  const developmentStarted = useCallback(() => {
    // The start receipt is already verified and retained. Refresh current task
    // and discussion facts independently; a failed read must not retry Start
    // or clear the original plan/unknown write owned by the child panel.
    void Promise.allSettled([Promise.resolve().then(() => callbacks.current.onMessagesRefresh()),
      Promise.resolve().then(() => callbacks.current.onDevelopmentStarted?.())]);
  }, []);
  const observed = useRef(new Set<string>());
  useEffect(() => {
    if (!current || current.state !== 'completed' || !current.message_id || observed.current.has(current.message_id) || blocked) return;
    observed.current.add(current.message_id); void callbacks.current.onMessagesRefresh();
  }, [current?.state, current?.message_id, blocked]);
  const refreshActual = useRef<() => Promise<void>>(async () => {});
  refreshActual.current = async () => {await Promise.all([detail.read(), history.read()]);};
  useEffect(() => {
    if (!current || !activeReply(current) || blocked) {setConnection(undefined); return;}
    const controller = new AbortController(), queue = new SnapshotRefreshQueue(() => refreshActual.current());
    void connectRunEvents({project, run: current.run_id, signal: controller.signal,
      onEvent: event => {if (event.type !== 'tool_progress') queue.request();},
      onState: value => {if (!controller.signal.aborted) {setConnection(value); if (value.state === 'live') queue.request();
        if (value.state === 'forbidden') refuse();}}});
    return () => {controller.abort(); queue.close();};
  }, [current?.run_id, current?.state, blocked, project, refuse]);

  function select(value: string) {
    setSelected(value); setNotice('');
    try {if (value) storage?.setItem(locator, value); else storage?.removeItem(locator);} catch {setNotice('locator_unavailable');}
  }
  async function consume(value: Reply | undefined, caller: typeof api) {
    if (!value || !alive.current || owner.current !== caller) return;
    select(value.id); setAck(false); setNotice(value.state === 'completed' ? 'stored' : 'requested');
    // A command may update the already selected reply. Selecting the same ID
    // does not refetch its detail; refresh it explicitly after the receipt,
    // without waiting for another SSE event to report a stop request.
    await Promise.allSettled([history.read(), ...(selected === value.id ? [detail.read()] : [])]);
    if (!alive.current || owner.current !== caller) return;
    if (value.state === 'completed' && !observed.current.has(value.message_id)) {
      observed.current.add(value.message_id); await callbacks.current.onMessagesRefresh();
    }
  }
  async function create() {
    if (!allowed || !page || !turn || !head || !profile || !selection || !limit) return;
    const caller = api;
    await consume(await action.run<Reply>('/task-conversation-replies', {project_id: project, task_id: taskId,
      expected_task_version: page.task_version, expected_last_sequence: head.sequence, reply_to_sequence: turn.sequence,
      context_message_limit: limit, tool_profile_id: profile.id, expected_tool_version: profile.version,
      model_execution_selection: selection, locale, acknowledge_model_use: true}), caller);
  }
  async function cancel() {
    if (!current || disabled || blocked || locked || !current.allowed_actions.includes('cancel') || detail.loading || detail.error) return;
    const caller = api;
    await consume(await action.run<Reply>('/task-conversation-replies/' + encodeURIComponent(current.id) + '/commands', {
      project_id: project, expected_version: current.version, expected_run_version: current.run_version, action: 'cancel'}), caller);
  }
  async function restore() {
    const original = action.pending;
    if (!original || blocked || disabled || !page?.can_post) return;
    const caller = api; await consume(await action.run<Reply>(original.path, original.body, original.method), caller);
  }
  async function reload() {
    const result = await history.read(); if (!result) throw new UiError('task-reply.refresh_failed');
    if (selected) await detail.read(); await callbacks.current.onMessagesRefresh(); setAck(false);
  }

  return <section className="task-reply" aria-label={t('title')}>
    <button type="button" className="task-reply-toggle" data-reply-toggle aria-expanded={open} aria-controls={id}
      disabled={open && (locked || developmentLocked)} onClick={() => setOpen(value => !value)}><Sparkles size={15} aria-hidden="true"/>
      <span>{t('title')}</span>{!blocked && current && <small>{t('state.' + current.state)}</small>}
      <ChevronDown size={15} aria-hidden="true"/></button>
    {open && <div id={id} className="task-reply-body">
      <p className="task-reply-muted">{t('intro')}</p>
      {blocked ? <div role="alert" className="task-reply-warning"><p>{t('access_changed')}</p>
        {!accessBlocked && <button type="button" onClick={() => {action.setError(''); setRevoked(false); setRetry(value => value + 1);}}>{t('check_access')}</button>}
        {!!action.pending && <p>{t('pending_hint')}</p>}
      </div> : <>
        <WriteNotice action={action} onRebase={reload}/>
        {action.pending && <div className="task-reply-recovery" role="status"><strong>{t('original_operation')}</strong>
          <p>{t('pending_hint')}</p><button type="button" data-reply-restore disabled={action.busy || disabled || !page?.can_post}
            onClick={() => void restore()}>{t('restore')}</button></div>}
        {notice && <p role="status" className="task-reply-muted">{t(notice)}</p>}
        {disabled && <p className="task-reply-muted">{t('save_note_first')}</p>}
        {page && !turn && <p className="task-reply-muted">{t('no_turn')}</p>}
        {!!history.error && <ErrorNotice error={history.error} onRetry={() => void history.read()}/>}
        {history.loading && !history.value && <Loading/>}
        {current && <div className="task-reply-current" aria-label={t('execution')}>
          <header><strong>{t('state.' + current.state)}</strong><a href={runHref(project, current.run_id)}
            onClick={event => {if (!navigate()) event.preventDefault();}}>{t('open_run')}<ArrowUpRight size={12}/></a></header>
          <p className="task-reply-muted">{t('hint.' + current.state)}</p>
          {connection && <p role="status" className="task-reply-muted">{runConnectionMessage(connection) || t('connected')}</p>}
          {detail.loading && <p className="task-reply-muted">{t('reading')}</p>}
          {!!detail.error && <ErrorNotice error={detail.error}/>}
          {!current.basis_current && <p className="task-reply-warning">{t('basis_changed')}</p>}
          {!!current.reason && <><p className="task-reply-copy">{renderDiagnostic(current.reason)}</p><details><summary>{t('original_reason')}</summary><p className="task-reply-copy">{current.reason}</p></details></>}
          <div className="task-reply-actions"><button type="button" data-reply-refresh disabled={locked || detail.loading} onClick={() => void reload().catch(action.setError)}><RefreshCw size={13}/>{t('refresh')}</button>
            {current.allowed_actions.includes('cancel') && <button type="button" data-reply-cancel disabled={disabled || locked || detail.loading || !!detail.error} onClick={() => void cancel()}>{t('cancel')}</button>}</div>
          <details><summary>{t('fixed_context')}</summary><p>{t(current.source.coverage.complete ? 'context_complete' : 'context_partial', {
            included: formatNumber(current.source.coverage.included), total: formatNumber(current.source.coverage.total),
            first: formatNumber(current.source.coverage.first_sequence), last: formatNumber(current.source.coverage.last_sequence)})}</p>
            <p>{t('source_basis', {task: formatNumber(current.source.task.version), turn: formatNumber(current.source.reply_to_sequence)})}</p>
            <code>{current.source.sha256}</code></details>
          <details><summary>{t('model_basis')}</summary><ModelExecutionFacts binding={current.model_execution}/></details>
          {current.state==='completed'&&<DiscussionDevelopmentPanel session={session} project={project} taskId={taskId} reply={current} page={page} disabled={disabled||locked} accessBlocked={blocked} onLockChange={setDevelopmentLocked} onRun={developmentStarted}/>}
        </div>}
        {!!selected && detail.loading && !current && <Loading/>}
        {!!selected && !!detail.error && !current && <ErrorNotice error={detail.error} onRetry={() => void detail.read()}/>}
        {!active && !completed && page && <div className="task-reply-config">
          {!!inventory.error && <ErrorNotice error={inventory.error} onRetry={() => void inventory.read()}/>}
          <Field label={t('profile')}><select data-reply-profile value={profileID} disabled={disabled || locked || inventory.loading}
            onChange={event => {setProfileID(event.target.value); setModel({ready: false}); setAck(false);}}><option value="">{t('choose_profile')}</option>
            {candidates.map(item => <option key={item.id} value={item.id}>{item.name} · v{formatNumber(item.version)}</option>)}</select></Field>
          {!inventory.loading && !candidates.length && <p className="task-reply-muted">{t('no_profile')}</p>}
          <p className="task-reply-muted">{t('profiles_loaded', {loaded: formatNumber(inventory.value?.items.length || 0), total: formatNumber(inventory.value?.total || 0)})}</p>
          {inventory.value?.next_cursor && <button type="button" disabled={locked || inventory.loading} onClick={() => void inventory.read(true)}>{t('more_profiles')}</button>}
          {profile && <ModelExecutionPanel key={profile.id + ':' + profile.version} scope={{project, profile: profile.id, version: profile.version, sha256: profile.profile.profile_sha256}}
            supported allowCodex={false} disabled={disabled || locked} onChange={updateModel}/>}
          <details><summary>{t('context_settings')}</summary><Field label={t('context_limit')}><input data-reply-window type="number" min={1} max={20} step={1}
            value={windowSize} disabled={disabled || locked} onChange={event => setWindowSize(event.target.value)}/></Field><p className="task-reply-muted">{t('context_hint')}</p></details>
          {turn && <p className="task-reply-muted">{t('target', {sequence: formatNumber(turn.sequence)})}</p>}
          {turn && !!limit && !includesTurn && <p className="task-reply-warning">{t('target_excluded')}</p>}
          <label className="task-reply-consent"><input data-reply-consent type="checkbox" checked={ack} disabled={disabled || locked}
            onChange={event => setAck(event.target.checked)}/><span>{t('consent')}</span></label>
          <button type="button" className="primary" data-reply-create disabled={!allowed} onClick={() => void create()}><Sparkles size={14}/>{t('ask')}</button>
        </div>}
        {active && <p className="task-reply-muted">{t('active_hint')}</p>}
        {completed && <p className="task-reply-muted">{t('followup')}</p>}
        {!!history.value?.items.length && <details className="task-reply-history"><summary>{t('history')}</summary>
          <ul>{history.value.items.map(item => <li key={item.id}><button type="button" disabled={locked || developmentLocked} aria-pressed={selected === item.id}
            onClick={() => select(item.id)}>{t('history_item', {sequence: formatNumber(item.source.reply_to_sequence),
              at: formatDateTime(item.created_at, {dateStyle: 'short', timeStyle: 'short'})})} · {t('state.' + item.state)}</button></li>)}</ul>
          {history.value.next_cursor && <button type="button" disabled={locked || history.loading} onClick={() => void history.read(true)}>{t('more_history')}</button>}
        </details>}
      </>}
    </div>}
  </section>;
}
