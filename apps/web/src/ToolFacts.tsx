import { DispatchRouteFacts } from "./execution-pools/DispatchRouteFacts";
import { stateLabel } from "./api";
import type { ToolEvidence as ContractToolEvidence } from "../../../packages/contracts/src";
import {
  taskPlanStarted,
  toolEventNames,
  toolStateNames,
  type ToolEvidence,
  type ToolProfile,
  type TaskDispatchPlan,
  type ToolEvent,
  type ToolUsage,
  type UsageState,
  type TaskContextRecord,
} from "./tool-model";
import "./tool.css";
export function TaskDispatchRunFacts({
  plan,
  onRun,
}: {
  plan: Pick<TaskDispatchPlan, "state" | "run_id">;
  onRun: (id: string) => void;
}) {
  if (!taskPlanStarted(plan))
    return (
      <p className="muted">
        {plan.run_id && (
          <>
            预留运行编号：<code>{plan.run_id}</code>。
          </>
        )}
        计划尚未创建正式运行，请审阅后显式启动。
      </p>
    );
  if (!plan.run_id)
    return (
      <p className="warning">
        计划已记录启动，但运行标识未读取。请重新读取并核对原操作，不要创建第二次执行。
      </p>
    );
  return (
    <div className="success">
      <p>本计划已关联正式运行，运行结果与任务验收分别处理。</p>
      <button type="button" onClick={() => onRun(plan.run_id)}>
        查看本计划真实运行
      </button>
    </div>
  );
}
function stamp(value?: string) {
  return value
    ? new Date(value).toLocaleString("zh-CN", { hour12: false }) + " · " + value
    : "尚无正式观察";
}
export function ToolProfileFacts({ profile }: { profile: ToolProfile }) {
  return (
    <section className="tool-facts" aria-label="固定工具配置">
      <div className="tool-section-heading">
        <h3>{profile.name}</h3>
        <span className="badge">
          {toolStateNames[profile.state] || profile.state}
        </span>
      </div>
      <dl>
        <dt>执行节点身份</dt>
        <dd>
          <code>{profile.worker_id}</code>
        </dd>
        <dt>命令与协议</dt>
        <dd>
          {profile.profile.command_id} · {profile.profile.adapter} /{" "}
          {profile.profile.protocol}
        </dd>
        <dt>固定版本</dt>
        <dd>{profile.profile.tool_version}</dd>
        <dt>操作版本</dt>
        <dd>v{profile.version}</dd>
        <dt>执行许可</dt>
        <dd>
          {profile.profile.read_only
            ? "只读命令"
            : "许可写入，按尝试隔离工作树"}
        </dd>
        <dt>配置指纹</dt>
        <dd>
          <code>{profile.profile.profile_sha256}</code>
        </dd>
        <dt>程序指纹</dt>
        <dd>
          <code>{profile.profile.executable_sha256}</code>
        </dd>
        <dt>最近上报</dt>
        <dd>{stamp(profile.last_seen_at)}</dd>
        <dt>当前派发条件</dt>
        <dd>
          {profile.available
            ? "上次读取可派发，启动时再次核验"
            : "未确认可派发"}
        </dd>
      </dl>
      <p className="muted">
        版本与探测不代表模型登录或任务完成，外部执行、平台测试和人工验收分别留存。
      </p>
      {profile.reason && (
        <div className="warning">
          <p>{toolReason(profile.reason)}</p>
          <details>
            <summary>原始原因</summary>
            <code>{profile.reason}</code>
          </details>
        </div>
      )}
      {profile.observation && (
        <details>
          <summary>最近一次非秘密观察</summary>
          <dl>
            <dt>当时命令可用</dt>
            <dd>
              {profile.observation.available ? "是，仅限该次本机探测" : "否"}
            </dd>
            <dt>当时版本</dt>
            <dd>{profile.observation.profile.tool_version || "未提供"}</dd>
            <dt>当时配置指纹</dt>
            <dd>
              <code>
                {profile.observation.profile.profile_sha256 || "未提供"}
              </code>
            </dd>
          </dl>
        </details>
      )}
    </section>
  );
}
export function toolReason(reason: string) {
  if (/[\u3400-\u9fff]/.test(reason)) return reason;
  return (
    (
      {
        inventory_stale:
          "执行节点上报已过期。请启动匹配配置的 Runner，等待新观察后重新读取。",
        worker_revoked: "执行节点凭证已撤销，不能再派发。",
        profile_paused: "配置已暂停。核对固定内容后需显式启用。",
        profile_revoked: "配置已永久撤销，历史证据保留。",
        profile_mismatch: "本机观察与固定配置指纹不匹配，请登记新的配置。",
        activation_authority_revoked:
          "原启用人的当前权限已失效，需要有权人员重新核对。",
      } as Record<string, string>
    )[reason] ||
    "当前条件未满足，请核对原始原因与最近观察；保存配置不代表可执行。"
  );
}
function ContextFacts({
  value,
  title,
}: {
  value: TaskContextRecord;
  title: string;
}) {
  return (
    <section className="tool-context">
      <h4>
        {title} · v{value.version}
      </h4>
      <strong>{value.title}</strong>
      <code className="muted">{value.id}</code>
      <p className="preserve">{value.description || "未填写说明"}</p>
      <h5>固定验收条目</h5>
      {value.acceptance_criteria.length ? (
        <ul>
          {value.acceptance_criteria.map((text, index) => (
            <li key={index}>{text}</li>
          ))}
        </ul>
      ) : (
        <p className="muted">未填写验收条目</p>
      )}
    </section>
  );
}
export function TaskDispatchFacts({ plan }: { plan: TaskDispatchPlan }) {
  const agent = plan.workflow.nodes.find(
      (node) => node.id === plan.agent_node_id,
    ),
    tests = plan.workflow.nodes.filter((node) => node.type === "test");
  return (
    <section className="tool-plan-facts" aria-label="不可变任务执行计划">
      <div className="tool-section-heading">
        <h3>固定执行依据</h3>
        <span className="badge">
          {plan.valid ? "当前依据有效" : "当前依据受阻"}
        </span>
      </div>
      <dl>
        <dt>计划</dt>
        <dd>
          <code>{plan.id}</code> · v{plan.version}
        </dd>
        <dt>原流程版本</dt>
        <dd>
          {plan.workflow_id} · v{plan.workflow_version}
        </dd>
        <dt>智能体节点</dt>
        <dd>{agent?.name || plan.agent_node_id}</dd>
        <dt>工具配置</dt>
        <dd>
          {plan.tool_profile_id} · v{plan.tool_profile_version}
        </dd>
        <dt>工具配置指纹</dt>
        <dd>
          <code>{plan.tool_profile_sha256}</code>
        </dd>
        <dt>失效时间</dt>
        <dd>{stamp(plan.expires_at)}</dd>
      </dl>
      {plan.reason && <p className="warning">{plan.reason}</p>}
      <DispatchRouteFacts route={plan.execution_route} />
      <ContextFacts title="任务" value={plan.context.task} />
      {plan.context.requirement && (
        <ContextFacts title="需求" value={plan.context.requirement} />
      )}
      <section className="tool-context">
        <h4>独立测试与人工验收</h4>
        {tests.length ? (
          <ul>
            {tests.map((node) => (
              <li key={node.id}>
                {node.name} · <code>{node.id}</code>
              </li>
            ))}
          </ul>
        ) : (
          <p className="warning">
            本流程没有独立测试节点，不构成完整验收依据。
          </p>
        )}
        <p>
          工具完成只形成执行事实；独立测试与人工验收继续使用平台门禁。启动时再次核对任务、需求、权限、工具与实时观察。
        </p>
      </section>
      {typeof agent?.config?.goal === "string" && agent.config.goal !== "" && (
        <details>
          <summary>将交给工具的完整固定输入</summary>
          <pre className="output">{String(agent.config.goal)}</pre>
        </details>
      )}
    </section>
  );
}
export function UsageFacts({
  usage,
  usageState,
  step = false,
}: {
  usage?: ToolUsage;
  usageState?: UsageState;
  step?: boolean;
}) {
  const entries: [keyof ToolUsage, string][] = [
    ["input_tokens", "输入 Token"],
    ["cached_input_tokens", "其中缓存输入"],
    ["cache_write_input_tokens", "缓存写入输入"],
    ["output_tokens", "输出 Token"],
    ["reasoning_output_tokens", "其中推理输出"],
  ];
  return (
    <section aria-label="报告用量与完整度">
      <p>
        {usageState === "complete"
          ? step
            ? "本步骤已报告用量；不代表整轮总量"
            : "本轮完整已报告用量"
          : usageState === "partial"
            ? "部分已报告小计，不代表整轮完整总量"
            : usageState === "unreported"
              ? "未报告用量"
              : "用量完整度未报告"}
      </p>
      {usageState !== "unreported" && usage && (
        <dl>
          {entries.map(([key, title]) => (
            <div className="tool-usage-row" key={key}>
              <dt>{title}</dt>
              <dd>
                {typeof usage[key] === "number" &&
                Number.isSafeInteger(usage[key]) &&
                usage[key]! >= 0
                  ? usage[key]
                  : "未报告"}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
export function ToolExecutionFacts({
  state,
  evidence,
  events = [],
}: {
  state: string;
  evidence?: ToolEvidence | ContractToolEvidence;
  events?: ToolEvent[];
}) {
  if (!evidence && !events.length) return null;
  const recent = events.slice(-40),
    last = events.at(-1),
    session =
      evidence?.session_id ||
      [...events].reverse().find((event) => event.session_id)?.session_id,
    usageEvent = [...events]
      .reverse()
      .find(
        (event) => event.kind === "usage" || event.usage_state || event.usage,
      ),
    usage = evidence ? evidence.usage : usageEvent?.usage,
    reportedState = evidence ? evidence.usage_state : usageEvent?.usage_state,
    usageState =
      reportedState === "complete" ||
      reportedState === "partial" ||
      reportedState === "unreported"
        ? reportedState
        : undefined;
  return (
    <section className="tool-execution-facts" aria-label="外部工具执行事实">
      <h4>外部工具执行事实</h4>
      <dl>
        <dt>平台执行状态</dt>
        <dd>{stateLabel(state)}</dd>
        {evidence && (
          <>
            <dt>固定工具</dt>
            <dd>
              {evidence.profile.command_id} · {evidence.profile.tool_version}
            </dd>
            <dt>工具报告</dt>
            <dd>
              {(
                {
                  completed: "工具报告本轮结束",
                  failed: "工具报告本轮失败",
                  unknown: "工具结果未知",
                } as Record<string, string>
              )[evidence.terminal] || "工具未提供可识别终态"}
            </dd>
            <dt>已记录事件数</dt>
            <dd>{evidence.event_count}</dd>
          </>
        )}
        {session && (
          <>
            <dt>外部会话标识</dt>
            <dd>
              <code>{session}</code>
            </dd>
          </>
        )}
      </dl>
      {session && (
        <p className="muted">
          会话标识仅供追溯与核对，不会自动续跑，不代表平台具备远程会话恢复权限。
        </p>
      )}
      <UsageFacts
        usage={usage ?? undefined}
        usageState={usageState}
        step={!evidence}
      />
      {state === "running" && last?.kind === "waiting_input" && (
        <p className="warning">
          等待外部输入，进程仍占用执行槽位。当前协议不支持在此答复或自动恢复，请核对本机工具。
        </p>
      )}
      {events.length > 40 && (
        <p className="muted">
          只显示当前窗口最近 40 项，完整事实请查看持久历史。
        </p>
      )}
      <div className="tool-event-list">
        {recent.map((event, index) => (
          <article key={index}>
            <strong>{toolEventNames[event.kind] || "工具事实"}</strong>
            {event.status && <small>{event.status}</small>}
            {event.message && <p className="preserve">{event.message}</p>}
          </article>
        ))}
      </div>
      <p className="muted">
        工具消息和用量是外部报告，不替代平台实际停止、测试证据或业务验收。
      </p>
    </section>
  );
}
