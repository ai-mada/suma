// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, it, expect, vi } from "vitest";
import { App } from "../App";
import { setLocale } from "./index";
const project = "project-records",
  id = "requirement-original";
const requirement = {
  id,
  project_id: project,
  version: 3,
  title: "用户原文 <code>原件</code>",
  description: "原始需求上下文",
  status: "open",
  acceptance_criteria: ["证据保持原文"],
  data: {},
};
let root: Root | undefined,
  host: HTMLDivElement,
  writes: { path: string; body: Record<string, unknown> }[];
beforeEach(async () => {
  localStorage.clear();
  sessionStorage.clear();
  await setLocale("en-US", { persist: false });
  history.replaceState(
    null,
    "",
    "/?" + new URLSearchParams({ view: "requirements", project, record: id }),
  );
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.appendChild(host);
  writes = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, options?: RequestInit) => {
      const path = new URL(String(input), location.href).pathname.replace(
        /^\/api\/v1/,
        "",
      );
      const body = options?.body ? JSON.parse(String(options.body)) : undefined;
      let value: unknown;
      if (body) {
        writes.push({ path, body });
        value = path.endsWith("/decompose")
          ? { items: [], total: 0 }
          : { ...requirement, ...body };
      } else if (path === "/auth/session")
        value = {
          user: {
            id: "user",
            name: "原始用户",
            email: "test@example.test",
            role: "owner",
          },
          organization: { id: "org", name: "原始组织" },
          projects: [{ id: project, title: "原始项目", version: 1 }],
        };
      else if (path === "/overview") value = { counts: { requirements: 1 } };
      else if (path === "/requirements")
        value = { items: [requirement], total: 1 };
      else if (path === "/requirements/" + id) value = requirement;
      else if (["/members", "/runs", "/organizations"].includes(path))
        value = { items: [], total: 0 };
      else throw Error("Unexpected fixture " + path);
      return new Response(JSON.stringify(value), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  await act(async () => {
    root = createRoot(host);
    root.render(<App />);
  });
  await settle();
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  host.remove();
  await setLocale("zh-CN", { persist: false });
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function settle() {
  for (let n = 0; n < 8; n++)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
}
async function click(name: string) {
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>("button"),
  ].find((item) => item.textContent?.trim() === name);
  expect(button, "button " + name).toBeDefined();
  await act(async () => button!.click());
  await settle();
}
it("uses complete professional record actions and detail labels while preserving user content", async () => {
  expect(host.textContent).toContain("New requirement");
  expect(host.textContent).toContain(
    "Filters and board counts apply to the loaded scope (1); project total: 1.",
  );
  expect(host.querySelector("aside.detail-pane h2")?.textContent).toBe(
    requirement.title,
  );
  expect(host.querySelector("aside.detail-pane")?.textContent).toContain(
    "Requirement details",
  );
  expect(host.querySelector("aside.detail-pane")?.textContent).toContain(
    "Assignee",
  );
  expect(
    host.querySelector('input[aria-label="Search requirements"]'),
  ).not.toBeNull();
  expect(
    host.querySelector('button[aria-label="Switch organization"]'),
  ).not.toBeNull();
  await click("Edit");
  const title = document.querySelector<HTMLInputElement>(
    "[role=dialog] input[name=title]",
  )!;
  expect(title.value).toBe(requirement.title);
  await act(async () => {
    await setLocale("zh-CN", { persist: false });
  });
  expect(title.isConnected).toBe(true);
  expect(title.value).toBe(requirement.title);
  await act(async () => {
    document
      .querySelector("[role=dialog] form")!
      .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  });
  await settle();
  expect(writes[0].path).toBe("/requirements/" + id);
  expect(writes[0].body).toMatchObject({
    title: requirement.title,
    description: requirement.description,
    acceptance_criteria: requirement.acceptance_criteria,
    expected_version: 3,
    project_id: project,
  });
});
it("decomposes through an editable localized draft without changing it on locale switch", async () => {
  await click("Split into tasks");
  const title = document.querySelector<HTMLInputElement>(
    '[role=dialog] input[aria-label="Task title"]',
  )!;
  expect(title.value).toBe("Implement: " + requirement.title);
  await act(async () => {
    await setLocale("zh-CN", { persist: false });
  });
  expect(title.value).toBe("Implement: " + requirement.title);
  await click("创建所选任务");
  expect(
    writes.find((item) => item.path.endsWith("/decompose"))?.body,
  ).toMatchObject({
    project_id: project,
    expected_version: 3,
    drafts: [
      {
        title: "Implement: " + requirement.title,
        description: requirement.description,
        acceptance_criteria: requirement.acceptance_criteria,
      },
    ],
  });
});
