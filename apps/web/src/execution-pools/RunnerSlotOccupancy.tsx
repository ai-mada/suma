import {formatDateTime, formatNumber, tr, useLocale} from "../i18n";
import type {ExecutionPoolRunnerSlotOccupancy} from "../../../../packages/contracts/src";
import "./RunnerSlotOccupancy.css";

type Props = {
  snapshot?: ExecutionPoolRunnerSlotOccupancy | null;
  configuredSlots: number;
  now: number;
  refreshing: boolean;
};

const buckets = ["running", "reconciling", "unknown"] as const;
const snapshotAgeLimit = 60_000;

function validSnapshot(snapshot: ExecutionPoolRunnerSlotOccupancy, configuredSlots: number) {
  const counts = [snapshot.held, snapshot.running, snapshot.reconciling, snapshot.unknown];
  return Number.isSafeInteger(configuredSlots) && configuredSlots > 0 &&
    typeof snapshot.observed_at === "string" && Number.isFinite(Date.parse(snapshot.observed_at)) &&
    counts.every(value => Number.isSafeInteger(value) && value >= 0) &&
    snapshot.held === snapshot.running + snapshot.reconciling + snapshot.unknown;
}

export function RunnerSlotOccupancy({snapshot, configuredSlots, now, refreshing}: Props) {
  useLocale();
  if (!snapshot || !validSnapshot(snapshot, configuredSlots)) {
    return <div className="runner-slot-occupancy" data-runner-slot-occupancy data-occupancy-state="unavailable">
      <strong>{tr("pool-runner-occupancy.title")}</strong>
      <span className="runner-slot-occupancy-state">{tr("pool-runner-occupancy.unavailable")}</span>
    </div>;
  }

  const stale = Math.abs(now - Date.parse(snapshot.observed_at)) >= snapshotAgeLimit;
  const state = refreshing ? "refreshing" : stale ? "stale" : "current";
  const scale = Math.max(configuredSlots, snapshot.held);
  return <section className="runner-slot-occupancy" aria-label={tr("pool-runner-occupancy.title")}
    data-runner-slot-occupancy data-occupancy-state={state}>
    <div className="runner-slot-occupancy-header">
      <div className="runner-slot-occupancy-total">
        <strong>{tr("pool-runner-occupancy.title")}</strong>
        <span data-slot-held>{formatNumber(snapshot.held)}</span>
        <span aria-hidden="true">/</span>
        <span>{tr("pool-runner-occupancy.configured_value", {count: formatNumber(configuredSlots)})}</span>
      </div>
      {state !== "current" && <span className="runner-slot-occupancy-state">
        {tr("pool-runner-occupancy." + state)}
      </span>}
    </div>
    <div className="runner-slot-occupancy-bar" aria-hidden="true">
      {buckets.filter(bucket => snapshot[bucket] > 0).map(bucket =>
        <span className={"runner-slot-" + bucket} key={bucket} style={{width: (snapshot[bucket] / scale) * 100 + "%"}}/>)}
    </div>
    <dl className="runner-slot-occupancy-breakdown">
      <div><dt><i className="runner-slot-running" aria-hidden="true"/>{tr("pool-runner-occupancy.running")}</dt>
        <dd data-slot-running>{formatNumber(snapshot.running)}</dd></div>
      <div><dt><i className="runner-slot-reconciling" aria-hidden="true"/>{tr("pool-runner-occupancy.reconciling")}</dt>
        <dd data-slot-reconciling>{formatNumber(snapshot.reconciling)}</dd></div>
      <div><dt><i className="runner-slot-unknown" aria-hidden="true"/>{tr("pool-runner-occupancy.unknown")}</dt>
        <dd data-slot-unknown>{formatNumber(snapshot.unknown)}</dd></div>
    </dl>
    <small className="runner-slot-occupancy-time">
      {tr("pool-runner-occupancy.observed_at")}{" "}
      <time dateTime={snapshot.observed_at} data-slot-observed-at>
        {formatDateTime(snapshot.observed_at, {dateStyle: "short", timeStyle: "medium"})}
      </time>
    </small>
  </section>;
}
