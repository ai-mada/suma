import {useApi} from './api-context';
import { Trans } from "./i18n/Trans";
import { tr as t, useLocale, formatDateTime, formatNumber, renderDiagnostic } from "./i18n";
import { useEffect, useMemo, useState } from "react";
import {
  ChevronRight,
  Plus,
  RefreshCw,
  ShieldCheck,
  Terminal,
  X,
} from "lucide-react";
import { api, query, type Session } from "./api";
import { Empty, ErrorNotice, Field, Loading, Modal } from "./ui";
import { ToolWriteRecovery } from "./tool-write";
import {
  toolSessionStorage,
  useToolAction,
  useToolList,
  useToolRead,
  type ToolAction,
} from "./tool-hooks";
import {
  parseToolManifest,
  toolStateNames,
  toolReturnRun,
  type ExecutionWorker,
  type ToolObservation,
  type ToolProfile,
} from "./tool-model";
import { ToolProfileFacts } from "./ToolFacts";
import "./tool.css";
import { ToolRecoveryNotice } from "./ToolActions";
import {ToolSetupGuide} from './ToolSetupGuide';
import type { ProjectPoolRunnerPage } from "../../../packages/contracts/src";
import {
  ToolIdentity,
  currentToolIdentity,
  type SharedIdentityRead,
  type IdentityRead,
} from "./execution-pools/ToolIdentity";
type Props = {
  project: string;
  session: Session;
  selected: string;
  onSelect: (id: string) => void;
  refresh: number;
  panel: string;
};
type Dialog =
  | { kind: "create" }
  | { kind: "worker" }
  | {
      kind: "decision";
      action: "enable" | "pause" | "revoke";
      profile: ToolProfile;
    }
  | { kind: "revoke-worker"; worker: ExecutionWorker };
function canLeave() {
  return window.dispatchEvent(new Event("suma:navigate", { cancelable: true }));
}
export function ToolWorkspace({
  project,
  session,
  selected,
  onSelect,
  refresh,
  panel,
}: Props) { const api=useApi(); useLocale();
  const [revision, setRevision] = useState(0),
    [search, setSearch] = useState(""),
    [dialog, setDialog] = useState<Dialog>(),
    [credential, setCredential] = useState<{
      worker_id: string;
      token?: string;
    }>(),
    [revealed, setRevealed] = useState(false),
    [live, setLive] = useState(true);
  const recovery = useMemo(
    () =>
      new ToolWriteRecovery(session.organization.id +
          ":" +
          session.user.id +
          ":" +
          project +
          ":profiles", toolSessionStorage(), api),
    [session.organization.id, session.user.id, project, api],
  );
  const action = useToolAction(recovery, () => setDialog(undefined)),
    profiles = useToolList<ToolProfile>(
      project,
      "tool-profiles",
      refresh + revision,
    ),
    workers = useToolRead<{ items: ExecutionWorker[] }>(
      "/release-workers" + query(project),
      refresh + revision,
    ),
    sharedWorkers = useToolRead<ProjectPoolRunnerPage>(
      "/project-execution/runners" + query(project),
      refresh + revision,
    ),
    detail = useToolRead<ToolProfile>(
      selected
        ? "/tool-profiles/" + encodeURIComponent(selected) + query(project)
        : "",
      refresh + revision,
    );
  useEffect(() => {
    if (!live) return;
    const timer = setInterval(() => void profiles.read(), 10000);
    return () => clearInterval(timer);
  }, [live, profiles.read]);
  useEffect(() => {
    if (!selected) return;
    const timer = setInterval(() => void detail.read(), 10000);
    return () => clearInterval(timer);
  }, [selected, detail.read]);
  const pending = !!action.pending,
    current = detail.data,
    stale = detail.loading || !!detail.error,
    filtered = profiles.items?.filter((profile) =>
      (
        profile.name +
        " " +
        profile.worker_id +
        " " +
        profile.profile.command_id
      )
        .toLowerCase()
        .includes(search.toLowerCase()),
    );
  function saved(path: string, value: unknown) {
    setDialog(undefined);
    setRevision((before) => before + 1);
    if (!value || typeof value !== "object") return;
    const result = value as Record<string, unknown>;
    if (path === "/release-workers") {
      setRevealed(false);
      setCredential({
        worker_id: String(result.worker_id || ""),
        token: typeof result.token === "string" ? result.token : undefined,
      });
    } else if (
      path.startsWith("/tool-profiles") &&
      typeof result.id === "string"
    )
      onSelect(result.id);
  }
  const returnRun = toolReturnRun(panel);
  return (
    <div className="tool-workspace">
      <div className="tool-intro">
        <div>
          <h2>{t("development.tools.execution_tools")}</h2>
          <p>{t("development.tools.review_local_permitted_configurations_live_observations_and")}</p>
        </div>
        <a
          href={
            "?" +
            new URLSearchParams({ project, view: "development", sub: "tasks" })
          }
        >
          {t("development.tools.return_to_task_dispatch")}</a>
      </div>
      <ToolSetupGuide/>
      {returnRun && (
        <a
          onClick={(event) => {
            if (!canLeave()) event.preventDefault();
          }}
          href={
            "?" +
            new URLSearchParams({
              project,
              view: "development",
              sub: "runs",
              run: returnRun.run,
              node: returnRun.node,
            })
          }
        >
          {t("development.tools.return_to_the_original_run_and_selected")}</a>
      )}
      <div className="info-strip">
        <ShieldCheck size={18} />
        <p>
          {t("development.tools.configuration_probing_actual_execution_and_acceptance_are")}</p>
      </div>
      <ToolRecoveryNotice action={action} onRestored={saved} />
      <div className={"tool-columns " + (selected ? "with-detail" : "")}>
        <section className="panel tool-list">
          <div className="list-toolbar">
            <span>
              {t("development.tools.fixed_tool_configuration")}<b>{profiles.items ? formatNumber(profiles.total) : "—"}</b>
            </span>
            <div className="inline">
              <button
                className="icon-button"
                aria-label={t("development.tools.refresh_tool_configurations")}
                disabled={profiles.loading}
                onClick={() => {
                  setLive(true);
                  void profiles.read();
                }}
              >
                <RefreshCw size={15} />
              </button>
              <button
                className="primary"
                disabled={pending || action.busy}
                onClick={() => {
                  action.clearError();
                  void sharedWorkers.read();
                  setDialog({ kind: "create" });
                }}
              >
                <Plus size={14} />
                {t("development.tools.register_configuration")}</button>
            </div>
          </div>
          <div className="filter-toolbar">
            <input
              aria-label={t("development.tools.search_loaded_tool_configurations")}
              placeholder={t("development.tools.filter_by_name_worker_or_command")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <small>
              {t("development.tools.loaded_count",{loaded:formatNumber(profiles.items?.length||0),total:formatNumber(profiles.total)})}
            </small>
          </div>
          {profiles.error && (
            <ErrorNotice
              error={profiles.error}
              onRetry={() =>
                void profiles.read(profiles.error_action === "more")
              }
            />
          )}{" "}
          {!profiles.items ? (
            profiles.loading ? (
              <Loading />
            ) : (
              <Empty
                title={t("development.tools.no_official_configurations_read_yet")}
                description={t("development.tools.restore_the_connection_first_historical_declarations_cannot")}
              />
            )
          ) : filtered?.length ? (
            <div>
              {filtered.map((profile) => (
                <button
                  className={
                    "tool-resource " +
                    (profile.id === selected ? "selected" : "")
                  }
                  key={profile.id}
                  onClick={() => {
                    if (canLeave()) onSelect(profile.id);
                  }}
                >
                  <span>
                    <strong>{profile.name}</strong>
                    <small>
                      {profile.worker_id} · {profile.profile.command_id}
                    </small>
                    <small>
                      {toolStateNames[profile.state]} ·{" "}
                      {profile.available ? t("development.tools.dispatchable_at_last_read") : t("development.tools.current_conditions_unconfirmed")}
                    </small>
                  </span>
                  <ChevronRight size={15} />
                </button>
              ))}
            </div>
          ) : (
            <Empty
              title={search ? t("development.tools.no_matching_configurations_in_this_window") : t("development.tools.no_execution_tools_registered")}
              description={
                search
                  ? t("development.tools.adjust_the_filter_or_load_more")
                  : t("development.tools.register_an_execution_worker_identity_then_choose")
              }
            />
          )}
          <div className="list-footer">
            <small>
              {live ? t("development.tools.auto_refresh") : t("development.tools.auto_refresh_paused")}</small>
            {profiles.next_cursor && (
              <button
                disabled={profiles.loading}
                onClick={() => {
                  setLive(false);
                  void profiles.read(true);
                }}
              >
                {t("development.tools.load_more_configurations")}</button>
            )}
          </div>
        </section>
        {selected && (
          <section className="panel tool-detail">
            <header className="detail-top">
              <button
                className="text-button"
                onClick={() => {
                  if (canLeave()) onSelect("");
                }}
              >
                {t("development.tools.return_to_configuration_list")}</button>
              <button
                className="icon-button"
                aria-label={t("development.tools.close_tool_details")}
                onClick={() => {
                  if (canLeave()) onSelect("");
                }}
              >
                <X size={17} />
              </button>
            </header>
            <div className="tool-detail-content">
              {detail.error && (
                <ErrorNotice
                  error={detail.error}
                  onRetry={() => void detail.read()}
                />
              )}{" "}
              {detail.loading && <p role="status">{t("development.tools.reading_current_dispatch_conditions")}</p>}
              {current && (
                <>
                  <ToolProfileFacts profile={current} />
                  {stale && (
                    <p className="warning">
                      {t("development.tools.the_last_read_is_retained_current_connection")}</p>
                  )}
                  <div className="tool-action-bar">
                    {current.state !== "revoked" && (
                      <>
                        {current.state === "paused" ? (
                          <button
                            className="primary"
                            disabled={stale || pending || action.busy}
                            onClick={() => {
                              action.clearError();
                              setDialog({
                                kind: "decision",
                                action: "enable",
                                profile: current,
                              });
                            }}
                          >
                            {t("development.tools.review_and_enable")}</button>
                        ) : (
                          <button
                            disabled={stale || pending || action.busy}
                            onClick={() => {
                              action.clearError();
                              setDialog({
                                kind: "decision",
                                action: "pause",
                                profile: current,
                              });
                            }}
                          >
                            {t("development.tools.pause_configuration")}</button>
                        )}
                        <button
                          disabled={stale || pending || action.busy}
                          onClick={() => {
                            action.clearError();
                            setDialog({
                              kind: "decision",
                              action: "revoke",
                              profile: current,
                            });
                          }}
                        >
                          {t("development.tools.revoke_configuration")}</button>
                      </>
                    )}
                  </div>
                </>
              )}
            </div>
          </section>
        )}
      </div>
      <section className="panel tool-workers">
        <div className="tool-section-heading">
          <div>
            <h3>{t("development.tools.execution_worker_identity")}</h3>
            <p className="muted">
              {t("development.tools.reuse_a_runner_credential_bound_by_the")}</p>
          </div>
          <button
            disabled={pending || action.busy}
            onClick={() => {
              action.clearError();
              setDialog({ kind: "worker" });
            }}
          >
            <Plus size={14} />
            {t("development.tools.register_execution_worker")}</button>
        </div>
        {workers.error && (
          <ErrorNotice
            error={workers.error}
            onRetry={() => void workers.read()}
          />
        )}{" "}
        {!workers.data ? (
          workers.loading ? (
            <Loading />
          ) : null
        ) : workers.data.items.length ? (
          workers.data.items.map((worker) => (
            <article className="tool-worker-row" key={worker.id}>
              <div>
                <strong>
                  <Terminal size={14} /> {worker.worker_id}
                </strong>
                <small>
                  {t("development.tools.expires_at",{status:worker.status === "active"?t("development.tools.credential_registered_consult_tool_observations_for_online"):t("development.tools.credential_revoked"),at:formatDateTime(worker.expires_at,{dateStyle:"short",timeStyle:"medium"})})}
                </small>
                <small>
                  {worker.deployment ? t("development.tools.also_permitted_to_release") : t("development.tools.no_deployment_permission")}
                </small>
              </div>
              {worker.status === "active" && (
                <button
                  disabled={pending || action.busy}
                  onClick={() => {
                    action.clearError();
                    setDialog({ kind: "revoke-worker", worker });
                  }}
                >
                  {t("development.tools.revoke_worker_credential")}</button>
              )}
            </article>
          ))
        ) : (
          <Empty
            title={t("development.tools.no_bound_execution_identity")}
            description={t("development.tools.registration_issues_a_one_time_project_scoped")}
          />
        )}
      </section>
      <Modal
        open={!!dialog}
        onClose={() => {
          if (!action.busy && !action.pending) setDialog(undefined);
        }}
        title={
          dialog?.kind === "create"
            ? t("development.tools.register_fixed_tool_configuration")
            : dialog?.kind === "worker"
              ? t("development.tools.register_execution_worker_identity")
              : dialog?.kind === "revoke-worker"
                ? t("development.tools.revoke_execution_worker_credential")
                : dialog?.kind === "decision"
                  ? {
                      enable: t("development.tools.review_and_enable_configuration"),
                      pause: t("development.tools.pause_tool_configuration"),
                      revoke: t("development.tools.revoke_fixed_configuration"),
                    }[dialog.action]
                  : t("development.tools.execution_tools")
        }
        description={t("development.tools.use_the_current_project_s_actual_identity")}
      >
        {action.pending ? (
          <ToolRecoveryNotice action={action} onRestored={saved} />
        ) : (
          action.error && <ErrorNotice error={action.error} />
        )}
        {dialog?.kind === "create" && (
          <ToolProfileForm
            project={project}
            workers={workers}
            sharedWorkers={sharedWorkers}
            action={action}
            onSaved={(value) => saved("/tool-profiles", value)}
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === "worker" && (
          <WorkerForm
            project={project}
            action={action}
            onSaved={(value) => saved("/release-workers", value)}
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === "decision" && (
          <ProfileDecisionForm
            project={project}
            value={dialog.profile}
            decision={dialog.action}
            action={action}
            onSaved={(value) =>
              saved("/tool-profiles/" + value.id + "/commands", value)
            }
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === "revoke-worker" && (
          <form
            className="tool-form"
            onSubmit={async (event) => {
              event.preventDefault();
              const worker = dialog.worker;
              const result = await action.run<ExecutionWorker>(
                "/release-workers/" + encodeURIComponent(worker.id),
                {
                  project_id: project,
                  expected_version: worker.version,
                  status: "revoked",
                },
                "PATCH",
              );
              if (result) saved("/release-workers/" + worker.id, result);
            }}
          >
            <p className="warning">
              <Trans i18nKey="development.tools.revoke_worker_warning" values={{worker:dialog.worker.worker_id}} components={{worker:<strong/>}}/></p>
            <div className="dialog-actions">
              <button
                type="button"
                disabled={action.busy || pending}
                onClick={() => setDialog(undefined)}
              >
                {t("development.workflow.cancel")}</button>
              <button
                className="primary"
                disabled={action.busy || pending || action.conflict}
              >
                {t("development.tools.confirm_worker_credential_revocation")}</button>
            </div>
          </form>
        )}
      </Modal>
      <Modal
        open={!!credential}
        onClose={() => {
          setCredential(undefined);
          setRevealed(false);
        }}
        title={t("development.tools.execution_worker_credential_for_this_registration")}
        description={t("development.tools.the_original_value_remains_only_in_this")}
      >
        {credential && (
          <div className="tool-token">
            <strong>{credential.worker_id}</strong>
            {credential.token ? (
              <>
                <p className="warning">
                  {t("development.tools.save_this_credential_in_the_controlled_runner")}</p>
                <Field label={t("development.tools.one_time_runner_credential")}>
                  <input
                    type={revealed ? "text" : "password"}
                    readOnly
                    value={credential.token}
                    autoComplete="off"
                  />
                </Field>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={revealed}
                    onChange={(event) => setRevealed(event.target.checked)}
                  />
                  {t("development.tools.show_the_original_value_in_this_dialog")}</label>
              </>
            ) : (
              <p className="warning">
                {t("development.tools.the_original_registration_receipt_was_recovered_but")}</p>
            )}
            <button
              onClick={() => {
                setCredential(undefined);
                setRevealed(false);
              }}
            >
              {t("development.tools.close_and_clear_the_original_value")}</button>
          </div>
        )}
      </Modal>
    </div>
  );
}
function ToolProfileForm({
  project,
  workers,
  sharedWorkers,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  workers: IdentityRead;
  sharedWorkers: SharedIdentityRead;
  action: ToolAction;
  onSaved: (value: ToolProfile) => void;
  onCancel: () => void;
}) { useLocale();
  const [manifest, setManifest] = useState(""),
    [observations, setObservations] = useState<ToolObservation[]>([]),
    [command, setCommand] = useState(""),
    [error, setError] = useState<unknown>(""),
    [name, setName] = useState(""),
    [worker, setWorker] = useState("");
  const selected = observations.find(
      (item) => item.profile.command_id === command,
    ),
    blocked = action.busy || !!action.pending;
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        setError("");
        if (!currentToolIdentity(worker, workers, sharedWorkers)) {
          setError(t("tools.identity.authorization_unconfirmed"));
          return;
        }
        if (!selected?.available) {
          setError(t("development.tools.choose_an_available_probed_configuration_with_complete"));
          return;
        }
        const result = await action.run<ToolProfile>("/tool-profiles", {
          project_id: project,
          worker_id: worker,
          name,
          profile: selected.profile,
        });
        if (result) onSaved(result);
      }}
    >
      {!!error && <ErrorNotice error={renderDiagnostic(error)} />}
      <p>
        <Trans i18nKey="development.tools.inventory_instructions" components={{command:<code/>}}/></p>
      <fieldset className="form-fields" disabled={blocked}>
        <Field label={t("development.tools.configuration_name")}>
          <input
            required
            maxLength={200}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <ToolIdentity
          local={workers}
          shared={sharedWorkers}
          value={worker}
          onChange={(value) => {
            setWorker(value);
            if (action.conflict) action.clearError();
          }}
        />
        <Field label={t("development.tools.local_non_secret_tool_inventory")}>
          <textarea
            rows={7}
            value={manifest}
            onChange={(event) => {
              setManifest(event.target.value);
              setObservations([]);
              setCommand("");
            }}
            placeholder={
              '{"items":[{"profile":{…},"available":true,"reason":""}]}'
            }
          />
        </Field>
        <button
          type="button"
          onClick={() => {
            try {
              const rows = parseToolManifest(manifest);
              setObservations(rows);
              setCommand("");
              setError("");
            } catch (error) {
              setError(error);
            }
          }}
        >
          {t("development.tools.validate_inventory_fields")}</button>
        {observations.length > 0 && (
          <>
            <Field label={t("development.tools.choose_an_inventory_configuration")}>
              <select
                required
                value={command}
                onChange={(event) => setCommand(event.target.value)}
              >
                <option value="">{t("development.tools.choose_a_probed_fixed_configuration")}</option>
                {observations.map((item) => (
                  <option
                    key={item.profile.command_id}
                    value={item.profile.command_id}
                    disabled={!item.available}
                  >
                    {t("development.tools.observed_configuration_option",{command:item.profile.command_id,version:item.profile.tool_version||t("development.tools.version_unconfirmed"),status:item.available?t("development.tools.dispatchable_at_last_read"):t("development.tools.unavailable")})}
                  </option>
                ))}
              </select>
            </Field>
            <div className="tool-observations">
              {observations
                .filter((item) => !item.available)
                .map((item) => (
                  <article key={item.profile.command_id}>
                    <strong>{t("development.tools.observation_unconfirmed",{command:item.profile.command_id})}</strong>
                    <p>{item.reason}</p>
                  </article>
                ))}
            </div>
          </>
        )}
        {selected && (
          <div className="tool-context">
            <strong>
              {selected.profile.adapter} / {selected.profile.protocol}
            </strong>
            <p>
              {selected.profile.tool_version} ·{" "}
              {selected.profile.read_only ? t("development.tools.read_only") : t("development.tools.write_permitted_separate_worktree")}
            </p>
            <code>{selected.profile.profile_sha256}</code>
          </div>
        )}
      </fieldset>
      <div className="dialog-actions">
        <button type="button" disabled={blocked} onClick={onCancel}>
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={
            blocked ||
            action.conflict ||
            !selected?.available ||
            !worker ||
            !currentToolIdentity(worker, workers, sharedWorkers) ||
            !name.trim()
          }
        >
          {t("development.tools.save_as_paused_configuration")}</button>
      </div>
    </form>
  );
}
function WorkerForm({
  project,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  action: ToolAction;
  onSaved: (value: unknown) => void;
  onCancel: () => void;
}) { useLocale();
  const [worker, setWorker] = useState("");
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        const result = await action.run("/release-workers", {
          project_id: project,
          worker_id: worker,
          profiles: [],
          deployment: false,
        });
        if (result) onSaved(result);
      }}
    >
      <p>
        {t("development.tools.register_a_tool_only_execution_identity_using")}</p>
      <fieldset
        className="form-fields"
        disabled={action.busy || !!action.pending}
      >
        <Field label={t("development.tools.unique_execution_worker_id")}>
          <input
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{0,99}"
            maxLength={100}
            value={worker}
            onChange={(event) => {
              setWorker(event.target.value);
              if (action.conflict) action.clearError();
            }}
            placeholder={t("development.tools.e_g_team_code_runner")}
          />
        </Field>
      </fieldset>
      {action.conflict && (
        <p className="warning">
          {t("development.tools.this_worker_id_has_a_previous_registration")}</p>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={onCancel}
        >
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={action.busy || !!action.pending || action.conflict}
        >
          {t("development.tools.register_and_get_this_credential")}</button>
      </div>
    </form>
  );
}
function ProfileDecisionForm({
  project,
  value,
  decision,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  value: ToolProfile;
  decision: "enable" | "pause" | "revoke";
  action: ToolAction;
  onSaved: (value: ToolProfile) => void;
  onCancel: () => void;
}) { const api=useApi(); useLocale();
  const [current, setCurrent] = useState(value),
    [confirmed, setConfirmed] = useState(false),
    [error, setError] = useState<unknown>("");
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        const result = await action.run<ToolProfile>(
          "/tool-profiles/" + encodeURIComponent(current.id) + "/commands",
          {
            project_id: project,
            action: decision,
            expected_version: current.version,
          },
        );
        if (result) onSaved(result);
      }}
    >
      <ToolProfileFacts profile={current} />
      {!!error && <ErrorNotice error={renderDiagnostic(error)} />}
      <p className={decision === "enable" ? "muted" : "warning"}>
        {decision === "enable"
          ? t("development.tools.enabling_still_requires_matching_current_identity_live")
          : decision === "pause"
            ? t("development.tools.pausing_blocks_subsequent_dispatch_only_operations_already")
            : t("development.tools.revocation_is_irreversible_original_configurations_runs_and")}
      </p>
      <label className="check">
        <input
          type="checkbox"
          checked={confirmed}
          disabled={action.busy || !!action.pending}
          onChange={(event) => setConfirmed(event.target.checked)}
        />
        {t("development.tools.confirm_"+decision,{version:current.version})}
      </label>
      {action.conflict && (
        <div className="conflict-banner">
          <p>{t("development.tools.the_state_changed_your_decision_is_retained")}</p>
          <button
            type="button"
            onClick={async () => {
              try {
                setCurrent(
                  await api<ToolProfile>(
                    "/tool-profiles/" +
                      encodeURIComponent(current.id) +
                      query(project),
                  ),
                );
                setConfirmed(false);
                action.clearError();
              } catch (error) {
                setError(error);
              }
            }}
          >
            {t("development.tools.read_current_configuration_keep_decision")}</button>
        </div>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={onCancel}
        >
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={
            action.busy || !!action.pending || action.conflict || !confirmed
          }
        >
          {decision === "enable"
            ? t("development.tools.confirm_enabling_this_configuration")
            : decision === "pause"
              ? t("development.tools.confirm_pausing_this_configuration")
              : t("development.tools.confirm_revoking_this_configuration")}
        </button>
      </div>
    </form>
  );
}
