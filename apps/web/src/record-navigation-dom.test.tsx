// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { normalizeResponse } from "./api";
import { RecordRequirementReference } from "./RecordLineage";
import { taskDeliveryHref } from "./task-delivery-navigation-model";
const project = "project-a",
  taskID = "task-exact",
  requirementID = "requirement-outside-loaded-window";
const task = {
  id: taskID,
  project_id: project,
  version: 3,
  title: "消费者任务详情",
  description: "任务源页面",
  status: "in_progress",
  requirement_id: requirementID,
  data: {},
  acceptance_criteria: ["精确打开需求"],
};
const requirement = {
  id: requirementID,
  project_id: project,
  version: 2,
  title: "消费者目标需求",
  description: "需求目标页面",
  status: "open",
  data: {},
  acceptance_criteria: ["可返回任务"],
};
let root: Root | undefined, container: HTMLDivElement;
beforeEach(() => {
  history.replaceState(
    null,
    "",
    "/?" +
      new URLSearchParams({
        project,
        view: "development",
        sub: "tasks",
        record: taskID,
      }),
  );
  localStorage.clear();
  sessionStorage.clear();
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string) => {
      const path = new URL(String(input), location.href).pathname.replace(
        /^\/api\/v1/,
        "",
      );
      let value: unknown;
      if (path === "/auth/session")
        value = {
          user: {
            id: "user",
            name: "消费者账号",
            email: "consumer@example.test",
            role: "owner",
          },
          organization: { id: "org", name: "消费者组织" },
          projects: [{ id: project, version: 1, title: "消费者项目" }],
        };
      else if (path === "/overview")
        value = { counts: { requirements: 1, tasks: 1 }, activity: [] };
      else if (path === "/tasks") value = { items: [task], total: 1 };
      else if (path === "/task-workspace") value = { items: [task], task_plans: [{task_id: task.id, iteration_id: "", plan_version: 0}], total: 1, as_of: "2026-10-29T00:00:00Z" };
      else if (path === "/tasks/" + taskID) value = task;
      else if (path === "/requirements/" + requirementID) value = requirement;
      else if (["/iterations", "/requirements", "/members", "/runs"].includes(path))
        value = { items: [], total: 0 };
      else throw Error("Unexpected fixture request " + path);
      return new Response(JSON.stringify(value), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  container.remove();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function renderConsumer(element: React.ReactNode) {
  await act(async () => {
    root = createRoot(container);
    root.render(element);
  });
}
async function settle(check: () => boolean) {
  for (let attempt = 0; attempt < 40; attempt++) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    if (check()) return;
  }
  expect(check()).toBe(true);
}
async function clickConsumer(
  link: HTMLAnchorElement,
  modifiers: MouseEventInit = {},
) {
  let preventedByConsumer = false;
  // React's delegated listener runs before document. Observe its decision, then
  // suppress JSDOM's unsupported full-document activation: this test requires
  // the real consumer to update App state/history, not native navigation.
  const probe = (event: MouseEvent) => {
    preventedByConsumer = event.defaultPrevented;
    event.preventDefault();
  };
  document.addEventListener("click", probe);
  try {
    await act(async () => {
      link.dispatchEvent(
        new MouseEvent("click", {
          bubbles: true,
          cancelable: true,
          button: 0,
          ...modifiers,
        }),
      );
    });
  } finally {
    document.removeEventListener("click", probe);
  }
  return preventedByConsumer;
}
function requirementLink() {
  return container.querySelector<HTMLAnchorElement>(
    'a[href*="view=requirements"]',
  );
}
function currentDetail(title: string) {
  return [...container.querySelectorAll("aside.detail-pane h2")].some(
    (node) => node.textContent === title,
  );
}

it("opens only an authorized task's delivery projection and keeps the close guard", async () => {
  history.replaceState(null, "", taskDeliveryHref(project, { kind: "delivery", id: taskID })!);
  const requests = vi.mocked(fetch);
  const defaultReply = requests.getMockImplementation()!;
  const exactTask = "/tasks/" + taskID;
  let allowTask!: (response: Response) => void;
  requests.mockImplementation(async (input, options) => {
    const url = new URL(String(input), location.href);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (path === "/task-workspace") return Response.json({ items: [], task_plans: [], total: 0 });
    if (path === exactTask) return new Promise<Response>((resolve) => { allowTask = resolve; });
    if (path === exactTask + "/delivery") {
      expect(url.searchParams.get("project_id")).toBe(project);
      expect(options?.method).toBe("GET");
      const empty = { scope: "current_run", total: 0, limit: 20, complete: true, items: [] };
      return Response.json({
        project_id: project, task_id: taskID, task_version: task.version, task_status: task.status,
        run_basis_state: "not_dispatched", run: null, formal_acceptance: false, expert_plans: empty, issues: empty,
      });
    }
    return defaultReply(input, options);
  });
  const deliveryReads = () => requests.mock.calls.filter(([input]) => String(input).includes(exactTask + "/delivery"));
  const selector = "[data-task-delivery-toggle]";
  await renderConsumer(<App />);
  await settle(() => !!allowTask);
  expect(container.querySelector(selector)).toBeNull();
  expect(deliveryReads()).toHaveLength(0);
  await act(async () => allowTask(Response.json(task)));
  await settle(() => container.querySelector(selector)?.getAttribute("aria-expanded") === "true" && deliveryReads().length === 1);
  expect(currentDetail(task.title)).toBe(true);
  const toggle = container.querySelector<HTMLButtonElement>(selector)!;
  const guard = (event: Event) => event.preventDefault();
  window.addEventListener("suma:navigate", guard);
  try {
    await act(async () => toggle.click());
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  } finally {
    window.removeEventListener("suma:navigate", guard);
  }
  await act(async () => toggle.click());
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(deliveryReads()).toHaveLength(1);
  expect(requests.mock.calls.every(([, options]) => !options?.body)).toBe(true);
});

it("navigates a team delivery origin and requires an explicit unpinned route after rebinding", async () => {
  const pinned = taskDeliveryHref(project, { kind: "delivery", id: taskID, runID: "former-run" })!;
  history.replaceState(null, "", "?project=" + project + "&view=development&sub=runs&layout=team");
  const requests = vi.mocked(fetch);
  const defaultReply = requests.getMockImplementation()!;
  const empty = { scope: "current_run", total: 0, limit: 20, complete: true, items: [] };
  requests.mockImplementation(async (input, options) => {
    const url = new URL(String(input), location.href);
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (path === "/tasks/" + taskID + "/delivery") {
      expect([...url.searchParams]).toEqual([["project_id", project]]);
      expect(options?.method).toBe("GET");
      return Response.json({
        project_id: project, task_id: taskID, task_version: task.version, task_status: task.status,
        run_basis_state: "not_dispatched", run: null, formal_acceptance: false, expert_plans: empty, issues: empty,
      });
    }
    return defaultReply(input, options);
  });
  await renderConsumer(<App />);
  await settle(() => !!container.querySelector("main"));
  const open = new CustomEvent("suma:open-task-delivery", { detail: pinned, cancelable: true });
  await act(async () => { window.dispatchEvent(open); });
  expect(open.defaultPrevented).toBe(true);
  await settle(() => !!container.querySelector('[data-task-delivery-origin][data-relation="unbound"]'));
  const unpin = container.querySelector<HTMLAnchorElement>("[data-task-delivery-current-link]")!;
  expect(new URL(location.href).searchParams.get("panel")).toBe("task-delivery:former-run");
  await act(async () => { unpin.click(); });
  await settle(() => new URL(location.href).searchParams.get("panel") === "task-delivery");
  expect(container.querySelector("[data-task-delivery-origin]")).toBeNull();
  expect(requests.mock.calls.every(([, options]) => !options?.body)).toBe(true);
});

describe("actual React consumer requirement navigation", () => {
  it("uses one wrapping clickable box for the label, icon and long requirement identifier", async () => {
    await renderConsumer(
      <RecordRequirementReference
        project={project}
        item={normalizeResponse(task)}
        onOpen={() => {}}
      />,
    );
    const link = container.querySelector<HTMLAnchorElement>("a")!;
    expect(link.style.display).toBe("inline-flex");
    expect(link.style.flexWrap).toBe("wrap");
    expect(link.style.maxWidth).toBe("100%");
    expect(link.children).toHaveLength(2);
    expect(link.children[0].tagName).toBe("SPAN");
    const id = link.querySelector<HTMLElement>("code")!;
    expect(id.style.minWidth).toBe("0");
    expect(id.style.overflowWrap).toBe("anywhere");
    expect(id.style.wordBreak).toBe("break-all");
    expect(id.textContent).toBe(requirementID);
    // JSDOM has no layout engine. Physical rect/center hit-testing is a
    // separate real-browser check; no getBoundingClientRect values are mocked.
  });
  it("clicks from task into the exact unloaded requirement and browser-back restores task detail", async () => {
    await renderConsumer(<App />);
    await settle(() => !!requirementLink());
    expect(currentDetail(task.title)).toBe(true);
    expect(await clickConsumer(requirementLink()!)).toBe(true);
    await settle(
      () =>
        new URLSearchParams(location.search).get("view") === "requirements" &&
        currentDetail(requirement.title),
    );
    expect(new URLSearchParams(location.search).get("record")).toBe(
      requirementID,
    );
    expect(new URLSearchParams(location.search).get("project")).toBe(project);
    await act(async () => history.back());
    await settle(
      () =>
        new URLSearchParams(location.search).get("view") === "development" &&
        currentDetail(task.title),
    );
    expect(new URLSearchParams(location.search).get("record")).toBe(taskID);
  });
  it("uses the App leave guard so an unknown operation cannot be bypassed through the requirement link", async () => {
    await renderConsumer(<App />);
    await settle(() => !!requirementLink());
    const guard = vi.fn((event: Event) => event.preventDefault());
    window.addEventListener("suma:navigate", guard);
    try {
      expect(await clickConsumer(requirementLink()!)).toBe(true);
      expect(guard).toHaveBeenCalledOnce();
      expect(new URLSearchParams(location.search).get("view")).toBe(
        "development",
      );
      expect(currentDetail(task.title)).toBe(true);
    } finally {
      window.removeEventListener("suma:navigate", guard);
    }
  });
  it("handles ordinary clicks through the real component callback while preserving modifier-key native links", async () => {
    const open = vi.fn();
    await renderConsumer(
      <RecordRequirementReference
        project={project}
        item={normalizeResponse(task)}
        onOpen={open}
      />,
    );
    const link = container.querySelector("a")!;
    expect(await clickConsumer(link, { ctrlKey: true })).toBe(false);
    expect(open).not.toHaveBeenCalled();
    expect(await clickConsumer(link, { metaKey: true })).toBe(false);
    expect(open).not.toHaveBeenCalled();
    expect(await clickConsumer(link)).toBe(true);
    expect(open).toHaveBeenCalledExactlyOnceWith(requirementID);
  });
});
