// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PoolPolicyWorkspace, ProjectRouteWorkspace } from "./Previews";
import type { ExecutionPool } from "../../../../packages/contracts/src";
const poolID = "a".repeat(32),
  project = "b".repeat(32),
  tenant = "c".repeat(32),
  grantID = "d".repeat(32),
  previewID = "e".repeat(32);
const session = {
  user: {
    id: "user",
    name: "测试管理员",
    email: "qa@example.test",
    role: "owner",
  },
  organization: { id: tenant, name: "当前组织" },
  projects: [{ id: project, title: "项目甲", version: 1 }],
};
const basePool: ExecutionPool = {
  id: poolID,
  owner_tenant_id: tenant,
  name: "团队共享池",
  description: "",
  version: 7,
  status: "active",
  policy: {
    concurrency_limit: 2,
    resource_limits: { gpu: null },
    budget_limit: null,
    tenant_weights: { [tenant]: 2 },
    version: 3,
    budget_spent: 0,
    resources: [],
    fairness: { mode: "tenant_project_weighted_round_robin", scope: "pool" },
    as_of: "2026-10-24T08:00:00Z",
  },
  allowed_actions: ["preview_policy"],
  created_at: "2026-10-24T08:00:00Z",
  updated_at: "2026-10-24T08:00:00Z",
};
const approvedGrant = {
  id: grantID,
  pool_id: poolID,
  pool_name: basePool.name,
  owner_tenant_id: tenant,
  tenant_id: tenant,
  project_id: project,
  shared_project_name: "项目甲",
  version: 4,
  status: "approved",
  project_weight: 2,
  request_sha256: "1".repeat(64),
  requested_by: "requestor",
  allowed_actions: ["withdraw"],
  can_open_project: true,
  created_at: "2026-10-24T08:00:00Z",
  updated_at: "2026-10-24T08:00:00Z",
};
let root: Root | undefined,
  host: HTMLDivElement,
  calls: { path: string; body: any }[],
  pool: typeof basePool,
  route: any,
  policyPreview: any,
  routePreview: any,
  expired: boolean,
  loseCommit: boolean;
const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  sessionStorage.clear();
  host = document.createElement("div");
  document.body.appendChild(host);
  calls = [];
  pool = structuredClone(basePool);
  expired = false;
  loseCommit = false;
  route = {
    tenant_id: tenant,
    project_id: project,
    version: 0,
    mode: "local",
    pool_id: "",
    grant_id: "",
    grant_version: 0,
    allowed_actions: ["preview"],
    as_of: "2026-10-24T08:00:00Z",
  };
  policyPreview = {
    id: previewID,
    pool_id: poolID,
    version: 1,
    state: "ready",
    valid: true,
    policy_version: 3,
    pool_version: 7,
    payload_sha256: "2".repeat(64),
    before: basePool.policy,
    policy: { ...basePool.policy, concurrency_limit: 0, budget_limit: 0 },
    changes: [
      { resource: "execution_slots", unit: "slots", before: 2, after: 0 },
    ],
    weight_changes: [{ tenant_id: tenant, before: 2, after: 3 }],
    impact: {
      ready_nodes: 4,
      active_attempts: 1,
      as_of: "2026-10-24T08:00:00Z",
    },
    created_at: "2026-10-24T08:00:00Z",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    applied_policy_version: 0,
    allowed_actions: ["commit"],
  };
  routePreview = {
    id: previewID,
    project_id: project,
    version: 1,
    state: "ready",
    valid: true,
    route_version: 0,
    payload_sha256: "3".repeat(64),
    before: route,
    route: {
      mode: "pool",
      pool_id: poolID,
      grant_id: grantID,
      grant_version: 4,
    },
    pool_name: basePool.name,
    pool_status: "active",
    impact: { existing_runs: 5, as_of: "2026-10-24T08:00:00Z" },
    created_at: "2026-10-24T08:00:00Z",
    expires_at: new Date(Date.now() + 600000).toISOString(),
    applied_route_version: 0,
    allowed_actions: ["commit"],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: unknown, options?: RequestInit) => {
      const path = new URL(String(input), location.href).pathname.replace(
        "/api/v1",
        "",
      );
      if (options?.body) {
        const body = JSON.parse(String(options.body));
        calls.push({ path, body });
        if (path === `/execution-pools/${poolID}/policy-previews`) {
          policyPreview = {
            ...policyPreview,
            policy: body.policy,
            policy_version: body.expected_version,
          };
          return json(policyPreview);
        }
        if (path.endsWith("/policy-previews/" + previewID + "/commit")) {
          pool = {
            ...pool,
            policy: { ...pool.policy, ...policyPreview.policy, version: 4 },
          };
          policyPreview = {
            ...policyPreview,
            state: "committed",
            version: 2,
            applied_policy_version: 4,
            allowed_actions: [],
          };
          if (
            loseCommit &&
            calls.filter((call) => call.path.endsWith("/commit")).length === 1
          )
            throw Error("commit response lost");
          return json(pool);
        }
        if (path === "/project-execution/route-previews")
          return json(routePreview);
        if (path.endsWith("/route-previews/" + previewID + "/commit")) {
          route = {
            ...route,
            ...routePreview.route,
            version: 1,
            pool_name: basePool.name,
            grant_state: "approved",
          };
          routePreview = {
            ...routePreview,
            state: "committed",
            version: 2,
            applied_route_version: 1,
            allowed_actions: [],
          };
          return json(route);
        }
        throw Error("Unexpected preview POST " + path);
      }
      if (path === `/execution-pools/${poolID}/policy-previews/${previewID}`)
        return json(policyPreview);
      if (path === "/project-execution/route") return json(route);
      if (path === "/project-execution/route-previews/" + previewID)
        return json(
          expired
            ? {
                ...routePreview,
                expires_at: "2000-01-01T00:00:00Z",
                allowed_actions: [],
              }
            : routePreview,
        );
      throw Error("Unexpected preview GET " + path);
    }),
  );
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  host.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function settle() {
  for (let i = 0; i < 6; i++)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
}
async function mount(node: React.ReactNode) {
  await act(async () => {
    root = createRoot(host);
    root.render(node);
  });
  await settle();
}
function button(text: string) {
  const item = [...host.querySelectorAll<HTMLButtonElement>("button")].find(
    (e) => e.textContent?.trim() === text,
  );
  expect(item, "button " + text).toBeDefined();
  return item!;
}
async function click(text: string) {
  await act(async () => button(text).click());
  await settle();
}
async function set(name: string, value: string) {
  const element = host.querySelector<HTMLInputElement | HTMLSelectElement>(
    `[name="${name}"]`,
  )!;
  expect(element, "field " + name).not.toBeNull();
  await act(async () => {
    const type =
      element.tagName === "SELECT" ? HTMLSelectElement : HTMLInputElement;
    Object.getOwnPropertyDescriptor(type.prototype, "value")!.set!.call(
      element,
      value,
    );
    element.dispatchEvent(
      new Event(element.tagName === "SELECT" ? "change" : "input", {
        bubbles: true,
      }),
    );
  });
}
function PolicyHarness() {
  const [id, setID] = useState("");
  const [current, setCurrent] = useState(pool);
  return (
    <PoolPolicyWorkspace
      session={session}
      pool={current}
      grants={[]}
      previewID={id}
      onPreview={setID}
      onApplied={setCurrent}
    />
  );
}
function RouteHarness() {
  const [id, setID] = useState("");
  return (
    <ProjectRouteWorkspace
      session={session}
      project={project}
      grants={[approvedGrant]}
      previewID={id}
      onPreview={setID}
      refresh={0}
    />
  );
}
describe("fixed execution policy and route preview consumers", () => {
  it("sends the policy version and explicit zero limits, then separately commits the exact preview version", async () => {
    await mount(<PolicyHarness />);
    await set("pool_concurrency", "0");
    await set("pool_budget", "0");
    await set("weight-" + tenant, "3");
    await click("预览资源池策略");
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toEqual({
      expected_version: 3,
      policy: {
        concurrency_limit: 0,
        budget_limit: 0,
        resource_limits: { gpu: null },
        tenant_weights: { [tenant]: 3 },
      },
      idempotency_key: expect.any(String),
    });
    expect(host.textContent).toContain("成功准入次数");
    expect(host.textContent).toContain("执行或待核对尝试");
    const primary = button("确认应用策略");
    primary.focus();
    await click("确认应用策略");
    expect(document.activeElement).toBe(primary);
    expect(primary.textContent).toBe("继续调整策略");
    expect(calls[1].body).toEqual({
      expected_version: 1,
      idempotency_key: expect.any(String),
    });
    expect(host.textContent).toContain("已应用策略v4");
    await click("继续调整策略");
    expect(calls).toHaveLength(2);
  });
  it("preserves the editable draft when a new policy is read and requires explicit rebasing", async () => {
    await mount(
      <PoolPolicyWorkspace
        session={session}
        pool={pool}
        grants={[]}
        previewID=""
        onPreview={() => {}}
        onApplied={() => {}}
      />,
    );
    await set("pool_concurrency", "5");
    await act(async () =>
      root!.render(
        <PoolPolicyWorkspace
          session={session}
          pool={{
            ...pool,
            policy: { ...pool.policy, version: 4, concurrency_limit: 9 },
          }}
          grants={[]}
          previewID=""
          onPreview={() => {}}
          onApplied={() => {}}
        />,
      ),
    );
    expect(
      host.querySelector<HTMLInputElement>('[name="pool_concurrency"]')?.value,
    ).toBe("5");
    expect(button("预览资源池策略").disabled).toBe(true);
    await click("以当前版本重新审阅草稿");
    expect(
      host.querySelector<HTMLInputElement>('[name="pool_concurrency"]')?.value,
    ).toBe("5");
    expect(button("预览资源池策略").disabled).toBe(false);
  });
  it("fixes an approved grant version into a future-run route preview and never auto commits it", async () => {
    await mount(<RouteHarness />);
    await set("execution_mode", "pool");
    await set("execution_grant", grantID);
    await click("预览默认路由");
    expect(calls).toHaveLength(1);
    expect(calls[0].body).toEqual({
      project_id: project,
      expected_version: 0,
      route: {
        mode: "pool",
        pool_id: poolID,
        grant_id: grantID,
        grant_version: 4,
      },
      idempotency_key: expect.any(String),
    });
    expect(host.textContent).toContain("5次已有运行保持原路由");
    await click("确认使用此路由");
    expect(calls[1].body).toEqual({
      project_id: project,
      expected_version: 1,
      idempotency_key: expect.any(String),
    });
    expect(host.textContent).toContain("已应用路由v1");
  });
  it("shows the server invalidation reason and blocks an otherwise ready unexpired preview", async () => {
    policyPreview = {
      ...policyPreview,
      valid: false,
      reason: "项目授权已变化，需要重新审阅",
      allowed_actions: [],
    };
    await mount(
      <PoolPolicyWorkspace
        session={session}
        pool={pool}
        grants={[]}
        previewID={previewID}
        onPreview={() => {}}
        onApplied={() => {}}
      />,
    );
    expect(host.textContent).toContain("项目授权已变化，需要重新审阅");
    expect(button("确认应用策略").disabled).toBe(true);
    expect(calls).toHaveLength(0);
  });
  it("recovers a lost policy commit using the exact original envelope instead of issuing a new decision", async () => {
    loseCommit = true;
    await mount(<PolicyHarness />);
    await click("预览资源池策略");
    await click("确认应用策略");
    expect(host.textContent).toContain("原操作结果尚未确认");
    expect(calls).toHaveLength(2);
    await click("恢复原操作回执");
    expect(calls).toHaveLength(3);
    expect(calls[2]).toEqual(calls[1]);
    expect(host.textContent).toContain("已应用策略v4");
  });
  it("opens an expired route preview as reviewable evidence without an enabled commit", async () => {
    expired = true;
    await mount(
      <ProjectRouteWorkspace
        session={session}
        project={project}
        grants={[approvedGrant]}
        previewID={previewID}
        onPreview={() => {}}
        refresh={0}
      />,
    );
    expect(host.textContent).toContain("预览已过期");
    expect(button("确认使用此路由").disabled).toBe(true);
    expect(calls).toHaveLength(0);
  });
});
