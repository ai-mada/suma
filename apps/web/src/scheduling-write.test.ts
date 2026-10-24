import { afterEach, describe, expect, it, vi } from "vitest";
import { SchedulingWriteRecovery } from "./scheduling-write";
afterEach(() => vi.unstubAllGlobals());
const memory = () => {
  const map = new Map<string, string>();
  return {
    getItem: (k: string) => map.get(k) || null,
    setItem: (k: string, v: string) => {
      map.set(k, v);
    },
    removeItem: (k: string) => {
      map.delete(k);
    },
  };
};
describe("policy writes retain a fixed nonsecret reviewed operation", () => {
  it("reloads the original request after unknown result, ignoring edited fields and newer versions", async () => {
    const storage = memory(),
      bodies: unknown[] = [];
    let count = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path: string, options: RequestInit) => {
        bodies.push(JSON.parse(options.body as string));
        if (!count++) throw Error("lost");
        return new Response(JSON.stringify({ id: "preview" }));
      }),
    );
    const first = new SchedulingWriteRecovery("org/user/project", storage);
    await expect(
      first.run("/scheduling/policy-previews", {
        project_id: "project",
        expected_version: 0,
        policy: {
          concurrency_limit: 0,
          resource_limits: { cpu: null },
          budget_limit: 0,
        },
      }),
    ).rejects.toThrow();
    const reload = new SchedulingWriteRecovery("org/user/project", storage);
    expect(reload.pending).toBeDefined();
    await reload.run("/scheduling/policy-previews", {
      project_id: "project",
      expected_version: 9,
      policy: {
        concurrency_limit: 100,
        resource_limits: {},
        budget_limit: null,
      },
    });
    expect(bodies[1]).toEqual(bodies[0]);
    expect(reload.pending).toBeUndefined();
    expect(
      new SchedulingWriteRecovery("other/user/project", storage).pending,
    ).toBeUndefined();
  });
  it("keeps unknown original commit on a later 403 rather than permitting a new write", async () => {
    const storage = memory();
    let count = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        if (!count++) throw Error("lost");
        return new Response(
          JSON.stringify({ error: { code: "forbidden", message: "撤权" } }),
          { status: 403 },
        );
      }),
    );
    const action = new SchedulingWriteRecovery("scope", storage);
    await expect(
      action.run("/scheduling/policy-previews/a/commit", {
        project_id: "p",
        expected_version: 1,
      }),
    ).rejects.toThrow();
    const key = action.pending?.idempotency_key;
    await expect(action.restore()).rejects.toThrow();
    expect(action.pending?.idempotency_key).toBe(key);
  });
  it("rejects corrupted or cross-scope restore envelopes before network activity", async () => {
    const storage = memory();
    storage.setItem(
      "suma-scheduling-recovery-v1:scope",
      JSON.stringify({
        path: "/auth/setup",
        body: { password: "secret" },
        method: "POST",
        idempotency_key: "x",
      }),
    );
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    const action = new SchedulingWriteRecovery("scope", storage);
    expect(action.error).toBeTruthy();
    await expect(
      action.run("/scheduling/policy-previews", {
        project_id: "p",
        expected_version: 0,
        policy: {
          concurrency_limit: 1,
          resource_limits: {},
          budget_limit: null,
        },
      }),
    ).rejects.toThrow();
    expect(fetch).not.toHaveBeenCalled();
  });
});
