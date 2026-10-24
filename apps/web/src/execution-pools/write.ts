import { tr, useLocale, UiError } from "../i18n";
import { api, ApiError, operation } from "../api";
import type { ToolPendingRequest } from "../tool-write";
type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value));
const paths =
  /^\/(?:execution-pools(?:\/[a-f0-9]{32}(?:\/commands|\/grants\/[a-f0-9]{32}\/commands|\/runners(?:\/[a-f0-9]{32}\/(?:credentials|commands))?|\/policy-previews(?:\/[a-f0-9]{32}\/commit)?)?)?|project-execution\/(?:grants(?:\/[a-f0-9]{32}\/commands)?|route-previews(?:\/[a-f0-9]{32}\/commit)?))$/;
function validate(
  path: string,
  body: Record<string, unknown>,
  project?: string,
) {
  if (!paths.test(path))
    throw new UiError("pools.recovery.operation_out_of_scope");
  if (
    path.startsWith("/project-execution/") &&
    (!project || body.project_id !== project)
  )
    throw new UiError("pools.recovery.project_scope_mismatch");
  const inspect = (item: unknown) => {
    if (!item || typeof item !== "object") return;
    for (const [key, value] of Object.entries(item)) {
      if (/secret|password|token|authorization|^credential$/i.test(key))
        throw new UiError("pools.recovery.secrets_forbidden");
      inspect(value);
    }
  };
  inspect(body);
  if (!body || Array.isArray(body) || typeof body !== "object")
    throw new UiError("pools.recovery.input_invalid");
  if (
    "expected_version" in body &&
    (!Number.isSafeInteger(body.expected_version) ||
      Number(body.expected_version) <
        (path === "/project-execution/route-previews" ? 0 : 1))
  )
    throw new UiError("pools.recovery.version_invalid");
}
/** Persist only the non-secret command envelope before sending. A lost registration
 * response must replay this identity; its token is never put in a receipt/store. */
export class PoolWriteRecovery {
  private value?: ToolPendingRequest;
  private initialError = "";
  private busy = false;
  private key: string;
  constructor(
    scope: string,
    private storage: StorageLike | undefined,
    private project?: string,
  ) {
    this.key = "suma-pool-recovery-v1:" + scope;
    try {
      const raw = storage?.getItem(this.key);
      if (raw) {
        const item = JSON.parse(raw) as ToolPendingRequest;
        validate(item.path, item.body, project);
        if (
          item.method !== "POST" ||
          typeof item.idempotency_key !== "string" ||
          !/^[A-Za-z0-9_.:-]{8,128}$/.test(item.idempotency_key)
        )
          throw Error("invalid");
        this.value = item;
      }
    } catch {
      this.initialError = tr("pools.recovery.record_unreadable");
    }
  }
  get pending() {
    return this.value ? clone(this.value) : undefined;
  }
  get error() {
    return this.initialError;
  }
  async restore<T>(): Promise<T> {
    if (!this.value) throw new UiError("pools.recovery.no_pending");
    return this.run<T>(this.value.path, this.value.body);
  }
  async run<T>(
    path: string,
    body: Record<string, unknown>,
    method: "POST" | "PATCH" = "POST",
  ): Promise<T> {
    if (this.initialError) throw Error(this.initialError);
    if (this.busy) throw new UiError("pools.recovery.original_pending");
    if (method !== "POST") throw new UiError("pools.recovery.post_only");
    const restoring = !!this.value;
    if (!this.value) {
      validate(path, body, this.project);
      const item: ToolPendingRequest = {
        path,
        body: clone(body),
        method: "POST",
        idempotency_key: operation(),
      };
      try {
        if (!this.storage) throw Error("unavailable");
        this.storage.setItem(this.key, JSON.stringify(item));
      } catch {
        throw new UiError("pools.recovery.record_unsaved");
      }
      this.value = item;
    }
    const request = this.value;
    this.busy = true;
    try {
      const result = await api<T>(
        request.path,
        { ...request.body, idempotency_key: request.idempotency_key },
        "POST",
      );
      this.clear();
      return result;
    } catch (error) {
      if (
        !restoring &&
        error instanceof ApiError &&
        error.status > 0 &&
        error.status < 500
      )
        this.clear();
      throw error;
    } finally {
      this.busy = false;
    }
  }
  private clear() {
    this.value = undefined;
    try {
      this.storage?.removeItem(this.key);
    } catch {
      /* If removal fails, refresh may safely replay the confirmed original command. */
    }
  }
}
