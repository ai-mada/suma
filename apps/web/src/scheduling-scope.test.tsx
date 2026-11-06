// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiViewProvider } from "./api-context";
import { useSchedulingPage } from "./scheduling-hooks";

type Item = { id: string };
type Page = ReturnType<typeof useSchedulingPage<Item>>;
type PendingRead = {
  url: URL;
  headers: Headers;
  signal?: AbortSignal;
  resolve: (response: Response) => void;
};

const requests: PendingRead[] = [];
let root: Root;
let page: Page;

function Probe() {
  page = useSchedulingPage<Item>("/scheduling/queue?project_id=same", (item) => item.id);
  return null;
}

async function show(user: string) {
  await act(async () => {
    root.render(
      <ApiViewProvider session={{ user: { id: user, name: user, email: "" }, organization: { id: "org", name: "Organization" } }}>
        <Probe />
      </ApiViewProvider>,
    );
  });
}

async function reply(index: number, value: unknown, status = 200) {
  await act(async () => requests[index].resolve(new Response(JSON.stringify(value), { status })));
}

async function reload(cursor: string) {
  await act(async () => { void page.read(cursor); });
}

async function retry() {
  await act(async () => { void page.retry(); });
}

beforeEach(() => {
  requests.length = 0;
  root = createRoot(document.createElement("div"));
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("fetch", vi.fn((path: string, options: RequestInit) => new Promise<Response>((resolve) => {
    requests.push({ url: new URL(path, "https://example.test"), headers: new Headers(options.headers), signal: options.signal ?? undefined, resolve });
  })));
});

afterEach(async () => {
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});

describe("scheduling read authority", () => {
  it("retires cached rows and late requests when the account changes and returns to the same project", async () => {
    await show("alice");
    await reply(0, { items: [{ id: "alice-old" }], total: 5, next_cursor: "alice-cursor" });
    await reload("alice-cursor");
    expect(page.items).toEqual([{ id: "alice-old" }]);

    await show("bob");
    expect(page.items).toEqual([]);
    expect(page.next).toBe("");
    expect(page.hasSnapshot).toBe(false);
    expect(requests[1].signal?.aborted).toBe(true);
    await show("alice");
    expect(requests[2].signal?.aborted).toBe(true);
    expect(requests.map(({ headers }) => [headers.get("X-Suma-Expected-User"), headers.get("X-Suma-Expected-Organization")])).toEqual([
      ["alice", "org"], ["alice", "org"], ["bob", "org"], ["alice", "org"],
    ]);

    await reply(1, { items: [{ id: "alice-late" }], total: 6 });
    await reply(2, { items: [{ id: "bob-late" }], total: 7 });
    expect(page.items).toEqual([]);
    await reply(3, { items: [{ id: "alice-current" }], total: 1 });
    expect(page.items).toEqual([{ id: "alice-current" }]);
  });

  it("discards the cursor and count on refusal, then waits for an authorized first page", async () => {
    await show("alice");
    await reply(0, { items: [{ id: "private" }], total: 4, next_cursor: "private-cursor" });
    await reload("private-cursor");
    await reply(1, { error: { message: "refused" } }, 403);
    expect(page.items).toEqual([]);
    expect(page.total).toBe(0);
    expect(page.next).toBe("");
    expect(page.hasSnapshot).toBe(false);
    expect(page.accessLost).toBe(true);

    await retry();
    expect(requests[2].url.searchParams.has("cursor")).toBe(false);
    expect(page.accessLost).toBe(true);
    await reply(2, { error: { message: "temporarily unavailable" } }, 503);
    expect(page.accessLost).toBe(true);
    expect(page.hasSnapshot).toBe(false);

    await retry();
    await reply(3, { items: [], total: 0 });
    expect(page.accessLost).toBe(false);
    expect(page.hasSnapshot).toBe(true);
    expect(page.total).toBe(0);
  });

  it("retains prior rows on a malformed continuation and retries the same page", async () => {
    await show("alice");
    await reply(0, { items: [{ id: "first" }], total: 2, next_cursor: "second-cursor" });
    await reload("second-cursor");
    await reply(1, { items: [], total: "unconfirmed" });
    expect(page.items).toEqual([{ id: "first" }]);
    expect(page.total).toBe(2);
    expect(page.next).toBe("second-cursor");
    expect(page.hasSnapshot).toBe(true);
    expect(page.accessLost).toBe(false);
    expect(requests).toHaveLength(2);

    await retry();
    expect(requests[2].url.searchParams.get("cursor")).toBe("second-cursor");
    await reply(2, { items: [{ id: "second" }], total: 2 });
    expect(page.items).toEqual([{ id: "first" }, { id: "second" }]);
    expect(page.next).toBe("");
  });
});
