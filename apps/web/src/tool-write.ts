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
    throw new Error("此操作不属于工具工作台可恢复写入范围。");
  const inspect = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    for (const [key, item] of Object.entries(value)) {
      if (
        /password|token|secret|credential|authorization|executable$|^args$|^env$/i.test(
          key,
        )
      )
        throw new Error("工具恢复记录不能包含秘密、凭证或本地执行配置。");
      inspect(item);
    }
  };
  inspect(body);
  if (!body || typeof body !== "object" || Array.isArray(body))
    throw new Error("恢复请求格式无效。");
}
export class ToolWriteRecovery {
  private value?: ToolPendingRequest;
  private initialError = "";
  private busy = false;
  private key: string;
  constructor(
    scope: string,
    private storage: StorageLike | undefined,
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
        "工具恢复记录无法读取。请保留当前页面核对，未发送新请求。";
    }
  }
  get pending() {
    return this.value ? copy(this.value) : undefined;
  }
  get error() {
    return this.initialError;
  }
  async restore<T = unknown>(): Promise<T> {
    if (!this.value) throw new Error("没有需要恢复的工具操作。");
    return this.run<T>(this.value.path, this.value.body, this.value.method);
  }
  async run<T = unknown>(
    path: string,
    body: Record<string, unknown>,
    method: "POST" | "PATCH" = "POST",
  ): Promise<T> {
    if (this.initialError) throw new Error(this.initialError);
    if (this.busy) throw new Error("原工具操作仍在等待回执，请勿重复提交。");
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
        throw new Error(
          "无法保存本页恢复记录，未发送请求。请启用会话存储后重试。",
        );
      }
      this.value = request;
    }
    const request = this.value;
    this.busy = true;
    try {
      const result = await api<T>(
        request.path,
        { ...request.body, idempotency_key: request.idempotency_key },
        request.method,
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
      /* Original identity remains safe to replay after refresh. */
    }
  }
}
