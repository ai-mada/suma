# 派发计划重开与回执核对

T-TASK-DISPATCH-ENVELOPE-236 · 速码 1.0.0 · 2026-11-05。 [派发合同](../../contracts/task-dispatch-options.md) · [证据摘要](task-dispatch-envelope-summary.json)

直接派发预览、重开计划和原启动回执共用固定依据检查。预览需要保留原任务修订、发布流程、开发节点的工具绑定和用户选入的知识修订。重开时，以当前读取返回的原计划为准。错配或拒绝访问时，界面隐藏计划内容和启动入口。重试成功后，启动需要再次明确确认。

| 验证 | 结果 |
| --- | --- |
| 定向 Web 回归 | 8 个测试文件共 30 项通过。修复前，空的任务上下文可被识别为普通预览回执。同项目且同计划 ID 的其他任务快照也会显示。修复后均拒绝。覆盖历史任务修订、跨组织回执、固定 Agent 配置、保留原操作标识、原计划从 `ready` 到 `started` 以及 Codex、OpenCode、Octop 回执。 |
| 知识选择 | 两条旧用例改为使用发布知识选择器。已验证固定正文、来源和链接。发布修订变化后，用户需移除原选择，再明确选入新修订。 |
| 浏览器 | 390×844 Chrome 使用生产派发组件和受控 HTTP 响应。覆盖错配隐藏、中英文反馈、键盘重试、访问拒绝和返回原计划后的重新确认。浏览器过程只有读取请求。首次夹具使用了不同的中文按钮名称，修正选择器后复用同一构建。 |
| 类型和国际化 | Web TypeScript 检查通过，10,237 对语言词条通过。 |

Multica 的历史运行操作先按准确运行定位，再核对当前调用权，其无效任务路由会显示明确的找不到状态。速码保留原有独立启动、保留运行和权威服务核验。参考本地 Multica 固定提交 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e` 的行为。 [历史运行处理](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/handler/task_lifecycle.go#L185-L253) · [任务直链](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/issue-detail-route.tsx#L71-L84)

本次证据覆盖直接派发浏览器端和回执行为。真实厂商执行、多机、Unix 信号收尾和 WorkBuddy 云候选交付仍需独立验收。
