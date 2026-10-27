// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToolWorkspace } from "../ToolWorkspace";
const project = "b".repeat(32),
  poolID = "a".repeat(32),
  workerID = "pool-" + "d".repeat(32);
const session = {
  user: {
    id: "user",
    name: "项目管理员",
    email: "qa@example.test",
    role: "owner",
  },
  organization: { id: "c".repeat(32), name: "项目组织" },
  projects: [{ id: project, title: "项目甲", version: 1 }],
};
const manifest = {
  items: [
    {
      profile: {
        command_id: "native-agent",
        adapter: "suma",
        protocol: "suma-jsonl-v1",
        profile_sha256: "1".repeat(64),
        executable_sha256: "2".repeat(64),
        tool_version: "fixture-1",
        read_only: true,
      },
      available: true,
      reason: "",
    },
  ],
};
let root: Root | undefined, host: HTMLDivElement, deny: boolean, posts: any[];
const json = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), {
    status,
    headers: { "Content-Type": "application/json" },
  });
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  sessionStorage.clear();
  host = document.createElement("div");
  document.body.appendChild(host);
  deny = false;
  posts = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: unknown, options?: RequestInit) => {
      const url = new URL(String(input), location.href),
        path = url.pathname.replace("/api/v1", "");
      if (options?.body) {
        const body = JSON.parse(String(options.body));
        posts.push(body);
        return json(
          {
            id: "e".repeat(32),
            version: 1,
            state: "paused",
            name: body.name,
            worker_id: body.worker_id,
            profile: body.profile,
            available: false,
            reason: "配置已暂停",
          },
          201,
        );
      }
      if (path === "/tool-profiles" || path === "/release-workers")
        return json({ items: [], total: 0 });
      if (path === "/project-execution/runners")
        return deny
          ? json(
              { error: { code: "forbidden", message: "当前项目范围不可读" } },
              403,
            )
          : json({
              items: [
                {
                  id: "d".repeat(32),
                  worker_id: workerID,
                  pool_id: poolID,
                  name: "共享池开发节点",
                  status: "active",
                  credential_expires_at: "2026-10-25T08:00:00Z",
                },
              ],
              as_of: "2026-10-24T08:00:00Z",
            });
      throw Error("Unexpected identity GET " + path);
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
  for (let i = 0; i < 5; i++)
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
}
async function mount() {
  await act(async () => {
    root = createRoot(host);
    root.render(
      <ToolWorkspace
        project={project}
        session={session}
        selected=""
        panel=""
        refresh={0}
        onSelect={() => {}}
      />,
    );
  });
  await settle();
  const button = [...host.querySelectorAll("button")].find(
    (b) => b.textContent?.trim() === "登记配置",
  )!;
  await act(async () => button.click());
  await settle();
}
function field(label: string) {
  return [...document.querySelectorAll("label.field")]
    .find((e) => e.firstElementChild?.textContent === label)
    ?.querySelector("input,textarea,select") as
    | HTMLInputElement
    | HTMLSelectElement
    | HTMLTextAreaElement;
}
async function value(label: string, text: string) {
  const element = field(label);
  expect(element).not.toBeNull();
  await act(async () => {
    const type =
      element.tagName === "SELECT"
        ? HTMLSelectElement
        : element.tagName === "TEXTAREA"
          ? HTMLTextAreaElement
          : HTMLInputElement;
    Object.getOwnPropertyDescriptor(type.prototype, "value")!.set!.call(
      element,
      text,
    );
    element.dispatchEvent(
      new Event(element.tagName === "SELECT" ? "change" : "input", {
        bubbles: true,
      }),
    );
  });
}
describe("project-authorized shared pool tool identities", () => {
  it("offers the current project shared runner separately from private identity management and saves only its worker ID", async () => {
    await mount();
    const selection = field("绑定已登记执行节点") as HTMLSelectElement;
    const option = selection.querySelector(`option[value="${workerID}"]`);
    expect(option).not.toBeNull();
    expect(option?.textContent).toContain("共享池");
    expect(
      host.querySelector(".tool-workers")?.textContent || "",
    ).not.toContain(workerID);
    await value("配置名称", "共享池工具");
    await value("绑定已登记执行节点", workerID);
    await value("本机非秘密工具清单", JSON.stringify(manifest));
    const check = [...document.querySelectorAll("button")].find(
      (b) => b.textContent?.trim() === "核对清单字段",
    )!;
    await act(async () => check.click());
    await value("选择清单内配置", "native-agent");
    const form = document.querySelector('[role="dialog"] form')!;
    await act(async () =>
      form.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      ),
    );
    await settle();
    expect(posts[0].worker_id).toBe(workerID);
    expect(posts[0].project_id).toBe(project);
    expect(posts[0]).not.toHaveProperty("pool_id");
  });
  it("reports a forbidden shared-runner read instead of presenting it as a successful empty list", async () => {
    deny = true;
    await mount();
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "当前项目范围不可读",
    );
    expect(
      field("绑定已登记执行节点").querySelector(`option[value="${workerID}"]`),
    ).toBeNull();
    expect(posts).toHaveLength(0);
  });
  it("does not turn a project-shared identity into a private revoke/manage entry", async () => {
    await mount();
    const close = document.querySelector<HTMLButtonElement>(
      '[role="dialog"] [aria-label="关闭对话框"]',
    )!;
    await act(async () => close.click());
    await settle();
    expect(host.textContent).not.toContain(workerID);
    expect(posts).toHaveLength(0);
  });
});
