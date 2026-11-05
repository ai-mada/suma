# WorkBuddy 待交付尝试定位验证

T-WORKBUDDY-DELIVERY-TRACE-238 · 速码 1.0.0 · 2026-11-05。 [任务交付合同](../../contracts/task-delivery-chain.md) · [证据摘要](workbuddy-delivery-trace-summary.json)

在任务详情或依赖图中打开“交付与质量”。当前绑定运行若存在 WorkBuddy 代码交付待核对尝试，链接会定位到确切的运行、节点和尝试。当前用户仍需通过运行详情的授权检查。候选、独立测试和人工验收继续分别记录。

| 验证 | 结果 |
| --- | --- |
| Suma HTTP 与 SQLite | 通过正式任务派发、原 Runner 领取、回环供应商响应和持久原结果，建立代码交付待核对状态。原节点排在 22 个节点的通用摘要之外，仍返回准确定位。错误等待原因、状态、类型、协议、未开始或过期尝试及围栏不符均被拒绝。任务改绑后原目标消失。读取未新增供应方调用或 DAG 事件。 |
| Web | 三个定向套件共 17 项通过，覆盖准确定位、伪造或过期目标、任务版本变化、权限失效和导航锁定。Web 类型检查、10,281 对中英文词条及生成合同检查通过。 |
| 窄屏浏览器 | 390×844 Chrome 使用实际组件和保存的 Suma HTTP 响应。中英文切换、403 后隐藏并重试、键盘打开原运行路由均通过。页面无横向溢出或脚本错误，只有三次交付接口读取。运行目标按原 URL 验证，夹具未加载目标运行详情。 |

交互参考 Multica 从任务入口携带准确运行身份的做法，复用速码既有运行路由。该阶段验证了定位和待核对状态。真实供应方 Git 交付、原云来源到本机候选的可信关联及同提交独立测试仍待完成。 [Multica 入口](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/issue-agent-header-chip.tsx#L232-L246) · [Multica 跳转](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/issue-detail.tsx#L2050-L2124)
