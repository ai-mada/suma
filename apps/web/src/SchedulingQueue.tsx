import { useId, useRef, useState, type MouseEvent, type RefObject } from "react";
import { ArrowRight, CircleAlert, CircleCheck, CircleHelp, RefreshCw, Server, Wrench } from "lucide-react";
import { query, time } from "./api";
import { useApiView } from "./api-context";
import { tr as t, useLocale, formatNumber, UiError } from "./i18n";
import { AdmissionFacts, poolRunnerRecoveryHint, schedulingBlockerTitle } from "./SchedulingFacts";
import { executionHref, validQueueRun, type QueueRoute, type ReadyEntry, type SchedulingBlocker } from "./scheduling-model";
import { useSchedulingPage } from "./scheduling-hooks";
import { toolReturnPanel } from "./tool-model";
import { ErrorNotice, Loading, Empty } from "./ui";
import "./scheduling.css";

const filters = ["all", "blocked", "unknown", "checked"] as const;
type QueueFilter = (typeof filters)[number];
type Props = {
  project: string;
  refresh: number;
  onRun: (run: string, node: string) => void;
  queueRoute?: QueueRoute;
  queueRun?: string;
  invalidQueue?: boolean;
  onQueueScope?: (route?: QueueRoute, run?: string) => void;
};
const statusIcons = { blocked: CircleAlert, unknown: CircleHelp, checked: CircleCheck };
const actionableToolBlockers = new Set(["tool_profile_changed", "tool_profile_paused", "tool_profile_revoked", "tool_activation_revoked"]);
const actionablePoolBlockers = new Set([
  "pool_scope_inactive", "pool_scope_version_mismatch", "pool_budget_exhausted", "pool_resource_capacity",
  "pool_paused", "pool_metadata_missing", "pool_policy_missing", "pool_grant_not_current",
  "pool_runner_registration_missing", "pool_runner_credential_unavailable",
]);
const toolIdentity = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const profileDigest = /^[a-f0-9]{64}$/;

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

function frozenRoute(admission: ReadyEntry["admission"]): QueueRoute | undefined {
  if (!admission) return;
  const pool = admission.execution_pool_id;
  if (pool === undefined || pool === "") return "project";
  if (typeof pool === "string") return "pool";
}

function toolDiagnosticsHref(project: string, item: ReadyEntry, admission: ReadyEntry["admission"], blocker?: SchedulingBlocker) {
  const binding = admission?.tool_binding;
  if (admission?.status !== "blocked" || !blocker || !actionableToolBlockers.has(blocker.code) ||
      blocker.run_id !== item.run_id || blocker.node_id !== item.node_id || !binding ||
      typeof binding.profile_id !== "string" || !toolIdentity.test(binding.profile_id) ||
      typeof binding.command_id !== "string" || !toolIdentity.test(binding.command_id) || binding.command_id !== item.command_id ||
      typeof binding.profile_sha256 !== "string" || !profileDigest.test(binding.profile_sha256) ||
      !validQueueRun(item.run_id) || !item.node_id) return undefined;

  const panel = toolReturnPanel(item.run_id, item.node_id);
  return panel ? "?" + new URLSearchParams({ project, view: "settings", sub: "tools", record: binding.profile_id, panel }) : undefined;
}

function guardQueueNavigation(event: MouseEvent<HTMLAnchorElement>) {
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
  if (!window.dispatchEvent(new Event("suma:navigate", { cancelable: true }))) event.preventDefault();
}

function QueueScopeForm({
  route, run, invalid, input, onChange,
}: {
  route?: QueueRoute;
  run?: string;
  invalid: boolean;
  input: RefObject<HTMLInputElement | null>;
  onChange: NonNullable<Props["onQueueScope"]>;
}) {
  const errorID = useId();
  const source = JSON.stringify([run, invalid]);
  const initial: { source: string; text: string; invalidRoute?: QueueRoute; submitted: boolean } = { source, text: run ?? "", submitted: false };
  const [draft, setDraft] = useState(initial);
  // Back/forward can change the committed run without remounting the form.
  if (draft.source !== source) setDraft(initial);
  const current = draft.source === source ? draft : initial;
  const invalidRun = current.submitted && !validQueueRun(current.text);
  const selected = invalid ? current.invalidRoute : route;

  return (
    <form className="scheduling-queue-scope" aria-label={t("workspace.queue.scope_label")} onSubmit={event => {
      event.preventDefault();
      if (current.text && !validQueueRun(current.text)) {
        setDraft({ ...current, submitted: true });
        input.current?.focus();
        return;
      }
      setDraft({ ...current, submitted: false });
      if (invalid || current.text !== (run ?? "")) onChange(selected, current.text || undefined);
    }}>
      <label>
        <span>{t("workspace.queue.route_label")}</span>
        <select data-queue-route value={selected ?? ""} onChange={event => {
          const next = event.target.value === "project" || event.target.value === "pool" ? event.target.value : undefined;
          if (invalid) setDraft({ ...current, invalidRoute: next });
          else onChange(next, run);
        }}>
          <option value="">{t("workspace.queue.route_all")}</option>
          <option value="project">{t("workspace.queue.route_project")}</option>
          <option value="pool">{t("workspace.queue.route_pool")}</option>
        </select>
      </label>
      <label className="scheduling-queue-run-field">
        <span>{t("workspace.queue.run_label")}</span>
        <input ref={input} type="text" data-queue-run-filter value={current.text} maxLength={100}
          autoComplete="off" spellCheck={false} aria-invalid={invalidRun || undefined}
          aria-describedby={invalidRun ? errorID : undefined} placeholder={t("workspace.queue.run_placeholder")}
          onChange={event => setDraft({ ...current, text: event.target.value, submitted: false })} />
      </label>
      <div className="scheduling-queue-scope-actions">
        <button type="submit" data-queue-apply>{t("workspace.queue.scope_apply")}</button>
        <button type="button" data-queue-clear onClick={() => {
          setDraft({ ...initial, text: "" });
          if (invalid || route || run) onChange();
          input.current?.focus();
        }}>{t("workspace.queue.scope_clear")}</button>
      </div>
      {invalidRun && <p id={errorID} className="scheduling-queue-scope-error" role="alert">{t("workspace.queue.run_invalid")}</p>}
    </form>
  );
}

export function SchedulingQueue(props: Props) {
  useLocale();
  const view = useApiView();
  if (!props.project) return <Empty title={t("workspace.select_project")} />;
  const scope = view?.organizationID && view.userID ? JSON.stringify([view.organizationID, view.userID, props.project]) : "";
  return <ProjectSchedulingQueue key={scope || props.project} scope={scope} {...props} />;
}

function ProjectSchedulingQueue({ project, refresh, onRun, scope, queueRoute, queueRun, invalidQueue, onQueueScope }: Props & { scope: string }) {
  const input = useRef<HTMLInputElement>(null);
  const invalid = !!invalidQueue || queueRun !== undefined && !validQueueRun(queueRun);
  const filterScope = JSON.stringify([scope, queueRoute, queueRun]);
  const [facet, setFacet] = useState<{ scope: string; filter: QueueFilter }>();
  const saved = scope && isQueueEntry(project) ? history.state?.sumaQueueFilter : undefined;
  const filter = facet?.scope === filterScope ? facet.filter
    : saved?.scope === filterScope ? filters.find(value => value === saved.filter) ?? "all" : "all";
  function chooseFilter(value: QueueFilter) {
    const next = { scope: filterScope, filter: value };
    setFacet(next);
    // Only the local status facet is stored in history state. Execution path
    // and run use the shareable URL, which is also the queue read scope.
    if (scope && isQueueEntry(project)) history.replaceState({ ...history.state, sumaQueueFilter: next }, "");
  }
  const params = new URLSearchParams();
  if (queueRoute) params.set("route", queueRoute);
  if (queueRun) params.set("run_id", queueRun);
  const selection = params.toString();
  const page = useSchedulingPage<ReadyEntry>(
    invalid ? "" : "/scheduling/queue" + query(project) + (selection ? "&" + selection : ""),
    item => {
      if (!item || typeof item.run_id !== "string" || typeof item.node_id !== "string" ||
          queueRun && item.run_id !== queueRun || queueRoute && frozenRoute(admissionFor(item)) !== queueRoute) {
        throw new UiError("development.errors.page_scope_invalid");
      }
      return item.run_id + "\0" + item.node_id;
    },
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
  const poolResourcesHref = "?" + new URLSearchParams({ project, view: "settings", sub: "project-execution" });

  return (
    <section className="panel scheduling-workspace scheduling-queue-workspace" aria-label={t("workspace.queue.label")}>
      <div className="panel-heading">
        <div>
          <h2>{t("workspace.queue.title")}</h2>
          <p>{t("workspace.queue.description")}</p>
        </div>
        <button type="button" aria-disabled={invalid || page.loading} onClick={() => {
          if (!invalid && !page.loading) void (page.error ? page.retry() : page.read());
        }}>
          <RefreshCw size={15} aria-hidden="true" />
          {t(page.error ? "workspace.queue.retry" : "workspace.queue.reload")}
        </button>
      </div>
      {onQueueScope && <QueueScopeForm route={queueRoute} run={queueRun} invalid={invalid} input={input} onChange={onQueueScope} />}
      {invalid && <p className="warning" role="alert">{t("workspace.queue.scope_invalid")}</p>}
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
            const toolBlocker = admission?.blockers.find(blocker => blocker.source === "tool");
            const toolHref = toolDiagnosticsHref(project, item, admission, toolBlocker);
            const poolID = admission?.execution_pool_id;
            const poolBlockers = admission?.status === "blocked" && typeof poolID === "string" && validQueueRun(poolID)
              ? admission.blockers.filter(blocker => blocker.source === "pool" && actionablePoolBlockers.has(blocker.code) &&
                blocker.run_id === item.run_id && blocker.node_id === item.node_id)
              : [];
            const poolBlocker = poolBlockers.find(blocker => poolRunnerRecoveryHint(blocker)) ?? poolBlockers[0];
            const poolHint = poolBlocker && poolRunnerRecoveryHint(poolBlocker);
            const hasPolicyBlocker = status === "blocked" && admission?.blockers.some(blocker =>
              blocker.source === "policy" && (blocker.code === "resource_capacity" || blocker.code === "budget_exhausted"));
            const StatusIcon = statusIcons[status];
            const route = frozenRoute(admission);
            return <li key={item.run_id + "\0" + item.node_id} data-queue-entry data-queue-status={status}>
              <article>
                <header>
                  <div>
                    <h3>{item.node_name || item.node_id}</h3>
                    <p>{item.workflow_name || item.run_id} · {item.type}
                      {route && <> · {t("workspace.queue.route_" + route)}</>}
                    </p>
                  </div>
                  <span className="scheduling-queue-status">
                    <StatusIcon size={15} aria-hidden="true" />
                    {t("workspace.queue.status_" + status)}
                  </span>
                </header>
                <p className="scheduling-queue-reason">
                  {primary ? schedulingBlockerTitle(primary) : t(status === "checked" ? "workspace.queue.next_dispatch" : "workspace.queue.conditions_unknown")}
                </p>
                {toolBlocker && toolBlocker !== primary && <p className="scheduling-queue-reason">{schedulingBlockerTitle(toolBlocker)}</p>}
                {poolBlocker && poolBlocker !== primary && <p className="scheduling-queue-reason">{schedulingBlockerTitle(poolBlocker)}</p>}
                {poolHint && <small className="scheduling-queue-next-step">{poolHint}</small>}
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
                  {toolHref && <a className="scheduling-tool-link" data-queue-tool-diagnostics href={toolHref}
                    aria-label={t("workspace.queue.open_named_tool_diagnostics", { node: item.node_name || item.node_id })}
                    onClick={guardQueueNavigation}>
                    <Wrench size={15} aria-hidden="true" /> {t("workspace.queue.open_tool_diagnostics")}
                  </a>}
                  {poolBlocker && <a className="scheduling-pool-link" data-queue-pool-resources href={poolResourcesHref}
                    aria-label={t("workspace.queue.open_named_pool_resources", { node: item.node_name || item.node_id })}
                    onClick={guardQueueNavigation}>
                    <Server size={15} aria-hidden="true" /> {t("workspace.queue.open_pool_resources")}
                  </a>}
                  {confirmed && onQueueScope && !queueRun && validQueueRun(item.run_id) &&
                    <button className="scheduling-queue-focus-run" type="button" data-queue-focus-run
                      aria-label={t("workspace.queue.focus_named_run", { run: item.run_id })}
                      onClick={() => {
                        onQueueScope(queueRoute, item.run_id);
                        input.current?.focus();
                      }}>{t("workspace.queue.focus_run")}</button>}
                  {hasPolicyBlocker && <a className="scheduling-policy-link" data-queue-policy href={policyHref}
                    onClick={guardQueueNavigation}>{t("workspace.queue.open_policy")}</a>}
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
