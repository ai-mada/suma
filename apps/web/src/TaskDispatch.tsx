import { useEffect, useMemo, useState } from "react";
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
  toolError,
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
}: Props) {
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
    [error, setError] = useState(""),
    [confirmed, setConfirmed] = useState(false);
  const recovery = useMemo(
      () =>
        new ToolWriteRecovery(
          session.organization.id +
            ":" +
            session.user.id +
            ":" +
            project +
            ":dispatch",
          toolSessionStorage(),
        ),
      [session.organization.id, session.user.id, project],
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
    profile = profiles.items?.find((item) => item.id === profileID),
    blocked = action.busy || !!action.pending;
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
      if (!id) throw Error("缺少当前任务，请从任务详情重新打开。");
      setCurrentTask(
        await api<RecordItem>(
          "/tasks/" + encodeURIComponent(id) + query(project),
        ),
      );
      await profiles.read();
      await workflows.read();
      action.clearError();
      setError("已读取当前任务和可选配置；请重新核对后生成新计划。");
    } catch (error) {
      setError(toolError(error));
    }
  }
  async function prepare() {
    setError("");
    try {
      if (!currentTask || !flow || !profile)
        throw new Error("请选择当前任务、已发布流程与实际可派发工具。");
      const result = await action.run<TaskDispatchPlan>(
        "/task-dispatch-plans",
        dispatchRequest(project, currentTask, flow, agentID, profile),
      );
      if (result) saved("/task-dispatch-plans", result);
    } catch (error) {
      setError(toolError(error));
    }
  }
  return (
    <Modal
      open
      onClose={() => {
        if (!action.busy && !action.pending) onClose();
      }}
      title="安排任务执行"
      description="先固定任务与需求依据，审阅工具、流程和独立测试，再显式启动同一计划。"
    >
      <div className="tool-dispatch">
        <ToolRecoveryNotice action={action} onRestored={saved} />
        {error && <ErrorNotice error={error} />}
        {planRead.error && (
          <ErrorNotice
            error={planRead.error}
            onRetry={() => void planRead.read()}
          />
        )}{" "}
        {planRead.loading && <p role="status">正在读取不可变执行计划…</p>}
        {!planID || planID === "new" ? (
          <form
            className="tool-form"
            onSubmit={(event) => {
              event.preventDefault();
              void prepare();
            }}
          >
            <p>
              本次只准备固定计划，不调用模型或启动进程。任务负责人和机器执行身份分别保留。
            </p>
            <button
              type="button"
              disabled={blocked}
              onClick={() => {
                void profiles.read();
                void workflows.read();
              }}
            >
              <RefreshCw size={14} />
              重新读取流程与工具条件
            </button>
            {currentTask ? (
              <div className="tool-context">
                <strong>
                  {currentTask.title || currentTask.id} · v{currentTask.version}
                </strong>
                <p>
                  服务端会在同一事务中读取任务说明、验收条目及关联需求；它们会作为明确标记的外部上下文交给工具。
                </p>
              </div>
            ) : (
              <Empty
                title="缺少派发任务"
                description="请从当前项目任务详情打开安排执行。"
              />
            )}
            <fieldset className="form-fields" disabled={blocked}>
              {workflows.error && (
                <ErrorNotice
                  error={workflows.error}
                  onRetry={() => void workflows.read()}
                />
              )}
              <Field label="已发布流程版本">
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
                  <option value="">选择包含智能体节点的固定流程</option>
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
              <Field label="接收固定任务上下文的节点">
                <select
                  required
                  value={agentID}
                  onChange={(event) => setAgentID(event.target.value)}
                >
                  <option value="">选择本次智能体节点</option>
                  {flow?.nodes
                    .filter((node) => node.type === "agent")
                    .map((node) => (
                      <option value={node.id} key={node.id}>
                        {node.name} · {node.id}
                      </option>
                    ))}
                </select>
              </Field>
              {profiles.error && (
                <ErrorNotice
                  error={profiles.error}
                  onRetry={() =>
                    void profiles.read(profiles.error_action === "more")
                  }
                />
              )}
              <Field
                label="真实工具配置"
                hint="开发派发当前使用已核实任务输入与终态的 Suma／Codex 结构化工具，并须启用且具有匹配新鲜观察；其他工具可用于独立命令流程。服务端在预览和启动时再次核验。"
              >
                <select
                  required
                  value={profileID}
                  onChange={(event) => setProfileID(event.target.value)}
                >
                  <option value="">选择可派发工具</option>
                  {profiles.items?.map((item) => (
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
                      {item.name} · {item.worker_id} · v{item.version}
                      {!taskToolProtocol(item.profile)
                        ? " · 暂不用于任务派发，可用于独立命令流程"
                        : item.profile.read_only
                          ? " · 只读配置，不用于任务开发"
                          : item.state !== "enabled" || !item.available
                            ? " · 当前受阻"
                            : ""}
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
                  加载更多工具配置
                </button>
              )}
              <small>
                已加载 {profiles.items?.length || 0} / {profiles.total} 项。
                <a
                  href={
                    "?" +
                    new URLSearchParams({
                      project,
                      view: "settings",
                      sub: "tools",
                    })
                  }
                >
                  查看工具配置和不可用原因
                </a>
              </small>
              {profile && (
                <div className="tool-context">
                  <strong>{profile.name}</strong>
                  <p>
                    执行节点 {profile.worker_id} · {profile.profile.adapter} /{" "}
                    {profile.profile.protocol} · {profile.profile.tool_version}
                  </p>
                  <code>{profile.profile.profile_sha256}</code>
                </div>
              )}
            </fieldset>
            {action.conflict && (
              <div className="conflict-banner">
                <p>
                  任务或配置依据已变化，选择仍保留。先读取最新版本，再明确创建新预览。
                </p>
                <button type="button" onClick={() => void refreshTask()}>
                  读取当前依据，保留选择
                </button>
              </div>
            )}
            <div className="dialog-actions">
              <button type="button" disabled={blocked} onClick={onClose}>
                取消
              </button>
              <button
                className="primary"
                disabled={
                  blocked ||
                  action.conflict ||
                  !currentTask ||
                  !flow ||
                  !agentID ||
                  !profile?.available ||
                  profiles.loading ||
                  !!profiles.error ||
                  workflows.loading ||
                  !!workflows.error
                }
              >
                固定并预览执行计划
              </button>
            </div>
          </form>
        ) : plan ? (
          <>
            <TaskDispatchFacts plan={plan} />
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
                  我已核对固定任务、需求、工具及测试范围。启动只创建运行，不代表测试或人工验收完成。
                </label>
                {action.conflict && (
                  <p className="warning">
                    原计划或当前依据已变化。不能只替换版本重试启动；读取当前任务后重新准备新计划，原计划继续保留。
                  </p>
                )}
                <div className="dialog-actions">
                  <button
                    type="button"
                    disabled={blocked}
                    onClick={() => {
                      setCreated(undefined);
                      setConfirmed(false);
                      action.clearError();
                      onPlan("new");
                      void refreshTask();
                    }}
                  >
                    <RefreshCw size={14} />
                    重新准备新计划
                  </button>
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
                    确认启动固定计划
                  </button>
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
