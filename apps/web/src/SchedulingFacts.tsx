import { tr, useLocale, renderDiagnostic } from "./i18n";
import { stateLabel, time } from "./api";
import { ToolExecutionFacts } from "./ToolFacts";
import {
  admissionNames,
  capacityLabel,
  executionHref,
  type SchedulingPolicy,
  type PolicyPreview,
  type NodeAdmission,
  type ExecutionAttempt,
  type SchedulingBlocker,
} from "./scheduling-model";
export function RunAdmissionReview({
  action,
  run,
}: {
  action: string;
  run: {
    id: string;
    version: number;
    workflow_version: number;
    state: string;
    nodes: { state: string }[];
  };
}) {
  return (
    <section className="scheduling-facts" aria-label="运行命令范围审阅">
      <h4>运行命令范围审阅</h4>
      <code>{run.id}</code>
      <p>
        运行 v{run.version} · 流程 v{run.workflow_version} ·{" "}
        {stateLabel(run.state)}
      </p>
      <p>
        当前完整快照：就绪节点{" "}
        {run.nodes.filter((node) => node.state === "ready").length} · 进行中节点{" "}
        {run.nodes.filter((node) => node.state === "running").length}
      </p>
      <p>
        {action === "pause"
          ? "暂停只阻止新的派发，已有执行继续排空。"
          : action === "resume"
            ? "恢复只重新开放派发，不会跳过当前资源、工具和权限核验。"
            : "取消先请求停止；只有实际停止回执或核对证据才能解除未知结果。"}
      </p>
      <small>
        这些是当前运行快照，不是资源预约或执行许可。确认使用此版本；服务端拒绝冲突后需要重新审阅。
      </small>
    </section>
  );
}
const resourceNames: Record<string, string> = {
  get execution_slots() {
    return tr("common.resource.executionSlots");
  },
  get budget() {
    return tr("common.resource.budget");
  },
  get cpu_millis() {
    return tr("common.resource.cpuMillis");
  },
  get memory_bytes() {
    return tr("common.resource.memoryBytes");
  },
  get gpu_slots() {
    return tr("common.resource.gpuSlots");
  },
  get model_concurrency() {
    return tr("common.resource.modelConcurrency");
  },
};
const unitNames: Record<string, string> = {
  get admission_units() {
    return tr("common.unit.admission");
  },
  get slots() {
    return tr("common.unit.slots");
  },
  get millicores() {
    return tr("common.unit.millicores");
  },
  get bytes() {
    return tr("common.unit.bytes");
  },
  get requests() {
    return tr("common.unit.requests");
  },
  get units() {
    return tr("common.unit.units");
  },
};
export const schedulingResourceLabel = (name: string) =>
  resourceNames[name] || name;
export const schedulingUnitLabel = (unit?: string) =>
  unit ? unitNames[unit] || unit : tr("common.unit.unknown");
function blockerTitle(blocker: SchedulingBlocker) {
  if (blocker.source === "pool") {
    const names: Record<string, string> = {
      pool_scope_inactive: "共享池项目授权未启用",
      pool_scope_version_mismatch: "共享池授权版本已变化",
      pool_budget_exhausted: "共享池累计执行额度不足",
      pool_paused: "共享池已暂停新派发",
      pool_metadata_missing: "共享池资料待核对",
      pool_grant_not_current: "共享池当前项目授权待核对",
    };
    if (blocker.code === "pool_resource_capacity")
      return (
        "共享池" +
        (blocker.resource === "execution_slots"
          ? "执行槽位不足"
          : (blocker.resource
              ? schedulingResourceLabel(blocker.resource)
              : "执行资源") + "不足")
      );
    return (
      names[blocker.code] || "共享池：" + (blocker.message || blocker.code)
    );
  }
  const names: Record<string, string> = {
    run_paused: "运行已暂停",
    run_cancelling: "正在取消，等待实际停止或结果核对",
    outcome_unknown: "执行结果待核对，核对前不能重新执行",
    dependencies_pending: "等待依赖或执行条件",
    approval_required: "等待人工审批",
    timer_waiting: "等待定时唤醒",
    expansion_waiting: "等待动态任务完成",
    child_waiting: "等待子流程完成",
    node_waiting: "节点等待中",
    budget_exhausted: "项目累计执行额度不足",
    worker_capability_unknown: "执行节点匹配待确认",
  };
  if (blocker.code === "resource_capacity") {
    if (blocker.resource === "execution_slots") return "项目执行槽位不足";
    const name = blocker.resource
      ? schedulingResourceLabel(blocker.resource)
      : "执行资源";
    return name + (/额度$|槽位$/.test(name) ? "不足" : "额度不足");
  }
  return names[blocker.code] || blocker.message || blocker.code;
}
const standardPolicyNotes = new Set([
  "容量为项目执行准入额度，并非实测硬件或操作系统硬限制",
  "降低容量只阻止新派发，已运行尝试继续；累计准入单位不退款",
  "影响数据为预览时快照；提交时重新核验权限与策略版本",
]);
export function PolicyFacts({ policy }: { policy: SchedulingPolicy }) {
  return (
    <section
      className="scheduling-facts scheduling-policy-summary"
      aria-label="当前配置准入额度"
    >
      <div className="scheduling-summary-heading">
        <h3>当前执行额度</h3>
        <span>
          {policy.default ? "默认策略 · " : ""}v{policy.version}
        </span>
        <small>读取于 {time(policy.as_of)}</small>
      </div>
      <dl className="scheduling-summary-values">
        <div>
          <dt>并发槽位</dt>
          <dd>
            {policy.concurrency_limit}
            <small>个</small>
          </dd>
        </div>
        <div>
          <dt>累计执行额度</dt>
          <dd>
            {capacityLabel(policy.budget_limit)}
            <small>{schedulingUnitLabel(policy.unit)}</small>
          </dd>
        </div>
        <div>
          <dt>累计已用</dt>
          <dd>
            {policy.budget_spent}
            <small>{schedulingUnitLabel(policy.unit)}</small>
          </dd>
        </div>
      </dl>
      {policy.budget_history !== "complete" && (
        <p className="warning">历史记录不完整，已用数仅包含已记录的执行。</p>
      )}
      <details className="scheduling-resource-details">
        <summary>资源明细与调度方式</summary>
        <p className="muted">
          {policy.fairness.mode === "project_run_round_robin" &&
          policy.fairness.scope === "project"
            ? "当前项目内运行轮转"
            : policy.fairness.mode + " / " + policy.fairness.scope}
        </p>
        {policy.resources.length > 0 ? (
          <table>
            <thead>
              <tr>
                <th>资源</th>
                <th>上限</th>
                <th>已用</th>
                <th>占用方式</th>
              </tr>
            </thead>
            <tbody>
              {policy.resources.map((resource) => (
                <tr key={resource.name}>
                  <td>{schedulingResourceLabel(resource.name)}</td>
                  <td>
                    {capacityLabel(resource.capacity)}
                    <small>{schedulingUnitLabel(resource.unit)}</small>
                  </td>
                  <td>
                    {resource.release_class === "cumulative"
                      ? resource.consumed
                      : resource.used}
                    <small>{schedulingUnitLabel(resource.unit)}</small>
                  </td>
                  <td>
                    {resource.release_class === "cumulative"
                      ? "累计使用"
                      : resource.release_class === "process"
                        ? "运行时占用"
                        : "结果确认前保留"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p>尚无额外资源明细。</p>
        )}
      </details>
    </section>
  );
}
export function PolicyPreviewFacts({ preview }: { preview: PolicyPreview }) {
  const notes = preview.impact.notes.filter(
    (note) => !standardPolicyNotes.has(note),
  );
  return (
    <section
      className="scheduling-facts scheduling-preview-facts"
      aria-label="策略变更审阅"
    >
      <h3>策略变更审阅</h3>
      <div className="scheduling-preview-meta">
        <strong>
          {preview.state === "committed"
            ? "已提交"
            : preview.valid
              ? "可审阅后确认"
              : "当前预览不可提交"}
        </strong>
        <span>依据策略 v{preview.policy_version}</span>
        <span>到期 {time(preview.expires_at)}</span>
      </div>
      {preview.reason && <p className="warning">{preview.reason}</p>}
      <ul className="scheduling-change-list">
        {preview.changes.map((change) => (
          <li key={change.resource}>
            <strong>{schedulingResourceLabel(change.resource)}</strong>
            <span>
              {capacityLabel(change.before)} → {capacityLabel(change.after)}{" "}
              {schedulingUnitLabel(change.unit)}
            </span>
          </li>
        ))}
      </ul>
      {!preview.changes.length && <p>本预览没有额度差异。</p>}
      <h4>预览时的执行情况</h4>
      <p>
        就绪节点 {preview.impact.ready_total} · 已占槽位{" "}
        {preview.impact.reserved_execution_slots} · 累计已用{" "}
        {preview.impact.budget_spent} 准入单位
      </p>
      <p className="muted">
        观察于 {time(preview.impact.observed_at)}
        ；确认时再次核验版本和当前权限。
        {preview.impact.budget_history !== "complete" ? "历史记录不完整。" : ""}
      </p>
      {notes.map((note, index) => (
        <p key={index}>{note}</p>
      ))}
      <details className="scheduling-preview-reference">
        <summary>预览依据与指纹</summary>
        <dl>
          <dt>固定预览</dt>
          <dd>
            <code>{preview.id}</code> · v{preview.version}
          </dd>
          <dt>固定摘要</dt>
          <dd>
            <code>{preview.policy_sha256}</code>
          </dd>
        </dl>
      </details>
    </section>
  );
}
export function AdmissionFacts({
  snapshot,
  project,
}: {
  snapshot: NodeAdmission;
  project?: string;
}) {
  return (
    <section className="scheduling-facts" aria-label="当前派发条件">
      <h4>当前派发条件</h4>
      <p>
        <strong>{admissionNames[snapshot.status] || snapshot.status}</strong> ·{" "}
        {stateLabel(snapshot.node_state)}
      </p>
      <small>
        判断于 {time(snapshot.evaluated_at)} · 运行 v{snapshot.run_version} ·
        策略 v{snapshot.policy_version}
      </small>
      {snapshot.execution_pool_id && (
        <p className="scheduling-pool-scope">
          共享池 <code>{snapshot.execution_pool_id}</code> · 共享池策略v
          {snapshot.pool_policy_version ?? "未提供"}
        </p>
      )}
      <ul className="scheduling-blockers">
        {snapshot.blockers.map((blocker, index) => (
          <li key={index}>
            <strong>{blockerTitle(blocker)}</strong>
            {blocker.resource && (
              <span>{schedulingResourceLabel(blocker.resource)}</span>
            )}
            <div className="scheduling-numbers">
              {(["required", "capacity", "used", "available"] as const).map(
                (key) =>
                  blocker[key] !== undefined && (
                    <span key={key}>
                      {
                        {
                          required: "请求",
                          capacity: "上限",
                          used: "已占",
                          available: "可用",
                        }[key]
                      }{" "}
                      {blocker[key]} {schedulingUnitLabel(blocker.unit)}
                    </span>
                  ),
              )}
            </div>
            {blocker.retry_at && (
              <small>下次检查不早于 {time(blocker.retry_at)}</small>
            )}
            <details className="scheduling-blocker-basis">
              <summary>诊断依据</summary>
              <small>
                {blocker.code} · {blocker.source}
              </small>
              {blocker.message && <p className="preserve">{blocker.message}</p>}
            </details>
            {project && blocker.run_id && blocker.node_id && (
              <a href={executionHref(project, blocker.run_id, blocker.node_id)}>
                查看相关运行节点
              </a>
            )}
          </li>
        ))}
      </ul>
      {!snapshot.blockers.length && (
        <p>
          {snapshot.status === "eligible"
            ? "本次读取满足已检查的准入条件；实际领取时仍需重新核验。"
            : "本次没有额外阻塞明细，不据此推断执行器可用。"}
        </p>
      )}
      {snapshot.execution_pool_id && (
        <details>
          <summary>本节点请求范围内的共享池资源</summary>
          <p>
            仅显示本节点所请求资源的汇总；这是配置准入账本，不是机器硬件测量。
          </p>
          {snapshot.pool_resources?.length ? (
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>资源</th>
                    <th>配置上限</th>
                    <th>当前预留</th>
                    <th>累计消费</th>
                  </tr>
                </thead>
                <tbody>
                  {snapshot.pool_resources.map((item) => (
                    <tr key={item.name}>
                      <td>
                        {schedulingResourceLabel(item.name)} ·{" "}
                        {schedulingUnitLabel(item.unit)}
                      </td>
                      <td>{capacityLabel(item.capacity)}</td>
                      <td>{item.used}</td>
                      <td>{item.consumed}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p>本次未提供请求范围内的共享池资源明细。</p>
          )}
        </details>
      )}
      <details>
        <summary>固定资源请求与当前预留</summary>
        {snapshot.requests.map((resource) => (
          <p key={resource.name}>
            {schedulingResourceLabel(resource.name)} 请求 {resource.amount}{" "}
            {schedulingUnitLabel(resource.unit)} ·{" "}
            {resource.release_class === "cumulative"
              ? "每次尝试累计消费"
              : "并发预留"}
          </p>
        ))}
        {snapshot.reservations.map((resource) => (
          <p key={resource.name}>
            已预留 {schedulingResourceLabel(resource.name)} {resource.amount}{" "}
            {schedulingUnitLabel(resource.unit)} · 尝试 {resource.attempt} /
            fence {resource.fence}
          </p>
        ))}
        {!snapshot.reservations.length && <p>本次读取没有并发资源预留。</p>}
      </details>
    </section>
  );
}
export function AttemptFacts({ attempt }: { attempt: ExecutionAttempt }) {
  const result = attempt.result,
    source = attempt.result_source;
  const sourceNames: Record<string, string> = {
    not_loaded: "仅元数据，未加载结果",
    unreported: "未报告终结结果",
    runner_result: "Runner 结果回执",
    manual_reconciliation: "人工核对记录",
    child_workflow: "子流程持久结果",
    managed_observation: "提供方观察核对",
  };
  return (
    <section
      className="scheduling-facts"
      aria-label={"历史尝试 " + attempt.attempt}
    >
      <h4>历史尝试 {attempt.attempt}</h4>
      <dl>
        <dt>结果来源</dt>
        <dd>{sourceNames[source] || "未报告来源"}</dd>
        <dt>持久执行状态</dt>
        <dd>{stateLabel(attempt.state)}</dd>
        <dt>执行身份</dt>
        <dd>
          <code>{attempt.worker_id || "尚未提供"}</code>
        </dd>
        <dt>Fence</dt>
        <dd>{attempt.fence}</dd>
        <dt>开始／结束</dt>
        <dd>
          {time(attempt.started_at)} / {time(attempt.completed_at)}
        </dd>
      </dl>
      {source === "manual_reconciliation" && (
        <section aria-label="人工核对结论">
          <h5>人工核对结论</h5>
          <p>
            {attempt.reconciliation?.outcome
              ? (
                  {
                    not_started: "人工确认未开始执行",
                    succeeded: "人工核对为成功",
                    failed: "人工核对为失败",
                  } as Record<string, string>
                )[attempt.reconciliation.outcome] ||
                attempt.reconciliation.outcome
              : "核对结论尚未提供"}
          </p>
          <p className="preserve">
            {attempt.reconciliation?.reason || "核对依据尚未提供"}
          </p>
          <small>这是已保存的人工核对事实，不补造机器执行回执。</small>
        </section>
      )}
      {source === "child_workflow" && (
        <section aria-label="固定子流程结果">
          <h5>固定子流程结果</h5>
          <code>{attempt.child_run_id || "子运行标识尚未提供"}</code>
          <p>持久状态：{stateLabel(attempt.state)}</p>
        </section>
      )}
      {result && source !== "not_loaded" && source !== "unreported" ? (
        <>
          <p>
            {result.outcome_unknown
              ? "结果未知，不能据此重派"
              : result.stopped && source === "runner_result"
                ? "已记录实际停止"
                : source === "managed_observation"
                  ? "提供方观察已经保存；不推断本机进程退出或外部效果已消除"
                  : "结果已保存；不推断外部副作用已消除"}
          </p>
          {result.stopped && source === "runner_result" && (
            <p>实际停止已回写，外部结果另行核对。</p>
          )}
          {(source === "runner_result" || source === "managed_observation") && (
            <p>
              {source === "runner_result" ? "进程退出码" : "提供方观察状态码"}{" "}
              {result.exit_code}
              {result.failure_class
                ? " · 失败分类 " + result.failure_class
                : ""}
            </p>
          )}
          {(result.output || result.error) && (
            <pre className="output">
              {result.output}
              {result.error && "\n" + result.error}
            </pre>
          )}
          <ToolExecutionFacts
            state={attempt.state}
            evidence={result.tool_evidence ?? undefined}
          />
          {result.workspace_evidence && (
            <details>
              <summary>本次尝试工作区证据</summary>
              <pre className="output">
                {JSON.stringify(result.workspace_evidence, null, 2)}
              </pre>
            </details>
          )}
        </>
      ) : source === "not_loaded" ? (
        <p className="muted">当前仅有尝试元数据，请打开精确详情读取结果。</p>
      ) : source === "manual_reconciliation" ||
        source === "child_workflow" ? null : (
        <p className="muted">
          终结结果尚未报告，不代表未执行或已停止；实际结果仍待核对。
        </p>
      )}
    </section>
  );
}
