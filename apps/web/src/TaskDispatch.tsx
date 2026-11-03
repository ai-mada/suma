import {ExecutionModelFacts} from './ExecutionModelFacts';
import {DevelopmentContinuationFacts} from './DevelopmentContinuationSelector';
import {CodexNativeSkillFacts} from './CodexNativeSkillFacts';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import {ModelExecutionPanel, type ModelChoice} from './ModelExecutionPanel';
import {useApi} from './api-context';
import type {KnowledgeReference} from './knowledge-model';
import {ProjectKnowledgePicker} from './ProjectKnowledgePicker';
import {knowledgeSelectionKey,validKnowledgePins} from './task-conversation-knowledge-model';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {RequirementDesignFacts} from './RequirementDesignFacts';
import {TaskDispatchOptionsPanel} from './TaskDispatchOptionsPanel';
import {TaskDispatchToolHandoff} from './TaskDispatchToolHandoff';
import {TaskDispatchTestBindings,type TestBindingsChoice} from './TaskDispatchTestBindings';
import type {ToolTaskHandoffPin} from './tool-task-handoff-model';
import {DispatchRecommendation} from './DispatchRecommendation';
import {UiError} from './i18n';
import './knowledge.css';
import {literalText} from './i18n/literal';
import { tr as t, useLocale, formatNumber, renderDiagnostic } from "./i18n";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Play, RefreshCw } from "lucide-react";
import {
  api,
  ApiError,
  query,
  type RecordItem,
  type Run,
  type Session,
  type Workflow,
} from "./api";
import { Empty, ErrorNotice, Field, Loading, Modal } from "./ui";
import { TaskDispatchFacts, TaskDispatchRunFacts } from "./ToolFacts";
import { ToolRecoveryNotice } from "./ToolActions";
import { ToolWriteRecovery } from "./tool-write";
import {
  dispatchRequest,
  taskToolProtocol,
  taskPlanStarted,
  type TaskDispatchPlan,
  type ToolProfile,
} from "./tool-model";
import {
  toolSessionStorage,
  useToolAction,
  useToolList,
  useToolRead,
} from "./tool-hooks";
type Props = {
  project: string;
  session: Session;
  task?: RecordItem;
  planID?: string;
  initialWorkflow?: Workflow;
  handoffTool?: ToolTaskHandoffPin;
  onPlan: (id: string, taskID?: string) => void;
  onClose: () => void;
  onRun: (id: string) => void;
};
export function TaskDispatchDialog(props:Props){
  const api=useApi(),owner=useRef({api,epoch:0});
  if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
  return <TaskDispatchScope key={JSON.stringify([props.session.organization.id,props.session.user.id,props.project,props.task?.id,props.handoffTool,owner.current.epoch])} {...props}/>;
}
function TaskDispatchScope({
  project,
  session,
  task,
  planID,
  initialWorkflow,
  handoffTool,
  onPlan,
  onClose,
  onRun,
}: Props) { const api=useApi(); useLocale();
  const [currentTask, setCurrentTask] = useState(task),
    [flowKey, setFlowKey] = useState(
      initialWorkflow ? initialWorkflow.id + "@" + initialWorkflow.version : "",
    ),
    [agentID, setAgentID] = useState(
      initialWorkflow?.nodes.filter((node) => node.type === "agent").length ===
        1
        ? initialWorkflow.nodes.find((node) => node.type === "agent")?.id || ""
        : "",
    ),
    [profileID, setProfileID] = useState(""),
    [created, setCreated] = useState<TaskDispatchPlan>(),
    [error, setError] = useState<unknown>(""),
    [confirmed, setConfirmed] = useState(false);
  const [adoptedProfile,setAdoptedProfile]=useState<ToolProfile>(),[profileScope,setProfileScope]=useState(''),[recommendationLocked,setRecommendationLocked]=useState(false);
  const [handoffSelectionKey,setHandoffSelectionKey]=useState(''),[manualToolIntent,setManualToolIntent]=useState(0);
  const [knowledgeSelection,setKnowledgeSelection]=useState<KnowledgeReference[]>([]),[knowledgeRefresh,setKnowledgeRefresh]=useState(0),[knowledgeAccessBlocked,setKnowledgeAccessBlocked]=useState(false),[knowledgeValidity,setKnowledgeValidity]=useState<{key:string;valid:boolean}>();
  const accessAlive=useRef(true),accessEpoch=useRef(0),accessFlight=useRef<object|undefined>(undefined),[accessRefreshing,setAccessRefreshing]=useState(false),accessCaller=useRef(api),accessTarget=useRef('');accessCaller.current=api;
  accessTarget.current=JSON.stringify([project,task?.id,task?.version,currentTask?.id]);
  useEffect(()=>{accessAlive.current=true;return()=>{accessAlive.current=false;accessEpoch.current++;};},[]);
  const knowledgeKey=knowledgeSelectionKey(knowledgeSelection,knowledgeRefresh);
  const selectionCurrent=!knowledgeAccessBlocked&&(!knowledgeSelection.length||knowledgeValidity?.key===knowledgeKey&&knowledgeValidity.valid);
  const changeKnowledgeValidity=useCallback((key:string,valid:boolean)=>setKnowledgeValidity(before=>before?.key===key&&before.valid===valid?before:{key,valid}),[]);
  const knowledgeDenied=useCallback(()=>{accessEpoch.current++;setKnowledgeAccessBlocked(true);},[]);
  useEffect(()=>{setKnowledgeSelection([]);},[project,task?.id,session.user.id,session.organization.id]);
  const recovery = useMemo(
      () =>
        new ToolWriteRecovery(session.organization.id +
            ":" +
            session.user.id +
            ":" +
            project +
            ":dispatch", toolSessionStorage(), api, (value,request)=>validateTaskDispatchResponse(value,request,api,project)),
      [session.organization.id, session.user.id, project, api],
    ),
    action = useToolAction(recovery, onClose);
  const workflows = useToolRead<{ items: Workflow[] }>(
      "/workflows" + query(project),
    ),
    profiles = useToolList<ToolProfile>(project, "tool-profiles");
  const planRead = useToolRead<TaskDispatchPlan>(
    planID && planID !== "new"
      ? "/task-dispatch-plans/" + encodeURIComponent(planID) + query(project)
      : "",
  );
  const plan = planRead.data || (created?.id === planID ? created : undefined),
    flow = workflows.data?.items.find(
      (item) => item.id + "@" + item.version === flowKey,
    ),
    directBlocked = action.busy || !!action.pending,
    blocked = directBlocked || recommendationLocked;
  const recommendationKnowledge=useMemo(()=>selectionCurrent&&validKnowledgePins(knowledgeSelection)?knowledgeSelection:undefined,[knowledgeSelection,selectionCurrent]);
  const selectionScope=JSON.stringify([session.organization.id,session.user.id,project,currentTask?.id,currentTask?.version,flow?.id,flow?.version,agentID]);
  const selectedProfileID=profileScope===selectionScope?profileID:'';
  const scopedAdopted=profileScope===selectionScope?adoptedProfile:undefined;
  // An adopted recommendation can belong to another inventory page. Keep its
  // exact current profile version in the existing selector until preview fixes
  // it, rather than silently substituting an older loaded version of that ID.
  const visibleProfiles=scopedAdopted?[...(profiles.items||[]).filter(item=>item.id!==scopedAdopted.id),scopedAdopted]:profiles.items||[];
  const profile=visibleProfiles.find(item=>item.id===selectedProfileID);
  const modelKey=JSON.stringify([selectionScope,profile?.id,profile?.version,profile?.profile.profile_sha256]);
  const [modelChoice,setModelChoice]=useState<ModelChoice & {key:string}>({key:'',ready:false});
  const updateModelChoice=useCallback((choice:ModelChoice)=>setModelChoice({...choice,key:modelKey}),[modelKey]);
  const modelRequired=profile?.profile.model_execution_protocol==='suma-model-execution-v1';
  const codexRequired=profile?.profile.protocol==='codex-app-server-v1';
  const modelReady=!(modelRequired||codexRequired)||modelChoice.key===modelKey&&modelChoice.ready;
  const testBasis=JSON.stringify([selectionScope,profile?.id,profile?.version,profile?.profile.profile_sha256,profile?.worker_id]);
  const [testChoice,setTestChoice]=useState<TestBindingsChoice&{key:string}>({key:'',ready:true,hasIntent:false,selections:[]});
  const testOwner=useRef({api,key:testBasis,epoch:0});
  if(testOwner.current.api!==api||testOwner.current.key!==testBasis)testOwner.current={api,key:testBasis,epoch:testOwner.current.epoch+1};
  const testEpoch=testOwner.current.epoch;
  const testScope=JSON.stringify([testBasis,testEpoch]);
  const currentTests=testChoice.key===testScope?testChoice:{ready:true,hasIntent:false,selections:[]};
  const updateTests=useCallback((value:TestBindingsChoice)=>{if(testOwner.current.api===api&&testOwner.current.key===testBasis&&testOwner.current.epoch===testEpoch)setTestChoice({...value,key:testScope});},[api,testBasis,testScope,testEpoch]);
  const handoffScope=JSON.stringify([selectionScope,manualToolIntent]);
  const actualProfileKey=profile?JSON.stringify([selectionScope,profile.id,profile.version,profile.profile.profile_sha256]):'';
  const handoffAdopted=!!actualProfileKey&&handoffSelectionKey===actualProfileKey;
  const handoffOwner=useRef({api,scope:handoffScope,epoch:0,blocked,adopted:handoffAdopted});
  if(handoffOwner.current.api!==api||handoffOwner.current.scope!==handoffScope)
    handoffOwner.current={api,scope:handoffScope,epoch:handoffOwner.current.epoch+1,blocked,adopted:handoffAdopted};
  handoffOwner.current.blocked=blocked;
  handoffOwner.current.adopted=handoffAdopted;
  const handoffEpoch=handoffOwner.current.epoch;
  useEffect(()=>{
    if(!blocked&&profileScope!==selectionScope){setProfileID('');setAdoptedProfile(undefined);setProfileScope(selectionScope);}
  },[selectionScope,profileScope,blocked]);
  useEffect(() => {
    setCurrentTask(task);
  }, [task?.id, task?.version]);
  useEffect(() => {
    setConfirmed(false);
  }, [planID]);
  function saved(path: string, value: unknown) {
    if (!value || typeof value !== "object") return;
    if (path.endsWith("/start")) {
      const run = value as Run;
      if (run.id) onRun(run.id);
      return;
    }
    const result = value as TaskDispatchPlan;
    setCreated(result);
    setConfirmed(false);
    onPlan(result.id, result.task_id);
  }
  async function refreshTask() {
    if(!accessAlive.current||accessFlight.current||blocked)return;
    const ticket=++accessEpoch.current,owner={},originalCaller=api,target=accessTarget.current;accessFlight.current=owner;setAccessRefreshing(true);
    const owns=()=>accessAlive.current&&accessCaller.current===originalCaller&&accessEpoch.current===ticket&&accessFlight.current===owner&&accessTarget.current===target;
    try {
      const id = currentTask?.id || plan?.task_id;
      if (!id) throw Error(t("development.dispatch.the_current_task_is_missing_reopen_from"));
      const refreshed=await api<RecordItem>("/tasks/"+encodeURIComponent(id)+query(project));
      if(!owns())return;
      const [profilesAccepted,workflowsAccepted]=await Promise.all([profiles.read(),workflows.read()]);
      if(!owns())return;
      if(!profilesAccepted||!workflowsAccepted){setKnowledgeAccessBlocked(true);return;}
      setCurrentTask(refreshed);
      setKnowledgeRefresh(value=>value+1);setKnowledgeAccessBlocked(false);
      action.clearError();
      setError(t("development.dispatch.the_current_task_and_available_configurations_were"));
    } catch (error) {
      if(owns()){if(error instanceof ApiError&&[401,403,404].includes(error.status))knowledgeDenied();setError(error);}
    }finally{if(accessFlight.current===owner){accessFlight.current=undefined;if(accessAlive.current)setAccessRefreshing(false);}}
  }
  async function prepare() {
    setError("");
    try {
      if (!currentTask || !flow || !profile || !modelReady || !currentTests.ready)
        throw new Error(t("development.dispatch.choose_a_current_task_a_published_workflow"));
      if(blocked||!selectionCurrent||!validKnowledgePins(knowledgeSelection))throw new UiError('knowledge.errors.ineligible');
      const result = await action.run<TaskDispatchPlan>(
        "/task-dispatch-plans",
        {...dispatchRequest(project, currentTask, flow, agentID, profile,currentTests.selections),knowledge:knowledgeSelection,...(modelRequired&&modelChoice.selection?{model_execution_selection:modelChoice.selection}:{}),...(codexRequired&&modelChoice.codexSelection?{codex_execution_selection:modelChoice.codexSelection}:{})},
      );
      if (result) saved("/task-dispatch-plans", result);
    } catch (error) {
      setError(error);
    }
  }
  return (
    <Modal
      open
      onClose={() => {
        if (!blocked) onClose();
        else setError(new UiError('dispatch-recommendation.draft_guard'));
      }}
      title={t("development.dispatch.arrange_task_execution")}
      description={t("development.dispatch.fix_task_and_requirement_context_first_review")}
    >
      <div className="tool-dispatch">
        <ToolRecoveryNotice action={action} onRestored={saved} />
        {!!error && <ErrorNotice error={renderDiagnostic(error)} />}
        {planRead.error && (
          <ErrorNotice
            error={planRead.error}
            onRetry={() => void planRead.read()}
          />
        )}{" "}
        {planRead.loading && <p role="status">{t("development.dispatch.reading_immutable_execution_plan")}</p>}
        {knowledgeAccessBlocked?<section className="tool-notice" role="alert"><p>{t('knowledge-consumers.access_denied')}</p><button type="button" disabled={blocked||accessRefreshing} onClick={()=>void refreshTask()}>{t('knowledge-consumers.recheck_access')}</button></section>:!planID || planID === "new" ? (
          <>
          {currentTask&&flow&&recommendationKnowledge&&flow.nodes.some(node=>node.id===agentID&&node.type==='agent')&&<DispatchRecommendation session={session} scope={{project_id:project,task_id:currentTask.id,expected_task_version:currentTask.version,workflow_id:flow.id,workflow_version:flow.version,agent_node_id:agentID,knowledge:recommendationKnowledge}} inventory={profiles} disabled={directBlocked} creationDisabled={currentTests.hasIntent||!currentTests.ready} testBindings={{...currentTests,workflow_id:flow.id,workflow_version:flow.version,agent_node_id:agentID}} onLockChange={setRecommendationLocked} onPrepared={plan=>saved('/task-dispatch-plans',plan)} onOpenPlan={id=>onPlan(id,currentTask.id)} onManual={()=>{requestAnimationFrame(()=>document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.focus());}}/>}
          <form
            className="tool-form"
            onSubmit={(event) => {
              event.preventDefault();
              void prepare();
            }}
          >
            <p>
              {t("development.dispatch.this_prepares_a_fixed_plan_without_calling")}</p>
            <button
              type="button"
              disabled={blocked}
              onClick={() => {
                void profiles.read();
                void workflows.read();
              }}
            >
              <RefreshCw size={14} />
              {t("development.dispatch.reload_workflow_and_tool_conditions")}</button>
            {currentTask ? (
              <div className="tool-context">
                <strong>
                  {currentTask.title || currentTask.id} · v{currentTask.version}
                </strong>
                <p>
                  {t("development.dispatch.the_server_reads_task_description_acceptance_criteria")}</p>
              </div>
            ) : (
              <Empty
                title={t("development.dispatch.missing_dispatch_task")}
                description={t("development.dispatch.open_execution_planning_from_task_details_in")}
              />
            )}
            <fieldset className="form-fields" disabled={blocked}>
              {workflows.error && (
                <ErrorNotice
                  error={workflows.error}
                  onRetry={() => void workflows.read()}
                />
              )}
              <Field label={t("development.dispatch.published_workflow_version")}>
                <select
                  required
                  value={flowKey}
                  onChange={(event) => {
                    setFlowKey(event.target.value);
                    const selected = workflows.data?.items.find(
                      (item) =>
                        item.id + "@" + item.version === event.target.value,
                    );
                    const agents =
                      selected?.nodes.filter((node) => node.type === "agent") ||
                      [];
                    setAgentID(agents.length === 1 ? agents[0].id : "");
                  }}
                >
                  <option value="">{t("development.dispatch.choose_a_fixed_workflow_with_an_agent")}</option>
                  {workflows.data?.items
                    .filter((item) =>
                      item.nodes.some((node) => node.type === "agent"),
                    )
                    .map((item) => (
                      <option
                        value={item.id + "@" + item.version}
                        key={item.id + "@" + item.version}
                      >
                        {item.name} · v{item.version}
                      </option>
                    ))}
                </select>
              </Field>
              <Field label={t("development.dispatch.node_receiving_the_fixed_task_context")}>
                <select
                  required
                  value={agentID}
                  onChange={(event) => setAgentID(event.target.value)}
                >
                  <option value="">{t("development.dispatch.choose_this_execution_s_agent_node")}</option>
                  {flow?.nodes
                    .filter((node) => node.type === "agent")
                    .map((node) => (
                      <option value={node.id} key={node.id}>
                        {node.name} · {node.id}
                      </option>
                    ))}
                </select>
              </Field>
              {handoffTool&&currentTask&&flow&&flow.nodes.some(node=>node.id===agentID&&node.type==='agent')&&<TaskDispatchToolHandoff
                project={project} pin={handoffTool} scopeKey={handoffScope} disabled={blocked} adopted={handoffAdopted}
                onInvalidate={()=>{const owner=handoffOwner.current;if(owner.api!==api||owner.scope!==handoffScope||owner.epoch!==handoffEpoch||owner.blocked||!owner.adopted)return;setProfileID('');setAdoptedProfile(undefined);setHandoffSelectionKey('');setModelChoice({key:'',ready:false});}}
                onAdopt={tool=>{const owner=handoffOwner.current;if(owner.api!==api||owner.scope!==handoffScope||owner.epoch!==handoffEpoch||owner.blocked)return;setProfileScope(selectionScope);setProfileID(tool.id);setAdoptedProfile(tool);setHandoffSelectionKey(JSON.stringify([selectionScope,tool.id,tool.version,tool.profile.profile_sha256]));}} />}
              {currentTask&&flow&&flow.nodes.some(node=>node.id===agentID&&node.type==='agent')&&<TaskDispatchOptionsPanel
                session={session}
                scope={{project_id:project,task_id:currentTask.id,expected_task_version:currentTask.version,workflow_id:flow.id,workflow_version:flow.version,agent_node_id:agentID}}
                disabled={blocked}
                onAdopt={tool=>{if(!blocked){setManualToolIntent(value=>value+1);setHandoffSelectionKey('');setProfileScope(selectionScope);setProfileID(tool.id);setAdoptedProfile(tool);}}}
                onUnavailable={()=>{if(!blocked){setProfileID('');setAdoptedProfile(undefined);}}}
              />}
              {profiles.error && (
                <ErrorNotice
                  error={profiles.error}
                  onRetry={() =>
                    void profiles.read(profiles.error_action === "more")
                  }
                />
              )}
              <Field
                label={t("development.dispatch.actual_tool_configuration")}
                hint={t("development.dispatch.verified_tool_requirements")}
              >
                <select
                  data-dispatch-manual-tool
                  required
                  value={selectedProfileID}
                  onChange={(event) => {setManualToolIntent(value=>value+1);setHandoffSelectionKey('');setProfileScope(selectionScope);setProfileID(event.target.value);setAdoptedProfile(undefined);}}
                >
                  <option value="">{t("development.dispatch.choose_a_dispatchable_tool")}</option>
                  {visibleProfiles.map((item) => (
                    <option
                      key={item.id}
                      value={item.id}
                      disabled={
                        item.state !== "enabled" ||
                        !item.available ||
                        item.profile.read_only ||
                        !taskToolProtocol(item.profile)
                      }
                    >
                      {literalText("development.dispatch.tool_option",{name:item.name,worker:item.worker_id,version:formatNumber(item.version),status:!taskToolProtocol(item.profile)?t("development.dispatch.unsupported"):item.profile.read_only?t("development.dispatch.read_only"):item.state!=="enabled"||!item.available?t("development.dispatch.blocked"):t("development.dispatch.eligible")})}
                    </option>
                  ))}
                </select>
              </Field>
              {profiles.next_cursor && (
                <button
                  type="button"
                  disabled={profiles.loading}
                  onClick={() => void profiles.read(true)}
                >
                  {t("development.dispatch.load_more_tool_configurations")}</button>
              )}
              <small>
                {t("development.tools.loaded_count",{loaded:formatNumber(profiles.items?.length||0),total:formatNumber(profiles.total)})}{" "}<a
                  href={
                    "?" +
                    new URLSearchParams({
                      project,
                      view: "settings",
                      sub: "tools",
                    })
                  }
                >
                  {t("development.dispatch.view_tool_configurations_and_unavailable_reasons")}</a>
              </small>
              {currentTask&&flow&&profile&&<TaskDispatchTestBindings project={project} workflow={flow} agentID={agentID} developer={profile} scopeKey={testScope} inventory={profiles} disabled={blocked} onChange={updateTests}/>}
              {profile && (
                <div className="tool-context">
                  <strong>{profile.name}</strong>
                  <p>
                    {t("development.dispatch.worker_summary",{worker:profile.worker_id,adapter:profile.profile.adapter,protocol:profile.profile.protocol,version:profile.profile.tool_version})}
                  </p>
                  <code>{profile.profile.profile_sha256}</code>
                </div>
              )}
              {profile&&<ModelExecutionPanel key={modelKey} scope={{project,profile:profile.id,version:profile.version,sha256:profile.profile.profile_sha256}} supported={modelRequired} fixedOpenCode={profile.profile.opencode_execution} fixedClaude={profile.profile.claude_execution} fixedClaudeSDK={profile.profile.claude_sdk_execution} fixedACP={profile.profile.acp_execution} codex={codexRequired} disabled={blocked} onChange={updateModelChoice}/>}
              <section aria-label={t('knowledge.dispatch.title')}>
                <h3>{t('knowledge.dispatch.title')}</h3><p className="muted">{t('knowledge.dispatch.hint')}</p>
                <ProjectKnowledgePicker userID={session.user.id} organizationID={session.organization.id} project={project} taskID={currentTask?.id||''} selected={knowledgeSelection} onChange={setKnowledgeSelection} disabled={blocked} accessBlocked={knowledgeAccessBlocked} refresh={knowledgeRefresh} onValidityChange={changeKnowledgeValidity} onAccessDenied={knowledgeDenied}/>
                <a href={'?'+new URLSearchParams({project,view:'knowledge',sub:'project-knowledge'})}>{t('knowledge.dispatch.open')}</a>
              </section>
            </fieldset>
            {action.conflict && (
              <div className="conflict-banner">
                <p>
                  {t("development.dispatch.task_or_configuration_context_changed_your_selections")}</p>
                <button type="button" onClick={() => void refreshTask()}>
                  {t("development.dispatch.read_current_context_keep_selections")}</button>
              </div>
            )}
            <div className="dialog-actions">
              <button type="button" disabled={blocked} onClick={onClose}>
                {t("development.workflow.cancel")}</button>
              <button
                className="primary"
                data-dispatch-prepare
                disabled={
                  blocked ||
                  action.conflict ||
                  !currentTask ||
                  !flow ||
                  !agentID ||
                  !profile?.available ||
                  !modelReady ||
                  !currentTests.ready ||
                  profiles.loading ||
                  !!profiles.error ||
                  workflows.loading ||
                  !!workflows.error ||
                  !selectionCurrent
                }
              >
                {t("development.dispatch.fix_and_preview_execution_plan")}</button>
            </div>
          </form>
          </>
        ) : plan ? (
          <>
            <TaskDispatchFacts plan={plan} />
            {plan.development_continuation&&<DevelopmentContinuationFacts binding={plan.development_continuation} project={project} taskID={plan.task_id}/>}
            <ExecutionModelFacts binding={plan.model_execution} codex={plan.codex_execution} openCode={plan.opencode_execution} claude={plan.claude_execution} claudeSDK={plan.claude_sdk_execution} acp={plan.acp_execution}/>
            {plan.codex_execution?<CodexNativeSkillFacts mode="plan" bundle={plan.execution_skills}/>:plan.context.knowledge?.some(item=>item.kind==='skill')&&<CodexNativeSkillFacts mode="text"/>}
            <RequirementDesignFacts project={project} value={plan.context.design??undefined}/>
            <KnowledgeContextFacts project={project} items={plan.context.knowledge}/>
            <TaskDispatchRunFacts plan={plan} onRun={onRun} />
            {!taskPlanStarted(plan) && (
              <>
                <label className="check tool-start-confirm">
                  <input
                    type="checkbox"
                    checked={confirmed}
                    disabled={blocked || !plan.valid}
                    onChange={(event) => setConfirmed(event.target.checked)}
                  />
                  {t("development.dispatch.i_reviewed_the_fixed_task_requirement_tool")}</label>
                {action.conflict && (
                  <p className="warning">
                    {t("development.dispatch.the_original_plan_or_current_context_changed")}</p>
                )}
                <div className="dialog-actions">
                  <button
                    type="button"
                    disabled={blocked}
                    onClick={() => {
                      setCreated(undefined);
                      setKnowledgeSelection([]);
                      setConfirmed(false);
                      action.clearError();
                      onPlan("new");
                      void refreshTask();
                    }}
                  >
                    <RefreshCw size={14} />
                    {t("development.dispatch.prepare_a_new_plan")}</button>
                  <button
                    className="primary"
                    disabled={
                      blocked ||
                      action.conflict ||
                      !confirmed ||
                      !plan.valid ||
                      planRead.loading ||
                      !!planRead.error
                    }
                    onClick={async () => {
                      const path =
                        "/task-dispatch-plans/" +
                        encodeURIComponent(plan.id) +
                        "/start";
                      const result = await action.run<Run>(path, {
                        project_id: project,
                        expected_version: plan.version,
                      });
                      if (result) saved(path, result);
                    }}
                  >
                    <Play size={14} />
                    {t("development.dispatch.confirm_starting_the_fixed_plan")}</button>
                </div>
              </>
            )}
          </>
        ) : !planRead.loading && !planRead.error ? (
          <Loading />
        ) : null}
      </div>
    </Modal>
  );
}
