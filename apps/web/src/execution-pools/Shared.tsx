import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { ErrorNotice, Loading } from "../ui";
import type { ToolAction } from "../tool-hooks";
import type { ReactNode } from "react";
export function PoolRecovery({
  action,
  onRestored,
}: {
  action: ToolAction;
  onRestored: (path: string, value: unknown) => void;
}) {
  useLocale();
  return (
    <>
      {action.error && <ErrorNotice error={action.error} />}{" "}
      {action.pending && (
        <section
          className="pool-recovery"
          aria-label={tr("pools.recovery.title")}
        >
          <strong>{tr("pools.recovery.original_unknown")}</strong>
          <p>{tr("pools.recovery.restore_notice")}</p>
          <code>{action.pending.idempotency_key}</code>
          <div className="inline wrap">
            <button
              disabled={action.busy}
              onClick={async () => {
                const request = action.pending;
                if (!request) return;
                const value = await action.restore();
                if (value) onRestored(request.path, value);
              }}
            >
              {tr("pools.recovery.restore_receipt")}
            </button>
            <button disabled={action.busy} onClick={action.defer}>
              {tr("pools.recovery.keep_identifier")}
            </button>
          </div>
        </section>
      )}
    </>
  );
}
export function PageState({
  loading,
  error,
  empty,
  children,
  retry,
}: {
  loading: boolean;
  error: string;
  empty: boolean;
  children: ReactNode;
  retry: () => void;
}) {
  useLocale();
  return (
    <>
      {error && (
        <>
          <ErrorNotice error={error} onRetry={retry} />
          <p className="warning">
            {tr("pools.recovery.current_authority_unknown")}
          </p>
        </>
      )}
      {loading && empty ? <Loading /> : children}
    </>
  );
}
export function PageFooter({
  count,
  total,
  next,
  loading,
  more,
}: {
  count: number;
  total: number;
  next: string;
  loading: boolean;
  more: () => void;
}) {
  useLocale();
  return (
    <footer className="pool-page-footer">
      <small>
        <Trans
          i18nKey="pools.list.loaded_counts"
          values={{ value1: count, value2: total }}
        />
      </small>
      {next && (
        <button disabled={loading} onClick={more}>
          {tr("pools.list.more")}
        </button>
      )}
    </footer>
  );
}
export function PoolStatus({ value }: { value: string }) {
  useLocale();
  return (
    <span className={"badge state-" + value}>
      {(
        {
          active: tr("pools.status.active"),
          paused: tr("pools.status.paused"),
          pending: tr("pools.status.pending"),
          approved: tr("pools.status.approved"),
          rejected: tr("pools.status.rejected"),
          revoked: tr("pools.status.revoked"),
          withdrawn: tr("pools.status.withdrawn"),
        } as Record<string, string>
      )[value] || value}
    </span>
  );
}
