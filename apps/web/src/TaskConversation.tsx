import {useCallback, useEffect, useId, useRef, useState, type FormEvent, type MouseEvent} from 'react';
import {ChevronDown, MessageSquare, RefreshCw, Send} from 'lucide-react';
import type {TaskConversationMessage} from '../../../packages/contracts/src';
import type {Session} from './api';
import {useApi} from './api-context';
import {tr, useLocale, UiError, formatDateTime, formatNumber} from './i18n';
import {ErrorNotice, Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard, useWriteAction, WriteNotice} from './write-action';
import {useTaskConversationMessage, useTaskConversationRead} from './task-conversation-hooks';
import {validConversationBody, validConversationReceipt, validConversationRequest} from './task-conversation-model';
import {conversationDraftKey, readConversationDraft, saveConversationDraft} from './task-conversation-mentions-model';
import {mentionSelectionKey, TaskConversationMentions} from './TaskConversationMentions';
import {TaskConversationKnowledge, RecordedConversationKnowledge} from './TaskConversationKnowledge';
import {knowledgeSelectionKey} from './task-conversation-knowledge-model';
import type {KnowledgeReference} from './knowledge-model';
import {TaskConversationReply} from './TaskConversationReply';
import {TaskSubscriptionControl} from './TaskSubscriptionControl';
import {CloudHistorySharedSource} from './CloudHistorySharedSource';
import {TurnSteeringPanel} from './TurnSteeringPanel';
import './TaskConversation.css';

type Props = {session: Session; project: string; taskId: string; refresh: number; accessBlocked: boolean;
  selectedMessage?: string; onAccessDenied?: () => void; onDevelopmentStarted?: () => Promise<unknown> | void};
const t = (key: string, values?: Record<string, unknown>) => tr('task-conversation.' + key, values);
const mt = (key: string, values?: Record<string, unknown>) => tr('task-conversation-mentions.' + key, values);

/** This is an immutable human/API discussion, not a synthetic agent transcript.
 * Posting never dispatches work, changes task scope or accepts a delivery. */
export function TaskConversation(props: Props) {
  const api = useApi(), caller = useRef({api, epoch: 0});
  if (caller.current.api !== api) caller.current = {api, epoch: caller.current.epoch + 1};
  const scope = JSON.stringify([props.session.user.id, props.session.organization.id, props.project, props.taskId]);
  return props.taskId ? <ConversationScope key={scope + ':' + caller.current.epoch} {...props} scope={scope}/> : null;
}

function ConversationScope({session, project, taskId, refresh, scope, accessBlocked, selectedMessage = '', onAccessDenied, onDevelopmentStarted}: Props & {scope: string}) {
  useLocale();
  const api = useApi(), bodyID = useId();
  const storage = toolSessionStorage(), draftKey = conversationDraftKey(session.user.id, session.organization.id, project, taskId);
  const restored = useRef(readConversationDraft(storage, draftKey));
  const [open, setOpen] = useState(!!selectedMessage || !!restored.current.draft.body || !!restored.current.draft.mention_resource_ids.length || !!restored.current.draft.knowledge?.length || !!restored.current.invalid);
  const [draft, setDraft] = useState(restored.current.draft.body), [mentionIDs, setMentionIDs] = useState(restored.current.draft.mention_resource_ids);
  const [knowledge, setKnowledge] = useState<KnowledgeReference[]>(restored.current.draft.knowledge || []);
  const [knowledgeValidity, setKnowledgeValidity] = useState<{key: string; valid: boolean}>();
  const [invalidDraft, setInvalidDraft] = useState(!!restored.current.invalid), [draftStorageFailed, setDraftStorageFailed] = useState(false);
  const [mentionValidity, setMentionValidity] = useState<{key: string; valid: boolean}>();
  const [revoked, setRevoked] = useState(false), [accessRetry, setAccessRetry] = useState(0), [replyLocked, setReplyLocked] = useState(false);
  const [steerMessage, setSteerMessage] = useState(''), [steerLocked, setSteerLocked] = useState(false);
  const steeringRegion = useRef<HTMLDivElement>(null);
  const [saved, setSaved] = useState<TaskConversationMessage>(), [blocked, setBlocked] = useState(false);
  const action = useWriteAction({scope: 'task-conversation:' + scope, storage: toolSessionStorage(),
    validateRequest: request => validConversationRequest(request, project, taskId),
    validateResponse: (value, request) => validConversationReceipt(value, request, session.user.id)});
  const writeDenied = !!action.errorStatus && [401, 403, 404].includes(action.errorStatus);
  const read = useTaskConversationRead(project, taskId, open && !writeDenied && !revoked && !accessBlocked, refresh + accessRetry);
  const located = useTaskConversationMessage(project, taskId, selectedMessage, open && !writeDenied && !revoked && !accessBlocked, refresh + accessRetry);
  const owner = useRef(api), alive = useRef(true);
  owner.current = api;
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const pendingBody = action.pending?.body.body;
  const text = typeof pendingBody === 'string' ? pendingBody : draft;
  const selectedIDs = Array.isArray(action.pending?.body.mention_resource_ids) ? action.pending.body.mention_resource_ids as string[] : action.pending ? [] : mentionIDs;
  const selectedKnowledge = Array.isArray(action.pending?.body.knowledge) ? action.pending.body.knowledge as KnowledgeReference[] : action.pending ? [] : knowledge;
  const locked = action.busy || !!action.pending;
  const interactionLocked = locked || replyLocked || steerLocked;
  const denied = accessBlocked || !!read.denied || !!located.denied || writeDenied || revoked;
  const page = denied ? undefined : read.page;
  const draftField = useRef<HTMLTextAreaElement>(null), continuationReady = useRef(false);
  continuationReady.current = !!page && page.project_id === project && page.task_id === taskId && page.can_post &&
    !denied && !interactionLocked && !invalidDraft && !draftStorageFailed && !action.conflict && !action.persistenceBlocked &&
    !read.loading && !read.error && !located.loading && !located.error;
  const continueDevelopment = useCallback(() => {
    const field = draftField.current;
    if (!alive.current || owner.current !== api || !continuationReady.current || !field?.isConnected || field.disabled) return;
    // Continuing an idea is navigation within this task. The original draft,
    // mentions and knowledge remain owner-authored until explicitly saved.
    field.focus(); field.scrollIntoView?.({block: 'nearest'});
  }, [api]);
  const dirty = !!draft || !!mentionIDs.length || !!knowledge.length || invalidDraft;
  const mentionsReady = !mentionIDs.length || mentionValidity?.key === mentionSelectionKey(mentionIDs, refresh + accessRetry) && mentionValidity.valid;
  const knowledgeReady = !knowledge.length || knowledgeValidity?.key === knowledgeSelectionKey(knowledge, refresh + accessRetry) && knowledgeValidity.valid;
  const changeValidity = useCallback((key: string, valid: boolean) => setMentionValidity(previous => previous?.key === key && previous.valid === valid ? previous : {key, valid}), []);
  const changeKnowledgeValidity = useCallback((key: string, valid: boolean) => setKnowledgeValidity(previous => previous?.key === key && previous.valid === valid ? previous : {key, valid}), []);
  const directoryDenied = useCallback(() => { setRevoked(true); onAccessDenied?.(); }, [onAccessDenied]);
  useLeaveGuard((dirty || draftStorageFailed) && !action.pending, action.setError);
  useEffect(() => {
    if (invalidDraft) return;
    try { if (storage || dirty) saveConversationDraft(storage, draftKey, {body: draft, mention_resource_ids: mentionIDs, ...(knowledge.length ? {knowledge} : {})}); setDraftStorageFailed(false); }
    catch { setDraftStorageFailed(true); }
  }, [storage, draftKey, draft, mentionIDs, knowledge, invalidDraft]);
  useEffect(() => { if (selectedMessage) setOpen(true); }, [selectedMessage]);
  useEffect(() => { if (action.pending) setOpen(true); }, [!!action.pending]);
  useEffect(() => { if (steerLocked) setOpen(true); }, [steerLocked]);
  useEffect(() => { if (steerMessage && !denied) {steeringRegion.current?.focus(); steeringRegion.current?.scrollIntoView?.({block: 'nearest'});} }, [steerMessage, denied]);
  // A rejected write revokes the visible read owner too. In-flight GETs are
  // cancelled/ignored, and another local diagnostic cannot clear this boundary.
  useEffect(() => { if (writeDenied || read.denied || located.denied) { setSaved(undefined); setRevoked(true); onAccessDenied?.(); } }, [writeDenied, read.denied, located.denied, onAccessDenied]);
  const focused = useRef<HTMLElement | undefined>(undefined);
  useEffect(() => {
    if (!selectedMessage) { focused.current = undefined; return; }
    if (denied || !open) return;
    const target = document.getElementById(bodyID + '-message-' + selectedMessage);
    if (target && target !== focused.current) { target.focus(); target.scrollIntoView?.({block: 'nearest'}); focused.current = target; }
  }, [selectedMessage, denied, open, page, located.value, bodyID]);

  function clearDraft() { setDraft(''); setMentionIDs([]); setKnowledge([]); setBlocked(false); }
  function retryDraftStorage(clearInvalid = false) {
    try {
      saveConversationDraft(storage, draftKey, clearInvalid ? {body: '', mention_resource_ids: []} : {body: draft, mention_resource_ids: mentionIDs, ...(knowledge.length ? {knowledge} : {})});
      setDraftStorageFailed(false); if (clearInvalid) {setInvalidDraft(false); clearDraft();}
    } catch {setDraftStorageFailed(true);}
  }

  function toggle() {
    if (open && (text || selectedIDs.length || selectedKnowledge.length || invalidDraft || draftStorageFailed || interactionLocked)) { setBlocked(true); return; }
    setOpen(value => !value); setBlocked(false);
  }
  function follow(event: MouseEvent<HTMLAnchorElement>) {
    if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) {
      event.preventDefault(); setBlocked(true);
    }
  }
  async function acknowledged(value: TaskConversationMessage | undefined, caller: typeof api) {
    if (!value || !alive.current || owner.current !== caller) return;
    clearDraft(); setSaved(value);
    await read.read();
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (interactionLocked || denied || invalidDraft || !mentionsReady || !knowledgeReady || !page?.can_post || !validConversationBody(draft)) return;
    setSaved(undefined); const caller = api;
    await acknowledged(await action.run<TaskConversationMessage>('/task-conversations', {
      project_id: project, task_id: taskId, expected_task_version: page.task_version, body: draft,
      ...(mentionIDs.length ? {mention_resource_ids: mentionIDs} : {}),
      ...(knowledge.length ? {knowledge} : {}),
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

  function renderMessage(message: TaskConversationMessage) {
    const ownHuman = message.actor_kind === 'human' && message.author_id === session.user.id;
    return <li key={message.id} id={bodyID + '-message-' + message.id} tabIndex={-1}
      className={message.id === selectedMessage ? 'is-located' : ''} data-conversation-message={message.id}>
      <article><header><strong>{message.author_name || t('unknown_author')}</strong>
        <span className="task-conversation-actor">{t('actor.' + message.actor_kind)}</span>
        <time dateTime={message.created_at}>{formatDateTime(message.created_at, {dateStyle: 'short', timeStyle: 'short'})}</time></header>
        <p className="task-conversation-message-text">{message.body}</p>
        {ownHuman && <div className="task-conversation-message-actions"><button type="button" data-conversation-steer={message.id}
          aria-pressed={steerMessage === message.id} disabled={interactionLocked || denied || !page?.can_post}
          onClick={() => {if (alive.current && owner.current === api && !interactionLocked && !denied && page?.can_post) setSteerMessage(message.id);}}>{tr('turn-steering.entry')}</button></div>}
        {message.cloud_observation&&<CloudHistorySharedSource source={message.cloud_observation}/>}
        {!!message.mentions?.length && <div className="task-conversation-recorded-mentions"><small>{mt('recorded')}</small><ul aria-label={mt('recorded')}>{message.mentions.map(mention => <li key={mention.resource_id}>{mention.name_at_post || mention.resource_id}</li>)}</ul></div>}
        <RecordedConversationKnowledge project={project} items={message.knowledge} refresh={refresh + accessRetry} onAccessDenied={directoryDenied}/>
        <details><summary>{t('basis')}</summary><div className="task-conversation-basis">
          <span>{t('task_version', {version: formatNumber(message.task_version)})}</span>
          {message.requirement_id && <span>{t('requirement_version', {version: formatNumber(message.requirement_version)})}</span>}
          {message.run_id && <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: message.run_id})} onClick={follow}>{t('original_run')}</a>}
          {message.reply_id && <span>{t('reply_to', {sequence: formatNumber(message.reply_to_sequence)})}</span>}
          {message.execution_run_id && <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: message.execution_run_id})} onClick={follow}>{t('reply_execution')}</a>}
        </div></details>
      </article>
    </li>;
  }

  return <section className="task-conversation" aria-label={t('title')}>
    <div className="task-conversation-heading"><h3><button type="button" className="task-conversation-toggle" aria-expanded={open}
      aria-controls={bodyID} disabled={open && interactionLocked} onClick={toggle} data-conversation-toggle>
      <MessageSquare size={15} aria-hidden="true"/><span>{t('title')}</span>
      <ChevronDown size={15} className={open ? 'is-open' : ''} aria-hidden="true"/>
    </button></h3><TaskSubscriptionControl session={session} project={project} taskId={taskId} refresh={refresh+accessRetry} accessBlocked={denied} onAccessDenied={directoryDenied}/></div>
    {blocked && <p className="task-conversation-warning" role="status">{t('draft_guard')}</p>}
    <div id={bodyID} className="task-conversation-body" hidden={!open} aria-busy={!!read.loading}>
      {open && <>
      <p className="task-conversation-hint">{t('boundary')}</p>
      {draftStorageFailed && <div className="task-conversation-warning" role="alert"><p>{mt('draft_storage')}</p><button type="button" disabled={interactionLocked} onClick={() => retryDraftStorage()}>{mt('retry_draft')}</button></div>}
      {invalidDraft && <div className="task-conversation-warning" role="alert"><p>{mt('draft_invalid')}</p><button type="button" disabled={interactionLocked} onClick={() => retryDraftStorage(true)}>{mt('clear_saved_draft')}</button></div>}
      {denied ? <div role="alert" className="task-conversation-warning"><p>{t('access_changed')}</p>
        {!accessBlocked && <button type="button" onClick={() => { action.setError(''); setRevoked(false); setAccessRetry(value => value + 1); }}>{t('check_access')}</button>}
        {accessBlocked && <p>{t('check_parent_access')}</p>}
        {dirty && !action.pending && <p>{t('private_draft')}</p>}
        {(!!text || !!selectedIDs.length || !!selectedKnowledge.length) && <button type="button" disabled={interactionLocked} onClick={clearDraft}>{t('clear_draft')}</button>}
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
        {located.loading && <Loading/>}
        {!!located.error && <ErrorNotice error={located.error} onRetry={() => void located.read()}/>}
        {located.value && !page?.items.some(message => message.id === selectedMessage) && <section className="task-conversation-located" aria-label={mt('located')}><strong>{mt('located')}</strong><p className="task-conversation-hint">{mt('located_separate')}</p><ol className="task-conversation-messages">{renderMessage(located.value.message)}</ol></section>}
        {page && <>
          <div className="task-conversation-history-heading"><span>{t('loaded', {loaded: formatNumber(page.items.length), total: formatNumber(page.total)})}</span>
            <button type="button" disabled={!!read.loading || action.busy} onClick={() => void read.read()} aria-label={t('refresh')}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></div>
          {!!page.next_before && <button type="button" disabled={!!read.loading} onClick={() => void read.read(true)} data-conversation-more>{t('older')}</button>}
          {page.items.length ? <ol className="task-conversation-messages" aria-label={t('messages')}>
            {page.items.map(renderMessage)}
          </ol> : <p className="task-conversation-empty">{t('empty')}</p>}
          {!page.can_post && <p className="task-conversation-hint" role="status">{t('read_only')}</p>}
          <form onSubmit={event => void submit(event)} data-conversation-form>
            <label htmlFor={bodyID + '-draft'}>{t('draft_label')}</label>
            <small className="task-conversation-hint">{t('task_version', {version: formatNumber(page.task_version)})}</small>
            <textarea ref={draftField} id={bodyID + '-draft'} name="conversation_body" value={text} maxLength={16000}
              rows={3} disabled={interactionLocked || invalidDraft || !page.can_post || action.conflict} placeholder={t('placeholder')}
              aria-describedby={bodyID + '-hint'} onChange={event => {setDraft(event.target.value); setSaved(undefined);}}/>
            <TaskConversationMentions project={project} taskID={taskId} userID={session.user.id} organizationID={session.organization.id}
              selectedIDs={selectedIDs} onChange={ids => {setMentionIDs(ids); setSaved(undefined);}} disabled={interactionLocked || invalidDraft || !page.can_post || action.conflict}
              accessBlocked={denied} refresh={refresh + accessRetry} onValidityChange={changeValidity} onAccessDenied={directoryDenied}/>
            <TaskConversationKnowledge project={project} taskID={taskId} userID={session.user.id} organizationID={session.organization.id}
              selected={selectedKnowledge} onChange={pins => {setKnowledge(pins);setSaved(undefined);}} disabled={interactionLocked || invalidDraft || !page.can_post || action.conflict}
              accessBlocked={denied} refresh={refresh + accessRetry} onValidityChange={changeKnowledgeValidity} onAccessDenied={directoryDenied}/>
            <div className="task-conversation-compose-footer"><small id={bodyID + '-hint'}>{t('characters', {count: formatNumber(Array.from(text).length)})}</small>
              <div><button type="button" disabled={interactionLocked || !text && !selectedIDs.length && !selectedKnowledge.length} onClick={clearDraft}>{t('clear_draft')}</button>
                <button type="submit" className="primary" disabled={interactionLocked || invalidDraft || !mentionsReady || !knowledgeReady || !page.can_post || action.conflict || !validConversationBody(draft)}><Send size={14} aria-hidden="true"/>{action.busy ? t('sending') : t('send')}</button></div>
            </div>
          </form>
        </>}
      </>}
      </>}
      <div ref={steeringRegion} tabIndex={-1} className="task-conversation-steering-region">
        <TurnSteeringPanel session={session} project={project} taskID={taskId} selectedMessage={steerMessage}
          refresh={refresh + accessRetry} accessBlocked={denied} disabled={locked || replyLocked || !!read.error || !!read.loading || !page?.can_post}
          onLockChange={setSteerLocked} onAccessDenied={directoryDenied}/>
      </div>
      {open && <TaskConversationReply session={session} project={project} taskId={taskId} page={page}
        accessBlocked={denied} disabled={locked || steerLocked || dirty || !!text || !!selectedIDs.length || !!selectedKnowledge.length || !!read.error || !!read.loading || !page?.can_post}
        refresh={refresh} onMessagesRefresh={read.read} onLockChange={setReplyLocked} onDevelopmentStarted={onDevelopmentStarted}
        onContinueDevelopment={continueDevelopment}/>}
    </div>
  </section>;
}
