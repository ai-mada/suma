// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ToolExecutionFacts } from "./ToolFacts";
import { toolFactsForAttempt, type ToolEvent } from "./tool-model";
import { setLocale, setServerCatalog } from "./i18n";

let host: HTMLDivElement, root: Root, warnings: unknown[][];
beforeEach(async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true); warnings = [];
  vi.spyOn(console, "error").mockImplementation((...args) => warnings.push(args));
  vi.spyOn(console, "warn").mockImplementation((...args) => warnings.push(args));
  await setLocale("zh-CN", { persist: false });
  setServerCatalog({ version: "native-list-fixture", template_format: "named-dot-v1", default_locale: "zh-CN", supported_locales: ["en-US", "zh-CN"], messages: [
    { key: "native.endpoint.rejected", parameters: [{ name: "status", kind: "integer" }], texts: { "en-US": "Endpoint rejected request (HTTP {{.status}})", "zh-CN": "端点拒绝请求（HTTP {{.status}}）" } },
  ] });
  host = document.createElement("div"); document.body.append(host); root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount()); host.remove();
  vi.unstubAllGlobals(); vi.restoreAllMocks(); expect(warnings).toEqual([]);
});
const fact = (status: number): ToolEvent => ({ schema_version: 1, command_id: "native", profile_sha256: "a".repeat(64), protocol: "suma-jsonl-v1", kind: "error",
  message: `Endpoint rejected request (HTTP ${status})`, diagnostic: { schema_version: 1, message_key: "native.endpoint.rejected", message_params: { status } } });
const target = { id: "native-node", attempt: 1, fence: 2, worker_id: "native-worker" };
const parent = (sequence: number, tool_event = fact(399 + sequence)) => ({ sequence, run_id: "run-original", node_id: target.id, type: "tool_progress",
  data: { attempt: target.attempt, fence: target.fence, worker_id: target.worker_id, tool_event } });
async function render(events: ToolEvent[]) { await act(async () => root.render(<ToolExecutionFacts state="running" events={events} />)); }
const disclosures = () => [...host.querySelectorAll<HTMLDetailsElement>(".tool-event-list details")];
function inspect(details: HTMLDetailsElement) { details.open = true; const summary = details.querySelector("summary")!; summary.focus(); expect(document.activeElement).toBe(summary); return summary; }
const original = (details: HTMLDetailsElement) => details.querySelector("pre")?.textContent;

it("does not transfer an open focused raw disclosure from HTTP 400 to HTTP 401 when event 41 arrives", async () => {
  const events = Array.from({ length: 40 }, (_, i) => fact(400 + i));
  const before = JSON.stringify(events); await render(events);
  const removed = disclosures()[0], summary = inspect(removed);
  expect(original(removed)).toBe("Endpoint rejected request (HTTP 400)");
  await render([...events, fact(440)]);
  const next = disclosures()[0];
  expect(removed.isConnected).toBe(false); expect(summary.isConnected).toBe(false);
  expect(original(next)).toBe("Endpoint rejected request (HTTP 401)");
  expect(next.open).toBe(false); expect(document.activeElement).not.toBe(next.querySelector("summary"));
  expect(JSON.stringify(events)).toBe(before);
});

it("keeps a durable event's disclosure and focus when that same event moves within the rolling window", async () => {
  const parents = Array.from({ length: 40 }, (_, i) => parent(i + 1));
  const before = JSON.stringify(parents); await render(toolFactsForAttempt(parents, target));
  const retained = disclosures()[1], summary = inspect(retained);
  await render(toolFactsForAttempt([...parents, parent(41)], target));
  expect(disclosures()[0]).toBe(retained); expect(retained.open).toBe(true);
  expect(original(retained)).toBe("Endpoint rejected request (HTTP 401)"); expect(document.activeElement).toBe(summary);
  expect(JSON.stringify(parents)).toBe(before);
});

it("distinguishes identical raw messages by their durable parent sequences on reorder and refreshed objects", async () => {
  const first = parent(7, fact(429)), second = parent(8, fact(429));
  await render(toolFactsForAttempt([first, second], target));
  const secondDetails = disclosures()[1], summary = inspect(secondDetails);
  await render(toolFactsForAttempt(JSON.parse(JSON.stringify([second, first])), target));
  expect(disclosures()[0]).toBe(secondDetails); expect(disclosures()[1].open).toBe(false);
  expect(secondDetails.open).toBe(true); expect(document.activeElement).toBe(summary);
  expect(original(secondDetails)).toBe("Endpoint rejected request (HTTP 429)");
});

it("retains one parent sequence across refresh, property reorder and locale changes without rewriting raw JSON", async () => {
  const parents = [parent(1), parent(2)]; const before = JSON.stringify(parents);
  await render(toolFactsForAttempt(parents, target));
  const retained = disclosures()[1], summary = inspect(retained);
  const reversedFact = Object.fromEntries(Object.entries(parents[1].data.tool_event).reverse()) as ToolEvent;
  await render(toolFactsForAttempt([{ ...parents[1], data: { ...parents[1].data, tool_event: reversedFact } }, parents[0]], target));
  await act(async () => { await setLocale("en-US", { persist: false }); });
  expect(disclosures()[0]).toBe(retained); expect(retained.open).toBe(true); expect(document.activeElement).toBe(summary);
  expect(summary.textContent).toBe("Original reason"); expect(original(retained)).toBe(parents[1].data.tool_event.message);
  expect(JSON.stringify(parents)).toBe(before);
  expect(JSON.stringify(toolFactsForAttempt(parents, target))).toBe(JSON.stringify(parents.map(event => event.data.tool_event)));
});

it("uses raw-content fallback for direct props without assigning another message a duplicate's interactive state", async () => {
  const duplicate = fact(429), distinct = fact(503);
  await render([duplicate, { ...duplicate }, distinct]);
  const retained = disclosures()[2], summary = inspect(retained);
  await render([JSON.parse(JSON.stringify(distinct)), { ...duplicate }, { ...duplicate }]);
  expect(disclosures()).toHaveLength(3); expect(disclosures()[0]).toBe(retained);
  expect(retained.open).toBe(true); expect(document.activeElement).toBe(summary);
  expect(disclosures().slice(1).every(details => !details.open)).toBe(true);
});

it.each(["message", "metadata"])("remounts the raw disclosure if %s changes despite the same parent sequence", async field => {
  const first = parent(7, fact(429));
  await render(toolFactsForAttempt([first], target));
  const removed = disclosures()[0]; inspect(removed);
  const changed = field === "message" ? fact(503) : { ...first.data.tool_event, status: "changed-upstream-metadata" };
  await render(toolFactsForAttempt([{ ...first, data: { ...first.data, tool_event: changed } }], target));
  expect(removed.isConnected).toBe(false); expect(disclosures()[0].open).toBe(false);
  expect(document.activeElement).not.toBe(disclosures()[0].querySelector("summary"));
  expect(original(disclosures()[0])).toBe(changed.message);
});

it("keeps duplicate copies of the same parent as separate sibling rows without duplicate React keys", async () => {
  const record = parent(7, fact(429));
  await render(toolFactsForAttempt([record, JSON.parse(JSON.stringify(record))], target));
  expect(disclosures()).toHaveLength(2);
  expect(disclosures().map(original)).toEqual([record.data.tool_event.message, record.data.tool_event.message]);
});

it.each(["run", "node", "attempt", "fence", "worker"])("does not transfer evidence state across a changed durable %s identity", async field => {
  const first = parent(7, fact(429));
  await render(toolFactsForAttempt([first], target));
  const removed = disclosures()[0]; inspect(removed);
  const nextTarget = { ...target }, next = { ...first, data: { ...first.data } };
  if (field === "run") next.run_id = "another-run";
  if (field === "node") next.node_id = nextTarget.id = "another-node";
  if (field === "attempt") next.data.attempt = nextTarget.attempt = 2;
  if (field === "fence") next.data.fence = nextTarget.fence = 3;
  if (field === "worker") next.data.worker_id = nextTarget.worker_id = "another-worker";
  await render(toolFactsForAttempt([next], nextTarget));
  expect(removed.isConnected).toBe(false); expect(disclosures()[0].open).toBe(false);
  expect(document.activeElement).not.toBe(disclosures()[0].querySelector("summary"));
});
