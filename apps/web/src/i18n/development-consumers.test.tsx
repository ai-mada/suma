// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { WorkflowView } from "../Workflow";
import { ToolWorkspace } from "../ToolWorkspace";
import { TaskDispatchDialog } from "../TaskDispatch";
import { ToolExecutionFacts } from "../ToolFacts";
import { CandidateSource } from "../CandidateSource";
import { WorkflowNodeList } from "../ResponsiveWorkflow";
import { setLocale, setServerCatalog } from "./index";
import type { Session, Workflow, RecordItem } from "../api";
import type { ToolProfile } from "../tool-model";
import type { ReactFlowProps } from "@xyflow/react";

const graph = vi.hoisted(() => ({ current: undefined as ReactFlowProps | undefined }));

// The graph renderer needs browser geometry; leave all workflow controls and
// state real while replacing only layout/canvas dependencies in jsdom.
vi.mock("@xyflow/react", () => ({
  ReactFlow: (props: ReactFlowProps & { children: ReactNode }) => { graph.current = props; return <div>{props.children}</div>; },
  Background: () => null, Controls: () => null, MiniMap: () => null,
  Handle: () => null, Position: { Left: "left", Right: "right" },
  MarkerType: { ArrowClosed: "arrowclosed" },
  applyNodeChanges: (_: unknown, nodes: unknown) => nodes,
  applyEdgeChanges: (_: unknown, edges: unknown) => edges,
}));
vi.mock("elkjs/lib/elk.bundled.js", () => ({ default: class {
  async layout(graph: object) { return graph; }
} }));
const project = "development-project";
const session: Session = { user: { id: "user", name: "Owner", email: "owner@example.test", role: "owner" }, organization: { id: "org", name: "Organization" }, projects: [{ id: project, version: 1, title: "Project", role: "admin" }] };
const task = { id: "task-original", project_id: project, title: "原始任务 <strong>原文</strong>", version: 4 } as RecordItem;
const flow: Workflow = { id: "flow-original", name: "原始流程", version: 3, nodes: [
  { id: "agent-original", name: "原始开发节点", type: "agent" },
  { id: "test-original", name: "原始独立测试", type: "test", command: ["test-original"], config: { candidate_from: "agent-original" } },
], edges: [] };
const profile = { id: "profile-original", project_id: project, worker_id: "runner-original", name: "原始工具", version: 5, state: "enabled", available: true,
  profile: { command_id: "command-original", adapter: "codex", protocol: "codex-jsonl-v1", profile_sha256: "a".repeat(64), executable_sha256: "b".repeat(64), tool_version: "original-version", read_only: false },
} as ToolProfile;
let host: HTMLDivElement, root: Root | undefined, writes: {path: string; body: Record<string, unknown>}[];
beforeEach(async () => {
  localStorage.clear(); sessionStorage.clear(); writes = []; graph.current = undefined;
  await setLocale("en-US", { persist: false });
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div"); document.body.append(host);
  vi.stubGlobal("fetch", vi.fn(async (input: string, options?: RequestInit) => {
    const path = new URL(String(input), location.href).pathname.replace(/^\/api\/v1/, "");
    const body = options?.body ? JSON.parse(String(options.body)) : undefined;
    let value: unknown;
    if (path.endsWith("/events/stream")) return new Response("{}", {status: 422});
    if (body) { writes.push({path, body}); value = path === "/release-workers" ? { worker_id: body.worker_id } : { id: "plan-original", task_id: task.id }; }
    else if (path === "/workflows") value = {items: [flow], total: 1};
    else if (path === "/workflows/flow-original") value = flow;
    else if (path === "/runs/run-original") value = {id: "run-original",project_id: project,workflow_id:flow.id,workflow_name:flow.name,workflow_version:flow.version,version:7,state:"running",nodes:[],created_at:"2026-10-24T00:00:00Z",updated_at:"2026-10-24T00:00:00Z"};
    else if (path === "/tool-profiles") value = {items: [profile], total: 1};
    else if (["/runs", "/tasks", "/release-workers", "/project-execution/runners"].includes(path)) value = {items: [], total: 0};
    else throw Error("Unexpected development fixture " + path);
    return new Response(JSON.stringify(value), {headers: {"Content-Type": "application/json"}});
  }));
});
afterEach(async () => {
  if (root) await act(async () => root?.unmount()); root = undefined;
  host.remove(); await setLocale("zh-CN", {persist: false});
  vi.unstubAllGlobals(); vi.restoreAllMocks();
});
async function settle() { for (let i = 0; i < 5; i++) await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
async function render(node: ReactNode) { await act(async () => {root = createRoot(host); root.render(node);}); await settle(); }
async function click(text: string) {
  const button = [...document.querySelectorAll<HTMLButtonElement>("button")].find(item => item.textContent?.trim() === text);
  expect(button, "button " + text).toBeDefined(); await act(async () => button!.click()); await settle();
}
async function change(element: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement, value: string) {
  await act(async () => { Object.getOwnPropertyDescriptor(element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype, "value")!.set!.call(element, value); element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? "change" : "input", {bubbles: true})); });
}
it("switches workflow controls without replacing its edited node draft", async () => {
  await render(<WorkflowView project={project} refresh={0} onRefresh={() => {}} onNode={() => {}} onRun={() => {}} />);
  await click("Add the first node");
  const input = host.querySelector<HTMLInputElement>('input[aria-label="Node name"]')!;
  expect(input).not.toBeNull(); await change(input, "用户节点草稿 <code>");
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(input.isConnected).toBe(true); expect(input.value).toBe("用户节点草稿 <code>");
  expect(host.textContent).toContain("删除节点"); expect(writes).toHaveLength(0);
});
it("relocalizes structured workflow validation without resubmitting or losing the draft", async () => {
  setServerCatalog({version:"fixture",template_format:"named-dot-v1",default_locale:"zh-CN",supported_locales:["zh-CN","en-US"],messages:[{key:"dag.command.single",parameters:[],texts:{"zh-CN":"必须指定一条命令。","en-US":"Specify exactly one command."}}]});
  const base = vi.mocked(fetch).getMockImplementation()!;
  vi.mocked(fetch).mockImplementation(async(input,options)=> {
    if(String(input).includes("/workflows/validate")) {
      writes.push({path:"/workflows/validate",body:JSON.parse(String(options?.body))});
      return new Response(JSON.stringify({valid:false,errors:["invalid: one command required"],diagnostics:[{message:"必须指定一条命令。",message_key:"dag.command.single",locale:"zh-CN",localized:true}]}));
    }
    return base(input,options);
  });
  await render(<WorkflowView project={project} refresh={0} onRefresh={()=>{}} onNode={()=>{}} onRun={()=>{}}/>);
  await click("Add the first node");
  const input=host.querySelector<HTMLInputElement>('input[aria-label="Node name"]')!;
  await change(input,"用户未发布草稿");
  await click("Validate");
  expect(host.textContent).toContain("Specify exactly one command.");
  const requests=vi.mocked(fetch).mock.calls.length;
  await act(async()=>{await setLocale("zh-CN",{persist:false});});
  expect(host.textContent).toContain("必须指定一条命令。");
  expect(input.isConnected).toBe(true);expect(input.value).toBe("用户未发布草稿");
  expect(writes).toHaveLength(1);expect((writes[0].body.workflow as Workflow).nodes[0].name).toBe("用户未发布草稿");
  expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
});
it("keeps a tool worker draft and submits its unchanged machine identity after switching", async () => {
  await render(<ToolWorkspace project={project} session={session} selected="" panel="" refresh={0} onSelect={() => {}} />);
  await click("Register execution worker");
  const input = document.querySelector<HTMLInputElement>('[role=dialog] input[placeholder="e.g. team-code-runner"]')!;
  expect(input).not.toBeNull(); await change(input, "unchanged-worker");
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(input.isConnected).toBe(true); expect(input.value).toBe("unchanged-worker");
  await click("登记并获取本次凭证");
  expect(writes[0]).toMatchObject({path: "/release-workers", body: {project_id: project, worker_id: "unchanged-worker", profiles: [], deployment: false}});
  expect(writes[0].body.idempotency_key).toEqual(expect.any(String));
});
it("preserves dispatch selections, request versions and raw tool evidence across locale changes", async () => {
  await render(<><TaskDispatchDialog project={project} session={session} task={task} planID="new" initialWorkflow={flow} onPlan={() => {}} onClose={() => {}} onRun={() => {}} /><ToolExecutionFacts state="running" events={[{schema_version: 1, kind: "message", command_id: "command-original", profile_sha256: "a".repeat(64), protocol: "codex-jsonl-v1", message: "原始工具输出 <strong>保留</strong>"}]} /></>);
  expect(document.body.textContent).toContain("Arrange task execution");
  const selects = [...document.querySelectorAll<HTMLSelectElement>('.tool-dispatch select')];
  await change(selects[2], profile.id);
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(selects.map(item => item.value)).toEqual(["flow-original@3", "agent-original", "profile-original"]);
  expect(document.body.textContent).toContain("原始工具输出 <strong>保留</strong>");
  expect(host.querySelector(".tool-event-list strong")?.textContent).toBe("工具消息");
  await click("固定并预览执行计划");
  expect(writes[0]).toMatchObject({path: "/task-dispatch-plans", body: {project_id: project, task_id: "task-original", expected_task_version: 4, workflow_id: "flow-original", workflow_version: 3, agent_node_id: "agent-original", tool_profile_id: "profile-original", expected_tool_version: 5}});
});
it("updates candidate validation and complete node states without replacing their raw identities", async () => {
  const invalid = {...flow, nodes: flow.nodes.map(node => node.id === "test-original" ? {...node, config: {candidate_from: "missing-original-source"}} : node)};
  await render(<><CandidateSource workflow={invalid} nodeID="test-original" onChange={() => {}}/><WorkflowNodeList nodes={[{id: "raw-node",name: "用户节点原名",type: "command",state: "ready",attempt: 3,fence: 4,reason: "原始后端诊断"}]} selected="raw-node" onSelect={() => {}}/></>);
  expect(host.textContent).toContain("The candidate source is no longer a valid ancestor");
  expect(host.textContent).toContain("Awaiting dispatch");
  const selected = host.querySelector('button[aria-pressed="true"]');
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(selected?.isConnected).toBe(true);
  expect(host.textContent).toContain("候选来源已不在当前合法上游中");
  expect(host.textContent).toContain("原始后端诊断");
  expect(host.textContent).toContain("用户节点原名");
  expect(writes).toHaveLength(0);
});
it("updates a live stream failure while retaining the same selected run and request count", async () => {
  await render(<WorkflowView project={project} runId="run-original" refresh={0} onRefresh={() => {}} onNode={() => {}} onRun={() => {}} />);
  await act(async () => {await new Promise(resolve => setTimeout(resolve, 180));});
  expect(host.textContent).toContain("Live event request failed (422)");
  const requests = vi.mocked(fetch).mock.calls.length;
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(host.textContent).toContain("实时事件请求失败（422）");
  expect(host.textContent).toContain("实时事件已停止");
  expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);
  expect(host.textContent).toContain("run-original");
  expect(writes).toHaveLength(0);
});
it("relocalizes a local manifest refusal while retaining the unsubmitted original inventory", async () => {
  await render(<ToolWorkspace project={project} session={session} selected="" panel="" refresh={0} onSelect={() => {}} />);
  await click("Register configuration");
  const manifest = document.querySelector<HTMLTextAreaElement>('[role=dialog] textarea')!;
  await change(manifest, "{ 用户清单原件 }");
  await click("Validate inventory fields");
  expect(document.body.textContent).toContain("The tool inventory is not valid JSON.");
  await act(async () => {await setLocale("zh-CN", {persist: false});});
  expect(document.body.textContent).toContain("工具清单不是有效 JSON。");
  expect(manifest.isConnected).toBe(true); expect(manifest.value).toBe("{ 用户清单原件 }");
  expect(writes).toHaveLength(0);
});

// Independent names for the events currently emitted by the DAG, including
// its node-state, run-terminal, command and parent-command event families.
const platformEventCases = [
  ["run_created", "Run created", "运行已创建"],
  ["run_succeeded", "Run succeeded", "运行成功"],
  ["run_failed", "Run failed", "运行失败"],
  ["run_cancelled", "Run cancelled", "运行已取消"],
  ["node_pending", "Node pending", "节点等待依赖"],
  ["node_ready", "Node ready for dispatch", "节点已就绪，等待派发"],
  ["node_running", "Node running", "节点执行中"],
  ["node_waiting", "Node waiting", "节点等待中"],
  ["node_reconciling", "Node result needs reconciliation", "节点结果待核对"],
  ["node_succeeded", "Node succeeded", "节点成功"],
  ["node_failed", "Node failed", "节点失败"],
  ["node_skipped", "Node skipped", "节点已跳过"],
  ["node_cancelled", "Node cancelled", "节点已取消"],
  ["command_pause", "Run pause requested", "已请求暂停运行"],
  ["command_resume", "Run resume requested", "已请求继续运行"],
  ["command_cancel", "Run cancellation requested", "已请求取消运行"],
  ["command_approval", "Approval command recorded", "审批操作已记录"],
  ["command_approve", "Approval command recorded", "审批操作已记录"],
  ["command_retry", "Node retry requested", "已请求重试节点"],
  ["command_compensate", "Compensation command recorded", "补偿操作已记录"],
  ["command_reconcile", "Reconciliation command recorded", "核对操作已记录"],
  ["parent_pause", "Parent requested pause", "上级运行已请求暂停"],
  ["parent_resume", "Parent requested resume", "上级运行已请求继续"],
  ["parent_cancel", "Parent requested cancellation", "上级运行已请求取消"],
  ["candidate_pinned", "Candidate commit pinned", "候选提交已固定"],
  ["candidate_waiting", "Waiting for candidate source", "等待候选来源"],
  ["compensation_requested", "Compensation requested", "已请求补偿"],
  ["compensation_state_changed", "Compensation state updated", "补偿状态已更新"],
  ["tool_progress", "Tool facts updated", "工具事实已更新"],
  ["node_progress", "Execution output updated", "执行输出已更新"],
  ["managed_observation_reconciled", "Provider observation reconciled", "提供方观察已核对"],
  ["map_manifest_saved", "Map input manifest saved", "映射输入清单已保存"],
  ["map_chunk_materialized", "Map node batch created", "映射节点批次已创建"],
  ["retry_scheduled", "Retry scheduled", "重试已安排"],
  ["retry_blocked", "Retry blocked", "重试受阻"],
  ["child_run_created", "Child run created", "子运行已创建"],
  ["child_run_completed", "Child run completed", "子运行已结束"],
  ["bound_execution_paused", "Managed child execution paused", "受控子执行已暂停"],
  ["subworkflow_timeout", "Child workflow timed out", "子工作流已超时"],
] as const;
it("localizes known platform events in live and persistent views while retaining protocol types, raw data and graph state", async () => {
  const cases = [...platformEventCases, ["vendor_custom_event", "vendor_custom_event", "vendor_custom_event"], ["constructor", "constructor", "constructor"]] as const;
  const events = cases.map(([type], index) => ({sequence:index+1,run_id:"run-original",node_id:"",type,at:"2026-10-24T00:00:00Z",data:{original:"原始事件数据 <strong>& {{source}}"}}));
  const originalEvents = JSON.stringify(events);
  const baseFetch = vi.mocked(fetch).getMockImplementation()!;
  const graphFlow = {...flow, edges:[{from:"agent-original",to:"test-original"}]};
  vi.mocked(fetch).mockImplementation(async (input, options) => {
    const path = new URL(String(input),location.href).pathname.replace(/^\/api\/v1/,"");
    if(path.endsWith("/events/stream")) {
      let closed=false;
      const stream=new ReadableStream<Uint8Array>({start(controller){
        controller.enqueue(new TextEncoder().encode(events.map(event=>`event: run_event\nid: ${event.sequence}\ndata: ${JSON.stringify(event)}\n\n`).join("")));
        options?.signal?.addEventListener("abort",()=>{if(!closed){closed=true;controller.close();}},{once:true});
      },cancel(){closed=true;}});
      return new Response(stream,{headers:{"Content-Type":"text/event-stream"}});
    }
    const value=path.endsWith("/events")?{items:events,next_cursor:events.length}:path==="/workflows"?{items:[graphFlow],total:1}:path==="/workflows/flow-original"?graphFlow:undefined;
    return value===undefined?baseFetch(input,options):new Response(JSON.stringify(value),{headers:{"Content-Type":"application/json"}});
  });
  await render(<WorkflowView project={project} runId="run-original" refresh={0} onRefresh={()=>{}} onNode={()=>{}} onRun={()=>{}}/>);
  await act(async()=>{await new Promise(resolve=>setTimeout(resolve,180));});
  await click("Show more loaded events");
  const inspector=host.querySelector(".canvas-inspector")!;
  const liveTitles=()=>[...inspector.querySelectorAll(".timeline-item strong")];
  const titleType=(item:Element)=>item.querySelector("code")?.textContent??item.querySelector("span")?.textContent;
  for(const [type,label]of cases){const title=liveTitles().find(item=>titleType(item)===type);expect(title?.querySelector("span")?.textContent,type).toBe(label);}
  expect(graph.current?.["aria-label"]).toBe("Workflow canvas");
  expect(graph.current?.nodes?.find(node=>node.id==="agent-original")?.ariaLabel).toBe("Node 原始开发节点 (agent-original): Not yet executed");
  expect(graph.current?.edges?.[0].ariaLabel).toBe("Dependency from agent-original to test-original");
  await click("View persistent history");
  const history=document.querySelector("[role=dialog]")!;
  for(const [type,label]of cases){const title=[...history.querySelectorAll(".timeline-item strong")].find(item=>titleType(item)===type);expect(title?.querySelector("span")?.textContent,type).toBe(label);}
  expect(history.querySelector("pre")?.textContent).toBe(JSON.stringify(events[0].data,null,2));
  const requests=vi.mocked(fetch).mock.calls.length;
  await act(async()=>{await setLocale("zh-CN",{persist:false});});
  expect(inspector.isConnected).toBe(true);expect(history.isConnected).toBe(true);
  for(const [type,,label]of cases){for(const titles of [liveTitles(),[...history.querySelectorAll(".timeline-item strong")]]){const title=titles.find(item=>titleType(item)===type);expect(title?.querySelector("span")?.textContent,type).toBe(label);}}
  expect(graph.current?.["aria-label"]).toBe("工作流画布");
  expect(graph.current?.nodes?.find(node=>node.id==="agent-original")?.ariaLabel).toBe("节点 原始开发节点 (agent-original)：尚未执行");
  expect(graph.current?.edges?.[0].ariaLabel).toBe("从 agent-original 到 test-original 的依赖");
  expect(host.querySelector<HTMLSelectElement>('select[aria-label="历史运行"]')?.value).toBe("run-original");
  expect(vi.mocked(fetch).mock.calls).toHaveLength(requests);expect(writes).toHaveLength(0);expect(JSON.stringify(events)).toBe(originalEvents);
});
