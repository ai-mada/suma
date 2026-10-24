import { tr, useLocale, UiError } from "../i18n";
import { Trans } from "../i18n/Trans";
import { useState } from "react";
import { Field, ErrorNotice } from "../ui";
import { policyInput } from "../scheduling-model";
import { poolName, bounded, type Pool, type Grant } from "./model";
import type { ExecutionPoolPolicyInput } from "../../../../packages/contracts/src";
export function PoolPolicyEditor({
  pool,
  grants,
  disabled,
  onPreview,
}: {
  pool: Pool;
  grants: Grant[];
  disabled: boolean;
  onPreview: (
    version: number,
    policy: ExecutionPoolPolicyInput,
  ) => Promise<void>;
}) {
  useLocale();
  const [version, setVersion] = useState(pool.policy.version),
    [concurrency, setConcurrency] = useState(
      String(pool.policy.concurrency_limit),
    ),
    [budget, setBudget] = useState(
      pool.policy.budget_limit === null ? "" : String(pool.policy.budget_limit),
    ),
    [resources, setResources] = useState(() =>
      Object.entries(pool.policy.resource_limits).map(([name, value]) => ({
        name,
        value: value === null ? "" : String(value),
      })),
    ),
    [weights, setWeights] = useState<Record<string, string>>(() =>
      Object.fromEntries(
        Object.entries(pool.policy.tenant_weights).map(([id, weight]) => [
          id,
          String(weight),
        ]),
      ),
    ),
    [candidate, setCandidate] = useState(""),
    [error, setError] = useState<Error | string>("");
  const stale = version !== pool.policy.version,
    tenants = [
      ...new Set(
        grants.filter((g) => g.status === "approved").map((g) => g.tenant_id),
      ),
    ];
  return (
    <form
      className="pool-policy-editor"
      onSubmit={async (event) => {
        event.preventDefault();
        if (disabled || stale) return;
        try {
          const input = policyInput(concurrency, budget, resources),
            tenant_weights: Record<string, number> = {};
          for (const [id, value] of Object.entries(weights)) {
            if (!/^[a-f0-9]{32}$/.test(id))
              throw new UiError("pools.validation.organization_id");
            tenant_weights[id] = bounded(
              value,
              1,
              100,
              tr("pools.policy.organization_weight"),
            );
          }
          setError("");
          await onPreview(version, { ...input, tenant_weights });
        } catch (error) {
          setError(error instanceof Error ? error : String(error));
        }
      }}
    >
      <p>
        <Trans
          i18nKey="pools.policy.draft_current_notice"
          values={{ value1: version, value2: pool.policy.version }}
        />
      </p>
      {error && <ErrorNotice error={error} />}{" "}
      {stale && (
        <div className="pool-recovery">
          <p>{tr("pools.policy.changed_draft_preserved")}</p>
          <button
            type="button"
            disabled={disabled}
            onClick={() => setVersion(pool.policy.version)}
          >
            {tr("pools.policy.review_again")}
          </button>
        </div>
      )}
      <fieldset className="pool-fields" disabled={disabled}>
        <div className="pool-form-grid">
          <Field label={tr("pools.facts.execution_slots")}>
            <input
              name="pool_concurrency"
              type="number"
              min={0}
              step={1}
              value={concurrency}
              onChange={(e) => setConcurrency(e.target.value)}
              required
            />
          </Field>
          <Field
            label={tr("pools.facts.cumulative_budget")}
            hint={tr("pools.form.budget_hint")}
          >
            <input
              name="pool_budget"
              type="number"
              min={0}
              step={1}
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
            />
          </Field>
        </div>
        <details>
          <summary>{tr("pools.policy.other_resources")}</summary>
          {resources.map((row, index) => (
            <div className="pool-resource-row" key={index}>
              <Field label={tr("pools.policy.resource_name")}>
                <input
                  value={row.name}
                  onChange={(e) =>
                    setResources((rows) =>
                      rows.map((r, i) =>
                        i === index ? { ...r, name: e.target.value } : r,
                      ),
                    )
                  }
                />
              </Field>
              <Field
                label={tr("pools.policy.configured_limit")}
                hint={tr("pools.policy.limit_hint")}
              >
                <input
                  type="number"
                  min={0}
                  value={row.value}
                  onChange={(e) =>
                    setResources((rows) =>
                      rows.map((r, i) =>
                        i === index ? { ...r, value: e.target.value } : r,
                      ),
                    )
                  }
                />
              </Field>
              <button
                type="button"
                aria-label={
                  tr("pools.policy.remove_resource_prefix") +
                  (row.name || index + 1)
                }
                onClick={() =>
                  setResources((rows) => rows.filter((_, i) => i !== index))
                }
              >
                {tr("pools.policy.remove")}
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() =>
              setResources((rows) => [...rows, { name: "", value: "" }])
            }
          >
            {tr("pools.policy.add_resource")}
          </button>
        </details>
        <details open={!!Object.keys(weights).length}>
          <summary>{tr("pools.policy.organization_weight")}</summary>
          <p>{tr("pools.policy.weights_notice")}</p>
          {Object.entries(weights).map(([id, value]) => (
            <div className="pool-resource-row" key={id}>
              <Field label={tr("pools.policy.organization_weight_prefix") + id}>
                <input
                  name={"weight-" + id}
                  type="number"
                  min={1}
                  max={100}
                  value={value}
                  onChange={(e) =>
                    setWeights((before) => ({
                      ...before,
                      [id]: e.target.value,
                    }))
                  }
                />
              </Field>
              <button
                type="button"
                onClick={() =>
                  setWeights((before) =>
                    Object.fromEntries(
                      Object.entries(before).filter(([key]) => key !== id),
                    ),
                  )
                }
              >
                {tr("pools.policy.reset_weight")}
              </button>
            </div>
          ))}
          <Field label={tr("pools.policy.select_approved_organization")}>
            <select
              value={candidate}
              onChange={(e) => setCandidate(e.target.value)}
            >
              <option value="">
                {tr("pools.policy.select_organization_id")}
              </option>
              {tenants
                .filter((id) => !(id in weights))
                .map((id) => (
                  <option key={id} value={id}>
                    {id}
                  </option>
                ))}
            </select>
          </Field>
          <button
            type="button"
            disabled={!candidate || candidate in weights}
            onClick={() => {
              if (candidate)
                setWeights((before) => ({ ...before, [candidate]: "1" }));
              setCandidate("");
            }}
          >
            {tr("pools.policy.add_organization_weight")}
          </button>
        </details>
      </fieldset>
      <div className="pool-row-actions">
        <button className="primary" disabled={disabled || stale}>
          {tr("pools.policy.preview")}
        </button>
      </div>
    </form>
  );
}
