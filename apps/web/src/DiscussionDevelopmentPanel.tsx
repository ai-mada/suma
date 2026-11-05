import {useCallback, useEffect, useId, useRef, useState, type MouseEvent} from 'react';
import {ArrowUpRight, ChevronDown, Code2, Play, RefreshCw} from 'lucide-react';
import type {DevelopmentContinuationSource, TaskConversationPage, TaskConversationReply, TaskDiscussionDevelopmentPlan, TaskDispatchOptions} from '../../../packages/contracts/src';
import {query, type Session, type Workflow} from './api';
import {useApi} from './api-context';
import {formatNumber, renderDiagnostic, tr, UiError, useLocale} from './i18n';
import {ErrorNotice, Field, Loading} from './ui';
import {ModelExecutionPanel, type ModelChoice} from './ModelExecutionPanel';
import {TaskDispatchFacts} from './ToolFacts';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {toolSessionStorage, useToolList} from './tool-hooks';
import {TaskDispatchTestBindings, type TestBindingsChoice} from './TaskDispatchTestBindings';
import type {ToolProfile} from './tool-model';
import {useWriteAction, WriteNotice} from './write-action';
import {useReplyRead} from './task-conversation-reply-hooks';
import {dispatchOptionsPath, validDispatchOptions, type DispatchOptionsScope} from './task-dispatch-options-model';
import {developmentCriteria, developmentGoal, validDiscussionDevelopment, validDiscussionDevelopmentReceipt, validDiscussionDevelopmentRequest} from './discussion-development-model';
import {validManagedTaskDispatchRun} from './task-dispatch-response';
import {sameToolProfile} from './tool-profile-reuse-model';
import {ExecutionModelFacts} from './ExecutionModelFacts';
import {CodexNativeSkillFacts} from './CodexNativeSkillFacts';
import {DevelopmentContinuationFacts,DevelopmentContinuationSelector} from './DevelopmentContinuationSelector';
import './DiscussionDevelopmentPanel.css';

const t = (key: string, values?: Record<string, unknown>) => tr('discussion-development.' + key, values);
type Props = {session: Session; project: string; taskId: string; reply: TaskConversationReply; page?: TaskConversationPage;
  disabled: boolean; accessBlocked: boolean; onLockChange: (locked: boolean) => void; onRun?: (run: string) => void;
  onContinue?: (origin:{planID:string;runID?:string})=>void};
const identifier = (value: unknown): value is string => typeof value === 'string' && /^[A-Za-z0-9_.-]{1,64}$/.test(value);
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const validWorkflows = (value: unknown): value is {items: Workflow[]} => object(value) && Array.isArray(value.items) &&
  value.items.length <= 1000 && value.items.every(item => object(item) && identifier(item.id) && Number.isSafeInteger(item.version) &&
    Number(item.version) > 0 && typeof item.name === 'string' && Array.isArray(item.nodes) && item.nodes.every(node => object(node) && typeof node.id === 'string' && typeof node.type === 'string'));

/** A reviewed reply becomes provenance for the original dispatch preview. The
 * fixed development run starts only through the separate explicit action. */
export function DiscussionDevelopmentPanel(props: Props) {
  const api = useApi(), owner = useRef({api, epoch: 0});
  if (owner.current.api !== api) owner.current = {api, epoch: owner.current.epoch + 1};
  const scope = JSON.stringify([props.session.user.id, props.session.organization.id, props.project, props.taskId, props.reply.id]);
  return <DevelopmentScope key={scope + ':' + owner.current.epoch} {...props} scope={scope}/>;
}

function DevelopmentScope({session, project, taskId, reply, page, disabled, accessBlocked, onLockChange, onRun, onContinue, scope}: Props & {scope: string}) {
  const api = useApi(); useLocale(); const id = useId(), storage = toolSessionStorage();
  const locator = 'suma-discussion-development:' + scope;
  const [selected, setSelected] = useState(() => {try {const saved = storage?.getItem(locator); return identifier(saved) ? saved : '';} catch {return '';}});
  const [open, setOpen] = useState(false), [denied, setDenied] = useState(false), [refresh, setRefresh] = useState(0);
  const [goal, setGoal] = useState(''), [criteria, setCriteria] = useState(''), [flowKey, setFlowKey] = useState(''), [agent, setAgent] = useState('');
  const [before, setBefore] = useState(''), [profile, setProfile] = useState<ToolProfile>(), [model, setModel] = useState<ModelChoice>({ready: false});
  const [consent, setConsent] = useState(false), [startConsent, setStartConsent] = useState(false), [created, setCreated] = useState<TaskDiscussionDevelopmentPlan>();
  const [continuation,setContinuation]=useState<{source?:DevelopmentContinuationSource;ready:boolean}>({ready:true});
  const callback = useRef({onLockChange, onRun,onContinue}); callback.current = {onLockChange, onRun,onContinue};
  const alive = useRef(true), owner = useRef(api); owner.current = api;
  const refuse = useCallback(() => setDenied(true), []);
  const action = useWriteAction({scope: 'discussion-development:' + scope, storage,
    validateRequest: request => validDiscussionDevelopmentRequest(request, project, taskId),
    validateResponse: async (value, request) => {
      if (request.path === '/task-discussion-development-plans') return validDiscussionDevelopmentReceipt(value, request, project, taskId, session.user.id);
      const match = /^\/task-discussion-development-plans\/([A-Za-z0-9_.-]+)\/start$/.exec(request.path);
      if (!match || !object(value) || value.project_id !== project) return false;
      const original = await api<unknown>('/task-discussion-development-plans/' + encodeURIComponent(match[1]) + query(project));
      return validDiscussionDevelopment(original, project, taskId) && original.id === match[1] && original.version >= Number(request.body.expected_version) &&
        validManagedTaskDispatchRun(value, original.dispatch_plan, project);
    }});
  const locked = action.busy || !!action.pending, blocked = accessBlocked || denied || action.errorStatus === 401 || action.errorStatus === 403;
  const workflows = useReplyRead('/workflows' + query(project), open && !blocked && !selected, validWorkflows, refuse, refresh);
  const flow = workflows.value?.items.find(item => item.id + '@' + item.version === flowKey);
  const dispatchScope: DispatchOptionsScope | undefined = flow && page && agent ? {project_id: project, task_id: taskId,
    expected_task_version: page.task_version, workflow_id: flow.id, workflow_version: flow.version, agent_node_id: agent} : undefined;
  const optionsPath = dispatchScope ? dispatchOptionsPath(dispatchScope, before) : '';
  const options = useReplyRead<TaskDispatchOptions>(optionsPath, open && !blocked && !selected && !!dispatchScope,
    (value): value is TaskDispatchOptions => !!dispatchScope && validDispatchOptions(value, dispatchScope, session.organization.id), refuse, refresh);
  const planRead = useReplyRead('/task-discussion-development-plans/' + encodeURIComponent(selected) + query(project),
    !!selected && !blocked, (value): value is TaskDiscussionDevelopmentPlan => validDiscussionDevelopment(value, project, taskId), refuse, refresh);
  const plan = planRead.value || (created?.id === selected ? created : undefined);
  const tools = options.value?.items.filter(item => item.eligible && !item.tool.profile.read_only).map(item => item.tool) || [];
  const currentProfile = profile && tools.find(tool => tool.id === profile.id && tool.version === profile.version && tool.worker_id === profile.worker_id && sameToolProfile(tool.profile, profile.profile));
  const testBasis = JSON.stringify([scope, reply.version, page?.task_version, flow?.id, flow?.version, agent,
    currentProfile?.id, currentProfile?.version, currentProfile?.worker_id, currentProfile?.profile.profile_sha256]);
  const testOwner = useRef({api, key: testBasis, epoch: 0});
  if (testOwner.current.api !== api || testOwner.current.key !== testBasis)
    testOwner.current = {api, key: testBasis, epoch: testOwner.current.epoch + 1};
  const testEpoch = testOwner.current.epoch, testScope = JSON.stringify([testBasis, testEpoch]);
  const [tests, setTests] = useState<TestBindingsChoice & {key: string}>({key: '', ready: true, hasIntent: false, selections: []});
  const currentTests = tests.key === testScope ? tests : {ready: true, hasIntent: false, selections: []};
  const changeTests = useCallback((value: TestBindingsChoice) => {
    if (testOwner.current.api !== api || testOwner.current.key !== testBasis || testOwner.current.epoch !== testEpoch) return;
    setTests({...value, key: testScope}); setConsent(false);
  }, [api, testBasis, testEpoch, testScope]);
  const modelRequired = profile?.profile.model_execution_protocol === 'suma-model-execution-v1', codexRequired = profile?.profile.protocol === 'codex-app-server-v1';
  const modelReady = !(modelRequired && codexRequired) && (modelRequired ? model.ready && !!model.selection && !model.codexSelection : codexRequired ? model.ready && !!model.codexSelection && !model.selection : !model.selection && !model.codexSelection);
  const message = page?.items.find(item => item.id === reply.message_id && item.reply_id === reply.id && item.actor_kind === 'assistant');
  const seeded = useRef(false);
  const checks = criteria.split('\n').map(value => value.trim()).filter(Boolean);
  const readyReply = reply.state === 'completed' && reply.basis_current && !!page?.can_post && page.task_id === taskId && reply.source.task.id===taskId && reply.source.task.version===page.task_version;
  const canPrepare = !blocked && !disabled && !locked && !action.conflict && !action.persistenceBlocked && readyReply &&
    !!flow && !!agent && !!currentProfile && modelReady && currentTests.ready && continuation.ready && consent && developmentGoal(goal) && developmentCriteria(checks) &&
    !options.loading && !options.error && !workflows.loading && !workflows.error;
  const modelChange = useCallback((value: ModelChoice) => {setModel(value); setConsent(false);}, []);
  const continuationChange=useCallback((value:{source?:DevelopmentContinuationSource;ready:boolean})=>{setContinuation(value);setConsent(false);},[]);
  useEffect(() => {alive.current = true; return () => {alive.current = false; callback.current.onLockChange(false);};}, []);
  useEffect(() => {callback.current.onLockChange(locked);}, [locked]);
  useEffect(() => {if (action.pending) setOpen(true);}, [!!action.pending]);
  useEffect(() => {setProfile(undefined); setModel({ready: false}); setConsent(false); setBefore('');}, [flowKey, agent, page?.task_version]);
  useEffect(() => {setStartConsent(false);}, [plan?.id, plan?.version, plan?.dispatch_plan.valid]);
  useEffect(() => {
    if (!seeded.current && message) {seeded.current = true; if (developmentGoal(message.body)) setGoal(value => value || message.body);}
  }, [message?.id]);
  function remember(value: TaskDiscussionDevelopmentPlan) {
    setCreated(value); setSelected(value.id); setStartConsent(false); setConsent(false);
    try {storage?.setItem(locator, value.id);} catch {action.setError(new UiError('task-reply.locator_unavailable'));}
  }
  async function prepare() {
    if (!canPrepare || !page || !flow || !currentProfile) return;
    const caller = api;
    const value = await action.run<TaskDiscussionDevelopmentPlan>('/task-discussion-development-plans', {project_id: project, task_id: taskId,
      expected_task_version: page.task_version, reply_id: reply.id, expected_reply_version: reply.version,
      workflow_id: flow.id, workflow_version: flow.version, agent_node_id: agent, tool_profile_id: profile.id,
      expected_tool_version: profile.version, ...(modelRequired && model.selection ? {model_execution_selection: model.selection} : {}),
      ...(codexRequired && model.codexSelection ? {codex_execution_selection: model.codexSelection} : {}), reviewed_goal: goal,
      ...(continuation.source?{continuation_source:continuation.source}:{}),
      ...(currentTests.selections.length ? {test_bindings: currentTests.selections} : {}),
      acceptance_criteria: checks, acknowledge_execution: true});
    if (value && alive.current && owner.current === caller) remember(value);
  }
  async function start() {
    if (!plan || blocked || disabled || locked || !startConsent || !plan.dispatch_plan.valid || plan.dispatch_plan.state !== 'ready') return;
    const caller = api;
    const run = await action.run<{id: string}>('/task-discussion-development-plans/' + encodeURIComponent(plan.id) + '/start', {
      project_id: project, expected_version: plan.version});
    if (run && alive.current && owner.current === caller) {await planRead.read(); callback.current.onRun?.(run.id);}
  }
  async function restore() {
    if (!action.pending || blocked || disabled) return;
    const original = action.pending, caller = api;
    const value = await action.run<unknown>(original.path, original.body, original.method);
    if (!value || !alive.current || owner.current !== caller) return;
    if (validDiscussionDevelopment(value, project, taskId)) remember(value);
    else if (object(value) && typeof value.id === 'string') {await planRead.read(); callback.current.onRun?.(value.id);}
  }
  function follow(event: MouseEvent) {if (!window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))) event.preventDefault();}
  function continueDiscussion(){if(!plan||blocked||locked||action.persistenceBlocked||planRead.loading||planRead.error||!alive.current||owner.current!==api)return;callback.current.onContinue?.({planID:plan.id,...(plan.dispatch_plan.state==='started'?{runID:plan.dispatch_plan.run_id}:{})});}
  return <section className="discussion-development" aria-label={t('title')}>
    <button type="button" className="discussion-development-toggle" data-development-toggle aria-expanded={open} aria-controls={id}
      disabled={open && locked} onClick={() => setOpen(value => !value)}><Code2 size={15}/><span>{t('title')}</span><ChevronDown size={15}/></button>
    {open && <div id={id} className="discussion-development-body">
      <p className="discussion-development-note">{t('intro')}</p>
      {blocked ? <p role="alert" className="discussion-development-warning">{t('access')}</p> : <>
        <WriteNotice action={action} onRebase={async () => {await planRead.read(); setRefresh(value => value + 1); setConsent(false); setStartConsent(false);}}/>
        {action.pending && <div role="status"><p>{t('pending')}</p><button type="button" data-development-restore disabled={action.busy || disabled} onClick={() => void restore()}>{t('original')}</button></div>}
        {plan ? <div className="discussion-development-plan"><header><strong>{t(plan.dispatch_plan.state === 'started' ? 'started' : 'created')}</strong>
          <button type="button" disabled={locked || planRead.loading} onClick={() => void planRead.read()}><RefreshCw size={13}/>{t('refresh')}</button></header>
          {!!planRead.error && <ErrorNotice error={planRead.error}/>}
          <p className="discussion-development-note">{t('boundary')}</p>
          {plan.development_continuation&&<DevelopmentContinuationFacts binding={plan.development_continuation} project={project} taskID={taskId}/>}
          <details><summary>{t('source_body')}</summary><p className="task-reply-copy">{plan.message.body}</p><code>{plan.source_sha256}</code></details>
          <details><summary>{t('review_plan')}</summary><TaskDispatchFacts plan={plan.dispatch_plan}/></details>
          <ExecutionModelFacts binding={plan.dispatch_plan.model_execution} codex={plan.dispatch_plan.codex_execution} openCode={plan.dispatch_plan.opencode_execution} claude={plan.dispatch_plan.claude_execution} claudeSDK={plan.dispatch_plan.claude_sdk_execution} acp={plan.dispatch_plan.acp_execution}/>
          {plan.dispatch_plan.codex_execution ? <CodexNativeSkillFacts mode="plan" bundle={plan.dispatch_plan.execution_skills}/> : plan.dispatch_plan.context.knowledge?.some(item => item.kind === 'skill') && <CodexNativeSkillFacts mode="text"/>}
          {!!plan.dispatch_plan.context.knowledge?.length && <div onClickCapture={event => {if (event.target instanceof Element && event.target.closest('a[href]')) follow(event);}}><KnowledgeContextFacts project={project} items={plan.dispatch_plan.context.knowledge}/></div>}
          {plan.dispatch_plan.state === 'started' ? <a href={'?' + new URLSearchParams({project, view: 'development', sub: 'runs', run: plan.dispatch_plan.run_id})}
            onClick={follow}>{t('open_run')} <ArrowUpRight size={12}/></a> : <>
            <p className="discussion-development-note">{t('start_separate')}</p>
            {!plan.dispatch_plan.valid && <p role="status" className="discussion-development-warning">{renderDiagnostic(plan.dispatch_plan.reason)}</p>}
            <label className="discussion-development-consent"><input type="checkbox" checked={startConsent} disabled={disabled || locked || !plan.dispatch_plan.valid}
              onChange={event => setStartConsent(event.target.checked)}/><span>{t('review_start')}</span></label>
            <button type="button" className="primary" data-development-start disabled={disabled || locked || !startConsent || !plan.dispatch_plan.valid || planRead.loading || !!planRead.error}
              onClick={() => void start()}><Play size={14}/>{t('start')}</button>
          </>}
          {onContinue&&<button type="button" data-development-continue disabled={blocked||locked||action.persistenceBlocked||planRead.loading||!!planRead.error} onClick={continueDiscussion}>{t('continue_discussion')}</button>}
        </div> : <>
          {selected && planRead.loading && <Loading/>}{!!planRead.error && <ErrorNotice error={planRead.error} onRetry={() => void planRead.read()}/>}
          {!selected && <>
            <p className="discussion-development-note">{t('source', {sequence: formatNumber(reply.message_sequence), version: formatNumber(page?.task_version || reply.source.task.version)})}</p>
            {message && <details><summary>{t('source_body')}</summary><p className="task-reply-copy">{message.body}</p></details>}
            {!readyReply && <p role="status" className="discussion-development-warning">{t('not_ready')}</p>}
            <Field label={t('goal')} hint={t('goal_hint')}><textarea data-development-goal value={goal} disabled={disabled || locked} onChange={event => {setGoal(event.target.value); setConsent(false);}}/></Field>
            <Field label={t('criteria')} hint={t('criteria_hint')}><textarea data-development-criteria value={criteria} disabled={disabled || locked} onChange={event => {setCriteria(event.target.value); setConsent(false);}}/></Field>
            {page&&<DevelopmentContinuationSelector project={project} taskID={taskId} taskVersion={page.task_version} disabled={disabled||locked||action.persistenceBlocked} accessBlocked={blocked} refresh={refresh} onChange={continuationChange} onAccessDenied={refuse}/>}
            {developmentCriteria(reply.source.task.acceptance_criteria) && <button type="button" disabled={disabled || locked}
              onClick={() => {setCriteria(reply.source.task.acceptance_criteria.join('\n')); setConsent(false);}}>{t('copy_task_criteria')}</button>}
            {workflows.loading && <Loading/>}{!!workflows.error && <ErrorNotice error={workflows.error} onRetry={() => void workflows.read()}/>}
            <div className="discussion-development-fields"><Field label={t('workflow')}><select value={flowKey} disabled={disabled || locked} onChange={event => {
              const selectedFlow = workflows.value?.items.find(item => item.id + '@' + item.version === event.target.value);
              const coding = selectedFlow?.nodes.filter(node => node.type === 'agent') || [];
              setFlowKey(event.target.value); setAgent(coding.length === 1 ? coding[0].id : '');}}>
              <option value="">{t('choose_workflow')}</option>{workflows.value?.items.filter(item => item.nodes.some(node => node.type === 'agent')).map(item =>
                <option key={item.id + '@' + item.version} value={item.id + '@' + item.version}>{item.name} · v{formatNumber(item.version)}</option>)}</select></Field>
              <Field label={t('agent')}><select value={agent} disabled={disabled || locked || !flow} onChange={event => setAgent(event.target.value)}>
                <option value="">{t('choose_agent')}</option>{flow?.nodes.filter(node => node.type === 'agent').map(node => <option key={node.id} value={node.id}>{node.name || node.id}</option>)}</select></Field></div>
            {dispatchScope && <section className="discussion-development-profile" aria-label={t('tools')}><div><strong>{t('tools')}</strong><button type="button" disabled={locked || options.loading}
              onClick={() => {setProfile(undefined); setBefore(''); setRefresh(value => value + 1);}}><RefreshCw size={13}/>{t('refresh')}</button></div>
              {options.loading && <Loading/>}{!!options.error && <ErrorNotice error={options.error} onRetry={() => void options.read()}/>}
              {options.value && <><p className="discussion-development-note">{t('tools_hint')}</p><p className="discussion-development-note">{t('page', {loaded: formatNumber(options.value.items.length), total: formatNumber(options.value.total)})}</p>
                {tools.length ? <ul className="discussion-development-tools">{tools.map(tool => <li key={tool.id}><button type="button" aria-pressed={profile?.id === tool.id} disabled={disabled || locked}
                  data-development-tool={tool.id} onClick={() => {setProfile(tool); setModel({ready: false}); setConsent(false);}}><span><strong>{tool.name}</strong><small>{tool.profile.adapter} · {tool.profile.protocol}</small></span><small>{t('tool_basis', {worker: tool.worker_id, version: formatNumber(tool.version)})}</small></button></li>)}</ul>
                  : <p>{t('no_tool')}</p>}{options.value.next_cursor && <button type="button" disabled={locked || options.loading} onClick={() => {setProfile(undefined); setBefore(options.value!.next_cursor);}}>{t('more')}</button>}</>}
            </section>}
            {page && flow && currentProfile && <DiscussionTestBindings key={testScope} project={project} task={{id:taskId,version:page.task_version}} workflow={flow} agentID={agent} developer={currentProfile}
              scopeKey={testScope} disabled={disabled || locked || blocked || action.persistenceBlocked} onChange={changeTests}/>}
            {profile && <div className="discussion-development-model"><p className="discussion-development-note">{t(codexRequired ? 'selection_codex' : modelRequired ? 'selection_platform' : 'selection_fixed')}</p><ModelExecutionPanel key={JSON.stringify([profile.id,profile.version,profile.profile.profile_sha256])} scope={{project, profile: profile.id, version: profile.version, sha256: profile.profile.profile_sha256}}
              supported={modelRequired} codex={codexRequired} fixedOpenCode={profile.profile.opencode_execution} fixedClaude={profile.profile.claude_execution} fixedClaudeSDK={profile.profile.claude_sdk_execution} fixedACP={profile.profile.acp_execution} disabled={disabled || locked} onChange={modelChange}/></div>}
            <label className="discussion-development-consent"><input type="checkbox" checked={consent} disabled={disabled || locked || !readyReply} onChange={event => setConsent(event.target.checked)}/><span>{t('consent')}</span></label>
            <div className="discussion-development-actions"><button type="button" className="primary" data-development-create disabled={!canPrepare} onClick={() => void prepare()}><Code2 size={14}/>{t('prepare')}</button></div>
          </>}
        </>}
      </>}
    </div>}
  </section>;
}

/** Load one shared tester directory only once this exact development choice is
 * visible. Every explicit row still uses the original picker's exact GET. */
function DiscussionTestBindings(props: Omit<Parameters<typeof TaskDispatchTestBindings>[0], 'inventory'>) {
  const inventory = useToolList<ToolProfile>(props.project, 'tool-profiles');
  return <TaskDispatchTestBindings {...props} inventory={inventory}/>;
}
