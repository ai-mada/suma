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
  if (!preview && !commit) throw Error("不属于执行调度策略写入范围。");
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
    throw Error("策略恢复范围或预期版本无效。");
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
      throw Error("策略输入无效。");
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
        throw Error("策略输入必须使用正式安全整数或明确 null。");
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
        "调度恢复记录无法读取，未发送新请求。请保留原标识核对。";
    }
  }
  get pending() {
    return this.value ? copy(this.value) : undefined;
  }
  get error() {
    return this.initialError;
  }
  async restore<T = unknown>(): Promise<T> {
    if (!this.value) throw Error("没有待恢复的策略操作。");
    return this.run<T>(this.value.path, this.value.body);
  }
  async run<T = unknown>(
    path: string,
    body: Record<string, unknown>,
    method: "POST" | "PATCH" = "POST",
  ): Promise<T> {
    if (this.initialError) throw Error(this.initialError);
    if (this.busy) throw Error("原操作仍在等待回执。");
    if (method !== "POST") throw Error("策略只允许正式 POST 命令。");
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
        throw Error("无法保存原操作，未发送请求。请启用会话存储。");
      }
      this.value = request;
    }
    const request = this.value;
    this.busy = true;
    try {
      const value = await api<T>(
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
