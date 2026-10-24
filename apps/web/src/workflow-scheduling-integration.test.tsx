// @vitest-environment jsdom
import { act, StrictMode, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactFlowProps } from "@xyflow/react";
import { WorkflowView } from "./Workflow";
import { App } from "./App";
import { setLocale } from "./i18n";
const layout = vi.hoisted(() => ({
  current: undefined as ReactFlowProps | undefined,
}));
vi.mock("@xyflow/react", () => ({
  ReactFlow: (props: ReactFlowProps & { children: ReactNode }) => {
    layout.current = props;
    return <div data-testid="layout-only-placeholder">{props.children}</div>;
  },
  Background: () => null,
  Controls: () => null,
  MiniMap: () => null,
  Handle: () => null,
  Position: { Left: "left", Right: "right" },
  MarkerType: { ArrowClosed: "arrowclosed" },
  applyNodeChanges: (_changes: unknown, nodes: unknown) => nodes,
  applyEdgeChanges: (_changes: unknown, edges: unknown) => edges,
}));
vi.mock("elkjs/lib/elk.bundled.js", () => ({
  default: class {
    async layout(graph: { children: { id: string }[] }) {
      return {
        ...graph,
        children: graph.children.map((node, index) => ({
          ...node,
          x: index * 240,
          y: 0,
        })),
      };
    }
  },
}));
let root: Root | undefined,
  container: HTMLDivElement,
  version: number,
  admissionReads: number,
  runReads: number,
  includeAttempts: boolean,
  pooledRoute: boolean,
  validatedDefinition:
    | { nodes: { id: string; priority?: number }[] }
    | undefined,
  streams: Set<ReadableStreamDefaultController<Uint8Array>>,
  errors: string[];
const definition = {
  id: "workflow",
  name: "持久调度刷新",
  version: 1,
  nodes: [
    {
      id: "api",
      name: "API 执行",
      type: "command",
      command: ["api-command"],
      priority: 3,
    },
    { id: "ui", name: "UI 执行", type: "command", command: ["ui-command"] },
  ],
  edges: [],
};
const node = (id: string) => ({
  id,
  name: id === "api" ? "API 执行" : "UI 执行",
  type: "command",
  command: [id + "-command"],
  state: "ready",
  attempt: 0,
  fence: 0,
  worker_id: "",
  reason: "run-read-" + version,
  idempotent: false,
  compensatable: false,
  output: "",
  error: "",
  started_at: "",
  completed_at: "",
});
const run = () => ({
  id: "run",
  ...(pooledRoute
    ? {
        execution_pool_id: "a".repeat(32),
        execution_route_version: 2,
        execution_pool_grant_id: "e".repeat(32),
        execution_pool_grant_version: 4,
      }
    : {}),
  workflow_id: "workflow",
  workflow_name: "持久调度刷新",
  workflow_version: 1,
  project_id: "project",
  tenant_id: "tenant",
  version,
  state: "running",
  created_at: "2026-10-24T06:00:00Z",
  updated_at: "2026-10-24T06:00:00Z",
  nodes: [node("api"), node("ui")],
});
const response = (value: unknown) =>
  new Response(JSON.stringify(value), {
    headers: { "Content-Type": "application/json" },
  });
beforeEach(async () => {
  await setLocale("zh-CN", { persist: false });
  vi.useFakeTimers({
    toFake: [
      "setTimeout",
      "clearTimeout",
      "setInterval",
      "clearInterval",
      "Date",
    ],
  });
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  version = 1;
  admissionReads = 0;
  runReads = 0;
  includeAttempts = false;
  pooledRoute = false;
  validatedDefinition = undefined;
  layout.current = undefined;
  history.replaceState(null, "", "/");
  localStorage.clear();
  sessionStorage.clear();
  streams = new Set();
  errors = [];
  container = document.createElement("div");
  document.body.appendChild(container);
  vi.spyOn(console, "error").mockImplementation((...values) => {
    errors.push(values.map(String).join(" "));
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: unknown, options?: RequestInit) => {
      const url = new URL(String(input), "http://local"),
        path = url.pathname.replace("/api/v1", "");
      if (path === "/workflows/validate") {
        validatedDefinition = JSON.parse(String(options?.body));
        return response({ valid: true, errors: [] });
      }
      if (path === "/auth/session")
        return response({
          user: {
            id: "user",
            name: "键盘测试",
            email: "qa@example.test",
            role: "owner",
          },
          organization: { id: "organization", name: "测试组织" },
          projects: [{ id: "project", version: 1, title: "测试项目" }],
        });
      if (path === "/organizations") return response({ items: [], total: 0 });
      if (path === "/overview")
        return response({
          counts: { requirements: 0, tasks: 0 },
          activity: [],
        });
      if (path === "/workflows")
        return response({ items: [definition], total: 1 });
      if (path === "/workflows/workflow") return response(definition);
      if (path === "/tasks") return response({ items: [], total: 0 });
      if (path === "/runs") return response({ items: [run()], total: 1 });
      if (path === "/runs/run") {
        runReads++;
        return response(run());
      }
      if (path === "/runs/run/events/stream") {
        let closed = false;
        let current: ReadableStreamDefaultController<Uint8Array>;
        const stream = new ReadableStream<Uint8Array>({
          start(controller) {
            current = controller;
            streams.add(controller);
            options?.signal?.addEventListener(
              "abort",
              () => {
                if (!closed) {
                  closed = true;
                  streams.delete(controller);
                  controller.close();
                }
              },
              { once: true },
            );
          },
          cancel() {
            closed = true;
            streams.delete(current);
          },
        });
        return new Response(stream, {
          headers: { "Content-Type": "text/event-stream" },
        });
      }
      const selected = path.match(
        /^\/runs\/run\/nodes\/(api|ui)\/scheduling$/,
      )?.[1];
      if (selected) {
        admissionReads++;
        return response({
          run_id: "run",
          node_id: selected,
          run_version: version,
          node_state: "ready",
          attempt: 0,
          fence: 0,
          policy_version: 1,
          evaluated_at: "2026-10-24T06:00:00Z",
          status: "blocked",
          blockers: [
            {
              code: "custom_qa",
              source: "policy",
              message: "diagnostic-" + selected + "-" + version,
              resource: "execution_slots",
              required: 1,
              capacity: 0,
              used: 0,
              available: 0,
              unit: "slots",
            },
          ],
          requests: [],
          reservations: [],
        });
      }
      const attempts = path.match(
        /^\/runs\/run\/nodes\/(api|ui)\/attempts(?:\/(1))?$/,
      );
      if (attempts) {
        const item = {
          run_id: "run",
          node_id: attempts[1],
          attempt: 1,
          fence: 1,
          worker_id: "worker",
          state: "succeeded",
          result_source: "unreported",
          started_at: "2026-10-24T06:00:00Z",
          completed_at: "2026-10-24T06:00:01Z",
        };
        return response(
          attempts[2]
            ? item
            : {
                items: includeAttempts ? [item] : [],
                total: includeAttempts ? 1 : 0,
              },
        );
      }
      throw Error("Unexpected real Workflow fixture request " + path);
    }),
  );
});
function graphSelection(ids: string[]) {
  const props = layout.current!;
  expect(
    props.onSelectionChange,
    "official ReactFlow selection callback must be connected",
  ).toBeTypeOf("function");
  props.onSelectionChange!({
    nodes: ids.map((id) => props.nodes!.find((node) => node.id === id)!),
    edges: [],
  });
}
describe("real WorkflowView graph selection and actual App routes", () => {
  it("keeps saved attempt tools scoped through actual App links, Back, reload and close while current attempt remains separate", async () => {
    includeAttempts = true;
    const original=vi.mocked(fetch).getMockImplementation()!;
    const saved=(attempt:number)=>({run_id:'run',node_id:'api',attempt,fence:attempt,worker_id:'worker-'+attempt,state:'failed',result_source:'unreported',started_at:'2026-10-25T00:00:00Z',completed_at:'2026-10-25T00:00:01Z'});
    vi.mocked(fetch).mockImplementation(async(input,options)=>{
      const url=new URL(String(input),'http://local'),path=url.pathname.replace('/api/v1','');
      if(path==='/runs/run')return response({...run(),nodes:[{...node('api'),attempt:2,fence:2,worker_id:'worker-2'},node('ui')]});
      if(path==='/runs/run/nodes/api/attempts')return response({items:[saved(1),saved(2)],total:2});
      const match=path.match(/^\/runs\/run\/nodes\/api\/attempts\/([12])(\/tool-events)?$/);
      if(match){const attempt=Number(match[1]),record=saved(attempt);if(!match[2])return response(record);const data={attempt,fence:attempt,worker_id:record.worker_id,tool_event:{legacy:'saved-only-attempt-'+attempt}};return response({schema_version:1,project_id:'project',run_id:'run',node_id:'api',attempt,fence:attempt,worker_id:record.worker_id,page_cursor:'saved-'+attempt,watermark:attempt,scanned_through:attempt,has_more:false,items:[{sequence:attempt,run_id:'run',node_id:'api',type:'tool_progress',at:'2026-10-25T00:00:00Z',data,raw_data:JSON.stringify(data)}]});}
      return original(input,options);
    });
    history.replaceState(null,'','/?project=project&view=development&sub=runs&run=run&node=api&attempt=1');
    await act(async()=>{root=createRoot(container);root.render(<App/>);});await advance(250);
    const savedRegion=()=>container.querySelector('.attempt-tool-history')!;
    expect(savedRegion().textContent).toContain('saved-only-attempt-1');
    expect(savedRegion().textContent).not.toContain('saved-only-attempt-2');
    expect(container.textContent).toContain('当前尝试 2');
    const count=vi.mocked(fetch).mock.calls.length;
    await act(async()=>{await setLocale('en-US',{persist:false});});await advance(1);
    expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
    expect(container.textContent).toContain('Current attempt 2');
    const link=container.querySelector<HTMLAnchorElement>('.scheduling-attempts a[href*="attempt=2"]')!;
    await act(async()=>link.click());await advance(50);
    expect(new URLSearchParams(location.search).get('attempt')).toBe('2');expect(savedRegion().textContent).toContain('saved-only-attempt-2');
    await act(async()=>history.back());await advance(25);
    expect(new URLSearchParams(location.search).get('attempt')).toBe('1');expect(savedRegion().textContent).toContain('saved-only-attempt-1');
    await act(async()=>root!.unmount());await act(async()=>{root=createRoot(container);root.render(<App/>);});await advance(250);
    expect(savedRegion().textContent).toContain('saved-only-attempt-1');
    const close=[...container.querySelectorAll('button')].find(button=>button.textContent==='Close historical attempt details')!;
    expect(close).toBeTruthy();await act(async()=>close.click());await advance(5);
    expect(container.querySelector('.attempt-tool-history')).toBeNull();expect(document.activeElement).toBe(container.querySelector('.scheduling-attempts'));
    expect(vi.mocked(fetch).mock.calls.every(([,options])=>!options?.method||options.method==='GET')).toBe(true);
  });
  it("shows the exact saved run route and grant without claiming access to the owning pool", async () => {
    pooledRoute = true;
    await render();
    await advance(250);
    const facts = container.querySelector(
      '[aria-label="本次运行固定执行路由"]',
    );
    expect(facts).not.toBeNull();
    expect(facts?.textContent).toContain("a".repeat(32));
    expect(facts?.textContent).toContain("路由v2");
    expect(facts?.textContent).toContain("授权v4");
    expect(facts?.querySelector("a")).toBeNull();
  });
  it("edits a real workflow node priority within the frozen0..9 contract and sends it to server validation", async () => {
    await act(async () => {
      root = createRoot(container);
      root.render(
        <WorkflowView
          project="project"
          refresh={0}
          onNode={() => {}}
          onRun={() => {}}
          onRefresh={() => {}}
        />,
      );
    });
    await advance(250);
    const workflow = container.querySelector<HTMLSelectElement>(
      '[aria-label="工作流版本"]',
    )!;
    await act(async () => {
      workflow.value = "workflow@1";
      workflow.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await advance(250);
    const nodeChoice = [...container.querySelectorAll("label.field")]
      .find((label) => label.firstElementChild?.textContent === "选择节点")
      ?.querySelector("select")!;
    await act(async () => {
      nodeChoice.value = "api";
      nodeChoice.dispatchEvent(new Event("change", { bubbles: true }));
    });
    const priority = container.querySelector<HTMLInputElement>(
      '[name="node_priority"]',
    );
    expect(priority).not.toBeNull();
    expect(priority!.value).toBe("3");
    expect(priority!.min).toBe("0");
    expect(priority!.max).toBe("9");
    await act(async () => {
      Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!.call(priority, "8");
      priority!.dispatchEvent(new Event("input", { bubbles: true }));
    });
    const validate = [...container.querySelectorAll("button")].find(
      (button) => button.textContent === "校验",
    )!;
    await act(async () => validate.click());
    await advance(1);
    expect(
      validatedDefinition?.nodes.find((node) => node.id === "api")?.priority,
    ).toBe(8);
    expect(container.textContent).toContain("仅影响同一运行内");
  });
  it("connects the official single-node keyboard selection callback and deduplicates the mouse notification", async () => {
    const onNode = vi.fn();
    await act(async () => {
      root = createRoot(container);
      root.render(
        <WorkflowView
          project="project"
          refresh={0}
          runId="run"
          nodeId="api"
          onNode={onNode}
          onRun={() => {}}
          onRefresh={() => {}}
        />,
      );
    });
    await advance(250);
    await act(async () => {
      graphSelection(["ui"]);
      layout.current!.onNodeClick!(
        new MouseEvent("click") as never,
        layout.current!.nodes!.find((node) => node.id === "ui")!,
      );
    });
    expect(onNode.mock.calls).toEqual([["ui"]]);
    expect(container.querySelector(".canvas-inspector h3")?.textContent).toBe(
      "UI 执行",
    );
  });
  it("updates actual App node/attempt links once and preserves them across empty, repeated and live-snapshot selection notifications", async () => {
    includeAttempts = true;
    history.replaceState(
      null,
      "",
      "/?project=project&view=development&sub=runs&run=run&node=api&attempt=1",
    );
    await act(async () => {
      root = createRoot(container);
      root.render(<App />);
    });
    await advance(250);
    expect(
      container.querySelector('[aria-label="所选历史尝试详情"]'),
    ).not.toBeNull();
    const selectionHandler = layout.current!.onSelectionChange;
    const push = vi.spyOn(history, "pushState");
    await act(async () => graphSelection(["ui"]));
    expect(new URLSearchParams(location.search).get("node")).toBe("ui");
    expect(new URLSearchParams(location.search).has("attempt")).toBe(false);
    expect(push).toHaveBeenCalledTimes(1);
    const attemptLink = container.querySelector<HTMLAnchorElement>(
      '.scheduling-attempts a[href*="attempt=1"]',
    )!;
    await act(async () => attemptLink.click());
    expect(new URLSearchParams(location.search).get("attempt")).toBe("1");
    push.mockClear();
    for (let index = 2; index <= 4; index++) {
      version = index;
      await act(async () => {
        event(index);
        graphSelection([]);
        graphSelection(["ui"]);
        graphSelection(["api", "ui"]);
      });
      await advance(15250);
      await act(async () => graphSelection(["ui"]));
      singleFacts("keyboard selected snapshot " + index);
      expect(new URLSearchParams(location.search).get("node")).toBe("ui");
      expect(new URLSearchParams(location.search).get("attempt")).toBe("1");
      expect(container.querySelector(".canvas-inspector h3")?.textContent).toBe(
        "UI 执行",
      );
      expect(layout.current!.onSelectionChange).toBe(selectionHandler);
    }
    expect(push).not.toHaveBeenCalled();
    expect(
      container.querySelector('[aria-label="所选历史尝试详情"]'),
    ).not.toBeNull();
    expect(
      layout.current!.nodes!.find((node) => node.id === "ui")?.selected,
    ).toBe(true);
    await act(async () => history.back());
    await advance(10);
    expect(new URLSearchParams(location.search).get("node")).toBe("ui");
    expect(new URLSearchParams(location.search).has("attempt")).toBe(false);
    await act(async () => history.back());
    await advance(10);
    expect(new URLSearchParams(location.search).get("node")).toBe("api");
    expect(new URLSearchParams(location.search).get("attempt")).toBe("1");
    expect(
      layout.current!.nodes!.find((node) => node.id === "api")?.selected,
    ).toBe(true);
    expect(container.querySelector(".canvas-inspector h3")?.textContent).toBe(
      "API 执行",
    );
    expect(push).not.toHaveBeenCalled();
  });
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount());
  root = undefined;
  container.remove();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
async function render(nodeId = "api") {
  await act(async () => {
    if (!root) root = createRoot(container);
    root.render(
      <StrictMode>
        <WorkflowView
          project="project"
          refresh={0}
          runId="run"
          nodeId={nodeId}
          onNode={() => {}}
          onRun={() => {}}
          onRefresh={() => {}}
        />
      </StrictMode>,
    );
  });
}
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}
function singleFacts(label: string) {
  expect(
    container.querySelectorAll(".node-scheduling-facts"),
    label +
      " " +
      errors
        .filter((message) => message.includes("same key"))
        .slice(-1)
        .join(" "),
  ).toHaveLength(1);
  expect(
    container.querySelectorAll('[aria-label="持久尝试历史"]'),
    label,
  ).toHaveLength(1);
  expect(
    container.querySelectorAll('[aria-label="当前派发条件"]'),
    label,
  ).toHaveLength(1);
}
function event(sequence: number) {
  const value = {
    sequence,
    run_id: "run",
    node_id: "api",
    type: "node_ready",
    at: "2026-10-24T06:00:00Z",
    data: { version },
  };
  for (const controller of streams)
    controller.enqueue(
      new TextEncoder().encode(
        "event: run_event\nid: " +
          sequence +
          "\ndata: " +
          JSON.stringify(value) +
          "\n\n",
      ),
    );
}
describe("real WorkflowView keeps a single scheduling inspector across live snapshots", () => {
  it("replaces the real inspector through repeated10s diagnostic and15s run snapshot updates", async () => {
    await render();
    await advance(250);
    singleFacts("initial");
    for (let index = 2; index <= 10; index++) {
      version = index;
      await act(async () => event(index));
      await advance(15250);
      singleFacts("snapshot " + index);
      expect(container.textContent).toContain("diagnostic-api-" + index);
      expect(
        container.querySelector(".node-scheduling-facts")!.textContent,
      ).not.toContain("diagnostic-api-" + (index - 1));
    }
    expect(admissionReads).toBeGreaterThan(8);
    expect(runReads).toBeGreaterThan(8);
    expect(
      errors.filter((message) => message.includes("same key")),
    ).toHaveLength(0);
  });
  it("replaces the selected real node instead of leaving old keyed inspector trees attached", async () => {
    await render();
    await advance(250);
    singleFacts("api");
    await render("ui");
    await advance(250);
    singleFacts("ui");
    expect(
      container.querySelector(".node-scheduling-facts")!.textContent,
    ).toContain("diagnostic-ui-1");
    expect(
      container.querySelector(".node-scheduling-facts")!.textContent,
    ).not.toContain("diagnostic-api-1");
    expect(
      errors.filter((message) => message.includes("same key")),
    ).toHaveLength(0);
  });
});
