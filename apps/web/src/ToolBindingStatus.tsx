import { tr as t, useLocale, formatDateTime, renderDiagnostic } from "./i18n";
import { useEffect, type MouseEvent } from "react";
import { ArrowRight } from "lucide-react";
import { query, stateLabel } from "./api";
import { ErrorNotice } from "./ui";
import {
  toolReturnPanel,
  type ToolProfile,
  type ToolRunBinding,
} from "./tool-model";
import { toolReason } from "./ToolFacts";
import { useToolRead } from "./tool-hooks";
import "./ToolBindingStatus.css";

const commandID = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/;
const digest = /^[a-f0-9]{64}$/;
const boundNodeTypes = new Set(["agent", "test", "command", "terminal_session"]);
const readInterval = 10_000;

type Props = {
  binding: ToolRunBinding;
  project: string;
  state: string;
  profile?: ToolProfile;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
};

function currentProfileStatus(profile: ToolProfile | undefined, project: string, binding: ToolRunBinding) {
  if (!profile ||
      profile.project_id !== project ||
      profile.id !== binding.profile_id ||
      typeof profile.available !== "boolean" ||
      typeof profile.worker_id !== "string" ||
      typeof profile.reason !== "string" ||
      (profile.last_seen_at !== undefined && typeof profile.last_seen_at !== "string") ||
      !["enabled", "paused", "revoked"].includes(profile.state) ||
      typeof profile.profile?.command_id !== "string" ||
      typeof profile.profile?.profile_sha256 !== "string") {
    return "unverified";
  }
  if (profile.profile.profile_sha256 !== binding.profile_sha256 ||
      profile.profile.command_id !== binding.command) return "mismatch";
  return profile.state === "enabled" && profile.available ? "eligible" : "blocked";
}

function guardProfileNavigation(event: MouseEvent<HTMLAnchorElement>) {
  if (event.button === 0 && !event.ctrlKey && !event.metaKey && !event.shiftKey && !event.altKey &&
      !window.dispatchEvent(new Event("suma:navigate", { cancelable: true }))) event.preventDefault();
}

export function ToolBindingFacts({
  binding,
  project,
  state,
  profile,
  loading,
  error,
  onRetry,
}: Props) {
  useLocale();
  const eligibility = error ? "unverified" : loading ? "loading" : currentProfileStatus(profile, project, binding);
  const matchedProfile = eligibility === "eligible" || eligibility === "blocked" ? profile : undefined;
  const statusLabel = {
    loading: "development.binding.reading_current_tool_eligibility",
    unverified: error ? "development.binding.current_eligibility_is_unconfirmed_the_last_read" :
      profile ? "run-tool-binding.unverified_response" : "development.binding.current_tool_eligibility_not_read_yet",
    mismatch: "run-tool-binding.fixed_configuration_mismatch",
    eligible: "development.binding.this_read_meets_new_dispatch_conditions_the",
    blocked: "development.binding.current_conditions_do_not_permit_new_dispatch",
  }[eligibility];
  const name = typeof matchedProfile?.name === "string" && matchedProfile.name.trim() ? matchedProfile.name : binding.profile_id;
  const reason = typeof matchedProfile?.reason === "string" ? matchedProfile.reason : "";
  const panel = toolReturnPanel(binding.run_id, binding.node_id, binding.attempt);
  const href = panel && "?" + new URLSearchParams({ project, view: "settings", sub: "tools", record: binding.profile_id, panel });
  return (
    <section
      className="tool-execution-facts tool-binding-status"
      aria-label={t("development.binding.fixed_tool_and_current_eligibility_for_new")}
      data-run-tool-binding
    >
      <header className="tool-binding-status__heading">
        <h4>{t("development.binding.fixed_tool_context")}</h4>
        <p><span>{t("development.binding.fixed_configuration")}</span><strong><bdi>{name}</bdi></strong></p>
        <p><span>{t("run-tool-binding.current_node_state")}</span>
          {state === "ready" ? t("development.binding.awaiting_dispatch_dependencies_ready") : stateLabel(state)}
        </p>
      </header>
      <div className="tool-binding-status__current" aria-busy={loading}>
        <h5>{t("development.binding.current_eligibility_for_new_dispatch")}</h5>
        {error && <ErrorNotice error={renderDiagnostic(error)} onRetry={onRetry} />}
        <p role="status" data-run-tool-eligibility={eligibility} className={eligibility === "eligible" ? "success" :
          eligibility === "blocked" || eligibility === "mismatch" ? "warning" : "muted"}>
          {t(statusLabel)}
        </p>
        {eligibility === "blocked" && reason && <p>{toolReason(reason, matchedProfile?.reason_diagnostic)}</p>}
      </div>
      {href && <a className="tool-binding-status__action" data-run-tool-profile-link href={href} onClick={guardProfileNavigation}>
        {t("development.binding.view_tool_configuration_and_current_reason")}<ArrowRight size={16} aria-hidden="true" />
      </a>}
      <details className="tool-binding-status__details">
        <summary>{t("run-tool-binding.details")}</summary>
        <dl>
          <dt>{t("development.binding.fixed_configuration")}</dt>
          <dd><code>{binding.profile_id}</code></dd>
          <dt>{t("development.binding.fixed_command")}</dt>
          <dd><code>{binding.command}</code></dd>
          <dt>{t("development.binding.fingerprint_fixed_for_this_run")}</dt>
          <dd><code>{binding.profile_sha256}</code></dd>
          {matchedProfile && <>
            <dt>{t("development.binding.current_worker_identity")}</dt>
            <dd><code>{matchedProfile.worker_id}</code></dd>
            <dt>{t("development.facts.last_report")}</dt>
            <dd>{matchedProfile.last_seen_at ? formatDateTime(matchedProfile.last_seen_at, { dateStyle: "short", timeStyle: "medium" }) :
              t("development.facts.no_official_observation_yet")}</dd>
          </>}
          {reason && <><dt>{t("development.facts.original_reason")}</dt><dd><code>{reason}</code></dd></>}
        </dl>
        <p className="muted">{t("development.binding.current_eligibility_describes_new_dispatch_only_it")}</p>
      </details>
    </section>
  );
}
export function ToolBindingStatus({
  project,
  runID,
  node,
  attempt,
}: {
  project: string;
  runID: string;
  attempt?: number;
  node: {
    id: string;
    type: string;
    state: string;
    command?: string[];
    tool_profile_id?: string;
    tool_profile_sha256?: string;
  };
}) {
  const { tool_profile_id: id, tool_profile_sha256: sha } = node;
  const command = Array.isArray(node.command) && node.command.length === 1 ? node.command[0] : undefined;
  if (!project || !runID || !node.id || !boundNodeTypes.has(node.type) ||
      typeof id !== "string" || !commandID.test(id) ||
      typeof sha !== "string" || !digest.test(sha) ||
      typeof command !== "string" || !commandID.test(command)) return null;

  const binding = { profile_id: id, profile_sha256: sha, command, run_id: runID, node_id: node.id, attempt };
  return <ToolBindingReader key={JSON.stringify([project, runID, node.id, id, sha, command])} project={project} state={node.state} binding={binding} />;
}

function ToolBindingReader({ binding, project, state }: Pick<Props, "binding" | "project" | "state">) {
  const read = useToolRead<ToolProfile>("/tool-profiles/" + encodeURIComponent(binding.profile_id) + query(project));
  useEffect(() => {
    if (read.loading || read.error) return;
    let requested = false;
    function refreshIfVisible() {
      if (requested || document.visibilityState === "hidden") return;
      requested = true;
      void read.read();
    }
    function recheckOnReturn() {
      if (read.readStartedAt !== undefined && performance.now() - read.readStartedAt >= readInterval) refreshIfVisible();
    }
    const timer = setInterval(refreshIfVisible, readInterval);
    document.addEventListener("visibilitychange", recheckOnReturn);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", recheckOnReturn);
    };
  }, [read.loading, read.error, read.read, read.readStartedAt]);
  return (
    <ToolBindingFacts
      project={project}
      state={state}
      profile={read.data}
      loading={read.loading}
      error={read.error}
      onRetry={() => void read.read()}
      binding={binding}
    />
  );
}
