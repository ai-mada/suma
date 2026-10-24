import { tr as t, useLocale } from "./i18n";
import { ErrorNotice } from "./ui";
import type { ToolAction } from "./tool-hooks";
export function ToolRecoveryNotice({
  action,
  onRestored,
}: {
  action: ToolAction;
  onRestored: (path: string, value: unknown) => void;
}) { useLocale();
  return (
    <>
      {action.error && <ErrorNotice error={action.error} />}{" "}
      {action.pending && (
        <section className="panel tool-notice tool-recovery">
          <strong>{t("development.recovery.the_original_tool_operation_s_result_is")}</strong>
          <p>
            {t("development.recovery.the_complete_non_secret_request_original_version")}</p>
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
              {action.busy ? t("development.recovery.reconciling") : t("development.recovery.recover_original_operation_receipt")}
            </button>
            <button disabled={action.busy} onClick={action.defer}>
              {t("development.recovery.keep_id_reconcile_later")}</button>
          </div>
        </section>
      )}
    </>
  );
}
