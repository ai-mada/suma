import { api, ApiError, operation } from "../api";
import { UiError } from "./index";

type PreferenceBody = {
  locale: "" | "zh-CN" | "en-US";
  expected_version: number;
  idempotency_key: string;
};
type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;
type Target = "account" | "organization";
type Envelope = { scope: string; target: Target; body: PreferenceBody };

function exactObject(value: unknown, keys: string[]): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value) &&
    Object.keys(value).length === keys.length &&
    keys.every((key) => Object.prototype.hasOwnProperty.call(value, key));
}

function validBody(value: unknown): value is PreferenceBody {
  if (!exactObject(value, ["locale", "expected_version", "idempotency_key"])) return false;
  return (value.locale === "" || value.locale === "zh-CN" || value.locale === "en-US") &&
    typeof value.expected_version === "number" && Number.isSafeInteger(value.expected_version) &&
    value.expected_version >= 0 && value.expected_version <= 2147483646 &&
    typeof value.idempotency_key === "string" && /^[\x20-\x7e]{8,128}$/.test(value.idempotency_key) &&
    value.idempotency_key.trim() !== "";
}

function validScope(scope: string, target: Target) {
  if (typeof scope !== "string" || (target !== "account" && target !== "organization")) return false;
  const parts = scope.split(":");
  return parts[0] === target && parts.length === (target === "account" ? 2 : 3) &&
    parts.slice(1).every((part) => /^[A-Za-z0-9_.-]{1,128}$/.test(part));
}

const copyBody = (body: PreferenceBody): PreferenceBody => ({
  locale: body.locale, expected_version: body.expected_version, idempotency_key: body.idempotency_key,
});
const sameBody = (left: PreferenceBody, right: PreferenceBody) =>
  left.locale === right.locale && left.expected_version === right.expected_version && left.idempotency_key === right.idempotency_key;

/** Persist only the fixed non-secret preference envelope before HTTP. Recovery
 * always asks the server to recheck current identity and role with that original
 * operation; a current rejection cannot prove a previous request never committed. */
export class LocalePreferenceRecovery {
  private value?: PreferenceBody;
  private problem?: UiError;
  private busy = false;
  private readonly key: string;
  private readonly storage?: StorageLike;

  constructor(private readonly scope: string, private readonly target: Target, storage?: StorageLike) {
    this.key = "suma-locale-recovery-v1:" + scope;
    if (!validScope(scope, target)) {
      this.problem = new UiError("preferences.corrupt");
      return;
    }
    try {
      this.storage = storage ?? globalThis.localStorage;
      this.value = this.readStored();
    } catch (error) {
      this.problem = error instanceof UiError ? error : new UiError("preferences.unavailable");
    }
  }

  get pending(): PreferenceBody | undefined { return this.value ? copyBody(this.value) : undefined; }
  get error(): UiError | undefined { return this.problem; }

  run<T>(locale: string, expectedVersion: number, decode: (value: unknown) => T): Promise<T> {
    return this.execute(false, locale, expectedVersion, decode);
  }

  restore<T>(decode: (value: unknown) => T): Promise<T> {
    return this.execute(true, "", 0, decode);
  }

  private fail(key: "preferences.corrupt" | "preferences.unavailable"): never {
    this.problem = new UiError(key);
    throw this.problem;
  }

  private readStored(): PreferenceBody | undefined {
    let raw: string | null;
    try {
      if (!this.storage) return this.fail("preferences.unavailable");
      raw = this.storage.getItem(this.key);
    } catch { return this.fail("preferences.unavailable"); }
    if (raw === null) return;
    try {
      if (new TextEncoder().encode(raw).length > 4096) return this.fail("preferences.corrupt");
      const item: unknown = JSON.parse(raw);
      // JSON.parse alone discards duplicate properties. This deliberately small
      // envelope has six distinct property names, so repeated decoded key tokens
      // can be rejected without accepting arbitrary paths or nested payloads.
      const seen = new Set<string>();
      for (const match of raw.matchAll(/"(?:\\.|[^"\\])*"/g)) {
        if (!/^\s*:/.test(raw.slice(match.index + match[0].length))) continue;
        const key = JSON.parse(match[0]) as string;
        if (seen.has(key)) return this.fail("preferences.corrupt");
        seen.add(key);
      }
      if (!exactObject(item, ["scope", "target", "body"]) || item.scope !== this.scope || item.target !== this.target || !validBody(item.body)) return this.fail("preferences.corrupt");
      return copyBody(item.body);
    } catch { return this.fail("preferences.corrupt"); }
  }

  private persist(body: PreferenceBody) {
    const envelope: Envelope = { scope: this.scope, target: this.target, body: copyBody(body) };
    const encoded = JSON.stringify(envelope);
    try {
      if (!this.storage || new TextEncoder().encode(encoded).length > 4096) return this.fail("preferences.unavailable");
      this.storage.setItem(this.key, encoded);
      // Some storage shims silently ignore writes. Never send until the fixed
      // identity can be read back, including after an explicit reload/restore.
      if (this.storage.getItem(this.key) !== encoded) return this.fail("preferences.unavailable");
    } catch { return this.fail("preferences.unavailable"); }
  }

  private clear(body: PreferenceBody) {
    try {
      const stored = this.readStored();
      if (stored && !sameBody(stored, body)) return this.fail("preferences.corrupt");
      this.storage!.removeItem(this.key);
      if (this.storage!.getItem(this.key) === null) this.value = undefined;
    } catch {
      // A confirmed response remains useful even if local cleanup is blocked.
      // Retaining the original identity prevents a later draft overwriting it.
    }
  }

  private async execute<T>(restoring: boolean, locale: string, expectedVersion: number, decode: (value: unknown) => T): Promise<T> {
    if (this.problem) throw this.problem;
    if (this.busy) throw new UiError("preferences.pending");
    this.busy = true;
    try {
      const stored = this.readStored();
      if (stored && this.value && !sameBody(stored, this.value)) return this.fail("preferences.corrupt");
      if (stored) this.value = stored;
      if (restoring) {
        if (!this.value) throw new UiError("preferences.pending");
        if (!stored) this.persist(this.value);
      } else {
        if (this.value) throw new UiError("preferences.pending");
        const candidate = { locale, expected_version: expectedVersion, idempotency_key: operation() };
        if (!validBody(candidate)) throw new UiError("preferences.protocol");
        this.value = copyBody(candidate);
        this.persist(this.value);
      }
      const request = copyBody(this.value!);
      let response: unknown;
      try {
        const scope = this.scope.split(":");
        response = await api<unknown>("/" + this.target + "/preferences", request, "PATCH", {
          "X-Suma-Expected-User": scope[1],
          ...(this.target === "organization" ? { "X-Suma-Expected-Organization": scope[2] } : {}),
        });
      } catch (error) {
        if (!restoring && error instanceof ApiError && error.status >= 400 && error.status < 500) this.clear(request);
        throw error;
      }
      let result: T;
      try { result = decode(response); }
      catch { throw new UiError("preferences.protocol"); }
      this.clear(request);
      return result;
    } finally { this.busy = false; }
  }
}
