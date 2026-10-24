import type { WNode } from "./api";
import { ErrorNotice, Field } from "./ui";
import { toolBinding, type ToolProfile } from "./tool-model";
import { useToolList } from "./tool-hooks";
export function ToolNodeFields({
  project,
  node,
  onChange,
}: {
  project: string;
  node: WNode;
  onChange: (patch: Partial<WNode>) => void;
}) {
  const profiles = useToolList<ToolProfile>(project, "tool-profiles"),
    id =
      typeof node.config?.tool_profile_id === "string"
        ? node.config.tool_profile_id
        : "",
    current = profiles.items?.find((item) => item.id === id);
  return (
    <section className="tool-binding">
      <Field
        label="固定外部工具配置"
        hint="命令与配置指纹一起固定。保存流程不会启用工具，实际派发仍核对执行节点身份、权限及新鲜观察。"
      >
        <select
          value={id}
          onChange={(event) =>
            onChange(
              toolBinding(
                node,
                event.target.value
                  ? profiles.items?.find(
                      (item) => item.id === event.target.value,
                    )
                  : undefined,
              ),
            )
          }
        >
          <option value="">不绑定；保留手工命令配置</option>
          {id && !current && (
            <option value={id}>{id} · 当前绑定，未在已加载窗口内</option>
          )}
          {profiles.items?.map((item) => (
            <option
              value={item.id}
              key={item.id}
              disabled={item.state === "revoked"}
            >
              {item.name} · {item.profile.command_id}
              {item.available ? "" : " · 当前不可派发"}
            </option>
          ))}
        </select>
      </Field>
      {profiles.error && (
        <ErrorNotice
          error={profiles.error}
          onRetry={() => void profiles.read(profiles.error_action === "more")}
        />
      )}{" "}
      {profiles.next_cursor && (
        <button
          type="button"
          disabled={profiles.loading}
          onClick={() => void profiles.read(true)}
        >
          加载更多真实工具配置
        </button>
      )}
      {id && (
        <small>
          固定配置 <code>{id}</code> ·{" "}
          <code>
            {String(node.config?.tool_profile_sha256 || "缺少指纹，不能发布")}
          </code>
        </small>
      )}
      {current && !current.available && (
        <p className="warning">
          该配置当前未满足派发条件。可保留编排草稿，启用与实时观察匹配后才会领取；不会消耗尝试来探测。
        </p>
      )}
      <a
        href={
          "?" +
          new URLSearchParams({
            project,
            view: "settings",
            sub: "tools",
            record: id,
          })
        }
      >
        查看执行工具与当前原因
      </a>
    </section>
  );
}
