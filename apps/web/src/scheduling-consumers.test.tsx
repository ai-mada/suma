import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  AdmissionFacts,
  AttemptFacts,
  PolicyFacts,
  PolicyPreviewFacts,
  RunAdmissionReview,
} from "./SchedulingFacts";
import type { ExecutionAttempt } from "./scheduling-model";
const snapshot = {
  run_id: "r",
  node_id: "n",
  run_version: 7,
  node_state: "ready",
  attempt: 2,
  fence: 5,
  policy_version: 3,
  evaluated_at: "2026-10-24T06:00:00Z",
  status: "blocked",
  blockers: [
    {
      code: "resource_capacity",
      message: "槽位不足",
      source: "policy",
      resource: "execution_slots",
      required: 1,
      capacity: 0,
      used: 0,
      available: 0,
    },
    {
      code: "budget_exhausted",
      message: "累计额度不足",
      source: "policy",
      capacity: 0,
      used: 0,
    },
  ],
  requests: [
    {
      name: "execution_slots",
      unit: "slots",
      amount: 1,
      release_class: "terminal",
    },
  ],
  reservations: [],
};
describe("scheduling facts remain separate from attempts and planning", () => {
  const attemptBase = {
    run_id: "r",
    node_id: "n",
    attempt: 2,
    fence: 4,
    worker_id: "worker",
    state: "succeeded",
    started_at: "2026-10-24T06:00:00Z",
    completed_at: "2026-10-24T06:01:00Z",
  };
  it("describes an actual manual reconciliation without fabricating a missing machine receipt", () => {
    const attempt = {
      ...attemptBase,
      result_source: "manual_reconciliation",
      reconciliation: {
        outcome: "succeeded",
        reason: "已核对远端固定操作及原始记录",
      },
      result: null,
    };
    const html = renderToStaticMarkup(createElement(AttemptFacts, { attempt }));
    for (const text of ["人工核对", "已核对远端固定操作及原始记录", "成功"])
      expect(html).toContain(text);
    expect(html).not.toContain("尚未保存终结回执");
    expect(html).not.toContain("进程退出码");
  });
  it("keeps child workflow terminal identity visible when there is no Runner result", () => {
    const html = renderToStaticMarkup(
      createElement(AttemptFacts, {
        attempt: {
          ...attemptBase,
          result_source: "child_workflow",
          child_run_id: "real-child-run",
          result: null,
        },
      }),
    );
    for (const text of ["子流程", "real-child-run", "已成功"])
      expect(html).toContain(text);
    expect(html).not.toContain("尚未保存终结回执");
    expect(html).not.toContain("进程退出码");
  });
  it("labels a managed observation result as provider status instead of a process exit", () => {
    const html = renderToStaticMarkup(
      createElement(AttemptFacts, {
        attempt: {
          ...attemptBase,
          result_source: "managed_observation",
          result: {
            run_id: "r",
            node_id: "n",
            attempt: 2,
            fence: 4,
            worker_id: "worker",
            exit_code: 0,
            output: "提供方实际观察",
            error: "",
            operation_id: "observation",
            stopped: false,
            outcome_unknown: false,
          },
        },
      }),
    );
    for (const text of ["提供方观察核对", "提供方观察状态码", "提供方实际观察"])
      expect(html).toContain(text);
    expect(html).not.toContain("进程退出码");
  });
  it("marks missing historical provenance as unreported rather than guessing a runner", () => {
    const attempt = {
      ...attemptBase,
      result_source: "unreported",
    } satisfies ExecutionAttempt;
    Reflect.deleteProperty(attempt, "result_source");
    const html = renderToStaticMarkup(createElement(AttemptFacts, { attempt }));
    expect(html).toContain("未报告来源");
    expect(html).not.toContain("进程退出码");
  });
  it.each([
    ["not_loaded", "打开精确详情"],
    ["unreported", "实际结果仍待核对"],
  ])(
    "keeps %s distinct from an inferred process result",
    (source, explanation) => {
      const html = renderToStaticMarkup(
        createElement(AttemptFacts, {
          attempt: { ...attemptBase, result_source: source, result: null },
        }),
      );
      expect(html).toContain(explanation);
      expect(html).not.toContain("进程退出码");
    },
  );
  it("reviews pause/resume snapshot scope without claiming stop or renewed eligibility", () => {
    const html = renderToStaticMarkup(
      createElement(RunAdmissionReview, {
        action: "resume",
        run: {
          id: "r",
          version: 7,
          workflow_version: 2,
          state: "paused",
          nodes: [{ state: "ready" }, { state: "running" }],
        },
      }),
    );
    for (const value of [
      "运行 v7",
      "流程 v2",
      "就绪节点 1",
      "进行中节点 1",
      "恢复只重新开放派发",
      "不会跳过当前资源、工具和权限核验",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("已经停止");
  });
  it("shows all simultaneous current blockers and explicit zeros without fake progress", () => {
    const html = renderToStaticMarkup(
      createElement(AdmissionFacts, { snapshot }),
    );
    for (const value of [
      "当前派发条件",
      "槽位不足",
      "执行槽位不足",
      "累计额度不足",
      "累计执行额度不足",
      "并发槽位",
      "诊断依据",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("%");
    expect(html).not.toContain("硬件充足");
  });
  it("retains unknown result and actual failed attempt independently of current ready admission", () => {
    const html = renderToStaticMarkup(
      createElement(AttemptFacts, {
        attempt: {
          run_id: "r",
          node_id: "n",
          attempt: 1,
          fence: 3,
          worker_id: "worker",
          state: "failed",
          result_source: "runner_result",
          started_at: "2026-10-24T00:00:00Z",
          completed_at: "2026-10-24T00:01:00Z",
          result: {
            run_id: "r",
            node_id: "n",
            attempt: 1,
            fence: 3,
            worker_id: "worker",
            operation_id: "op",
            error: "",
            exit_code: 0,
            stopped: true,
            outcome_unknown: true,
            output: "原操作需核对",
          },
        },
      }),
    );
    for (const value of [
      "历史尝试 1",
      "worker",
      "结果未知",
      "原操作需核对",
      "实际停止",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("重试");
  });
  it("describes configured admission and project-only rotation without inventing billing", () => {
    const html = renderToStaticMarkup(
      createElement(PolicyFacts, {
        policy: {
          project_id: "p",
          version: 0,
          default: true,
          concurrency_limit: 0,
          resource_limits: { gpu: null },
          budget_limit: null,
          budget_spent: 0,
          unit: "admission_units",
          budget_history: "complete",
          as_of: "2026-10-24T06:00:00Z",
          resources: [
            {
              name: "execution_slots",
              unit: "slots",
              capacity: 0,
              used: 0,
              consumed: 0,
              mode: "limited",
              release_class: "process",
            },
            {
              name: "budget",
              unit: "admission_units",
              capacity: null,
              used: 0,
              consumed: 0,
              mode: "unlimited",
              release_class: "cumulative",
            },
            {
              name: "custom-license",
              unit: "units",
              capacity: null,
              used: 0,
              consumed: 0,
              mode: "unlimited",
              release_class: "effect",
            },
          ],
          allowed_actions: [],
          fairness: { mode: "project_run_round_robin", scope: "project" },
        },
      }),
    );
    for (const value of [
      "当前执行额度",
      "当前项目内运行轮转",
      "不限制",
      "准入单位",
      "custom-license",
      "0",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("￥");
    expect(html).not.toContain("权重设置");
    expect(html).not.toContain("admission_units");
    expect(html).not.toContain("execution_slots");
  });
  it("reviews exact change and existing effects before explicit commit", () => {
    const html = renderToStaticMarkup(
      createElement(PolicyPreviewFacts, {
        preview: {
          id: "preview",
          project_id: "p",
          version: 1,
          state: "ready",
          policy_version: 0,
          policy: {
            concurrency_limit: 0,
            resource_limits: {},
            budget_limit: 0,
          },
          previous_policy: {
            concurrency_limit: 1,
            resource_limits: {},
            budget_limit: null,
          },
          changes: [
            { resource: "execution_slots", unit: "slots", before: 1, after: 0 },
          ],
          impact: {
            observed_at: "2026-10-24T06:00:00Z",
            ready_total: 2,
            reserved_execution_slots: 1,
            budget_spent: 3,
            budget_history: "complete",
            resources: [],
            notes: [],
          },
          policy_sha256: "a".repeat(64),
          created_at: "",
          expires_at: "",
          valid: true,
        },
      }),
    );
    for (const value of [
      "策略变更审阅",
      "并发槽位",
      "准入单位",
      "确认时再次核验版本和当前权限",
      "2",
      "3",
    ])
      expect(html).toContain(value);
  });
});
