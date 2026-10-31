import {useApi} from './api-context';
import { Trans } from "./i18n/Trans";
import { tr as t, useLocale, formatDateTime, formatNumber, renderDiagnostic } from "./i18n";
import { useCallback, useEffect, useId, useMemo, useState } from "react";
import {
  ChevronRight,
  Plus,
  RefreshCw,
  ShieldCheck,
  Terminal,
  X,
  Pause,
  Play,
  Search,
  CircleHelp,
  CheckCircle2,
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
import { ToolProfileFacts, toolReason } from "./ToolFacts";
import { ToolReadiness, matchesToolFilter, type ToolFilter } from './ToolReadiness';
import {ToolInventoryField} from './ToolInventoryField';
import {ToolDiscoveryPanel} from './ToolDiscoveryPanel';
import "./tool.css";
import { ToolRecoveryNotice } from "./ToolActions";
import {ToolSetupGuide} from './ToolSetupGuide';
import {ToolQuickConnect} from './ToolQuickConnect';
import {validQuickConnectReceipt} from './tool-quick-connect-model';
import {AdapterConnections,type AdapterDescriptor} from './AdapterConnections';
import {DeveloperClientConnections} from './DeveloperClientConnections';
import {WorkBuddyConnections} from './WorkBuddyConnections';
import type {DeveloperClientID} from './developer-client-connections-model';
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
  | { kind: "create"; adapter?:AdapterDescriptor }
  | { kind: "quick"; adapter?:AdapterDescriptor }
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
export function ToolWorkspace(props:Props) {
  // Node observations, one-time credentials and dialogs belong to this exact
  // actor/project. Original uncertain writes remain in their scoped recovery key.
  return <ScopedToolWorkspace key={JSON.stringify([props.session.organization.id,props.session.user.id,props.project])} {...props}/>;
}
function ScopedToolWorkspace({
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
    [live, setLive] = useState(true),
    [filter, setFilter] = useState<ToolFilter>('all'),
    [guideOpen, setGuideOpen] = useState(false),
    [receipt, setReceipt] = useState<{key:string;name:string;scope:string}>();
  const contextKey=JSON.stringify([session.organization.id,session.user.id,project]);
  const guideID = useId();
  const [connectionSelection,setConnectionSelection]=useState<{context:string;nonce:number;client?:DeveloperClientID;preset?:'harness'|'octop'|'qoder';cloud?:boolean}>();
  const connection=connectionSelection?.context===contextKey?connectionSelection:undefined;
  const recovery = useMemo(
    () =>
      new ToolWriteRecovery(session.organization.id +
          ":" +
          session.user.id +
          ":" +
          project +
          ":profiles", toolSessionStorage(), api,
        (value, request) => request.path !== '/tool-profiles' || validQuickConnectReceipt(value, request.body)),
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
  const refreshAll = useCallback(() => {
    if (!profiles.loading) void profiles.read();
    if (!workers.loading) void workers.read();
    if (!sharedWorkers.loading) void sharedWorkers.read();
    if (selected && !detail.loading) void detail.read();
  }, [profiles.loading, profiles.read, workers.loading, workers.read, sharedWorkers.loading, sharedWorkers.read, selected, detail.loading, detail.read]);
  useEffect(() => {
    if (!live) return;
    const timer = setInterval(() => {
      if (document.visibilityState !== 'hidden') refreshAll();
    }, 10000);
    return () => clearInterval(timer);
  }, [live, refreshAll]);
  const pending = !!action.pending,
    current = detail.data,
    stale = detail.loading || !!detail.error,
    uncertain = profiles.loading || !!profiles.error || !profiles.items,
    filtered = profiles.items?.filter((profile) => matchesToolFilter(profile, filter) &&
      (
        profile.name +
        " " +
        profile.worker_id +
        " " +
        profile.profile.command_id + ' ' + profile.profile.adapter
      )
        .toLowerCase()
        .includes(search.trim().toLowerCase()),
    );
  const identitiesKnown = !!workers.data && !workers.loading && !workers.error && !!sharedWorkers.data && !sharedWorkers.loading && !sharedWorkers.error;
  const identityCount = new Set([
    ...(workers.data?.items || []), ...(sharedWorkers.data?.items || []),
  ].filter(item => item.status === 'active').map(item => item.worker_id)).size;
  function openRegister() {
    action.clearError();
    void workers.read();
    void sharedWorkers.read();
    setDialog({kind:'create'});
  }
  function openQuickConnect() {
    action.clearError();
    void workers.read();
    void sharedWorkers.read();
    setDialog({kind:'quick'});
  }
  function openGuide() {
    setConnectionSelection(undefined);
    setGuideOpen(true);
    requestAnimationFrame(() => document.getElementById(guideID)?.querySelector('summary')?.focus());
  }
  function setupConnection(descriptor:AdapterDescriptor){
    if(pending||action.busy||!canLeave()||descriptor.implementation==='planned')return;
    if(descriptor.id==='workbuddy-cloud'&&descriptor.direction==='suma_to_tool'&&descriptor.entrypoint_kind==='cloud_api'&&descriptor.implementation==='implemented'&&descriptor.setup_kind==='manual_guide'&&!descriptor.adapter&&!descriptor.protocol){setConnectionSelection(previous=>({context:contextKey,nonce:(previous?.nonce||0)+1,cloud:true}));return;}
    if(descriptor.direction==='suma_to_tool'&&descriptor.setup_kind==='runner_profile'&&descriptor.implementation==='implemented'&&descriptor.adapter&&descriptor.protocol){
      action.clearError();void workers.read();void sharedWorkers.read();setDialog({kind:'quick',adapter:descriptor});return;
    }
    const clients:Record<string,DeveloperClientID>={'cursor-client':'cursor','claude-code-client':'claude-code','codex-client':'codex','opencode-client':'opencode','qoder-client':'qoder','zed-client':'zed','workbuddy-connector':'workbuddy'};
    if(descriptor.direction==='tool_to_suma'&&clients[descriptor.id]){setConnectionSelection(previous=>({context:contextKey,nonce:(previous?.nonce||0)+1,client:clients[descriptor.id]}));setGuideOpen(true);return;}
    if(descriptor.id==='harness-guide'&&descriptor.setup_kind==='manual_guide'){setConnectionSelection(previous=>({context:contextKey,nonce:(previous?.nonce||0)+1,preset:'harness'}));setGuideOpen(true);}
  }
  function saved(path: string, value: unknown) {
    setDialog(undefined);
    setRevision((before) => before + 1);
    if (!value || typeof value !== "object") return;
    const result = value as Record<string, unknown>;
    if(path==='/tool-profiles')setReceipt({key:'registered',name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/tool-profiles/')&&['enabled','paused','revoked'].includes(String(result.state)))setReceipt({key:String(result.state),name:String(result.name||''),scope:contextKey});
    else if(path.startsWith('/release-workers/'))setReceipt({key:'worker_revoked',name:String(result.worker_id||''),scope:contextKey});
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
      <ToolReadiness items={profiles.items} total={profiles.total} uncertain={uncertain} identitiesKnown={identitiesKnown} identityCount={identityCount} disabled={pending || action.busy} onWorker={() => { action.clearError(); setDialog({kind:'worker'}); }} onRegister={openQuickConnect} onGuide={openGuide} onSelect={id => { if(canLeave()) onSelect(id); }}/>
      <WorkBuddyConnections project={project} panel={panel} openRequest={connection?.cloud?connection.nonce:undefined}/>
      <details className="panel adapter-catalog-disclosure" key={contextKey}>
        <summary>
          <ChevronRight aria-hidden="true" size={20}/>
          <span className="adapter-catalog-disclosure-copy"><strong>{t('adapter-connections.title')}</strong><span>{t('adapter-connections.intro')}</span></span>
          <span className="adapter-catalog-disclosure-action"><span className="when-closed">{t('adapter-connections.open_catalog')}</span><span className="when-open">{t('adapter-connections.close_catalog')}</span></span>
        </summary>
        <AdapterConnections project={project} onSetup={setupConnection}/>
      </details>
      {connection?.client?<section className="panel"><button type="button" onClick={openGuide}>{t('adapter-connections.return_guide')}</button><DeveloperClientConnections key={contextKey} project={project} request={{client:connection.client,nonce:connection.nonce}}/></section>:<ToolSetupGuide project={project} id={guideID} expanded={guideOpen} onExpandedChange={setGuideOpen} presetRequest={connection?.preset?{preset:connection.preset,nonce:connection.nonce}:undefined}/>}
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
      <ToolRecoveryNotice action={action} onRestored={saved} />
      {receipt?.scope===contextKey&&<div className="tool-save-receipt" data-tool-receipt role="status"><CheckCircle2 size={18} aria-hidden="true"/><p>{t('tool-workspace.receipt.'+receipt.key,{name:receipt.name})}</p><button className="icon-button" aria-label={t('tool-workspace.dismiss_receipt')} onClick={()=>setReceipt(undefined)}><X size={16}/></button></div>}
      <div className="tool-refresh-bar">
        <small>{live ? t("development.tools.auto_refresh") : t("development.tools.auto_refresh_paused")}</small>
        <div className="inline">
          <button className="text-button" onClick={() => setLive(value=>!value)}>{live?<Pause size={13}/>:<Play size={13}/>} {t(live?'tool-workspace.pause_refresh':'tool-workspace.resume_refresh')}</button>
          <button aria-label={t("development.tools.refresh_tool_configurations")} disabled={profiles.loading} onClick={refreshAll}><RefreshCw size={14}/>{t('tool-workspace.refresh')}</button>
        </div>
      </div>
      <div className={"tool-columns " + (selected ? "with-detail" : "")}>
        <section className="panel tool-list">
          <div className="list-toolbar">
            <span>
              {t("development.tools.fixed_tool_configuration")}<b>{profiles.items ? formatNumber(profiles.total) : "—"}</b>
            </span>
            <div className="inline">
              <button
                className="primary"
                disabled={pending || action.busy}
                onClick={openQuickConnect}
              >
                <Plus size={14} />
                {t('tool-quick-connect.title')}</button>
              <details className="tool-quick-advanced"><summary>{t('tool-quick-connect.advanced')}</summary><button type="button" disabled={pending||action.busy} onClick={openRegister}>{t('development.tools.register_configuration')}</button></details>
            </div>
          </div>
          <div className="tool-filter-tabs" role="group" aria-label={t('tool-workspace.filter')}>
            {(['all','available','paused','attention','revoked'] as ToolFilter[]).map(value => <button key={value} aria-pressed={filter===value} onClick={() => setFilter(value)}>{t('tool-workspace.filter.'+value)} <span>{uncertain?'—':formatNumber(profiles.items?.filter(item=>matchesToolFilter(item,value)).length||0)}</span></button>)}
          </div>
          <div className="filter-toolbar">
            <Search size={16} aria-hidden="true"/>
            <input
              aria-label={t("development.tools.search_loaded_tool_configurations")}
              placeholder={t("development.tools.filter_by_name_worker_or_command")}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <small aria-live="polite">
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
                      {profile.profile.adapter} · {profile.worker_id} · {profile.profile.command_id}
                    </small>
                    {profile.reason && <small className="tool-resource-reason">{toolReason(profile.reason, profile.reason_diagnostic)}</small>}
                  </span>
                  <span className={'tool-status-pill '+(!uncertain&&profile.state==='enabled'&&profile.available?'available':profile.state)}>{uncertain?t('tool-workspace.unconfirmed'):profile.state==='enabled'&&profile.available?t('tool-workspace.eligible'):profile.state==='enabled'?t('tool-workspace.filter.attention'):toolStateNames[profile.state]}</span>
                  <ChevronRight size={15} />
                </button>
              ))}
            </div>
          ) : (
            <Empty
              title={search || filter!=='all' ? t("development.tools.no_matching_configurations_in_this_window") : t("development.tools.no_execution_tools_registered")}
              description={
                search || filter!=='all'
                  ? t("development.tools.adjust_the_filter_or_load_more")
                  : t("development.tools.register_an_execution_worker_identity_then_choose")
              }
              action={search || filter!=='all'?<button onClick={() => {setSearch('');setFilter('all');}}>{t('tool-workspace.clear_filters')}</button>:<button onClick={openGuide}><CircleHelp size={14}/>{t('tool-workspace.setup_guide')}</button>}
            />
          )}
          {profiles.next_cursor && <div className="list-footer">
              <button
                disabled={profiles.loading}
                onClick={() => {
                  setLive(false);
                  void profiles.read(true);
                }}
              >
                {t("development.tools.load_more_configurations")}</button>
          </div>}
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
                  {stale && (
                    <p className="warning">
                      {t("development.tools.the_last_read_is_retained_current_connection")}</p>
                  )}
                  <ToolProfileFacts key={current.id} profile={current} confirmed={!stale}>
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
                  </ToolProfileFacts>
                </>
              )}
            </div>
          </section>
        )}
      </div>
      <div className="tool-boundary-note"><ShieldCheck size={16}/><p>{t("development.tools.configuration_probing_actual_execution_and_acceptance_are")}</p></div>
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
          dialog?.kind === "quick"
            ? t('tool-quick-connect.title')
            : dialog?.kind === "create"
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
            key={contextKey+":"+(dialog.adapter?.id||"")}
            project={project}
            adapter={dialog.adapter}
            workers={workers}
            sharedWorkers={sharedWorkers}
            action={action}
            onSaved={(value) => saved("/tool-profiles", value)}
            onCancel={() => setDialog(undefined)}
          />
        )}
        {dialog?.kind === 'quick' && <ToolQuickConnect
          key={contextKey+':'+(dialog.adapter?.id||'')}
          project={project}
          adapter={dialog.adapter}
          workers={workers}
          sharedWorkers={sharedWorkers}
          action={action}
          onRegistered={value=>{setReceipt({key:'registered',name:value.name,scope:contextKey});void profiles.read();onSelect(value.id);}}
          onEnable={profile=>{action.clearError();setDialog({kind:'decision',action:'enable',profile});}}
          onAdvanced={adapter=>{action.clearError();if(adapter?.id==='qoder-acp'&&adapter.direction==='suma_to_tool'&&adapter.adapter==='qoder'&&adapter.protocol==='acp-stdio-v1'){setDialog(undefined);setConnectionSelection(previous=>({context:contextKey,nonce:(previous?.nonce||0)+1,preset:'qoder'}));setGuideOpen(true);requestAnimationFrame(()=>document.getElementById(guideID)?.querySelector('summary')?.focus());}else setDialog({kind:'create',adapter});}}
          onWorker={()=>{action.clearError();setDialog({kind:'worker'});}}
        />}
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
  adapter,
  workers,
  sharedWorkers,
  action,
  onSaved,
  onCancel,
}: {
  project: string;
  adapter?:AdapterDescriptor;
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
    [submitError, setSubmitError] = useState<unknown>(''),
    [importing, setImporting] = useState(false),
    [name, setName] = useState(""),
    [nameEdited, setNameEdited] = useState(false),
    [source,setSource] = useState<'none'|'manual'|'discovery'>('none'),
    [manualRevision,setManualRevision] = useState(0),
    [worker, setWorker] = useState("");
  const matchesAdapter=(item:ToolObservation)=>!adapter||(item.profile.adapter===adapter.adapter&&item.profile.protocol===adapter.protocol);
  const matchingObservations=observations.filter(matchesAdapter);
  const selected = matchingObservations.find(
      (item) => item.profile.command_id === command,
    ),
    blocked = action.busy || !!action.pending;
  const formID=useId();
  const identityReady=!!currentToolIdentity(worker,workers,sharedWorkers);
  const next=!identityReady?'identity':importing?'reading':!manifest.trim()?'inventory':error?'invalid':!matchingObservations.length?'empty':!selected?.available?'choose':!name.trim()?'name':'ready';
  function inspectManifest(text:string) {
    setManifest(text);
    setObservations([]);
    setCommand('');
    setError('');
    if (!text.trim()) return;
    try {
      const rows = parseToolManifest(text);
      const available = rows.filter(item=>item.available&&matchesAdapter(item));
      setObservations(rows);
      if (available.length===1) {
        setCommand(available[0].profile.command_id);
        if (!nameEdited) setName(available[0].profile.command_id);
      } else if (!nameEdited) setName('');
    } catch (error) {
      setError(error);
    }
  }
  return (
    <form
      className="tool-form"
      onSubmit={async (event) => {
        event.preventDefault();
        setSubmitError("");
        if(importing)return;
        if (!currentToolIdentity(worker, workers, sharedWorkers)) {
          setSubmitError(t("tools.identity.authorization_unconfirmed"));
          return;
        }
        if (!selected?.available) {
          setSubmitError(t("development.tools.choose_an_available_probed_configuration_with_complete"));
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
      {!!submitError && <ErrorNotice error={renderDiagnostic(submitError)} />}
      {adapter&&<div className="tool-form-intro" role="status"><strong>{adapter.product}</strong><p>{t('adapter-connections.selected_execution',{adapter:adapter.adapter||'',protocol:adapter.protocol||''})}</p></div>}
      {adapter&&manifest.trim()&&!error&&!matchingObservations.length&&<p className="warning" role="status">{t('adapter-connections.no_matching_inventory')}</p>}
      <p className="tool-form-intro">
        {t('tool-discovery.form_intro')}</p>
      <fieldset className="form-fields" disabled={blocked}>
        <section className="tool-form-section" aria-labelledby={formID+'-identity'}>
        <h3 id={formID+'-identity'}><span aria-hidden="true">{formatNumber(1)}</span>{t('tool-workspace.form_identity')}</h3>
        <ToolIdentity
          local={workers}
          shared={sharedWorkers}
          value={worker}
          onChange={(value) => {
            setWorker(value);
            setImporting(false);
            if(source==='discovery') {inspectManifest('');setSource('none');if(!nameEdited)setName('');}
            if (action.conflict) action.clearError();
          }}
        />
        </section>
        <section className="tool-form-section" aria-labelledby={formID+'-inventory'}>
        <h3 id={formID+'-inventory'}><span aria-hidden="true">{formatNumber(2)}</span>{t('tool-discovery.inventory_source')}</h3>
        <ToolDiscoveryPanel project={project} worker={worker} authorized={identityReady} disabled={blocked||importing} autoAdopt={source==='none'} manualRevision={manualRevision} onAdopt={items=>{inspectManifest(JSON.stringify({items}));setSource('discovery');}}/>
        <details className="tool-manual-inventory"><summary>{t('tool-discovery.manual_inventory')}</summary>
        <p className="tool-form-intro"><Trans i18nKey="development.tools.inventory_instructions" components={{command:<code/>}}/></p>
        <ToolInventoryField key={worker} value={manifest} error={error} disabled={blocked} onChange={text=>{setSource('manual');setManualRevision(n=>n+1);inspectManifest(text);}} onReading={reading=>{setImporting(reading);if(reading){setSource('manual');setManualRevision(n=>n+1);}}}/>
        <button
          type="button"
          className="text-button"
          disabled={importing}
          onClick={() => inspectManifest(manifest)}
        >
          {t("development.tools.validate_inventory_fields")}</button>
        </details>
        </section>
        <section className="tool-form-section" aria-labelledby={formID+'-configuration'}>
        <h3 id={formID+'-configuration'}><span aria-hidden="true">{formatNumber(3)}</span>{t('tool-workspace.form_configuration')}</h3>
        {matchingObservations.length > 0 && (
          <>
            <p className={'tool-import-result '+(matchingObservations.some(item=>item.available)?'':'needs-attention')} role="status"><ShieldCheck size={16}/>{t('tool-workspace.import_result',{count:formatNumber(matchingObservations.length),available:formatNumber(matchingObservations.filter(item=>item.available).length)})}</p>
            <Field label={t("development.tools.choose_an_inventory_configuration")}>
              <select
                required
                value={command}
                onChange={(event) => {setCommand(event.target.value);if(!nameEdited)setName(event.target.value);}}
              >
                <option value="">{t("development.tools.choose_a_probed_fixed_configuration")}</option>
                {matchingObservations.map((item) => (
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
              {matchingObservations
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
        <Field label={t("development.tools.configuration_name")} hint={t('tool-workspace.name_help')}>
          <input required maxLength={200} value={name} onChange={event=>{setName(event.target.value);setNameEdited(true);}}/>
        </Field>
        {selected && (
          <div className="tool-context">
            <strong>
              {selected.profile.adapter} / {selected.profile.protocol}
            </strong>
            <p>
              {selected.profile.tool_version} ·{" "}
              {selected.profile.read_only ? t("development.tools.read_only") : t("development.tools.write_permitted_separate_worktree")}
            </p>
            <details className="tool-import-fingerprint"><summary>{t('development.facts.configuration_fingerprint')}</summary><code>{selected.profile.profile_sha256}</code></details>
          </div>
        )}
        </section>
      </fieldset>
      <div className="tool-profile-submit">
      <p id={formID+'-next'} data-tool-form-next className={'tool-form-next '+(next==='ready'?'ready':'')} aria-live="polite">{next==='ready'&&<ShieldCheck size={16} aria-hidden="true"/>}{t('tool-workspace.form_next.'+next)}</p>
      <div className="dialog-actions">
        <button type="button" disabled={blocked} onClick={onCancel}>
          {t("development.workflow.cancel")}</button>
        <button
          className="primary"
          disabled={
            blocked ||
            importing ||
            action.conflict ||
            !selected?.available ||
            !worker ||
            !currentToolIdentity(worker, workers, sharedWorkers) ||
            !name.trim()
          }
          aria-describedby={formID+'-next'}
        >
          {action.busy?t('tool-workspace.saving'):t("development.tools.save_as_paused_configuration")}</button>
      </div>
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
      {decision==='enable'&&current.profile.adapter==='qoder'&&current.profile.protocol==='acp-stdio-v1'&&<p className="muted">{t('acp-runtime.setup_policy')}</p>}
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
