// @vitest-environment jsdom
import { setLocale } from "../i18n";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ExecutionPools, ProjectExecution } from "./index";
import { App } from "../App";
const poolID = "a".repeat(32),
  project = "b".repeat(32),
  tenant = "c".repeat(32),
  runnerID = "d".repeat(32),
  grantID = "e".repeat(32);
const session = {
  user: {
    id: "user",
    name: "资源管理员",
    email: "qa@example.test",
    role: "owner",
  },
  organization: { id: tenant, name: "当前组织" },
  projects: [{ id: project, title: "项目甲", version: 1 }],
};
const pool = {
  id: poolID,
  owner_tenant_id: tenant,
  name: "团队共享池",
  description: "真实配置额度",
  version: 1,
  status: "active",
  policy: {
    concurrency_limit: 2,
    resource_limits: {},
    budget_limit: null,
    tenant_weights: {},
    version: 1,
    budget_spent: 0,
    resources: [],
    fairness: { mode: "tenant_project_weighted_round_robin", scope: "pool" },
    as_of: "2026-10-24T08:00:00Z",
  },
  allowed_actions: ["pause", "register_runner", "preview_policy"],
  created_at: "2026-10-24T08:00:00Z",
  updated_at: "2026-10-24T08:00:00Z",
};
const grant = {
  id: grantID,
  pool_id: poolID,
  pool_name: pool.name,
  owner_tenant_id: tenant,
  tenant_id: "f".repeat(32),
  project_id: project,
  shared_project_name: "经项目方确认的共享名称",
  version: 3,
  status: "pending",
  project_weight: 2,
  request_sha256: "1".repeat(64),
  requested_by: "requestor",
  allowed_actions: ["approve", "reject"],
  can_open_project: false,
  created_at: "2026-10-24T08:00:00Z",
  updated_at: "2026-10-24T08:00:00Z",
};
const runner = {
  id: runnerID,
  pool_id: poolID,
  worker_id: "pool-" + runnerID,
  name: "本机开发节点",
  version: 1,
  status: "active",
  max_slots: 2,
  credential_version: 1,
  credential_expires_at: "2026-10-25T08:00:00Z",
  allowed_actions: ["rotate_credential", "revoke"],
  created_at: "2026-10-24T08:00:00Z",
};
let root: Root | undefined,
  host: HTMLDivElement,
  requests: { path: string; body: Record<string, unknown> }[],
  runners: (typeof runner)[],
  allowCreate: boolean,
  allowRequest: boolean,
  unavailable: boolean,
  registerCalls: number,
  loseRegistration: boolean;
const json = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
beforeEach(() => {
  history.replaceState(null, "", "/");
  localStorage.clear();
  sessionStorage.clear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.appendChild(host);
  requests = [];
  runners = [];
  allowCreate = true;
  allowRequest = true;
  unavailable = false;
  registerCalls = 0;
  loseRegistration = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: unknown, options?: RequestInit) => {
      const u = new URL(String(input), location.href),
        path = u.pathname.replace("/api/v1", "");
      if (options?.body) {
        const body = JSON.parse(String(options.body));
        requests.push({ path, body });
        if (path.endsWith("/runners")) {
          registerCalls++;
          runners = [runner];
          if (loseRegistration && registerCalls === 1)
            throw Error("response lost");
          return json({
            runner,
            ...(registerCalls === 1
              ? { token: "local-one-time-private-token" }
              : {}),
            replayed: registerCalls > 1,
            recovery_action: registerCalls > 1 ? "rotate_credential" : "",
          });
        }
        if (path.endsWith("/credentials")) {
          runners = [{ ...runner, version: 2, credential_version: 2 }];
          return json({
            runner: runners[0],
            token: "rotated-once-private",
            replayed: false,
          });
        }
        if (path.endsWith("/runners/" + runnerID + "/commands")) {
          runners = [
            { ...runner, version: 2, status: "revoked", allowed_actions: [] },
          ];
          return json(runners[0]);
        }
        if (path.includes("/grants/") && path.endsWith("/commands"))
          return json({
            ...grant,
            status: "approved",
            version: 4,
            allowed_actions: ["revoke"],
          });
        if (path === "/project-execution/grants")
          return json(
            {
              ...grant,
              tenant_id: tenant,
              status: "pending",
              allowed_actions: ["withdraw"],
            },
            201,
          );
        if (path === "/execution-pools") return json(pool, 201);
        throw Error("Unexpected POST " + path);
      }
      if (path === "/auth/session") return json({ ...session, projects: [] });
      if (path === "/organizations") return json({ items: [], total: 0 });
      if (path === "/overview") return json({ counts: {}, activity: [] });
      if (path === "/execution-pools")
        return json({
          items: [pool],
          total: 1,
          allowed_actions: allowCreate ? ["create"] : [],
        });
      if (path === "/execution-pools/" + poolID) {
        if (unavailable) throw Error("offline");
        return json(pool);
      }
      if (path.endsWith("/grants") && path.startsWith("/execution-pools/"))
        return json({ items: [grant], total: 1, allowed_actions: [] });
      if (path.endsWith("/runners"))
        return json({
          items: runners,
          total: runners.length,
          allowed_actions: ["register"],
        });
      if (path === "/project-execution/route")
        return json({
          tenant_id: tenant,
          project_id: project,
          version: 0,
          mode: "local",
          pool_id: "",
          grant_id: "",
          grant_version: 0,
          allowed_actions: allowRequest ? ["preview"] : [],
          as_of: "2026-10-24T08:00:00Z",
        });
      if (path === "/project-execution/grants")
        return json({
          items: [],
          total: 0,
          allowed_actions: allowRequest ? ["request"] : [],
        });
      throw Error("Unexpected GET " + path);
    }),
  );
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  host.remove();
  await setLocale("zh-CN", { persist: false });
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function mount(element: React.ReactNode) {
  await act(async () => {
    root = createRoot(host);
    root.render(element);
  });
  await settle();
}
async function settle() {
  for (let i = 0; i < 6; i++)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
}
const props = {
  session,
  selected: poolID,
  panel: "grants",
  refresh: 0,
  onSelect: vi.fn(),
  onPanel: vi.fn(),
};
function button(text: string) {
  const item = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (e) => e.textContent?.trim() === text,
  );
  expect(item, "button " + text).toBeDefined();
  return item!;
}
async function openManualRunnerRegistration() {
  const manual = document.querySelector<HTMLButtonElement>("[data-pool-runner-register-manual]");
  if (!manual) throw new Error("Manual runner registration button is missing");
  await act(async () => manual.click());
  await settle();
}
async function click(text: string) {
  await act(async () => button(text).click());
  await settle();
}
async function input(name: string, value: string) {
  const field = document.querySelector<HTMLInputElement>(
    `input[name="${name}"]`,
  )!;
  expect(field, "input " + name).not.toBeNull();
  await act(async () => {
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function submit() {
  const form = document.querySelector("[role=dialog] form")!;
  expect(form).not.toBeNull();
  await act(async () =>
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    ),
  );
  await settle();
}
describe("execution pool actual React consumers", () => {
  it("preserves user-provided markup-looking organization names as literal text across both languages", async () => {
    const name =
      'R&D <strong data-user="true">Alpha</strong> &amp; <code>tools</code> {{value2}}';
    await mount(
      <ExecutionPools
        {...props}
        selected=""
        session={{
          ...session,
          organization: { ...session.organization, name },
        }}
      />,
    );
    const scope = () => host.querySelector(".pool-scope")!;
    expect(scope().textContent).toContain(name);
    expect(scope().querySelector("[data-user],strong,code")).toBeNull();
    await act(async () => {
      await setLocale("en-US", { persist: false });
    });
    expect(scope().textContent).toContain(name);
    expect(scope().querySelector("[data-user],strong,code")).toBeNull();
  });

  it("changes language inside the real modal while preserving user text, focus and formal operation input", async () => {
    await mount(<ExecutionPools {...props} selected="" />);
    await click("创建资源池");
    await input("name", "用户命名不自动翻译");
    const select = document.querySelector<HTMLSelectElement>(
      '[role="dialog"] .language-selector select',
    );
    expect(select).not.toBeNull();
    select!.focus();
    await act(async () => {
      select!.value = "en-US";
      select!.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await settle();
    expect(document.activeElement).toBe(select);
    expect(
      document.querySelector<HTMLInputElement>(
        '[role="dialog"] input[name="name"]',
      )?.value,
    ).toBe("用户命名不自动翻译");
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "Pool name",
    );
    expect(button("Confirm creation")).toBeDefined();
    await submit();
    expect(requests[0].body.name).toBe("用户命名不自动翻译");
  });
  it("opens the organization pool workspace from actual App even with no current project", async () => {
    history.replaceState(null, "", "/?view=settings&sub=execution-pools");
    await mount(<App />);
    expect(host.textContent).toContain("团队共享池");
    expect(host.textContent).not.toContain("创建第一个项目");
    expect(new URLSearchParams(location.search).get("project")).toBeNull();
    expect(
      host.querySelector(".page-title .context-chip")?.textContent,
    ).toContain(session.organization.name);
    expect(
      host.querySelector('.breadcrumbs [aria-label="当前项目"]'),
    ).toBeNull();
  });
  it("opens the created pool in actual App after the confirmed write releases its leave guard", async () => {
    history.replaceState(null, "", "/?view=settings&sub=execution-pools");
    await mount(<App />);
    await click("创建资源池");
    await input("name", "新建共享池");
    await submit();
    expect(new URLSearchParams(location.search).get("record")).toBe(poolID);
    expect(new URLSearchParams(location.search).get("panel")).toBe("overview");
    expect(host.querySelector("h2")?.textContent).toBe(pool.name);
  });
  it("blocks new writes when the persisted original recovery envelope is corrupt", async () => {
    sessionStorage.setItem(
      "suma-pool-recovery-v1:" +
        JSON.stringify([tenant, session.user.id, "organization", ""]),
      "{broken",
    );
    await mount(<ExecutionPools {...props} selected="" />);
    expect(button("创建资源池").disabled).toBe(true);
    expect(host.querySelector('[role="alert"]')?.textContent).toContain(
      "恢复记录无法读取",
    );
    expect(requests).toHaveLength(0);
  });
  it("restores the policy tab from the actual App URL into the live editable policy consumer", async () => {
    history.replaceState(
      null,
      "",
      `/?view=settings&sub=execution-pools&record=${poolID}&panel=policy`,
    );
    await mount(<App />);
    expect(host.querySelector('[name="pool_concurrency"]')).not.toBeNull();
    expect(
      host.querySelector(
        'nav[aria-label="资源池页面"] button[aria-current="page"]',
      )?.textContent,
    ).toBe("策略");
  });
  it("integrates the actual current-route consumer before the project authorization list", async () => {
    await mount(<ProjectExecution {...props} project={project} selected="" />);
    expect(
      host.querySelector('[aria-label="项目默认执行路由"]'),
    ).not.toBeNull();
    expect(host.textContent).toContain("当前默认：项目本地执行");
    expect(host.querySelector('[name="execution_mode"]')).not.toBeNull();
  });
  it("uses server page actions instead of owner role to expose pool creation", async () => {
    allowCreate = false;
    await mount(<ExecutionPools {...props} selected="" />);
    expect(host.textContent).toContain("团队共享池");
    expect(
      [...host.querySelectorAll("button")].some(
        (e) => e.textContent === "创建资源池",
      ),
    ).toBe(false);
  });
  it("reviews the fixed shared label and exact version before owner approval without fabricating project access", async () => {
    await mount(<ExecutionPools {...props} />);
    expect(host.textContent).toContain(grant.shared_project_name);
    expect(host.textContent).toContain("待审批");
    expect(host.querySelector('a[href*="' + project + '"]')).toBeNull();
    await click("批准申请");
    expect(document.querySelector("[role=dialog]")?.textContent).toContain(
      grant.request_sha256,
    );
    await click("确认批准");
    expect(requests[0]).toEqual({
      path: `/execution-pools/${poolID}/grants/${grantID}/commands`,
      body: {
        action: "approve",
        expected_version: 3,
        idempotency_key: expect.any(String),
      },
    });
  });
  it("registers only name and bounded slot/TTL settings and keeps the issued secret out of persistent browser state", async () => {
    await mount(<ExecutionPools {...props} panel="runners" />);
    await openManualRunnerRegistration();
    await input("name", "本机开发节点");
    await input("max_slots", "2");
    await submit();
    const request = requests[0];
    expect(request.path).toBe(`/execution-pools/${poolID}/runners`);
    expect(request.body).toEqual({
      name: "本机开发节点",
      max_slots: 2,
      ttl_seconds: 86400,
      idempotency_key: expect.any(String),
    });
    expect(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="一次性执行凭证"]',
      )?.value,
    ).toBe("local-one-time-private-token");
    expect(
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ).not.toContain("local-one-time-private-token");
    expect(location.href).not.toContain("local-one-time-private-token");
    await click("已安全保存，关闭凭证");
    expect(
      document.querySelector('input[aria-label="一次性执行凭证"]'),
    ).toBeNull();
    expect(host.textContent).toContain("尚未收到联络记录");
  });
  it("creates a pool with explicit known zero limits instead of replacing them with defaults", async () => {
    await mount(<ExecutionPools {...props} selected="" />);
    await click("创建资源池");
    await input("name", "零派发审阅池");
    await input("concurrency", "0");
    await input("budget", "0");
    await submit();
    expect(requests[0]).toEqual({
      path: "/execution-pools",
      body: {
        name: "零派发审阅池",
        description: "",
        policy: {
          concurrency_limit: 0,
          budget_limit: 0,
          resource_limits: {},
          tenant_weights: {},
        },
        idempotency_key: expect.any(String),
      },
    });
  });
  it("requires explicit version-bound rotation and revocation without treating revoked credentials as a stopped process", async () => {
    runners = [runner];
    await mount(<ExecutionPools {...props} panel="runners" />);
    await click("轮换凭证");
    await submit();
    expect(requests[0]).toEqual({
      path: `/execution-pools/${poolID}/runners/${runnerID}/credentials`,
      body: {
        expected_version: 1,
        ttl_seconds: 86400,
        idempotency_key: expect.any(String),
      },
    });
    expect(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="一次性执行凭证"]',
      )?.value,
    ).toBe("rotated-once-private");
    await click("已安全保存，关闭凭证");
    await click("撤销节点凭证");
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "不会停止操作系统进程",
    );
    await click("确认撤销凭证");
    expect(requests[1].body).toEqual({
      action: "revoke",
      expected_version: 2,
      idempotency_key: expect.any(String),
    });
    expect(host.textContent).toContain("已撤销");
    expect(
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ).not.toContain("rotated-once-private");
  });
  it("rejects out-of-range runner slots before sending any identity registration", async () => {
    await mount(<ExecutionPools {...props} panel="runners" />);
    await openManualRunnerRegistration();
    await input("name", "本机开发节点");
    await input("max_slots", "129");
    await submit();
    expect(requests).toHaveLength(0);
    expect(
      document.querySelector('[role="dialog"] [role="alert"]')?.textContent,
    ).toContain("1–128");
  });
  it("discards a late credential response after changing the organization scope", async () => {
    await mount(<ExecutionPools {...props} panel="runners" />);
    const original = vi.mocked(fetch).getMockImplementation()!;
    let release: ((response: Response) => void) | undefined;
    vi.mocked(fetch).mockImplementation(async (input, options) => {
      if (options?.body && String(input).endsWith("/runners"))
        return new Promise<Response>((resolve) => {
          release = resolve;
        });
      return original(input, options);
    });
    await openManualRunnerRegistration();
    await input("name", "等待回执节点");
    await input("max_slots", "2");
    await submit();
    expect(release).toBeDefined();
    await act(async () =>
      root!.render(
        <ExecutionPools
          {...props}
          selected=""
          session={{
            ...session,
            organization: { id: "f".repeat(32), name: "切换后组织" },
          }}
        />,
      ),
    );
    await act(async () =>
      release!(json({ runner, token: "late-private-secret", replayed: false })),
    );
    await settle();
    expect(host.textContent).toContain("切换后组织");
    expect(
      document.querySelector('input[aria-label="一次性执行凭证"]'),
    ).toBeNull();
    expect(
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ).not.toContain("late-private-secret");
  });
  it("recovers a lost registration with the same operation and shows metadata without inventing a recoverable secret", async () => {
    loseRegistration = true;
    await mount(<ExecutionPools {...props} panel="runners" />);
    await openManualRunnerRegistration();
    await input("name", "本机开发节点");
    await input("max_slots", "2");
    await submit();
    expect(document.querySelector("[role=dialog]")?.textContent).toContain(
      "原操作结果尚未确认",
    );
    await click("恢复原操作回执");
    expect(requests).toHaveLength(2);
    expect(requests[1]).toEqual(requests[0]);
    expect(
      document.querySelector('input[aria-label="一次性执行凭证"]'),
    ).toBeNull();
    expect(document.querySelector("[role=dialog]")?.textContent).toContain(
      "凭证无法再次取回",
    );
    expect(
      JSON.stringify({ ...sessionStorage, ...localStorage }),
    ).not.toContain("local-one-time-private-token");
  });
  it("explains exactly which project metadata is shared before an explicit project request", async () => {
    await mount(<ProjectExecution {...props} project={project} selected="" />);
    await click("申请资源池");
    const dialog = document.querySelector("[role=dialog]")!;
    expect(dialog.textContent).toContain("项目甲");
    expect(dialog.textContent).toContain("不授予需求、代码或日志读取权限");
    await input("pool_id", poolID);
    await input("project_weight", "2");
    await submit();
    expect(requests[0]).toEqual({
      path: "/project-execution/grants",
      body: {
        project_id: project,
        pool_id: poolID,
        project_weight: 2,
        idempotency_key: expect.any(String),
      },
    });
  });
  it("does not expose a project request action when current server authorization disallows it", async () => {
    allowRequest = false;
    await mount(<ProjectExecution {...props} project={project} selected="" />);
    expect(host.textContent).toContain("当前项目");
    expect(
      [...host.querySelectorAll("button")].some(
        (e) => e.textContent === "申请资源池",
      ),
    ).toBe(false);
  });
  it("shows a failed pool read as unconfirmed and never opens stale management actions", async () => {
    unavailable = true;
    await mount(<ExecutionPools {...props} />);
    expect(host.querySelector('[role="alert"]')).not.toBeNull();
    expect(
      [...host.querySelectorAll("button")].some(
        (e) => e.textContent === "暂停资源池" && !e.disabled,
      ),
    ).toBe(false);
  });
});
