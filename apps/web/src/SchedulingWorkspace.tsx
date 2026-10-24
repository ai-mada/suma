import { tr as t, useLocale, formatNumber, renderDiagnostic } from "./i18n";
import { useEffect, useMemo, useRef, useState } from "react";
import { RefreshCw, Plus, Trash2 } from "lucide-react";
import { api, query, type Session } from "./api";
import { ErrorNotice, Field, Loading } from "./ui";
import { useLeaveGuard } from "./write-action";
import { useToolAction, useToolRead, toolSessionStorage } from "./tool-hooks";
import { SchedulingWriteRecovery } from "./scheduling-write";
import {
  policyInput,
  type SchedulingPolicy,
  type PolicyPreview,
} from "./scheduling-model";
import {
  PolicyFacts,
  PolicyPreviewFacts,
  schedulingResourceLabel,
} from "./SchedulingFacts";
import "./scheduling.css";
type Action = ReturnType<typeof useToolAction>;
export function SchedulingWorkspace({
  project,
  session,
  selected,
  onSelect,
  refresh,
}: {
  project: string;
  session: Session;
  selected: string;
  onSelect: (id: string) => void;
  refresh: number;
}) { useLocale();
  const workspaceRoot = useRef<HTMLElement>(null),
    editorFocus = useRef(false),
    inflightCommit = useRef<string | undefined>(undefined),
    previewRoot = useRef<HTMLElement>(null),
    focusPreview = useRef<string | undefined>(undefined);
  const scope = JSON.stringify([
      session.organization.id,
      session.user.id,
      project,
    ]),
    recovery = useMemo(
      () => new SchedulingWriteRecovery(scope, toolSessionStorage(), project),
      [scope, project],
    ),
    action = useToolAction(recovery),
    read = useToolRead<SchedulingPolicy>(
      "/scheduling/policy" + query(project),
      refresh,
    ),
    previewRead = useToolRead<PolicyPreview>(
      selected
        ? "/scheduling/policy-previews/" +
            encodeURIComponent(selected) +
            query(project)
        : "",
    ),
    [seed, setSeed] = useState<PolicyPreview>(),
    [committed, setCommitted] = useState<SchedulingPolicy>(),
    [appliedPreview, setAppliedPreview] = useState<{
      id: string;
      policyVersion: number;
    }>(),
    [editorEpoch, setEditorEpoch] = useState(0);
  const policy =
      committed && (!read.data || committed.version >= read.data.version)
        ? committed
        : read.data,
    preview = previewRead.data || (seed?.id === selected ? seed : undefined),
    writeBlocked = action.busy || !!action.pending || !!recovery.error,
    previewCommitted =
      preview?.state === "committed" ||
      (!!selected && appliedPreview?.id === selected),
    commitBlocked =
      writeBlocked ||
      previewRead.loading ||
      !!previewRead.error ||
      !!read.error ||
      !preview?.valid ||
      preview.state !== "ready" ||
      preview.policy_version !== policy?.version ||
      Date.parse(preview.expires_at) <= Date.now() ||
      !policy?.allowed_actions.includes("commit"),
    submittingCurrent = action.busy && inflightCommit.current === preview?.id;
  useEffect(() => {
    if (!editorFocus.current || selected || writeBlocked) return;
    editorFocus.current = false;
    workspaceRoot.current
      ?.querySelector<HTMLInputElement>(
        'input[name="concurrency"]:not(:disabled)',
      )
      ?.focus({ preventScroll: true });
  }, [selected, writeBlocked]);
  function returnToDraft() {
    editorFocus.current = true;
    onSelect("");
  }
  useEffect(() => {
    if (
      focusPreview.current !== selected ||
      !selected ||
      previewRead.loading ||
      previewRead.error ||
      previewRead.data?.id !== selected
    )
      return;
    focusPreview.current = undefined;
    previewRoot.current?.scrollIntoView?.({ block: "start", behavior: "auto" });
    previewRoot.current?.focus({ preventScroll: true });
  }, [selected, previewRead.loading, previewRead.error, previewRead.data?.id]);
  function opened(result: PolicyPreview) {
    focusPreview.current = result.id;
    setSeed(result);
    onSelect(result.id);
  }
  function resolved(path: string, value: unknown) {
    if (path.endsWith("/commit")) {
      const id = path.match(
        /^\/scheduling\/policy-previews\/([^/]+)\/commit$/,
      )?.[1];
      if (id)
        setAppliedPreview({
          id: decodeURIComponent(id),
          policyVersion: (value as SchedulingPolicy).version,
        });
      setCommitted(value as SchedulingPolicy);
      setEditorEpoch((x) => x + 1);
      void read.read();
      void previewRead.read();
    } else {
      const result = value as PolicyPreview;
      opened(result);
    }
  }
  async function commit() {
    if (!preview || previewCommitted || commitBlocked || inflightCommit.current)
      return;
    inflightCommit.current = preview.id;
    const path =
      "/scheduling/policy-previews/" +
      encodeURIComponent(preview.id) +
      "/commit";
    try {
      const result = await action.run<SchedulingPolicy>(path, {
        project_id: project,
        expected_version: preview.version,
      });
      if (result) resolved(path, result);
    } finally {
      inflightCommit.current = undefined;
    }
  }
  return (
    <section
      ref={workspaceRoot}
      className="panel scheduling-workspace"
      aria-label={t("development.policy.current_project_execution_scheduling")}
    >
      <div className="panel-heading">
        <div>
          <h2>{t("development.policy.execution_scheduling")}</h2>
          <p>{t("development.policy.set_concurrency_limits_and_cumulative_budgets_changes")}</p>
        </div>
        <button disabled={read.loading} onClick={() => void read.read()}>
          <RefreshCw size={15} />
          {t("development.policy.read_current_policy")}</button>
      </div>
      {read.error && (
        <>
          <ErrorNotice error={read.error} onRetry={() => void read.read()} />
          <p className="warning">
            {t("development.policy.current_policy_read_unconfirmed_new_changes_cannot")}</p>
        </>
      )}
      {action.error && <ErrorNotice error={action.error} />}{" "}
      {action.pending && (
        <section
          className="scheduling-facts scheduling-recovery"
          aria-label={t("development.policy.original_policy_operation_needs_reconciliation")}
        >
          <h3>{t("development.policy.original_policy_operation_result_unconfirmed")}</h3>
          <p>{t("development.policy.the_original_path_version_complete_non_secret")}</p>
          <code>{action.pending.idempotency_key}</code>
          <div className="scheduling-actions">
            <button
              disabled={action.busy}
              className="primary"
              onClick={async () => {
                const original = action.pending;
                if (!original) return;
                const value = await action.restore();
                if (value) resolved(original.path, value);
              }}
            >
              {t("development.policy.recover_original_policy_operation_receipt")}</button>
            <button disabled={action.busy} onClick={action.defer}>
              {t("development.recovery.keep_id_reconcile_later")}</button>
          </div>
        </section>
      )}
      {!policy ? (
        read.loading ? (
          <Loading />
        ) : null
      ) : (
        <>
          <PolicyFacts policy={policy} />
          <p className="scheduling-policy-note">
            {t("development.policy.reducing_limits_affects_new_dispatch_only_existing")}</p>
          <div
            className={
              "scheduling-policy-grid" + (selected ? " has-preview" : "")
            }
          >
            <PolicyEditor
              key={project + ":" + editorEpoch}
              policy={policy}
              project={project}
              action={action}
              disabled={
                writeBlocked ||
                !!selected ||
                !!read.error ||
                !policy.allowed_actions.includes("preview")
              }
              onCreated={opened}
            />
            {selected && (
              <section
                ref={previewRoot}
                tabIndex={-1}
                aria-label={t("development.policy.fixed_policy_preview")}
              >
                {previewRead.error && (
                  <ErrorNotice
                    error={previewRead.error}
                    onRetry={() => void previewRead.read()}
                  />
                )}{" "}
                {!preview && previewRead.loading && <Loading />}
                {preview?.project_id === project && (
                  <>
                    <PolicyPreviewFacts preview={preview} />
                    {preview.state === "ready" &&
                      !previewCommitted &&
                      preview.policy_version !== policy.version && (
                        <p className="warning">
                          {t("development.policy.the_current_policy_version_changed_return_to")}</p>
                      )}
                    <div className="scheduling-actions">
                      <button
                        className="primary"
                        disabled={
                          !submittingCurrent &&
                          (previewCommitted ? writeBlocked : commitBlocked)
                        }
                        aria-disabled={
                          previewCommitted ? writeBlocked : commitBlocked
                        }
                        aria-busy={submittingCurrent}
                        onClick={() => {
                          if (previewCommitted) {
                            if (!writeBlocked) returnToDraft();
                          } else if (!commitBlocked) void commit();
                        }}
                      >
                        {previewCommitted
                          ? t("development.policy.continue_adjusting_policy")
                          : submittingCurrent
                            ? t("development.policy.confirming")
                            : t("development.policy.confirm_applying_this_fixed_policy")}
                      </button>
                      <button
                        disabled={writeBlocked}
                        onClick={() => void previewRead.read()}
                      >
                        {t("development.policy.recheck_preview")}</button>
                      {!previewCommitted && (
                        <button disabled={writeBlocked} onClick={returnToDraft}>
                          {t("development.policy.return_to_draft_and_preview_again")}</button>
                      )}
                    </div>
                    {previewCommitted && (
                      <p role="status" aria-live="polite">
                        {t("development.policy.committed_version",{version:appliedPreview?.id===selected?appliedPreview.policyVersion:(preview.applied_policy_version??t("development.scheduling.not_provided_yet"))})}</p>
                    )}
                  </>
                )}
              </section>
            )}
          </div>
        </>
      )}
    </section>
  );
}
function PolicyEditor({
  policy,
  project,
  action,
  disabled,
  onCreated,
}: {
  policy: SchedulingPolicy;
  project: string;
  action: Action;
  disabled: boolean;
  onCreated: (value: PolicyPreview) => void;
}) { useLocale();
  const [concurrency, setConcurrency] = useState(
      String(policy.concurrency_limit),
    ),
    [budget, setBudget] = useState(
      policy.budget_limit === null ? "" : String(policy.budget_limit),
    ),
    [rows, setRows] = useState(
      Object.entries(policy.resource_limits).map(([name, value]) => ({
        name,
        value: value === null ? "" : String(value),
      })),
    ),
    [version, setVersion] = useState(policy.version),
    [dirty, setDirty] = useState(false),
    [error, setError] = useState<unknown>("");
  useLeaveGuard(dirty && !disabled, setError);
  async function submit(form: HTMLFormElement) {
    try {
      const values = new FormData(form),
        input = policyInput(
          String(values.get("concurrency") ?? ""),
          String(values.get("budget") ?? ""),
          rows.map((row, index) => ({
            name: String(values.get("resource-name-" + index) ?? row.name),
            value: String(values.get("resource-value-" + index) ?? row.value),
          })),
        );
      setError("");
      const result = await action.run<PolicyPreview>(
        "/scheduling/policy-previews",
        { project_id: project, expected_version: version, policy: input },
      );
      if (result) {
        setDirty(false);
        onCreated(result);
      }
    } catch (error) {
      setError(error);
    }
  }
  return (
    <form
      className="scheduling-policy-form"
      onSubmit={(event) => {
        event.preventDefault();
        void submit(event.currentTarget);
      }}
      onChange={() => setDirty(true)}
    >
      <h3>{t("development.policy.prepare_policy_change")}</h3>
      <p className="muted">
        {t("development.policy.draft_version",{version})}</p>
      {!!error && <ErrorNotice error={renderDiagnostic(error)} />}
      <fieldset disabled={disabled}>
        <div className="scheduling-primary-fields">
          <Field
            label={t("development.policy.concurrent_execution_slot_limit")}
            hint={t("development.policy.maximum_simultaneous_attempts_0_pauses_new_dispatch")}
          >
            <input
              name="concurrency"
              inputMode="numeric"
              required
              value={concurrency}
              onChange={(e) => setConcurrency(e.target.value)}
            />
          </Field>
          <Field
            label={t("development.scheduling.cumulative_execution_budget")}
            hint={t("development.policy.each_attempt_uses_1_admission_unit_by")}
          >
            <input
              name="budget"
              inputMode="numeric"
              value={budget}
              onChange={(e) => setBudget(e.target.value)}
            />
          </Field>
        </div>
        <details className="scheduling-extra-resources">
          <summary>
            {rows.length?t("development.policy.resources_count",{count:formatNumber(rows.length)}):t("development.policy.other_resource_constraints")}
          </summary>
          <p className="muted">
            {t("development.policy.resource_names_must_match_the_workflow_blank")}</p>
          {rows.map((row, index) => (
            <div key={index} className="scheduling-resource-row">
              <Field
                label={t("development.policy.resource_name_number",{number:formatNumber(index+1)})}
                hint={
                  schedulingResourceLabel(row.name) !== row.name
                    ? schedulingResourceLabel(row.name)
                    : undefined
                }
              >
                <input
                  name={"resource-name-" + index}
                  value={row.name}
                  onChange={(e) =>
                    setRows((items) =>
                      items.map((item, i) =>
                        i === index ? { ...item, name: e.target.value } : item,
                      ),
                    )
                  }
                />
              </Field>
              <Field label={t("development.policy.resource_quota_number",{number:formatNumber(index+1)})}>
                <input
                  name={"resource-value-" + index}
                  inputMode="numeric"
                  value={row.value}
                  onChange={(e) =>
                    setRows((items) =>
                      items.map((item, i) =>
                        i === index ? { ...item, value: e.target.value } : item,
                      ),
                    )
                  }
                />
              </Field>
              <button
                type="button"
                aria-label={t("development.policy.remove_resource_name",{name:row.name||formatNumber(index+1)})}
                onClick={() => {
                  setRows((items) => items.filter((_, i) => i !== index));
                  setDirty(true);
                }}
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => {
              setRows((items) => [...items, { name: "", value: "" }]);
              setDirty(true);
            }}
          >
            <Plus size={14} />
            {t("development.policy.add_resource_constraint")}</button>
        </details>
        <div className="scheduling-actions">
          <button className="primary" type="submit">
            {t("development.policy.generate_fixed_preview")}</button>
          <button
            type="button"
            onClick={() => {
              setConcurrency(String(policy.concurrency_limit));
              setBudget(
                policy.budget_limit === null ? "" : String(policy.budget_limit),
              );
              setRows(
                Object.entries(policy.resource_limits).map(([name, value]) => ({
                  name,
                  value: value === null ? "" : String(value),
                })),
              );
              setVersion(policy.version);
              setDirty(false);
              setError("");
            }}
          >
            {t("development.policy.discard_draft_use_the_read_policy")}</button>
        </div>
      </fieldset>
      {!policy.allowed_actions.includes("preview") && (
        <p className="warning">
          {t("development.policy.this_account_cannot_preview_or_approve_policies")}</p>
      )}
      {action.conflict && (
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={async () => {
            try {
              const latest = await api<SchedulingPolicy>(
                "/scheduling/policy" + query(project),
              );
              setVersion(latest.version);
              action.clearError();
              setError(
                t("development.policy.the_draft_s_base_version_was_updated"),
              );
            } catch (error) {
              setError(error);
            }
          }}
        >
          {t("development.policy.check_current_version_keep_draft_and_preview")}</button>
      )}
    </form>
  );
}
