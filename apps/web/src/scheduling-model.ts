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
export type SchedulingPage<T> = {
  items: T[];
  total: number;
  next_cursor?: string;
};
export function amountInput(value: string, nullable = false): number | null {
  if (nullable && !value.trim()) return null;
  if (!/^(0|[1-9][0-9]*)$/.test(value.trim()))
    throw Error("额度必须是非负整数；空白仅用于明确不限制。");
  const amount = Number(value);
  if (!Number.isSafeInteger(amount)) throw Error("额度超过安全整数范围。");
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
      throw Error("资源名称无效、重复或与执行槽位／预算保留字段冲突。");
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
  blocked: "当前条件受阻",
  unknown: "执行资格尚未确认",
  eligible: "当前准入检查通过",
  not_pending: "当前不等待派发",
};
export const capacityLabel = (value: number | null | undefined) =>
  value === null ? "不限制" : value === undefined ? "未提供" : String(value);
