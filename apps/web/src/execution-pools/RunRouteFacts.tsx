import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import type { Run } from "../../../../packages/contracts/src";
import "./pools.css";
type SavedRoute = Pick<
  Run,
  | "id"
  | "execution_pool_id"
  | "execution_route_version"
  | "execution_pool_grant_id"
  | "execution_pool_grant_version"
>;
/** Historical binding is read from this saved run, never from today's project
 * default. Pool ownership does not imply permission to open that organization. */
export function RunRouteFacts({ run }: { run: SavedRoute }) {
  useLocale();
  const version = run.execution_route_version;
  return (
    <section
      className="pool-run-route"
      aria-label={tr("pools.run.fixed_route")}
    >
      {run.execution_pool_id ? (
        <details>
          <summary>
            <Trans
              i18nKey="pools.run.fixed_shared_pool"
              values={{
                value1:
                  version === undefined
                    ? tr("pools.run.route_version_missing")
                    : tr("pools.run.route_version") + version,
              }}
            />
          </summary>
          <dl>
            <dt>{tr("pools.run.pool")}</dt>
            <dd>
              <code>{run.execution_pool_id}</code>
            </dd>
            <dt>{tr("pools.route.fixed_grant")}</dt>
            <dd>
              <code>
                {run.execution_pool_grant_id || tr("pools.run.not_provided")}
              </code>{" "}
              ·{" "}
              {run.execution_pool_grant_version === undefined
                ? tr("pools.run.grant_version_missing")
                : tr("pools.run.grant_version") +
                  run.execution_pool_grant_version}
            </dd>
          </dl>
          <small>{tr("pools.run.fixed_basis_notice")}</small>
        </details>
      ) : (
        <small>
          {version === undefined
            ? tr("pools.run.no_binding_notice")
            : tr("pools.run.fixed_route_version") +
              version +
              tr("pools.run.no_shared_pool")}
        </small>
      )}
    </section>
  );
}
