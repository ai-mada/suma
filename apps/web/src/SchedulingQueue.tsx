import { useState } from "react";
import { ArrowRight, CircleAlert, CircleCheck, CircleHelp, RefreshCw } from "lucide-react";
import { query, time } from "./api";
import { useApiView } from "./api-context";
import { tr as t, useLocale, formatNumber } from "./i18n";
import { AdmissionFacts, schedulingBlockerTitle } from "./SchedulingFacts";
import { executionHref, type ReadyEntry } from "./scheduling-model";
import { useSchedulingPage } from "./scheduling-hooks";
import { ErrorNotice, Loading, Empty } from "./ui";
import "./scheduling.css";

const filters = ["all", "blocked", "unknown", "checked"] as const;
type QueueFilter = (typeof filters)[number];
type Props = { project: string; refresh: number; onRun: (run: string, node: string) => void };
const statusIcons = { blocked: CircleAlert, unknown: CircleHelp, checked: CircleCheck };

function isQueueEntry(project: string) {
  const query = new URLSearchParams(location.search);
  return query.get("project") === project && query.get("view") === "development" &&
    query.get("sub") === "runs" && query.get("layout") === "queue" && !query.get("run");
}

function admissionFor(item: ReadyEntry) {
  const admission = item.admission;
  if (!admission || admission.run_id !== item.run_id || admission.node_id !== item.node_id ||
      admission.run_version !== item.run_version || admission.node_state !== "ready" ||
      item.node_state !== "ready" || admission.attempt !== item.attempt || admission.fence !== item.fence ||
      !Number.isFinite(Date.parse(admission.evaluated_at)) ||
      !Array.isArray(admission.requests) || !Array.isArray(admission.reservations) ||
      !Array.isArray(admission.blockers) || admission.blockers.some(blocker =>
        !blocker || typeof blocker.code !== "string" || typeof blocker.source !== "string" ||
        (blocker.run_id !== undefined && blocker.run_id !== item.run_id) ||
        (blocker.node_id !== undefined && blocker.node_id !== item.node_id))) return undefined;
  if (admission.status === "blocked" && !admission.blockers.some(blocker => blocker.code !== "worker_capability_unknown") ||
      admission.status === "eligible" && admission.blockers.length > 0 ||
      !["blocked", "eligible", "unknown"].includes(admission.status)) return undefined;
  return admission;
}

export function SchedulingQueue(props: Props) {
  useLocale();
  const view = useApiView();
  if (!props.project) return <Empty title={t("workspace.select_project")} />;
  const scope = view?.organizationID && view.userID ? JSON.stringify([view.organizationID, view.userID, props.project]) : "";
  return <ProjectSchedulingQueue key={scope || props.project} scope={scope} {...props} />;
}

function ProjectSchedulingQueue({ project, refresh, onRun, scope }: Props & { scope: string }) {
  const [filter, setFilter] = useState<QueueFilter>(() => {
    const saved = scope && isQueueEntry(project) ? history.state?.sumaQueueFilter : undefined;
    return saved?.scope === scope ? filters.find(value => value === saved.filter) ?? "all" : "all";
  });
  function chooseFilter(value: QueueFilter) {
    setFilter(value);
    // Store only the selected facet on this exact history entry. Going back
    // restores the choice and the queue still reads fresh authorized records.
    if (scope && isQueueEntry(project)) history.replaceState({ ...history.state, sumaQueueFilter: { scope, filter: value } }, "");
  }
  const page = useSchedulingPage<ReadyEntry>(
    "/scheduling/queue" + query(project),
    (item) => item.run_id + "\0" + item.node_id,
    refresh,
  );
  const confirmed = page.hasSnapshot && !page.loading && !page.failure;
  const counts = { all: page.items.length, blocked: 0, unknown: 0, checked: 0 };
  const rows = page.items.map(item => {
    const admission = confirmed ? admissionFor(item) : undefined;
    const status: keyof typeof statusIcons = admission?.status === "eligible" ? "checked" : admission?.status === "blocked" ? "blocked" : "unknown";
    counts[status]++;
    return { item, admission, status };
  });
  const visible = filter === "all" || !confirmed ? rows : rows.filter(row => row.status === filter);
  const policyHref = "?" + new URLSearchParams({ project, view: "settings", sub: "scheduling" });

  return (
    <section className="panel scheduling-workspace scheduling-queue-workspace" aria-label={t("workspace.queue.label")}>
      <div className="panel-heading">
        <div>
          <h2>{t("workspace.queue.title")}</h2>
          <p>{t("workspace.queue.description")}</p>
        </div>
        <button type="button" aria-disabled={page.loading} onClick={() => {
          if (!page.loading) void (page.error ? page.retry() : page.read());
        }}>
          <RefreshCw size={15} aria-hidden="true" />
          {t(page.error ? "workspace.queue.retry" : "workspace.queue.reload")}
        </button>
      </div>
      {page.error && <>
        <ErrorNotice error={page.failure || page.error} />
        {page.hasSnapshot && <p className="warning">{t("workspace.queue.unconfirmed")}</p>}
      </>}
      {page.loading && page.hasSnapshot && <p role="status">{t("workspace.queue.refreshing")}</p>}
      {page.hasSnapshot && <div className="scheduling-queue-triage">
        <div className="scheduling-queue-filters" role="group" aria-label={t("workspace.queue.filter_label")}>
          {filters.map(value => <button type="button" key={value} data-queue-filter={value}
            aria-pressed={filter === value} disabled={!confirmed} onClick={() => chooseFilter(value)}>
            <span>{t("workspace.queue.filter_" + value)}</span>
            <span className="scheduling-queue-filter-count">{confirmed || value === "all" ? formatNumber(counts[value]) : "—"}</span>
          </button>)}
        </div>
        <p className="loaded-scope" role="status">
          {t("workspace.queue.loaded", { loaded: formatNumber(page.items.length), total: formatNumber(page.total) })}
          {confirmed && filter !== "all" && <> · {t("workspace.queue.filtered_count", { count: formatNumber(visible.length) })}</>}
        </p>
      </div>}
      {page.loading && !page.items.length ? <Loading /> : visible.length ? (
        <ul className="scheduling-queue">
          {visible.map(({ item, admission, status }) => {
            const primary = admission?.blockers.find(blocker => blocker.code !== "worker_capability_unknown") ?? admission?.blockers[0];
            const hasPolicyBlocker = status === "blocked" && admission?.blockers.some(blocker =>
              blocker.source === "policy" && (blocker.code === "resource_capacity" || blocker.code === "budget_exhausted"));
            const StatusIcon = statusIcons[status];
            return <li key={item.run_id + "\0" + item.node_id} data-queue-entry data-queue-status={status}>
              <article>
                <header>
                  <div>
                    <h3>{item.node_name || item.node_id}</h3>
                    <p>{item.workflow_name || item.run_id} · {item.type}</p>
                  </div>
                  <span className="scheduling-queue-status">
                    <StatusIcon size={15} aria-hidden="true" />
                    {t("workspace.queue.status_" + status)}
                  </span>
                </header>
                <p className="scheduling-queue-reason">
                  {primary ? schedulingBlockerTitle(primary) : t(status === "checked" ? "workspace.queue.next_dispatch" : "workspace.queue.conditions_unknown")}
                </p>
                <small className="muted">
                  {t("workspace.queue.ready_at", { at: time(item.ready_at), attempts: formatNumber(item.attempt) })}
                </small>
                <div className="scheduling-queue-actions">
                  <a className="scheduling-open-link" href={executionHref(project, item.run_id, item.node_id)}
                    aria-label={t("workspace.queue.open_named_node", { node: item.node_name || item.node_id })}
                    onClick={event => {
                      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
                      event.preventDefault();
                      onRun(item.run_id, item.node_id);
                    }}>
                    {t("workspace.queue.open_node")} <ArrowRight size={15} aria-hidden="true" />
                  </a>
                  {hasPolicyBlocker && <a className="scheduling-policy-link" data-queue-policy href={policyHref}
                    onClick={event => {
                      if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey &&
                          !window.dispatchEvent(new Event("suma:navigate", { cancelable: true }))) event.preventDefault();
                    }}>{t("workspace.queue.open_policy")}</a>}
                </div>
                {admission && <details className="scheduling-queue-details" data-queue-details>
                  <summary>{t("workspace.queue.diagnostic_details")}</summary>
                  <AdmissionFacts snapshot={admission} />
                </details>}
              </article>
            </li>;
          })}
        </ul>
      ) : confirmed ? <p className="scheduling-queue-empty">
        {t(page.items.length ? "workspace.queue.filtered_empty" : "workspace.queue.empty")}
      </p> : null}
      {page.next && !page.error && (
        <button className="scheduling-queue-more" type="button" aria-disabled={page.loading}
          onClick={() => { if (!page.loading) void page.read(page.next); }}>
          {t("workspace.queue.more")}
        </button>
      )}
    </section>
  );
}
