import {useCallback, useEffect, useId, useRef, useState, type MouseEvent} from 'react';
import {ArrowUpRight, ChevronDown, GitCommitHorizontal, Play, RefreshCw} from 'lucide-react';
import type {WorkspaceTerminalSession, WorkspaceTerminalRecoveryView, WorkspaceTerminalCandidatePlan, WorkspaceTerminalCandidatePlanPage} from '../../../packages/contracts/src';
import {query, type Run} from './api';
import {useApi} from './api-context';
import {formatDateTime, formatNumber, renderDiagnostic, tr, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {useWriteAction, WriteNotice} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {useReplyCollection, useReplyRead} from './task-conversation-reply-hooks';
import {terminalEnded, terminalID} from './terminal-model';
import {validTerminalRecoveryView} from './terminal-recovery-model';
import {terminalCandidatePreviewReceipt, terminalCandidateProducers, terminalCandidateTool, validTerminalCandidatePlan, terminalCandidateMatchesSession,
  validTerminalCandidateRequest, validTerminalCandidateTools, validTerminalCandidateWorkflows, validTerminalCandidatePage} from './terminal-candidate-model';
import {TerminalCandidateHandoffPanel} from './TerminalCandidateHandoffPanel';
import {TerminalBaselineFacts} from './TerminalBaselineFacts';
import {validManagedRunIdentity} from './task-dispatch-response';
import {executionHref} from './scheduling-model';

const t = (key: string, values?: Record<string, unknown>) => tr('terminal.' + key, values);
type Props = {project: string; task: string; taskVersion?: number; session: WorkspaceTerminalSession; scope: string; active: boolean; disabled: boolean;
  onDenied: () => void; onLockChange: (locked: boolean) => void; onRefresh: () => void};
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);

/** Previewing stopped changes, starting their fixed check/seal and independent
 * tests, and formal human acceptance remain separate actions and facts. */
export function TerminalCandidatePanel({project, task, taskVersion, session, scope, active, disabled, onDenied, onLockChange, onRefresh}: Props) {
  const api = useApi(); useLocale(); const id = useId(), storage = toolSessionStorage(), locator = 'suma-terminal-candidate:' + scope + ':' + session.id;
  const [open, setOpen] = useState(false), [refresh, setRefresh] = useState(0), [flowKey, setFlowKey] = useState(''), [producer, setProducer] = useState(''), [profileID, setProfileID] = useState('');
  const [selected, setSelected] = useState(() => {try {const saved = storage?.getItem(locator); return terminalID(saved) ? saved : '';} catch {return '';}});
  const [newPreview, setNewPreview] = useState(false);
  const [consent, setConsent] = useState(false), [startConsent, setStartConsent] = useState(false), [created, setCreated] = useState<WorkspaceTerminalCandidatePlan>();
  const [handoffLocked, setHandoffLocked] = useState(false);
  const callbacks = useRef({onDenied, onLockChange, onRefresh}); callbacks.current = {onDenied, onLockChange, onRefresh};
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  const action = useWriteAction({scope: 'terminal-candidate:' + scope + ':' + session.id, storage,
    validateRequest: request => validTerminalCandidateRequest(request, project, session.id),
    validateResponse: async (value, request) => {
      if (request.path.endsWith('/candidate-plans')) return terminalCandidatePreviewReceipt(value, request, project, task, session.id, session.run_id) &&
        terminalCandidateMatchesSession(value, session);
      const match = /^\/workspace\/terminal-candidate-plans\/([A-Za-z0-9_.-]+)\/start$/.exec(request.path);
      if (!match || !object(value) || value.project_id !== project) return false;
      const original = await api<unknown>('/workspace/terminal-candidate-plans/' + encodeURIComponent(match[1]) + query(project));
      return validTerminalCandidatePlan(original, project, task, session.id) && original.id === match[1] && terminalCandidateMatchesSession(original, session) &&
        original.version >= Number(request.body.expected_version) && original.state === 'started' && validManagedRunIdentity(value, project, original.run_id);
    }});
  const locked = action.busy || !!action.pending || handoffLocked, blocked = action.errorStatus === 401 || action.errorStatus === 403;
  const refuse = useCallback(() => callbacks.current.onDenied(), []);
  const history = useReplyRead('/workspace/terminal-sessions/' + encodeURIComponent(session.id) + '/candidate-plans' + query(project), open && active && !blocked,
    (value): value is WorkspaceTerminalCandidatePlanPage => validTerminalCandidatePage(value, project, task, session.id, session.run_id) && value.items.every(plan => terminalCandidateMatchesSession(plan, session)), refuse, refresh);
  const proof = useReplyRead('/workspace/terminal-sessions/' + encodeURIComponent(session.id) + '/recovery' + query(project), open && active && newPreview && !selected && !blocked,
    (value): value is WorkspaceTerminalRecoveryView => validTerminalRecoveryView(value, session.id, session.run_id), refuse, refresh);
  const workflows = useReplyRead('/workflows' + query(project), open && active && newPreview && !selected && !blocked, validTerminalCandidateWorkflows, refuse, refresh);
  const profiles = useReplyCollection('/tool-profiles' + query(project) + '&limit=100', 'cursor', open && active && newPreview && !selected && !blocked,
    (value): value is {items: import('./tool-model').ToolProfile[]; total: number; next_cursor?: string} => validTerminalCandidateTools(value, project), refuse, refresh);
  const planRead = useReplyRead('/workspace/terminal-candidate-plans/' + encodeURIComponent(selected) + query(project), open && active && !!selected && !blocked,
    (value): value is WorkspaceTerminalCandidatePlan => validTerminalCandidatePlan(value, project, task, session.id) && value.id === selected && terminalCandidateMatchesSession(value, session), refuse, refresh);
  const plan = planRead.value || (created?.id === selected ? created : undefined);
  const workbuddy = plan?.source.workbuddy_origin;
  const flow = workflows.value?.items.find(item => item.id + '@' + item.version === flowKey), command = flow?.nodes.find(node => node.id === producer)?.command?.[0];
  const tools = profiles.value?.items.filter(tool => terminalCandidateTool(tool, session.worker_id, command)) || [];
  const profile = tools.find(tool => tool.id === profileID);
  const currentTaskVersion = Math.max(taskVersion || 0, history.value?.task_version || 0);
  const stopped = terminalEnded(session) && !!session.source_snapshot_sha256 && !!session.base_commit && !!session.admin_base_commit && proof.value?.tree_stopped && !proof.value.can_recover && !proof.loading && !proof.error;
  const ready = newPreview && !disabled && !locked && !blocked && !action.conflict && !action.persistenceBlocked && !!stopped && !!currentTaskVersion && consent &&
    !!flow && terminalCandidateProducers(flow).some(node => node.id === producer) && !!profile && !!command && profile.profile.command_id === command &&
    !workflows.loading && !workflows.error && !profiles.loading && !profiles.error;
  useEffect(() => {alive.current = true; return () => {alive.current = false; callbacks.current.onLockChange(false);};}, []);
  useEffect(() => {callbacks.current.onLockChange(locked);}, [locked]);
  useEffect(() => {if (blocked) callbacks.current.onDenied();}, [blocked]);
  useEffect(() => {setConsent(false);}, [session.version, taskVersion, refresh]);
  useEffect(() => {setStartConsent(false);}, [plan?.id, plan?.version, plan?.valid]);
  useEffect(() => {if (action.pending) {setOpen(true); const match = /^\/workspace\/terminal-candidate-plans\/([A-Za-z0-9_.-]+)\/start$/.exec(action.pending.path); if (match) setSelected(match[1]);}}, [!!action.pending]);
  useEffect(() => {
    if (!history.value || history.loading || history.error || selected || newPreview || action.pending) return;
    if (history.value.items.length === 1) remember(history.value.items[0]);
    else if (!history.value.items.length) setNewPreview(true);
  }, [history.value, history.loading, history.error, selected, newPreview, !!action.pending]);
  function remember(value: WorkspaceTerminalCandidatePlan) {
    setCreated(value); setSelected(value.id); setNewPreview(false); setConsent(false); setStartConsent(false);
    try {storage?.setItem(locator, value.id);} catch {/* The immutable plan and operation receipt remain on the server. */}
  }
  async function prepare() {
    if (!ready || !profile || !flow || !currentTaskVersion) return;
    const caller = api, value = await action.run<WorkspaceTerminalCandidatePlan>('/workspace/terminal-sessions/' + encodeURIComponent(session.id) + '/candidate-plans', {
      project_id: project, expected_session_version: session.version, expected_task_version: currentTaskVersion, tool_profile_id: profile.id,
      expected_tool_version: profile.version, workflow_id: flow.id, workflow_version: flow.version, producer_node_id: producer, acknowledge_stopped_changes: true});
    if (value && alive.current && owner.current === caller) remember(value);
  }
  async function start() {
    if (!plan || disabled || blocked || locked || !startConsent || !plan.valid || plan.state !== 'ready' || planRead.loading || planRead.error) return;
    const caller = api, value = await action.run<Run>('/workspace/terminal-candidate-plans/' + encodeURIComponent(plan.id) + '/start', {project_id: project, expected_version: plan.version});
    if (value && alive.current && owner.current === caller) {setStartConsent(false); await planRead.read(); callbacks.current.onRefresh();}
  }
  async function restore() {
    const pending = action.pending, caller = api;
    if (!pending || disabled || blocked || action.busy) return;
    const value = await action.run<unknown>(pending.path, pending.body, pending.method);
    if (!value || !alive.current || owner.current !== caller) return;
    if (validTerminalCandidatePlan(value, project, task, session.id)) remember(value);
    else {await planRead.read(); callbacks.current.onRefresh();}
  }
  function follow(event: MouseEvent) {if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}
  return <section className="terminal-candidate" aria-label={t('candidate.title')}>
    <button type="button" className="terminal-subsection-toggle" data-terminal-candidate-toggle aria-expanded={open} aria-controls={id} disabled={locked}
      onClick={() => setOpen(value => !value)}><GitCommitHorizontal size={14} aria-hidden="true"/><span>{t('candidate.title')}</span><ChevronDown size={14} aria-hidden="true"/></button>
    {open && <div id={id} className="terminal-subsection-body">
      <p className="terminal-note">{t('candidate.intro')}</p>
      <WriteNotice action={action} onRebase={async () => {setConsent(false); setStartConsent(false); setRefresh(value => value + 1); callbacks.current.onRefresh(); action.clearConflict();}}/>
      {action.pending && <div role="status"><p>{t('candidate.pending')}</p><button type="button" disabled={disabled || action.busy || blocked} onClick={() => void restore()}>{t('original')}</button></div>}
      <button type="button" disabled={locked || workflows.loading || profiles.loading || planRead.loading} onClick={() => {setConsent(false); setStartConsent(false); setRefresh(value => value + 1); callbacks.current.onRefresh();}}><RefreshCw size={13} aria-hidden="true"/>{t('candidate.refresh')}</button>
      {history.loading && <Loading/>}{!!history.error && <ErrorNotice error={history.error} onRetry={() => void history.read()}/>}
      {history.value && <>
        <Field label={t('candidate.saved_plans')}><select data-terminal-candidate-plans value={selected || (newPreview ? '__new__' : '')} disabled={locked || history.loading}
          onChange={event => {
            const saved = history.value?.items.find(item => item.id === event.target.value);
            if (saved) remember(saved);
            else {setSelected(''); setCreated(undefined); setNewPreview(event.target.value === '__new__'); setConsent(false); setStartConsent(false); try {storage?.removeItem(locator);} catch {/* Only a read locator is removed. */}}
          }}><option value="" disabled>{t('candidate.choose_plan')}</option><option value="__new__">{t('candidate.new_preview')}</option>
          {history.value.items.map(item => <option key={item.id} value={item.id}>{t(item.state === 'started' ? 'candidate.started' : 'candidate.prepared')} · {item.id}</option>)}
          {selected && !history.value.items.some(item => item.id === selected) && <option value={selected}>{selected}</option>}</select></Field>
        <p className="terminal-note">{t('candidate.recent_plans')}</p>
      </>}
      {plan ? <>
        <strong role="status">{t(plan.state === 'started' ? 'candidate.started' : 'candidate.prepared')}</strong>
        <p className="terminal-note">{t('candidate.boundary')}</p>
        {!!plan.reason && <p className="terminal-warning">{renderDiagnostic(plan.reason)}</p>}
        {workbuddy && <section className="terminal-baseline-facts" aria-label={t('candidate.workbuddy_origin')}>
          <strong>{t('candidate.workbuddy_origin')}</strong>
          <dl className="terminal-facts">
            <dt>{t('recovery.run')}</dt><dd>{locked ? workbuddy.run_id :
              <a href={executionHref(project, workbuddy.run_id, workbuddy.node_id, workbuddy.attempt)} onClick={follow}>{workbuddy.run_id}</a>}</dd>
            <dt>{t('candidate.workbuddy_node')}</dt><dd>{workbuddy.node_id}</dd>
            <dt>{t('candidate.workbuddy_attempt')}</dt><dd>{formatNumber(workbuddy.attempt)}</dd>
          </dl>
          <p className="terminal-note">{t('candidate.workbuddy_local')}</p>
          {plan.state === 'ready' && plan.valid && <p className="terminal-warning">{t('candidate.workbuddy_start')}</p>}
        </section>}
        <details><summary>{t('candidate.source')}</summary><TerminalBaselineFacts project={project} binding={plan.source.code_baseline} adminCommit={plan.source.admin_base_commit} selectedCommit={plan.source.base_commit} title={t('baseline.frozen')}/>
          <dl className="terminal-facts"><dt>{t('recovery.run')}</dt><dd>{plan.source.original.run_id}</dd><dt>{t('worker')}</dt><dd>{plan.worker_id}</dd><dt>{t('candidate.snapshot')}</dt><dd>{plan.source.snapshot_sha256}</dd></dl></details>
        <details><summary>{t('candidate.review')}</summary><p>{plan.context.task.title} · v{formatNumber(plan.task_version)}</p>
          <ul>{plan.context.task.acceptance_criteria.map((criterion, index) => <li key={index}>{criterion}</li>)}</ul>
          <ol>{plan.workflow.nodes.map(node => <li key={node.id}>{node.name || node.id} · {tr('development.workflow.type.' + node.type)}<small>{node.command?.join(' ')}</small></li>)}</ol>
          <p className="terminal-note">{t('candidate.expires', {at: formatDateTime(plan.expires_at, {dateStyle: 'short', timeStyle: 'medium'})})}</p></details>
        {plan.state === 'started' ? <div className="terminal-controls"><a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: plan.run_id})} onClick={follow}>{t('candidate.open_run')} <ArrowUpRight size={12} aria-hidden="true"/></a>
          <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'tasks', record: task})} onClick={follow}>{t('candidate.open_task')} <ArrowUpRight size={12} aria-hidden="true"/></a></div> : <>
          <label className="terminal-recovery-consent"><input type="checkbox" checked={startConsent} disabled={disabled || locked || !plan.valid} onChange={event => setStartConsent(event.target.checked)}/><span>{t('candidate.start_consent')}</span></label>
          <button type="button" data-terminal-candidate-start className="primary" disabled={disabled || locked || !plan.valid || !startConsent || planRead.loading || !!planRead.error} onClick={() => void start()}><Play size={14} aria-hidden="true"/>{t('candidate.start')}</button>
        </>}
        {plan.state === 'started' && <TerminalCandidateHandoffPanel project={project} plan={plan} scope={scope} active={active && !blocked}
          disabled={disabled || action.busy || !!action.pending} onDenied={refuse} onLockChange={setHandoffLocked} onRefresh={() => callbacks.current.onRefresh()}/>}
      </> : <>
        {(workflows.loading || profiles.loading || proof.loading || planRead.loading) && <Loading/>}
        {!!workflows.error && <ErrorNotice error={workflows.error} onRetry={() => void workflows.read()}/>}{!!profiles.error && <ErrorNotice error={profiles.error} onRetry={() => void profiles.read()}/>}
        {!!proof.error && <ErrorNotice error={proof.error} onRetry={() => void proof.read()}/>}
        {newPreview && !selected && <>
          {!stopped && <p className="terminal-warning" role="status">{t('candidate.stop_required')}</p>}
          <div className="terminal-candidate-fields"><Field label={t('candidate.workflow')}><select data-terminal-candidate-workflow value={flowKey} disabled={disabled || locked} onChange={event => {
            const selectedFlow = workflows.value?.items.find(item => item.id + '@' + item.version === event.target.value), producers = terminalCandidateProducers(selectedFlow);
            setFlowKey(event.target.value); setProducer(producers.length === 1 ? producers[0].id : ''); setProfileID(''); setConsent(false);
          }}><option value="">{t('candidate.choose_workflow')}</option>{workflows.value?.items.filter(item => terminalCandidateProducers(item).length).map(item => <option key={item.id + '@' + item.version} value={item.id + '@' + item.version}>{item.name} · v{formatNumber(item.version)}</option>)}</select></Field>
          <Field label={t('candidate.profile')}><select data-terminal-candidate-profile value={profileID} disabled={disabled || locked || !producer} onChange={event => {setProfileID(event.target.value); setConsent(false);}}>
            <option value="">{t('candidate.choose_profile')}</option>{tools.map(tool => <option key={tool.id} value={tool.id}>{tool.name} · v{formatNumber(tool.version)}</option>)}</select></Field></div>
          <p className="terminal-note">{t('candidate.workflow_hint')}</p>
          {flow && !tools.length && !profiles.loading && <p className="terminal-warning">{t('candidate.no_profile')}</p>}
          {workflows.value && !workflows.value.items.some(item => terminalCandidateProducers(item).length) && <p>{t('candidate.no_workflow')}</p>}
          {profiles.value && <p className="terminal-note">{t('candidate.profile_page', {loaded: formatNumber(profiles.value.items.length), total: formatNumber(profiles.value.total)})}</p>}
          {profiles.value?.next_cursor && <button type="button" disabled={locked || profiles.loading} onClick={() => {setConsent(false); void profiles.read(true);}}>{t('candidate.more_profiles')}</button>}
          <label className="terminal-recovery-consent"><input type="checkbox" checked={consent} disabled={disabled || locked || !stopped} onChange={event => setConsent(event.target.checked)}/><span>{t('candidate.preview_consent')}</span></label>
          <button type="button" data-terminal-candidate-preview className="primary" disabled={!ready} onClick={() => void prepare()}><GitCommitHorizontal size={14} aria-hidden="true"/>{t('candidate.preview')}</button>
        </>}
      </>}
      {!!planRead.error && <ErrorNotice error={planRead.error} onRetry={() => void planRead.read()}/>}
    </div>}
  </section>;
}
