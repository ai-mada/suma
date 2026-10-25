import { tr as t, UiError, renderDiagnostic } from "./i18n";
import { api, ApiError, operation } from "./api";
import { policyInput } from "./scheduling-model";
import type { ToolPendingRequest } from "./tool-write";
type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const copy = <T>(value: T): T => JSON.parse(JSON.stringify(value));
function validate(
  path: string,
  body: Record<string, unknown>,
  project?: string,
) {
  const preview = path === "/scheduling/policy-previews",
    commit = /^\/scheduling\/policy-previews\/[A-Za-z0-9_.-]+\/commit$/.test(
      path,
    );
  if (!preview && !commit) throw new UiError("development.errors.policy_scope");
  const fields = preview
    ? ["project_id", "expected_version", "policy"]
    : ["project_id", "expected_version"];
  if (
    Object.keys(body).some((key) => !fields.includes(key)) ||
    typeof body.project_id !== "string" ||
    !body.project_id ||
    (project && body.project_id !== project) ||
    !Number.isSafeInteger(body.expected_version) ||
    Number(body.expected_version) < (commit ? 1 : 0)
  )
    throw new UiError("development.errors.policy_scope_version");
  if (preview) {
    const policy = body.policy as Record<string, unknown>;
    if (
      !policy ||
      Object.keys(policy).some(
        (key) =>
          !["concurrency_limit", "budget_limit", "resource_limits"].includes(
            key,
          ),
      ) ||
      !policy.resource_limits ||
      typeof policy.resource_limits !== "object" ||
      Array.isArray(policy.resource_limits)
    )
      throw new UiError("development.errors.policy_invalid");
    const input = policyInput(
      String(policy.concurrency_limit),
      policy.budget_limit === null ? "" : String(policy.budget_limit),
      Object.entries(policy.resource_limits).map(([name, value]) => ({
        name,
        value: value === null ? "" : String(value),
      })),
    );
    if (JSON.stringify(input) !== JSON.stringify(body.policy)) {
      if (
        input.concurrency_limit !== policy.concurrency_limit ||
        input.budget_limit !== policy.budget_limit ||
        Object.entries(input.resource_limits).some(
          ([key, v]) =>
            v !== (policy.resource_limits as Record<string, unknown>)[key],
        )
      )
        throw new UiError("development.errors.policy_safe_integer");
    }
  }
}
export class SchedulingWriteRecovery {
  private value?: ToolPendingRequest;
  private initialError = "";
  private busy = false;
  private key: string;
  constructor(
    scope: string,
    private storage: StorageLike | undefined,
    private project?: string,
    private send: typeof api = api,
  ) {
    this.key = "suma-scheduling-recovery-v1:" + scope;
    try {
      const raw = storage?.getItem(this.key);
      if (raw) {
        const item = JSON.parse(raw) as ToolPendingRequest;
        validate(item.path, item.body, project);
        if (
          item.method !== "POST" ||
          typeof item.idempotency_key !== "string" ||
          !/^[A-Za-z0-9_.:-]{1,200}$/.test(item.idempotency_key)
        )
          throw Error("invalid");
        this.value = item;
      }
    } catch {
      this.initialError =
        t("development.errors.policy_recovery_unreadable");
    }
  }
  get pending() {
    return this.value ? copy(this.value) : undefined;
  }
  get error() {
    return renderDiagnostic(this.initialError);
  }
  async restore<T = unknown>(): Promise<T> {
    if (!this.value) throw new UiError("development.errors.no_policy_pending");
    return this.run<T>(this.value.path, this.value.body);
  }
  async run<T = unknown>(
    path: string,
    body: Record<string, unknown>,
    method: "POST" | "PATCH" = "POST",
  ): Promise<T> {
    if (this.initialError) throw Error(this.initialError);
    if (this.busy) throw new UiError("development.errors.receipt_pending");
    if (method !== "POST") throw new UiError("development.errors.policy_post_only");
    const restoring = !!this.value;
    if (!this.value) {
      validate(path, body, this.project);
      const request: ToolPendingRequest = {
        path,
        body: copy(body),
        method: "POST",
        idempotency_key: operation(),
      };
      try {
        if (!this.storage) throw Error("unavailable");
        this.storage.setItem(this.key, JSON.stringify(request));
      } catch {
        throw new UiError("development.errors.policy_storage_unavailable");
      }
      this.value = request;
    }
    const request = this.value;
    this.busy = true;
    try {
      const value = await this.send<T>(
        request.path,
        { ...request.body, idempotency_key: request.idempotency_key },
        "POST",
      );
      this.clear();
      return value;
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
      /* Same original identity remains safe to replay. */
    }
  }
}
