import { Trans } from "./i18n/Trans";
import { tr as t, useLocale, formatNumber } from "./i18n";
import { stateLabel, time } from "./api";
import { ToolExecutionFacts } from "./ToolFacts";
import {
  admissionNames,
  capacityLabel,
  executionHref,
  type SchedulingPolicy,
  type PolicyPreview,
  type NodeAdmission,
  type ExecutionAttempt,
  type SchedulingBlocker,
} from "./scheduling-model";
export function RunAdmissionReview({
  action,
  run,
}: {
  action: string;
  run: {
    id: string;
    version: number;
    workflow_version: number;
    state: string;
    nodes: { state: string }[];
  };
}) { useLocale();
  return (
    <section className="scheduling-facts" aria-label={t("development.scheduling.review_run_command_scope")}>
      <h4>{t("development.scheduling.review_run_command_scope")}</h4>
      <code>{run.id}</code>
      <p>
        {t("development.scheduling.run_versions",{run:run.version,workflow:run.workflow_version,state:stateLabel(run.state)})}
      </p>
      <p>
        {t("development.scheduling.run_snapshot",{ready:formatNumber(run.nodes.filter(node=>node.state==="ready").length),running:formatNumber(run.nodes.filter(node=>node.state==="running").length)})}
      </p>
      <p>
        {action === "pause"
          ? t("development.scheduling.pausing_blocks_new_dispatch_only_existing_execution")
          : action === "resume"
            ? t("development.scheduling.resuming_reopens_dispatch_without_bypassing_current_resource")
            : t("development.scheduling.cancellation_requests_a_stop_first_only_an")}
      </p>
      <small>
        {t("development.scheduling.this_is_a_current_run_snapshot_not")}</small>
    </section>
  );
}
const resourceNames: Record<string, string> = {
  get execution_slots() {
    return t("common.resource.executionSlots");
  },
  get budget() {
    return t("common.resource.budget");
  },
  get cpu_millis() {
    return t("common.resource.cpuMillis");
  },
  get memory_bytes() {
    return t("common.resource.memoryBytes");
  },
  get gpu_slots() {
    return t("common.resource.gpuSlots");
  },
  get model_concurrency() {
    return t("common.resource.modelConcurrency");
  },
};
const unitNames: Record<string, string> = {
  get admission_units() {
    return t("common.unit.admission");
  },
  get slots() {
    return t("common.unit.slots");
  },
  get millicores() {
    return t("common.unit.millicores");
  },
  get bytes() {
    return t("common.unit.bytes");
  },
  get requests() {
    return t("common.unit.requests");
  },
  get units() {
    return t("common.unit.units");
  },
};
export const schedulingResourceLabel = (name: string) =>
  resourceNames[name] || name;
export const schedulingUnitLabel = (unit?: string) =>
  unit ? unitNames[unit] || unit : t("common.unit.unknown");
export const schedulingNodeStateLabel = (state: string) =>
  state === "waiting" ? t("development.scheduling.node_waiting") : stateLabel(state);
function blockerTitle(blocker: SchedulingBlocker) {
  if (blocker.source === "pool") {
    const names: Record<string, string> = {
      pool_scope_inactive: t("development.scheduling.shared_pool_project_grant_is_disabled"),
      pool_scope_version_mismatch: t("development.scheduling.shared_pool_grant_version_changed"),
      pool_budget_exhausted: t("development.scheduling.shared_pool_cumulative_execution_budget_exhausted"),
      pool_paused: t("development.scheduling.shared_pool_paused_new_dispatch"),
      pool_metadata_missing: t("development.scheduling.shared_pool_metadata_needs_reconciliation"),
      pool_grant_not_current: t("development.scheduling.shared_pool_current_project_grant_needs_reconciliation"),
    };
    if (blocker.code === "pool_resource_capacity")
      return blocker.resource==="execution_slots"?t("development.scheduling.shared_pool_slots_exhausted"):t("development.scheduling.pool_resource_exhausted",{resource:blocker.resource?schedulingResourceLabel(blocker.resource):t("development.scheduling.execution_resource")});
    return names[blocker.code] || t("development.scheduling.pool_unknown",{reason:blocker.message||blocker.code});
  }
  const names: Record<string, string> = {
    run_paused: t("development.scheduling.run_paused"),
    run_cancelling: t("development.scheduling.cancelling_awaiting_actual_stop_or_result_reconciliation"),
    outcome_unknown: t("development.scheduling.execution_result_requires_reconciliation_before_re_execution"),
    dependencies_pending: t("development.scheduling.awaiting_dependencies_or_execution_conditions"),
    approval_required: t("development.scheduling.awaiting_human_approval"),
    timer_waiting: t("development.scheduling.awaiting_timer_wake_up"),
    expansion_waiting: t("development.scheduling.awaiting_dynamic_tasks"),
    child_waiting: t("development.scheduling.awaiting_child_workflow"),
    node_waiting: t("development.scheduling.node_waiting"),
    budget_exhausted: t("development.scheduling.project_cumulative_execution_budget_exhausted"),
    worker_capability_unknown: t("development.scheduling.execution_worker_matching_unconfirmed"),
  };
  if (blocker.code === "resource_capacity") {
    if (blocker.resource === "execution_slots") return t("development.scheduling.project_execution_slots_exhausted");
    const name = blocker.resource
      ? schedulingResourceLabel(blocker.resource)
      : t("development.scheduling.execution_resource");
    return t("development.scheduling.resource_exhausted",{resource:name});
  }
  return names[blocker.code] || blocker.message || blocker.code;
}
const standardPolicyNotes = new Set([
  "容量为项目执行准入额度，并非实测硬件或操作系统硬限制",
  "降低容量只阻止新派发，已运行尝试继续；累计准入单位不退款",
  "影响数据为预览时快照；提交时重新核验权限与策略版本",
]);
export function PolicyFacts({ policy }: { policy: SchedulingPolicy }) { useLocale();
  return (
    <section
      className="scheduling-facts scheduling-policy-summary"
      aria-label={t("development.scheduling.currently_configured_admission_quotas")}
    >
      <div className="scheduling-summary-heading">
        <h3>{t("development.scheduling.current_execution_quotas")}</h3>
        <span>
          {policy.default?t("development.scheduling.default_version",{version:policy.version}):"v"+policy.version}
        </span>
        <small>{t("development.scheduling.read_time",{at:time(policy.as_of)})}</small>
      </div>
      <dl className="scheduling-summary-values">
        <div>
          <dt>{t("development.scheduling.concurrent_slots")}</dt>
          <dd>
            {formatNumber(policy.concurrency_limit)}
            <small>{t("development.scheduling.slots")}</small>
          </dd>
        </div>
        <div>
          <dt>{t("development.scheduling.cumulative_execution_budget")}</dt>
          <dd>
            {capacityLabel(policy.budget_limit)}
            <small>{schedulingUnitLabel(policy.unit)}</small>
          </dd>
        </div>
        <div>
          <dt>{t("development.scheduling.cumulative_used")}</dt>
          <dd>
            {formatNumber(policy.budget_spent)}
            <small>{schedulingUnitLabel(policy.unit)}</small>
          </dd>
        </div>
      </dl>
      {policy.budget_history !== "complete" && (
        <p className="warning">{t("development.scheduling.incomplete_history_used_totals_include_recorded_executions")}</p>
      )}
      <details className="scheduling-resource-details">
        <summary>{t("development.scheduling.resource_details_and_scheduling")}</summary>
        <p className="muted">
          {policy.fairness.mode === "project_run_round_robin" &&
          policy.fairness.scope === "project"
            ? t("development.scheduling.round_robin_runs_within_the_current_project")
            : policy.fairness.mode + " / " + policy.fairness.scope}
        </p>
        {policy.resources.length > 0 ? (
          <table>
            <thead>
              <tr>
                <th>{t("development.scheduling.resource")}</th>
                <th>{t("development.scheduling.limit")}</th>
                <th>{t("development.scheduling.used")}</th>
                <th>{t("development.scheduling.reservation_mode")}</th>
              </tr>
            </thead>
            <tbody>
              {policy.resources.map((resource) => (
                <tr key={resource.name}>
                  <td>{schedulingResourceLabel(resource.name)}</td>
                  <td>
                    {capacityLabel(resource.capacity)}
                    <small>{schedulingUnitLabel(resource.unit)}</small>
                  </td>
                  <td>
                    {resource.release_class === "cumulative"
                      ? formatNumber(resource.consumed)
                      : formatNumber(resource.used)}
                    <small>{schedulingUnitLabel(resource.unit)}</small>
                  </td>
                  <td>
                    {resource.release_class === "cumulative"
                      ? t("development.scheduling.cumulative_consumption")
                      : resource.release_class === "process"
                        ? t("development.scheduling.held_while_running")
                        : t("development.scheduling.held_until_result_confirmed")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p>{t("development.scheduling.no_additional_resource_details")}</p>
        )}
      </details>
    </section>
  );
}
export function PolicyPreviewFacts({ preview }: { preview: PolicyPreview }) { useLocale();
  const notes = preview.impact.notes.filter(
    (note) => !standardPolicyNotes.has(note),
  );
  return (
    <section
      className="scheduling-facts scheduling-preview-facts"
      aria-label={t("development.scheduling.review_policy_change")}
    >
      <h3>{t("development.scheduling.review_policy_change")}</h3>
      <div className="scheduling-preview-meta">
        <strong>
          {preview.state === "committed"
            ? t("development.scheduling.committed")
            : preview.valid
              ? t("development.scheduling.ready_for_review_and_confirmation")
              : t("development.scheduling.this_preview_cannot_currently_be_submitted")}
        </strong>
        <span>{t("development.scheduling.policy_basis",{version:preview.policy_version})}</span>
        <span>{t("development.scheduling.expiry_time",{at:time(preview.expires_at)})}</span>
      </div>
      {preview.reason && <p className="warning">{preview.reason}</p>}
      <ul className="scheduling-change-list">
        {preview.changes.map((change) => (
          <li key={change.resource}>
            <strong>{schedulingResourceLabel(change.resource)}</strong>
            <span>
              {capacityLabel(change.before)} → {capacityLabel(change.after)}{" "}
              {schedulingUnitLabel(change.unit)}
            </span>
          </li>
        ))}
      </ul>
      {!preview.changes.length && <p>{t("development.scheduling.this_preview_has_no_quota_changes")}</p>}
      <h4>{t("development.scheduling.execution_at_preview_time")}</h4>
      <p>
        {t("development.scheduling.preview_counts",{ready:formatNumber(preview.impact.ready_total),slots:formatNumber(preview.impact.reserved_execution_slots),spent:formatNumber(preview.impact.budget_spent)})}</p>
      <p className="muted">
        {t("development.scheduling.observed_time",{at:time(preview.impact.observed_at)})}{" "}{preview.impact.budget_history !== "complete" ? t("development.scheduling.history_is_incomplete") : ""}
      </p>
      {notes.map((note, index) => (
        <p key={index}>{note}</p>
      ))}
      <details className="scheduling-preview-reference">
        <summary>{t("development.scheduling.preview_context_and_fingerprint")}</summary>
        <dl>
          <dt>{t("development.scheduling.fixed_preview")}</dt>
          <dd>
            <code>{preview.id}</code> · v{preview.version}
          </dd>
          <dt>{t("development.scheduling.fixed_digest")}</dt>
          <dd>
            <code>{preview.policy_sha256}</code>
          </dd>
        </dl>
      </details>
    </section>
  );
}
export function AdmissionFacts({
  snapshot,
  project,
}: {
  snapshot: NodeAdmission;
  project?: string;
}) { useLocale();
  return (
    <section className="scheduling-facts" aria-label={t("development.facts.current_dispatch_conditions")}>
      <h4>{t("development.facts.current_dispatch_conditions")}</h4>
      <p>
        <strong>{admissionNames[snapshot.status] || snapshot.status}</strong> ·{" "}
        {schedulingNodeStateLabel(snapshot.node_state)}
      </p>
      <small>
        {t("development.scheduling.evaluation_versions",{at:time(snapshot.evaluated_at),run:snapshot.run_version,policy:snapshot.policy_version})}
      </small>
      {snapshot.execution_pool_id && (
        <p className="scheduling-pool-scope">
          <Trans i18nKey="development.scheduling.pool_version" values={{id:snapshot.execution_pool_id,version:snapshot.pool_policy_version??t("development.workflow.not_provided")}} components={{id:<code/>}}/>
        </p>
      )}
      <ul className="scheduling-blockers">
        {snapshot.blockers.map((blocker, index) => (
          <li key={index}>
            <strong>{blockerTitle(blocker)}</strong>
            {blocker.resource && (
              <span>{schedulingResourceLabel(blocker.resource)}</span>
            )}
            <div className="scheduling-numbers">
              {(["required", "capacity", "used", "available"] as const).map(
                (key) =>
                  blocker[key] !== undefined && (
                    <span key={key}>
                      {
                        {
                          required: t("development.scheduling.required"),
                          capacity: t("development.scheduling.limit"),
                          used: t("development.scheduling.reserved"),
                          available: t("development.scheduling.available"),
                        }[key]
                      }{" "}
                      {typeof blocker[key]==="number"?formatNumber(blocker[key]):blocker[key]} {schedulingUnitLabel(blocker.unit)}
                    </span>
                  ),
              )}
            </div>
            {blocker.retry_at && (
              <small>{t("development.scheduling.next_check",{at:time(blocker.retry_at)})}</small>
            )}
            <details className="scheduling-blocker-basis">
              <summary>{t("development.scheduling.diagnostic_context")}</summary>
              <small>
                {blocker.code} · {blocker.source}
              </small>
              {blocker.message && <p className="preserve">{blocker.message}</p>}
            </details>
            {project && blocker.run_id && blocker.node_id && (
              <a href={executionHref(project, blocker.run_id, blocker.node_id)}>
                {t("development.scheduling.view_related_execution_node")}</a>
            )}
          </li>
        ))}
      </ul>
      {!snapshot.blockers.length && (
        <p>
          {snapshot.status === "eligible"
            ? t("development.scheduling.this_read_meets_the_checked_admission_conditions")
            : t("development.scheduling.no_additional_blockers_were_returned_this_does")}
        </p>
      )}
      {snapshot.execution_pool_id && (
        <details>
          <summary>{t("development.scheduling.shared_pool_resources_within_this_node_s")}</summary>
          <p>
            {t("development.scheduling.only_resources_requested_by_this_node_are")}</p>
          {snapshot.pool_resources?.length ? (
            <div className="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{t("development.scheduling.resource")}</th>
                    <th>{t("development.scheduling.configured_limit")}</th>
                    <th>{t("development.scheduling.currently_reserved")}</th>
                    <th>{t("development.scheduling.cumulative_consumed")}</th>
                  </tr>
                </thead>
                <tbody>
                  {snapshot.pool_resources.map((item) => (
                    <tr key={item.name}>
                      <td>
                        {schedulingResourceLabel(item.name)} ·{" "}
                        {schedulingUnitLabel(item.unit)}
                      </td>
                      <td>{capacityLabel(item.capacity)}</td>
                      <td>{formatNumber(item.used)}</td>
                      <td>{formatNumber(item.consumed)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p>{t("development.scheduling.no_shared_pool_resource_details_were_provided")}</p>
          )}
        </details>
      )}
      <details>
        <summary>{t("development.scheduling.fixed_resource_requests_and_current_reservations")}</summary>
        {snapshot.requests.map((resource) => (
          <p key={resource.name}>
            {t("development.scheduling.resource_request",{resource:schedulingResourceLabel(resource.name),amount:formatNumber(resource.amount),unit:schedulingUnitLabel(resource.unit),mode:resource.release_class==="cumulative"?t("development.scheduling.cumulative_consumption_per_attempt"):t("development.scheduling.concurrent_reservation")})}
          </p>
        ))}
        {snapshot.reservations.map((resource) => (
          <p key={resource.name}>
            {t("development.scheduling.resource_reservation",{resource:schedulingResourceLabel(resource.name),amount:formatNumber(resource.amount),unit:schedulingUnitLabel(resource.unit),attempt:formatNumber(resource.attempt),fence:resource.fence})}
          </p>
        ))}
        {!snapshot.reservations.length && <p>{t("development.scheduling.no_concurrent_resource_reservations_in_this_read")}</p>}
      </details>
    </section>
  );
}
export function AttemptFacts({ attempt }: { attempt: ExecutionAttempt }) { useLocale();
  const result = attempt.result,
    source = attempt.result_source;
  const sourceNames: Record<string, string> = {
    not_loaded: t("development.scheduling.metadata_only_result_not_loaded"),
    unreported: t("development.scheduling.terminal_result_unreported"),
    runner_result: t("development.scheduling.runner_result_receipt"),
    manual_reconciliation: t("development.scheduling.manual_reconciliation_record"),
    child_workflow: t("development.scheduling.persistent_child_workflow_result"),
    managed_observation: t("development.scheduling.provider_observation_reconciliation"),
  };
  return (
    <section
      className="scheduling-facts"
      aria-label={t("development.scheduling.attempt_number",{attempt:formatNumber(attempt.attempt)})}
    >
      <h4>{t("development.scheduling.attempt_number",{attempt:formatNumber(attempt.attempt)})}</h4>
      <dl>
        <dt>{t("development.scheduling.result_source")}</dt>
        <dd>{sourceNames[source] || t("development.scheduling.source_unreported")}</dd>
        <dt>{t("development.scheduling.persistent_execution_state")}</dt>
        <dd>{schedulingNodeStateLabel(attempt.state)}</dd>
        <dt>{t("development.scheduling.execution_identity")}</dt>
        <dd>
          <code>{attempt.worker_id || t("development.scheduling.not_provided_yet")}</code>
        </dd>
        <dt>{t("development.scheduling.fence")}</dt>
        <dd>{attempt.fence}</dd>
        <dt>{t("development.scheduling.started_finished")}</dt>
        <dd>
          {time(attempt.started_at)} / {time(attempt.completed_at)}
        </dd>
      </dl>
      {source === "manual_reconciliation" && (
        <section aria-label={t("development.scheduling.manual_reconciliation_conclusion")}>
          <h5>{t("development.scheduling.manual_reconciliation_conclusion")}</h5>
          <p>
            {attempt.reconciliation?.outcome
              ? (
                  {
                    not_started: t("development.scheduling.manually_confirmed_not_started"),
                    succeeded: t("development.scheduling.manually_reconciled_as_succeeded"),
                    failed: t("development.scheduling.manually_reconciled_as_failed"),
                  } as Record<string, string>
                )[attempt.reconciliation.outcome] ||
                attempt.reconciliation.outcome
              : t("development.scheduling.reconciliation_conclusion_not_provided_yet")}
          </p>
          <p className="preserve">
            {attempt.reconciliation?.reason || t("development.scheduling.reconciliation_evidence_not_provided_yet")}
          </p>
          <small>{t("development.scheduling.this_is_a_saved_manual_reconciliation_fact")}</small>
        </section>
      )}
      {source === "child_workflow" && (
        <section aria-label={t("development.scheduling.fixed_child_workflow_result")}>
          <h5>{t("development.scheduling.fixed_child_workflow_result")}</h5>
          <code>{attempt.child_run_id || t("development.scheduling.child_run_id_not_provided_yet")}</code>
          <p>{t("development.scheduling.persistent_state_value",{state:schedulingNodeStateLabel(attempt.state)})}</p>
        </section>
      )}
      {result && source !== "not_loaded" && source !== "unreported" ? (
        <>
          <p>
            {result.outcome_unknown
              ? t("development.scheduling.unknown_result_cannot_authorize_redispatch")
              : result.stopped && source === "runner_result"
                ? t("development.scheduling.actual_stop_recorded")
                : source === "managed_observation"
                  ? t("development.scheduling.provider_observation_saved_local_process_exit_or")
                  : t("development.scheduling.result_saved_removal_of_external_side_effects")}
          </p>
          {result.stopped && source === "runner_result" && (
            <p>{t("development.scheduling.actual_stop_reported_external_results_need_separate")}</p>
          )}
          {(source === "runner_result" || source === "managed_observation") && (
            <p>
              {t(result.failure_class?"development.scheduling.result_failure_class":"development.scheduling.result_code",{label:source==="runner_result"?t("development.scheduling.process_exit_code"):t("development.scheduling.provider_observation_status_code"),code:result.exit_code,failure:result.failure_class||""})}
            </p>
          )}
          {(result.output || result.error) && (
            <pre className="output">
              {result.output}
              {result.error && "\n" + result.error}
            </pre>
          )}
          <ToolExecutionFacts
            state={attempt.state}
            evidence={result.tool_evidence ?? undefined}
          />
          {result.workspace_evidence && (
            <details>
              <summary>{t("development.scheduling.workspace_evidence_for_this_attempt")}</summary>
              <pre className="output">
                {JSON.stringify(result.workspace_evidence, null, 2)}
              </pre>
            </details>
          )}
        </>
      ) : source === "not_loaded" ? (
        <p className="muted">{t("development.scheduling.only_attempt_metadata_is_available_open_exact")}</p>
      ) : source === "manual_reconciliation" ||
        source === "child_workflow" ? null : (
        <p className="muted">
          {t("development.scheduling.the_terminal_result_is_unreported_this_does")}</p>
      )}
    </section>
  );
}
