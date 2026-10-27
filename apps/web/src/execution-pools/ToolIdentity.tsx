import { tr, useLocale } from "../i18n";
import { Trans } from "../i18n/Trans";
import type { ProjectPoolRunnerPage } from "../../../../packages/contracts/src";
import type { ExecutionWorker } from "../tool-model";
import type { useToolRead } from "../tool-hooks";
import { ErrorNotice, Field } from "../ui";
import { time } from "../api";
export type IdentityRead = ReturnType<
  typeof useToolRead<{ items: ExecutionWorker[] }>
>;
export type SharedIdentityRead = ReturnType<
  typeof useToolRead<ProjectPoolRunnerPage>
>;
/** A project-authorized shared identity is a read-only option, not a private
 * credential owned by this project. Never merge it into revoke/admin records. */
export function currentToolIdentity(
  worker: string,
  local: IdentityRead,
  shared: SharedIdentityRead,
) {
  return (
    (!local.loading &&
      !local.error &&
      local.data?.items.some(
        (item) => item.worker_id === worker && item.status === "active",
      )) ||
    (!shared.loading &&
      !shared.error &&
      shared.data?.items.some(
        (item) => item.worker_id === worker && item.status === "active",
      ))
  );
}
export function ToolIdentity({
  local,
  shared,
  value,
  onChange,
}: {
  local: IdentityRead;
  shared: SharedIdentityRead;
  value: string;
  onChange: (worker: string) => void;
}) {
  useLocale();
  const localReady = !local.loading && !local.error,
    poolReady = !shared.loading && !shared.error,
    pool = poolReady
      ? shared.data?.items.find((item) => item.worker_id === value)
      : undefined;
  return (
    <>
      {local.error && (
        <ErrorNotice error={local.error} onRetry={() => void local.read()} />
      )}{" "}
      {shared.error && (
        <ErrorNotice error={shared.error} onRetry={() => void shared.read()} />
      )}{" "}
      {shared.loading && <p role="status">{tr("pools.identity.loading")}</p>}
      <Field label={tr("pools.identity.bind_registered")}>
        <select
          required
          value={value}
          onChange={(event) => onChange(event.target.value)}
        >
          <option value="">{tr("pools.identity.select")}</option>
          <optgroup label={tr("pools.identity.local_group")}>
            {localReady &&
              local.data?.items
                .filter((item) => item.status === "active")
                .map((item) => (
                  <option key={item.id} value={item.worker_id}>
                    {item.worker_id}
                  </option>
                ))}
          </optgroup>
          <optgroup label={tr("pools.identity.shared_group")}>
            {poolReady &&
              shared.data?.items
                .filter((item) => item.status === "active")
                .map((item) => (
                  <option key={item.id} value={item.worker_id}>
                    <Trans
                      i18nKey="pools.identity.shared_option"
                      values={{ value1: item.name, value2: item.worker_id }}
                    />
                  </option>
                ))}
          </optgroup>
        </select>
      </Field>
      {pool && (
        <p className="pool-scope">
          <Trans
            i18nKey="pools.identity.source_and_expiry"
            values={{
              value1: pool.pool_id,
              value2: time(pool.credential_expires_at),
            }}
            components={{ code: <code /> }}
          />
        </p>
      )}
      {poolReady && shared.data && !shared.data.items.length && (
        <p className="muted">{tr("pools.identity.no_matching")}</p>
      )}
      {shared.data && (
        <small>
          <Trans
            i18nKey="pools.identity.read_and_recheck"
            values={{ value1: time(shared.data.as_of) }}
          />
        </small>
      )}
      <div>
        <button
          type="button"
          disabled={shared.loading}
          onClick={() => void shared.read()}
        >
          {tr("pools.identity.reload")}
        </button>
      </div>
    </>
  );
}
