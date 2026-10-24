import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ToolBindingFacts } from "./ToolBindingStatus";
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
      "本次平台执行",
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
    expect(html).toContain("指纹不匹配");
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
});
