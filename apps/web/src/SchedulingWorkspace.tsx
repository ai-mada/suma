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
}) {
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
      aria-label="当前项目执行调度"
    >
      <div className="panel-heading">
        <div>
          <h2>执行调度</h2>
          <p>设置并行执行上限与累计额度，审阅后生效。</p>
        </div>
        <button disabled={read.loading} onClick={() => void read.read()}>
          <RefreshCw size={15} />
          读取当前策略
        </button>
      </div>
      {read.error && (
        <>
          <ErrorNotice error={read.error} onRetry={() => void read.read()} />
          <p className="warning">
            当前策略读取未确认；不允许依据旧数据发起新调整。
          </p>
        </>
      )}
      {action.error && <ErrorNotice error={action.error} />}{" "}
      {action.pending && (
        <section
          className="scheduling-facts scheduling-recovery"
          aria-label="策略原操作待核对"
        >
          <h3>原策略操作结果尚未确认</h3>
          <p>保留原路径、版本、完整非秘密策略和操作标识；只能恢复同一请求。</p>
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
              恢复原策略操作回执
            </button>
            <button disabled={action.busy} onClick={action.defer}>
              保留标识，稍后核对
            </button>
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
            降低上限只影响新派发，已有执行继续；累计已用额度不退还。
          </p>
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
                aria-label="固定策略预览"
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
                          当前策略版本已变化，请返回草稿重新生成预览。旧预览不能无感重基。
                        </p>
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
                          ? "继续调整策略"
                          : submittingCurrent
                            ? "正在确认…"
                            : "确认应用此固定策略"}
                      </button>
                      <button
                        disabled={writeBlocked}
                        onClick={() => void previewRead.read()}
                      >
                        重新核对预览
                      </button>
                      {!previewCommitted && (
                        <button disabled={writeBlocked} onClick={returnToDraft}>
                          返回草稿重新预览
                        </button>
                      )}
                    </div>
                    {previewCommitted && (
                      <p role="status" aria-live="polite">
                        该预览已经提交，生效策略版本{" "}
                        {appliedPreview?.id === selected
                          ? appliedPreview.policyVersion
                          : (preview.applied_policy_version ?? "尚未提供")}
                        ；不能再次提交。
                      </p>
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
}) {
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
    [error, setError] = useState("");
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
      setError((error as Error).message);
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
      <h3>准备策略调整</h3>
      <p className="muted">
        草稿所据版本 v{version}
        。读取当前策略不会覆盖草稿。预览后审阅差异，再显式确认。
      </p>
      {error && <ErrorNotice error={error} />}
      <fieldset disabled={disabled}>
        <div className="scheduling-primary-fields">
          <Field
            label="并发执行槽位上限"
            hint="最多同时执行的尝试数。0 暂停新派发。"
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
            label="累计执行额度"
            hint="每次尝试默认使用 1 准入单位。空白不限制，0 暂停新执行。"
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
            其他资源约束{rows.length ? "（" + rows.length + "）" : ""}
          </summary>
          <p className="muted">
            资源名称须与工作流一致；空白不限制，0 表示无可用额度。
          </p>
          {rows.map((row, index) => (
            <div key={index} className="scheduling-resource-row">
              <Field
                label={"资源名称 " + (index + 1)}
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
              <Field label={"资源额度 " + (index + 1)}>
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
                aria-label={"移除资源 " + (row.name || index + 1)}
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
            添加资源约束
          </button>
        </details>
        <div className="scheduling-actions">
          <button className="primary" type="submit">
            生成固定预览
          </button>
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
            放弃草稿，使用已读策略
          </button>
        </div>
      </fieldset>
      {!policy.allowed_actions.includes("preview") && (
        <p className="warning">
          当前账号不具备预览和批准策略的权限，配置事实仍可读取。
        </p>
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
                "已更新草稿所据版本，字段仍保留。请重新生成并审阅预览。",
              );
            } catch (error) {
              setError((error as Error).message);
            }
          }}
        >
          核对当前版本，保留草稿重做预览
        </button>
      )}
    </form>
  );
}
