import {validCodexSelection} from './codex-execution-model';
import { tr as t, UiError, renderDiagnostic } from "./i18n";
import { api, ApiError, operation } from "./api";
export type ToolPendingRequest = {
  path: string;
  body: Record<string, unknown>;
  method: "POST" | "PATCH";
  idempotency_key: string;
};
type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const allowedPath =
  /^\/(?:tool-profiles(?:\/[A-Za-z0-9_.-]+\/commands)?|release-workers(?:\/[A-Za-z0-9_.-]+)?|task-dispatch-plans(?:\/[A-Za-z0-9_.-]+\/start)?)$/;
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value));
function validate(path: string, body: unknown) {
  if (!allowedPath.test(path))
    throw new UiError("development.errors.operation_scope");
  if (path === '/task-dispatch-plans' && body && typeof body === 'object' && Object.hasOwn(body,'codex_execution_selection') && !validCodexSelection((body as Record<string,unknown>).codex_execution_selection)) throw new UiError("development.errors.request_invalid");
  const inspect = (value: unknown, parents: string[] = []) => {
    if (!value || typeof value !== "object") return;
    for (const [key, item] of Object.entries(value)) {
      // Token allowances are public numbers, not authentication tokens. Keep
      // this exception tied to the exact typed selection path and request.
      const modelNumber = path === '/task-dispatch-plans' &&
        typeof item === 'number' && Number.isSafeInteger(item) &&
        ((parents.join('.') === 'model_execution_selection.parameters' && key === 'max_output_tokens' && item > 0 && item <= 131072) ||
         (parents.join('.') === 'model_execution_selection.budget' && key === 'token_limit' && item >= 0) ||
         (parents.join('.') === 'codex_execution_selection' && key === 'token_notice_limit' && item > 0));
      if (
        !modelNumber &&
        /password|token|secret|credential|authorization|executable$|^args$|^env$/i.test(
          key,
        )
      )
        throw new UiError("development.errors.recovery_secrets");
      inspect(item, [...parents, key]);
    }
  };
  inspect(body);
  if (!body || typeof body !== "object" || Array.isArray(body))
    throw new UiError("development.errors.request_invalid");
}
export class ToolWriteRecovery {
  private value?: ToolPendingRequest;
  private initialError = "";
  private busy = false;
  private key: string;
  constructor(
    scope: string,
    private storage: StorageLike | undefined,
    private send: typeof api = api,
    private validateResponse?: (value: unknown, request: ToolPendingRequest) => boolean | Promise<boolean>,
  ) {
    this.key = "suma-tool-recovery-v1:" + scope;
    try {
      const raw = storage?.getItem(this.key);
      if (raw) {
        const value = JSON.parse(raw) as ToolPendingRequest;
        validate(value.path, value.body);
        if (!value.idempotency_key || !["POST", "PATCH"].includes(value.method))
          throw Error("invalid");
        this.value = value;
      }
    } catch {
      this.initialError =
        t("development.errors.tool_recovery_unreadable");
    }
  }
  get pending() {
    return this.value ? copy(this.value) : undefined;
  }
  get error() {
    return renderDiagnostic(this.initialError);
  }
  async restore<T = unknown>(): Promise<T> {
    if (!this.value) throw new UiError("development.errors.no_tool_pending");
    return this.run<T>(this.value.path, this.value.body, this.value.method);
  }
  async run<T = unknown>(
    path: string,
    body: Record<string, unknown>,
    method: "POST" | "PATCH" = "POST",
  ): Promise<T> {
    if (this.initialError) throw new Error(this.initialError);
    if (this.busy) throw new UiError("development.errors.tool_receipt_pending");
    const restoring = !!this.value;
    if (!this.value) {
      validate(path, body);
      const request = {
        path,
        body: copy(body),
        method,
        idempotency_key: operation(),
      };
      try {
        if (!this.storage) throw Error("unavailable");
        this.storage.setItem(this.key, JSON.stringify(request));
      } catch {
        throw new UiError("development.errors.tool_storage_unavailable");
      }
      this.value = request;
    }
    const request = this.value;
    this.busy = true;
    try {
      const result = await this.send<T>(
        request.path,
        { ...request.body, idempotency_key: request.idempotency_key },
        request.method,
      );
      if (this.validateResponse) {
        let accepted = false;
        try { accepted = await this.validateResponse(result, copy(request)); } catch { /* The effect is still uncertain. */ }
        if (!accepted) throw new UiError("codex-execution.invalid_response");
      }
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
      /* Original identity remains safe to replay after refresh. */
    }
  }
}
