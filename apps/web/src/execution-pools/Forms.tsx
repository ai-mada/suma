import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { useState, type ReactNode } from "react";
import { Field, ErrorNotice } from "../ui";
import { amountInput } from "../scheduling-model";
import { bounded, poolName, type Runner, type Grant } from "./model";
export function PoolCreateForm({
  blocked,
  onSubmit,
  notice,
}: {
  blocked: boolean;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  notice: ReactNode;
}) {
  useLocale();
  const [error, setError] = useState<Error | string>("");
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        if (blocked) return;
        try {
          const values = new FormData(event.currentTarget);
          setError("");
          await onSubmit({
            name: poolName(String(values.get("name"))),
            description: String(values.get("description") || ""),
            policy: {
              concurrency_limit: amountInput(String(values.get("concurrency"))),
              budget_limit: amountInput(String(values.get("budget")), true),
              resource_limits: {},
              tenant_weights: {},
            },
          });
        } catch (error) {
          setError(error instanceof Error ? error : String(error));
        }
      }}
    >
      {notice}
      {error && <ErrorNotice error={error} />}
      <fieldset className="pool-fields" disabled={blocked}>
        <Field label={tr("pools.form.pool_name")}>
          <input name="name" required maxLength={200} />
        </Field>
        <Field label={tr("pools.form.description")}>
          <textarea name="description" maxLength={4000} rows={2} />
        </Field>
        <div className="pool-form-grid">
          <Field
            label={tr("pools.facts.execution_slots")}
            hint={tr("pools.form.capacity_hint")}
          >
            <input
              name="concurrency"
              type="number"
              min={0}
              step={1}
              defaultValue={1}
              required
            />
          </Field>
          <Field
            label={tr("pools.facts.cumulative_budget")}
            hint={tr("pools.form.budget_hint")}
          >
            <input name="budget" type="number" min={0} step={1} />
          </Field>
        </div>
      </fieldset>
      <div className="dialog-actions">
        <button className="primary" disabled={blocked}>
          {tr("pools.form.confirm_create")}
        </button>
      </div>
    </form>
  );
}
export function RunnerForm({
  runner,
  blocked,
  onSubmit,
  notice,
}: {
  runner?: Runner;
  blocked: boolean;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  notice: ReactNode;
}) {
  useLocale();
  const [error, setError] = useState<Error | string>("");
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        if (blocked) return;
        try {
          const values = new FormData(event.currentTarget);
          setError("");
          const ttl_seconds = bounded(
            String(values.get("ttl_seconds")),
            3600,
            2592000,
            tr("pools.form.credential_ttl"),
          );
          await onSubmit(
            runner
              ? { expected_version: runner.version, ttl_seconds }
              : {
                  name: poolName(String(values.get("name"))),
                  max_slots: bounded(
                    String(values.get("max_slots")),
                    1,
                    128,
                    tr("pools.runner.registered_slots"),
                  ),
                  ttl_seconds,
                },
          );
        } catch (error) {
          setError(error instanceof Error ? error : String(error));
        }
      }}
    >
      {notice}
      {error && <ErrorNotice error={error} />}
      <p>
        {runner
          ? tr("pools.form.rotate_notice")
          : tr("pools.form.machine_identity_notice")}
      </p>
      <fieldset className="pool-fields" disabled={blocked}>
        {!runner && (
          <>
            <Field label={tr("pools.form.runner_name")}>
              <input name="name" required maxLength={200} />
            </Field>
            <Field
              label={tr("pools.runner.registered_slots")}
              hint={tr("pools.form.slots_hint")}
            >
              <input
                name="max_slots"
                type="number"
                min={1}
                max={128}
                defaultValue={1}
                required
              />
            </Field>
          </>
        )}
        <Field label={tr("pools.form.credential_ttl")}>
          <select name="ttl_seconds" defaultValue={86400}>
            <option value={3600}>{tr("pools.form.ttl_hour")}</option>
            <option value={86400}>{tr("pools.form.ttl_day")}</option>
            <option value={604800}>{tr("pools.form.ttl_week")}</option>
            <option value={2592000}>{tr("pools.form.ttl_month")}</option>
          </select>
        </Field>
      </fieldset>
      <div className="dialog-actions">
        <button className="primary" disabled={blocked}>
          {runner
            ? tr("pools.form.confirm_rotate")
            : tr("pools.form.confirm_register")}
        </button>
      </div>
    </form>
  );
}
export function GrantRequestForm({
  project,
  projectName,
  tenant,
  blocked,
  onSubmit,
  notice,
}: {
  project: string;
  projectName: string;
  tenant: string;
  blocked: boolean;
  onSubmit: (body: Record<string, unknown>) => Promise<void>;
  notice: ReactNode;
}) {
  useLocale();
  const [error, setError] = useState<Error | string>("");
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        if (blocked) return;
        try {
          const values = new FormData(event.currentTarget),
            pool_id = String(values.get("pool_id") || "").trim();
          if (!/^[a-f0-9]{32}$/.test(pool_id))
            throw new UiError("pools.grant.pool_id_required");
          setError("");
          await onSubmit({
            project_id: project,
            pool_id,
            project_weight: bounded(
              String(values.get("project_weight")),
              1,
              100,
              tr("pools.grant.project_weight"),
            ),
          });
        } catch (error) {
          setError(error instanceof Error ? error : String(error));
        }
      }}
    >
      {notice}
      {error && <ErrorNotice error={error} />}
      <div className="pool-shared-scope">
        <strong>{tr("pools.grant.shared_to_owner")}</strong>
        <p>
          <Trans
            i18nKey="pools.grant.project_snapshot"
            values={{ value1: projectName }}
          />
        </p>
        <p>
          {tr("pools.grant.organization")}
          <code>{tenant}</code>
          <br />
          {tr("pools.grant.project")}
          <code>{project}</code>
        </p>
        <p>{tr("pools.grant.metadata_only_notice")}</p>
      </div>
      <fieldset className="pool-fields" disabled={blocked}>
        <Field label={tr("pools.grant.pool_id")}>
          <input name="pool_id" required maxLength={32} autoComplete="off" />
        </Field>
        <Field
          label={tr("pools.grant.project_weight")}
          hint={tr("pools.grant.weight_hint")}
        >
          <input
            name="project_weight"
            type="number"
            min={1}
            max={100}
            defaultValue={1}
            required
          />
        </Field>
      </fieldset>
      <div className="dialog-actions">
        <button className="primary" disabled={blocked}>
          {tr("pools.grant.confirm_request")}
        </button>
      </div>
    </form>
  );
}
export function GrantDecision({
  grant,
  action,
  blocked,
  onConfirm,
  notice,
}: {
  grant: Grant;
  action: string;
  blocked: boolean;
  onConfirm: () => void;
  notice: ReactNode;
}) {
  useLocale();
  return (
    <>
      {notice}
      <div className="pool-shared-scope">
        <strong>{grant.shared_project_name}</strong>
        <p>
          <Trans
            i18nKey="pools.grant.organization_project"
            values={{ value1: grant.tenant_id, value2: grant.project_id }}
          />
        </p>
        <p>
          <Trans
            i18nKey="pools.grant.weight_fixed_version"
            values={{ value1: grant.project_weight, value2: grant.version }}
          />
        </p>
        <code>{grant.request_sha256}</code>
      </div>
      <p>
        {action === "approve"
          ? tr("pools.grant.approve_notice")
          : action === "reject"
            ? tr("pools.grant.reject_notice")
            : tr("pools.grant.revoke_notice")}
      </p>
      <div className="dialog-actions">
        <button
          className={action === "approve" ? "primary" : "danger-quiet"}
          disabled={blocked}
          onClick={onConfirm}
        >
          {
            (
              {
                approve: tr("pools.grant.confirm_approve"),
                reject: tr("pools.grant.confirm_reject"),
                revoke: tr("pools.grant.confirm_revoke"),
                withdraw: tr("pools.grant.confirm_withdraw"),
              } as Record<string, string>
            )[action]
          }
        </button>
      </div>
    </>
  );
}
