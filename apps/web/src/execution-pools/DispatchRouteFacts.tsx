import type { ProjectExecutionRoute } from "../../../../packages/contracts/src";
import { tr, useLocale } from "../i18n";
/** This basis belongs to the immutable dispatch preview. Missing historical data
 * stays missing; reading today's project default would fabricate that basis. */
export function DispatchRouteFacts({
  route,
}: {
  route?: ProjectExecutionRoute | null;
}) {
  useLocale();
  return (
    <section
      className="tool-context"
      aria-label={tr("pools.dispatch.fixed_route")}
    >
      <h4>{tr("pools.dispatch.fixed_route")}</h4>
      {!route ? (
        <p className="warning">{tr("pools.dispatch.unrecorded")}</p>
      ) : (
        <>
          <p>
            {route.pool_id
              ? tr("pools.dispatch.shared")
              : tr("pools.dispatch.local")}{" "}
            · {tr("pools.dispatch.route_version", { version: route.version })}
          </p>
          {route.pool_id && (
            <dl>
              <dt>{tr("pools.dispatch.pool")}</dt>
              <dd>
                <code>{route.pool_id}</code>
              </dd>
              <dt>{tr("pools.dispatch.grant")}</dt>
              <dd>
                <code>{route.grant_id}</code> ·{" "}
                {tr("pools.dispatch.grant_version", {
                  version: route.grant_version,
                })}
              </dd>
            </dl>
          )}
          <small>{tr("pools.dispatch.notice")}</small>
        </>
      )}
    </section>
  );
}
