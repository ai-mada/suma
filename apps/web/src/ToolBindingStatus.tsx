import { useEffect } from "react";
import { query, stateLabel } from "./api";
import { ErrorNotice } from "./ui";
import {
  toolReturnPanel,
  type ToolProfile,
  type ToolRunBinding,
} from "./tool-model";
import { toolReason } from "./ToolFacts";
import { useToolRead } from "./tool-hooks";
type Props = {
  binding: ToolRunBinding;
  project: string;
  state: string;
  profile?: ToolProfile;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
};
export function ToolBindingFacts({
  binding,
  project,
  state,
  profile,
  loading,
  error,
  onRetry,
}: Props) {
  const matches =
    profile?.project_id === project &&
    profile.id === binding.profile_id &&
    profile.profile.profile_sha256 === binding.profile_sha256 &&
    profile.profile.command_id === binding.command;
  const eligible = matches && profile?.state === "enabled" && profile.available;
  const href =
    "?" +
    new URLSearchParams({
      project,
      view: "settings",
      sub: "tools",
      record: binding.profile_id,
      panel: toolReturnPanel(binding.run_id, binding.node_id),
    });
  return (
    <section
      className="tool-execution-facts tool-binding-status"
      aria-label="固定工具与当前新派发资格"
    >
      <h4>固定工具依据</h4>
      <dl>
        <dt>本次平台执行</dt>
        <dd>
          {state === "ready" ? "待派发（依赖已就绪）" : stateLabel(state)}
        </dd>
        <dt>固定命令</dt>
        <dd>
          <code>{binding.command || "未提供唯一许可命令"}</code>
        </dd>
        <dt>固定配置</dt>
        <dd>
          <code>{binding.profile_id}</code>
        </dd>
        <dt>本次固定指纹</dt>
        <dd>
          <code>{binding.profile_sha256 || "尚未读取到固定指纹"}</code>
        </dd>
      </dl>
      <h5>当前新派发资格</h5>
      {error ? (
        <>
          <ErrorNotice error={error} onRetry={onRetry} />
          <p className="warning">
            当前资格未确认，不根据上次读取结果声称可派发。
          </p>
        </>
      ) : !profile ? (
        <p role="status">
          {loading ? "正在读取当前工具资格…" : "尚未读取到当前工具资格。"}
        </p>
      ) : (
        <>
          <p className={eligible ? "success" : "warning"}>
            {!matches
              ? "本次固定指纹不匹配当前配置，不能据此新派发。"
              : eligible
                ? "本次读取满足新派发条件；实际领取时仍由服务端重新核验。"
                : "当前未满足新派发条件。"}
          </p>
          {!eligible && matches && (
            <p>{toolReason(profile.reason || "profile_" + profile.state)}</p>
          )}
          <dl>
            <dt>当前节点身份</dt>
            <dd>
              <code>{profile.worker_id}</code>
            </dd>
            <dt>最近上报</dt>
            <dd>{profile.last_seen_at || "尚无正式观察"}</dd>
          </dl>
          {loading && (
            <small className="muted">
              正在更新当前资格；以上保留上次读取事实。
            </small>
          )}
        </>
      )}
      <p className="muted">
        当前资格只描述新的派发，不会改变本次运行已发生的状态、尝试、测试或人工验收。
      </p>
      <a href={href}>查看工具配置与当前原因</a>
    </section>
  );
}
export function ToolBindingStatus({
  project,
  runID,
  node,
}: {
  project: string;
  runID: string;
  node: {
    id: string;
    type: string;
    state: string;
    command?: string[];
    tool_profile_id?: string;
    tool_profile_sha256?: string;
  };
}) {
  const id = node.tool_profile_id || "",
    read = useToolRead<ToolProfile>(
      id ? "/tool-profiles/" + encodeURIComponent(id) + query(project) : "",
    );
  useEffect(() => {
    if (!id) return;
    const timer = setInterval(() => void read.read(), 10000);
    return () => clearInterval(timer);
  }, [id, read.read]);
  if (node.type !== "agent" || !id) return null;
  return (
    <ToolBindingFacts
      project={project}
      state={node.state}
      profile={read.data}
      loading={read.loading}
      error={read.error}
      onRetry={() => void read.read()}
      binding={{
        profile_id: id,
        profile_sha256: node.tool_profile_sha256 || "",
        command: node.command?.length === 1 ? node.command[0] : "",
        run_id: runID,
        node_id: node.id,
      }}
    />
  );
}
