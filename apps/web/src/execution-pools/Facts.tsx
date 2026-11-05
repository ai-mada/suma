import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { time } from "../api";
import { capacityLabel } from "../scheduling-model";
import {
  schedulingResourceLabel,
  schedulingUnitLabel,
} from "../SchedulingFacts";
import type { PoolPolicy, Grant, Runner } from "./model";
import { PoolStatus } from "./Shared";
import { RunnerSlotOccupancy } from "./RunnerSlotOccupancy";
export function PoolPolicyFacts({ policy }: { policy: PoolPolicy }) {
  useLocale();
  return (
    <section
      className="pool-policy-facts"
      aria-label={tr("pools.facts.current")}
    >
      <dl className="pool-metrics">
        <div>
          <dt>{tr("pools.facts.execution_slots")}</dt>
          <dd>{policy.concurrency_limit}</dd>
        </div>
        <div>
          <dt>{tr("pools.facts.cumulative_budget")}</dt>
          <dd>{capacityLabel(policy.budget_limit)}</dd>
        </div>
        <div>
          <dt>{tr("pools.facts.cumulative_used")}</dt>
          <dd>
            <Trans
              i18nKey="pools.facts.used_units"
              values={{ value1: policy.budget_spent }}
            />
          </dd>
        </div>
      </dl>
      <small>
        <Trans
          i18nKey="pools.facts.policy_read_at"
          values={{ value1: policy.version, value2: time(policy.as_of) }}
        />
      </small>
      <details>
        <summary>{tr("pools.facts.resource_details")}</summary>
        <p>{tr("pools.facts.admission_explanation")}</p>
        <p>
          <Trans
            i18nKey="pools.facts.scheduling_scope"
            values={{
              value1:
                policy.fairness.mode === "tenant_project_weighted_round_robin"
                  ? tr("pools.facts.hierarchical_weighted")
                  : policy.fairness.mode,
              value2: policy.fairness.scope,
            }}
          />
        </p>
        {policy.resources.length ? (
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>{tr("pools.facts.resource")}</th>
                  <th>{tr("pools.facts.limit")}</th>
                  <th>{tr("pools.facts.current_reserved")}</th>
                  <th>{tr("pools.facts.cumulative_spent")}</th>
                </tr>
              </thead>
              <tbody>
                {policy.resources.map((item) => (
                  <tr key={item.name}>
                    <td>
                      {schedulingResourceLabel(item.name)} ·{" "}
                      {schedulingUnitLabel(item.unit)}
                    </td>
                    <td>{capacityLabel(item.capacity)}</td>
                    <td>{item.used}</td>
                    <td>{item.consumed}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p>{tr("pools.facts.ledger_missing")}</p>
        )}
      </details>
    </section>
  );
}
export function GrantFacts({ grant }: { grant: Grant }) {
  useLocale();
  return (
    <>
      <div className="pool-row-title">
        <strong>
          {grant.shared_project_name || tr("pools.grant.shared_name_missing")}
        </strong>
        <PoolStatus value={grant.status} />
      </div>
      <p>
        <Trans
          i18nKey="pools.grant.weight_version"
          values={{ value1: grant.project_weight, value2: grant.version }}
        />
      </p>
      <details>
        <summary>{tr("pools.grant.request_basis")}</summary>
        <dl>
          <dt>{tr("pools.grant.organization")}</dt>
          <dd>
            <code>{grant.tenant_id}</code>
          </dd>
          <dt>{tr("pools.grant.project")}</dt>
          <dd>
            <code>{grant.project_id}</code>
          </dd>
          <dt>{tr("pools.grant.request_hash")}</dt>
          <dd>
            <code>{grant.request_sha256}</code>
          </dd>
          <dt>{tr("pools.grant.submitted_at")}</dt>
          <dd>{time(grant.created_at)}</dd>
        </dl>
      </details>
    </>
  );
}
export function RunnerFacts({ runner, occupancyNow, refreshing }: {
  runner: Runner;
  occupancyNow: number;
  refreshing: boolean;
}) {
  useLocale();
  return (
    <>
      <div className="pool-row-title">
        <strong>{runner.name}</strong>
        <PoolStatus value={runner.status} />
      </div>
      <code>{runner.worker_id}</code>
      <p>
        <Trans
          i18nKey="pools.runner.slots_versions"
          values={{
            value1: runner.max_slots,
            value2: runner.version,
            value3: runner.credential_version,
          }}
        />
      </p>
      <p>
        <Trans
          i18nKey="pools.runner.contact_expiry"
          values={{
            value1: runner.last_seen_at
              ? tr("pools.runner.last_seen") + time(runner.last_seen_at)
              : tr("pools.runner.no_contact"),
            value2: time(runner.credential_expires_at),
          }}
        />
      </p>
      <small>{tr("pools.runner.registration_notice")}</small>
      <RunnerSlotOccupancy snapshot={runner.slot_occupancy} configuredSlots={runner.max_slots}
        now={occupancyNow} refreshing={refreshing} />
    </>
  );
}
