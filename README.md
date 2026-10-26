# 速码 suma

**速码一栈式智能研发平台**：连接需求、设计、任务、智能体执行、测试验收与问题回流。支持浏览器和本地桌面。

当前是可运行的开发版本。功能与验证范围见 [交付状态](docs/delivery/status.md)，完整目标见 [实施计划](docs/delivery/implementation-plan.md)。

## 本轮进展：固定失败证据的智能分诊

T-P4-030／1.0.0 已完成限定本地验证：准确失败证据经专属只读模型诊断，人工编辑修复建议并明确采用为需求草稿，保留原运行、模型原件与来源回链。真实同机 Control／Runner／DAG／账本配合一次自有本地固定模型响应，记录 40 Token、估算 57 CNY micros；这不代表厂商模型质量或商业账单。最终中英冷回读及主预览前向更新完成，原 177 表完整结构与行保持，仅加三张空表及一个索引至 180 表。见[限定验证](docs/quality/failure-diagnosis-verification.md)、[设计与 AI SOP](docs/architecture/failure-diagnosis.md)、[English](docs/en/failure-diagnosis.md)。完整 P0–P7 持续实施。

下一步 T-P4-031 待实现：多源已验收候选合成，对精确合成结果独立测试，再明确固定下游代码基线交接；业务依赖验收不自动合并或继承代码。见[实施待办](docs/delivery/implementation-plan.md)。

## 历史进展：专家团队依赖编排

T-P4-029 团队任务支持固定依赖、逐项进度、暂停／恢复／停止、未开始项修订及只读历史。服务端在准确前置运行正式验收后自动派发后继，CLI 与中英文界面共用合同。实际同机双 Runner 完成两项开发、8 项独立正式测试与两次明确验收，029／1.0.0 主预览更新作为历史证据保留。见[功能与实际边界](docs/quality/expert-team-orchestration-verification.md)、[设计与AI SOP](docs/architecture/expert-team-orchestration.md)、[CLI](contracts/expert-team-orchestration-cli.md)。完整平台持续开发。

## 历史进展：精确尝试的工具历史

T-P4-012 已实现按项目、运行、节点和 attempt 读取持久工具事实。运行详情中的“历史尝试”可分页查看原始正文和当前语言说明；刷新保持选中的尝试，翻页保持同一个读取窗口。历史页默认 50 条，可选 5／25／50／100 条，不受当前实时工具窗口 40 行限制。读取逐次核对当前权限，原文和执行依据不因显示语言变化。

本阶段已完成本地验证：分页原文与数据库一致，纯读取没有改变执行或业务状态，验证服务已关闭，主预览已更新且原数据保持。本机原生 HTTP 429 重试与独立合成协议大页验证分开记录，不代表厂商模型效果或任务验收。见[架构与操作边界](docs/architecture/attempt-tool-history.md)、[专项验证](docs/quality/attempt-tool-history-verification.md)和[精简摘要](docs/quality/attempt-tool-history-summary.json)。

## 本地运行

要求 Go 1.26+（构建固定 1.26.10）、Node.js 22+、pnpm 11。可通过 `SUMA_GO_EXE` 指向私有工具链；构建脚本不修改系统环境。

```powershell
pnpm install
pnpm build
go run ./cmd/suma-control
```

打开 http://127.0.0.1:4310 ，首次创建组织、项目和管理员。数据保存到 `.suma/`。默认只监听本机，密码至少 12 个字符。

开发前端：`pnpm dev`；控制面继续监听 4310，前端 5173 代理 API。编辑项目文件前，设置 `SUMA_WORKSPACE_ROOT` 为明确授权的代码根目录，在 IDE 中绑定项目工作区。

执行节点需要从设置中创建凭证，并明确配置可执行命令。参见 [Runner 与工具](docs/integrations/tools.md)。原生智能体参见 [运行时](docs/agents/runtime.md)。

```powershell
go test ./...
pnpm typecheck
pnpm test
pnpm build
pnpm check:docs
```

浏览器、桌面、数据库、模型账户和规模验证分别记录；未配置的服务在界面中显示不可用。

## 文档与交付

- [设计文档总览](docs/README.md)
- [系统架构](docs/architecture/overview.md)
- [DAG 调度](docs/scheduler/engine.md)
- [执行尝试与持久工具历史](docs/architecture/attempt-tool-history.md)
- [产品与用户旅程](docs/product/prd.md)
- [部署与恢复](docs/deployment/server.md)
- [真实制品与本机发布](docs/deployment/release-control.md)
- [四程序跨平台运行包](docs/deployment/release-runtime-bundles.md)
- [当前验证报告](docs/quality/verification.md)

代码在本仓库开发；运行数据、凭证、依赖和测试缓存均不进入 Git。
