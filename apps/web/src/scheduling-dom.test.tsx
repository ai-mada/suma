// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SchedulingWorkspace } from "./SchedulingWorkspace";
import { SchedulingQueue } from "./SchedulingQueue";
import { NodeSchedulingFacts } from "./NodeSchedulingFacts";
import { App } from "./App";
import { setLocale } from "./i18n";
import { parseWorkbenchRoute } from "./navigation";
import { switchedRoute } from "./governance-model";
import type {
  SchedulingPolicy,
  PolicyPreview,
  ExecutionAttempt,
} from "./scheduling-model";
vi.mock("./Workflow", () => ({
  WorkflowView: ({
    runId,
    nodeId,
    attemptId,
  }: {
    runId: string;
    nodeId: string;
    attemptId?: string;
  }) => (
    <div data-testid="route-consumer">
      运行 {runId} 节点 {nodeId} 尝试 {attemptId || "当前"}
    </div>
  ),
}));
const session = {
  user: { id: "u", name: "测试账号", email: "a@example.test", role: "owner" },
  organization: { id: "o", name: "组织" },
  projects: [
    { id: "p", version: 1, title: "项目" },
    { id: "other", version: 1, title: "其他项目" },
  ],
};
const policy: SchedulingPolicy = {
  project_id: "p",
  version: 0,
  default: true,
  concurrency_limit: 1,
  resource_limits: { gpu: null },
  budget_limit: null,
  budget_spent: 0,
  unit: "admission_units",
  budget_history: "complete",
  as_of: "2026-10-24T06:00:00Z",
  resources: [],
  allowed_actions: ["preview", "commit"],
  fairness: { mode: "project_run_round_robin", scope: "project" },
};
const preview: PolicyPreview = {
  id: "fixed-preview",
  project_id: "p",
  version: 1,
  state: "ready",
  policy_version: 0,
  policy: {
    concurrency_limit: 0,
    resource_limits: { gpu: null },
    budget_limit: 0,
  },
  previous_policy: {
    concurrency_limit: 1,
    resource_limits: { gpu: null },
    budget_limit: null,
  },
  changes: [
    { resource: "execution_slots", unit: "slots", before: 1, after: 0 },
  ],
  impact: {
    observed_at: "2026-10-24T06:00:00Z",
    ready_total: 1,
    reserved_execution_slots: 0,
    budget_spent: 0,
    budget_history: "complete",
    resources: [],
    notes: [],
  },
  policy_sha256: "a".repeat(64),
  created_at: "2026-10-24T06:00:00Z",
  expires_at: "2099-10-24T06:15:00Z",
  valid: true,
};
const entry = {
  run_id: "run-exact",
  node_id: "map[2]",
  node_name: "隔离开发",
  workflow_name: "真实固定流程",
  run_version: 7,
  node_state: "ready",
  attempt: 0,
  fence: 0,
  ready_at: "2026-10-24T06:00:00Z",
  sequence: 1,
  type: "command",
  requests: [],
};
let root: Root | undefined, container: HTMLDivElement;
beforeEach(async () => {
  await setLocale("zh-CN", { persist: false });
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  sessionStorage.clear();
  localStorage.clear();
  history.replaceState(null, "", "/?project=p&view=settings&sub=scheduling");
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  container.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function render(element: ReactNode) {
  await act(async () => {
    if (!root) root = createRoot(container);
    root.render(element);
  });
}
async function flush(check: () => boolean) {
  for (let i = 0; i < 40; i++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    if (check()) return;
  }
  expect(check()).toBe(true);
}
async function click(text: string) {
  const button = [
    ...container.querySelectorAll<HTMLButtonElement>("button"),
  ].find((item) => item.textContent?.trim() === text);
  expect(button, "button " + text).toBeTruthy();
  await act(async () => button!.click());
}
async function input(name: string, value: string) {
  const element = container.querySelector<HTMLInputElement>(
    `input[name="${name}"]`,
  )!;
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(element, value);
    element.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
function response(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function path(input: unknown) {
  return new URL(String(input), location.href).pathname.replace("/api/v1", "");
}
describe("actual scheduling consumers with synthetic HTTP fixtures", () => {
  it("keeps the keyboard confirmation button mounted and focused as the single Continue action without another commit", async () => {
    let committed = false,
      release: ((value: Response) => void) | undefined,
      posts = 0;
    const selected = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const route = path(input);
        if (route === "/scheduling/policy")
          return Promise.resolve(
            response(committed ? { ...policy, version: 1 } : policy),
          );
        if (route.endsWith("/commit")) {
          posts++;
          return new Promise<Response>((resolve) => {
            release = resolve;
          });
        }
        return Promise.resolve(
          response(
            committed
              ? {
                  ...preview,
                  state: "committed",
                  version: 2,
                  valid: false,
                  applied_policy_version: 1,
                }
              : preview,
          ),
        );
      }),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(() => container.textContent!.includes("策略变更审阅"));
    const confirm = [
      ...container.querySelectorAll<HTMLButtonElement>("button"),
    ].find((button) => button.textContent === "确认应用此固定策略")!;
    confirm.focus();
    await act(async () => {
      confirm.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Enter",
          bubbles: true,
          cancelable: true,
        }),
      );
      /* JSDOM has no native Enter activation. Exercise the browser button activation through React, without faking focus or blur. */ confirm.click();
    });
    expect(posts).toBe(1);
    expect(confirm.disabled).toBe(false);
    expect(confirm.getAttribute("aria-disabled")).toBe("true");
    expect(document.activeElement).toBe(confirm);
    await act(async () => confirm.click());
    expect(posts).toBe(1);
    committed = true;
    await act(async () => release!(response({ ...policy, version: 1 })));
    await flush(() => confirm.textContent === "继续调整策略");
    expect(
      [...container.querySelectorAll("button")].filter(
        (button) => button.textContent === "继续调整策略",
      ),
    ).toHaveLength(1);
    expect(confirm.disabled).toBe(false);
    expect(document.activeElement).toBe(confirm);
    expect(container.contains(confirm)).toBe(true);
    await act(async () => confirm.click());
    expect(selected).toHaveBeenLastCalledWith("");
    expect(posts).toBe(1);
  });
  it("does not focus Continue when the user has moved to another enabled control during submission", async () => {
    let committed = false,
      release: ((value: Response) => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const route = path(input);
        if (route === "/scheduling/policy")
          return Promise.resolve(
            response(committed ? { ...policy, version: 1 } : policy),
          );
        if (route.endsWith("/commit"))
          return new Promise<Response>((resolve) => {
            release = resolve;
          });
        return Promise.resolve(
          response(
            committed
              ? {
                  ...preview,
                  state: "committed",
                  version: 2,
                  valid: false,
                  applied_policy_version: 1,
                }
              : preview,
          ),
        );
      }),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={() => {}}
        refresh={0}
      />,
    );
    await flush(() => container.textContent!.includes("策略变更审阅"));
    await click("确认应用此固定策略");
    const other = [
      ...container.querySelectorAll<HTMLButtonElement>("button"),
    ].find((button) => button.textContent?.trim() === "读取当前策略")!;
    other.focus();
    committed = true;
    await act(async () => release!(response({ ...policy, version: 1 })));
    await flush(() => container.textContent!.includes("已经提交"));
    expect(document.activeElement).toBe(other);
  });
  it("opens a committed preview as Continue only and cannot POST it again", async () => {
    const selected = vi.fn(),
      fetch = vi.fn(async (input: unknown, options?: RequestInit) => {
        expect(options?.method).toBe("GET");
        return response(
          path(input) === "/scheduling/policy"
            ? { ...policy, version: 1 }
            : {
                ...preview,
                state: "committed",
                version: 2,
                valid: false,
                applied_policy_version: 1,
              },
        );
      });
    vi.stubGlobal("fetch", fetch);
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(() => container.textContent!.includes("继续调整策略"));
    expect(container.textContent).not.toContain("确认应用此固定策略");
    await click("继续调整策略");
    expect(selected).toHaveBeenCalledWith("");
    expect(
      fetch.mock.calls.every(([, options]) => options?.method === "GET"),
    ).toBe(true);
  });
  it("retains loaded older attempts and their focused link when the first page updates", async () => {
    const attempt = (value: number) => ({
      run_id: "r",
      node_id: "n",
      attempt: value,
      fence: value,
      worker_id: "worker",
      state: "failed",
      result_source: "not_loaded",
      started_at: "",
      completed_at: "",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const url = new URL(String(input), location.href);
        if (url.pathname.endsWith("/scheduling"))
          return response({
            run_id: "r",
            node_id: "n",
            run_version: 8,
            node_state: "ready",
            attempt: 2,
            fence: 2,
            policy_version: 1,
            evaluated_at: "",
            status: "unknown",
            blockers: [],
            requests: [],
            reservations: [],
          });
        return response(
          url.searchParams.has("cursor")
            ? { items: [attempt(1)], total: 2 }
            : { items: [attempt(2)], total: 2, next_cursor: "older" },
        );
      }),
    );
    const props = {
      project: "p",
      run: "r",
      node: "n",
      currentAttempt: 2,
      onAttempt: () => {},
    };
    await render(<NodeSchedulingFacts {...props} runVersion={7} />);
    await flush(() => container.textContent!.includes("已加载 1 / 2"));
    await click("加载更早尝试");
    await flush(() => container.textContent!.includes("已加载 2 / 2"));
    const older = [...container.querySelectorAll<HTMLAnchorElement>("a")].find(
      (anchor) => anchor.textContent!.includes("尝试 1"),
    )!;
    older.focus();
    await render(<NodeSchedulingFacts {...props} runVersion={8} />);
    await flush(() => !container.textContent!.includes("正在更新尝试历史"));
    expect(container.textContent).toContain("已加载 2 / 2");
    expect(document.activeElement).toBe(older);
  });
  it("removes protected attempt evidence on list access denial and reloads the same selection after recovery", async () => {
    let denied = false;
    let exactReads = 0;
    const selected = vi.fn();
    const identity = {
      run_id: "run-exact",
      node_id: "map[2]",
      attempt: 1,
      fence: 2,
      worker_id: "worker",
    };
    const attempt: ExecutionAttempt = {
      ...identity,
      state: "failed",
      result_source: "runner_result",
      started_at: "",
      completed_at: "2026-10-24T06:01:00Z",
      result: {
        ...identity,
        exit_code: 2,
        output: "已选尝试受保护输出",
        error: "",
        operation_id: "original-attempt",
        stopped: false,
        outcome_unknown: false,
      },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const route = path(input);
        if (route.endsWith("/scheduling"))
          return response({
            ...identity,
            run_version: 7,
            node_state: "failed",
            policy_version: 1,
            evaluated_at: "",
            status: "not_pending",
            blockers: [],
            requests: [],
            reservations: [],
          });
        if (route.endsWith("/attempts/1")) {
          exactReads++;
          return response(attempt);
        }
        if (route.endsWith("/attempts/1/tool-events")) {
          const data = { ...identity, tool_event: { legacy: "受保护的工具历史" } };
          return response({
            ...identity,
            schema_version: 1,
            project_id: "p",
            page_cursor: "saved-attempt",
            watermark: 1,
            scanned_through: 1,
            has_more: false,
            items: [{
              sequence: 1,
              run_id: identity.run_id,
              node_id: identity.node_id,
              type: "tool_progress",
              at: "2026-10-24T06:01:00Z",
              data,
              raw_data: JSON.stringify(data),
            }],
          });
        }
        expect(route).toMatch(/\/attempts$/);
        if (denied) return response({ error: { code: "forbidden" } }, 403);
        return response({
          items: [{ ...attempt, result_source: "not_loaded", result: undefined }],
          total: 2,
          next_cursor: "earlier-attempts",
        });
      }),
    );
    await render(
      <NodeSchedulingFacts
        project="p"
        run={identity.run_id}
        node={identity.node_id}
        runVersion={7}
        currentAttempt={1}
        selectedAttempt="1"
        onAttempt={selected}
      />,
    );
    await flush(() => container.textContent!.includes("受保护的工具历史"));
    expect(container.textContent).toContain("已选尝试受保护输出");
    expect(container.textContent).toContain("已加载 1 / 2 次正式尝试");
    const reloadButton = [...container.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "重新读取尝试历史",
    );

    denied = true;
    await click("重新读取尝试历史");
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect([...container.querySelectorAll("button")].filter(
      (button) => button.textContent?.trim() === "重新读取",
    )).toEqual([reloadButton]);
    expect(container.querySelector('a[aria-current="page"]')).toBeNull();
    expect(container.querySelector(".attempt-tool-history")).toBeNull();
    expect(container.textContent).not.toContain("已选尝试受保护输出");
    expect(container.textContent).not.toContain("次正式尝试");
    expect(container.textContent).not.toContain("加载更早尝试");
    expect(exactReads).toBe(1);
    expect(selected).not.toHaveBeenCalled();

    denied = false;
    await click("重新读取");
    await flush(() => container.textContent!.includes("受保护的工具历史"));
    expect(exactReads).toBe(2);
    expect(container.querySelector('a[aria-current="page"]')).not.toBeNull();
    expect(selected).not.toHaveBeenCalled();
  });
  it("focuses only the newly created ready preview once, and announces commit without stealing focus", async () => {
    let committed = false;
    const selected = vi.fn();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const route = path(input);
        if (route === "/scheduling/policy")
          return response(committed ? { ...policy, version: 1 } : policy);
        if (route.endsWith("/commit")) {
          committed = true;
          return response({ ...policy, version: 1 });
        }
        if (route === "/scheduling/policy-previews")
          return response(preview, 201);
        return response(
          committed
            ? {
                ...preview,
                state: "committed",
                version: 2,
                valid: false,
                applied_policy_version: 1,
              }
            : preview,
        );
      }),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected=""
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(() => !!container.querySelector("form"));
    await act(async () => container.querySelector("form")!.requestSubmit());
    await flush(() => selected.mock.calls.length === 1);
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(
      () =>
        document.activeElement?.getAttribute("aria-label") === "固定策略预览",
    );
    const reread = [
      ...container.querySelectorAll<HTMLButtonElement>("button"),
    ].find((button) => button.textContent === "重新核对预览")!;
    reread.focus();
    await click("重新核对预览");
    expect(document.activeElement).toBe(reread);
    await click("确认应用此固定策略");
    await flush(
      () => !!container.querySelector('[role="status"][aria-live="polite"]'),
    );
    expect(container.querySelector('[role="status"]')!.textContent).toContain(
      "生效策略版本 1",
    );
    expect(document.activeElement).toBe(reread);
  });
  it("does not steal focus when an existing preview deep link finishes loading", async () => {
    let release: ((value: Response) => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) =>
        path(input) === "/scheduling/policy"
          ? Promise.resolve(response(policy))
          : new Promise<Response>((resolve) => {
              release = resolve;
            }),
      ),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={() => {}}
        refresh={0}
      />,
    );
    await flush(() => !!container.querySelector("form"));
    const trigger = [
      ...container.querySelectorAll<HTMLButtonElement>("button"),
    ].find((button) => button.textContent === "读取当前策略")!;
    trigger.focus();
    await act(async () => release!(response(preview)));
    expect(document.activeElement).toBe(trigger);
  });
  it("refreshes a selected running attempt to terminal at the same attempt number and ignores its earlier delayed response", async () => {
    let complete = false,
      release: ((value: Response) => void) | undefined,
      exactReads = 0;
    const attempt: ExecutionAttempt = {
      run_id: "run-exact",
      node_id: "map[2]",
      attempt: 1,
      fence: 2,
      worker_id: "worker",
      state: "running",
      result_source: "unreported",
      started_at: "",
      completed_at: "",
    };
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) => {
        const route = path(input);
        if (route.endsWith("/scheduling"))
          return Promise.resolve(
            response({
              run_id: "run-exact",
              node_id: "map[2]",
              run_version: complete ? 8 : 7,
              node_state: complete ? "succeeded" : "running",
              attempt: 1,
              fence: 2,
              policy_version: 1,
              evaluated_at: "",
              status: "not_pending",
              blockers: [],
              requests: [],
              reservations: [],
            }),
          );
        if (route.endsWith("/attempts/1")) {
          exactReads++;
          if (exactReads === 2)
            return new Promise<Response>((resolve) => {
              release = resolve;
            });
          return Promise.resolve(
            response(
              complete
                ? {
                    ...attempt,
                    state: "succeeded",
                    result_source: "runner_result",
                    completed_at: "2026-10-24T06:01:00Z",
                    result: {
                      run_id: "run-exact",
                      node_id: "map[2]",
                      attempt: 1,
                      fence: 2,
                      worker_id: "worker",
                      exit_code: 0,
                      output: "同号尝试真实终态",
                      error: "",
                      operation_id: "done",
                      stopped: false,
                      outcome_unknown: false,
                    },
                  }
                : attempt,
            ),
          );
        }
        return Promise.resolve(
          response({
            items: [
              {
                ...attempt,
                result_source: "not_loaded",
                state: complete ? "succeeded" : "running",
              },
            ],
            total: 1,
          }),
        );
      }),
    );
    const props = {
      project: "p",
      run: "run-exact",
      node: "map[2]",
      currentAttempt: 1,
      selectedAttempt: "1",
      onAttempt: () => {},
    };
    await render(<NodeSchedulingFacts {...props} runVersion={7} />);
    await flush(() => container.textContent!.includes("历史尝试 1"));
    await click("重新读取所选尝试");
    expect(release).toBeDefined();
    const anchor = container.querySelector<HTMLAnchorElement>(
      'a[aria-current="page"]',
    )!;
    anchor.focus();
    complete = true;
    await render(<NodeSchedulingFacts {...props} runVersion={8} />);
    await flush(() => container.textContent!.includes("同号尝试真实终态"));
    expect(container.textContent).toContain("已成功");
    expect(document.activeElement).toBe(anchor);
    await act(async () => release!(response({ ...attempt, state: "running" })));
    expect(container.textContent).toContain("同号尝试真实终态");
    expect(document.activeElement).toBe(anchor);
  });
  it("previews exact zero/unlimited input and requires a separate explicit confirmation", async () => {
    const bodies: Record<string, unknown>[] = [];
    let committed = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown, options?: RequestInit) => {
        const route = path(input);
        if (route === "/scheduling/policy")
          return response(
            committed
              ? {
                  ...policy,
                  version: 1,
                  default: false,
                  concurrency_limit: 0,
                  budget_limit: 0,
                }
              : policy,
          );
        if (route === "/scheduling/policy-previews") {
          bodies.push(JSON.parse(options!.body as string));
          return response(preview, 201);
        }
        if (route === "/scheduling/policy-previews/fixed-preview")
          return response(
            committed
              ? {
                  ...preview,
                  state: "committed",
                  version: 2,
                  valid: false,
                  applied_policy_version: 1,
                }
              : preview,
          );
        if (route.endsWith("/commit")) {
          bodies.push(JSON.parse(options!.body as string));
          committed = true;
          return response({
            ...policy,
            version: 1,
            default: false,
            concurrency_limit: 0,
            budget_limit: 0,
          });
        }
        throw Error(route);
      }),
    );
    const selected = vi.fn();
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected=""
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(() => !!container.querySelector('input[name="concurrency"]'));
    await input("concurrency", "0");
    await input("budget", "0");
    await act(async () => container.querySelector("form")!.requestSubmit());
    expect(bodies).toHaveLength(1);
    expect(bodies[0]).toMatchObject({
      project_id: "p",
      expected_version: 0,
      policy: {
        concurrency_limit: 0,
        budget_limit: 0,
        resource_limits: { gpu: null },
      },
    });
    expect(selected).toHaveBeenCalledWith("fixed-preview");
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={selected}
        refresh={0}
      />,
    );
    await flush(() => container.textContent!.includes("策略变更审阅"));
    expect(bodies).toHaveLength(1);
    await click("确认应用此固定策略");
    await flush(() => committed);
    expect(bodies[1]).toMatchObject({ project_id: "p", expected_version: 1 });
    expect(bodies[1]).not.toHaveProperty("policy");
  });
  it("does not replace an edited policy draft when the current policy is reread", async () => {
    let fresh = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response(
          fresh ? { ...policy, version: 8, concurrency_limit: 99 } : policy,
        ),
      ),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected=""
        onSelect={() => {}}
        refresh={0}
      />,
    );
    await flush(() => !!container.querySelector('input[name="concurrency"]'));
    await input("concurrency", "7");
    fresh = true;
    await click("读取当前策略");
    await flush(() => container.textContent!.includes("v8"));
    expect(
      container.querySelector<HTMLInputElement>('input[name="concurrency"]')!
        .value,
    ).toBe("7");
    expect(container.textContent).toContain("草稿所据版本 v0");
  });
  it("restores the same unknown commit after remount despite a newer policy version", async () => {
    const bodies: unknown[] = [];
    let sent = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown, options?: RequestInit) => {
        const route = path(input);
        if (route === "/scheduling/policy")
          return response(sent ? { ...policy, version: 9 } : policy);
        if (route.endsWith("/commit")) {
          bodies.push(JSON.parse(options!.body as string));
          if (!sent++) throw Error("ack lost");
          return response({ ...policy, version: 10 });
        }
        return response(preview);
      }),
    );
    const element = (
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={() => {}}
        refresh={0}
      />
    );
    await render(element);
    await flush(() => container.textContent!.includes("策略变更审阅"));
    await click("确认应用此固定策略");
    await flush(() =>
      container.textContent!.includes("原策略操作结果尚未确认"),
    );
    await act(async () => root!.unmount());
    root = undefined;
    await render(element);
    await flush(() =>
      container.textContent!.includes("原策略操作结果尚未确认"),
    );
    await click("恢复原策略操作回执");
    await flush(() => bodies.length === 2);
    expect(bodies[1]).toEqual(bodies[0]);
  });
  it("reads an exact historical attempt outside the loaded page and keeps stale diagnostic out of current facts", async () => {
    const older: ExecutionAttempt = {
      run_id: "run-exact",
      node_id: "map[2]",
      attempt: 1,
      fence: 2,
      worker_id: "historic-worker",
      state: "failed",
      result_source: "runner_result",
      started_at: "",
      completed_at: "",
      result: {
        run_id: "run-exact",
        node_id: "map[2]",
        attempt: 1,
        fence: 2,
        worker_id: "historic-worker",
        exit_code: 2,
        output: "历史失败证据",
        error: "",
        operation_id: "op",
        stopped: false,
        outcome_unknown: false,
      },
    };
    const requests: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const route = path(input);
        requests.push(route);
        if (route.endsWith("/scheduling"))
          return response({
            run_id: "run-exact",
            node_id: "map[2]",
            run_version: 6,
            node_state: "ready",
            attempt: 2,
            fence: 3,
            policy_version: 1,
            evaluated_at: "",
            status: "blocked",
            blockers: [
              {
                code: "stale",
                message: "不应作为当前阻塞显示",
                source: "policy",
              },
            ],
            requests: [],
            reservations: [],
          });
        if (route.endsWith("/attempts/1")) return response(older);
        return response({
          items: [
            {
              ...older,
              attempt: 2,
              result_source: "not_loaded",
              result: undefined,
            },
          ],
          total: 2,
          next_cursor: "opaque",
        });
      }),
    );
    await render(
      <NodeSchedulingFacts
        project="p"
        run="run-exact"
        node="map[2]"
        runVersion={7}
        currentAttempt={2}
        selectedAttempt="1"
        onAttempt={() => {}}
      />,
    );
    await flush(() => container.textContent!.includes("历史失败证据"));
    expect(container.textContent).not.toContain("不应作为当前阻塞显示");
    expect(container.textContent).toContain("诊断依据早于当前运行");
    expect(requests).toContain("/runs/run-exact/nodes/map%5B2%5D/attempts/1");
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "所选历史尝试详情",
    );
    expect(container.textContent).toContain("已加载 1 / 2 次正式尝试");
  });
  it("uses the App queue link to preserve exact node then browser-back restores the queue", async () => {
    history.replaceState(
      null,
      "",
      "/?project=p&view=development&sub=runs&layout=queue",
    );
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const route = path(input);
        if (route === "/auth/session") return response(session);
        if (route === "/overview")
          return response({ counts: {}, activity: [] });
        if (route === "/scheduling/queue")
          return response({ items: [entry], total: 1 });
        throw Error("Unexpected " + route);
      }),
    );
    await render(<App />);
    await flush(() => !!container.querySelector("a.scheduling-open-link"));
    const link = container.querySelector<HTMLAnchorElement>(
      "a.scheduling-open-link",
    )!;
    await act(async () =>
      link.dispatchEvent(
        new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }),
      ),
    );
    await flush(() =>
      container.textContent!.includes("运行 run-exact 节点 map[2]"),
    );
    expect(parseWorkbenchRoute(location.search)).toMatchObject({
      run: "run-exact",
      node: "map[2]",
      attempt: "",
    });
    await act(async () => {
      history.back();
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
    await flush(
      () =>
        container.textContent!.includes("就绪队列") &&
        !container.querySelector('[data-testid="route-consumer"]'),
    );
    expect(parseWorkbenchRoute(location.search).layout).toBe("queue");
  });
  it("validates attempt URL and clears it when an inaccessible project is replaced", () => {
    const route = parseWorkbenchRoute(
      "?project=p&view=development&sub=runs&run=r&node=n&attempt=2",
    );
    expect(route.attempt).toBe("2");
    expect(parseWorkbenchRoute("?attempt=0").attempt).toBe("");
    expect(parseWorkbenchRoute("?attempt=01").attempt).toBe("");
    expect(
      switchedRoute(route, {
        ...session,
        projects: [{ id: "new", version: 1 }],
      }),
    ).toMatchObject({ project: "new", run: "", node: "", attempt: "" });
  });
  it("blocks stale preview versions and refuses to create policy controls for a read-only actor", async () => {
    let readonly = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) =>
        response(
          path(input) === "/scheduling/policy"
            ? {
                ...policy,
                version: 2,
                allowed_actions: readonly ? [] : ["preview", "commit"],
              }
            : preview,
        ),
      ),
    );
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected="fixed-preview"
        onSelect={() => {}}
        refresh={0}
      />,
    );
    await flush(() => container.textContent!.includes("当前策略版本已变化"));
    const confirm = [
      ...container.querySelectorAll<HTMLButtonElement>("button"),
    ].find((button) => button.textContent === "确认应用此固定策略")!;
    expect(confirm.disabled).toBe(true);
    readonly = true;
    await render(
      <SchedulingWorkspace
        project="p"
        session={session}
        selected=""
        onSelect={() => {}}
        refresh={1}
      />,
    );
    await flush(() => container.textContent!.includes("当前账号不具备"));
    expect(container.querySelector("fieldset")!.disabled).toBe(true);
  });
  it("keeps a superseded project queue response from appearing in the newly selected scope", async () => {
    let release: ((value: Response) => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: unknown) =>
        new URL(String(input), location.href).searchParams.get("project_id") ===
        "p"
          ? new Promise<Response>((resolve) => {
              release = resolve;
            })
          : Promise.resolve(
              response({
                items: [{ ...entry, node_name: "其他项目正式节点" }],
                total: 1,
              }),
            ),
      ),
    );
    await render(<SchedulingQueue project="p" refresh={0} onRun={() => {}} />);
    await render(
      <SchedulingQueue project="other" refresh={0} onRun={() => {}} />,
    );
    await flush(() => container.textContent!.includes("其他项目正式节点"));
    await act(async () =>
      release!(
        response({
          items: [{ ...entry, node_name: "禁止跨范围回写" }],
          total: 200,
        }),
      ),
    );
    expect(container.textContent).not.toContain("禁止跨范围回写");
    expect(container.textContent).not.toContain("/ 200");
  });
});
