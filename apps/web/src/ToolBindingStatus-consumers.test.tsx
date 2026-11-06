// @vitest-environment jsdom
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ToolBindingFacts, ToolBindingStatus } from "./ToolBindingStatus";
import type { ToolProfile, ToolRunBinding } from "./tool-model";
const binding: ToolRunBinding = {
  profile_id: "profile",
  profile_sha256: "a".repeat(64),
  command: "coder",
  run_id: "run-1",
  node_id: "agent-1",
};
const profile: ToolProfile = {
  id: "profile",
  project_id: "p",
  worker_id: "worker",
  name: "编码工具",
  version: 3,
  state: "enabled",
  profile: {
    command_id: "coder",
    adapter: "suma",
    protocol: "suma-jsonl-v1",
    profile_sha256: "a".repeat(64),
    executable_sha256: "b".repeat(64),
    tool_version: "1.0",
    read_only: false,
  },
  enabled_by: "u",
  created_at: "",
  updated_at: "",
  last_seen_at: "2026-10-24T00:00:00Z",
  available: false,
  reason: "inventory_stale",
};
describe("pinned run tool and current eligibility", () => {
  it("shows pinned command and profile before any process event or evidence exists", () => {
    const html = renderToStaticMarkup(
      createElement(ToolBindingFacts, {
        project: "p",
        binding,
        state: "ready",
        loading: true,
      }),
    );
    for (const text of [
      "固定工具依据",
      "coder",
      "profile",
      "a".repeat(64),
      "当前新派发资格",
      "正在读取",
    ])
      expect(html).toContain(text);
    expect(html).not.toContain("已验证");
    expect(html).not.toContain("已创建会话");
  });
  it("keeps current stale eligibility separate from the already successful run and preserves run/node return context", () => {
    const html = renderToStaticMarkup(
      createElement(ToolBindingFacts, {
        project: "p",
        binding,
        state: "succeeded",
        profile,
      }),
    );
    for (const text of [
      "当前节点状态",
      "已成功",
      "当前新派发资格",
      "未满足",
      "上报已过期",
      "不会改变本次运行",
      "record=profile",
      "return-run",
      "run-1",
      "agent-1",
    ])
      expect(html).toContain(text);
  });
  it("does not accept a fresh but mismatched profile as eligible for the pinned node", () => {
    const html = renderToStaticMarkup(
      createElement(ToolBindingFacts, {
        project: "p",
        binding,
        state: "ready",
        profile: {
          ...profile,
          available: true,
          reason: "",
          profile: { ...profile.profile, profile_sha256: "c".repeat(64) },
        },
      }),
    );
    expect(html).toContain("固定指纹或命令与当前配置不匹配");
    expect(html).not.toContain("当前节点身份");
    expect(html).not.toContain("本次读取满足");
  });
  it("retains immutable binding metadata while a current read fails", () => {
    const html = renderToStaticMarkup(
      createElement(ToolBindingFacts, {
        project: "p",
        binding,
        state: "running",
        profile: { ...profile, available: true },
        error: "当前权限未确认",
      }),
    );
    expect(html).toContain("coder");
    expect(html).toContain("当前资格未确认");
    expect(html).not.toContain("本次读取满足");
  });

  it.each([
    { id: "foreign-profile" },
    { project_id: "foreign-project" },
    { profile: undefined },
    { available: "true" },
  ])("withholds unverified profile facts for %j", patch => {
    const html = renderToStaticMarkup(<ToolBindingFacts binding={binding} project="p" state="ready"
      profile={{ ...profile, name: "private configuration", worker_id: "private worker", ...patch } as ToolProfile} />);
    expect(html).toContain("当前配置响应未能通过核对");
    expect(html).not.toContain("private configuration");
    expect(html).not.toContain("private worker");
    expect(html).not.toContain("本次读取满足");
  });

  it("hides a previous current read during refresh", () => {
    const html = renderToStaticMarkup(<ToolBindingFacts binding={binding} project="p" state="ready"
      profile={{ ...profile, available: true, worker_id: "previous worker" }} loading />);
    expect(html).toContain("正在读取");
    expect(html).not.toContain("previous worker");
    expect(html).not.toContain("本次读取满足");
  });

  it("loads exact fixed profiles for every supported node type and skips incomplete bindings", async () => {
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    const fetcher = vi.fn(async () => Response.json(profile));
    vi.stubGlobal("fetch", fetcher);
    const node = { id: "node-1", type: "agent", state: "failed", command: ["coder"], tool_profile_id: "profile", tool_profile_sha256: "a".repeat(64) };
    try {
      const types = ["agent", "test", "command", "terminal_session"];
      const incomplete = [
        { type: "approval" },
        { tool_profile_id: "" },
        { tool_profile_id: "path/tool" },
        { tool_profile_sha256: undefined },
        { tool_profile_sha256: "a".repeat(64) + "\n" },
        { command: [] },
        { command: ["coder", "--flag"] },
        { command: ["../coder"] },
      ];
      await act(async () => root.render(<>
        {types.map(type => <ToolBindingStatus key={type} project="p" runID="run-1" node={{ ...node, type }} attempt={2} />)}
        {incomplete.map((patch, index) => <ToolBindingStatus key={index} project="p" runID="run-1" node={{ ...node, ...patch }} />)}
      </>));
      expect(fetcher).toHaveBeenCalledTimes(types.length);
      expect(host.querySelectorAll("[data-run-tool-eligibility=blocked]")).toHaveLength(types.length);
      const links = host.querySelectorAll<HTMLAnchorElement>("[data-run-tool-profile-link]");
      expect(links).toHaveLength(types.length);
      const link = links[0]!;
      expect(new URLSearchParams(link.getAttribute("href")?.slice(1)).get("panel")).toBe("return-run:run-1/node-1/2");
      const guard = vi.fn((event: Event) => event.preventDefault());
      window.addEventListener("suma:navigate", guard);
      try {
        expect(link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }))).toBe(false);
        expect(guard).toHaveBeenCalledTimes(1);
        const modified = new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true });
        link.addEventListener("click", event => event.preventDefault(), { once: true });
        link.dispatchEvent(modified);
        expect(guard).toHaveBeenCalledTimes(1);
      } finally {
        window.removeEventListener("suma:navigate", guard);
      }
    } finally {
      await act(async () => root.unmount());
      host.remove();
      vi.unstubAllGlobals();
    }
  });

  it("allows one current poll at a time and withholds earlier eligibility after refusal", async () => {
    const host = document.createElement("div");
    document.body.append(host);
    const root = createRoot(host);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let finish!: (response: Response) => void;
    const refresh = new Promise<Response>(resolve => { finish = resolve; });
    const fetcher = vi.fn().mockResolvedValueOnce(Response.json({ ...profile, available: true }))
      .mockReturnValueOnce(refresh);
    vi.stubGlobal("fetch", fetcher);
    try {
      await act(async () => root.render(<ToolBindingStatus project="p" runID="run-1" node={{
        id: "node-1", type: "test", state: "succeeded", command: ["coder"], tool_profile_id: "profile", tool_profile_sha256: "a".repeat(64),
      }} />));
      expect(host.querySelector("[data-run-tool-eligibility=eligible]")).not.toBeNull();
      await act(async () => { vi.advanceTimersByTime(45_000); });
      expect(fetcher).toHaveBeenCalledTimes(2);
      expect(host.querySelector("[data-run-tool-eligibility=loading]")).not.toBeNull();
      expect(host.textContent).not.toContain("当前节点身份");
      await act(async () => finish(Response.json({ error: { code: "forbidden", message: "Forbidden" } }, { status: 403 })));
      await act(async () => { vi.advanceTimersByTime(45_000); });
      expect(fetcher).toHaveBeenCalledTimes(2);
      expect(host.querySelector("[data-run-tool-eligibility=unverified]")).not.toBeNull();
      expect(host.textContent).not.toContain("当前节点身份");
    } finally {
      await act(async () => root.unmount());
      host.remove();
      vi.restoreAllMocks();
      vi.unstubAllGlobals();
      vi.useRealTimers();
    }
  });
});
