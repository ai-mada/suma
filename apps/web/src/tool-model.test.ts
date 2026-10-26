import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./api";
import {
  dispatchRequest,
  parseToolManifest,
  toolBinding,
  toolFactsForAttempt,
  toolEventRows,
  toolReturnPanel,
  toolReturnRun,
  taskPlanStarted,
  taskToolProtocol,
  type ToolProfile,
  type ToolProfileDefinition,
} from "./tool-model";
import { ToolWriteRecovery } from "./tool-write";
const definition: ToolProfileDefinition = {
  command_id: "coder",
  adapter: "suma",
  protocol: "suma-jsonl-v1",
  profile_sha256: "a".repeat(64),
  executable_sha256: "b".repeat(64),
  tool_version: "1.0",
  read_only: false,
};
const profile: ToolProfile = {
  id: "profile",
  project_id: "p",
  worker_id: "worker",
  name: "编码工具",
  version: 4,
  state: "enabled",
  profile: definition,
  enabled_by: "u",
  created_at: "",
  updated_at: "",
  available: true,
  reason: "",
};
const workflow = {
  id: "flow",
  name: "编码及测试",
  version: 2,
  nodes: [
    {
      id: "code",
      name: "编码",
      type: "agent",
      command: ["old"],
      config: { goal: "原始约束" },
    },
    {
      id: "test",
      name: "测试",
      type: "test",
      command: ["verify"],
      config: { candidate_from: "code" },
    },
  ],
  edges: [{ from: "code", to: "test" }],
};
function storage() {
  const values = new Map<string, string>();
  return {
    values,
    getItem: (key: string) => values.get(key) || null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
}
afterEach(() => vi.unstubAllGlobals());
describe("tool manifest and fixed task choices", () => {
  it("accepts the new fixed CLI protocols without accepting a cross-adapter protocol", () => {
    for(const [adapter,protocol] of [['harness','dsh-multica-jsonl-v1'],['codebuddy','codebuddy-jsonl-v1']]){
      const candidate={...definition,adapter,protocol};
      expect(parseToolManifest(JSON.stringify([{profile:candidate,available:true,reason:''}]))[0].profile).toEqual(candidate);
      expect(taskToolProtocol(candidate)).toBe(true);
      expect(taskToolProtocol({...candidate,protocol:'suma-jsonl-v1'})).toBe(false);
    }
  });
  it("uses persisted started state rather than a reserved run identifier as the execution fact", () => {
    expect(taskPlanStarted({ state: "ready", run_id: "reserved-run-id" })).toBe(
      false,
    );
    expect(taskPlanStarted({ state: "started", run_id: "actual-run-id" })).toBe(
      true,
    );
    expect(taskPlanStarted({ state: "started", run_id: "" })).toBe(true);
  });
  it("keeps the exact run and node in a bounded tool-workspace return route", () => {
    const panel = toolReturnPanel("run-1", "map:entry/child");
    expect(toolReturnRun(panel)).toEqual({
      run: "run-1",
      node: "map:entry/child",
    });
    expect(toolReturnRun("other:run-1/node")).toBeUndefined();
    expect(toolReturnRun("return-run:bad/too/many")).toBeUndefined();
  });
  it("accepts only nonsecret describe-tools observations and retains unavailable explanations", () => {
    expect(
      parseToolManifest(
        JSON.stringify([{ profile: definition, available: true, reason: "" }]),
      ),
    ).toEqual([{ profile: definition, available: true, reason: "" }]);
    const unavailable = {
      profile: {
        command_id: "missing",
        adapter: "command",
        protocol: "text-v1",
        tool_version: "",
      },
      available: false,
      reason: "binary_missing",
    };
    expect(
      parseToolManifest(JSON.stringify({ items: [unavailable] }))[0],
    ).toEqual(unavailable);
  });
  it("bounds observation reasons at 1000 UTF-8 bytes rather than characters", () => {
    const reason = "中".repeat(333) + "a";
    expect(new TextEncoder().encode(reason).byteLength).toBe(1000);
    expect(
      parseToolManifest(
        JSON.stringify([{ profile: definition, available: true, reason }]),
      )[0].reason,
    ).toBe(reason);
    for (const oversized of [reason + "a", "中".repeat(334), "a".repeat(1001)])
      expect(() =>
        parseToolManifest(
          JSON.stringify([
            { profile: definition, available: true, reason: oversized },
          ]),
        ),
      ).toThrow();
  });
  it("requires a meaningful reason when a configured tool is unavailable", () => {
    for (const reason of ["", " \n\t"])
      expect(() =>
        parseToolManifest(
          JSON.stringify([{ profile: definition, available: false, reason }]),
        ),
      ).toThrow();
    expect(
      parseToolManifest(
        JSON.stringify([
          { profile: definition, available: false, reason: "CLI probe failed" },
        ]),
      )[0].available,
    ).toBe(false);
    expect(
      parseToolManifest(
        JSON.stringify([{ profile: definition, available: true, reason: "" }]),
      )[0].available,
    ).toBe(true);
  });
  it("rejects secrets, raw execution paths, malformed pair/hash and duplicate command identities", () => {
    const base = { profile: definition, available: true, reason: "" };
    for (const value of [
      [{ ...base, token: "private" }],
      [{ ...base, profile: { ...definition, executable: "C:/private" } }],
      [{ ...base, profile: { ...definition, protocol: "codex-jsonl-v1" } }],
      [{ ...base, profile: { ...definition, profile_sha256: "A".repeat(64) } }],
      [base, base],
    ])
      expect(() => parseToolManifest(JSON.stringify(value))).toThrow();
  });
  it("binds the command to the selected immutable profile without changing unrelated node config", () => {
    expect(toolBinding(workflow.nodes[0], profile)).toEqual({
      command: ["coder"],
      config: {
        goal: "原始约束",
        tool_profile_id: "profile",
        tool_profile_sha256: "a".repeat(64),
      },
    });
  });
  it("creates a versioned preview request with no mutable task text or executable parameters", () => {
    expect(
      dispatchRequest(
        "p",
        { id: "task", version: 7, title: "当前任务" },
        workflow,
        "code",
        profile,
      ),
    ).toEqual({
      project_id: "p",
      task_id: "task",
      expected_task_version: 7,
      workflow_id: "flow",
      workflow_version: 2,
      agent_node_id: "code",
      tool_profile_id: "profile",
      expected_tool_version: 4,
    });
    expect(() =>
      dispatchRequest(
        "other",
        { id: "task", version: 7 },
        workflow,
        "code",
        profile,
      ),
    ).toThrow();
    expect(() =>
      dispatchRequest(
        "p",
        { id: "task", version: 7 },
        workflow,
        "test",
        profile,
      ),
    ).toThrow();
  });
  it("refuses read-only task development or a workflow without the chosen candidate test", () => {
    expect(() =>
      dispatchRequest("p", { id: "task", version: 7 }, workflow, "code", {
        ...profile,
        profile: { ...definition, read_only: true },
      }),
    ).toThrow("写入");
    expect(() =>
      dispatchRequest(
        "p",
        { id: "task", version: 7 },
        { ...workflow, nodes: [workflow.nodes[0]] },
        "code",
        profile,
      ),
    ).toThrow("独立测试");
  });
  it("limits task dispatch to verified structured protocols while retaining generic workflow tools", () => {
    for (const variant of [
      { adapter: "command", protocol: "text-v1" },
      { adapter: "opencode", protocol: "opencode-jsonl-v1" },
      { adapter: "codex", protocol: "text-v1" },
    ]) {
      const candidate = { ...profile, profile: { ...definition, ...variant } };
      expect(() =>
        dispatchRequest(
          "p",
          { id: "task", version: 7 },
          workflow,
          "code",
          candidate,
        ),
      ).toThrow("结构化");
      if (variant.adapter !== "codex")
        expect(toolBinding(workflow.nodes[0], candidate).command).toEqual([
          "coder",
        ]);
    }
    expect(
      dispatchRequest("p", { id: "task", version: 7 }, workflow, "code", {
        ...profile,
        profile: {
          ...definition,
          adapter: "codex",
          protocol: "codex-jsonl-v1",
        },
      }).tool_profile_id,
    ).toBe("profile");
  });
  it("never shows another attempt, worker or fence as current tool progress", () => {
    const event = {
      schema_version: 1,
      command_id: "coder",
      profile_sha256: "a".repeat(64),
      protocol: "suma-jsonl-v1",
      kind: "message",
      message: "当前输出",
    };
    const record = {
      type: "tool_progress",
      node_id: "code",
      data: { attempt: 2, fence: 3, worker_id: "worker", tool_event: event },
    };
    const other = [
      { ...record, data: { ...record.data, attempt: 1 } },
      { ...record, data: { ...record.data, fence: 2 } },
      { ...record, data: { ...record.data, worker_id: "other" } },
    ];
    expect(
      toolFactsForAttempt([...other, record], {
        id: "code",
        attempt: 2,
        fence: 3,
        worker_id: "worker",
      }),
    ).toEqual([event]);
  });
  it("carries durable presentation provenance without adding fields to the persisted event or its JSON projection", () => {
    const fact = Object.freeze({ schema_version: 1, command_id: "native", profile_sha256: "a".repeat(64), protocol: "suma-jsonl-v1", kind: "message", message: "Raw original evidence" });
    const record = Object.freeze({ sequence: 17, run_id: "original-run", node_id: "code", type: "tool_progress",
      data: Object.freeze({ attempt: 2, fence: 3, worker_id: "worker", tool_event: fact }) });
    const before = JSON.stringify(record);
    const projected = toolFactsForAttempt([record], { id: "code", attempt: 2, fence: 3, worker_id: "worker" });
    expect(projected).toEqual([fact]); expect(projected[0]).not.toBe(fact);
    expect(JSON.stringify(projected)).toBe(JSON.stringify([fact]));
    expect(Object.keys(projected[0]).sort()).toEqual(Object.keys(fact).sort());
    expect(JSON.stringify(record)).toBe(before);
  });
  it.each([
    { sequence: undefined, run_id: "run" },
    { sequence: 0, run_id: "run" },
    { sequence: -1, run_id: "run" },
    { sequence: 1.5, run_id: "run" },
    { sequence: Number.MAX_SAFE_INTEGER + 1, run_id: "run" },
    { sequence: "7", run_id: "run" },
    { sequence: 7, run_id: undefined },
    { sequence: 7, run_id: "" },
  ])("uses raw-content fallback when parent provenance is unproven: $sequence / $run_id", ({ sequence, run_id }) => {
    const fact = { schema_version: 1 as const, command_id: "native", profile_sha256: "a".repeat(64), protocol: "suma-jsonl-v1", kind: "message", message: "Raw original" };
    const record = { sequence, run_id, type: "tool_progress", node_id: "code", data: { attempt: 2, fence: 3, worker_id: "worker", tool_event: fact } };
    const projected = toolFactsForAttempt([record], { id: "code", attempt: 2, fence: 3, worker_id: "worker" });
    expect(toolEventRows(projected)[0].key).toBe(toolEventRows([fact])[0].key);
    expect(JSON.stringify(projected)).toBe(JSON.stringify([fact]));
  });
});
describe("tool write recovery", () => {
  it("persists the exact operation before sending and replays it after reload without rebasing", async () => {
    const store = storage(),
      calls: Record<string, unknown>[] = [];
    let count = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path: string, options: { body: string }) => {
        calls.push(JSON.parse(options.body));
        if (!count++) throw Error("lost ACK");
        return new Response(JSON.stringify({ ...profile, version: 5 }), {
          status: 200,
        });
      }),
    );
    const first = new ToolWriteRecovery("org:user:p", store);
    await expect(
      first.run("/tool-profiles/profile/commands", {
        project_id: "p",
        expected_version: 4,
        action: "enable",
      }),
    ).rejects.toBeInstanceOf(ApiError);
    expect(first.pending?.body.expected_version).toBe(4);
    expect(store.values.size).toBe(1);
    const second = new ToolWriteRecovery("org:user:p", store);
    await second.restore();
    expect(calls[1]).toEqual(calls[0]);
    expect(second.pending).toBeUndefined();
    expect(store.values.size).toBe(0);
  });
  it("retains an unknown operation when a later replay is forbidden and isolates users/projects", async () => {
    const store = storage();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw Error("lost");
      }),
    );
    const first = new ToolWriteRecovery("org:u:p", store);
    await expect(
      first.run("/task-dispatch-plans/plan/start", {
        project_id: "p",
        expected_version: 1,
      }),
    ).rejects.toThrow();
    expect(new ToolWriteRecovery("org:other:p", store).pending).toBeUndefined();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ error: { code: "forbidden", message: "撤权" } }),
            { status: 403 },
          ),
      ),
    );
    const restored = new ToolWriteRecovery("org:u:p", store);
    await expect(restored.restore()).rejects.toThrow();
    expect(restored.pending).toBeDefined();
  });
  it("rejects credential fields or unrelated paths before any network request", async () => {
    const send = vi.fn();
    vi.stubGlobal("fetch", send);
    const recovery = new ToolWriteRecovery("p", storage());
    await expect(
      recovery.run("/tool-profiles", { project_id: "p", token: "secret" }),
    ).rejects.toThrow("秘密");
    await expect(recovery.run("/runs", { project_id: "p" })).rejects.toThrow(
      "范围",
    );
    expect(send).not.toHaveBeenCalled();
  });
});
