import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  filterWorkflowNodes,
  WorkflowNodeList,
  WorkflowRunSummary,
  type WorkflowListNode,
} from "./ResponsiveWorkflow";
const nodes: WorkflowListNode[] = Array.from({ length: 135 }, (_, index) => ({
  id: "node-" + index,
  name: "完整节点名称 " + index,
  type: index === 134 ? "agent" : "test",
  state: index === 134 ? "reconciling" : "succeeded",
  attempt: index === 134 ? 2 : 1,
  fence: index === 134 ? 4 : 1,
  reason: index === 134 ? "等待人工核对原进程" : "",
}));
describe("compact workflow run operations", () => {
  it("retains the complete long run name, actual run state and selected node attempt", () => {
    const name = "需要完整换行展示的任务派发运行名称".repeat(6);
    const html = renderToStaticMarkup(
      <WorkflowRunSummary
        run={{ id: "actual-run-id", workflow_name: name, state: "cancelling" }}
        node={nodes[134]}
      />,
    );
    for (const value of [
      name,
      "actual-run-id",
      "取消中",
      "当前节点",
      "完整节点名称 134",
      "当前尝试 2",
      "结果未知",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("100%");
  });
  it("keeps every node keyboard-operable instead of truncating the list at100", () => {
    const html = renderToStaticMarkup(
      <WorkflowNodeList
        nodes={nodes}
        selected="node-134"
        onSelect={() => {}}
      />,
    );
    expect(html).toContain("完整节点名称 134");
    expect(html).toContain("135");
    expect(html.match(/aria-pressed="(?:true|false)"/g)).toHaveLength(135);
    expect(html.match(/aria-pressed="true"/g)).toHaveLength(1);
  });
  it("shows current fence, attempt and reconciliation reason without inventing execution completion", () => {
    const html = renderToStaticMarkup(
      <WorkflowNodeList
        nodes={[nodes[134]]}
        selected="node-134"
        onSelect={() => {}}
      />,
    );
    for (const value of [
      "当前尝试 2",
      "租约 4",
      "结果未知",
      "等待人工核对原进程",
    ])
      expect(html).toContain(value);
    expect(html).not.toContain("已验收");
    expect(html).not.toContain("自动恢复");
  });
  it("searches all actual nodes including IDs and real state labels", () => {
    expect(filterWorkflowNodes(nodes, "node-134")).toEqual([nodes[134]]);
    expect(filterWorkflowNodes(nodes, "结果未知")).toEqual([nodes[134]]);
    expect(filterWorkflowNodes(nodes, "不存在的节点")).toEqual([]);
    expect(filterWorkflowNodes(nodes, "")).toHaveLength(135);
  });
  it("distinguishes ready nodes with no actual attempt from started processes", () => {
    const html = renderToStaticMarkup(
      <WorkflowNodeList
        nodes={[
          {
            id: "ready",
            name: "待领取开发",
            type: "agent",
            state: "ready",
            attempt: 0,
            fence: 0,
          },
        ]}
        onSelect={() => {}}
      />,
    );
    expect(html).toContain("等待派发");
    expect(html).toContain("尚未领取");
    expect(html).not.toContain("当前尝试 0");
  });
});
