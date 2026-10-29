import {useCallback, useEffect, useId, useRef, useState} from 'react';
import {ArrowUpRight, ChevronDown, Plus, RefreshCw, Square, TerminalSquare} from 'lucide-react';
import type {WorkspaceTerminalSession, WorkspaceTerminalPage, WorkspaceTerminalCommandReceipt} from '../../../packages/contracts/src';
import {query} from './api';
import {useApi, useApiView} from './api-context';
import {formatDateTime, formatNumber, renderDiagnostic, tr, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {useWriteAction, WriteNotice} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {useReplyRead} from './task-conversation-reply-hooks';
import {terminalID, validTerminalCommand, validTerminalPage, validTerminalRecovery, validTerminalSession} from './terminal-model';
import {useTerminalConnection, type TerminalWriter} from './terminal-hooks';
import {TerminalViewport} from './TerminalViewport';
import {TerminalRecoveryPanel} from './TerminalRecoveryPanel';
import {TerminalCandidatePanel} from './TerminalCandidatePanel';
import './TerminalPanel.css';

const t = (key: string, values?: Record<string, unknown>) => tr('terminal.' + key, values);
export type TerminalPanelProps = {project: string; taskId?: string; active?: boolean; disabled?: boolean};

/** Browser and desktop share this surface. A selected IDE task is context,
 * never permission to launch a shell in the editor's shared directory. */
export function TerminalPanel(props: TerminalPanelProps) {
  const api = useApi(), view = useApiView(), owner = useRef({api, epoch: 0}); useLocale();
  if (owner.current.api !== api) owner.current = {api, epoch: owner.current.epoch + 1};
  const scope = JSON.stringify([view?.userID || '', view?.organizationID || '', props.project, props.taskId || '']);
  return <TerminalScope key={scope + ':' + owner.current.epoch} {...props} scope={scope}/>;
}

function TerminalScope({project, taskId = '', active = true, disabled = false, scope}: TerminalPanelProps & {scope: string}) {
  const api = useApi(); useLocale(); const id = useId(), storage = toolSessionStorage(), locator = 'suma-terminal-session:' + scope;
  const [open, setOpen] = useState(false), [visited, setVisited] = useState(false), [detached, setDetached] = useState(false);
  const [selected, setSelected] = useState(() => {try {const value = storage?.getItem(locator); return terminalID(value) ? value : '';} catch {return '';}});
  const [profile, setProfile] = useState(''), [seconds, setSeconds] = useState('1200'), [consent, setConsent] = useState(false), [stopConsent, setStopConsent] = useState(false);
  const [denied, setDenied] = useState(false), [refresh, setRefresh] = useState(0), [observed, setObserved] = useState<WorkspaceTerminalSession>();
  const [writer, setWriter] = useState<TerminalWriter>();
  const [recoveryLocked, setRecoveryLocked] = useState(false);
  const [candidateLocked, setCandidateLocked] = useState(false);
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  const refuse = useCallback(() => setDenied(true), []);
  const action = useWriteAction({scope: 'terminal-controls:' + scope, storage,
    validateRequest: request => validTerminalRecovery(request, project, taskId),
    validateResponse: (value, request) => {
      if (request.path === '/workspace/terminal-sessions') return validTerminalSession(value, project, taskId) &&
        value.task_version === request.body.expected_task_version && value.tool_profile_id === request.body.tool_profile_id;
      const match = /^\/workspace\/terminal-sessions\/([A-Za-z0-9_.-]+)\/commands$/.exec(request.path);
      return !!match && validTerminalCommand(value, project, taskId, match[1]) && value.input === null &&
        value.session.control_epoch === request.body.control_epoch && value.session.version > Number(request.body.expected_version) &&
        (request.body.action !== 'ack' || value.session.output_ack_sequence >= Number(request.body.output_sequence));
    }});
  const locked = action.busy || !!action.pending || recoveryLocked || candidateLocked, blocked = denied || action.errorStatus === 401 || action.errorStatus === 403;
  const sessions = useReplyRead<WorkspaceTerminalPage>('/workspace/terminal-sessions' + query(project) + '&task_id=' + encodeURIComponent(taskId),
    open && active && !!taskId && !blocked, (value): value is WorkspaceTerminalPage => validTerminalPage(value, project, taskId), refuse, refresh);
  const detail = useReplyRead('/workspace/terminal-sessions/' + encodeURIComponent(selected) + query(project),
    open && active && !!taskId && !!selected && !blocked, (value): value is WorkspaceTerminalSession => validTerminalSession(value, project, taskId) && value.id === selected, refuse, refresh);
  const listedSessions = sessions.value?.items || [];
  const session = [listedSessions.find(item => item.id === selected), detail.value, observed]
    .filter((item): item is WorkspaceTerminalSession => !!item && item.id === selected)
    .reduce<WorkspaceTerminalSession | undefined>((latest, item) => !latest || item.version >= latest.version ? item : latest, undefined);
  // A successful creation/command is already an authorized current fact. Keep
  // its option visible immediately, without waiting for a list refresh, and
  // never downgrade its state to an older catalog snapshot.
  const sessionOptions = listedSessions.map(item => session && session.id === item.id && session.version >= item.version ? session : item);
  if (session && !sessionOptions.some(item => item.id === session.id)) sessionOptions.unshift(session);
  const sessionChange = useCallback((value: WorkspaceTerminalSession) => setObserved(previous => previous?.id === value.id && previous.version > value.version ? previous : value), []);
  const controlSender = useCallback((body: Record<string, unknown>) => action.run<WorkspaceTerminalCommandReceipt>(
    '/workspace/terminal-sessions/' + encodeURIComponent(selected) + '/commands', body), [selected, action.run]);
  const runtime = useTerminalConnection({project, task: taskId, session,
    active: open && active && !blocked && !detached && !detail.loading && !detail.error, writer,
    controlBlocked: locked || disabled || blocked, sendControl: controlSender, onSession: sessionChange, onDenied: refuse});
  const setConsumer = useCallback((value?: TerminalWriter) => setWriter(() => value), []);
  const resize = useCallback((columns: number, rows: number) => {
    if (columns < 20 || columns > 500 || rows < 5 || rows > 200 || !open || !active || detached || blocked) return;
    if (session?.columns !== columns || session?.rows !== rows) void runtime.control('resize', {columns, rows});
  }, [open, active, detached, blocked, session?.columns, session?.rows, runtime.control]);
  useEffect(() => {alive.current = true; return () => {alive.current = false;};}, []);
  useEffect(() => {if (action.pending) {setOpen(true); setVisited(true);}}, [!!action.pending]);
  useEffect(() => {setStopConsent(false);}, [selected, session?.state]);
  function select(value: string) {
    if (locked) return;
    setWriter(undefined); setSelected(value); setObserved(undefined); setDetached(false); setConsent(false); setStopConsent(false);
    try {if (value) storage?.setItem(locator, value); else storage?.removeItem(locator);} catch {/* A locator is only a preference; the server retains the session. */}
  }
  function remember(value: WorkspaceTerminalSession) {
    if (value.id !== selected) setWriter(undefined);
    setSelected(value.id); setObserved(value); setDetached(false); setVisited(true); setConsent(false);
    try {storage?.setItem(locator, value.id);} catch {/* No shell bytes or permissions are stored here. */}
  }
  async function create() {
    const page = sessions.value, tool = page?.profiles.find(item => item.id === profile), duration = Number(seconds);
    if (!page?.can_create || !tool || !tool.available || tool.state !== 'enabled' || !consent || blocked || locked || disabled || !/^[1-9]\d*$/.test(seconds) || duration < 60 || duration > 3600) return;
    const caller = api;
    const value = await action.run<WorkspaceTerminalSession>('/workspace/terminal-sessions', {project_id: project, task_id: taskId,
      expected_task_version: page.task_version, tool_profile_id: tool.id, expected_tool_version: tool.version, columns: 80, rows: 24,
      timeout_seconds: duration, acknowledge_shell: true});
    if (value && alive.current && owner.current === caller) remember(value);
  }
  async function restore() {
    const original = action.pending;
    if (!original || disabled || blocked) return;
    const caller = api;
    const value = await runtime.exclusive(() => action.run<unknown>(original.path, original.body, original.method));
    if (!value || !alive.current || owner.current !== caller) return;
    if (validTerminalSession(value, project, taskId)) remember(value);
    else if (value && typeof value === 'object' && 'session' in value && validTerminalSession(value.session, project, taskId)) remember(value.session);
  }
  const selectedProfile = sessions.value?.profiles.find(tool => tool.id === profile);
  const canCreate = !!sessions.value?.can_create && selectedProfile?.available && selectedProfile.state === 'enabled' && consent && !locked && !disabled && !blocked && /^[1-9]\d*$/.test(seconds) && Number(seconds) >= 60 && Number(seconds) <= 3600;
  return <section className="workspace-terminal" aria-label={t('title')}>
    <button type="button" className="terminal-toggle" data-terminal-toggle aria-expanded={open} aria-controls={id}
      onClick={() => {setVisited(true); setOpen(value => !value);}}><TerminalSquare size={15}/><span>{t('title')}</span><ChevronDown size={15}/></button>
    {visited && <div className="terminal-body" id={id} hidden={!open}>
      <p className="terminal-note">{t('intro')}</p>
      {!taskId ? <p>{t('select_task')}</p> : blocked ? <div role="alert"><p className="terminal-warning">{t('access_changed')}</p>
        <button type="button" onClick={() => {action.setError(''); setDenied(false); setRefresh(value => value + 1);}}>{t('check_access')}</button></div> : <>
        <WriteNotice action={action} onRebase={async () => {await detail.read(); await sessions.read(); action.clearConflict();}}/>
        {action.pending && <div role="status"><p>{t('pending')}</p><button type="button" disabled={action.busy || disabled} onClick={() => void restore()}>{t('original')}</button></div>}
        <div className="terminal-session-list"><label htmlFor={id + '-sessions'}>{t('sessions')}</label>
          <select id={id + '-sessions'} value={selected} disabled={locked || sessions.loading} onChange={event => select(event.target.value)}>
            <option value="">{t('choose_session')}</option>{sessionOptions.map(item => <option key={item.id} value={item.id}>{item.tool_name} · {t('state.' + item.state)}</option>)}</select>
          <button type="button" disabled={locked || sessions.loading} onClick={() => {setRefresh(value => value + 1); setConsent(false);}}><RefreshCw size={13}/>{t('refresh')}</button>
          {selected && <button type="button" disabled={locked} onClick={() => select('')}><Plus size={13}/>{t('new_session')}</button>}</div>
        {sessions.value && <p className="terminal-note">{t('recent_sessions')}</p>}
        {sessions.loading && !session && <Loading/>}{!!sessions.error && <ErrorNotice error={sessions.error} onRetry={() => void sessions.read()}/>}
        {!!detail.error && <ErrorNotice error={detail.error} onRetry={() => void detail.read()}/>}
        {!selected && sessions.value && <><p className="terminal-note">{!sessions.value.items.length ? t('no_sessions') : t('detach_hint')}</p>
          {sessions.value.can_create && <div className="terminal-create"><Field label={t('agent')}><select value={profile} disabled={locked || disabled} onChange={event => {setProfile(event.target.value); setConsent(false);}}>
            <option value="">{t('choose_profile')}</option>{sessions.value.profiles.map(tool => <option key={tool.id} value={tool.id} disabled={!tool.available || tool.state !== 'enabled'}>{tool.name} · v{formatNumber(tool.version)}</option>)}</select></Field>
            <Field label={t('duration')} hint={t('duration_hint')}><input inputMode="numeric" value={seconds} disabled={locked || disabled} onChange={event => {setSeconds(event.target.value); setConsent(false);}}/></Field>
            {!sessions.value.profiles.length && <p>{t('no_profile')}</p>}
            {!sessions.value.profiles_complete && <p className="terminal-warning">{t('profiles_partial')}</p>}
            <label className="terminal-create-consent"><input type="checkbox" checked={consent} disabled={locked || disabled} onChange={event => setConsent(event.target.checked)}/><span>{t('consent')}</span></label>
            <button type="button" data-terminal-create className="primary" disabled={!canCreate} onClick={() => void create()}><TerminalSquare size={14}/>{t('create')}</button></div>}
        </>}
        {session && <div className="terminal-session"><header className="terminal-status"><div><strong>{session.tool_name}</strong><span>{t('state.' + session.state)}</span></div>
          <span className="terminal-connection" data-state={runtime.connection} role="status"><i aria-hidden="true"/>{t('connection.' + runtime.connection)}</span></header>
          {session.state === 'stop_requested' && <p className="terminal-warning">{t('stop_pending')}</p>}
          {!!session.reason && <p className="terminal-warning">{renderDiagnostic(session.reason)}</p>}
          {!!runtime.error && <ErrorNotice error={runtime.error}/>}
          {runtime.gap && <p role="status" className="terminal-warning">{t('output_gap')}</p>}
          {runtime.inputIssue && <div className="terminal-input-issue" role="alert"><p>{t(runtime.inputIssue === 'unknown' ? 'input_unknown' : 'input_rejected')}</p>
            <button type="button" disabled={locked || disabled} onClick={() => void runtime.acknowledgeInputIssue()}>{t('check_input')}</button></div>}
          {!runtime.inputIssue && runtime.lastInput && <p className={runtime.lastInput === 'delivery_unknown' ? 'terminal-warning' : 'terminal-note'}>{t(runtime.lastInput === 'delivery_unknown' ? 'input_unknown_reviewed' : runtime.lastInput === 'delivered' ? 'input_delivered' : 'input_accepted')}</p>}
          <div hidden={detail.loading || !!detail.error}><TerminalViewport key={session.id} inputEnabled={runtime.acceptsInput} onReady={setConsumer} onInput={runtime.input} onResize={resize}/></div>
          <div className="terminal-controls"><button type="button" disabled={locked} onClick={() => {setDetached(value => !value); if (detached) setRefresh(value => value + 1);}}>{t(detached ? 'reconnect' : 'detach')}</button>
            {session.allowed_actions.includes('stop') && <><label><input type="checkbox" checked={stopConsent} disabled={locked || disabled} onChange={event => setStopConsent(event.target.checked)}/>{t('stop_confirm')}</label>
              <button type="button" className="terminal-stop" data-terminal-stop disabled={!stopConsent || locked || disabled} onClick={() => void runtime.control('stop')}><Square size={12}/>{t('stop')}</button></>}</div>
          {!session.allowed_actions.includes('input') && session.state === 'running' && <p className="terminal-note">{t('read_only')}</p>}
          <p className="terminal-note">{t('detach_hint')}</p>
          <TerminalRecoveryPanel key={session.id} project={project} task={taskId} session={session} scope={scope}
            active={open && active && !blocked} disabled={disabled || action.busy || !!action.pending || candidateLocked} onSession={sessionChange} onDenied={refuse} onLockChange={setRecoveryLocked}/>
          <TerminalCandidatePanel key={'candidate:' + session.id} project={project} task={taskId} taskVersion={sessions.value?.task_version} session={session} scope={scope}
            active={open && active && !blocked} disabled={disabled || action.busy || !!action.pending || recoveryLocked} onDenied={refuse} onLockChange={setCandidateLocked} onRefresh={() => setRefresh(value => value + 1)}/>
          <details><summary>{t('details')}</summary><dl className="terminal-facts"><dt>{t('directory')}</dt><dd>{session.directory}</dd><dt>{t('worker')}</dt><dd>{session.worker_id}</dd>
            <dt>{t('expires')}</dt><dd>{formatDateTime(session.expires_at, {dateStyle: 'short', timeStyle: 'medium'})}</dd></dl>
            <p className="terminal-note">{t('received', {sequence: formatNumber(session.output_last_sequence)})}</p>
            <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: session.run_id})} onClick={event => {if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}}>{t('open_run')} <ArrowUpRight size={12}/></a></details>
        </div>}
      </>}
    </div>}
  </section>;
}
