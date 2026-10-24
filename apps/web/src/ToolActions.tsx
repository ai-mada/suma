import { ErrorNotice } from "./ui";
import type { ToolAction } from "./tool-hooks";
export function ToolRecoveryNotice({
  action,
  onRestored,
}: {
  action: ToolAction;
  onRestored: (path: string, value: unknown) => void;
}) {
  return (
    <>
      {action.error && <ErrorNotice error={action.error} />}{" "}
      {action.pending && (
        <section className="panel tool-notice tool-recovery">
          <strong>原工具操作的结果尚未确认</strong>
          <p>
            本页已保存完整非秘密请求、原版本及操作标识。恢复会重复同一请求，不会根据后台新版本重写它。
          </p>
          <code>{action.pending.idempotency_key}</code>
          <div className="inline wrap">
            <button
              className="primary"
              disabled={action.busy}
              onClick={async () => {
                const original = action.pending;
                if (!original) return;
                const value = await action.restore();
                if (value) onRestored(original.path, value);
              }}
            >
              {action.busy ? "正在核对…" : "恢复原操作回执"}
            </button>
            <button disabled={action.busy} onClick={action.defer}>
              保留标识，稍后核对
            </button>
          </div>
        </section>
      )}
    </>
  );
}
