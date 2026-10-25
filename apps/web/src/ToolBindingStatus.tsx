import { tr as t, useLocale, formatDateTime, renderDiagnostic } from "./i18n";
import { useEffect } from "react";
import { query, stateLabel } from "./api";
import { ErrorNotice } from "./ui";
import {
  toolReturnPanel,
  type ToolProfile,
  type ToolRunBinding,
} from "./tool-model";
import { toolReason } from "./ToolFacts";
import { useToolRead } from "./tool-hooks";
type Props = {
  binding: ToolRunBinding;
  project: string;
  state: string;
  profile?: ToolProfile;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
};
export function ToolBindingFacts({
  binding,
  project,
  state,
  profile,
  loading,
  error,
  onRetry,
}: Props) { useLocale();
  const matches =
    profile?.project_id === project &&
    profile.id === binding.profile_id &&
    profile.profile.profile_sha256 === binding.profile_sha256 &&
    profile.profile.command_id === binding.command;
  const eligible = matches && profile?.state === "enabled" && profile.available;
  const href =
    "?" +
    new URLSearchParams({
      project,
      view: "settings",
      sub: "tools",
      record: binding.profile_id,
      panel: toolReturnPanel(binding.run_id, binding.node_id),
    });
  return (
    <section
      className="tool-execution-facts tool-binding-status"
      aria-label={t("development.binding.fixed_tool_and_current_eligibility_for_new")}
    >
      <h4>{t("development.binding.fixed_tool_context")}</h4>
      <dl>
        <dt>{t("development.binding.this_platform_execution")}</dt>
        <dd>
          {state === "ready" ? t("development.binding.awaiting_dispatch_dependencies_ready") : stateLabel(state)}
        </dd>
        <dt>{t("development.binding.fixed_command")}</dt>
        <dd>
          <code>{binding.command || t("development.binding.no_unique_permitted_command_provided")}</code>
        </dd>
        <dt>{t("development.binding.fixed_configuration")}</dt>
        <dd>
          <code>{binding.profile_id}</code>
        </dd>
        <dt>{t("development.binding.fingerprint_fixed_for_this_run")}</dt>
        <dd>
          <code>{binding.profile_sha256 || t("development.binding.fixed_fingerprint_not_read_yet")}</code>
        </dd>
      </dl>
      <h5>{t("development.binding.current_eligibility_for_new_dispatch")}</h5>
      {error ? (
        <>
          <ErrorNotice error={renderDiagnostic(error)} onRetry={onRetry} />
          <p className="warning">
            {t("development.binding.current_eligibility_is_unconfirmed_the_last_read")}</p>
        </>
      ) : !profile ? (
        <p role="status">
          {loading ? t("development.binding.reading_current_tool_eligibility") : t("development.binding.current_tool_eligibility_not_read_yet")}
        </p>
      ) : (
        <>
          <p className={eligible ? "success" : "warning"}>
            {!matches
              ? t("development.binding.the_run_s_fixed_fingerprint_does_not")
              : eligible
                ? t("development.binding.this_read_meets_new_dispatch_conditions_the")
                : t("development.binding.current_conditions_do_not_permit_new_dispatch")}
          </p>
          {matches && profile.reason && (
            <div>
              <p>{toolReason(profile.reason, profile.reason_diagnostic)}</p>
              <details>
                <summary>{t("development.facts.original_reason")}</summary>
                <code>{profile.reason}</code>
              </details>
            </div>
          )}
          <dl>
            <dt>{t("development.binding.current_worker_identity")}</dt>
            <dd>
              <code>{profile.worker_id}</code>
            </dd>
            <dt>{t("development.facts.last_report")}</dt>
            <dd>{profile.last_seen_at ? formatDateTime(profile.last_seen_at,{dateStyle:"short",timeStyle:"medium"}) : t("development.facts.no_official_observation_yet")}</dd>
          </dl>
          {loading && (
            <small className="muted">
              {t("development.binding.updating_eligibility_the_previous_read_is_retained")}</small>
          )}
        </>
      )}
      <p className="muted">
        {t("development.binding.current_eligibility_describes_new_dispatch_only_it")}</p>
      <a href={href}>{t("development.binding.view_tool_configuration_and_current_reason")}</a>
    </section>
  );
}
export function ToolBindingStatus({
  project,
  runID,
  node,
}: {
  project: string;
  runID: string;
  node: {
    id: string;
    type: string;
    state: string;
    command?: string[];
    tool_profile_id?: string;
    tool_profile_sha256?: string;
  };
}) { useLocale();
  const id = node.tool_profile_id || "",
    read = useToolRead<ToolProfile>(
      id ? "/tool-profiles/" + encodeURIComponent(id) + query(project) : "",
    );
  useEffect(() => {
    if (!id) return;
    const timer = setInterval(() => void read.read(), 10000);
    return () => clearInterval(timer);
  }, [id, read.read]);
  if (node.type !== "agent" || !id) return null;
  return (
    <ToolBindingFacts
      project={project}
      state={node.state}
      profile={read.data}
      loading={read.loading}
      error={read.error}
      onRetry={() => void read.read()}
      binding={{
        profile_id: id,
        profile_sha256: node.tool_profile_sha256 || "",
        command: node.command?.length === 1 ? node.command[0] : "",
        run_id: runID,
        node_id: node.id,
      }}
    />
  );
}
