import {useApi} from '../api-context';
import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { useMemo, useState } from "react";
import { Modal } from "../ui";
import { label, query } from "../api";
import { toolSessionStorage, useToolAction } from "../tool-hooks";
import { PoolWriteRecovery } from "./write";
import { usePoolPage } from "./hooks";
import { type PoolProps, type Grant } from "./model";
import { GrantFacts } from "./Facts";
import { GrantRequestForm, GrantDecision } from "./Forms";
import { PageFooter, PageState, PoolRecovery } from "./Shared";
import { ProjectRouteWorkspace } from "./Previews";
import "./pools.css";
const mayLeave = () =>
  window.dispatchEvent(new Event("suma:navigate", { cancelable: true }));
export function ProjectExecutionWorkspace({
  session,
  project = "",
  refresh,
  onSelect,
  selected,
  panel,
  onPanel,
}: PoolProps) { const api=useApi();
  useLocale();
  const [revision, setRevision] = useState(0),
    [dialog, setDialog] = useState<"request" | Grant>();
  const recovery = useMemo(
    () =>
      new PoolWriteRecovery(JSON.stringify([
          session.organization.id,
          session.user.id,
          "project",
          project,
        ]), toolSessionStorage(), project, api),
    [session.organization.id, session.user.id, project, api],
  );
  const action = useToolAction(recovery, () => setDialog(undefined)),
    page = usePoolPage<Grant>(
      project ? "/project-execution/grants" + query(project) : "",
      refresh + revision,
    ),
    blocked = action.busy || !!action.pending || !!recovery.error;
  function resolved(_path: string, value: unknown) {
    setDialog(undefined);
    setRevision((n) => n + 1);
    const grant = value as Grant;
    if (grant.id) onSelect(grant.id);
  }
  async function send(path: string, body: Record<string, unknown>) {
    const result = await action.run(path, body);
    if (result) resolved(path, result);
  }
  const notice = <PoolRecovery action={action} onRestored={resolved} />,
    projectRecord = session.projects.find((item) => item.id === project);
  return (
    <div
      className="execution-pools"
      aria-label={tr("pools.project.resources_title")}
    >
      {project && (
        <ProjectRouteWorkspace
          session={session}
          project={project}
          grants={page.items}
          previewID={panel.startsWith("route:") ? panel.slice(6) : ""}
          onPreview={(id) => onPanel(id ? "route:" + id : "")}
          refresh={refresh + revision}
        />
      )}
      <section className="panel">
        <header className="pool-toolbar">
          <div>
            <h2>{tr("pools.project.resources_current")}</h2>
            <p>
              <Trans
                i18nKey="pools.project.records_notice"
                values={{
                  value1: projectRecord
                    ? label(projectRecord)
                    : project || tr("pools.project.select_first"),
                }}
              />
            </p>
          </div>
          <div className="inline wrap">
            <button
              disabled={page.loading || !project}
              onClick={() => void page.read()}
            >
              {tr("pools.workspace.reload_grants")}
            </button>
            {page.allowed.includes("request") && (
              <button
                className="primary"
                disabled={blocked}
                onClick={() => {
                  if (mayLeave()) {
                    action.clearError();
                    setDialog("request");
                  }
                }}
              >
                {tr("pools.project.request_pool")}
              </button>
            )}
          </div>
        </header>
        {!dialog && notice}
        <PageState
          loading={page.loading}
          error={page.error}
          empty={!page.items.length}
          retry={() => void page.read()}
        >
          {page.items.length ? (
            <ul className="pool-record-list">
              {page.items.map((item) => (
                <li
                  key={item.id}
                  aria-current={item.id === selected ? "true" : undefined}
                >
                  <h3>{item.pool_name || item.pool_id}</h3>
                  <GrantFacts grant={item} />
                  <p>{tr("pools.project.approval_route_notice")}</p>
                  {item.allowed_actions.includes("withdraw") && (
                    <button
                      disabled={blocked || page.loading || !!page.error}
                      onClick={() => {
                        if (mayLeave()) {
                          action.clearError();
                          setDialog(item);
                        }
                      }}
                    >
                      {tr("pools.project.withdraw_grant")}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            !page.loading &&
            !page.error && (
              <p className="pool-empty">
                {tr("pools.project.no_pool_requests")}
              </p>
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
      <Modal
        open={!!dialog}
        onClose={() => {
          if (!blocked) setDialog(undefined);
        }}
        title={
          dialog === "request"
            ? tr("pools.project.request_shared_pool")
            : tr("pools.project.withdraw_grant")
        }
        description={tr("pools.project.bilateral_confirmation")}
      >
        {dialog === "request" ? (
          <GrantRequestForm
            project={project}
            projectName={projectRecord ? label(projectRecord) : project}
            tenant={session.organization.id}
            blocked={blocked}
            notice={notice}
            onSubmit={(body) => send("/project-execution/grants", body)}
          />
        ) : (
          dialog && (
            <GrantDecision
              grant={dialog}
              action="withdraw"
              blocked={blocked}
              notice={notice}
              onConfirm={() =>
                void send(
                  "/project-execution/grants/" + dialog.id + "/commands",
                  {
                    project_id: project,
                    action: "withdraw",
                    expected_version: dialog.version,
                  },
                )
              }
            />
          )
        )}
      </Modal>
    </div>
  );
}
