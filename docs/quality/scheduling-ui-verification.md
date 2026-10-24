# 执行调度工作台前端验证

状态：T-P4-005 前端自动化及正式构建通过；真实后端和最终浏览器旅程由主集成 QA 独立记录。日期：2026-10-24。负责人：UI 实现代理。

关联：[冻结合同](../../contracts/scheduling-kernel.md)、[交互设计](../design/scheduling-workspace.md)。

## 交付与修复

项目设置支持执行策略草稿、固定预览、显式确认与未知操作恢复；执行记录提供正式就绪队列、当前多重阻塞、持久尝试分页和精确深链。配置额度、机器资源测量、当前派发资格、过去执行及业务验收分别表达。已知零、缺失、无限和部分用量不会互相替代；不显示未实现的共享资源池、跨租户权重或推算进度。

界面按真实浏览器反馈收紧策略摘要、资源明细及中文阻塞标题；主要编辑入口前置。确认成功后同一按钮转为唯一“继续调整策略”，维持键盘链且不再次提交。运行详情两个兄弟组件使用不同 key 命名空间，修复定时刷新遗留旧面板。

图的鼠标点击与 ReactFlow 正式 `onSelectionChange` 共用单节点选择入口。只同步非空且仅一个节点的选择，重复同 ID 通知去重；空／多选通知不会清除外部详情。URL 恢复反向更新图选中，切换节点按现有 App 规则清理旧 attempt；窄屏完整节点列表继续可用。主集成已在本次最终构建实际验证图上 Enter／Space 和精确 attempt 的键盘进入。

源码入口：[WorkflowView](../../apps/web/src/Workflow.tsx)、[策略工作区](../../apps/web/src/SchedulingWorkspace.tsx)、[节点事实](../../apps/web/src/NodeSchedulingFacts.tsx)、[展示事实](../../apps/web/src/SchedulingFacts.tsx)、[就绪队列](../../apps/web/src/SchedulingQueue.tsx)。

## 最终检查

| 检查 | 实际结果 |
|---|---|
| 完整前端回归 | **244项、24个文件通过** |
| 真实 Workflow 集成 | **4项通过**；含连续9轮诊断／快照、切节点、正式单节点选择回调、真实 App 的节点／attempt深链与返回 |
| 类型 | 退出0；直接使用169模型统一合同 |
| 格式与行尾 | 新增集成测试格式检查、`apps/web`标准差异空白检查退出0；两份触达源码均为LF |
| 正式网页构建 | 退出0，Vite **38.06秒**；保留既有大块提示，未据此宣称大图性能达标 |

最终入口：`index-jI-XtZ8U.js`；样式：`index-CNpu4DxB.css`；工作流块：`Workflow-DzzzFIDL.js`。此前242项的 `index-kLa04S5Q.js` 是键盘同步修复之前的切面。

## 实际浏览器局部结果

主集成在最终 `index-jI-XtZ8U.js` 页面，从 verify-parallel／attempt1 用图节点 Enter 切到 api，URL 更新且旧 attempt 清理；对 ui 使用 Space 后 URL 和真实详情一起切换。历史尝试链接 Enter 打开已保存的 Runner 回执并显示实际 exit0。持续观察 **101669毫秒**，自动快照下 ui／attempt1 保持，调度事实、尝试历史及所选尝试详情始终各1份；刷新仍恢复同一回执。最终浏览器警告／错误记录为空。此前177272毫秒属于 `index-kLa04S5Q.js`，两次观察分别记录。四宽度局部结果见[阶段验收](scheduling-verification.md)，不能据此声称完整宽度旅程通过。

## 证据及覆盖边界

新增键盘回归先得到2项真实失败：正式选择回调没有接线；修复后目标4项通过。真实 App 用例验证切节点清旧 attempt、打开新精确 attempt，空／多选／同ID通知和3轮 SSE／定时快照不会重复改写历史或丢失选择，浏览器返回恢复原节点及 attempt。首轮类型检查发现可空节点状态类型不足，原失败日志保留，修正后重跑完整244项、类型与构建通过。

集成测试保留实际 WorkflowView、NodeSchedulingFacts、App、SSE读取和快照队列，仅替代 ReactFlow／ELK 布局；调用正式选择回调验证消费接线，不伪称 JSDOM 能证明第三方组件的原生键盘行为或几何。早期队列路由用例使用 Workflow 替身，只证明接线，不等于完整父树验收。所有合成 HTTP 与模拟时钟结论均独立于真实后端吞吐、长时间停留及四宽度走查。

本地原始证据位于 `.cache/e2e/scheduling-stage/ui-keyboard-selection-*`，最终来源和资产摘要在 `.cache/e2e/scheduling-stage/ui-keyboard-selection-verification.json`。历史原件仍保留在 `docs/quality/evidence/scheduling-ui/`、`docs/quality/evidence/scheduling-ui-final/`；这些本地日志、截图、快照和清单不进入本次 Git 提交，也不是部署依赖。未覆盖或改写原227／231／237／240／242切面。

后端Go606、24个交付程序、真实 cap0→cap2、并行领取、独立测试、人工审批、90秒以上观察及最终浏览器键盘验收由主 QA 汇总；本页不替代该报告。数据库集群、生产发布与规模公平性分别验收。
