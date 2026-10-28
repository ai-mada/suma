import {OpenCodeExecutionFacts} from './OpenCodeExecutionFacts';
import {OctopExecutionFacts} from './OctopExecutionFacts';
import {openCodeRunFacts,type OpenCodeRunTest} from './opencode-run-model';
import {ExecutionModelFacts} from './ExecutionModelFacts';
import { Trans } from "./i18n/Trans";
import { NativeToolMessage } from "./NativeToolMessage";
import { renderDisplayDiagnostic } from "./display-diagnostic";
import { tr as t, useLocale, formatDateTime, formatNumber } from "./i18n";
import { DispatchRouteFacts } from "./execution-pools/DispatchRouteFacts";
import { stateLabel } from "./api";
import type {WorkspaceEvidence} from './api';
import type {AttemptToolScope} from './attempt-tool-history';
import {WorkspaceOperations} from './WorkspaceOperations';
import {workspaceOperationsForAttempt} from './workspace-operations-model';
import type { ModelBinding, OpenCodeExecutionCapability } from "../../../packages/contracts/src";
import {
  taskPlanStarted,
  toolEventNames,
  toolStateNames,
  toolEventRows,
  type ToolEvidence,
  type ToolProfile,
  type TaskDispatchPlan,
  type ToolEvent,
  type ToolUsage,
  type UsageState,
  type TaskContextRecord,
} from "./tool-model";
import "./tool.css";
export function TaskDispatchRunFacts({
  plan,
  onRun,
}: {
  plan: Pick<TaskDispatchPlan, "state" | "run_id">;
  onRun: (id: string) => void;
}) { useLocale();
  if (!taskPlanStarted(plan))
    return (
      <p className="muted">
        {plan.run_id && (
          <>
            <Trans i18nKey="development.facts.reserved_run" values={{id:plan.run_id}} components={{run:<code/>}}/>
          </>
        )}
        {t("development.facts.the_plan_has_not_created_an_official")}</p>
    );
  if (!plan.run_id)
    return (
      <p className="warning">
        {t("development.facts.the_plan_records_a_start_but_its")}</p>
    );
  return (
    <div className="success">
      <p>{t("development.facts.this_plan_is_linked_to_an_official")}</p>
      <button type="button" onClick={() => onRun(plan.run_id)}>
        {t("development.facts.view_this_plan_s_actual_run")}</button>
    </div>
  );
}
function stamp(value?: string) {
  return value
    ? formatDateTime(value, { dateStyle: "short", timeStyle: "medium", hour12: false }) + " · " + value
    : t("development.facts.no_official_observation_yet");
}
export function ToolProfileFacts({ profile }: { profile: ToolProfile }) { useLocale();
  return (
    <section className="tool-facts" aria-label={t("development.tools.fixed_tool_configuration")}>
      <div className="tool-section-heading">
        <h3>{profile.name}</h3>
        <span className="badge">
          {toolStateNames[profile.state] || profile.state}
        </span>
      </div>
      <dl>
        <dt>{t("development.tools.execution_worker_identity")}</dt>
        <dd>
          <code>{profile.worker_id}</code>
        </dd>
        <dt>{t("development.facts.command_and_protocol")}</dt>
        <dd>
          {profile.profile.command_id} · {profile.profile.adapter} /{" "}
          {profile.profile.protocol}
        </dd>
        <dt>{t("development.facts.fixed_version")}</dt>
        <dd>{profile.profile.tool_version}</dd>
        <dt>{t("development.facts.operation_version")}</dt>
        <dd>v{profile.version}</dd>
        <dt>{t("development.facts.execution_permission")}</dt>
        <dd>
          {profile.profile.read_only
            ? t("development.facts.read_only_command")
            : t("development.facts.write_permitted_worktree_isolated_per_attempt")}
        </dd>
        <dt>{t("development.facts.configuration_fingerprint")}</dt>
        <dd>
          <code>{profile.profile.profile_sha256}</code>
        </dd>
        <dt>{t("development.facts.executable_fingerprint")}</dt>
        <dd>
          <code>{profile.profile.executable_sha256}</code>
        </dd>
        <dt>{t("development.facts.last_report")}</dt>
        <dd>{stamp(profile.last_seen_at)}</dd>
        <dt>{t("development.facts.current_dispatch_conditions")}</dt>
        <dd>
          {profile.available
            ? t("development.facts.dispatchable_at_last_read_verified_again_at")
            : t("development.facts.dispatch_eligibility_unconfirmed")}
        </dd>
      </dl>
      <p className="muted">
        {t("development.facts.version_and_probing_do_not_prove_model")}</p>
      <OpenCodeExecutionFacts capability={profile.profile.opencode_execution}/>
      {profile.profile.adapter==='octop'&&profile.profile.protocol==='octop-harness-acp-v1'&&<OctopExecutionFacts capability={profile.profile.octop_execution}/>}
      {'workspace_tools_version' in profile.profile&&profile.profile.workspace_tools_version==='attempt-workspace-v1'&&<p className="muted">{t('native-workspace.capability')}</p>}
      {profile.profile.conflict_repair_protocol==='suma.candidate-conflict-repair.v1'&&<p className="muted">{t('candidate-conflict-repair.capability')}</p>}
      {profile.reason && (
        <div className="warning">
          <p>{toolReason(profile.reason, profile.reason_diagnostic)}</p>
          <details>
            <summary>{t("development.facts.original_reason")}</summary>
            <code>{profile.reason}</code>
          </details>
        </div>
      )}
      {profile.observation && (
        <details>
          <summary>{t("development.facts.latest_non_secret_observation")}</summary>
          <dl>
            <dt>{t("development.facts.command_available_at_that_time")}</dt>
            <dd>
              {profile.observation.available ? t("development.facts.yes_for_that_local_probe_only") : t("development.facts.no")}
            </dd>
            <dt>{t("workspace-capability.read")}</dt>
            <dd><code>{profile.observation.workspace_observation_protocol || t("workspace-capability.missing")}</code></dd>
            <dt>{t("workspace-capability.write")}</dt>
            <dd><code>{profile.observation.workspace_write_protocol || t("workspace-capability.missing")}</code></dd>
            <dt>{t("development.facts.version_at_that_time")}</dt>
            <dd>{profile.observation.profile.tool_version || t("development.workflow.not_provided")}</dd>
            <dt>{t("development.facts.configuration_fingerprint_at_that_time")}</dt>
            <dd>
              <code>
                {profile.observation.profile.profile_sha256 || t("development.workflow.not_provided")}
              </code>
            </dd>
          </dl>
          <p className="muted">{t("workspace-capability.help")}</p>
        </details>
      )}
    </section>
  );
}
export function toolReason(reason: string, diagnostic?: unknown) {
  if (diagnostic !== undefined && diagnostic !== null)
    return renderDisplayDiagnostic(reason, diagnostic).text;
  const reasons: Record<string, string> = {
        inventory_stale:
          t("development.facts.the_worker_report_expired_start_a_runner"),
        worker_revoked: t("development.facts.the_worker_credential_was_revoked_dispatch_is"),
        profile_paused: t("development.facts.this_configuration_is_paused_review_its_fixed"),
        profile_revoked: t("development.facts.this_configuration_was_permanently_revoked_historical_evidence"),
        profile_mismatch: t("development.facts.the_local_observation_does_not_match_the"),
        activation_authority_revoked:
          t("development.facts.the_original_enabling_user_s_current_permissions"),
      };
  return Object.hasOwn(reasons, reason) ? reasons[reason] : reason;
}
function ContextFacts({
  value,
  title,
}: {
  value: TaskContextRecord;
  title: string;
}) { useLocale();
  return (
    <section className="tool-context">
      <h4>
        {title} · v{value.version}
      </h4>
      <strong>{value.title}</strong>
      <code className="muted">{value.id}</code>
      <p className="preserve">{value.description || t("development.facts.no_description_provided")}</p>
      <h5>{t("development.facts.fixed_acceptance_criteria")}</h5>
      {value.acceptance_criteria.length ? (
        <ul>
          {value.acceptance_criteria.map((text, index) => (
            <li key={index}>{text}</li>
          ))}
        </ul>
      ) : (
        <p className="muted">{t("development.facts.no_acceptance_criteria_provided")}</p>
      )}
    </section>
  );
}
export function TaskDispatchFacts({ plan }: { plan: TaskDispatchPlan }) { useLocale();
  const agent = plan.workflow.nodes.find(
      (node) => node.id === plan.agent_node_id,
    ),
    tests = plan.workflow.nodes.filter((node) => node.type === "test");
  return (
    <section className="tool-plan-facts" aria-label={t("development.facts.immutable_task_execution_plan")}>
      <div className="tool-section-heading">
        <h3>{t("development.facts.fixed_execution_context")}</h3>
        <span className="badge">
          {plan.valid ? t("development.facts.current_context_valid") : t("development.facts.current_context_blocked")}
        </span>
      </div>
      <dl>
        <dt>{t("development.facts.plan")}</dt>
        <dd>
          <code>{plan.id}</code> · v{plan.version}
        </dd>
        <dt>{t("development.facts.original_workflow_version")}</dt>
        <dd>
          {plan.workflow_id} · v{plan.workflow_version}
        </dd>
        <dt>{t("development.facts.agent_node")}</dt>
        <dd>{agent?.name || plan.agent_node_id}</dd>
        <dt>{t("development.facts.tool_configuration")}</dt>
        <dd>
          {plan.tool_profile_id} · v{plan.tool_profile_version}
        </dd>
        <dt>{t("development.facts.tool_configuration_fingerprint")}</dt>
        <dd>
          <code>{plan.tool_profile_sha256}</code>
        </dd>
        <dt>{t("development.facts.expires_at")}</dt>
        <dd>{stamp(plan.expires_at)}</dd>
      </dl>
      {plan.reason && <p className="warning">{plan.reason}</p>}
      <DispatchRouteFacts route={plan.execution_route} />
      <ContextFacts title={t("development.facts.task")} value={plan.context.task} />
      {plan.context.requirement && (
        <ContextFacts title={t("development.facts.requirement")} value={plan.context.requirement} />
      )}
      <section className="tool-context">
        <h4>{t("development.facts.independent_testing_and_human_acceptance")}</h4>
        {tests.length ? (
          <ul>
            {tests.map((node) => (
              <li key={node.id}>
                {node.name} · <code>{node.id}</code>
              </li>
            ))}
          </ul>
        ) : (
          <p className="warning">
            {t("development.facts.this_workflow_has_no_independent_test_node")}</p>
        )}
        <p>
          {t("development.facts.tool_completion_establishes_execution_facts_only_independent")}</p>
      </section>
      {typeof agent?.config?.goal === "string" && agent.config.goal !== "" && (
        <details>
          <summary>{t("development.facts.complete_fixed_input_for_the_tool")}</summary>
          <pre className="output">{String(agent.config.goal)}</pre>
        </details>
      )}
    </section>
  );
}
export function UsageFacts({
  usage,
  usageState,
  step = false,
}: {
  usage?: ToolUsage;
  usageState?: UsageState;
  step?: boolean;
}) { useLocale();
  const entries: [keyof ToolUsage, string][] = [
    ["input_tokens", t("development.facts.input_tokens")],
    ["cached_input_tokens", t("development.facts.of_which_cached_input")],
    ["cache_write_input_tokens", t("development.facts.cache_write_input")],
    ["output_tokens", t("development.facts.output_tokens")],
    ["reasoning_output_tokens", t("development.facts.of_which_reasoning_output")],
  ];
  return (
    <section aria-label={t("development.facts.reported_usage_and_completeness")}>
      <p>
        {usageState === "complete"
          ? step
            ? t("development.facts.usage_reported_for_this_step_not_a")
            : t("development.facts.complete_reported_usage_for_this_turn")
          : usageState === "partial"
            ? t("development.facts.partial_reported_subtotal_not_a_complete_whole")
            : usageState === "unreported"
              ? t("development.facts.usage_unreported")
              : t("development.facts.usage_completeness_unreported")}
      </p>
      {usageState !== "unreported" && usage && (
        <dl>
          {entries.map(([key, title]) => (
            <div className="tool-usage-row" key={key}>
              <dt>{title}</dt>
              <dd>
                {typeof usage[key] === "number" &&
                Number.isSafeInteger(usage[key]) &&
                usage[key]! >= 0
                  ? formatNumber(usage[key]!)
                  : t("development.facts.unreported")}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}
export function ToolExecutionFacts({
  state,
  evidence,
  events = [],
  fixedOpenCode,
  modelBinding,
  project,
  tests,
  workspaceOperations,
}: {
  state: string;
  evidence?: ToolEvidence;
  events?: ToolEvent[];
  fixedOpenCode?:OpenCodeExecutionCapability;
  modelBinding?:ModelBinding;
  project?:string;
  tests?:OpenCodeRunTest[];
  workspaceOperations?:{scope:AttemptToolScope;events:readonly unknown[];evidence?:WorkspaceEvidence;profileSHA?:string;unconfirmed?:boolean};
}) { useLocale();
  const openCode=openCodeRunFacts({state,evidence,events,fixedOpenCode}),octop=evidence?.profile.adapter==='octop'&&evidence.profile.protocol==='octop-harness-acp-v1';
  const host=workspaceOperations?workspaceOperationsForAttempt(workspaceOperations.events,workspaceOperations.scope,workspaceOperations.profileSHA):undefined;
  const hostFacts=workspaceOperations&&host?<WorkspaceOperations scope={workspaceOperations.scope} projection={host} workspaceEvidence={workspaceOperations.evidence} context="current" unconfirmed={workspaceOperations.unconfirmed}/>:null;
  if (!evidence && !events.length&&!openCode&&!modelBinding) return hostFacts;
  const recent = toolEventRows(events).slice(-40),
    last = events.at(-1),
    session =
      evidence?.session_id ||
      [...events].reverse().find((event) => event.session_id)?.session_id,
    usageEvent = [...events]
      .reverse()
      .find(
        (event) => event.kind === "usage" || event.usage_state || event.usage,
      ),
    usage = evidence ? evidence.usage : usageEvent?.usage,
    reportedState = evidence ? evidence.usage_state : usageEvent?.usage_state,
    usageState =
      reportedState === "complete" ||
      reportedState === "partial" ||
      reportedState === "unreported"
        ? reportedState
        : undefined;
  return (
    <section className="tool-execution-facts" aria-label={t("development.facts.external_tool_execution_facts")}>
      <h4>{t("development.facts.external_tool_execution_facts")}</h4>
      <dl>
        <dt>{t("development.facts.platform_execution_state")}</dt>
        <dd>{stateLabel(state)}</dd>
        {evidence && (
          <>
            <dt>{t("development.facts.fixed_tool")}</dt>
            <dd>
              {evidence.profile.command_id} · {evidence.profile.tool_version}
            </dd>
            {!openCode&&!octop&&<><dt>{t("development.facts.tool_report")}</dt>
            <dd>
              {(
                {
                  completed: t("development.facts.tool_reported_turn_completion"),
                  failed: t("development.facts.tool_reported_turn_failure"),
                  unknown: t("development.facts.tool_result_unknown"),
                } as Record<string, string>
              )[evidence.terminal] || t("development.facts.tool_provided_no_recognized_terminal_result")}
            </dd></>}
            <dt>{t("development.facts.recorded_events")}</dt>
            <dd>{formatNumber(evidence.event_count)}</dd>
          </>
        )}
        {!openCode&&!octop&&session && (
          <>
            <dt>{t("development.facts.external_session_id")}</dt>
            <dd>
              <code>{session}</code>
            </dd>
          </>
        )}
      </dl>
      {!openCode&&!octop&&session && (
        <p className="muted">
          {t("development.facts.the_session_id_is_for_traceability_and")}</p>
      )}
      <ExecutionModelFacts binding={modelBinding} evidence={evidence} state={state} openCodeRun={openCode} tests={tests} project={project}/>
      <UsageFacts
        usage={usage ?? undefined}
        usageState={usageState}
        step={!evidence}
      />
      {hostFacts}
      {state === "running" && last?.kind === "waiting_input" && (
        <p className="warning">
          {t("development.facts.waiting_for_external_input_the_process_still")}</p>
      )}
      {events.length > 40 && (
        <p className="muted">
          {t("development.facts.only_the_latest_40_items_in_this")}</p>
      )}
      <div className="tool-event-list">
        {recent.map(({ event, key }) => (
          <article key={key}>
            <ToolEventContent event={event}/>
          </article>
        ))}
      </div>
      <p className="muted">
        {t("development.facts.tool_messages_and_usage_are_external_reports")}</p>
    </section>
  );
}
// Shared typed projection only; each caller owns its source/raw envelope and cap.
export function ToolEventContent({event}:{event:ToolEvent}) { useLocale();
  return <><strong>{toolEventNames[event.kind] || t("development.facts.tool_fact")}</strong>
    {event.status && <small>{event.status}</small>}<NativeToolMessage event={event}/></>;
}
