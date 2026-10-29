# 开发客户端接入

阶段：T-EXPERIENCE-133。官方字段核对日期：2026-10-29。配置导出复用[项目 MCP](project-mcp.md)，不安装客户端、不登录厂商账号，也不为客户端创建执行档案。当前服务端授权、工具目录、原操作恢复和草稿权限保持原合同。

在项目「工具管理 → 接入向导 → 接入开发客户端」选择客户端。打开面板才读取当前项目的可信地址及原用户／组织；复制或下载的片段不含令牌。每个条目使用 `suma_<project_id>`。合并时保留已有配置，不用下载片段覆盖整份文件。客户端账号、组织或项目变化后重新获取片段。连接成功必须在实际客户端中另行核实。

| 客户端 | 本阶段交付 | 项目文件／认证字段 | 官方依据 |
| --- | --- | --- | --- |
| Cursor | 可复制、下载 JSON 片段 | `.cursor/mcp.json`；`mcpServers`、`url`、`headers.Authorization = "Bearer ${env:SUMA_MCP_TOKEN}"` | [MCP 与配置插值](https://cursor.com/docs/mcp) |
| Claude Code | 可复制、下载 JSON 片段 | `.mcp.json`；`mcpServers`、`type:"http"`、`url`、`headers.Authorization = "Bearer ${SUMA_MCP_TOKEN}"` | [项目范围与环境变量](https://code.claude.com/docs/en/mcp) |
| Codex | 可复制、下载 TOML 片段 | `.codex/config.toml`，需可信项目；`mcp_servers`、`url`、`bearer_token_env_var`、`http_headers` | [MCP 配置](https://learn.chatgpt.com/docs/extend/mcp?surface=cli) |
| OpenCode | 可复制、下载 JSON 片段 | `opencode.json`；`mcp`、`type:"remote"`、`oauth:false`、`url`、`headers.Authorization = "Bearer {env:SUMA_MCP_TOKEN}"` | [远程 MCP 与 API key](https://opencode.ai/docs/mcp-servers/)、[项目配置](https://opencode.ai/docs/config/#project) |
| Qoder | 具体手动指南 | IDE 的 MCP → My Servers → Add 支持 Streamable HTTP；CLI 文档另列 HTTP `headers`。尚未确认远程头的环境变量插值，不导出文件 | [IDE 指南](https://docs.qoder.com/user-guide/chat/model-context-protocol)、[CLI 字段参考](https://docs.qoder.com/cli/mcp-reference) |
| Zed | 具体手动指南 | Settings → AI → MCP Servers → Add Remote Server；官方示例为 `context_servers` 中的 `url` 和字面 `headers`。尚未确认本阶段需要的项目范围环境令牌引用，不导出文件 | [Zed MCP](https://zed.dev/docs/ai/mcp) |
| WorkBuddy | 具体手动指南 | 官方 connector 包需凭据表单与明确客户端版本，不等同 CodeBuddy CLI。当前未生成或安装包 | [连接器、Token 认证及版本字段](https://open.workbuddy.cn/en/docs/connector) |
| Harness | 执行方向说明 | 现有 Octop Harness / ACP 是受控执行运行时；没有名为 Harness 的统一 MCP 客户端 JSON。其他同名产品需指定实际实现再核实 | [Octop 官方项目](https://github.com/TencentCloud/Octop)、[现有受控运行时指南](octop.md) |

由有权项目管理员签发专用、可撤销、有到期时间的 Agent 或 API 令牌。只在客户端进程环境设置 `SUMA_MCP_TOKEN`；片段中的环境引用必须保留字面值，不能替换为真实秘密后提交、截图或发给模型。Agent 令牌仍只有原四项只读工具；API 工具仍依据当前真实权限。不要使用 Runner 令牌、浏览器 Cookie 或厂商模型密钥。

核对 `X-Suma-Expected-User`、`X-Suma-Expected-Organization` 和 `X-Suma-Language`。这些静态字段防止误用原身份，不提供授权。地址只来自服务端可信配置：远程 HTTPS，明确同主机时可使用回环 HTTP。客户端位于容器或另一台机器时，`127.0.0.1` 指向它自身；请管理员配置真实可达的 `SUMA_PUBLIC_URL`，不要手改片段猜测目的地。尚未配置或路径不符时，面板关闭导出。

合并文件后重新打开客户端，审阅项目的信任与 MCP 权限，实际列出工具并读取项目概览；CLI／IDE 的授权与设置界面以安装版本为准。写草稿必须使用稳定原 `idempotency_key`，结果未知时恢复同一操作；设计写入继续带当前版本。回到速码核对真实草稿和版本，再按正式评审／派发继续。平台不会因配置下载而显示「已连接」。

本阶段只增加无秘密导出、手动指南和边界说明。没有新增 OAuth、SDK 调度器、任意外部命令、厂商安装器、自动配置写入或商业模型调用；也没有以官方文档核对代替真实厂商客户端验收。实际类型检查、组件检查和浏览器证据由本阶段集成报告记录。
