# 开发工具 MCP 真实只读连接检查 / Developer MCP connection check

状态：T-P4-036，命令已实现并通过限定本地验证。该检查验证单次 CLI 到速码控制面 MCP 的真实调用；远程 Runner 工作区、IDE 常驻连接和厂商接入仍未完成。

使用已签发且当前有效的项目 API 或 Agent 凭证，将其放在本次进程的 `SUMA_TOKEN` 中。命令不签发凭证，不更改 IDE 配置，不读取本机凭证文件。所有全局参数放在命令前：

```powershell
suma --control http://127.0.0.1:4310 --project PROJECT_ID --language zh-CN developer check
suma --control https://suma.example --project PROJECT_ID --language en-US --timeout 15s developer check
```

检查固定使用 `CONTROL/api/v1/mcp/projects/PROJECT_ID`，通过项目现有当前授权完成官方 Go MCP SDK 的 `initialize`、工具分页列表和一次 `suma_project_overview` 只读调用。只有目录明确将该工具声明为只读且结果中的项目 ID 与原项目相同，才输出成功 JSON。目录中的其他工具仅表示本次实际返回的声明；不会调用写工具或验证它们的写权限。

报告包含 `checked_at`、`scope=single_cli_invocation`、服务身份／版本、实际协商协议、当前工具名称／只读声明，以及概览结构化结果的字节数和 SHA256。摘要针对解析后重新编码的结构化 JSON，不是原始 HTTP 字节。报告不输出概览原文、模型信息、服务器指令、工具 schemas 或凭证；不会写回“已连接”状态。

命令使用当前服务支持的 MCP `2025-11-25` 初始化协议，完整检查受 `--timeout` 限制；不重连、不打开常驻 SSE，不跟随重定向。只允许回环 HTTP 或远端 HTTPS，响应每次最多 1 MiB、目录最多 10 页／200 工具，概览最多 128 KiB；重复工具／循环游标会明确失败。失败时不输出成功报告，只在 stderr 返回封闭中英文错误；退出码 0 成功、1 服务／协议／范围失败、2 用法错误。

## English

Provide an existing, currently valid project API or Agent credential in the current process's `SUMA_TOKEN`, then run either command above. Remote control origins require HTTPS; local loopback HTTP is supported. The command never creates credentials or changes IDE configuration.

The official Go MCP SDK performs a real initialization, bounded tool listing and one fixed read-only project overview call against the original control origin and project. Success requires a read-only declaration and an overview result matching that project. Other listed tools are declarations observed during this check, not proof that their writes work.

The JSON report records this invocation's `checked_at`, server/protocol facts, current declarations and a SHA256 of the re-encoded structured read result. It omits raw project data and secrets. No persistent connection status is changed. Redirects, unencrypted remote origins, oversized output, invalid pagination and failed or cross-project reads fail the check. This is a control-plane MCP diagnostic; remote workspace browsing, persistent IDE connectivity and physical multi-host acceptance remain pending. See the [outbound broker design](../architecture/remote-workspace-broker.md).
