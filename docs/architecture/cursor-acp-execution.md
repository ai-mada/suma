# Cursor ACP 架构与 AI SOP

T-CURSOR-ACP-189；suma 1.0.0。权威为[合同](../../contracts/cursor-acp-execution.md)，结果范围见[验证报告](../quality/cursor-acp-execution-verification.md)。

## 复用边界

公开目录为 `cursor-cli`，Profile 为 `adapter=cursor`、`protocol=acp-stdio-v1`。复用 ACP Capability／Evidence 和节点 `acp_execution`；Runtime 必须准确等于 Adapter。Cursor 不是 Qoder 别名，不使用新的 JSONL 协议、ModelExecution 或 Codex selection。

```mermaid
flowchart LR
    U[明确检测与选择模型] --> H[Go 私有包核验与配置摘要]
    H --> I[原 Runner 库存与明确启用]
    I --> P[任务／讨论／专家 Preview]
    P --> S[原 Start 与普通／Pool Claim]
    S --> A[独立工作树与 ACP SDK]
    A --> Q[原真人一次权限]
    Q --> C[准确候选封存]
    C --> T[不同 Worker 同提交测试／JUnit]
    T --> R[当前有权账号明确 Review]
```

平台生产分支直接复用成熟 ACP 准入：当前 Profile、库存、固定模型、配置 SHA、原任务与流程版本、路由／Grant、lease、attempt／fence和预算分别复核。Preview、Start、Claim／原子记账均拒绝不具备硬控制能力的 enforce 执行。固定模型通过 `fixed_by_tool` 投影，无需另建业务表、HTTP 接口或派发器。

## 本地深模块

`BuildCursorACPCommand`由 Go 生成固定包入口和参数，包根、完整文件闭包、实际宿主与配置摘要留在私有本机配置。拒绝链接／重解析路径、依赖变化和不受支持入口；不把编辑器或任意 shell launcher 当作可执行证明。

执行使用新的 HOME、配置、缓存和临时目录，只从明确本地引用映射 `CURSOR_API_KEY`。工作树及其祖先中的可执行 Cursor／Claude 设置、MCP、hooks，以及固定企业 hooks 路径必须通过当前拒绝门；发现冲突不自动删除用户配置。普通规则与指令文件不因此全部禁用。固定 sandbox 参数和这些检查不构成整个 CLI 的 OS 隔离证明。

原 `acp-go-sdk 0.13.5`负责会话交互，严格帧层先验证范围和形状。准确 `configOptions`及 `session/set_config_option` ACK 后才能 Prompt；Cursor 的附加模型信息不能替代该 ACK。安全通知只是活动事实，不创建任务、不批准权限、不结束质量门。未支持的阻塞扩展明确失败；不默认同意 `ask_question`、`create_plan`或持久权限。

原 Client／Journal／outbox 保持完整 Assignment、稳定操作键及来源。未知原生副作用不得重发 Prompt；HTTP 回执恢复与实际新工作分开。取消请求、RPC 终态、进程 Wait／树停止、候选与测试证据均独立记录。用量只能 observed，不能冒充完整计费覆盖。

## AI 实施与排障 SOP

1. 读当前状态与本合同，按共享类型、Runtime、平台和 Web 分工；优先复用 ACP 深模块，不加 Cursor 专用调度链。
2. 先固定真实包、模型 ID、版本与密钥引用；包或配置变化后撤下旧观察，拒绝静默模型、登录或参数回退。
3. 检查当前授权、原配置和工作区来源；普通／Pool 使用原身份和完整范围，不由 renderer 正文授予能力。
4. 按原请求准确显示一次权限；未知状态保留原操作，不换 key 重跑模型或伪造允许。
5. 收齐原结果与停止事实后，沿原候选、同提交独测、JUnit和 Review；模型 ACK 不授予质量或验收。
6. 只验证新增接缝与直接受影响原组。分别保留厂商无账号探测、受控协议、真实 Git／HTTP／SQLite和 GUI 证据；不把其中一层推广为全部平台通过。

本轮厂商包只证实版本／帮助／初始化及未认证拒绝；无厂商 Prompt。受控完整链使用非厂商原生宿主，实际独立 Node／JUnit与隔离账号 Review 已闭合；源码跨 OS 分支不等于所有系统或物理多机已实机验收。
