import {useEffect, useId, useRef, useState, type FormEvent, type MouseEvent} from 'react';
import {ChevronDown, MessageSquare, RefreshCw, Send} from 'lucide-react';
import type {TaskConversationMessage} from '../../../packages/contracts/src';
import type {Session} from './api';
import {useApi} from './api-context';
import {tr, useLocale, UiError, formatDateTime, formatNumber} from './i18n';
import {ErrorNotice, Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard, useWriteAction, WriteNotice} from './write-action';
import {useTaskConversationRead} from './task-conversation-hooks';
import {validConversationBody, validConversationReceipt, validConversationRequest} from './task-conversation-model';
import {TaskConversationReply} from './TaskConversationReply';
import './TaskConversation.css';

type Props = {session: Session; project: string; taskId: string; refresh: number; accessBlocked: boolean;
  onDevelopmentStarted?: () => Promise<unknown> | void};
const t = (key: string, values?: Record<string, unknown>) => tr('task-conversation.' + key, values);

/** This is an immutable human/API discussion, not a synthetic agent transcript.
 * Posting never dispatches work, changes task scope or accepts a delivery. */
export function TaskConversation(props: Props) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  const scope = JSON.stringify([props.session.user.id, props.session.organization.id, props.project, props.taskId]);
  return props.taskId ? <ConversationScope key={scope + ':' + caller.current.epoch} {...props} scope={scope}/> : null;
}

function ConversationScope({session, project, taskId, refresh, scope, accessBlocked, onDevelopmentStarted}: Props & {scope: string}) {
  useLocale();
  const api = useApi(), bodyID = useId();
  const [open, setOpen] = useState(false), [draft, setDraft] = useState('');
  const [revoked, setRevoked] = useState(false), [accessRetry, setAccessRetry] = useState(0), [replyLocked, setReplyLocked] = useState(false);
  const [saved, setSaved] = useState<TaskConversationMessage>(), [blocked, setBlocked] = useState(false);
  const action = useWriteAction({scope: 'task-conversation:' + scope, storage: toolSessionStorage(),
    validateRequest: request => validConversationRequest(request, project, taskId),
    validateResponse: (value, request) => validConversationReceipt(value, request, session.user.id)});
  const writeDenied = action.errorStatus === 401 || action.errorStatus === 403;
  const read = useTaskConversationRead(project, taskId, open && !writeDenied && !revoked && !accessBlocked, refresh + accessRetry);
  const owner = useRef(api), alive = useRef(true);
  owner.current = api;
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const pendingBody = action.pending?.body.body;
  const text = typeof pendingBody === 'string' ? pendingBody : draft;
  const locked = action.busy || !!action.pending;
  const denied = accessBlocked || !!read.denied || writeDenied || revoked;
  const page = denied ? undefined : read.page;
  useLeaveGuard(!!draft && !action.pending, action.setError);
  useEffect(() => { if (action.pending) setOpen(true); }, [!!action.pending]);
  // A rejected write revokes the visible read owner too. In-flight GETs are
  // cancelled/ignored, and another local diagnostic cannot clear this boundary.
  useEffect(() => { if (writeDenied) setRevoked(true); }, [writeDenied]);

  function toggle() {
    if (open && (text || locked || replyLocked)) { setBlocked(true); return; }
    setOpen(value => !value); setBlocked(false);
  }
  function follow(event: MouseEvent<HTMLAnchorElement>) {
    if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) {
      event.preventDefault(); setBlocked(true);
    }
  }
  async function acknowledged(value: TaskConversationMessage | undefined, caller: typeof api) {
    if (!value || !alive.current || owner.current !== caller) return;
    setDraft(''); setSaved(value); setBlocked(false);
    await read.read();
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (locked || denied || !page?.can_post || !validConversationBody(draft)) return;
    setSaved(undefined); const caller = api;
    await acknowledged(await action.run<TaskConversationMessage>('/task-conversations', {
      project_id: project, task_id: taskId, expected_task_version: page.task_version, body: draft,
    }), caller);
  }
  async function restore() {
    const original = action.pending;
    if (!original || denied || !page?.can_post) return;
    const caller = api;
    await acknowledged(await action.run<TaskConversationMessage>(original.path, original.body, original.method), caller);
  }
  async function reloadBasis() {
    const value = await read.read();
    if (!value) throw new UiError('task-conversation.refresh_failed');
  }

  return <section className="task-conversation" aria-label={t('title')}>
    <h3><button type="button" className="task-conversation-toggle" aria-expanded={open}
      aria-controls={bodyID} disabled={open && (locked || replyLocked)} onClick={toggle} data-conversation-toggle>
      <MessageSquare size={15} aria-hidden="true"/><span>{t('title')}</span>
      <ChevronDown size={15} className={open ? 'is-open' : ''} aria-hidden="true"/>
    </button></h3>
    {blocked && <p className="task-conversation-warning" role="status">{t('draft_guard')}</p>}
    {open && <div id={bodyID} className="task-conversation-body" aria-busy={!!read.loading}>
      <p className="task-conversation-hint">{t('boundary')}</p>
      {denied ? <div role="alert" className="task-conversation-warning"><p>{t('access_changed')}</p>
        {!accessBlocked && <button type="button" onClick={() => { action.setError(''); setRevoked(false); setAccessRetry(value => value + 1); }}>{t('check_access')}</button>}
        {accessBlocked && <p>{t('check_parent_access')}</p>}
        {!!draft && !action.pending && <p>{t('private_draft')}</p>}
        {!!text && <button type="button" disabled={locked} onClick={() => {setDraft(''); setBlocked(false);}}>{t('clear_draft')}</button>}
        {action.pending && <p>{t('pending_hint')}</p>}
      </div> : <>
        <WriteNotice action={action} onRebase={reloadBasis}/>
        {action.pending && <div className="task-conversation-recovery" role="status">
          <strong>{t('original_pending')}</strong><p>{t('pending_hint')}</p>
          <button type="button" disabled={action.busy || !page?.can_post} onClick={() => void restore()} data-conversation-restore>{t('restore')}</button>
        </div>}
        {saved && <p className="task-conversation-success" role="status">{t('saved')}</p>}
        {read.loading && <Loading/>}
        {!!read.error && <ErrorNotice error={read.error} onRetry={() => void read.read()}/>}
        {page && <>
          <div className="task-conversation-history-heading"><span>{t('loaded', {loaded: formatNumber(page.items.length), total: formatNumber(page.total)})}</span>
            <button type="button" disabled={!!read.loading || action.busy} onClick={() => void read.read()} aria-label={t('refresh')}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></div>
          {!!page.next_before && <button type="button" disabled={!!read.loading} onClick={() => void read.read(true)} data-conversation-more>{t('older')}</button>}
          {page.items.length ? <ol className="task-conversation-messages" aria-label={t('messages')}>
            {page.items.map(message => <li key={message.id} data-conversation-message>
              <article><header><strong>{message.author_name || t('unknown_author')}</strong>
                <span className="task-conversation-actor">{t('actor.' + message.actor_kind)}</span>
                <time dateTime={message.created_at}>{formatDateTime(message.created_at, {dateStyle: 'short', timeStyle: 'short'})}</time></header>
                <p className="task-conversation-message-text">{message.body}</p>
                <details><summary>{t('basis')}</summary><div className="task-conversation-basis">
                  <span>{t('task_version', {version: formatNumber(message.task_version)})}</span>
                  {message.requirement_id && <span>{t('requirement_version', {version: formatNumber(message.requirement_version)})}</span>}
                  {message.run_id && <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: message.run_id})} onClick={follow}>{t('original_run')}</a>}
                  {message.reply_id && <span>{t('reply_to', {sequence: formatNumber(message.reply_to_sequence)})}</span>}
                  {message.execution_run_id && <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: message.execution_run_id})} onClick={follow}>{t('reply_execution')}</a>}
                </div></details>
              </article>
            </li>)}
          </ol> : <p className="task-conversation-empty">{t('empty')}</p>}
          {!page.can_post && <p className="task-conversation-hint" role="status">{t('read_only')}</p>}
          <form onSubmit={event => void submit(event)} data-conversation-form>
            <label htmlFor={bodyID + '-draft'}>{t('draft_label')}</label>
            <small className="task-conversation-hint">{t('task_version', {version: formatNumber(page.task_version)})}</small>
            <textarea id={bodyID + '-draft'} name="conversation_body" value={text} maxLength={16000}
              rows={3} disabled={locked || !page.can_post || action.conflict} placeholder={t('placeholder')}
              aria-describedby={bodyID + '-hint'} onChange={event => {setDraft(event.target.value); setSaved(undefined);}}/>
            <div className="task-conversation-compose-footer"><small id={bodyID + '-hint'}>{t('characters', {count: formatNumber(Array.from(text).length)})}</small>
              <div><button type="button" disabled={locked || !text} onClick={() => {setDraft(''); setBlocked(false);}}>{t('clear_draft')}</button>
                <button type="submit" className="primary" disabled={locked || !page.can_post || action.conflict || !validConversationBody(draft)}><Send size={14} aria-hidden="true"/>{action.busy ? t('sending') : t('send')}</button></div>
            </div>
          </form>
        </>}
      </>}
      <TaskConversationReply session={session} project={project} taskId={taskId} page={page}
        accessBlocked={denied} disabled={locked || !!text || !!read.error || !!read.loading || !page?.can_post}
        refresh={refresh} onMessagesRefresh={read.read} onLockChange={setReplyLocked} onDevelopmentStarted={onDevelopmentStarted}/>
    </div>}
  </section>;
}
