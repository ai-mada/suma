import { tr as t, useLocale, formatNumber } from "./i18n";
import { useEffect, useRef } from "react";
import { ApiError, query } from "./api";
import { ErrorNotice, Loading } from "./ui";
import { useToolRead } from "./tool-hooks";
import { useSchedulingPage } from "./scheduling-hooks";
import { AdmissionFacts, AttemptFacts, schedulingNodeStateLabel } from "./SchedulingFacts";
import { AttemptToolEventHistory } from "./AttemptToolEventHistory";
import {
  admissionCurrent,
  executionHref,
  parseAttempt,
  type NodeAdmission,
  type ExecutionAttempt,
} from "./scheduling-model";
import "./scheduling.css";
import type {ExecutionSkillBundle} from '../../../packages/contracts/src';
export function NodeSchedulingFacts({
  project,
  run,
  node,
  runVersion,
  currentAttempt,
  nodeState,
  nodeFence,
  selectedAttempt,
  onAttempt,
  historyRefresh = 0,
  executionSkills,
}: {
  project: string;
  run: string;
  node: string;
  runVersion: number;
  currentAttempt: number;
  nodeState?: string;
  nodeFence?: number;
  selectedAttempt?: string;
  onAttempt: (value: string) => void;
  historyRefresh?: number | string;
  executionSkills?:ExecutionSkillBundle;
}) { useLocale();
  const detailRoot = useRef<HTMLDivElement>(null),
    listRoot = useRef<HTMLElement>(null),
    detailAccessLost = useRef(false);
  const base =
      "/runs/" + encodeURIComponent(run) + "/nodes/" + encodeURIComponent(node),
    snapshotKey = [runVersion, currentAttempt, nodeFence, nodeState].join(":"),
    diagnostic = useToolRead<NodeAdmission>(
      base + "/scheduling" + query(project),
      runVersion,
    ),
    page = useSchedulingPage<ExecutionAttempt>(
      base + "/attempts" + query(project),
      (item) => String(item.attempt),
      snapshotKey,
      true,
    ),
    attempt = parseAttempt(selectedAttempt || ""),
    exact = useToolRead<ExecutionAttempt>(
      attempt ? base + "/attempts/" + attempt + query(project) : "",
      snapshotKey,
    );
  useEffect(() => {
    const timer = setInterval(() => void diagnostic.read(), 10000);
    return () => clearInterval(timer);
  }, [diagnostic.read]);
  useEffect(() => {
    if (attempt && exact.data?.attempt === attempt) {
      detailRoot.current?.scrollIntoView?.({
        block: "start",
        behavior: "auto",
      });
      detailRoot.current?.focus({ preventScroll: true });
    }
  }, [attempt, exact.data?.attempt]);
  // A retry cannot revive a denied cached record before authorization succeeds.
  if (exact.failure instanceof ApiError && [401,403,404].includes(exact.failure.status)) detailAccessLost.current = true;
  else if (!exact.loading && !exact.error) detailAccessLost.current = false;
  const current =
      diagnostic.data &&
      admissionCurrent(diagnostic.data, run, node, runVersion),
    record = detailAccessLost.current ? undefined : exact.data;
  return (
    <div className="node-scheduling-facts">
      {diagnostic.error && (
        <>
          <ErrorNotice
            error={diagnostic.error}
            onRetry={() => void diagnostic.read()}
          />
          <p className="warning">
            {t("development.attempts.current_dispatch_conditions_are_unconfirmed_old_data")}</p>
        </>
      )}
      {!diagnostic.error && current ? (
        <AdmissionFacts snapshot={diagnostic.data!} project={project} />
      ) : !diagnostic.error ? (
        <p role="status">
          {diagnostic.loading
            ? t("development.tools.reading_current_dispatch_conditions")
            : t("development.attempts.the_diagnostic_predates_the_current_run_read")}
        </p>
      ) : null}
      <button
        disabled={diagnostic.loading}
        onClick={() => void diagnostic.read()}
      >
        {t("development.attempts.recheck_current_conditions")}</button>
      <section
        ref={listRoot}
        tabIndex={-1}
        className="scheduling-attempts"
        aria-label={t("development.attempts.persistent_attempt_history")}
      >
        <h4>{t("development.attempts.recorded_executions")}</h4>
        <p className="muted">{t("development.attempts.attempts_come_from_a_persistent_table_independent")}</p>
        <button
          disabled={page.loading}
          onClick={() => void page.read("", true)}
        >
          {t("development.attempts.reload_attempt_history")}</button>
        {page.loading && page.items.length > 0 && (
          <p role="status">{t("development.attempts.updating_attempt_history_the_previous_read_is")}</p>
        )}
        {page.error && (
          <p className="warning">
            {t("development.attempts.history_could_not_be_refreshed_any_records")}</p>
        )}
        {page.error && (
          <ErrorNotice error={page.error} onRetry={() => void page.read()} />
        )}
        <ul>
          {page.items.map((item) => (
            <li key={item.attempt}>
              <a
                className="scheduling-open-link"
                aria-current={attempt === item.attempt ? "page" : undefined}
                href={executionHref(project, run, node, item.attempt)}
                onClick={(event) => {
                  if (
                    event.defaultPrevented ||
                    event.button !== 0 ||
                    event.metaKey ||
                    event.ctrlKey ||
                    event.shiftKey ||
                    event.altKey
                  )
                    return;
                  event.preventDefault();
                  onAttempt(String(item.attempt));
                }}
              >
                {t("development.attempts.attempt_summary",{attempt:formatNumber(item.attempt),state:schedulingNodeStateLabel(item.state),fence:item.fence})}
              </a>
            </li>
          ))}
        </ul>
        {page.loading && !page.items.length && <Loading />}
        {!page.loading && !page.error && !page.items.length && (
          <p>{t("development.attempts.no_claimed_attempts_yet_waiting_does_not")}</p>
        )}
        <small>
          {t("development.attempts.loaded_count",{count:formatNumber(page.items.length),total:formatNumber(page.total)})}</small>
        {page.next && (
          <button
            disabled={page.loading}
            onClick={() => void page.read(page.next)}
          >
            {t("development.attempts.load_earlier_attempts")}</button>
        )}
        {attempt && (
          <>
            <button disabled={exact.loading} onClick={() => void exact.read()}>
              {t("development.attempts.reload_selected_attempt")}</button>
            {exact.loading && record && (
              <p role="status">{t("development.attempts.updating_the_selected_attempt_the_previous_read")}</p>
            )}
            {exact.error && record && (
              <p className="warning">
                {t("development.attempts.the_selected_attempt_is_unconfirmed_the_previous")}</p>
            )}
            <button
              onClick={() => {
                onAttempt("");
                listRoot.current?.focus({ preventScroll: true });
              }}
            >
              {t("development.attempts.close_historical_attempt_details")}</button>
            {exact.error && (
              <ErrorNotice
                error={exact.error}
                onRetry={() => void exact.read()}
              />
            )}{" "}
            {exact.loading && !record && <Loading />}
            {record &&
            record.run_id === run &&
            record.node_id === node &&
            record.attempt === attempt ? (
              <div ref={detailRoot} tabIndex={-1} aria-label={t("development.attempts.selected_historical_attempt_details")}>
                <AttemptFacts attempt={record} executionSkills={executionSkills}/>
                <AttemptToolEventHistory key={JSON.stringify([project,run,node,record.attempt,record.fence,record.worker_id])} project={project} attempt={record} refresh={historyRefresh}/>
              </div>
            ) : !exact.loading && !exact.error ? (
              <p className="warning">
                {t("development.attempts.the_historical_attempt_does_not_belong_to")}</p>
            ) : null}
          </>
        )}
      </section>
    </div>
  );
}
