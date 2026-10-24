import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api";
import { UiError } from "./index";
import { LocalePreferenceRecovery } from "./preference-write";

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const prefix = "suma-locale-recovery-v1:";
const scope = "account:user-1";
const body = { locale: "en-US", expected_version: 0, idempotency_key: "original-operation-123" };
const stored = (scopeValue = scope, target = "account", value: unknown = body) => JSON.stringify({ scope: scopeValue, target, body: value });

function memoryStorage() {
  const values = new Map<string, string>();
  return {
    values,
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { values.set(key, value); }),
    removeItem: vi.fn((key: string) => { values.delete(key); }),
  };
}

const decode = (value: unknown) => {
  if (!value || typeof value !== "object" || !Number.isSafeInteger((value as { version?: unknown }).version)) throw Error("invalid fixture response");
  return value as { version: number };
};

const ok = (value: unknown = { version: 1 }) => new Response(JSON.stringify(value), { status: 200 });
const rejected = (status: number) => new Response(JSON.stringify({ error: { code: status === 403 ? "forbidden" : "version_conflict", message: "original server diagnostic" } }), { status });

beforeEach(() => { vi.stubGlobal("fetch", vi.fn(async () => ok())); });
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

describe("durable locale preference recovery", () => {
  it("persists the exact non-secret operation before the actual PATCH and clears only after valid decoding", async () => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    vi.stubGlobal("fetch", vi.fn(async (path: string, init: RequestInit) => {
      const sent = JSON.parse(String(init.body));
      expect(path).toBe("/api/v1/account/preferences");
      expect(init.method).toBe("PATCH");
      expect(init.credentials).toBe("same-origin");
      expect(new Headers(init.headers).get("X-Suma-Expected-User")).toBe("user-1");
      expect(new Headers(init.headers).has("X-Suma-Expected-Organization")).toBe(false);
      expect(Object.keys(sent).sort()).toEqual(["expected_version", "idempotency_key", "locale"]);
      expect(sent).toMatchObject({ locale: "", expected_version: 0 });
      expect(sent.idempotency_key).toMatch(/^[\x20-\x7e]{8,128}$/);
      expect(JSON.parse(storage.getItem(prefix + scope)!)).toEqual({ scope, target: "account", body: sent });
      return ok();
    }));
    const result = await recovery.run("", 0, (value) => {
      expect(recovery.pending).toMatchObject({ locale: "", expected_version: 0 });
      expect(storage.getItem(prefix + scope)).not.toBeNull();
      return decode(value);
    });
    expect(result).toEqual({ version: 1 });
    expect(recovery.pending).toBeUndefined();
    expect(storage.getItem(prefix + scope)).toBeNull();
  });

  it("derives the organization endpoint without sending identity fields", async () => {
    const recovery = new LocalePreferenceRecovery("organization:user-1:org-1", "organization", memoryStorage());
    await recovery.run("zh-CN", 2, decode);
    const [path, init] = vi.mocked(fetch).mock.calls[0];
    expect(path).toBe("/api/v1/organization/preferences");
    expect(new Headers(init?.headers).get("X-Suma-Expected-User")).toBe("user-1");
    expect(new Headers(init?.headers).get("X-Suma-Expected-Organization")).toBe("org-1");
    expect(JSON.parse(String(init?.body))).toMatchObject({ locale: "zh-CN", expected_version: 2 });
    expect(String(init?.body)).not.toMatch(/user_id|organization_id|org-1/);
  });

  it("reloads after response loss and restores the same full body and operation", async () => {
    const storage = memoryStorage();
    vi.mocked(fetch).mockRejectedValueOnce(new TypeError("response lost"));
    const first = new LocalePreferenceRecovery(scope, "account", storage);
    await expect(first.run("en-US", 0, decode)).rejects.toMatchObject({ status: 0 });
    const original = first.pending;
    const saved = storage.getItem(prefix + scope);
    const reloaded = new LocalePreferenceRecovery(scope, "account", storage);
    expect(reloaded.pending).toEqual(original);
    await expect(reloaded.run("zh-CN", 7, decode)).rejects.toMatchObject({ ui_key: "preferences.pending" });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(storage.getItem(prefix + scope)).toBe(saved);
    await reloaded.restore(decode);
    const secondBody = JSON.parse(String(vi.mocked(fetch).mock.calls[1][1]?.body));
    expect(secondBody).toEqual(original);
    expect(new Headers(vi.mocked(fetch).mock.calls[1][1]?.headers).get("X-Suma-Expected-User")).toBe("user-1");
    expect(reloaded.pending).toBeUndefined();
  });

  it.each([400, 401, 403, 404, 409, 422, 429])("clears a first determinate %i but keeps the server error", async (status) => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    vi.mocked(fetch).mockResolvedValueOnce(rejected(status));
    await expect(recovery.run("en-US", 0, decode)).rejects.toMatchObject({ status, message: "original server diagnostic" });
    expect(recovery.pending).toBeUndefined();
    expect(storage.getItem(prefix + scope)).toBeNull();
  });

  it.each([401, 403, 409, 422])("retains an original operation when restoring gets current %i", async (status) => {
    const storage = memoryStorage();
    storage.values.set(prefix + scope, stored());
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    vi.mocked(fetch).mockResolvedValueOnce(rejected(status));
    await expect(recovery.restore(decode)).rejects.toBeInstanceOf(ApiError);
    expect(recovery.pending).toEqual(body);
    expect(storage.getItem(prefix + scope)).toBe(stored());
    expect(JSON.parse(String(vi.mocked(fetch).mock.calls[0][1]?.body))).toEqual(body);
  });

  it.each([500, 502, 503])("retains a new operation after an uncertain %i", async (status) => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    vi.mocked(fetch).mockResolvedValueOnce(rejected(status));
    await expect(recovery.run("en-US", 0, decode)).rejects.toMatchObject({ status });
    expect(recovery.pending).toBeDefined();
    expect(storage.getItem(prefix + scope)).not.toBeNull();
  });

  it("keeps an invalid successful response pending until the original operation decodes", async () => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    vi.mocked(fetch).mockResolvedValueOnce(ok({ locale: "en-US" }));
    await expect(recovery.run("en-US", 0, decode)).rejects.toMatchObject({ ui_key: "preferences.protocol" });
    const original = recovery.pending;
    expect(storage.getItem(prefix + scope)).not.toBeNull();
    await recovery.restore(decode);
    expect(JSON.parse(String(vi.mocked(fetch).mock.calls[1][1]?.body))).toEqual(original);
    expect(recovery.pending).toBeUndefined();
  });

  it("retains an unparseable HTTP200 response and prevents duplicate clicks synchronously", async () => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    let finish!: (value: Response) => void;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
    const first = recovery.run("en-US", 0, decode);
    await expect(recovery.run("zh-CN", 0, decode)).rejects.toMatchObject({ ui_key: "preferences.pending" });
    await expect(recovery.restore(decode)).rejects.toMatchObject({ ui_key: "preferences.pending" });
    expect(fetch).toHaveBeenCalledTimes(1);
    finish(new Response("{bad", { status: 200 }));
    await expect(first).rejects.toMatchObject({ status: 0, code: "response_unknown" });
    expect(recovery.pending).toBeDefined();
  });

  it("uses browser localStorage when no storage is supplied", async () => {
    const storage = memoryStorage();
    vi.stubGlobal("localStorage", storage);
    await new LocalePreferenceRecovery(scope, "account").run("en-US", 0, decode);
    expect(storage.setItem).toHaveBeenCalledOnce();
  });

  it.each(["missing", "get", "set", "silently_ignored_set"])("fails before sending when storage is %s", async (failure) => {
    const memory = memoryStorage();
    let storage: StorageLike | undefined = memory;
    if (failure === "missing") { storage = undefined; vi.stubGlobal("localStorage", undefined); }
    if (failure === "get") memory.getItem.mockImplementation(() => { throw Error("blocked"); });
    if (failure === "set") memory.setItem.mockImplementation(() => { throw Error("quota"); });
    if (failure === "silently_ignored_set") memory.setItem.mockImplementation(() => {});
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    await expect(recovery.run("en-US", 0, decode)).rejects.toMatchObject({ ui_key: "preferences.unavailable" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("keeps confirmed identity if durable removal fails", async () => {
    const storage = memoryStorage();
    storage.removeItem.mockImplementation(() => { throw Error("blocked remove"); });
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    await expect(recovery.run("en-US", 0, decode)).resolves.toEqual({ version: 1 });
    expect(recovery.pending).toBeDefined();
    await expect(recovery.run("zh-CN", 1, decode)).rejects.toMatchObject({ ui_key: "preferences.pending" });
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("returns a detached pending body and never overwrites another instance's operation", async () => {
    const storage = memoryStorage();
    const earlier = new LocalePreferenceRecovery(scope, "account", storage);
    storage.values.set(prefix + scope, stored());
    await expect(earlier.run("zh-CN", 9, decode)).rejects.toMatchObject({ ui_key: "preferences.pending" });
    const exposed = earlier.pending!;
    exposed.locale = "zh-CN";
    expect(earlier.pending).toEqual(body);
    expect(storage.getItem(prefix + scope)).toBe(stored());
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each([
    ["alias", "en", 0], ["null_locale", null, 0], ["negative", "en-US", -1],
    ["fractional", "en-US", 0.5], ["unsafe", "en-US", 2 ** 53], ["overflow", "en-US", 2147483647],
  ])("rejects new invalid %s before persistence", async (_name, locale, version) => {
    const storage = memoryStorage();
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    await expect(recovery.run(locale as string, version as number, decode)).rejects.toBeInstanceOf(UiError);
    expect(fetch).not.toHaveBeenCalled();
    expect(storage.setItem).not.toHaveBeenCalled();
  });

  it.each([
    ["bad_json", "{"], ["null", "null"], ["array", "[]"], ["empty", ""],
    ["other_account", stored("account:foreign")], ["other_target", stored(scope, "organization")],
    ["other_organization", stored("organization:user-1:other-org", "organization")],
    ["injected_endpoint", JSON.stringify({ scope, target: "account", body, path: "/auth/logout" })],
    ["injected_secret", stored(scope, "account", { ...body, token: "never-send-me" })],
    ["missing_locale", stored(scope, "account", { expected_version: 0, idempotency_key: body.idempotency_key })],
    ["null_locale", stored(scope, "account", { ...body, locale: null })],
    ["alias_locale", stored(scope, "account", { ...body, locale: "en" })],
    ["fractional_version", stored(scope, "account", { ...body, expected_version: 0.5 })],
    ["string_version", stored(scope, "account", { ...body, expected_version: "0" })],
    ["null_version", stored(scope, "account", { ...body, expected_version: null })],
    ["unsafe_version", stored(scope, "account", { ...body, expected_version: 2 ** 53 })],
    ["overflow_version", stored(scope, "account", { ...body, expected_version: 2147483647 })],
    ["control_key", stored(scope, "account", { ...body, idempotency_key: "original\noperation" })],
    ["unicode_key", stored(scope, "account", { ...body, idempotency_key: "original-中文-operation" })],
    ["short_key", stored(scope, "account", { ...body, idempotency_key: "short" })],
    ["duplicate_field", `{"scope":"${scope}","target":"account","body":{"locale":"zh-CN","locale":"en-US","expected_version":0,"idempotency_key":"original-operation-123"}}`],
    ["too_large", " ".repeat(4097)],
    ["prototype", stored(scope, "account", JSON.parse(`{"locale":"en-US","expected_version":0,"idempotency_key":"original-operation-123","__proto__":{"token":"never"}}`))],
  ])("fails closed without modifying corrupt %s", async (_name, raw) => {
    const storage = memoryStorage();
    storage.values.set(prefix + scope, raw);
    const recovery = new LocalePreferenceRecovery(scope, "account", storage);
    expect(recovery.error).toMatchObject({ ui_key: "preferences.corrupt" });
    await expect(recovery.run("zh-CN", 0, decode)).rejects.toMatchObject({ ui_key: "preferences.corrupt" });
    await expect(recovery.restore(decode)).rejects.toMatchObject({ ui_key: "preferences.corrupt" });
    expect(storage.getItem(prefix + scope)).toBe(raw);
    expect(storage.setItem).not.toHaveBeenCalled();
    expect(storage.removeItem).not.toHaveBeenCalled();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("isolates accounts and organizations even when another scope has pending work", async () => {
    const storage = memoryStorage();
    storage.values.set(prefix + scope, stored());
    const second = new LocalePreferenceRecovery("account:user-2", "account", storage);
    await second.run("zh-CN", 0, decode);
    expect(storage.getItem(prefix + scope)).toBe(stored());
    const orgScope = "organization:user-1:org-2";
    await new LocalePreferenceRecovery(orgScope, "organization", storage).run("", 0, decode);
    expect(storage.getItem(prefix + scope)).toBe(stored());
  });
});
