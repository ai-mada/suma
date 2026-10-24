// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkflowView } from "./Workflow";
import { setLocale } from "./i18n";
import type { Workflow } from "./api";

// Keep the real editor and request path; only canvas geometry is unavailable
// in jsdom, so it does not participate in these command contract regressions.
vi.mock("@xyflow/react", () => ({
  ReactFlow: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  Background: () => null, Controls: () => null, MiniMap: () => null,
  Handle: () => null, Position: { Left: "left", Right: "right" },
  MarkerType: { ArrowClosed: "arrowclosed" },
  applyNodeChanges: (_: unknown, nodes: unknown) => nodes,
  applyEdgeChanges: (_: unknown, edges: unknown) => edges,
}));
vi.mock("elkjs/lib/elk.bundled.js", () => ({ default: class {
  async layout(graph: object) { return graph; }
} }));

let host: HTMLDivElement, root: Root | undefined, flow: Workflow;
let writes: { path: string; body: { workflow: Workflow } }[];
let consoleErrors: unknown[][];
beforeEach(async () => {
  await setLocale("en-US", { persist: false });
  localStorage.clear(); sessionStorage.clear(); writes = []; consoleErrors = [];
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.spyOn(console, "error").mockImplementation((...values) => consoleErrors.push(values));
  host = document.createElement("div"); document.body.append(host);
  flow = { id: "command-flow", name: "Original workflow", version: 3,
    nodes: [{ id: "build", name: "Original build", type: "command", command: ["build-original"] }], edges: [] };
  vi.stubGlobal("fetch", vi.fn(async (input: string, options?: RequestInit) => {
    const path = new URL(String(input), location.href).pathname.replace(/^\/api\/v1/, "");
    let value: unknown;
    if (options?.body) {
      writes.push({ path, body: JSON.parse(String(options.body)) });
      value = { valid: true, errors: [] };
    } else if (path === "/workflows") value = { items: [flow], total: 1 };
    else if (["/runs", "/tasks", "/tool-profiles", "/release-workers"].includes(path)) value = { items: [], total: 0 };
    else throw Error("Unexpected command fixture " + path);
    return new Response(JSON.stringify(value), { headers: { "Content-Type": "application/json" } });
  }));
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount()); root = undefined;
  host.remove(); await setLocale("zh-CN", { persist: false });
  vi.unstubAllGlobals(); vi.restoreAllMocks();
  expect(consoleErrors).toEqual([]);
});
async function change(element: HTMLInputElement | HTMLSelectElement, value: string) {
  expect(element).not.toBeNull();
  await act(async () => {
    Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype, "value")!.set!.call(element, value);
    element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  });
}
async function button(label: string) {
  const target = [...host.querySelectorAll("button")].find(item => item.textContent?.trim() === label);
  expect(target, label).toBeDefined(); await act(async () => target!.click());
}
async function renderEditor() {
  await act(async () => { root = createRoot(host); root.render(<WorkflowView project="project" refresh={0} onRefresh={() => {}} onNode={() => {}} onRun={() => {}} />); });
  await change(host.querySelector<HTMLSelectElement>('select[aria-label="Workflow version"]')!, "command-flow@3");
  const nodeSelector = [...host.querySelectorAll("label")].find(item => item.querySelector("span")?.textContent === "Choose a node")!.querySelector("select")!;
  await change(nodeSelector, "build");
}
const commandInput = (locale = "en") => host.querySelector<HTMLInputElement>(locale === "en"
  ? 'input[aria-label="Runner command identifier"],input[aria-label="Custom execution reference (optional)"]'
  : 'input[aria-label="Runner 命令标识符"],input[aria-label="自定义执行引用（可选）"]')!;

it("edits one allowlisted identifier and clears it without retaining an argument array", async () => {
  await renderEditor();
  const input = commandInput();
  expect(input).not.toBeNull(); expect(input.tagName).toBe("INPUT");
  expect(input.value).toBe("build-original");
  expect(host.textContent).toContain("Executable and arguments are configured on the Runner");
  expect(host.textContent).not.toContain("One argument per line");
  await button("Validate");
  expect(host.textContent).toContain("The current definition passed server validation");
  input.focus(); await change(input, "package.test-2");
  expect(document.activeElement).toBe(input);
  expect(host.textContent).toContain("Unpublished changes");
  expect(host.textContent).not.toContain("The current definition passed server validation");
  await button("Validate");
  expect(writes.at(-1)?.body.workflow.nodes[0].command).toEqual(["package.test-2"]);
  await change(input, ""); await button("Validate");
  expect(writes.at(-1)?.body.workflow.nodes[0].command).toEqual([]);
  expect(writes.every(write => write.path === "/workflows/validate")).toBe(true);
});

it.each(["command", "agent", "test", "script", "task"])("uses one ID for the %s execution type", async type => {
  flow.nodes[0].type = type;
  await renderEditor();
  await change(commandInput(), "next-id"); await button("Validate");
  expect(writes[0].body.workflow.nodes[0]).toMatchObject({ type, command: ["next-id"] });
});

it.each(["command", "agent", "test", "script", "task"].flatMap(type => [
  { type, command: undefined }, { type, command: [] as string[] },
]))("requires an ID only for compiler command nodes: $type / $command", async ({ type, command }) => {
  const required = ["command", "agent", "test"].includes(type);
  flow.nodes[0].type = type; flow.nodes[0].command = command;
  flow.nodes[0].config = { capability: "custom:worker" };
  await renderEditor();
  const input = commandInput();
  expect(input.required).toBe(required);
  expect(input.getAttribute("aria-label")).toBe(required ? "Runner command identifier" : "Custom execution reference (optional)");
  expect(input.getAttribute("aria-invalid")).toBe(String(required));
  expect(input.checkValidity()).toBe(!required);
  if (!required) {
    expect(host.textContent).not.toContain("An empty value cannot execute commands");
    expect(host.textContent).toContain("custom workers that declare task/script capabilities");
    expect(host.textContent).toContain("This reference does not enable local Runner execution");
  }
  const requests = vi.mocked(fetch).mock.calls.length;
  await act(async () => { await setLocale("zh-CN", { persist: false }); });
  expect(commandInput("zh")).toBe(input);
  if (!required) {
    expect(input.getAttribute("aria-label")).toBe("自定义执行引用（可选）");
    expect(host.textContent).toContain("由声明 task/script 能力的自定义执行节点处理");
    expect(host.textContent).toContain("此引用不会赋予本地 Runner 执行能力");
  }
  expect(writes).toHaveLength(0); expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
  await button("校验");
  expect(writes[0].body.workflow.nodes[0]).toMatchObject({ type, config: { capability: "custom:worker" } });
  expect(writes[0].body.workflow.nodes[0].command).toEqual(command);
});

it.each(["approval", "noop", "external_workflow"])("does not request a Runner command ID for %s nodes", async type => {
  flow.nodes[0].type = type; flow.nodes[0].command = [];
  await renderEditor();
  expect(commandInput()).toBeNull();
  expect(host.textContent).not.toContain("An empty value cannot execute commands");
});

it("retains the agent tool binding path when editing its command ID", async () => {
  flow.nodes[0].type = "agent";
  flow.nodes[0].config = { tool_profile_id: "original-profile", tool_profile_sha256: "a".repeat(64), goal: "Original human goal" };
  await renderEditor();
  const binding = [...host.querySelectorAll("label")].find(item => item.querySelector("span")?.textContent === "Fixed external tool configuration")?.querySelector("select");
  expect(binding).toBeDefined(); expect(binding?.value).toBe("original-profile");
  await change(commandInput(), "next-id"); await button("Validate");
  expect(writes[0].body.workflow.nodes[0]).toMatchObject({ command: ["next-id"], config: flow.nodes[0].config });
});

it.each([
  { command: ["build-original"] },
  { command: [" build-original "] },
  { command: ["build-original", "--raw-argument", "line\nvalue", ""] },
])(
  "retains raw command $command and validation when changing locale without another request", async ({ command }) => {
    flow.nodes[0].command = command;
    const original = [...flow.nodes[0].command];
    await renderEditor();
    const input = commandInput(); expect(input).not.toBeNull();
    await button("Validate");
    expect(writes[0].body.workflow.nodes[0].command).toEqual(original);
    const requests = vi.mocked(fetch).mock.calls.length;
    input.focus();
    await act(async () => { await setLocale("zh-CN", { persist: false }); });
    expect(commandInput("zh")).toBe(input);
    expect(document.activeElement).toBe(input);
    expect(host.textContent).toContain("当前定义通过服务端校验");
    expect(host.textContent).not.toContain("有未发布修改");
    expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
    expect(writes).toHaveLength(1);
    await button("校验");
    expect(writes[1].body.workflow.nodes[0].command).toEqual(original);
    if (original.length > 1) {
      expect(host.querySelector("pre")?.textContent).toBe(JSON.stringify(original, null, 2));
      expect(input.getAttribute("aria-invalid")).toBe("true");
      await change(input, "replacement"); await button("校验");
      expect(writes.at(-1)?.body.workflow.nodes[0].command).toEqual(["replacement"]);
    }
    expect(writes.every(write => write.path === "/workflows/validate")).toBe(true);
  },
);
