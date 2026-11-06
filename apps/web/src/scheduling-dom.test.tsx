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
  it("triages loaded queue rows without promoting stale or mismatched admission", async () => {
    const admission = {
      run_id: entry.run_id, node_id: entry.node_id, run_version: entry.run_version,
      node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
      policy_version: 1, evaluated_at: entry.ready_at,
      requests: [], reservations: [], status: "blocked",
      blockers: [
        { code: "worker_capability_unknown", source: "worker", message: "待核对资格" },
        { code: "resource_capacity", source: "policy", resource: "execution_slots", message: "限额证据" },
      ],
    };
    const checked = { ...entry, node_id: "checked", admission: { ...admission, node_id: "checked", status: "eligible", blockers: [] } };
    let status = 200;
    const fetcher = vi.fn(async (input: unknown) => {
      const params = new URL(String(input), location.origin).searchParams;
      if (status !== 200) return response({ error: { code: "failed", message: "Read unavailable" } }, status);
      return params.has("cursor") ? response({ items: [checked], total: 3 }) : response({
        items: [{ ...entry, admission }, { ...entry, node_id: "other", admission }], total: 3, next_cursor: "next-page",
      });
    });
    vi.stubGlobal("fetch", fetcher);
    const onRun = vi.fn();
    await render(<SchedulingQueue project="p" refresh={0} onRun={onRun} />);
    await flush(() => !!container.querySelector('[data-queue-filter="blocked"]'));
    const filter = (key: string) => container.querySelector<HTMLButtonElement>(`[data-queue-filter="${key}"]`)!;
    const rows = () => [...container.querySelectorAll('[data-queue-entry]')];
    const select = async (key: string) => act(async () => filter(key).click());
    expect(rows().map(row => row.getAttribute('data-queue-status'))).toEqual(['blocked', 'unknown']);
    expect(container.querySelector('[data-queue-details]')?.hasAttribute('open')).toBe(false);
    expect(rows()[0].querySelectorAll('a.scheduling-open-link')).toHaveLength(1);
    const policyHref = new URL(rows()[0].querySelector<HTMLAnchorElement>('[data-queue-policy]')!.href);
    expect(policyHref.searchParams.get('project')).toBe('p');
    expect(policyHref.searchParams.get('sub')).toBe('scheduling');
    expect(filter('all').textContent).toContain('2');
    await select('checked');
    expect(filter('checked').getAttribute('aria-pressed')).toBe('true');
    expect(rows()).toHaveLength(0);
    await click('加载更多就绪节点');
    await flush(() => rows().length === 1);
    expect(rows()[0].getAttribute('data-queue-status')).toBe('checked');
    expect(fetcher.mock.calls[1][0]).toContain('cursor=next-page');
    const link = rows()[0].querySelector<HTMLAnchorElement>('a.scheduling-open-link')!;
    await act(async () => link.click());
    expect(onRun).toHaveBeenCalledWith(entry.run_id, 'checked');
    status = 503;
    await click('重新读取队列');
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect(container.querySelector('[data-queue-details], [data-queue-policy], [data-queue-entry][data-queue-status="checked"]')).toBeNull();
    status = 403;
    await click('重试读取队列');
    await flush(() => rows().length === 0);
    expect(container.querySelector('[data-queue-filter]')).toBeNull();
  });
  it("opens diagnostics only for an exact current fixed tool blocker", async () => {
    const binding = { profile_id: "profile-1", profile_sha256: "a".repeat(64), command_id: "verify" };
    const row = (node_id: string, code: string, tool_binding = binding) => ({
      ...entry, node_id, command_id: binding.command_id,
      admission: {
        run_id: entry.run_id, node_id, run_version: entry.run_version,
        node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
        policy_version: 1, evaluated_at: entry.ready_at, requests: [], reservations: [], status: "blocked",
        blockers: [{ code, source: "tool", message: "source tool reason", run_id: entry.run_id, node_id }],
        tool_binding,
      },
    });
    const exact = row(entry.node_id, "tool_profile_paused");
    const missing = row("missing", "tool_profile_missing");
    const malformed = row("malformed", "tool_profile_changed", { ...binding, profile_sha256: "invalid" });
    const wrongCommand = row("wrong-command", "tool_profile_revoked", { ...binding, command_id: "other" });
    const wrongNode = row("wrong-node", "tool_activation_revoked");
    wrongNode.admission.blockers[0].node_id = entry.node_id;
    let fail = false;
    vi.stubGlobal("fetch", vi.fn(async () => fail
      ? response({ error: { message: "Read unavailable" } }, 503)
      : response({ items: [exact, missing, malformed, wrongCommand, wrongNode], total: 5 })));

    await render(<SchedulingQueue project="p" refresh={0} onRun={() => {}} />);
    await flush(() => !!container.querySelector("[data-queue-tool-diagnostics]"));
    const links = container.querySelectorAll<HTMLAnchorElement>("[data-queue-tool-diagnostics]");
    expect(links).toHaveLength(1);
    const link = links[0];
    expect(link.closest("[data-queue-entry]")?.querySelector(".scheduling-queue-reason")?.textContent).toBe("固定工具档案已暂停");
    expect(link.closest("[data-queue-entry]")?.querySelector(".scheduling-open-link")).not.toBeNull();
    expect(Object.fromEntries(new URL(link.href).searchParams)).toEqual({
      project: "p", view: "settings", sub: "tools", record: "profile-1", panel: "return-run:run-exact/map%5B2%5D",
    });

    const guard = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener("suma:navigate", guard);
    try {
      expect(link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }))).toBe(false);
      window.addEventListener("click", event => event.preventDefault(), { once: true });
      link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true }));
      expect(guard).toHaveBeenCalledOnce();
    } finally {
      window.removeEventListener("suma:navigate", guard);
    }

    fail = true;
    await click("重新读取队列");
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect(container.querySelector("[data-queue-tool-diagnostics]")).toBeNull();
    expect(container.querySelector(".scheduling-open-link")).not.toBeNull();
  });
  it("distinguishes shared Runner registration and credentials and offers the current project's resources", async () => {
    const row = (node_id: string, code: string) => ({
      ...entry, node_id, node_name: node_id,
      admission: {
        run_id: entry.run_id, node_id, run_version: entry.run_version,
        node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
        execution_pool_id: "pool-a", policy_version: 1, evaluated_at: entry.ready_at,
        requests: [], reservations: [], status: "blocked",
        blockers: [{ code, source: "pool", message: "source pool reason", run_id: entry.run_id, node_id }],
      },
    });
    const registration = row("registration", "pool_runner_registration_missing");
    const credential = row("credential", "pool_runner_credential_unavailable");
    credential.admission.blockers.unshift({
      code: "resource_capacity", source: "policy", message: "policy reason", run_id: entry.run_id, node_id: "credential",
    });
    const paused = row("paused", "pool_paused");
    const wrongNode = row("wrong-node", "pool_runner_credential_unavailable");
    wrongNode.admission.blockers[0].node_id = "credential";
    const noPool = row("no-pool", "pool_runner_registration_missing");
    noPool.admission.execution_pool_id = "";
    const wrongSource = row("wrong-source", "pool_runner_registration_missing");
    wrongSource.admission.blockers[0].source = "worker";
    let fail = false;
    vi.stubGlobal("fetch", vi.fn(async () => fail
      ? response({ error: { message: "Read unavailable" } }, 503)
      : response({ items: [registration, credential, paused, wrongNode, noPool, wrongSource], total: 6 })));

    await render(<SchedulingQueue project="p" refresh={0} onRun={() => {}} />);
    await flush(() => !!container.querySelector("[data-queue-pool-resources]"));
    const links = container.querySelectorAll<HTMLAnchorElement>("[data-queue-pool-resources]");
    expect(links).toHaveLength(3);
    const [registrationCard, credentialCard] = Array.from(links, link => link.closest("[data-queue-entry]")!);
    expect(registrationCard.querySelector(".scheduling-queue-reason")?.textContent).toBe("共享池没有启用的执行节点登记");
    expect(registrationCard.querySelector(".scheduling-queue-next-step")?.textContent).toBe("请联系共享池管理员登记执行节点。");
    expect(credentialCard.querySelectorAll(".scheduling-queue-reason")[1]?.textContent).toBe("共享池没有可用的节点凭证");
    expect(credentialCard.querySelector(".scheduling-queue-next-step")?.textContent).toBe("请联系共享池管理员核对并恢复节点凭证。");
    expect(credentialCard.querySelector(".scheduling-blockers")?.textContent).toContain("共享池没有可用的节点凭证");
    expect(credentialCard.querySelector("[data-queue-policy]")).not.toBeNull();
    expect(registrationCard.querySelector(".scheduling-open-link")).not.toBeNull();
    expect(Object.fromEntries(new URL(links[0].href).searchParams)).toEqual({
      project: "p", view: "settings", sub: "project-execution",
    });

    const guard = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener("suma:navigate", guard);
    try {
      expect(links[0].dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }))).toBe(false);
      expect(guard).toHaveBeenCalledOnce();
    } finally {
      window.removeEventListener("suma:navigate", guard);
    }

    fail = true;
    await click("重新读取队列");
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect(container.querySelector("[data-queue-pool-resources], .scheduling-queue-next-step")).toBeNull();
    expect(container.querySelector(".scheduling-open-link")).not.toBeNull();
  });
  it("uses the App queue link to open the exact node and browser Back restores the shared queue scope", async () => {
    history.replaceState(
      null,
      "",
      "/?project=p&view=development&sub=runs&layout=queue&queue_route=pool&queue_run=run-exact&queue_tool=tool-a",
    );
    const reads: URLSearchParams[] = [];
    const poolEntry = {
      ...entry,
      command_id: "codex-cli",
      admission: {
        tool_binding: { profile_id: "tool-a", command_id: "codex-cli", profile_sha256: "a".repeat(64) },
        execution_pool_id: "pool-a", run_id: entry.run_id, node_id: entry.node_id,
        run_version: entry.run_version, node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
        policy_version: 1, evaluated_at: entry.ready_at, status: "unknown", blockers: [], requests: [], reservations: [],
      },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: unknown) => {
        const route = path(input);
        if (route === "/auth/session") return response(session);
        if (route === "/overview")
          return response({ counts: {}, activity: [] });
        if (route === "/scheduling/queue") {
          const params = new URL(String(input), location.origin).searchParams;
          expect(params.get("route")).toBe("pool");
          expect(params.get("run_id")).toBe("run-exact");
          reads.push(params);
          return response({ items: [poolEntry], total: 1 });
        }
        throw Error("Unexpected " + route);
      }),
    );
    await render(<App />);
    await flush(() => !!container.querySelector("a.scheduling-open-link"));
    expect(reads.at(-1)?.get("tool_profile_id")).toBe("tool-a");
    expect(container.querySelector("[data-queue-tool-scope] code")?.textContent).toBe("tool-a");
    await act(async () => container.querySelector<HTMLButtonElement>('[data-queue-filter="unknown"]')!.click());
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
    expect(new URLSearchParams(location.search).has("queue_run")).toBe(false);
    expect(new URLSearchParams(location.search).has("queue_tool")).toBe(false);
    await act(async () => {
      history.back();
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
    await flush(
      () =>
        container.textContent!.includes("就绪队列") &&
        !container.querySelector('[data-testid="route-consumer"]'),
    );
    expect(parseWorkbenchRoute(location.search)).toMatchObject({
      layout: "queue", queue_route: "pool", queue_run: "run-exact", queue_tool: "tool-a",
    });
    await flush(() => container.querySelector('[data-queue-filter="unknown"]')?.getAttribute('aria-pressed') === 'true');
    expect(reads.at(-1)?.get("tool_profile_id")).toBe("tool-a");

    await act(async () => container.querySelector<HTMLButtonElement>("[data-queue-tool-clear]")!.click());
    await flush(() => reads.at(-1)?.has("tool_profile_id") === false);
    expect(parseWorkbenchRoute(location.search)).toMatchObject({
      layout: "queue", queue_route: "pool", queue_run: "run-exact",
    });
    expect(parseWorkbenchRoute(location.search).queue_tool).toBeUndefined();
    expect(document.activeElement?.hasAttribute("data-queue-run-filter")).toBe(true);
  });

  it("requires a fresh scope for an invalid link and rejects a mismatched continuation", async () => {
    const queue = "?project=p&view=development&sub=runs&layout=queue";
    expect(parseWorkbenchRoute(queue + "&queue_route=pool&queue_route=project").queue_invalid).toBe("1");
    expect(parseWorkbenchRoute(queue + "&queue_run=").queue_invalid).toBe("1");
    expect(parseWorkbenchRoute(queue + "&queue_tool=tool-a%0A").queue_invalid).toBe("1");
    expect(parseWorkbenchRoute(queue + "&queue_tool=").queue_invalid).toBe("1");
    expect(parseWorkbenchRoute("?view=development&sub=runs&layout=queue&queue_run=run-exact").queue_invalid).toBe("1");
    history.replaceState(null, "", "/" + queue + "&queue_tool=tool-a&queue_tool=tool-b");
    const reads: URLSearchParams[] = [];
    const poolEntry = {
      ...entry,
      admission: {
        execution_pool_id: "pool-a", run_id: entry.run_id, node_id: entry.node_id,
        run_version: entry.run_version, node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
        policy_version: 1, evaluated_at: entry.ready_at, status: "unknown", blockers: [], requests: [], reservations: [],
      },
    };
    let resolveOld: (result: Response) => void = () => {};
    const old = new Promise<Response>(resolve => { resolveOld = resolve; });
    vi.stubGlobal("fetch", vi.fn(async (input: unknown) => {
      const route = path(input);
      if (route === "/auth/session") return response(session);
      if (route === "/overview") return response({ counts: {}, activity: [] });
      if (route === "/scheduling/queue") {
        const params = new URL(String(input), location.origin).searchParams;
        reads.push(params);
        if (params.get("cursor") === "scoped-next") {
          const wrongRun = { ...poolEntry, run_id: "different-run", node_name: "范围外记录" };
          return response({ items: [wrongRun], total: 2, next_cursor: "foreign" });
        }
        if (params.get("run_id") === entry.run_id) {
          return response({ items: [poolEntry], total: 2, next_cursor: "scoped-next" });
        }
        if (params.get("route") === "pool") return old;
        return response({ items: [{ ...entry, node_name: "原范围记录" }], total: 14, next_cursor: "old-next" });
      }
      throw Error("Unexpected " + route);
    }));

    await render(<App />);
    await flush(() => !!container.querySelector('[data-queue-clear]'));
    expect(container.querySelector('[role="alert"]')?.textContent).toContain("链接中的队列范围无效");
    expect(reads).toHaveLength(0);
    await act(async () => container.querySelector<HTMLButtonElement>('[data-queue-clear]')!.click());
    await flush(() => container.textContent!.includes("原范围记录"));
    expect(document.activeElement?.hasAttribute('data-queue-run-filter')).toBe(true);

    const route = container.querySelector<HTMLSelectElement>('[data-queue-route]')!;
    await act(async () => {
      route.value = "pool";
      route.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(reads.at(-1)?.get("route")).toBe("pool");
    expect(reads.at(-1)?.has("cursor")).toBe(false);
    expect(container.textContent).not.toContain("原范围记录");
    expect(container.querySelector(".loaded-scope")).toBeNull();

    const filter = container.querySelector<HTMLInputElement>('[data-queue-run-filter]')!;
    const apply = container.querySelector<HTMLButtonElement>('[data-queue-apply]')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(filter, "run /bad");
      filter.dispatchEvent(new Event("input", { bubbles: true }));
      apply.click();
    });
    expect(filter.getAttribute("aria-invalid")).toBe("true");
    expect(reads).toHaveLength(2);
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(filter, entry.run_id);
      filter.dispatchEvent(new Event("input", { bubbles: true }));
    });
    await act(async () => apply.click());
    await flush(() => container.textContent!.includes("当前范围已加载 1 / 2"));
    expect(reads.at(-1)?.get("run_id")).toBe(entry.run_id);
    expect(reads.at(-1)?.get("route")).toBe("pool");
    await act(async () => resolveOld(response({ items: [], total: 51, next_cursor: "stale" })));
    expect(container.textContent).not.toContain("51");
    await click("加载更多就绪节点");
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect(reads.at(-1)?.get("cursor")).toBe("scoped-next");
    expect(container.textContent).not.toContain("范围外记录");
    expect(container.querySelectorAll("[data-queue-entry]")).toHaveLength(1);

    await act(async () => container.querySelector<HTMLButtonElement>('[data-queue-clear]')!.click());
    await flush(() => container.textContent!.includes("原范围记录"));
    const focusRun = container.querySelector<HTMLButtonElement>('[data-queue-focus-run]')!;
    await act(async () => {
      focusRun.focus();
      focusRun.click();
    });
    await flush(() => filter.value === entry.run_id);
    expect(document.activeElement).toBe(filter);
    expect(reads.at(-1)?.get("run_id")).toBe(entry.run_id);
  });
  it("retries the same tool-scoped continuation without accepting a differently cased profile", async () => {
    const scope = { project: "p", queueTool: "Tool-A", queueRoute: "pool" as const, refresh: 0, onRun: vi.fn() };
    const admission = {
      execution_pool_id: "pool-a", run_id: entry.run_id, node_id: entry.node_id,
      run_version: entry.run_version, node_state: entry.node_state, attempt: entry.attempt, fence: entry.fence,
      policy_version: 1, evaluated_at: entry.ready_at, status: "unknown", blockers: [], requests: [], reservations: [],
      tool_binding: { profile_id: "Tool-A", command_id: "codex-cli", profile_sha256: "a".repeat(64) },
    };
    const reads: URLSearchParams[] = [];
    let mismatch = true;
    vi.stubGlobal("fetch", vi.fn(async (input: unknown) => {
      const params = new URL(String(input), location.origin).searchParams;
      reads.push(params);
      const first = { ...entry, command_id: "codex-cli", admission };
      if (!params.has("cursor")) return response({ items: [first], total: 2, next_cursor: "next-tool" });
      const tool_binding = { ...admission.tool_binding, profile_id: mismatch ? "tool-a" : "Tool-A" };
      const node_id = "next-node";
      return response({
        items: [{ ...first, node_id, node_name: mismatch ? "范围外工具" : "固定工具续页", admission: { ...admission, node_id, tool_binding } }],
        total: 2,
      });
    }));

    await render(<SchedulingQueue {...scope} />);
    await flush(() => container.textContent!.includes("当前范围已加载 1 / 2"));
    await click("加载更多就绪节点");
    await flush(() => !!container.querySelector('[role="alert"]'));
    expect(container.textContent).not.toContain("范围外工具");
    expect(reads.at(-1)?.get("cursor")).toBe("next-tool");
    mismatch = false;
    await click("重试读取队列");
    await flush(() => container.textContent!.includes("固定工具续页"));
    expect(reads.map(params => [params.get("route"), params.get("tool_profile_id"), params.get("cursor")])).toEqual([
      ["pool", "Tool-A", null], ["pool", "Tool-A", "next-tool"], ["pool", "Tool-A", "next-tool"],
    ]);
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
