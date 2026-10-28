import {OpenCodeExecutionFacts} from './OpenCodeExecutionFacts';
import {OctopExecutionFacts} from './OctopExecutionFacts';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import {CodexExecutionFacts} from './CodexModelExecutionPanel';
import {ModelExecutionPanel, ModelExecutionFacts, type ModelChoice} from './ModelExecutionPanel';
import {useApi} from './api-context';
import {useKnowledgeEntries} from './knowledge-hooks';
import {publishedKnowledgeReference,pinnedKnowledgeReferences,type KnowledgeSummary} from './knowledge-model';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {RequirementDesignFacts} from './RequirementDesignFacts';
import {TaskDispatchOptionsPanel} from './TaskDispatchOptionsPanel';
import {UiError} from './i18n';
import './knowledge.css';
import {literalText} from './i18n/literal';
import { tr as t, useLocale, formatNumber, renderDiagnostic } from "./i18n";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Play, RefreshCw } from "lucide-react";
import {
  api,
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
  onPlan: (id: string, taskID?: string) => void;
  onClose: () => void;
  onRun: (id: string) => void;
};
export function TaskDispatchDialog({
  project,
  session,
  task,
  planID,
  initialWorkflow,
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
  const [adoptedProfile,setAdoptedProfile]=useState<ToolProfile>(),[profileScope,setProfileScope]=useState('');
  const [knowledgeSelection,setKnowledgeSelection]=useState<KnowledgeSummary[]>([]);
  const knowledge=useKnowledgeEntries(project,{kind:'all',state:'published',q:''},0,!planID||planID==='new');
  const selectionCurrent=knowledgeSelection.every(selected=>knowledge.items?.some(current=>current.id===selected.id&&current.published_eligible&&current.eligibility==='current'&&current.published_revision===selected.published_revision&&current.published_sha256===selected.published_sha256));
  function selectKnowledge(entry:KnowledgeSummary){
    try{
      const ref=publishedKnowledgeReference(entry,project);
      const present=knowledgeSelection.some(item=>item.id===ref.id&&item.published_revision===ref.revision&&item.published_sha256===ref.sha256);
      const next=knowledgeSelection.filter(item=>item.id!==entry.id);
      if(present){setKnowledgeSelection(next);return;}
      if(next.length>=16)throw new UiError('knowledge.errors.selection_limit');
      setKnowledgeSelection([...next,entry]);
    }catch(error){setError(error);}
  }
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
    blocked = action.busy || !!action.pending;
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
    try {
      const id = currentTask?.id || plan?.task_id;
      if (!id) throw Error(t("development.dispatch.the_current_task_is_missing_reopen_from"));
      setCurrentTask(
        await api<RecordItem>(
          "/tasks/" + encodeURIComponent(id) + query(project),
        ),
      );
      await profiles.read();
      await workflows.read();
      action.clearError();
      setError(t("development.dispatch.the_current_task_and_available_configurations_were"));
    } catch (error) {
      setError(error);
    }
  }
  async function prepare() {
    setError("");
    try {
      if (!currentTask || !flow || !profile || !modelReady)
        throw new Error(t("development.dispatch.choose_a_current_task_a_published_workflow"));
      const result = await action.run<TaskDispatchPlan>(
        "/task-dispatch-plans",
        {...dispatchRequest(project, currentTask, flow, agentID, profile),knowledge:pinnedKnowledgeReferences(knowledgeSelection,project),...(modelRequired&&modelChoice.selection?{model_execution_selection:modelChoice.selection}:{}),...(codexRequired&&modelChoice.codexSelection?{codex_execution_selection:modelChoice.codexSelection}:{})},
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
        if (!action.busy && !action.pending) onClose();
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
        {!planID || planID === "new" ? (
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
              {currentTask&&flow&&flow.nodes.some(node=>node.id===agentID&&node.type==='agent')&&<TaskDispatchOptionsPanel
                session={session}
                scope={{project_id:project,task_id:currentTask.id,expected_task_version:currentTask.version,workflow_id:flow.id,workflow_version:flow.version,agent_node_id:agentID}}
                disabled={blocked}
                onAdopt={tool=>{if(!blocked){setProfileScope(selectionScope);setProfileID(tool.id);setAdoptedProfile(tool);}}}
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
                  required
                  value={selectedProfileID}
                  onChange={(event) => {setProfileScope(selectionScope);setProfileID(event.target.value);setAdoptedProfile(undefined);}}
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
              {profile && (
                <div className="tool-context">
                  <strong>{profile.name}</strong>
                  <p>
                    {t("development.dispatch.worker_summary",{worker:profile.worker_id,adapter:profile.profile.adapter,protocol:profile.profile.protocol,version:profile.profile.tool_version})}
                  </p>
                  <code>{profile.profile.profile_sha256}</code>
                </div>
              )}
              {profile&&<ModelExecutionPanel key={modelKey} scope={{project,profile:profile.id,version:profile.version,sha256:profile.profile.profile_sha256}} supported={modelRequired} fixedOpenCode={profile.profile.opencode_execution} fixedOctop={profile.profile.octop_execution} codex={codexRequired} disabled={blocked} onChange={updateModelChoice}/>}
              <section aria-label={t('knowledge.dispatch.title')}>
                <h3>{t('knowledge.dispatch.title')}</h3><p className="muted">{t('knowledge.dispatch.hint')}</p>
                <div className="knowledge-actions"><button type="button" disabled={blocked||knowledge.loading} onClick={()=>void knowledge.read()}><RefreshCw size={14}/>{t('knowledge.dispatch.refresh')}</button><a href={'?'+new URLSearchParams({project,view:'knowledge',sub:'project-knowledge'})}>{t('knowledge.dispatch.open')}</a></div>
                {Boolean(knowledge.error)&&<ErrorNotice error={knowledge.error} onRetry={()=>void knowledge.read(knowledge.more)}/>}
                {knowledge.loading&&!knowledge.items&&<Loading/>}
                <div className="knowledge-dispatch-list">{knowledge.items?.map(entry=>{const checked=knowledgeSelection.some(item=>item.id===entry.id&&item.published_revision===entry.published_revision&&item.published_sha256===entry.published_sha256);return <label className="knowledge-dispatch-choice" key={entry.id}><input type="checkbox" checked={checked} disabled={blocked||!entry.published_eligible||entry.eligibility!=='current'} onChange={()=>selectKnowledge(entry)}/><span><strong>{entry.published_title||entry.id}</strong><small>{t('knowledge.kind.'+entry.kind)} · {t('knowledge.revision',{revision:formatNumber(entry.published_revision)})}{entry.has_draft?' · '+t('knowledge.has_draft'):''}</small>{entry.published_summary&&<small>{entry.published_summary}</small>}<small>{t('knowledge.eligible.'+entry.eligibility)}</small></span></label>;})}</div>
                {knowledge.items?.length===0&&!knowledge.loading&&<p className="muted">{t('knowledge.dispatch.empty')}</p>}
                {knowledge.cursor&&<button type="button" disabled={blocked||knowledge.loading} onClick={()=>void knowledge.read(true)}>{t('knowledge.more')}</button>}
                <p>{t('knowledge.dispatch.selected',{count:knowledgeSelection.length})}</p>
                {knowledgeSelection.length>0&&<><ul>{knowledgeSelection.map(entry=><li key={entry.id}>{entry.published_title||entry.id} · {t('knowledge.revision',{revision:formatNumber(entry.published_revision)})} <button type="button" disabled={blocked} onClick={()=>setKnowledgeSelection(before=>before.filter(item=>item.id!==entry.id))}>{t('knowledge.dispatch.remove')}</button></li>)}</ul>{!selectionCurrent&&<p className="warning">{t('knowledge.errors.ineligible')}</p>}<button type="button" disabled={blocked} onClick={()=>setKnowledgeSelection([])}>{t('knowledge.dispatch.clear')}</button></>}
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
                disabled={
                  blocked ||
                  action.conflict ||
                  !currentTask ||
                  !flow ||
                  !agentID ||
                  !profile?.available ||
                  !modelReady ||
                  profiles.loading ||
                  !!profiles.error ||
                  workflows.loading ||
                  !!workflows.error ||
                  (knowledgeSelection.length>0&&(knowledge.loading||!!knowledge.error||!selectionCurrent))
                }
              >
                {t("development.dispatch.fix_and_preview_execution_plan")}</button>
            </div>
          </form>
        ) : plan ? (
          <>
            <TaskDispatchFacts plan={plan} />
            <ModelExecutionFacts binding={plan.model_execution}/><CodexExecutionFacts binding={plan.codex_execution}/><OpenCodeExecutionFacts capability={plan.opencode_execution}/><OctopExecutionFacts capability={plan.octop_execution}/>
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
