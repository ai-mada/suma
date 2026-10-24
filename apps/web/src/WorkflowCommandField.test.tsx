// @vitest-environment jsdom
import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkflowCommandField } from "./WorkflowCommandField";
import { setLocale } from "./i18n";

let host: HTMLDivElement, root: Root | undefined;
beforeEach(async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  await setLocale("en-US", { persist: false });
  host = document.createElement("div"); document.body.append(host);
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount()); root = undefined;
  host.remove(); await setLocale("zh-CN", { persist: false }); vi.unstubAllGlobals();
});
async function render(command: string[] | undefined, disabled = false, required = true) {
  const onChange = vi.fn();
  function Editor() {
    const [value, setValue] = useState(command);
    return <WorkflowCommandField command={value} disabled={disabled} required={required} onChange={next => { onChange(next); setValue(next); }} />;
  }
  await act(async () => { root = createRoot(host); root.render(<Editor />); });
  return { input: host.querySelector("input")!, onChange };
}
async function edit(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
const describedText = (input: HTMLInputElement) => input.getAttribute("aria-describedby")!.split(" ").map(id => document.getElementById(id)?.textContent).join(" ");

it("replaces the command with one exact ID on human edit, then clears to an empty array", async () => {
  const { input, onChange } = await render(["original"]);
  input.focus(); await edit(input, "A9.build-test_2");
  expect(onChange.mock.calls).toEqual([[["A9.build-test_2"]]]);
  expect(input.value).toBe("A9.build-test_2"); expect(document.activeElement).toBe(input);
  await edit(input, ""); expect(onChange.mock.calls.at(-1)).toEqual([[]]);
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(describedText(input)).toContain("An empty value cannot execute commands");
});

it.each([
  { command: ["build", "--arg", "", "raw\nline", "<script>raw</script>"] },
  { command: [" build "] },
  { command: ["npm test"] },
  { command: ["build\n"] },
  { command: ["build\r\nother"] },
  { command: ["a".repeat(101)] },
  { command: ["编译"] },
  { command: ["-build"] },
])("preserves complete invalid original $command until an explicit edit", async ({ command }) => {
  const original = [...command];
  const { input, onChange } = await render(command);
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(host.querySelector("pre")?.textContent).toBe(JSON.stringify(original, null, 2));
  expect(host.querySelector("script")).toBeNull();
  const before = input.value; input.focus();
  await act(async () => { await setLocale("zh-CN", { persist: false }); });
  expect(host.querySelector("input")).toBe(input); expect(input.value).toBe(before);
  expect(document.activeElement).toBe(input); expect(onChange).not.toHaveBeenCalled();
  expect(command).toEqual(original);
  expect(host.querySelector("pre")?.textContent).toBe(JSON.stringify(original, null, 2));
  expect(describedText(input)).toContain(command.length > 1 ? "多个命令值" : "1–100");
  await edit(input, "replacement");
  expect(onChange).toHaveBeenCalledExactlyOnceWith(["replacement"]);
  expect(input.getAttribute("aria-invalid")).not.toBe("true");
  expect(host.querySelector("pre")).toBeNull();
});

it.each([undefined, []])("keeps an absent command %j empty without synthesizing access", async command => {
  const { input, onChange } = await render(command);
  expect(input.value).toBe(""); expect(input.required).toBe(true);
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(onChange).not.toHaveBeenCalled(); expect(host.querySelector("pre")).toBeNull();
});

it.each([{ command: undefined }, { command: [] as string[] }])("permits absent generic-worker commands $command without claiming execution availability", async ({ command }) => {
  const { input, onChange } = await render(command, false, false);
  expect(input.value).toBe(""); expect(input.required).toBe(false);
  expect(input.getAttribute("aria-invalid")).toBe("false"); expect(input.checkValidity()).toBe(true);
  expect(input.getAttribute("aria-label")).toBe("Custom execution reference (optional)");
  expect(input.closest("label")?.querySelector("span")?.textContent).toBe("Custom execution reference (optional)");
  expect(describedText(input)).toContain("custom workers that declare task/script capabilities");
  expect(describedText(input)).toContain("This reference does not enable local Runner execution");
  expect(onChange).not.toHaveBeenCalled();
  await edit(input, "local-build"); expect(onChange).toHaveBeenLastCalledWith(["local-build"]);
  await edit(input, ""); expect(onChange).toHaveBeenLastCalledWith([]);
  expect(input.getAttribute("aria-invalid")).toBe("false");
});

it.each([{ command: [""] }, { command: ["build", "--arg", "raw\nvalue"] }])("keeps malformed optional command arrays $command until human input replaces them", async ({ command }) => {
  const original = [...command];
  const { input, onChange } = await render(command, false, false);
  expect(input.required).toBe(false); expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(host.querySelector("pre")?.textContent).toBe(JSON.stringify(original, null, 2));
  await act(async () => { await setLocale("zh-CN", { persist: false }); });
  expect(host.querySelector("pre")?.textContent).toBe(JSON.stringify(original, null, 2));
  expect(onChange).not.toHaveBeenCalled(); expect(command).toEqual(original);
  await edit(input, "replacement"); expect(onChange).toHaveBeenCalledExactlyOnceWith(["replacement"]);
});

it.each(["a", "A9.build_test-2", "a".repeat(100)])("accepts the exact shared ID alphabet and boundary: %s", async value => {
  const { input, onChange } = await render([value]);
  expect(input.value).toBe(value); expect(input.getAttribute("aria-invalid")).not.toBe("true");
  expect(input.checkValidity()).toBe(true); expect(onChange).not.toHaveBeenCalled();
});

it("exposes the label, help and disabled state through the native input", async () => {
  const { input, onChange } = await render(["build"], true);
  expect(input.type).toBe("text"); expect(input.disabled).toBe(true);
  expect(input.getAttribute("aria-label")).toBe("Runner command identifier");
  expect(input.closest("label")?.querySelector("span")?.textContent).toBe("Runner command identifier");
  expect(describedText(input)).toContain("one existing allowlisted command ID");
  expect(describedText(input)).toContain("Executable and arguments are configured on the Runner");
  expect(host.querySelector("select,textarea")).toBeNull(); expect(onChange).not.toHaveBeenCalled();
});
