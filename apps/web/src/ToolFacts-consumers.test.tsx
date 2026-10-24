import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  TaskDispatchFacts,
  TaskDispatchRunFacts,
  ToolExecutionFacts,
  ToolProfileFacts,
} from "./ToolFacts";
import type { TaskDispatchPlan, ToolProfile } from "./tool-model";
const profile: ToolProfile = {
  id: "profile",
  project_id: "p",
  worker_id: "runner",
  name: "编码工具",
  version: 3,
  state: "paused",
  profile: {
    command_id: "code",
    adapter: "codex",
    protocol: "codex-jsonl-v1",
    profile_sha256: "a".repeat(64),
    executable_sha256: "b".repeat(64),
    tool_version: "vendor-version",
    read_only: false,
  },
  enabled_by: "u",
  created_at: "",
  updated_at: "",
  available: false,
  reason: "inventory_stale",
  last_seen_at: "2026-10-24T00:00:00Z",
};
describe("tool execution facts are separate from acceptance", () => {
  it("never links a ready plan reserved run ID as if a real run already existed", () => {
    const pending = renderToStaticMarkup(
      createElement(TaskDispatchRunFacts, {
        plan: { state: "ready", run_id: "reserved-run-id" },
        onRun: () => {},
      }),
    );
    expect(pending).toContain("预留");
    expect(pending).toContain("尚未创建正式运行");
    expect(pending).not.toContain("查看本计划真实运行");
    expect(pending).not.toContain("已关联正式运行");
    const started = renderToStaticMarkup(
      createElement(TaskDispatchRunFacts, {
        plan: { state: "started", run_id: "actual-run-id" },
        onRun: () => {},
      }),
    );
    expect(started).toContain("查看本计划真实运行");
    const unknown = renderToStaticMarkup(
      createElement(TaskDispatchRunFacts, {
        plan: { state: "started", run_id: "" },
        onRun: () => {},
      }),
    );
    expect(unknown).not.toContain("查看本计划真实运行");
    expect(unknown).toContain("运行标识未读取");
  });
  it("shows enabled/configured/observed/dispatch state separately with stale inventory explanation", () => {
    const html = renderToStaticMarkup(
      createElement(ToolProfileFacts, { profile }),
    );
    for (const value of [
      "编码工具",
      "runner",
      "vendor-version",
      "已暂停",
      "最近上报",
      "inventory_stale",
      "未确认可派发",
      "不代表模型登录或任务完成",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("接入成功");
    expect(html).not.toContain("测试通过");
  });
  it("shows complete fixed task and requirement context with expiry and explicit acceptance boundary", () => {
    const plan: TaskDispatchPlan = {
      id: "plan",
      project_id: "p",
      version: 1,
      state: "ready",
      task_id: "task",
      task_version: 7,
      requirement_id: "req",
      requirement_version: 2,
      workflow_id: "flow",
      workflow_version: 4,
      agent_node_id: "code",
      tool_profile_id: "profile",
      tool_profile_version: 3,
      tool_profile_sha256: "a".repeat(64),
      run_id: "reserved-plan-run-id",
      created_at: "",
      expires_at: "2026-10-24T00:30:00Z",
      valid: true,
      reason: "",
      context: {
        task: {
          id: "task",
          version: 7,
          title: "固定任务",
          description: "完整任务说明",
          acceptance_criteria: ["必须复测"],
        },
        requirement: {
          id: "req",
          version: 2,
          title: "固定需求",
          description: "完整需求说明",
          acceptance_criteria: ["需求验收"],
        },
      },
      workflow: {
        id: "flow",
        name: "固定流程",
        version: 4,
        nodes: [
          { id: "code", name: "开发", type: "agent" },
          { id: "test", name: "独立测试", type: "test" },
        ],
        edges: [],
      },
    };
    const html = renderToStaticMarkup(
      createElement(TaskDispatchFacts, { plan }),
    );
    for (const value of [
      "固定任务",
      "完整任务说明",
      "必须复测",
      "固定需求",
      "完整需求说明",
      "需求验收",
      "独立测试",
      "人工验收",
      "2026",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("已验收");
    expect(html).not.toContain("测试通过");
  });
  it("keeps provider terminal success distinct from failed DAG state and does not invent resume actions", () => {
    const html = renderToStaticMarkup(
      createElement(ToolExecutionFacts, {
        state: "failed",
        evidence: {
          profile: profile.profile,
          session_id: "session-1",
          terminal: "completed",
          event_count: 4,
          usage: { input_tokens: 0, output_tokens: 5 },
        },
      }),
    );
    for (const text of [
      "平台执行状态",
      "失败",
      "工具报告",
      "session-1",
      "输入 Token",
      "0",
      "输出 Token",
      "5",
      "不会自动续跑",
    ])
      expect(html).toContain(text);
    expect(html).not.toContain("<button");
    expect(html).not.toContain("已验收");
  });
  it("displays waiting-input as an occupied-process fact without fake reply or recovered state", () => {
    const html = renderToStaticMarkup(
      createElement(ToolExecutionFacts, {
        state: "running",
        events: [
          {
            schema_version: 1,
            command_id: "code",
            profile_sha256: "a".repeat(64),
            protocol: "codex-jsonl-v1",
            kind: "waiting_input",
            message: "需要本机操作",
          },
        ],
      }),
    );
    expect(html).toContain("等待外部输入");
    expect(html).toContain("仍占用执行槽位");
    expect(html).toContain("不支持在此答复");
    expect(html).not.toContain("<button");
  });
  it("never describes a finished run as still occupying a slot because an older event waited for input", () => {
    const html = renderToStaticMarkup(
      createElement(ToolExecutionFacts, {
        state: "succeeded",
        evidence: {
          profile: profile.profile,
          terminal: "completed",
          event_count: 2,
        },
        events: [
          {
            schema_version: 1,
            command_id: "code",
            profile_sha256: "a".repeat(64),
            protocol: "codex-jsonl-v1",
            kind: "waiting_input",
          },
        ],
      }),
    );
    expect(html).not.toContain("仍占用执行槽位");
    expect(html).toContain("工具报告本轮结束");
  });
});
