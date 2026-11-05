// @vitest-environment jsdom
import {act, type ReactNode} from "react";
import {createRoot, type Root} from "react-dom/client";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import {setLocale} from "../i18n";
import {PoolWorkspace} from "./PoolWorkspace";
import {RunnerSlotOccupancy} from "./RunnerSlotOccupancy";

const now = Date.parse("2026-11-06T10:00:00Z");
const observedAt = new Date(now).toISOString();
const occupancy = {observed_at: observedAt, held: 4, running: 2, reconciling: 1, unknown: 1};
const poolID = "a".repeat(32);
const runnerID = "b".repeat(32);
const tenantID = "c".repeat(32);
let host: HTMLDivElement;
let root: Root;

beforeEach(() => {
  setLocale("en-US");
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

const mount = async (node: ReactNode) => act(async () => root.render(node));
const find = (selector: string) => host.querySelector<HTMLElement>(selector);
const state = () => find("[data-runner-slot-occupancy]")?.dataset.occupancyState;

describe("Runner slot occupancy", () => {
  it("uses authoritative held totals and keeps all three underlying attempt states visible", async () => {
    await mount(<RunnerSlotOccupancy snapshot={occupancy} configuredSlots={6} now={now} refreshing={false}/>);
    expect(state()).toBe("current");
    expect(find("[data-slot-held]")?.textContent).toBe("4");
    expect(find("[data-slot-running]")?.textContent).toBe("2");
    expect(find("[data-slot-reconciling]")?.textContent).toBe("1");
    expect(find("[data-slot-unknown]")?.textContent).toBe("1");
    expect(find("[data-slot-observed-at]")?.getAttribute("datetime")).toBe(observedAt);
    expect(find(".runner-slot-occupancy-bar")?.getAttribute("aria-hidden")).toBe("true");
  });

  it("distinguishes a complete zero from missing or contradictory data", async () => {
    await mount(<RunnerSlotOccupancy snapshot={{...occupancy, held: 0, running: 0, reconciling: 0, unknown: 0}}
      configuredSlots={2} now={now} refreshing={false}/>);
    expect(state()).toBe("current");
    expect(find("[data-slot-held]")?.textContent).toBe("0");
    expect(find(".runner-slot-occupancy-bar")?.children).toHaveLength(0);
    for (const snapshot of [undefined, {...occupancy, held: 0}]) {
      await mount(<RunnerSlotOccupancy snapshot={snapshot} configuredSlots={2} now={now} refreshing={false}/>);
      expect(state()).toBe("unavailable");
      expect(find("[data-slot-held]")).toBeNull();
    }
  });

  it("marks refreshing and aged snapshots without clearing held counts or changing their time", async () => {
    await mount(<RunnerSlotOccupancy snapshot={occupancy} configuredSlots={2} now={now} refreshing/>);
    expect(state()).toBe("refreshing");
    await mount(<RunnerSlotOccupancy snapshot={occupancy} configuredSlots={2} now={now + 60_000} refreshing={false}/>);
    expect(state()).toBe("stale");
    expect(find("[data-slot-held]")?.textContent).toBe("4");
    expect(find("[data-slot-observed-at]")?.getAttribute("datetime")).toBe(observedAt);
  });

  it("clears prior runner rows on access denial and renders only the new snapshot after retry", async () => {
    const session = {
      user: {id: "pool-admin", name: "Pool admin", email: "pool-admin@example.test", role: "admin"},
      organization: {id: tenantID, name: "Current organization"},
      projects: [],
    };
    const pool = {id: poolID, owner_tenant_id: tenantID, name: "Current pool", description: "",
      status: "active", version: 1, allowed_actions: [], created_at: observedAt, updated_at: observedAt};
    const runner = {id: runnerID, pool_id: poolID, worker_id: "pool-" + runnerID, name: "Current runner",
      status: "active", max_slots: 6, version: 1, credential_version: 1,
      credential_expires_at: observedAt, created_at: observedAt, allowed_actions: []};
    const empty = {items: [], total: 0, allowed_actions: []};
    let denied = false;
    let reads = 0;
    let pendingRetry: Promise<Response> | undefined;
    vi.stubGlobal("fetch", vi.fn(async (url: string) => {
      const path = new URL(url, location.href).pathname;
      if (path.endsWith("/runners")) {
        reads++;
        if (denied) return Response.json({error: {code: "forbidden", message: "Access denied"}}, {status: 403});
        const snapshot = reads === 1 ? occupancy : {...occupancy, held: 0, running: 0, reconciling: 0, unknown: 0};
        return pendingRetry ?? Response.json({...empty, items: [{...runner, slot_occupancy: snapshot}], total: 1});
      }
      if (path.endsWith("/" + poolID)) return Response.json(pool);
      return Response.json(empty);
    }));
    await mount(<PoolWorkspace session={session} selected={poolID} panel="runners" refresh={0}
      onSelect={() => {}} onPanel={() => {}}/>);
    expect(find("[data-slot-held]")?.textContent).toBe("4");
    denied = true;
    await act(async () => find("[data-pool-runner-refresh]")?.click());
    expect(find("[data-pool-runner]")).toBeNull();
    expect(find("[data-slot-held]")).toBeNull();
    expect(find("[data-pool-runner-refresh]")).not.toBeNull();
    denied = false;
    let resolveRetry!: (response: Response) => void;
    pendingRetry = new Promise(resolve => {resolveRetry = resolve;});
    await act(async () => find("[data-pool-runner-refresh]")?.click());
    expect(find("[data-pool-runner]")).toBeNull();
    const fresh = {...occupancy, held: 0, running: 0, reconciling: 0, unknown: 0};
    await act(async () => resolveRetry(Response.json({
      ...empty, items: [{...runner, slot_occupancy: fresh}], total: 1,
    })));
    expect(find("[data-slot-held]")?.textContent).toBe("0");
  });
});
