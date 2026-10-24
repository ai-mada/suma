import { tr } from "./i18n";
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
  toolError,
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
}: Props) {
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
      new ToolWriteRecovery(
        session.organization.id +
          ":" +
          session.user.id +
          ":" +
          project +
          ":profiles",
        toolSessionStorage(),
      ),
    [session.organization.id, session.user.id, project],
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
          <h2>执行工具</h2>
          <p>把本机许可配置、实时观察和正式派发条件放在同一处核对。</p>
        </div>
        <a
          href={
            "?" +
            new URLSearchParams({ project, view: "development", sub: "tasks" })
          }
        >
          返回任务派发
        </a>
      </div>
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
          返回原运行与选中节点
        </a>
      )}
      <div className="info-strip">
        <ShieldCheck size={18} />
        <p>
          配置、探测、实际执行和验收分别记录。清单不包含程序路径、参数、环境值或第三方登录文件；模型登录健康需另行验证。
        </p>
      </div>
      <ToolRecoveryNotice action={action} onRestored={saved} />
      <div className={"tool-columns " + (selected ? "with-detail" : "")}>
        <section className="panel tool-list">
          <div className="list-toolbar">
            <span>
              固定工具配置 <b>{profiles.items ? profiles.total : "—"}</b>
            </span>
            <div className="inline">
              <button
                className="icon-button"
                aria-label="刷新工具配置"
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
                登记配置
              </button>
            </div>
          </div>
          <div className="filter-toolbar">
            <input
              aria-label="搜索已加载工具配置"
              placeholder="按名称、节点或命令筛选…"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <small>
              已加载 {profiles.items?.length || 0} / {profiles.total}
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
                title="尚未读取到正式配置"
                description="请先恢复连接，无法根据旧声明派发。"
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
                      {profile.available ? "上次读取可派发" : "当前条件未确认"}
                    </small>
                  </span>
                  <ChevronRight size={15} />
                </button>
              ))}
            </div>
          ) : (
            <Empty
              title={search ? "当前窗口没有匹配配置" : "尚未登记执行工具"}
              description={
                search
                  ? "可调整筛选或加载更多。"
                  : "先登记执行节点身份，再从本机 --describe-tools 非秘密清单选择固定配置。登记后保持暂停。"
              }
            />
          )}
          <div className="list-footer">
            <small>
              {live ? "每 10 秒更新当前窗口" : "自动更新已暂停，保留加载范围"}
              ；配置变化需重新登记。
            </small>
            {profiles.next_cursor && (
              <button
                disabled={profiles.loading}
                onClick={() => {
                  setLive(false);
                  void profiles.read(true);
                }}
              >
                加载更多配置
              </button>
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
                返回配置列表
              </button>
              <button
                className="icon-button"
                aria-label="关闭工具详情"
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
              {detail.loading && <p role="status">正在读取当前派发条件…</p>}
              {current && (
                <>
                  <ToolProfileFacts profile={current} />
                  {stale && (
                    <p className="warning">
                      保留上次读取结果；当前连接或权限未确认，管理操作暂不可用。
                    </p>
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
                            核对并启用
                          </button>
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
                            暂停配置
                          </button>
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
                          撤销配置
                        </button>
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
            <h3>执行节点身份</h3>
            <p className="muted">
              复用平台已绑定的 Runner
              凭证。凭证有效不等于节点在线；工具上报仅在数据库时间 60 秒内有效。
            </p>
          </div>
          <button
            disabled={pending || action.busy}
            onClick={() => {
              action.clearError();
              setDialog({ kind: "worker" });
            }}
          >
            <Plus size={14} />
            登记执行节点
          </button>
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
                  {worker.status === "active"
                    ? "凭证已登记，在线状态需看工具观察"
                    : "凭证已撤销"}{" "}
                  · 到期 {new Date(worker.expires_at).toLocaleString("zh-CN")}
                </small>
                <small>
                  {worker.deployment ? "另具发布许可" : "未授予部署许可"}
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
                  撤销节点凭证
                </button>
              )}
            </article>
          ))
        ) : (
          <Empty
            title="尚无绑定执行身份"
            description="登记后将获得一次性项目范围 Runner 凭证，请保存到本机进程环境。"
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
            ? "登记固定工具配置"
            : dialog?.kind === "worker"
              ? "登记执行节点身份"
              : dialog?.kind === "revoke-worker"
                ? "撤销执行节点凭证"
                : dialog?.kind === "decision"
                  ? {
                      enable: "核对并启用配置",
                      pause: "暂停工具配置",
                      revoke: "撤销固定配置",
                    }[dialog.action]
                  : "执行工具"
        }
        description="使用当前项目的真实身份与固定配置；结果未知时恢复同一操作。"
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
              撤销 {dialog.worker.worker_id}{" "}
              的凭证会阻止其后续派发与写回。已经发生的副作用与未知结果仍需核对，不会自动停止远端进程。
            </p>
            <div className="dialog-actions">
              <button
                type="button"
                disabled={action.busy || pending}
                onClick={() => setDialog(undefined)}
              >
                取消
              </button>
              <button
                className="primary"
                disabled={action.busy || pending || action.conflict}
              >
                确认撤销节点凭证
              </button>
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
        title="本次执行节点凭证"
        description="原文只保留在当前弹窗内存，不进入浏览器恢复存储或读取接口。"
      >
        {credential && (
          <div className="tool-token">
            <strong>{credential.worker_id}</strong>
            {credential.token ? (
              <>
                <p className="warning">
                  请将本次凭证保存到受控 Runner
                  的进程环境。关闭或刷新后无法再次读取；不要发送给模型或填写为原生
                  Agent 凭证。
                </p>
                <Field label="一次性 Runner 凭证">
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
                  在当前弹窗显示本次原文
                </label>
              </>
            ) : (
              <p className="warning">
                已恢复原身份登记回执，但一次性凭证没有随历史回执返回。不能声称原文可找回；请核对已有
                Runner，必要时撤销该身份后用新标识登记。
              </p>
            )}
            <button
              onClick={() => {
                setCredential(undefined);
                setRevealed(false);
              }}
            >
              关闭并清除本次原文
            </button>
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
}) {
  const [manifest, setManifest] = useState(""),
    [observations, setObservations] = useState<ToolObservation[]>([]),
    [command, setCommand] = useState(""),
    [error, setError] = useState(""),
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
          setError(tr("tools.identity.authorization_unconfirmed"));
          return;
        }
        if (!selected?.available) {
          setError("请选择清单中有完整指纹且探测可用的配置。");
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
      {error && <ErrorNotice error={error} />}
      <p>
        在目标 Runner 本机沿用相同许可命令文件与执行开关，运行{" "}
        <code>suma-runner --describe-tools</code>
        ，仅粘贴非秘密清单。此步骤不会传入执行地址、命令参数或第三方登录资料；保存后仍为暂停。
      </p>
      <fieldset className="form-fields" disabled={blocked}>
        <Field label="配置名称">
          <input
            required
            maxLength={200}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Field>
        <ToolIdentity
          legacy={workers}
          shared={sharedWorkers}
          value={worker}
          onChange={(value) => {
            setWorker(value);
            if (action.conflict) action.clearError();
          }}
        />
        <Field label="本机非秘密工具清单">
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
              setError(toolError(error));
            }
          }}
        >
          核对清单字段
        </button>
        {observations.length > 0 && (
          <>
            <Field label="选择清单内配置">
              <select
                required
                value={command}
                onChange={(event) => setCommand(event.target.value)}
              >
                <option value="">选择已探测的固定配置</option>
                {observations.map((item) => (
                  <option
                    key={item.profile.command_id}
                    value={item.profile.command_id}
                    disabled={!item.available}
                  >
                    {item.profile.command_id} ·{" "}
                    {item.profile.tool_version || "版本未确认"}
                    {item.available ? "" : " · 不可用"}
                  </option>
                ))}
              </select>
            </Field>
            <div className="tool-observations">
              {observations
                .filter((item) => !item.available)
                .map((item) => (
                  <article key={item.profile.command_id}>
                    <strong>{item.profile.command_id} · 未确认可用</strong>
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
              {selected.profile.read_only ? "只读" : "许可写入，独立工作树"}
            </p>
            <code>{selected.profile.profile_sha256}</code>
          </div>
        )}
      </fieldset>
      <div className="dialog-actions">
        <button type="button" disabled={blocked} onClick={onCancel}>
          取消
        </button>
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
          保存为暂停配置
        </button>
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
}) {
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
        复用平台执行节点凭证，登记纯工具执行身份。不会授予制品证明签发或部署权限；本次凭证由服务端生成，只显示一次。
      </p>
      <fieldset
        className="form-fields"
        disabled={action.busy || !!action.pending}
      >
        <Field label="唯一执行节点标识">
          <input
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{0,99}"
            maxLength={100}
            value={worker}
            onChange={(event) => {
              setWorker(event.target.value);
              if (action.conflict) action.clearError();
            }}
            placeholder="例如 team-code-runner"
          />
        </Field>
      </fieldset>
      {action.conflict && (
        <p className="warning">
          该节点标识已有历史登记，不能覆盖；请核对原身份并使用新的唯一标识。
        </p>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={onCancel}
        >
          取消
        </button>
        <button
          className="primary"
          disabled={action.busy || !!action.pending || action.conflict}
        >
          登记并获取本次凭证
        </button>
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
}) {
  const [current, setCurrent] = useState(value),
    [confirmed, setConfirmed] = useState(false),
    [error, setError] = useState("");
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
      {error && <ErrorNotice error={error} />}
      <p className={decision === "enable" ? "muted" : "warning"}>
        {decision === "enable"
          ? "启用后仍须当前执行身份、实时观察与固定指纹匹配。启动时还会核对启用者当前权限。"
          : decision === "pause"
            ? "暂停只阻止后续派发，已经开始的操作继续独立核对。"
            : "撤销不可恢复；原配置、运行和证据继续保留。已发生的操作不会自动撤销。"}
      </p>
      <label className="check">
        <input
          type="checkbox"
          checked={confirmed}
          disabled={action.busy || !!action.pending}
          onChange={(event) => setConfirmed(event.target.checked)}
        />
        我已核对本配置 v{current.version}，确认
        {decision === "enable"
          ? "启用"
          : decision === "pause"
            ? "暂停"
            : "撤销"}
        。
      </label>
      {action.conflict && (
        <div className="conflict-banner">
          <p>状态已经变化，原决定保留。读取最新版本后需重新确认。</p>
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
                setError(toolError(error));
              }
            }}
          >
            读取当前配置，保留决定
          </button>
        </div>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          disabled={action.busy || !!action.pending}
          onClick={onCancel}
        >
          取消
        </button>
        <button
          className="primary"
          disabled={
            action.busy || !!action.pending || action.conflict || !confirmed
          }
        >
          {decision === "enable"
            ? "确认启用本配置"
            : decision === "pause"
              ? "确认暂停本配置"
              : "确认撤销本配置"}
        </button>
      </div>
    </form>
  );
}
