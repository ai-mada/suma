import {useApi} from '../api-context';
import { tr, useLocale, UiError, renderDiagnostic } from "../i18n";
import { Trans } from "../i18n/Trans";
import { useEffect, useMemo, useRef, useState } from "react";
import { query, time, stateLabel, type Session } from "../api";
import { Field, ErrorNotice, Loading } from "../ui";
import { toolSessionStorage, useToolAction, useToolRead } from "../tool-hooks";
import { capacityLabel } from "../scheduling-model";
import { schedulingResourceLabel } from "../SchedulingFacts";
import { PoolWriteRecovery } from "./write";
import { PoolRecovery, PoolStatus } from "./Shared";
import { PoolPolicyFacts } from "./Facts";
import { PoolPolicyEditor } from "./PolicyEditor";
import { poolPath, type Pool, type Grant } from "./model";
import type {
  ExecutionPoolPolicyPreview,
  ExecutionPoolPolicyInput,
  ExecutionRouteView,
  ExecutionRouteInput,
  ExecutionRoutePreview,
} from "../../../../packages/contracts/src";
const mayLeave = () =>
  window.dispatchEvent(new Event("suma:navigate", { cancelable: true }));
/** Precise GET is required before confirmation. A newly created receipt only
 * selects the URL; it never bypasses current authorization, expiry or version. */
export function PoolPolicyWorkspace({
  session,
  pool,
  grants,
  previewID,
  onPreview,
  onApplied,
  readOnly = false,
}: {
  session: Session;
  pool: Pool;
  grants: Grant[];
  previewID: string;
  onPreview: (id: string) => void;
  onApplied: (pool: Pool) => void;
  readOnly?: boolean;
}) { const api=useApi();
  useLocale();
  const root = useRef<HTMLElement>(null),
    focusPreview = useRef<string | undefined>(undefined),
    focusEditor = useRef(false),
    inflight = useRef(false);
  const [applied, setApplied] = useState<Pool>(),
    [receipt, setReceipt] = useState<{ id: string; version: number }>(),
    [editor, setEditor] = useState(0),
    [submitting, setSubmitting] = useState(false);
  const current =
    applied && applied.policy.version > pool.policy.version ? applied : pool;
  const recovery = useMemo(
    () =>
      new PoolWriteRecovery(JSON.stringify([
          session.organization.id,
          session.user.id,
          "policy",
          pool.id,
        ]), toolSessionStorage(), undefined, api),
    [session.organization.id, session.user.id, pool.id, api],
  );
  const action = useToolAction(recovery),
    read = useToolRead<ExecutionPoolPolicyPreview>(
      previewID
        ? poolPath(pool.id) +
            "/policy-previews/" +
            encodeURIComponent(previewID)
        : "",
    ),
    preview = read.data;
  const blocked =
      action.busy || !!action.pending || !!recovery.error || readOnly,
    committed = preview?.state === "committed" || receipt?.id === previewID,
    expired = !!preview && Date.parse(preview.expires_at) <= Date.now(),
    stale =
      !!preview &&
      (preview.policy_version !== current.policy.version ||
        preview.pool_version !== current.version),
    canCommit =
      !!preview &&
      !blocked &&
      !read.loading &&
      !read.error &&
      preview.state === "ready" &&
      preview.valid &&
      !expired &&
      !stale &&
      preview.allowed_actions.includes("commit");
  useEffect(() => {
    if (
      focusPreview.current === previewID &&
      preview?.id === previewID &&
      !read.loading &&
      !read.error
    ) {
      focusPreview.current = undefined;
      root.current
        ?.querySelector<HTMLElement>("[data-preview-review]")
        ?.focus({ preventScroll: true });
    }
  }, [previewID, preview?.id, read.loading, read.error]);
  useEffect(() => {
    if (!previewID && focusEditor.current) {
      focusEditor.current = false;
      root.current
        ?.querySelector<HTMLInputElement>('[name="pool_concurrency"]')
        ?.focus({ preventScroll: true });
    }
  }, [previewID, editor]);
  function resolved(path: string, value: unknown) {
    if (path.endsWith("/commit")) {
      const next = value as Pool;
      setApplied(next);
      setReceipt({ id: previewID, version: next.policy.version });
      onApplied(next);
      void read.read();
    } else {
      const created = value as ExecutionPoolPolicyPreview;
      focusPreview.current = created.id;
      onPreview(created.id);
    }
  }
  async function create(version: number, policy: ExecutionPoolPolicyInput) {
    const path = poolPath(pool.id) + "/policy-previews",
      value = await action.run(path, { expected_version: version, policy });
    if (value) resolved(path, value);
  }
  async function commit() {
    if (!canCommit || !preview || inflight.current) return;
    if (Date.parse(preview.expires_at) <= Date.now()) {
      action.setError(tr("pools.preview.expired"));
      return;
    }
    inflight.current = true;
    setSubmitting(true);
    try {
      const path =
          poolPath(pool.id) + "/policy-previews/" + preview.id + "/commit",
        value = await action.run(path, { expected_version: preview.version });
      if (value) resolved(path, value);
    } finally {
      inflight.current = false;
      setSubmitting(false);
    }
  }
  function toEditor() {
    if (!mayLeave()) return;
    focusEditor.current = true;
    setEditor((n) => n + 1);
    onPreview("");
  }
  return (
    <section ref={root} aria-label={tr("pools.preview.policy_workspace")}>
      <PoolRecovery action={action} onRestored={resolved} />
      {!previewID ? (
        <>
          <PoolPolicyFacts policy={current.policy} />
          <PoolPolicyEditor
            key={editor}
            pool={current}
            grants={grants}
            disabled={
              blocked || !current.allowed_actions.includes("preview_policy")
            }
            onPreview={create}
          />
        </>
      ) : (
        <>
          <button className="pool-back" disabled={blocked} onClick={toEditor}>
            {tr("pools.preview.back_policy_draft")}
          </button>
          <button disabled={read.loading} onClick={() => void read.read()}>
            {tr("pools.preview.reload")}
          </button>
          {read.error && (
            <ErrorNotice error={read.error} onRetry={() => void read.read()} />
          )}{" "}
          {read.loading && !preview && <Loading />}
          {preview && (
            <section
              tabIndex={-1}
              data-preview-review
              aria-label={tr("pools.preview.policy_review")}
            >
              <h3>{tr("pools.preview.review_policy")}</h3>
              <p>
                <Trans
                  i18nKey="pools.preview.fixed_versions"
                  values={{
                    value1: preview.policy_version,
                    value2: preview.pool_version,
                    value3: preview.version,
                  }}
                />
              </p>
              <p>
                <Trans
                  i18nKey="pools.preview.observation_counts"
                  values={{
                    value1: time(preview.impact.as_of),
                    value2: preview.impact.ready_nodes,
                    value3: preview.impact.active_attempts,
                  }}
                />
              </p>
              <ul>
                {preview.changes.map((change) => (
                  <li key={change.resource}>
                    {schedulingResourceLabel(change.resource)}：
                    {capacityLabel(change.before)} →{" "}
                    {capacityLabel(change.after)}
                  </li>
                ))}
                {preview.weight_changes.map((change) => (
                  <li key={change.tenant_id}>
                    <Trans
                      i18nKey="pools.preview.organization_weight_change"
                      values={{
                        value1: change.tenant_id,
                        value2: change.before,
                        value3: change.after,
                      }}
                      components={{ code: <code /> }}
                    />
                  </li>
                ))}
              </ul>
              <p>{tr("pools.preview.weight_unit_notice")}</p>
              <details>
                <summary>{tr("pools.preview.fixed_inputs")}</summary>
                <code>{preview.payload_sha256}</code>
                <pre className="pool-json">
                  {JSON.stringify(preview.policy, null, 2)}
                </pre>
              </details>
              <p>
                <Trans
                  i18nKey="pools.preview.expires"
                  values={{ value1: time(preview.expires_at) }}
                />
              </p>
              {!preview.valid && !committed && preview.reason && (
                <p className="warning">{renderDiagnostic(preview.reason)}</p>
              )}
              {expired && (
                <p className="warning">{tr("pools.preview.expired_confirm")}</p>
              )}
              {stale && !committed && (
                <p className="warning">
                  {tr("pools.preview.policy_basis_changed")}
                </p>
              )}
              {!preview.allowed_actions.includes("commit") &&
                !committed &&
                !expired && (
                  <p className="warning">
                    {tr("pools.preview.policy_permission_missing")}
                  </p>
                )}
              {committed && (
                <p role="status" aria-live="polite">
                  <Trans
                    i18nKey="pools.preview.applied_policy"
                    values={{
                      value1:
                        receipt?.id === previewID
                          ? receipt.version
                          : preview.applied_policy_version,
                    }}
                  />
                </p>
              )}
              <button
                className="primary"
                disabled={committed ? blocked : !submitting && !canCommit}
                aria-disabled={submitting || (!committed && !canCommit)}
                onClick={() => {
                  if (committed) toEditor();
                  else if (!submitting) void commit();
                }}
              >
                {committed
                  ? tr("pools.preview.adjust_policy")
                  : submitting
                    ? tr("pools.preview.confirming")
                    : tr("pools.preview.confirm_policy")}
              </button>
            </section>
          )}
        </>
      )}
    </section>
  );
}
export function ProjectRouteWorkspace({
  session,
  project,
  grants,
  previewID,
  onPreview,
  refresh,
}: {
  session: Session;
  project: string;
  grants: Grant[];
  previewID: string;
  onPreview: (id: string) => void;
  refresh: number;
}) { const api=useApi();
  useLocale();
  const root = useRef<HTMLElement>(null),
    focusPreview = useRef<string | undefined>(undefined),
    focusEditor = useRef(false),
    inflight = useRef(false),
    [applied, setApplied] = useState<ExecutionRouteView>(),
    [receipt, setReceipt] = useState<{ id: string; version: number }>(),
    [editor, setEditor] = useState(0),
    [submitting, setSubmitting] = useState(false);
  const recovery = useMemo(
    () =>
      new PoolWriteRecovery(JSON.stringify([
          session.organization.id,
          session.user.id,
          "route",
          project,
        ]), toolSessionStorage(), project, api),
    [session.organization.id, session.user.id, project, api],
  );
  const action = useToolAction(recovery),
    read = useToolRead<ExecutionRouteView>(
      "/project-execution/route" + query(project),
      refresh,
    ),
    previewRead = useToolRead<ExecutionRoutePreview>(
      previewID
        ? "/project-execution/route-previews/" +
            encodeURIComponent(previewID) +
            query(project)
        : "",
    ),
    preview = previewRead.data,
    current =
      applied && (!read.data || applied.version > read.data.version)
        ? applied
        : read.data;
  const blocked = action.busy || !!action.pending || !!recovery.error,
    committed = preview?.state === "committed" || receipt?.id === previewID,
    expired = !!preview && Date.parse(preview.expires_at) <= Date.now(),
    canCommit =
      !!preview &&
      !!current &&
      !blocked &&
      !read.loading &&
      !read.error &&
      !previewRead.loading &&
      !previewRead.error &&
      preview.state === "ready" &&
      preview.valid &&
      preview.route_version === current.version &&
      !expired &&
      preview.allowed_actions.includes("commit");
  useEffect(() => {
    if (
      focusPreview.current === previewID &&
      preview?.id === previewID &&
      !previewRead.loading &&
      !previewRead.error
    ) {
      focusPreview.current = undefined;
      root.current
        ?.querySelector<HTMLElement>("[data-preview-review]")
        ?.focus({ preventScroll: true });
    }
  }, [previewID, preview?.id, previewRead.loading, previewRead.error]);
  useEffect(() => {
    if (!previewID && focusEditor.current) {
      focusEditor.current = false;
      root.current
        ?.querySelector<HTMLSelectElement>('[name="execution_mode"]')
        ?.focus({ preventScroll: true });
    }
  }, [previewID, editor]);
  function resolved(path: string, value: unknown) {
    if (path.endsWith("/commit")) {
      const next = value as ExecutionRouteView;
      setApplied(next);
      setReceipt({ id: previewID, version: next.version });
      void read.read();
      void previewRead.read();
    } else {
      const created = value as ExecutionRoutePreview;
      focusPreview.current = created.id;
      onPreview(created.id);
    }
  }
  async function create(version: number, route: ExecutionRouteInput) {
    const path = "/project-execution/route-previews",
      value = await action.run(path, {
        project_id: project,
        expected_version: version,
        route,
      });
    if (value) resolved(path, value);
  }
  async function commit() {
    if (!canCommit || !preview || inflight.current) return;
    if (Date.parse(preview.expires_at) <= Date.now()) {
      action.setError(tr("pools.preview.expired"));
      return;
    }
    inflight.current = true;
    setSubmitting(true);
    try {
      const path =
          "/project-execution/route-previews/" + preview.id + "/commit",
        value = await action.run(path, {
          project_id: project,
          expected_version: preview.version,
        });
      if (value) resolved(path, value);
    } finally {
      inflight.current = false;
      setSubmitting(false);
    }
  }
  function toEditor() {
    if (!mayLeave()) return;
    focusEditor.current = true;
    setEditor((n) => n + 1);
    onPreview("");
  }
  return (
    <section
      ref={root}
      className="panel"
      aria-label={tr("pools.route.project_default")}
    >
      <header className="pool-toolbar">
        <div>
          <h2>{tr("pools.route.future_default")}</h2>
          <p>{tr("pools.route.immutable_notice")}</p>
        </div>
        <button disabled={read.loading} onClick={() => void read.read()}>
          {tr("pools.route.reload")}
        </button>
      </header>
      <PoolRecovery action={action} onRestored={resolved} />
      {read.error && (
        <ErrorNotice error={read.error} onRetry={() => void read.read()} />
      )}{" "}
      {read.loading && !current && <Loading />}
      {current && (
        <p>
          <Trans
            i18nKey="pools.route.current_default_facts"
            values={{
              value1:
                current.mode === "pool"
                  ? current.pool_name || current.pool_id
                  : tr("pools.route.local"),
              value2: current.version,
              value3: time(current.as_of),
            }}
          />
        </p>
      )}
      {current?.mode === "pool" && (
        <p>
          <Trans
            i18nKey="pools.route.current_grant_facts"
            values={{ value2: current.grant_version }}
            components={{
              value1: (
                <span>
                  {current.grant_state ? (
                    <PoolStatus value={current.grant_state} />
                  ) : (
                    tr("pools.route.status_missing")
                  )}
                </span>
              ),
            }}
          />
        </p>
      )}
      {!previewID ? (
        current && (
          <RouteEditor
            key={editor}
            route={current}
            grants={grants}
            disabled={
              blocked ||
              read.loading ||
              !!read.error ||
              !current.allowed_actions.includes("preview")
            }
            onPreview={create}
          />
        )
      ) : (
        <>
          <button disabled={blocked} onClick={toEditor}>
            {tr("pools.route.back_draft")}
          </button>
          <button
            disabled={previewRead.loading}
            onClick={() => void previewRead.read()}
          >
            {tr("pools.preview.reload")}
          </button>
          {previewRead.error && (
            <ErrorNotice
              error={previewRead.error}
              onRetry={() => void previewRead.read()}
            />
          )}{" "}
          {previewRead.loading && !preview && <Loading />}
          {preview && (
            <section
              tabIndex={-1}
              data-preview-review
              aria-label={tr("pools.route.review_default")}
            >
              <h3>{tr("pools.route.review_future")}</h3>
              <p>
                {preview.before.mode === "pool"
                  ? preview.before.pool_name || preview.before.pool_id
                  : tr("pools.route.local")}{" "}
                →{" "}
                {preview.route.mode === "pool"
                  ? preview.pool_name || preview.route.pool_id
                  : tr("pools.route.local")}
              </p>
              <p>
                <Trans
                  i18nKey="pools.route.existing_count"
                  values={{
                    value1: preview.impact.existing_runs,
                    value2: time(preview.impact.as_of),
                  }}
                />
              </p>
              {preview.route.mode === "pool" && (
                <p>
                  <Trans
                    i18nKey="pools.route.fixed_grant_facts"
                    values={{
                      value1: preview.route.grant_id,
                      value2: preview.route.grant_version,
                      value3: stateLabel(preview.pool_status),
                    }}
                    components={{ code: <code /> }}
                  />
                </p>
              )}
              <details>
                <summary>{tr("pools.preview.fixed_inputs")}</summary>
                <code>{preview.payload_sha256}</code>
                <pre className="pool-json">
                  {JSON.stringify(preview.route, null, 2)}
                </pre>
              </details>
              <p>
                <Trans
                  i18nKey="pools.route.preview_versions"
                  values={{
                    value1: preview.route_version,
                    value2: preview.version,
                    value3: time(preview.expires_at),
                  }}
                />
              </p>
              {!preview.valid && !committed && preview.reason && (
                <p className="warning">{renderDiagnostic(preview.reason)}</p>
              )}
              {expired && (
                <p className="warning">{tr("pools.preview.expired_confirm")}</p>
              )}
              {current &&
                current.version !== preview.route_version &&
                !committed && (
                  <p className="warning">{tr("pools.route.default_changed")}</p>
                )}
              {!preview.allowed_actions.includes("commit") &&
                !committed &&
                !expired && (
                  <p className="warning">
                    {tr("pools.route.preview_permission_missing")}
                  </p>
                )}
              {committed && (
                <p role="status" aria-live="polite">
                  <Trans
                    i18nKey="pools.route.applied"
                    values={{
                      value1:
                        receipt?.id === previewID
                          ? receipt.version
                          : preview.applied_route_version,
                    }}
                  />
                </p>
              )}
              <button
                className="primary"
                disabled={committed ? blocked : !submitting && !canCommit}
                aria-disabled={submitting || (!committed && !canCommit)}
                onClick={() => {
                  if (committed) toEditor();
                  else if (!submitting) void commit();
                }}
              >
                {committed
                  ? tr("pools.route.adjust")
                  : submitting
                    ? tr("pools.preview.confirming")
                    : tr("pools.route.confirm")}
              </button>
            </section>
          )}
        </>
      )}
    </section>
  );
}
function RouteEditor({
  route,
  grants,
  disabled,
  onPreview,
}: {
  route: ExecutionRouteView;
  grants: Grant[];
  disabled: boolean;
  onPreview: (version: number, input: ExecutionRouteInput) => Promise<void>;
}) {
  useLocale();
  const [version, setVersion] = useState(route.version),
    [mode, setMode] = useState(route.mode),
    [selected, setSelected] = useState(route.grant_id || ""),
    [error, setError] = useState<Error | string>("");
  const approved = grants.filter(
      (grant) =>
        grant.project_id === route.project_id && grant.status === "approved",
    ),
    grant = approved.find((item) => item.id === selected),
    currentGrant =
      !grant &&
      route.mode === "pool" &&
      route.grant_state === "approved" &&
      selected === route.grant_id,
    stale = version !== route.version;
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        if (disabled || stale) return;
        try {
          let input: ExecutionRouteInput = { mode: "local" };
          if (mode === "pool") {
            if (!grant && !currentGrant)
              throw new UiError("pools.route.select_full_grant");
            input = {
              mode: "pool",
              pool_id: grant?.pool_id || route.pool_id,
              grant_id: grant?.id || route.grant_id,
              grant_version: grant?.version || route.grant_version,
            };
          }
          setError("");
          await onPreview(version, input);
        } catch (error) {
          setError(error instanceof Error ? error : String(error));
        }
      }}
    >
      {error && <ErrorNotice error={error} />}
      <p>
        <Trans
          i18nKey="pools.route.draft_change_notice"
          values={{ value1: version }}
        />
      </p>
      {stale && (
        <div className="pool-recovery">
          <p>{tr("pools.route.changed_draft_preserved")}</p>
          <button
            type="button"
            disabled={disabled}
            onClick={() => setVersion(route.version)}
          >
            {tr("pools.route.review_draft_again")}
          </button>
        </div>
      )}
      <fieldset className="pool-fields" disabled={disabled}>
        <Field label={tr("pools.route.execution_method")}>
          <select
            name="execution_mode"
            value={mode}
            onChange={(e) => setMode(e.target.value)}
          >
            <option value="local">{tr("pools.route.local")}</option>
            <option value="pool">{tr("pools.route.approved_pool")}</option>
          </select>
        </Field>
        {mode === "pool" && (
          <Field label={tr("pools.route.fixed_project_grant")}>
            <select
              name="execution_grant"
              required
              value={selected}
              onChange={(e) => setSelected(e.target.value)}
            >
              <option value="">
                {tr("pools.route.select_approved_grant")}
              </option>
              {currentGrant && (
                <option value={route.grant_id}>
                  <Trans
                    i18nKey="pools.route.grant_option"
                    values={{
                      value1: route.pool_name || route.pool_id,
                      value2: route.grant_version,
                    }}
                  />
                </option>
              )}
              {approved.map((item) => (
                <option key={item.id} value={item.id}>
                  <Trans
                    i18nKey="pools.project.grant_option"
                    values={{
                      value1: item.pool_name || item.pool_id,
                      value2: item.version,
                    }}
                  />
                </option>
              ))}
            </select>
            <small>{tr("pools.route.loaded_grants_only")}</small>
          </Field>
        )}
      </fieldset>
      <button className="primary" disabled={disabled || stale}>
        {tr("pools.route.preview")}
      </button>
    </form>
  );
}
