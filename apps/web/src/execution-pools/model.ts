import { tr, useLocale, UiError } from "../i18n";
import type { Session } from "../api";
export type {
  ExecutionPool as Pool,
  ExecutionPoolPage as PoolPage,
  ExecutionPoolGrant as Grant,
  ExecutionPoolGrantPage as GrantPage,
  ExecutionPoolRunner as Runner,
  ExecutionPoolRunnerPage as RunnerPage,
  ExecutionPoolCredentialResponse as Credential,
  ExecutionPoolPolicy as PoolPolicy,
} from "../../../../packages/contracts/src";
export type PoolProps = {
  session: Session;
  project?: string;
  selected: string;
  panel: string;
  refresh: number;
  onSelect: (id: string) => void;
  onPanel: (panel: string) => void;
};
export const poolPath = (id: string) =>
  "/execution-pools/" + encodeURIComponent(id);
export const poolStatus = (status: string) =>
  ({
    active: tr("pools.status.active"),
    paused: tr("pools.status.paused"),
    pending: tr("pools.status.requested"),
    approved: tr("pools.status.approved"),
    rejected: tr("pools.status.rejected"),
    revoked: tr("pools.status.revoked"),
    withdrawn: tr("pools.status.withdrawn"),
  })[status] || status;
export function bounded(
  value: string,
  min: number,
  max: number,
  label: string,
) {
  if (
    !/^(0|[1-9][0-9]*)$/.test(value) ||
    !Number.isSafeInteger(Number(value)) ||
    Number(value) < min ||
    Number(value) > max
  )
    throw new UiError("pools.validation.integer_range", { label, min, max });
  return Number(value);
}
export function poolName(value: string) {
  const name = value.trim();
  if (!name || new TextEncoder().encode(name).length > 200)
    throw new UiError("pools.validation.name");
  return name;
}
export function projectHref(project: string) {
  return (
    "?" +
    new URLSearchParams({ project, view: "settings", sub: "project-execution" })
  );
}
