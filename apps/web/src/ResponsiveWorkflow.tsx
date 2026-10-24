import { useState, useSyncExternalStore } from "react";
import { ChevronRight, Search } from "lucide-react";
import { stateLabel } from "./api";
import "./responsive-workflow.css";
export type WorkflowListNode = {
  id: string;
  name: string;
  type: string;
  state?: string;
  attempt?: number;
  fence?: number;
  reason?: string;
};
export type WorkflowRunSummaryData = {
  id: string;
  workflow_name: string;
  state: string;
};
const compactQuery = "(max-width:850px)";
function compactSnapshot() {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia(compactQuery).matches
  );
}
function subscribeCompact(listener: () => void) {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function")
    return () => {};
  const media = window.matchMedia(compactQuery);
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
}
export function useCompactWorkflow() {
  return useSyncExternalStore(subscribeCompact, compactSnapshot, () => false);
}
export function workflowNodeButtonID(id: string) {
  return "workflow-node-choice-" + encodeURIComponent(id);
}
function nodeState(state?: string) {
  return state === "ready"
    ? "等待派发"
    : state === "pending"
      ? "等待依赖"
      : stateLabel(state);
}
function attemptLabel(node: WorkflowListNode) {
  return node.attempt && node.attempt > 0
    ? "当前尝试 " + node.attempt
    : "尚未领取";
}
export function filterWorkflowNodes(nodes: WorkflowListNode[], query: string) {
  const text = query.trim().toLocaleLowerCase();
  return text
    ? nodes.filter((node) =>
        (node.id + " " + node.name + " " + nodeState(node.state))
          .toLocaleLowerCase()
          .includes(text),
      )
    : nodes;
}
export function WorkflowNodeList({
  nodes,
  selected,
  onSelect,
}: {
  nodes: WorkflowListNode[];
  selected?: string;
  onSelect: (id: string) => void;
}) {
  const [search, setSearch] = useState(""),
    visible = filterWorkflowNodes(nodes, search);
  return (
    <section className="workflow-node-list" aria-label="完整执行节点列表">
      <div className="workflow-list-heading">
        <h3>执行节点</h3>
        <small>完整范围 {nodes.length} 个节点</small>
      </div>
      <label className="workflow-node-search">
        <Search size={15} />
        <input
          type="search"
          aria-label="查找执行节点"
          placeholder="按节点名称、标识或状态查找…"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      {search && (
        <p className="muted" role="status">
          匹配 {visible.length} / {nodes.length} 个正式节点；未截断原始范围。
        </p>
      )}
      <ul>
        {visible.map((node) => (
          <li key={node.id}>
            <button
              type="button"
              id={workflowNodeButtonID(node.id)}
              aria-pressed={selected === node.id}
              onClick={() => onSelect(node.id)}
            >
              <span className="workflow-node-choice-text">
                <strong>{node.name}</strong>
                <small>
                  {attemptLabel(node)}
                  {node.fence && node.fence > 0 ? " · 租约 " + node.fence : ""}
                </small>
                <code>{node.id}</code>
                {node.reason && (
                  <span className="workflow-node-reason">{node.reason}</span>
                )}
              </span>
              <span className="workflow-node-choice-status">
                <span className={"badge state-" + node.state}>
                  {nodeState(node.state)}
                </span>
                <ChevronRight size={16} />
              </span>
            </button>
          </li>
        ))}
      </ul>
      {!visible.length && (
        <p className="workflow-node-empty">
          {search
            ? "完整范围内没有匹配节点，请调整查找内容。"
            : "正在等待当前运行的正式节点。"}
        </p>
      )}
    </section>
  );
}
export function WorkflowRunSummary({
  run,
  node,
}: {
  run: WorkflowRunSummaryData;
  node?: WorkflowListNode;
}) {
  return (
    <section className="workflow-run-summary" aria-label="当前运行与节点">
      <div className="workflow-run-summary-heading">
        <h2>{run.workflow_name}</h2>
        <span className={"badge state-" + run.state}>
          {stateLabel(run.state)}
        </span>
      </div>
      <p className="workflow-run-id">
        运行 <code>{run.id}</code>
      </p>
      {node ? (
        <div className="workflow-selected-context">
          <strong>当前节点：{node.name}</strong>
          <p>
            {nodeState(node.state)} · {attemptLabel(node)}
          </p>
          <code>{node.id}</code>
        </div>
      ) : (
        <p className="muted">
          尚未选择节点。可从完整列表进入详情、查看证据和核对操作。
        </p>
      )}
    </section>
  );
}
