# 资源池节点本机诊断验证

T-POOL-DIAGNOSTIC-GUIDE-233，速码 1.0.0，2026-11-05。 [诊断合同](../../contracts/runner-journal-inspection.md#从资源池节点打开诊断) · [交互设计](../design/execution-pools.md#节点本机诊断) · [证据摘要](pool-diagnostic-guide-summary.json)

资源池所有方可从当前节点行打开“本机诊断”。按目标 Shell 和原状态目录生成总览命令。操作者可把总览 JSON 带回当前弹窗，查看异常槽位并复制对应原日志命令。输入只在本页处理。界面提示摘要来源未经验证，业务与机器状态由控制面记录确定。

| 范围 | 结果 |
| --- | --- |
| 模型与相邻回归 | 5 项诊断模型和 10 项资源池恢复检查通过。覆盖字段白名单、重复键、异常计数、槽位顺序、部分摘要和 PowerShell／Bash 路径引用。 |
| 浏览器 | Chrome 390×844 实际渲染 `PoolWorkspace`、节点行和弹窗，节点列表使用隔离只读接口。输入上一阶段 Windows CLI 的原停止摘要和部分读取摘要，验证命令复制、异常槽位显示、非法字段拒绝、焦点与组织切换。粘贴与预览发出 0 个请求，无横向溢出或浏览器错误。 |
| 类型与双语 | 资源池入口、诊断与相邻测试的定向 TypeScript 检查通过。中英文 10,174 对词条检查通过，生产硬编码提示 0。 |

工作区初次类型检查暴露本阶段路径校验的类型收窄错误，以及另一在途功能的类型错误。本阶段路径校验已修正并重新运行定向类型、模型和浏览器验证。截图和日志保存在 `.cache/pool-diagnostic-guide-233/`。

参考 Multica 从具体机器组织命令、独立显示在线事实的做法。速码沿用当前资源池节点和本机诊断合同。 [Multica 固定版本命令区](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/packages/views/runtimes/components/machine-cli-section.tsx#L6-L37) · [离线排查指南](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/apps/docs/content/docs/daemon-runtimes.mdx#L301-L309)

多主机运维、Linux／macOS 实机、真实厂商和平台全流程验收仍在原计划中。 [完整实施计划](../delivery/implementation-plan.md)
