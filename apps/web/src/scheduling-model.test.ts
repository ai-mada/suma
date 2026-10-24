import { describe, expect, it } from "vitest";
import {
  policyInput,
  parseAttempt,
  admissionCurrent,
  executionHref,
} from "./scheduling-model";
describe("scheduling inputs preserve enforced zero and unlimited", () => {
  it("preserves zero concurrency, zero budget and explicit resource unlimited", () => {
    expect(
      policyInput("0", "0", [
        { name: "gpu", value: "" },
        { name: "cpu", value: "0" },
      ]),
    ).toEqual({
      concurrency_limit: 0,
      budget_limit: 0,
      resource_limits: { gpu: null, cpu: 0 },
    });
  });
  it("blank budget is unlimited but blank concurrency cannot silently default", () => {
    expect(policyInput("1", "", []).budget_limit).toBeNull();
    expect(() => policyInput("", "", [])).toThrow();
  });
  it.each(["-1", "1.1", "1e3", "9007199254740992", "Infinity"])(
    "rejects invalid unsigned admission amount %s",
    (v) => {
      expect(() => policyInput(v, "", [])).toThrow();
    },
  );
  it("rejects duplicate or reserved resource keys before preview", () => {
    for (const rows of [
      [
        { name: "cpu", value: "1" },
        { name: "cpu", value: "2" },
      ],
      [{ name: "budget", value: "1" }],
      [{ name: "execution_slots", value: "1" }],
      [{ name: "bad key", value: "1" }],
    ])
      expect(() => policyInput("1", "", rows)).toThrow();
  });
  it("accepts only canonical positive attempt deep links", () => {
    expect(parseAttempt("12")).toBe(12);
    for (const value of ["", "0", "-1", "1.5", "01", "1e2", "9007199254740992"])
      expect(parseAttempt(value)).toBeUndefined();
  });
  it("keeps project and exact execution identifiers without leaking input into URL syntax", () => {
    const q = new URL(executionHref("p", "r/x", "map[2]", 2), "http://local")
      .searchParams;
    expect(Object.fromEntries(q)).toMatchObject({
      project: "p",
      view: "development",
      sub: "runs",
      run: "r/x",
      node: "map[2]",
      attempt: "2",
    });
  });
  it("never calls a prior run/node/version diagnostic current", () => {
    const snapshot = { run_id: "r", node_id: "n", run_version: 7 };
    expect(admissionCurrent(snapshot, "r", "n", 7)).toBe(true);
    expect(admissionCurrent(snapshot, "r", "n", 8)).toBe(false);
    expect(admissionCurrent(snapshot, "r", "other", 7)).toBe(false);
  });
});
