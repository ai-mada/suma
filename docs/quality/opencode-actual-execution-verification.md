# OpenCode 真实执行链路与运行交互验证

日期：2026-10-28；阶段：T-OPENCODE-105；产品：1.0.0。基线 `64ecdd80bdf0d3266cbb6c7e41d39bcd064e777b`。[精简摘要](opencode-actual-execution-summary.json)、[当前架构与 SOP](../architecture/opencode-execution.md)、[完整范围](../delivery/implementation-plan.md)。

## 本次实现

- 修正固定 OpenCode 1.19.4 服务解码后显式增加的 agent options／permission 形态，保持严格配置白名单，不接受额外命令、插件、模型端点或附加 Agent。
- 私有协议只传 `server_parent_stopped`；公开 `server_stopped` 必须同时具有父进程关闭与 Go 进程组／Windows Job 清理证明。取消 ACK 不替代该证明，移除重复 taskkill 依赖和旧私有字段别名。
- 每次串行工具调用使用当前模型轮次的独立 MCP 连接并关闭，避免缓存已取消的上一轮信号。原操作标识、宿主账本与禁止未知效果重试保持。
- 工具错误显示 failed／error，不再被误记为活动完成；进度不决定整次运行终态。
- 简洁运行卡区分等待、执行中、回合完成、平台后处理失败、停止和待核对；原模型只来自准确冻结流程或原回执。证据折叠展示，测试按钮定位同一运行的真实节点，最多20项并说明范围；中英文、键盘原生披露和刷新保持同一来源。

## 真实工具与完整路径

使用[官方固定发布](https://github.com/anomalyco/opencode/releases/tag/v1.19.4)的 Windows x64 baseline 便携包，无系统安装、登录或用户全局配置改动。官方 ZIP SHA256：`21cac96abb146b0f6e770680ea7855fc4f354c7966aab39f5be9859b0df1deac`；211,699,592字节的实际 exe SHA256：`eba9fc042ac622648a66ac5764c5ad41a73244eabcd7a00527d651b7c443efbd`。

真实 Control／SQLite → 固定任务／工具预览与启动 → 实际 Worker → 官方 exe／serve／SDK → 自有 loopback 模型协议响应 → 真实宿主 MCP 读取／CAS写入／固定开发检查 → Go 进程清理 → 宿主 Git 封存及 bundle 上传 → 另一 Worker 下载／导入 → 精确候选 SHA 正式测试。

模型端点为自有确定性 SSE，驱动真实 OpenCode 工具循环；没有调用商业模型、没有证明模型效果或账单。仅六个已批准自定义工具出现在实际请求中，没有附加工具、隐式标题或自动附加模型请求。

成功运行 `fdd87e1256d950e8ea0dec1b67037e92`：四次自有模型协议请求；宿主 generation1、一个文件效果、固定开发检查通过；候选 `5dbe757909dd0aee053c5c4ba916995defa4bfd1`，bundle SHA `0a7293e9c4ac209339bba6d1ee680bb0352b807bcbbd738e582340853ddb2715`。开发执行者 `opencode-actual-developer105`，独立测试执行者 `opencode-independent-tester105`。两个管理员原仓库保持基线，任务仍为 `in_progress`，没有人工验收、合并或部署。

账本保留 partial／unpriced。203输入、43输出是自有端点最后一条响应声明的计数，不是整轮实际模型 Token 或商业结算；未知项没有补零。

## 检查与原始失败

| 检查 | 实际结果 |
| --- | --- |
| SDK | 配置归一化与父进程协议检查的22项通过；此后仅复验4项受影响检查：真实 MCP 客户端跨流信号、串行／清理／原操作、工具错误进度。随后单次构建通过；未把重叠检查累计为全套 |
| Runner | `004532989` 回执：载体、四种父／宿主证明组合、取消、原结果／候选检查43.655秒通过。`005503774`：工具错误不成为完成／终态，0.174秒通过 |
| 实际完整路径 | `010113780` 回执退出0，391.199秒；包括300秒有界界面保留及连接关闭等待，不能当作编码性能基准 |
| Web | 4文件33个不同受影响用例通过；补充冻结范围负例单独复验。类型检查、4653对语言键、生产硬编码0、42.42秒Web构建通过 |
| 宿主与产物 | 受影响Go包 vet、五个Windows amd64程序构建通过；五个实际 `--version` 均为1.0.0；本次未新增依赖、数据库表或公开接口 |

保留原失败：实际预检健康／严格配置通过，但旧重复停止检查未确认；原PID和原始taskkill原因未被保存，因此不推断其具体错误原因，WMI拒绝也不当作没有进程。随后单一宿主清理设计及父进程测试通过，实际成功回执具有 Go 合成的停止证明。

首次完整运行105a读取成功，写工具为 `workspace_fatal`，宿主账本generation0且没有写入操作。核对官方缓存 SQLite 与固定源代码后确认：每轮LLM流的取消信号会随该轮结束失效，原缓存 MCP 客户端错误沿用首轮信号。修正后另建105b验证，未重放105a，原失败记录和数据不改写。默认未开启 opt-in 的 Go 检查只编译／跳过，不算真实执行通过。

## 界面与恢复证据

105a实际执行中核对固定模型、运行状态和原session；105b实际回执核对两节点成功、原模型、消息关联、停止与候选。一次浏览器自动审批超时没有被当作动作完成；重新读取后，原有界服务已经到期。

后续只用原数据库只读备份与一个制品文件建立审阅副本，在同一当前控制面的 maintenance 模式恢复页面，无 Runner 或模型。完成中文／英文、刷新保持原run／node、测试按钮定位另一执行者及同一候选SHA。原数据库前后SHA均为 `a837ef875152ab5668e18b7cc8895e3c09f5db7cb27da642083a496573d479b0`；核对进程路径、创建时间与字节摘要后关闭自有审阅进程。

原始记录在 `.cache/e2e/opencode-105/`，Go命令在 `.cache/mcp019-deps/`；SDK因果与检查摘要在 `packages/opencode-runtime/.cache/phase105*-summary.json`。Git只保存必要源码、设计和精简摘要。

最终Go manifest：`dist/runtime/builds/20261028T012825420Z-9d8a03be/manifest.json`；460个输入SHA `1357da5fb681af729d6d208bc4e03a6b12bcfdb5a7d3c673fba58f7ebf555593`，前后不变。SDK入口SHA `1f8d6d6e85f928d73385628117c2c0697c3901e47c65d185e92fda91fdfc809f`；manifest SHA `da5f6d8c1c257055f80183a3ace71c572af916dd8d71ae108966be92bfbd92d8`；3621项资产／57,547,180字节。

## 仍待完成

本次是Windows本机两个真实Worker与官方工具、自有模型协议的限定闭环；商业模型效果／身份／计费、物理多机、Unix／macOS运行、跨平台安装、完整IDE、远程会话恢复、生产和规模验收继续待办。Web现有大块警告保留，未执行千节点图性能或完整设备矩阵。全平台P0–P7和需求1–22仍全部保留。
