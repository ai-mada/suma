import {AccountingReasonExplanation} from './AccountingReasonExplanation';
import { Trans } from "./i18n/Trans";
import { tr as t, useLocale, formatNumber } from "./i18n";
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
    ? t("development.responsive.awaiting_dispatch")
    : state === "pending"
      ? t("development.responsive.awaiting_dependencies")
      : stateLabel(state);
}
function attemptLabel(node: WorkflowListNode) {
  return node.attempt && node.attempt > 0
    ? t("development.responsive.attempt_number",{attempt:formatNumber(node.attempt)})
    : t("development.responsive.not_yet_claimed");
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
}) { useLocale();
  const [search, setSearch] = useState(""),
    visible = filterWorkflowNodes(nodes, search);
  return (
    <section className="workflow-node-list" aria-label={t("development.responsive.complete_execution_node_list")}>
      <div className="workflow-list-heading">
        <h3>{t("development.responsive.execution_nodes")}</h3>
        <small>{t("development.responsive.full_count",{count:nodes.length,value:formatNumber(nodes.length)})}</small>
      </div>
      <label className="workflow-node-search">
        <Search size={15} />
        <input
          type="search"
          aria-label={t("development.responsive.find_execution_node")}
          placeholder={t("development.responsive.search_by_node_name_id_or_state")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      {search && (
        <p className="muted" role="status">
          {t("development.responsive.matches_count",{count:formatNumber(visible.length),total:formatNumber(nodes.length)})}</p>
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
                {node.type === 'terminal_session' && <small>{t('development.workflow.type.terminal_session')}</small>}
                <small>
                  {node.fence && node.fence > 0?t("development.responsive.attempt_fence",{attempt:attemptLabel(node),fence:node.fence}):attemptLabel(node)}
                </small>
                <code>{node.id}</code>
                {node.reason && (
                  <span className="workflow-node-reason">{node.reason}<AccountingReasonExplanation reason={node.reason}/></span>
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
            ? t("development.responsive.no_matching_nodes_in_the_full_scope")
            : t("development.responsive.waiting_for_this_run_s_official_nodes")}
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
}) { useLocale();
  return (
    <section className="workflow-run-summary" aria-label={t("development.responsive.current_run_and_node")}>
      <div className="workflow-run-summary-heading">
        <h2>{run.workflow_name}</h2>
        <span className={"badge state-" + run.state}>
          {stateLabel(run.state)}
        </span>
      </div>
      <p className="workflow-run-id">
        <Trans i18nKey="development.responsive.run_id" values={{id:run.id}} components={{id:<code/>}}/>
      </p>
      {node ? (
        <div className="workflow-selected-context">
          <strong>{t("development.responsive.current_node_name",{name:node.name})}</strong>
          <p>
            {nodeState(node.state)} · {attemptLabel(node)}
          </p>
          <code>{node.id}</code>
        </div>
      ) : (
        <p className="muted">
          {t("development.responsive.no_node_selected_open_details_from_the")}</p>
      )}
    </section>
  );
}
