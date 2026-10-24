import type { RecordItem, Workflow } from "./api";
export type ToolProfileDefinition = {
  command_id: string;
  adapter: string;
  protocol: string;
  profile_sha256: string;
  executable_sha256: string;
  tool_version: string;
  read_only: boolean;
};
export type ToolObservation = {
  profile: ToolProfileDefinition;
  available: boolean;
  reason: string;
};
export type ToolProfile = {
  id: string;
  project_id: string;
  worker_id: string;
  name: string;
  version: number;
  state: "paused" | "enabled" | "revoked";
  profile: ToolProfileDefinition;
  enabled_by: string;
  created_at: string;
  updated_at: string;
  observation?: ToolObservation;
  last_seen_at?: string;
  available: boolean;
  reason: string;
};
export type ToolUsage = {
  input_tokens?: number;
  cached_input_tokens?: number;
  cache_write_input_tokens?: number;
  output_tokens?: number;
  reasoning_output_tokens?: number;
};
export type UsageState = "complete" | "partial" | "unreported";
export type ToolEvent = {
  schema_version: 1;
  command_id: string;
  profile_sha256: string;
  protocol: string;
  kind: string;
  session_id?: string;
  item_id?: string;
  message?: string;
  status?: string;
  usage?: ToolUsage;
  usage_state?: UsageState;
};
export type ToolEvidence = {
  profile: ToolProfileDefinition;
  session_id?: string;
  terminal: "completed" | "failed" | "unknown";
  event_count: number;
  usage?: ToolUsage;
  usage_state?: UsageState;
};
export type TaskContextRecord = {
  id: string;
  version: number;
  title: string;
  description: string;
  acceptance_criteria: string[];
};
export type TaskDispatchPlan = {
  id: string;
  project_id: string;
  version: number;
  state: string;
  task_id: string;
  task_version: number;
  requirement_id?: string;
  requirement_version?: number;
  workflow_id: string;
  workflow_version: number;
  agent_node_id: string;
  tool_profile_id: string;
  tool_profile_version: number;
  tool_profile_sha256: string;
  run_id: string;
  created_at: string;
  expires_at: string;
  context: { task: TaskContextRecord; requirement?: TaskContextRecord };
  workflow: Workflow;
  valid: boolean;
  reason: string;
};
export function taskPlanStarted(
  plan: Pick<TaskDispatchPlan, "state" | "run_id">,
) {
  return plan.state === "started";
}
export type ToolPage<T> = { items: T[]; total: number; next_cursor?: string };
export type ExecutionWorker = {
  id: string;
  project_id: string;
  worker_id: string;
  version: number;
  status: string;
  deployment: boolean;
  created_at: string;
  expires_at: string;
};
export type ToolRunBinding = {
  profile_id: string;
  profile_sha256: string;
  command: string;
  run_id: string;
  node_id: string;
};
export function toolReturnPanel(run: string, node: string) {
  if (
    !/^[\x21-\x7e]{1,150}$/.test(run) ||
    (node && !/^[\x21-\x7e]{1,150}$/.test(node))
  )
    return "";
  return (
    "return-run:" + encodeURIComponent(run) + "/" + encodeURIComponent(node)
  );
}
export function toolReturnRun(
  panel: string,
): { run: string; node: string } | undefined {
  if (!panel.startsWith("return-run:") || panel.length > 1000) return undefined;
  try {
    const pieces = panel.slice(11).split("/");
    if (pieces.length !== 2) return undefined;
    const run = decodeURIComponent(pieces[0]),
      node = decodeURIComponent(pieces[1]);
    if (toolReturnPanel(run, node) !== panel) return undefined;
    return { run, node };
  } catch {
    return undefined;
  }
}
const encoder = new TextEncoder();
const id = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const digest = /^[a-f0-9]{64}$/;
const protocols: Record<string, string> = {
  command: "text-v1",
  suma: "suma-jsonl-v1",
  codex: "codex-jsonl-v1",
  opencode: "opencode-jsonl-v1",
};
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("清单必须使用正式 JSON 对象。");
  return value as Record<string, unknown>;
}
function fields(value: Record<string, unknown>, allowed: string[]) {
  if (Object.keys(value).some((key) => !allowed.includes(key)))
    throw new Error(
      "清单包含非公开字段或秘密信息，请使用 --describe-tools 的非秘密输出。",
    );
}
export function parseToolManifest(text: string): ToolObservation[] {
  if (encoder.encode(text).byteLength > 256 * 1024)
    throw new Error("工具清单超过 256 KiB，请缩小配置范围。");
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    throw new Error("工具清单不是有效 JSON。");
  }
  if (!Array.isArray(raw)) {
    const envelope = record(raw);
    fields(envelope, ["items", "schema_version", "execution_enabled"]);
    raw = envelope.items;
  }
  if (!Array.isArray(raw) || raw.length > 128)
    throw new Error("工具清单须包含最多 128 项真实观察。");
  const seen = new Set<string>();
  return raw.map((item) => {
    const observation = record(item);
    fields(observation, ["profile", "available", "reason"]);
    const profile = record(observation.profile);
    fields(profile, [
      "command_id",
      "adapter",
      "protocol",
      "profile_sha256",
      "executable_sha256",
      "tool_version",
      "read_only",
    ]);
    if (
      typeof profile.command_id !== "string" ||
      !id.test(profile.command_id) ||
      seen.has(profile.command_id)
    )
      throw new Error("清单的命令标识无效或重复。");
    seen.add(profile.command_id);
    if (
      typeof profile.adapter !== "string" ||
      protocols[profile.adapter] !== profile.protocol
    )
      throw new Error("适配器与协议不匹配，不能猜测厂商协议。");
    if (
      typeof observation.available !== "boolean" ||
      typeof observation.reason !== "string" ||
      encoder.encode(observation.reason).byteLength > 1000 ||
      (!observation.available && !observation.reason.trim())
    )
      throw new Error(
        "清单须声明可用状态，原因最多 1000 个 UTF-8 字节；不可用时须填写明确原因。",
      );
    for (const name of ["profile_sha256", "executable_sha256"])
      if (
        profile[name] !== undefined &&
        profile[name] !== "" &&
        (typeof profile[name] !== "string" ||
          !digest.test(profile[name] as string))
      )
        throw new Error("工具指纹须为小写 SHA-256。");
    if (
      profile.tool_version !== undefined &&
      (typeof profile.tool_version !== "string" ||
        encoder.encode(profile.tool_version).byteLength > 256)
    )
      throw new Error("工具版本格式无效。");
    if (
      profile.read_only !== undefined &&
      typeof profile.read_only !== "boolean"
    )
      throw new Error("工具只读声明格式无效。");
    if (
      observation.available &&
      (!digest.test(String(profile.profile_sha256 || "")) ||
        !digest.test(String(profile.executable_sha256 || "")) ||
        typeof profile.tool_version !== "string" ||
        !profile.tool_version ||
        typeof profile.read_only !== "boolean")
    )
      throw new Error("可用工具必须具有完整配置、程序指纹、版本与读写声明。");
    return observation as ToolObservation;
  });
}
export function toolBinding(
  node: Record<string, unknown>,
  profile?: ToolProfile,
): Record<string, unknown> {
  const config = {
    ...(node.config &&
    typeof node.config === "object" &&
    !Array.isArray(node.config)
      ? node.config
      : {}),
  } as Record<string, unknown>;
  delete config.tool_profile_id;
  delete config.tool_profile_sha256;
  if (!profile) return { config };
  if (!profile.id || !digest.test(profile.profile.profile_sha256))
    throw new Error("请选择具有固定配置指纹的正式工具。");
  return {
    command: [profile.profile.command_id],
    config: {
      ...config,
      tool_profile_id: profile.id,
      tool_profile_sha256: profile.profile.profile_sha256,
    },
  };
}
export function taskToolProtocol(profile: ToolProfileDefinition) {
  return (
    (profile.adapter === "suma" && profile.protocol === "suma-jsonl-v1") ||
    (profile.adapter === "codex" && profile.protocol === "codex-jsonl-v1")
  );
}
export function dispatchRequest(
  project: string,
  task: RecordItem,
  workflow: Workflow,
  node: string,
  profile: ToolProfile,
): Record<string, unknown> {
  if (
    !task.id ||
    !Number.isInteger(task.version) ||
    task.version < 1 ||
    (task.project_id && task.project_id !== project)
  )
    throw new Error("请读取当前项目的正式任务版本。");
  if (
    profile.project_id !== project ||
    profile.state !== "enabled" ||
    !profile.available
  )
    throw new Error("工具配置当前尚未确认可派发，请核对当前项目与实时观察。");
  if (profile.profile.read_only)
    throw new Error(
      "任务开发需要许可写入的隔离工具；只读配置请通过独立流程使用。",
    );
  if (!taskToolProtocol(profile.profile))
    throw new Error(
      "任务派发仅支持已核实任务输入与终态的 Suma／Codex 结构化协议；其他配置请通过独立命令流程使用。",
    );
  if (
    !workflow.id ||
    !Number.isInteger(workflow.version) ||
    workflow.version < 1 ||
    workflow.nodes.find((item) => item.id === node)?.type !== "agent"
  )
    throw new Error("请选择已发布流程内真实的智能体节点。");
  if (
    !workflow.nodes.some(
      (item) =>
        item.type === "test" &&
        item.config?.candidate_from === node &&
        item.command?.length === 1,
    )
  )
    throw new Error(
      "流程缺少取得所选开发节点固定候选的独立测试，请先完善工作流。",
    );
  return {
    project_id: project,
    task_id: task.id,
    expected_task_version: task.version,
    workflow_id: workflow.id,
    workflow_version: workflow.version,
    agent_node_id: node,
    tool_profile_id: profile.id,
    expected_tool_version: profile.version,
  };
}
const eventKinds = new Set([
  "session_started",
  "turn_started",
  "activity_started",
  "activity_updated",
  "activity_completed",
  "message",
  "usage",
  "error",
  "turn_completed",
  "turn_failed",
  "waiting_input",
]);
export function toolFactsForAttempt(
  events: unknown[],
  node: unknown,
): ToolEvent[] {
  const target = node as
    | { id?: string; attempt?: number; fence?: number; worker_id?: string }
    | undefined;
  if (!target?.id || !target.attempt || !target.fence || !target.worker_id)
    return [];
  return events.flatMap((raw) => {
    const event = raw as {
      type?: string;
      node_id?: string;
      data?: Record<string, unknown>;
    };
    if (
      event.type !== "tool_progress" ||
      event.node_id !== target.id ||
      event.data?.attempt !== target.attempt ||
      event.data?.fence !== target.fence ||
      event.data?.worker_id !== target.worker_id
    )
      return [];
    const fact = event.data!.tool_event as ToolEvent | undefined;
    if (
      !fact ||
      fact.schema_version !== 1 ||
      !eventKinds.has(fact.kind) ||
      !id.test(fact.command_id) ||
      !digest.test(fact.profile_sha256) ||
      !Object.values(protocols).includes(fact.protocol)
    )
      return [];
    return [fact];
  });
}
export const toolStateNames: Record<string, string> = {
  paused: "已暂停",
  enabled: "已启用",
  revoked: "已撤销",
};
export const toolEventNames: Record<string, string> = {
  session_started: "外部会话已记录",
  turn_started: "工具开始处理",
  activity_started: "活动开始",
  activity_updated: "活动进展",
  activity_completed: "活动结束",
  message: "工具消息",
  usage: "工具用量报告",
  error: "工具错误",
  turn_completed: "工具报告本轮结束",
  turn_failed: "工具报告本轮失败",
  waiting_input: "等待外部输入",
};
