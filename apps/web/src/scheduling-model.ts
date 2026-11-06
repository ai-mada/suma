import { tr as t, UiError, formatNumber } from "./i18n";
export type {
  SchedulingPolicyInput as PolicyInput,
  SchedulingPolicyView as SchedulingPolicy,
  ResourceSnapshot,
  SchedulingPolicyPreview as PolicyPreview,
  SchedulingBlocker,
  ResourceRequest,
  ResourceReservationSnapshot as ResourceReservation,
  NodeAdmissionSnapshot as NodeAdmission,
  ReadyEntry,
  ExecutionAttempt,
} from "../../../packages/contracts/src";
import type {
  SchedulingPolicyInput as PolicyInput,
  NodeAdmissionSnapshot as NodeAdmission,
} from "../../../packages/contracts/src";
export type QueueRoute = "project" | "pool";
export function validQueueIdentifier(value: string): boolean {
  return value.length <= 100 && /^[A-Za-z0-9]/.test(value) && !/[^A-Za-z0-9_.-]/.test(value);
}
export type SchedulingPage<T> = {
  items: T[];
  total: number;
  next_cursor?: string;
};
export function amountInput(value: string, nullable = false): number | null {
  if (nullable && !value.trim()) return null;
  if (!/^(0|[1-9][0-9]*)$/.test(value.trim()))
    throw new UiError("common.form.quotaInteger");
  const amount = Number(value);
  if (!Number.isSafeInteger(amount)) throw new UiError("common.form.quotaSafe");
  return amount;
}
export function policyInput(
  concurrency: string,
  budget: string,
  rows: { name: string; value: string }[],
): PolicyInput {
  const resource_limits: Record<string, number | null> = {};
  for (const row of rows) {
    const name = row.name.trim();
    if (
      !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(name) ||
      ["budget", "execution_slots"].includes(name) ||
      Object.hasOwn(resource_limits, name)
    )
      throw new UiError("common.form.resourceName");
    resource_limits[name] = amountInput(row.value, true);
  }
  return {
    concurrency_limit: amountInput(concurrency)!,
    budget_limit: amountInput(budget, true),
    resource_limits,
  };
}
export function parseAttempt(value: string): number | undefined {
  return /^[1-9][0-9]*$/.test(value) && Number.isSafeInteger(Number(value))
    ? Number(value)
    : undefined;
}
export function executionHref(
  project: string,
  run: string,
  node: string,
  attempt?: number,
) {
  return (
    "?" +
    new URLSearchParams({
      project,
      view: "development",
      sub: "runs",
      run,
      node,
      ...(attempt ? { attempt: String(attempt) } : {}),
    })
  );
}
export function admissionCurrent(
  snapshot: Pick<NodeAdmission, "run_id" | "node_id" | "run_version">,
  run: string,
  node: string,
  version: number,
) {
  return (
    snapshot.run_id === run &&
    snapshot.node_id === node &&
    snapshot.run_version >= version
  );
}
export const admissionNames: Record<string, string> = {
  get blocked() { return t("development.admission.current_conditions_blocked"); },
  get unknown() { return t("development.admission.execution_eligibility_unconfirmed"); },
  get eligible() { return t("development.admission.current_admission_checks_passed"); },
  get not_pending() { return t("development.admission.not_currently_awaiting_dispatch"); },
};
export const capacityLabel = (value: number | null | undefined) =>
  value === null
    ? t("common.unlimited")
    : value === undefined
      ? t("common.notProvided")
      : formatNumber(value);
