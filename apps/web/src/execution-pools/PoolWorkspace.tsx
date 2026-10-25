import {useApi} from '../api-context';
import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { useMemo, useState } from "react";
import { Plus, RefreshCw, ChevronRight } from "lucide-react";
import { useToolAction, useToolRead, toolSessionStorage } from "../tool-hooks";
import { Modal } from "../ui";
import { PoolWriteRecovery } from "./write";
import { usePoolPage } from "./hooks";
import {
  poolPath,
  projectHref,
  type PoolProps,
  type Pool,
  type Grant,
  type Runner,
  type Credential,
} from "./model";
import { PageState, PageFooter, PoolRecovery, PoolStatus } from "./Shared";
import { PoolPolicyFacts, GrantFacts, RunnerFacts } from "./Facts";
import { PoolCreateForm, RunnerForm, GrantDecision } from "./Forms";
import { CredentialDialog } from "./CredentialDialog";
import { PoolPolicyWorkspace } from "./Previews";
import "./pools.css";
type Dialog =
  | { kind: "create" | "pause" | "resume" | "register" }
  | { kind: "grant"; grant: Grant; action: string }
  | { kind: "rotate" | "revoke-runner"; runner: Runner };
const mayLeave = () =>
  window.dispatchEvent(new Event("suma:navigate", { cancelable: true }));
/** Organization scope is deliberate: no current project is needed or inferred.
 * A pool owner may govern grants but never gains a consuming project's data. */
export function PoolWorkspace({
  session,
  selected,
  panel,
  refresh,
  onSelect,
  onPanel,
}: PoolProps) { const api=useApi();
  useLocale();
  const [revision, setRevision] = useState(0),
    [dialog, setDialog] = useState<Dialog>(),
    [credential, setCredential] = useState<Credential>();
  const recovery = useMemo(
    () =>
      new PoolWriteRecovery(JSON.stringify([
          session.organization.id,
          session.user.id,
          "organization",
          selected,
        ]), toolSessionStorage(), undefined, api),
    [session.organization.id, session.user.id, selected, api],
  );
  const action = useToolAction(recovery, () => setDialog(undefined)),
    page = usePoolPage<Pool>("/execution-pools", refresh + revision),
    detail = useToolRead<Pool>(
      selected ? poolPath(selected) : "",
      refresh + revision,
    ),
    grants = usePoolPage<Grant>(
      selected ? poolPath(selected) + "/grants" : "",
      refresh + revision,
    ),
    runners = usePoolPage<Runner>(
      selected ? poolPath(selected) + "/runners" : "",
      refresh + revision,
    );
  const pool = detail.data,
    stale = detail.loading || !!detail.error,
    blocked = action.busy || !!action.pending || !!recovery.error,
    tab = panel.startsWith("policy")
      ? "policy"
      : panel === "runners"
        ? "runners"
        : panel === "overview"
          ? "overview"
          : "grants";
  function resolved(path: string, value: unknown) {
    setDialog(undefined);
    setRevision((n) => n + 1);
    if (path === "/execution-pools") {
      const created = value as Pool;
      onSelect(created.id);
      onPanel("overview");
    } else if (path.endsWith("/runners") || path.endsWith("/credentials")) {
      const issued = value as Credential;
      if (issued.runner) setCredential(issued);
    }
  }
  async function send(path: string, body: Record<string, unknown>) {
    const result = await action.run(path, body);
    if (result) resolved(path, result);
  }
  const notice = <PoolRecovery action={action} onRestored={resolved} />;
  const open = (value: Dialog) => {
    if (!mayLeave()) return;
    action.clearError();
    setDialog(value);
  };
  return (
    <div className="execution-pools" aria-label={tr("pools.workspace.title")}>
      <p className="pool-scope">
        <Trans
          i18nKey="pools.workspace.organization_context"
          values={{ value1: session.organization.name }}
        />
      </p>
      {!dialog && notice}
      {!selected ? (
        <section className="panel">
          <header className="pool-toolbar">
            <div>
              <h2>{tr("pools.workspace.title")}</h2>
              <p>{tr("pools.workspace.subtitle")}</p>
            </div>
            <div className="inline wrap">
              <button disabled={page.loading} onClick={() => void page.read()}>
                <RefreshCw size={14} />
                {tr("pools.workspace.reload")}
              </button>
              {page.allowed.includes("create") && (
                <button
                  className="primary"
                  disabled={blocked}
                  onClick={() => open({ kind: "create" })}
                >
                  {tr("pools.workspace.create")}
                </button>
              )}
            </div>
          </header>
          <PageState
            loading={page.loading}
            error={page.error}
            empty={!page.items.length}
            retry={() => void page.read()}
          >
            {page.items.length ? (
              <ul className="pool-list">
                {page.items.map((item) => (
                  <li key={item.id}>
                    <a
                      className="pool-open"
                      href={
                        "?" +
                        new URLSearchParams({
                          view: "settings",
                          sub: "execution-pools",
                          record: item.id,
                          panel: "overview",
                        })
                      }
                      onClick={(event) => {
                        if (
                          event.button ||
                          event.metaKey ||
                          event.ctrlKey ||
                          event.shiftKey ||
                          event.altKey
                        )
                          return;
                        event.preventDefault();
                        if (mayLeave()) {
                          onSelect(item.id);
                          onPanel("overview");
                        }
                      }}
                    >
                      <span>
                        <strong>{item.name}</strong>
                        <small>
                          <Trans
                            i18nKey="pools.workspace.list_capacity"
                            values={{
                              value1: item.policy.concurrency_limit,
                              value2: item.policy.budget_spent,
                            }}
                          />
                        </small>
                      </span>
                      <PoolStatus value={item.status} />
                      <ChevronRight size={15} />
                    </a>
                  </li>
                ))}
              </ul>
            ) : (
              !page.loading &&
              !page.error && (
                <p className="pool-empty">{tr("pools.workspace.empty")}</p>
              )
            )}
          </PageState>
          <PageFooter
            count={page.items.length}
            total={page.total}
            next={page.next}
            loading={page.loading}
            more={() => void page.read(page.next)}
          />
        </section>
      ) : (
        <section className="panel">
          <button
            className="pool-back"
            onClick={() => {
              if (mayLeave()) {
                onSelect("");
                onPanel("");
              }
            }}
          >
            {tr("pools.workspace.back")}
          </button>
          <PageState
            loading={detail.loading}
            error={detail.error}
            empty={!pool}
            retry={() => void detail.read()}
          >
            {pool && (
              <>
                <header className="pool-toolbar">
                  <div>
                    <h2>{pool.name}</h2>
                    <p>
                      {pool.description ||
                        tr("pools.workspace.description_missing")}
                    </p>
                  </div>
                  <div className="inline wrap">
                    <PoolStatus value={pool.status} />
                    <button
                      disabled={detail.loading}
                      onClick={() => void detail.read()}
                    >
                      {tr("pools.workspace.reload_pool")}
                    </button>
                    {pool.allowed_actions.includes("pause") && (
                      <button
                        disabled={blocked || stale}
                        onClick={() => open({ kind: "pause" })}
                      >
                        {tr("pools.workspace.pause_pool")}
                      </button>
                    )}
                    {pool.allowed_actions.includes("resume") && (
                      <button
                        disabled={blocked || stale}
                        onClick={() => open({ kind: "resume" })}
                      >
                        {tr("pools.workspace.resume_pool")}
                      </button>
                    )}
                  </div>
                </header>
                <p className="pool-scope">
                  <Trans
                    i18nKey="pools.workspace.metadata_identity"
                    values={{ value1: pool.id, value2: pool.version }}
                    components={{ code: <code /> }}
                  />
                </p>
                <nav
                  className="pool-tabs"
                  aria-label={tr("pools.workspace.pages")}
                >
                  {[
                    ["overview", tr("pools.workspace.overview")],
                    ["grants", tr("pools.workspace.project_grants")],
                    ["runners", tr("pools.workspace.runners")],
                    ["policy", tr("pools.workspace.policy")],
                  ].map(([id, name]) => (
                    <button
                      key={id}
                      aria-current={tab === id ? "page" : undefined}
                      onClick={() => {
                        if (mayLeave()) onPanel(id);
                      }}
                    >
                      {name}
                    </button>
                  ))}
                </nav>
                {tab === "overview" ? (
                  <PoolPolicyFacts policy={pool.policy} />
                ) : tab === "policy" ? (
                  <PoolPolicyWorkspace
                    session={session}
                    pool={pool}
                    grants={grants.items}
                    readOnly={stale}
                    previewID={
                      panel.startsWith("policy:") ? panel.slice(7) : ""
                    }
                    onPreview={(id) => onPanel(id ? "policy:" + id : "policy")}
                    onApplied={() => {
                      setRevision((n) => n + 1);
                      void detail.read();
                    }}
                  />
                ) : tab === "grants" ? (
                  <>
                    <header className="pool-toolbar">
                      <h3>{tr("pools.workspace.grant_inbox")}</h3>
                      <button
                        disabled={grants.loading}
                        onClick={() => void grants.read()}
                      >
                        {tr("pools.workspace.reload_grants")}
                      </button>
                    </header>
                    <p>{tr("pools.workspace.grant_notice")}</p>
                    <PageState
                      loading={grants.loading}
                      error={grants.error}
                      empty={!grants.items.length}
                      retry={() => void grants.read()}
                    >
                      {grants.items.length ? (
                        <ul className="pool-record-list">
                          {grants.items.map((item) => (
                            <li key={item.id}>
                              <GrantFacts grant={item} />
                              <div className="pool-row-actions">
                                {item.can_open_project && (
                                  <a
                                    className="scheduling-open-link"
                                    href={projectHref(item.project_id)}
                                    onClick={(event) => {
                                      if (!mayLeave()) event.preventDefault();
                                    }}
                                  >
                                    {tr("pools.workspace.view_project")}
                                  </a>
                                )}
                                {[
                                  ["approve", tr("pools.workspace.approve")],
                                  ["reject", tr("pools.workspace.reject")],
                                  [
                                    "revoke",
                                    tr("pools.workspace.revoke_grant"),
                                  ],
                                ]
                                  .filter(([id]) =>
                                    item.allowed_actions.includes(id),
                                  )
                                  .map(([id, name]) => (
                                    <button
                                      key={id}
                                      disabled={
                                        blocked ||
                                        stale ||
                                        grants.loading ||
                                        !!grants.error
                                      }
                                      onClick={() =>
                                        open({
                                          kind: "grant",
                                          grant: item,
                                          action: id,
                                        })
                                      }
                                    >
                                      {name}
                                    </button>
                                  ))}
                              </div>
                            </li>
                          ))}
                        </ul>
                      ) : (
                        !grants.loading &&
                        !grants.error && (
                          <p className="pool-empty">
                            {tr("pools.workspace.no_requests")}
                          </p>
                        )
                      )}
                    </PageState>
                    <PageFooter
                      count={grants.items.length}
                      total={grants.total}
                      next={grants.next}
                      loading={grants.loading}
                      more={() => void grants.read(grants.next)}
                    />
                  </>
                ) : (
                  <>
                    <header className="pool-toolbar">
                      <h3>{tr("pools.workspace.independent_nodes")}</h3>
                      <div className="inline wrap">
                        <button
                          disabled={runners.loading}
                          onClick={() => void runners.read()}
                        >
                          {tr("pools.workspace.reload_nodes")}
                        </button>
                        {runners.allowed.includes("register") && (
                          <button
                            className="primary"
                            disabled={blocked || stale}
                            onClick={() => open({ kind: "register" })}
                          >
                            <Plus size={14} />
                            {tr("pools.workspace.register_node")}
                          </button>
                        )}
                      </div>
                    </header>
                    <PageState
                      loading={runners.loading}
                      error={runners.error}
                      empty={!runners.items.length}
                      retry={() => void runners.read()}
                    >
                      {runners.items.length ? (
                        <ul className="pool-record-list">
                          {runners.items.map((item) => (
                            <li key={item.id}>
                              <RunnerFacts runner={item} />
                              <div className="pool-row-actions">
                                {item.allowed_actions.includes(
                                  "rotate_credential",
                                ) && (
                                  <button
                                    disabled={
                                      blocked ||
                                      stale ||
                                      runners.loading ||
                                      !!runners.error
                                    }
                                    onClick={() =>
                                      open({ kind: "rotate", runner: item })
                                    }
                                  >
                                    {tr("pools.workspace.rotate")}
                                  </button>
                                )}
                                {item.allowed_actions.includes("revoke") && (
                                  <button
                                    className="danger-quiet"
                                    disabled={
                                      blocked ||
                                      stale ||
                                      runners.loading ||
                                      !!runners.error
                                    }
                                    onClick={() =>
                                      open({
                                        kind: "revoke-runner",
                                        runner: item,
                                      })
                                    }
                                  >
                                    {tr("pools.workspace.revoke_node")}
                                  </button>
                                )}
                              </div>
                            </li>
                          ))}
                        </ul>
                      ) : (
                        !runners.loading &&
                        !runners.error && (
                          <p className="pool-empty">
                            {tr("pools.workspace.no_nodes")}
                          </p>
                        )
                      )}
                    </PageState>
                    <PageFooter
                      count={runners.items.length}
                      total={runners.total}
                      next={runners.next}
                      loading={runners.loading}
                      more={() => void runners.read(runners.next)}
                    />
                  </>
                )}
              </>
            )}
          </PageState>
        </section>
      )}
      <Modal
        open={!!dialog}
        onClose={() => {
          if (!blocked) setDialog(undefined);
        }}
        title={
          dialog?.kind === "create"
            ? tr("pools.workspace.create")
            : dialog?.kind === "register"
              ? tr("pools.workspace.register_node")
              : dialog?.kind === "rotate"
                ? tr("pools.workspace.rotate_execution_credential")
                : dialog?.kind === "revoke-runner"
                  ? tr("pools.workspace.revoke_node")
                  : dialog?.kind === "grant"
                    ? tr("pools.workspace.review_fixed_request")
                    : dialog?.kind === "resume"
                      ? tr("pools.workspace.resume_pool")
                      : tr("pools.workspace.pause_pool")
        }
        description={tr("pools.workspace.operation_scope")}
      >
        {dialog?.kind === "create" ? (
          <PoolCreateForm
            blocked={blocked}
            notice={notice}
            onSubmit={(body) => send("/execution-pools", body)}
          />
        ) : dialog?.kind === "register" || dialog?.kind === "rotate" ? (
          <RunnerForm
            runner={dialog.kind === "rotate" ? dialog.runner : undefined}
            blocked={blocked}
            notice={notice}
            onSubmit={(body) =>
              send(
                poolPath(selected) +
                  "/runners" +
                  (dialog.kind === "rotate"
                    ? "/" + dialog.runner.id + "/credentials"
                    : ""),
                body,
              )
            }
          />
        ) : dialog?.kind === "grant" ? (
          <GrantDecision
            grant={dialog.grant}
            action={dialog.action}
            blocked={blocked}
            notice={notice}
            onConfirm={() =>
              void send(
                poolPath(selected) + "/grants/" + dialog.grant.id + "/commands",
                {
                  action: dialog.action,
                  expected_version: dialog.grant.version,
                },
              )
            }
          />
        ) : (
          dialog && (
            <>
              {notice}
              <p>
                {dialog.kind === "revoke-runner"
                  ? tr("pools.workspace.revoke_notice")
                  : dialog.kind === "pause"
                    ? tr("pools.workspace.pause_notice")
                    : tr("pools.workspace.resume_notice")}
              </p>
              <button
                className="primary"
                disabled={blocked || stale}
                onClick={() =>
                  void send(
                    dialog.kind === "revoke-runner"
                      ? poolPath(selected) +
                          "/runners/" +
                          dialog.runner.id +
                          "/commands"
                      : poolPath(selected) + "/commands",
                    {
                      action:
                        dialog.kind === "revoke-runner"
                          ? "revoke"
                          : dialog.kind,
                      expected_version:
                        dialog.kind === "revoke-runner"
                          ? dialog.runner.version
                          : pool!.version,
                    },
                  )
                }
              >
                {tr(
                  dialog.kind === "revoke-runner"
                    ? "pools.workspace.confirm_revoke_credential"
                    : dialog.kind === "pause"
                      ? "pools.workspace.confirm_pause"
                      : "pools.workspace.confirm_resume",
                )}
              </button>
            </>
          )
        )}
      </Modal>
      <CredentialDialog
        key={
          credential
            ? credential.runner.id + ":" + credential.runner.credential_version
            : "closed"
        }
        value={credential}
        onClose={() => setCredential(undefined)}
      />
    </div>
  );
}
