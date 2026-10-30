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
  if(path.startsWith('/task-dispatch-plans')&&body&&typeof body==='object'&&['octop_execution','octop_execution_selection','octop','fixed_octop'].some(key=>Object.hasOwn(body,key)))throw new UiError('development.errors.request_invalid');
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
const independentOwner = () => true;
const ownershipError = (key: 'changed' | 'unconfirmed' | 'cleanup_failed') => new UiError('operation-ownership.' + key);
export class ToolWriteRecovery {
  private value?: ToolPendingRequest;
  private raw?: string;
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
      if (!storage) return;
      const raw = storage.getItem(this.key);
      if (raw !== null) {
        const value = JSON.parse(raw) as ToolPendingRequest;
        validate(value.path, value.body);
        if (!value.idempotency_key || !["POST", "PATCH"].includes(value.method)) throw Error("invalid");
        this.value = value;
        this.raw = raw;
      }
    } catch {
      this.initialError = t("development.errors.tool_recovery_unreadable");
    }
  }
  get pending() { return this.value ? copy(this.value) : undefined; }
  get error() { return renderDiagnostic(this.initialError); }
  async restore<T = unknown>(ownsRequest: () => boolean = independentOwner): Promise<T> {
    this.assertOwner(ownsRequest);
    if (!this.value) throw new UiError("development.errors.no_tool_pending");
    return this.run<T>(this.value.path, this.value.body, this.value.method, ownsRequest);
  }
  async run<T = unknown>(path: string, body: Record<string, unknown>, method: "POST" | "PATCH" = "POST", ownsRequest: () => boolean = independentOwner): Promise<T> {
    this.assertOwner(ownsRequest);
    if (this.initialError) throw new Error(this.initialError);
    if (this.busy) throw new UiError("development.errors.tool_receipt_pending");
    const restoring = !!this.value;
    this.busy = true;
    try {
      if (!this.value) {
        if (this.readStored() !== null) throw ownershipError('unconfirmed');
        validate(path, body);
        const request: ToolPendingRequest = { path, body: copy(body), method, idempotency_key: operation() };
        const raw = JSON.stringify(request);
        this.assertOwner(ownsRequest);
        if (this.readStored() !== null) throw ownershipError('unconfirmed');
        try {
          if (!this.storage) throw Error('unavailable');
          this.storage.setItem(this.key, raw);
        } catch {
          // This error is confined to staging, before any transport call.
          throw new UiError("development.errors.tool_storage_unavailable");
        }
        this.value = request;
        this.raw = raw;
      }
      const request = this.value, raw = this.raw;
      if (!request || raw === undefined) throw ownershipError('unconfirmed');
      this.assertRequest(request, raw, ownsRequest);
      let result: T;
      try {
        result = await this.send<T>(request.path, { ...request.body, idempotency_key: request.idempotency_key }, request.method);
      } catch (error) {
        this.assertRequest(request, raw, ownsRequest);
        // Only a fresh transport refusal proves this send did not commit.
        // Validator and journal errors never enter this branch.
        if (!restoring && error instanceof ApiError && error.status >= 400 && error.status < 500) this.clear(request, raw, ownsRequest);
        throw error;
      }
      this.assertRequest(request, raw, ownsRequest);
      if (this.validateResponse) {
        let accepted = false;
        try { accepted = await this.validateResponse(result, copy(request)); } catch { /* The effect is still uncertain. */ }
        this.assertRequest(request, raw, ownsRequest);
        if (!accepted) throw new UiError("codex-execution.invalid_response");
      }
      this.clear(request, raw, ownsRequest);
      return result;
    } finally {
      this.busy = false;
    }
  }
  private assertOwner(ownsRequest: () => boolean) {
    let owned = false;
    try { owned = ownsRequest(); } catch { /* An unprovable owner cannot consume. */ }
    if (!owned) throw ownershipError('changed');
  }
  private readStored(): string | null {
    try {
      if (!this.storage) throw Error('unavailable');
      const raw = this.storage.getItem(this.key);
      if (raw !== null && typeof raw !== 'string') throw Error('invalid');
      return raw;
    } catch { throw ownershipError('unconfirmed'); }
  }
  private assertRequest(request: ToolPendingRequest, raw: string, ownsRequest: () => boolean) {
    this.assertOwner(ownsRequest);
    if (this.value !== request || this.raw !== raw) throw ownershipError('changed');
    if (this.readStored() !== raw) throw ownershipError('unconfirmed');
    this.assertOwner(ownsRequest);
  }
  private clear(request: ToolPendingRequest, raw: string, ownsRequest: () => boolean) {
    this.assertRequest(request, raw, ownsRequest);
    try {
      if (!this.storage) throw Error('unavailable');
      this.storage.removeItem(this.key);
    } catch { throw ownershipError('cleanup_failed'); }
    this.value = undefined;
    this.raw = undefined;
  }
}
