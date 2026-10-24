// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import {
  normalizeLocale,
  resolveLocale,
  getLocale,
  setLocale,
  formatNumber,
  formatDateTime,
  formatRelativeTime,
  renderDiagnostic,
  setServerCatalog,
} from "./index";
import { LanguageSelector } from "./LanguageSelector";
import { api, stateLabel, time } from "../api";
let root: Root | undefined, host: HTMLDivElement;
beforeEach(async () => {
  localStorage.clear();
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  await setLocale("zh-CN", { persist: false });
  host = document.createElement("div");
  document.body.appendChild(host);
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  host.remove();
  await setLocale("zh-CN", { persist: false });
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
describe("shared frontend locale foundation", () => {
  it("matches BCP47 regional aliases across browser, desktop and Go without accepting controls or Hant", () => {
    for (const value of ["en-GB", "en_AU", "en-Latn-CA"])
      expect(normalizeLocale(value)).toBe("en-US");
    for (const value of ["zh-SG", "zh-Hans-CN"])
      expect(normalizeLocale(value)).toBe("zh-CN");
    for (const value of [
      "en\n",
      "en\u0085",
      "zh-HK",
      "zh-Hant-CN",
      "en-Cyrl",
      "en-" + "x".repeat(129),
    ])
      expect(normalizeLocale(value)).toBeUndefined();
  });
  it("validates exact typed parameter bytes, controls and identifier alphabet before replacing a source diagnostic", async () => {
    setServerCatalog({
      version: "typed",
      template_format: "named-dot-v1",
      default_locale: "zh-CN",
      supported_locales: ["zh-CN", "en-US"],
      messages: [
        {
          key: "typed",
          parameters: [
            { name: "field", kind: "identifier" },
            { name: "text", kind: "text" },
          ],
          texts: {
            "zh-CN": "{{.field}} {{.text}}",
            "en-US": "{{.field}} {{.text}}",
          },
        },
      ],
    });
    await setLocale("en-US", { persist: false });
    for (const field of ["project/id", "a@b", "plus+field", "x".repeat(101)])
      expect(
        renderDiagnostic({
          message: "保留具体原诊断",
          message_key: "typed",
          message_params: { field, text: "valid" },
        }),
      ).toBe("保留具体原诊断");
    for (const text of ["", "x\n", "x\u0085", "汉".repeat(171), "\ud800"])
      expect(
        renderDiagnostic({
          message: "保留具体原诊断",
          message_key: "typed",
          message_params: { field: "_.:-", text },
        }),
      ).toBe("保留具体原诊断");
    expect(
      renderDiagnostic({
        message: "original",
        message_key: "typed",
        message_params: { field: "_.:-", text: "valid" },
      }),
    ).toBe("_.:- valid");
  });
  it("rejects malformed or duplicate catalog definitions without installing an unsafe template", () => {
    const message = {
      key: "duplicate",
      parameters: [],
      texts: { "zh-CN": "原文", "en-US": "Source" },
    };
    const envelope = {
      version: "invalid",
      template_format: "named-dot-v1",
      default_locale: "zh-CN",
      supported_locales: ["zh-CN", "en-US"],
    };
    for (const messages of [
      [message, message],
      [{ ...message, parameters: [{ name: "missing", kind: "text" }] }],
      [{ ...message, parameters: [{ name: "x", kind: "other" }] }],
      [{ ...message, texts: { "zh-CN": "{{if .x}}", "en-US": "{{if .x}}" } }],
    ])
      expect(() =>
        setServerCatalog({ ...envelope, messages } as never),
      ).toThrow();
  });
  it("normalizes supported aliases without pretending traditional Chinese is implemented", () => {
    expect(normalizeLocale("en")).toBe("en-US");
    expect(normalizeLocale("ZH-hans")).toBe("zh-CN");
    expect(normalizeLocale("zh-TW")).toBeUndefined();
    expect(normalizeLocale("zh-Hant")).toBeUndefined();
  });
  it("uses explicit, organization, Accept and default in order while unknown sources fall through", () => {
    expect(
      resolveLocale({ explicit: "xx", organization: "en", accept: ["zh"] }),
    ).toMatchObject({ locale: "en-US", source: "organization" });
    expect(resolveLocale({ accept: ["zh-TW", "en-US"] })).toMatchObject({
      locale: "en-US",
      source: "accept",
    });
  });
  it("switches a native selector without resetting another control or stealing its focus", async () => {
    await act(async () => {
      root = createRoot(host);
      root.render(
        <>
          <LanguageSelector />
          <input aria-label="用户原文" defaultValue="保留原始内容" />
        </>,
      );
    });
    const select = host.querySelector("select");
    expect(select).not.toBeNull();
    select!.focus();
    await act(async () => {
      select!.value = "en-US";
      select!.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(getLocale()).toBe("en-US");
    expect(document.documentElement.lang).toBe("en-US");
    expect(document.activeElement).toBe(select);
    expect(host.querySelector("input")?.value).toBe("保留原始内容");
    expect(localStorage.getItem("suma-locale-v1")).toBe("en-US");
  });
  it("uses Intl and keeps known zero visible", async () => {
    await setLocale("en-US", { persist: false });
    expect(formatNumber(1000)).toBe(
      new Intl.NumberFormat("en-US").format(1000),
    );
    expect(formatNumber(0)).toBe("0");
    expect(formatRelativeTime(-1, "day")).toBe(
      new Intl.RelativeTimeFormat("en-US", { numeric: "auto" }).format(
        -1,
        "day",
      ),
    );
    expect(
      formatDateTime("2026-10-24T08:00:00Z", {
        timeZone: "UTC",
        month: "long",
        day: "numeric",
      }),
    ).toBe(
      new Intl.DateTimeFormat("en-US", {
        timeZone: "UTC",
        month: "long",
        day: "numeric",
      }).format(new Date("2026-10-24T08:00:00Z")),
    );
  });
  it("localizes common state/time and adds language headers without changing a formal write body", async () => {
    await setLocale("en-US", { persist: false });
    const fetcher = vi.fn(
      async (_input?: unknown, _options?: RequestInit) =>
        new Response(JSON.stringify({ ok: true }), {
          headers: { "Content-Type": "application/json" },
        }),
    );
    vi.stubGlobal("fetch", fetcher);
    const body = {
      expected_version: 4,
      idempotency_key: "operation-stays-fixed",
    };
    await api("/fixture", body);
    expect(fetcher.mock.calls[0][1]).toMatchObject({
      headers: expect.objectContaining({ "Accept-Language": "en-US" }),
      body: JSON.stringify(body),
    });
    expect(stateLabel("ready")).toBe("Awaiting dispatch");
    expect(time("2026-10-24T08:00:00Z")).toBe(
      new Intl.DateTimeFormat("en-US", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date("2026-10-24T08:00:00Z")),
    );
  });
  it("retranslates only a stable server key or exact static source and retains unknown or invalid diagnostics", async () => {
    setServerCatalog({
      version: "opaque",
      template_format: "named-dot-v1",
      default_locale: "zh-CN",
      supported_locales: ["zh-CN", "en-US"],
      messages: [
        {
          key: "pool.limit",
          parameters: [{ name: "limit", kind: "integer" }],
          texts: { "zh-CN": "上限{{.limit}}", "en-US": "Limit {{.limit}}" },
        },
        {
          key: "pool.paused",
          parameters: [],
          texts: { "zh-CN": "共享池已暂停", "en-US": "Pool paused" },
        },
      ],
    });
    await setLocale("en-US", { persist: false });
    expect(
      renderDiagnostic({
        message: "具体原文",
        message_key: "pool.limit",
        message_params: { limit: 0 },
      }),
    ).toBe("Limit 0");
    expect(renderDiagnostic("共享池已暂停")).toBe("Pool paused");
    expect(
      renderDiagnostic({
        message: "保留具体失败",
        message_key: "pool.limit",
        message_params: { limit: 1.5 },
      }),
    ).toBe("保留具体失败");
    expect(
      renderDiagnostic({ message: "特定未知失败", code: "invalid_input" }),
    ).toBe("特定未知失败");
    await setLocale("zh-CN", { persist: false });
    expect(renderDiagnostic("Pool paused")).toBe("共享池已暂停");
  });
});
